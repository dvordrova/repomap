package surfacediscovery

import (
	"encoding/json"
	"go/constant"
	"go/token"
	"go/types"
	"sort"

	"github.com/dvordrova/repomap/internal/sourcevalue"
	"golang.org/x/tools/go/ssa"
)

func (a *analyzer) valueAnchor(pos token.Pos) *sourcevalue.Anchor {
	if !pos.IsValid() {
		return nil
	}
	loc := a.location(pos)
	if !validRepositoryDirectCallLocation(loc) {
		return nil
	}
	return &sourcevalue.Anchor{Path: loc.Path, Line: loc.Line, Column: loc.Column}
}

// sourceValue retains only compiler-observed expression structure. Unknown
// loads, calls and cycles remain explicit frontiers; no method-name heuristics
// or execution of repository code participates in this capture.
func (a *analyzer) sourceValue(value ssa.Value, active map[ssa.Value]bool, observed ...ssa.Instruction) *sourcevalue.Value {
	if value == nil {
		return &sourcevalue.Value{Kind: "unknown"}
	}
	unknown := &sourcevalue.Value{Kind: "unknown", Anchor: a.valueAnchor(value.Pos())}
	readAt, _ := value.(ssa.Instruction)
	if len(observed) > 0 {
		readAt = observed[0]
	}
	if active[value] {
		unknown.Text = "cyclic value"
		return unknown
	}
	active[value] = true
	defer delete(active, value)
	switch v := value.(type) {
	case *ssa.Const:
		if v.Value != nil && v.Value.Kind() == constant.String {
			return &sourcevalue.Value{Kind: "literal", Text: constant.StringVal(v.Value)}
		}
	case *ssa.Parameter:
		parent := v.Parent()
		if parent == nil {
			return unknown
		}
		offset := 0
		if parent.Signature.Recv() != nil {
			offset = 1
		}
		for i, parameter := range parent.Params {
			if parameter != v {
				continue
			}
			pos := parent.Pos()
			if parent.Syntax() != nil {
				pos = parent.Syntax().Pos()
			}
			if owner := a.valueAnchor(pos); owner != nil {
				if i < offset {
					return &sourcevalue.Value{Kind: "receiver", Text: v.Name(), Owner: owner, Anchor: a.valueAnchor(v.Pos())}
				}
				return &sourcevalue.Value{Kind: "parameter", Text: v.Name(), Position: i - offset + 1, Owner: owner, Anchor: a.valueAnchor(v.Pos())}
			}
		}
		unknown.Text = v.Name()
	case *ssa.FreeVar:
		// A closure's value comes from its actual MakeClosure binding, not
		// from a same-named variable found elsewhere in the package.
		fn := v.Parent()
		if fn == nil || fn.Parent() == nil {
			return unknown
		}
		index := -1
		for i, free := range fn.FreeVars {
			if free == v {
				index = i
				break
			}
		}
		var values []*sourcevalue.Value
		for _, block := range fn.Parent().Blocks {
			for _, instruction := range block.Instrs {
				if closure, ok := instruction.(*ssa.MakeClosure); ok && closure.Fn == fn && index >= 0 && index < len(closure.Bindings) {
					values = append(values, a.sourceValue(closure.Bindings[index], active, closure))
				}
			}
		}
		if len(values) > 0 {
			return sourceAlternatives(values, unknown.Anchor)
		}
	case *ssa.BinOp:
		if v.Op == token.ADD && types.Identical(v.Type().Underlying(), types.Typ[types.String]) {
			return &sourcevalue.Value{Kind: "concat", Anchor: a.valueAnchor(v.Pos()), Parts: []sourcevalue.Value{*a.sourceValue(v.X, active, readAt), *a.sourceValue(v.Y, active, readAt)}}
		}
	case *ssa.Call:
		if anchor := a.valueAnchor(v.Pos()); anchor != nil {
			name := ""
			if fn := v.Common().StaticCallee(); fn != nil {
				name = fn.Name()
			} else if v.Common().Method != nil {
				name = v.Common().Method.Name()
			}
			return &sourcevalue.Value{Kind: "call_result", Text: name, Anchor: anchor}
		}
	case *ssa.Extract:
		return a.sourceValue(v.Tuple, active, readAt)
	case *ssa.ChangeType:
		return a.sourceValue(v.X, active, readAt)
	case *ssa.Convert:
		return a.sourceValue(v.X, active, readAt)
	case *ssa.MakeInterface:
		return a.sourceValue(v.X, active, readAt)
	case *ssa.ChangeInterface:
		return a.sourceValue(v.X, active, readAt)
	case *ssa.UnOp:
		if v.Op == token.MUL {
			return a.sourceValue(v.X, active, v)
		}
	case *ssa.Phi:
		// A control-flow join is not a single source expression. Keep its
		// frontier instead of expanding an exponential set of runtime paths.
		unknown.Text = "conditional value"
	case *ssa.Alloc:
		var stores []*ssa.Store
		fields := make(map[int][]*ssa.Store)
		if refs := v.Referrers(); refs != nil {
			for _, ref := range *refs {
				if store, ok := ref.(*ssa.Store); ok && store.Addr == v {
					stores = append(stores, store)
				}
				if field, ok := ref.(*ssa.FieldAddr); ok && field.X == v && field.Referrers() != nil {
					for _, reference := range *field.Referrers() {
						if store, ok := reference.(*ssa.Store); ok && store.Addr == field {
							fields[field.Field] = append(fields[field.Field], store)
						}
					}
				}
			}
		}
		if len(stores) > 0 {
			return a.initialStoreValue(v, stores, readAt, active)
		}
		if len(fields) > 0 {
			if pointer, ok := v.Type().Underlying().(*types.Pointer); ok {
				if structure, ok := pointer.Elem().Underlying().(*types.Struct); ok {
					result := &sourcevalue.Value{Kind: "record", Anchor: unknown.Anchor}
					for i := 0; i < structure.NumFields(); i++ {
						if stores := fields[i]; len(stores) > 0 {
							result.Parts = append(result.Parts, sourcevalue.Value{Kind: "field_value", Text: structure.Field(i).Name(), Parts: []sourcevalue.Value{*a.initialStoreValue(v, stores, readAt, active)}})
						}
					}
					return result
				}
			}
		}
	case *ssa.FieldAddr:
		return a.fieldSourceValue(v.X, v.Field, v.Pos(), active, readAt)
	case *ssa.Field:
		return a.fieldSourceValue(v.X, v.Field, v.Pos(), active, readAt)
	case *ssa.Lookup:
		return &sourcevalue.Value{Kind: "index", Anchor: a.valueAnchor(v.Pos()), Parts: []sourcevalue.Value{*a.sourceValue(v.X, active, readAt), *a.sourceValue(v.Index, active, readAt)}}
	case *ssa.Index:
		return &sourcevalue.Value{Kind: "index", Anchor: a.valueAnchor(v.Pos()), Parts: []sourcevalue.Value{*a.sourceValue(v.X, active, readAt), *a.sourceValue(v.Index, active, readAt)}}
	}
	return unknown
}

func (a *analyzer) fieldSourceValue(receiver ssa.Value, field int, pos token.Pos, active map[ssa.Value]bool, readAt ssa.Instruction) *sourcevalue.Value {
	typ := receiver.Type()
	if pointer, ok := typ.Underlying().(*types.Pointer); ok {
		typ = pointer.Elem()
	}
	if fields, ok := typ.Underlying().(*types.Struct); ok && field < fields.NumFields() {
		return &sourcevalue.Value{Kind: "field", Text: fields.Field(field).Name(), Anchor: a.valueAnchor(pos), Parts: []sourcevalue.Value{*a.sourceValue(receiver, active, readAt)}}
	}
	return &sourcevalue.Value{Kind: "unknown", Anchor: a.valueAnchor(pos)}
}

// This is an initializer observation, not reaching-definitions analysis. A
// multi-write cell or a write outside the allocation block stays unresolved.
// The consuming load/call/return bounds source order; Alloc.Pos alone precedes
// even the constructor's own initialization and is not a read location.
func (a *analyzer) initialStoreValue(alloc *ssa.Alloc, stores []*ssa.Store, readAt ssa.Instruction, active map[ssa.Value]bool) *sourcevalue.Value {
	unknown := &sourcevalue.Value{Kind: "unknown", Text: "mutable value", Anchor: a.valueAnchor(alloc.Pos())}
	if len(stores) != 1 {
		unknown.Text = "multiple writes"
		return unknown
	}
	store := stores[0]
	if store.Block() != alloc.Block() || !sourceStoreBeforeRead(store, readAt) {
		unknown.Text = "write not established before read"
		return unknown
	}
	return a.sourceValue(store.Val, active, store)
}

func sourceStoreBeforeRead(store *ssa.Store, read ssa.Instruction) bool {
	if read == nil || read.Parent() != store.Parent() || read.Block() == nil || store.Block() == nil {
		return false
	}
	if read.Block() != store.Block() {
		return store.Block().Dominates(read.Block())
	}
	seen := false
	for _, instruction := range read.Block().Instrs {
		if instruction == read {
			return seen
		}
		if instruction == store {
			seen = true
		}
	}
	return false
}

func sourceAlternatives(values []*sourcevalue.Value, anchor *sourcevalue.Anchor) *sourcevalue.Value {
	byContent := make(map[string]*sourcevalue.Value)
	for _, value := range values {
		raw, _ := json.Marshal(value)
		byContent[string(raw)] = value
	}
	keys := make([]string, 0, len(byContent))
	for key := range byContent {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 1 {
		return byContent[keys[0]]
	}
	result := &sourcevalue.Value{Kind: "alternatives", Anchor: anchor}
	for _, key := range keys {
		result.Parts = append(result.Parts, *byContent[key])
	}
	return result
}

// sourceReturn exposes only the retained expression returned by a local native
// callable. It does not inline calls made by that expression or read dependency
// implementations. The consumer follows its original call-site identity.
func (a *analyzer) sourceReturn(fn *ssa.Function) *sourcevalue.Value {
	if fn == nil || fn.Syntax() == nil || !a.isRepositoryFunction(fn) {
		return nil
	}
	var values []*sourcevalue.Value
	for _, block := range fn.Blocks {
		for _, instruction := range block.Instrs {
			if result, ok := instruction.(*ssa.Return); ok && len(result.Results) > 0 {
				values = append(values, a.sourceValue(result.Results[0], make(map[ssa.Value]bool), result))
			}
		}
	}
	if len(values) == 0 {
		return nil
	}
	result := sourceAlternatives(values, a.valueAnchor(fn.Syntax().Pos()))
	if result.Kind == "record" || result.Kind == "alternatives" {
		result.Owner = a.valueAnchor(fn.Syntax().Pos())
	}
	return result
}
