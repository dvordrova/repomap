package semanticdiscovery

import (
	"fmt"
	"sort"
	"strings"
)

const fanInSystemPrefix = `You are the global semantic editor for a bounded repository model. You receive independently validated leaf observations together with the original local facts that support them. Return valid JSON only. Facts and opaque IDs remain authoritative. Do not create or quote repository paths, file names, symbols, flows, relations, evidence, focus IDs, runtime order, or IDs.`

const fanInUserPrefix = `Return exactly this JSON shape:
{
  "version": 1,
  "artifacts": [
    {
      "candidate_id": "one exact selected candidate id",
      "verdict": "supported | mixed | insufficient_evidence",
      "title": "short path-free title",
      "summary": "short evidence-aware summary",
      "claims": [
        {
          "title": "short claim title",
          "text": "one claim or explicit unresolved gap",
          "basis": "direct | compositional | interpretive | unresolved",
          "support_ids": ["exact original fact ids"],
          "observation_refs": [{"task_id": "exact leaf task id", "observation_index": 0}],
          "missing_refs": [{"task_id": "exact leaf task id", "missing_index": 0}]
        }
      ],
      "aliases": ["short search phrase"],
      "likely_questions": ["question this artifact answers"],
      "related_candidate_ids": ["other exact selected candidate ids"]
    }
  ]
}

Rules:
- Return exactly one artifact for every selected candidate, including honest insufficient_evidence artifacts.
- Only fan-in decides artifact verdicts. A leaf status is a partial result, not a global conclusion.
- direct, compositional, and interpretive claims require observation_refs. Their support_ids must exactly equal the union of the referenced observations' original fact IDs.
- direct and unresolved claims may reference only the leaf for their own artifact candidate. interpretive and compositional claims may combine leaves, but at least one reference must belong to their own candidate.
- compositional claims require at least two facts from at least two independent source groups.
- unresolved claims require missing_refs and explicit limitation language. Their support_ids must exactly equal the union of all referenced observations and missing-evidence items.
- Before returning JSON, check every unresolved claim mechanically: missing_refs must be non-empty and text must literally say missing, unknown, unresolved, insufficient, cannot determine, or not established. If no matching missing-evidence item exists, do not emit that unresolved claim; use a supported basis with observation_refs only when the observations prove it.
- Never use basis=unresolved with empty missing_refs, and never cite a missing_ref from another candidate merely to satisfy the shape.
- Existing IDs alone are not proof: claim prose must overlap its supporting fact statements, and behavioral or ordering language needs matching fact capability or an explicitly cited missing capability for an unresolved claim.
- A missing-only leaf is still only a partial result. Another leaf may resolve one of its declared missing capabilities only through a validated claim with semantically matching capability-bearing facts. Otherwise preserve the gap in a mixed or insufficient_evidence verdict.
- related_candidate_ids are navigation hints only and never evidence.
- Prefer the validation-safe baseline: emit one direct claim for each useful observation, copying its exact support_ids and citing only that observation_ref. Emit one unresolved claim for every missing_evidence item, copying its exact support_ids and citing only that missing_ref. Combine observations only when a genuinely cross-leaf explanation is clearer.
- If a leaf has observations and any unresolved missing_evidence, use mixed. If it has only observations, use supported. If it has only missing_evidence, use insufficient_evidence. Never silently resolve or omit a leaf gap.
- A mixed or insufficient summary must explicitly say what is missing, unknown, unresolved, insufficient, cannot be determined, or not established, while overlapping the cited fact language.
- Do not mention paths, file names, symbols, source locations, or create repository objects in prose.`

const fanInPayloadMarker = "\n\nVariable validated leaves and original local fact JSON:\n"

type fanInPromptPayload struct {
	Version       int                    `json:"version"`
	BundleSHA256  string                 `json:"bundle_sha256"`
	RepoName      string                 `json:"repo_name"`
	Candidates    []promptCandidateScope `json:"candidates"`
	Facts         []Fact                 `json:"facts"`
	ValidatedLeaf []fanInPromptLeaf      `json:"validated_leaves"`
}

type fanInPromptLeaf struct {
	TaskID      string       `json:"task_id"`
	CandidateID string       `json:"candidate_id"`
	Artifact    LeafArtifact `json:"artifact"`
}

type fanInContext struct {
	bundleFacts map[string]Fact
	results     map[string]LeafResult
	candidates  map[string]OpportunityCandidate
}

// BuildFanInPrompt includes validated partial results and their original
// facts. Missing-evidence-only leaves remain valid input.
func BuildFanInPrompt(bundle Bundle, results []LeafResult) (Prompt, error) {
	payload, err := buildFanInPayload(bundle, results)
	if err != nil {
		return Prompt{}, err
	}
	_, encoded, err := hashJSON("fan-in input", payload)
	if err != nil {
		return Prompt{}, err
	}
	if len(encoded) > maxRecordBytes {
		return Prompt{}, fmt.Errorf("semantic discovery: fan-in payload is too large")
	}
	return Prompt{
		Version:         FanInPromptVersion,
		System:          fanInSystemPrefix,
		User:            fanInUserPrefix + fanInPayloadMarker + string(encoded),
		ThinkingProfile: ThinkingMax,
		ProgressLabel:   "semantic fan-in synthesis",
	}, nil
}

func buildFanInPayload(bundle Bundle, results []LeafResult) (fanInPromptPayload, error) {
	validated, _, err := validateLeafResults(bundle, results)
	if err != nil {
		return fanInPromptPayload{}, err
	}
	bundleHash, _, err := BundleHash(bundle)
	if err != nil {
		return fanInPromptPayload{}, err
	}

	factIDs := make(map[string]struct{})
	payload := fanInPromptPayload{
		Version:       1,
		BundleSHA256:  bundleHash,
		RepoName:      bundle.RepoName,
		Candidates:    make([]promptCandidateScope, 0, len(validated)),
		ValidatedLeaf: make([]fanInPromptLeaf, 0, len(validated)),
	}
	for _, result := range validated {
		candidate := result.Task.Candidate
		payload.Candidates = append(payload.Candidates, promptCandidateScope{
			ID: candidate.ID, Kind: candidate.Kind,
		})
		payload.ValidatedLeaf = append(payload.ValidatedLeaf, fanInPromptLeaf{
			TaskID:      result.Task.ID,
			CandidateID: result.Task.Candidate.ID,
			Artifact:    NormalizeLeafArtifact(result.Artifact),
		})
		addIDs(factIDs, candidateFactIDs(candidate))
	}
	known := factIndex(bundle)
	for _, id := range sortedSet(factIDs) {
		payload.Facts = append(payload.Facts, known[id])
	}
	payload.Facts = modelFacts(payload.Facts)
	sort.Slice(payload.Candidates, func(i, j int) bool {
		return payload.Candidates[i].ID < payload.Candidates[j].ID
	})
	sort.Slice(payload.ValidatedLeaf, func(i, j int) bool {
		return payload.ValidatedLeaf[i].TaskID < payload.ValidatedLeaf[j].TaskID
	})
	return payload, nil
}

func ParseFanInArtifact(raw []byte) (FanInArtifact, error) {
	var artifact FanInArtifact
	if err := decodeStrict(raw, &artifact, maxProposalBytes); err != nil {
		return FanInArtifact{}, fmt.Errorf("semantic discovery: invalid fan-in artifact json: %w", err)
	}
	return artifact, nil
}

func NormalizeFanInArtifact(artifact FanInArtifact) FanInArtifact {
	result := artifact
	result.Artifacts = append([]ArtifactProposal(nil), artifact.Artifacts...)
	for artifactIndex := range result.Artifacts {
		normalizeFanInProposal(&result.Artifacts[artifactIndex])
	}
	sort.Slice(result.Artifacts, func(i, j int) bool {
		return result.Artifacts[i].CandidateID < result.Artifacts[j].CandidateID
	})
	return result
}

func normalizeFanInProposal(proposal *ArtifactProposal) {
	proposal.Title = normalizeText(proposal.Title)
	proposal.Summary = normalizeText(proposal.Summary)
	proposal.Aliases = normalizeTextListPreservingInvalid(proposal.Aliases)
	proposal.LikelyQuestions = normalizeTextListPreservingInvalid(proposal.LikelyQuestions)
	proposal.RelatedCandidateIDs = sortedStringsPreservingInvalid(proposal.RelatedCandidateIDs)
	proposal.Claims = append([]ProposedClaim(nil), proposal.Claims...)
	for claimIndex := range proposal.Claims {
		claim := &proposal.Claims[claimIndex]
		claim.Title = normalizeText(claim.Title)
		claim.Text = normalizeText(claim.Text)
		claim.SupportIDs = sortedStringsPreservingInvalid(claim.SupportIDs)
		claim.ObservationRefs = append([]ObservationRef(nil), claim.ObservationRefs...)
		claim.MissingRefs = append([]MissingEvidenceRef(nil), claim.MissingRefs...)
		sort.Slice(claim.ObservationRefs, func(i, j int) bool {
			if claim.ObservationRefs[i].TaskID != claim.ObservationRefs[j].TaskID {
				return claim.ObservationRefs[i].TaskID < claim.ObservationRefs[j].TaskID
			}
			return claim.ObservationRefs[i].ObservationIndex < claim.ObservationRefs[j].ObservationIndex
		})
		sort.Slice(claim.MissingRefs, func(i, j int) bool {
			if claim.MissingRefs[i].TaskID != claim.MissingRefs[j].TaskID {
				return claim.MissingRefs[i].TaskID < claim.MissingRefs[j].TaskID
			}
			return claim.MissingRefs[i].MissingIndex < claim.MissingRefs[j].MissingIndex
		})
	}
}

func normalizeStringList(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizeText(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return sortedUnique(normalized)
}

func normalizeTextListPreservingInvalid(values []string) []string {
	result := append([]string(nil), values...)
	for index := range result {
		result[index] = normalizeText(result[index])
	}
	sort.Strings(result)
	return result
}

func ValidateFanInArtifact(bundle Bundle, results []LeafResult, artifact FanInArtifact) error {
	_, context, err := validateLeafResults(bundle, results)
	if err != nil {
		return err
	}
	return validateFanInArtifact(context, artifact, true)
}

// ValidatePartialFanInArtifact accepts a non-empty, unique subset of the
// validated leaf candidates. Every retained proposal still undergoes the
// complete fan-in proposal validation against all supplied leaf results.
func ValidatePartialFanInArtifact(
	bundle Bundle,
	results []LeafResult,
	artifact FanInArtifact,
) error {
	_, context, err := validateLeafResults(bundle, results)
	if err != nil {
		return err
	}
	return validateFanInArtifact(context, artifact, false)
}

func validateFanInArtifact(
	context fanInContext,
	artifact FanInArtifact,
	requireAll bool,
) error {
	if artifact.Version != FanInArtifactVersion {
		return fmt.Errorf("semantic discovery: unsupported fan-in artifact version %d", artifact.Version)
	}
	if requireAll && len(artifact.Artifacts) != len(context.candidates) {
		return fmt.Errorf("semantic discovery: fan-in must return exactly one artifact per candidate")
	}
	if !requireAll && (len(artifact.Artifacts) == 0 || len(artifact.Artifacts) > len(context.candidates)) {
		return fmt.Errorf("semantic discovery: partial fan-in artifact count is invalid")
	}
	seen := make(map[string]struct{}, len(artifact.Artifacts))
	for index, proposal := range artifact.Artifacts {
		if _, duplicate := seen[proposal.CandidateID]; duplicate {
			return fmt.Errorf("semantic discovery: duplicate fan-in candidate id %q", proposal.CandidateID)
		}
		seen[proposal.CandidateID] = struct{}{}
		if err := validateFanInProposal(context, proposal); err != nil {
			return fmt.Errorf("semantic discovery: artifacts[%d]: %w", index, err)
		}
	}
	if requireAll {
		for id := range context.candidates {
			if _, exists := seen[id]; !exists {
				return fmt.Errorf("semantic discovery: fan-in omits candidate %q", id)
			}
		}
	}
	return nil
}

func validateFanInProposal(context fanInContext, proposal ArtifactProposal) error {
	if !validCanonicalVerdict(proposal.Verdict) {
		return fmt.Errorf("unsupported artifact verdict %q", proposal.Verdict)
	}
	_, err := validateFanInProposalContent(context, proposal)
	return err
}

func canonicalizeFanInArtifact(
	context fanInContext,
	artifact FanInArtifact,
) (FanInArtifact, error) {
	canonical := NormalizeFanInArtifact(artifact)
	for index := range canonical.Artifacts {
		if !validCanonicalVerdict(canonical.Artifacts[index].Verdict) {
			return FanInArtifact{}, fmt.Errorf(
				"artifacts[%d]: unsupported artifact verdict %q",
				index,
				canonical.Artifacts[index].Verdict,
			)
		}
		input, err := validateFanInProposalContent(context, canonical.Artifacts[index])
		if err != nil {
			return FanInArtifact{}, fmt.Errorf("artifacts[%d]: %w", index, err)
		}
		canonical.Artifacts[index].Verdict = DeriveVerdict(input)
	}
	return canonical, nil
}

func validateFanInProposalContent(
	context fanInContext,
	proposal ArtifactProposal,
) (VerdictInput, error) {
	candidate, exists := context.candidates[proposal.CandidateID]
	if !exists {
		return VerdictInput{}, fmt.Errorf("unknown candidate id %q", proposal.CandidateID)
	}
	if err := validateProposalMetadata(proposal, context.candidates); err != nil {
		return VerdictInput{}, err
	}

	missingRefs := make(map[string]struct{})
	hasUnresolved := false
	allSupport := make(map[string]struct{})
	allMissingCapabilities := make(map[Capability]struct{})
	seenClaims := make(map[string]struct{}, len(proposal.Claims))
	for index, claim := range proposal.Claims {
		identity := claimIdentity(claim)
		if _, duplicate := seenClaims[identity]; duplicate {
			return VerdictInput{}, fmt.Errorf("claims[%d] duplicates another claim", index)
		}
		seenClaims[identity] = struct{}{}
		claimSupport, missingCapabilities, err := validateFanInClaim(
			context,
			candidate.ID,
			claim,
		)
		if err != nil {
			return VerdictInput{}, fmt.Errorf("claims[%d]: %w", index, err)
		}
		addIDs(allSupport, claimSupport)
		for capability := range missingCapabilities {
			allMissingCapabilities[capability] = struct{}{}
		}
		for _, ref := range claim.MissingRefs {
			missingRefs[missingRefKey(ref)] = struct{}{}
		}
		if claim.Basis == ClaimUnresolved {
			hasUnresolved = true
		}
	}
	if err := validateSummarySupport(
		proposal.Summary,
		factsForKnownIDs(sortedSet(allSupport), context.bundleFacts),
		allMissingCapabilities,
		hasUnresolved,
	); err != nil {
		return VerdictInput{}, err
	}
	if !hasBoundedLexicalOverlap(
		proposal.Summary,
		factsForKnownIDs(candidateScopedSupport(candidate, allSupport), context.bundleFacts),
	) {
		return VerdictInput{}, fmt.Errorf("artifact summary is unrelated to its candidate fact scope")
	}

	result := resultForCandidate(context, candidate.ID)
	if result == nil {
		return VerdictInput{}, fmt.Errorf("candidate has no validated leaf result")
	}
	for index, missing := range result.Artifact.MissingEvidence {
		key := missingRefKey(MissingEvidenceRef{TaskID: result.Task.ID, MissingIndex: index})
		if _, included := missingRefs[key]; included {
			continue
		}
		if !missingResolvedByClaims(context, missing, proposal.Claims) {
			return VerdictInput{}, fmt.Errorf("artifact omits its leaf missing-evidence item %d", index)
		}
	}
	if err := validateIntentProposal(candidate, proposal, context.bundleFacts); err != nil {
		return VerdictInput{}, err
	}
	return verdictInputForProposal(
		proposal,
		candidate,
		result.Artifact.Contradictions,
		len(missingRefs),
		context.bundleFacts,
	), nil
}

func missingResolvedByClaims(
	context fanInContext,
	missing LeafMissingEvidence,
	claims []ProposedClaim,
) bool {
	for _, missingCapability := range missing.MissingCapabilities {
		resolved := false
		for _, claim := range claims {
			if claim.Basis == ClaimUnresolved {
				continue
			}
			facts := factsForKnownIDs(claim.SupportIDs, context.bundleFacts)
			if hasCapabilityOverlap(missing.Explanation, facts, missingCapability) {
				resolved = true
				break
			}
		}
		if !resolved {
			return false
		}
	}
	return len(missing.MissingCapabilities) > 0
}

func validateFanInClaim(
	context fanInContext,
	candidateID string,
	claim ProposedClaim,
) ([]string, map[Capability]struct{}, error) {
	if err := validateModelText("claim title", claim.Title, maxTitleBytes, true); err != nil {
		return nil, nil, err
	}
	if err := validateModelText("claim text", claim.Text, maxModelTextBytes, true); err != nil {
		return nil, nil, err
	}
	if err := validateIDList("claim support ids", claim.SupportIDs, claim.Basis != ClaimUnresolved); err != nil {
		return nil, nil, err
	}
	if _, err := factsForIDs(claim.SupportIDs, context.bundleFacts); err != nil {
		return nil, nil, err
	}
	observationSupport, err := resolveObservationRefs(context, claim.ObservationRefs)
	if err != nil {
		return nil, nil, err
	}
	missingSupport, missingCapabilities, err := resolveMissingRefs(context, claim.MissingRefs)
	if err != nil {
		return nil, nil, err
	}
	expectedSupport := make(map[string]struct{})
	addIDs(expectedSupport, observationSupport)
	addIDs(expectedSupport, missingSupport)
	if !equalStringSets(claim.SupportIDs, sortedSet(expectedSupport)) {
		return nil, nil, fmt.Errorf("claim support ids do not exactly match cited leaf material")
	}
	if err := validateFanInClaimCandidateLineage(context, candidateID, claim); err != nil {
		return nil, nil, err
	}

	facts := factsForKnownIDs(claim.SupportIDs, context.bundleFacts)
	switch claim.Basis {
	case ClaimDirect:
		if len(claim.ObservationRefs) == 0 || len(claim.MissingRefs) != 0 || len(facts) < 1 {
			return nil, nil, fmt.Errorf("direct claim needs observations and at least one fact")
		}
		if err := validateSemanticSupport("direct claim", claim.Text, facts); err != nil {
			return nil, nil, err
		}
	case ClaimCompositional:
		if len(claim.ObservationRefs) == 0 || len(claim.MissingRefs) != 0 || len(facts) < 2 || sourceGroupCount(facts) < 2 {
			return nil, nil, fmt.Errorf("compositional claim needs observations from at least two source groups")
		}
		if err := validateSemanticSupport("compositional claim", claim.Text, facts); err != nil {
			return nil, nil, err
		}
	case ClaimInterpretive:
		if len(claim.ObservationRefs) == 0 || len(claim.MissingRefs) != 0 || len(facts) < 1 {
			return nil, nil, fmt.Errorf("interpretive claim needs observations and at least one fact")
		}
		if err := validateSemanticSupport("interpretive claim", claim.Text, facts); err != nil {
			return nil, nil, err
		}
	case ClaimUnresolved:
		if len(claim.MissingRefs) == 0 || !hasExplicitLimitation(claim.Text) {
			return nil, nil, fmt.Errorf("unresolved claim needs explicit limitation language and missing refs")
		}
		if len(facts) == 0 || !hasBoundedLexicalOverlap(claim.Text, facts) {
			return nil, nil, fmt.Errorf("unresolved claim is unrelated to its cited facts")
		}
		if err := validateMissingAwareSemantics(claim.Text, facts, missingCapabilities, true); err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, fmt.Errorf("unsupported claim basis %q", claim.Basis)
	}
	return sortedSet(expectedSupport), missingCapabilities, nil
}

func validateFanInClaimCandidateLineage(
	context fanInContext,
	candidateID string,
	claim ProposedClaim,
) error {
	refTaskIDs := make([]string, 0, len(claim.ObservationRefs)+len(claim.MissingRefs))
	for _, ref := range claim.ObservationRefs {
		refTaskIDs = append(refTaskIDs, ref.TaskID)
	}
	for _, ref := range claim.MissingRefs {
		refTaskIDs = append(refTaskIDs, ref.TaskID)
	}
	if len(refTaskIDs) == 0 {
		return fmt.Errorf("claim has no candidate leaf lineage")
	}
	ownRefs := 0
	for _, taskID := range refTaskIDs {
		result, exists := context.results[taskID]
		if !exists {
			return fmt.Errorf("claim references unknown leaf task %q", taskID)
		}
		if result.Task.Candidate.ID == candidateID {
			ownRefs++
		}
	}
	if ownRefs == 0 {
		return fmt.Errorf("claim has no reference to its candidate leaf")
	}
	if claim.Basis != ClaimCompositional && claim.Basis != ClaimInterpretive &&
		ownRefs != len(refTaskIDs) {
		return fmt.Errorf("%s claim cannot borrow another candidate's leaf", claim.Basis)
	}
	return nil
}

func resolveObservationRefs(context fanInContext, refs []ObservationRef) ([]string, error) {
	seen := make(map[string]struct{}, len(refs))
	support := make(map[string]struct{})
	for _, ref := range refs {
		if err := validateOpaque("observation task id", ref.TaskID); err != nil {
			return nil, err
		}
		result, exists := context.results[ref.TaskID]
		if !exists {
			return nil, fmt.Errorf("unknown observation task id %q", ref.TaskID)
		}
		if ref.ObservationIndex < 0 || ref.ObservationIndex >= len(result.Artifact.Observations) {
			return nil, fmt.Errorf("unknown observation index for task %q", ref.TaskID)
		}
		key := fmt.Sprintf("%s:%d", ref.TaskID, ref.ObservationIndex)
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("duplicate observation ref %q", key)
		}
		seen[key] = struct{}{}
		addIDs(support, result.Artifact.Observations[ref.ObservationIndex].SupportIDs)
	}
	return sortedSet(support), nil
}

func resolveMissingRefs(
	context fanInContext,
	refs []MissingEvidenceRef,
) ([]string, map[Capability]struct{}, error) {
	seen := make(map[string]struct{}, len(refs))
	support := make(map[string]struct{})
	capabilities := make(map[Capability]struct{})
	for _, ref := range refs {
		if err := validateOpaque("missing-evidence task id", ref.TaskID); err != nil {
			return nil, nil, err
		}
		result, exists := context.results[ref.TaskID]
		if !exists {
			return nil, nil, fmt.Errorf("unknown missing-evidence task id %q", ref.TaskID)
		}
		if ref.MissingIndex < 0 || ref.MissingIndex >= len(result.Artifact.MissingEvidence) {
			return nil, nil, fmt.Errorf("unknown missing-evidence index for task %q", ref.TaskID)
		}
		key := missingRefKey(ref)
		if _, duplicate := seen[key]; duplicate {
			return nil, nil, fmt.Errorf("duplicate missing-evidence ref %q", key)
		}
		seen[key] = struct{}{}
		missing := result.Artifact.MissingEvidence[ref.MissingIndex]
		addIDs(support, missing.SupportIDs)
		for _, capability := range missing.MissingCapabilities {
			capabilities[capability] = struct{}{}
		}
	}
	return sortedSet(support), capabilities, nil
}

func validateMissingAwareSemantics(
	text string,
	facts []Fact,
	missing map[Capability]struct{},
	allowLimitation bool,
) error {
	if behaviorPattern.MatchString(text) {
		supported := factsSupportCapability(facts, CapabilityBehavior)
		explicitlyMissing := missingSupportsCapability(missing, CapabilityBehavior)
		if (!supported || !hasCapabilityOverlap(text, facts, CapabilityBehavior)) && !explicitlyMissing {
			return fmt.Errorf("semantic discovery: prose asserts behavior without support or an explicit behavior gap")
		}
	}
	if sequencePattern.MatchString(text) {
		supported := factsSupportCapability(facts, CapabilitySequence)
		explicitlyMissing := missingSupportsCapability(missing, CapabilitySequence)
		if (!supported || !hasCapabilityOverlap(text, facts, CapabilitySequence)) && !explicitlyMissing {
			return fmt.Errorf("semantic discovery: prose asserts ordering without support or an explicit ordering gap")
		}
	}
	if limitationPattern.MatchString(text) && !allowLimitation {
		if !factsSupportCapability(facts, CapabilityLimitation) ||
			!hasCapabilityOverlap(text, facts, CapabilityLimitation) {
			return fmt.Errorf("semantic discovery: prose asserts a limitation without limitation-capable support")
		}
	}
	if universalPattern.MatchString(text) {
		repositoryScoped := false
		for _, fact := range facts {
			if fact.Scope == FactScopeRepository && hasBoundedLexicalOverlap(text, []Fact{fact}) {
				repositoryScoped = true
				break
			}
		}
		if !repositoryScoped {
			return fmt.Errorf("semantic discovery: repository-wide prose has only bounded support")
		}
	}
	return nil
}

func validateSummarySupport(
	text string,
	facts []Fact,
	missing map[Capability]struct{},
	hasUnresolved bool,
) error {
	if len(facts) == 0 || !hasBoundedLexicalOverlap(text, facts) {
		return fmt.Errorf("semantic discovery: artifact summary is unrelated to claim support")
	}
	return validateMissingAwareSemantics(text, facts, missing, hasUnresolved)
}

func validateLeafResults(bundle Bundle, results []LeafResult) ([]LeafResult, fanInContext, error) {
	context := fanInContext{
		bundleFacts: factIndex(bundle),
		results:     make(map[string]LeafResult),
		candidates:  make(map[string]OpportunityCandidate),
	}
	if err := bundle.Validate(); err != nil {
		return nil, context, err
	}
	if len(results) == 0 || len(results) > MaxSelectedCandidates {
		return nil, context, fmt.Errorf("semantic discovery: fan-in needs between 1 and %d leaf results", MaxSelectedCandidates)
	}
	validated := append([]LeafResult(nil), results...)
	for index, result := range validated {
		if err := result.Task.Validate(); err != nil {
			return nil, context, fmt.Errorf("semantic discovery: leaves[%d] task: %w", index, err)
		}
		planned, err := PlanLeafTasks(bundle, []OpportunityCandidate{result.Task.Candidate})
		if err != nil {
			return nil, context, err
		}
		expectedHash, _, err := LeafTaskHash(planned[0])
		if err != nil {
			return nil, context, err
		}
		actualHash, _, err := LeafTaskHash(result.Task)
		if err != nil {
			return nil, context, err
		}
		if expectedHash != actualHash {
			return nil, context, fmt.Errorf("semantic discovery: leaf task does not match current bundle")
		}
		if err := ValidateLeafArtifact(result.Task, result.Artifact); err != nil {
			return nil, context, fmt.Errorf("semantic discovery: leaves[%d] artifact: %w", index, err)
		}
		if _, duplicate := context.results[result.Task.ID]; duplicate {
			return nil, context, fmt.Errorf("semantic discovery: duplicate leaf task %q", result.Task.ID)
		}
		if _, duplicate := context.candidates[result.Task.Candidate.ID]; duplicate {
			return nil, context, fmt.Errorf("semantic discovery: duplicate leaf candidate %q", result.Task.Candidate.ID)
		}
		context.results[result.Task.ID] = result
		context.candidates[result.Task.Candidate.ID] = result.Task.Candidate
	}
	sort.Slice(validated, func(i, j int) bool { return validated[i].Task.ID < validated[j].Task.ID })
	return validated, context, nil
}

func resultForCandidate(context fanInContext, candidateID string) *LeafResult {
	for _, result := range context.results {
		if result.Task.Candidate.ID == candidateID {
			copy := result
			return &copy
		}
	}
	return nil
}

func factsForKnownIDs(ids []string, known map[string]Fact) []Fact {
	facts := make([]Fact, 0, len(ids))
	for _, id := range ids {
		if fact, exists := known[id]; exists {
			facts = append(facts, fact)
		}
	}
	return facts
}

func missingRefKey(ref MissingEvidenceRef) string {
	return fmt.Sprintf("%s:%d", ref.TaskID, ref.MissingIndex)
}

// UnsupportedClaimCount counts claim-level lineage, lexical, capability, and
// source-group failures without making an otherwise rejected response usable.
func UnsupportedClaimCount(bundle Bundle, results []LeafResult, artifact FanInArtifact) int {
	_, context, err := validateLeafResults(bundle, results)
	if err != nil {
		count := 0
		for _, proposal := range artifact.Artifacts {
			count += len(proposal.Claims)
		}
		return count
	}
	count := 0
	for _, proposal := range artifact.Artifacts {
		if _, exists := context.candidates[proposal.CandidateID]; !exists {
			count += len(proposal.Claims)
			continue
		}
		for _, claim := range proposal.Claims {
			if _, _, err := validateFanInClaim(context, proposal.CandidateID, claim); err != nil {
				count++
			}
		}
	}
	return count
}

func joinUnknowns(values ...[]string) []string {
	combined := []string{}
	for _, group := range values {
		combined = append(combined, group...)
	}
	return normalizeStringList(combined)
}

func claimIdentity(claim ProposedClaim) string {
	return stableID(
		"semantic-claim",
		string(claim.Basis),
		strings.ToLower(normalizeText(claim.Text)),
		strings.Join(sortedUnique(claim.SupportIDs), "\x00"),
	)
}
