package semanticdiscovery

import (
	"fmt"
	"sort"
	"strings"
)

const monolithicSystemPrefix = `You are a semantic editor for a bounded repository model. The supplied facts are the complete authoritative input. Return valid JSON only. Use only supplied opaque IDs. Do not create or quote repository paths, file names, symbols, flows, relations, evidence, focus IDs, runtime order, or IDs.`

const monolithicUserPrefix = `Produce the same artifact proposal contract as fan-in:
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
          "observation_refs": [],
          "missing_refs": []
        }
      ],
      "aliases": ["short search phrase"],
      "likely_questions": ["question this artifact answers"],
      "related_candidate_ids": ["other exact selected candidate ids"]
    }
  ]
}

Rules:
- The top-level object must contain numeric "version": 1 and the "artifacts" array directly. Do not wrap, rename, or omit either field.
- Return exactly one artifact for every selected candidate.
- Each selected candidate includes its exact support_ids. direct and unresolved claims may cite only that candidate's support_ids. An interpretive or compositional claim may also cite another selected candidate's facts, but it must include at least one support_id from its own candidate.
- observation_refs and missing_refs must be empty; this baseline works directly from original facts.
- Every support_id must be an exact supplied fact ID. Existing IDs alone are not proof: prose must overlap the supporting statements and behavioral or ordering language needs matching fact capability.
- direct and interpretive claims require at least one supporting fact.
- compositional claims require at least two facts from at least two independent source groups.
- unresolved claims require at least one limitation-capable fact and explicit limitation language.
- related_candidate_ids are navigation hints only and never evidence.
- Do not mention paths, file names, symbols, source locations, or create repository objects in prose.`

const monolithicPayloadMarker = "\n\nVariable selected candidates and original local fact JSON:\n"

type monolithicPromptPayload struct {
	Version      int                        `json:"version"`
	BundleSHA256 string                     `json:"bundle_sha256"`
	RepoName     string                     `json:"repo_name"`
	Candidates   []monolithicCandidateScope `json:"candidates"`
	Facts        []Fact                     `json:"facts"`
}

type monolithicCandidateScope struct {
	ID         string       `json:"id"`
	Kind       ArtifactKind `json:"artifact_kind"`
	SupportIDs []string     `json:"support_ids"`
}

type monolithicContext struct {
	bundleFacts  map[string]Fact
	allowedFacts map[string]Fact
	candidates   map[string]OpportunityCandidate
}

func BuildMonolithicPrompt(bundle Bundle, candidates []OpportunityCandidate) (Prompt, error) {
	payload, err := buildMonolithicPayload(bundle, candidates)
	if err != nil {
		return Prompt{}, err
	}
	_, encoded, err := hashJSON("monolithic input", payload)
	if err != nil {
		return Prompt{}, err
	}
	if len(encoded) > maxRecordBytes {
		return Prompt{}, fmt.Errorf("semantic discovery: monolithic payload is too large")
	}
	return Prompt{
		Version:         MonolithicPromptVersion,
		System:          monolithicSystemPrefix,
		User:            monolithicUserPrefix + monolithicPayloadMarker + string(encoded),
		ThinkingProfile: ThinkingMax,
		ProgressLabel:   "semantic monolithic baseline",
	}, nil
}

func buildMonolithicPayload(
	bundle Bundle,
	candidates []OpportunityCandidate,
) (monolithicPromptPayload, error) {
	context, err := validateMonolithicCandidates(bundle, candidates)
	if err != nil {
		return monolithicPromptPayload{}, err
	}
	bundleHash, _, err := BundleHash(bundle)
	if err != nil {
		return monolithicPromptPayload{}, err
	}
	payload := monolithicPromptPayload{
		Version:      1,
		BundleSHA256: bundleHash,
		RepoName:     bundle.RepoName,
		Candidates:   make([]monolithicCandidateScope, 0, len(candidates)),
	}
	for _, candidate := range candidates {
		payload.Candidates = append(payload.Candidates, monolithicCandidateScope{
			ID: candidate.ID, Kind: candidate.Kind,
			SupportIDs: candidateFactIDs(candidate),
		})
	}
	sort.Slice(payload.Candidates, func(i, j int) bool {
		return payload.Candidates[i].ID < payload.Candidates[j].ID
	})
	for _, id := range sortedFactMapIDs(context.allowedFacts) {
		payload.Facts = append(payload.Facts, context.allowedFacts[id])
	}
	payload.Facts = modelFacts(payload.Facts)
	return payload, nil
}

func ParseMonolithicArtifact(raw []byte) (FanInArtifact, error) {
	return ParseFanInArtifact(raw)
}

func NormalizeMonolithicArtifact(artifact FanInArtifact) FanInArtifact {
	return NormalizeFanInArtifact(artifact)
}

func ValidateMonolithicArtifact(
	bundle Bundle,
	candidates []OpportunityCandidate,
	artifact FanInArtifact,
) error {
	context, err := validateMonolithicCandidates(bundle, candidates)
	if err != nil {
		return err
	}
	return validateMonolithicArtifact(context, artifact, true)
}

// ValidatePartialMonolithicArtifact accepts a non-empty, unique subset of
// selected candidates. Every retained proposal still undergoes the complete
// monolithic proposal validation against the original local facts.
func ValidatePartialMonolithicArtifact(
	bundle Bundle,
	candidates []OpportunityCandidate,
	artifact FanInArtifact,
) error {
	context, err := validateMonolithicCandidates(bundle, candidates)
	if err != nil {
		return err
	}
	return validateMonolithicArtifact(context, artifact, false)
}

func validateMonolithicArtifact(
	context monolithicContext,
	artifact FanInArtifact,
	requireAll bool,
) error {
	if artifact.Version != FanInArtifactVersion {
		return fmt.Errorf("semantic discovery: unsupported monolithic artifact version %d", artifact.Version)
	}
	if requireAll && len(artifact.Artifacts) != len(context.candidates) {
		return fmt.Errorf("semantic discovery: monolithic response must contain one artifact per candidate")
	}
	if !requireAll && (len(artifact.Artifacts) == 0 || len(artifact.Artifacts) > len(context.candidates)) {
		return fmt.Errorf("semantic discovery: partial monolithic artifact count is invalid")
	}
	seen := make(map[string]struct{}, len(artifact.Artifacts))
	for index, proposal := range artifact.Artifacts {
		if _, duplicate := seen[proposal.CandidateID]; duplicate {
			return fmt.Errorf("semantic discovery: duplicate monolithic candidate id %q", proposal.CandidateID)
		}
		seen[proposal.CandidateID] = struct{}{}
		if err := validateMonolithicProposal(context, proposal); err != nil {
			return fmt.Errorf("semantic discovery: artifacts[%d]: %w", index, err)
		}
	}
	if requireAll {
		for id := range context.candidates {
			if _, exists := seen[id]; !exists {
				return fmt.Errorf("semantic discovery: monolithic response omits candidate %q", id)
			}
		}
	}
	return nil
}

func validateMonolithicProposal(context monolithicContext, proposal ArtifactProposal) error {
	if !validCanonicalVerdict(proposal.Verdict) {
		return fmt.Errorf("unsupported artifact verdict %q", proposal.Verdict)
	}
	_, err := validateMonolithicProposalContent(context, proposal)
	return err
}

func validateMonolithicProposalContent(
	context monolithicContext,
	proposal ArtifactProposal,
) (VerdictInput, error) {
	candidate, exists := context.candidates[proposal.CandidateID]
	if !exists {
		return VerdictInput{}, fmt.Errorf("unknown candidate id %q", proposal.CandidateID)
	}
	if err := validateProposalMetadata(proposal, context.candidates); err != nil {
		return VerdictInput{}, err
	}
	allSupport := make(map[string]struct{})
	seenClaims := make(map[string]struct{}, len(proposal.Claims))
	for index, claim := range proposal.Claims {
		identity := claimIdentity(claim)
		if _, duplicate := seenClaims[identity]; duplicate {
			return VerdictInput{}, fmt.Errorf("claims[%d] duplicates another claim", index)
		}
		seenClaims[identity] = struct{}{}
		if err := validateMonolithicClaim(context, candidate, claim); err != nil {
			return VerdictInput{}, fmt.Errorf("claims[%d]: %w", index, err)
		}
		addIDs(allSupport, claim.SupportIDs)
	}
	if err := validateSemanticSupport(
		"artifact summary",
		proposal.Summary,
		factsForKnownIDs(sortedSet(allSupport), context.allowedFacts),
	); err != nil {
		return VerdictInput{}, err
	}
	if !hasBoundedLexicalOverlap(
		proposal.Summary,
		factsForKnownIDs(candidateScopedSupport(candidate, allSupport), context.allowedFacts),
	) {
		return VerdictInput{}, fmt.Errorf("artifact summary is unrelated to its candidate fact scope")
	}
	if err := validateIntentProposal(candidate, proposal, context.allowedFacts); err != nil {
		return VerdictInput{}, err
	}
	return verdictInputForProposal(proposal, candidate, nil, 0, context.allowedFacts), nil
}

func validateMonolithicClaim(
	context monolithicContext,
	candidate OpportunityCandidate,
	claim ProposedClaim,
) error {
	if err := validateModelText("claim title", claim.Title, maxTitleBytes, true); err != nil {
		return err
	}
	if err := validateModelText("claim text", claim.Text, maxModelTextBytes, true); err != nil {
		return err
	}
	if len(claim.ObservationRefs) != 0 || len(claim.MissingRefs) != 0 {
		return fmt.Errorf("monolithic claim cannot contain leaf references")
	}
	if err := validateIDList("claim support ids", claim.SupportIDs, true); err != nil {
		return err
	}
	facts, err := factsForIDs(claim.SupportIDs, context.allowedFacts)
	if err != nil {
		return err
	}
	candidateSupport := make(map[string]struct{}, len(candidateFactIDs(candidate)))
	addIDs(candidateSupport, candidateFactIDs(candidate))
	ownSupport := 0
	for _, id := range claim.SupportIDs {
		if _, exists := candidateSupport[id]; exists {
			ownSupport++
		}
	}
	if ownSupport == 0 {
		return fmt.Errorf("claim has no support from its candidate fact scope")
	}
	switch claim.Basis {
	case ClaimDirect:
		if len(facts) < 1 {
			return fmt.Errorf("%s claim needs at least one fact", claim.Basis)
		}
		if ownSupport != len(claim.SupportIDs) {
			return fmt.Errorf("%s claim cannot borrow another candidate's fact scope", claim.Basis)
		}
	case ClaimInterpretive:
		if len(facts) < 1 {
			return fmt.Errorf("%s claim needs at least one fact", claim.Basis)
		}
	case ClaimCompositional:
		if len(facts) < 2 || sourceGroupCount(facts) < 2 {
			return fmt.Errorf("compositional claim needs facts from at least two source groups")
		}
	case ClaimUnresolved:
		if !hasExplicitLimitation(claim.Text) {
			return fmt.Errorf("unresolved claim needs explicit limitation language")
		}
		if ownSupport != len(claim.SupportIDs) {
			return fmt.Errorf("unresolved claim cannot borrow another candidate's fact scope")
		}
	default:
		return fmt.Errorf("unsupported claim basis %q", claim.Basis)
	}
	return validateSemanticSupport(string(claim.Basis)+" claim", claim.Text, facts)
}

func validateProposalMetadata(
	proposal ArtifactProposal,
	candidates map[string]OpportunityCandidate,
) error {
	if err := validateModelText("artifact title", proposal.Title, maxTitleBytes, true); err != nil {
		return err
	}
	if err := validateModelText("artifact summary", proposal.Summary, maxModelTextBytes, true); err != nil {
		return err
	}
	if len(proposal.Claims) == 0 || len(proposal.Claims) > maxClaimsPerArtifact {
		return fmt.Errorf("artifact claim count must be between 1 and %d", maxClaimsPerArtifact)
	}
	if len(proposal.Aliases) > maxAliasesPerArtifact || len(proposal.LikelyQuestions) > maxQuestionsPerArtifact {
		return fmt.Errorf("artifact metadata exceeds item limits")
	}
	seenAliases := make(map[string]struct{}, len(proposal.Aliases))
	for _, alias := range proposal.Aliases {
		if err := validateModelText("artifact alias", alias, maxTitleBytes, true); err != nil {
			return err
		}
		if _, duplicate := seenAliases[alias]; duplicate {
			return fmt.Errorf("artifact repeats alias %q", alias)
		}
		seenAliases[alias] = struct{}{}
	}
	seenQuestions := make(map[string]struct{}, len(proposal.LikelyQuestions))
	for _, question := range proposal.LikelyQuestions {
		if err := validateModelText("artifact likely question", question, maxQuestionBytes, true); err != nil {
			return err
		}
		if _, duplicate := seenQuestions[question]; duplicate {
			return fmt.Errorf("artifact repeats likely question %q", question)
		}
		seenQuestions[question] = struct{}{}
	}
	if err := validateIDList("related candidate ids", proposal.RelatedCandidateIDs, false); err != nil {
		return err
	}
	for _, id := range proposal.RelatedCandidateIDs {
		if id == proposal.CandidateID {
			return fmt.Errorf("artifact cannot relate to itself")
		}
		if _, exists := candidates[id]; !exists {
			return fmt.Errorf("unknown related candidate id %q", id)
		}
	}
	return nil
}

func candidateScopedSupport(
	candidate OpportunityCandidate,
	support map[string]struct{},
) []string {
	result := make([]string, 0, len(candidateFactIDs(candidate)))
	for _, id := range candidateFactIDs(candidate) {
		if _, exists := support[id]; exists {
			result = append(result, id)
		}
	}
	return sortedUnique(result)
}

func validateMonolithicCandidates(
	bundle Bundle,
	candidates []OpportunityCandidate,
) (monolithicContext, error) {
	context := monolithicContext{
		bundleFacts:  factIndex(bundle),
		allowedFacts: make(map[string]Fact),
		candidates:   make(map[string]OpportunityCandidate),
	}
	if err := bundle.Validate(); err != nil {
		return context, err
	}
	if len(candidates) == 0 || len(candidates) > MaxSelectedCandidates {
		return context, fmt.Errorf("semantic discovery: monolithic stage needs between 1 and %d candidates", MaxSelectedCandidates)
	}
	for index, candidate := range candidates {
		if err := validateOpportunityCandidate(candidate, context.bundleFacts); err != nil {
			return context, fmt.Errorf("semantic discovery: candidates[%d]: %w", index, err)
		}
		if _, duplicate := context.candidates[candidate.ID]; duplicate {
			return context, fmt.Errorf("semantic discovery: duplicate monolithic candidate %q", candidate.ID)
		}
		context.candidates[candidate.ID] = candidate
		for _, id := range candidateFactIDs(candidate) {
			context.allowedFacts[id] = context.bundleFacts[id]
		}
	}
	return context, nil
}

func sortedFactMapIDs(facts map[string]Fact) []string {
	ids := make([]string, 0, len(facts))
	for id := range facts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// UnsupportedMonolithicClaimCount counts strict claim failures even when the
// complete monolithic response is rejected.
func UnsupportedMonolithicClaimCount(
	bundle Bundle,
	candidates []OpportunityCandidate,
	artifact FanInArtifact,
) int {
	context, err := validateMonolithicCandidates(bundle, candidates)
	if err != nil {
		count := 0
		for _, proposal := range artifact.Artifacts {
			count += len(proposal.Claims)
		}
		return count
	}
	count := 0
	for _, proposal := range artifact.Artifacts {
		candidate, exists := context.candidates[proposal.CandidateID]
		if !exists {
			count += len(proposal.Claims)
			continue
		}
		for _, claim := range proposal.Claims {
			if err := validateMonolithicClaim(context, candidate, claim); err != nil {
				count++
			}
		}
	}
	return count
}

// MaterializeMonolithicArtifacts resolves navigation and IDs from local facts
// while preserving the baseline's direct fact lineage.
func MaterializeMonolithicArtifacts(
	bundle Bundle,
	candidates []OpportunityCandidate,
	artifact FanInArtifact,
) ([]Artifact, error) {
	return materializeMonolithicArtifacts(bundle, candidates, artifact, true)
}

// MaterializePartialMonolithicArtifacts resolves a validated subset of
// monolithic proposals. Relations to candidates absent from the subset are
// omitted rather than materialized as dangling artifact IDs.
func MaterializePartialMonolithicArtifacts(
	bundle Bundle,
	candidates []OpportunityCandidate,
	artifact FanInArtifact,
) ([]Artifact, error) {
	return materializeMonolithicArtifacts(bundle, candidates, artifact, false)
}

func materializeMonolithicArtifacts(
	bundle Bundle,
	candidates []OpportunityCandidate,
	artifact FanInArtifact,
	requireAll bool,
) ([]Artifact, error) {
	context, err := validateMonolithicCandidates(bundle, candidates)
	if err != nil {
		return nil, err
	}
	if err := validateMonolithicArtifact(context, artifact, requireAll); err != nil {
		return nil, err
	}
	normalized := NormalizeMonolithicArtifact(artifact)
	for index := range normalized.Artifacts {
		input, err := validateMonolithicProposalContent(context, normalized.Artifacts[index])
		if err != nil {
			return nil, err
		}
		normalized.Artifacts[index].Verdict = DeriveVerdict(input)
	}
	artifactIDs := make(map[string]string, len(normalized.Artifacts))
	for _, proposal := range normalized.Artifacts {
		artifactIDs[proposal.CandidateID] = stableID("semantic-artifact", proposal.CandidateID)
	}
	result := make([]Artifact, 0, len(normalized.Artifacts))
	for _, proposal := range normalized.Artifacts {
		candidate := context.candidates[proposal.CandidateID]
		item := Artifact{
			Version:         ArtifactVersion,
			ID:              artifactIDs[proposal.CandidateID],
			CandidateID:     candidate.ID,
			Kind:            candidate.Kind,
			Title:           proposal.Title,
			Summary:         proposal.Summary,
			Question:        materializedQuestion(candidate),
			Verdict:         proposal.Verdict,
			Aliases:         append([]string(nil), proposal.Aliases...),
			LikelyQuestions: append([]string(nil), proposal.LikelyQuestions...),
			Confidence:      materializedConfidence(candidate.Confidence, proposal.Verdict),
		}
		allSupport := make(map[string]struct{})
		for _, claim := range proposal.Claims {
			facts := factsForKnownIDs(claim.SupportIDs, context.allowedFacts)
			groups := make([]string, 0, len(facts))
			for _, fact := range facts {
				groups = append(groups, fact.SourceGroup)
			}
			statementID := stableID(
				"semantic-statement",
				item.ID,
				string(claim.Basis),
				claim.Text,
				strings.Join(claim.SupportIDs, "\x00"),
			)
			focus, evidence := navigationForFacts(facts)
			item.Statements = append(item.Statements, Statement{
				ID: statementID, Text: claim.Text, Basis: claim.Basis,
				SupportIDs:   append([]string(nil), claim.SupportIDs...),
				SourceGroups: sortedUnique(groups),
			})
			item.Steps = append(item.Steps, Step{
				ID:    stableID("semantic-step", item.ID, statementID),
				Title: claim.Title, Explanation: claim.Text,
				StatementIDs: []string{statementID}, Focus: focus, Evidence: evidence,
			})
			addIDs(allSupport, claim.SupportIDs)
			if claim.Basis == ClaimUnresolved {
				item.Unknowns = append(item.Unknowns, claim.Text)
			}
		}
		item.Unknowns = joinUnknowns(item.Unknowns)
		item.UsedFactIDs = sortedSet(allSupport)
		for _, id := range candidateFactIDs(candidate) {
			if _, used := allSupport[id]; !used {
				item.UnusedAvailableFactIDs = append(
					item.UnusedAvailableFactIDs,
					id,
				)
			}
		}
		item.UnusedAvailableFactIDs = sortedUnique(item.UnusedAvailableFactIDs)
		item.Focus, item.Evidence = navigationForFacts(
			factsForKnownIDs(sortedSet(allSupport), context.allowedFacts),
		)
		for _, relatedCandidateID := range proposal.RelatedCandidateIDs {
			if relatedArtifactID, exists := artifactIDs[relatedCandidateID]; exists {
				item.RelatedArtifactIDs = append(item.RelatedArtifactIDs, relatedArtifactID)
			}
		}
		item.RelatedArtifactIDs = sortedUnique(item.RelatedArtifactIDs)
		applyIntentMaterialization(&item, candidate, proposal.Claims, context.allowedFacts)
		if err := validateMaterializedArtifact(item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}
