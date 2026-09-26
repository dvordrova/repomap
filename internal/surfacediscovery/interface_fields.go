package surfacediscovery

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// interfaceFieldStore is one write to an interface field. underBranch marks a
// non-nil store inside an if, switch or select of its function: the field
// holds that value only on the paths the branch takes.
type interfaceFieldStore struct {
	store       *ssa.Store
	underBranch bool
}

// These are writes to the exact compiler-owned field, not a search for types
// with compatible methods. Different instances can hold different values, so
// a field-derived call always retains an open receiver frontier.
func (capture *dynamicHandoffCapture) collectInterfaceFieldStores(a *analyzer, functions []*ssa.Function) {
	if capture == nil || !capture.enabled {
		return
	}
	capture.interfaceFields = make(map[*types.Var][]interfaceFieldStore)
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
				// A nil store puts nothing callable into the field, so a branch
				// around it decides nothing a call through the field can reach,
				// as the C adapter ignores a null store.
				stored, isConst := store.Val.(*ssa.Const)
				nilStore := isConst && stored.IsNil()
				capture.interfaceFields[field] = append(capture.interfaceFields[field],
					interfaceFieldStore{store: store, underBranch: !nilStore && storeUnderBranch(function, store.Pos())})
			}
		}
	}
}

// storeUnderBranch reports whether position lies in a branch of an if, a case
// of a switch or type switch, or a clause of a select in function's own body.
// A loop body is no branch, and a function literal is a function of its own,
// as in the C adapter.
func storeUnderBranch(function *ssa.Function, position token.Pos) bool {
	var body ast.Node
	switch syntax := function.Syntax().(type) {
	case *ast.FuncDecl:
		body = syntax.Body
	case *ast.FuncLit:
		body = syntax.Body
	case *ast.RangeStmt:
		body = syntax.Body
	}
	if body == nil || !position.IsValid() {
		return false
	}
	within := func(node ast.Node) bool {
		return node != nil && node.Pos() <= position && position < node.End()
	}
	under := false
	ast.Inspect(body, func(node ast.Node) bool {
		if under || node == nil || !within(node) {
			return false
		}
		switch node := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.IfStmt:
			under = within(node.Body) || within(node.Else)
		case *ast.CaseClause, *ast.CommClause:
			under = true
		}
		return !under
	})
	return under
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
