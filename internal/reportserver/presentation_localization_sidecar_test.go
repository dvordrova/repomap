package reportserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/llmbundle"
	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/orient"
	reportpkg "github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/studymap"
	"github.com/dvordrova/repomap/internal/tasklens"
)

func TestServeRussianProjectionUsesSharedRunPresentation(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(repository, "batch.go"),
		[]byte("package example\n\nfunc Core() {}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	const runID = "20260731-193000-shared-presentation"
	writeRun(t, runsDir, runID, repository, "canonical report")
	runDir := filepath.Join(runsDir, runID)

	canonical := &reportpkg.ReportData{
		FormatVersion: reportpkg.CurrentFormatVersion,
		RepoName:      "example.test/coherent",
		ProjectGuess:  "repository orientation",
		OpenablePaths: []string{"batch.go"},
		ArchitectureCanvas: &reportpkg.ArchitectureCanvas{
			Version:  reportpkg.ArchitectureCanvasVersion,
			Title:    "Repository architecture",
			Subtitle: "A bounded architecture view.",
			Subsystems: []reportpkg.ArchitectureSubsystem{{
				ID:           "subsystem-core",
				Name:         "Core subsystem",
				Description:  "Owns the central behavior.",
				ComponentIDs: []componentmap.ComponentID{"component-core"},
			}},
			Components: []reportpkg.ArchitectureComponent{{
				ID:          "component-core",
				SubsystemID: "subsystem-core",
				Name:        "Core component",
				Description: "Coordinates the example service.",
				Members: []componentmap.Candidate{{
					ID: componentmap.MemberID{
						Kind:  componentmap.MemberSymbol,
						Value: "example.Core",
					},
					Name: "example.Core",
					Facts: []componentmap.LocalFact{{
						Kind:      componentmap.FactDeclaration,
						Value:     "example.Core",
						Certainty: evidence.CertaintyStatic,
						Location: &evidence.Location{
							Path: "batch.go",
							Line: 3,
						},
					}},
				}},
			}},
		},
	}
	reportPath := filepath.Join(runDir, "report.json")
	if err := reportpkg.WriteReportJSON(canonical, reportPath); err != nil {
		t.Fatal(err)
	}
	canonicalJSON, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	canonicalState, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(runDir, reportpkg.RunManifestFilename)
	manifestJSON, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := reportpkg.DecodeRunManifest(manifestJSON)
	if err != nil {
		t.Fatal(err)
	}
	manifest.ReportSHA256 = fmt.Sprintf("%x", sha256.Sum256(canonicalJSON))
	manifestJSON, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestJSON, 0o600); err != nil {
		t.Fatal(err)
	}

	meta := metadata{
		RepoName:  canonical.RepoName,
		RepoPath:  repository,
		CreatedAt: runID,
	}
	meta.EffectiveOptions.ReportLanguage = localization.LocaleRussian
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(runDir, "metadata.json"),
		metaJSON,
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	direct, err := reportpkg.PreparePresentationLocalization(
		canonical,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}
	producerData, err := reportpkg.PrepareRunPresentation(runDir, canonical, nil)
	if err != nil {
		t.Fatal(err)
	}
	producer, err := reportpkg.PreparePresentationLocalization(
		producerData,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(producer.Input.Fields) <= len(direct.Input.Fields) ||
		producer.Canonical.SHA256 == direct.Canonical.SHA256 {
		t.Fatalf(
			"fixture did not reproduce pre/post-enrichment mismatch: direct=%d/%s producer=%d/%s",
			len(direct.Input.Fields),
			direct.Canonical.SHA256,
			len(producer.Input.Fields),
			producer.Canonical.SHA256,
		)
	}
	if got, marshalErr := json.Marshal(canonical); marshalErr != nil ||
		!bytes.Equal(got, canonicalState) {
		t.Fatal("producer preparation mutated canonical report state")
	}

	translations := make(map[string]string, len(producer.Input.Fields))
	for _, field := range producer.Input.Fields {
		parts := []string{"Русский текст"}
		for _, placeholder := range field.Placeholders {
			for count := 0; count < placeholder.Count; count++ {
				parts = append(parts, placeholder.Token)
			}
		}
		translations[field.ID] = strings.Join(parts, " ")
	}
	projection := localization.Projection{
		Version:         localization.ProjectionVersion,
		CanonicalSHA256: producer.Canonical.SHA256,
		Locale:          localization.LocaleRussian,
		Translations:    translations,
	}
	if err := reportpkg.WritePresentationLocalizationSuccess(
		runDir,
		producer,
		projection,
		true,
		"request-sha",
		"cache-key",
	); err != nil {
		t.Fatal(err)
	}

	producerState, err := json.Marshal(producerData)
	if err != nil {
		t.Fatal(err)
	}
	ruData, ruStatus := reportpkg.LoadPresentationLocalization(
		runDir,
		producerData,
		localization.LocaleRussian,
	)
	if ruStatus.State != reportpkg.PresentationLocalizationSucceeded ||
		ruData.ReportLanguage != localization.LocaleRussian ||
		ruData.ProjectGuess != "Русский текст" {
		t.Fatalf("producer RU projection/status = %#v/%#v", ruData, ruStatus)
	}
	if got, marshalErr := json.Marshal(producerData); marshalErr != nil ||
		!bytes.Equal(got, producerState) {
		t.Fatal("RU projection load mutated prepared canonical presentation")
	}
	enData, enStatus := reportpkg.LoadPresentationLocalization(
		runDir,
		producerData,
		localization.LocaleEnglish,
	)
	if enStatus.State != "" || enData.ReportLanguage != "" ||
		enData.ProjectGuess != "repository orientation" {
		t.Fatalf("EN presentation unexpectedly used RU projection: %#v/%#v", enData, enStatus)
	}

	var logs []string
	handler, err := NewHandler(Options{
		RunsDir:      runsDir,
		InitialRunID: runID,
		Capability:   testCapability,
		CaptureRepository: func(
			context.Context,
			string,
		) (freshness.RepositoryState, error) {
			return manifest.RepositoryState, nil
		},
		Logf: func(format string, args ...any) {
			logs = append(logs, fmt.Sprintf(format, args...))
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	response, err := server.Client().Get(
		server.URL + capabilityURLPrefix(testCapability) +
			"/runs/" + runID + "/report.html",
	)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("serve status = %d: %s", response.StatusCode, body)
	}
	for _, required := range []string{
		`"report_language":"ru"`,
		`rm-localization-status--succeeded`,
		"Русский текст",
	} {
		if !strings.Contains(string(body), required) {
			t.Fatalf("localized report is missing %q", required)
		}
	}
	for _, line := range logs {
		if strings.Contains(line, "localization failed") {
			t.Fatalf("server rejected producer projection: %s", line)
		}
	}
	if got, readErr := os.ReadFile(reportPath); readErr != nil ||
		!bytes.Equal(got, canonicalJSON) {
		t.Fatal("producer/server preparation or RU load changed canonical report.json")
	}
	t.Logf(
		"presentation inventory direct=%d shared=%d sha=%s",
		len(direct.Input.Fields),
		len(producer.Input.Fields),
		producer.Canonical.SHA256,
	)
}

func TestServeRussianProjectionHydratesStudyAndTaskWarningMetadata(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	runsDir := t.TempDir()
	const runID = "20260731-140000-localized-warning-hydration"
	writeRun(t, runsDir, runID, repository, "canonical report")
	runDir := filepath.Join(runsDir, runID)
	copyTaskWarningFixture(t, runDir)
	if err := os.WriteFile(
		filepath.Join(runDir, "snapshot.json"),
		[]byte("{}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	const confidenceWarning = "local confidence gate capped orientation from 0.90 to 0.60 because focused retrieval is incomplete"
	orientationJSON := []byte(`{"confidence":0.6,"warnings":["` + confidenceWarning + `"]}`)
	if err := os.WriteFile(
		filepath.Join(runDir, "orientation_report.json"),
		orientationJSON,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	warningSidecar, err := orient.EncodeConfidenceWarningDiagnostics(
		orientationJSON,
		[]orient.ConfidenceWarningDiagnostic{{
			WarningIndex:   0,
			Code:           orient.ConfidenceWarningOrientationCapped,
			CandidateIndex: -1,
			Proposed:       0.9,
			Capped:         0.6,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(runDir, orient.ConfidenceWarningDiagnosticsFile),
		warningSidecar,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	modelBundleJSON := []byte("{}\n")
	if err := os.WriteFile(
		filepath.Join(runDir, "llm_bundle.json"),
		modelBundleJSON,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	writeOrientationSelectionFixture(t, runDir, modelBundleJSON)
	if err := os.WriteFile(
		filepath.Join(runDir, studymap.StatusFile),
		[]byte(`{
  "version": 1,
  "state": "failed",
  "failure_reason": "no_supported_source_adapter"
}
`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	canonical, err := reportpkg.ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	canonical.FormatVersion = reportpkg.CurrentFormatVersion
	canonical.RepoName = "example.test/fuego"
	reportPath := filepath.Join(runDir, "report.json")
	if err := reportpkg.WriteReportJSON(canonical, reportPath); err != nil {
		t.Fatal(err)
	}
	reportJSON, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(runDir, reportpkg.RunManifestFilename)
	manifestJSON, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := reportpkg.DecodeRunManifest(manifestJSON)
	if err != nil {
		t.Fatal(err)
	}
	manifest.ReportSHA256 = fmt.Sprintf("%x", sha256.Sum256(reportJSON))
	manifest.SnapshotSHA256 = taskWarningFixtureSHA256(t, runDir, "snapshot.json")
	manifest.OpenablePaths = append([]string(nil), canonical.OpenablePaths...)
	manifest.MaterialInputs.ModelBundleSHA256 = taskWarningFixtureSHA256(
		t,
		runDir,
		"llm_bundle.json",
	)
	manifest.MaterialInputs.OrientationContextSelectionSHA256 = taskWarningFixtureSHA256(
		t,
		runDir,
		llmbundle.OrientationContextSelectionFilename,
	)
	manifest.MaterialInputs.TaskBundleSHA256 = taskWarningFixtureSHA256(
		t,
		runDir,
		tasklens.BundleFile,
	)
	manifest.MaterialInputs.TaskAttemptSHA256 = taskWarningFixtureSHA256(
		t,
		runDir,
		tasklens.AttemptFile,
	)
	manifest.MaterialInputs.TaskPackSHA256 = taskWarningFixtureSHA256(
		t,
		runDir,
		tasklens.PackFile,
	)
	manifest.MaterialInputs.TaskStatusSHA256 = taskWarningFixtureSHA256(
		t,
		runDir,
		tasklens.StatusFile,
	)
	manifest.MaterialInputs.TaskRetrievalTraceSHA256 = taskWarningFixtureSHA256(
		t,
		runDir,
		tasklens.TraceJSONFile,
	)
	manifest.MaterialInputs.TaskRetrievalTraceMarkdownSHA256 = taskWarningFixtureSHA256(
		t,
		runDir,
		tasklens.TraceMarkdownFile,
	)
	manifestJSON, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestJSON, 0o600); err != nil {
		t.Fatal(err)
	}

	meta := metadata{
		RepoName:  canonical.RepoName,
		RepoPath:  repository,
		CreatedAt: runID,
	}
	meta.EffectiveOptions.ReportLanguage = localization.LocaleRussian
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(runDir, "metadata.json"),
		metaJSON,
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	prepared, err := reportpkg.PreparePresentationLocalization(
		canonical,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}
	translations := make(map[string]string, len(prepared.Input.Fields))
	for _, field := range prepared.Input.Fields {
		parts := []string{"Русский текст"}
		for _, placeholder := range field.Placeholders {
			for count := 0; count < placeholder.Count; count++ {
				parts = append(parts, placeholder.Token)
			}
		}
		translations[field.ID] = strings.Join(parts, " ")
	}
	projection := localization.Projection{
		Version:         localization.ProjectionVersion,
		CanonicalSHA256: prepared.Canonical.SHA256,
		Locale:          localization.LocaleRussian,
		Translations:    translations,
	}
	if err := reportpkg.WritePresentationLocalizationSuccess(
		runDir,
		prepared,
		projection,
		true,
		"request-sha",
		"cache-key",
	); err != nil {
		t.Fatal(err)
	}

	handler, err := NewHandler(Options{
		RunsDir:      runsDir,
		InitialRunID: runID,
		Capability:   testCapability,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	response, err := server.Client().Get(
		server.URL + capabilityURLPrefix(testCapability) +
			"/runs/" + runID + "/report.html",
	)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("serve status = %d: %s", response.StatusCode, body)
	}
	for _, required := range []string{
		`"report_language":"ru"`,
		`"main.warning.study_no_source_adapter"`,
		`"main.warning.confidence_orientation_capped_incomplete"`,
		`"presentation_warnings":[{"message_id":"main.task_lens.warning.anchor_explanation_replaced","index":1}]`,
		"Русский текст",
	} {
		if !strings.Contains(string(body), required) {
			t.Fatalf("localized report is missing %q", required)
		}
	}

	if err := os.WriteFile(
		filepath.Join(runDir, orient.ConfidenceWarningDiagnosticsFile),
		[]byte(`{"version":99,"orientation_report_sha256":"","diagnostics":[]}`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	degradedResponse, err := server.Client().Get(
		server.URL + capabilityURLPrefix(testCapability) +
			"/runs/" + runID + "/report.html",
	)
	if err != nil {
		t.Fatal(err)
	}
	degradedBody, readErr := io.ReadAll(degradedResponse.Body)
	degradedResponse.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if degradedResponse.StatusCode != http.StatusOK {
		t.Fatalf(
			"degraded serve status = %d: %s",
			degradedResponse.StatusCode,
			degradedBody,
		)
	}
	for _, required := range []string{
		`"report_language":"ru"`,
		`rm-localization-status--failed`,
		`data-rm-message="main.localization.ru_unavailable_canonical_en"`,
		confidenceWarning,
	} {
		if !strings.Contains(string(degradedBody), required) {
			t.Fatalf("degraded localized report is missing %q", required)
		}
	}
}

func copyTaskWarningFixture(t *testing.T, runDir string) {
	t.Helper()
	for _, name := range []string{
		tasklens.BundleFile,
		tasklens.AttemptFile,
		tasklens.PackFile,
		tasklens.StatusFile,
		tasklens.TraceJSONFile,
		tasklens.TraceMarkdownFile,
	} {
		raw, err := os.ReadFile(filepath.Join("testdata", "task-warning", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runDir, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func taskWarningFixtureSHA256(t *testing.T, runDir, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(runDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

func TestLoadRunsKeepsCanonicalRunWithInvalidOptionalLocalizationSidecars(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(t *testing.T, runDir string)
	}{
		{
			name: "missing",
			mutate: func(*testing.T, string) {
			},
		},
		{
			name: "malformed regular status",
			mutate: func(t *testing.T, runDir string) {
				t.Helper()
				if err := os.WriteFile(
					filepath.Join(runDir, reportpkg.PresentationLocalizationStatusFile),
					[]byte("{"),
					0o600,
				); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "status is directory",
			mutate: func(t *testing.T, runDir string) {
				t.Helper()
				if err := os.Mkdir(
					filepath.Join(runDir, reportpkg.PresentationLocalizationStatusFile),
					0o700,
				); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repository := t.TempDir()
			if err := os.WriteFile(
				filepath.Join(repository, "batch.go"),
				[]byte("package example\n"),
				0o600,
			); err != nil {
				t.Fatal(err)
			}
			runsDir := t.TempDir()
			const runID = "20260731-120000-localization"
			writeRun(t, runsDir, runID, repository, "canonical report")
			runDir := filepath.Join(runsDir, runID)
			metadataPath := filepath.Join(runDir, "metadata.json")
			meta := metadata{
				RepoName:  filepath.Base(repository),
				RepoPath:  repository,
				CreatedAt: runID,
			}
			meta.EffectiveOptions.ReportLanguage = "ru"
			metadataJSON, err := json.Marshal(meta)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(metadataPath, metadataJSON, 0o600); err != nil {
				t.Fatal(err)
			}
			test.mutate(t, runDir)

			runs, err := (&handler{runsDir: runsDir}).loadRuns()
			if err != nil {
				t.Fatalf("loadRuns: %v", err)
			}
			if len(runs) != 1 || runs[0].ID != runID {
				t.Fatalf("loaded runs = %#v, want canonical run %q", runs, runID)
			}
			if runs[0].Report == nil || runs[0].Manifest == nil {
				t.Fatalf(
					"canonical authority missing: report=%v manifest=%v",
					runs[0].Report != nil,
					runs[0].Manifest != nil,
				)
			}
			if runs[0].RequestedLocale != "ru" {
				t.Fatalf("requested locale = %q, want ru", runs[0].RequestedLocale)
			}
		})
	}
}

func TestRunArtifactSignatureKeepsStrictCanonicalAndAuthorityArtifacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{name: "report json", path: "report.json"},
		{name: "run manifest", path: reportpkg.RunManifestFilename},
		{name: "investigation status", path: "task_investigation_status.json"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repository := t.TempDir()
			runsDir := t.TempDir()
			const runID = "20260731-130000-strict"
			writeRun(t, runsDir, runID, repository, "canonical report")
			artifactPath := filepath.Join(runsDir, runID, test.path)
			if err := os.Remove(artifactPath); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if err := os.Mkdir(artifactPath, 0o700); err != nil {
				t.Fatal(err)
			}

			if _, err := runArtifactSignature(filepath.Join(runsDir, runID)); err == nil {
				t.Fatalf("runArtifactSignature accepted non-regular %s", test.path)
			}
		})
	}
}
