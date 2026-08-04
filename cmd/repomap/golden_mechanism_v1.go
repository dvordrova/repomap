package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

const (
	goldenMechanismV1ProbeFile          = "golden_mechanism_v1_probe.json"
	goldenMechanismV1ProjectionFile     = "golden_mechanism_v1_projection.json"
	goldenMechanismV1SupplementFile     = "golden_mechanism_v1_supplement.json"
	goldenMechanismV1FixtureFile        = "golden_mechanism_v1_fixture.json"
	goldenMechanismV1ReservationFile    = "golden_mechanism_v1_call_reservation.json"
	goldenMechanismV1ResponseFile       = "golden_mechanism_v1_response_attempt.json"
	goldenMechanismV1StatusFile         = "golden_mechanism_v1_status.json"
	goldenMechanismV1ReplayStatusFile   = "golden_mechanism_v1_replay_status.json"
	goldenDirectoryArtifactID           = "semantic-artifact-d432113d97c6d2f0f9d8e0db"
	goldenDirectoryArtifactSHA256       = "baa7fcaa8a1f08acc511fef64fc77c03847602413c99a20ef23e80222fe81df1"
	goldenMechanismV1MaxModelCalls      = 1
	goldenMechanismV1AcceptanceMinScore = 3
)

type goldenMechanismV1Mode string

const (
	goldenMechanismV1Live    goldenMechanismV1Mode = "live"
	goldenMechanismV1Prepare goldenMechanismV1Mode = "prepare"
	goldenMechanismV1Replay  goldenMechanismV1Mode = "replay"
)

type goldenMechanismV1QuestionCandidate struct {
	Question      string   `json:"question"`
	Boundary      string   `json:"boundary"`
	Files         int      `json:"estimated_files"`
	Functions     int      `json:"estimated_functions"`
	Packages      []string `json:"packages"`
	RelationKinds []string `json:"relation_kinds"`
	MainProofRisk string   `json:"main_proof_risk"`
	Chosen        bool     `json:"chosen"`
}

type goldenMechanismV1FactDigest struct {
	ID            string   `json:"id"`
	SHA256        string   `json:"sha256"`
	EvidenceIDs   []string `json:"evidence_ids"`
	Capabilities  []string `json:"capabilities"`
	AnswerAspects []string `json:"answer_aspects"`
}

type goldenMechanismV1Acceptance struct {
	MinLocalRubricScore           int      `json:"min_local_rubric_score"`
	MinCoveredAspects             int      `json:"min_covered_aspects"`
	MinKeyCoveredAspects          int      `json:"min_key_covered_aspects"`
	AllKeyAspectsNeedStrongClaims bool     `json:"all_key_aspects_need_direct_or_compositional_claims"`
	ClaimCountRange               []int    `json:"claim_count_range"`
	UncoveredAspectsNeedUnknowns  bool     `json:"uncovered_aspects_need_unknowns"`
	NoResponseSpecificChanges     bool     `json:"no_response_specific_changes"`
	StopConditions                []string `json:"stop_conditions"`
}

type goldenMechanismV1Fixture struct {
	Version                int                                  `json:"version"`
	State                  string                               `json:"state"`
	RepositoryRevision     string                               `json:"repository_revision"`
	CandidateID            string                               `json:"candidate_id"`
	Title                  string                               `json:"title"`
	ExactQuestion          string                               `json:"exact_question"`
	Boundary               string                               `json:"boundary"`
	Excluded               []string                             `json:"excluded"`
	QuestionCandidates     []goldenMechanismV1QuestionCandidate `json:"question_candidates"`
	Aspects                []goldenCaddyfileAspectDefinition    `json:"aspects"`
	ProbePlan              goldenmechanism.Plan                 `json:"probe_plan"`
	ProbeSHA256            string                               `json:"probe_sha256"`
	ProjectionSHA256       string                               `json:"projection_sha256"`
	SupplementSHA256       string                               `json:"supplement_sha256"`
	EnrichedBundleSHA256   string                               `json:"enriched_bundle_sha256"`
	FactDigests            []goldenMechanismV1FactDigest        `json:"fact_digests"`
	EvidenceIDs            []string                             `json:"evidence_ids"`
	AvailableCapabilities  []semanticdiscovery.Capability       `json:"available_capabilities"`
	MissingCapabilities    []semanticdiscovery.Capability       `json:"missing_capabilities"`
	Acceptance             goldenMechanismV1Acceptance          `json:"acceptance"`
	MaxModelCalls          int                                  `json:"max_model_calls"`
	PromptVersion          string                               `json:"prompt_version"`
	PromptSHA256           string                               `json:"prompt_sha256"`
	ExistingArtifactID     string                               `json:"existing_directory_artifact_id"`
	ExistingArtifactSHA256 string                               `json:"existing_directory_artifact_sha256"`
}

type goldenMechanismV1Status struct {
	Version                    int                                        `json:"version"`
	State                      string                                     `json:"state"`
	FailureClass               string                                     `json:"failure_class,omitempty"`
	FailureReason              string                                     `json:"failure_reason,omitempty"`
	CandidateID                string                                     `json:"candidate_id"`
	Question                   string                                     `json:"question"`
	RepositoryRevision         string                                     `json:"repository_revision,omitempty"`
	FixtureSHA256              string                                     `json:"fixture_sha256,omitempty"`
	PromptVersion              string                                     `json:"prompt_version,omitempty"`
	PromptSHA256               string                                     `json:"prompt_sha256,omitempty"`
	ProbeCalls                 int                                        `json:"targeted_probe_calls"`
	RepositoryAnalyzers        int                                        `json:"repository_analyzers"`
	ModelCalls                 int                                        `json:"model_calls"`
	ProbeBudget                *goldenmechanism.BudgetStats               `json:"probe_budget,omitempty"`
	Synthesis                  *semanticDiscoveryStageMetrics             `json:"synthesis,omitempty"`
	Reduction                  *semanticdiscovery.FanInReductionReport    `json:"reduction,omitempty"`
	ClaimCoverage              *semanticdiscovery.ClaimCoverageAssessment `json:"claim_coverage,omitempty"`
	Artifacts                  []goldenMechanismArtifactSummary           `json:"artifacts,omitempty"`
	DirectoryArtifactPreserved bool                                       `json:"directory_artifact_preserved"`
	CanonicalFactsFile         string                                     `json:"canonical_facts_file,omitempty"`
	CanonicalRecordFile        string                                     `json:"canonical_record_file,omitempty"`
	ReportFile                 string                                     `json:"report_file,omitempty"`
}

type goldenMechanismV1ReplayStatus struct {
	Version                    int      `json:"version"`
	State                      string   `json:"state"`
	ModelCalls                 int      `json:"model_calls"`
	RepositoryAnalyzers        int      `json:"repository_analyzers"`
	TargetedProbeCalls         int      `json:"targeted_probe_calls"`
	ArtifactIDs                []string `json:"artifact_ids"`
	ArtifactSHA256             []string `json:"artifact_sha256"`
	DirectoryArtifactPreserved bool     `json:"directory_artifact_preserved"`
	SearchIndexed              bool     `json:"search_indexed"`
	HTMLContainsArtifacts      bool     `json:"html_contains_artifacts"`
	EvidenceCount              int      `json:"evidence_count"`
	ReportFile                 string   `json:"report_file"`
}

type goldenMechanismV1Reservation struct {
	Version       int    `json:"version"`
	State         string `json:"state"`
	CandidateID   string `json:"candidate_id"`
	FixtureSHA256 string `json:"fixture_sha256"`
	PromptVersion string `json:"prompt_version"`
	PromptSHA256  string `json:"prompt_sha256"`
	MaxCalls      int    `json:"max_calls"`
}

type goldenMechanismV1Existing struct {
	Supplement report.SemanticSupplement
	ProbeSHA   string
	Facts      []semanticdiscovery.Fact
	Record     semanticdiscovery.Record
	Candidate  semanticdiscovery.OpportunityCandidate
	Leaf       semanticdiscovery.LeafResult
	Proposal   semanticdiscovery.ArtifactProposal
	Artifact   semanticdiscovery.Artifact
}

type goldenMechanismV1Input struct {
	Loaded      goldenMechanismRun
	Existing    goldenMechanismV1Existing
	Probe       goldenmechanism.Result
	Projection  goldenProjection
	Supplement  report.SemanticSupplement
	Bundle      semanticdiscovery.Bundle
	Proposal    semanticdiscovery.OpportunityProposal
	Leaf        semanticdiscovery.LeafResult
	Prompt      semanticdiscovery.Prompt
	Fixture     goldenMechanismV1Fixture
	FixtureHash string
}

func runGoldenMechanismV1CLI(args []string, stdout, stderr io.Writer) error {
	runDir, mode, err := parseGoldenMechanismV1Args(args)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if mode == goldenMechanismV1Replay {
		return replayGoldenMechanismV1(ctx, runDir, stdout, stderr)
	}
	return runGoldenMechanismV1(ctx, runDir, mode, stdout, stderr)
}

func parseGoldenMechanismV1Args(args []string) (string, goldenMechanismV1Mode, error) {
	runDir := ""
	mode := goldenMechanismV1Live
	for _, arg := range args {
		switch arg {
		case "--prepare":
			if mode != goldenMechanismV1Live {
				return "", "", goldenMechanismV1Usage()
			}
			mode = goldenMechanismV1Prepare
		case "--replay":
			if mode != goldenMechanismV1Live {
				return "", "", goldenMechanismV1Usage()
			}
			mode = goldenMechanismV1Replay
		default:
			if strings.HasPrefix(arg, "-") || runDir != "" {
				return "", "", goldenMechanismV1Usage()
			}
			runDir = arg
		}
	}
	if runDir == "" {
		return "", "", goldenMechanismV1Usage()
	}
	return runDir, mode, nil
}

func goldenMechanismV1Usage() error {
	return fmt.Errorf(
		"Usage: repomap dev golden-mechanism-v1 <run-dir> [--prepare | --replay]",
	)
}

func runGoldenMechanismV1(
	ctx context.Context,
	runDir string,
	mode goldenMechanismV1Mode,
	stdout io.Writer,
	stderr io.Writer,
) (returnErr error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return err
	}
	status := goldenMechanismV1Status{
		Version: 1, State: "started",
		CandidateID: goldenCaddyfileErrorCandidateID,
		Question:    goldenCaddyfileErrorQuestion,
	}
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
		if err := writeGoldenJSON(filepath.Join(absDir, goldenMechanismV1StatusFile), status); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()

	fixturePath := filepath.Join(absDir, goldenMechanismV1FixtureFile)
	if mode == goldenMechanismV1Prepare {
		if _, err := os.Lstat(fixturePath); err == nil {
			input, loadErr := loadGoldenMechanismV1Input(ctx, absDir)
			if loadErr != nil {
				status.FailureClass = "frozen_fixture_invalid"
				return loadErr
			}
			status.State = "fixture_already_frozen"
			status.RepositoryRevision = input.Loaded.manifest.RepositoryState.Head
			status.FixtureSHA256 = input.FixtureHash
			status.PromptVersion = input.Prompt.Version
			status.PromptSHA256 = input.Fixture.PromptSHA256
			fmt.Fprintf(stdout, "Frozen fixture: %s\n", fixturePath)
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		input, actualBudget, prepareErr := prepareGoldenMechanismV1(ctx, absDir)
		if prepareErr != nil {
			status.FailureClass = "fixture_preparation_failed"
			return prepareErr
		}
		status.State = "fixture_frozen"
		status.RepositoryRevision = input.Loaded.manifest.RepositoryState.Head
		status.FixtureSHA256 = input.FixtureHash
		status.PromptVersion = input.Prompt.Version
		status.PromptSHA256 = input.Fixture.PromptSHA256
		status.ProbeCalls = 1
		status.ProbeBudget = &actualBudget
		fmt.Fprintf(
			stderr,
			"repomap: Golden Mechanism v1 froze %d facts from %d files and %d functions; no model call\n",
			len(input.Projection.Facts),
			actualBudget.FilesParsed,
			actualBudget.FunctionsIncluded,
		)
		fmt.Fprintf(stdout, "Frozen fixture: %s\nProjection: %s\n", fixturePath, filepath.Join(absDir, goldenMechanismV1ProjectionFile))
		return nil
	}

	input, err := loadGoldenMechanismV1Input(ctx, absDir)
	if err != nil {
		status.FailureClass = "frozen_fixture_invalid"
		return err
	}
	status.RepositoryRevision = input.Loaded.manifest.RepositoryState.Head
	status.FixtureSHA256 = input.FixtureHash
	status.PromptVersion = input.Prompt.Version
	status.PromptSHA256 = input.Fixture.PromptSHA256
	client, err := deepseek.NewFromEnv()
	if err != nil {
		status.FailureClass = "provider_unavailable"
		return fmt.Errorf("golden mechanism v1: provider configuration: %w", err)
	}
	client.OnWait = func(progress deepseek.WaitProgress) {
		fmt.Fprintf(
			stderr,
			"repomap: %s still running after %s (Ctrl-C to cancel)\n",
			progress.Stage,
			progress.Elapsed.Round(time.Second),
		)
	}
	reservationPath := filepath.Join(absDir, goldenMechanismV1ReservationFile)
	if err := reserveGoldenMechanismV1Call(reservationPath, input); err != nil {
		status.FailureClass = "single_call_already_reserved"
		return err
	}
	fmt.Fprintln(stderr, "repomap: running the one allowed cold synthesis call over the frozen Caddyfile error fixture")
	counted := &countingSemanticDiscoveryEditor{delegate: client}
	synthesis, synthesisErr := executeGoldenMechanismSynthesis(
		ctx,
		input.Bundle,
		input.Proposal,
		input.Leaf,
		counted,
	)
	status.ModelCalls = counted.calls
	status.Synthesis = &synthesis.Metrics
	status.Reduction = &synthesis.Reduction
	if saveErr := saveGoldenMechanismV1Response(absDir, synthesis, synthesisErr); saveErr != nil {
		status.FailureClass = "response_artifact_write_failed"
		return errors.Join(synthesisErr, saveErr)
	}
	if counted.calls != goldenMechanismV1MaxModelCalls {
		status.FailureClass = "single_call_count_invalid"
		return errors.Join(
			synthesisErr,
			fmt.Errorf("golden mechanism v1: provider calls = %d, want exactly one", counted.calls),
		)
	}
	if synthesisErr != nil {
		status.State = "rejected"
		status.FailureClass = classifyGoldenMechanismValidationFailure(synthesis.Reduction)
		status.FailureReason = semanticDiscoveryReason(synthesisErr.Error())
		return synthesisErr
	}
	artifact := synthesis.Artifacts[0]
	assessment, err := semanticdiscovery.AssessClaimCoverage(
		input.Bundle,
		[]semanticdiscovery.LeafResult{input.Leaf},
		synthesis.FanIn.Artifacts[0],
	)
	if err != nil {
		status.FailureClass = "claim_coverage_failed"
		return err
	}
	status.ClaimCoverage = &assessment
	summary, err := summarizeGoldenMechanismArtifact(input.Projection.Candidate, artifact)
	if err != nil {
		status.FailureClass = "golden_rubric_failed"
		return err
	}
	if summary.LocalRubricScore < goldenMechanismV1AcceptanceMinScore {
		status.FailureClass = "golden_rubric_failed"
		return fmt.Errorf("golden mechanism v1: artifact score %d is below %d", summary.LocalRubricScore, goldenMechanismV1AcceptanceMinScore)
	}
	recordBytes, artifacts, err := combineGoldenMechanismV1Record(input, synthesis.FanIn)
	if err != nil {
		status.FailureClass = "combined_record_invalid"
		return err
	}
	supplementBytes, err := marshalGoldenJSON(input.Supplement)
	if err != nil {
		return err
	}
	if err := publishGoldenMechanismArtifacts(
		ctx,
		input.Loaded,
		supplementBytes,
		append(recordBytes, '\n'),
		artifacts,
	); err != nil {
		status.FailureClass = "combined_publication_failed"
		return err
	}
	status.State = "published"
	status.Artifacts = make([]goldenMechanismArtifactSummary, 0, len(artifacts))
	for _, item := range artifacts {
		candidate := input.Existing.Candidate
		if item.CandidateID == input.Projection.Candidate.ID {
			candidate = input.Projection.Candidate
		}
		itemSummary, summarizeErr := summarizeGoldenMechanismArtifact(candidate, item)
		if summarizeErr != nil {
			return summarizeErr
		}
		status.Artifacts = append(status.Artifacts, itemSummary)
	}
	status.DirectoryArtifactPreserved = true
	status.CanonicalFactsFile = filepath.Join(absDir, report.GoldenMechanismFactsFile)
	status.CanonicalRecordFile = filepath.Join(absDir, report.GoldenMechanismRecordFile)
	status.ReportFile = filepath.Join(absDir, "report.html")
	fmt.Fprintf(
		stderr,
		"repomap: cold artifact accepted with %d supported steps, %d covered aspects, score %d/4; directory artifact preserved\n",
		summary.SupportedSteps,
		len(summary.CoveredAnswerAspects),
		summary.LocalRubricScore,
	)
	fmt.Fprintf(stdout, "Golden mechanisms: %s\nReport: %s\n", status.CanonicalRecordFile, status.ReportFile)
	return nil
}

func prepareGoldenMechanismV1(
	ctx context.Context,
	runDir string,
) (goldenMechanismV1Input, goldenmechanism.BudgetStats, error) {
	loaded, err := loadGoldenMechanismRun(ctx, runDir)
	if err != nil {
		return goldenMechanismV1Input{}, goldenmechanism.BudgetStats{}, err
	}
	existing, err := loadGoldenMechanismV1Existing(loaded)
	if err != nil {
		return goldenMechanismV1Input{}, goldenmechanism.BudgetStats{}, err
	}
	original, err := goldenMechanismV1BaseCandidate(loaded.record)
	if err != nil {
		return goldenMechanismV1Input{}, goldenmechanism.BudgetStats{}, err
	}
	plan, err := caddyfileErrorPlan(loaded.bundle, loaded.data)
	if err != nil {
		return goldenMechanismV1Input{}, goldenmechanism.BudgetStats{}, err
	}
	probe, err := goldenmechanism.Probe(ctx, loaded.analysisRoot, plan)
	if err != nil {
		return goldenMechanismV1Input{}, goldenmechanism.BudgetStats{}, err
	}
	actualBudget := probe.Budget
	if probe.Partial {
		return goldenMechanismV1Input{}, actualBudget, fmt.Errorf(
			"golden mechanism v1: bounded probe is partial (%s)",
			probe.StopReason,
		)
	}
	stableProbe := probe
	stableProbe.Budget.ElapsedMillis = 0
	probeBytes, err := marshalGoldenJSON(stableProbe)
	if err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	if err := ensureGoldenMechanismV1Safe("probe", probeBytes); err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	probeSHA := digestSHA256(probeBytes)
	if err := writeFrozenGoldenFile(
		filepath.Join(runDir, goldenMechanismV1ProbeFile),
		probeBytes,
	); err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}

	projection, err := projectCaddyfileError(loaded.bundle, original, stableProbe)
	if err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	combinedFacts := append(
		append([]semanticdiscovery.Fact(nil), existing.Facts...),
		projection.Facts...,
	)
	bindings := []report.SemanticSupplementCandidateBinding{
		{
			CandidateID: goldenDirectoryListingCandidateID,
			ProbeSHA256: existing.ProbeSHA,
			FactIDs:     goldenFactIDs(existing.Facts),
		},
		{
			CandidateID: goldenCaddyfileErrorCandidateID,
			ProbeSHA256: probeSHA,
			FactIDs:     goldenFactIDs(projection.Facts),
		},
	}
	supplement, bundle, err := report.PrepareSemanticSupplementSet(
		loaded.data,
		bindings,
		combinedFacts,
	)
	if err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	proposal := semanticdiscovery.OpportunityProposal{
		Version:    semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{projection.Candidate},
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(bundle, proposal); err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	leaf, err := buildCaddyfileErrorLeaf(bundle, projection.Candidate)
	if err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	projection.Leaf = leaf
	projectionBytes, err := marshalGoldenJSON(projection)
	if err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	if err := ensureGoldenMechanismV1Safe("projection", projectionBytes); err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	projectionSHA := digestSHA256(projectionBytes)
	if err := writeFrozenGoldenFile(
		filepath.Join(runDir, goldenMechanismV1ProjectionFile),
		projectionBytes,
	); err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	supplementBytes, err := marshalGoldenJSON(supplement)
	if err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	if err := writeFrozenGoldenFile(
		filepath.Join(runDir, goldenMechanismV1SupplementFile),
		supplementBytes,
	); err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	prompt, err := semanticdiscovery.BuildGoldenMechanismPrompt(bundle, leaf)
	if err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	promptBytes, err := marshalGoldenJSON(prompt)
	if err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	bundleSHA, _, err := semanticdiscovery.BundleHash(bundle)
	if err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	fixture, err := buildGoldenMechanismV1Fixture(
		loaded,
		existing,
		plan,
		projection,
		probeSHA,
		projectionSHA,
		digestSHA256(supplementBytes),
		bundleSHA,
		prompt,
		digestSHA256(promptBytes),
	)
	if err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	fixtureBytes, err := marshalGoldenJSON(fixture)
	if err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	if err := ensureGoldenMechanismV1Safe("fixture", fixtureBytes); err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	if err := writeFrozenGoldenFile(
		filepath.Join(runDir, goldenMechanismV1FixtureFile),
		fixtureBytes,
	); err != nil {
		return goldenMechanismV1Input{}, actualBudget, err
	}
	return goldenMechanismV1Input{
		Loaded: loaded, Existing: existing, Probe: stableProbe,
		Projection: projection, Supplement: supplement, Bundle: bundle,
		Proposal: proposal, Leaf: leaf, Prompt: prompt,
		Fixture: fixture, FixtureHash: digestSHA256(fixtureBytes),
	}, actualBudget, nil
}

func loadGoldenMechanismV1Input(
	ctx context.Context,
	runDir string,
) (goldenMechanismV1Input, error) {
	loaded, err := loadGoldenMechanismRun(ctx, runDir)
	if err != nil {
		return goldenMechanismV1Input{}, err
	}
	existing, err := loadGoldenMechanismV1Existing(loaded)
	if err != nil {
		return goldenMechanismV1Input{}, err
	}
	fixtureRaw, err := readBoundedRegularFile(
		filepath.Join(runDir, goldenMechanismV1FixtureFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil {
		return goldenMechanismV1Input{}, err
	}
	var fixture goldenMechanismV1Fixture
	if err := decodeGoldenFixture(fixtureRaw, &fixture); err != nil {
		return goldenMechanismV1Input{}, err
	}
	if err := validateGoldenMechanismV1FixtureIdentity(fixture, loaded, existing); err != nil {
		return goldenMechanismV1Input{}, err
	}
	probeRaw, err := readBoundedRegularFile(
		filepath.Join(runDir, goldenMechanismV1ProbeFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil {
		return goldenMechanismV1Input{}, err
	}
	if digestSHA256(probeRaw) != fixture.ProbeSHA256 {
		return goldenMechanismV1Input{}, fmt.Errorf("golden mechanism v1: frozen probe hash changed")
	}
	var probe goldenmechanism.Result
	if err := decodeGoldenFixture(probeRaw, &probe); err != nil {
		return goldenMechanismV1Input{}, err
	}
	if err := probe.Validate(); err != nil || probe.Partial {
		return goldenMechanismV1Input{}, fmt.Errorf("golden mechanism v1: frozen probe is invalid or partial: %w", err)
	}
	projectionRaw, err := readBoundedRegularFile(
		filepath.Join(runDir, goldenMechanismV1ProjectionFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil {
		return goldenMechanismV1Input{}, err
	}
	if digestSHA256(projectionRaw) != fixture.ProjectionSHA256 {
		return goldenMechanismV1Input{}, fmt.Errorf("golden mechanism v1: frozen projection hash changed")
	}
	var projection goldenProjection
	if err := decodeGoldenFixture(projectionRaw, &projection); err != nil {
		return goldenMechanismV1Input{}, err
	}
	if err := validateGoldenMechanismV1Projection(fixture, projection); err != nil {
		return goldenMechanismV1Input{}, err
	}
	combinedFacts := append(
		append([]semanticdiscovery.Fact(nil), existing.Facts...),
		projection.Facts...,
	)
	bindings := []report.SemanticSupplementCandidateBinding{
		{
			CandidateID: goldenDirectoryListingCandidateID,
			ProbeSHA256: existing.ProbeSHA,
			FactIDs:     goldenFactIDs(existing.Facts),
		},
		{
			CandidateID: goldenCaddyfileErrorCandidateID,
			ProbeSHA256: fixture.ProbeSHA256,
			FactIDs:     goldenFactIDs(projection.Facts),
		},
	}
	supplement, bundle, err := report.PrepareSemanticSupplementSet(
		loaded.data,
		bindings,
		combinedFacts,
	)
	if err != nil {
		return goldenMechanismV1Input{}, err
	}
	supplementBytes, err := marshalGoldenJSON(supplement)
	if err != nil {
		return goldenMechanismV1Input{}, err
	}
	frozenSupplement, err := readBoundedRegularFile(
		filepath.Join(runDir, goldenMechanismV1SupplementFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil {
		return goldenMechanismV1Input{}, err
	}
	if digestSHA256(supplementBytes) != fixture.SupplementSHA256 ||
		!bytes.Equal(supplementBytes, frozenSupplement) {
		return goldenMechanismV1Input{}, fmt.Errorf("golden mechanism v1: frozen supplement changed")
	}
	bundleSHA, _, err := semanticdiscovery.BundleHash(bundle)
	if err != nil || bundleSHA != fixture.EnrichedBundleSHA256 {
		return goldenMechanismV1Input{}, fmt.Errorf("golden mechanism v1: frozen bundle changed")
	}
	proposal := semanticdiscovery.OpportunityProposal{
		Version:    semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{projection.Candidate},
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(bundle, proposal); err != nil {
		return goldenMechanismV1Input{}, err
	}
	leaf, err := buildCaddyfileErrorLeaf(bundle, projection.Candidate)
	if err != nil {
		return goldenMechanismV1Input{}, err
	}
	leafBytes, err := marshalGoldenJSON(leaf)
	if err != nil {
		return goldenMechanismV1Input{}, err
	}
	frozenLeafBytes, err := marshalGoldenJSON(projection.Leaf)
	if err != nil {
		return goldenMechanismV1Input{}, err
	}
	if !bytes.Equal(leafBytes, frozenLeafBytes) {
		return goldenMechanismV1Input{}, fmt.Errorf("golden mechanism v1: frozen leaf changed")
	}
	prompt, err := semanticdiscovery.BuildGoldenMechanismPrompt(bundle, leaf)
	if err != nil {
		return goldenMechanismV1Input{}, err
	}
	promptBytes, err := marshalGoldenJSON(prompt)
	if err != nil {
		return goldenMechanismV1Input{}, err
	}
	if prompt.Version != fixture.PromptVersion || digestSHA256(promptBytes) != fixture.PromptSHA256 {
		return goldenMechanismV1Input{}, fmt.Errorf("golden mechanism v1: frozen prompt changed")
	}
	return goldenMechanismV1Input{
		Loaded: loaded, Existing: existing, Probe: probe,
		Projection: projection, Supplement: supplement, Bundle: bundle,
		Proposal: proposal, Leaf: leaf, Prompt: prompt,
		Fixture: fixture, FixtureHash: digestSHA256(fixtureRaw),
	}, nil
}

func loadGoldenMechanismV1Existing(
	loaded goldenMechanismRun,
) (goldenMechanismV1Existing, error) {
	factsRaw, err := readBoundedRegularFile(
		filepath.Join(loaded.runDir, report.GoldenMechanismFactsFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil {
		return goldenMechanismV1Existing{}, err
	}
	var supplement report.SemanticSupplement
	if err := decodeGoldenFixture(factsRaw, &supplement); err != nil {
		return goldenMechanismV1Existing{}, err
	}
	if supplement.Version != 1 ||
		supplement.CandidateID != goldenDirectoryListingCandidateID ||
		supplement.ProbeSHA256 == "" ||
		len(supplement.Facts) == 0 {
		return goldenMechanismV1Existing{}, fmt.Errorf(
			"golden mechanism v1: expected the pre-v1 single-candidate canonical supplement",
		)
	}
	recordRaw, err := readBoundedRegularFile(
		filepath.Join(loaded.runDir, report.GoldenMechanismRecordFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil {
		return goldenMechanismV1Existing{}, err
	}
	record, err := semanticdiscovery.DecodeRecord(recordRaw)
	if err != nil {
		return goldenMechanismV1Existing{}, err
	}
	result := goldenMechanismV1Existing{
		Supplement: supplement,
		ProbeSHA:   supplement.ProbeSHA256,
		Facts:      append([]semanticdiscovery.Fact(nil), supplement.Facts...),
		Record:     record,
	}
	for _, candidate := range record.Opportunity.Candidates {
		if candidate.ID == goldenDirectoryListingCandidateID {
			result.Candidate = candidate
			break
		}
	}
	for _, leaf := range record.Leaves {
		if leaf.Task.Candidate.ID == goldenDirectoryListingCandidateID {
			result.Leaf = leaf
			break
		}
	}
	for _, proposal := range record.FanIn.Artifacts {
		if proposal.CandidateID == goldenDirectoryListingCandidateID {
			result.Proposal = proposal
			break
		}
	}
	for _, artifact := range loaded.data.SemanticArtifacts {
		if artifact.CandidateID == goldenDirectoryListingCandidateID {
			result.Artifact = artifact
			break
		}
	}
	if result.Candidate.ID == "" || result.Leaf.Task.ID == "" ||
		result.Proposal.CandidateID == "" || result.Artifact.ID == "" {
		return goldenMechanismV1Existing{}, fmt.Errorf(
			"golden mechanism v1: existing directory-listing canonical record is incomplete",
		)
	}
	if result.Artifact.ID != goldenDirectoryArtifactID {
		return goldenMechanismV1Existing{}, fmt.Errorf(
			"golden mechanism v1: existing directory artifact id changed",
		)
	}
	digest, err := goldenMechanismArtifactSHA256(result.Artifact)
	if err != nil {
		return goldenMechanismV1Existing{}, err
	}
	if digest != goldenDirectoryArtifactSHA256 {
		return goldenMechanismV1Existing{}, fmt.Errorf(
			"golden mechanism v1: existing directory artifact hash changed",
		)
	}
	return result, nil
}

func goldenMechanismV1BaseCandidate(
	record semanticdiscovery.Record,
) (semanticdiscovery.OpportunityCandidate, error) {
	for _, candidate := range record.Opportunity.Candidates {
		if candidate.ID == goldenCaddyfileBaseCandidateID {
			if candidate.Kind != semanticdiscovery.ArtifactMechanism {
				break
			}
			return candidate, nil
		}
	}
	return semanticdiscovery.OpportunityCandidate{}, fmt.Errorf(
		"golden mechanism v1: saved Caddyfile error candidate is unavailable",
	)
}

func buildGoldenMechanismV1Fixture(
	loaded goldenMechanismRun,
	existing goldenMechanismV1Existing,
	plan goldenmechanism.Plan,
	projection goldenProjection,
	probeSHA string,
	projectionSHA string,
	supplementSHA string,
	bundleSHA string,
	prompt semanticdiscovery.Prompt,
	promptSHA string,
) (goldenMechanismV1Fixture, error) {
	if projection.Candidate.CapabilityContract == nil || projection.Candidate.IntentContract == nil {
		return goldenMechanismV1Fixture{}, fmt.Errorf("golden mechanism v1: projected contracts are required")
	}
	factDigests, evidenceIDs, err := goldenMechanismV1FactDigests(projection.Facts)
	if err != nil {
		return goldenMechanismV1Fixture{}, err
	}
	return goldenMechanismV1Fixture{
		Version:            1,
		State:              "frozen_before_model_response",
		RepositoryRevision: loaded.manifest.RepositoryState.Head,
		CandidateID:        goldenCaddyfileErrorCandidateID,
		Title:              goldenCaddyfileErrorTitle,
		ExactQuestion:      goldenCaddyfileErrorQuestion,
		Boundary: "One concrete top-level request-matcher parser error, its file/line wrapping, " +
			"the bounded parser return chain, and the built-in adapter's early parse-error return.",
		Excluded: []string{
			"CLI presentation and process exit",
			"lexer and heredoc errors",
			"generic SyntaxErr behavior and every Caddyfile error family",
			"server-type setup internals after successful parsing",
		},
		QuestionCandidates:   goldenMechanismV1QuestionCandidates(),
		Aspects:              goldenCaddyfileErrorAspectDefinitions(),
		ProbePlan:            plan,
		ProbeSHA256:          probeSHA,
		ProjectionSHA256:     projectionSHA,
		SupplementSHA256:     supplementSHA,
		EnrichedBundleSHA256: bundleSHA,
		FactDigests:          factDigests,
		EvidenceIDs:          evidenceIDs,
		AvailableCapabilities: append(
			[]semanticdiscovery.Capability(nil),
			projection.Candidate.CapabilityContract.AvailableCapabilities...,
		),
		MissingCapabilities: append(
			[]semanticdiscovery.Capability(nil),
			projection.Candidate.CapabilityContract.MissingCapabilities...,
		),
		Acceptance: goldenMechanismV1Acceptance{
			MinLocalRubricScore:           goldenMechanismV1AcceptanceMinScore,
			MinCoveredAspects:             projection.Candidate.IntentContract.MinCovered,
			MinKeyCoveredAspects:          projection.Candidate.IntentContract.MinKeyCovered,
			AllKeyAspectsNeedStrongClaims: true,
			ClaimCountRange:               []int{3, 7},
			UncoveredAspectsNeedUnknowns:  true,
			NoResponseSpecificChanges:     true,
			StopConditions: []string{
				"Stop before synthesis if the bounded probe or frozen fixture is incomplete.",
				"After one provider attempt, reject and stop on provider, JSON, lineage, support, capability, temporal, aspect, unknown, intent, or rubric failure.",
				"Do not retry, expand evidence, edit prose, or change the fixture after observing the response.",
			},
		},
		MaxModelCalls:          goldenMechanismV1MaxModelCalls,
		PromptVersion:          prompt.Version,
		PromptSHA256:           promptSHA,
		ExistingArtifactID:     existing.Artifact.ID,
		ExistingArtifactSHA256: goldenDirectoryArtifactSHA256,
	}, nil
}

func goldenMechanismV1QuestionCandidates() []goldenMechanismV1QuestionCandidate {
	return []goldenMechanismV1QuestionCandidate{
		{
			Question:  "How are Caddyfile parse errors enriched with source location and context?",
			Boundary:  "Selected parser-helper error construction and wrapping.",
			Files:     3,
			Functions: 8,
			Packages:  []string{"caddyfile"},
			RelationKinds: []string{
				"direct_call", "error_return", "error_wrap", "context_attachment", "test_evidence",
			},
			MainProofRisk: "Universal wording would incorrectly include lexer and generic syntax-error paths that build context differently.",
		},
		{
			Question:  goldenCaddyfileErrorQuestion,
			Boundary:  "Selected matcher error through parser entry functions and built-in adapter return.",
			Files:     4,
			Functions: 10,
			Packages:  []string{"caddyfile"},
			RelationKinds: []string{
				"direct_call", "local_error_handoff", "error_wrap", "branch", "test_evidence", "limitation",
			},
			MainProofRisk: "Returning an identifier is insufficient unless the probe proves the same local call-result binding is checked and returned.",
			Chosen:        true,
		},
		{
			Question:  "How does a Caddyfile syntax error become a user-visible CLI error?",
			Boundary:  "Adapter registration, command selection, callback wrapper, logger, and terminal output.",
			Files:     8,
			Functions: 14,
			Packages:  []string{"caddyfile", "httpcaddyfile", "caddyconfig", "caddycmd"},
			RelationKinds: []string{
				"registry_binding", "interface_dispatch", "closure_binding", "logger_output", "stderr_sink",
			},
			MainProofRisk: "The current bounded probe does not resolve registry/interface dispatch, returned closures, or logger-to-stderr globals without several new analyzers.",
		},
	}
}

func goldenMechanismV1FactDigests(
	facts []semanticdiscovery.Fact,
) ([]goldenMechanismV1FactDigest, []string, error) {
	result := make([]goldenMechanismV1FactDigest, 0, len(facts))
	allEvidence := make([]string, 0)
	for _, fact := range facts {
		raw, err := json.Marshal(fact)
		if err != nil {
			return nil, nil, err
		}
		item := goldenMechanismV1FactDigest{ID: fact.ID, SHA256: digestSHA256(raw)}
		for _, reference := range fact.Evidence {
			item.EvidenceIDs = append(item.EvidenceIDs, reference.ID)
			allEvidence = append(allEvidence, reference.ID)
		}
		for _, capability := range fact.Capabilities {
			item.Capabilities = append(item.Capabilities, string(capability))
		}
		for _, keyword := range fact.Keywords {
			if strings.HasPrefix(keyword, "answer_aspect:") {
				item.AnswerAspects = append(
					item.AnswerAspects,
					strings.TrimPrefix(keyword, "answer_aspect:"),
				)
			}
		}
		sort.Strings(item.EvidenceIDs)
		sort.Strings(item.Capabilities)
		sort.Strings(item.AnswerAspects)
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, sortedGoldenStrings(allEvidence), nil
}

func validateGoldenMechanismV1FixtureIdentity(
	fixture goldenMechanismV1Fixture,
	loaded goldenMechanismRun,
	existing goldenMechanismV1Existing,
) error {
	if fixture.Version != 1 || fixture.State != "frozen_before_model_response" ||
		fixture.RepositoryRevision != loaded.manifest.RepositoryState.Head ||
		fixture.CandidateID != goldenCaddyfileErrorCandidateID ||
		fixture.Title != goldenCaddyfileErrorTitle ||
		fixture.ExactQuestion != goldenCaddyfileErrorQuestion ||
		fixture.MaxModelCalls != goldenMechanismV1MaxModelCalls ||
		fixture.PromptVersion != semanticdiscovery.GoldenMechanismPromptVersion ||
		fixture.ExistingArtifactID != existing.Artifact.ID ||
		fixture.ExistingArtifactSHA256 != goldenDirectoryArtifactSHA256 {
		return fmt.Errorf("golden mechanism v1: frozen fixture identity changed")
	}
	if !reflect.DeepEqual(fixture.QuestionCandidates, goldenMechanismV1QuestionCandidates()) ||
		!reflect.DeepEqual(fixture.Aspects, goldenCaddyfileErrorAspectDefinitions()) {
		return fmt.Errorf("golden mechanism v1: frozen survey or answer aspects changed")
	}
	if fixture.Acceptance.MinLocalRubricScore != goldenMechanismV1AcceptanceMinScore ||
		fixture.Acceptance.MinCoveredAspects != 6 ||
		fixture.Acceptance.MinKeyCoveredAspects != 4 ||
		!fixture.Acceptance.AllKeyAspectsNeedStrongClaims ||
		!fixture.Acceptance.UncoveredAspectsNeedUnknowns ||
		!fixture.Acceptance.NoResponseSpecificChanges ||
		!slices.Equal(fixture.Acceptance.ClaimCountRange, []int{3, 7}) {
		return fmt.Errorf("golden mechanism v1: frozen acceptance rubric changed")
	}
	return nil
}

func validateGoldenMechanismV1Projection(
	fixture goldenMechanismV1Fixture,
	projection goldenProjection,
) error {
	if projection.Candidate.ID != goldenCaddyfileErrorCandidateID ||
		projection.Candidate.Title != goldenCaddyfileErrorTitle ||
		projection.Candidate.QuestionAnswered != goldenCaddyfileErrorQuestion ||
		len(projection.Facts) != 7 || projection.Leaf.Task.ID == "" {
		return fmt.Errorf("golden mechanism v1: frozen projection identity changed")
	}
	factDigests, evidenceIDs, err := goldenMechanismV1FactDigests(projection.Facts)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(factDigests, fixture.FactDigests) ||
		!slices.Equal(evidenceIDs, fixture.EvidenceIDs) {
		return fmt.Errorf("golden mechanism v1: frozen fact or evidence hashes changed")
	}
	if projection.Candidate.CapabilityContract == nil ||
		!slices.Equal(
			projection.Candidate.CapabilityContract.AvailableCapabilities,
			fixture.AvailableCapabilities,
		) ||
		!slices.Equal(
			projection.Candidate.CapabilityContract.MissingCapabilities,
			fixture.MissingCapabilities,
		) {
		return fmt.Errorf("golden mechanism v1: frozen capability partition changed")
	}
	return nil
}

func combineGoldenMechanismV1Record(
	input goldenMechanismV1Input,
	newFanIn semanticdiscovery.FanInArtifact,
) ([]byte, []semanticdiscovery.Artifact, error) {
	if len(newFanIn.Artifacts) != 1 ||
		newFanIn.Artifacts[0].CandidateID != goldenCaddyfileErrorCandidateID {
		return nil, nil, fmt.Errorf("golden mechanism v1: new fan-in identity changed")
	}
	proposal := semanticdiscovery.OpportunityProposal{
		Version: semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{
			input.Existing.Candidate,
			input.Projection.Candidate,
		},
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(input.Bundle, proposal); err != nil {
		return nil, nil, err
	}
	selected, err := semanticdiscovery.SelectOpportunities(input.Bundle, proposal, 2)
	if err != nil {
		return nil, nil, err
	}
	if len(selected) != 2 {
		return nil, nil, fmt.Errorf("golden mechanism v1: combined selection lost a candidate")
	}
	selectedIDs := []string{selected[0].ID, selected[1].ID}
	sort.Strings(selectedIDs)
	if !slices.Equal(
		selectedIDs,
		[]string{goldenCaddyfileErrorCandidateID, goldenDirectoryListingCandidateID},
	) {
		return nil, nil, fmt.Errorf("golden mechanism v1: combined selection changed")
	}
	leaves := []semanticdiscovery.LeafResult{input.Existing.Leaf, input.Leaf}
	combinedFanIn := semanticdiscovery.FanInArtifact{
		Version: semanticdiscovery.FanInArtifactVersion,
		Artifacts: []semanticdiscovery.ArtifactProposal{
			input.Existing.Proposal,
			newFanIn.Artifacts[0],
		},
	}
	recordBytes, err := semanticdiscovery.EncodeRecord(
		input.Bundle,
		proposal,
		selected,
		leaves,
		combinedFanIn,
	)
	if err != nil {
		return nil, nil, err
	}
	artifacts, err := semanticdiscovery.ReplayRecord(input.Bundle, recordBytes)
	if err != nil {
		return nil, nil, err
	}
	if len(artifacts) != 2 {
		return nil, nil, fmt.Errorf("golden mechanism v1: combined replay returned %d artifacts", len(artifacts))
	}
	if err := requireGoldenArtifacts(
		artifacts,
		[]semanticdiscovery.Artifact{input.Existing.Artifact},
	); err != nil {
		return nil, nil, fmt.Errorf("golden mechanism v1: directory artifact regression: %w", err)
	}
	newArtifacts := 0
	for _, artifact := range artifacts {
		if artifact.CandidateID == goldenCaddyfileErrorCandidateID {
			newArtifacts++
		}
	}
	if newArtifacts != 1 {
		return nil, nil, fmt.Errorf("golden mechanism v1: combined replay lost the new artifact")
	}
	return recordBytes, artifacts, nil
}

func reserveGoldenMechanismV1Call(
	path string,
	input goldenMechanismV1Input,
) error {
	record := goldenMechanismV1Reservation{
		Version: 1, State: "reserved_before_provider_call",
		CandidateID:   goldenCaddyfileErrorCandidateID,
		FixtureSHA256: input.FixtureHash,
		PromptVersion: input.Prompt.Version,
		PromptSHA256:  input.Fixture.PromptSHA256,
		MaxCalls:      goldenMechanismV1MaxModelCalls,
	}
	raw, err := marshalGoldenJSON(record)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("golden mechanism v1: the single synthesis call is already reserved")
		}
		return err
	}
	_, writeErr := file.Write(raw)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return errors.Join(writeErr, closeErr)
	}
	return nil
}

func saveGoldenMechanismV1Response(
	runDir string,
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
		filepath.Join(runDir, goldenMechanismV1ResponseFile),
		goldenMechanismResponseAttempt{
			Version: 1, CandidateID: goldenCaddyfileErrorCandidateID,
			PromptVersion:    semanticdiscovery.GoldenMechanismPromptVersion,
			ValidationStatus: validationStatus,
			FailureClass:     failureClass,
			Reduction:        &synthesis.Reduction,
			Content:          string(synthesis.RawResponse),
		},
	)
}

func goldenFactIDs(facts []semanticdiscovery.Fact) []string {
	result := make([]string, 0, len(facts))
	for _, fact := range facts {
		result = append(result, fact.ID)
	}
	sort.Strings(result)
	return result
}

func ensureGoldenMechanismV1Safe(label string, raw []byte) error {
	if kind, sensitive := secretscan.Detect(string(raw)); sensitive {
		return fmt.Errorf("golden mechanism v1: %s contains an obvious %s", label, kind)
	}
	return nil
}

func writeFrozenGoldenFile(path string, raw []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
		existing, readErr := readBoundedRegularFile(path, maxGoldenSavedFileBytes)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(existing, raw) {
			return fmt.Errorf("golden mechanism v1: frozen file %s already exists with different bytes", filepath.Base(path))
		}
		return nil
	}
	_, writeErr := file.Write(raw)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return errors.Join(writeErr, closeErr)
	}
	return nil
}

func replayGoldenMechanismV1(
	_ context.Context,
	runDir string,
	stdout io.Writer,
	_ io.Writer,
) (returnErr error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(absDir, report.RunManifestFilename)
	if _, err := os.Lstat(manifestPath); err == nil {
		return fmt.Errorf(
			"golden mechanism v1 replay refuses an authorized run; copy it and remove %s first",
			report.RunManifestFilename,
		)
	} else if !os.IsNotExist(err) {
		return err
	}
	status := goldenMechanismV1ReplayStatus{
		Version: 1, State: "started",
		ReportFile: filepath.Join(absDir, "report.html"),
	}
	defer func() {
		if returnErr != nil {
			status.State = "failed"
		}
		if err := writeGoldenJSON(
			filepath.Join(absDir, goldenMechanismV1ReplayStatusFile),
			status,
		); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()

	before, err := report.ReadRunDir(absDir)
	if err != nil {
		return err
	}
	artifacts, err := goldenMechanismV1PublishedArtifacts(before.SemanticArtifacts)
	if err != nil {
		return err
	}
	bundle, err := report.BuildSemanticDiscoveryBundle(before)
	if err != nil {
		return err
	}
	recordRaw, err := readBoundedRegularFile(
		filepath.Join(absDir, report.GoldenMechanismRecordFile),
		maxGoldenSavedFileBytes,
	)
	if err != nil {
		return err
	}
	replayed, err := semanticdiscovery.ReplayRecord(bundle, recordRaw)
	if err != nil {
		return err
	}
	if err := requireGoldenArtifacts(replayed, artifacts); err != nil {
		return err
	}
	beforeHashes, err := goldenMechanismV1ArtifactHashes(artifacts)
	if err != nil {
		return err
	}
	if err := report.Generate(absDir); err != nil {
		return err
	}
	after, err := report.ReadRunDir(absDir)
	if err != nil {
		return err
	}
	afterArtifacts, err := goldenMechanismV1PublishedArtifacts(after.SemanticArtifacts)
	if err != nil {
		return err
	}
	if err := requireGoldenArtifacts(afterArtifacts, artifacts); err != nil {
		return err
	}
	afterHashes, err := goldenMechanismV1ArtifactHashes(afterArtifacts)
	if err != nil {
		return err
	}
	if !slices.Equal(beforeHashes, afterHashes) {
		return fmt.Errorf("golden mechanism v1 replay changed artifact hashes")
	}
	html, err := readBoundedRegularFile(status.ReportFile, maxGoldenSavedFileBytes)
	if err != nil {
		return err
	}
	status.State = "replayed"
	status.ArtifactSHA256 = beforeHashes
	status.DirectoryArtifactPreserved = false
	status.SearchIndexed = true
	status.HTMLContainsArtifacts = true
	for _, artifact := range afterArtifacts {
		status.ArtifactIDs = append(status.ArtifactIDs, artifact.ID)
		status.EvidenceCount += len(artifact.Evidence)
		if artifact.ID == goldenDirectoryArtifactID {
			digest, hashErr := goldenMechanismArtifactSHA256(artifact)
			if hashErr != nil || digest != goldenDirectoryArtifactSHA256 {
				return fmt.Errorf("golden mechanism v1 replay changed the directory artifact")
			}
			status.DirectoryArtifactPreserved = true
		}
		if !semanticSearchContainsArtifact(after, artifact.ID) {
			status.SearchIndexed = false
		}
		if !bytes.Contains(html, []byte(artifact.ID)) {
			status.HTMLContainsArtifacts = false
		}
	}
	sort.Strings(status.ArtifactIDs)
	if !status.DirectoryArtifactPreserved || !status.SearchIndexed ||
		!status.HTMLContainsArtifacts || status.EvidenceCount == 0 {
		return fmt.Errorf("golden mechanism v1 replay projection is incomplete")
	}
	fmt.Fprintf(
		stdout,
		"No-model replay: %s\nArtifacts: %s\nReport: %s\n",
		status.State,
		strings.Join(status.ArtifactIDs, ", "),
		status.ReportFile,
	)
	return nil
}

func goldenMechanismV1PublishedArtifacts(
	artifacts []semanticdiscovery.Artifact,
) ([]semanticdiscovery.Artifact, error) {
	result := make([]semanticdiscovery.Artifact, 0, 2)
	for _, artifact := range artifacts {
		switch artifact.CandidateID {
		case goldenDirectoryListingCandidateID, goldenCaddyfileErrorCandidateID:
			result = append(result, artifact)
		}
	}
	if len(result) != 2 {
		return nil, fmt.Errorf("golden mechanism v1: expected two published Golden artifacts")
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CandidateID < result[j].CandidateID
	})
	return result, nil
}

func goldenMechanismV1ArtifactHashes(
	artifacts []semanticdiscovery.Artifact,
) ([]string, error) {
	items := append([]semanticdiscovery.Artifact(nil), artifacts...)
	sort.Slice(items, func(i, j int) bool { return items[i].CandidateID < items[j].CandidateID })
	result := make([]string, 0, len(items))
	for _, artifact := range items {
		digest, err := goldenMechanismArtifactSHA256(artifact)
		if err != nil {
			return nil, err
		}
		result = append(result, digest)
	}
	return result, nil
}
