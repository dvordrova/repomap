package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/studymap"
)

type semanticJournalEntry struct {
	record   debugdump.SemanticExchangeRecord
	request  []byte
	response []byte
}

func openSemanticJournalTestWriter(t *testing.T, runDir string) *debugdump.Writer {
	t.Helper()
	writer, err := debugdump.OpenWriter(runDir, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	return writer
}

func readSemanticJournalEntries(t *testing.T, runDir string) []semanticJournalEntry {
	t.Helper()
	directories, err := os.ReadDir(filepath.Join(runDir, debugdump.SemanticExchangesDir))
	if err != nil {
		t.Fatal(err)
	}
	entries := make([]semanticJournalEntry, 0, len(directories))
	for _, directory := range directories {
		if !directory.IsDir() {
			continue
		}
		dir := filepath.Join(runDir, debugdump.SemanticExchangesDir, directory.Name())
		metadata, err := os.ReadFile(filepath.Join(dir, debugdump.SemanticExchangeMetaFile))
		if err != nil {
			t.Fatal(err)
		}
		var record debugdump.SemanticExchangeRecord
		if err := json.Unmarshal(metadata, &record); err != nil {
			t.Fatal(err)
		}
		request, err := os.ReadFile(filepath.Join(dir, record.Request.File))
		if err != nil {
			t.Fatal(err)
		}
		response, err := os.ReadFile(filepath.Join(dir, record.Response.File))
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, semanticJournalEntry{record: record, request: request, response: response})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].record.Stage != entries[j].record.Stage {
			return entries[i].record.Stage < entries[j].record.Stage
		}
		if entries[i].record.InstanceOrdinal != entries[j].record.InstanceOrdinal {
			return entries[i].record.InstanceOrdinal < entries[j].record.InstanceOrdinal
		}
		return entries[i].record.SemanticAttemptOrdinal < entries[j].record.SemanticAttemptOrdinal
	})
	return entries
}

func TestArchitectureSemanticJournalRecordsLiveCacheRejectedAndOmittedRaw(t *testing.T) {
	bundle := architectureSynthesisTestBundle()
	runsDir := t.TempDir()
	response := architectureSynthesisTestResponse(t, bundle)
	provider := &architectureSynthesisStub{response: response}

	liveDir := filepath.Join(runsDir, "live")
	if err := os.Mkdir(liveDir, 0o700); err != nil {
		t.Fatal(err)
	}
	liveWriter := openSemanticJournalTestWriter(t, liveDir)
	if _, err := ensureArchitectureSynthesisWithOptions(
		context.Background(), bundle, liveDir, "journal-revision", "test", "model", provider,
		architectureSynthesisOptions{exchangeWriter: liveWriter},
	); err != nil {
		t.Fatal(err)
	}
	live := readSemanticJournalEntries(t, liveDir)
	if len(live) != 1 || live[0].record.State != debugdump.SemanticStateAccepted ||
		live[0].record.SemanticCalls != 1 || live[0].record.TransportAttempts != 1 ||
		!bytes.Equal(live[0].response, response) {
		t.Fatalf("live architecture journal = %#v", live)
	}

	cacheDir := filepath.Join(runsDir, "cache")
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cacheWriter := openSemanticJournalTestWriter(t, cacheDir)
	outcome, err := ensureArchitectureSynthesisWithOptions(
		context.Background(), bundle, cacheDir, "journal-revision", "test", "model", provider,
		architectureSynthesisOptions{exchangeWriter: cacheWriter},
	)
	if err != nil {
		t.Fatal(err)
	}
	cached := readSemanticJournalEntries(t, cacheDir)
	if !outcome.Cached || provider.calls != 1 || len(cached) != 1 ||
		cached[0].record.State != debugdump.SemanticStateCacheHit ||
		cached[0].record.SemanticCalls != 0 || cached[0].record.TransportAttempts != 0 ||
		!bytes.Equal(cached[0].response, response) {
		t.Fatalf("cached architecture journal/outcome = %#v / %#v", cached, outcome)
	}

	rejectedDir := filepath.Join(runsDir, "rejected")
	if err := os.Mkdir(rejectedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	rejectedWriter := openSemanticJournalTestWriter(t, rejectedDir)
	if _, err := ensureArchitectureSynthesisWithOptions(
		context.Background(), bundle, rejectedDir, "journal-rejected", "test", "model",
		&architectureSynthesisStub{response: []byte("not json")},
		architectureSynthesisOptions{disableCache: true, exchangeWriter: rejectedWriter},
	); err != nil {
		t.Fatal(err)
	}
	rejected := readSemanticJournalEntries(t, rejectedDir)
	if len(rejected) != 1 || rejected[0].record.State != debugdump.SemanticStateRejected ||
		rejected[0].record.ValidationCode != debugdump.SemanticValidationResponse {
		t.Fatalf("rejected architecture journal = %#v", rejected)
	}

	sensitive := []byte(`api_key="company-secret-value-12345"`)
	omittedProvider := &architectureSynthesisStub{response: sensitive}
	omittedLiveDir := filepath.Join(runsDir, "omitted-live")
	omittedCacheDir := filepath.Join(runsDir, "omitted-cache")
	for _, dir := range []string{omittedLiveDir, omittedCacheDir} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ensureArchitectureSynthesis(
		context.Background(), bundle, omittedLiveDir, "journal-omitted", "test", "model", omittedProvider,
	); err != nil {
		t.Fatal(err)
	}
	omittedWriter := openSemanticJournalTestWriter(t, omittedCacheDir)
	if _, err := ensureArchitectureSynthesisWithOptions(
		context.Background(), bundle, omittedCacheDir, "journal-omitted", "test", "model", omittedProvider,
		architectureSynthesisOptions{exchangeWriter: omittedWriter},
	); err != nil {
		t.Fatal(err)
	}
	omitted := readSemanticJournalEntries(t, omittedCacheDir)
	if len(omitted) != 1 || omitted[0].record.State != debugdump.SemanticStateCacheHit ||
		omitted[0].record.Response.Storage != "raw_unavailable" ||
		omitted[0].record.Response.UnavailableCode != debugdump.SemanticUnavailableOmitted ||
		omitted[0].record.Response.OriginalBytes != len(sensitive) ||
		bytes.Contains(omitted[0].response, []byte("company-secret")) {
		t.Fatalf("omitted architecture cache journal = %#v", omitted)
	}
}

func TestGuidedTourSemanticJournalPreservesTwoAttemptAndCacheContracts(t *testing.T) {
	bundle := guidedTourTestBundle()
	runsDir := t.TempDir()
	rejectedResponse := guidedTourTestProposal(t, bundle, true)
	acceptedResponse := guidedTourTestProposal(t, bundle, false)
	runDir := filepath.Join(runsDir, "live")
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writer := openSemanticJournalTestWriter(t, runDir)
	provider := &guidedTourEditorStub{responses: [][]byte{rejectedResponse, acceptedResponse}}
	outcome, err := ensureGuidedTourWithOptions(
		context.Background(), bundle, runDir, "test", "model", provider,
		guidedTourRunOptions{exchangeWriter: writer},
	)
	if err != nil {
		t.Fatal(err)
	}
	entries := readSemanticJournalEntries(t, runDir)
	if outcome.SemanticCalls != 1 || outcome.RetryCount != 1 || provider.calls != 2 || len(entries) != 2 ||
		entries[0].record.SemanticAttemptOrdinal != 1 || entries[0].record.State != debugdump.SemanticStateRejected ||
		entries[1].record.SemanticAttemptOrdinal != 2 || entries[1].record.State != debugdump.SemanticStateAccepted ||
		!bytes.Equal(entries[0].response, rejectedResponse) || !bytes.Equal(entries[1].response, acceptedResponse) {
		t.Fatalf("guided journal/outcome = %#v / %#v", entries, outcome)
	}

	cacheDir := filepath.Join(runsDir, "cache")
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cacheWriter := openSemanticJournalTestWriter(t, cacheDir)
	cachedOutcome, err := ensureGuidedTourWithOptions(
		context.Background(), bundle, cacheDir, "test", "model", provider,
		guidedTourRunOptions{exchangeWriter: cacheWriter},
	)
	if err != nil {
		t.Fatal(err)
	}
	cached := readSemanticJournalEntries(t, cacheDir)
	if !cachedOutcome.Cached || provider.calls != 2 || len(cached) != 1 ||
		cached[0].record.State != debugdump.SemanticStateCacheHit ||
		cached[0].record.SemanticCalls != 0 || cached[0].record.TransportAttempts != 0 ||
		!bytes.Equal(cached[0].response, acceptedResponse) {
		t.Fatalf("guided cache journal/outcome = %#v / %#v", cached, cachedOutcome)
	}
}

func TestGuidedTourSemanticJournalFailureDoesNotChangeAcceptedOutcome(t *testing.T) {
	runDir := t.TempDir()
	writer := openSemanticJournalTestWriter(t, runDir)
	var warnings bytes.Buffer
	writer.SetWarningWriter(&warnings)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	bundle := guidedTourTestBundle()
	outcome, err := ensureGuidedTourWithOptions(
		context.Background(), bundle, runDir, "test", "model",
		&guidedTourEditorStub{response: guidedTourTestProposal(t, bundle, false)},
		guidedTourRunOptions{disableCache: true, exchangeWriter: writer},
	)
	if err != nil || outcome.ValidationState != "accepted" {
		t.Fatalf("accepted Guided Tour changed by journal failure: outcome=%#v err=%v", outcome, err)
	}
	want := "warning: semantic exchange journal unavailable: stage=guided_tour code=artifact_write_failed\n"
	if warnings.String() != want {
		t.Fatalf("journal warning = %q, want %q", warnings.String(), want)
	}
}

func TestStudySemanticJournalRecordsValidatedStagesAndDeterministicReviews(t *testing.T) {
	bundle, directions := studyMapV32ReviewFixture(t)
	provider := &studyMapV32TypedRoundTripProvider{t: t, bundle: bundle, directions: directions}
	runDir := t.TempDir()
	writer := openSemanticJournalTestWriter(t, runDir)
	record, reduction, stages, err := prepareStudyMapV32WithOptions(
		context.Background(), runDir, bundle, provider, studyMapRunOptions{exchangeWriter: writer},
	)
	if err != nil {
		t.Fatal(err)
	}
	entries := readSemanticJournalEntries(t, runDir)
	if len(record.Directions) == 0 || reduction.Reviewed != len(directions.Directions) ||
		len(stages) != 2+len(directions.Directions) || len(entries) != len(stages) {
		t.Fatalf("Study publication/stages/journal = %d/%d/%d/%d", len(record.Directions), reduction.Reviewed, len(stages), len(entries))
	}
	stageCounts := map[string]int{}
	reviewOrdinals := make([]int, 0, len(directions.Directions))
	for _, entry := range entries {
		stageCounts[entry.record.Stage]++
		if entry.record.State != debugdump.SemanticStateAccepted ||
			entry.record.ValidationCode != debugdump.SemanticValidationAccepted ||
			entry.record.SemanticCalls != 1 || entry.record.TransportAttempts != 1 ||
			entry.record.RequestProvenance != debugdump.SemanticRequestPrepared {
			t.Fatalf("Study semantic entry = %#v", entry.record)
		}
		if entry.record.Stage == debugdump.SemanticStageStudyReview {
			reviewOrdinals = append(reviewOrdinals, entry.record.InstanceOrdinal)
		}
	}
	if stageCounts[debugdump.SemanticStageStudyBrief] != 1 ||
		stageCounts[debugdump.SemanticStageStudyDirections] != 1 ||
		stageCounts[debugdump.SemanticStageStudyReview] != len(directions.Directions) {
		t.Fatalf("Study journal stages = %#v", stageCounts)
	}
	for index, ordinal := range reviewOrdinals {
		if ordinal != index+1 {
			t.Fatalf("review ordinals = %v", reviewOrdinals)
		}
	}
}

type studyReviewJournalCacheProvider struct{}

func (studyReviewJournalCacheProvider) SemanticDiscoveryPromptJSON(prompt semanticdiscovery.Prompt) ([]byte, error) {
	return json.Marshal(prompt)
}

func (studyReviewJournalCacheProvider) DiscoverSemanticsMeasured(
	context.Context,
	semanticdiscovery.Prompt,
) (modelresearch.ProviderResult, error) {
	return modelresearch.ProviderResult{}, errors.New("validated cache hit called live provider")
}

func (studyReviewJournalCacheProvider) loadStudyReview(
	_ semanticdiscovery.Prompt,
	_ []byte,
	bundle studymap.Bundle,
	direction studymap.DirectionCandidate,
) (modelresearch.ProviderResult, bool, error) {
	reviewBundle, err := studymap.BuildReviewBundle(bundle, direction)
	if err != nil {
		return modelresearch.ProviderResult{}, false, err
	}
	proposal := studymap.ReviewProposal{
		Version: studymap.ReviewProposalVersion, DirectionID: reviewBundle.DirectionID,
	}
	roles := []studymap.ReadingRole{
		studymap.ReadingRolePublicOrCLIEntry,
		studymap.ReadingRoleCoreOrchestration,
		studymap.ReadingRoleEffectOrIntegrationBoundary,
	}
	for index, anchor := range reviewBundle.Anchors {
		proposal.Reviews = append(proposal.Reviews, studymap.AnchorReview{
			AnchorID: anchor.AnchorID, Fit: studymap.AnchorFitDirect,
			SupportedObservation: "This fragment defines the selected function.",
			Role:                 roles[index%len(roles)],
			OverclaimReasons:     []studymap.OverclaimReason{studymap.OverclaimNone},
		})
	}
	raw, err := json.Marshal(proposal)
	return modelresearch.ProviderResult{Content: raw, Attempts: 9}, true, err
}

func (studyReviewJournalCacheProvider) storeStudyReview(
	context.Context,
	semanticdiscovery.Prompt,
	[]byte,
	studymap.Bundle,
	studymap.DirectionCandidate,
	[]byte,
) {
}

func TestStudyReviewSemanticJournalRecordsValidatedCacheRawWithoutLiveCall(t *testing.T) {
	bundle, directions := studyMapV32ReviewFixture(t)
	directions.Directions = directions.Directions[:2]
	runDir := t.TempDir()
	writer := openSemanticJournalTestWriter(t, runDir)
	reviews, _, stages, issues, err := reviewStudyMapDirectionsWithOptions(
		context.Background(), runDir, bundle, directions, "bundle-sha",
		studyReviewJournalCacheProvider{}, studyMapRunOptions{exchangeWriter: writer},
	)
	if err != nil {
		t.Fatal(err)
	}
	entries := readSemanticJournalEntries(t, runDir)
	if len(reviews) != 2 || len(stages) != 2 || len(issues) != 0 || len(entries) != 2 {
		t.Fatalf("cached Study reviews/stages/issues/journal = %d/%d/%d/%d", len(reviews), len(stages), len(issues), len(entries))
	}
	for index, entry := range entries {
		if entry.record.Stage != debugdump.SemanticStageStudyReview ||
			entry.record.InstanceOrdinal != index+1 ||
			entry.record.State != debugdump.SemanticStateCacheHit ||
			entry.record.ValidationCode != debugdump.SemanticValidationCache ||
			entry.record.SemanticCalls != 0 || entry.record.TransportAttempts != 0 ||
			entry.record.Response.Storage != "raw_content" || len(entry.response) == 0 {
			t.Fatalf("cached Study semantic entry = %#v", entry.record)
		}
	}
}
