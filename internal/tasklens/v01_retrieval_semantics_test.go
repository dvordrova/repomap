package tasklens

import "testing"

func TestDeriveTaskProfileUsesWholeWordsAndSpecificPhrases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		task string
		kind TaskKind
		want TaskProfile
	}{
		{
			name: "schema vocabulary wins over incidental panic",
			task: "The generated schema tag is nullable and the generator panics.",
			kind: TaskBug,
			want: TaskProfileDataTagTransformation,
		},
		{
			name: "nullable is not null",
			task: "Preserve nullable fields in the generated output.",
			kind: TaskBug,
			want: TaskProfileDataTagTransformation,
		},
		{
			name: "standalone nil and panic",
			task: "A nil body makes validation panic.",
			kind: TaskBug,
			want: TaskProfileNilPanic,
		},
		{
			name: "substrings do not classify",
			task: "The terror label is annulled.",
			kind: TaskBug,
			want: TaskProfileUnknown,
		},
		{
			name: "hyphenated status phrase",
			task: "Return not-acceptable for unsupported media types.",
			kind: TaskBug,
			want: TaskProfileErrorStatusMapping,
		},
		{
			name: "error normalization phrase",
			task: "Normalize an error value versus a pointer before public serialization.",
			kind: TaskBug,
			want: TaskProfileErrorNormalizationPrivacy,
		},
		{
			name: "explicit configuration kind",
			task: "A generated schema is nullable and can panic.",
			kind: TaskConfiguration,
			want: TaskProfileConfigurationPropagation,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := DeriveTaskProfile(test.task, test.kind); got != test.want {
				t.Fatalf("DeriveTaskProfile(%q, %q) = %q, want %q", test.task, test.kind, got, test.want)
			}
		})
	}
}

func TestVerificationFrontierRequiresStrongLinkToDecisiveRelation(t *testing.T) {
	t.Parallel()

	entry := semanticAnchor("entry", "pkg/entry.go", "Entry")
	decisiveAnchor := semanticAnchor("decisive", "pkg/decisive.go", "Decisive")
	exactTest := semanticAnchor("exact-test", "pkg/decisive_test.go", "TestDecisive")
	unrelatedTest := semanticAnchor("unrelated-test", "pkg/unrelated_test.go", "TestTag")
	fixture := semanticAnchor("fixture", "pkg/testdata/golden.json", "golden")
	document := semanticAnchorWithText(
		"document",
		"README.md",
		"verification",
		"go test ./pkg # Entry",
	)
	decisive := semanticRelation(RelationDirectCall, entry, decisiveAnchor)
	relations := []Relation{
		decisive,
		semanticRelation(RelationDirectCall, exactTest, decisiveAnchor),
		semanticRelation(RelationSharedStateAlias, entry, unrelatedTest),
	}

	frontier := buildVerificationFrontier(
		[]Anchor{entry, decisiveAnchor, exactTest, unrelatedTest, fixture, document},
		relations,
		decisive,
		[]Term{{Normalized: "tag", Weight: 10}},
	)
	if err := frontier.Validate(); err != nil {
		t.Fatalf("frontier Validate() error = %v", err)
	}
	if len(frontier.Anchors) != 1 || frontier.Anchors[0].AnchorID != exactTest.ID ||
		frontier.Anchors[0].Authority != VerificationExactExistingTest {
		t.Fatalf("exact verification anchors = %#v", frontier.Anchors)
	}
	if frontier.Fixture != nil {
		t.Fatalf("unlinked fixture received exact authority: %#v", frontier.Fixture)
	}
	if frontier.CommandOrEffect != nil {
		t.Fatalf("unlinked command received exact authority: %#v", frontier.CommandOrEffect)
	}
}

func TestVerificationFrontierUsesProposedAuthorityForUnlinkedSibling(t *testing.T) {
	t.Parallel()

	entry := semanticAnchor("entry", "pkg/entry.go", "Entry")
	decisiveAnchor := semanticAnchor("decisive", "pkg/decisive.go", "Decisive")
	sibling := semanticAnchorWithText(
		"sibling-test",
		"pkg/decisive_test.go",
		"TestSibling",
		"func TestSibling() { checkDecisiveTag() }",
	)
	decisive := semanticRelation(RelationDirectCall, entry, decisiveAnchor)
	relations := []Relation{
		decisive,
		semanticRelation(RelationSharedStateAlias, decisiveAnchor, sibling),
	}

	frontier := buildVerificationFrontier(
		[]Anchor{entry, decisiveAnchor, sibling},
		relations,
		decisive,
		[]Term{{Normalized: "decisive", Weight: 10}},
	)
	if err := frontier.Validate(); err != nil {
		t.Fatalf("frontier Validate() error = %v", err)
	}
	if len(frontier.Anchors) != 1 || frontier.Anchors[0].AnchorID != sibling.ID ||
		frontier.Anchors[0].Authority != VerificationProposedTestLocation {
		t.Fatalf("fallback verification anchors = %#v", frontier.Anchors)
	}
	if frontier.HasExactAnchorOrEffect() {
		t.Fatal("unlinked sibling test was treated as exact verification")
	}
}

func TestVerificationFrontierAcceptsLinkedFixtureAndCommand(t *testing.T) {
	t.Parallel()

	entry := semanticAnchor("entry", "pkg/entry.go", "Entry")
	decisiveAnchor := semanticAnchor("decisive", "pkg/decisive.go", "Decisive")
	fixture := semanticAnchor("fixture", "pkg/testdata/golden.json", "golden")
	document := semanticAnchorWithText(
		"document",
		"README.md",
		"verification",
		"go test ./pkg # Entry",
	)
	decisive := semanticRelation(RelationValueTransformed, entry, decisiveAnchor)
	relations := []Relation{
		decisive,
		semanticRelation(RelationFixtureRecords, decisiveAnchor, fixture),
		semanticRelation(RelationDocumentedUses, entry, document),
	}

	frontier := buildVerificationFrontier(
		[]Anchor{entry, decisiveAnchor, fixture, document},
		relations,
		decisive,
		nil,
	)
	if err := frontier.Validate(); err != nil {
		t.Fatalf("frontier Validate() error = %v", err)
	}
	if frontier.Fixture == nil || frontier.Fixture.AnchorID != fixture.ID {
		t.Fatalf("linked fixture = %#v", frontier.Fixture)
	}
	if frontier.CommandOrEffect == nil || frontier.CommandOrEffect.AnchorID != document.ID {
		t.Fatalf("linked command = %#v", frontier.CommandOrEffect)
	}
	if !frontier.HasExactAnchorOrEffect() {
		t.Fatal("linked fixture and command did not provide exact verification")
	}
}

func TestVerificationFrontierRanksExactTestsNearDecisiveEndpoints(t *testing.T) {
	t.Parallel()

	send := semanticAnchor("send", "http/serialization.go", "Send", RoleSymptomSite)
	sendError := semanticAnchor("send-error", "http/serialization.go", "SendError", RoleErrorMapping)
	sqliteTest := semanticAnchor("sqlite-test", "storage/sqlite/sqlite_test.go", "TestSQLiteError")
	otherTest := semanticAnchor("other-test", "http/serialization_test.go", "TestFallback")
	testSend := semanticAnchor("test-send", "http/serialization_test.go", "TestSend")
	testSendError := semanticAnchor("test-send-error", "http/serialization_test.go", "TestSendError")
	farFixture := semanticAnchor("far-fixture", "storage/sqlite/testdata/error.golden.json", "error-golden")
	nearFixture := semanticAnchor("near-fixture", "http/testdata/send-error.golden.json", "send-error-golden")
	sqliteTest.Score = 10_000
	otherTest.Score = 9_000
	farFixture.Score = 10_000

	decisive := semanticRelation(RelationErrorMapped, send, sendError)
	relations := []Relation{
		decisive,
		semanticRelation(RelationTestExercises, sendError, sqliteTest),
		semanticRelation(RelationTestExercises, sendError, otherTest),
		semanticRelation(RelationDirectCall, testSend, send),
		semanticRelation(RelationDirectCall, testSendError, sendError),
		semanticRelation(RelationFixtureRecords, sendError, farFixture),
		semanticRelation(RelationFixtureRecords, sendError, nearFixture),
	}
	anchors := []Anchor{
		farFixture,
		sqliteTest,
		otherTest,
		send,
		nearFixture,
		testSend,
		sendError,
		testSendError,
	}
	terms := []Term{{Normalized: "send", Weight: 12}}

	assertFrontier := func(t *testing.T, frontier VerificationFrontier) {
		t.Helper()
		if err := frontier.Validate(); err != nil {
			t.Fatalf("frontier Validate() error = %v", err)
		}
		if len(frontier.Anchors) != MaxVerificationAnchors {
			t.Fatalf("verification anchors = %#v", frontier.Anchors)
		}
		want := []string{testSendError.ID, testSend.ID}
		for index, anchorID := range want {
			if frontier.Anchors[index].AnchorID != anchorID {
				t.Fatalf("verification anchors = %#v, want IDs %v", frontier.Anchors, want)
			}
		}
		if frontier.Fixture == nil || frontier.Fixture.AnchorID != nearFixture.ID {
			t.Fatalf("verification fixture = %#v, want %q", frontier.Fixture, nearFixture.ID)
		}
	}

	assertFrontier(t, buildVerificationFrontier(anchors, relations, decisive, terms))
	for left, right := 0, len(anchors)-1; left < right; left, right = left+1, right-1 {
		anchors[left], anchors[right] = anchors[right], anchors[left]
	}
	for left, right := 0, len(relations)-1; left < right; left, right = left+1, right-1 {
		relations[left], relations[right] = relations[right], relations[left]
	}
	assertFrontier(t, buildVerificationFrontier(anchors, relations, decisive, terms))
}

func TestSelectDecisiveRelationPrefersErrorHandoffOverConstructorMention(t *testing.T) {
	t.Parallel()

	constructor := semanticAnchor(
		"constructor",
		"http/server.go",
		"NewServer",
		RoleSymptomSite,
		RoleErrorMapping,
	)
	handoff := semanticAnchor(
		"handoff",
		"http/serialization.go",
		"SendError",
		RoleErrorMapping,
		RoleIntegrationBoundary,
	)
	status := semanticAnchor(
		"status",
		"http/errors.go",
		"NotAcceptableError.StatusCode",
		RoleErrorCreation,
	)
	constructor.Score = 500
	handoff.Score = 50
	status.Score = 200
	constructorMention := semanticRelation(RelationErrorMapped, constructor, status)
	handoffMapping := semanticRelation(RelationErrorMapped, handoff, status)
	terms := []Term{{Normalized: "server", Weight: 16}}

	for _, relations := range [][]Relation{
		{constructorMention, handoffMapping},
		{handoffMapping, constructorMention},
	} {
		got, ok := selectDecisiveRelation(
			relations,
			[]Anchor{constructor, handoff, status},
			terms,
			TaskProfileErrorStatusMapping,
		)
		if !ok {
			t.Fatal("selectDecisiveRelation() found no decisive relation")
		}
		if got.ID != handoffMapping.ID {
			t.Fatalf("decisive relation = %#v, want handoff mapping %#v", got, handoffMapping)
		}
	}
}

func TestErrorNormalizationCheapExitRequiresMaterializedRepresentation(t *testing.T) {
	t.Parallel()

	publicType := semanticAnchorWithText(
		"public-type",
		"pkg/problem.go",
		"Problem",
		"type Problem struct { Cause error }",
		RolePublicErrorType,
	)
	exposure := semanticAnchorWithText(
		"exposure",
		"pkg/problem.go",
		"Problem.Error",
		"func (p Problem) Error() string { return p.Cause.Error() }",
		RolePublicErrorExposure,
	)
	exactTest := semanticAnchorWithText(
		"normalization-test",
		"pkg/problem_test.go",
		"TestNormalizeProblem",
		"func TestNormalizeProblem(t *testing.T) { _ = NormalizeProblem(errors.New(\"boom\")) }",
		RoleVerificationAnchor,
	)

	ambiguity := func(normalizer Anchor) int {
		decisive := semanticRelation(RelationErrorMapped, normalizer, exposure)
		anchors := []Anchor{publicType, normalizer, exposure, exactTest}
		relations := []Relation{
			decisive,
			semanticRelation(RelationErrorExposed, exposure, publicType),
			semanticRelation(RelationTestExercises, normalizer, exactTest),
		}
		coverage := RoleCoverage{
			Profile: TaskProfileErrorNormalizationPrivacy,
			Key: []RoleCoverageItem{
				semanticCoverage(RolePublicErrorType, publicType),
				semanticCoverage(RoleErrorNormalizer, normalizer),
				semanticCoverage(RolePublicErrorExposure, exposure),
			},
		}
		frontier := buildVerificationFrontier(anchors, relations, decisive, nil)
		return unresolvedCompetingHypotheses(coverage, anchors, relations, decisive, frontier)
	}

	forwarder := semanticAnchorWithText(
		"normalizer",
		"pkg/problem.go",
		"DispatchProblem",
		`func DispatchProblem(err error) error {
			var target interface{ Error() string }
			if errors.As(err, &target) { return NormalizeProblem(err) }
			return err
		}`,
		RoleErrorNormalizer,
	)
	if got := ambiguity(forwarder); got == 0 {
		t.Fatal("forwarding dispatcher without retained normalization flow passed cheap-exit sufficiency")
	}

	materializingNormalizer := semanticAnchorWithText(
		"value-normalizer",
		"pkg/problem.go",
		"NormalizeProblem",
		`func NormalizeProblem(err error) error {
			normalized := Problem{Cause: err}
			var target Problem
			if errors.As(err, &target) { normalized = target }
			return normalized
		}`,
		RoleErrorNormalizer,
	)
	if got := ambiguity(materializingNormalizer); got == 0 {
		t.Fatal("one-sided representation normalization passed value/pointer sufficiency")
	}

	completeNormalizer := semanticAnchorWithText(
		"normalizer",
		"pkg/problem.go",
		"NormalizeProblem",
		`func NormalizeProblem(err error) error {
			normalized := Problem{Cause: err}
			var value Problem
			if errors.As(err, &value) { normalized = value }
			var pointer *Problem
			if errors.As(err, &pointer) { normalized = *pointer }
			return normalized
		}`,
		RoleErrorNormalizer,
	)
	if got := ambiguity(completeNormalizer); got != 0 {
		t.Fatalf("complete local representation normalization ambiguity = %d, want 0", got)
	}
}

func TestCompletionRetainsConcreteErrorNormalizerAndLinkedVerification(t *testing.T) {
	t.Parallel()

	contract, err := DefaultRoleContract(TaskProfileErrorNormalizationPrivacy)
	if err != nil {
		t.Fatal(err)
	}
	publicType := semanticAnchorWithText(
		"completion-public-type",
		"pkg/problem.go",
		"Problem",
		"type Problem struct { Cause error }",
		RolePublicErrorType,
	)
	dispatcher := semanticAnchorWithText(
		"completion-dispatcher",
		"pkg/problem.go",
		"DispatchProblem",
		`func DispatchProblem(err error) error {
			var target Problem
			if errors.As(err, &target) { return NormalizeProblem(err) }
			return err
		}`,
		RoleErrorNormalizer,
	)
	exposure := semanticAnchorWithText(
		"completion-exposure",
		"pkg/problem.go",
		"Problem.Error",
		"func (p Problem) Error() string { return p.Cause.Error() }",
		RolePublicErrorExposure,
	)
	for _, anchor := range []*Anchor{&publicType, &dispatcher, &exposure} {
		anchor.Package = "fixture"
	}
	anchors := []Anchor{publicType, dispatcher, exposure}
	for index := len(anchors); index < MaxRetainedAnchors; index++ {
		suffix := string(rune('a' + index))
		filler := semanticAnchor(
			"filler-"+suffix,
			"pkg/fillers/filler"+suffix+".go",
			"Filler"+string(rune('A'+index)),
			RoleRepresentativeImplementation,
		)
		anchors = append(anchors, filler)
	}

	normalizer := semanticAnchorWithText(
		"completion-normalizer",
		"pkg/problem.go",
		"NormalizeProblem",
		`func NormalizeProblem(err error) error {
			normalized := Problem{Cause: err}
			var target Problem
			if errors.As(err, &target) { normalized = target }
			return normalized
		}`,
		RoleErrorNormalizer,
		RolePublicErrorExposure,
	)
	normalizer.Package = "fixture"
	exactTest := semanticAnchorWithText(
		"completion-test",
		"pkg/problem_test.go",
		"TestNormalizeProblem",
		"func TestNormalizeProblem(t *testing.T) { _ = NormalizeProblem(errors.New(\"boom\")) }",
		RoleVerificationAnchor,
	)
	exactTest.Package = "fixture"
	candidates := []anchorCandidate{
		{anchor: normalizer, score: 100},
		{anchor: exactTest, score: 100},
	}

	completed, expansions := completeMissingRoleAnchors(anchors, candidates, nil, contract)
	if expansions == 0 || expansions > MaxFrontierExpansions || !containsAnchorID(completed, normalizer.ID) {
		t.Fatalf("normalizer completion = expansions:%d retained:%t", expansions, containsAnchorID(completed, normalizer.ID))
	}
	completed = completeVerificationAnchor(completed, candidates, nil, contract)
	relations := collectRelations(completed, nil)
	decisive, found := selectDecisiveRelation(relations, completed, nil, contract.Profile)
	if !found || decisive.LeftID != normalizer.ID && decisive.RightID != normalizer.ID {
		t.Fatalf("decisive relation = %#v, want concrete normalizer endpoint", decisive)
	}
	frontier := buildVerificationFrontier(completed, relations, decisive, nil)
	if len(frontier.Anchors) == 0 || frontier.Anchors[0].AnchorID != exactTest.ID ||
		frontier.Anchors[0].Authority != VerificationExactExistingTest {
		t.Fatalf("verification frontier = %#v, want linked exact test %q", frontier, exactTest.ID)
	}
}

func TestCheapExitAmbiguityIsDerivedFromConnectedCompleteEvidence(t *testing.T) {
	t.Parallel()

	entry := semanticAnchor("entry", "pkg/entry.go", "Entry", RolePublicOrCLIEntry)
	decisiveAnchor := semanticAnchor("decisive", "pkg/transform.go", "Transform", RoleTransformation)
	output := semanticAnchor("output", "pkg/generated.go", "Generated", RoleGeneratedOutput)
	exactTest := semanticAnchor("exact-test", "pkg/transform_test.go", "TestTransform")
	anchors := []Anchor{entry, decisiveAnchor, output, exactTest}
	decisive := semanticRelation(RelationValueTransformed, entry, decisiveAnchor)
	relations := []Relation{
		decisive,
		semanticRelation(RelationTypeNameGenerated, decisiveAnchor, output),
		semanticRelation(RelationTestExercises, output, exactTest),
	}
	coverage := RoleCoverage{
		Profile: TaskProfileDataTagTransformation,
		Key: []RoleCoverageItem{
			semanticCoverage(RolePublicOrCLIEntry, entry),
			semanticCoverage(RoleTransformation, decisiveAnchor),
			semanticCoverage(RoleGeneratedOutput, output),
		},
	}
	frontier := buildVerificationFrontier(anchors, relations, decisive, nil)

	if got := unresolvedCompetingHypotheses(coverage, anchors, relations, decisive, frontier); got != 0 {
		t.Fatalf("complete connected data-profile evidence ambiguity = %d, want 0", got)
	}

	disconnectedRelations := []Relation{
		decisive,
		semanticRelation(RelationSharedStateAlias, decisiveAnchor, output),
		semanticRelation(RelationTestExercises, decisiveAnchor, exactTest),
	}
	disconnectedFrontier := buildVerificationFrontier(anchors, disconnectedRelations, decisive, nil)
	if got := unresolvedCompetingHypotheses(
		coverage,
		anchors,
		disconnectedRelations,
		decisive,
		disconnectedFrontier,
	); got == 0 {
		t.Fatal("disconnected key role did not leave an unresolved hypothesis")
	}

	partialAnchors := append([]Anchor(nil), anchors...)
	partialAnchors[2].Scope = partialSemanticScope()
	partialFrontier := buildVerificationFrontier(partialAnchors, relations, decisive, nil)
	if got := unresolvedCompetingHypotheses(
		coverage,
		partialAnchors,
		relations,
		decisive,
		partialFrontier,
	); got == 0 {
		t.Fatal("partial key-role scope did not leave an unresolved hypothesis")
	}

	extraOutput := semanticAnchor("extra-output", "other/generated.go", "OtherGenerated", RoleGeneratedOutput)
	extraCoverage := coverage
	extraCoverage.Key = append([]RoleCoverageItem(nil), coverage.Key...)
	extraCoverage.Key[2].AnchorIDs = []string{output.ID, extraOutput.ID}
	extraAnchors := append(append([]Anchor(nil), anchors...), extraOutput)
	if got := unresolvedCompetingHypotheses(
		extraCoverage,
		extraAnchors,
		relations,
		decisive,
		frontier,
	); got != 0 {
		t.Fatalf("unrelated extra role candidate ambiguity = %d, want 0", got)
	}
	alternative := semanticAnchor("alternative", "other/transform.go", "OtherTransform")
	alternativeEntry := semanticAnchor("alternative-entry", "other/entry.go", "OtherEntry", RolePublicOrCLIEntry)
	alternativeCoverage := extraCoverage
	alternativeCoverage.Key = append([]RoleCoverageItem(nil), extraCoverage.Key...)
	alternativeCoverage.Key[0].AnchorIDs = []string{entry.ID, alternativeEntry.ID}
	alternativeCoverage.Key[1].AnchorIDs = []string{decisiveAnchor.ID, alternative.ID}
	alternativeAnchors := append(extraAnchors, alternativeEntry, alternative)
	alternativeRelations := append(
		append([]Relation(nil), relations...),
		semanticRelation(RelationValueTransformed, alternativeEntry, alternative),
		semanticRelation(RelationValueTransformed, alternative, extraOutput),
	)
	if got := unresolvedCompetingHypotheses(
		alternativeCoverage,
		alternativeAnchors,
		alternativeRelations,
		decisive,
		frontier,
	); got == 0 {
		t.Fatal("alternative decisive key-role component did not leave an unresolved hypothesis")
	}
}

func TestCheapExitAreasIncludeKeyRoleAndVerificationWitnesses(t *testing.T) {
	t.Parallel()

	entry := semanticAnchor("area-entry", "pkg/entry.go", "Entry", RolePublicOrCLIEntry)
	decisiveAnchor := semanticAnchor("area-decisive", "pkg/transform.go", "Transform", RoleTransformation)
	output := semanticAnchor("area-output", "generated/output.go", "Generated", RoleGeneratedOutput)
	exactTest := semanticAnchor("area-test", "checks/transform_test.go", "TestTransform")
	anchors := []Anchor{entry, decisiveAnchor, output, exactTest}
	decisive := semanticRelation(RelationValueTransformed, entry, decisiveAnchor)
	relations := []Relation{
		decisive,
		semanticRelation(RelationTypeNameGenerated, decisiveAnchor, output),
		semanticRelation(RelationTestExercises, output, exactTest),
	}
	coverage := RoleCoverage{
		Profile: TaskProfileDataTagTransformation,
		Key: []RoleCoverageItem{
			semanticCoverage(RolePublicOrCLIEntry, entry),
			semanticCoverage(RoleTransformation, decisiveAnchor),
			semanticCoverage(RoleGeneratedOutput, output),
		},
	}
	frontier := buildVerificationFrontier(anchors, relations, decisive, nil)

	areas := cheapExitAreaIDs(decisive, coverage, anchors, relations, frontier)
	want := []string{"checks", "generated", "pkg"}
	if len(areas) != len(want) {
		t.Fatalf("cheap-exit areas = %v, want %v", areas, want)
	}
	for index := range want {
		if areas[index] != want[index] {
			t.Fatalf("cheap-exit areas = %v, want %v", areas, want)
		}
	}
}

func semanticAnchor(name, filePath, symbol string, roles ...AnchorRole) Anchor {
	return semanticAnchorWithText(name, filePath, symbol, symbol, roles...)
}

func semanticAnchorWithText(
	name string,
	filePath string,
	symbol string,
	text string,
	roles ...AnchorRole,
) Anchor {
	return Anchor{
		ID:          OpaqueID("anchor", name),
		Path:        filePath,
		Symbol:      symbol,
		StartLine:   1,
		EndLine:     1,
		Excerpt:     []SourceLine{{Line: 1, Text: text}},
		Scope:       completeSemanticScope(),
		RoleHints:   append([]AnchorRole(nil), roles...),
		EvidenceIDs: []string{OpaqueID("evidence", name)},
	}
}

func semanticRelation(kind RelationKind, left, right Anchor) Relation {
	return Relation{
		ID:          OpaqueID("relation", string(kind), left.ID, right.ID),
		LeftID:      left.ID,
		RightID:     right.ID,
		Kind:        string(kind),
		SupportType: SupportLocallyObserved,
		EvidenceIDs: []string{OpaqueID("relation-evidence", string(kind), left.ID, right.ID)},
		Scope:       "Exact retained anchor scopes only.",
	}
}

func semanticCoverage(role AnchorRole, anchors ...Anchor) RoleCoverageItem {
	ids := make([]string, 0, len(anchors))
	for _, anchor := range anchors {
		ids = append(ids, anchor.ID)
	}
	return RoleCoverageItem{
		Role:           role,
		MinimumAnchors: len(ids),
		AnchorIDs:      ids,
		Represented:    true,
	}
}

func completeSemanticScope() SourceScope {
	return SourceScope{
		ScopeKind:             SourceScopeCompleteEnclosingSymbol,
		ScopeStart:            1,
		ScopeEnd:              1,
		SourceTotalLines:      1,
		NegativeEvidenceBasis: NegativeEvidenceNone,
	}
}

func partialSemanticScope() SourceScope {
	return SourceScope{
		ScopeKind:                SourceScopePartialWindow,
		ScopeStart:               1,
		ScopeEnd:                 1,
		SourceTotalLines:         2,
		Truncated:                true,
		TruncationReason:         "per-anchor byte budget",
		TaskMatchesOutsideWindow: true,
		NegativeEvidenceBasis:    NegativeEvidenceNone,
	}
}
