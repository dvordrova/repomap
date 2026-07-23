package semanticdiscovery

import (
	"fmt"
	"sort"
	"strings"
)

const answerAspectKeywordPrefix = "answer_aspect:"

func candidateFactIDs(candidate OpportunityCandidate) []string {
	ids := append([]string(nil), candidate.SupportIDs...)
	ids = append(ids, candidate.EnrichmentSupportIDs...)
	return sortedUnique(ids)
}

func validateCapabilityContract(
	contract *CapabilityContract,
	facts []Fact,
) error {
	if contract == nil {
		return nil
	}
	if !validCapabilityResolution(contract.Resolution) {
		return fmt.Errorf("unsupported capability resolution %q", contract.Resolution)
	}
	if err := validateCapabilityList(
		"required capabilities",
		contract.RequiredCapabilities,
		true,
	); err != nil {
		return err
	}
	if err := validateCapabilityList(
		"available capabilities",
		contract.AvailableCapabilities,
		false,
	); err != nil {
		return err
	}
	if err := validateCapabilityList(
		"missing capabilities",
		contract.MissingCapabilities,
		false,
	); err != nil {
		return err
	}

	required := capabilityListSet(contract.RequiredCapabilities)
	available := capabilityListSet(contract.AvailableCapabilities)
	missing := capabilityListSet(contract.MissingCapabilities)
	for capability := range available {
		if _, requiredCapability := required[capability]; !requiredCapability {
			return fmt.Errorf("available capability %q is not required", capability)
		}
		if _, alsoMissing := missing[capability]; alsoMissing {
			return fmt.Errorf("capability %q is both available and missing", capability)
		}
		if !factsSupportCapability(facts, capability) {
			return fmt.Errorf("available capability %q has no supporting fact", capability)
		}
	}
	for capability := range missing {
		if _, requiredCapability := required[capability]; !requiredCapability {
			return fmt.Errorf("missing capability %q is not required", capability)
		}
	}
	for capability := range required {
		_, isAvailable := available[capability]
		_, isMissing := missing[capability]
		if isAvailable == isMissing {
			return fmt.Errorf("required capability %q is not in exactly one partition", capability)
		}
	}
	switch contract.Resolution {
	case CapabilityResolutionReady:
		if len(missing) != 0 {
			return fmt.Errorf("ready capability contract still has missing capabilities")
		}
	case CapabilityResolutionRequiresProbe,
		CapabilityResolutionInsufficientEvidence:
		if len(missing) == 0 {
			return fmt.Errorf("capability resolution %q requires a missing capability", contract.Resolution)
		}
	case CapabilityResolutionPartial:
		if len(available) == 0 || len(missing) == 0 {
			return fmt.Errorf("partial capability contract needs available and missing capabilities")
		}
	}
	return nil
}

func validateIntentContract(contract *IntentContract) error {
	if contract == nil {
		return nil
	}
	if len(contract.RequiredAnswerAspects) == 0 ||
		len(contract.RequiredAnswerAspects) > maxAnswerAspects {
		return fmt.Errorf(
			"intent answer aspect count must be between 1 and %d",
			maxAnswerAspects,
		)
	}
	seen := make(map[string]struct{}, len(contract.RequiredAnswerAspects))
	keyCount := 0
	for index, aspect := range contract.RequiredAnswerAspects {
		if err := validateOpaque("answer aspect id", aspect.ID); err != nil {
			return fmt.Errorf("answer aspects[%d]: %w", index, err)
		}
		if _, duplicate := seen[aspect.ID]; duplicate {
			return fmt.Errorf("duplicate answer aspect id %q", aspect.ID)
		}
		seen[aspect.ID] = struct{}{}
		if err := validateModelText(
			"answer aspect label",
			aspect.Label,
			maxTitleBytes,
			true,
		); err != nil {
			return fmt.Errorf("answer aspects[%d]: %w", index, err)
		}
		if err := validateCapabilityList(
			"answer aspect required capabilities",
			aspect.RequiredCapabilities,
			true,
		); err != nil {
			return fmt.Errorf("answer aspects[%d]: %w", index, err)
		}
		if aspect.Key {
			keyCount++
		}
	}
	if contract.MinCovered <= 0 || contract.MinCovered > len(contract.RequiredAnswerAspects) {
		return fmt.Errorf("intent minimum covered aspect count is invalid")
	}
	if contract.MinKeyCovered < 0 || contract.MinKeyCovered > keyCount {
		return fmt.Errorf("intent minimum key-covered aspect count is invalid")
	}
	if len(contract.LocalSearchAliases) == 0 ||
		len(contract.LocalSearchAliases) > maxAliasesPerArtifact {
		return fmt.Errorf(
			"intent local search alias count must be between 1 and %d",
			maxAliasesPerArtifact,
		)
	}
	seenAliases := make(map[string]struct{}, len(contract.LocalSearchAliases))
	for _, alias := range contract.LocalSearchAliases {
		if err := validateModelText("intent local search alias", alias, maxQuestionBytes, true); err != nil {
			return err
		}
		if _, duplicate := seenAliases[alias]; duplicate {
			return fmt.Errorf("intent repeats local search alias %q", alias)
		}
		seenAliases[alias] = struct{}{}
	}
	return nil
}

func validateIntentCapabilityContract(
	intent *IntentContract,
	capabilities *CapabilityContract,
) error {
	if intent == nil || capabilities == nil {
		return nil
	}
	required := capabilityListSet(capabilities.RequiredCapabilities)
	for _, aspect := range intent.RequiredAnswerAspects {
		for _, capability := range aspect.RequiredCapabilities {
			if _, exists := required[capability]; !exists {
				return fmt.Errorf(
					"answer aspect %q requires capability %q absent from the capability contract",
					aspect.ID,
					capability,
				)
			}
		}
	}
	return nil
}

func validateCapabilityList(field string, capabilities []Capability, required bool) error {
	if required && len(capabilities) == 0 {
		return fmt.Errorf("%s are empty", field)
	}
	seen := make(map[Capability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if !validCapability(capability) {
			return fmt.Errorf("%s contain unsupported capability %q", field, capability)
		}
		if _, duplicate := seen[capability]; duplicate {
			return fmt.Errorf("%s repeat capability %q", field, capability)
		}
		seen[capability] = struct{}{}
	}
	return nil
}

func capabilityListSet(capabilities []Capability) map[Capability]struct{} {
	result := make(map[Capability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		result[capability] = struct{}{}
	}
	return result
}

func validateIntentProposal(
	candidate OpportunityCandidate,
	proposal ArtifactProposal,
	known map[string]Fact,
) error {
	if candidate.IntentContract == nil {
		return nil
	}
	if len(proposal.Claims) < 3 || len(proposal.Claims) > 7 {
		return fmt.Errorf("intent-bound artifact claim count must be between 3 and 7")
	}
	resolved := 0
	for _, claim := range proposal.Claims {
		if claim.Basis != ClaimUnresolved {
			resolved++
		}
	}
	if resolved < 3 {
		return fmt.Errorf("intent-bound artifact needs at least three resolved statements")
	}
	covered, _, keyCovered := deriveIntentCoverage(candidate, proposal.Claims, known)
	if len(covered) < candidate.IntentContract.MinCovered {
		return fmt.Errorf(
			"intent-bound artifact covers %d aspects, requires %d",
			len(covered),
			candidate.IntentContract.MinCovered,
		)
	}
	if keyCovered < candidate.IntentContract.MinKeyCovered {
		return fmt.Errorf(
			"intent-bound artifact covers %d key aspects, requires %d",
			keyCovered,
			candidate.IntentContract.MinKeyCovered,
		)
	}
	if !equalStringSets(proposal.Aliases, candidate.IntentContract.LocalSearchAliases) {
		return fmt.Errorf("intent-bound artifact aliases do not match local search aliases")
	}
	if !equalStringSets(proposal.LikelyQuestions, []string{candidate.QuestionAnswered}) {
		return fmt.Errorf("intent-bound artifact likely questions do not preserve the original question")
	}
	if !hasIntentMetadataOverlap(candidate, proposal) {
		return fmt.Errorf("intent-bound artifact title and summary lose the original question")
	}
	return nil
}

func deriveIntentCoverage(
	candidate OpportunityCandidate,
	claims []ProposedClaim,
	known map[string]Fact,
) (covered []string, uncovered []string, keyCovered int) {
	if candidate.IntentContract == nil {
		return nil, nil, 0
	}
	for _, aspect := range candidate.IntentContract.RequiredAnswerAspects {
		isCovered := false
		for _, claim := range claims {
			if claimCoversAnswerAspect(claim, known, aspect) {
				isCovered = true
				break
			}
		}
		if isCovered {
			covered = append(covered, aspect.ID)
			if aspect.Key {
				keyCovered++
			}
			continue
		}
		uncovered = append(uncovered, aspect.ID)
	}
	return covered, uncovered, keyCovered
}

func claimCoversAnswerAspect(
	claim ProposedClaim,
	known map[string]Fact,
	aspect AnswerAspect,
) bool {
	if claim.Basis == ClaimUnresolved {
		return false
	}
	for _, id := range claim.SupportIDs {
		fact, exists := known[id]
		if !exists || !factCoversAnswerAspect(fact, aspect) {
			continue
		}
		if hasFocusedFactOverlap(claim.Text, fact) {
			return true
		}
	}
	return false
}

func hasFocusedFactOverlap(text string, fact Fact) bool {
	// Claim support and capability semantics have already been validated before
	// aspect projection. Requiring half of a fact's vocabulary here made a
	// legitimate bounded compositional claim lose each of its constituent
	// aspects merely because it summarized several observations. Keep a
	// substantial per-fact threshold so unrelated support padding cannot gain
	// coverage, while allowing one claim to compress several validated facts.
	claimTerms := tokens(text)
	supportTerms := factTerms(fact)
	if len(claimTerms) == 0 || len(supportTerms) == 0 {
		return false
	}
	overlap := 0
	for term := range claimTerms {
		if _, exists := supportTerms[term]; exists {
			overlap++
		}
	}
	smaller := min(len(claimTerms), len(supportTerms))
	required := max(3, (smaller+2)/3)
	return overlap >= required
}

func factCoversAnswerAspect(fact Fact, aspect AnswerAspect) bool {
	marker := answerAspectKeywordPrefix + aspect.ID
	hasMarker := false
	for _, keyword := range fact.Keywords {
		if keyword == marker {
			hasMarker = true
			break
		}
	}
	if !hasMarker {
		return false
	}
	for _, capability := range aspect.RequiredCapabilities {
		if !factSupportsCapability(fact, capability) {
			return false
		}
	}
	return true
}

func hasIntentMetadataOverlap(
	candidate OpportunityCandidate,
	proposal ArtifactProposal,
) bool {
	anchors := []string{candidate.Title, candidate.QuestionAnswered}
	anchors = append(anchors, candidate.IntentContract.LocalSearchAliases...)
	anchorTerms := tokens(strings.Join(anchors, " "))
	proposalTerms := tokens(proposal.Title + " " + proposal.Summary)
	overlap := 0
	for term := range proposalTerms {
		if _, exists := anchorTerms[term]; exists {
			overlap++
		}
	}
	requiredOverlap := 2
	if len(anchorTerms) < requiredOverlap || len(proposalTerms) < requiredOverlap {
		requiredOverlap = 1
	}
	return overlap >= requiredOverlap
}

func applyIntentMaterialization(
	artifact *Artifact,
	candidate OpportunityCandidate,
	claims []ProposedClaim,
	known map[string]Fact,
) {
	if candidate.IntentContract == nil {
		return
	}
	candidate = canonicalOpportunityCandidate(candidate)
	covered, uncovered, _ := deriveIntentCoverage(candidate, claims, known)
	for _, aspect := range candidate.IntentContract.RequiredAnswerAspects {
		artifact.RequiredAspectIDs = append(artifact.RequiredAspectIDs, aspect.ID)
	}
	artifact.CoveredAspectIDs = covered
	artifact.UncoveredAspectIDs = uncovered
	artifact.Aliases = sortedUnique(append(
		artifact.Aliases,
		candidate.IntentContract.LocalSearchAliases...,
	))

	aspectsByID := make(map[string]AnswerAspect, len(candidate.IntentContract.RequiredAnswerAspects))
	for _, aspect := range candidate.IntentContract.RequiredAnswerAspects {
		aspectsByID[aspect.ID] = aspect
	}
	for _, id := range uncovered {
		aspect := aspectsByID[id]
		artifact.Unknowns = append(
			artifact.Unknowns,
			"Evidence remains insufficient for answer aspect: "+aspect.Label,
		)
	}
	artifact.Unknowns = joinUnknowns(artifact.Unknowns)
}

func canonicalOpportunityCandidate(candidate OpportunityCandidate) OpportunityCandidate {
	result := candidate
	result.SupportIDs = sortedUnique(candidate.SupportIDs)
	result.EnrichmentSupportIDs = sortedUnique(candidate.EnrichmentSupportIDs)
	result.MissingInformation = sortedUnique(candidate.MissingInformation)
	if candidate.CapabilityContract != nil {
		contract := *candidate.CapabilityContract
		contract.RequiredCapabilities = canonicalCapabilities(contract.RequiredCapabilities)
		contract.AvailableCapabilities = canonicalCapabilities(contract.AvailableCapabilities)
		contract.MissingCapabilities = canonicalCapabilities(contract.MissingCapabilities)
		result.CapabilityContract = &contract
	}
	if candidate.IntentContract != nil {
		contract := *candidate.IntentContract
		contract.LocalSearchAliases = sortedUnique(contract.LocalSearchAliases)
		contract.RequiredAnswerAspects = append(
			[]AnswerAspect(nil),
			candidate.IntentContract.RequiredAnswerAspects...,
		)
		for index := range contract.RequiredAnswerAspects {
			contract.RequiredAnswerAspects[index].RequiredCapabilities = canonicalCapabilities(
				contract.RequiredAnswerAspects[index].RequiredCapabilities,
			)
		}
		sort.Slice(contract.RequiredAnswerAspects, func(i, j int) bool {
			return contract.RequiredAnswerAspects[i].ID < contract.RequiredAnswerAspects[j].ID
		})
		result.IntentContract = &contract
	}
	if candidate.ProductIntent != nil {
		intent := *candidate.ProductIntent
		intent.CentralAnchorIDs = sortedUnique(intent.CentralAnchorIDs)
		intent.ArchitectureAreaAnchorIDs = sortedUnique(intent.ArchitectureAreaAnchorIDs)
		intent.SearchQueries = sortedUniqueNormalizedText(intent.SearchQueries)
		intent.ExpectedPath = canonicalOpportunityExpectedPath(intent.ExpectedPath)
		intent.BoundedFrontier = append([]OpportunityFrontier(nil), intent.BoundedFrontier...)
		for index := range intent.BoundedFrontier {
			frontier := &intent.BoundedFrontier[index]
			frontier.FromAnchorIDs = sortedUnique(frontier.FromAnchorIDs)
			frontier.DesiredCapabilities = canonicalCapabilities(frontier.DesiredCapabilities)
			frontier.Rationale = normalizeText(frontier.Rationale)
		}
		sort.Slice(intent.BoundedFrontier, func(i, j int) bool {
			left := strings.Join(intent.BoundedFrontier[i].FromAnchorIDs, "\x00") + "\x01" +
				strings.Join(capabilityStrings(intent.BoundedFrontier[i].DesiredCapabilities), "\x00") + "\x01" +
				intent.BoundedFrontier[i].Rationale
			right := strings.Join(intent.BoundedFrontier[j].FromAnchorIDs, "\x00") + "\x01" +
				strings.Join(capabilityStrings(intent.BoundedFrontier[j].DesiredCapabilities), "\x00") + "\x01" +
				intent.BoundedFrontier[j].Rationale
			return left < right
		})
		intent.OnboardingRationale = normalizeText(intent.OnboardingRationale)
		intent.InvestigationRationale = normalizeText(intent.InvestigationRationale)
		result.ProductIntent = &intent
	}
	return result
}

func canonicalOpportunityExpectedPath(path OpportunityExpectedPath) OpportunityExpectedPath {
	path.InputTrigger = canonicalOpportunityExpectation(path.InputTrigger)
	path.CoreWork = canonicalOpportunityExpectation(path.CoreWork)
	path.ObservableEffect = canonicalOpportunityExpectation(path.ObservableEffect)
	return path
}

func canonicalOpportunityExpectation(expectation OpportunityExpectation) OpportunityExpectation {
	expectation.Description = normalizeText(expectation.Description)
	expectation.SupportIDs = sortedUnique(expectation.SupportIDs)
	expectation.RequiredCapabilities = canonicalCapabilities(expectation.RequiredCapabilities)
	return expectation
}

func sortedUniqueNormalizedText(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = normalizeText(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i]) < strings.ToLower(result[j])
	})
	return result
}

func capabilityStrings(values []Capability) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}

func canonicalCapabilities(capabilities []Capability) []Capability {
	result := append([]Capability(nil), capabilities...)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
