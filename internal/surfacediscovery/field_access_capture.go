package surfacediscovery

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// fieldSelections is what the typed syntax already alive says about fields:
// where a selector names a field as written (x.f, at f), and which struct
// declaration of the target's packages holds each field. SSA computes a
// field's address at such a selector, and also where no selector names it:
// a composite literal's element and an implicit step through an embedded
// field.
type fieldSelections struct {
	named   map[token.Pos]*types.Var
	holders map[*types.Var]fieldHolder
}

// fieldHolder is the package-level struct type declaring a field, as the
// core object index declares it.
type fieldHolder struct {
	pkg, typeName string
}

// fieldRole is what the code does with one field it names.
type fieldRole uint8

const (
	// fieldStep passes the field through: a struct value a further field
	// is taken from, or an array element on the way to one.
	fieldStep fieldRole = iota
	// fieldRead uses the field: its value, its address, a method called on
	// it, the pointer, slice or map it holds to reach an element or a
	// further field.
	fieldRead
	// fieldWrite is the destination of an assignment, of ++ or --, or an
	// element of an array field there.
	fieldWrite
)

// fieldSelections reads the admitted packages' typed syntax once.
func (a *analyzer) fieldSelections() *fieldSelections {
	if a.fields != nil {
		return a.fields
	}
	result := &fieldSelections{named: make(map[token.Pos]*types.Var), holders: make(map[*types.Var]fieldHolder)}
	for _, admitted := range a.input.Packages {
		facts := a.packageFacts[admitted.Path]
		if facts == nil || facts.TypesInfo == nil {
			continue
		}
		for expression, selection := range facts.TypesInfo.Selections {
			if field, ok := selection.Obj().(*types.Var); ok && selection.Kind() == types.FieldVal && expression.Sel != nil {
				result.named[expression.Sel.Pos()] = field.Origin()
			}
		}
		// The struct types the core object index declares: package-level
		// declarations in the repository's own files.
		for _, file := range facts.Syntax {
			if file == nil {
				continue
			}
			if location := a.location(file.Package); !validRepositoryDirectCallLocation(location) || location.Column <= 0 {
				continue
			}
			for _, declaration := range file.Decls {
				general, ok := declaration.(*ast.GenDecl)
				if !ok {
					continue
				}
				for _, spec := range general.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok || typeSpec.Name == nil || typeSpec.Name.Name == "_" {
						continue
					}
					if _, structure := typeSpec.Type.(*ast.StructType); !structure {
						continue
					}
					object, ok := facts.TypesInfo.Defs[typeSpec.Name].(*types.TypeName)
					if !ok || object.Pkg() == nil || object.Pkg().Path() != admitted.Path {
						continue
					}
					structure, ok := types.Unalias(object.Type()).Underlying().(*types.Struct)
					if !ok {
						continue
					}
					for position := 0; position < structure.NumFields(); position++ {
						if field := structure.Field(position); field.Name() != "_" {
							result.holders[field.Origin()] = fieldHolder{pkg: admitted.Path, typeName: object.Name()}
						}
					}
				}
			}
		}
	}
	a.fields = result
	return result
}

// namedField is the field an SSA field address takes where a selector names
// it, nil for a composite literal's element or an implicit step through an
// embedded field.
func (selections *fieldSelections) namedField(address *ssa.FieldAddr) *types.Var {
	if address == nil {
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
	field := structure.Field(address.Field).Origin()
	if selections.named[address.Pos()] != field {
		return nil
	}
	return field
}

// recordFieldAccesses records each field of a repository struct the body of
// function names, read or written, at the field as written. The SSA
// function owns the body, a closure its own; what each selector does with
// its field and the path it reaches it by are read from the body's typed
// syntax, as the C adapter reads clang's: SSA lifts a local into the value
// it holds and computes the address of x.f twice for x.f += v, so neither
// the chain as written nor one fact per site survives in it.
func (a *analyzer) recordFieldAccesses(function *ssa.Function, callerID string) {
	if a == nil || a.directCallIndex == nil || function == nil || callerID == "" {
		return
	}
	var body *ast.BlockStmt
	switch syntax := function.Syntax().(type) {
	case *ast.FuncDecl:
		body = syntax.Body
	case *ast.FuncLit:
		body = syntax.Body
	}
	facts := a.packageFacts[functionPackagePath(function)]
	if body == nil || facts == nil || facts.TypesInfo == nil {
		return
	}
	info := facts.TypesInfo
	selections := a.fieldSelections()
	var parents []ast.Node
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil {
			parents = parents[:len(parents)-1]
			return true
		}
		// A function literal is a closure of its own.
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		if selector, ok := node.(*ast.SelectorExpr); ok {
			a.recordFieldSelector(info, selections, parents, selector, callerID)
		}
		parents = append(parents, node)
		return true
	})
}

// recordFieldSelector records one selector naming a field of a repository
// struct, unless the code only passes the field through.
func (a *analyzer) recordFieldSelector(info *types.Info, selections *fieldSelections, parents []ast.Node, selector *ast.SelectorExpr, callerID string) {
	selection := info.Selections[selector]
	if selection == nil || selection.Kind() != types.FieldVal {
		return
	}
	field, ok := selection.Obj().(*types.Var)
	if !ok {
		return
	}
	field = field.Origin()
	holder, known := selections.holders[field]
	if !known {
		return
	}
	role := fieldSyntaxRole(info, parents, selector)
	if role == fieldStep {
		return
	}
	site := a.location(selector.Sel.Pos())
	if !validRepositoryDirectCallLocation(site) || site.Column <= 0 {
		return
	}
	a.directCallIndex.recordFieldAccess(DirectCallFieldAccess{
		CallerID: callerID, Package: holder.pkg, Type: holder.typeName, Field: field.Name(),
		Path: a.fieldSyntaxPath(info, selections, selector), Write: role == fieldWrite, Site: site,
	})
}

// fieldSyntaxRole is what the expressions around a field selector do with
// the field: the destination of =, of a compound assignment, of ++ or --
// and of a range clause's assignment is written, and so is an element of an
// array field there; a struct value a further field is taken from, and an
// array field indexed on the way to one, is passed through; everything else
// reads: the value, the address (&c.stats), a method called on it, and the
// pointer, slice or map it holds to reach an element or a further field.
func fieldSyntaxRole(info *types.Info, parents []ast.Node, selector *ast.SelectorExpr) fieldRole {
	var child ast.Expr = selector
	for position := len(parents) - 1; position >= 0; position-- {
		switch parent := parents[position].(type) {
		case *ast.ParenExpr:
			child = parent
			continue
		case *ast.AssignStmt:
			if parent.Tok != token.DEFINE && slices.Contains(parent.Lhs, child) {
				return fieldWrite
			}
		case *ast.IncDecStmt:
			return fieldWrite
		case *ast.RangeStmt:
			if parent.Tok == token.ASSIGN && (parent.Key == child || parent.Value == child) {
				return fieldWrite
			}
		case *ast.IndexExpr:
			// An element of an array field is the field's own storage.
			if indexed := info.TypeOf(parent.X); indexed != nil && parent.X == child {
				if _, array := indexed.Underlying().(*types.Array); array {
					child = parent
					continue
				}
			}
		case *ast.SelectorExpr:
			// A struct value a further field is taken from is passed
			// through; a pointer it holds is read to reach the field.
			if further := info.Selections[parent]; further != nil && further.Kind() == types.FieldVal && parent.X == child {
				if value := info.TypeOf(child); value != nil {
					if _, pointer := value.Underlying().(*types.Pointer); !pointer {
						return fieldStep
					}
				}
			}
		}
		return fieldRead
	}
	return fieldRead
}

// fieldSyntaxPath is a field as the code reaches it: the package variable the
// chain starts from or, from any other value (a parameter, a local, a
// call's result), the struct type declaring the chain's first named field,
// then each named field. Elements, dereferences and implicit steps through
// embedded fields are left out: serverState.db[j].value is
// serverState.db.value, e.value stateEntry.value.
func (a *analyzer) fieldSyntaxPath(info *types.Info, selections *fieldSelections, selector *ast.SelectorExpr) string {
	var names []string
	var first *types.Var
	root := ""
	var current ast.Expr = selector
walk:
	for {
		switch expression := current.(type) {
		case *ast.SelectorExpr:
			selection := info.Selections[expression]
			if selection == nil {
				// A qualified identifier: another package's variable.
				root = a.packageVariableName(info.Uses[expression.Sel])
				break walk
			}
			field, ok := selection.Obj().(*types.Var)
			if !ok || selection.Kind() != types.FieldVal {
				break walk
			}
			first = field.Origin()
			names = append(names, first.Name())
			current = expression.X
		case *ast.ParenExpr:
			current = expression.X
		case *ast.IndexExpr:
			current = expression.X
		case *ast.StarExpr:
			current = expression.X
		case *ast.Ident:
			root = a.packageVariableName(info.Uses[expression])
			break walk
		default:
			break walk
		}
	}
	if root != "" {
		names = append(names, root)
	} else if holder, known := selections.holders[first]; known {
		names = append(names, holder.typeName)
	}
	slices.Reverse(names)
	return strings.Join(names, ".")
}

// packageVariableName is the name of a package-level variable of one of the
// target's packages, empty for any other object.
func (a *analyzer) packageVariableName(object types.Object) string {
	variable, ok := object.(*types.Var)
	if !ok || variable.IsField() || variable.Pkg() == nil || variable.Parent() != variable.Pkg().Scope() ||
		!a.admittedPackages[variable.Pkg().Path()] {
		return ""
	}
	return variable.Name()
}
