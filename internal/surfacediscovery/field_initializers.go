package surfacediscovery

import (
	"cmp"
	"go/types"
	"slices"

	"golang.org/x/tools/go/ssa"

	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// storedField is one store the program's code makes into a field of a
// repository struct: `x.f = v` (the field address at the selector) or a
// composite literal's element `T{f: v}` (at the key's colon).
type storedField struct {
	store *ssa.Store
	site  Location
}

// fieldOutsideWrite is a site where code the program does not show may write
// a field: the field's address handed to a call (`fs.StringVar(&c.addr, …)`),
// or the address of a value of its struct handed to a call outside the
// repository (`json.Unmarshal(data, &cfg)`, `engine.Find(&ldaps)`).
type fieldOutsideWrite struct {
	site Location
}

// fieldStores indexes, once for the target, every write to each field of a
// repository struct and every site where code outside the program may write
// one. A field read whose instance the walk cannot follow references them
// (fieldWritesRef); the target's table holds them once (fieldWrites).
type fieldStores struct {
	writes  map[*types.Var][]storedField
	outside map[*types.Var][]fieldOutsideWrite
	// keys are the fields a read referenced, by key; pending those whose
	// writes are not yet in the table (fieldWrites).
	keys    map[string]*types.Var
	pending []*types.Var
}

// collectFieldStores reads every repository function's stores and calls
// once, before any value is read, so the order functions are recorded in
// cannot change what a field holds.
func (a *analyzer) collectFieldStores(functions []*ssa.Function) {
	selections := a.fieldSelections()
	stores := &fieldStores{writes: make(map[*types.Var][]storedField), outside: make(map[*types.Var][]fieldOutsideWrite), keys: make(map[string]*types.Var)}
	a.fieldStores = stores
	holderFields := make(map[types.Type][]*types.Var)
	for field, holder := range selections.holders {
		if field.Pkg() == nil {
			continue
		}
		if object, ok := field.Pkg().Scope().Lookup(holder.typeName).(*types.TypeName); ok {
			holderFields[object.Type()] = append(holderFields[object.Type()], field)
		}
	}
	for named := range holderFields {
		slices.SortFunc(holderFields[named], func(x, y *types.Var) int { return cmp.Compare(x.Pos(), y.Pos()) })
	}
	outside := func(field *types.Var, site Location) {
		stores.outside[field] = append(stores.outside[field], fieldOutsideWrite{site: site})
	}
	for _, function := range functions {
		if a.ctx.Err() != nil {
			return
		}
		if function == nil || function.Synthetic != "" || !a.isRepositoryFunction(function) {
			continue
		}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				switch instruction := instruction.(type) {
				case *ssa.Store:
					address, ok := instruction.Addr.(*ssa.FieldAddr)
					if !ok {
						continue
					}
					field := addressedField(address)
					if field == nil {
						continue
					}
					if _, known := selections.holders[field]; !known {
						continue
					}
					position := address.Pos()
					if !position.IsValid() {
						position = instruction.Pos()
					}
					site := a.location(position)
					if !validRepositoryDirectCallLocation(site) {
						continue
					}
					stores.writes[field] = append(stores.writes[field], storedField{store: instruction, site: site})
				case ssa.CallInstruction:
					common := instruction.Common()
					site := a.location(instruction.Pos())
					if !validRepositoryDirectCallLocation(site) {
						continue
					}
					callee := common.StaticCallee()
					repository := callee != nil && a.isRepositoryFunction(callee)
					for _, argument := range common.Args {
						argument = unwrapInterface(argument)
						if address, ok := argument.(*ssa.FieldAddr); ok {
							if field := addressedField(address); field != nil {
								if _, known := selections.holders[field]; known {
									outside(field, site)
								}
							}
							continue
						}
						if repository {
							continue
						}
						if named := pointedStruct(argument.Type()); named != nil {
							for _, field := range holderFields[named] {
								outside(field, site)
							}
						}
					}
				}
			}
		}
	}
	for field := range stores.writes {
		slices.SortStableFunc(stores.writes[field], func(x, y storedField) int { return compareLocations(x.site, y.site) })
	}
	for field := range stores.outside {
		slices.SortStableFunc(stores.outside[field], func(x, y fieldOutsideWrite) int { return compareLocations(x.site, y.site) })
	}
}

func compareLocations(x, y Location) int {
	return cmp.Or(cmp.Compare(x.Path, y.Path), cmp.Compare(x.Line, y.Line), cmp.Compare(x.Column, y.Column))
}

// addressedField is the field an SSA field address takes, by its origin.
func addressedField(address *ssa.FieldAddr) *types.Var {
	if address == nil || address.X == nil {
		return nil
	}
	pointer, ok := address.X.Type().Underlying().(*types.Pointer)
	if !ok {
		return nil
	}
	structure, ok := pointer.Elem().Underlying().(*types.Struct)
	if !ok || address.Field < 0 || address.Field >= structure.NumFields() {
		return nil
	}
	return structure.Field(address.Field).Origin()
}

// pointedStruct is the named struct a pointer argument lets a callee fill:
// *T, *[]T, *[]*T or *map[K]T, by T's origin; nil for anything else.
func pointedStruct(typ types.Type) types.Type {
	pointer, ok := types.Unalias(typ).Underlying().(*types.Pointer)
	if !ok {
		return nil
	}
	element := types.Unalias(pointer.Elem())
	switch container := element.Underlying().(type) {
	case *types.Slice:
		element = types.Unalias(container.Elem())
	case *types.Map:
		element = types.Unalias(container.Elem())
	}
	if inner, ok := element.Underlying().(*types.Pointer); ok {
		element = types.Unalias(inner.Elem())
	}
	named, ok := element.(*types.Named)
	if !ok {
		return nil
	}
	if _, ok := named.Underlying().(*types.Struct); !ok {
		return nil
	}
	return named.Origin()
}

func unwrapInterface(value ssa.Value) ssa.Value {
	for {
		switch current := value.(type) {
		case *ssa.MakeInterface:
			value = current.X
		case *ssa.ChangeInterface:
			value = current.X
		default:
			return value
		}
	}
}

// fieldWritesRef is what a read of field holds when its instance cannot be
// followed: a reference, by the field's key, to every write the code makes
// into it (fieldWrites), stored once for the target however many reads
// name it. Nil for a field no write and no outside writer reaches.
func (a *analyzer) fieldWritesRef(field *types.Var) *sourcevalue.Value {
	stores := a.fieldStores
	if stores == nil || field == nil || len(stores.writes[field]) == 0 && len(stores.outside[field]) == 0 {
		return nil
	}
	key := a.fieldKey(field)
	if key == "" {
		return nil
	}
	if _, known := stores.keys[key]; !known {
		stores.keys[key] = field
		stores.pending = append(stores.pending, field)
	}
	return &sourcevalue.Value{Kind: "field_writes", Text: key}
}

// fieldKey names a repository struct's field as package path, type and
// field: example.com/app/object.Ormer.Engine.
func (a *analyzer) fieldKey(field *types.Var) string {
	holder, known := a.fieldSelections().holders[field]
	if !known {
		return ""
	}
	return holder.pkg + "." + holder.typeName + "." + field.Name()
}

// fieldWrites are, once per field a read referenced, every write the code
// makes into it, in source order, each a field_value at the write's site,
// and, at each site where code the program does not show may write it, an
// unknown alternative: one write is that write, several are alternatives,
// none chosen. A write of nil or another zero constant puts no value there.
// A write's own field reads reference their fields in turn, which join the
// table.
func (a *analyzer) fieldWrites() []DirectCallFieldWrites {
	stores := a.fieldStores
	if stores == nil {
		return nil
	}
	var result []DirectCallFieldWrites
	for len(stores.pending) > 0 {
		field := stores.pending[0]
		stores.pending = stores.pending[1:]
		var parts []sourcevalue.Value
		for _, write := range stores.writes[field] {
			if constantValue, ok := write.store.Val.(*ssa.Const); ok && (constantValue.Value == nil || zeroConstant(constantValue.Value)) {
				continue
			}
			value := a.sourceValue(write.store.Val, make(map[ssa.Value]bool), write.store)
			site := write.site
			parts = append(parts, sourcevalue.Value{Kind: "field_value", Text: field.Name(),
				Anchor: &sourcevalue.Anchor{Path: site.Path, Line: site.Line, Column: site.Column}, Parts: []sourcevalue.Value{*value}})
		}
		for position, written := range stores.outside[field] {
			if position > 0 && written.site == stores.outside[field][position-1].site {
				continue
			}
			site := written.site
			anchor := &sourcevalue.Anchor{Path: site.Path, Line: site.Line, Column: site.Column}
			parts = append(parts, sourcevalue.Value{Kind: "field_value", Text: field.Name(), Anchor: anchor,
				Parts: []sourcevalue.Value{{Kind: "unknown", Text: "written by a call it is handed to", Anchor: anchor}}})
		}
		var value sourcevalue.Value
		switch len(parts) {
		case 0:
			value = sourcevalue.Value{Kind: "unknown", Text: "no write stores a value"}
		case 1:
			value = parts[0]
		default:
			value = sourcevalue.Value{Kind: "alternatives", Parts: parts}
		}
		result = append(result, DirectCallFieldWrites{Field: a.fieldKey(field), Value: value})
	}
	slices.SortFunc(result, func(x, y DirectCallFieldWrites) int { return cmp.Compare(x.Field, y.Field) })
	return result
}

// recordHoldsField reports whether a record value already holds a field.
func recordHoldsField(value *sourcevalue.Value, name string) bool {
	if value == nil || value.Kind != "record" {
		return false
	}
	for _, part := range value.Parts {
		if part.Kind == "field_value" && part.Text == name {
			return true
		}
	}
	return false
}
