package surfacediscovery

import (
	"encoding/json"
	"go/ast"
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
	// A join this value already expanded (a later join reading an earlier
	// one on two edges) stays its frontier: the value grows with the code,
	// not with the number of paths through it.
	if _, expanded := active[value]; expanded {
		unknown.Text = "conditional value"
		return unknown
	}
	active[value] = true
	defer func() {
		if _, join := value.(*ssa.Phi); join {
			active[value] = false
			return
		}
		delete(active, value)
	}()
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
		// A control-flow join is each incoming value, in edge order: none is
		// picked, and an edge whose value is not followed stays its unknown.
		// A word an edge carries is anchored where the code stores it in
		// the variable, as other languages anchor their literals.
		stores := a.joinedVariableStores(v)
		values := make([]*sourcevalue.Value, 0, len(v.Edges))
		for _, edge := range v.Edges {
			value := a.sourceValue(edge, active, readAt)
			if value.Kind == "literal" && value.Anchor == nil {
				if at := stores[value.Text]; at.IsValid() {
					anchored := *value
					anchored.Anchor = a.valueAnchor(at)
					value = &anchored
				}
			}
			values = append(values, value)
		}
		return orderedAlternatives(values, unknown.Anchor)
	case *ssa.Alloc:
		var stores []*ssa.Store
		fields := make(map[int][]*ssa.Store)
		handed := make(map[int]token.Pos)
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
						// The field's address handed to a call: the callee may
						// write it (flag.StringVar(&e.url, …)).
						if call, ok := reference.(ssa.CallInstruction); ok && handsValue(call, field) {
							if _, seen := handed[field.Field]; !seen {
								handed[field.Field] = call.Pos()
							}
						}
					}
				}
			}
		}
		if len(stores) > 0 {
			return a.initialStoreValue(v, stores, readAt, active)
		}
		// new(T) and &T{…} produce a value here: a record of the fields
		// stored, empty when none was.
		if pointer, ok := v.Type().Underlying().(*types.Pointer); ok {
			if structure, ok := pointer.Elem().Underlying().(*types.Struct); ok {
				result := &sourcevalue.Value{Kind: "record", Anchor: unknown.Anchor}
				for i := 0; i < structure.NumFields(); i++ {
					var held []*sourcevalue.Value
					if stores := fields[i]; len(stores) > 0 {
						held = append(held, a.initialStoreValue(v, stores, readAt, active))
					}
					if at, ok := handed[i]; ok {
						held = append(held, &sourcevalue.Value{Kind: "unknown", Text: "written by a call it is handed to", Anchor: a.valueAnchor(at)})
					}
					switch len(held) {
					case 1:
						result.Parts = append(result.Parts, sourcevalue.Value{Kind: "field_value", Text: structure.Field(i).Name(), Parts: []sourcevalue.Value{*held[0]}})
					case 2:
						result.Parts = append(result.Parts, sourcevalue.Value{Kind: "field_value", Text: structure.Field(i).Name(), Parts: []sourcevalue.Value{{Kind: "alternatives", Parts: []sourcevalue.Value{*held[0], *held[1]}}}})
					}
				}
				return result
			}
		}
	case *ssa.FieldAddr:
		return a.fieldSourceValue(v.X, v.Field, v.Pos(), active, readAt)
	case *ssa.Field:
		return a.fieldSourceValue(v.X, v.Field, v.Pos(), active, readAt)
	case *ssa.Lookup:
		return &sourcevalue.Value{Kind: "index", Anchor: a.valueAnchor(v.Pos()), Parts: []sourcevalue.Value{*a.sourceValue(v.X, active, readAt), *a.indexValue(v.Index, active, readAt)}}
	case *ssa.Index:
		return &sourcevalue.Value{Kind: "index", Anchor: a.valueAnchor(v.Pos()), Parts: []sourcevalue.Value{*a.sourceValue(v.X, active, readAt), *a.indexValue(v.Index, active, readAt)}}
	case *ssa.IndexAddr:
		// args[0] of a slice: the element's address, read through a load.
		return &sourcevalue.Value{Kind: "index", Anchor: a.valueAnchor(v.Pos()), Parts: []sourcevalue.Value{*a.sourceValue(v.X, active, readAt), *a.indexValue(v.Index, active, readAt)}}
	case *ssa.Global:
		// A package variable (os.Args) is not followed; its name says which.
		if v.Pkg != nil && v.Pkg.Pkg != nil {
			unknown.Text = v.Pkg.Pkg.Name() + "." + v.Name()
		}
	}
	return unknown
}

// indexValue is an element's index: a constant number as written, anything
// else as any source value.
func (a *analyzer) indexValue(value ssa.Value, active map[ssa.Value]bool, readAt ssa.Instruction) *sourcevalue.Value {
	if c, ok := value.(*ssa.Const); ok && c.Value != nil && c.Value.Kind() == constant.Int {
		return &sourcevalue.Value{Kind: "literal", Text: c.Value.ExactString()}
	}
	return a.sourceValue(value, active, readAt)
}

func (a *analyzer) fieldSourceValue(receiver ssa.Value, field int, pos token.Pos, active map[ssa.Value]bool, readAt ssa.Instruction) *sourcevalue.Value {
	typ := receiver.Type()
	if pointer, ok := typ.Underlying().(*types.Pointer); ok {
		typ = pointer.Elem()
	}
	if fields, ok := typ.Underlying().(*types.Struct); ok && field < fields.NumFields() {
		name := fields.Field(field).Name()
		instance := a.sourceValue(receiver, active, readAt)
		value := &sourcevalue.Value{Kind: "field", Text: name, Anchor: a.valueAnchor(pos), Parts: []sourcevalue.Value{*instance}}
		// An instance the value names a record of already holds the field;
		// any other may hold what any write of the field put there: a
		// reference to the field's writes, stored once for the target
		// (DirectCallIndex.FieldWrites).
		if !recordHoldsField(instance, name) {
			value.Initializer = a.fieldWritesRef(fields.Field(field).Origin())
		}
		return value
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

// orderedAlternatives keeps each distinct value once, where it first comes;
// values all the same are that one value.
func orderedAlternatives(values []*sourcevalue.Value, anchor *sourcevalue.Anchor) *sourcevalue.Value {
	seen := make(map[string]bool)
	result := &sourcevalue.Value{Kind: "alternatives", Anchor: anchor}
	for _, value := range values {
		raw, _ := json.Marshal(value)
		if seen[string(raw)] {
			continue
		}
		seen[string(raw)] = true
		result.Parts = append(result.Parts, *value)
	}
	if len(result.Parts) == 1 {
		return &result.Parts[0]
	}
	return result
}

// sourceReturn exposes only the retained expression returned by a local native
// callable. It does not inline calls made by that expression or read dependency
// implementations. The consumer follows its original call-site identity.
// errorReturn says whether a return is a function's failure: its last
// result is an error that is not the nil constant, and every other result
// is its type's zero value written as a constant ("", 0, false, nil).
func errorReturn(result *ssa.Return) bool {
	if len(result.Results) < 2 {
		return false
	}
	last := result.Results[len(result.Results)-1]
	if !types.Identical(last.Type(), types.Universe.Lookup("error").Type()) {
		return false
	}
	if written, ok := last.(*ssa.Const); ok && written.IsNil() {
		return false
	}
	for _, value := range result.Results[:len(result.Results)-1] {
		written, ok := value.(*ssa.Const)
		if !ok || written.Value != nil && !zeroConstant(written.Value) {
			return false
		}
	}
	return true
}

func zeroConstant(value constant.Value) bool {
	switch value.Kind() {
	case constant.String:
		return constant.StringVal(value) == ""
	case constant.Bool:
		return !constant.BoolVal(value)
	case constant.Int, constant.Float, constant.Complex:
		return constant.Sign(value) == 0
	}
	return false
}

func (a *analyzer) sourceReturn(fn *ssa.Function) *sourcevalue.Value {
	if fn == nil || fn.Syntax() == nil || !a.isRepositoryFunction(fn) {
		return nil
	}
	var values, failures []*sourcevalue.Value
	for _, block := range fn.Blocks {
		for _, instruction := range block.Instrs {
			if result, ok := instruction.(*ssa.Return); ok && len(result.Results) > 0 {
				value := a.sourceValue(result.Results[0], make(map[ssa.Value]bool), result)
				// A word returned as written is anchored at its return,
				// as a join's stored word is where it is stored.
				if constant, ok := result.Results[0].(*ssa.Const); ok && value.Kind == "literal" && value.Anchor == nil && constant.Pos() == token.NoPos {
					anchored := *value
					anchored.Anchor = a.valueAnchor(result.Pos())
					value = &anchored
				}
				if errorReturn(result) {
					failures = append(failures, value)
					continue
				}
				values = append(values, value)
			}
		}
	}
	// A return handing back zero values with an error that is not nil
	// (`return "", err`) is the function failing: its caller uses no other
	// result then, so what it returns is what the other returns give.
	// litestream's expand returned "" beside its error, and the WAL path
	// db.path + "-wal" read "-wal". A function that only fails keeps them.
	if len(values) == 0 {
		values = failures
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

// joinedVariableStores are, for the variable a join merges, the one place
// each word is stored in it, by the word: `res = "https://cdn.casbin.org"`
// stores that word at its literal. The variable is the one the join's
// position declares (go/ssa lifts a local to joins at its declaration), so a
// variable of the same name in another block is another. A word stored
// twice has no one place, and a variable ever assigned another variable's
// value (`res = x`) has none at all: the join's edge may carry x's word
// from where x was stored, which no syntax of res names. A store of a
// call's or an operation's result (casdoor's `res, _ :=
// web.AppConfig.String(key)`) carries no word and leaves the others theirs.
func (a *analyzer) joinedVariableStores(phi *ssa.Phi) map[string]token.Pos {
	function := phi.Parent()
	if function == nil || !phi.Pos().IsValid() {
		return nil
	}
	facts := a.packageFacts[functionPackagePath(function)]
	if facts == nil || facts.TypesInfo == nil || function.Syntax() == nil {
		return nil
	}
	info := facts.TypesInfo
	var variable types.Object
	ast.Inspect(function.Syntax(), func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok && ident.Pos() == phi.Pos() {
			variable = info.Defs[ident]
		}
		return variable == nil
	})
	if variable == nil {
		return nil
	}
	stores := make(map[string]token.Pos)
	twice := make(map[string]bool)
	copied := false
	store := func(name *ast.Ident, value ast.Expr) {
		if name == nil || value == nil || info.ObjectOf(name) != variable {
			return
		}
		word, ok := info.Types[value]
		if !ok || word.Value == nil || word.Value.Kind() != constant.String {
			// Only another variable's value can carry a word the join's
			// edge holds as a constant; a call's or an operation's result
			// (`res, _ := web.AppConfig.String(key)`) carries none.
			if name, ok := ast.Unparen(value).(*ast.Ident); ok {
				if _, ok := info.ObjectOf(name).(*types.Var); ok {
					copied = true
				}
			}
			return
		}
		text := constant.StringVal(word.Value)
		if _, seen := stores[text]; seen {
			twice[text] = true
		}
		stores[text] = value.Pos()
	}
	ast.Inspect(function.Syntax(), func(node ast.Node) bool {
		switch statement := node.(type) {
		case *ast.AssignStmt:
			if statement.Tok != token.ASSIGN && statement.Tok != token.DEFINE {
				// res += "x" stores no word of its own.
				for _, left := range statement.Lhs {
					if name, ok := left.(*ast.Ident); ok && info.ObjectOf(name) == variable {
						copied = true
					}
				}
				return true
			}
			if len(statement.Lhs) == len(statement.Rhs) {
				for i, left := range statement.Lhs {
					if name, ok := left.(*ast.Ident); ok {
						store(name, statement.Rhs[i])
					}
				}
			}
			// A multiple assignment from one call, map index, receive or
			// type assertion stores that expression's results, no word.
		case *ast.ValueSpec:
			for i, name := range statement.Names {
				if i < len(statement.Values) {
					store(name, statement.Values[i])
				}
			}
		case *ast.RangeStmt:
			for _, left := range []ast.Expr{statement.Key, statement.Value} {
				if name, ok := left.(*ast.Ident); ok && info.ObjectOf(name) == variable {
					copied = true
				}
			}
		case *ast.UnaryExpr:
			if name, ok := statement.X.(*ast.Ident); ok && statement.Op == token.AND && info.ObjectOf(name) == variable {
				copied = true
			}
		}
		return true
	})
	if copied {
		return nil
	}
	for text := range twice {
		delete(stores, text)
	}
	return stores
}

// handsValue reports whether a call hands value as one of its arguments.
func handsValue(call ssa.CallInstruction, value ssa.Value) bool {
	for _, argument := range call.Common().Args {
		if unwrapInterface(argument) == value {
			return true
		}
	}
	return false
}
