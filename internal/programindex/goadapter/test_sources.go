package goadapter

import (
	"fmt"
	"strconv"

	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/programindex"
)

// projectTestSources adds parsed declarations to the existing neutral index.
// They have no callable seeds, public link identities or inferred call edges.
// Test-only packages are source containers, not additional product targets.
func (projection *goProjection) projectTestSources(sources []gofacts.TestSource) error {
	types := make(map[string]string)
	for _, object := range projection.objects {
		if object.Kind == programindex.ObjectType {
			types[object.ContainerRef+"\x00"+object.Name] = object.SourceRef
		}
	}
	packages := make(map[string]string)
	// Discovery and the typed producer own different module-ID namespaces.
	// Join only their exact declared module path and repository directory.
	modules := make(map[string]string)
	for _, pkg := range projection.core.Packages {
		modules[pkg.Module+"\x00"+pkg.ModuleDir] = pkg.ModuleID
	}
	for _, source := range sources {
		if source.ModuleID != projection.target.ModuleID {
			return fmt.Errorf("Go test source %s belongs to another component module", source.Path)
		}
		if _, ok := projection.repository.ID(source.Path); !ok {
			return fmt.Errorf("Go test source %s is outside the corpus", source.Path)
		}
		if !source.DeclarationsScanned {
			continue // unavailable source is retained in the producer inventory
		}
		if err := gofacts.ValidatePackageDeclarations(source.Declarations); err != nil {
			return err
		}
		moduleID := modules[source.ModulePath+"\x00"+source.ModuleDir]
		moduleRef := projection.moduleRefs[moduleID]
		if moduleRef == "" {
			return fmt.Errorf("Go test source %s has no exact module container", source.Path)
		}
		packageRef := projection.packageRefs[packageKey(moduleID, source.PackagePath)]
		if source.External || packageRef == "" {
			key := source.PackagePath + "\x00" + source.PackageName
			packageRef = packages[key]
			if packageRef == "" {
				packageRef = stableRef("go-test-package", source.ModuleID, source.PackagePath, source.PackageName)
				if err := projection.addObject(programindex.ObjectInput{SourceRef: packageRef, Kind: programindex.ObjectPackage,
					Name: source.PackageName, Visibility: programindex.VisibilityInternal, OwnerRef: moduleRef, ContainerRef: moduleRef}); err != nil {
					return err
				}
				projection.addContains(moduleRef, packageRef, source.PackageName, nil)
				packages[key] = packageRef
			}
		}
		packages[source.Path] = packageRef
		// Receivers may be declared in a later test file. Index all types before
		// projecting methods; source order never decides receiver ownership.
		for _, declaration := range source.Declarations {
			if declaration.Kind == gofacts.PackageDeclarationType {
				types[packageRef+"\x00"+declaration.Name] = testDeclarationRef(declaration)
			}
		}
	}
	for _, source := range sources {
		if !source.DeclarationsScanned {
			continue
		}
		for _, declaration := range source.Declarations {
			if declaration.Path != source.Path || declaration.Line < 1 || declaration.Column < 1 {
				return fmt.Errorf("Go test declaration %s has no exact source", declaration.Label())
			}
			owner := packages[source.Path]
			kind := programindex.ObjectFunction
			switch declaration.Kind {
			case gofacts.PackageDeclarationType:
				kind = programindex.ObjectType
			case gofacts.PackageDeclarationConst, gofacts.PackageDeclarationVar:
				kind = programindex.ObjectVariable
			case gofacts.PackageDeclarationMethod:
				kind = programindex.ObjectMethod
				owner = types[owner+"\x00"+declaration.Receiver]
				if owner == "" {
					return fmt.Errorf("Go test method %s in %s has no declared receiver type", declaration.Label(), source.Path)
				}
			}
			ref := testDeclarationRef(declaration)
			location := &programindex.Location{Path: source.Path, Line: declaration.Line, Column: declaration.Column}
			if err := projection.addObject(programindex.ObjectInput{SourceRef: ref, Kind: kind, Name: declaration.Name,
				Visibility: programindex.VisibilityInternal, OwnerRef: owner, ContainerRef: owner, Location: location}); err != nil {
				return err
			}
			projection.relations = append(projection.relations, programindex.RelationInput{
				SourceRef: stableRef("go-test-contains", owner, ref), Kind: programindex.RelationContains,
				FromRef: owner, ToRefs: []string{ref}, Resolution: programindex.ResolutionExact, TargetsObserved: 1,
				Location: location, WitnessesObserved: 1, Witnesses: []programindex.Witness{{Kind: "go_test_declaration",
					Detail: "build-selected test source; parsed declaration; calls not analyzed", Location: location}},
			})
		}
	}
	return nil
}

func testDeclarationRef(declaration gofacts.PackageDeclaration) string {
	return stableRef("go-test-declaration", declaration.Path, strconv.Itoa(declaration.Line), strconv.Itoa(declaration.Column), string(declaration.Kind), declaration.Label())
}
