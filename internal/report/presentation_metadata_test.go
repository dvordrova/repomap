package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/studymap"
	"github.com/dvordrova/repomap/internal/tasklens"
)

func TestHydrateRunPresentationMetadataRestoresTypedStudyAndTaskWarnings(
	t *testing.T,
) {
	t.Parallel()

	runDir := t.TempDir()
	bundle, attempt, pack, status := taskInvestigationArtifactFixture(t)
	proposal, err := tasklens.DecodeProposal([]byte(attempt.RawResponse))
	if err != nil {
		t.Fatal(err)
	}
	proposal.Anchors[0].Why = ""
	pack, warnings, _, err := tasklens.ReduceProposalWithDiagnostics(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 {
		t.Fatalf("reduction warnings = %#v, want one typed warning", warnings)
	}
	pack, sufficient := FinalizeTaskInvestigationPack(
		pack,
		"accepted_with_rejections",
	)
	responseRaw, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	attempt.State = "accepted_with_rejections"
	attempt.RawResponse = string(responseRaw)
	attempt.ResponseSHA256 = tasklens.SHA256(responseRaw)
	attempt.Provider.ResponseBytes = len(responseRaw)
	attempt.Warnings = warnings
	status.State = "accepted_partial"
	status.Sufficient = sufficient
	status.Provider = attempt.Provider
	status.Warnings = append(append([]string(nil), warnings...), pack.Warnings...)
	writeTaskInvestigationArtifacts(t, runDir, bundle, attempt, pack, status)

	if err := os.WriteFile(
		filepath.Join(runDir, "snapshot.json"),
		[]byte("{}\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
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

	replayed, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.TaskInvestigation == nil {
		t.Fatalf("Task Lens replay is absent; warnings = %#v", replayed.Warnings)
	}
	canonicalJSON, err := json.Marshal(reportDataForPersistence(replayed))
	if err != nil {
		t.Fatal(err)
	}
	var canonical ReportData
	if err := json.Unmarshal(canonicalJSON, &canonical); err != nil {
		t.Fatal(err)
	}
	if len(canonical.PresentationWarningKinds) != 0 ||
		len(canonical.TaskInvestigation.PresentationWarnings) != 0 ||
		len(canonical.TaskInvestigation.warningDiagnostics) != 0 {
		t.Fatalf("canonical report retained transient metadata: %#v", canonical)
	}
	var mismatched ReportData
	if err := json.Unmarshal(canonicalJSON, &mismatched); err != nil {
		t.Fatal(err)
	}
	mismatched.TaskInvestigation.Task = "different canonical task"
	if err := hydratePresentationMetadataFromReplay(
		&mismatched,
		replayed,
	); err == nil {
		t.Fatal("hydration accepted a different canonical Task Lens projection")
	}
	if len(mismatched.PresentationWarningKinds) != 0 ||
		len(mismatched.TaskInvestigation.warningDiagnostics) != 0 {
		t.Fatalf("failed hydration partially mutated metadata: %#v", mismatched)
	}

	if err := HydrateRunPresentationMetadata(runDir, &canonical); err != nil {
		t.Fatal(err)
	}
	if len(canonical.PresentationWarningKinds) != len(canonical.Warnings) ||
		canonical.PresentationWarningKinds[len(canonical.PresentationWarningKinds)-1] !=
			studyPublicationMessageNoSourceAdapter {
		t.Fatalf(
			"Study presentation warning kinds = %#v",
			canonical.PresentationWarningKinds,
		)
	}
	if len(canonical.TaskInvestigation.warningDiagnostics) != 1 ||
		canonical.TaskInvestigation.warningDiagnostics[0].Code !=
			tasklens.WarningAnchorExplanationReplaced {
		t.Fatalf(
			"Task Lens diagnostics = %#v",
			canonical.TaskInvestigation.warningDiagnostics,
		)
	}
	rendered := reportDataForRendering(&canonical)
	if len(rendered.TaskInvestigation.PresentationWarnings) != 1 ||
		rendered.TaskInvestigation.PresentationWarnings[0].MessageID !=
			"main.task_lens.warning.anchor_explanation_replaced" {
		t.Fatalf(
			"Task Lens presentation warnings = %#v",
			rendered.TaskInvestigation.PresentationWarnings,
		)
	}
}
