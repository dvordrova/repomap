package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestVerifyReportJSONRequiresExactProgramAndCapturedAuthority(t *testing.T) {
	manifest, data := manifestProgramOnlyReportFixture(t, "python")
	reportJSON, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	manifest.ReportSHA256 = manifestSHA256(reportJSON)
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
		t.Fatalf("Python Program-only report was rejected: %v", err)
	}

	for name, test := range map[string]struct {
		mutate func(*ReportData)
		want   string
	}{
		"captured revision": {
			mutate: func(value *ReportData) { value.CapturedRevision = strings.Repeat("0", 40) },
			want:   "captured revision",
		},
		"captured input count": {
			mutate: func(value *ReportData) { value.CapturedInputCount++ },
			want:   "captured input count",
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := *data
			test.mutate(&candidate)
			raw, marshalErr := json.Marshal(&candidate)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			bound := manifest
			bound.ReportSHA256 = manifestSHA256(raw)
			if err := bound.VerifyReportJSON(raw); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("authority drift error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestVerifyReportJSONUsesGenericProgramIndexSemanticAuthorityForPythonAndGo(t *testing.T) {
	pythonManifest, python := manifestProgramOnlyReportFixture(t, "python")
	pythonJSON, err := json.Marshal(python)
	if err != nil {
		t.Fatal(err)
	}
	pythonManifest.ReportSHA256 = manifestSHA256(pythonJSON)
	if err := pythonManifest.VerifyReportJSON(pythonJSON); err != nil {
		t.Fatalf("Python program page was rejected: %v", err)
	}

	goManifest, goData := manifestProgramOnlyReportFixture(t, "go")
	goJSON, err := json.Marshal(goData)
	if err != nil {
		t.Fatal(err)
	}
	goManifest.ReportSHA256 = manifestSHA256(goJSON)
	if err := goManifest.VerifyReportJSON(goJSON); err != nil {
		t.Fatalf("Go page without optional analysis extension was rejected: %v", err)
	}
}

func TestVerifyReportJSONRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	manifest, data := manifestProgramOnlyReportFixture(t, "python")
	reportJSON, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}

	var withUnknown map[string]any
	if err := json.Unmarshal(reportJSON, &withUnknown); err != nil {
		t.Fatal(err)
	}
	withUnknown["entry_call"] = map[string]any{"version": 2}
	unknownJSON, err := json.Marshal(withUnknown)
	if err != nil {
		t.Fatal(err)
	}
	unknownManifest := manifest
	unknownManifest.ReportSHA256 = manifestSHA256(unknownJSON)
	if err := unknownManifest.VerifyReportJSON(unknownJSON); err == nil ||
		!strings.Contains(err.Error(), `unknown field "entry_call"`) {
		t.Fatalf("unknown report field error = %v", err)
	}

	trailingJSON := append(append([]byte(nil), reportJSON...), []byte(`{}`)...)
	trailingManifest := manifest
	trailingManifest.ReportSHA256 = manifestSHA256(trailingJSON)
	if err := trailingManifest.VerifyReportJSON(trailingJSON); err == nil ||
		!strings.Contains(err.Error(), "multiple json values") {
		t.Fatalf("trailing report value error = %v", err)
	}
}

func TestDecodeRunManifestRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	manifestJSON, err := json.Marshal(validRunManifestFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	var withUnknown map[string]any
	if err := json.Unmarshal(manifestJSON, &withUnknown); err != nil {
		t.Fatal(err)
	}
	withUnknown["legacy_authority"] = true
	unknownJSON, err := json.Marshal(withUnknown)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeRunManifest(unknownJSON); err == nil ||
		!strings.Contains(err.Error(), `unknown field "legacy_authority"`) {
		t.Fatalf("unknown manifest field error = %v", err)
	}
	var withRemovedAtlas map[string]any
	if err := json.Unmarshal(manifestJSON, &withRemovedAtlas); err != nil {
		t.Fatal(err)
	}
	material, ok := withRemovedAtlas["material_inputs"].(map[string]any)
	if !ok {
		t.Fatal("manifest fixture has no material inputs")
	}
	material["repository_atlas_sha256"] = strings.Repeat("f", 64)
	removedAtlasJSON, err := json.Marshal(withRemovedAtlas)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeRunManifest(removedAtlasJSON); err == nil ||
		!strings.Contains(err.Error(), `unknown field "repository_atlas_sha256"`) {
		t.Fatalf("removed Atlas manifest field error = %v", err)
	}
	if _, err := DecodeRunManifest(append(manifestJSON, []byte(`{}`)...)); err == nil ||
		!strings.Contains(err.Error(), "multiple json values") {
		t.Fatalf("trailing manifest value error = %v", err)
	}
}

func TestDecodeRunManifestRejectsPreviousVersions(t *testing.T) {
	for _, version := range []int{24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37} {
		manifest := validRunManifestFixture(t)
		manifest.Version = version
		encoded, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeRunManifest(encoded); err == nil ||
			!strings.Contains(err.Error(), fmt.Sprintf("unsupported version %d", version)) {
			t.Fatalf("previous manifest version %d error = %v", version, err)
		}
	}
}

func manifestProgramOnlyReportFixture(t *testing.T, language string) (RunManifest, *ReportData) {
	t.Helper()
	index, groups, documentation := reportFinalGraphFixture(
		t, reportProgramIndexFixture(t, language, "executable"),
	)
	portfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := NewGroupGraphView([]groupindex.Index{groups}, index.Target.ID)
	if err != nil {
		t.Fatal(err)
	}
	target := index.Target.Snapshot()
	manifest := validRunManifestFixture(t)
	manifest.MaterialInputs.ProgramTargetID, manifest.MaterialInputs.ProgramTargetSHA256, err =
		reportProgramTargetMaterial(&target)
	if err != nil {
		t.Fatal(err)
	}
	data := &ReportData{
		FormatVersion:      CurrentFormatVersion,
		RepoName:           "fixture",
		CapturedRevision:   manifest.RepositoryState.Head,
		CapturedInputCount: len(manifest.CapturedInputs),
		OpenablePaths:      append([]string(nil), manifest.OpenablePaths...),
		ProgramPortfolio:   portfolio,
		GroupGraph:         graph,
	}
	data.TargetOutcomePortfolio = reportTargetOutcomeViewFixture(t, []TargetNavigationPage{{
		RunID:            "run-fixture",
		ProgramTarget:    index.Target.Snapshot(),
		ArtifactFilename: programindex.ArtifactFilename,
	}}, index.Target.ID)
	documentationRaw, err := documentationreduce.Encode(documentation)
	if err != nil {
		t.Fatal(err)
	}
	groupsRaw, err := groupindex.Encode(groups)
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.ReducedDocumentationSHA256 = manifestSHA256(documentationRaw)
	manifest.MaterialInputs.GroupsIndexSHA256 = manifestSHA256(groupsRaw)
	return manifest, data
}
