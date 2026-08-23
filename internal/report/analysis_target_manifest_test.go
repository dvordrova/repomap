package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/gofacts"
)

func TestParseSnapshotProjectsValidatedAnalysisTarget(t *testing.T) {
	target := reportAnalysisTargetFixture(t)
	raw, err := json.Marshal(struct {
		RepoName       string                 `json:"repo_name"`
		AnalysisTarget *analysistarget.Target `json:"analysis_target"`
	}{RepoName: "product", AnalysisTarget: target})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	data := &ReportData{}
	if err := parseSnapshot(path, data); err != nil {
		t.Fatalf("parseSnapshot: %v", err)
	}
	if data.AnalysisTarget == nil || data.AnalysisTarget.Ref != target.Ref {
		t.Fatalf("analysis target = %#v, want ref %q", data.AnalysisTarget, target.Ref)
	}
	if err := data.AnalysisTarget.Validate(); err != nil {
		t.Fatalf("projected analysis target: %v", err)
	}

	drifted := target.Snapshot()
	drifted.PackagePath += "/other"
	raw, err = json.Marshal(struct {
		RepoName       string                 `json:"repo_name"`
		AnalysisTarget *analysistarget.Target `json:"analysis_target"`
	}{RepoName: "product", AnalysisTarget: &drifted})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	data = &ReportData{}
	err = parseSnapshot(path, data)
	if err == nil || !strings.Contains(err.Error(), "ref binding mismatch") || data.AnalysisTarget != nil {
		t.Fatalf("drifted target error/data = %v / %#v", err, data.AnalysisTarget)
	}
}

func TestReportAnalysisTargetBindingIsExact(t *testing.T) {
	target := reportAnalysisTargetFixture(t)
	targetRef, targetSHA256, err := reportAnalysisTargetBinding(target)
	if err != nil {
		t.Fatal(err)
	}
	if targetRef != target.Ref || targetSHA256 == "" {
		t.Fatalf("analysis target binding = %q / %q", targetRef, targetSHA256)
	}
	drifted := target.Snapshot()
	drifted.PackagePath += "/other"
	if _, _, err := reportAnalysisTargetBinding(&drifted); err == nil ||
		!strings.Contains(err.Error(), "ref binding mismatch") {
		t.Fatalf("drifted target error = %v", err)
	}
}

func reportAnalysisTargetFixture(t *testing.T) *analysistarget.Target {
	t.Helper()
	return reportAnalysisTargetForDir(t, "cmd/product")
}

func reportAnalysisTargetForDir(t *testing.T, dir string) *analysistarget.Target {
	t.Helper()
	const modulePath = "example.com/product"
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-root", ModulePath: modulePath, ModuleDir: ".", GoMod: "go.mod", Main: true,
		}},
		Packages: []gofacts.PackageFact{{
			CanonicalPath: modulePath + "/" + dir, Name: "main", ModuleID: "module-root",
			ModulePath: modulePath, PackageDir: dir, ModuleRelativeDir: dir,
			DisplayPath: dir, Locality: "local",
		}},
		EntrypointPackages: []gofacts.Entrypoint{{
			ModulePath: modulePath, ImportPath: modulePath + "/" + dir, Dir: dir,
			PackageDir: dir, ModuleRelativeDir: dir, ModuleDir: ".", Kind: "cli",
			GoFiles: []string{"main.go"}, Anchors: []gofacts.EntrypointAnchor{{
				Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
				Path: dir + "/main.go", Line: 10,
			}},
		}},
	}
	candidates, err := analysistarget.Candidates(facts)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("analysis target fixture candidates = %#v, %v", candidates, err)
	}
	target := candidates[0].Target.Snapshot()
	return &target
}
