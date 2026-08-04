package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const goldenMechanismV03FixtureSHA256 = "c19cead4c702bf5799628e8f0814ac96ecca6fd7ad7674040f8ffb03d1c934bc"

const (
	goldenMechanismCaddyArtifactSHA256 = "baa7fcaa8a1f08acc511fef64fc77c03847602413c99a20ef23e80222fe81df1"
	goldenMechanismCaddyProbeSHA256    = "5a8c93ed65dc166e52fea16613d9a3a0587700dfc8b83e8d952a092e64b9ba48"
	goldenMechanismCaddyID             = "semantic-mechanism-6b311068024809a10f1e24eb"
)

func TestGoldenMechanismV03AcceptsSavedResponseWithoutMandatoryFactUse(t *testing.T) {
	t.Parallel()

	input := goldenMechanismV02UnitInput(t)
	raw, err := os.ReadFile(filepath.Join(
		"testdata",
		"golden_mechanism_caddy_response_v3.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	if digestSHA256(raw) != goldenMechanismV03FixtureSHA256 {
		t.Fatal("saved v0.2 regression fixture changed")
	}
	parsed, err := semanticdiscovery.ParseFanInArtifact(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := semanticdiscovery.ValidatePartialFanInArtifact(
		input.bundle,
		[]semanticdiscovery.LeafResult{input.leaf},
		parsed,
	); err != nil {
		t.Fatalf("fixed response fan-in validation error = %v", err)
	}
	evaluated, err := evaluateGoldenMechanismResponse(
		input.bundle,
		input.proposal,
		input.leaf,
		raw,
	)
	if err != nil {
		t.Fatalf(
			"evaluateGoldenMechanismResponse() error = %v; task = %s; reduction = %#v",
			err,
			input.leaf.Task.ID,
			evaluated.Reduction,
		)
	}
	proposal := evaluated.FanIn.Artifacts[0]
	if err := semanticdiscovery.ValidateLocalSequenceClaims(
		input.bundle,
		proposal,
		goldenDirectoryListingEntryFactID,
		input.sequenceFact.ID,
	); err != nil {
		t.Fatalf("ValidateLocalSequenceClaims() error = %v", err)
	}
	assessment, err := semanticdiscovery.AssessClaimCoverage(
		input.bundle,
		[]semanticdiscovery.LeafResult{input.leaf},
		proposal,
	)
	if err != nil {
		t.Fatalf("AssessClaimCoverage() error = %v", err)
	}
	artifact := evaluated.Artifacts[0]
	if artifact.Verdict != semanticdiscovery.VerdictMixed ||
		len(evaluated.Reduction.VerdictDiagnostics) != 0 {
		t.Fatalf(
			"Caddy verdict = %q, diagnostics = %#v",
			artifact.Verdict,
			evaluated.Reduction.VerdictDiagnostics,
		)
	}
	artifactSHA, err := goldenMechanismArtifactSHA256(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if artifactSHA != goldenMechanismCaddyArtifactSHA256 {
		t.Fatalf("Caddy artifact SHA = %s, want %s", artifactSHA, goldenMechanismCaddyArtifactSHA256)
	}
	if err := validateGoldenMechanismV03Assessment(
		input.projection.Candidate,
		input.sequenceFact.ID,
		assessment,
		artifact,
	); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(assessment.UsedFactIDs, goldenMechanismFixedFactIDs) ||
		!slices.Equal(
			assessment.UnusedAvailableFactIDs,
			[]string{goldenMechanismSequenceFactID},
		) ||
		!slices.Equal(assessment.TemporalClaimIndexes, []int{1, 3}) ||
		len(assessment.CoveredAspectIDs) != 7 ||
		!slices.Equal(assessment.UncoveredAspectIDs, []string{"known_unknowns"}) {
		t.Fatalf("assessment = %#v", assessment)
	}
	summary, err := summarizeGoldenMechanismArtifact(
		input.projection.Candidate,
		artifact,
	)
	if err != nil {
		t.Fatalf("summarizeGoldenMechanismArtifact() error = %v", err)
	}
	if summary.LocalRubricScore != 4 {
		t.Fatalf("rubric score = %d, want 4", summary.LocalRubricScore)
	}
	replayed, err := semanticdiscovery.ReplayRecord(input.bundle, evaluated.RecordBytes)
	if err != nil {
		t.Fatalf("ReplayRecord() error = %v", err)
	}
	if len(replayed) != 1 ||
		!slices.Equal(replayed[0].UsedFactIDs, artifact.UsedFactIDs) ||
		!slices.Equal(
			replayed[0].UnusedAvailableFactIDs,
			artifact.UnusedAvailableFactIDs,
		) {
		t.Fatalf("replay usage metadata = %#v", replayed)
	}
	replayedSHA, err := goldenMechanismArtifactSHA256(replayed[0])
	if err != nil {
		t.Fatal(err)
	}
	if replayed[0].Verdict != semanticdiscovery.VerdictMixed || replayedSHA != artifactSHA {
		t.Fatalf("Caddy replay verdict/hash = %q/%s", replayed[0].Verdict, replayedSHA)
	}
	record, err := semanticdiscovery.DecodeRecord(evaluated.RecordBytes)
	if err != nil {
		t.Fatal(err)
	}
	mechanism, projected, err := semanticdiscovery.ExtractMechanism(
		input.bundle,
		record,
		input.projection.Candidate.ID,
		caddyDirectoryMechanismIdentity,
		semanticdiscovery.MechanismProbeInput{
			ContractVersion: 1,
			ID:              caddyDirectoryMechanismIdentity.IntentKey,
			SHA256:          goldenMechanismCaddyProbeSHA256,
		},
	)
	if err != nil {
		t.Fatalf("ExtractMechanism() error = %v", err)
	}
	projectedSHA, err := goldenMechanismArtifactSHA256(projected)
	if err != nil {
		t.Fatal(err)
	}
	if projected.Verdict != semanticdiscovery.VerdictMixed ||
		projectedSHA != goldenMechanismCaddyArtifactSHA256 ||
		mechanism.ID != goldenMechanismCaddyID {
		t.Fatalf(
			"Caddy mechanism regression: verdict=%q artifact_sha=%s id=%s",
			projected.Verdict,
			projectedSHA,
			mechanism.ID,
		)
	}
}

func TestGoldenMechanismV03RequiresEveryKeyAspect(t *testing.T) {
	t.Parallel()

	input := goldenMechanismV02UnitInput(t)
	raw, err := os.ReadFile(filepath.Join(
		"testdata",
		"golden_mechanism_caddy_response_v3.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	evaluated, err := evaluateGoldenMechanismResponse(
		input.bundle,
		input.proposal,
		input.leaf,
		raw,
	)
	if err != nil {
		t.Fatalf("evaluation error = %v; reduction = %#v", err, evaluated.Reduction)
	}
	artifact := evaluated.Artifacts[0]
	for index := range artifact.Statements {
		if slices.Contains(artifact.Statements[index].AspectIDs, "response_output") {
			artifact.Statements[index].AspectIDs = slices.DeleteFunc(
				artifact.Statements[index].AspectIDs,
				func(id string) bool { return id == "response_output" },
			)
		}
	}
	_, err = summarizeGoldenMechanismArtifact(input.projection.Candidate, artifact)
	if err == nil || !strings.Contains(err.Error(), "every key answer aspect") {
		t.Fatalf("summarizeGoldenMechanismArtifact() error = %v", err)
	}
}

func TestGoldenMechanismV03ReplayRefusesAuthorizedRun(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	manifestPath := filepath.Join(runDir, "run_manifest.json")
	want := []byte("authority must remain untouched")
	if err := os.WriteFile(manifestPath, want, 0o600); err != nil {
		t.Fatal(err)
	}
	err := replayGoldenMechanismV03(
		context.Background(),
		runDir,
		io.Discard,
		io.Discard,
	)
	if err == nil || !strings.Contains(err.Error(), "refuses an authorized run") {
		t.Fatalf("replayGoldenMechanismV03() error = %v", err)
	}
	got, readErr := os.ReadFile(manifestPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(want) {
		t.Fatalf("manifest = %q, want %q", got, want)
	}
}
