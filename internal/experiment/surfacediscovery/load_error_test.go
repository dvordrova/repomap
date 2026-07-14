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
		foundDiagnostic = diagnostic.OwningExecutable == "cmd/broken" &&
			diagnostic.ExecutableRole == ExecutableRoleSecondaryService &&
			diagnostic.Availability == AvailabilityUnavailable &&
			diagnostic.Location != nil && diagnostic.Location.Path == "cmd/broken/main.go" &&
			strings.Contains(diagnostic.Message, "undefined: missingGeneratedAsset")
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
