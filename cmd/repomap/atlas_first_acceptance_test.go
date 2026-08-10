package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/entrycall"
	"github.com/dvordrova/repomap/internal/mechanismstudy"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/studymap"
	"github.com/dvordrova/repomap/internal/targetportfolio"
	"github.com/dvordrova/repomap/internal/themestudy"
)

type atlasFirstAcceptanceStage string

const (
	atlasFirstStageTargetPortfolio   atlasFirstAcceptanceStage = "target_portfolio"
	atlasFirstStageArchitecture      atlasFirstAcceptanceStage = "architecture"
	atlasFirstStageStudyScout        atlasFirstAcceptanceStage = "theme_scout"
	atlasFirstStageStudyAdjudication atlasFirstAcceptanceStage = "theme_adjudication"
	atlasFirstStageStudyMechanism    atlasFirstAcceptanceStage = "mechanism_study"
	atlasFirstStageEntryCall         atlasFirstAcceptanceStage = "entry_call_compression"
)

type atlasFirstAcceptanceProvider struct {
	t                      *testing.T
	repositoryType         atlasstudy.RepositoryType
	rejectArchitecture     bool
	failArchitectureCall   bool
	lengthArchitectureCall bool
	includeBadStudySibling bool
	rejectAllAdjudication  bool

	mu     sync.Mutex
	stages []atlasFirstAcceptanceStage
	bodies map[atlasFirstAcceptanceStage][][]byte
}

func TestRunDefaultAtlasFirstPublishesArchitectureAndStudy(t *testing.T) {
	repo := atlasFirstAcceptanceRepository(t, "testdata/atlas_first_service")
	provider := &atlasFirstAcceptanceProvider{
		t: t, repositoryType: atlasstudy.RepositoryService,
		includeBadStudySibling: true,
	}
	runDir, manifest, data := runAtlasFirstAcceptance(t, repo, provider)

	provider.assertStages(t,
		atlasFirstStageArchitecture,
		atlasFirstStageStudyScout,
		atlasFirstStageStudyAdjudication,
		atlasFirstStageEntryCall,
	)
	provider.assertStageCalls(t, atlasFirstStageTargetPortfolio, 0)
	if data.RepositoryGraph == nil || len(data.RepositoryGraph.PackageEdges) != 1 {
		t.Fatalf(
			"top-level Atlas-first exact package edges = %#v, want one preserved edge",
			data.RepositoryGraph,
		)
	}
	assertAtlasFirstAcceptedArchitecture(t, data)
	assertAtlasFirstAcceptedStudy(t, data)
	assertAtlasFirstLocalSubstrateUnchanged(t, data)
	assertAtlasFirstDiagnostics(t, runDir, map[string]string{
		debugdump.SemanticStageArchitecture:         "accepted_partial",
		debugdump.SemanticStageAtlasStudy:           "accepted_partial",
		debugdump.SemanticStageMechanismStudy:       "accepted",
		debugdump.SemanticStageEntryCallCompression: "accepted",
	})
	assertAtlasFirstSemanticStages(t, runDir,
		debugdump.SemanticStageArchitecture,
		debugdump.SemanticStageAtlasStudy,
		debugdump.SemanticStageEntryCallCompression,
	)
	assertAtlasFirstAcceptedArtifacts(t, runDir)
	assertAtlasFirstAcceptedManifest(t, manifest)
	assertNoLegacyAtlasFirstArtifacts(t, runDir)

	scoutBytes, err := os.ReadFile(filepath.Join(runDir, themestudy.ScoutResultArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	scout, err := themestudy.DecodeScoutResult(scoutBytes)
	if err != nil {
		t.Fatal(err)
	}
	if scout.Version != themestudy.ScoutResultVersion || scout.State != "accepted_partial" ||
		scout.Status.Received != 2 || scout.Status.Accepted != 1 || scout.Status.Rejected != 1 ||
		len(scout.Status.Issues) != 1 || scout.Status.Issues[0].Code != themestudy.ScoutIssueUnknownRef {
		t.Fatalf("invalid sibling did not preserve the valid theme candidate: %#v", scout.Status)
	}
	themesBytes, err := os.ReadFile(filepath.Join(runDir, themestudy.StudyThemesArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	themes, err := themestudy.DecodeStudyThemes(themesBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(themes.Cards) == 0 {
		t.Fatalf("theme shelf = %#v, want at least one published card", themes)
	}
	// The Scout rejection is recorded in the Scout result (accepted_partial);
	// the published shelf is the surviving candidate. The report projection
	// carries the final partial state.
	if data.AtlasStudy.State != atlasstudy.ProductStateAcceptedPartial {
		t.Fatalf("study state = %q, want accepted_partial after a rejected Scout sibling", data.AtlasStudy.State)
	}
}

func TestRunDefaultAtlasFirstLibraryPublishesArchitectureAndStudy(t *testing.T) {
	repo := atlasFirstAcceptanceRepository(t, "testdata/atlas_first_library")
	provider := &atlasFirstAcceptanceProvider{
		t: t, repositoryType: atlasstudy.RepositoryLibrary,
	}
	runDir, manifest, data := runAtlasFirstAcceptance(t, repo, provider)

	provider.assertStages(t, atlasFirstStageArchitecture, atlasFirstStageStudyScout, atlasFirstStageStudyAdjudication)
	if data.RepositoryGraph == nil || len(data.RepositoryGraph.Packages) != 1 {
		t.Fatalf("library package graph = %#v, want exactly one package", data.RepositoryGraph)
	}
	if data.RepositoryAtlas == nil || len(data.RepositoryAtlas.Relations) != 0 {
		t.Fatalf("library Atlas relations = %#v, want no synthetic startup relation", data.RepositoryAtlas)
	}
	assertAtlasFirstAcceptedArchitecture(t, data)
	assertAtlasFirstAcceptedStudy(t, data)
	assertAtlasFirstLocalSubstrateUnchanged(t, data)
	assertAtlasFirstDiagnostics(t, runDir, map[string]string{
		debugdump.SemanticStageArchitecture:         "accepted_partial",
		debugdump.SemanticStageAtlasStudy:           "accepted",
		debugdump.SemanticStageMechanismStudy:       "accepted",
		debugdump.SemanticStageEntryCallCompression: "unavailable",
	})
	assertAtlasFirstSemanticStages(t, runDir,
		debugdump.SemanticStageArchitecture,
		debugdump.SemanticStageAtlasStudy,
	)
	assertAtlasFirstAcceptedArtifacts(t, runDir)
	assertAtlasFirstAcceptedManifest(t, manifest)
	assertNoLegacyAtlasFirstArtifacts(t, runDir)
}

func TestRunDefaultAtlasFirstRejectedArchitectureKeepsLocalCanvasAndCallsStudy(t *testing.T) {
	repo := atlasFirstAcceptanceRepository(t, "testdata/atlas_first_library")
	provider := &atlasFirstAcceptanceProvider{
		t: t, repositoryType: atlasstudy.RepositoryService,
		rejectArchitecture: true,
	}
	runDir, manifest, data := runAtlasFirstAcceptance(t, repo, provider)

	provider.assertStages(t, atlasFirstStageArchitecture, atlasFirstStageStudyScout, atlasFirstStageStudyAdjudication)
	if data.ArchitectureSynthesis == nil ||
		data.ArchitectureSynthesis.State != report.ArchitectureSynthesisFailed ||
		!data.ArchitectureSynthesis.ProposalRejected ||
		data.ArchitectureSynthesis.ProposalAccepted {
		t.Fatalf("rejected Architecture status = %#v", data.ArchitectureSynthesis)
	}
	if data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		(data.ArchitectureCanvas.ArchitectureSource != componentmap.SourceLocalAnchors &&
			data.ArchitectureCanvas.ArchitectureSource != componentmap.SourceLocalPackages) ||
		len(data.ArchitectureCanvas.Components) == 0 {
		t.Fatalf("rejected enrichment erased canonical local canvas: %#v", data.ArchitectureCanvas)
	}
	assertAtlasFirstAcceptedStudy(t, data)
	assertAtlasFirstDiagnostics(t, runDir, map[string]string{
		debugdump.SemanticStageArchitecture:         "rejected",
		debugdump.SemanticStageAtlasStudy:           "accepted",
		debugdump.SemanticStageMechanismStudy:       "accepted",
		debugdump.SemanticStageEntryCallCompression: "unavailable",
	})
	assertAtlasFirstSemanticStages(t, runDir,
		debugdump.SemanticStageArchitecture,
		debugdump.SemanticStageAtlasStudy,
	)
	if _, err := os.Stat(filepath.Join(runDir, report.ArchitectureSynthesisFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected Architecture persisted enrichment: %v", err)
	}
	for _, name := range []string{
		report.ArchitectureSynthesisStatusFile,
		themestudy.ScoutRequestArtifactFilename,
		themestudy.ScoutResultArtifactFilename,
		themestudy.ScoutStatusArtifactFilename,
		themestudy.ExpansionArtifactFilename,
		themestudy.AdjudicationRequestArtifactFilename,
		themestudy.AdjudicationResultArtifactFilename,
		themestudy.AdjudicationStatusArtifactFilename,
		themestudy.StudyThemesArtifactFilename,
		"report.json",
		"report.html",
		report.RunManifestFilename,
	} {
		if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
			t.Fatalf("rejected enrichment run missing %s: %v", name, err)
		}
	}
	assertAtlasFirstAcceptedManifest(t, manifest)
	assertNoLegacyAtlasFirstArtifacts(t, runDir)
}

func TestRunDefaultAtlasFirstArchitectureProviderFailureKeepsLocalCanvas(t *testing.T) {
	repo := atlasFirstAcceptanceRepository(t, "testdata/atlas_first_service")
	provider := &atlasFirstAcceptanceProvider{
		t: t, repositoryType: atlasstudy.RepositoryService,
		failArchitectureCall: true,
	}
	runDir, _, data := runAtlasFirstAcceptance(t, repo, provider)

	provider.assertStages(t,
		atlasFirstStageArchitecture,
		atlasFirstStageStudyScout,
		atlasFirstStageStudyAdjudication,
		atlasFirstStageEntryCall,
	)
	provider.assertStageCalls(t, atlasFirstStageTargetPortfolio, 0)
	if data.ArchitectureSynthesis == nil ||
		data.ArchitectureSynthesis.State != report.ArchitectureSynthesisFailed ||
		data.ArchitectureSynthesis.ProposalAccepted ||
		data.ArchitectureSynthesis.ProposalRejected {
		t.Fatalf("provider-failed Architecture status = %#v", data.ArchitectureSynthesis)
	}
	if data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		(data.ArchitectureCanvas.ArchitectureSource != componentmap.SourceLocalAnchors &&
			data.ArchitectureCanvas.ArchitectureSource != componentmap.SourceLocalPackages) ||
		len(data.ArchitectureCanvas.Components) == 0 {
		t.Fatalf("provider failure erased canonical local canvas: %#v", data.ArchitectureCanvas)
	}
	assertAtlasFirstAcceptedStudy(t, data)
	assertAtlasFirstDiagnostics(t, runDir, map[string]string{
		debugdump.SemanticStageArchitecture:         "failed",
		debugdump.SemanticStageAtlasStudy:           "accepted",
		debugdump.SemanticStageMechanismStudy:       "accepted",
		debugdump.SemanticStageEntryCallCompression: "accepted",
	})
}

// Syn corpus regression: the model used f* file refs as Adjudication
// readings even though only the candidate's a* anchor refs are eligible.
// Item-local validation rejects every theme. The accepted Scout prefix must
// close with a durable failed Adjudication status so the ordinary run can
// still publish a manifest-bound report and classify it as DEGRADED.
func TestRunDefaultAtlasFirstRejectedAdjudicationPublishesDegradedReport(t *testing.T) {
	repo := atlasFirstAcceptanceRepository(t, "testdata/atlas_first_service")
	provider := &atlasFirstAcceptanceProvider{
		t: t, repositoryType: atlasstudy.RepositoryService,
		rejectAllAdjudication: true,
	}
	runDir, manifest, data := runAtlasFirstAcceptance(t, repo, provider)

	provider.assertStages(t,
		atlasFirstStageArchitecture,
		atlasFirstStageStudyScout,
		atlasFirstStageStudyAdjudication,
		atlasFirstStageEntryCall,
	)
	provider.assertStageCalls(t, atlasFirstStageTargetPortfolio, 0)
	assertAtlasFirstAcceptedArchitecture(t, data)
	if data.AtlasStudy == nil || data.AtlasStudy.State != atlasstudy.ProductStateFailed ||
		data.AtlasStudy.FailureCode != atlasstudy.FailureValidation || data.AtlasStudy.Themes != nil {
		t.Fatalf("rejected Adjudication Study = %#v", data.AtlasStudy)
	}
	publication, err := assessRunPublication(runDir)
	if err != nil {
		t.Fatalf("assess degraded publication: %v", err)
	}
	if publication.Status != publicationDegraded ||
		!reflect.DeepEqual(publication.Reasons, []publicationReason{publicationReasonStudyFailed}) {
		t.Fatalf("rejected Adjudication publication = %#v", publication)
	}
	if manifest.ReportSHA256 == "" {
		t.Fatal("degraded publication is not report-bound")
	}
	for _, name := range []string{
		"report.json", "report.html", report.RunManifestFilename,
		themestudy.ScoutRequestArtifactFilename,
		themestudy.ScoutResultArtifactFilename,
		themestudy.ScoutStatusArtifactFilename,
		themestudy.ExpansionArtifactFilename,
		themestudy.AdjudicationRequestArtifactFilename,
		themestudy.AdjudicationStatusArtifactFilename,
	} {
		if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
			t.Fatalf("degraded run missing %s: %v", name, err)
		}
	}
	for _, name := range []string{
		themestudy.AdjudicationResultArtifactFilename,
		themestudy.StudyThemesArtifactFilename,
	} {
		if _, err := os.Lstat(filepath.Join(runDir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("rejected Adjudication published %s: %v", name, err)
		}
	}
	statusBytes, err := os.ReadFile(filepath.Join(runDir, themestudy.AdjudicationStatusArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	status, err := themestudy.DecodeAdjudicationStatus(statusBytes)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != string(atlasstudy.ProductStateFailed) ||
		status.FailureCode != string(atlasstudy.FailureValidation) ||
		status.Status.Received == 0 || status.Status.Accepted != 0 ||
		status.Status.Rejected != status.Status.Received || len(status.Status.Issues) == 0 ||
		status.Status.Issues[0].Code != themestudy.AdjIssueAnchorOutsideCandidate {
		t.Fatalf("failed Adjudication status = %#v", status)
	}
	assertAtlasFirstDiagnostics(t, runDir, map[string]string{
		debugdump.SemanticStageArchitecture:         "accepted_partial",
		debugdump.SemanticStageAtlasStudy:           "response_validation_failed",
		debugdump.SemanticStageMechanismStudy:       "not_called",
		debugdump.SemanticStageEntryCallCompression: "accepted",
	})
	entries := readSemanticJournalEntries(t, runDir)
	stageCounts := make(map[string]int)
	var adjudication *debugdump.SemanticExchangeRecord
	for index := range entries {
		record := &entries[index].record
		stageCounts[record.Stage]++
		if record.Stage == debugdump.SemanticStageAtlasStudy && record.InstanceOrdinal == 2 {
			adjudication = record
		}
	}
	if !reflect.DeepEqual(stageCounts, map[string]int{
		debugdump.SemanticStageArchitecture:         1,
		debugdump.SemanticStageAtlasStudy:           2,
		debugdump.SemanticStageEntryCallCompression: 1,
	}) || adjudication == nil ||
		adjudication.State != debugdump.SemanticStateRejected ||
		adjudication.ValidationCode != debugdump.SemanticValidationResponse {
		t.Fatalf("rejected Adjudication journal = %#v", entries)
	}
}

// TestRunDefaultAtlasFirstArchitectureOutputExhaustionPublishesLocalProduct
// is the Decision 215 continuation proof: an attempted Architecture provider
// output exhaustion (finish_reason=length with a partial repeated-ref
// response) records a durable failed status and accounting, keeps the partial
// response diagnostic-only, and the run continues through both D213 semantic
// stages and the published report bound to the canonical local Canvas.
func TestRunDefaultAtlasFirstArchitectureOutputExhaustionPublishesLocalProduct(t *testing.T) {
	repo := atlasFirstAcceptanceRepository(t, "testdata/atlas_first_service")
	provider := &atlasFirstAcceptanceProvider{
		t: t, repositoryType: atlasstudy.RepositoryService,
		lengthArchitectureCall: true,
	}
	runDir, _, data := runAtlasFirstAcceptance(t, repo, provider)

	provider.assertStages(t,
		atlasFirstStageArchitecture,
		atlasFirstStageStudyScout,
		atlasFirstStageStudyAdjudication,
		atlasFirstStageEntryCall,
	)
	provider.assertStageCalls(t, atlasFirstStageTargetPortfolio, 0)
	if data.ArchitectureSynthesis == nil ||
		data.ArchitectureSynthesis.State != report.ArchitectureSynthesisFailed ||
		data.ArchitectureSynthesis.ErrorCode != report.ArchitectureSynthesisErrorProviderOutputLimit ||
		data.ArchitectureSynthesis.FinishReason != "length" ||
		data.ArchitectureSynthesis.ResponseComplete ||
		data.ArchitectureSynthesis.ConfiguredMaxTokens <= 0 ||
		data.ArchitectureSynthesis.ObservedOutputTokens <= 0 {
		t.Fatalf("output-exhausted Architecture status = %#v", data.ArchitectureSynthesis)
	}
	if data.ArchitectureSynthesis.ProposalAccepted || data.ArchitectureSynthesis.ProposalRejected ||
		data.ArchitectureSynthesis.ProposalPartial || data.ArchitectureSynthesis.MembershipCounted ||
		data.ArchitectureSynthesis.ArchitectureSource != "" ||
		len(data.ArchitectureSynthesis.ValidationCodes) != 0 {
		t.Fatalf("output-exhausted Architecture status published partial response evidence: %#v", data.ArchitectureSynthesis)
	}
	if data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		len(data.ArchitectureCanvas.Components) == 0 {
		t.Fatalf("output exhaustion erased canonical local canvas: %#v", data.ArchitectureCanvas)
	}
	assertAtlasFirstAcceptedStudy(t, data)
	assertAtlasFirstDiagnostics(t, runDir, map[string]string{
		debugdump.SemanticStageArchitecture:         "resource_exhausted",
		debugdump.SemanticStageAtlasStudy:           "accepted",
		debugdump.SemanticStageMechanismStudy:       "accepted",
		debugdump.SemanticStageEntryCallCompression: "accepted",
	})
	if _, err := os.Lstat(filepath.Join(runDir, report.ArchitectureSynthesisFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output exhaustion published a synthesis record: %v", err)
	}
	// The manifest binds the failed Architecture status alongside the local
	// Canvas.
	statusData, err := os.ReadFile(filepath.Join(runDir, report.ArchitectureSynthesisStatusFile))
	if err != nil {
		t.Fatalf("failed Architecture status missing: %v", err)
	}
	var status report.ArchitectureSynthesisStatus
	if err := json.Unmarshal(statusData, &status); err != nil {
		t.Fatal(err)
	}
	if status.ErrorCode != report.ArchitectureSynthesisErrorProviderOutputLimit {
		t.Fatalf("manifest-bound failed status = %#v", status)
	}
}

// TestEtcdArchitectureOutputExhaustionReplayPublishesCompleteReport is the
// Decision 215 etcd acceptance replay: the real CLI runs against the local
// etcd repository with a deterministic provider that serves the exact
// etcd-shaped Architecture output exhaustion (one subsystem, one
// component, an open member_refs repeating a bounded package-ref block,
// finish_reason=length), and valid D213 Scout + Adjudication fixtures. The
// run must complete: failed Architecture status and accounting durable, no
// synthesis record, Study accepted, report + manifest published, and the
// canonical local Canvas bound.
func TestEtcdArchitectureOutputExhaustionReplayPublishesCompleteReport(t *testing.T) {
	repo := "/Users/dvordrova/git/etcd"
	if info, err := os.Stat(repo); err != nil || !info.IsDir() {
		t.Skipf("local etcd repository unavailable: %v", err)
	}
	provider := &atlasFirstAcceptanceProvider{
		t: t, repositoryType: atlasstudy.RepositoryLibrary,
		lengthArchitectureCall: true,
	}
	runDir, manifest, data := runAtlasFirstAcceptance(t, repo, provider)

	assertAtlasFirstMechanismExecutionMatchesPlan(t, runDir, provider)
	if data.ArchitectureSynthesis == nil ||
		data.ArchitectureSynthesis.State != report.ArchitectureSynthesisFailed ||
		data.ArchitectureSynthesis.ErrorCode != report.ArchitectureSynthesisErrorProviderOutputLimit ||
		data.ArchitectureSynthesis.FinishReason != "length" ||
		data.ArchitectureSynthesis.ResponseComplete ||
		data.ArchitectureSynthesis.ConfiguredMaxTokens != 64_000 ||
		data.ArchitectureSynthesis.ObservedOutputTokens != 64_000 {
		t.Fatalf("etcd output-exhausted Architecture status = %#v", data.ArchitectureSynthesis)
	}
	if data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		len(data.ArchitectureCanvas.Components) == 0 {
		t.Fatalf("etcd output exhaustion erased canonical local canvas: %#v", data.ArchitectureCanvas)
	}
	// Study must have executed (accepted or an honest zero-candidate failure);
	// it must not be absent from a run that continued past Architecture.
	if data.AtlasStudy == nil && data.StudyPublication == nil {
		t.Fatalf("etcd continuation did not execute Study")
	}
	if _, err := os.Lstat(filepath.Join(runDir, report.ArchitectureSynthesisFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("etcd output exhaustion published a synthesis record: %v", err)
	}
	// The run manifest binds the published report, which carries the failed
	// Architecture status alongside the local Canvas.
	if manifest.ReportSHA256 == "" {
		t.Fatalf("etcd manifest missing report binding")
	}
	// The published report.json binds the failed status and the local Canvas.
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatalf("read etcd report.json: %v", err)
	}
	if !bytes.Contains(reportJSON, []byte(`"error_code": "provider_output_limit"`)) &&
		!bytes.Contains(reportJSON, []byte(`"error_code":"provider_output_limit"`)) {
		t.Logf("architecture_synthesis in report.json: %s", string(bytesBetween(reportJSON, []byte(`"architecture_synthesis":`))))
		t.Fatalf("etcd report.json missing provider_output_limit binding")
	}
}

// assertAtlasFirstMechanismExecutionMatchesPlan keeps the real-repository
// replay bound to the persisted D260 plan instead of assuming that every
// accepted Study shelf has a provider-eligible target-rooted trail. A zero
// batch plan is a complete, provider-free result when every card remains an
// honest prepared investigation with a closed local frontier.
func assertAtlasFirstMechanismExecutionMatchesPlan(
	t *testing.T,
	runDir string,
	provider *atlasFirstAcceptanceProvider,
) {
	t.Helper()
	read := func(name string) []byte {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(runDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return raw
	}
	factsRaw := read(mechanismstudy.FactsArtifactFilename)
	candidatesRaw := read(mechanismstudy.CandidatesArtifactFilename)
	resultRaw := read(mechanismstudy.ResultArtifactFilename)
	statusRaw := read(mechanismstudy.StatusArtifactFilename)
	facts, err := mechanismstudy.DecodeFacts(factsRaw)
	if err != nil {
		t.Fatalf("decode mechanism facts: %v", err)
	}
	status, err := mechanismstudy.DecodeStatus(factsRaw, candidatesRaw, resultRaw, statusRaw)
	if err != nil {
		t.Fatalf("decode mechanism status: %v", err)
	}
	planned := len(facts.Plan.Batches)
	provider.assertStagesWithMechanismBatches(t, true, planned, planned)
	if status.PlannedBatchCount != planned || status.ProviderCallCount != planned {
		t.Fatalf(
			"mechanism execution does not close persisted plan: planned=%d status=%#v",
			planned,
			status,
		)
	}
	if planned != 0 {
		return
	}
	if status.State != mechanismstudy.StatusComplete || status.PreparedCardCount == 0 ||
		status.MechanismCardCount != 0 {
		t.Fatalf("zero-call mechanism plan is not an honest complete prepared result: %#v", status)
	}
	for _, card := range facts.Compilation.Cards {
		if len(card.Frontier) == 0 {
			t.Fatalf("zero-call mechanism card %s has no closed local frontier", card.Ref)
		}
	}
}

func bytesBetween(data, marker []byte) []byte {
	index := bytes.Index(data, marker)
	if index < 0 {
		return []byte("MARKER ABSENT")
	}
	end := index + len(marker) + 400
	if end > len(data) {
		end = len(data)
	}
	return data[index:end]
}

func runAtlasFirstAcceptance(
	t *testing.T,
	repo string,
	provider *atlasFirstAcceptanceProvider,
) (string, report.RunManifest, *report.ReportData) {
	t.Helper()
	clearLLMEnv(t)
	runsDir := t.TempDir()
	server := httptest.NewServer(provider)
	defer server.Close()
	configureAtlasFirstAcceptanceProvider(t, server.URL)

	var stderr bytes.Buffer
	err := runDefaultWithDeps(
		repo,
		[]string{
			"--debug-dir", runsDir,
			"--lang", "en",
			"--no-cache",
			"--no-open",
			"--no-serve",
		},
		defaultRunDeps{ctx: context.Background(), stdout: io.Discard, stderr: &stderr},
	)
	if err != nil {
		t.Fatalf("runDefaultWithDeps() error = %v\nstderr:\n%s", err, stderr.String())
	}
	runDir := atlasFirstAcceptanceRunDir(t, runsDir)
	manifest, data := readAtlasFirstAcceptanceRun(t, runDir)
	publication, publicationErr := assessRunPublication(runDir)
	if publicationErr != nil {
		t.Fatalf("assess generated publication: %v", publicationErr)
	}
	wantRunState := "Run:\n  state: " + publication.consoleState()
	if !strings.Contains(stderr.String(), wantRunState) {
		t.Fatalf("final console is not bound to verified publication state %q:\n%s", wantRunState, stderr.String())
	}
	assertNavigatorRetiredFromRun(t, runDir)
	return runDir, manifest, data
}

func assertNavigatorRetiredFromRun(t *testing.T, runDir string) {
	t.Helper()
	for _, name := range []string{
		"navigator_request.v1.json",
		"navigator_result.v1.json",
		"navigator_status.v1.json",
	} {
		if _, err := os.Lstat(filepath.Join(runDir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("retired Navigator artifact %s is present: %v", name, err)
		}
	}
	for _, entry := range readSemanticJournalEntries(t, runDir) {
		if entry.record.Stage == "navigator" {
			t.Fatalf("retired Navigator semantic exchange is present: %#v", entry.record)
		}
	}
	manifestJSON, err := os.ReadFile(filepath.Join(runDir, report.RunManifestFilename))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(manifestJSON, []byte(`"navigator_`)) {
		t.Fatalf("retired Navigator material binding is present: %s", manifestJSON)
	}
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(reportJSON, []byte(`"navigator":`)) {
		t.Fatalf("retired Navigator report product is present")
	}
}

func configureAtlasFirstAcceptanceProvider(t *testing.T, endpoint string) {
	t.Helper()
	t.Setenv("REPOMAP_LLM_ENDPOINT", endpoint)
	t.Setenv("REPOMAP_LLM_MODEL", "fixture-atlas-first-model")
	t.Setenv("REPOMAP_LLM_AUTH", "none")
	t.Setenv("REPOMAP_LLM_MAX_TOKENS", "64000")
	t.Setenv("REPOMAP_LLM_TIMEOUT", "5s")
}

func atlasFirstAcceptanceRunDir(t *testing.T, runsDir string) string {
	t.Helper()
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		t.Fatal(err)
	}
	var runDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(runsDir, entry.Name())
		if _, err := os.Stat(filepath.Join(candidate, "metadata.json")); err == nil {
			runDirs = append(runDirs, candidate)
		}
	}
	if len(runDirs) != 1 {
		t.Fatalf("run directories = %v, want exactly one", runDirs)
	}
	return runDirs[0]
}

func readAtlasFirstAcceptanceRun(
	t *testing.T,
	runDir string,
) (report.RunManifest, *report.ReportData) {
	t.Helper()
	manifest, err := report.ReadRunManifest(runDir)
	if err != nil {
		t.Fatalf("ReadRunManifest(%s): %v", runDir, err)
	}
	reportJSON, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data report.ReportData
	if err := json.Unmarshal(reportJSON, &data); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != report.CurrentRunManifestVersion ||
		manifest.ReportFormatVersion != report.CurrentFormatVersion ||
		data.FormatVersion != report.CurrentFormatVersion {
		t.Fatalf(
			"Atlas-first versions manifest/report = %d/%d/%d",
			manifest.Version, manifest.ReportFormatVersion, data.FormatVersion,
		)
	}
	return manifest, &data
}

func (provider *atlasFirstAcceptanceProvider) ServeHTTP(
	writer http.ResponseWriter,
	request *http.Request,
) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		provider.t.Errorf("read provider request: %v", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	if request.Method != http.MethodPost || request.Header.Get("Authorization") != "" ||
		!strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
		provider.t.Errorf("invalid local provider request method/header")
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	stage, combined, err := atlasFirstAcceptanceRequestStage(body)
	if err != nil {
		provider.t.Errorf("unrecognized semantic request: %v\n%s", err, body)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	provider.mu.Lock()
	provider.stages = append(provider.stages, stage)
	if provider.bodies == nil {
		provider.bodies = make(map[atlasFirstAcceptanceStage][][]byte)
	}
	provider.bodies[stage] = append(provider.bodies[stage], append([]byte(nil), body...))
	provider.mu.Unlock()
	if stage == atlasFirstStageArchitecture && provider.failArchitectureCall {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"error":{"message":"fixture provider call failed"}}`))
		return
	}
	if stage == atlasFirstStageArchitecture && provider.lengthArchitectureCall {
		// Decision 215: the provider exhausts its output budget with a partial
		// response. Like the etcd incident, it emits one subsystem, one
		// component, an open member_refs array, and then repeats the same
		// bounded package-ref block many times with no closing JSON. The block
		// is generated deterministically, not stored as a large fixture.
		writer.Header().Set("Content-Type", "application/json")
		var partial strings.Builder
		partial.WriteString(`{"records":[{"kind":"subsystem","ref":"g1","name":"Fixture core","description":"Fixture grouping"},{"kind":"component","ref":"c1","subsystem_ref":"g1","name":"Fixture component","description":"Fixture component grouping","member_refs":[`)
		block := []string{
			`{"kind":"package","ref":"p1"}`,
			`{"kind":"package","ref":"p2"}`,
			`{"kind":"package","ref":"p3"}`,
			`{"kind":"package","ref":"p4"}`,
			`{"kind":"package","ref":"p5"}`,
			`{"kind":"package","ref":"p6"}`,
			`{"kind":"package","ref":"p7"}`,
			`{"kind":"package","ref":"p8"}`,
		}
		for index := 0; index < 128; index++ {
			if index > 0 {
				partial.WriteByte(',')
			}
			partial.WriteString(block[index%len(block)])
		}
		if err := json.NewEncoder(writer).Encode(map[string]any{
			"choices": []any{map[string]any{
				"finish_reason": "length",
				"message": map[string]any{
					"role": "assistant", "content": partial.String(),
				},
			}},
			"usage": map[string]any{
				"prompt_tokens": 42197, "completion_tokens": 64000,
			},
		}); err != nil {
			provider.t.Errorf("encode length-ended architecture response: %v", err)
		}
		return
	}

	response, err := provider.responseForStage(stage, combined)
	if err != nil {
		provider.t.Errorf("build %s response: %v", stage, err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write(response)
}

func (provider *atlasFirstAcceptanceProvider) responseForStage(
	stage atlasFirstAcceptanceStage,
	combined string,
) ([]byte, error) {
	switch stage {
	case atlasFirstStageTargetPortfolio:
		return atlasFirstAcceptanceTargetPortfolioResponse(combined)
	case atlasFirstStageArchitecture:
		return atlasFirstAcceptanceArchitectureResponse(combined, provider.rejectArchitecture)
	case atlasFirstStageStudyScout:
		return atlasFirstAcceptanceScoutResponse(combined, provider.includeBadStudySibling)
	case atlasFirstStageStudyAdjudication:
		return atlasFirstAcceptanceAdjudicationResponse(combined, provider.rejectAllAdjudication)
	case atlasFirstStageStudyMechanism:
		return atlasFirstAcceptanceMechanismResponse(combined)
	case atlasFirstStageEntryCall:
		return atlasFirstAcceptanceEntryCallResponse(combined)
	default:
		return nil, fmt.Errorf("unsupported stage %q", stage)
	}
}

func atlasFirstAcceptanceRequestStage(
	body []byte,
) (atlasFirstAcceptanceStage, string, error) {
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return "", "", err
	}
	if len(request.Messages) != 2 {
		return "", "", fmt.Errorf("message count = %d", len(request.Messages))
	}
	combined := request.Messages[0].Content + "\n" + request.Messages[1].Content
	switch {
	case strings.Contains(combined, "You select the useful product analysis targets") &&
		strings.Contains(combined, "Exact bounded target catalog JSON:\n"):
		return atlasFirstStageTargetPortfolio, combined, nil
	case strings.Contains(combined, "Use conceptual member, anchor, and unit refs as opaque request-local values.") &&
		strings.Contains(combined, "Bounded candidate request:\n"):
		return atlasFirstStageArchitecture, combined, nil
	case strings.Contains(combined, "theme_kind is one of: user_journey") &&
		strings.Contains(combined, "Request bundle JSON:\n"):
		return atlasFirstStageStudyScout, combined, nil
	case strings.Contains(combined, "Review proposed Study themes against their exact source evidence") &&
		strings.Contains(combined, "Request bundle JSON:\n"):
		return atlasFirstStageStudyAdjudication, combined, nil
	case strings.Contains(combined, "identifying an unordered set of zero or more useful mechanisms") &&
		strings.Contains(combined, "Exact request bundle JSON:\n"):
		return atlasFirstStageStudyMechanism, combined, nil
	case strings.Contains(combined, "You identify the smallest useful direct-call family set") &&
		strings.Contains(combined, "Exact bounded request JSON:\n"):
		return atlasFirstStageEntryCall, combined, nil
	default:
		return "", combined, fmt.Errorf("request is not a current Atlas-first stage")
	}
}

func atlasFirstAcceptanceTargetPortfolioResponse(combined string) ([]byte, error) {
	const marker = "Exact bounded target catalog JSON:\n"
	position := strings.LastIndex(combined, marker)
	if position < 0 {
		return nil, fmt.Errorf("target portfolio request marker is absent")
	}
	var request targetportfolio.Request
	if err := json.Unmarshal([]byte(combined[position+len(marker):]), &request); err != nil {
		return nil, fmt.Errorf("decode target portfolio request: %w", err)
	}
	if len(request.Targets) == 0 {
		return nil, fmt.Errorf("target portfolio request has no targets")
	}
	selected := request.Targets[0]
	for _, target := range request.Targets {
		if target.Kind == targetportfolio.TargetExecutable &&
			(target.DisplayPath == "." || target.DisplayPath == "server") {
			selected = target
			break
		}
	}
	content, err := json.Marshal(targetportfolio.Response{
		Version: targetportfolio.ResultVersion, RequestRef: request.RequestRef,
		DefaultRef: selected.Ref, TargetRefs: []string{selected.Ref},
	})
	if err != nil {
		return nil, err
	}
	return atlasFirstAcceptanceCompletion(content, 97, 13), nil
}

func atlasFirstAcceptanceEntryCallResponse(combined string) ([]byte, error) {
	const marker = "Exact bounded request JSON:\n"
	position := strings.LastIndex(combined, marker)
	if position < 0 {
		return nil, fmt.Errorf("entry-call request marker is absent")
	}
	var request entrycall.Request
	if err := json.Unmarshal([]byte(combined[position+len(marker):]), &request); err != nil {
		return nil, fmt.Errorf("decode entry-call request: %w", err)
	}
	response := entrycall.Response{
		Version: entrycall.ResultVersion, RequestRef: request.RequestRef,
		Entries:          make([]entrycall.ResponseEntry, 0, len(request.Entries)),
		SurfaceProposals: []entrycall.ResponseSurfaceProposal{},
	}
	for _, entry := range request.Entries {
		response.Entries = append(response.Entries, entrycall.ResponseEntry{
			RootRef: entry.Ref, FamilyRefs: []string{},
		})
	}
	content, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	return atlasFirstAcceptanceCompletion(content, 113, 17), nil
}

func atlasFirstAcceptanceMechanismResponse(combined string) ([]byte, error) {
	const marker = "Exact request bundle JSON:\n"
	position := strings.LastIndex(combined, marker)
	if position < 0 {
		return nil, fmt.Errorf("mechanism request bundle marker is absent")
	}
	var request mechanismstudy.Request
	if err := json.Unmarshal([]byte(combined[position+len(marker):]), &request); err != nil {
		return nil, fmt.Errorf("decode mechanism request: %w", err)
	}
	response := mechanismstudy.Response{
		Version:       mechanismstudy.ResultVersion,
		CatalogRef:    request.CatalogRef,
		CatalogSHA256: request.CatalogSHA256,
		RequestRef:    request.RequestRef,
		Cards:         make([]mechanismstudy.ResponseCard, 0, len(request.Cards)),
	}
	for _, card := range request.Cards {
		response.Cards = append(response.Cards, mechanismstudy.ResponseCard{
			CardRef: card.Ref, Mechanisms: []mechanismstudy.Candidate{},
		})
	}
	content, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	return atlasFirstAcceptanceCompletion(content, 211, 17), nil
}

func atlasFirstAcceptanceArchitectureResponse(
	combined string,
	reject bool,
) ([]byte, error) {
	const marker = "Bounded candidate request:\n"
	requestJSON := combined[strings.LastIndex(combined, marker)+len(marker):]
	var request componentmap.SynthesisRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		return nil, fmt.Errorf("decode Architecture request: %w", err)
	}
	if len(request.Candidates) == 0 {
		return nil, fmt.Errorf("Architecture request has no candidates")
	}
	members := make([]componentmap.SynthesisMemberRef, 0, len(request.Candidates))
	for _, candidate := range request.Candidates {
		members = append(members, candidate.Ref)
	}
	if reject {
		members[0].Ref += "-unknown-provider-member"
	}
	anchors := make([]componentmap.SynthesisAnchorRef, 0, len(request.BehaviorAnchors))
	for _, anchor := range request.BehaviorAnchors {
		anchors = append(anchors, anchor.Ref)
	}
	type architectureWireComponent struct {
		Kind         string                            `json:"kind"`
		SubsystemRef string                            `json:"subsystem_ref"`
		Name         string                            `json:"name"`
		Description  string                            `json:"description"`
		MemberRefs   []componentmap.SynthesisMemberRef `json:"member_refs"`
		AnchorRefs   []componentmap.SynthesisAnchorRef `json:"anchor_refs"`
		Hypothesis   bool                              `json:"hypothesis"`
	}
	type architectureWireSubsystem struct {
		Kind        string `json:"kind"`
		Ref         string `json:"ref"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	proposal := struct {
		Records []any `json:"records"`
	}{
		Records: []any{
			architectureWireSubsystem{
				Kind: "subsystem", Ref: "g1",
				Name: "Repository system", Description: "Conceptual grouping over exact local facts.",
			},
			architectureWireComponent{
				Kind: "component", SubsystemRef: "g1",
				Name: "Repository core", Description: "Groups the supplied local responsibilities.",
				MemberRefs: members, AnchorRefs: anchors, Hypothesis: len(anchors) == 0,
			},
		},
	}
	content, err := json.Marshal(proposal)
	if err != nil {
		return nil, err
	}
	return atlasFirstAcceptanceCompletion(content, 211, 73), nil
}

// atlasFirstAcceptanceWireBundle extracts the model-visible request bundle
// JSON that BuildScoutPrompt/BuildAdjudicationPrompt embed after the
// "Request bundle JSON:" marker.
func atlasFirstAcceptanceWireBundle(combined string) ([]byte, error) {
	const marker = "Request bundle JSON:\n"
	index := strings.LastIndex(combined, marker)
	if index < 0 {
		return nil, fmt.Errorf("theme request bundle marker is absent")
	}
	raw := []byte(combined[index+len(marker):])
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var bundle json.RawMessage
	if err := decoder.Decode(&bundle); err != nil {
		return nil, fmt.Errorf("decode theme request bundle boundary: %w", err)
	}
	const adjudicationReminder = `Response contract reminder after the request bundle: emit only the allowlisted response fields shown in the schema. Do not echo input-only theme_kind, anchor_refs, expansion_file_refs, or any other candidate/source fields.
Each reading observation must describe evidence from its own anchor_ref, not evidence belonging to another anchor or candidate.
Do not emit an unknown when the supplied evidence or the retained readings already answer it.`
	trailing := strings.TrimSpace(string(raw[decoder.InputOffset():]))
	if trailing != "" && trailing != adjudicationReminder {
		return nil, fmt.Errorf("theme request bundle has unexpected trailing prompt text")
	}
	return append([]byte(nil), bundle...), nil
}

func TestAtlasFirstAcceptanceWireBundleStopsBeforeAdjudicationReminder(t *testing.T) {
	combined := "system\nRequest bundle JSON:\n{\"candidates\":[]}\n" +
		"Response contract reminder after the request bundle: emit only the allowlisted response fields shown in the schema. Do not echo input-only theme_kind, anchor_refs, expansion_file_refs, or any other candidate/source fields.\n" +
		"Each reading observation must describe evidence from its own anchor_ref, not evidence belonging to another anchor or candidate.\n" +
		"Do not emit an unknown when the supplied evidence or the retained readings already answer it."
	raw, err := atlasFirstAcceptanceWireBundle(combined)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"candidates":[]}` {
		t.Fatalf("request bundle = %s", raw)
	}
}

// atlasFirstAcceptanceScoutResponse builds a valid Theme Scout response from
// the request bundle: candidate themes over the exact a* anchors and f*
// expansion refs, with one deliberately unknown-ref sibling when requested so
// the run exercises item-local rejection (accepted_partial).
func atlasFirstAcceptanceScoutResponse(combined string, includeBadSibling bool) ([]byte, error) {
	raw, err := atlasFirstAcceptanceWireBundle(combined)
	if err != nil {
		return nil, err
	}
	var wire struct {
		Vocabulary struct {
			Files []struct {
				Ref string `json:"ref"`
			} `json:"files"`
		} `json:"vocabulary"`
		SeedPacks struct {
			Packs []struct {
				Seed struct {
					Ref string `json:"ref"`
				} `json:"seed"`
			} `json:"packs"`
		} `json:"seed_packs"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("decode Theme Scout wire: %w", err)
	}
	var anchorRefs []string
	for _, pack := range wire.SeedPacks.Packs {
		if pack.Seed.Ref != "" {
			anchorRefs = append(anchorRefs, pack.Seed.Ref)
		}
	}
	if len(anchorRefs) == 0 {
		return nil, fmt.Errorf("Theme Scout wire has no a* seed anchor")
	}
	var fileRefs []string
	for _, file := range wire.Vocabulary.Files {
		if file.Ref != "" {
			fileRefs = append(fileRefs, file.Ref)
		}
	}
	expansionRef := ""
	if len(fileRefs) > 0 {
		expansionRef = fileRefs[0]
	}
	theme := func(title, question string, refs []string) map[string]any {
		return map[string]any{
			"title":               title,
			"question":            question,
			"theme_kind":          string(themestudy.KindSiblingImplementationFamily),
			"anchor_refs":         refs,
			"expansion_file_refs": []string{expansionRef},
			"why_it_matters":      "The accepted anchors participate in one bounded editorial responsibility.",
			"expected_learning":   "The reader can inspect the exact anchors and their bounded source.",
			"relation_claim":      "editorial_only",
			"focused":             false,
		}
	}
	// The candidate contract bounds each theme to MaxThemeAnchors anchors and
	// rejects duplicate normalized question/learning pairs, so large
	// repositories (etcd) are covered by several bounded candidates with
	// distinct questions instead of one oversized theme.
	var themes []any
	for offset := 0; offset < len(anchorRefs); offset += themestudy.MaxThemeAnchors {
		end := min(offset+themestudy.MaxThemeAnchors, len(anchorRefs))
		themes = append(themes, theme(
			fmt.Sprintf("Shared responsibilities across exact anchors %d", offset/themestudy.MaxThemeAnchors+1),
			fmt.Sprintf("How do anchors %d through %d work together in this repository?", offset+1, end),
			anchorRefs[offset:end],
		))
	}
	if includeBadSibling {
		themes = append(themes, map[string]any{
			"title":               "Deliberately invalid sibling",
			"question":            "This candidate references an unknown anchor to prove item-local rejection.",
			"theme_kind":          string(themestudy.KindCrossCuttingPolicy),
			"anchor_refs":         []string{"a999999"},
			"expansion_file_refs": []string{expansionRef},
			"why_it_matters":      "The unknown ref must reject only this candidate.",
			"expected_learning":   "The valid sibling survives.",
			"relation_claim":      "editorial_only",
			"focused":             true,
		})
	}
	content, err := json.Marshal(map[string]any{"themes": themes})
	if err != nil {
		return nil, err
	}
	return atlasFirstAcceptanceCompletion(content, 211, 31), nil
}

// atlasFirstAcceptanceAdjudicationResponse builds a valid Theme Adjudication
// response from the request bundle: for every t* candidate, a direct
// reading over its own a* anchors with a supported observation, final
// question-aligned editorial prose, and stable reading order.
func atlasFirstAcceptanceAdjudicationResponse(combined string, rejectAll bool) ([]byte, error) {
	raw, err := atlasFirstAcceptanceWireBundle(combined)
	if err != nil {
		return nil, err
	}
	var wire struct {
		Candidates []struct {
			Ref        string   `json:"ref"`
			AnchorRefs []string `json:"anchor_refs"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("decode Theme Adjudication wire: %w", err)
	}
	if len(wire.Candidates) == 0 {
		return nil, fmt.Errorf("Theme Adjudication wire has no candidate")
	}
	themes := make([]any, 0, len(wire.Candidates))
	for _, candidate := range wire.Candidates {
		if len(candidate.AnchorRefs) == 0 {
			return nil, fmt.Errorf("Theme Adjudication candidate %s has no anchor", candidate.Ref)
		}
		// Phase 3 canonical wire: one readings array whose position IS the
		// reading order; support is direct/supporting only, no role field.
		readingRefs := candidate.AnchorRefs
		if rejectAll {
			// Reproduce the fresh Syn response shape: f* is a valid file ref in
			// the request, but it is never an eligible exact reading anchor.
			readingRefs = []string{"f2", "f4", "f3"}
		}
		readings := make([]any, 0, len(readingRefs))
		for _, anchor := range readingRefs {
			readings = append(readings, map[string]any{
				"anchor_ref":  anchor,
				"support":     "direct",
				"observation": "The exact source pack shows this anchor participating in the theme.",
			})
		}
		themes = append(themes, map[string]any{
			"candidate_ref":     candidate.Ref,
			"final_title":       "Accepted theme for " + candidate.Ref,
			"final_question":    "What shared responsibility do the exact anchors implement?",
			"why_it_matters":    "The final question identifies a source-backed responsibility worth understanding.",
			"expected_learning": "The retained readings explain that bounded responsibility.",
			"readings":          readings,
			"unknowns":          []string{},
		})
	}
	content, err := json.Marshal(map[string]any{"themes": themes})
	if err != nil {
		return nil, err
	}
	return atlasFirstAcceptanceCompletion(content, 211, 73), nil
}

func atlasFirstAcceptanceCompletion(content []byte, inputTokens, outputTokens int) []byte {
	envelope, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"finish_reason": "stop",
			"message":       map[string]any{"role": "assistant", "content": string(content)},
		}},
		"usage": map[string]any{
			"prompt_tokens":            inputTokens,
			"completion_tokens":        outputTokens,
			"prompt_cache_hit_tokens":  0,
			"prompt_cache_miss_tokens": inputTokens,
		},
	})
	return envelope
}

func (provider *atlasFirstAcceptanceProvider) assertStages(
	t *testing.T,
	want ...atlasFirstAcceptanceStage,
) {
	t.Helper()
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if !reflect.DeepEqual(provider.stages, want) {
		t.Fatalf("provider stage order = %v, want %v", provider.stages, want)
	}
	for stage, bodies := range provider.bodies {
		if len(bodies) != 1 {
			t.Fatalf("provider stage %s request count = %d, want one", stage, len(bodies))
		}
	}
}

func (provider *atlasFirstAcceptanceProvider) assertStageCalls(
	t *testing.T,
	stage atlasFirstAcceptanceStage,
	want int,
) {
	t.Helper()
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if got := len(provider.bodies[stage]); got != want {
		t.Fatalf("provider stage %s request count = %d, want %d", stage, got, want)
	}
}

func (provider *atlasFirstAcceptanceProvider) assertStagesWithMechanismBatches(
	t *testing.T,
	includeTargetPortfolio bool,
	minimum int,
	maximum int,
) {
	t.Helper()
	provider.mu.Lock()
	defer provider.mu.Unlock()
	prefix := []atlasFirstAcceptanceStage{
		atlasFirstStageArchitecture,
		atlasFirstStageStudyScout,
		atlasFirstStageStudyAdjudication,
	}
	if includeTargetPortfolio {
		prefix = append([]atlasFirstAcceptanceStage{atlasFirstStageTargetPortfolio}, prefix...)
	} else if calls := len(provider.bodies[atlasFirstStageTargetPortfolio]); calls != 0 {
		t.Fatalf("provider stage %s request count = %d, want zero", atlasFirstStageTargetPortfolio, calls)
	}
	if len(provider.stages) < len(prefix)+minimum+1 || len(provider.stages) > len(prefix)+maximum+1 ||
		!reflect.DeepEqual(provider.stages[:min(len(provider.stages), len(prefix))], prefix) {
		t.Fatalf("provider stage order = %v, want %v followed by %d..%d mechanism batches and entry-call",
			provider.stages, prefix, minimum, maximum)
	}
	if provider.stages[len(provider.stages)-1] != atlasFirstStageEntryCall {
		t.Fatalf("provider stage order = %v, want entry-call last", provider.stages)
	}
	for _, stage := range provider.stages[len(prefix) : len(provider.stages)-1] {
		if stage != atlasFirstStageStudyMechanism {
			t.Fatalf("provider stage order = %v, want only mechanism batches after %v", provider.stages, prefix)
		}
	}
	for _, stage := range prefix {
		if len(provider.bodies[stage]) != 1 {
			t.Fatalf("provider stage %s request count = %d, want one", stage, len(provider.bodies[stage]))
		}
	}
	if count := len(provider.bodies[atlasFirstStageStudyMechanism]); count < minimum || count > maximum {
		t.Fatalf("mechanism request count = %d, want %d..%d", count, minimum, maximum)
	}
	if len(provider.bodies[atlasFirstStageEntryCall]) != 1 {
		t.Fatalf("entry-call request count = %d, want one", len(provider.bodies[atlasFirstStageEntryCall]))
	}
}

func atlasFirstAcceptanceRepository(t *testing.T, fixtureDir string) string {
	t.Helper()
	repo := t.TempDir()
	var names []string
	err := filepath.WalkDir(fixtureDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(fixtureDir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		destination := filepath.Join(repo, relative)
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(destination, data, 0o600); err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	runGit(t, repo, "init", "--quiet")
	args := append([]string{"add", "--"}, names...)
	runGit(t, repo, args...)
	t.Setenv("GIT_AUTHOR_DATE", "2026-01-01T00:00:00Z")
	t.Setenv("GIT_COMMITTER_DATE", "2026-01-01T00:00:00Z")
	commitTestRepository(t, repo)
	return repo
}

func assertAtlasFirstAcceptedArchitecture(t *testing.T, data *report.ReportData) {
	t.Helper()
	if data.ArchitectureSynthesis == nil ||
		(data.ArchitectureSynthesis.State != report.ArchitectureSynthesisSucceeded &&
			data.ArchitectureSynthesis.State != report.ArchitectureSynthesisCached) ||
		!data.ArchitectureSynthesis.ProposalAccepted ||
		data.ArchitectureSynthesis.ProposalRejected ||
		data.ArchitectureSynthesis.FallbackSelected ||
		data.ArchitectureCanvas == nil || data.ArchitectureCanvas.Fallback ||
		(data.ArchitectureCanvas.ArchitectureSource != componentmap.SourceValidatedModel &&
			data.ArchitectureCanvas.ArchitectureSource != componentmap.SourcePartialModel &&
			data.ArchitectureCanvas.ArchitectureSource != componentmap.SourceNormalizedModel) {
		t.Fatalf("accepted Architecture = %#v / %#v", data.ArchitectureSynthesis, data.ArchitectureCanvas)
	}
}

func assertAtlasFirstAcceptedStudy(t *testing.T, data *report.ReportData) {
	t.Helper()
	if data.AtlasStudy == nil || data.AtlasStudy.Version != themestudy.ScoutResultVersion ||
		data.AtlasStudy.Themes == nil || len(data.AtlasStudy.Themes.Cards) == 0 {
		t.Fatalf("accepted theme Study = %#v", data.AtlasStudy)
	}
	// D213 admits exactly the accepted state pairs: a fully accepted shelf or
	// a partial acceptance with rejected siblings uncovered. Anything else —
	// including a third state value — is a failure.
	switch data.AtlasStudy.State {
	case atlasstudy.ProductStateAccepted, atlasstudy.ProductStateAcceptedPartial:
	default:
		t.Fatalf("accepted theme Study = %#v", data.AtlasStudy)
	}
	for _, card := range data.AtlasStudy.Themes.Cards {
		if len(card.Readings) == 0 {
			t.Fatalf("theme card has no exact reading: %#v", card)
		}
	}
}

func assertAtlasFirstLocalSubstrateUnchanged(t *testing.T, data *report.ReportData) {
	t.Helper()
	input, err := report.BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatal(err)
	}
	local, err := report.ProjectArchitectureCanvas(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(data.ArchitectureCanvas.BehaviorAnchors, local.BehaviorAnchors) ||
		!reflect.DeepEqual(data.ArchitectureCanvas.StructuralFacts, local.StructuralFacts) {
		t.Fatalf("model grouping changed exact local anchors/relations")
	}
	gotCandidates := make([]componentmap.Candidate, 0)
	for _, component := range data.ArchitectureCanvas.Components {
		gotCandidates = append(gotCandidates, component.Members...)
	}
	sort.Slice(gotCandidates, func(i, j int) bool {
		if gotCandidates[i].ID.Kind != gotCandidates[j].ID.Kind {
			return gotCandidates[i].ID.Kind < gotCandidates[j].ID.Kind
		}
		return gotCandidates[i].ID.Value < gotCandidates[j].ID.Value
	})
	wantCandidates := make([]componentmap.Candidate, 0, len(input.CandidateBundle.Candidates))
	wantStructuralLocators := make([]componentmap.Candidate, 0, len(input.CandidateBundle.Candidates))
	for _, candidate := range input.CandidateBundle.Candidates {
		if candidate.Role == componentmap.CandidateRoleStructuralLocator {
			wantStructuralLocators = append(wantStructuralLocators, candidate)
			continue
		}
		wantCandidates = append(wantCandidates, candidate)
	}
	sort.Slice(wantCandidates, func(i, j int) bool {
		if wantCandidates[i].ID.Kind != wantCandidates[j].ID.Kind {
			return wantCandidates[i].ID.Kind < wantCandidates[j].ID.Kind
		}
		return wantCandidates[i].ID.Value < wantCandidates[j].ID.Value
	})
	if !reflect.DeepEqual(gotCandidates, wantCandidates) {
		t.Fatalf("model grouping changed exact local candidates\ngot:  %#v\nwant: %#v", gotCandidates, wantCandidates)
	}
	gotStructuralLocators := make([]componentmap.Candidate, 0, len(data.ArchitectureCanvas.StructuralLocators))
	for _, locator := range data.ArchitectureCanvas.StructuralLocators {
		gotStructuralLocators = append(gotStructuralLocators, locator.Locator)
	}
	sort.Slice(gotStructuralLocators, func(i, j int) bool {
		if gotStructuralLocators[i].ID.Kind != gotStructuralLocators[j].ID.Kind {
			return gotStructuralLocators[i].ID.Kind < gotStructuralLocators[j].ID.Kind
		}
		return gotStructuralLocators[i].ID.Value < gotStructuralLocators[j].ID.Value
	})
	sort.Slice(wantStructuralLocators, func(i, j int) bool {
		if wantStructuralLocators[i].ID.Kind != wantStructuralLocators[j].ID.Kind {
			return wantStructuralLocators[i].ID.Kind < wantStructuralLocators[j].ID.Kind
		}
		return wantStructuralLocators[i].ID.Value < wantStructuralLocators[j].ID.Value
	})
	if !reflect.DeepEqual(gotStructuralLocators, wantStructuralLocators) {
		t.Fatalf("model grouping changed exact structural locators\ngot:  %#v\nwant: %#v", gotStructuralLocators, wantStructuralLocators)
	}
}

func assertAtlasFirstDiagnostics(
	t *testing.T,
	runDir string,
	wantStates map[string]string,
) {
	t.Helper()
	metadataBytes, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata debugdump.RunMeta
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
		t.Fatal(err)
	}
	if !metadata.ProviderAccountingComplete || len(metadata.RequestAttempts) != len(wantStates) {
		t.Fatalf("Atlas-first diagnostic totals = %#v", metadata)
	}
	totalCalls := 0
	seenStages := make(map[string]struct{}, len(metadata.RequestAttempts))
	for _, attempt := range metadata.RequestAttempts {
		if _, duplicate := seenStages[attempt.Stage]; duplicate {
			t.Fatalf("Atlas-first diagnostic duplicated stage %q: %#v", attempt.Stage, metadata.RequestAttempts)
		}
		seenStages[attempt.Stage] = struct{}{}
		wantState, ok := wantStates[attempt.Stage]
		if !ok || attempt.State != wantState {
			t.Fatalf("Atlas-first stage diagnostic = %#v, want states %#v", attempt, wantStates)
		}
		maxCalls := 1
		if attempt.Stage == debugdump.SemanticStageAtlasStudy {
			maxCalls = 2
		} else if attempt.Stage == debugdump.SemanticStageMechanismStudy {
			maxCalls = mechanismstudy.MaxProviderCalls
		}
		if attempt.ProviderCallCount < 0 || attempt.ProviderCallCount > maxCalls {
			t.Fatalf("Atlas-first stage call count exceeds its bounded plan: %#v", attempt)
		}
		if attempt.TransportAttemptCount < attempt.ProviderCallCount {
			t.Fatalf("Atlas-first transport accounting is smaller than semantic calls: %#v", attempt)
		}
		totalCalls += attempt.ProviderCallCount
	}
	if metadata.ProviderRequestCount != totalCalls {
		t.Fatalf(
			"Atlas-first provider total = %d, want sum of stage calls %d: %#v",
			metadata.ProviderRequestCount,
			totalCalls,
			metadata.RequestAttempts,
		)
	}
}

func assertAtlasFirstSemanticStages(t *testing.T, runDir string, want ...string) {
	t.Helper()
	entries := readSemanticJournalEntries(t, runDir)
	seen := make(map[string]struct{}, len(entries))
	var got []string
	for _, entry := range entries {
		if _, dup := seen[entry.record.Stage]; dup {
			continue
		}
		seen[entry.record.Stage] = struct{}{}
		got = append(got, entry.record.Stage)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("semantic stages = %v, want current Atlas-first stages %v", got, want)
	}
}

func assertAtlasFirstAcceptedArtifacts(t *testing.T, runDir string) {
	t.Helper()
	artifactNames := []string{
		report.ArchitectureSynthesisFile,
		report.ArchitectureSynthesisStatusFile,
		themestudy.ScoutRequestArtifactFilename,
		themestudy.ScoutResultArtifactFilename,
		themestudy.ScoutStatusArtifactFilename,
		themestudy.ExpansionArtifactFilename,
		themestudy.AdjudicationRequestArtifactFilename,
		themestudy.AdjudicationResultArtifactFilename,
		themestudy.AdjudicationStatusArtifactFilename,
		themestudy.StudyThemesArtifactFilename,
		"report.json",
		"report.html",
		report.RunManifestFilename,
	}
	for _, name := range artifactNames {
		if _, err := os.Stat(filepath.Join(runDir, name)); err != nil {
			t.Fatalf("accepted Atlas-first run missing %s: %v", name, err)
		}
	}
}

func assertAtlasFirstAcceptedManifest(
	t *testing.T,
	manifest report.RunManifest,
) {
	t.Helper()
	inputs := manifest.MaterialInputs
	if inputs.RepositoryAtlasSHA256 == "" ||
		inputs.ThemeScoutRequestSHA256 == "" ||
		inputs.ThemeScoutResultSHA256 == "" ||
		inputs.ThemeScoutStatusSHA256 == "" ||
		inputs.ThemeSourceExpansionSHA256 == "" ||
		inputs.ThemeAdjudicationRequestSHA256 == "" ||
		inputs.ThemeAdjudicationResultSHA256 == "" ||
		inputs.ThemeAdjudicationStatusSHA256 == "" ||
		inputs.StudyThemesSHA256 == "" ||
		inputs.ModelBundleSHA256 != "" ||
		inputs.OrientationContextSelectionSHA256 != "" {
		t.Fatalf("Atlas-first manifest bindings = %#v", inputs)
	}
}

func assertNoLegacyAtlasFirstArtifacts(t *testing.T, runDir string) {
	t.Helper()
	for _, name := range []string{
		"orientation_report.json",
		"llm_bundle.json",
		guidedTourBundleFile,
		guidedTourMonolithicFile,
		studymap.RecordFile,
		studymap.BundleFile,
		studyMapBriefShapeFile,
		studyMapDirectionsFile,
		studyMapReviewsFile,
		"flows",
	} {
		if _, err := os.Stat(filepath.Join(runDir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Atlas-first run retained legacy artifact %s: %v", name, err)
		}
	}
}
