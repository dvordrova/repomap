package tasklens

import (
	"fmt"
	"go/ast"
	"path"
	"reflect"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// DeriveTaskProfile selects one generic role contract from task concepts. It
// deliberately uses no repository- or episode-specific symbol names.
func DeriveTaskProfile(task string, kind TaskKind) TaskProfile {
	words := normalizedTaskWords(task)
	hasAny := func(values ...string) bool {
		for _, value := range values {
			if containsTaskPhrase(words, value) {
				return true
			}
		}
		return false
	}
	switch {
	case kind == TaskOperational:
		return TaskProfileOperationalRelease
	case kind == TaskExtension:
		return TaskProfileExtensionContribution
	case kind == TaskConfiguration:
		return TaskProfileConfigurationPropagation
	case hasAny("release", "annotated tag", "before pushing", "git tag"):
		return TaskProfileOperationalRelease
	case hasAny("extension", "adapter", "contribution"):
		return TaskProfileExtensionContribution
	case hasAny(
		"generated", "tag", "tags", "omitempty", "nullable", "schema", "schemas",
		"example value", "generated output", "generated fixture",
	):
		// Strong data-contract vocabulary takes precedence over incidental
		// mentions of nil, null, or a generator panic.
		return TaskProfileDataTagTransformation
	case hasAny("nil", "panic", "panics", "panicked", "panicking", "null"):
		return TaskProfileNilPanic
	case hasAny("configuration", "config field", "setting is ignored"):
		return TaskProfileConfigurationPropagation
	case hasAny("value versus a pointer", "value vs", "pointer", "nested developer", "public serialization", "privacy", "normalization") &&
		hasAny("error", "err"):
		return TaskProfileErrorNormalizationPrivacy
	case hasAny("wrong status", "incorrect status", "not-acceptable", "not acceptable", "media types", "error-mapping"):
		return TaskProfileErrorStatusMapping
	case hasAny("transformation", "parsing"):
		return TaskProfileDataTagTransformation
	default:
		return TaskProfileUnknown
	}
}

func normalizedTaskWords(value string) []string {
	return strings.FieldsFunc(strings.ToLower(value), func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsNumber(character)
	})
}

func containsTaskPhrase(words []string, phrase string) bool {
	phraseWords := normalizedTaskWords(phrase)
	if len(phraseWords) == 0 || len(phraseWords) > len(words) {
		return false
	}
	for start := 0; start <= len(words)-len(phraseWords); start++ {
		if slices.Equal(words[start:start+len(phraseWords)], phraseWords) {
			return true
		}
	}
	return false
}

func additionalRoleHints(anchor Anchor, kind TaskKind, taskText string) []AnchorRole {
	profile := DeriveTaskProfile(taskText, kind)
	content := strings.ToLower(anchor.Symbol + "\n" + anchor.Path + "\n" + anchorText(anchor))
	lowerPath := strings.ToLower(anchor.Path)
	lowerSymbol := strings.ToLower(anchor.Symbol)
	symbolWords := identifierWordSet(anchor.Symbol)
	hasAny := func(values ...string) bool {
		for _, value := range values {
			if strings.Contains(content, value) {
				return true
			}
		}
		return false
	}
	symbolHas := func(values ...string) bool {
		for _, value := range values {
			if _, exists := symbolWords[value]; exists {
				return true
			}
		}
		return false
	}
	hasErrorSubject := symbolHas("error", "err", "failure")
	var roles []AnchorRole
	add := func(role AnchorRole) {
		if !slices.Contains(roles, role) {
			roles = append(roles, role)
		}
	}

	if lowerPath == "go.work" || strings.HasSuffix(lowerPath, "/go.work") ||
		lowerPath == "go.mod" || strings.HasSuffix(lowerPath, "/go.mod") {
		add(RoleModuleTopology)
	}
	if isTestPath(anchor.Path) {
		add(RoleVerificationAnchor)
		if strings.Contains(lowerPath, "/example") || strings.HasPrefix(lowerPath, "example") {
			add(RoleExample)
		}
		if strings.Contains(lowerPath, "testdata/") ||
			strings.Contains(lowerPath, "golden") || strings.Contains(lowerPath, "snapshot") {
			add(RoleGeneratedOutput)
		}
		return roles
	}
	if isDocumentPath(anchor.Path) && hasAny("go test", "make ", "release", "usage", "run ") {
		add(RoleRepositoryVerificationCommand)
	}
	if path.Ext(lowerPath) == ".sh" {
		add(RoleOperationalEntry)
		add(RoleProceduralBody)
		if hasAny("confirm", "conflict", "exists", "semantic version", "semver", "before", "dry") {
			add(RoleSafetyCheck)
		}
	}
	if path.Base(lowerPath) == "makefile" {
		add(RoleOperationalEntry)
		if hasAny("go test", "check", "verify") {
			add(RoleRepositoryVerificationCommand)
		}
	}
	if hasAny("interface {") && hasAny("register", "extension", "adapter", "handler") {
		add(RoleExtensionPort)
	}
	if hasAny("wire", "compose", "mount", "use(", "register(", "append(") {
		add(RoleWiringComposition)
	}

	switch profile {
	case TaskProfileDataTagTransformation:
		if hasAny("parse", "transform", "convert", "customiz", "tag", "example", "nullable", "required") {
			add(RoleTransformation)
		}
		if strings.Contains(lowerPath, "golden") || strings.Contains(lowerPath, "generated") ||
			strings.Contains(lowerPath, "testdata/") || hasAny("code generated") {
			add(RoleGeneratedOutput)
		}
		if startsLikeExportedIdentifier(baseSymbol(anchor.Symbol)) &&
			hasAny("register", "customiz", "generate", "route", "schema") {
			add(RolePublicOrCLIEntry)
		}
	case TaskProfileErrorStatusMapping:
		if taskTermsAppearInAnchor(taskText, anchor) && !isDocumentPath(anchor.Path) {
			add(RoleSymptomSite)
		}
		if hasAny("errors.new(", "fmt.errorf(") ||
			hasErrorSubject && symbolHas("new", "create", "status", "code") {
			add(RoleErrorCreation)
		}
		if anchorHasExactErrorMappingSyntax(anchor) || hasErrorSubject && symbolHas(
			"send", "write", "respond", "response", "handle", "handler",
			"map", "mapper", "normalize", "translate", "serialize", "status",
		) {
			add(RoleErrorMapping)
		}
	case TaskProfileNilPanic:
		if hasAny("reflect.typeof(", "reflect.valueof(", ".elem()", "interface()", "[0]") {
			add(RoleUnsafeOperation)
		}
		if hasAny("body", "decode", "readjson", "validate", "transform") {
			add(RoleNilHandoff)
		}
		if startsLikeExportedIdentifier(baseSymbol(anchor.Symbol)) || strings.Contains(lowerSymbol, ".body") {
			add(RolePublicOrCLIEntry)
		}
	case TaskProfileConfigurationPropagation:
		if hasAny("config", "option", "setting") && hasAny(" struct", "type ", "default") {
			add(RoleConfigurationSource)
		}
		if hasAny("with", "copy", "apply", "merge") && hasAny("config", "option", "setting") &&
			hasAny(" = ", "=") {
			add(RoleConfigurationCopy)
		}
		if taskTermsAppearInAnchor(taskText, anchor) && hasAny("if ", "return", "openapi", "config") {
			add(RoleEffectiveDestination)
		}
	case TaskProfileErrorNormalizationPrivacy:
		if hasAny("type ", " struct") && hasAny("error", "err") {
			add(RolePublicErrorType)
		}
		if anchorHasErrorRepresentationExtraction(anchor) || hasErrorSubject && symbolHas(
			"normalize", "normalizer", "handle", "handler", "map", "mapper",
			"cast", "coerce", "convert", "translate",
		) {
			add(RoleErrorNormalizer)
		}
		if anchorHasExactExposureSyntax(anchor) || hasErrorSubject && symbolHas(
			"public", "send", "write", "respond", "response", "serialize",
			"render", "expose", "encode",
		) {
			add(RolePublicErrorExposure)
		}
	}
	return roles
}

func anchorText(anchor Anchor) string {
	var builder strings.Builder
	for _, line := range anchor.Excerpt {
		builder.WriteString(line.Text)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func taskTermsAppearInAnchor(taskText string, anchor Anchor) bool {
	for _, term := range extractTerms(taskText) {
		if term.Weight >= 8 && !genericGrepTerm(term.Normalized) && anchorContainsExact(anchor, term.Normalized) {
			return true
		}
	}
	return false
}

func selectAnchorsForRoleContract(
	candidates []anchorCandidate,
	terms []Term,
	contract RoleContract,
) []Anchor {
	selected := make([]Anchor, 0, min(MaxRetainedAnchors, len(candidates)))
	seen := make(map[string]struct{})
	perFile := make(map[string]int)
	add := func(anchor Anchor) {
		if len(selected) >= MaxRetainedAnchors || perFile[anchor.Path] >= maxAnchorsPerFile {
			return
		}
		if _, exists := seen[anchor.ID]; exists {
			return
		}
		seen[anchor.ID] = struct{}{}
		perFile[anchor.Path]++
		selected = append(selected, anchor)
	}
	reserve := func(requirements []RoleRequirement, extraPerRole int) {
		for _, requirement := range requirements {
			target := requirement.MinimumAnchors
			if extraPerRole > 0 {
				target = extraPerRole
			}
			ranked := append([]anchorCandidate(nil), candidates...)
			sort.SliceStable(ranked, func(left, right int) bool {
				leftScore := roleCandidateScore(ranked[left], requirement.Role, terms, contract.Profile)
				rightScore := roleCandidateScore(ranked[right], requirement.Role, terms, contract.Profile)
				if leftScore != rightScore {
					return leftScore > rightScore
				}
				return ranked[left].anchor.ID < ranked[right].anchor.ID
			})
			added := 0
			for _, candidate := range ranked {
				if !slices.Contains(candidate.anchor.RoleHints, requirement.Role) {
					continue
				}
				before := len(selected)
				add(candidate.anchor)
				if len(selected) > before {
					added++
				}
				if added >= target {
					break
				}
			}
		}
	}
	reserve(contract.Key, 0)
	reserve(contract.Supporting, 0)
	for _, anchor := range selectAnchors(candidates, terms) {
		add(anchor)
	}
	return selected
}

func roleCandidateScore(
	candidate anchorCandidate,
	role AnchorRole,
	terms []Term,
	profile TaskProfile,
) int {
	score := candidate.score - strings.Count(candidate.anchor.Path, "/")*12
	termWeights := make(map[string]int, len(terms))
	for _, term := range terms {
		termWeights[term.ID] = term.Weight
	}
	for _, termID := range candidate.terms {
		score += termWeights[termID] * 2
	}
	score += symbolTaskAffinity(candidate.anchor.Symbol, terms)
	if candidate.anchor.Scope.isComplete() {
		score += 24
	}

	words := identifierWordSet(candidate.anchor.Symbol)
	has := func(values ...string) bool {
		for _, value := range values {
			if _, ok := words[value]; ok {
				return true
			}
		}
		return false
	}
	switch role {
	case RolePublicOrCLIEntry:
		if has("register", "serve", "main", "command", "run") {
			score += 120
		} else if has("handle", "send", "read", "generate", "output") {
			score += 80
		} else if has("new", "create", "build") {
			score += 40
		}
		if profile == TaskProfileNilPanic && has("validate", "transform", "body") {
			score += 180
		}
	case RoleSymptomSite:
		if has("send", "response", "status", "handle", "validate", "register") {
			score += 100
		}
		if profile == TaskProfileErrorStatusMapping && has("send") {
			score += 220
		}
	case RoleTransformation:
		if has("parse", "transform", "convert", "customizer", "customize", "determine", "decode", "encode", "normalize") {
			score += 180
		}
	case RoleErrorCreation:
		if has("error", "err") {
			score += 100
		}
	case RoleErrorMapping:
		if has("map", "mapper", "status", "serialize", "serializer", "handler") {
			score += 130
		}
		if has("send") && has("error", "err") {
			score += 180
		}
		hasErrorAction := has(
			"send", "write", "respond", "response", "handle", "handler",
			"map", "mapper", "normalize", "translate", "serialize", "render",
		)
		if hasErrorAction && has("error", "err", "failure", "status") {
			score += 120
		}
	case RoleUnsafeOperation:
		if has("validate", "dereference", "reflect", "unsafe") {
			score += 160
		}
	case RoleNilHandoff:
		if has("body", "validate", "decode", "read", "transform") {
			score += 120
		}
	case RoleConfigurationCopy:
		if has("with", "copy", "apply", "merge", "set") {
			score += 160
		}
	case RoleConfigurationSource, RoleEffectiveDestination:
		if has("config", "configuration", "option", "settings", "engine") {
			score += 100
		}
	case RoleErrorNormalizer:
		if has("handle", "handler", "normalize", "normalizer") {
			score += 180
		}
	case RolePublicErrorExposure:
		if has("public", "send", "serialize", "response", "handler") {
			score += 140
		}
	case RoleVerificationAnchor:
		if isTestPath(candidate.anchor.Path) {
			score += 100
		}
	case RoleGeneratedOutput:
		if isGeneratedFixtureAnchor(candidate.anchor) {
			score += 180
		}
	case RoleDocumentationContract:
		if strings.Contains(strings.ToLower(path.Base(candidate.anchor.Path)), "contribut") ||
			strings.Contains(strings.ToLower(path.Base(candidate.anchor.Path)), "release") {
			score += 160
		}
	case RoleModuleTopology:
		base := path.Base(strings.ToLower(candidate.anchor.Path))
		if base == "go.work" {
			score += 220
		} else if base == "go.mod" && path.Dir(candidate.anchor.Path) == "." {
			score += 140
		}
	case RoleOperationalEntry, RoleProceduralBody, RoleSafetyCheck:
		if isOperationalSourcePath(candidate.anchor.Path) {
			score += 180
		}
	}
	return score
}

func symbolTaskAffinity(symbol string, terms []Term) int {
	words := identifierWordSet(symbol)
	score := 0
	for word := range words {
		for _, term := range terms {
			if lexicalStemOverlap(word, term.Normalized) {
				score += term.Weight * 12
			}
		}
	}
	return min(score, 600)
}

func lexicalStemOverlap(left, right string) bool {
	left = strings.ToLower(strings.TrimSpace(left))
	right = strings.ToLower(strings.TrimSpace(right))
	if left == right && len(left) >= 3 {
		return true
	}
	minimum := min(len(left), len(right))
	if minimum < 5 {
		return false
	}
	common := 0
	for common < minimum && left[common] == right[common] {
		common++
	}
	return common >= 5
}

func completeMissingRoleAnchors(
	anchors []Anchor,
	candidates []anchorCandidate,
	terms []Term,
	contract RoleContract,
) ([]Anchor, int) {
	result := append([]Anchor(nil), anchors...)
	expansions := 0
	for expansions < MaxFrontierExpansions {
		coverage, err := EvaluateRoleCoverage(contract, result)
		if err != nil {
			break
		}

		relations := collectRelations(result, terms)
		decisive, decisiveFound := selectDecisiveRelation(
			relations,
			result,
			terms,
			contract.Profile,
		)
		protected := map[string]struct{}{}
		if decisiveFound {
			protected[decisive.LeftID] = struct{}{}
			protected[decisive.RightID] = struct{}{}
		}

		candidateIndex := -1
		var completed []Anchor
		if missing := coverage.MissingKeyRoles(); len(missing) > 0 {
			candidateIndex, completed = bestMissingRoleCompletion(
				result,
				candidates,
				terms,
				contract,
				missing,
				protected,
			)
		} else {
			candidateIndex, completed = bestDecisiveRelationCompletion(
				result,
				candidates,
				terms,
				contract,
				decisive,
				decisiveFound,
				protected,
			)
		}
		if candidateIndex < 0 {
			break
		}
		result = completed
		// Both completion selectors return -1 or an index from ranging candidates.
		//nolint:nilaway
		if expansions == 0 {
			candidates[candidateIndex].stage = RetrievalStageCompletion1
		} else {
			candidates[candidateIndex].stage = RetrievalStageCompletion2
		}
		expansions++
	}
	return result, expansions
}

func bestMissingRoleCompletion(
	anchors []Anchor,
	candidates []anchorCandidate,
	terms []Term,
	contract RoleContract,
	missing []AnchorRole,
	protected map[string]struct{},
) (int, []Anchor) {
	bestIndex, bestScore := -1, -1
	var best []Anchor
	for index, candidate := range candidates {
		if containsAnchorID(anchors, candidate.anchor.ID) {
			continue
		}
		covered := 0
		fit := 0
		for _, role := range missing {
			if !slices.Contains(candidate.anchor.RoleHints, role) {
				continue
			}
			covered++
			fit = max(fit, roleCandidateScore(candidate, role, terms, contract.Profile))
		}
		if covered == 0 {
			continue
		}
		trial, ok := completionTrialAnchors(anchors, candidate.anchor, contract, protected)
		if !ok {
			continue
		}
		coverage, err := EvaluateRoleCoverage(contract, trial)
		if err != nil || len(coverage.MissingKeyRoles()) >= len(missing) {
			continue
		}
		score := covered*10_000 + fit
		if score > bestScore || score == bestScore &&
			(bestIndex < 0 || candidate.anchor.ID < candidates[bestIndex].anchor.ID) {
			bestIndex, bestScore, best = index, score, trial
		}
	}
	return bestIndex, best
}

func bestDecisiveRelationCompletion(
	anchors []Anchor,
	candidates []anchorCandidate,
	terms []Term,
	contract RoleContract,
	current Relation,
	currentFound bool,
	protected map[string]struct{},
) (int, []Anchor) {
	currentScore := -1
	if currentFound && isStrongExactLocalRelation(current) {
		currentScore = decisiveRelationScore(current, anchors, terms, contract.Profile)
	}
	bestIndex, bestGain, bestFit := -1, 0, -1
	var best []Anchor
	for index, candidate := range candidates {
		if containsAnchorID(anchors, candidate.anchor.ID) || isTestOrExampleAnchor(candidate.anchor) ||
			isGeneratedFixtureAnchor(candidate.anchor) {
			continue
		}
		trial, ok := completionTrialAnchors(anchors, candidate.anchor, contract, protected)
		if !ok {
			continue
		}
		coverage, err := EvaluateRoleCoverage(contract, trial)
		if err != nil || len(coverage.MissingKeyRoles()) > 0 {
			continue
		}
		trialRelations := collectRelations(trial, terms)
		decisive, found := selectDecisiveRelation(
			trialRelations,
			trial,
			terms,
			contract.Profile,
		)
		if !found || !isStrongExactLocalRelation(decisive) ||
			(decisive.LeftID != candidate.anchor.ID && decisive.RightID != candidate.anchor.ID) {
			continue
		}
		gain := decisiveRelationScore(decisive, trial, terms, contract.Profile) - currentScore
		if gain <= 0 {
			continue
		}
		fit := candidate.score
		for _, requirement := range contract.Key {
			if slices.Contains(candidate.anchor.RoleHints, requirement.Role) {
				fit = max(fit, roleCandidateScore(
					candidate,
					requirement.Role,
					terms,
					contract.Profile,
				))
			}
		}
		if gain > bestGain || gain == bestGain && (fit > bestFit || fit == bestFit &&
			(bestIndex < 0 || candidate.anchor.ID < candidates[bestIndex].anchor.ID)) {
			bestIndex, bestGain, bestFit, best = index, gain, fit, trial
		}
	}
	if bestIndex >= 0 {
		return bestIndex, best
	}
	return bestParallelTransformationCompletion(
		anchors,
		candidates,
		terms,
		contract,
		current,
		currentFound,
		protected,
	)
}

// bestParallelTransformationCompletion spends a remaining completion slot on
// a sibling transformation branch only when an exact retained caller invokes
// both helpers from the same source file. This keeps scalar/collection or
// encode/decode branches together without constructing a repository-wide call
// graph or treating lexical similarity as a call edge.
func bestParallelTransformationCompletion(
	anchors []Anchor,
	candidates []anchorCandidate,
	terms []Term,
	contract RoleContract,
	current Relation,
	currentFound bool,
	protected map[string]struct{},
) (int, []Anchor) {
	if contract.Profile != TaskProfileDataTagTransformation || !currentFound ||
		RelationKind(current.Kind) != RelationDirectCall || !isStrongExactLocalRelation(current) {
		return -1, nil
	}

	anchorByID := make(map[string]Anchor, len(anchors))
	for _, anchor := range anchors {
		anchorByID[anchor.ID] = anchor
	}
	caller, callerFound := anchorByID[current.LeftID]
	transformation, transformationFound := anchorByID[current.RightID]
	if !callerFound || !transformationFound ||
		!slices.Contains(caller.RoleHints, RolePublicOrCLIEntry) ||
		!slices.Contains(transformation.RoleHints, RoleTransformation) {
		return -1, nil
	}

	bestIndex, bestFit := -1, -1
	var best []Anchor
	for index, candidate := range candidates {
		if containsAnchorID(anchors, candidate.anchor.ID) ||
			candidate.anchor.Path != transformation.Path ||
			!slices.Contains(candidate.anchor.RoleHints, RoleTransformation) {
			continue
		}
		trial, ok := completionTrialAnchors(anchors, candidate.anchor, contract, protected)
		if !ok {
			continue
		}
		linked := false
		for _, relation := range collectRelations(trial, terms) {
			if relation.LeftID == caller.ID && relation.RightID == candidate.anchor.ID &&
				RelationKind(relation.Kind) == RelationDirectCall &&
				isStrongExactLocalRelation(relation) {
				linked = true
				break
			}
		}
		if !linked {
			continue
		}
		fit := roleCandidateScore(
			candidate,
			RoleTransformation,
			terms,
			contract.Profile,
		)
		if fit > bestFit || fit == bestFit &&
			(bestIndex < 0 || candidate.anchor.ID < candidates[bestIndex].anchor.ID) {
			bestIndex, bestFit, best = index, fit, trial
		}
	}
	return bestIndex, best
}

// completeVerificationAnchor is the single bounded verification probe that
// follows decisive-relation completion. It can replace one lower-value
// retained anchor, but it never spends or disguises a completion expansion.
func completeVerificationAnchor(
	anchors []Anchor,
	candidates []anchorCandidate,
	terms []Term,
	contract RoleContract,
) []Anchor {
	relations := collectRelations(anchors, terms)
	decisive, found := selectDecisiveRelation(relations, anchors, terms, contract.Profile)
	if !found || !isStrongExactLocalRelation(decisive) {
		return anchors
	}
	protected := map[string]struct{}{decisive.LeftID: {}, decisive.RightID: {}}
	bestIndex, bestScore := -1, -1
	var best []Anchor
	for index, candidate := range candidates {
		if containsAnchorID(anchors, candidate.anchor.ID) ||
			(!isTestOrExampleAnchor(candidate.anchor) && !isGeneratedFixtureAnchor(candidate.anchor) &&
				!isDocumentPath(candidate.anchor.Path)) {
			continue
		}
		trial, ok := completionTrialAnchors(anchors, candidate.anchor, contract, protected)
		if !ok {
			continue
		}
		trialRelations := collectRelations(trial, terms)
		trialDecisive, exists := relationByID(trialRelations, decisive.ID)
		if !exists {
			continue
		}
		frontier := buildVerificationFrontier(trial, trialRelations, trialDecisive, terms)
		score, exact := verificationCandidateFrontierScore(candidate.anchor, frontier)
		if !exact {
			continue
		}
		score += candidate.score + symbolTaskAffinity(candidate.anchor.Symbol, terms) +
			verificationGraphProximity(candidate.anchor, trial, trialDecisive, trialRelations)
		if score > bestScore || score == bestScore &&
			(bestIndex < 0 || candidate.anchor.ID < candidates[bestIndex].anchor.ID) {
			bestIndex, bestScore, best = index, score, trial
		}
	}
	if bestIndex < 0 {
		return anchors
	}
	// bestIndex is assigned only from the candidates range above.
	//nolint:nilaway
	candidates[bestIndex].stage = RetrievalStageVerification
	return best
}

func verificationGraphProximity(
	anchor Anchor,
	anchors []Anchor,
	decisive Relation,
	relations []Relation,
) int {
	distance, linked := decisiveStrongDistances(decisive, anchors, relations)[anchor.ID]
	if linked {
		return max(0, 10_000-distance*2_000)
	}
	return 0
}

func verificationCandidateFrontierScore(anchor Anchor, frontier VerificationFrontier) (int, bool) {
	for index, item := range frontier.Anchors {
		if item.AnchorID != anchor.ID {
			continue
		}
		switch item.Authority {
		case VerificationExactExistingTest:
			return 50_000 - index*1_000, true
		case VerificationExactExample:
			return 30_000 - index*1_000, true
		default:
			return 0, false
		}
	}
	if frontier.Fixture != nil && frontier.Fixture.AnchorID == anchor.ID &&
		frontier.Fixture.Authority == VerificationExactGeneratedFixture {
		return 40_000, true
	}
	if frontier.CommandOrEffect != nil && frontier.CommandOrEffect.AnchorID == anchor.ID &&
		frontier.CommandOrEffect.Authority == VerificationDocumentedCommand {
		return 20_000, true
	}
	return 0, false
}

func relationByID(relations []Relation, id string) (Relation, bool) {
	for _, relation := range relations {
		if relation.ID == id {
			return relation, true
		}
	}
	return Relation{}, false
}

func completionTrialAnchors(
	anchors []Anchor,
	candidate Anchor,
	contract RoleContract,
	protected map[string]struct{},
) ([]Anchor, bool) {
	if containsAnchorID(anchors, candidate.ID) {
		return nil, false
	}
	currentCoverage, err := EvaluateRoleCoverage(contract, anchors)
	if err != nil {
		return nil, false
	}
	currentMissing := len(currentCoverage.MissingKeyRoles())
	build := func(remove int) ([]Anchor, bool) {
		trial := make([]Anchor, 0, min(MaxRetainedAnchors, len(anchors)+1))
		for index, anchor := range anchors {
			if index != remove {
				trial = append(trial, anchor)
			}
		}
		trial = append(trial, candidate)
		if !withinPerFileAnchorLimit(trial) {
			return nil, false
		}
		coverage, coverageErr := EvaluateRoleCoverage(contract, trial)
		if coverageErr != nil || len(coverage.MissingKeyRoles()) > currentMissing {
			return nil, false
		}
		return trial, true
	}
	if len(anchors) < MaxRetainedAnchors {
		return build(-1)
	}
	for index := len(anchors) - 1; index >= 0; index-- {
		if _, keep := protected[anchors[index].ID]; keep {
			continue
		}
		if trial, ok := build(index); ok {
			return trial, true
		}
	}
	return nil, false
}

func withinPerFileAnchorLimit(anchors []Anchor) bool {
	counts := make(map[string]int)
	for _, anchor := range anchors {
		counts[anchor.Path]++
		if counts[anchor.Path] > maxAnchorsPerFile {
			return false
		}
	}
	return true
}

func containsAnchorID(anchors []Anchor, id string) bool {
	for _, anchor := range anchors {
		if anchor.ID == id {
			return true
		}
	}
	return false
}

func selectDecisiveRelation(
	relations []Relation,
	anchors []Anchor,
	terms []Term,
	profile TaskProfile,
) (Relation, bool) {
	anchorByID := make(map[string]Anchor, len(anchors))
	for _, anchor := range anchors {
		anchorByID[anchor.ID] = anchor
	}
	bestIndex, bestScore := -1, -1
	for index, relation := range relations {
		if relation.SupportType != SupportLocallyObserved {
			continue
		}
		_, leftOK := anchorByID[relation.LeftID]
		_, rightOK := anchorByID[relation.RightID]
		if !leftOK || !rightOK {
			continue
		}
		score := decisiveRelationScore(relation, anchors, terms, profile)
		betterTie := score == bestScore && bestIndex >= 0 && relation.ID < relations[bestIndex].ID
		if score > bestScore || betterTie {
			bestIndex, bestScore = index, score
		}
	}
	if bestIndex < 0 || bestScore <= 0 {
		return Relation{}, false
	}
	return relations[bestIndex], true
}

func decisiveRelationScore(
	relation Relation,
	anchors []Anchor,
	terms []Term,
	profile TaskProfile,
) int {
	priority := map[string]int{
		string(RelationConfigApplied):    100,
		string(RelationErrorMapped):      95,
		string(RelationErrorCreated):     90,
		string(RelationValueTransformed): 85,
		string(RelationFieldCopy):        80,
		string(RelationDirectCall):       75,
		string(RelationScriptInvokes):    70,
		string(RelationErrorExposed):     65,
		string(RelationFieldRead):        60,
		string(RelationFieldWrite):       55,
		string(RelationFixtureRecords):   50,
		string(RelationTestExercises):    40,
		string(RelationDocumentedUses):   30,
	}
	anchorByID := make(map[string]Anchor, len(anchors))
	for _, anchor := range anchors {
		anchorByID[anchor.ID] = anchor
	}
	left, leftOK := anchorByID[relation.LeftID]
	right, rightOK := anchorByID[relation.RightID]
	if !leftOK || !rightOK {
		return -1
	}
	score := priority[relation.Kind]*10 +
		(left.Score+right.Score)/4 +
		symbolTaskAffinity(left.Symbol, terms) +
		symbolTaskAffinity(right.Symbol, terms) +
		decisiveProfileFit(profile, RelationKind(relation.Kind), left, right)
	// Tests and fixtures verify a production relation; they do not replace it
	// as the model-facing decisive source relation merely because their helper
	// names have high task affinity.
	if isTestOrExampleAnchor(left) || isTestOrExampleAnchor(right) {
		score -= 1_500
	}
	if isGeneratedFixtureAnchor(left) || isGeneratedFixtureAnchor(right) {
		score -= 1_000
	}
	return score
}

func decisiveProfileFit(profile TaskProfile, kind RelationKind, left, right Anchor) int {
	hasEither := func(role AnchorRole) bool {
		return slices.Contains(left.RoleHints, role) || slices.Contains(right.RoleHints, role)
	}
	hasAcross := func(first, second AnchorRole) bool {
		return slices.Contains(left.RoleHints, first) && slices.Contains(right.RoleHints, second) ||
			slices.Contains(right.RoleHints, first) && slices.Contains(left.RoleHints, second)
	}
	score := 0
	switch profile {
	case TaskProfileDataTagTransformation:
		if hasAcross(RolePublicOrCLIEntry, RoleTransformation) {
			score += 1_000
		}
		if hasAcross(RoleTransformation, RoleGeneratedOutput) {
			score += 700
		}
		if kind == RelationDirectCall || kind == RelationValueTransformed || kind == RelationTypeNameGenerated {
			score += 300
		}
	case TaskProfileErrorStatusMapping:
		if hasAcross(RoleErrorCreation, RoleErrorMapping) {
			score += 1_000
		}
		if hasEither(RoleSymptomSite) && (hasEither(RoleErrorCreation) || hasEither(RoleErrorMapping)) {
			score += 700
		}
		if kind == RelationErrorMapped || kind == RelationErrorCreated || kind == RelationErrorExposed {
			score += 300
		}
		score += errorStatusRelationSourceFit(left)
	case TaskProfileNilPanic:
		if hasAcross(RoleUnsafeOperation, RoleNilHandoff) {
			score += 1_100
		}
		if hasAcross(RoleUnsafeOperation, RolePublicOrCLIEntry) {
			score += 800
		}
		if kind == RelationDirectCall || kind == RelationFieldRead {
			score += 250
		}
	case TaskProfileConfigurationPropagation:
		if hasAcross(RoleConfigurationSource, RoleConfigurationCopy) ||
			hasAcross(RoleConfigurationCopy, RoleEffectiveDestination) {
			score += 1_100
		}
		if kind == RelationConfigApplied || kind == RelationFieldCopy || kind == RelationFieldRead || kind == RelationFieldWrite {
			score += 350
		}
	case TaskProfileErrorNormalizationPrivacy:
		if hasAcross(RolePublicErrorType, RoleErrorNormalizer) ||
			hasAcross(RoleErrorNormalizer, RolePublicErrorExposure) {
			score += 1_100
		}
		if kind == RelationErrorMapped || kind == RelationErrorExposed || kind == RelationDirectCall {
			score += 300
		}
		if anchorMaterializesErrorRepresentation(left) || anchorMaterializesErrorRepresentation(right) {
			// A locally materialized errors.As/type-switch target is stronger
			// normalization evidence than a dispatcher that merely forwards the
			// original error to an unretained helper.
			score += 1_800
		}
	case TaskProfileOperationalRelease:
		if hasAcross(RoleOperationalEntry, RoleModuleTopology) ||
			hasAcross(RoleProceduralBody, RoleModuleTopology) ||
			hasAcross(RoleOperationalEntry, RoleDocumentationContract) {
			score += 1_000
		}
		if kind == RelationScriptInvokes || kind == RelationDocumentedUses {
			score += 350
		}
	case TaskProfileExtensionContribution:
		if hasAcross(RoleExtensionPort, RoleRepresentativeImplementation) ||
			hasAcross(RoleRepresentativeImplementation, RoleWiringComposition) {
			score += 1_000
		}
	}
	return score
}

func errorStatusRelationSourceFit(source Anchor) int {
	words := identifierWordSet(baseSymbol(source.Symbol))
	hasWord := func(values ...string) bool {
		for _, value := range values {
			if _, exists := words[value]; exists {
				return true
			}
		}
		return false
	}

	// Prefer the operation that maps or hands an error to the public boundary.
	// Constructors and wiring functions can mention the same error symbols, but
	// that mention is weaker evidence for the status selected at the boundary.
	isHandoff := hasWord(
		"send", "write", "respond", "response", "handle", "handler",
		"map", "normalize", "translate", "serialize", "render", "expose",
	)
	hasErrorSubject := hasWord("error", "err", "failure", "status")
	score := 0
	if isHandoff && hasErrorSubject {
		score += 2_100
	} else if isHandoff {
		score += 450
	}

	isConstructorOrWiring := hasWord(
		"new", "create", "build", "init", "initialize", "register", "setup", "wire",
	)
	if isConstructorOrWiring && !isHandoff {
		score -= 700
	}
	return score
}

func buildVerificationFrontier(
	anchors []Anchor,
	relations []Relation,
	decisive Relation,
	terms []Term,
) VerificationFrontier {
	frontier := VerificationFrontier{Anchors: []VerificationItem{}}
	if decisive.ID != "" {
		frontier.DecisiveAnchorID = decisive.LeftID
	}
	linked := verificationSourceComponent(decisive, anchors, relations)
	rankedLinked := rankLinkedVerificationAnchors(decisive, anchors, relations, terms)
	addAnchor := func(anchor Anchor, authority VerificationAuthority, text string) {
		if len(frontier.Anchors) >= MaxVerificationAnchors {
			return
		}
		for _, existing := range frontier.Anchors {
			if existing.AnchorID == anchor.ID {
				return
			}
		}
		frontier.Anchors = append(frontier.Anchors, VerificationItem{
			ID: OpaqueID("verification", string(authority), anchor.ID), Authority: authority,
			AnchorID: anchor.ID, Path: anchor.Path, Symbol: anchor.Symbol, Text: text,
			EvidenceIDs: append([]string(nil), anchor.EvidenceIDs...),
		})
	}
	for _, anchor := range rankedLinked {
		if !isTestOrExampleAnchor(anchor) {
			continue
		}
		authority := VerificationExactExistingTest
		if isExampleAnchor(anchor) {
			authority = VerificationExactExample
		}
		addAnchor(
			anchor,
			authority,
			"Exact retained test or example calls a decisive endpoint or asserts task-observable concepts; this does not establish coverage of every task case.",
		)
	}
	if len(frontier.Anchors) == 0 {
		if anchor, ok := nearestSiblingVerificationAnchor(anchors, decisive, linked, terms); ok {
			addAnchor(
				anchor,
				VerificationProposedTestLocation,
				"Nearest retained sibling test location; no exact strong link to the decisive relation was observed.",
			)
		}
	}
	for _, anchor := range rankedLinked {
		if frontier.Fixture == nil && isGeneratedFixtureAnchor(anchor) {
			item := VerificationItem{
				ID:        OpaqueID("verification", string(VerificationExactGeneratedFixture), anchor.ID),
				Authority: VerificationExactGeneratedFixture, AnchorID: anchor.ID,
				Path: anchor.Path, Symbol: anchor.Symbol,
				Text:        "Exact retained generated or golden fixture is strongly linked to the decisive relation.",
				EvidenceIDs: append([]string(nil), anchor.EvidenceIDs...),
			}
			frontier.Fixture = &item
		}
		if frontier.CommandOrEffect == nil &&
			(isDocumentPath(anchor.Path) || path.Base(strings.ToLower(anchor.Path)) == "makefile") {
			if command := exactDocumentedCommand(anchor); command != "" {
				item := VerificationItem{
					ID:        OpaqueID("verification", string(VerificationDocumentedCommand), anchor.ID, command),
					Authority: VerificationDocumentedCommand, AnchorID: anchor.ID,
					Path: anchor.Path, Symbol: anchor.Symbol, Text: command,
					EvidenceIDs: append([]string(nil), anchor.EvidenceIDs...),
				}
				frontier.CommandOrEffect = &item
			}
		}
	}
	if len(frontier.Anchors) == 0 && frontier.Fixture == nil && frontier.CommandOrEffect == nil {
		frontier.Anchors = append(frontier.Anchors, VerificationItem{
			ID:          OpaqueID("verification", "missing", frontier.DecisiveAnchorID),
			Authority:   VerificationMissingEvidence,
			Text:        "No exact repository-owned test, fixture, example, or documented command was retained.",
			EvidenceIDs: []string{},
		})
	}
	return frontier
}

type linkedVerificationAnchor struct {
	anchor            Anchor
	authorityRank     int
	endpointLinkRank  int
	graphDistance     int
	directoryDistance int
	endpointAffinity  int
	taskAffinity      int
}

type verificationBinding struct {
	endpointLinkRank int
	graphDistance    int
}

func rankLinkedVerificationAnchors(
	decisive Relation,
	anchors []Anchor,
	relations []Relation,
	terms []Term,
) []Anchor {
	anchorByID := make(map[string]Anchor, len(anchors))
	for _, anchor := range anchors {
		anchorByID[anchor.ID] = anchor
	}
	sourceDistances := verificationSourceDistances(decisive, anchors, relations)
	bindings := make(map[string]verificationBinding)
	boundTests := make(map[string]struct{})
	for _, anchor := range anchors {
		if !isTestOrExampleAnchor(anchor) {
			continue
		}
		binding, bound := bindVerificationTest(
			anchor,
			anchors,
			relations,
			sourceDistances,
			terms,
		)
		if !bound {
			continue
		}
		bindings[anchor.ID] = binding
		boundTests[anchor.ID] = struct{}{}
	}
	for _, anchor := range anchors {
		switch {
		case isGeneratedFixtureAnchor(anchor):
			if binding, bound := bindVerificationFixture(
				anchor,
				relations,
				sourceDistances,
				boundTests,
			); bound {
				bindings[anchor.ID] = binding
			}
		case isDocumentPath(anchor.Path) || path.Base(strings.ToLower(anchor.Path)) == "makefile":
			if exactDocumentedCommand(anchor) == "" {
				continue
			}
			if binding, bound := bindVerificationCommand(
				anchor,
				anchors,
				relations,
				sourceDistances,
			); bound {
				bindings[anchor.ID] = binding
			}
		}
	}
	endpoints := []Anchor{anchorByID[decisive.LeftID], anchorByID[decisive.RightID]}
	ranked := make([]linkedVerificationAnchor, 0, len(bindings))
	for _, anchor := range anchors {
		binding, linked := bindings[anchor.ID]
		if !linked {
			continue
		}
		directoryDistance := maxInt()
		for _, endpoint := range endpoints {
			if endpoint.ID == "" {
				continue
			}
			directoryDistance = min(
				directoryDistance,
				directoryDistanceBetweenAnchors(anchor, endpoint),
			)
		}
		ranked = append(ranked, linkedVerificationAnchor{
			anchor:            anchor,
			authorityRank:     verificationAnchorAuthorityRank(anchor),
			endpointLinkRank:  binding.endpointLinkRank,
			graphDistance:     binding.graphDistance,
			directoryDistance: directoryDistance,
			endpointAffinity:  verificationEndpointAffinity(anchor, endpoints),
			taskAffinity:      symbolTaskAffinity(anchor.Symbol, terms),
		})
	}
	sort.Slice(ranked, func(left, right int) bool {
		leftRank, rightRank := ranked[left], ranked[right]
		if leftRank.authorityRank != rightRank.authorityRank {
			return leftRank.authorityRank < rightRank.authorityRank
		}
		if leftRank.endpointLinkRank != rightRank.endpointLinkRank {
			return leftRank.endpointLinkRank < rightRank.endpointLinkRank
		}
		if leftRank.graphDistance != rightRank.graphDistance {
			return leftRank.graphDistance < rightRank.graphDistance
		}
		if leftRank.directoryDistance != rightRank.directoryDistance {
			return leftRank.directoryDistance < rightRank.directoryDistance
		}
		if leftRank.endpointAffinity != rightRank.endpointAffinity {
			return leftRank.endpointAffinity > rightRank.endpointAffinity
		}
		if leftRank.taskAffinity != rightRank.taskAffinity {
			return leftRank.taskAffinity > rightRank.taskAffinity
		}
		if leftRank.anchor.Score != rightRank.anchor.Score {
			return leftRank.anchor.Score > rightRank.anchor.Score
		}
		if leftRank.anchor.Path != rightRank.anchor.Path {
			return leftRank.anchor.Path < rightRank.anchor.Path
		}
		if leftRank.anchor.Symbol != rightRank.anchor.Symbol {
			return leftRank.anchor.Symbol < rightRank.anchor.Symbol
		}
		return leftRank.anchor.ID < rightRank.anchor.ID
	})
	result := make([]Anchor, 0, len(ranked))
	for _, item := range ranked {
		result = append(result, item.anchor)
	}
	return result
}

func verificationSourceComponent(
	decisive Relation,
	anchors []Anchor,
	relations []Relation,
) map[string]struct{} {
	component := make(map[string]struct{})
	for anchorID := range verificationSourceDistances(decisive, anchors, relations) {
		component[anchorID] = struct{}{}
	}
	return component
}

func verificationSourceDistances(
	decisive Relation,
	anchors []Anchor,
	relations []Relation,
) map[string]int {
	distances := make(map[string]int)
	if !isStrongExactLocalRelation(decisive) {
		return distances
	}
	anchorByID := make(map[string]Anchor, len(anchors))
	for _, anchor := range anchors {
		anchorByID[anchor.ID] = anchor
	}
	var queue []string
	for _, anchorID := range []string{decisive.LeftID, decisive.RightID} {
		anchor, exists := anchorByID[anchorID]
		if !exists || isVerificationTerminalAnchor(anchor) {
			continue
		}
		if _, duplicate := distances[anchorID]; duplicate {
			continue
		}
		distances[anchorID] = 0
		queue = append(queue, anchorID)
	}
	if len(queue) == 0 {
		return distances
	}
	adjacent := make(map[string][]string)
	for _, relation := range relations {
		if !isVerificationSourceRelation(relation) {
			continue
		}
		left, leftExists := anchorByID[relation.LeftID]
		right, rightExists := anchorByID[relation.RightID]
		if !leftExists || !rightExists ||
			isVerificationTerminalAnchor(left) || isVerificationTerminalAnchor(right) {
			continue
		}
		adjacent[relation.LeftID] = append(adjacent[relation.LeftID], relation.RightID)
		adjacent[relation.RightID] = append(adjacent[relation.RightID], relation.LeftID)
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, neighbor := range adjacent[current] {
			if _, seen := distances[neighbor]; seen {
				continue
			}
			distances[neighbor] = distances[current] + 1
			queue = append(queue, neighbor)
		}
	}
	return distances
}

func isVerificationSourceRelation(relation Relation) bool {
	if relation.SupportType != SupportLocallyObserved || len(relation.EvidenceIDs) == 0 {
		return false
	}
	switch RelationKind(relation.Kind) {
	case RelationDirectCall,
		RelationFieldCopy,
		RelationFieldRead,
		RelationFieldWrite,
		RelationErrorCreated,
		RelationErrorMapped,
		RelationErrorExposed,
		RelationValueTransformed,
		RelationTypeNameGenerated,
		RelationConfigApplied,
		RelationScriptInvokes:
		return true
	default:
		return false
	}
}

func isVerificationTerminalAnchor(anchor Anchor) bool {
	return isTestOrExampleAnchor(anchor) ||
		isGeneratedFixtureAnchor(anchor) ||
		isDocumentPath(anchor.Path) ||
		path.Base(strings.ToLower(anchor.Path)) == "makefile"
}

func bindVerificationTest(
	candidate Anchor,
	anchors []Anchor,
	relations []Relation,
	sourceDistances map[string]int,
	terms []Term,
) (verificationBinding, bool) {
	best := verificationBinding{}
	found := false
	consider := func(endpointRank, distance int) {
		if !found || endpointRank < best.endpointLinkRank ||
			endpointRank == best.endpointLinkRank && distance < best.graphDistance {
			best = verificationBinding{
				endpointLinkRank: endpointRank,
				graphDistance:    distance,
			}
			found = true
		}
	}
	for _, relation := range relations {
		if !isStrongExactLocalRelation(relation) {
			continue
		}
		otherID := ""
		switch {
		case relation.LeftID == candidate.ID:
			otherID = relation.RightID
		case relation.RightID == candidate.ID:
			otherID = relation.LeftID
		default:
			continue
		}
		distance, linked := sourceDistances[otherID]
		if !linked {
			continue
		}
		switch RelationKind(relation.Kind) {
		case RelationDirectCall:
			consider(0, distance+1)
		case RelationTestExercises:
			consider(1, distance+1)
		}
	}
	for _, source := range anchors {
		distance, linked := sourceDistances[source.ID]
		if !linked || !sameGoPackage(candidate, source) {
			continue
		}
		if testCallsReceiverMethod(candidate, source) {
			consider(1, distance+1)
		}
	}
	if observableAssertionConceptCount(candidate, anchors, sourceDistances, terms) >= 2 {
		distance := maxInt()
		for sourceID, sourceDistance := range sourceDistances {
			source := verificationAnchorByID(anchors, sourceID)
			if source.ID != "" && sameGoPackage(candidate, source) {
				distance = min(distance, sourceDistance+1)
			}
		}
		if distance != maxInt() {
			consider(2, distance)
		}
	}
	return best, found
}

func bindVerificationFixture(
	candidate Anchor,
	relations []Relation,
	sourceDistances map[string]int,
	boundTests map[string]struct{},
) (verificationBinding, bool) {
	bestDistance := maxInt()
	for _, relation := range relations {
		if RelationKind(relation.Kind) != RelationFixtureRecords ||
			!isStrongExactLocalRelation(relation) {
			continue
		}
		otherID := ""
		switch {
		case relation.LeftID == candidate.ID:
			otherID = relation.RightID
		case relation.RightID == candidate.ID:
			otherID = relation.LeftID
		default:
			continue
		}
		if distance, linked := sourceDistances[otherID]; linked {
			bestDistance = min(bestDistance, distance+1)
			continue
		}
		if _, linked := boundTests[otherID]; linked {
			bestDistance = min(bestDistance, 2)
		}
	}
	if bestDistance == maxInt() {
		return verificationBinding{}, false
	}
	return verificationBinding{endpointLinkRank: 2, graphDistance: bestDistance}, true
}

func bindVerificationCommand(
	candidate Anchor,
	anchors []Anchor,
	relations []Relation,
	sourceDistances map[string]int,
) (verificationBinding, bool) {
	bestDistance := maxInt()
	for _, relation := range relations {
		if !isStrongExactLocalRelation(relation) {
			continue
		}
		kind := RelationKind(relation.Kind)
		switch kind {
		case RelationDocumentedUses:
		case RelationScriptInvokes:
		default:
			continue
		}
		otherID := ""
		switch {
		case relation.LeftID == candidate.ID:
			otherID = relation.RightID
		case relation.RightID == candidate.ID:
			otherID = relation.LeftID
		default:
			continue
		}
		if distance, linked := sourceDistances[otherID]; linked {
			if kind == RelationDocumentedUses {
				source := verificationAnchorByID(anchors, otherID)
				target := baseSymbol(source.Symbol)
				if source.ID == "" || len(target) < 3 ||
					!anchorContainsExact(candidate, target) {
					continue
				}
			}
			bestDistance = min(bestDistance, distance+1)
		}
	}
	if bestDistance == maxInt() {
		return verificationBinding{}, false
	}
	return verificationBinding{endpointLinkRank: 2, graphDistance: bestDistance}, true
}

func testCallsReceiverMethod(test, production Anchor) bool {
	if !test.Scope.isComplete() || !sameGoPackage(test, production) {
		return false
	}
	method, receiver := receiverMethodNames(production.Symbol)
	if method == "" || receiver == "" {
		return false
	}
	parsed, err := parseGoAnchor(test)
	if err != nil {
		return false
	}
	found := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		if found {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != method {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		found = ok && lexicalStemOverlap(qualifier.Name, receiver)
		return !found
	})
	return found
}

func receiverMethodNames(symbol string) (method, receiver string) {
	symbol = strings.Trim(strings.TrimSpace(symbol), "()*")
	dot := strings.LastIndex(symbol, ".")
	if dot <= 0 || dot == len(symbol)-1 {
		return "", ""
	}
	method = strings.Trim(symbol[dot+1:], "()*")
	receiver = strings.Trim(symbol[:dot], "()*")
	if packageDot := strings.LastIndex(receiver, "."); packageDot >= 0 {
		receiver = receiver[packageDot+1:]
	}
	if bracket := strings.Index(receiver, "["); bracket >= 0 {
		receiver = receiver[:bracket]
	}
	return method, receiver
}

func observableAssertionConceptCount(
	candidate Anchor,
	anchors []Anchor,
	sourceDistances map[string]int,
	terms []Term,
) int {
	if !candidate.Scope.isComplete() || len(sourceDistances) == 0 {
		return 0
	}
	parsed, err := parseGoAnchor(candidate)
	if err != nil {
		return 0
	}
	sourceWords := make(map[string]struct{})
	samePackageSource := false
	for sourceID := range sourceDistances {
		source := verificationAnchorByID(anchors, sourceID)
		if source.ID == "" || !sameGoPackage(candidate, source) {
			continue
		}
		samePackageSource = true
		addVerificationNodeWords(sourceWords, source.Symbol)
		for _, line := range source.Excerpt {
			addVerificationNodeWords(sourceWords, line.Text)
		}
	}
	if !samePackageSource {
		return 0
	}
	assertedConcepts := make(map[string]struct{})
	addConcepts := func(node ast.Node) {
		observedWords := make(map[string]struct{})
		functionIdentifiers := make(map[*ast.Ident]struct{})
		ast.Inspect(node, func(current ast.Node) bool {
			call, ok := current.(*ast.CallExpr)
			if !ok {
				return true
			}
			ast.Inspect(call.Fun, func(functionNode ast.Node) bool {
				if identifier, isIdentifier := functionNode.(*ast.Ident); isIdentifier {
					functionIdentifiers[identifier] = struct{}{}
				}
				return true
			})
			return true
		})
		ast.Inspect(node, func(current ast.Node) bool {
			switch value := current.(type) {
			case *ast.Ident:
				if _, isFunctionIdentifier := functionIdentifiers[value]; isFunctionIdentifier {
					return true
				}
				addVerificationNodeWords(observedWords, value.Name)
			case *ast.BasicLit:
				addVerificationNodeWords(observedWords, value.Value)
			}
			return true
		})
		for _, term := range terms {
			if term.Weight < 8 || genericGrepTerm(term.Normalized) {
				continue
			}
			for concept := range identifierWordSet(term.Normalized) {
				if len(concept) < 3 || genericGrepTerm(concept) ||
					!verificationWordsOverlap(observedWords, concept) ||
					!verificationWordsOverlap(sourceWords, concept) {
					continue
				}
				assertedConcepts[concept] = struct{}{}
			}
		}
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.CallExpr:
			selector, ok := value.Fun.(*ast.SelectorExpr)
			if !ok || !isAssertionCall(selector) {
				return true
			}
			for _, argument := range value.Args {
				addConcepts(argument)
			}
		case *ast.IfStmt:
			if verificationBodyFailsTest(value.Body) {
				addConcepts(value.Cond)
			}
		}
		return true
	})
	return len(assertedConcepts)
}

func isAssertionCall(selector *ast.SelectorExpr) bool {
	qualifier, ok := selector.X.(*ast.Ident)
	if !ok || qualifier.Name != "assert" && qualifier.Name != "require" {
		return false
	}
	switch selector.Sel.Name {
	case "Equal", "NotEqual", "True", "False", "Contains", "NotContains",
		"Nil", "NotNil", "Len", "ElementsMatch", "InDelta", "Error",
		"NoError", "ErrorIs", "ErrorAs", "Regexp", "Match":
		return true
	default:
		return false
	}
}

func verificationBodyFailsTest(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if found {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if identifier, ok := call.Fun.(*ast.Ident); ok && identifier.Name == "panic" {
			found = true
			return false
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch selector.Sel.Name {
		case "Fatal", "Fatalf", "Error", "Errorf", "Fail", "FailNow":
			found = true
		}
		return !found
	})
	return found
}

func addVerificationNodeWords(destination map[string]struct{}, value string) {
	for word := range identifierWordSet(value) {
		destination[word] = struct{}{}
	}
}

func verificationWordsOverlap(words map[string]struct{}, concept string) bool {
	for word := range words {
		if lexicalStemOverlap(word, concept) {
			return true
		}
	}
	return false
}

func verificationAnchorByID(anchors []Anchor, id string) Anchor {
	for _, anchor := range anchors {
		if anchor.ID == id {
			return anchor
		}
	}
	return Anchor{}
}

func verificationEndpointLinkRank(
	candidateID string,
	endpoints []Anchor,
	relations []Relation,
) int {
	endpointIDs := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.ID != "" {
			endpointIDs[endpoint.ID] = struct{}{}
		}
	}

	rank := 3
	for _, relation := range relations {
		otherID := ""
		switch {
		case relation.LeftID == candidateID:
			otherID = relation.RightID
		case relation.RightID == candidateID:
			otherID = relation.LeftID
		default:
			continue
		}
		if _, ok := endpointIDs[otherID]; !ok {
			continue
		}

		relationRank := 2
		switch RelationKind(relation.Kind) {
		case RelationDirectCall:
			relationRank = 0
		case RelationTestExercises:
			relationRank = 1
		}
		rank = min(rank, relationRank)
	}
	return rank
}

func verificationAnchorAuthorityRank(anchor Anchor) int {
	switch {
	case isGeneratedFixtureAnchor(anchor):
		return 1
	case isExampleAnchor(anchor):
		return 2
	case strings.HasSuffix(strings.ToLower(anchor.Path), "_test.go"):
		return 0
	case isDocumentPath(anchor.Path) || path.Base(strings.ToLower(anchor.Path)) == "makefile":
		return 3
	default:
		return 4
	}
}

func directoryDistanceBetweenAnchors(left, right Anchor) int {
	return directoryDistance(path.Dir(left.Path), path.Dir(right.Path))
}

func verificationEndpointAffinity(candidate Anchor, endpoints []Anchor) int {
	candidateWords := identifierWordSet(baseSymbol(candidate.Symbol))
	affinity := 0
	for _, endpoint := range endpoints {
		if endpoint.ID == "" {
			continue
		}
		for candidateWord := range candidateWords {
			if candidateWord == "test" || candidateWord == "example" {
				continue
			}
			for endpointWord := range identifierWordSet(baseSymbol(endpoint.Symbol)) {
				if lexicalStemOverlap(candidateWord, endpointWord) {
					affinity++
				}
			}
		}
	}
	return affinity
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

func decisiveStrongComponent(
	decisive Relation,
	anchors []Anchor,
	relations []Relation,
) map[string]struct{} {
	component := make(map[string]struct{})
	for anchorID := range decisiveStrongDistances(decisive, anchors, relations) {
		component[anchorID] = struct{}{}
	}
	return component
}

func decisiveStrongDistances(
	decisive Relation,
	anchors []Anchor,
	relations []Relation,
) map[string]int {
	distances := make(map[string]int)
	if !isStrongExactLocalRelation(decisive) {
		return distances
	}
	anchorIDs := make(map[string]struct{}, len(anchors))
	for _, anchor := range anchors {
		anchorIDs[anchor.ID] = struct{}{}
	}
	if _, ok := anchorIDs[decisive.LeftID]; !ok {
		return distances
	}
	if _, ok := anchorIDs[decisive.RightID]; !ok {
		return distances
	}

	adjacent := make(map[string][]string)
	for _, relation := range relations {
		if !isStrongExactLocalRelation(relation) {
			continue
		}
		if _, ok := anchorIDs[relation.LeftID]; !ok {
			continue
		}
		if _, ok := anchorIDs[relation.RightID]; !ok {
			continue
		}
		adjacent[relation.LeftID] = append(adjacent[relation.LeftID], relation.RightID)
		adjacent[relation.RightID] = append(adjacent[relation.RightID], relation.LeftID)
	}

	queue := []string{decisive.LeftID, decisive.RightID}
	distances[decisive.LeftID] = 0
	distances[decisive.RightID] = 0
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, neighbor := range adjacent[current] {
			if _, seen := distances[neighbor]; seen {
				continue
			}
			distances[neighbor] = distances[current] + 1
			queue = append(queue, neighbor)
		}
	}
	return distances
}

func isStrongExactLocalRelation(relation Relation) bool {
	if relation.SupportType != SupportLocallyObserved || len(relation.EvidenceIDs) == 0 {
		return false
	}
	switch RelationKind(relation.Kind) {
	case RelationDirectCall,
		RelationFieldCopy,
		RelationFieldRead,
		RelationFieldWrite,
		RelationErrorCreated,
		RelationErrorMapped,
		RelationErrorExposed,
		RelationValueTransformed,
		RelationTypeNameGenerated,
		RelationConfigApplied,
		RelationScriptInvokes,
		RelationTestExercises,
		RelationFixtureRecords,
		RelationDocumentedUses:
		return true
	default:
		return false
	}
}

func isGeneratedFixtureAnchor(anchor Anchor) bool {
	lower := strings.ToLower(anchor.Path)
	return isTestPath(anchor.Path) &&
		(strings.Contains(lower, "testdata/") || strings.Contains(lower, "golden") ||
			strings.Contains(lower, "snapshot"))
}

func isExampleAnchor(anchor Anchor) bool {
	lowerPath := strings.ToLower(anchor.Path)
	isExampleGo := strings.HasSuffix(lowerPath, ".go") &&
		(strings.Contains(lowerPath, "/examples/") || strings.HasPrefix(lowerPath, "examples/") ||
			strings.Contains(lowerPath, "/example/") || strings.HasPrefix(lowerPath, "example/"))
	return isExampleGo ||
		strings.HasPrefix(baseSymbol(anchor.Symbol), "Example")
}

func isTestOrExampleAnchor(anchor Anchor) bool {
	return (strings.HasSuffix(strings.ToLower(anchor.Path), "_test.go") || isExampleAnchor(anchor)) &&
		!isGeneratedFixtureAnchor(anchor)
}

func nearestSiblingVerificationAnchor(
	anchors []Anchor,
	decisive Relation,
	linked map[string]struct{},
	terms []Term,
) (Anchor, bool) {
	type rankedAnchor struct {
		anchor       Anchor
		distance     int
		sharesTerm   bool
		originalRank int
	}
	anchorByID := make(map[string]Anchor, len(anchors))
	for _, anchor := range anchors {
		anchorByID[anchor.ID] = anchor
	}
	endpoints := []Anchor{anchorByID[decisive.LeftID], anchorByID[decisive.RightID]}
	candidates := make([]rankedAnchor, 0, len(anchors))
	for index, anchor := range anchors {
		if !isTestOrExampleAnchor(anchor) {
			continue
		}
		if _, exact := linked[anchor.ID]; exact {
			continue
		}
		distance := int(^uint(0) >> 1)
		sibling := false
		for _, endpoint := range endpoints {
			if endpoint.ID == "" {
				continue
			}
			candidateDistance := directoryDistance(path.Dir(anchor.Path), path.Dir(endpoint.Path))
			distance = min(distance, candidateDistance)
			sibling = sibling || candidateDistance == 0
		}
		if !sibling {
			continue
		}
		candidates = append(candidates, rankedAnchor{
			anchor:       anchor,
			distance:     distance,
			sharesTerm:   sharesStrongTaskTerm(anchor, terms),
			originalRank: index,
		})
	}
	if len(candidates) == 0 {
		return Anchor{}, false
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		if candidates[left].distance != candidates[right].distance {
			return candidates[left].distance < candidates[right].distance
		}
		if candidates[left].sharesTerm != candidates[right].sharesTerm {
			return candidates[left].sharesTerm
		}
		if candidates[left].anchor.Score != candidates[right].anchor.Score {
			return candidates[left].anchor.Score > candidates[right].anchor.Score
		}
		if candidates[left].anchor.Path != candidates[right].anchor.Path {
			return candidates[left].anchor.Path < candidates[right].anchor.Path
		}
		if candidates[left].anchor.Symbol != candidates[right].anchor.Symbol {
			return candidates[left].anchor.Symbol < candidates[right].anchor.Symbol
		}
		return candidates[left].originalRank < candidates[right].originalRank
	})
	return candidates[0].anchor, true
}

func directoryDistance(left, right string) int {
	leftParts := splitPathParts(left)
	rightParts := splitPathParts(right)
	shared := 0
	for shared < len(leftParts) && shared < len(rightParts) && leftParts[shared] == rightParts[shared] {
		shared++
	}
	return len(leftParts) - shared + len(rightParts) - shared
}

func splitPathParts(value string) []string {
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == "/" {
		return nil
	}
	return strings.Split(strings.Trim(cleaned, "/"), "/")
}

func sharesStrongTaskTerm(anchor Anchor, terms []Term) bool {
	for _, term := range terms {
		if term.Weight >= 8 && !genericGrepTerm(term.Normalized) &&
			anchorContainsExact(anchor, term.Normalized) {
			return true
		}
	}
	return false
}

func exactDocumentedCommand(anchor Anchor) string {
	for _, line := range anchor.Excerpt {
		trimmed := strings.TrimSpace(strings.Trim(line.Text, "`"))
		if strings.HasPrefix(trimmed, "go test ") || strings.HasPrefix(trimmed, "make ") {
			return "Documented repository command: " + trimmed
		}
	}
	return ""
}

func cheapExitAreaIDs(
	decisive Relation,
	coverage RoleCoverage,
	anchors []Anchor,
	relations []Relation,
	frontier VerificationFrontier,
) []string {
	if decisive.ID == "" {
		return nil
	}
	component := decisiveStrongComponent(decisive, anchors, relations)
	anchorByID := make(map[string]Anchor, len(anchors))
	for _, anchor := range anchors {
		anchorByID[anchor.ID] = anchor
	}
	areas := make(map[string]struct{})
	addArea := func(anchorID string) {
		anchor, exists := anchorByID[anchorID]
		if !exists {
			return
		}
		area := path.Dir(anchor.Path)
		if area == "." {
			area = "repository_root"
		}
		areas[area] = struct{}{}
	}
	addArea(decisive.LeftID)
	addArea(decisive.RightID)
	for _, role := range coverage.Key {
		roleAnchorIDs := make(map[string]struct{}, len(role.AnchorIDs))
		for _, anchorID := range role.AnchorIDs {
			roleAnchorIDs[anchorID] = struct{}{}
		}
		witnesses := 0
		for _, anchor := range anchors {
			if witnesses >= role.MinimumAnchors {
				break
			}
			if _, coversRole := roleAnchorIDs[anchor.ID]; !coversRole {
				continue
			}
			if _, connected := component[anchor.ID]; !connected || !completeRetainedScope(anchor.Scope) {
				continue
			}
			addArea(anchor.ID)
			witnesses++
		}
	}
	for _, item := range frontier.allItems() {
		if !isExactVerificationAuthority(item.Authority) {
			continue
		}
		if _, connected := component[item.AnchorID]; !connected || item.AnchorID == "" {
			continue
		}
		addArea(item.AnchorID)
		break
	}
	result := make([]string, 0, len(areas))
	for area := range areas {
		result = append(result, area)
	}
	sort.Strings(result)
	return result
}

func isExactVerificationAuthority(authority VerificationAuthority) bool {
	switch authority {
	case VerificationExactExistingTest,
		VerificationExactGeneratedFixture,
		VerificationExactExample,
		VerificationDocumentedCommand:
		return true
	default:
		return false
	}
}

func unresolvedCompetingHypotheses(
	coverage RoleCoverage,
	anchors []Anchor,
	relations []Relation,
	decisive Relation,
	frontier VerificationFrontier,
) int {
	component := decisiveStrongComponent(decisive, anchors, relations)
	if !keyRolesShareCompleteStrongComponent(coverage, anchors, decisive, component) ||
		!profileHasConcreteDecisiveEvidence(coverage.Profile, anchors, component) ||
		hasCompetingKeyRoleComponent(coverage, relations, component) ||
		!hasExactLinkedVerification(frontier, decisive, component) {
		return 1
	}
	return 0
}

func profileHasConcreteDecisiveEvidence(
	profile TaskProfile,
	anchors []Anchor,
	component map[string]struct{},
) bool {
	if profile != TaskProfileErrorNormalizationPrivacy {
		return true
	}
	publicTypes := make(map[string]struct{})
	nestedCarrier := false
	for _, anchor := range anchors {
		if _, connected := component[anchor.ID]; !connected ||
			!completeRetainedScope(anchor.Scope) {
			continue
		}
		if slices.Contains(anchor.RoleHints, RolePublicErrorType) {
			publicTypes[strings.ToLower(baseSymbol(anchor.Symbol))] = struct{}{}
			if anchorDefinesNestedErrorCarrier(anchor) {
				nestedCarrier = true
			}
		}
	}
	if len(publicTypes) == 0 || !nestedCarrier {
		return false
	}

	forms := errorRepresentationForms(0)
	materialized := false
	for _, anchor := range anchors {
		if _, connected := component[anchor.ID]; !connected ||
			!completeRetainedScope(anchor.Scope) ||
			!slices.Contains(anchor.RoleHints, RoleErrorNormalizer) ||
			!anchorMaterializesErrorRepresentation(anchor) {
			continue
		}
		materialized = true
		forms |= anchorErrorRepresentationForms(anchor, publicTypes)
	}
	return materialized && forms&errorRepresentationValue != 0 &&
		forms&errorRepresentationPointer != 0
}

type errorRepresentationForms uint8

const (
	errorRepresentationValue errorRepresentationForms = 1 << iota
	errorRepresentationPointer
)

func anchorDefinesNestedErrorCarrier(anchor Anchor) bool {
	parsed, err := parseGoAnchor(anchor)
	if err != nil {
		return false
	}
	found := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		if found {
			return false
		}
		structure, ok := node.(*ast.StructType)
		if !ok {
			return true
		}
		for _, field := range structure.Fields.List {
			if astExpressionContainsIdentifier(field.Type, "error") {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func anchorErrorRepresentationForms(
	anchor Anchor,
	publicTypes map[string]struct{},
) errorRepresentationForms {
	parsed, err := parseGoAnchor(anchor)
	if err != nil {
		return 0
	}
	declaredTypes := make(map[string]ast.Expr)
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.ValueSpec:
			if typed.Type == nil {
				return true
			}
			for _, name := range typed.Names {
				declaredTypes[name.Name] = typed.Type
			}
		case *ast.AssignStmt:
			if len(typed.Lhs) != len(typed.Rhs) {
				return true
			}
			for index, left := range typed.Lhs {
				identifier, ok := left.(*ast.Ident)
				if !ok {
					continue
				}
				literal, ok := typed.Rhs[index].(*ast.CompositeLit)
				if ok {
					declaredTypes[identifier.Name] = literal.Type
				}
			}
		}
		return true
	})

	forms := errorRepresentationForms(0)
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.CallExpr:
			if !isErrorsAsCall(typed) || len(typed.Args) < 2 {
				return true
			}
			address, ok := typed.Args[1].(*ast.UnaryExpr)
			if !ok {
				return true
			}
			switch target := address.X.(type) {
			case *ast.Ident:
				forms |= errorRepresentationFormForType(declaredTypes[target.Name], publicTypes)
			case *ast.CompositeLit:
				forms |= errorRepresentationFormForType(target.Type, publicTypes)
			}
		case *ast.TypeSwitchStmt:
			for _, statement := range typed.Body.List {
				clause, ok := statement.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, expression := range clause.List {
					forms |= errorRepresentationFormForType(expression, publicTypes)
				}
			}
		}
		return true
	})
	return forms
}

func errorRepresentationFormForType(
	typeExpression ast.Expr,
	publicTypes map[string]struct{},
) errorRepresentationForms {
	if typeExpression == nil {
		return 0
	}
	form := errorRepresentationValue
	if pointer, ok := typeExpression.(*ast.StarExpr); ok {
		form = errorRepresentationPointer
		typeExpression = pointer.X
	}
	typeName := strings.ToLower(astTypeBaseName(typeExpression))
	if _, relevant := publicTypes[typeName]; !relevant {
		return 0
	}
	return form
}

func astTypeBaseName(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return typed.Sel.Name
	case *ast.ParenExpr:
		return astTypeBaseName(typed.X)
	case *ast.IndexExpr:
		return astTypeBaseName(typed.X)
	case *ast.IndexListExpr:
		return astTypeBaseName(typed.X)
	default:
		return ""
	}
}

func astExpressionContainsIdentifier(expression ast.Expr, wanted string) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		found = ok && identifier.Name == wanted
		return !found
	})
	return found
}

func anchorHasErrorRepresentationExtraction(anchor Anchor) bool {
	parsed, err := parseGoAnchor(anchor)
	if err != nil {
		return false
	}
	found := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		if found {
			return false
		}
		switch typed := node.(type) {
		case *ast.CallExpr:
			found = isErrorsAsCall(typed)
		case *ast.TypeSwitchStmt:
			found = true
		}
		return !found
	})
	return found
}

func anchorMaterializesErrorRepresentation(anchor Anchor) bool {
	parsed, err := parseGoAnchor(anchor)
	if err != nil {
		return false
	}
	targets := make(map[string]struct{})
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.CallExpr:
			if !isErrorsAsCall(typed) || len(typed.Args) < 2 {
				return true
			}
			address, ok := typed.Args[1].(*ast.UnaryExpr)
			if !ok {
				return true
			}
			if identifier, ok := address.X.(*ast.Ident); ok {
				targets[identifier.Name] = struct{}{}
			}
		case *ast.TypeSwitchStmt:
			assignment, ok := typed.Assign.(*ast.AssignStmt)
			if !ok || len(assignment.Lhs) != 1 {
				return true
			}
			if identifier, ok := assignment.Lhs[0].(*ast.Ident); ok && identifier.Name != "_" {
				targets[identifier.Name] = struct{}{}
			}
		}
		return true
	})
	if len(targets) == 0 {
		return false
	}

	materialized := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		if materialized {
			return false
		}
		switch typed := node.(type) {
		case *ast.AssignStmt:
			for _, expression := range typed.Rhs {
				if call, ok := expression.(*ast.CallExpr); ok && isErrorsAsCall(call) {
					continue
				}
				if expressionUsesAnyIdentifier(expression, targets) {
					materialized = true
					return false
				}
			}
		case *ast.ReturnStmt:
			for _, expression := range typed.Results {
				if expressionUsesAnyIdentifier(expression, targets) {
					materialized = true
					return false
				}
			}
		}
		return true
	})
	return materialized
}

func isErrorsAsCall(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "As" {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	return ok && packageName.Name == "errors"
}

func expressionUsesAnyIdentifier(expression ast.Expr, names map[string]struct{}) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok {
			_, found = names[identifier.Name]
		}
		return !found
	})
	return found
}

func keyRolesShareCompleteStrongComponent(
	coverage RoleCoverage,
	anchors []Anchor,
	decisive Relation,
	component map[string]struct{},
) bool {
	if len(component) == 0 || len(coverage.MissingKeyRoles()) > 0 {
		return false
	}
	anchorByID := make(map[string]Anchor, len(anchors))
	for _, anchor := range anchors {
		anchorByID[anchor.ID] = anchor
	}
	for _, anchorID := range []string{decisive.LeftID, decisive.RightID} {
		anchor, exists := anchorByID[anchorID]
		if !exists || !completeRetainedScope(anchor.Scope) {
			return false
		}
	}
	for _, role := range coverage.Key {
		if !role.Represented || len(role.AnchorIDs) < role.MinimumAnchors {
			return false
		}
		connectedComplete := 0
		for _, anchorID := range role.AnchorIDs {
			if _, connected := component[anchorID]; !connected {
				continue
			}
			anchor, exists := anchorByID[anchorID]
			if exists && completeRetainedScope(anchor.Scope) {
				connectedComplete++
			}
		}
		if connectedComplete < role.MinimumAnchors {
			return false
		}
	}
	return true
}

func hasCompetingKeyRoleComponent(
	coverage RoleCoverage,
	relations []Relation,
	decisiveComponent map[string]struct{},
) bool {
	adjacent := make(map[string][]string)
	for _, relation := range relations {
		if !isStrongExactLocalRelation(relation) || !isDecisiveRelationKind(RelationKind(relation.Kind)) {
			continue
		}
		if _, connected := decisiveComponent[relation.LeftID]; connected {
			continue
		}
		if _, connected := decisiveComponent[relation.RightID]; connected {
			continue
		}
		adjacent[relation.LeftID] = append(adjacent[relation.LeftID], relation.RightID)
		adjacent[relation.RightID] = append(adjacent[relation.RightID], relation.LeftID)
	}
	seen := make(map[string]struct{})
	for start := range adjacent {
		if _, visited := seen[start]; visited {
			continue
		}
		component := make(map[string]struct{})
		queue := []string{start}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			if _, visited := seen[current]; visited {
				continue
			}
			seen[current] = struct{}{}
			component[current] = struct{}{}
			queue = append(queue, adjacent[current]...)
		}
		coversAll := true
		for _, role := range coverage.Key {
			witnesses := 0
			for _, anchorID := range role.AnchorIDs {
				if _, present := component[anchorID]; present {
					witnesses++
				}
			}
			if witnesses < role.MinimumAnchors {
				coversAll = false
				break
			}
		}
		if coversAll {
			return true
		}
	}
	return false
}

func isDecisiveRelationKind(kind RelationKind) bool {
	switch kind {
	case RelationConfigApplied,
		RelationErrorMapped,
		RelationErrorCreated,
		RelationValueTransformed,
		RelationFieldCopy,
		RelationDirectCall,
		RelationScriptInvokes,
		RelationErrorExposed,
		RelationFieldRead,
		RelationFieldWrite:
		return true
	default:
		return false
	}
}

func completeRetainedScope(scope SourceScope) bool {
	return scope.Validate() == nil && scope.isComplete()
}

func hasExactLinkedVerification(
	frontier VerificationFrontier,
	decisive Relation,
	component map[string]struct{},
) bool {
	if !frontier.HasExactAnchorOrEffect() ||
		(frontier.DecisiveAnchorID != decisive.LeftID && frontier.DecisiveAnchorID != decisive.RightID) {
		return false
	}
	for _, item := range frontier.allItems() {
		switch item.Authority {
		case VerificationExactExistingTest,
			VerificationExactGeneratedFixture,
			VerificationExactExample,
			VerificationDocumentedCommand:
			if _, linked := component[item.AnchorID]; linked && item.AnchorID != "" {
				return true
			}
		}
	}
	return false
}

func localVerificationEffectForBundle(bundle Bundle) string {
	if bundle.Verification.CommandOrEffect != nil {
		return truncateUTF8(bundle.Verification.CommandOrEffect.Text, 1024)
	}
	profileEffect := map[TaskProfile]string{
		TaskProfileNilPanic:                  "The retained nil-capable input path completes without the locally observed unsafe operation panicking.",
		TaskProfileConfigurationPropagation:  "The configured value is present at the retained effective destination after the copy or apply site.",
		TaskProfileErrorStatusMapping:        "The retained typed error path maps to its client-facing status instead of the generic fallback status.",
		TaskProfileDataTagTransformation:     "The retained generated output or fixture records the typed source value produced by the transformation path.",
		TaskProfileErrorNormalizationPrivacy: "Value and pointer error inputs produce the same retained public status and serialization shape without developer-only detail.",
		TaskProfileOperationalRelease:        "Every retained module target is checked for conflicts before any tag or push mutation is reached.",
	}
	if effect := profileEffect[bundle.Profile]; effect != "" {
		return effect
	}
	return "Observe the bounded repository-owned effect at the retained verification anchor."
}

func validateBundleV01Contract(bundle Bundle) error {
	profile := DeriveTaskProfile(bundle.Task.Text, bundle.KindHint)
	if bundle.Profile != profile {
		return fmt.Errorf("task lens: task profile does not match deterministic derivation")
	}
	contract, err := DefaultRoleContract(profile)
	if err != nil || bundle.RoleContract.Profile != profile ||
		!slices.Equal(bundle.RoleContract.Key, contract.Key) ||
		!slices.Equal(bundle.RoleContract.Supporting, contract.Supporting) ||
		!slices.Equal(bundle.RoleContract.Optional, contract.Optional) {
		return fmt.Errorf("task lens: role contract does not match task profile")
	}
	coverage, err := EvaluateRoleCoverage(contract, bundle.Anchors)
	if err != nil || !equalRoleCoverage(coverage, bundle.RoleCoverage) {
		return fmt.Errorf("task lens: role coverage does not match retained anchors")
	}
	if err := bundle.Verification.Validate(); err != nil {
		return err
	}
	anchorIDs := make(map[string]struct{}, len(bundle.Anchors))
	for _, anchor := range bundle.Anchors {
		anchorIDs[anchor.ID] = struct{}{}
	}
	if err := validateFrontierAnchorIDs(bundle.Verification, anchorIDs); err != nil {
		return err
	}
	decisive, decisiveFound := selectDecisiveRelation(
		bundle.Relations,
		bundle.Anchors,
		bundle.Terms,
		bundle.Profile,
	)
	expectedDecisiveID := ""
	if decisiveFound {
		expectedDecisiveID = decisive.ID
	}
	if bundle.DecisiveRelationID != expectedDecisiveID {
		return fmt.Errorf("task lens: decisive relation does not match deterministic ranking")
	}
	expectedFrontier := buildVerificationFrontier(bundle.Anchors, bundle.Relations, decisive, bundle.Terms)
	if !reflect.DeepEqual(expectedFrontier, bundle.Verification) {
		return fmt.Errorf("task lens: verification frontier does not match retained evidence")
	}
	expected := EvaluateCheapExit(CheapExitInput{
		AreaIDs: cheapExitAreaIDs(
			decisive,
			coverage,
			bundle.Anchors,
			bundle.Relations,
			bundle.Verification,
		),
		MissingKeyRoles:         coverage.MissingKeyRoles(),
		DecisiveRelationKind:    RelationKind(decisive.Kind),
		DecisiveRelationSupport: decisive.SupportType,
		Verification:            bundle.Verification,
		UnresolvedCompetingHypotheses: unresolvedCompetingHypotheses(
			coverage,
			bundle.Anchors,
			bundle.Relations,
			decisive,
			bundle.Verification,
		),
	})
	if !equalCheapExitDecision(expected, bundle.CheapExit) {
		return fmt.Errorf("task lens: cheap-exit decision does not match deterministic gates")
	}
	return nil
}

// GroundV01Contract fills the immutable pre-synthesis contract projection for
// a bundle whose anchors, evidence, grounded terms, and relations are already
// fixed. It is useful to deterministic fixture builders and replay tools.
func GroundV01Contract(bundle *Bundle) error {
	if bundle == nil {
		return fmt.Errorf("task lens: bundle is required")
	}
	bundle.Profile = DeriveTaskProfile(bundle.Task.Text, bundle.KindHint)
	contract, err := DefaultRoleContract(bundle.Profile)
	if err != nil {
		return err
	}
	bundle.RoleContract = contract
	coverage, err := EvaluateRoleCoverage(contract, bundle.Anchors)
	if err != nil {
		return err
	}
	bundle.RoleCoverage = coverage
	decisive, found := selectDecisiveRelation(
		bundle.Relations,
		bundle.Anchors,
		bundle.Terms,
		bundle.Profile,
	)
	if found {
		bundle.DecisiveRelationID = decisive.ID
	} else {
		bundle.DecisiveRelationID = ""
	}
	bundle.Verification = buildVerificationFrontier(bundle.Anchors, bundle.Relations, decisive, bundle.Terms)
	bundle.CheapExit = EvaluateCheapExit(CheapExitInput{
		AreaIDs: cheapExitAreaIDs(
			decisive,
			coverage,
			bundle.Anchors,
			bundle.Relations,
			bundle.Verification,
		),
		MissingKeyRoles:         coverage.MissingKeyRoles(),
		DecisiveRelationKind:    RelationKind(decisive.Kind),
		DecisiveRelationSupport: decisive.SupportType,
		Verification:            bundle.Verification,
		UnresolvedCompetingHypotheses: unresolvedCompetingHypotheses(
			coverage,
			bundle.Anchors,
			bundle.Relations,
			decisive,
			bundle.Verification,
		),
	})
	return nil
}

func equalRoleCoverage(left, right RoleCoverage) bool {
	return left.Profile == right.Profile && slices.EqualFunc(left.Key, right.Key, equalRoleCoverageItem) &&
		slices.EqualFunc(left.Supporting, right.Supporting, equalRoleCoverageItem) &&
		slices.EqualFunc(left.Optional, right.Optional, equalRoleCoverageItem)
}

func equalRoleCoverageItem(left, right RoleCoverageItem) bool {
	return left.Role == right.Role && left.MinimumAnchors == right.MinimumAnchors &&
		left.Represented == right.Represented && slices.Equal(left.AnchorIDs, right.AnchorIDs)
}

func equalCheapExitDecision(left, right CheapExitDecision) bool {
	return left.Eligible == right.Eligible && left.Route == right.Route &&
		slices.Equal(left.Reasons, right.Reasons) && slices.EqualFunc(left.Gates, right.Gates, func(a, b CheapExitGateResult) bool {
		return a == b
	})
}

func buildRetrievalTrace(bundle Bundle, candidates []anchorCandidate) RetrievalTrace {
	selected := make(map[string]int, len(bundle.Anchors))
	selectedPerPath := make(map[string]int)
	for index, anchor := range bundle.Anchors {
		selected[anchor.ID] = index + 1
		selectedPerPath[anchor.Path]++
	}
	trace := RetrievalTrace{
		Version: RetrievalTraceVersion, TaskKind: bundle.KindHint, TaskProfile: bundle.Profile,
		TaskTerms: []RetrievalTaskTerm{}, CandidatesBeforeRanking: []RetrievalCandidate{},
		Relationships: []RetrievalRelationship{}, SelectedAnchors: []RetrievalSelection{},
		DroppedAnchors: []RetrievalDrop{}, SourceScopes: []RetrievalSourceScope{},
		RoleCoverage: bundle.RoleCoverage, VerificationFrontier: bundle.Verification,
		Budgets: bundle.Budgets,
	}
	for _, term := range bundle.Terms {
		trace.TaskTerms = append(trace.TaskTerms, RetrievalTaskTerm{
			Text: term.Text, Normalized: term.Normalized, Found: term.Found, Weight: term.Weight,
		})
	}
	for index, candidate := range candidates {
		components := traceScoreComponents(candidate, bundle.RoleContract)
		stage := candidate.stage
		if stage == "" {
			stage = RetrievalStageInitial
		}
		trace.CandidatesBeforeRanking = append(trace.CandidatesBeforeRanking, RetrievalCandidate{
			ID: candidate.anchor.ID, Stage: stage, DiscoveryOrder: index + 1,
			Path: candidate.anchor.Path, Symbol: candidate.anchor.Symbol,
			Roles: append([]AnchorRole(nil), candidate.anchor.RoleHints...),
			Score: candidate.score, ScoreComponents: components,
		})
		if rank, ok := selected[candidate.anchor.ID]; ok {
			trace.SelectedAnchors = append(trace.SelectedAnchors, RetrievalSelection{
				CandidateID: candidate.anchor.ID, AnchorID: candidate.anchor.ID, Rank: rank,
				Reason: traceSelectionReason(candidate.anchor, bundle.RoleContract, stage),
			})
		} else {
			trace.DroppedAnchors = append(trace.DroppedAnchors, RetrievalDrop{
				CandidateID: candidate.anchor.ID,
				Reason: traceDropReason(
					candidate,
					bundle,
					selectedPerPath,
				),
			})
		}
	}
	// Selected anchor order is authoritative even when a duplicate candidate
	// was collapsed before trace projection.
	sort.Slice(trace.SelectedAnchors, func(i, j int) bool {
		return trace.SelectedAnchors[i].Rank < trace.SelectedAnchors[j].Rank
	})
	for _, anchor := range bundle.Anchors {
		trace.SourceScopes = append(trace.SourceScopes, RetrievalSourceScope{
			AnchorID: anchor.ID, Scope: anchor.Scope,
		})
	}
	for _, relation := range bundle.Relations {
		trace.Relationships = append(trace.Relationships, RetrievalRelationship{
			ID: relation.ID, LeftID: relation.LeftID, RightID: relation.RightID,
			Kind: RelationKind(relation.Kind), SupportType: relation.SupportType,
			EvidenceIDs: append([]string(nil), relation.EvidenceIDs...),
			Scope:       "Exact retained anchor scopes only.", NonGuarantees: relation.Scope,
		})
	}
	trace.Limits = []RetrievalLimit{
		traceLimit("initial_candidates", MaxInitialCandidates, bundle.Budgets.CandidateItemsFound, bundle.Budgets.CandidateLimitBound,
			"candidate paths beyond the bounded initial set were not generated as anchors"),
		traceLimit("retained_anchors", MaxRetainedAnchors, bundle.Budgets.AnchorItemsFound, bundle.Budgets.AnchorLimitBound,
			"anchor candidates beyond the role-aware retention bound were dropped"),
		traceLimit("read_files", MaxReadFiles, bundle.Budgets.ReadFiles, bundle.Budgets.FileLimitBound,
			"candidate files remained unread after the repository file budget"),
		traceLimit("read_bytes", MaxReadBytes, bundle.Budgets.ReadBytes, bundle.Budgets.ByteLimitBound,
			"candidate source remained outside the repository byte budget"),
		traceLimit("source_scan_bytes", MaxSourceScanBytes, bundle.Budgets.SourceScanBytes, bundle.Budgets.SourceScanLimitBound,
			"a selected file extended beyond the bounded complete-source scan"),
		traceLimit("retained_source_bytes", MaxRetainedSourceBytes, bundle.Budgets.RetainedSourceBytes, bundle.Budgets.RetainedByteLimitBound,
			"complete candidate anchors exceeded the retained source byte budget"),
		traceLimit("completion_expansions", MaxFrontierExpansions, bundle.Budgets.FrontierExpansions,
			completionLimitCausedRoleLoss(bundle, candidates),
			"a generated candidate for a still-missing key role remained after two bounded completion expansions"),
	}
	return trace
}

func completionLimitCausedRoleLoss(bundle Bundle, candidates []anchorCandidate) bool {
	if bundle.Budgets.FrontierExpansions < MaxFrontierExpansions {
		return false
	}
	missing := bundle.RoleCoverage.MissingKeyRoles()
	if len(missing) == 0 {
		return false
	}
	selected := make(map[string]struct{}, len(bundle.Anchors))
	for _, anchor := range bundle.Anchors {
		selected[anchor.ID] = struct{}{}
	}
	for _, candidate := range candidates {
		if _, retained := selected[candidate.anchor.ID]; retained {
			continue
		}
		for _, role := range missing {
			if slices.Contains(candidate.anchor.RoleHints, role) {
				return true
			}
		}
	}
	return false
}

// GroundedSelectedRetrievalTrace builds a minimal replay trace for fixtures or
// imported runs that possess the canonical selected bundle but no trustworthy
// pre-ranking log. It never fabricates dropped candidates.
func GroundedSelectedRetrievalTrace(bundle Bundle) (RetrievalTrace, error) {
	candidates := make([]anchorCandidate, 0, len(bundle.Anchors))
	for _, anchor := range bundle.Anchors {
		var hits []string
		for _, term := range bundle.Terms {
			if anchorContainsExact(anchor, term.Normalized) {
				hits = append(hits, term.ID)
			}
		}
		candidates = append(candidates, anchorCandidate{anchor: anchor, score: anchor.Score, terms: hits})
	}
	trace := buildRetrievalTrace(bundle, candidates)
	if err := trace.Validate(); err != nil {
		return RetrievalTrace{}, err
	}
	return trace, nil
}

func traceScoreComponents(candidate anchorCandidate, contract RoleContract) []RetrievalScoreComponent {
	direct := 0
	for range candidate.terms {
		direct += 10
	}
	missingRole := 0
	for _, requirement := range contract.Key {
		if slices.Contains(candidate.anchor.RoleHints, requirement.Role) {
			missingRole += 40
		}
	}
	production := 20
	testFixture := 0
	examplePenalty := 0
	if isTestPath(candidate.anchor.Path) {
		production = 0
		testFixture = 20
		if !slices.Contains(candidate.anchor.RoleHints, RoleVerificationAnchor) {
			examplePenalty = -10
		}
	}
	scope := 0
	if !candidate.anchor.Scope.Truncated {
		scope = 20
	}
	known := direct + missingRole + production + testFixture + scope + examplePenalty
	repositoryRole := candidate.score - known
	return []RetrievalScoreComponent{
		{Kind: RetrievalScoreDirectTaskTerm, Value: direct, Detail: "sum of exact retained task-term hits"},
		{Kind: RetrievalScoreMissingRole, Value: missingRole, Detail: "fit to immutable key-role reservations"},
		{Kind: RetrievalScoreExactRelation, Value: 0, Detail: "relations are computed after retained anchor identity is fixed"},
		{Kind: RetrievalScoreProductionRelevance, Value: production},
		{Kind: RetrievalScoreTestFixtureRelevance, Value: testFixture},
		{Kind: RetrievalScoreScopeCompleteness, Value: scope},
		{Kind: RetrievalScoreRepositoryRole, Value: repositoryRole, Detail: "residual repository/path score; components sum to the exact pre-ranking score"},
		{Kind: RetrievalScoreDistance, Value: 0, Detail: "no repository-wide graph distance was computed"},
		{Kind: RetrievalScoreAdjacentPenalty, Value: 0},
		{Kind: RetrievalScoreExampleOnlyPenalty, Value: examplePenalty},
	}
}

func traceSelectionReason(
	anchor Anchor,
	contract RoleContract,
	stage RetrievalStage,
) string {
	switch stage {
	case RetrievalStageCompletion1:
		return "selected by bounded completion expansion 1 for a missing key role or exact decisive-component relation"
	case RetrievalStageCompletion2:
		return "selected by bounded completion expansion 2 for a missing key role or exact decisive-component relation"
	case RetrievalStageVerification:
		return "selected by the single bounded verification probe through an exact decisive-component link"
	}
	for _, requirement := range contract.Key {
		if slices.Contains(anchor.RoleHints, requirement.Role) {
			return "reserved before supporting anchors for key role " + string(requirement.Role)
		}
	}
	for _, requirement := range contract.Supporting {
		if slices.Contains(anchor.RoleHints, requirement.Role) {
			return "reserved after key roles for supporting role " + string(requirement.Role)
		}
	}
	return "retained by bounded exact-term score and file diversity"
}

func traceDropReason(
	candidate anchorCandidate,
	bundle Bundle,
	selectedPerPath map[string]int,
) string {
	if candidate.stage == RetrievalStageCompletion1 || candidate.stage == RetrievalStageCompletion2 ||
		candidate.stage == RetrievalStageVerification {
		if bundle.Budgets.RetainedByteLimitBound {
			return "selected by a bounded expansion, then removed by the retained-source byte bound"
		}
		return "selected by a bounded expansion, then replaced by a later higher-authority bounded selection"
	}
	if selectedPerPath[candidate.anchor.Path] >= maxAnchorsPerFile {
		return "dropped because the per-file anchor limit was filled by higher role/score candidates"
	}
	if len(bundle.Anchors) >= MaxRetainedAnchors {
		return "dropped because the retained-anchor limit was filled after key/supporting role reservation and score ordering"
	}
	if len(candidate.terms) == 0 {
		return "dropped because it had no exact retained task-term hit and filled no unrepresented reserved role"
	}
	return "dropped because higher-scoring candidates already covered its task terms and role hints"
}

func traceLimit(name string, limit, observed int, causedLoss bool, reason string) RetrievalLimit {
	item := RetrievalLimit{Name: name, Limit: limit, Observed: observed}
	item.Applied = observed >= limit || causedLoss
	item.CausedLoss = causedLoss
	if causedLoss {
		item.LossReason = reason
	}
	return item
}
