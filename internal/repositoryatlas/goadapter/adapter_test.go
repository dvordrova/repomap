package goadapter

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

func TestProjectBuildsTruthfulProcessEntrySlice(t *testing.T) {
	atlas, err := Project(singleAppInput(
		"fixture", "module-fixture", "example.com/fixture", ".",
		"example.com/fixture/cmd/app", "cmd/app", "cmd/app/main.go", 7, "trigger-app",
	))
	if err != nil {
		t.Fatal(err)
	}

	if len(atlas.Units) != 4 || len(atlas.Entities) != 2 || len(atlas.Evidence) != 1 ||
		len(atlas.Observations) != 2 || len(atlas.Relations) != 1 {
		t.Fatalf("Atlas shape = units %d, entities %d, evidence %d, observations %d, relations %d",
			len(atlas.Units), len(atlas.Entities), len(atlas.Evidence), len(atlas.Observations), len(atlas.Relations))
	}
	module := unitOfKind(t, atlas, repositoryatlas.UnitModule)
	app := unitOfKind(t, atlas, repositoryatlas.UnitApp)
	pkg := unitOfKind(t, atlas, repositoryatlas.UnitPackage)
	if app.ParentID != module.ID || pkg.ParentID != module.ID {
		t.Fatalf("app/package parents = %q/%q, want sibling children of %q", app.ParentID, pkg.ParentID, module.ID)
	}
	if app.ParentID == pkg.ID || pkg.ParentID == app.ID {
		t.Fatal("adapter invented app/package containment")
	}

	surface := entityOfKind(t, atlas, repositoryatlas.EntitySurface)
	operation := entityOfKind(t, atlas, repositoryatlas.EntityOperation)
	if surface.ID != "trigger-app" || surface.UnitID != app.ID || operation.UnitID != app.ID {
		t.Fatalf("process slice entities = %#v / %#v, app = %#v", surface, operation, app)
	}
	item := atlas.Evidence[0]
	if item.UnitID != app.ID || item.Location.Path != "cmd/app/main.go" || item.Location.Line != 7 ||
		item.Symbol != "example.com/fixture/cmd/app.main" {
		t.Fatalf("exact process evidence = %#v", item)
	}
	relation := atlas.Relations[0]
	if relation.UnitID != app.ID || relation.Kind != repositoryatlas.RelationExposes ||
		relation.Source != (repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: surface.ID}) ||
		relation.Target != (repositoryatlas.EntityRef{Kind: repositoryatlas.EntityOperation, ID: operation.ID}) ||
		relation.Phase != repositoryatlas.PhaseStartup || relation.Authority != repositoryatlas.AuthorityResolved {
		t.Fatalf("process relation = %#v", relation)
	}
	for _, entity := range atlas.Entities {
		encoded, marshalErr := json.Marshal(entity)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if bytes.Contains(encoded, []byte("main.go")) || bytes.Contains(encoded, []byte(".main")) ||
			bytes.Contains(encoded, []byte(`"path"`)) || bytes.Contains(encoded, []byte(`"symbol"`)) {
			t.Fatalf("source locator was promoted to entity: %s", encoded)
		}
	}
	for _, entity := range atlas.Entities {
		if entity.Kind == repositoryatlas.EntityBoundary || entity.Kind == repositoryatlas.EntityResource ||
			entity.Kind == repositoryatlas.EntityContract {
			t.Fatalf("unproved entity was emitted: %#v", entity)
		}
	}
}

func TestProjectUsesSameCanonicalShapeAcrossFixtures(t *testing.T) {
	left, err := Project(singleAppInput(
		"alpha", "module-alpha", "example.com/alpha", ".",
		"example.com/alpha/cmd/alpha", "cmd/alpha", "cmd/alpha/main.go", 5, "trigger-alpha",
	))
	if err != nil {
		t.Fatal(err)
	}
	right, err := Project(singleAppInput(
		"beta", "module-beta", "example.net/beta", "service",
		"example.net/beta/service", "service", "service/main.go", 19, "trigger-beta",
	))
	if err != nil {
		t.Fatal(err)
	}
	leftJSON, err := repositoryatlas.CanonicalJSON(left)
	if err != nil {
		t.Fatal(err)
	}
	rightJSON, err := repositoryatlas.CanonicalJSON(right)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(canonicalJSONShape(t, leftJSON), canonicalJSONShape(t, rightJSON)) {
		t.Fatalf("canonical fixture shapes differ:\n%s\n%s", leftJSON, rightJSON)
	}
}

func TestProjectSeparatesMixedMultiModuleUnitsAndIsOrderDeterministic(t *testing.T) {
	alpha := singleAppInput(
		"workspace", "module-alpha", "example.com/alpha", "alpha",
		"example.com/alpha/cmd/app", "alpha/cmd/app", "alpha/cmd/app/main.go", 7, "trigger-alpha",
	)
	beta := singleAppInput(
		"workspace", "module-beta", "example.com/beta", "beta",
		"example.com/beta/cmd/app", "beta/cmd/app", "beta/cmd/app/main.go", 11, "trigger-beta",
	)
	input := Input{RepositoryName: "workspace"}
	input.Facts.Modules = append(input.Facts.Modules, alpha.Facts.Modules...)
	input.Facts.Modules = append(input.Facts.Modules, beta.Facts.Modules...)
	input.Facts.Packages = append(input.Facts.Packages, alpha.Facts.Packages...)
	input.Facts.Packages = append(input.Facts.Packages, beta.Facts.Packages...)
	input.Facts.Packages = append(input.Facts.Packages,
		gofacts.PackageFact{
			CanonicalPath: "example.com/alpha/internal/lib", Name: "lib", ModuleID: "module-alpha",
			ModulePath: "example.com/alpha", PackageDir: "alpha/internal/lib", ModuleRelativeDir: "internal/lib",
		},
	)
	input.Facts.EntrypointPackages = append(input.Facts.EntrypointPackages, alpha.Facts.EntrypointPackages...)
	input.Facts.EntrypointPackages = append(input.Facts.EntrypointPackages, beta.Facts.EntrypointPackages...)
	input.Catalog.Triggers = append(input.Catalog.Triggers, alpha.Catalog.Triggers...)
	input.Catalog.Triggers = append(input.Catalog.Triggers, beta.Catalog.Triggers...)

	forward, err := Project(input)
	if err != nil {
		t.Fatal(err)
	}
	reversedInput := input
	reversedInput.Facts.Modules = reverseCopy(input.Facts.Modules)
	reversedInput.Facts.Packages = reverseCopy(input.Facts.Packages)
	reversedInput.Facts.EntrypointPackages = reverseCopy(input.Facts.EntrypointPackages)
	reversedInput.Catalog.Triggers = reverseCopy(input.Catalog.Triggers)
	reversed, err := Project(reversedInput)
	if err != nil {
		t.Fatal(err)
	}
	forwardJSON, err := repositoryatlas.CanonicalJSON(forward)
	if err != nil {
		t.Fatal(err)
	}
	reversedJSON, err := repositoryatlas.CanonicalJSON(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(forwardJSON, reversedJSON) {
		t.Fatalf("projection depends on source ordering:\n%s\n%s", forwardJSON, reversedJSON)
	}

	moduleByName := make(map[string]repositoryatlas.Unit)
	appParentByName := make(map[string]string)
	packageParentByName := make(map[string]string)
	for _, unit := range forward.Units {
		switch unit.Kind {
		case repositoryatlas.UnitModule:
			moduleByName[unit.Name] = unit
		case repositoryatlas.UnitApp:
			appParentByName[unit.Name] = unit.ParentID
		case repositoryatlas.UnitPackage:
			packageParentByName[unit.Name] = unit.ParentID
		}
	}
	if appParentByName["example.com/alpha/cmd/app"] != moduleByName["example.com/alpha"].ID ||
		appParentByName["example.com/beta/cmd/app"] != moduleByName["example.com/beta"].ID ||
		packageParentByName["example.com/alpha/internal/lib"] != moduleByName["example.com/alpha"].ID {
		t.Fatalf("multi-module ownership crossed units: apps %#v, packages %#v, modules %#v",
			appParentByName, packageParentByName, moduleByName)
	}
	entityUnits := make(map[string]struct{})
	for _, entity := range forward.Entities {
		entityUnits[entity.UnitID] = struct{}{}
	}
	if len(entityUnits) != 2 {
		t.Fatalf("multi-module process slices share a Unit: %#v", entityUnits)
	}
}

func TestProjectOmitsUnprovedProcessSlice(t *testing.T) {
	input := singleAppInput(
		"fixture", "module-fixture", "example.com/fixture", ".",
		"example.com/fixture/cmd/app", "cmd/app", "cmd/app/main.go", 7, "trigger-app",
	)
	input.Catalog.Triggers[0].Provenance = nil
	atlas, err := Project(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(atlas.Entities) != 0 || len(atlas.Evidence) != 0 || len(atlas.Observations) != 0 || len(atlas.Relations) != 0 {
		t.Fatalf("unproved process slice was emitted: %#v", atlas)
	}
	if unitOfKind(t, atlas, repositoryatlas.UnitApp).ParentID != "module-fixture" {
		t.Fatal("authoritative app topology disappeared with an absent surface proof")
	}
}

func singleAppInput(
	repositoryName, moduleID, modulePath, moduleDir, packagePath, packageDir, sourcePath string,
	line int,
	triggerID string,
) Input {
	anchor := gofacts.EntrypointAnchor{
		Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
		Path: sourcePath, Line: line,
	}
	entrypoint := gofacts.Entrypoint{
		ModulePath: modulePath, ImportPath: packagePath, Dir: packageDir,
		PackageDir: packageDir, ModuleRelativeDir: packageDir, ModuleDir: moduleDir,
		Anchors: []gofacts.EntrypointAnchor{anchor},
	}
	location := surfacediscovery.Location{Path: sourcePath, Line: line}
	return Input{
		RepositoryName: repositoryName,
		Facts: gofacts.Facts{
			Modules: []gofacts.ModuleFact{{ID: moduleID, ModulePath: modulePath, ModuleDir: moduleDir}},
			Packages: []gofacts.PackageFact{{
				CanonicalPath: packagePath, Name: "main", ModuleID: moduleID,
				ModulePath: modulePath, PackageDir: packageDir, ModuleRelativeDir: packageDir,
			}},
			EntrypointPackages: []gofacts.Entrypoint{entrypoint},
		},
		Catalog: surfacediscovery.TriggerCatalog{Triggers: []surfacediscovery.TriggerRecord{{
			ID: triggerID, Kind: "process_entry", Identity: surfacediscovery.Identity{Name: "main"},
			ProcessEntrypoint: surfacediscovery.Symbol{
				ID: packagePath + ".main", Package: packagePath, Name: "main", Location: location,
			},
			RegistrationSite: location, Certainty: "static", Resolution: "exact",
			Evidence: []surfacediscovery.Evidence{{
				ID: "process-entry:" + sourcePath, Kind: "process_entry_declaration", Location: location,
			}},
			Provenance: []surfacediscovery.Provenance{{
				Provider: "gofacts", Version: "entrypoint-anchor-v1", Operation: "build_selected_main_declaration",
			}},
		}}},
	}
}

func unitOfKind(t *testing.T, atlas repositoryatlas.Atlas, kind repositoryatlas.UnitKind) repositoryatlas.Unit {
	t.Helper()
	var matches []repositoryatlas.Unit
	for _, unit := range atlas.Units {
		if unit.Kind == kind {
			matches = append(matches, unit)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("units of kind %q = %#v", kind, matches)
	}
	return matches[0]
}

func entityOfKind(t *testing.T, atlas repositoryatlas.Atlas, kind repositoryatlas.EntityKind) repositoryatlas.Entity {
	t.Helper()
	for _, entity := range atlas.Entities {
		if entity.Kind == kind {
			return entity
		}
	}
	t.Fatalf("entity of kind %q not found", kind)
	return repositoryatlas.Entity{}
}

func canonicalJSONShape(t *testing.T, encoded []byte) any {
	t.Helper()
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatal(err)
	}
	return scalarIndependentShape(value)
}

func scalarIndependentShape(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		result := make([]any, 0, len(keys)*2)
		for _, key := range keys {
			result = append(result, key, scalarIndependentShape(typed[key]))
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = scalarIndependentShape(item)
		}
		return result
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	default:
		return "unknown"
	}
}

func reverseCopy[T any](values []T) []T {
	result := append([]T(nil), values...)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}
