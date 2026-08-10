package analysistarget

import (
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestScopeGoFactsRepomapExecutableDropsOtherCommands(t *testing.T) {
	facts := syntheticFacts("module-root", "github.com/dvordrova/repomap", []syntheticPackage{
		{path: "github.com/dvordrova/repomap/cmd/quality-evaluate", dir: "cmd/quality-evaluate", executable: true, line: 16},
		{path: "github.com/dvordrova/repomap/cmd/repomap", dir: "cmd/repomap", executable: true, line: 43},
		{path: "github.com/dvordrova/repomap/internal/report", dir: "internal/report"},
	})
	facts.InternalEdges = []gofacts.Edge{
		{From: "github.com/dvordrova/repomap/cmd/repomap", To: "github.com/dvordrova/repomap/internal/report"},
	}
	resolution, err := Resolve(facts, Options{})
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := ScopeGoFacts(facts, *resolution.Selected)
	if err != nil {
		t.Fatal(err)
	}
	assertPackagePaths(t, scoped, []string{
		"github.com/dvordrova/repomap/cmd/repomap",
		"github.com/dvordrova/repomap/internal/report",
	})
	if len(scoped.EntrypointPackages) != 1 || scoped.EntrypointPackages[0].ImportPath != resolution.Selected.PackagePath {
		t.Fatalf("entrypoints = %#v", scoped.EntrypointPackages)
	}
	if len(facts.Packages) != 3 {
		t.Fatal("ScopeGoFacts mutated its input")
	}
}

func TestScopeGoFactsMobyOverrideAndTelebotRootLibrary(t *testing.T) {
	moby := syntheticFacts("module-root", "github.com/moby/moby/v2", []syntheticPackage{
		{path: "github.com/moby/moby/v2/cmd/docker-proxy", dir: "cmd/docker-proxy", executable: true, line: 20},
		{path: "github.com/moby/moby/v2/cmd/dockerd", dir: "cmd/dockerd", executable: true, line: 16},
		{path: "github.com/moby/moby/v2/daemon", dir: "daemon"},
	})
	moby.InternalEdges = []gofacts.Edge{{From: "github.com/moby/moby/v2/cmd/dockerd", To: "github.com/moby/moby/v2/daemon"}}
	selected, err := Resolve(moby, Options{Override: "cmd/dockerd"})
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := ScopeGoFacts(moby, *selected.Selected)
	if err != nil {
		t.Fatal(err)
	}
	assertPackagePaths(t, scoped, []string{"github.com/moby/moby/v2/cmd/dockerd", "github.com/moby/moby/v2/daemon"})

	telebot := syntheticFacts("module-root", "gopkg.in/telebot.v3", []syntheticPackage{
		{path: "gopkg.in/telebot.v3", dir: "."},
		{path: "gopkg.in/telebot.v3/layout", dir: "layout"},
		{path: "gopkg.in/telebot.v3/middleware", dir: "middleware"},
	})
	library, err := Resolve(telebot, Options{})
	if err != nil {
		t.Fatal(err)
	}
	telebotScoped, err := ScopeGoFacts(telebot, *library.Selected)
	if err != nil {
		t.Fatal(err)
	}
	assertPackagePaths(t, telebotScoped, []string{"gopkg.in/telebot.v3", "gopkg.in/telebot.v3/layout", "gopkg.in/telebot.v3/middleware"})
	if len(telebotScoped.EntrypointPackages) != 0 {
		t.Fatalf("library entrypoints = %#v", telebotScoped.EntrypointPackages)
	}
}

func TestTargetValidateRejectsRefDrift(t *testing.T) {
	facts := syntheticFacts("module-root", "gopkg.in/telebot.v3", []syntheticPackage{{path: "gopkg.in/telebot.v3", dir: "."}})
	resolution, err := Resolve(facts, Options{})
	if err != nil {
		t.Fatal(err)
	}
	target := resolution.Selected.Snapshot()
	if _, err := target.CanonicalJSON(); err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	target.PackagePath += "/drift"
	if err := target.Validate(); err == nil {
		t.Fatal("Validate accepted a drifted self-seal")
	}
}

func TestScopeGoFactsModuleLibraryUsesExactNonMainModuleInventory(t *testing.T) {
	facts := syntheticFacts("module-root", "example.com/mixed", []syntheticPackage{
		{path: "example.com/mixed", dir: ".", executable: true, line: 4},
		{path: "example.com/mixed/api", dir: "api"},
		{path: "example.com/mixed/internal/store", dir: "internal/store"},
	})
	candidates, err := Candidates(facts)
	if err != nil {
		t.Fatal(err)
	}
	var target Target
	for _, candidate := range candidates {
		if candidate.Target.Kind == KindModuleLibrary {
			target = candidate.Target
		}
	}
	if target.Ref == "" {
		t.Fatalf("no module-library candidate: %#v", candidates)
	}

	scoped, err := ScopeGoFacts(facts, target)
	if err != nil {
		t.Fatal(err)
	}
	assertPackagePaths(t, scoped, []string{
		"example.com/mixed/api",
		"example.com/mixed/internal/store",
	})
	if len(scoped.EntrypointPackages) != 0 || len(scoped.Modules) != 1 ||
		scoped.Modules[0].PackagesCount != 2 {
		t.Fatalf("module-library scope = %#v", scoped)
	}

	driftedFacts := facts
	driftedFacts.Packages = append([]gofacts.PackageFact(nil), facts.Packages[:2]...)
	if _, err := ScopeGoFacts(driftedFacts, target); err == nil ||
		!strings.Contains(err.Error(), "module package inventory mismatch") {
		t.Fatalf("scope accepted drifted module inventory: %v", err)
	}
}

func assertPackagePaths(t *testing.T, facts gofacts.Facts, want []string) {
	t.Helper()
	got := make([]string, 0, len(facts.Packages))
	for _, pkg := range facts.Packages {
		got = append(got, pkg.CanonicalPath)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("packages = %#v, want %#v", got, want)
	}
}
