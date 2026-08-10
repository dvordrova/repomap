package mechanismstudy

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestMockResponseRoundTripRestoresBackendOwnedPathAndReadings(t *testing.T) {
	compilation, batch := compileChainBatch(t)
	raw, err := MockResponse(batch)
	if err != nil {
		t.Fatalf("MockResponse: %v", err)
	}
	response := decodeResponse(t, raw)
	if len(response.Cards) != 1 || len(response.Cards[0].Mechanisms) != 1 ||
		len(response.Cards[0].Mechanisms[0].EdgeRefs) != 2 {
		t.Fatalf("mock response is not one refs-only fixture: %+v", response)
	}
	result, err := ResolveResponse(compilation, batch, raw)
	if err != nil {
		t.Fatalf("ResolveResponse: %v", err)
	}
	if len(result.Cards) != 1 || result.Cards[0].State != OutcomeMechanism || len(result.Cards[0].Mechanisms) != 1 {
		t.Fatalf("mock result = %+v", result)
	}
	mechanism := result.Cards[0].Mechanisms[0]
	if len(mechanism.ReadingRefs) == 0 || len(mechanism.NodeRefs) != len(mechanism.EdgeRefs)+1 {
		t.Fatalf("backend did not derive path/readings: %+v", mechanism)
	}
	assertMechanismFollowsExactEdges(t, batch.Request.Cards[0], mechanism)
}

func TestResolveResponseSalvagesValidSiblingAfterInvalidFirst(t *testing.T) {
	compilation, batch := compileChainBatch(t)
	card := batch.Request.Cards[0]
	valid := chainCandidate(t, card, "entry", "service", "persist")
	invalid := Candidate{EdgeRefs: make([]string, MaxEdgesPerMechanism+1)}
	for position := range invalid.EdgeRefs {
		invalid.EdgeRefs[position] = valid.EdgeRefs[position%len(valid.EdgeRefs)]
	}
	result, err := ResolveResponse(compilation, batch, responseJSON(t, batch, []Candidate{invalid, valid}))
	if err != nil {
		t.Fatalf("ResolveResponse: %v", err)
	}
	got := result.Cards[0]
	if got.State != OutcomeMechanism || len(got.Mechanisms) != 1 ||
		len(got.Issues) != 1 || got.Issues[0].Code != IssueOverBound {
		t.Fatalf("invalid sibling suppressed valid path: %+v", got)
	}
}

func TestResolveResponseAdversarialCandidatesCloseItemLocally(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, card Card, valid Candidate) Candidate
		code   IssueCode
	}{
		{
			name: "invented edge ref", code: IssueUnknownRef,
			mutate: func(_ *testing.T, _ Card, candidate Candidate) Candidate {
				candidate.EdgeRefs[0] = "e9999"
				return candidate
			},
		},
		{
			name: "duplicate edge", code: IssueDuplicateRef,
			mutate: func(_ *testing.T, _ Card, candidate Candidate) Candidate {
				candidate.EdgeRefs[1] = candidate.EdgeRefs[0]
				return candidate
			},
		},
		{
			name: "disconnected edge set", code: IssueDisconnected,
			mutate: func(t *testing.T, card Card, _ Candidate) Candidate {
				first := requireEdgeByLabels(t, card, "main", "entry")
				second := requireEdgeByLabels(t, card, "service", "persist")
				return Candidate{EdgeRefs: []string{second.Ref, first.Ref}}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compilation, batch := compileChainBatch(t)
			card := batch.Request.Cards[0]
			invalid := test.mutate(t, card, chainCandidate(t, card, "entry", "service", "persist"))
			result, err := ResolveResponse(compilation, batch, responseJSON(t, batch, []Candidate{invalid}))
			if err != nil {
				t.Fatalf("ResolveResponse: %v", err)
			}
			got := result.Cards[0]
			if got.State != OutcomePrepared || len(got.Mechanisms) != 0 ||
				len(got.Issues) != 1 || got.Issues[0].Code != test.code {
				t.Fatalf("adversarial result = %+v, want prepared/%s", got, test.code)
			}
			if countResultFrontier(got, FrontierResponseInvalid) != 1 {
				t.Fatalf("response_invalid frontier = %+v", got.Frontier)
			}
		})
	}
}

func TestValidateCandidateDerivesAllReadingTiesAndRejectsNone(t *testing.T) {
	edges := map[string]Edge{
		"e1": {Ref: "e1", CallerRef: "n1", CalleeRef: "n2"},
		"e2": {Ref: "e2", CallerRef: "n2", CalleeRef: "n3"},
	}
	mechanism, code := validateCandidate(
		Candidate{EdgeRefs: []string{"e2", "e1"}}, edges,
		[]Reading{{Ref: "r1", RootNodeRef: "n1"}, {Ref: "r2", RootNodeRef: "n3"}},
		nil,
	)
	if code != "" || !reflect.DeepEqual(mechanism.ReadingRefs, []string{"r1", "r2"}) ||
		!reflect.DeepEqual(mechanism.NodeRefs, []string{"n1", "n2", "n3"}) {
		t.Fatalf("derived mechanism = %+v code=%s", mechanism, code)
	}
	if _, code := validateCandidate(
		Candidate{EdgeRefs: []string{"e1", "e2"}}, edges,
		[]Reading{{Ref: "r3", RootNodeRef: "n9"}},
		nil,
	); code != IssueNoReadingTie {
		t.Fatalf("untied path code = %s, want %s", code, IssueNoReadingTie)
	}
}

func TestResolveResponseIsUnorderedCanonicalSet(t *testing.T) {
	compilation, batch := compileChainBatch(t)
	candidates := append(
		append([]Candidate(nil), chainCandidates(t, batch.Request.Cards[0])[:MaxMechanismsPerCard]...),
		Candidate{EdgeRefs: []string{"e9998", "e9999"}},
	)
	permuted := append([]Candidate(nil), candidates...)
	slices.Reverse(permuted)

	first := resolveResponseForTest(t, compilation, batch, candidates)
	second := resolveResponseForTest(t, compilation, batch, permuted)
	firstJSON := marshalJSON(t, first)
	secondJSON := marshalJSON(t, second)
	if !reflect.DeepEqual(firstJSON, secondJSON) {
		t.Fatalf("candidate order changed result bytes:\nfirst  %s\nsecond %s", firstJSON, secondJSON)
	}
	if len(first.Cards[0].Mechanisms) != MaxMechanismsPerCard {
		t.Fatalf("valid sibling mechanisms = %+v", first.Cards[0])
	}
	if len(first.Cards[0].Issues) != 1 || first.Cards[0].Issues[0].Code != IssueUnknownRef {
		t.Fatalf("invalid sibling issue = %+v", first.Cards[0].Issues)
	}
	for _, mechanism := range first.Cards[0].Mechanisms {
		assertMechanismFollowsExactEdges(t, batch.Request.Cards[0], mechanism)
	}
}

func TestResolveResponseOverfullValidSetClosesWholeCardWithoutFirstWinner(t *testing.T) {
	compilation, batch := compileChainBatch(t)
	candidates := chainCandidates(t, batch.Request.Cards[0])
	if len(candidates) != MaxMechanismsPerCard+1 {
		t.Fatalf("fixture candidates = %d, want %d", len(candidates), MaxMechanismsPerCard+1)
	}
	permuted := append([]Candidate(nil), candidates...)
	slices.Reverse(permuted)

	first := resolveResponseForTest(t, compilation, batch, candidates)
	second := resolveResponseForTest(t, compilation, batch, permuted)
	firstJSON := marshalJSON(t, first)
	secondJSON := marshalJSON(t, second)
	if !reflect.DeepEqual(firstJSON, secondJSON) {
		t.Fatalf("overfull set order changed result bytes:\nfirst  %s\nsecond %s", firstJSON, secondJSON)
	}
	got := first.Cards[0]
	if got.State != OutcomePrepared || len(got.Mechanisms) != 0 ||
		len(got.Issues) != 1 || got.Issues[0].Code != IssueOverBound ||
		countResultFrontier(got, FrontierResponseInvalid) != len(candidates) {
		t.Fatalf("overfull valid set selected a winner: %+v", got)
	}
}

func TestResolveResponseIgnoresEdgeRefOrderAndRejectsDuplicatePath(t *testing.T) {
	compilation, batch := compileChainBatch(t)
	card := batch.Request.Cards[0]
	first := chainCandidate(t, card, "entry", "service", "persist")
	second := Candidate{EdgeRefs: append([]string(nil), first.EdgeRefs...)}
	slices.Reverse(second.EdgeRefs)
	result := resolveResponseForTest(t, compilation, batch, []Candidate{first, second})
	got := result.Cards[0]
	if len(got.Mechanisms) != 1 || len(got.Issues) != 1 || got.Issues[0].Code != IssueDuplicatePath {
		t.Fatalf("duplicate path result = %+v", got)
	}
	expectedEdges := []string{
		requireEdgeByLabels(t, card, "entry", "service").Ref,
		requireEdgeByLabels(t, card, "service", "persist").Ref,
	}
	if !reflect.DeepEqual(got.Mechanisms[0].EdgeRefs, expectedEdges) {
		t.Fatalf("reconstructed edge order = %v, want %v", got.Mechanisms[0].EdgeRefs, expectedEdges)
	}
}

func TestResolveResponseRequiresTwoEdgesAndRejectsRemovedProseFields(t *testing.T) {
	compilation, batch := compileChainBatch(t)
	card := batch.Request.Cards[0]
	valid := chainCandidate(t, card, "entry", "service", "persist")
	oneHop := Candidate{EdgeRefs: valid.EdgeRefs[:1]}
	result := resolveResponseForTest(t, compilation, batch, []Candidate{oneHop})
	if result.Cards[0].State != OutcomePrepared || result.Cards[0].Issues[0].Code != IssueInvalidShape {
		t.Fatalf("one-hop response became mechanism: %+v", result.Cards[0])
	}

	legacy := strings.Replace(
		string(responseJSON(t, batch, []Candidate{valid})),
		`"edge_refs":`, `"explanation":"runtime overclaim","edge_refs":`, 1,
	)
	if _, err := ResolveResponse(compilation, batch, []byte(legacy)); err == nil {
		t.Fatal("strict v2 decoder accepted removed provider prose")
	}
}

func TestResolveResponseRejectsInventedEnvelopeAndTrailingJSONAtomically(t *testing.T) {
	compilation, batch := compileChainBatch(t)
	response := Response{
		Version: ResultVersion, CatalogRef: batch.Request.CatalogRef,
		CatalogSHA256: strings.Repeat("f", 64), RequestRef: batch.Request.RequestRef,
		Cards: []ResponseCard{},
	}
	if _, err := ResolveResponse(compilation, batch, marshalJSON(t, response)); err == nil {
		t.Fatal("mismatched catalog identity was accepted")
	}
	valid, err := MockResponse(batch)
	if err != nil {
		t.Fatalf("MockResponse: %v", err)
	}
	valid = append(valid, []byte(` {}`)...)
	if _, err := ResolveResponse(compilation, batch, valid); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
}

func compileChainBatch(t *testing.T) (*Compilation, RequestBatch) {
	t.Helper()
	index := buildChainIndex(t)
	root := requireNodeBySymbol(t, index, "example.com/mechanism.entry")
	otherRoot := requireNodeBySymbol(t, index, "example.com/mechanism.sideLeaf")
	compilation, err := CompileContexts([]ExactContext{{
		Label: "Startup", Question: "What work follows the entry?",
		Readings: []ExactReading{
			{Label: "Entry", Path: root.Declaration.Path, Line: root.Declaration.Line, Symbol: root.Symbol.ID},
			{Label: "Side leaf", Path: otherRoot.Declaration.Path, Line: otherRoot.Declaration.Line, Symbol: otherRoot.Symbol.ID},
		},
	}}, index, repositoryBinding())
	if err != nil {
		t.Fatalf("CompileContexts: %v", err)
	}
	batches, err := BuildRequestBatches(compilation)
	if err != nil || len(batches) != 1 {
		t.Fatalf("BuildRequestBatches: batches=%d err=%v", len(batches), err)
	}
	return compilation, batches[0]
}

func chainCandidate(t *testing.T, card Card, firstLabel, middleLabel, lastLabel string) Candidate {
	t.Helper()
	first := requireEdgeByLabels(t, card, firstLabel, middleLabel)
	second := requireEdgeByLabels(t, card, middleLabel, lastLabel)
	// Deliberately reverse wire order: it must never become graph order.
	return Candidate{EdgeRefs: []string{second.Ref, first.Ref}}
}

func chainCandidates(t *testing.T, card Card) []Candidate {
	t.Helper()
	return []Candidate{
		chainCandidate(t, card, "main", "entry", "service"),
		chainCandidate(t, card, "main", "entry", "side"),
		chainCandidate(t, card, "entry", "service", "persist"),
		chainCandidate(t, card, "entry", "side", "sideLeaf"),
	}
}

func responseJSON(t *testing.T, batch RequestBatch, candidates []Candidate) []byte {
	t.Helper()
	return marshalJSON(t, Response{
		Version: ResultVersion, CatalogRef: batch.Request.CatalogRef,
		CatalogSHA256: batch.Request.CatalogSHA256, RequestRef: batch.Request.RequestRef,
		Cards: []ResponseCard{{CardRef: batch.Request.Cards[0].Ref, Mechanisms: candidates}},
	})
}

func resolveResponseForTest(t *testing.T, compilation *Compilation, batch RequestBatch, candidates []Candidate) Result {
	t.Helper()
	result, err := ResolveResponse(compilation, batch, responseJSON(t, batch, candidates))
	if err != nil {
		t.Fatalf("ResolveResponse: %v", err)
	}
	return result
}

func assertMechanismFollowsExactEdges(t *testing.T, card Card, mechanism Mechanism) {
	t.Helper()
	for position, edgeRef := range mechanism.EdgeRefs {
		edge := requirePublicEdge(t, card, edgeRef)
		if edge.CallerRef != mechanism.NodeRefs[position] || edge.CalleeRef != mechanism.NodeRefs[position+1] {
			t.Fatalf("backend order does not follow exact endpoints: %+v", mechanism)
		}
	}
}

func requireEdgeByLabels(t *testing.T, card Card, callerLabel, calleeLabel string) Edge {
	t.Helper()
	nodeByLabel := make(map[string]string, len(card.Nodes))
	for _, node := range card.Nodes {
		nodeByLabel[node.Label] = node.Ref
		if separator := strings.LastIndex(node.Label, " · "); separator >= 0 {
			nodeByLabel[node.Label[separator+len(" · "):]] = node.Ref
		}
	}
	caller, callerOK := nodeByLabel[callerLabel]
	callee, calleeOK := nodeByLabel[calleeLabel]
	if !callerOK || !calleeOK {
		t.Fatalf("nodes %q/%q not found in %+v", callerLabel, calleeLabel, card.Nodes)
	}
	for _, edge := range card.Edges {
		if edge.CallerRef == caller && edge.CalleeRef == callee {
			return edge
		}
	}
	t.Fatalf("edge %s -> %s not found in %+v", callerLabel, calleeLabel, card.Edges)
	return Edge{}
}

func requirePublicEdge(t *testing.T, card Card, ref string) Edge {
	t.Helper()
	for _, edge := range card.Edges {
		if edge.Ref == ref {
			return edge
		}
	}
	t.Fatalf("edge %q not found", ref)
	return Edge{}
}

func countResultFrontier(card CardResult, reason FrontierReason) int {
	for _, item := range card.Frontier {
		if item.Reason == reason {
			return item.Count
		}
	}
	return 0
}

func decodeResponse(t *testing.T, raw []byte) Response {
	t.Helper()
	var response Response
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return response
}
