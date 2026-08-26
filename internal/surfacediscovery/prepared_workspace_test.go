package surfacediscovery

import (
	"context"
	"strings"
	"testing"
)

func TestPreparedWorkspaceKeepsCrossModuleReplacementInOneTypedUniverse(t *testing.T) {
	repository := writePreparedWorkspaceModules(t)
	input := preparedWorkspaceAppInput()
	options := defaultHostOptions(repository)
	options.DirectCallDepth = 2

	workspace, err := PrepareWorkspace(context.Background(), options, []Input{input})
	if err != nil {
		t.Fatal(err)
	}
	if workspace.packageLoadCalls != 1 || workspace.ssaBuilds != 1 || len(workspace.universes) != 1 {
		t.Fatalf(
			"prepared work = loads %d, SSA builds %d, universes %d; want one coherent load/build universe",
			workspace.packageLoadCalls, workspace.ssaBuilds, len(workspace.universes),
		)
	}
	binding := workspace.bindings[input.AnalysisTarget.TargetRef]
	universe := workspace.universes[binding.universeKey]
	for _, packagePath := range []string{"example.com/app", "example.com/lib"} {
		facts := universe.packageFacts[packagePath]
		ssaPackage := universe.ssaPackages[packagePath]
		if facts == nil || ssaPackage == nil || facts.Types != ssaPackage.Pkg {
			t.Fatalf(
				"package %q crossed packages/types/SSA universes: facts=%p types=%p SSA=%p",
				packagePath, facts, func() any {
					if facts == nil {
						return nil
					}
					return facts.Types
				}(), ssaPackage,
			)
		}
	}

	result, err := workspace.Analyze(context.Background(), options, input)
	if err != nil {
		t.Fatal(err)
	}
	wantEdges := map[string]bool{"main->Run": true, "Run->helper": true}
	gotEdges := targetDirectCallEdgeNames(result.DirectCallIndex)
	for edge := range wantEdges {
		if !gotEdges[edge] {
			t.Fatalf("cross-module edge %q is absent from %v", edge, gotEdges)
		}
	}

	// The diagnostic is a separate contract: if a future internal regression
	// breaks this exact identity join, users see bounded expected/resolved keys
	// and are not told to rewrite a valid repository.
	delete(universe.ssaPackages, "example.com/lib")
	_, err = workspace.Analyze(context.Background(), options, input)
	if err == nil || !strings.Contains(err.Error(), "expected=2 resolved=1") ||
		!strings.Contains(err.Error(), `"lib:example.com/lib"`) ||
		!strings.Contains(err.Error(), "internal repomap package/type/SSA identity invariant failed") ||
		!strings.Contains(err.Error(), "repository changes are not required") {
		t.Fatalf("prepared projection diagnostic = %v", err)
	}
}

func TestPreparedWorkspaceSeparatesIncompatibleModuleLoadContexts(t *testing.T) {
	repository := writePreparedWorkspaceModules(t)
	app := preparedWorkspaceAppInput()
	library := Input{
		ModuleDirs: []string{"lib"},
		Packages:   []PackageInput{{Path: "example.com/lib", ModuleDir: "lib"}},
		AnalysisTarget: &AnalysisTargetInput{
			TargetRef: "target-lib", Kind: AnalysisTargetModuleLibrary,
			ModuleID: "module-lib", ModulePath: "example.com/lib", ModuleDir: "lib",
			TargetPackages: []string{"example.com/lib"},
		},
	}
	options := defaultHostOptions(repository)
	workspace, err := PrepareWorkspace(context.Background(), options, []Input{app, library})
	if err != nil {
		t.Fatal(err)
	}
	if workspace.packageLoadCalls != 2 || workspace.ssaBuilds != 2 || len(workspace.universes) != 2 {
		t.Fatalf(
			"prepared incompatible contexts = loads %d, SSA builds %d, universes %d; want two",
			workspace.packageLoadCalls, workspace.ssaBuilds, len(workspace.universes),
		)
	}
	appUniverse := workspace.bindings[app.AnalysisTarget.TargetRef].universeKey
	libraryUniverse := workspace.bindings[library.AnalysisTarget.TargetRef].universeKey
	if appUniverse == libraryUniverse {
		t.Fatal("different module resolution contexts shared one typed universe")
	}
	if _, err := workspace.Analyze(context.Background(), options, app); err != nil {
		t.Fatalf("analyze app universe: %v", err)
	}
	if _, err := workspace.Analyze(context.Background(), options, library); err != nil {
		t.Fatalf("analyze library universe: %v", err)
	}
}

func TestPreparedProjectionDiagnosticKeysAreBoundedAndExact(t *testing.T) {
	values := []string{
		"m:k00", "m:k01", "m:k02", "m:k03", "m:k04",
		"m:k05", "m:k06", "m:k07", "m:k08", "m:k09",
	}
	got := boundedPreparedKeys(values)
	if !strings.Contains(got, `"m:k00"`) || !strings.Contains(got, `"m:k07"`) ||
		strings.Contains(got, `"m:k08"`) || !strings.Contains(got, "+2 more") {
		t.Fatalf("bounded exact package keys = %s", got)
	}
}

func writePreparedWorkspaceModules(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	writeTargetScopeFile(t, repository, "app/go.mod", `module example.com/app

go 1.24

require example.com/lib v0.0.0

replace example.com/lib => ../lib
`)
	writeTargetScopeFile(t, repository, "app/main.go", `package main

import "example.com/lib"

func main() { lib.Run() }
`)
	writeTargetScopeFile(t, repository, "lib/go.mod", `module example.com/lib

go 1.24
`)
	writeTargetScopeFile(t, repository, "lib/lib.go", `package lib

func Run() { helper() }
func helper() {}
`)
	return repository
}

func preparedWorkspaceAppInput() Input {
	return Input{
		ModuleDirs: []string{"app", "lib"},
		Packages: []PackageInput{
			{Path: "example.com/app", ModuleDir: "app"},
			{Path: "example.com/lib", ModuleDir: "lib"},
		},
		AnalysisTarget: &AnalysisTargetInput{
			TargetRef: "target-app", Kind: AnalysisTargetExecutablePackage,
			ModuleID: "module-app", ModulePath: "example.com/app", ModuleDir: "app",
			PackagePath: "example.com/app", TargetPackages: []string{"example.com/app"},
			Roots: []AnalysisTargetRootInput{{Path: "app/main.go", Line: 5}},
		},
	}
}
