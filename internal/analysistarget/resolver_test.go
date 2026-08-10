package analysistarget

import (
	"errors"
	"testing"

	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestResolveRepomapLikeMultipleCommands(t *testing.T) {
	facts := syntheticFacts(
		"module-root", "github.com/dvordrova/repomap",
		[]syntheticPackage{
			{path: "github.com/dvordrova/repomap/cmd/quality-evaluate", dir: "cmd/quality-evaluate", executable: true, line: 16},
			{path: "github.com/dvordrova/repomap/cmd/repomap", dir: "cmd/repomap", executable: true, line: 43},
			{path: "github.com/dvordrova/repomap/cmd/symbol-playground", dir: "cmd/symbol-playground", executable: true, line: 23},
			{path: "github.com/dvordrova/repomap/internal/report", dir: "internal/report"},
		},
	)

	resolution, err := Resolve(facts, Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertSelectedPackage(t, resolution, "github.com/dvordrova/repomap/cmd/repomap")
	if resolution.Reason != "unique executable matching the main module name" {
		t.Fatalf("reason = %q", resolution.Reason)
	}
	if roots := resolution.Selected.Roots; len(roots) != 1 || roots[0].Path != "cmd/repomap/main.go" || roots[0].Line != 43 {
		t.Fatalf("selected roots = %#v", roots)
	}
	reversed := syntheticFacts(
		"module-root", "github.com/dvordrova/repomap",
		[]syntheticPackage{
			{path: "github.com/dvordrova/repomap/internal/report", dir: "internal/report"},
			{path: "github.com/dvordrova/repomap/cmd/symbol-playground", dir: "cmd/symbol-playground", executable: true, line: 23},
			{path: "github.com/dvordrova/repomap/cmd/repomap", dir: "cmd/repomap", executable: true, line: 43},
			{path: "github.com/dvordrova/repomap/cmd/quality-evaluate", dir: "cmd/quality-evaluate", executable: true, line: 16},
		},
	)
	reversedResolution, err := Resolve(reversed, Options{})
	if err != nil {
		t.Fatalf("Resolve reversed: %v", err)
	}
	if reversedResolution.Selected == nil || reversedResolution.Selected.Ref != resolution.Selected.Ref {
		t.Fatalf("target identity changed with fact order: %#v != %#v", reversedResolution.Selected, resolution.Selected)
	}

	override, err := Resolve(facts, Options{Override: "cmd/quality-evaluate"})
	if err != nil {
		t.Fatalf("Resolve override: %v", err)
	}
	assertSelectedPackage(t, override, "github.com/dvordrova/repomap/cmd/quality-evaluate")
}

func TestResolveRepomapIgnoresNestedModulesDuringAutomaticSelection(t *testing.T) {
	facts := syntheticFacts(
		"module-root", "github.com/dvordrova/repomap",
		[]syntheticPackage{
			{path: "github.com/dvordrova/repomap/cmd/repomap", dir: "cmd/repomap", executable: true, line: 43},
			{path: "github.com/dvordrova/repomap/cmd/quality-evaluate", dir: "cmd/quality-evaluate", executable: true, line: 16},
		},
	)
	nested := gofacts.ModuleFact{
		ID: "module-fixture", ModulePath: "example.com/beegoapp",
		ModuleDir: "internal/surfacediscovery/testdata/beego", GoMod: "go.mod", Main: true,
	}
	facts.Modules = append(facts.Modules, nested)
	facts.Packages = append(facts.Packages, gofacts.PackageFact{
		CanonicalPath: "example.com/beegoapp", Name: "main", ModuleID: nested.ID,
		ModulePath: nested.ModulePath, PackageDir: ".", ModuleRelativeDir: ".",
		DisplayPath: nested.ModuleDir, Locality: "local",
	})
	facts.EntrypointPackages = append(facts.EntrypointPackages, gofacts.Entrypoint{
		ModulePath: nested.ModulePath, ImportPath: nested.ModulePath,
		Dir: nested.ModuleDir, PackageDir: ".", ModuleRelativeDir: ".",
		ModuleDir: nested.ModuleDir, Kind: "unknown", GoFiles: []string{"main.go"},
		Anchors: []gofacts.EntrypointAnchor{{
			Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
			Path: nested.ModuleDir + "/main.go", Line: 7,
		}},
	})

	resolution, err := Resolve(facts, Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertSelectedPackage(t, resolution, "github.com/dvordrova/repomap/cmd/repomap")

	explicit, err := Resolve(facts, Options{Override: nested.ModulePath})
	if err != nil {
		t.Fatalf("Resolve nested override: %v", err)
	}
	assertSelectedPackage(t, explicit, nested.ModulePath)
}

func TestResolveMobyLikeDockerdRequiresExplicitTarget(t *testing.T) {
	facts := syntheticFacts(
		"module-root", "github.com/moby/moby/v2",
		[]syntheticPackage{
			{path: "github.com/moby/moby/v2/cmd/docker-proxy", dir: "cmd/docker-proxy", executable: true, line: 20},
			{path: "github.com/moby/moby/v2/cmd/dockerd", dir: "cmd/dockerd", executable: true, line: 16},
			{path: "github.com/moby/moby/v2/daemon", dir: "daemon"},
		},
	)

	resolution, err := Resolve(facts, Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.State != ResolutionAmbiguous || resolution.Selected != nil {
		t.Fatalf("Moby-like resolution guessed a primary target: %#v", resolution)
	}

	override, err := Resolve(facts, Options{Override: "cmd/dockerd"})
	if err != nil {
		t.Fatalf("Resolve dockerd override: %v", err)
	}
	assertSelectedPackage(t, override, "github.com/moby/moby/v2/cmd/dockerd")
}

func TestResolveTelebotLikeRootLibrary(t *testing.T) {
	facts := syntheticFacts(
		"module-root", "gopkg.in/telebot.v3",
		[]syntheticPackage{
			{path: "gopkg.in/telebot.v3", dir: "."},
			{path: "gopkg.in/telebot.v3/layout", dir: "layout"},
			{path: "gopkg.in/telebot.v3/middleware", dir: "middleware"},
		},
	)

	resolution, err := Resolve(facts, Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertSelectedPackage(t, resolution, "gopkg.in/telebot.v3")
	if resolution.Selected.Kind != KindLibraryPackage || resolution.Selected.RootBoundary != RootBoundaryExactPublicAPI {
		t.Fatalf("selected library target = %#v", resolution.Selected)
	}
	if len(resolution.Selected.Roots) != 0 {
		t.Fatalf("library target invented roots: %#v", resolution.Selected.Roots)
	}
}

func TestResolveFailsClosedOnAmbiguousExecutables(t *testing.T) {
	facts := syntheticFacts(
		"module-root", "example.com/workspace",
		[]syntheticPackage{
			{path: "example.com/workspace/cmd/api", dir: "cmd/api", executable: true, line: 10},
			{path: "example.com/workspace/cmd/worker", dir: "cmd/worker", executable: true, line: 20},
		},
	)

	resolution, err := Resolve(facts, Options{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.State != ResolutionAmbiguous || resolution.Selected != nil {
		t.Fatalf("ambiguous resolution = %#v", resolution)
	}
	if _, err := Resolve(facts, Options{Override: "cmd/missing"}); !errors.Is(err, ErrOverrideNotFound) {
		t.Fatalf("missing override error = %v", err)
	}
}

type syntheticPackage struct {
	path       string
	dir        string
	executable bool
	kind       string
	line       int
}

func syntheticFacts(moduleID, modulePath string, definitions []syntheticPackage) gofacts.Facts {
	facts := gofacts.Facts{Modules: []gofacts.ModuleFact{{
		ID: moduleID, ModulePath: modulePath, ModuleDir: ".", GoMod: "go.mod", Main: true,
	}}}
	for _, definition := range definitions {
		facts.Packages = append(facts.Packages, gofacts.PackageFact{
			CanonicalPath: definition.path, Name: packageName(definition), ModuleID: moduleID,
			ModulePath: modulePath, PackageDir: definition.dir, ModuleRelativeDir: definition.dir,
			DisplayPath: definition.dir, Locality: "local",
		})
		if !definition.executable {
			continue
		}
		kind := definition.kind
		if kind == "" {
			kind = "unknown"
		}
		facts.EntrypointPackages = append(facts.EntrypointPackages, gofacts.Entrypoint{
			ModulePath: modulePath, ImportPath: definition.path, Dir: definition.dir,
			PackageDir: definition.dir, ModuleRelativeDir: definition.dir, ModuleDir: ".", Kind: kind,
			GoFiles: []string{"main.go"}, Anchors: []gofacts.EntrypointAnchor{{
				Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
				Path: definition.dir + "/main.go", Line: definition.line,
			}},
		})
	}
	return facts
}

func packageName(definition syntheticPackage) string {
	if definition.executable {
		return "main"
	}
	if definition.dir == "." {
		return "library"
	}
	return definition.dir
}

func assertSelectedPackage(t *testing.T, resolution Resolution, packagePath string) {
	t.Helper()
	if resolution.State != ResolutionSelected || resolution.Selected == nil {
		t.Fatalf("resolution = %#v", resolution)
	}
	if resolution.Selected.PackagePath != packagePath {
		t.Fatalf("selected package = %q, want %q", resolution.Selected.PackagePath, packagePath)
	}
	if resolution.Selected.Ref == "" {
		t.Fatal("selected target has no bound identity")
	}
}
