package tasklens

import "testing"

func TestV02VerificationBindsReceiverMethodTest(t *testing.T) {
	t.Parallel()

	entry := semanticAnchorWithText(
		"receiver-entry",
		"fixture/entry.go",
		"Handle",
		"func Handle(processor *Processor) error { return processor.Apply() }",
	)
	method := semanticAnchorWithText(
		"receiver-method",
		"fixture/processor.go",
		"Processor.Apply",
		"func (processor *Processor) Apply() error { return nil }",
	)
	exactTest := semanticAnchorWithText(
		"receiver-method-test",
		"fixture/processor_test.go",
		"TestProcessorApply",
		"func TestProcessorApply(t *testing.T) { processor := &Processor{}; if err := processor.Apply(); err != nil { t.Fatal(err) } }",
	)
	entry.Package = "fixture"
	method.Package = "fixture"
	exactTest.Package = "fixture"

	decisive := semanticRelation(RelationDirectCall, entry, method)
	relations := append(
		[]Relation{decisive},
		collectRelations([]Anchor{entry, method, exactTest}, nil)...,
	)
	frontier := buildVerificationFrontier(
		[]Anchor{entry, method, exactTest},
		relations,
		decisive,
		nil,
	)

	if len(frontier.Anchors) != 1 ||
		frontier.Anchors[0].AnchorID != exactTest.ID ||
		frontier.Anchors[0].Authority != VerificationExactExistingTest {
		t.Fatalf(
			"receiver-method verification = %#v, want exact test anchor %q",
			frontier.Anchors,
			exactTest.ID,
		)
	}
}

func TestV02VerificationBindsTaskObservableAssertions(t *testing.T) {
	t.Parallel()

	entry := semanticAnchorWithText(
		"observable-entry",
		"transport/handler.go",
		"Handle",
		"func Handle() Response { return negotiate() }",
	)
	mechanism := semanticAnchorWithText(
		"observable-mechanism",
		"transport/negotiate.go",
		"negotiate",
		"func negotiate() Response { return Response{StatusCode: 406} }",
	)
	correctTest := semanticAnchorWithText(
		"observable-assertion-test",
		"transport/negotiate_test.go",
		"TestObservedResponse",
		`func TestObservedResponse(t *testing.T) { got := observe(); if got.StatusCode != 406 { t.Fatalf("status = %d, want 406", got.StatusCode) } }`,
	)
	unrelatedSibling := semanticAnchorWithText(
		"observable-unrelated-sibling",
		"transport/status_test.go",
		"TestStatusDocumentation",
		`func TestStatusDocumentation(t *testing.T) { label := "status 406"; got := unrelated(); if got != 1 { t.Fatalf("count = %d, want 1", got) }; _ = label }`,
	)
	entry.Package = "transport"
	mechanism.Package = "transport"
	correctTest.Package = "transport"
	unrelatedSibling.Package = "transport"
	correctTest.Score = 1
	unrelatedSibling.Score = 1_000

	decisive := semanticRelation(RelationDirectCall, entry, mechanism)
	frontier := buildVerificationFrontier(
		[]Anchor{entry, mechanism, unrelatedSibling, correctTest},
		[]Relation{decisive},
		decisive,
		[]Term{
			{Normalized: "status", Weight: 12},
			{Normalized: "406", Weight: 12},
		},
	)

	if len(frontier.Anchors) != 1 ||
		frontier.Anchors[0].AnchorID != correctTest.ID ||
		frontier.Anchors[0].Authority != VerificationExactExistingTest {
		t.Fatalf(
			"task-observable verification = %#v, want exact assertion-bearing test %q and no sibling %q",
			frontier.Anchors,
			correctTest.ID,
			unrelatedSibling.ID,
		)
	}
}

func TestV02VerificationAssertionsIgnoreCalledFunctionNames(t *testing.T) {
	t.Parallel()

	entry := semanticAnchorWithText(
		"assertion-entry",
		"schema/customizer.go",
		"Customize",
		"func Customize() Schema { return parseRequiredNullable() }",
	)
	mechanism := semanticAnchorWithText(
		"assertion-mechanism",
		"schema/customizer.go",
		"parseRequiredNullable",
		"func parseRequiredNullable() Schema { return Schema{Required: true, Nullable: false} }",
	)
	correctTest := semanticAnchorWithText(
		"assertion-correct-test",
		"schema/customizer_test.go",
		"TestRequiredNullable",
		"func TestRequiredNullable(t *testing.T) { assert.True(t, requiredField); assert.False(t, nullableField) }",
	)
	calleeOnlyTest := semanticAnchorWithText(
		"assertion-callee-only-test",
		"schema/openapi_test.go",
		"TestValidateOpenAPI",
		`func TestValidateOpenAPI(t *testing.T) { require.True(t, validateOpenAPI("/openapi")) }`,
	)
	for _, anchor := range []*Anchor{&entry, &mechanism, &correctTest, &calleeOnlyTest} {
		anchor.Package = "schema"
	}
	calleeOnlyTest.Score = 1_000
	decisive := semanticRelation(RelationDirectCall, entry, mechanism)
	frontier := buildVerificationFrontier(
		[]Anchor{entry, mechanism, calleeOnlyTest, correctTest},
		[]Relation{decisive},
		decisive,
		[]Term{
			{Normalized: "required", Weight: 16},
			{Normalized: "nullable", Weight: 16},
			{Normalized: "validate", Weight: 8},
			{Normalized: "openapi", Weight: 10},
		},
	)

	if len(frontier.Anchors) != 1 ||
		frontier.Anchors[0].AnchorID != correctTest.ID ||
		frontier.Anchors[0].Authority != VerificationExactExistingTest {
		t.Fatalf(
			"assertion verification = %#v, want focused test %q and no callee-only test %q",
			frontier.Anchors,
			correctTest.ID,
			calleeOnlyTest.ID,
		)
	}
}

func TestV02GenericDocumentBridgeCannotAuthorizeExactVerification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		candidate Anchor
		bridge    func(document, candidate Anchor) []Relation
	}{
		{
			name: "test",
			candidate: semanticAnchorWithText(
				"document-bridge-test",
				"cache/cache_test.go",
				"TestCache",
				"func TestCache(t *testing.T) { t.Fatal(\"unrelated\") }",
			),
			bridge: func(document, candidate Anchor) []Relation {
				return []Relation{semanticRelation(RelationDocumentedUses, document, candidate)}
			},
		},
		{
			name: "fixture",
			candidate: semanticAnchorWithText(
				"document-bridge-fixture",
				"cache/testdata/cache.golden.json",
				"cache-golden",
				`{"unrelated": true}`,
			),
			bridge: func(document, candidate Anchor) []Relation {
				return []Relation{semanticRelation(RelationFixtureRecords, document, candidate)}
			},
		},
		{
			name: "command",
			candidate: semanticAnchorWithText(
				"document-bridge-command",
				"docs/testing.md",
				"Testing",
				"go test ./...",
			),
			bridge: func(_, _ Anchor) []Relation {
				return nil
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			entry := semanticAnchorWithText(
				"document-bridge-entry-"+test.name,
				"service/entry.go",
				"Serve",
				"func Serve() error { return applyPolicy() }",
			)
			mechanism := semanticAnchorWithText(
				"document-bridge-mechanism-"+test.name,
				"service/policy.go",
				"applyPolicy",
				"func applyPolicy() error { return nil }",
			)
			document := semanticAnchorWithText(
				"document-bridge-document-"+test.name,
				"docs/overview.md",
				"Overview",
				"This generic overview links repository areas.",
			)
			if test.name == "command" {
				document = test.candidate
			}

			decisive := semanticRelation(RelationDirectCall, entry, mechanism)
			relations := []Relation{
				decisive,
				semanticRelation(RelationDocumentedUses, mechanism, document),
			}
			relations = append(relations, test.bridge(document, test.candidate)...)
			anchors := []Anchor{entry, mechanism, document}
			if test.candidate.ID != document.ID {
				anchors = append(anchors, test.candidate)
			}

			frontier := buildVerificationFrontier(anchors, relations, decisive, nil)
			if frontier.HasExactAnchorOrEffect() {
				t.Fatalf(
					"generic document bridge authorized exact %s verification: %#v",
					test.name,
					frontier,
				)
			}
		})
	}
}

func TestV02VerificationFrontierRejectsDocumentedCommandWithoutAnchorID(t *testing.T) {
	t.Parallel()

	command := VerificationItem{
		ID:          OpaqueID("verification", "documented-command-without-anchor"),
		Authority:   VerificationDocumentedCommand,
		Path:        "docs/testing.md",
		Symbol:      "Testing",
		Text:        "Documented repository command: go test ./...",
		EvidenceIDs: []string{OpaqueID("evidence", "documented-command-without-anchor")},
	}
	frontier := VerificationFrontier{
		DecisiveAnchorID: OpaqueID("anchor", "decisive"),
		Anchors:          []VerificationItem{},
		CommandOrEffect:  &command,
	}

	if err := frontier.Validate(); err == nil {
		t.Fatal("VerificationFrontier.Validate() accepted a documented command without AnchorID")
	}
}
