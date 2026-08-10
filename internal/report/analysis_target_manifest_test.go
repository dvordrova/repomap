package report

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/freshness"
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
	if warning := parseSnapshot(path, data); warning != "" {
		t.Fatalf("parseSnapshot warning = %q", warning)
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
		AnalysisTarget *analysistarget.Target `json:"analysis_target"`
	}{AnalysisTarget: &drifted})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	data = &ReportData{}
	warning := parseSnapshot(path, data)
	if !strings.Contains(warning, "ref binding mismatch") || data.AnalysisTarget != nil {
		t.Fatalf("drifted target warning/data = %q / %#v", warning, data.AnalysisTarget)
	}
}

func TestRunManifestBindsExactAnalysisTarget(t *testing.T) {
	manifest := validRunManifestFixture(t)
	target := reportAnalysisTargetFixture(t)
	targetRef, targetSHA256, err := reportAnalysisTargetBinding(target)
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.AnalysisTargetRef = targetRef
	manifest.MaterialInputs.AnalysisTargetSHA256 = targetSHA256
	encode := func(t *testing.T, value *analysistarget.Target) []byte {
		t.Helper()
		reportJSON, err := json.Marshal(ReportData{
			FormatVersion:   CurrentFormatVersion,
			AnalysisTarget:  value,
			RepositoryGraph: reportAnalysisTargetGraphFixture(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return reportJSON
	}

	reportJSON := encode(t, target)
	manifest.OpenablePaths = nil
	manifest.Components = nil
	manifest.ReportSHA256 = manifestSHA256(reportJSON)
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
		t.Fatalf("VerifyReportJSON: %v", err)
	}

	t.Run("missing report target", func(t *testing.T) {
		missing := encode(t, nil)
		candidate := manifest
		candidate.ReportSHA256 = manifestSHA256(missing)
		if err := candidate.VerifyReportJSON(missing); err == nil ||
			!strings.Contains(err.Error(), "analysis target is missing") {
			t.Fatalf("missing target error = %v", err)
		}
	})

	t.Run("tampered report target", func(t *testing.T) {
		drifted := target.Snapshot()
		drifted.PackagePath += "/other"
		tampered := encode(t, &drifted)
		candidate := manifest
		candidate.ReportSHA256 = manifestSHA256(tampered)
		if err := candidate.VerifyReportJSON(tampered); err == nil ||
			!strings.Contains(err.Error(), "ref binding mismatch") {
			t.Fatalf("tampered target error = %v", err)
		}
	})

	t.Run("different valid report target", func(t *testing.T) {
		other := reportAnalysisTargetForDir(t, "cmd/other")
		mismatched := encode(t, other)
		candidate := manifest
		candidate.ReportSHA256 = manifestSHA256(mismatched)
		if err := candidate.VerifyReportJSON(mismatched); err == nil ||
			!strings.Contains(err.Error(), "identity does not match report") {
			t.Fatalf("mismatched target error = %v", err)
		}
	})
}

func TestGenerateAuthorizedAllowsReportWithoutGoPackagesOrAnalysisTarget(t *testing.T) {
	repository := newRunManifestRepository(t)
	initial, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	writeTestFile(t, runDir, "snapshot.json", `{"repo_name":"non-go-fixture"}`)
	writeRunManifestMetadata(t, runDir, repository)
	writeTestFile(t, runDir, "orientation_report.json", `{
		"project_guess":"non-Go fixture",
		"candidate_flows":[],
		"warnings":[]
	}`)
	current, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := ConfirmRunAuthority(repository, initial, current)
	if err != nil {
		t.Fatal(err)
	}
	if err := GenerateAuthorized(runDir, authority); err != nil {
		t.Fatalf("GenerateAuthorized: %v", err)
	}
	manifest, err := ReadRunManifest(runDir)
	if err != nil {
		t.Fatalf("ReadRunManifest: %v", err)
	}
	if manifest.MaterialInputs.AnalysisTargetRef != "" ||
		manifest.MaterialInputs.AnalysisTargetSHA256 != "" {
		t.Fatalf("non-Go manifest target binding = %#v", manifest.MaterialInputs)
	}
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report ReportData
	if err := json.Unmarshal(reportJSON, &report); err != nil {
		t.Fatal(err)
	}
	if report.AnalysisTarget != nil ||
		(report.RepositoryGraph != nil && len(report.RepositoryGraph.Packages) > 0) {
		t.Fatalf("non-Go report target/graph = %#v / %#v", report.AnalysisTarget, report.RepositoryGraph)
	}
}

func TestGenerateAuthorizedBindsAnalysisTargetWhenGoGraphIsDegraded(t *testing.T) {
	tests := []struct {
		name  string
		graph *RepositoryGraph
	}{
		{name: "missing graph"},
		{name: "empty graph", graph: &RepositoryGraph{Version: 2, Packages: []PackageInfo{}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runDir := t.TempDir()
			target := reportAnalysisTargetFixture(t)
			data := &ReportData{
				FormatVersion:   CurrentFormatVersion,
				RepoName:        "fixture",
				AnalysisTarget:  target,
				RepositoryGraph: test.graph,
			}
			writeAuthorizedAnalysisTargetReportFixture(t, runDir, data)

			manifest, err := ReadRunManifest(runDir)
			if err != nil {
				t.Fatalf("ReadRunManifest: %v", err)
			}
			if manifest.MaterialInputs.AnalysisTargetRef != target.Ref ||
				manifest.MaterialInputs.AnalysisTargetSHA256 == "" {
				t.Fatalf("analysis target material = %#v", manifest.MaterialInputs)
			}
		})
	}
}

func writeAuthorizedAnalysisTargetReportFixture(t *testing.T, runDir string, data *ReportData) {
	t.Helper()
	writeRunManifestSnapshot(t, runDir, data.RepoName)
	repository := newRunManifestRepository(t)
	state, err := freshness.CaptureRepository(context.Background(), repository)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := ConfirmRunAuthority(repository, state, state)
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(runDir, "report.json")
	if err := WriteReportJSON(data, reportPath); err != nil {
		t.Fatalf("WriteReportJSON: %v", err)
	}
	reportJSON, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAuthorizedRunManifest(runDir, data, reportJSON, authority); err != nil {
		t.Fatalf("writeAuthorizedRunManifest: %v", err)
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
	resolution, err := analysistarget.Resolve(facts, analysistarget.Options{})
	if err != nil {
		t.Fatalf("resolve analysis target fixture: %v", err)
	}
	if resolution.Selected == nil {
		t.Fatalf("analysis target fixture resolution = %#v", resolution)
	}
	target := resolution.Selected.Snapshot()
	return &target
}

func writeRunManifestSnapshot(t *testing.T, runDir, repoName string) {
	t.Helper()
	raw, err := json.Marshal(struct {
		RepoName       string                 `json:"repo_name"`
		AnalysisTarget *analysistarget.Target `json:"analysis_target"`
		GoFacts        any                    `json:"go_facts"`
	}{
		RepoName: repoName, AnalysisTarget: reportAnalysisTargetFixture(t),
		GoFacts: reportAnalysisTargetGoFactsFixture(),
	})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, runDir, "snapshot.json", string(raw))
}

func snapshotJSONWithAnalysisTarget(t *testing.T, raw []byte) []byte {
	t.Helper()
	var snapshot map[string]any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("decode snapshot fixture: %v", err)
	}
	snapshot["analysis_target"] = reportAnalysisTargetFixture(t)
	if _, present := snapshot["go_facts"]; !present {
		snapshot["go_facts"] = reportAnalysisTargetGoFactsFixture()
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("encode snapshot fixture: %v", err)
	}
	return encoded
}

func reportAnalysisTargetGraphFixture() *RepositoryGraph {
	return &RepositoryGraph{
		Version: 2,
		Modules: []ModuleInfo{{
			ID: "module-root", Path: "example.com/product", Dir: ".", DisplayName: "product",
		}},
		Packages: []PackageInfo{{
			CanonicalPath: "example.com/product/cmd/product", Name: "main",
			ModuleID: "module-root", ModulePath: "example.com/product",
			Dir: "cmd/product", ModuleRelativeDir: "cmd/product", DisplayPath: "cmd/product",
			Locality: "local", Files: []string{"cmd/product/main.go"},
		}},
	}
}

func reportAnalysisTargetGoFactsFixture() map[string]any {
	return map[string]any{
		"modules": []map[string]any{{
			"id": "module-root", "module_path": "example.com/product",
			"module_dir": ".", "display_name": "product",
		}},
		"packages": []map[string]any{{
			"canonical_path": "example.com/product/cmd/product", "name": "main",
			"module_id": "module-root", "module_path": "example.com/product",
			"dir": "cmd/product", "module_relative_dir": "cmd/product",
			"display_path": "cmd/product", "locality": "local",
			"files": []string{"cmd/product/main.go"},
		}},
	}
}

func bindRunManifestAnalysisTarget(t *testing.T, manifest *RunManifest, data *ReportData) {
	t.Helper()
	ref, digest, err := reportAnalysisTargetMaterial(data.AnalysisTarget, data.RepositoryGraph)
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.AnalysisTargetRef = ref
	manifest.MaterialInputs.AnalysisTargetSHA256 = digest
}
