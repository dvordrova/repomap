package goadapter

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/dvordrova/repomap/internal/boundary"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/navigator"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestProjectAddsOneExactPackageDeclarationEvidence(t *testing.T) {
	input := singleAppInput(
		"fixture", "module-fixture", "example.com/fixture", ".",
		"example.com/fixture/cmd/app", "cmd/app", "cmd/app/main.go", 7, "trigger-app",
	)
	input.Facts.Packages[0].Files = []string{"cmd/app/z.go", "cmd/app/main.go"}
	input.PackageDeclarations = map[string]evidence.Location{
		"example.com/fixture/cmd/app": {Path: "cmd/app/main.go", Line: 1, Column: 1},
	}
	atlas, err := Project(input)
	if err != nil {
		t.Fatal(err)
	}
	packageUnit := unitOfKind(t, atlas, repositoryatlas.UnitPackage)
	var declarations []repositoryatlas.Evidence
	for _, item := range atlas.Evidence {
		if item.Provenance.Operation == PackageDeclarationEvidenceOperation {
			declarations = append(declarations, item)
		}
	}
	if len(declarations) != 1 {
		t.Fatalf("package declaration evidence = %#v", declarations)
	}
	item := declarations[0]
	if item.UnitID != packageUnit.ID || item.Symbol != "main" ||
		item.Location != (evidence.Location{Path: "cmd/app/main.go", Line: 1, Column: 1}) ||
		item.Provenance.Provider != PackageDeclarationEvidenceProvider ||
		item.Provenance.Version != PackageDeclarationEvidenceVersion ||
		item.Provenance.Operation != "package_declaration" ||
		!IsPersistedPackageDeclarationEvidence(item, packageUnit) ||
		!IsExactPackageDeclarationEvidence(item, packageUnit, input.Facts.Packages[0]) {
		t.Fatalf("package declaration evidence = %#v, unit = %#v", item, packageUnit)
	}

	tampered := item
	tampered.Provenance.Operation = "package-declaration"
	if IsExactPackageDeclarationEvidence(tampered, packageUnit, input.Facts.Packages[0]) {
		t.Fatal("non-canonical provenance operation was accepted")
	}
	tamperedUnit := packageUnit
	tamperedUnit.Name = "example.com/fixture/cmd/other"
	if IsPersistedPackageDeclarationEvidence(item, tamperedUnit) {
		t.Fatal("package evidence survived a Unit identity change")
	}
}

func TestProjectOmitsInvalidOrAmbiguousPackageDeclarationsWithoutDroppingUnits(t *testing.T) {
	input := Input{
		RepositoryName: "fixture",
		Facts: gofacts.Facts{
			Modules: []gofacts.ModuleFact{{
				ID: "module-fixture", ModulePath: "example.com/fixture", ModuleDir: ".",
			}},
			Packages: []gofacts.PackageFact{
				{
					CanonicalPath: "example.com/fixture/alpha", Name: "alpha",
					ModuleID: "module-fixture", ModulePath: "example.com/fixture",
					Files: []string{"shared.go"},
				},
				{
					CanonicalPath: "example.com/fixture/beta", Name: "beta",
					ModuleID: "module-fixture", ModulePath: "example.com/fixture",
					Files: []string{"shared.go"},
				},
				{
					CanonicalPath: "example.com/fixture/gamma", Name: "gamma",
					ModuleID: "module-fixture", ModulePath: "example.com/fixture",
					Files: []string{"gamma.go"},
				},
			},
		},
		PackageDeclarations: map[string]evidence.Location{
			"example.com/fixture/alpha": {Path: "shared.go", Line: 1, Column: 1},
			"example.com/fixture/beta":  {Path: "shared.go", Line: 1, Column: 1},
			"example.com/fixture/gamma": {Path: "outside.go", Line: 1, Column: 1},
		},
	}
	atlas, err := Project(input)
	if err != nil {
		t.Fatal(err)
	}
	packageUnits := 0
	for _, unit := range atlas.Units {
		if unit.Kind == repositoryatlas.UnitPackage {
			packageUnits++
		}
	}
	if packageUnits != 3 {
		t.Fatalf("package Units = %d, want 3", packageUnits)
	}
	for _, item := range atlas.Evidence {
		if item.Provenance.Operation == PackageDeclarationEvidenceOperation {
			t.Fatalf("ambiguous or outside-package declaration was emitted: %#v", item)
		}
	}
}

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

func TestProjectKeepsExactAppWhenPackageRowsWereExplicitlyCapped(t *testing.T) {
	input := singleAppInput(
		"fixture", "module-fixture", "example.com/fixture", ".",
		"example.com/fixture/cmd/app", "cmd/app", "cmd/app/main.go", 7, "trigger-app",
	)
	input.Facts.Packages = nil
	atlas, err := Project(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(atlas.Entities) != 2 || len(atlas.Relations) != 1 {
		t.Fatalf("exact process slice disappeared with capped package rows: %#v", atlas)
	}
	app := unitOfKind(t, atlas, repositoryatlas.UnitApp)
	module := unitOfKind(t, atlas, repositoryatlas.UnitModule)
	if app.ParentID != module.ID || app.Name != "example.com/fixture/cmd/app" {
		t.Fatalf("exact app ownership = %#v / %#v", app, module)
	}
	for _, unit := range atlas.Units {
		if unit.Kind == repositoryatlas.UnitPackage {
			t.Fatalf("adapter invented capped package unit: %#v", unit)
		}
	}
}

func TestProjectStartupEligibilityUsesClosedRoleAndAvailabilityContract(t *testing.T) {
	markUnavailablePrimary := func(record *surfacediscovery.TriggerRecord) {
		record.Availability = surfacediscovery.AvailabilityUnavailable
		record.ApplicationClass = surfacediscovery.ApplicationSurface
		record.SurfaceRole = surfacediscovery.SurfaceRoleEntrySurface
		record.TraceReadiness = surfacediscovery.TraceReadinessPartial
	}
	tests := []struct {
		name   string
		mutate func(*surfacediscovery.TriggerRecord)
		want   bool
	}{
		{name: "available primary application", want: true},
		{name: "available secondary service", mutate: func(record *surfacediscovery.TriggerRecord) {
			record.ExecutableRole = surfacediscovery.ExecutableRoleSecondaryService
		}, want: true},
		{name: "available tooling", mutate: func(record *surfacediscovery.TriggerRecord) {
			record.ExecutableRole = surfacediscovery.ExecutableRoleTooling
		}},
		{name: "available test helper", mutate: func(record *surfacediscovery.TriggerRecord) {
			record.ExecutableRole = surfacediscovery.ExecutableRoleTestOrHelper
		}},
		{name: "available unknown role", mutate: func(record *surfacediscovery.TriggerRecord) {
			record.ExecutableRole = surfacediscovery.ExecutableRoleUnknown
		}},
		{name: "available missing role", mutate: func(record *surfacediscovery.TriggerRecord) {
			record.ExecutableRole = ""
		}},
		{name: "unknown availability", mutate: func(record *surfacediscovery.TriggerRecord) {
			record.Availability = surfacediscovery.AvailabilityUnknown
		}},
		{name: "unavailable exact primary partial entry", mutate: markUnavailablePrimary, want: true},
		{name: "unavailable primary without application ownership", mutate: func(record *surfacediscovery.TriggerRecord) {
			markUnavailablePrimary(record)
			record.ApplicationClass = surfacediscovery.DependencyOnly
		}},
		{name: "unavailable primary without entry surface", mutate: func(record *surfacediscovery.TriggerRecord) {
			markUnavailablePrimary(record)
			record.SurfaceRole = surfacediscovery.SurfaceRoleRejected
		}},
		{name: "unavailable primary without partial readiness", mutate: func(record *surfacediscovery.TriggerRecord) {
			markUnavailablePrimary(record)
			record.TraceReadiness = surfacediscovery.TraceReadinessUnsupported
		}},
		{name: "unavailable secondary service", mutate: func(record *surfacediscovery.TriggerRecord) {
			markUnavailablePrimary(record)
			record.ExecutableRole = surfacediscovery.ExecutableRoleSecondaryService
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := singleAppInput(
				"fixture", "module-fixture", "example.com/fixture", ".",
				"example.com/fixture/cmd/app", "cmd/app", "cmd/app/main.go", 7, "trigger-app",
			)
			if test.mutate != nil {
				test.mutate(&input.Catalog.Triggers[0])
			}
			atlas, err := Project(input)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(atlas.Relations); got != boolCount(test.want) {
				t.Fatalf("startup relations = %d, want eligible=%t; Atlas = %#v", got, test.want, atlas)
			}
			if test.want && (len(atlas.Entities) != 2 || len(atlas.Evidence) != 1 || len(atlas.Observations) != 2) {
				t.Fatalf("eligible process entry Atlas vertical = %#v", atlas)
			}
			if !test.want && (len(atlas.Entities) != 0 || len(atlas.Evidence) != 0 || len(atlas.Observations) != 0) {
				t.Fatalf("ineligible process entry became Atlas vertical: %#v", atlas)
			}
		})
	}
}

func TestGotifyLikeUnavailablePrimaryIsOnlyNavigatorStartupAction(t *testing.T) {
	root := singleAppInput(
		"server", "module-server", "example.com/server", ".",
		"example.com/server", ".", "app.go", 35, "trigger-root",
	)
	root.Catalog.Triggers[0].Availability = surfacediscovery.AvailabilityUnavailable
	root.Catalog.Triggers[0].UnavailableReason = "package or dependency closure is ill-typed under the recorded build scenario"
	root.Catalog.Triggers[0].ApplicationClass = surfacediscovery.ApplicationSurface
	root.Catalog.Triggers[0].SurfaceRole = surfacediscovery.SurfaceRoleEntrySurface
	root.Catalog.Triggers[0].TraceReadiness = surfacediscovery.TraceReadinessPartial

	example := singleAppInput(
		"server", "module-server", "example.com/server", ".",
		"example.com/server/plugin/example/echo", "plugin/example/echo", "plugin/example/echo/main.go", 21, "trigger-example",
	)
	example.Catalog.Triggers[0].ExecutableRole = surfacediscovery.ExecutableRoleTooling
	broken := singleAppInput(
		"server", "module-server", "example.com/server", ".",
		"example.com/server/plugin/testing/broken", "plugin/testing/broken", "plugin/testing/broken/main.go", 34, "trigger-broken",
	)
	broken.Catalog.Triggers[0].ExecutableRole = surfacediscovery.ExecutableRoleTestOrHelper

	root.Facts.Packages = append(root.Facts.Packages, example.Facts.Packages[0], broken.Facts.Packages[0])
	root.Facts.EntrypointPackages = append(
		root.Facts.EntrypointPackages,
		example.Facts.EntrypointPackages[0],
		broken.Facts.EntrypointPackages[0],
	)
	root.Catalog.Triggers = append(root.Catalog.Triggers, example.Catalog.Triggers[0], broken.Catalog.Triggers[0])

	atlas, err := Project(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(atlas.Relations) != 1 || len(atlas.Entities) != 2 || len(atlas.Evidence) != 1 ||
		atlas.Evidence[0].Location.Path != "app.go" || atlas.Evidence[0].Location.Line != 35 {
		t.Fatalf("Gotify-like startup Atlas = %#v", atlas)
	}

	product, err := navigator.CompileProduct(navigator.ProductInput{
		Atlas: atlas,
		Limits: navigator.Limits{
			MaxWireBytes: 1 << 20, MaxResponseBytes: 64 << 10, MaxUnitLabelBytes: 512,
			MaxSeeds: 16, MaxDirectTrails: 16, MaxIntersections: 16,
			MaxEvidence: 32, MaxGaps: 0, MaxActions: 16,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request, err := product.RequestRecord()
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Actions) != 1 || request.Actions[0].Surface.ID != "trigger-root" ||
		len(request.Actions[0].EvidenceIDs) != 1 || request.Actions[0].EvidenceIDs[0] != atlas.Evidence[0].ID {
		t.Fatalf("Navigator advertised actions = %#v", request.Actions)
	}

	var wire struct {
		Actions []struct {
			Ref string `json:"ref"`
		} `json:"actions"`
	}
	if err := json.Unmarshal([]byte(request.WireJSON), &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.Actions) != 1 || wire.Actions[0].Ref == "" {
		t.Fatalf("Navigator wire actions = %#v", wire.Actions)
	}
	response, err := json.Marshal(map[string]any{"action_refs": []string{wire.Actions[0].Ref}})
	if err != nil {
		t.Fatal(err)
	}
	selected, err := product.ResolveRecommendation(response)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Selected == nil || selected.Selected.Surface.ID != "trigger-root" ||
		len(selected.Selected.EvidenceIDs) != 1 || selected.Selected.EvidenceIDs[0] != atlas.Evidence[0].ID {
		t.Fatalf("Navigator selected recommendation = %#v", selected.Selected)
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
			ExecutableRole: surfacediscovery.ExecutableRolePrimaryApplication,
			Availability:   surfacediscovery.AvailabilityAvailable,
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

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
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

func TestProjectEmitsTypedBoundaryAndResourceEntitiesWithExactEvidence(t *testing.T) {
	input := singleAppInput(
		"fixture", "module-fixture", "example.com/fixture", ".",
		"example.com/fixture/cmd/app", "cmd/app", "cmd/app/main.go", 7, "trigger-app",
	)
	input.BoundaryObservations = []boundary.Observation{
		{
			Class: boundary.ClassPersistentStorage, ImportPath: "database/sql",
			PackagePath: "example.com/fixture/cmd/app",
			Location:    evidence.Location{Path: "cmd/app/main.go", Line: 41, Column: 9},
			Symbol:      "main",
		},
		{
			Class: boundary.ClassOutboundClient, ImportPath: "net/http",
			PackagePath: "example.com/fixture/cmd/app",
			Location:    evidence.Location{Path: "cmd/app/main.go", Line: 42, Column: 9},
			Symbol:      "main",
		},
	}
	atlas, err := Project(input)
	if err != nil {
		t.Fatal(err)
	}
	packageUnit := unitOfKind(t, atlas, repositoryatlas.UnitPackage)
	if packageUnit.ID == "" {
		t.Fatal("package unit missing")
	}

	var boundaryEntities, resourceEntities []repositoryatlas.Entity
	for _, entity := range atlas.Entities {
		switch entity.Kind {
		case repositoryatlas.EntityBoundary:
			boundaryEntities = append(boundaryEntities, entity)
		case repositoryatlas.EntityResource:
			resourceEntities = append(resourceEntities, entity)
		}
	}
	if len(boundaryEntities) != 2 {
		t.Fatalf("boundary entities = %#v", boundaryEntities)
	}
	if len(resourceEntities) != 2 {
		t.Fatalf("resource entities = %#v", resourceEntities)
	}
	for _, entity := range append(append([]repositoryatlas.Entity{}, boundaryEntities...), resourceEntities...) {
		if entity.UnitID != packageUnit.ID {
			t.Fatalf("boundary/resource entity %s not on package unit %s", entity.ID, packageUnit.ID)
		}
	}

	var boundaryEvidence []repositoryatlas.Evidence
	for _, item := range atlas.Evidence {
		if item.Provenance.Provider == BoundaryObservationEvidenceProvider {
			boundaryEvidence = append(boundaryEvidence, item)
		}
	}
	if len(boundaryEvidence) != 2 {
		t.Fatalf("boundary evidence = %#v", boundaryEvidence)
	}
	for _, item := range boundaryEvidence {
		if item.UnitID != packageUnit.ID || item.Location.Path != "cmd/app/main.go" ||
			item.Symbol != "main" || item.Provenance.Version != BoundaryObservationEvidenceVersion ||
			item.Provenance.Operation != BoundaryObservationEvidenceOperation {
			t.Fatalf("boundary evidence = %#v", item)
		}
	}

	boundaryObservationCount := 0
	resourceObservationCount := 0
	for _, observation := range atlas.Observations {
		switch observation.Subject.Kind {
		case repositoryatlas.EntityBoundary:
			boundaryObservationCount++
		case repositoryatlas.EntityResource:
			resourceObservationCount++
		default:
			// Pre-existing process-entry observations remain untouched.
			continue
		}
		if observation.UnitID != packageUnit.ID || len(observation.EvidenceRefs) != 1 {
			t.Fatalf("observation = %#v", observation)
		}
	}
	if boundaryObservationCount != 2 || resourceObservationCount != 2 {
		t.Fatalf("boundary observations=%d resource observations=%d", boundaryObservationCount, resourceObservationCount)
	}

	// Validation must accept the new entity kinds and observations.
	if err := atlas.Validate(); err != nil {
		t.Fatalf("validated Atlas with boundary entities: %v", err)
	}
}

func TestProjectOmitsBoundaryObservationOutsideKnownPackage(t *testing.T) {
	input := singleAppInput(
		"fixture", "module-fixture", "example.com/fixture", ".",
		"example.com/fixture/cmd/app", "cmd/app", "cmd/app/main.go", 7, "trigger-app",
	)
	input.BoundaryObservations = []boundary.Observation{
		{
			Class: boundary.ClassPersistentStorage, ImportPath: "database/sql",
			PackagePath: "example.com/unknown/package",
			Location:    evidence.Location{Path: "elsewhere/main.go", Line: 3, Column: 5},
			Symbol:      "Run",
		},
	}
	atlas, err := Project(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, entity := range atlas.Entities {
		if entity.Kind == repositoryatlas.EntityBoundary || entity.Kind == repositoryatlas.EntityResource {
			t.Fatalf("unexpected boundary/resource entity for unknown package: %#v", entity)
		}
	}
	for _, item := range atlas.Evidence {
		if item.Provenance.Provider == BoundaryObservationEvidenceProvider {
			t.Fatalf("unexpected boundary evidence for unknown package: %#v", item)
		}
	}
}
