package surfacediscovery

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestAnalyzeRejectsUnsafeSelectedTargetEvenWhenSiblingPackageIsSSASafe(t *testing.T) {
	options := DefaultOptions(filepath.Join("testdata", "partial_load"))
	_, err := AnalyzeWithInput(options, Input{
		RepositoryName: "partial_load", ModuleDirs: []string{"."},
		Packages: []PackageInput{
			{Path: "example.com/partial_load/cmd/broken", ModuleDir: "."},
			{Path: "example.com/partial_load/cmd/partial_load", ModuleDir: "."},
		},
		AnalysisTarget: &AnalysisTargetInput{
			Kind: AnalysisTargetExecutablePackage, PackagePath: "example.com/partial_load/cmd/broken",
			Roots: []AnalysisTargetRootInput{{Path: "cmd/broken/main.go", Line: 3}},
		},
	})
	var targetErr *AnalysisTargetSSAUnavailableError
	if !errors.As(err, &targetErr) || targetErr.Reason != AnalysisTargetPackageNotSSASafe ||
		targetErr.Package != "example.com/partial_load/cmd/broken" ||
		!IsAnalysisTargetSSAUnavailable(err) {
		t.Fatalf("unsafe selected target error = %#v / %v", targetErr, err)
	}
}

func TestAnalyzeSelectedTargetSSAFailureExplainsMissingNestedModuleEmbedBuildInput(t *testing.T) {
	repository := filepath.Join("testdata", "nested_embed_missing")
	_, err := AnalyzeWithInput(DefaultOptions(repository), Input{
		RepositoryName: "nested_embed_missing",
		ModuleDirs:     []string{"cli"},
		Packages: []PackageInput{
			{Path: "example.com/nested_embed_missing", ModuleDir: "cli"},
			{Path: "example.com/nested_embed_missing/cmd", ModuleDir: "cli"},
		},
		Entrypoints: []EntrypointInput{{
			Package: "example.com/nested_embed_missing", PackageDir: ".", ModuleDir: "cli",
			Anchors: []EntrypointAnchorInput{{
				Kind: ProcessEntryAnchorGoMain, Path: "cli/main.go", Line: 5,
			}},
		}},
		AnalysisTarget: &AnalysisTargetInput{
			Kind: AnalysisTargetExecutablePackage, PackagePath: "example.com/nested_embed_missing",
			Roots: []AnalysisTargetRootInput{{Path: "cli/main.go", Line: 5}},
		},
	})
	var targetErr *AnalysisTargetSSAUnavailableError
	if !errors.As(err, &targetErr) || targetErr.Reason != AnalysisTargetPackageNotSSASafe ||
		targetErr.Package != "example.com/nested_embed_missing" {
		t.Fatalf("nested embed target error = %#v / %v", targetErr, err)
	}
	if targetErr.Diagnostic == nil ||
		targetErr.Diagnostic.Package != "example.com/nested_embed_missing/cmd" ||
		targetErr.Diagnostic.Location == nil ||
		targetErr.Diagnostic.Location.Path != "cli/cmd/license.go" ||
		targetErr.Diagnostic.Location.Line != 5 ||
		!strings.Contains(targetErr.Diagnostic.Message, "embedded/LICENSE") ||
		!strings.Contains(targetErr.Diagnostic.Message, "no matching files found") {
		t.Fatalf("nested embed diagnostic = %#v", targetErr.Diagnostic)
	}
	message := err.Error()
	for _, want := range []string{
		"package example.com/nested_embed_missing/cmd failed",
		"cli/cmd/license.go:5",
		"embedded/LICENSE",
		"no matching files found",
		"prepare missing generated/build inputs",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("target error %q does not contain %q", message, want)
		}
	}
}

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

func TestPackageErrorLocationResolvesModuleRelativeFilenameOnce(t *testing.T) {
	root := t.TempDir()
	moduleDir := filepath.Join(root, "cli")
	packageDir := filepath.Join(moduleDir, "cmd")
	sourcePath := filepath.Join(packageDir, "license.go")
	pkg := &packages.Package{
		Dir:     packageDir,
		GoFiles: []string{sourcePath},
		Module:  &packages.Module{Dir: moduleDir},
	}
	analyzer := analyzer{root: root}

	location := analyzer.packageErrorLocation(pkg, "cmd/license.go:12:12")
	if location == nil || location.Path != "cli/cmd/license.go" ||
		location.Line != 12 || location.Column != 12 {
		t.Fatalf("module-relative diagnostic location = %#v", location)
	}
	if unknown := analyzer.packageErrorLocation(pkg, "cmd/missing.go:3:1"); unknown != nil {
		t.Fatalf("unknown diagnostic source became openable: %#v", unknown)
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

	t.Setenv("GOTOOLCHAIN", "")
	err := checkSurfaceGoVersion(root, true)
	if err == nil {
		t.Fatal("error = nil, want an incompatible-toolchain error")
	}
	for _, want := range []string{"requires go99.0", runtime.Version()} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err, want)
		}
	}
}

// Decision 231 owner preference: REPOMAP_GOTOOLCHAIN=auto (or local+auto)
// defers the toolchain decision to the Go loader — a module requiring a
// newer Go is NOT an admission-gate failure in that environment. The loader
// resolves the toolchain; the runtime-version check would otherwise produce
// a false negative on machines whose go command can auto-download. A plain
// GOTOOLCHAIN value must NOT change repomap behavior.
func TestCheckSurfaceGoVersionAutoToolchainDefersToLoader(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/future\n\ngo 99.0\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	t.Setenv("REPOMAP_GOTOOLCHAIN", "auto")
	if err := checkSurfaceGoVersion(root, true); err != nil {
		t.Fatalf("REPOMAP_GOTOOLCHAIN=auto: error = %v, want nil (defer to loader)", err)
	}
	t.Setenv("REPOMAP_GOTOOLCHAIN", "local+auto")
	if err := checkSurfaceGoVersion(root, true); err != nil {
		t.Fatalf("REPOMAP_GOTOOLCHAIN=local+auto: error = %v, want nil (defer to loader)", err)
	}
}

// Long-horizon program Phase 1A: ONLINE/default analysis defers toolchain
// selection to the Go loader (automatic acquisition) — a module requiring a
// newer Go is not an admission-gate failure. Only OFFLINE analysis, which
// cannot acquire a toolchain, keeps the honest runtime-version gate.
func TestCheckSurfaceGoVersionOnlineDefaultDefersOfflineGates(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/future\n\ngo 99.0\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REPOMAP_GOTOOLCHAIN", "")
	t.Setenv("GOTOOLCHAIN", "")
	// Online default: defer to loader, never a gate.
	if err := checkSurfaceGoVersion(root, false); err != nil {
		t.Fatalf("online default must defer to the Go loader: %v", err)
	}
	// Offline: honest admission gate remains.
	if err := checkSurfaceGoVersion(root, true); err == nil {
		t.Fatal("offline must keep the runtime-version admission gate")
	}
}
