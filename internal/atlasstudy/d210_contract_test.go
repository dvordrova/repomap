package atlasstudy

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

func TestD210CompileBuildsTypedSupportAndBackendSpanWireDeterministically(t *testing.T) {
	input := testInput()
	product := mustCompileTestProduct(t, input)
	var wire wireProjection
	if err := json.Unmarshal(product.WireJSON(), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Version != 9 || len(wire.RouteSupports) != 2 || len(wire.RouteSpans) != 1 {
		t.Fatalf("typed wire = version:%d supports:%d spans:%d", wire.Version, len(wire.RouteSupports), len(wire.RouteSpans))
	}
	span := wire.RouteSpans[0]
	if span.Ref != "sp1" || span.Kind != RouteSpanSystemPath ||
		span.Question != input.RouteSpans[0].QuestionEnglish || len(span.RequiredSupportRefs) != 2 ||
		len(span.AllowedTargetRefs) != 2 {
		t.Fatalf("typed span = %#v", span)
	}
	coverage := product.Coverage()
	// The D211 frontier is span-driven: the single advertised system-path span
	// allows only the config and startup targets, so the third considered
	// reading target is deliberately not selected and coverage stays explicit.
	if coverage.Complete || coverage.TargetsConsidered != 3 || coverage.TargetsSelected != 2 ||
		coverage.SpansConsidered != 1 || coverage.SpansSelected != 1 || coverage.CandidateSHA256 == "" {
		t.Fatalf("coverage = %#v", coverage)
	}

	reordered := cloneTestInput(input)
	slices.Reverse(reordered.ReadingTargets)
	slices.Reverse(reordered.ReadingSupports)
	slices.Reverse(reordered.RouteSpans)
	reorderedProduct := mustCompileTestProduct(t, reordered)
	if string(product.WireJSON()) != string(reorderedProduct.WireJSON()) ||
		!reflect.DeepEqual(product.Coverage(), reorderedProduct.Coverage()) {
		t.Fatal("producer permutation changed the typed request")
	}
	if product.CatalogSHA256() != reorderedProduct.CatalogSHA256() {
		t.Fatal("producer permutation changed catalog identity")
	}
}

func TestD210CompileFailsProviderFreeWhenRoleBudgetCannotBeRepresented(t *testing.T) {
	input := testInput()
	input.Limits.MaxReadingTargets = 1
	_, err := Compile(input)
	var unavailable *CandidateUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("Compile error = %T %v", err, err)
	}
}

func TestD210EveryRequestEncodableCoverageCanPersistAllStatusStates(t *testing.T) {
	input := testInput()
	targetID := input.ReadingTargets[0].ID
	input.ReadingTargets = input.ReadingTargets[:1]
	input.ReadingTargets[0].PrincipalRefs = []CanonicalRef{{Kind: RefComponent, ID: "component-api-canonical"}}
	input.Architecture.Components[0].ReadingTargetIDs = []string{targetID}
	input.Surfaces = nil
	input.Evidence = nil
	input.ProducerRelations = nil
	input.ReadingSupports = nil

	const packageBuckets = 530
	required := make([]string, 0, packageBuckets)
	for ordinal := range packageBuckets {
		supportID := fmt.Sprintf("support-large-%04d", ordinal)
		required = append(required, supportID)
		input.ReadingSupports = append(input.ReadingSupports, ReadingSupport{
			ID: supportID, TargetID: targetID,
			PackageBucket: fmt.Sprintf("package-bucket-%04d-%064x", ordinal, ordinal+1),
			Role:          SupportEntryHandoff, Authority: repositoryatlas.AuthorityResolved,
		})
	}
	span := RouteSpan{
		Kind:            RouteSpanFocused,
		QuestionEnglish: "Where is this exact producer-supported code boundary?",
		QuestionRussian: "Где находится эта точная подтверждённая продюсером граница кода?",
		TargetJob:       JobFirstContact, LearningStage: StageOrientation,
		RequiredSupportIDs: required, AllowedTargetIDs: []string{targetID},
	}
	first, second := span, span
	first.ID, second.ID = "span-large-a", "span-large-b"
	input.RouteSpans = []RouteSpan{first, second}

	product := mustCompileTestProduct(t, input)
	request := mustRequestRecord(t, product)
	if len(request.CandidateCoverage.PerPackage) != packageBuckets {
		t.Fatalf("per-package coverage = %d, want %d", len(request.CandidateCoverage.PerPackage), packageBuckets)
	}
	if _, err := EncodeRequestRecord(request); err != nil {
		t.Fatalf("large exact request is not encodable before provider call: %v", err)
	}

	statuses := []Status{product.PreparedStatus()}
	failed, err := product.FailureStatus(FailureProvider)
	if err != nil {
		t.Fatal(err)
	}
	statuses = append(statuses, failed)
	requested := []CanonicalRef{
		{Kind: RefRouteSpan, ID: "span-large-a"},
		{Kind: RefRouteSpan, ID: "span-large-b"},
	}
	coverage := product.Coverage()
	partial := product.status(ProductStateAcceptedPartial, 1, SpanCoverage{
		ConsideredSpanCount:     coverage.SpansConsidered,
		AdvertisedSpanCount:     len(product.selectedSpanIDs),
		ModelSelectedSpanCount:  1,
		AcceptedSpanCount:       1,
		FrontierComplete:        len(product.selectedSpanIDs) == coverage.SpansConsidered,
		SupportCoverageComplete: true,
	}, "", "")
	_ = requested
	statuses = append(statuses, partial)

	for _, status := range statuses {
		if err := product.ValidateStatus(status); err != nil {
			t.Fatalf("%s status does not bind compiled product: %v", status.State, err)
		}
		encoded, err := EncodeStatus(status)
		if err != nil {
			t.Fatalf("%s status cannot persist before/after provider call: %v", status.State, err)
		}
		if len(encoded) <= 64<<10 {
			t.Fatalf("%s regression fixture is only %d bytes; it does not cross the old status ceiling", status.State, len(encoded))
		}
		if _, err := DecodeStatus(encoded); err != nil {
			t.Fatalf("%s large status does not round-trip: %v", status.State, err)
		}
	}
}

func TestD210OneLocatorMayCoverSeveralObservedRolesWithinOneSlot(t *testing.T) {
	input := testInput()
	input.ReadingTargets = input.ReadingTargets[:1]
	input.Architecture.Components[0].ReadingTargetIDs = []string{"anchor-start-canonical"}
	input.Surfaces[0].ReadingTargetIDs = nil
	input.ReadingTargets[0].PrincipalRefs = []CanonicalRef{{Kind: RefComponent, ID: "component-api-canonical"}}
	input.ReadingSupports = []ReadingSupport{
		{ID: "support-entry", TargetID: "anchor-start-canonical", PackageBucket: "pkg-main", Role: SupportProcessEntry, Authority: repositoryatlas.AuthorityObserved},
		{ID: "support-surface", TargetID: "anchor-start-canonical", PackageBucket: "pkg-main", Role: SupportSurface, Authority: repositoryatlas.AuthorityResolved},
	}
	input.ProducerRelations = nil
	input.RouteSpans = []RouteSpan{{
		ID: "span-focused", Kind: RouteSpanFocused,
		QuestionEnglish: "Where is the exact observed application boundary?",
		QuestionRussian: "Где находится точная наблюдаемая граница приложения?",
		TargetJob:       JobFirstContact, LearningStage: StageOrientation,
		RequiredSupportIDs: []string{"support-entry", "support-surface"},
		AllowedTargetIDs:   []string{"anchor-start-canonical"},
	}}
	input.Limits.MaxReadingTargets = 1
	product := mustCompileTestProduct(t, input)
	if product.Coverage().TargetsSelected != 1 || len(product.input.ReadingSupports) != 2 {
		t.Fatalf("plural-support selection = %#v", product.Coverage())
	}
}

func TestD210SupportAuthorityMatrixRejectsSemanticUpgrades(t *testing.T) {
	for _, authority := range []repositoryatlas.Authority{
		repositoryatlas.AuthorityPartial, repositoryatlas.AuthorityInferred,
		repositoryatlas.AuthorityConflicted, repositoryatlas.AuthorityUnknown,
	} {
		input := testInput()
		input.ReadingSupports[0].Authority = authority
		if _, err := Compile(input); err == nil {
			t.Fatalf("exact support accepted authority %q", authority)
		}
	}
	if !validSupportAuthority(SupportSurfaceCandidate, repositoryatlas.AuthorityPartial) ||
		validSupportAuthority(SupportSurfaceCandidate, repositoryatlas.AuthorityResolved) {
		t.Fatal("surface-candidate authority matrix is not closed")
	}
}

func TestD210ProducerRelationRegistryRejectsMismatchesAndDuplicateProducerIdentity(t *testing.T) {
	input := testInput()
	input.ProducerRelations[0].ProducerID = ""
	if _, err := Compile(input); err == nil {
		t.Fatal("empty producer relation identity accepted")
	}
	input = testInput()
	input.ProducerRelations[0].ToTargetID = "anchor-route-canonical"
	if _, err := Compile(input); err == nil {
		t.Fatal("producer relation A to B support paired with A to C target accepted")
	}
	input = testInput()
	input.ProducerRelations[0].ToSupportID = "support-route"
	if _, err := Compile(input); err == nil {
		t.Fatal("producer relation A to C support paired with A to B target accepted")
	}
	input = testInput()
	secondSpan := input.RouteSpans[0]
	secondSpan.ID = "span-start-route-canonical"
	secondSpan.RequiredSupportIDs = []string{"support-route-canonical", "support-start-canonical"}
	secondSpan.AllowedTargetIDs = []string{"anchor-route-canonical", "anchor-start-canonical"}
	secondSpan.Joins = []RouteSpanJoin{{RelationID: "relation-start-route-canonical"}}
	input.RouteSpans = append(input.RouteSpans, secondSpan)
	input.ProducerRelations[1].ProducerID = input.ProducerRelations[0].ProducerID
	if _, err := Compile(input); err == nil {
		t.Fatal("duplicate producer relation identity accepted")
	}
	input = testInput()
	input.RouteSpans[0].Joins[0].RelationID = input.ProducerRelations[1].ID
	if _, err := Compile(input); err == nil {
		t.Fatal("duplicate/mismatched span relation accepted")
	}
}

func TestD210SystemPathIsExactlyOneDirectedProducerRelation(t *testing.T) {
	if _, err := Compile(testInput()); err != nil {
		t.Fatalf("exact one-relation system path rejected: %v", err)
	}

	multi := testInput()
	multi.RouteSpans[0].RequiredSupportIDs = []string{
		"support-config-canonical", "support-route-canonical", "support-start-canonical",
	}
	multi.RouteSpans[0].AllowedTargetIDs = []string{
		"anchor-config-canonical", "anchor-route-canonical", "anchor-start-canonical",
	}
	multi.RouteSpans[0].Joins = []RouteSpanJoin{
		{RelationID: "relation-start-config-canonical"},
		{RelationID: "relation-start-route-canonical"},
	}
	if _, err := Compile(multi); err == nil {
		t.Fatal("two real producer relations accepted as one system path")
	}

	disjoint := disjointEntryHandoffD210Input()
	if _, err := Compile(disjoint); err == nil {
		t.Fatal("disjoint A to B plus C to D relations accepted as one system path")
	}

	mixedFlows := mixedSavedFlowD210Input()
	if _, err := Compile(mixedFlows); err == nil {
		t.Fatal("edges from separate saved flows accepted as one system path")
	}
}

func TestD210SavedFlowJoinRequiresExactAdjacentAcceptedEdge(t *testing.T) {
	input := savedFlowD210Input()
	if _, err := Compile(input); err != nil {
		t.Fatalf("exact adjacent saved-flow edge rejected: %v", err)
	}
	for _, toOrdinal := range []int{0, 3} {
		changed := cloneTestInput(input)
		changed.ProducerRelations[0].ToStepOrdinal = toOrdinal
		if _, err := Compile(changed); err == nil {
			t.Fatalf("saved-flow ordinal 1 -> %d accepted", toOrdinal)
		}
	}
}

func TestD210PrivateProducerRelationsAreNotProviderVisible(t *testing.T) {
	product := mustCompileTestProduct(t, testInput())
	wire := string(product.WireJSON())
	for _, private := range []string{"handoff-start-config-canonical", "handoff-start-route-canonical", "relation-start-config-canonical"} {
		if strings.Contains(wire, private) {
			t.Fatalf("provider wire exposes private producer relation %q", private)
		}
	}
}

func TestD210CandidateIdentityBindsFullBackendSpanPromise(t *testing.T) {
	base := mustCompileTestProduct(t, testInput()).Coverage().CandidateSHA256
	changes := []func(*RouteSpan){
		func(span *RouteSpan) { span.QuestionEnglish = "Which exact producer handoffs leave the entry?" },
		func(span *RouteSpan) { span.TargetJob = JobMaintain },
		func(span *RouteSpan) { span.LearningStage = StageOperations },
	}
	for _, change := range changes {
		input := testInput()
		change(&input.RouteSpans[0])
		if got := mustCompileTestProduct(t, input).Coverage().CandidateSHA256; got == base {
			t.Fatal("backend span promise did not change candidate identity")
		}
	}
}

func TestD210ShortRefsSkipPrivateIdentityCollisions(t *testing.T) {
	input := testInput()
	input.ReadingSupports[0].ID = "rs1"
	input.RouteSpans[0].RequiredSupportIDs[0] = "rs1"
	input.ProducerRelations[0].ToSupportID = "rs1"
	input.ProducerRelations[0].ID = "rr1"
	input.RouteSpans[0].Joins[0].RelationID = "rr1"
	input.RouteSpans[0].ID = "sp1"
	product := mustCompileTestProduct(t, input)
	if got := catalogObject(t, product.Catalog(), RefRouteSupport, "rs1").Ref; got == "rs1" {
		t.Fatal("route support short ref collides with raw canonical identity")
	}
	if got := catalogObject(t, product.Catalog(), RefRouteSpan, "sp1").Ref; got == "sp1" {
		t.Fatal("route span short ref collides with raw canonical identity")
	}
	if got := catalogObject(t, product.Catalog(), RefRouteRelation, "rr1").Ref; got == "rr1" {
		t.Fatal("route relation short ref collides with raw canonical identity")
	}
}

func TestD210BreadthRotatesPackagesAndRecordsExplicitPartialCoverage(t *testing.T) {
	input := testInput()
	for ordinal, packageBucket := range []string{"package-config-canonical", "package-other-canonical"} {
		id := "anchor-breadth-" + string(rune('a'+ordinal))
		input.ReadingTargets = append(input.ReadingTargets, ReadingTarget{
			ID: id, Owner: CanonicalRef{Kind: RefComponent, ID: "component-api-canonical"},
			RelatedComponentIDs: []string{"component-api-canonical"},
			PrincipalRefs:       []CanonicalRef{{Kind: RefComponent, ID: "component-api-canonical"}},
			Kind:                ReadingTargetFunction, Label: "Additional exact call boundary",
			Fact: "Records another exact producer-owned call boundary.", Authority: repositoryatlas.AuthorityObserved,
			Location: evidence.Location{Path: "internal/breadth/" + id + ".go", Line: ordinal + 1}, Symbol: "example.com/" + id,
		})
		input.Architecture.Components[0].ReadingTargetIDs = append(input.Architecture.Components[0].ReadingTargetIDs, id)
		input.ReadingSupports = append(input.ReadingSupports, ReadingSupport{
			ID: "support-" + id, TargetID: id, PackageBucket: packageBucket,
			Role: SupportEntryHandoff, Authority: repositoryatlas.AuthorityResolved,
		})
	}
	input.Limits.MaxReadingTargets = 4
	product := mustCompileTestProduct(t, input)
	coverage := product.Coverage()
	if coverage.Complete || coverage.TargetsConsidered != 5 || coverage.TargetsSelected != 2 ||
		coverage.SpansConsidered != 1 || coverage.SpansSelected != 1 {
		t.Fatalf("partial breadth coverage = %#v", coverage)
	}
	wantRoles := []CandidateCoverageCount{
		{Key: "entry_handoff", Considered: 4, Selected: 1},
		{Key: "process_entry", Considered: 1, Selected: 1},
	}
	if !reflect.DeepEqual(coverage.PerRole, wantRoles) {
		t.Fatalf("per-role breadth coverage = %#v", coverage.PerRole)
	}
	wantPackages := []CandidateCoverageCount{
		{Key: "package-config-canonical", Considered: 2, Selected: 1},
		{Key: "package-main-canonical", Considered: 1, Selected: 1},
		{Key: "package-other-canonical", Considered: 1, Selected: 0},
		{Key: "package-server-canonical", Considered: 1, Selected: 0},
	}
	if !reflect.DeepEqual(coverage.PerPackage, wantPackages) {
		t.Fatalf("per-package breadth coverage = %#v", coverage.PerPackage)
	}
	// The D211 frontier is span-driven: the single advertised span covers the
	// process entry and one entry handoff, and the extra breadth supports are
	// not required by any span. Their exact packages stay visible in the
	// bounded coverage aggregates as considered-but-unselected, and the
	// unselected breadth target never enters the advertised catalog.
	if _, ok := product.byCanonical[CanonicalRef{Kind: RefReadingTarget, ID: "anchor-breadth-b"}]; ok {
		t.Fatal("span-unreferenced breadth target entered the advertised catalog")
	}
}

func TestD210SpanBreadthRotatesExactSupportPackagesWithinLimitedSlots(t *testing.T) {
	supports := map[string]ReadingSupport{
		"support-a-1": {ID: "support-a-1", TargetID: "target-a-1", PackageBucket: "package-a", Role: SupportEntryHandoff, Authority: repositoryatlas.AuthorityResolved},
		"support-a-2": {ID: "support-a-2", TargetID: "target-a-2", PackageBucket: "package-a", Role: SupportEntryHandoff, Authority: repositoryatlas.AuthorityResolved},
		"support-b":   {ID: "support-b", TargetID: "target-b", PackageBucket: "package-b", Role: SupportEntryHandoff, Authority: repositoryatlas.AuthorityResolved},
		"support-c":   {ID: "support-c", TargetID: "target-c", PackageBucket: "package-c", Role: SupportEntryHandoff, Authority: repositoryatlas.AuthorityResolved},
		"support-d":   {ID: "support-d", TargetID: "target-d", PackageBucket: "package-d", Role: SupportEntryHandoff, Authority: repositoryatlas.AuthorityResolved},
	}
	spans := []RouteSpan{
		{ID: "span-a-1", RequiredSupportIDs: []string{"support-a-1"}, AllowedTargetIDs: []string{"target-a-1"}},
		{ID: "span-a-2", RequiredSupportIDs: []string{"support-a-2"}, AllowedTargetIDs: []string{"target-a-2"}},
		{ID: "span-bc", RequiredSupportIDs: []string{"support-b", "support-c"}, AllowedTargetIDs: []string{"target-b", "target-c"}},
		{ID: "span-d", RequiredSupportIDs: []string{"support-d"}, AllowedTargetIDs: []string{"target-d"}},
	}
	want := []string{"span-a-1", "span-bc", "span-d"}
	if got := selectedSpanIDs(selectSpansByRole(spans, supports, nil, []SupportRole{SupportEntryHandoff}, 3)); !reflect.DeepEqual(got, want) {
		t.Fatalf("package-diverse spans = %v, want %v", got, want)
	}
	slices.Reverse(spans)
	if got := selectedSpanIDs(selectSpansByRole(spans, supports, nil, []SupportRole{SupportEntryHandoff}, 3)); !reflect.DeepEqual(got, want) {
		t.Fatalf("permuted package-diverse spans = %v, want %v", got, want)
	}
}

func TestD210CompileAdvertisesPackageDiverseSpans(t *testing.T) {
	input := testInput()
	for ordinal, id := range []string{"target-extra-c", "target-extra-d"} {
		target := input.ReadingTargets[0]
		target.ID = id
		target.Location = evidence.Location{Path: "internal/extra/" + id + ".go", Line: ordinal + 1}
		target.Symbol = "example.com/" + id
		input.ReadingTargets = append(input.ReadingTargets, target)
		input.Architecture.Components[0].ReadingTargetIDs = append(input.Architecture.Components[0].ReadingTargetIDs, id)
	}
	targetIDs := []string{
		input.ReadingTargets[0].ID, input.ReadingTargets[1].ID, input.ReadingTargets[2].ID,
		"target-extra-c", "target-extra-d",
	}
	packages := []string{"package-a", "package-a", "package-b", "package-c", "package-d"}
	spanIDs := []string{"span-a-1", "span-a-2", "span-b", "span-c", "span-d"}
	input.ReadingSupports = nil
	input.ProducerRelations = nil
	input.RouteSpans = nil
	for index, targetID := range targetIDs {
		supportID := "support-diverse-" + string(rune('a'+index))
		input.ReadingSupports = append(input.ReadingSupports, ReadingSupport{
			ID: supportID, TargetID: targetID, PackageBucket: packages[index],
			Role: SupportEntryHandoff, Authority: repositoryatlas.AuthorityResolved,
		})
		input.RouteSpans = append(input.RouteSpans, RouteSpan{
			ID: spanIDs[index], Kind: RouteSpanFocused,
			QuestionEnglish: "Where is this exact observed handoff?",
			QuestionRussian: "Где находится эта точная наблюдаемая передача управления?",
			TargetJob:       JobFirstContact, LearningStage: StageOrientation,
			RequiredSupportIDs: []string{supportID}, AllowedTargetIDs: []string{targetID},
		})
	}
	input.Limits.MaxAdvertisedSpans = 3
	product := mustCompileTestProduct(t, input)
	if got, want := selectedSpanIDs(product.input.RouteSpans), []string{"span-a-1", "span-b", "span-c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("advertised package-diverse spans = %v, want %v", got, want)
	}
}

func TestD210LocalizedQuestionsBindWireButNotCanonicalDirectionIdentity(t *testing.T) {
	english := mustCompileTestProduct(t, testInput())
	russianInput := cloneTestInput(testInput())
	russianInput.Language = LanguageRussian
	russian := mustCompileTestProduct(t, russianInput)
	if english.WireSHA256() == russian.WireSHA256() || english.CatalogSHA256() == russian.CatalogSHA256() {
		t.Fatal("localized backend questions did not bind request identity")
	}
	englishDirection, code := english.resolveDirection(0, validD210ProviderDirection(t, english))
	if code != "" {
		t.Fatalf("English direction: %s", code)
	}
	russianDirection, code := russian.resolveDirection(0, validD210ProviderDirection(t, russian))
	if code != "" {
		t.Fatalf("Russian direction: %s", code)
	}
	if englishDirection.Question == russianDirection.Question {
		t.Fatal("localized question was not restored locally")
	}
	englishDirection.Question = russianDirection.Question
	if stableDirectionID(englishDirection) != russianDirection.ID {
		t.Fatal("localized prose changed canonical direction identity")
	}
}

func TestD210ResolverEnforcesExactSpanCoverageWithoutPadding(t *testing.T) {
	product := mustCompileTestProduct(t, testInput())
	valid := validD210ProviderDirection(t, product)
	resolved, code := product.resolveDirection(0, valid)
	if code != "" || resolved.Span != (CanonicalRef{Kind: RefRouteSpan, ID: "span-start-canonical"}) ||
		resolved.Question != testInput().RouteSpans[0].QuestionEnglish {
		t.Fatalf("valid direction = %#v / %s", resolved, code)
	}

	incomplete := valid
	incomplete.Reading = incomplete.Reading[:1]
	if _, code := product.resolveDirection(0, incomplete); code != IssueSpanSupportIncomplete {
		t.Fatalf("incomplete code = %s", code)
	}

	wrongKind := valid
	wrongKind.SpanRef = refFor(t, product, RefReadingTarget, "anchor-start-canonical")
	if _, code := product.resolveDirection(0, wrongKind); code != IssueWrongKindSpanRef {
		t.Fatalf("wrong-kind code = %s", code)
	}

	raw := valid
	raw.SpanRef = "span-start-canonical"
	if _, code := product.resolveDirection(0, raw); code != IssueRawCanonicalRef {
		t.Fatalf("raw span code = %s", code)
	}

	unknown := valid
	unknown.SpanRef = "sp999"
	if _, code := product.resolveDirection(0, unknown); code != IssueUnknownRef {
		t.Fatalf("unknown span code = %s", code)
	}
}

func TestD210ResolverPreservesValidSiblingAndRejectsDuplicateSpanItemLocally(t *testing.T) {
	product := mustCompileTestProduct(t, testInput())
	valid := validD210ProviderDirection(t, product)
	bad := valid
	bad.Reading = bad.Reading[:1]
	items := []json.RawMessage{marshalTestJSON(t, bad), marshalTestJSON(t, valid)}
	directions, diagnostics, _ := product.resolveDirections(items)
	if len(directions) != 1 || diagnostics.DirectionsRejected != 1 ||
		len(diagnostics.Issues) != 1 || diagnostics.Issues[0].Code != IssueSpanSupportIncomplete {
		t.Fatalf("item-local result = %#v / %#v", directions, diagnostics)
	}

	items = []json.RawMessage{marshalTestJSON(t, valid), marshalTestJSON(t, valid)}
	directions, diagnostics, _ = product.resolveDirections(items)
	if len(directions) != 1 || len(diagnostics.Issues) != 1 || diagnostics.Issues[0].Code != IssueDuplicateSpanRef {
		t.Fatalf("duplicate span result = %#v / %#v", directions, diagnostics)
	}
}

func validD210ProviderDirection(t *testing.T, product Product) providerDirection {
	t.Helper()
	component := refFor(t, product, RefComponent, "component-api-canonical")
	return providerDirection{
		SpanRef:         refFor(t, product, RefRouteSpan, "span-start-canonical"),
		WhyItMatters:    "This exact observed boundary helps orient the reader.",
		LearningOutcome: "The reader can identify the exact observed entry and connected boundaries.",
		TargetJob:       JobFirstContact,
		LearningStage:   StageOrientation,
		PrincipalRefs:   []string{component},
		Reading: []providerReading{
			{TargetRef: refFor(t, product, RefReadingTarget, "anchor-config-canonical"), Label: ReadingStart, WhatToLookFor: "Inspect the exact configuration boundary."},
			{TargetRef: refFor(t, product, RefReadingTarget, "anchor-start-canonical"), Label: ReadingVerify, WhatToLookFor: "Inspect the exact process entry."},
		},
	}
}

func disjointEntryHandoffD210Input() Input {
	input := testInput()
	input.ReadingTargets = append(input.ReadingTargets, ReadingTarget{
		ID: "anchor-worker-start-canonical", Kind: ReadingTargetEntrypoint,
		Label: "Worker startup", Fact: "Starts a separate exact worker.",
		Authority: repositoryatlas.AuthorityObserved,
		Location:  evidence.Location{Path: "cmd/worker/main.go", Line: 10}, Symbol: "RunWorker",
		PrincipalRefs: []CanonicalRef{{Kind: RefComponent, ID: "component-api-canonical"}},
	})
	input.Architecture.Components[0].ReadingTargetIDs = append(
		input.Architecture.Components[0].ReadingTargetIDs, "anchor-worker-start-canonical",
	)
	input.ReadingSupports = append(input.ReadingSupports, ReadingSupport{
		ID: "support-worker-start-canonical", TargetID: "anchor-worker-start-canonical",
		PackageBucket: "package-worker-canonical", Role: SupportProcessEntry,
		Authority: repositoryatlas.AuthorityObserved,
	})
	input.ProducerRelations = append(input.ProducerRelations, RouteProducerRelation{
		ID: "relation-worker-route-canonical", Kind: RouteRelationEntryHandoff,
		ProducerID:    "handoff-worker-route-canonical",
		FromSupportID: "support-worker-start-canonical", ToSupportID: "support-route-canonical",
		FromTargetID: "anchor-worker-start-canonical", ToTargetID: "anchor-route-canonical",
	})
	input.RouteSpans[0].RequiredSupportIDs = []string{
		"support-config-canonical", "support-route-canonical",
		"support-start-canonical", "support-worker-start-canonical",
	}
	input.RouteSpans[0].AllowedTargetIDs = []string{
		"anchor-config-canonical", "anchor-route-canonical",
		"anchor-start-canonical", "anchor-worker-start-canonical",
	}
	input.RouteSpans[0].Joins = []RouteSpanJoin{
		{RelationID: "relation-start-config-canonical"},
		{RelationID: "relation-worker-route-canonical"},
	}
	return input
}

func mixedSavedFlowD210Input() Input {
	input := savedFlowD210Input()
	third := ReadingTarget{
		ID: "anchor-flow-third-canonical", Kind: ReadingTargetFunction,
		Label: "Another saved-flow step", Fact: "Belongs to a different exact saved flow.",
		Authority: repositoryatlas.AuthorityObserved,
		Location:  evidence.Location{Path: "internal/flow/third.go", Line: 30}, Symbol: "Third",
		PrincipalRefs: []CanonicalRef{{Kind: RefComponent, ID: "component-api-canonical"}},
	}
	input.ReadingTargets = append(input.ReadingTargets, third)
	input.Architecture.Components[0].ReadingTargetIDs = append(
		input.Architecture.Components[0].ReadingTargetIDs, third.ID,
	)
	input.ReadingSupports = append(input.ReadingSupports, ReadingSupport{
		ID: "support-flow-3", TargetID: third.ID, PackageBucket: "pkg-c",
		Role: SupportSavedFlow, Authority: repositoryatlas.AuthorityResolved,
	})
	input.ProducerRelations = append(input.ProducerRelations, RouteProducerRelation{
		ID: "flow-edge-2", Kind: RouteRelationSavedFlowEdge,
		ProducerID:    "saved-flow-edge-other-canonical",
		FromSupportID: "support-flow-2", ToSupportID: "support-flow-3",
		FromTargetID: input.ReadingTargets[1].ID, ToTargetID: third.ID,
		SavedFlowID: "flow-other-canonical", FromStepID: "other-step-1", ToStepID: "other-step-2",
		FromStepOrdinal: 1, ToStepOrdinal: 2,
	})
	input.RouteSpans[0].RequiredSupportIDs = []string{"support-flow-1", "support-flow-2", "support-flow-3"}
	input.RouteSpans[0].AllowedTargetIDs = []string{
		input.ReadingTargets[0].ID, input.ReadingTargets[1].ID, third.ID,
	}
	input.RouteSpans[0].Joins = []RouteSpanJoin{{RelationID: "flow-edge-1"}, {RelationID: "flow-edge-2"}}
	return input
}

func savedFlowD210Input() Input {
	input := testInput()
	input.ReadingTargets = input.ReadingTargets[:2]
	input.Architecture.Components[0].ReadingTargetIDs = []string{"anchor-config-canonical", "anchor-start-canonical"}
	input.Surfaces = nil
	input.Evidence = nil
	for index := range input.ReadingTargets {
		input.ReadingTargets[index].PrincipalRefs = []CanonicalRef{{Kind: RefComponent, ID: "component-api-canonical"}}
	}
	input.ReadingSupports = []ReadingSupport{
		{ID: "support-flow-1", TargetID: input.ReadingTargets[0].ID, PackageBucket: "pkg-a", Role: SupportSavedFlow, Authority: repositoryatlas.AuthorityResolved},
		{ID: "support-flow-2", TargetID: input.ReadingTargets[1].ID, PackageBucket: "pkg-b", Role: SupportSavedFlow, Authority: repositoryatlas.AuthorityResolved},
	}
	input.ProducerRelations = []RouteProducerRelation{{
		ID: "flow-edge-1", Kind: RouteRelationSavedFlowEdge, ProducerID: "saved-flow-edge-canonical",
		FromSupportID: "support-flow-1", ToSupportID: "support-flow-2",
		FromTargetID: input.ReadingTargets[0].ID, ToTargetID: input.ReadingTargets[1].ID,
		SavedFlowID: "flow-canonical", FromStepID: "step-1", ToStepID: "step-2",
		FromStepOrdinal: 1, ToStepOrdinal: 2,
	}}
	input.RouteSpans = []RouteSpan{{
		ID: "span-flow", Kind: RouteSpanSystemPath,
		QuestionEnglish: "How does the exact saved flow connect these boundaries?",
		QuestionRussian: "Как точный сохранённый поток связывает эти границы?",
		TargetJob:       JobMaintain, LearningStage: StageOperations,
		RequiredSupportIDs: []string{"support-flow-1", "support-flow-2"},
		AllowedTargetIDs:   []string{input.ReadingTargets[0].ID, input.ReadingTargets[1].ID},
		Joins:              []RouteSpanJoin{{RelationID: "flow-edge-1"}},
	}}
	return input
}
