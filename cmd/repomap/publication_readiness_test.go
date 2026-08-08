package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/mechanismstudy"
	"github.com/dvordrova/repomap/internal/report"
)

func TestAssessPublicationDistinguishesReadyDegradedAndFailed(t *testing.T) {
	t.Parallel()

	ready := readyPublicationFixture()
	tests := []struct {
		name    string
		mutate  func(*report.ReportData)
		want    publicationReadiness
		reasons []publicationReason
	}{
		{name: "complete current report", want: publicationReady},
		{
			name: "partial Architecture remains publishable but degraded",
			mutate: func(data *report.ReportData) {
				data.ArchitectureSynthesis.ProposalPartial = true
				data.ArchitectureSynthesis.ArchitectureSource = string(componentmap.SourcePartialModel)
				data.ArchitectureCanvas.ArchitectureSource = componentmap.SourcePartialModel
				data.ArchitectureCanvas.ValidationOutcome = componentmap.ValidationAcceptedPartial
			},
			want:    publicationDegraded,
			reasons: []publicationReason{publicationReasonArchitecturePartial},
		},
		{
			name: "unavailable Study remains publishable but degraded",
			mutate: func(data *report.ReportData) {
				data.AtlasStudy = &report.AtlasStudyReportStatus{State: atlasstudy.ProductStateUnavailable}
			},
			want:    publicationDegraded,
			reasons: []publicationReason{publicationReasonStudyUnavailable},
		},
		{
			name: "explicit offline stages satisfy the requested local mode",
			mutate: func(data *report.ReportData) {
				data.ArchitectureSynthesis = &report.ArchitectureSynthesisStatus{
					State:           report.ArchitectureSynthesisUnavailable,
					UnavailableCode: report.ArchitectureSynthesisUnavailableOfflineCode,
				}
				data.ArchitectureCanvas.ArchitectureSource = componentmap.SourcePackageFallback
				data.ArchitectureCanvas.ValidationOutcome = componentmap.ValidationAccepted
				data.AtlasStudy = &report.AtlasStudyReportStatus{
					State:           atlasstudy.ProductStateUnavailable,
					UnavailableCode: report.AtlasStudyUnavailableOffline,
				}
			},
			want: publicationReady,
		},
		{
			name: "accepted but incomplete Study is degraded",
			mutate: func(data *report.ReportData) {
				data.AtlasStudy.FrontierComplete = false
			},
			want:    publicationDegraded,
			reasons: []publicationReason{publicationReasonStudyIncomplete},
		},
		{
			name: "failed Architecture and Study are bounded reasons",
			mutate: func(data *report.ReportData) {
				data.ArchitectureSynthesis.State = report.ArchitectureSynthesisFailed
				data.AtlasStudy = &report.AtlasStudyReportStatus{State: atlasstudy.ProductStateFailed}
			},
			want: publicationDegraded,
			reasons: []publicationReason{
				publicationReasonArchitectureFailed,
				publicationReasonStudyFailed,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := clonePublicationFixture(t, ready)
			if test.mutate != nil {
				test.mutate(data)
			}
			got := assessPublication(data)
			if got.Status != test.want || !reflect.DeepEqual(got.Reasons, test.reasons) {
				t.Fatalf("assessment = %#v, want status %q reasons %#v", got, test.want, test.reasons)
			}
		})
	}

	if got := assessPublication(nil); got.Status != publicationFailed ||
		!reflect.DeepEqual(got.Reasons, []publicationReason{publicationReasonArtifactsInvalid}) {
		t.Fatalf("nil publication assessment = %#v", got)
	}
}

func TestStudyInvestigationPublicationReasonsRemainConsoleOnly(t *testing.T) {
	t.Parallel()

	if got, err := studyInvestigationPublicationReasons(t.TempDir(), report.MaterialInputs{}); err != nil || len(got) != 0 {
		t.Fatalf("absent family = %#v, %v, want no reason", got, err)
	}
	for _, test := range []struct {
		state mechanismstudy.StatusState
		want  []publicationReason
	}{
		{state: mechanismstudy.StatusComplete},
		{state: mechanismstudy.StatusPartial, want: []publicationReason{publicationReasonInvestigationPartial}},
		{state: mechanismstudy.StatusFailed, want: []publicationReason{publicationReasonInvestigationFailed}},
	} {
		got, err := studyInvestigationStatusReasons(mechanismstudy.Status{State: test.state})
		if err != nil || !reflect.DeepEqual(got, test.want) {
			t.Fatalf("state %q reasons = %#v, %v, want %#v", test.state, got, err, test.want)
		}
		if len(got) != 0 {
			assessment := publicationAssessment{Status: publicationDegraded, Reasons: got}
			if detail := assessment.consoleDetails(); len(detail) != 1 || !strings.Contains(detail[0], "Study investigation") {
				t.Fatalf("state %q console details = %#v", test.state, detail)
			}
		}
	}
}

func TestAssessRunPublicationFailsClosedWithoutManifestIntegrity(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), []byte(`{"format_version":39}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := assessRunPublication(runDir)
	if err == nil {
		t.Fatal("report existence without a verified manifest was accepted")
	}
	if got.Status != publicationFailed ||
		!reflect.DeepEqual(got.Reasons, []publicationReason{publicationReasonArtifactsInvalid}) {
		t.Fatalf("assessment = %#v, want FAILED integrity reason", got)
	}

	facts := collectCorpusRunFacts(corpusRepo{Repository: "example/repo"}, runDir)
	if facts.PublicationStatus != publicationFailed ||
		!reflect.DeepEqual(facts.PublicationReasons, []publicationReason{publicationReasonArtifactsInvalid}) {
		t.Fatalf("corpus facts = %#v, want FAILED integrity reason", facts)
	}
}

func TestVerifyPublishedHTMLRejectsMissingAndInvalidApplicationShells(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	path := filepath.Join(runDir, "report.html")
	if err := verifyPublishedHTML(path); err == nil {
		t.Fatal("missing report HTML was accepted")
	}
	for _, test := range []struct {
		name string
		raw  string
	}{
		{name: "empty", raw: ""},
		{name: "arbitrary html", raw: "<html><body>not a repomap report</body></html>"},
		{name: "data marker without html shell", raw: `<script id="rm-report-data">{}</script>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(test.raw), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := verifyPublishedHTML(path); err == nil {
				t.Fatalf("invalid report HTML was accepted: %q", test.raw)
			}
		})
	}
	if err := os.WriteFile(path, []byte(`<html><script id="rm-report-data">{}</script></html>`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyPublishedHTML(path); err != nil {
		t.Fatalf("bounded report application shell rejected: %v", err)
	}
}

func TestRunCorpusCLIRecordsMissingRepositoryAsFailed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	matrixPath := filepath.Join(t.TempDir(), "matrix.json")
	matrix := corpusMatrix{Repositories: []corpusRepo{{Repository: "service/example"}}}
	raw, err := json.Marshal(matrix)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(matrixPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = runCorpusCLI([]string{root, "--matrix", matrixPath}, &output)
	if err == nil || !strings.Contains(err.Error(), "1 publication(s) failed integrity") {
		t.Fatalf("runCorpusCLI error = %v", err)
	}
	for _, want := range []string{"service/example", "FAILED", "artifacts_missing_or_invalid"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("corpus output missing %q:\n%s", want, output.String())
		}
	}
	acceptedJSON, err := os.ReadFile(filepath.Join(root, "corpus_acceptance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var facts []corpusRunFacts
	if err := json.Unmarshal(acceptedJSON, &facts); err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || facts[0].PublicationStatus != publicationFailed ||
		!reflect.DeepEqual(facts[0].PublicationReasons, []publicationReason{publicationReasonArtifactsInvalid}) {
		t.Fatalf("corpus acceptance = %#v", facts)
	}
}

func readyPublicationFixture() *report.ReportData {
	return &report.ReportData{
		ArchitectureCanvas: &report.ArchitectureCanvas{
			ValidationOutcome:  componentmap.ValidationAccepted,
			ArchitectureSource: componentmap.SourceValidatedModel,
			Components:         []report.ArchitectureComponent{{}},
		},
		ArchitectureSynthesis: &report.ArchitectureSynthesisStatus{
			State:              report.ArchitectureSynthesisSucceeded,
			ProposalAccepted:   true,
			ArchitectureSource: string(componentmap.SourceValidatedModel),
		},
		AtlasStudy: &report.AtlasStudyReportStatus{
			State:                   atlasstudy.ProductStateAccepted,
			FrontierComplete:        true,
			SelectedItemsComplete:   true,
			SupportCoverageComplete: true,
			PortfolioTargetMet:      true,
			Themes: &report.AtlasStudyThemesProjection{
				Cards: []report.StudyThemeCard{{FinalTitle: "Theme"}},
			},
		},
	}
}

func clonePublicationFixture(t *testing.T, data *report.ReportData) *report.ReportData {
	t.Helper()
	clone := *data
	canvas := *data.ArchitectureCanvas
	canvas.Components = append([]report.ArchitectureComponent(nil), data.ArchitectureCanvas.Components...)
	clone.ArchitectureCanvas = &canvas
	architecture := *data.ArchitectureSynthesis
	clone.ArchitectureSynthesis = &architecture
	study := *data.AtlasStudy
	themes := *data.AtlasStudy.Themes
	themes.Cards = append([]report.StudyThemeCard(nil), data.AtlasStudy.Themes.Cards...)
	study.Themes = &themes
	clone.AtlasStudy = &study
	return &clone
}
