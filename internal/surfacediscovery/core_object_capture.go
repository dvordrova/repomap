package surfacediscovery

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"

	"github.com/dvordrova/repomap/internal/gocoreobject"
)

// captureCoreObjectIndex projects exact target-scoped declarations while the
// ordinary packages/types/SSA objects are still alive. It performs no package
// load, parsing pass, type check, SSA build, or source read.
func (a *analyzer) captureCoreObjectIndex(direct DirectCallIndex) (gocoreobject.Index, error) {
	if a == nil || a.program == nil || a.input.AnalysisTarget == nil {
		return gocoreobject.Index{}, fmt.Errorf("go core object index: target-scoped typed program is unavailable")
	}
	directNodes := make(map[string]struct{}, len(direct.Nodes))
	for _, node := range direct.Nodes {
		directNodes[node.ID] = struct{}{}
	}
	target := a.input.AnalysisTarget
	input := gocoreobject.Input{
		Scenario: gocoreobject.Scenario{
			ID: a.scenario.ID, GOOS: a.scenario.GOOS, GOARCH: a.scenario.GOARCH,
			Tags: append([]string(nil), a.scenario.Tags...),
		},
		Scope: gocoreobject.Scope{
			TargetRef: target.TargetRef, TargetKind: target.Kind,
			TargetModuleID: target.ModuleID, TargetModulePath: target.ModulePath,
			TargetModuleDir: target.ModuleDir, TargetPackage: target.PackagePath,
			TargetPackages: append([]string(nil), target.TargetPackages...),
		},
		Packages: []gocoreobject.Package{}, Types: []gocoreobject.TypeDeclaration{},
		Callables: []gocoreobject.CallableDeclaration{},
	}
	for _, admitted := range a.input.Packages {
		facts := a.packageFacts[admitted.Path]
		if !packageSafeForSSA(facts) || facts.TypesInfo == nil || len(facts.Syntax) == 0 {
			return gocoreobject.Index{}, fmt.Errorf(
				"go core object index: admitted package %q has no complete typed syntax", admitted.Path,
			)
		}
		owner, module, ok := a.externalCallPackage(admitted.Path)
		if !ok {
			return gocoreobject.Index{}, fmt.Errorf(
				"go core object index: admitted package %q has no repository module identity", admitted.Path,
			)
		}
		representativeSource, err := a.coreObjectRepresentativeSource(facts.Syntax, admitted.Path)
		if err != nil {
			return gocoreobject.Index{}, err
		}
		input.Packages = append(input.Packages, gocoreobject.Package{
			ModuleID: module.ID, Module: module.Path, ModuleDir: module.Directory,
			Path: owner.PackagePath, RepresentativeSource: representativeSource,
		})
		for _, file := range facts.Syntax {
			if err := a.captureCoreObjectFile(
				&input, facts.TypesInfo, file, admitted.Path, directNodes,
			); err != nil {
				return gocoreobject.Index{}, err
			}
		}
	}
	if a.input.MatchInterfaceImplementations {
		if err := a.captureInterfaceImplementations(&input); err != nil {
			return gocoreobject.Index{}, err
		}
	}
	return gocoreobject.New(input)
}

type coreNamedType struct {
	declaration gocoreobject.TypeDeclaration
	named       *types.Named
}

// captureInterfaceImplementations uses the already-loaded Go type universe.
// An inverted method-name index narrows candidates before types.Implements
// performs the authoritative method-set check. No call graph or source pass is
// needed, and interface compatibility remains distinct from observed values.
func (a *analyzer) captureInterfaceImplementations(input *gocoreobject.Input) error {
	if a == nil || input == nil {
		return fmt.Errorf("go core object index: interface matching input is unavailable")
	}
	entries := make([]coreNamedType, 0, len(input.Types))
	for _, declaration := range input.Types {
		facts := a.packageFacts[declaration.Package]
		if facts == nil || facts.Types == nil {
			return fmt.Errorf("go core object index: interface matching package %q is unavailable", declaration.Package)
		}
		object, ok := facts.Types.Scope().Lookup(declaration.Name).(*types.TypeName)
		if !ok {
			return fmt.Errorf("go core object index: interface matching type %s.%s is unavailable", declaration.Package, declaration.Name)
		}
		named, ok := types.Unalias(object.Type()).(*types.Named)
		if !ok || declaration.Kind == gocoreobject.TypeAlias {
			continue
		}
		entries = append(entries, coreNamedType{declaration: declaration, named: named})
	}

	candidates := make([]coreNamedType, 0, len(entries))
	byMethod := make(map[string]map[int]struct{})
	for _, entry := range entries {
		if entry.declaration.Kind == gocoreobject.TypeInterface {
			continue
		}
		index := len(candidates)
		candidates = append(candidates, entry)
		seen := make(map[string]struct{})
		for _, receiver := range []types.Type{entry.named, types.NewPointer(entry.named)} {
			set := types.NewMethodSet(receiver)
			for position := 0; position < set.Len(); position++ {
				method, ok := set.At(position).Obj().(*types.Func)
				if !ok {
					continue
				}
				key := method.Id()
				if _, duplicate := seen[key]; duplicate {
					continue
				}
				seen[key] = struct{}{}
				if byMethod[key] == nil {
					byMethod[key] = make(map[int]struct{})
				}
				byMethod[key][index] = struct{}{}
			}
		}
	}

	for _, entry := range entries {
		if entry.declaration.Kind != gocoreobject.TypeInterface {
			continue
		}
		iface, ok := entry.named.Underlying().(*types.Interface)
		if !ok {
			return fmt.Errorf("go core object index: %s.%s lost interface type", entry.declaration.Package, entry.declaration.Name)
		}
		iface.Complete()
		candidateIDs := make([]int, 0, len(candidates))
		if iface.NumMethods() == 0 {
			for index := range candidates {
				candidateIDs = append(candidateIDs, index)
			}
		} else {
			var narrow map[int]struct{}
			narrowSelected := false
			for position := 0; position < iface.NumMethods(); position++ {
				set := byMethod[iface.Method(position).Id()]
				if !narrowSelected || len(set) < len(narrow) {
					narrow = set
					narrowSelected = true
				}
			}
			for index := range narrow {
				candidateIDs = append(candidateIDs, index)
			}
			sort.Ints(candidateIDs)
		}
		for _, index := range candidateIDs {
			implementation := candidates[index]
			valueReceiver := types.Implements(implementation.named, iface)
			pointerReceiver := types.Implements(types.NewPointer(implementation.named), iface)
			if !valueReceiver && !pointerReceiver {
				continue
			}
			input.InterfaceImplementations = append(input.InterfaceImplementations, gocoreobject.InterfaceImplementation{
				InterfacePackage: entry.declaration.Package, InterfaceName: entry.declaration.Name,
				ImplementationPackage: implementation.declaration.Package,
				ImplementationName:    implementation.declaration.Name,
				ValueReceiver:         valueReceiver, PointerReceiver: pointerReceiver,
			})
		}
	}
	return nil
}

// coreObjectRepresentativeSource retains one exact repository-local source
// identity from the typed syntax that is already alive for this package. It
// does not read, parse, load, or infer another file. Generated syntax outside
// the repository is deliberately ineligible for this repository-owned fact.
func (a *analyzer) coreObjectRepresentativeSource(files []*ast.File, packagePath string) (string, error) {
	representative := ""
	for _, file := range files {
		if file == nil {
			continue
		}
		location := a.location(file.Package)
		if !validRepositoryDirectCallLocation(location) || location.Column <= 0 {
			continue
		}
		if representative == "" || location.Path < representative {
			representative = location.Path
		}
	}
	if representative == "" {
		return "", fmt.Errorf(
			"go core object index: admitted package %q has no repository-local typed source",
			packagePath,
		)
	}
	return representative, nil
}

func (a *analyzer) captureCoreObjectFile(
	input *gocoreobject.Input,
	info *types.Info,
	file *ast.File,
	packagePath string,
	directNodes map[string]struct{},
) error {
	if a == nil || input == nil || info == nil || file == nil {
		return fmt.Errorf("go core object index: typed syntax is unavailable for package %q", packagePath)
	}
	// packages.Load includes compiler-generated cgo syntax alongside the
	// rewritten repository source. The rewritten source retains repository
	// locations through its line directives; wholly generated helper files do
	// not and cannot contribute repository-owned declarations.
	fileLocation := a.location(file.Package)
	if !validRepositoryDirectCallLocation(fileLocation) || fileLocation.Column <= 0 {
		return nil
	}
	for _, declaration := range file.Decls {
		switch value := declaration.(type) {
		case *ast.GenDecl:
			for _, spec := range value.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || typeSpec.Name == nil || typeSpec.Name.Name == "_" {
					continue
				}
				object, ok := info.Defs[typeSpec.Name].(*types.TypeName)
				if !ok || object.Pkg() == nil || object.Pkg().Path() != packagePath {
					return fmt.Errorf(
						"go core object index: type declaration %s.%s has no exact object",
						packagePath, typeSpec.Name.Name,
					)
				}
				location, err := a.coreObjectLocation(object.Pos())
				if err != nil {
					return err
				}
				input.Types = append(input.Types, gocoreobject.TypeDeclaration{
					Kind: coreObjectTypeKind(object), Package: packagePath, Name: object.Name(),
					Signature: types.ObjectString(object, packageQualifier),
					Exported:  object.Exported(), Location: location, EndLine: a.location(typeSpec.End()).Line,
				})
				if _, ok := typeSpec.Type.(*ast.StructType); ok {
					structure, ok := types.Unalias(object.Type()).Underlying().(*types.Struct)
					if !ok {
						return fmt.Errorf("go core object index: struct %s.%s has no exact type", packagePath, object.Name())
					}
					for position := 0; position < structure.NumFields(); position++ {
						field := structure.Field(position)
						if field.Name() == "_" {
							continue // blank fields have no addressable declaration
						}
						location, err := a.coreObjectLocation(field.Pos())
						if err != nil {
							return err
						}
						signature := field.Name() + " " + types.TypeString(field.Type(), packageQualifier)
						owner := &input.Types[len(input.Types)-1]
						owner.Fields = append(owner.Fields, gocoreobject.FieldDeclaration{
							Name: field.Name(), Signature: signature, Tag: structure.Tag(position),
							Exported: field.Exported(), Location: location,
						})
					}
				}
				if iface, ok := typeSpec.Type.(*ast.InterfaceType); ok {
					for _, field := range iface.Methods.List {
						// Embedded interfaces declare no method here. Their methods
						// retain their original owner and source location.
						if _, ok := field.Type.(*ast.FuncType); !ok {
							continue
						}
						for _, name := range field.Names {
							method, ok := info.Defs[name].(*types.Func)
							if !ok || method.Pkg() == nil || method.Pkg().Path() != packagePath {
								return fmt.Errorf("go core object index: interface method %s.%s has no exact object", object.Name(), name.Name)
							}
							location, err := a.coreObjectLocation(method.Pos())
							if err != nil {
								return err
							}
							input.Callables = append(input.Callables, gocoreobject.CallableDeclaration{
								Kind: gocoreobject.CallableMethod, Package: packagePath, Name: method.Name(),
								Receiver:  types.TypeString(object.Type(), packageQualifier),
								Signature: types.TypeString(method.Type(), packageQualifier),
								Exported:  method.Exported(), Location: location,
								// An interface declaration is not an implementation or call.
							})
						}
					}
				}
			}
		case *ast.FuncDecl:
			if value.Name == nil || value.Name.Name == "_" {
				continue
			}
			object, ok := info.Defs[value.Name].(*types.Func)
			if !ok || object.Pkg() == nil || object.Pkg().Path() != packagePath {
				return fmt.Errorf(
					"go core object index: callable declaration %s.%s has no exact object",
					packagePath, value.Name.Name,
				)
			}
			signature, ok := object.Type().(*types.Signature)
			if !ok {
				return fmt.Errorf(
					"go core object index: callable declaration %s.%s has no exact signature",
					packagePath, value.Name.Name,
				)
			}
			location, err := a.coreObjectLocation(object.Pos())
			if err != nil {
				return err
			}
			kind := gocoreobject.CallableFunction
			receiver := ""
			if signature.Recv() != nil {
				kind = gocoreobject.CallableMethod
				receiver = types.TypeString(signature.Recv().Type(), packageQualifier)
			}
			directCallNodeID := ""
			if function := a.program.FuncValue(object); function != nil {
				if origin := function.Origin(); origin != nil {
					function = origin
				}
				if node, _, available := a.directCallNode(function, a.scenario); available {
					// Program.FuncValue can materialize a source declaration that
					// ssautil.AllFunctions omitted from the direct-call inventory.
					// Keep the core declaration, but advertise the optional join only
					// when the finished direct index owns that exact node identity.
					if _, indexed := directNodes[node.ID]; indexed {
						directCallNodeID = node.ID
					}
				}
			}
			input.Callables = append(input.Callables, gocoreobject.CallableDeclaration{
				Kind: kind, Package: packagePath, Name: object.Name(), Receiver: receiver,
				Signature: types.TypeString(signature, packageQualifier), Exported: object.Exported(),
				Location: location, EndLine: a.location(value.End()).Line, DirectCallNodeID: directCallNodeID,
				Parameters: typedNames(signature.Params()), Results: typedNames(signature.Results()),
			})
		}
	}
	return nil
}

// typedNames reads a signature's values with the declared type each carries:
// *model.User, []model.User and model.User all carry model.User.
func typedNames(values *types.Tuple) []gocoreobject.TypedName {
	if values == nil || values.Len() == 0 {
		return nil
	}
	result := make([]gocoreobject.TypedName, 0, values.Len())
	for position := 0; position < values.Len(); position++ {
		value := values.At(position)
		typed := gocoreobject.TypedName{Name: value.Name(), Type: types.TypeString(value.Type(), packageQualifier)}
		if typed.Name == "_" {
			typed.Name = ""
		}
		carried := value.Type()
		for {
			switch inner := carried.(type) {
			case *types.Pointer:
				carried = inner.Elem()
				continue
			case *types.Slice:
				carried = inner.Elem()
				continue
			case *types.Array:
				carried = inner.Elem()
				continue
			}
			break
		}
		if named, ok := types.Unalias(carried).(*types.Named); ok && named.Obj() != nil && named.Obj().Pkg() != nil {
			typed.Package, typed.TypeName = named.Obj().Pkg().Path(), named.Obj().Name()
		}
		result = append(result, typed)
	}
	return result
}

func coreObjectTypeKind(object *types.TypeName) gocoreobject.TypeKind {
	if object == nil {
		return gocoreobject.TypeNamed
	}
	if object.IsAlias() {
		return gocoreobject.TypeAlias
	}
	named, ok := object.Type().(*types.Named)
	if !ok {
		return gocoreobject.TypeNamed
	}
	switch named.Underlying().(type) {
	case *types.Struct:
		return gocoreobject.TypeStruct
	case *types.Interface:
		return gocoreobject.TypeInterface
	default:
		return gocoreobject.TypeNamed
	}
}

func (a *analyzer) coreObjectLocation(position token.Pos) (gocoreobject.Location, error) {
	location := a.location(position)
	if !validRepositoryDirectCallLocation(location) || location.Column <= 0 {
		return gocoreobject.Location{}, fmt.Errorf("go core object index: declaration has no repository-local location")
	}
	return gocoreobject.Location{
		Path: location.Path, Line: location.Line, Column: location.Column,
	}, nil
}
