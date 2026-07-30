package surfacediscovery

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestAnalyzeIsolatesIllTypedExecutableAndKeepsExactProcessEntries(t *testing.T) {
	options := DefaultOptions(filepath.Join("testdata", "partial_load"))
	result, err := AnalyzeWithInput(options, Input{
		RepositoryName: "partial_load",
		Entrypoints: []EntrypointInput{
			{
				Package: "example.com/partial_load/cmd/partial_load", PackageDir: "cmd/partial_load",
				Anchors: []EntrypointAnchorInput{{Kind: ProcessEntryAnchorGoMain, Path: "cmd/partial_load/main.go", Line: 7}},
			},
			{
				Package: "example.com/partial_load/cmd/broken", PackageDir: "cmd/broken",
				Anchors: []EntrypointAnchorInput{{Kind: ProcessEntryAnchorGoMain, Path: "cmd/broken/main.go", Line: 3}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	route := onlyTriggerOfKind(t, result, "http_route")
	if route.Identity.Path.Text != "/health" || route.ProcessEntrypoint.Package != "example.com/partial_load/cmd/partial_load" {
		t.Fatalf("healthy executable route = %#v", route)
	}
	entries := triggersOfKind(result, "process_entry")
	if len(entries) != 2 || result.Coverage.ProcessEntries != 2 ||
		result.Coverage.AvailableProcessEntries != 1 || result.Coverage.UnavailableProcessEntries != 1 {
		t.Fatalf("process entries/coverage = %#v / %#v", entries, result.Coverage)
	}
	for _, entry := range entries {
		switch entry.ProcessEntrypoint.Package {
		case "example.com/partial_load/cmd/partial_load":
			if entry.ExecutableRole != ExecutableRolePrimaryApplication || entry.Availability != AvailabilityAvailable ||
				entry.SurfaceRole != SurfaceRoleEntrySurface || entry.TraceReadiness != TraceReadinessPartial ||
				entry.Quality.Identity != SurfaceQualityExact {
				t.Fatalf("healthy process entry = %#v", entry)
			}
		case "example.com/partial_load/cmd/broken":
			if entry.ExecutableRole != ExecutableRoleSecondaryService || entry.Availability != AvailabilityUnavailable ||
				entry.SurfaceRole != SurfaceRoleEntrySurface || entry.TraceReadiness != TraceReadinessPartial ||
				entry.ProcessEntrypoint.Location != (Location{Path: "cmd/broken/main.go", Line: 3}) ||
				len(entry.Evidence) != 1 || entry.Evidence[0].Kind != "process_entry_declaration" {
				t.Fatalf("broken process entry = %#v", entry)
			}
		default:
			t.Fatalf("unexpected process entry = %#v", entry)
		}
	}
	if result.Coverage.PackageDiagnosticCount == 0 || result.Coverage.UnavailablePackageCount == 0 {
		t.Fatalf("package failure was not retained: %#v", result.Coverage)
	}
	foundDiagnostic := false
	for _, diagnostic := range result.Coverage.PackageDiagnostics {
		if diagnostic.Package != "example.com/partial_load/cmd/broken" {
			continue
		}
		if diagnostic.OwningExecutable == "cmd/broken" &&
			diagnostic.ExecutableRole == ExecutableRoleSecondaryService &&
			diagnostic.Availability == AvailabilityUnavailable &&
			diagnostic.Location != nil && diagnostic.Location.Path == "cmd/broken/main.go" &&
			strings.Contains(diagnostic.Message, "undefined: missingGeneratedAsset") {
			foundDiagnostic = true
		}
	}
	if !foundDiagnostic {
		t.Fatalf("owned repository-relative diagnostic not found: %#v", result.Coverage.PackageDiagnostics)
	}
	if !slices.Contains(result.Coverage.PackagesSkipped, "example.com/partial_load/cmd/broken") {
		t.Fatalf("ill-typed executable was not excluded from SSA: %#v", result.Coverage.PackagesSkipped)
	}
	processAnchors := 0
	for _, anchor := range result.Grounding.Anchors {
		if anchor.Kind == "process_entry" {
			processAnchors++
		}
	}
	if processAnchors != 2 || result.Grounding.RepositoryArchetype.Selected == "library_framework" ||
		!strings.Contains(result.Grounding.RepositoryArchetype.Evidence[0], "2 exact build-selected") {
		t.Fatalf("degraded process grounding = %#v", result.Grounding)
	}
}

func TestAnalyzeLoadsNestedModuleFromDeterministicInput(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	moduleRoot := filepath.Join(repository, "service")
	if err := os.MkdirAll(moduleRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(moduleRoot, "go.mod"),
		[]byte("module example.com/nested\n\ngo 1.26\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(moduleRoot, "main.go"),
		[]byte(`package main

import "net/http"

func health(http.ResponseWriter, *http.Request) {}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
}
`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	result, err := AnalyzeWithInput(DefaultOptions(repository), Input{
		RepositoryName: "nested",
		ModuleDirs:     []string{"service"},
		Entrypoints: []EntrypointInput{{
			Package: "example.com/nested", PackageDir: "service", ModuleDir: "service",
			Anchors: []EntrypointAnchorInput{{
				Kind: ProcessEntryAnchorGoMain, Path: "service/main.go", Line: 7,
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	route := onlyTriggerOfKind(t, result, "http_route")
	if route.Identity.Path.Text != "/health" ||
		route.RegistrationSite.Path != "service/main.go" {
		t.Fatalf("nested route = %#v", route)
	}
	if result.Coverage.PackagesInspected == 0 ||
		result.Coverage.FunctionsInspected == 0 ||
		result.Coverage.AvailableProcessEntries != 1 ||
		result.Coverage.UnavailableProcessEntries != 0 {
		t.Fatalf("nested module coverage = %#v", result.Coverage)
	}
}

func TestAnalyzeLoadsTwoIndependentModules(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	for _, fixture := range []struct {
		dir        string
		modulePath string
		route      string
	}{
		{dir: "alpha", modulePath: "example.com/alpha", route: "/alpha"},
		{dir: "beta", modulePath: "example.com/beta", route: "/beta"},
	} {
		moduleRoot := filepath.Join(repository, fixture.dir)
		if err := os.MkdirAll(moduleRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(moduleRoot, "go.mod"),
			[]byte("module "+fixture.modulePath+"\n\ngo 1.26\n"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		source := `package main

import "net/http"

func handler(http.ResponseWriter, *http.Request) {}

func main() {
	http.HandleFunc("` + fixture.route + `", handler)
}
`
		if err := os.WriteFile(filepath.Join(moduleRoot, "main.go"), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	result, err := AnalyzeWithInput(DefaultOptions(repository), Input{
		RepositoryName: "multi",
		ModuleDirs:     []string{"alpha", "beta"},
		Entrypoints: []EntrypointInput{
			{
				Package: "example.com/alpha", PackageDir: "alpha", ModuleDir: "alpha",
				Anchors: []EntrypointAnchorInput{{
					Kind: ProcessEntryAnchorGoMain, Path: "alpha/main.go", Line: 7,
				}},
			},
			{
				Package: "example.com/beta", PackageDir: "beta", ModuleDir: "beta",
				Anchors: []EntrypointAnchorInput{{
					Kind: ProcessEntryAnchorGoMain, Path: "beta/main.go", Line: 7,
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	routes := triggersOfKind(result, "http_route")
	if len(routes) != 2 {
		t.Fatalf("multi-module route count = %d, want 2: %#v", len(routes), routes)
	}
	got := map[string]string{}
	for _, route := range routes {
		got[route.Identity.Path.Text] = route.RegistrationSite.Path
	}
	if got["/alpha"] != "alpha/main.go" || got["/beta"] != "beta/main.go" {
		t.Fatalf("multi-module routes = %#v", got)
	}
	if result.Coverage.AvailableProcessEntries != 2 {
		t.Fatalf("available multi-module process entries = %d, want 2", result.Coverage.AvailableProcessEntries)
	}
}

func TestClassifyExecutableRoleUsesRepositoryStructureWithoutNamingExecutables(t *testing.T) {
	tests := []struct {
		name       string
		repository string
		directory  string
		kind       string
		want       string
	}{
		{name: "repository-named application", repository: "project", directory: "cmd/project", want: ExecutableRolePrimaryApplication},
		{name: "secondary service", repository: "project", directory: "cmd/server", want: ExecutableRoleSecondaryService},
		{name: "developer tool", repository: "project", directory: "cmd/dev/inspect", want: ExecutableRoleTooling},
		{name: "test utility", repository: "project", directory: "cmd/server/testutil", want: ExecutableRoleTestOrHelper},
		{name: "existing tool hint", repository: "project", directory: "cmd/inspect", kind: "tool", want: ExecutableRoleTooling},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entrypoint := processEntrypoint{
				packageDir: test.directory, kind: test.kind, owner: test.directory,
				anchor: EntrypointAnchorInput{Path: test.directory + "/main.go"},
			}
			if got := classifyExecutableRole(test.repository, entrypoint); got != test.want {
				t.Fatalf("classifyExecutableRole() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCheckSurfaceGoVersionRejectsNewerTargetBeforePackageLoading(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/future\n\ngo 99.0\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	err := checkSurfaceGoVersion(root)
	if err == nil {
		t.Fatal("error = nil, want an incompatible-toolchain error")
	}
	for _, want := range []string{"requires go99.0", runtime.Version()} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err, want)
		}
	}
}
