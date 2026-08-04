package semanticdiscovery

import (
	"fmt"
	"sort"
	"strings"
)

func ParseOpportunityProposal(raw []byte) (OpportunityProposal, error) {
	var proposal OpportunityProposal
	if err := decodeStrict(raw, &proposal, maxProposalBytes); err != nil {
		return OpportunityProposal{}, fmt.Errorf("semantic discovery: invalid opportunity proposal json: %w", err)
	}
	return proposal, nil
}

// NormalizeOpportunityProposal removes unknown support IDs and malformed
// candidates while recording every reduction. Unknown IDs never become an
// accepted candidate, and candidates left without support are dropped.
func NormalizeOpportunityProposal(
	bundle Bundle,
	proposal OpportunityProposal,
) (OpportunityProposal, NormalizationReport) {
	report := NormalizationReport{}
	known := factIndex(bundle)
	result := OpportunityProposal{Version: proposal.Version, Candidates: []OpportunityCandidate{}}
	for index, candidate := range proposal.Candidates {
		candidate.Title = normalizeText(candidate.Title)
		candidate.QuestionAnswered = normalizeText(candidate.QuestionAnswered)
		if !validArtifactKind(candidate.Kind) || !validExpectedValue(candidate.ExpectedValue) ||
			!validConfidence(candidate.Confidence) {
			report.Issues = append(report.Issues, NormalizationIssue{
				CandidateIndex: index, Code: "invalid_candidate_enum",
			})
			continue
		}
		if validateModelText("opportunity title", candidate.Title, maxTitleBytes, true) != nil ||
			validateModelText("opportunity question", candidate.QuestionAnswered, maxQuestionBytes, true) != nil {
			report.Issues = append(report.Issues, NormalizationIssue{
				CandidateIndex: index, Code: "repository_bearing_or_malformed_prose",
			})
			continue
		}

		support := sortedUnique(candidate.SupportIDs)
		candidate.SupportIDs = candidate.SupportIDs[:0]
		for _, id := range support {
			if _, exists := known[id]; !exists {
				report.Issues = append(report.Issues, NormalizationIssue{
					CandidateIndex: index, Code: "unknown_support_id", Detail: id,
				})
				continue
			}
			candidate.SupportIDs = append(candidate.SupportIDs, id)
		}
		if len(candidate.SupportIDs) == 0 {
			report.Issues = append(report.Issues, NormalizationIssue{
				CandidateIndex: index, Code: "candidate_without_known_support",
			})
			continue
		}
		initialSupport := make(map[string]struct{}, len(candidate.SupportIDs))
		addIDs(initialSupport, candidate.SupportIDs)
		enrichment := sortedUnique(candidate.EnrichmentSupportIDs)
		candidate.EnrichmentSupportIDs = candidate.EnrichmentSupportIDs[:0]
		for _, id := range enrichment {
			if _, duplicate := initialSupport[id]; duplicate {
				report.Issues = append(report.Issues, NormalizationIssue{
					CandidateIndex: index, Code: "duplicate_enrichment_support_id", Detail: id,
				})
				continue
			}
			if _, exists := known[id]; !exists {
				report.Issues = append(report.Issues, NormalizationIssue{
					CandidateIndex: index, Code: "unknown_enrichment_support_id", Detail: id,
				})
				continue
			}
			candidate.EnrichmentSupportIDs = append(candidate.EnrichmentSupportIDs, id)
		}
		if !opportunityKindGrounded(candidate, known) {
			report.Issues = append(report.Issues, NormalizationIssue{
				CandidateIndex: index, Code: "candidate_kind_not_locally_grounded",
			})
			continue
		}
		if candidate.CapabilityContract != nil {
			derived := deriveOpportunityCapabilityContract(candidate.CapabilityContract, known, candidateFactIDs(candidate))
			if !sameCapabilityContract(candidate.CapabilityContract, derived) {
				report.Issues = append(report.Issues, NormalizationIssue{
					CandidateIndex: index, Code: "capability_contract_derived",
				})
			}
			candidate.CapabilityContract = derived
		}
		candidate.ProductIntent = normalizeOpportunityExpectationSupport(
			candidate.ProductIntent,
			known,
			candidate.SupportIDs,
			index,
			&report,
		)
		candidate = canonicalOpportunityCandidate(candidate)
		if err := validateOpportunityProductIntent(candidate, known); err != nil {
			report.Issues = append(report.Issues, NormalizationIssue{
				CandidateIndex: index, Code: "invalid_product_intent",
				Detail: boundedNormalizationDetail(err.Error()),
			})
			// ProductIntent is optional planning metadata. Discard the whole
			// invalid object without discarding an otherwise grounded candidate.
			candidate.ProductIntent = nil
		}

		missing := make([]string, 0, len(candidate.MissingInformation))
		for _, item := range candidate.MissingInformation {
			item = normalizeText(item)
			if item == "" {
				continue
			}
			if err := validateModelText("opportunity missing information", item, maxModelTextBytes, true); err != nil {
				report.Issues = append(report.Issues, NormalizationIssue{
					CandidateIndex: index, Code: "invalid_missing_information",
				})
				continue
			}
			missing = append(missing, item)
		}
		candidate.MissingInformation = sortedUnique(missing)
		candidate.ID = opportunityID(candidate)
		result.Candidates = append(result.Candidates, candidate)
	}

	result.Candidates = deduplicateOpportunities(result.Candidates, &report)
	if len(result.Candidates) > MaxOpportunityCandidates {
		result.Candidates = result.Candidates[:MaxOpportunityCandidates]
		report.Issues = append(report.Issues, NormalizationIssue{
			CandidateIndex: -1, Code: "candidate_limit_applied",
		})
	}
	return result, report
}

// normalizeOpportunityExpectationSupport keeps an answer-shape proposal usable
// when the model assigns a real local fact to a capability that fact does not
// establish. Expected-path support is a planning hint, not a semantic claim:
// clearing the unsupported binding preserves the missing capability for the
// bounded planner while preventing the fact from masquerading as proof.
func normalizeOpportunityExpectationSupport(
	intent *OpportunityProductIntent,
	known map[string]Fact,
	candidateSupportIDs []string,
	candidateIndex int,
	report *NormalizationReport,
) *OpportunityProductIntent {
	if intent == nil {
		return nil
	}
	result := *intent
	allowed := make(map[string]struct{}, len(candidateSupportIDs))
	addIDs(allowed, candidateSupportIDs)
	architectureIDs := make([]string, 0, len(result.ArchitectureAreaAnchorIDs))
	droppedArchitectureIDs := make([]string, 0)
	for _, id := range result.ArchitectureAreaAnchorIDs {
		if _, ok := allowed[id]; ok {
			architectureIDs = append(architectureIDs, id)
			continue
		}
		droppedArchitectureIDs = append(droppedArchitectureIDs, id)
	}
	result.ArchitectureAreaAnchorIDs = architectureIDs
	if len(droppedArchitectureIDs) > 0 {
		report.Issues = append(report.Issues, NormalizationIssue{
			CandidateIndex: candidateIndex,
			Code:           "architecture_anchor_support_reduced",
			Detail:         strings.Join(sortedUnique(droppedArchitectureIDs), ","),
		})
	}
	expectations := []struct {
		label string
		value *OpportunityExpectation
	}{
		{label: "input_trigger", value: &result.ExpectedPath.InputTrigger},
		{label: "core_work", value: &result.ExpectedPath.CoreWork},
		{label: "observable_effect", value: &result.ExpectedPath.ObservableEffect},
	}
	for _, item := range expectations {
		expectation := item.value
		if len(expectation.SupportIDs) == 0 {
			continue
		}
		facts := factsForKnownIDs(expectation.SupportIDs, known)
		unsupported := make([]Capability, 0, len(expectation.RequiredCapabilities))
		for _, capability := range expectation.RequiredCapabilities {
			if !factsSupportCapability(facts, capability) {
				unsupported = append(unsupported, capability)
			}
		}
		if len(unsupported) == 0 {
			continue
		}
		expectation.SupportIDs = nil
		report.Issues = append(report.Issues, NormalizationIssue{
			CandidateIndex: candidateIndex,
			Code:           "expected_path_support_reduced",
			Detail: item.label + ": " + strings.Join(
				capabilityStrings(canonicalCapabilities(unsupported)),
				",",
			),
		})
	}
	return &result
}

func deriveOpportunityCapabilityContract(
	proposed *CapabilityContract,
	known map[string]Fact,
	factIDs []string,
) *CapabilityContract {
	if proposed == nil {
		return nil
	}
	required := canonicalCapabilities(proposed.RequiredCapabilities)
	facts := factsForKnownIDs(factIDs, known)
	derived := &CapabilityContract{RequiredCapabilities: required}
	for _, capability := range required {
		if factsSupportCapability(facts, capability) {
			derived.AvailableCapabilities = append(derived.AvailableCapabilities, capability)
		} else {
			derived.MissingCapabilities = append(derived.MissingCapabilities, capability)
		}
	}
	switch {
	case len(derived.MissingCapabilities) == 0:
		derived.Resolution = CapabilityResolutionReady
	case len(derived.AvailableCapabilities) == 0:
		derived.Resolution = CapabilityResolutionInsufficientEvidence
	default:
		derived.Resolution = CapabilityResolutionPartial
	}
	return derived
}

func sameCapabilityContract(left *CapabilityContract, right *CapabilityContract) bool {
	if left == nil || right == nil {
		return left == right
	}
	return sameCapabilities(left.RequiredCapabilities, right.RequiredCapabilities) &&
		sameCapabilities(left.AvailableCapabilities, right.AvailableCapabilities) &&
		sameCapabilities(left.MissingCapabilities, right.MissingCapabilities) &&
		left.Resolution == right.Resolution
}

func sameCapabilities(left []Capability, right []Capability) bool {
	left = canonicalCapabilities(left)
	right = canonicalCapabilities(right)
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

func opportunityKindGrounded(candidate OpportunityCandidate, known map[string]Fact) bool {
	facts := factsForKnownIDs(candidateFactIDs(candidate), known)
	hasBehavior := false
	hasSequence := false
	hasLimitation := false
	kinds := make(map[FactKind]struct{})
	for _, fact := range facts {
		kinds[fact.Kind] = struct{}{}
		for _, capability := range fact.Capabilities {
			if isBehaviorCapability(capability) {
				hasBehavior = true
			}
			if capability == CapabilitySequence || capability == CapabilityLifecycle {
				hasSequence = true
			}
			if capability == CapabilityLimitation {
				hasLimitation = true
			}
		}
	}
	hasKind := func(kind FactKind) bool {
		_, exists := kinds[kind]
		return exists
	}
	switch candidate.Kind {
	case ArtifactMechanism:
		return hasBehavior || hasSequence
	case ArtifactDependencyUsage:
		return (hasKind(FactDependency) || hasKind(FactPackageImport)) &&
			(hasBehavior || hasLimitation)
	case ArtifactRepositoryPattern:
		return len(facts) >= 2
	case ArtifactContributionGuide:
		return hasKind(FactTestReference) &&
			(hasBehavior || hasSequence || hasKind(FactSourceSignal))
	case ArtifactGoLearning:
		return hasKind(FactSourceSignal) || hasKind(FactFlowStep) || hasKind(FactTestReference)
	case ArtifactRepositoryStory:
		return hasSequence
	default:
		return false
	}
}

func ValidateOpportunityProposal(bundle Bundle, proposal OpportunityProposal) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	if proposal.Version != OpportunityProposalVersion {
		return fmt.Errorf("semantic discovery: unsupported opportunity proposal version %d", proposal.Version)
	}
	if len(proposal.Candidates) == 0 || len(proposal.Candidates) > MaxOpportunityCandidates {
		return fmt.Errorf("semantic discovery: opportunity candidate count must be between 1 and %d", MaxOpportunityCandidates)
	}
	known := factIndex(bundle)
	seen := make(map[string]struct{}, len(proposal.Candidates))
	for index, candidate := range proposal.Candidates {
		if err := validateOpportunityCandidate(candidate, known); err != nil {
			return fmt.Errorf("semantic discovery: candidates[%d]: %w", index, err)
		}
		if _, duplicate := seen[candidate.ID]; duplicate {
			return fmt.Errorf("semantic discovery: duplicate opportunity id %q", candidate.ID)
		}
		seen[candidate.ID] = struct{}{}
	}
	return nil
}

func validateOpportunityCandidate(candidate OpportunityCandidate, known map[string]Fact) error {
	if !validArtifactKind(candidate.Kind) {
		return fmt.Errorf("unsupported artifact kind %q", candidate.Kind)
	}
	if err := validateOpaque("opportunity id", candidate.ID); err != nil {
		return err
	}
	if candidate.ID != opportunityID(candidate) {
		return fmt.Errorf("opportunity id does not match its local identity")
	}
	if err := validateModelText("opportunity title", candidate.Title, maxTitleBytes, true); err != nil {
		return err
	}
	if err := validateModelText("opportunity question", candidate.QuestionAnswered, maxQuestionBytes, true); err != nil {
		return err
	}
	if err := validateIDList("opportunity support ids", candidate.SupportIDs, true); err != nil {
		return err
	}
	if err := validateIDList(
		"opportunity enrichment support ids",
		candidate.EnrichmentSupportIDs,
		false,
	); err != nil {
		return err
	}
	initialSupport := make(map[string]struct{}, len(candidate.SupportIDs))
	addIDs(initialSupport, candidate.SupportIDs)
	for _, id := range candidate.EnrichmentSupportIDs {
		if _, duplicate := initialSupport[id]; duplicate {
			return fmt.Errorf("enrichment support id %q duplicates initial support", id)
		}
	}
	facts, err := factsForIDs(candidateFactIDs(candidate), known)
	if err != nil {
		return err
	}
	if !opportunityKindGrounded(candidate, known) {
		return fmt.Errorf("artifact kind is not grounded by the selected local fact capabilities")
	}
	if !validExpectedValue(candidate.ExpectedValue) {
		return fmt.Errorf("unsupported expected value %q", candidate.ExpectedValue)
	}
	if !validConfidence(candidate.Confidence) {
		return fmt.Errorf("unsupported confidence %q", candidate.Confidence)
	}
	if err := validateCapabilityContract(candidate.CapabilityContract, facts); err != nil {
		return fmt.Errorf("capability contract: %w", err)
	}
	if err := validateIntentContract(candidate.IntentContract); err != nil {
		return fmt.Errorf("intent contract: %w", err)
	}
	if err := validateIntentCapabilityContract(
		candidate.IntentContract,
		candidate.CapabilityContract,
	); err != nil {
		return fmt.Errorf("intent contract: %w", err)
	}
	if err := validateOpportunityProductIntent(candidate, known); err != nil {
		return fmt.Errorf("product intent: %w", err)
	}
	for _, missing := range candidate.MissingInformation {
		if err := validateModelText("opportunity missing information", missing, maxModelTextBytes, true); err != nil {
			return err
		}
	}
	return nil
}

func validateOpportunityProductIntent(
	candidate OpportunityCandidate,
	known map[string]Fact,
) error {
	intent := candidate.ProductIntent
	if intent == nil {
		return nil
	}
	if !validOpportunityKind(intent.OpportunityKind) {
		return fmt.Errorf("unsupported opportunity kind %q", intent.OpportunityKind)
	}
	if !validOpportunityUserJob(intent.TargetUserJob) {
		return fmt.Errorf("unsupported target user job %q", intent.TargetUserJob)
	}
	if !opportunityKindMatchesUserJob(intent.OpportunityKind, intent.TargetUserJob) {
		return fmt.Errorf("opportunity kind does not match target user job")
	}
	if !validOpportunityEstimatedCost(intent.EstimatedCost) {
		return fmt.Errorf("unsupported estimated cost %q", intent.EstimatedCost)
	}
	if !strings.HasSuffix(strings.TrimSpace(candidate.QuestionAnswered), "?") {
		return fmt.Errorf("natural question must end with a question mark")
	}
	support := make(map[string]struct{}, len(candidate.SupportIDs))
	for _, id := range candidate.SupportIDs {
		support[id] = struct{}{}
	}
	if err := validateProductIntentIDSubset(
		"central anchor ids",
		intent.CentralAnchorIDs,
		support,
		1,
		4,
	); err != nil {
		return err
	}
	if err := validateProductIntentIDSubset(
		"architecture area anchor ids",
		intent.ArchitectureAreaAnchorIDs,
		support,
		0,
		4,
	); err != nil {
		return err
	}
	expectations := []struct {
		label string
		value OpportunityExpectation
	}{
		{label: "input trigger", value: intent.ExpectedPath.InputTrigger},
		{label: "core work", value: intent.ExpectedPath.CoreWork},
		{label: "observable effect", value: intent.ExpectedPath.ObservableEffect},
	}
	for _, expectation := range expectations {
		if err := validateOpportunityExpectation(expectation.label, expectation.value, support, known); err != nil {
			return err
		}
	}
	if candidate.CapabilityContract == nil {
		return fmt.Errorf("capability contract is required")
	}
	required := capabilityListSet(candidate.CapabilityContract.RequiredCapabilities)
	for _, expectation := range []OpportunityExpectation{
		intent.ExpectedPath.InputTrigger,
		intent.ExpectedPath.CoreWork,
		intent.ExpectedPath.ObservableEffect,
	} {
		for _, capability := range expectation.RequiredCapabilities {
			if _, ok := required[capability]; !ok {
				return fmt.Errorf("expected-path capability %q is not required by the candidate", capability)
			}
		}
	}
	if len(intent.BoundedFrontier) > 2 {
		return fmt.Errorf("bounded frontier has more than 2 items")
	}
	central := make(map[string]struct{}, len(intent.CentralAnchorIDs))
	for _, id := range intent.CentralAnchorIDs {
		central[id] = struct{}{}
	}
	for index, frontier := range intent.BoundedFrontier {
		if err := validateProductIntentIDSubset(
			fmt.Sprintf("bounded frontier[%d] anchor ids", index),
			frontier.FromAnchorIDs,
			central,
			1,
			4,
		); err != nil {
			return err
		}
		if err := validateCapabilityList(
			fmt.Sprintf("bounded frontier[%d] desired capabilities", index),
			frontier.DesiredCapabilities,
			true,
		); err != nil {
			return err
		}
		if err := validateModelText(
			fmt.Sprintf("bounded frontier[%d] rationale", index),
			frontier.Rationale,
			maxModelTextBytes,
			true,
		); err != nil {
			return err
		}
	}
	rationales := []struct {
		label string
		value string
	}{
		{label: "onboarding rationale", value: intent.OnboardingRationale},
		{label: "investigation rationale", value: intent.InvestigationRationale},
	}
	for _, rationale := range rationales {
		if err := validateModelText(rationale.label, rationale.value, maxModelTextBytes, true); err != nil {
			return err
		}
	}
	if len(intent.SearchQueries) < 2 || len(intent.SearchQueries) > 6 {
		return fmt.Errorf("search query count must be between 2 and 6")
	}
	seenQueries := make(map[string]struct{}, len(intent.SearchQueries))
	for index, query := range intent.SearchQueries {
		if err := validateModelText(
			fmt.Sprintf("search_queries[%d]", index),
			query,
			maxQuestionBytes,
			true,
		); err != nil {
			return err
		}
		key := strings.ToLower(normalizeText(query))
		if _, duplicate := seenQueries[key]; duplicate {
			return fmt.Errorf("duplicate search query")
		}
		seenQueries[key] = struct{}{}
	}
	return nil
}

func validateOpportunityExpectation(
	label string,
	expectation OpportunityExpectation,
	support map[string]struct{},
	known map[string]Fact,
) error {
	if err := validateModelText(
		label+" description",
		expectation.Description,
		maxModelTextBytes,
		true,
	); err != nil {
		return err
	}
	if err := validateProductIntentIDSubset(
		label+" support ids",
		expectation.SupportIDs,
		support,
		0,
		4,
	); err != nil {
		return err
	}
	if err := validateCapabilityList(
		label+" required capabilities",
		expectation.RequiredCapabilities,
		true,
	); err != nil {
		return err
	}
	if len(expectation.SupportIDs) == 0 {
		return nil
	}
	facts := factsForKnownIDs(expectation.SupportIDs, known)
	for _, capability := range expectation.RequiredCapabilities {
		if !factsSupportCapability(facts, capability) {
			return fmt.Errorf("%s support ids do not provide required capability %q", label, capability)
		}
	}
	return nil
}

func boundedNormalizationDetail(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 256 {
		value = value[:256]
	}
	return value
}

func validateProductIntentIDSubset(
	label string,
	ids []string,
	allowed map[string]struct{},
	minimum int,
	maximum int,
) error {
	if len(ids) < minimum || len(ids) > maximum {
		return fmt.Errorf("%s count must be between %d and %d", label, minimum, maximum)
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if err := validateOpaque(label, id); err != nil {
			return err
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("%s repeats id %q", label, id)
		}
		if _, ok := allowed[id]; !ok {
			return fmt.Errorf("%s contains id outside the allowed candidate anchors", label)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func opportunityKindMatchesUserJob(kind OpportunityKind, job OpportunityUserJob) bool {
	switch kind {
	case OpportunityKindCentralBehavior:
		return job == OpportunityUserJobFirstContact
	case OpportunityKindQuestionPath:
		return job == OpportunityUserJobExploration
	case OpportunityKindExtensionPath:
		return job == OpportunityUserJobContribution
	case OpportunityKindMaintenanceBoundary:
		return job == OpportunityUserJobMaintenance
	default:
		return false
	}
}

// SelectOpportunities picks a deterministic, kind-diverse prefix. It does not
// reinterpret candidate prose or promote confidence.
func SelectOpportunities(
	bundle Bundle,
	proposal OpportunityProposal,
	limit int,
) ([]OpportunityCandidate, error) {
	if err := ValidateOpportunityProposal(bundle, proposal); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > MaxSelectedCandidates {
		return nil, fmt.Errorf("semantic discovery: selected candidate limit must be between 1 and %d", MaxSelectedCandidates)
	}
	ranked := append([]OpportunityCandidate(nil), proposal.Candidates...)
	known := factIndex(bundle)
	sort.Slice(ranked, func(i, j int) bool {
		return selectedOpportunityLess(known, ranked[i], ranked[j])
	})

	selected := make([]OpportunityCandidate, 0, min(limit, len(ranked)))
	selectedIDs := make(map[string]struct{})
	selectedKinds := make(map[ArtifactKind]struct{})
	for _, candidate := range ranked {
		if len(selected) == limit {
			break
		}
		if _, duplicateKind := selectedKinds[candidate.Kind]; duplicateKind {
			continue
		}
		selected = append(selected, candidate)
		selectedIDs[candidate.ID] = struct{}{}
		selectedKinds[candidate.Kind] = struct{}{}
	}
	for _, candidate := range ranked {
		if len(selected) == limit {
			break
		}
		if _, exists := selectedIDs[candidate.ID]; exists {
			continue
		}
		selected = append(selected, candidate)
		selectedIDs[candidate.ID] = struct{}{}
	}
	return selected, nil
}

func selectedOpportunityLess(
	known map[string]Fact,
	left OpportunityCandidate,
	right OpportunityCandidate,
) bool {
	if expectedValueRank(left.ExpectedValue) != expectedValueRank(right.ExpectedValue) {
		return expectedValueRank(left.ExpectedValue) > expectedValueRank(right.ExpectedValue)
	}
	if confidenceRank(left.Confidence) != confidenceRank(right.Confidence) {
		return confidenceRank(left.Confidence) > confidenceRank(right.Confidence)
	}
	leftBehavior := opportunityBehaviorSupport(known, candidateFactIDs(left))
	rightBehavior := opportunityBehaviorSupport(known, candidateFactIDs(right))
	if leftBehavior != rightBehavior {
		return leftBehavior > rightBehavior
	}
	if len(candidateFactIDs(left)) != len(candidateFactIDs(right)) {
		return len(candidateFactIDs(left)) > len(candidateFactIDs(right))
	}
	return left.ID < right.ID
}

func opportunityBehaviorSupport(known map[string]Fact, ids []string) int {
	count := 0
	for _, id := range ids {
		fact, exists := known[id]
		if !exists {
			continue
		}
		for _, capability := range fact.Capabilities {
			if isBehaviorCapability(capability) || capability == CapabilitySequence {
				count++
				break
			}
		}
	}
	return count
}

func OpportunityHash(proposal OpportunityProposal) (string, []byte, error) {
	canonical := proposal
	canonical.Candidates = append([]OpportunityCandidate(nil), proposal.Candidates...)
	for index := range canonical.Candidates {
		canonical.Candidates[index] = canonicalOpportunityCandidate(canonical.Candidates[index])
	}
	sort.Slice(canonical.Candidates, func(i, j int) bool {
		return canonical.Candidates[i].ID < canonical.Candidates[j].ID
	})
	return hashJSON("opportunity proposal", canonical)
}

func opportunityID(candidate OpportunityCandidate) string {
	return stableID(
		"semantic-candidate",
		string(candidate.Kind),
		strings.ToLower(normalizeText(candidate.Title)),
		strings.ToLower(normalizeText(candidate.QuestionAnswered)),
		strings.Join(sortedUnique(candidate.SupportIDs), "\x00"),
	)
}

func deduplicateOpportunities(
	candidates []OpportunityCandidate,
	report *NormalizationReport,
) []OpportunityCandidate {
	sort.Slice(candidates, func(i, j int) bool { return opportunityLess(candidates[i], candidates[j]) })
	result := make([]OpportunityCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		duplicate := false
		for _, existing := range result {
			if candidate.Kind != existing.Kind {
				continue
			}
			if normalizedOpportunityText(candidate) == normalizedOpportunityText(existing) ||
				supportJaccard(candidate.SupportIDs, existing.SupportIDs) >= 0.75 {
				duplicate = true
				break
			}
		}
		if duplicate {
			if report != nil {
				report.Issues = append(report.Issues, NormalizationIssue{
					CandidateIndex: -1, Code: "duplicate_candidate_removed", Detail: candidate.ID,
				})
			}
			continue
		}
		result = append(result, candidate)
	}
	return result
}

func normalizedOpportunityText(candidate OpportunityCandidate) string {
	return strings.ToLower(normalizeText(candidate.Title + " " + candidate.QuestionAnswered))
}

func supportJaccard(left, right []string) float64 {
	leftSet := make(map[string]struct{}, len(left))
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range left {
		leftSet[value] = struct{}{}
	}
	for _, value := range right {
		rightSet[value] = struct{}{}
	}
	intersection := 0
	for value := range leftSet {
		if _, exists := rightSet[value]; exists {
			intersection++
		}
	}
	union := len(leftSet) + len(rightSet) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func opportunityLess(left, right OpportunityCandidate) bool {
	if expectedValueRank(left.ExpectedValue) != expectedValueRank(right.ExpectedValue) {
		return expectedValueRank(left.ExpectedValue) > expectedValueRank(right.ExpectedValue)
	}
	if confidenceRank(left.Confidence) != confidenceRank(right.Confidence) {
		return confidenceRank(left.Confidence) > confidenceRank(right.Confidence)
	}
	if len(candidateFactIDs(left)) != len(candidateFactIDs(right)) {
		return len(candidateFactIDs(left)) > len(candidateFactIDs(right))
	}
	return left.ID < right.ID
}

func expectedValueRank(value ExpectedValue) int {
	switch value {
	case ExpectedValueHigh:
		return 3
	case ExpectedValueMedium:
		return 2
	default:
		return 1
	}
}

func confidenceRank(value Confidence) int {
	switch value {
	case ConfidenceHigh:
		return 3
	case ConfidenceMedium:
		return 2
	default:
		return 1
	}
}
