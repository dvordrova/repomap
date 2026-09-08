package surfacediscovery

import (
	"go/token"
	"go/types"

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
