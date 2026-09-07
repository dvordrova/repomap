package surfacediscovery

import (
	"go/token"
	"go/types"

	"github.com/dvordrova/repomap/internal/godynamichandoff"
	"golang.org/x/tools/go/ssa"
)

// These are writes to the exact compiler-owned field, not a search for types
// with compatible methods. Different instances can hold different values, so
// a field-derived call always retains an open receiver frontier.
func (capture *dynamicHandoffCapture) collectInterfaceFieldStores(a *analyzer, functions []*ssa.Function) {
	if capture == nil || !capture.enabled {
		return
	}
	capture.interfaceFields = make(map[*types.Var][]*ssa.Store)
	for _, function := range functions {
		if a.ctx.Err() != nil {
			return
		}
		if function == nil || function.Synthetic != "" || !a.isRepositoryFunction(function) {
			continue
		}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				store, ok := instruction.(*ssa.Store)
				if !ok {
					continue
				}
				address, ok := store.Addr.(*ssa.FieldAddr)
				if !ok {
					continue
				}
				field, _ := interfaceField(address)
				if field == nil || !validRepositoryDirectCallLocation(a.location(store.Pos())) {
					continue
				}
				capture.interfaceFields[field] = append(capture.interfaceFields[field], store)
			}
		}
	}
}

func interfaceField(address *ssa.FieldAddr) (*types.Var, types.Type) {
	if address == nil || address.X == nil {
		return nil, nil
	}
	pointer, ok := address.X.Type().Underlying().(*types.Pointer)
	if !ok {
		return nil, nil
	}
	container, ok := pointer.Elem().Underlying().(*types.Struct)
	if !ok || address.Field < 0 || address.Field >= container.NumFields() {
		return nil, nil
	}
	field := container.Field(address.Field)
	if iface, ok := field.Type().Underlying().(*types.Interface); !ok || iface.Complete().NumMethods() == 0 {
		return nil, nil
	}
	return field, pointer.Elem()
}

func interfaceReceiverField(value ssa.Value) (*types.Var, types.Type) {
	for {
		switch current := value.(type) {
		case *ssa.ChangeInterface:
			value = current.X
		case *ssa.UnOp:
			if current.Op != token.MUL {
				return nil, nil
			}
			address, _ := current.X.(*ssa.FieldAddr)
			return interfaceField(address)
		default:
			return nil, nil
		}
	}
}

func resolveInterfaceField(a *analyzer, value *ssa.UnOp, method *types.Func, resolved map[*ssa.Function]struct{}, assignments map[*ssa.Function][]godynamichandoff.Location, active map[ssa.Value]bool) int {
	field, _ := interfaceReceiverField(value)
	if field == nil || a.dynamicHandoffCapture == nil {
		return 1
	}
	// A known store is not proof that this instance was created there. The
	// remaining receiver is deliberately unresolved, even for one observed type.
	unresolved := 1
	for _, store := range a.dynamicHandoffCapture.interfaceFields[field] {
		candidates := make(map[*ssa.Function]struct{})
		origins := make(map[*ssa.Function][]godynamichandoff.Location)
		unresolved += resolveDynamicInterfaceValue(a, store.Val, method, candidates, origins, active)
		location := dynamicLocation(a.location(store.Pos()))
		for function := range candidates {
			resolved[function] = struct{}{}
			assignments[function] = append(assignments[function], origins[function]...)
			assignments[function] = append(assignments[function], location)
		}
	}
	return unresolved
}

// A factory's concrete return expression identifies a possible value. Interface
// parameters, external factories and recursive unresolved returns stay open.
// This does not instantiate or execute the factory.
func resolveInterfaceReturns(a *analyzer, call *ssa.Call, result int, method *types.Func, resolved map[*ssa.Function]struct{}, assignments map[*ssa.Function][]godynamichandoff.Location, active map[ssa.Value]bool) int {
	callee := call.Common().StaticCallee()
	if callee == nil || !a.isRepositoryFunction(callee) || len(callee.Blocks) == 0 {
		return 1
	}
	unresolved, returns := 0, 0
	for _, block := range callee.Blocks {
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			returns++
			if result >= len(returned.Results) {
				unresolved++
				continue
			}
			unresolved += resolveDynamicInterfaceValue(a, returned.Results[result], method, resolved, assignments, active)
		}
	}
	if returns == 0 {
		unresolved++
	}
	return unresolved
}
