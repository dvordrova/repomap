package semanticdiscovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxBundleBytes             = 256 << 10
	maxFacts                   = 512
	maxPlannerContextItems     = 64
	maxFactStatementBytes      = 4 << 10
	maxModelTextBytes          = 4 << 10
	maxTitleBytes              = 256
	maxQuestionBytes           = 512
	maxOpaqueIDBytes           = 160
	maxKeywordsPerFact         = 16
	maxEvidencePerFact         = 16
	maxObservationsPerLeaf     = 8
	maxContradictionsPerLeaf   = 6
	maxMissingEvidencePerLeaf  = 8
	maxClaimsPerArtifact       = 12
	maxAliasesPerArtifact      = 12
	maxQuestionsPerArtifact    = 8
	maxAnswerAspects           = 12
	maxProposalBytes           = 128 << 10
	maxRecordBytes             = 512 << 10
	connectionNeedsCombination = "needs_combination"
)

var (
	repositoryReferencePattern = regexp.MustCompile(
		`(?i)(?:` +
			`[[:alnum:]_@.+-]+(?:[\\/][[:alnum:]_@.+-]+)+|` +
			`\b[[:alnum:]_@+-][[:alnum:]_@.+-]*\.(?:go|py|js|ts|java|rs|rb|php|cs|cpp|c|h|md)\b|` +
			`\b(?:readme|makefile|dockerfile)\b|` +
			`\b[[:alnum:]_]+\.[[:alnum:]_]+(?:\.[[:alnum:]_]+)*\b` +
			`)`,
	)
	behaviorPattern = regexp.MustCompile(
		`(?i)\b(` +
			`executes?|parses?|initiates?|invokes?|calls?|dispatches?|routes?|forwards?|` +
			`enqueues?|schedules?|reads?|persists?|processes?|responds?|transfers?|delegates?|` +
			`publishes?|emits?|queues?|delivers?|submits?|propagates?|registers?|launches?|` +
			`spawns?|awaits?|waits?|selects?|consumes?|stores?|fetches?|retrieves?|connects?|` +
			`opens?|closes?|transforms?|converts?|orchestrates?|runs?|produces?|generates?|` +
			`assembles?|initiali[sz]es?|serves?|handles?|defines?|builds?|creates?|writes?|` +
			`loads?|returns?|passes?|sends?|receives?|triggers?|coordinates?|combines?|` +
			`validates?|rejects?|accepts?|checks?|uses?` +
			`)\b`,
	)
	sequencePattern = regexp.MustCompile(
		`(?i)(?:\b(?:before|after|then|next|finally|first|last|sequence|order|lifecycle|` +
			`upstream|downstream|followed by|starts? with|ends? with)\b|→|->)`,
	)
	limitationPattern = regexp.MustCompile(
		`(?i)\b(?:unknown|unresolved|unproven|missing|insufficient|not establish(?:ed)?|` +
			`cannot determine|cannot show|do(?:es)? not establish|doesn't establish|gap|limitation)\b`,
	)
	evidenceGapPattern = regexp.MustCompile(`(?i)^evidence gap:\s+\S`)
	universalPattern   = regexp.MustCompile(
		`(?i)\b(?:all|always|every|entire|only|never|repository-wide|whole repository)\b`,
	)
)

func hasExplicitLimitation(text string) bool {
	return evidenceGapPattern.MatchString(strings.TrimSpace(text)) ||
		limitationPattern.MatchString(text)
}

var stopwords = map[string]struct{}{
	"about": {}, "after": {}, "also": {}, "and": {}, "are": {}, "because": {},
	"been": {}, "being": {}, "between": {}, "does": {}, "from": {}, "have": {},
	"into": {}, "only": {}, "that": {}, "the": {}, "their": {}, "this": {},
	"through": {}, "uses": {}, "using": {}, "what": {}, "when": {}, "where": {},
	"which": {}, "with": {}, "without": {}, "would": {},
	"будет": {}, "быть": {}, "где": {}, "для": {}, "есть": {}, "здесь": {},
	"как": {}, "какие": {}, "какой": {}, "когда": {}, "который": {}, "между": {},
	"может": {}, "после": {}, "почему": {}, "проект": {}, "репозиторий": {},
	"через": {}, "чтобы": {}, "этого": {}, "этот": {},
}

func validateOpaque(field, value string) error {
	if value == "" || len(value) > maxOpaqueIDBytes || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return fmt.Errorf("semantic discovery: %s is empty, malformed, or too long", field)
	}
	for _, char := range value {
		if char < 0x21 || char == 0x7f || char == '/' || char == '\\' {
			return fmt.Errorf("semantic discovery: %s contains whitespace, control, or path characters", field)
		}
	}
	return nil
}

func validateLocalText(field, value string, limit int, required bool) error {
	if len(value) > limit || strings.TrimSpace(value) != value || !utf8.ValidString(value) {
		return fmt.Errorf("semantic discovery: %s is malformed or too long", field)
	}
	if required && value == "" {
		return fmt.Errorf("semantic discovery: %s is empty", field)
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return fmt.Errorf("semantic discovery: %s contains control characters", field)
		}
	}
	return nil
}

func validateModelText(field, value string, limit int, required bool) error {
	if err := validateLocalText(field, value, limit, required); err != nil {
		return err
	}
	if value != "" && repositoryReferencePattern.MatchString(value) {
		return fmt.Errorf("semantic discovery: %s contains a repository-bearing reference", field)
	}
	return nil
}

func validArtifactKind(kind ArtifactKind) bool {
	switch kind {
	case ArtifactMechanism, ArtifactDependencyUsage, ArtifactRepositoryPattern,
		ArtifactContributionGuide, ArtifactGoLearning, ArtifactRepositoryStory:
		return true
	default:
		return false
	}
}

func validFactKind(kind FactKind) bool {
	switch kind {
	case FactRepositoryPurpose, FactComponent, FactFlow, FactFlowStep, FactRuntimeSurface,
		FactPackageImport, FactDependency, FactREADMEClaim, FactDomainTerm, FactTestReference,
		FactSourceSignal, FactGuidedStep, FactWarning, FactUnknown:
		return true
	default:
		return false
	}
}

func validCapability(capability Capability) bool {
	switch capability {
	case CapabilityStatic,
		CapabilityBehavior,
		CapabilitySequence,
		CapabilityLimitation,
		CapabilityEntry,
		CapabilityDirectCall,
		CapabilityBranch,
		CapabilityDataRead,
		CapabilityDataWrite,
		CapabilityDataTransformation,
		CapabilityOutputEffect,
		CapabilityErrorPath,
		CapabilityTestEvidence,
		CapabilityOwnership,
		CapabilityLifecycle:
		return true
	default:
		return false
	}
}

func validCapabilityResolution(resolution CapabilityResolution) bool {
	switch resolution {
	case CapabilityResolutionRequiresProbe,
		CapabilityResolutionReady,
		CapabilityResolutionPartial,
		CapabilityResolutionInsufficientEvidence:
		return true
	default:
		return false
	}
}

func validOpportunityKind(kind OpportunityKind) bool {
	switch kind {
	case OpportunityKindCentralBehavior,
		OpportunityKindQuestionPath,
		OpportunityKindExtensionPath,
		OpportunityKindMaintenanceBoundary:
		return true
	default:
		return false
	}
}

func validOpportunityUserJob(job OpportunityUserJob) bool {
	switch job {
	case OpportunityUserJobFirstContact,
		OpportunityUserJobExploration,
		OpportunityUserJobContribution,
		OpportunityUserJobMaintenance:
		return true
	default:
		return false
	}
}

func validOpportunityEstimatedCost(cost OpportunityEstimatedCost) bool {
	switch cost {
	case OpportunityEstimatedCostLow,
		OpportunityEstimatedCostMedium,
		OpportunityEstimatedCostHigh:
		return true
	default:
		return false
	}
}

func validScope(scope FactScope) bool {
	switch scope {
	case FactScopeLocal, FactScopeComponent, FactScopeFlow, FactScopeRepository:
		return true
	default:
		return false
	}
}

func validExpectedValue(value ExpectedValue) bool {
	return value == ExpectedValueLow || value == ExpectedValueMedium || value == ExpectedValueHigh
}

func validConfidence(value Confidence) bool {
	return value == ConfidenceLow || value == ConfidenceMedium || value == ConfidenceHigh
}

func stableID(prefix string, parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return prefix + "-" + hex.EncodeToString(hash.Sum(nil))[:24]
}

func hashJSON(prefix string, value any) (string, []byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", nil, fmt.Errorf("semantic discovery: encode canonical %s: %w", prefix, err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), encoded, nil
}

func sortedUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortedStringsPreservingInvalid(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func normalizeText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func tokens(value string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(char rune) bool {
		return !unicode.IsLetter(char) && !unicode.IsDigit(char)
	}) {
		if utf8.RuneCountInString(token) < 3 {
			continue
		}
		if _, stopword := stopwords[token]; stopword {
			continue
		}
		result[token] = struct{}{}
	}
	return result
}

func factTerms(fact Fact) map[string]struct{} {
	result := tokens(fact.Statement)
	for _, keyword := range fact.Keywords {
		for token := range tokens(keyword) {
			result[token] = struct{}{}
		}
	}
	return result
}

func hasBoundedLexicalOverlap(text string, facts []Fact) bool {
	claimTerms := tokens(text)
	if len(claimTerms) == 0 {
		return false
	}
	supportTerms := make(map[string]struct{})
	for _, fact := range facts {
		for term := range factTerms(fact) {
			supportTerms[term] = struct{}{}
		}
	}
	overlap := 0
	for term := range claimTerms {
		if _, exists := supportTerms[term]; exists {
			overlap++
		}
	}
	if len(claimTerms) >= 6 {
		return overlap >= 2
	}
	return overlap >= 1
}

func capabilitySet(facts []Fact) map[Capability]struct{} {
	result := make(map[Capability]struct{})
	for _, fact := range facts {
		for _, capability := range fact.Capabilities {
			result[capability] = struct{}{}
		}
	}
	return result
}

func isBehaviorCapability(capability Capability) bool {
	switch capability {
	case CapabilityBehavior,
		CapabilityEntry,
		CapabilityDirectCall,
		CapabilityBranch,
		CapabilityDataRead,
		CapabilityDataWrite,
		CapabilityDataTransformation,
		CapabilityOutputEffect,
		CapabilityErrorPath,
		CapabilityTestEvidence,
		CapabilityLifecycle:
		return true
	default:
		return false
	}
}

func factSupportsCapability(fact Fact, required Capability) bool {
	for _, capability := range fact.Capabilities {
		switch required {
		case CapabilityBehavior:
			if isBehaviorCapability(capability) {
				return true
			}
		case CapabilitySequence:
			if capability == CapabilitySequence || capability == CapabilityLifecycle {
				return true
			}
		default:
			if capability == required {
				return true
			}
		}
	}
	return false
}

func factsSupportCapability(facts []Fact, required Capability) bool {
	for _, fact := range facts {
		if factSupportsCapability(fact, required) {
			return true
		}
	}
	return false
}

func missingSupportsCapability(missing map[Capability]struct{}, required Capability) bool {
	for capability := range missing {
		switch required {
		case CapabilityBehavior:
			if isBehaviorCapability(capability) {
				return true
			}
		case CapabilitySequence:
			if capability == CapabilitySequence || capability == CapabilityLifecycle {
				return true
			}
		default:
			if capability == required {
				return true
			}
		}
	}
	return false
}

func validateSemanticSupport(field, text string, facts []Fact) error {
	if len(facts) == 0 {
		return fmt.Errorf("semantic discovery: %s has no supporting facts", field)
	}
	if !hasBoundedLexicalOverlap(text, facts) {
		return fmt.Errorf("semantic discovery: %s is lexically unrelated to its supporting facts", field)
	}
	if behaviorPattern.MatchString(text) {
		if !factsSupportCapability(facts, CapabilityBehavior) ||
			!hasCapabilityOverlap(text, facts, CapabilityBehavior) {
			return fmt.Errorf("semantic discovery: %s asserts behavior without behavior-capable support", field)
		}
	}
	if sequencePattern.MatchString(text) {
		if !factsSupportCapability(facts, CapabilitySequence) ||
			!hasCapabilityOverlap(text, facts, CapabilitySequence) {
			return fmt.Errorf("semantic discovery: %s asserts ordering without sequence-capable support", field)
		}
	}
	if limitationPattern.MatchString(text) {
		if !factsSupportCapability(facts, CapabilityLimitation) ||
			!hasCapabilityOverlap(text, facts, CapabilityLimitation) {
			return fmt.Errorf("semantic discovery: %s asserts a limitation without limitation-capable support", field)
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
			return fmt.Errorf("semantic discovery: %s makes a repository-wide claim from bounded support", field)
		}
	}
	return nil
}

func hasCapabilityOverlap(text string, facts []Fact, capability Capability) bool {
	for _, fact := range facts {
		if factSupportsCapability(fact, capability) && hasBoundedLexicalOverlap(text, []Fact{fact}) {
			return true
		}
	}
	return false
}

func validateIDList(field string, values []string, required bool) error {
	if required && len(values) == 0 {
		return fmt.Errorf("semantic discovery: %s is empty", field)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := validateOpaque(field, value); err != nil {
			return err
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("semantic discovery: %s contains duplicate id %q", field, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateFocus(focus Focus) error {
	for field, values := range map[string][]string{
		"focus component ids": focus.ComponentIDs,
		"focus flow ids":      focus.FlowIDs,
		"focus surface ids":   focus.SurfaceIDs,
	} {
		if err := validateIDList(field, values, false); err != nil {
			return err
		}
	}
	return nil
}

func validateEvidence(reference EvidenceRef) error {
	if err := validateOpaque("evidence id", reference.ID); err != nil {
		return err
	}
	if err := validateOpaque("evidence kind", reference.Kind); err != nil {
		return err
	}
	if err := validateLocalText("evidence label", reference.Label, maxModelTextBytes, false); err != nil {
		return err
	}
	if reference.Path != "" {
		if path.IsAbs(reference.Path) || path.Clean(reference.Path) != reference.Path ||
			reference.Path == "." || reference.Path == ".." || strings.HasPrefix(reference.Path, "../") ||
			strings.Contains(reference.Path, `\`) {
			return fmt.Errorf("semantic discovery: evidence path is not repository-relative")
		}
	}
	if reference.Line < 0 || reference.Column < 0 || reference.Column > 0 && reference.Line == 0 {
		return fmt.Errorf("semantic discovery: evidence location is invalid")
	}
	return nil
}

func validateFactSource(source FactSource) error {
	if err := validateLocalText("fact source path", source.Path, maxModelTextBytes, true); err != nil {
		return err
	}
	if path.IsAbs(source.Path) || path.Clean(source.Path) != source.Path ||
		source.Path == "." || source.Path == ".." || strings.HasPrefix(source.Path, "../") ||
		strings.Contains(source.Path, `\`) {
		return fmt.Errorf("semantic discovery: fact source path is not repository-relative")
	}
	if source.StartLine <= 0 || source.EndLine < source.StartLine {
		return fmt.Errorf("semantic discovery: fact source line range is invalid")
	}
	if err := validateLocalText(
		"fact source enclosing symbol",
		source.EnclosingSymbol,
		maxModelTextBytes,
		true,
	); err != nil {
		return err
	}
	if len(source.ContentSHA256) != sha256.Size*2 {
		return fmt.Errorf("semantic discovery: fact source content sha256 must be exactly 64 hexadecimal characters")
	}
	for _, char := range source.ContentSHA256 {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return fmt.Errorf("semantic discovery: fact source content sha256 must be exactly 64 hexadecimal characters")
		}
	}
	return nil
}

func factsForIDs(ids []string, known map[string]Fact) ([]Fact, error) {
	result := make([]Fact, 0, len(ids))
	for _, id := range ids {
		fact, exists := known[id]
		if !exists {
			return nil, fmt.Errorf("semantic discovery: unknown support id %q", id)
		}
		result = append(result, fact)
	}
	return result, nil
}

func sourceGroupCount(facts []Fact) int {
	groups := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		groups[fact.SourceGroup] = struct{}{}
	}
	return len(groups)
}

func decodeStrict(raw []byte, target any, limit int) error {
	if len(raw) == 0 || len(raw) > limit {
		return fmt.Errorf("semantic discovery: json is empty or too large")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("semantic discovery: trailing json values")
		}
		return fmt.Errorf("semantic discovery: trailing json: %w", err)
	}
	return nil
}
