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
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/goldenmechanism"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
)

const freshRepoDemoReplayStatusFile = "fresh_repo_replay_status.json"

type freshSavedReplayRuntime struct {
	publish func(
		runDir string,
		identity semanticdiscovery.MechanismIdentity,
		candidateID string,
		probeRaw []byte,
		supplement report.SemanticSupplement,
		recordRaw []byte,
		want semanticdiscovery.Artifact,
	) (semanticdiscovery.Mechanism, int, error)
	resolveRole func(string, string) (report.OnboardingRole, error)
}

type freshSavedCandidateEvaluation struct {
	Attempt    freshRepoCandidateAttempt
	Plan       freshRepoCandidatePlan
	Candidate  semanticdiscovery.OpportunityCandidate
	ProbeRaw   []byte
	Supplement report.SemanticSupplement
	Synthesis  goldenMechanismSynthesis
	Artifact   semanticdiscovery.Artifact
	Summary    goldenMechanismArtifactSummary
}

func replaySavedFreshRepoMechanisms(
	ctx context.Context,
	runDir string,
) (result freshRepoDemoResult, returnErr error) {
	started := time.Now()
	status := freshRepoDemoStatus{Version: 1, State: "started"}
	defer func() {
		status.WallMillis = time.Since(started).Milliseconds()
		if returnErr != nil {
			status.State = "failed"
			if status.FailureReason == "" {
				status.FailureReason = semanticDiscoveryReason(returnErr.Error())
			}
		}
		result.Status = status
		if err := writeGoldenJSON(
			filepath.Join(runDir, freshRepoDemoReplayStatusFile),
			status,
		); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()

	if ctx == nil {
		return result, fmt.Errorf("fresh repository replay: context is required")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	absRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return result, fmt.Errorf("fresh repository replay: resolve run directory: %w", err)
	}
	data, err := report.ReadRunDir(absRunDir)
	if err != nil {
		return result, fmt.Errorf("fresh repository replay: read saved run: %w", err)
	}
	var candidates freshRepoCandidatesArtifact
	if err := readFreshReplayJSON(
		filepath.Join(absRunDir, freshRepoDemoCandidatesFile),
		&candidates,
	); err != nil {
		return result, fmt.Errorf("fresh repository replay: read candidates: %w", err)
	}
	var sourceFacts freshRepoSourceFactsArtifact
	if err := readFreshReplayJSON(
		filepath.Join(absRunDir, freshRepoDemoFactsFile),
		&sourceFacts,
	); err != nil {
		return result, fmt.Errorf("fresh repository replay: read source facts: %w", err)
	}
	if err := preserveLegacyFreshMechanism(absRunDir); err != nil {
		return result, err
	}

	status, err = replayFreshSavedCandidates(
		ctx,
		absRunDir,
		data,
		candidates,
		sourceFacts,
		freshSavedReplayRuntime{
			publish:     publishFreshMechanism,
			resolveRole: freshPublishedOnboardingRole,
		},
	)
	if err != nil {
		return result, err
	}
	return result, nil
}

func replayFreshSavedCandidates(
	ctx context.Context,
	runDir string,
	data *report.ReportData,
	candidates freshRepoCandidatesArtifact,
	sourceFacts freshRepoSourceFactsArtifact,
	runtime freshSavedReplayRuntime,
) (freshRepoDemoStatus, error) {
	status := freshRepoDemoStatus{
		Version:            1,
		State:              "started",
		SourceWindows:      sourceFacts.Windows,
		SourceFunctions:    sourceFacts.Functions,
		SourceFacts:        len(sourceFacts.Facts),
		QuestionsProposed:  len(candidates.Proposal.Candidates),
		CandidatesSelected: len(candidates.Selected),
	}
	if ctx == nil {
		return status, fmt.Errorf("fresh repository replay: context is required")
	}
	if data == nil {
		return status, fmt.Errorf("fresh repository replay: report data is required")
	}
	if runtime.publish == nil || runtime.resolveRole == nil {
		return status, fmt.Errorf("fresh repository replay: publication runtime is required")
	}
	if err := ctx.Err(); err != nil {
		return status, err
	}

	candidateByID, sources, err := validateFreshReplayInputs(data, candidates, sourceFacts)
	if err != nil {
		return status, err
	}
	for _, plan := range candidates.Selected {
		if err := ctx.Err(); err != nil {
			return status, err
		}
		candidate := candidateByID[plan.CandidateID]
		evaluated, semanticRejection, evaluationErr := evaluateFreshSavedCandidate(
			data,
			runDir,
			candidate,
			plan,
			sources,
		)
		if evaluationErr != nil {
			status.Attempts = append(status.Attempts, evaluated.Attempt)
			if semanticRejection {
				continue
			}
			return status, evaluationErr
		}

		mechanism, visibleSteps, err := runtime.publish(
			runDir,
			plan.Identity,
			candidate.ID,
			evaluated.ProbeRaw,
			evaluated.Supplement,
			evaluated.Synthesis.RecordBytes,
			evaluated.Artifact,
		)
		if err != nil {
			evaluated.Attempt.reject("publication", err)
			status.Attempts = append(status.Attempts, evaluated.Attempt)
			return status, err
		}
		role, err := runtime.resolveRole(runDir, evaluated.Summary.ID)
		if err != nil {
			evaluated.Attempt.reject("onboarding_role", err)
			status.Attempts = append(status.Attempts, evaluated.Attempt)
			return status, err
		}
		evaluated.Attempt.State = "published"
		evaluated.Attempt.VisibleSteps = visibleSteps
		evaluated.Attempt.OnboardingRole = role
		status.Attempts = append(status.Attempts, evaluated.Attempt)
		status.PublishedMechanisms = append(
			status.PublishedMechanisms,
			freshPublishedMechanism{
				CandidateID: candidate.ID,
				MechanismID: mechanism.ID,
				ArtifactID:  evaluated.Summary.ID,
				Role:        role,
			},
		)
		status.State = "replayed"
		if status.PublishedArtifact == nil || role == report.OnboardingRolePrimaryBehavior {
			status.PublishedCandidateID = candidate.ID
			status.PublishedMechanismID = mechanism.ID
			summary := evaluated.Summary
			status.PublishedArtifact = &summary
		}
		if !freshContinueAfterPublication(role) {
			return status, nil
		}
	}
	if len(status.PublishedMechanisms) == 0 {
		status.State = "no_publishable_candidate"
		status.FailureReason = "all_saved_responses_rejected"
	}
	return status, nil
}

func validateFreshReplayInputs(
	data *report.ReportData,
	candidates freshRepoCandidatesArtifact,
	sourceFacts freshRepoSourceFactsArtifact,
) (
	map[string]semanticdiscovery.OpportunityCandidate,
	[]freshSourceFunction,
	error,
) {
	if candidates.Version != 1 || sourceFacts.Version != 1 {
		return nil, nil, fmt.Errorf("fresh repository replay: unsupported saved artifact version")
	}
	if len(candidates.Proposal.Candidates) == 0 ||
		len(candidates.Proposal.Candidates) > freshRepoDemoMaxQuestions ||
		len(candidates.Selected) == 0 || len(candidates.Selected) > freshRepoDemoMaxCandidates {
		return nil, nil, fmt.Errorf("fresh repository replay: saved candidate counts are outside bounds")
	}
	if len(sourceFacts.Facts) == 0 || len(sourceFacts.Facts) > freshRepoDemoMaxSourceFacts ||
		sourceFacts.Functions != len(sourceFacts.Facts) {
		return nil, nil, fmt.Errorf("fresh repository replay: saved source facts are outside bounds")
	}

	sources, factByID, err := freshReplaySources(sourceFacts.Facts)
	if err != nil {
		return nil, nil, err
	}
	data.SemanticSupplementalFacts = append(
		[]semanticdiscovery.Fact(nil),
		sourceFacts.Facts...,
	)
	planningBundle, err := report.BuildSemanticDiscoveryBundle(data)
	if err != nil {
		return nil, nil, fmt.Errorf("fresh repository replay: rebuild planning bundle: %w", err)
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(
		planningBundle,
		candidates.Proposal,
	); err != nil {
		return nil, nil, fmt.Errorf("fresh repository replay: validate saved candidates: %w", err)
	}

	candidateByID := make(map[string]semanticdiscovery.OpportunityCandidate, len(candidates.Proposal.Candidates))
	for _, candidate := range candidates.Proposal.Candidates {
		candidateByID[candidate.ID] = candidate
	}
	seen := make(map[string]struct{}, len(candidates.Selected))
	for _, plan := range candidates.Selected {
		candidate, exists := candidateByID[plan.CandidateID]
		if !exists {
			return nil, nil, fmt.Errorf(
				"fresh repository replay: selected candidate %q is unavailable",
				plan.CandidateID,
			)
		}
		if _, duplicate := seen[plan.CandidateID]; duplicate {
			return nil, nil, fmt.Errorf(
				"fresh repository replay: duplicate selected candidate %q",
				plan.CandidateID,
			)
		}
		seen[plan.CandidateID] = struct{}{}
		if err := validateFreshReplayPlan(plan, candidate, factByID); err != nil {
			return nil, nil, err
		}
	}
	return candidateByID, sources, nil
}

func freshReplaySources(
	facts []semanticdiscovery.Fact,
) ([]freshSourceFunction, map[string]semanticdiscovery.Fact, error) {
	sources := make([]freshSourceFunction, 0, len(facts))
	byID := make(map[string]semanticdiscovery.Fact, len(facts))
	for _, fact := range facts {
		if fact.Source == nil || fact.Source.Path == "" || fact.Source.EnclosingSymbol == "" {
			return nil, nil, fmt.Errorf(
				"fresh repository replay: source fact %q has no exact function scope",
				fact.ID,
			)
		}
		if _, duplicate := byID[fact.ID]; duplicate {
			return nil, nil, fmt.Errorf("fresh repository replay: duplicate source fact %q", fact.ID)
		}
		byID[fact.ID] = fact
		sources = append(sources, freshSourceFunction{
			Function: sourcewindowfacts.Function{
				Path:          fact.Source.Path,
				Symbol:        fact.Source.EnclosingSymbol,
				StartLine:     fact.Source.StartLine,
				EndLine:       fact.Source.EndLine,
				ContentSHA256: fact.Source.ContentSHA256,
			},
			Fact: fact,
		})
	}
	return sources, byID, nil
}

func validateFreshReplayPlan(
	plan freshRepoCandidatePlan,
	candidate semanticdiscovery.OpportunityCandidate,
	factByID map[string]semanticdiscovery.Fact,
) error {
	knownFacts := make(map[string]semanticdiscovery.Fact, len(factByID))
	for id, fact := range factByID {
		knownFacts[id] = fact
	}
	if plan.Primary != nil {
		for _, fact := range append(
			append([]semanticdiscovery.Fact(nil), plan.Primary.AnchorFacts...),
			plan.Primary.ProjectedFacts...,
		) {
			if fact.ID == "" {
				return fmt.Errorf("fresh repository replay: primary planner retained an empty fact id")
			}
			knownFacts[fact.ID] = fact
		}
	}
	if plan.CandidateID != candidate.ID || plan.Question != candidate.QuestionAnswered ||
		plan.Kind != candidate.Kind {
		return fmt.Errorf("fresh repository replay: candidate plan identity mismatch")
	}
	if plan.Identity.RepositoryNamespace == "" || plan.Identity.IntentKey == "" ||
		plan.Identity.Scope.Kind == "" || plan.Identity.Scope.Value == "" ||
		plan.Probe.MechanismID != plan.Identity.IntentKey {
		return fmt.Errorf("fresh repository replay: candidate %q has invalid mechanism identity", candidate.ID)
	}
	minimumAnchors := 3
	if plan.Primary != nil {
		minimumAnchors = 1
	}
	if len(plan.AnchorFactIDs) < minimumAnchors || len(plan.AnchorFactIDs) > freshRepoDemoMaxSeedFuncs ||
		len(plan.AnchorEvidenceIDs) != len(plan.AnchorFactIDs) ||
		!sort.StringsAreSorted(plan.AnchorFactIDs) ||
		!sort.StringsAreSorted(plan.AnchorEvidenceIDs) {
		return fmt.Errorf("fresh repository replay: candidate %q anchors are invalid", candidate.ID)
	}
	for _, id := range plan.AnchorFactIDs {
		if _, exists := knownFacts[id]; !exists {
			return fmt.Errorf("fresh repository replay: candidate %q has unknown anchor fact %q", candidate.ID, id)
		}
	}
	for _, evidenceID := range plan.AnchorEvidenceIDs {
		found := false
		for _, fact := range knownFacts {
			for _, reference := range fact.Evidence {
				if reference.ID == evidenceID {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return fmt.Errorf("fresh repository replay: candidate %q has unknown anchor evidence", candidate.ID)
		}
	}
	minimumProbeSeeds := 3
	maximumDepth := 3
	maximumFunctions := 15
	maximumSourceBytes := 128 << 10
	maximumTimeout := 5 * time.Second
	if plan.Primary != nil {
		maximumSelectedFrontiers := freshPrimaryMaxPathFrontiers
		maximumPrimaryFunctions := freshPrimaryMaxFunctions
		maximumAdditionalFunctions := freshPrimaryMaxAdditionalFuncs
		switch plan.Primary.Version {
		case 1:
			// Legacy v1 counted both named expansions in one frontier list and
			// allowed the historical fifteen-function inspection cap.
			maximumSelectedFrontiers = 2
			maximumPrimaryFunctions = 15
			maximumAdditionalFunctions = 11
			maximumFunctions = 15
		case freshPrimaryPlanVersion:
			maximumFunctions = freshPrimaryMaxFunctions
		default:
			return fmt.Errorf(
				"fresh repository replay: candidate %q has unsupported primary plan version %d",
				candidate.ID,
				plan.Primary.Version,
			)
		}
		minimumProbeSeeds = 1
		maximumDepth = freshPrimaryMaxDepth
		maximumSourceBytes = freshPrimaryMaxRetainedBytes
		maximumTimeout = freshPrimaryTimeout
		if plan.Primary.IntentKey != plan.Identity.IntentKey ||
			plan.Primary.CandidateID != candidate.ID ||
			plan.Primary.Question != candidate.QuestionAnswered ||
			len(plan.Primary.SelectedFrontiers) > maximumSelectedFrontiers ||
			len(plan.Primary.AdditionalFilesRead) > freshPrimaryMaxAdditionalFiles ||
			plan.Primary.Limits.MaxFunctions > maximumPrimaryFunctions ||
			plan.Primary.Limits.MaxAdditionalFuncs > maximumAdditionalFunctions ||
			plan.Primary.RetainedSourceBytes > freshPrimaryMaxRetainedBytes {
			return fmt.Errorf("fresh repository replay: candidate %q primary plan is outside bounds", candidate.ID)
		}
	}
	if len(plan.Probe.Seeds) < minimumProbeSeeds ||
		len(plan.Probe.Seeds) > freshRepoDemoMaxSeedFuncs+freshRepoDemoMaxFrontier ||
		len(plan.Probe.ExpansionAllowlist) > freshRepoDemoMaxFrontier {
		return fmt.Errorf("fresh repository replay: candidate %q probe frontier is outside bounds", candidate.ID)
	}
	for _, seed := range plan.Probe.Seeds {
		fact, exists := knownFacts[seed.OriginFactID]
		if !exists || !freshFactHasEvidence(fact, seed.OriginEvidenceID) ||
			seed.Path == "" || seed.Symbol == "" {
			return fmt.Errorf("fresh repository replay: candidate %q has an unbound probe seed", candidate.ID)
		}
	}
	limits := plan.Probe.Limits
	if limits.MaxDepth <= 0 || limits.MaxDepth > maximumDepth ||
		limits.MaxFiles <= 0 || limits.MaxFiles > freshRepoDemoMaxProbeFiles ||
		limits.MaxFunctions <= 0 || limits.MaxFunctions > maximumFunctions ||
		limits.MaxParsedSourceBytes <= 0 || limits.MaxParsedSourceBytes > freshRepoDemoMaxParsedBytes ||
		limits.MaxSourceBytes <= 0 || limits.MaxSourceBytes > maximumSourceBytes ||
		limits.MaxFunctionLines <= 0 || limits.MaxFunctionLines > 220 ||
		limits.MaxFunctionBytes <= 0 || limits.MaxFunctionBytes > 48<<10 ||
		limits.Timeout <= 0 || limits.Timeout > maximumTimeout {
		return fmt.Errorf("fresh repository replay: candidate %q probe limits are outside bounds", candidate.ID)
	}
	return nil
}

func freshFactHasEvidence(fact semanticdiscovery.Fact, evidenceID string) bool {
	for _, reference := range fact.Evidence {
		if reference.ID == evidenceID {
			return true
		}
	}
	return false
}

func evaluateFreshSavedCandidate(
	data *report.ReportData,
	runDir string,
	candidate semanticdiscovery.OpportunityCandidate,
	plan freshRepoCandidatePlan,
	sources []freshSourceFunction,
) (evaluation freshSavedCandidateEvaluation, semanticRejection bool, returnErr error) {
	started := time.Now()
	evaluation.Plan = plan
	evaluation.Candidate = candidate
	evaluation.Attempt = freshRepoCandidateAttempt{
		CandidateID: candidate.ID,
		Question:    candidate.QuestionAnswered,
		State:       "replaying",
		IntentKey:   plan.Identity.IntentKey,
	}
	defer func() {
		evaluation.Attempt.WallMillis = time.Since(started).Milliseconds()
	}()

	attemptDir := filepath.Join(runDir, freshRepoDemoAttemptsDir, candidate.ID)
	probeRaw, err := readBoundedRegularFile(
		filepath.Join(attemptDir, "probe.json"),
		freshRepoOnboardingMaxBundleBytes,
	)
	if err != nil {
		evaluation.Attempt.reject("saved_probe", err)
		return evaluation, false, fmt.Errorf("fresh repository replay: read %s probe: %w", candidate.ID, err)
	}
	var probe goldenmechanism.Result
	if err := decodeFreshReplayJSON(probeRaw, &probe); err != nil {
		evaluation.Attempt.reject("saved_probe", err)
		return evaluation, false, fmt.Errorf("fresh repository replay: decode %s probe: %w", candidate.ID, err)
	}
	if err := probe.Validate(); err != nil {
		evaluation.Attempt.reject("saved_probe", err)
		return evaluation, false, fmt.Errorf("fresh repository replay: validate %s probe: %w", candidate.ID, err)
	}
	if err := validateFreshReplayProbe(plan.Probe, probe); err != nil {
		evaluation.Attempt.reject("saved_probe_binding", err)
		return evaluation, false, err
	}
	evaluation.ProbeRaw = probeRaw
	probeDigest := sha256.Sum256(probeRaw)
	probeSHA := hex.EncodeToString(probeDigest[:])
	evaluation.Attempt.ProbeBudget = &probe.Budget
	evaluation.Attempt.ProbePartial = probe.Partial
	evaluation.Attempt.ProbeStopReason = probe.StopReason
	evaluation.Attempt.ProbeSHA256 = probeSHA

	work, err := freshReplayCandidateWork(candidate, plan, sources)
	if err != nil {
		evaluation.Attempt.reject("candidate_binding", err)
		return evaluation, false, err
	}
	probeFacts, aspects, err := freshProbeFacts(work, probe)
	if err != nil {
		evaluation.Attempt.reject("fact_projection", err)
		return evaluation, true, err
	}
	evaluation.Attempt.ProbeFacts = len(probeFacts)
	work.Plan.Aspects = aspects
	facts, projectedCandidate, err := freshCandidateProjection(work, probeFacts, aspects)
	if err != nil {
		evaluation.Attempt.reject("candidate_projection", err)
		return evaluation, true, err
	}
	supplement, bundle, err := report.PrepareSemanticSupplement(
		data,
		projectedCandidate.ID,
		probeSHA,
		facts,
	)
	if err != nil {
		evaluation.Attempt.reject("supplement", err)
		return evaluation, true, err
	}
	proposal := semanticdiscovery.OpportunityProposal{
		Version:    semanticdiscovery.OpportunityProposalVersion,
		Candidates: []semanticdiscovery.OpportunityCandidate{projectedCandidate},
	}
	if err := semanticdiscovery.ValidateOpportunityProposal(bundle, proposal); err != nil {
		evaluation.Attempt.reject("candidate_validation", err)
		return evaluation, true, err
	}
	leaf, err := freshValidatedLeaf(bundle, projectedCandidate, probeFacts)
	if err != nil {
		evaluation.Attempt.reject("leaf_validation", err)
		return evaluation, true, err
	}

	responseRaw, err := readBoundedRegularFile(
		filepath.Join(attemptDir, "response_attempt.json"),
		freshRepoOnboardingMaxBundleBytes,
	)
	if err != nil {
		evaluation.Attempt.reject("saved_response", err)
		if errors.Is(err, os.ErrNotExist) {
			return evaluation, true, fmt.Errorf(
				"fresh repository replay: candidate %s has no saved synthesis response: %w",
				candidate.ID,
				err,
			)
		}
		return evaluation, false, fmt.Errorf("fresh repository replay: read %s response: %w", candidate.ID, err)
	}
	var response goldenMechanismResponseAttempt
	if err := decodeFreshReplayJSON(responseRaw, &response); err != nil {
		evaluation.Attempt.reject("saved_response", err)
		return evaluation, false, fmt.Errorf("fresh repository replay: decode %s response: %w", candidate.ID, err)
	}
	if response.Version != 1 || response.CandidateID != candidate.ID ||
		strings.TrimSpace(response.Content) == "" {
		err := fmt.Errorf("fresh repository replay: saved response identity mismatch for %q", candidate.ID)
		evaluation.Attempt.reject("saved_response_binding", err)
		return evaluation, false, err
	}

	synthesis, evaluationErr := evaluateGoldenMechanismResponse(
		bundle,
		proposal,
		leaf,
		[]byte(response.Content),
	)
	evaluation.Synthesis = synthesis
	evaluation.Supplement = supplement
	evaluation.Attempt.Reduction = &evaluation.Synthesis.Reduction
	if evaluationErr != nil {
		evaluation.Attempt.reject("synthesis_validation", evaluationErr)
		return evaluation, true, evaluationErr
	}
	if len(synthesis.Artifacts) != 1 {
		err := fmt.Errorf("fresh repository replay: candidate %q produced no single artifact", candidate.ID)
		evaluation.Attempt.reject("synthesis_validation", err)
		return evaluation, true, err
	}
	evaluation.Artifact = synthesis.Artifacts[0]
	if plan.Primary != nil {
		if err := validateFreshPrimaryArtifact(evaluation.Artifact, plan.Primary, probeFacts); err != nil {
			evaluation.Attempt.reject("primary_relevance", err)
			return evaluation, true, err
		}
	}
	evaluation.Summary, err = summarizeGoldenMechanismArtifact(
		projectedCandidate,
		evaluation.Artifact,
	)
	if err != nil {
		evaluation.Attempt.reject("publishability", err)
		return evaluation, true, err
	}
	evaluation.Candidate = projectedCandidate
	evaluation.Attempt.Artifact = &evaluation.Summary
	evaluation.Attempt.State = "validated"
	return evaluation, false, nil
}

func freshReplayCandidateWork(
	candidate semanticdiscovery.OpportunityCandidate,
	plan freshRepoCandidatePlan,
	sources []freshSourceFunction,
) (freshCandidateWork, error) {
	byID := make(map[string]freshSourceFunction, len(sources))
	for _, source := range sources {
		byID[source.Fact.ID] = source
	}
	initialSources := append([]freshSourceFunction(nil), sources...)
	if plan.Primary != nil {
		for _, fact := range plan.Primary.AnchorFacts {
			if fact.Source == nil {
				continue
			}
			source := freshSourceFunction{
				Function: sourcewindowfacts.Function{
					Path: fact.Source.Path, Symbol: fact.Source.EnclosingSymbol,
					StartLine: fact.Source.StartLine, EndLine: fact.Source.EndLine,
					ContentSHA256: fact.Source.ContentSHA256,
				},
				Fact: fact,
			}
			byID[fact.ID] = source
			initialSources = append(initialSources, source)
		}
	}
	seeds := make([]freshSourceFunction, 0, len(plan.AnchorFactIDs))
	for _, id := range plan.AnchorFactIDs {
		source, exists := byID[id]
		if !exists {
			return freshCandidateWork{}, fmt.Errorf(
				"fresh repository replay: anchor fact %q is unavailable",
				id,
			)
		}
		seeds = append(seeds, source)
	}
	return freshCandidateWork{
		Candidate:      candidate,
		Plan:           plan,
		Seeds:          seeds,
		InitialSources: initialSources,
		Primary: func() *freshPrimaryCandidateWork {
			if plan.Primary == nil {
				return nil
			}
			return &freshPrimaryCandidateWork{
				ProbeFacts: append([]semanticdiscovery.Fact(nil), plan.Primary.ProjectedFacts...),
			}
		}(),
	}, nil
}

func validateFreshReplayProbe(
	plan goldenmechanism.Plan,
	probe goldenmechanism.Result,
) error {
	if probe.MechanismID != plan.MechanismID || len(probe.Seeds) != len(plan.Seeds) {
		return fmt.Errorf("fresh repository replay: saved probe does not match its candidate plan")
	}
	for index, resolution := range probe.Seeds {
		if resolution.Seed != plan.Seeds[index] {
			return fmt.Errorf("fresh repository replay: saved probe seed[%d] changed", index)
		}
	}
	if probe.Budget.FilesParsed > plan.Limits.MaxFiles ||
		probe.Budget.FunctionsIncluded > plan.Limits.MaxFunctions ||
		probe.Budget.ParsedSourceBytes > plan.Limits.MaxParsedSourceBytes ||
		probe.Budget.IncludedSourceBytes > plan.Limits.MaxSourceBytes ||
		probe.Budget.MaxDepthReached > plan.Limits.MaxDepth {
		return fmt.Errorf("fresh repository replay: saved probe exceeds its candidate budget")
	}
	for _, function := range probe.Functions {
		if len(function.Source) > plan.Limits.MaxFunctionLines {
			return fmt.Errorf("fresh repository replay: saved function exceeds its line budget")
		}
		functionBytes := 0
		for index, line := range function.Source {
			functionBytes += len(line.Text)
			if index > 0 {
				functionBytes++
			}
		}
		if functionBytes > plan.Limits.MaxFunctionBytes {
			return fmt.Errorf("fresh repository replay: saved function exceeds its byte budget")
		}
	}
	return nil
}

func readFreshReplayJSON(path string, target any) error {
	raw, err := readBoundedRegularFile(path, freshRepoOnboardingMaxBundleBytes)
	if err != nil {
		return err
	}
	return decodeFreshReplayJSON(raw, target)
}

func decodeFreshReplayJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("fresh repository replay: trailing json")
	}
	return nil
}
