package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const (
	chiDispatchQuestion = "How does Mux dispatch an HTTP request to an endpoint or 404/405?"
	// The semantic prose validator treats slash-separated tokens as possible
	// repository paths. Keep the operator-facing question exact while spelling
	// out the same alternatives in model-visible candidate prose.
	chiDispatchCandidateQuestion  = "How does Mux dispatch an HTTP request to an endpoint, not found, or method not allowed?"
	chiDispatchIntentKey          = "chi-request-dispatch"
	chiDispatchProjectionFile     = "chi_request_dispatch_projection.json"
	chiDispatchSupplementFile     = "chi_request_dispatch_supplement.json"
	chiDispatchFixtureFile        = "chi_request_dispatch_fixture.json"
	chiDispatchReservationFile    = "chi_request_dispatch_call_reservation.json"
	chiDispatchResponseFile       = "chi_request_dispatch_response_attempt.json"
	chiDispatchResponseReplayFile = "chi_request_dispatch_response_replay_status.json"
	chiDispatchStatusFile         = "chi_request_dispatch_status.json"
	chiDispatchReplayFile         = "chi_request_dispatch_replay_status.json"
	chiDispatchMaxModelCalls      = 1
	chiDispatchMinScore           = 3
	chiDispatchFixtureSHA256      = "9f8ffba8fe5b34a838147194866e338c64332cc5b194337e34e16b0f48f2d53d"
	chiDispatchProjectionSHA256   = "3dbced1fd30cc23256f7636a5dad4676a5e6312df45f0591f7d88ac9cc22381a"
	chiDispatchSupplementSHA256   = "e50427b4596d352beba84c25dff38a397f0bb8fe24313c350bfb7cb924564595"
	chiDispatchResponseSHA256     = "bd980b40ac93abe5de763944e9f4017c345f8a89824c19d7bdb8fcf44172b008"
	chiDispatchContentSHA256      = "260f612030dbe8e4dfb708bba55b3dc0aa7c9d19a5a3b625a1e4dd914299e901"
)

var chiDispatchIdentity = semanticdiscovery.MechanismIdentity{
	RepositoryNamespace: "github.com/go-chi/chi/v5",
	IntentKey:           chiDispatchIntentKey,
	Scope: semanticdiscovery.MechanismScope{
		Kind:  semanticdiscovery.MechanismScopeGoPackage,
		Value: "github.com/go-chi/chi/v5",
	},
}

type chiDispatchMode string

const (
	chiDispatchLive           chiDispatchMode = "live"
	chiDispatchPrepare        chiDispatchMode = "prepare"
	chiDispatchReplay         chiDispatchMode = "replay"
	chiDispatchResponseReplay chiDispatchMode = "response_replay"
)

type chiDispatchRun struct {
	runDir       string
	analysisRoot string
	manifest     report.RunManifest
	current      freshness.RepositoryState
	data         *report.ReportData
	baseBundle   semanticdiscovery.Bundle
}

type chiDispatchFixture struct {
	Version              int                  `json:"version"`
	State                string               `json:"state"`
	RepositoryRevision   string               `json:"repository_revision"`
	Question             string               `json:"question"`
	IntentKey            string               `json:"intent_key"`
	CandidateID          string               `json:"candidate_id"`
	ProbePlan            goldenmechanism.Plan `json:"probe_plan"`
	ProbeSHA256          string               `json:"probe_sha256"`
	ProjectionSHA256     string               `json:"projection_sha256"`
	SupplementSHA256     string               `json:"supplement_sha256"`
	EnrichedBundleSHA256 string               `json:"enriched_bundle_sha256"`
	PromptVersion        string               `json:"prompt_version"`
	PromptSHA256         string               `json:"prompt_sha256"`
	MaxModelCalls        int                  `json:"max_model_calls"`
}

type chiDispatchPrepared struct {
	Loaded      chiDispatchRun
	Probe       goldenmechanism.Result
	Projection  goldenProjection
	Supplement  report.SemanticSupplement
	Bundle      semanticdiscovery.Bundle
	Proposal    semanticdiscovery.OpportunityProposal
	Leaf        semanticdiscovery.LeafResult
	Prompt      semanticdiscovery.Prompt
	Fixture     chiDispatchFixture
	FixtureHash string
}

type chiDispatchStatus struct {
	Version             int                                     `json:"version"`
	State               string                                  `json:"state"`
	FailureClass        string                                  `json:"failure_class,omitempty"`
	FailureReason       string                                  `json:"failure_reason,omitempty"`
	Question            string                                  `json:"question"`
	CandidateID         string                                  `json:"candidate_id,omitempty"`
	RepositoryRevision  string                                  `json:"repository_revision,omitempty"`
	FixtureSHA256       string                                  `json:"fixture_sha256,omitempty"`
	ProbeCalls          int                                     `json:"bounded_retrieval_calls"`
	RepositoryAnalyzers int                                     `json:"repository_analyzers"`
	ModelCalls          int                                     `json:"model_calls"`
	ProbeBudget         *goldenmechanism.BudgetStats            `json:"probe_budget,omitempty"`
	Synthesis           *semanticDiscoveryStageMetrics          `json:"synthesis,omitempty"`
	Reduction           *semanticdiscovery.FanInReductionReport `json:"reduction,omitempty"`
	Artifact            *goldenMechanismArtifactSummary         `json:"artifact,omitempty"`
	MechanismID         string                                  `json:"mechanism_id,omitempty"`
	ReportFile          string                                  `json:"report_file,omitempty"`
}

type chiDispatchReservation struct {
	Version       int    `json:"version"`
	State         string `json:"state"`
	CandidateID   string `json:"candidate_id"`
	FixtureSHA256 string `json:"fixture_sha256"`
	PromptVersion string `json:"prompt_version"`
	PromptSHA256  string `json:"prompt_sha256"`
	MaxCalls      int    `json:"max_calls"`
}

type chiDispatchReplayStatus struct {
	Version             int      `json:"version"`
	State               string   `json:"state"`
	ModelCalls          int      `json:"model_calls"`
	RepositoryAnalyzers int      `json:"repository_analyzers"`
	BoundedRetrievals   int      `json:"bounded_retrieval_calls"`
	MechanismID         string   `json:"mechanism_id"`
	ArtifactID          string   `json:"artifact_id"`
	EvidencePaths       []string `json:"evidence_paths"`
	SearchIndexed       bool     `json:"search_indexed"`
	FocusNonEmpty       bool     `json:"focus_non_empty"`
	HTMLRendered        bool     `json:"html_rendered"`
}

type chiDispatchResponseReplayStatus struct {
	Version               int                                     `json:"version"`
	State                 string                                  `json:"state"`
	FailureClass          string                                  `json:"failure_class,omitempty"`
	FailureReason         string                                  `json:"failure_reason,omitempty"`
	FixtureSHA256         string                                  `json:"fixture_sha256"`
	ProjectionSHA256      string                                  `json:"projection_sha256"`
	SupplementSHA256      string                                  `json:"supplement_sha256"`
	ResponseSHA256        string                                  `json:"response_sha256"`
	ResponseContentSHA256 string                                  `json:"response_content_sha256"`
	ModelCalls            int                                     `json:"model_calls"`
	ProbeCalls            int                                     `json:"probe_calls"`
	RepositoryAnalyzers   int                                     `json:"repository_analyzers"`
	Reduction             *semanticdiscovery.FanInReductionReport `json:"reduction,omitempty"`
	Artifact              *goldenMechanismArtifactSummary         `json:"artifact,omitempty"`
	MechanismID           string                                  `json:"mechanism_id,omitempty"`
	ReportFile            string                                  `json:"report_file,omitempty"`
}

func runChiRequestDispatchCLI(args []string, stdout, stderr io.Writer) error {
	runDir, mode, err := parseChiRequestDispatchArgs(args)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if mode == chiDispatchReplay {
		return replayChiRequestDispatch(runDir, stdout)
	}
	if mode == chiDispatchResponseReplay {
		return replaySavedChiDispatchResponse(ctx, runDir, stdout, stderr)
	}
	return runChiRequestDispatch(ctx, runDir, mode, stdout, stderr)
}

func parseChiRequestDispatchArgs(args []string) (string, chiDispatchMode, error) {
	mode := chiDispatchLive
	runDir := ""
	for _, arg := range args {
		switch arg {
		case "--prepare":
			if mode != chiDispatchLive {
				return "", "", chiRequestDispatchUsage()
			}
			mode = chiDispatchPrepare
		case "--replay":
			if mode != chiDispatchLive {
				return "", "", chiRequestDispatchUsage()
			}
			mode = chiDispatchReplay
		case "--replay-response":
			if mode != chiDispatchLive {
				return "", "", chiRequestDispatchUsage()
			}
			mode = chiDispatchResponseReplay
		default:
			if strings.HasPrefix(arg, "-") || runDir != "" {
				return "", "", chiRequestDispatchUsage()
			}
			runDir = arg
		}
	}
	if runDir == "" {
		return "", "", chiRequestDispatchUsage()
	}
	return runDir, mode, nil
}

func chiRequestDispatchUsage() error {
	return fmt.Errorf("Usage: repomap dev chi-request-dispatch <run-dir> [--prepare | --replay-response | --replay]")
}

func runChiRequestDispatch(
	ctx context.Context,
	runDir string,
	mode chiDispatchMode,
	stdout io.Writer,
	stderr io.Writer,
) (returnErr error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return err
	}
	status := chiDispatchStatus{Version: 1, State: "started", Question: chiDispatchQuestion}
	defer func() {
		if returnErr != nil && status.State != "rejected" {
			status.State = "failed"
			if status.FailureClass == "" {
				status.FailureClass = "local_stage_failure"
			}
			if status.FailureReason == "" {
				status.FailureReason = semanticDiscoveryReason(returnErr.Error())
			}
		}
		if writeErr := writeGoldenJSON(filepath.Join(absDir, chiDispatchStatusFile), status); writeErr != nil {
			returnErr = errors.Join(returnErr, writeErr)
		}
	}()

	if mode == chiDispatchPrepare {
		prepared, budget, prepareErr := prepareChiDispatch(ctx, absDir)
		if budget.FilesParsed > 0 {
			status.ProbeCalls = 1
			status.ProbeBudget = &budget
		}
		if prepareErr != nil {
			status.FailureClass = "fixture_preparation_failed"
			return prepareErr
		}
		verified, verifyErr := loadPreparedChiDispatch(ctx, absDir)
		if verifyErr != nil || verified.FixtureHash != prepared.FixtureHash {
			status.FailureClass = "frozen_fixture_invalid"
			if verifyErr != nil {
				return verifyErr
			}
			return fmt.Errorf("chi request dispatch: frozen fixture hash changed during verification")
		}
		status.State = "fixture_frozen"
		status.CandidateID = prepared.Projection.Candidate.ID
		status.RepositoryRevision = prepared.Loaded.manifest.RepositoryState.Head
		status.FixtureSHA256 = prepared.FixtureHash
		fmt.Fprintf(
			stderr,
			"repomap: chi dispatch froze %d deterministic facts from four saved windows and two frontier functions; no model call\n",
			len(prepared.Projection.Facts),
		)
		fmt.Fprintf(stdout, "Frozen fixture: %s\n", filepath.Join(absDir, chiDispatchFixtureFile))
		return nil
	}

	prepared, err := loadPreparedChiDispatch(ctx, absDir)
	if err != nil {
		status.FailureClass = "frozen_fixture_invalid"
		return err
	}
	status.CandidateID = prepared.Projection.Candidate.ID
	status.RepositoryRevision = prepared.Loaded.manifest.RepositoryState.Head
	status.FixtureSHA256 = prepared.FixtureHash
	status.ProbeCalls = 1
	probeBudget := prepared.Probe.Budget
	status.ProbeBudget = &probeBudget

	client, err := deepseek.NewFromEnv()
	if err != nil {
		status.FailureClass = "provider_unavailable"
		return fmt.Errorf("chi request dispatch: provider configuration: %w", err)
	}
	client.OnWait = func(progress deepseek.WaitProgress) {
		fmt.Fprintf(
			stderr,
			"repomap: %s still running after %s (Ctrl-C to cancel)\n",
			progress.Stage,
			progress.Elapsed.Round(time.Second),
		)
	}
	if err := reserveChiDispatchCall(
		filepath.Join(absDir, chiDispatchReservationFile),
		prepared,
	); err != nil {
		status.FailureClass = "single_call_already_reserved"
		return err
	}

	fmt.Fprintln(stderr, "repomap: running the one allowed chi request-dispatch synthesis over frozen facts")
	counted := &countingSemanticDiscoveryEditor{delegate: client}
	synthesis, synthesisErr := executeGoldenMechanismSynthesis(
		ctx,
		prepared.Bundle,
		prepared.Proposal,
		prepared.Leaf,
		counted,
	)
	status.ModelCalls = counted.calls
	status.Synthesis = &synthesis.Metrics
	status.Reduction = &synthesis.Reduction
	if saveErr := saveChiDispatchResponse(absDir, prepared, synthesis, synthesisErr); saveErr != nil {
		status.FailureClass = "response_artifact_write_failed"
		return errors.Join(synthesisErr, saveErr)
	}
	if counted.calls != chiDispatchMaxModelCalls {
		status.FailureClass = "single_call_count_invalid"
		return errors.Join(
			synthesisErr,
			fmt.Errorf("chi request dispatch: provider calls = %d, want exactly one", counted.calls),
		)
	}
	if synthesisErr != nil {
		status.State = "rejected"
		status.FailureClass = classifyGoldenMechanismValidationFailure(synthesis.Reduction)
		status.FailureReason = semanticDiscoveryReason(synthesisErr.Error())
		return synthesisErr
	}

	artifact := synthesis.Artifacts[0]
	summary, err := summarizeGoldenMechanismArtifact(prepared.Projection.Candidate, artifact)
	if err != nil {
		status.FailureClass = "fixed_rubric_failed"
		return err
	}
	status.Artifact = &summary
	if summary.LocalRubricScore < chiDispatchMinScore {
		status.FailureClass = "fixed_rubric_failed"
		return fmt.Errorf(
			"chi request dispatch: artifact score %d is below %d",
			summary.LocalRubricScore,
			chiDispatchMinScore,
		)
	}

	mechanism, err := publishChiDispatch(
		ctx,
		prepared,
		append(append([]byte(nil), synthesis.RecordBytes...), '\n'),
		artifact,
	)
	if err != nil {
		status.FailureClass = "publication_failed"
		return err
	}
	status.State = "published"
	status.MechanismID = mechanism.ID
	status.ReportFile = filepath.Join(absDir, "report.html")
	fmt.Fprintf(
		stderr,
		"repomap: chi dispatch accepted with %d supported steps, %d covered aspects, score %d/4\n",
		summary.SupportedSteps,
		len(summary.CoveredAnswerAspects),
		summary.LocalRubricScore,
	)
	fmt.Fprintf(
		stdout,
		"Mechanism: %s\nContent: %s\nReport: %s\n",
		mechanism.ID,
		mechanism.ContentSHA256,
		status.ReportFile,
	)
	return nil
}

func loadChiDispatchRun(ctx context.Context, runDir string) (chiDispatchRun, error) {
	manifest, err := report.ReadRunManifest(runDir)
	if err != nil {
		return chiDispatchRun{}, fmt.Errorf("chi request dispatch: verify saved run manifest: %w", err)
	}
	analysisRoot, err := manifest.ResolveAnalysisRoot()
	if err != nil {
		return chiDispatchRun{}, err
	}
	current, err := freshness.CaptureRepository(ctx, analysisRoot)
	if err != nil {
		return chiDispatchRun{}, fmt.Errorf("chi request dispatch: capture repository state: %w", err)
	}
	if current.Head != manifest.RepositoryState.Head {
		return chiDispatchRun{}, fmt.Errorf("chi request dispatch: repository revision changed")
	}
	if err := manifest.VerifyRepositoryState(current); err != nil {
		return chiDispatchRun{}, err
	}
	data, err := report.ReadRunDir(runDir)
	if err != nil {
		return chiDispatchRun{}, err
	}
	if !chiReportOwnsScope(data) {
		return chiDispatchRun{}, fmt.Errorf("chi request dispatch: fixed chi package is not owned by this report")
	}
	data.SemanticSupplementalFacts = nil
	bundle, err := report.BuildSemanticDiscoveryBundle(data)
	if err != nil {
		return chiDispatchRun{}, err
	}
	return chiDispatchRun{
		runDir: runDir, analysisRoot: analysisRoot, manifest: manifest,
		current: current, data: data, baseBundle: bundle,
	}, nil
}

func chiReportOwnsScope(data *report.ReportData) bool {
	if data == nil || data.RepositoryGraph == nil {
		return false
	}
	for _, pkg := range data.RepositoryGraph.Packages {
		if pkg.CanonicalPath == chiDispatchIdentity.Scope.Value &&
			pkg.ModulePath == chiDispatchIdentity.RepositoryNamespace &&
			(pkg.Locality == "" || pkg.Locality == "local") {
			return true
		}
	}
	return false
}

func loadPreparedChiDispatch(ctx context.Context, runDir string) (chiDispatchPrepared, error) {
	loaded, err := loadChiDispatchRun(ctx, runDir)
	if err != nil {
		return chiDispatchPrepared{}, err
	}
	fixtureRaw, err := readBoundedRegularFile(filepath.Join(runDir, chiDispatchFixtureFile), maxGoldenSavedFileBytes)
	if err != nil {
		return chiDispatchPrepared{}, fmt.Errorf("chi request dispatch: run --prepare before the single model call: %w", err)
	}
	var fixture chiDispatchFixture
	if err := decodeGoldenFixture(fixtureRaw, &fixture); err != nil {
		return chiDispatchPrepared{}, err
	}
	if fixture.Version != 1 || fixture.State != "frozen_before_model_response" ||
		fixture.RepositoryRevision != loaded.manifest.RepositoryState.Head ||
		fixture.Question != chiDispatchQuestion || fixture.IntentKey != chiDispatchIntentKey ||
		fixture.MaxModelCalls != chiDispatchMaxModelCalls ||
		fixture.PromptVersion != semanticdiscovery.GoldenMechanismPromptVersion {
		return chiDispatchPrepared{}, fmt.Errorf("chi request dispatch: frozen fixture identity changed")
	}

	probeRaw, err := readBoundedRegularFile(
		filepath.Join(runDir, report.GoldenMechanismProbeFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil || digestSHA256(probeRaw) != fixture.ProbeSHA256 {
		return chiDispatchPrepared{}, fmt.Errorf("chi request dispatch: frozen probe changed")
	}
	var probe goldenmechanism.Result
	if err := decodeGoldenFixture(probeRaw, &probe); err != nil {
		return chiDispatchPrepared{}, err
	}
	if err := probe.Validate(); err != nil {
		return chiDispatchPrepared{}, err
	}

	projectionRaw, err := readBoundedRegularFile(
		filepath.Join(runDir, chiDispatchProjectionFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil || digestSHA256(projectionRaw) != fixture.ProjectionSHA256 {
		return chiDispatchPrepared{}, fmt.Errorf("chi request dispatch: frozen projection changed")
	}
	var projection goldenProjection
	if err := decodeGoldenFixture(projectionRaw, &projection); err != nil {
		return chiDispatchPrepared{}, err
	}
	if projection.Candidate.ID != fixture.CandidateID || projection.Leaf.Task.Candidate.ID != fixture.CandidateID {
		return chiDispatchPrepared{}, fmt.Errorf("chi request dispatch: frozen candidate changed")
	}

	supplement, bundle, err := report.PrepareSemanticSupplement(
		loaded.data,
		projection.Candidate.ID,
		fixture.ProbeSHA256,
		projection.Facts,
	)
	if err != nil {
		return chiDispatchPrepared{}, err
	}
	supplementRaw, err := marshalGoldenJSON(supplement)
	if err != nil || digestSHA256(supplementRaw) != fixture.SupplementSHA256 {
		return chiDispatchPrepared{}, fmt.Errorf("chi request dispatch: frozen supplement changed")
	}
	frozenSupplement, err := readBoundedRegularFile(
		filepath.Join(runDir, chiDispatchSupplementFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil || !bytes.Equal(frozenSupplement, supplementRaw) {
		return chiDispatchPrepared{}, fmt.Errorf("chi request dispatch: frozen supplement bytes changed")
	}
	bundleSHA, _, err := semanticdiscovery.BundleHash(bundle)
	if err != nil || bundleSHA != fixture.EnrichedBundleSHA256 {
		return chiDispatchPrepared{}, fmt.Errorf("chi request dispatch: frozen enriched bundle changed")
	}
	proposal := semanticdiscovery.OpportunityProposal{
		Version:    semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{projection.Candidate},
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(bundle, proposal); err != nil {
		return chiDispatchPrepared{}, err
	}
	if err := semanticdiscovery.ValidateLeafArtifact(projection.Leaf.Task, projection.Leaf.Artifact); err != nil {
		return chiDispatchPrepared{}, err
	}
	prompt, err := semanticdiscovery.BuildGoldenMechanismPrompt(bundle, projection.Leaf)
	if err != nil {
		return chiDispatchPrepared{}, err
	}
	promptRaw, err := marshalGoldenJSON(prompt)
	if err != nil || digestSHA256(promptRaw) != fixture.PromptSHA256 {
		return chiDispatchPrepared{}, fmt.Errorf("chi request dispatch: frozen prompt changed")
	}
	return chiDispatchPrepared{
		Loaded: loaded, Probe: probe, Projection: projection, Supplement: supplement,
		Bundle: bundle, Proposal: proposal, Leaf: projection.Leaf, Prompt: prompt,
		Fixture: fixture, FixtureHash: digestSHA256(fixtureRaw),
	}, nil
}

func reserveChiDispatchCall(path string, prepared chiDispatchPrepared) error {
	record := chiDispatchReservation{
		Version: 1, State: "reserved_before_provider_call",
		CandidateID:   prepared.Projection.Candidate.ID,
		FixtureSHA256: prepared.FixtureHash,
		PromptVersion: prepared.Prompt.Version,
		PromptSHA256:  prepared.Fixture.PromptSHA256,
		MaxCalls:      chiDispatchMaxModelCalls,
	}
	raw, err := marshalGoldenJSON(record)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("chi request dispatch: the single synthesis call is already reserved")
		}
		return err
	}
	_, writeErr := file.Write(raw)
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}

func saveChiDispatchResponse(
	runDir string,
	prepared chiDispatchPrepared,
	synthesis goldenMechanismSynthesis,
	synthesisErr error,
) error {
	validationStatus := "accepted"
	failureClass := ""
	if synthesisErr != nil {
		validationStatus = "rejected"
		failureClass = classifyGoldenMechanismValidationFailure(synthesis.Reduction)
	}
	return writeGoldenJSON(
		filepath.Join(runDir, chiDispatchResponseFile),
		goldenMechanismResponseAttempt{
			Version:          1,
			CandidateID:      prepared.Projection.Candidate.ID,
			PromptVersion:    semanticdiscovery.GoldenMechanismPromptVersion,
			ValidationStatus: validationStatus,
			FailureClass:     failureClass,
			Reduction:        &synthesis.Reduction,
			Content:          string(synthesis.RawResponse),
		},
	)
}

func publishChiDispatch(
	ctx context.Context,
	prepared chiDispatchPrepared,
	recordBytes []byte,
	want semanticdiscovery.Artifact,
) (mechanism semanticdiscovery.Mechanism, returnErr error) {
	supplementBytes, err := marshalGoldenJSON(prepared.Supplement)
	if err != nil {
		return mechanism, err
	}
	paths := []string{
		filepath.Join(prepared.Loaded.runDir, report.GoldenMechanismFactsFile),
		filepath.Join(prepared.Loaded.runDir, report.GoldenMechanismRecordFile),
		filepath.Join(prepared.Loaded.runDir, semanticdiscovery.MechanismFile),
		filepath.Join(prepared.Loaded.runDir, "report.json"),
		filepath.Join(prepared.Loaded.runDir, "report.html"),
		filepath.Join(prepared.Loaded.runDir, report.RunManifestFilename),
	}
	backups, err := backupGoldenFiles(paths)
	if err != nil {
		return mechanism, err
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if restoreErr := restoreGoldenFiles(backups); restoreErr != nil {
			returnErr = errors.Join(returnErr, restoreErr)
		}
	}()
	if err := writeAtomicFile(paths[0], supplementBytes, 0o600); err != nil {
		return mechanism, err
	}
	if err := writeAtomicFile(paths[1], recordBytes, 0o600); err != nil {
		return mechanism, err
	}
	extracted, artifact, err := report.ExtractMechanismV1(
		prepared.Loaded.runDir,
		prepared.Projection.Candidate.ID,
		chiDispatchIdentity,
	)
	if err != nil {
		return mechanism, err
	}
	if err := requireGoldenArtifact([]semanticdiscovery.Artifact{artifact}, want); err != nil {
		return mechanism, err
	}
	mechanismRaw, err := semanticdiscovery.EncodeMechanism(extracted)
	if err != nil {
		return mechanism, err
	}
	if err := writeAtomicFile(paths[2], mechanismRaw, 0o600); err != nil {
		return mechanism, err
	}

	data, err := report.ReadRunDir(prepared.Loaded.runDir)
	if err != nil {
		return mechanism, err
	}
	if err := verifyChiDispatchReport(data, prepared.Projection.Candidate.ID, artifact.ID); err != nil {
		return mechanism, err
	}
	current, err := freshness.CaptureRepository(ctx, prepared.Loaded.analysisRoot)
	if err != nil {
		return mechanism, err
	}
	if err := prepared.Loaded.manifest.VerifyRepositoryState(current); err != nil {
		return mechanism, err
	}
	authority, err := report.ConfirmRunAuthorityScoped(
		ctx,
		prepared.Loaded.analysisRoot,
		prepared.Loaded.manifest.RepositoryState,
		current,
		report.CapturedInputPaths(data),
		true,
	)
	if err != nil {
		return mechanism, err
	}
	if err := report.GenerateAuthorized(prepared.Loaded.runDir, authority); err != nil {
		return mechanism, err
	}
	finalData, err := report.ReadRunDir(prepared.Loaded.runDir)
	if err != nil {
		return mechanism, err
	}
	if err := verifyChiDispatchReport(finalData, prepared.Projection.Candidate.ID, artifact.ID); err != nil {
		return mechanism, err
	}
	committed = true
	return extracted, nil
}

func verifyChiDispatchReport(data *report.ReportData, candidateID, artifactID string) error {
	artifact, err := requirePublishedUserMechanism(data, artifactID, true)
	if err != nil {
		return fmt.Errorf("chi request dispatch: %w", err)
	}
	if artifact.CandidateID != candidateID {
		return fmt.Errorf("chi request dispatch: published candidate changed")
	}
	if len(artifact.Focus.ComponentIDs)+len(artifact.Focus.FlowIDs)+len(artifact.Focus.SurfaceIDs) == 0 {
		return fmt.Errorf("chi request dispatch: mechanism focus is empty")
	}
	paths := make(map[string]struct{})
	for _, step := range artifact.Steps {
		for _, reference := range step.Evidence {
			paths[reference.Path] = struct{}{}
		}
	}
	for _, required := range []string{"mux.go", "tree.go", "context.go"} {
		if _, exists := paths[required]; !exists {
			return fmt.Errorf("chi request dispatch: mechanism steps do not expose %s evidence", required)
		}
	}
	return nil
}

func replaySavedChiDispatchResponse(
	ctx context.Context,
	runDir string,
	stdout io.Writer,
	stderr io.Writer,
) (returnErr error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return err
	}
	status := chiDispatchResponseReplayStatus{
		Version:               1,
		State:                 "started",
		FixtureSHA256:         chiDispatchFixtureSHA256,
		ProjectionSHA256:      chiDispatchProjectionSHA256,
		SupplementSHA256:      chiDispatchSupplementSHA256,
		ResponseSHA256:        chiDispatchResponseSHA256,
		ResponseContentSHA256: chiDispatchContentSHA256,
	}
	defer func() {
		if returnErr != nil && status.State != "rejected" {
			status.State = "failed"
			if status.FailureClass == "" {
				status.FailureClass = "local_replay_failure"
			}
			if status.FailureReason == "" {
				status.FailureReason = semanticDiscoveryReason(returnErr.Error())
			}
		}
		if writeErr := writeGoldenJSON(
			filepath.Join(absDir, chiDispatchResponseReplayFile),
			status,
		); writeErr != nil {
			returnErr = errors.Join(returnErr, writeErr)
		}
	}()

	responseRaw, err := verifyFixedChiDispatchReplayInputs(absDir)
	if err != nil {
		status.FailureClass = "fixed_input_changed"
		return err
	}
	var saved goldenMechanismResponseAttempt
	if err := decodeChiDispatchJSON(responseRaw, &saved); err != nil {
		status.FailureClass = "saved_response_invalid"
		return err
	}
	if saved.Version != 1 || saved.CandidateID == "" ||
		saved.PromptVersion != semanticdiscovery.GoldenMechanismPromptVersion ||
		saved.ValidationStatus != "rejected" ||
		chiDispatchDigest([]byte(saved.Content)) != chiDispatchContentSHA256 {
		status.FailureClass = "saved_response_identity_changed"
		return fmt.Errorf("chi request dispatch: saved response identity changed")
	}

	prepared, err := loadPreparedChiDispatch(ctx, absDir)
	if err != nil {
		status.FailureClass = "frozen_fixture_invalid"
		return err
	}
	if saved.CandidateID != prepared.Projection.Candidate.ID ||
		prepared.FixtureHash != chiDispatchFixtureSHA256 {
		status.FailureClass = "saved_response_identity_changed"
		return fmt.Errorf("chi request dispatch: saved response no longer matches the frozen candidate")
	}

	fmt.Fprintln(
		stderr,
		"repomap: replaying the fixed chi response through local validators; no model, probe, or repository analyzer",
	)
	evaluated, evaluationErr := evaluateGoldenMechanismResponse(
		prepared.Bundle,
		prepared.Proposal,
		prepared.Leaf,
		[]byte(saved.Content),
	)
	status.Reduction = &evaluated.Reduction
	if evaluationErr != nil {
		status.State = "rejected"
		status.FailureClass = classifyGoldenMechanismValidationFailure(evaluated.Reduction)
		status.FailureReason = semanticDiscoveryReason(evaluationErr.Error())
		return evaluationErr
	}
	if err := validateChiDispatchDerivedVerdict(evaluated); err != nil {
		status.FailureClass = "local_verdict_invalid"
		return err
	}

	artifact := evaluated.Artifacts[0]
	summary, err := summarizeGoldenMechanismArtifact(prepared.Projection.Candidate, artifact)
	if err != nil {
		status.FailureClass = "fixed_rubric_failed"
		return err
	}
	status.Artifact = &summary
	if summary.LocalRubricScore < chiDispatchMinScore {
		status.FailureClass = "fixed_rubric_failed"
		return fmt.Errorf(
			"chi request dispatch: replayed artifact score %d is below %d",
			summary.LocalRubricScore,
			chiDispatchMinScore,
		)
	}

	mechanism, err := publishChiDispatch(
		ctx,
		prepared,
		append(append([]byte(nil), evaluated.RecordBytes...), '\n'),
		artifact,
	)
	if err != nil {
		status.FailureClass = "publication_failed"
		return err
	}
	if err := replayChiRequestDispatch(absDir, io.Discard); err != nil {
		status.FailureClass = "no_model_replay_failed"
		return err
	}

	status.State = "published_and_replayed"
	status.MechanismID = mechanism.ID
	status.ReportFile = filepath.Join(absDir, "report.html")
	fmt.Fprintf(
		stdout,
		"Saved-response replay: %s\nVerdict: %s\nMechanism: %s\nReport: %s\n",
		status.State,
		artifact.Verdict,
		mechanism.ID,
		status.ReportFile,
	)
	return nil
}

func verifyFixedChiDispatchReplayInputs(runDir string) ([]byte, error) {
	inputs := []struct {
		name string
		want string
	}{
		{name: chiDispatchFixtureFile, want: chiDispatchFixtureSHA256},
		{name: chiDispatchProjectionFile, want: chiDispatchProjectionSHA256},
		{name: chiDispatchSupplementFile, want: chiDispatchSupplementSHA256},
		{name: chiDispatchResponseFile, want: chiDispatchResponseSHA256},
	}
	var response []byte
	for _, input := range inputs {
		raw, err := readBoundedRegularFile(
			filepath.Join(runDir, input.name),
			maxGoldenSavedFileBytes,
		)
		if err != nil {
			return nil, err
		}
		if got := chiDispatchDigest(raw); got != input.want {
			return nil, fmt.Errorf(
				"chi request dispatch: fixed input %s changed: %s",
				input.name,
				got,
			)
		}
		if input.name == chiDispatchResponseFile {
			response = raw
		}
	}
	return response, nil
}

func validateChiDispatchDerivedVerdict(evaluated goldenMechanismSynthesis) error {
	if len(evaluated.Artifacts) != 1 ||
		evaluated.Artifacts[0].Verdict != semanticdiscovery.VerdictMixed ||
		len(evaluated.Reduction.VerdictDiagnostics) != 1 {
		return fmt.Errorf("chi request dispatch: fixed response did not derive one mixed artifact")
	}
	diagnostic := evaluated.Reduction.VerdictDiagnostics[0]
	wantReasons := []semanticdiscovery.VerdictReason{
		semanticdiscovery.VerdictReasonUnresolvedClaimPresent,
		semanticdiscovery.VerdictReasonMissingEvidenceRetained,
		semanticdiscovery.VerdictReasonRequiredAspectUncovered,
	}
	if diagnostic.Code != "model_verdict_mismatch" ||
		diagnostic.ModelVerdict != semanticdiscovery.VerdictSupported ||
		diagnostic.DerivedVerdict != semanticdiscovery.VerdictMixed ||
		!equalVerdictReasons(diagnostic.Reasons, wantReasons) {
		return fmt.Errorf("chi request dispatch: fixed response verdict diagnostic changed")
	}
	return nil
}

func equalVerdictReasons(left, right []semanticdiscovery.VerdictReason) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func replayChiRequestDispatch(runDir string, stdout io.Writer) (returnErr error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return err
	}
	status := chiDispatchReplayStatus{Version: 1, State: "started"}
	defer func() {
		if returnErr != nil {
			status.State = "failed"
		}
		if writeErr := writeGoldenJSON(filepath.Join(absDir, chiDispatchReplayFile), status); writeErr != nil {
			returnErr = errors.Join(returnErr, writeErr)
		}
	}()
	data, err := report.ReadRunDir(absDir)
	if err != nil {
		return err
	}
	mechanismRaw, err := readBoundedRegularFile(
		filepath.Join(absDir, semanticdiscovery.MechanismFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil {
		return err
	}
	mechanism, err := semanticdiscovery.DecodeMechanism(mechanismRaw)
	if err != nil {
		return err
	}
	if mechanism.Identity != chiDispatchIdentity {
		return fmt.Errorf("chi request dispatch: saved Mechanism identity changed")
	}
	var artifact semanticdiscovery.Artifact
	for _, item := range data.SemanticArtifacts {
		if item.CandidateID != mechanism.Payload.Candidate.ID {
			continue
		}
		if artifact.ID != "" {
			return fmt.Errorf("chi request dispatch: replay produced duplicate mechanism artifacts")
		}
		artifact = item
	}
	if artifact.ID == "" {
		return fmt.Errorf("chi request dispatch: replay did not materialize the saved mechanism")
	}
	artifactID := artifact.ID
	if err := verifyChiDispatchReport(data, mechanism.Payload.Candidate.ID, artifactID); err != nil {
		return err
	}
	html, err := report.RenderHTML(data)
	if err != nil {
		return err
	}
	status.State = "replayed"
	status.MechanismID = mechanism.ID
	status.ArtifactID = artifactID
	status.SearchIndexed = semanticSearchContainsArtifact(data, artifactID)
	status.HTMLRendered = bytes.Contains(html, []byte(artifactID))
	status.FocusNonEmpty = len(artifact.Focus.ComponentIDs)+
		len(artifact.Focus.FlowIDs)+
		len(artifact.Focus.SurfaceIDs) > 0
	paths := make(map[string]struct{})
	for _, step := range artifact.Steps {
		for _, reference := range step.Evidence {
			paths[reference.Path] = struct{}{}
		}
	}
	for path := range paths {
		status.EvidencePaths = append(status.EvidencePaths, path)
	}
	sort.Strings(status.EvidencePaths)
	if !status.SearchIndexed || !status.HTMLRendered || !status.FocusNonEmpty {
		return fmt.Errorf("chi request dispatch: no-model replay projection is incomplete")
	}
	fmt.Fprintf(
		stdout,
		"No-model replay: %s\nMechanism: %s\nEvidence: %s\n",
		status.State,
		status.MechanismID,
		strings.Join(status.EvidencePaths, ", "),
	)
	return nil
}

func chiDispatchDigest(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func decodeChiDispatchJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("chi request dispatch: trailing json")
	}
	return nil
}
