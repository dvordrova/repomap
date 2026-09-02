package surfacediscovery

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

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
	return gocoreobject.New(input)
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
					Exported: object.Exported(), Location: location,
				})
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
				Location: location, DirectCallNodeID: directCallNodeID,
			})
		}
	}
	return nil
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
