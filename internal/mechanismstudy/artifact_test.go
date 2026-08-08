package mechanismstudy

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/themestudy"
)

func TestPersistedArtifactFamilyRestoresSealedAuthorityAndPublication(t *testing.T) {
	compilation, _ := compileChainBatch(t)
	plan, err := PlanRequestBatches(compilation)
	if err != nil || len(plan.Batches) != 1 {
		t.Fatalf("PlanRequestBatches: batches=%d err=%v", len(plan.Batches), err)
	}
	factsRaw, err := EncodeFacts(compilation, plan)
	if err != nil {
		t.Fatalf("EncodeFacts: %v", err)
	}
	restored, err := DecodeFacts(factsRaw)
	if err != nil {
		t.Fatalf("DecodeFacts: %v", err)
	}
	if restored.SHA256 != sha256Hex(factsRaw) || restored.Compilation.CatalogSHA256 != compilation.CatalogSHA256 ||
		len(restored.Plan.Batches) != 1 {
		t.Fatalf("restored facts identity = %+v", restored)
	}
	if _, err := BuildPrompt(restored.Plan.Batches[0]); err != nil {
		t.Fatalf("restored batch lost private seal: %v", err)
	}
	reencoded, err := EncodeFacts(restored.Compilation, restored.Plan)
	if err != nil || !bytes.Equal(reencoded, factsRaw) {
		t.Fatalf("facts round trip changed canonical bytes: err=%v", err)
	}

	responseRaw, err := MockResponse(plan.Batches[0])
	if err != nil {
		t.Fatalf("MockResponse: %v", err)
	}
	candidate, err := ParseBatchCandidate(compilation, plan.Batches[0], responseRaw)
	if err != nil {
		t.Fatalf("ParseBatchCandidate: %v", err)
	}
	candidatesRaw, err := EncodeCandidates(factsRaw, []BatchCandidate{candidate})
	if err != nil {
		t.Fatalf("EncodeCandidates: %v", err)
	}
	resultRaw, err := EncodeResult(factsRaw, candidatesRaw)
	if err != nil {
		t.Fatalf("EncodeResult: %v", err)
	}
	result, err := DecodeResult(factsRaw, candidatesRaw, resultRaw)
	if err != nil {
		t.Fatalf("DecodeResult: %v", err)
	}
	publication, err := PublicationCards(restored.Compilation, result.Cards)
	if err != nil {
		t.Fatalf("PublicationCards: %v", err)
	}
	if len(publication) != 1 || publication[0].StudyOrdinal != 1 || publication[0].Outcome != OutcomeMechanism ||
		publication[0].StudyCanonicalID == "" || len(publication[0].Mechanisms) != 1 {
		t.Fatalf("publication card = %+v", publication)
	}
	mechanism := publication[0].Mechanisms[0]
	if len(mechanism.ReadingOrdinals) == 0 || len(mechanism.Nodes) != len(mechanism.Edges)+1 {
		t.Fatalf("publication mechanism = %+v", mechanism)
	}
	for position, edge := range mechanism.Edges {
		if edge.From != position+1 || edge.To != position+2 || !edge.Invocation.Valid() || edge.WitnessCount <= 0 ||
			!validPublicationLocation(edge.Callsite) || !validPublicationLocation(mechanism.Nodes[position].Declaration) {
			t.Fatalf("publication edge %d lost exact authority: %+v", position, edge)
		}
	}
	if encoded, err := json.Marshal(publication); err != nil || string(encoded) != "[{}]" {
		t.Fatalf("neutral publication seam leaked through JSON: %s err=%v", encoded, err)
	}

	statusRaw, err := EncodeStatus(factsRaw, candidatesRaw, resultRaw, StatusExecution{Batches: []BatchExecution{{
		RequestRef: plan.Batches[0].Request.RequestRef, RequestSHA256: plan.Batches[0].WireSHA256,
		State: BatchAccepted, ProviderCalls: 1, TransportAttempts: 2,
	}}})
	if err != nil {
		t.Fatalf("EncodeStatus: %v", err)
	}
	status, err := DecodeStatus(factsRaw, candidatesRaw, resultRaw, statusRaw)
	if err != nil {
		t.Fatalf("DecodeStatus: %v", err)
	}
	if status.State != StatusComplete || status.MechanismCardCount != 1 || status.PreparedCardCount != 0 ||
		status.AcceptedBatchCount != 1 || status.ProviderCallCount != 1 || status.TransportAttemptCount != 2 {
		t.Fatalf("status = %+v", status)
	}
}

func TestPublicationRetainsExactSourceThemeAndReadingOrdinals(t *testing.T) {
	index := buildChainIndex(t)
	root := requireNodeBySymbol(t, index, "example.com/mechanism.entry")
	study := themestudy.StudyThemes{
		Version: themestudy.StudyThemesVersion, Revision: fixtureRevision,
		Cards: []themestudy.ThemeCard{{
			Ordinal: 7, CanonicalID: "theme-seven", FinalTitle: "Startup",
			FinalQuestion: "What exact work follows startup?",
			Readings: []themestudy.Reading{
				{Label: "Supporting", Symbol: root.Symbol.ID, Path: root.Declaration.Path,
					Line: root.Declaration.Line, Fit: themestudy.FitSupporting},
				readingForNode(root, themestudy.FitDirect),
			},
		}},
	}
	compilation, err := Compile(study, index, studyBinding())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	plan, err := PlanRequestBatches(compilation)
	if err != nil || len(plan.Batches) != 1 {
		t.Fatalf("PlanRequestBatches: batches=%d err=%v", len(plan.Batches), err)
	}
	raw, _ := MockResponse(plan.Batches[0])
	candidate, err := ParseBatchCandidate(compilation, plan.Batches[0], raw)
	if err != nil {
		t.Fatalf("ParseBatchCandidate: %v", err)
	}
	results, err := AggregateResults(compilation, plan, []BatchCandidate{candidate})
	if err != nil {
		t.Fatalf("AggregateResults: %v", err)
	}
	publication, err := PublicationCards(compilation, results)
	if err != nil {
		t.Fatalf("PublicationCards: %v", err)
	}
	if len(publication) != 1 || publication[0].StudyOrdinal != 7 ||
		publication[0].StudyCanonicalID != "theme-seven" ||
		!reflect.DeepEqual(publication[0].ReadingOrdinals, []int{2}) ||
		!reflect.DeepEqual(publication[0].Mechanisms[0].ReadingOrdinals, []int{2}) {
		t.Fatalf("source ordinals were renumbered: %+v", publication)
	}
}

func TestCandidateAndAggregateReplayAreOrderIndependent(t *testing.T) {
	compilation, batch := compileChainBatch(t)
	plan, err := PlanRequestBatches(compilation)
	if err != nil {
		t.Fatalf("PlanRequestBatches: %v", err)
	}
	factsRaw, err := EncodeFacts(compilation, plan)
	if err != nil {
		t.Fatalf("EncodeFacts: %v", err)
	}
	candidates := chainCandidates(t, batch.Request.Cards[0])[:MaxMechanismsPerCard]
	first, err := ParseBatchCandidate(compilation, batch, responseJSON(t, batch, candidates))
	if err != nil {
		t.Fatalf("ParseBatchCandidate first: %v", err)
	}
	permuted := make([]Candidate, len(candidates))
	for position, candidate := range candidates {
		permuted[len(candidates)-1-position] = Candidate{EdgeRefs: append([]string(nil), candidate.EdgeRefs...)}
		slices.Reverse(permuted[len(candidates)-1-position].EdgeRefs)
	}
	second, err := ParseBatchCandidate(compilation, batch, responseJSON(t, batch, permuted))
	if err != nil {
		t.Fatalf("ParseBatchCandidate second: %v", err)
	}
	firstCandidates, err := EncodeCandidates(factsRaw, []BatchCandidate{first})
	if err != nil {
		t.Fatalf("EncodeCandidates first: %v", err)
	}
	secondCandidates, err := EncodeCandidates(factsRaw, []BatchCandidate{second})
	if err != nil {
		t.Fatalf("EncodeCandidates second: %v", err)
	}
	if !bytes.Equal(firstCandidates, secondCandidates) {
		t.Fatalf("unordered candidate set changed canonical artifact:\nfirst  %s\nsecond %s", firstCandidates, secondCandidates)
	}
	firstResult, err := EncodeResult(factsRaw, firstCandidates)
	if err != nil {
		t.Fatalf("EncodeResult first: %v", err)
	}
	secondResult, err := EncodeResult(factsRaw, secondCandidates)
	if err != nil {
		t.Fatalf("EncodeResult second: %v", err)
	}
	if !bytes.Equal(firstResult, secondResult) {
		t.Fatalf("unordered candidate set changed final result bytes")
	}
}

func TestAggregateAndCandidateArtifactsIgnoreBatchInputOrder(t *testing.T) {
	compilation, plan := compileManyContextCompilation(t)
	if len(plan.Batches) < 2 {
		t.Fatalf("fixture produced %d batches, want at least two", len(plan.Batches))
	}
	factsRaw, err := EncodeFacts(compilation, plan)
	if err != nil {
		t.Fatalf("EncodeFacts: %v", err)
	}
	ordered := make([]BatchCandidate, 0, len(plan.Batches))
	for _, batch := range plan.Batches {
		raw, err := MockResponse(batch)
		if err != nil {
			t.Fatalf("MockResponse %s: %v", batch.Request.RequestRef, err)
		}
		candidate, err := ParseBatchCandidate(compilation, batch, raw)
		if err != nil {
			t.Fatalf("ParseBatchCandidate %s: %v", batch.Request.RequestRef, err)
		}
		ordered = append(ordered, candidate)
	}
	reversed := append([]BatchCandidate(nil), ordered...)
	slices.Reverse(reversed)
	firstCards, err := AggregateResults(compilation, plan, ordered)
	if err != nil {
		t.Fatalf("AggregateResults ordered: %v", err)
	}
	secondCards, err := AggregateResults(compilation, plan, reversed)
	if err != nil {
		t.Fatalf("AggregateResults reversed: %v", err)
	}
	if !reflect.DeepEqual(firstCards, secondCards) {
		t.Fatalf("batch order changed aggregate cards")
	}
	firstArtifact, err := EncodeCandidates(factsRaw, ordered)
	if err != nil {
		t.Fatalf("EncodeCandidates ordered: %v", err)
	}
	secondArtifact, err := EncodeCandidates(factsRaw, reversed)
	if err != nil {
		t.Fatalf("EncodeCandidates reversed: %v", err)
	}
	if !bytes.Equal(firstArtifact, secondArtifact) {
		t.Fatal("batch order changed canonical candidates artifact")
	}
}

func TestPlannerRetainsPreparedSuffixAtCallCeiling(t *testing.T) {
	index := buildChainIndex(t)
	root := requireNodeBySymbol(t, index, "example.com/mechanism.entry")
	contexts := make([]ExactContext, MaxCards)
	for position := range contexts {
		contexts[position] = ExactContext{
			Label: "Context " + string(rune('A'+position)), Question: "What work follows this exact entry?",
			Readings: []ExactReading{{
				Label: "Entry", Path: root.Declaration.Path, Line: root.Declaration.Line, Symbol: root.Symbol.ID,
			}},
		}
	}
	compilation, err := CompileContexts(contexts, index, repositoryBinding())
	if err != nil {
		t.Fatalf("CompileContexts: %v", err)
	}
	limited, err := planRequestBatchesWithCallLimit(compilation, 1)
	if err != nil {
		t.Fatalf("planRequestBatchesWithCallLimit: %v", err)
	}
	if len(limited.Batches) != 1 || len(limited.UnrequestedCardRefs) == 0 ||
		!sortedUniqueTypedRefs(limited.UnrequestedCardRefs, 't') {
		t.Fatalf("limited plan lost prepared suffix: batches=%d suffix=%v", len(limited.Batches), limited.UnrequestedCardRefs)
	}
	plannedCards := len(limited.Batches[0].Request.Cards) + len(limited.UnrequestedCardRefs)
	if plannedCards != len(compilation.Cards) {
		t.Fatalf("plan accounted for %d/%d eligible cards", plannedCards, len(compilation.Cards))
	}
	production, err := PlanRequestBatches(compilation)
	if err != nil || len(production.Batches) > MaxProviderCalls {
		t.Fatalf("production plan: batches=%d err=%v", len(production.Batches), err)
	}
}

func TestStatusClosesPartialFailureUntouchedSuffixAndCancellationAccounting(t *testing.T) {
	compilation, _ := compileManyContextCompilation(t)
	plan, err := PlanRequestBatches(compilation)
	if err != nil || len(plan.Batches) < 2 {
		t.Fatalf("multi-batch plan: batches=%d err=%v", len(plan.Batches), err)
	}
	factsRaw, err := EncodeFacts(compilation, plan)
	if err != nil {
		t.Fatalf("EncodeFacts: %v", err)
	}
	response, err := MockResponse(plan.Batches[0])
	if err != nil {
		t.Fatalf("MockResponse: %v", err)
	}
	candidate, err := ParseBatchCandidate(compilation, plan.Batches[0], response)
	if err != nil {
		t.Fatalf("ParseBatchCandidate: %v", err)
	}
	candidatesRaw, err := EncodeCandidates(factsRaw, []BatchCandidate{candidate})
	if err != nil {
		t.Fatalf("EncodeCandidates: %v", err)
	}
	resultRaw, err := EncodeResult(factsRaw, candidatesRaw)
	if err != nil {
		t.Fatalf("EncodeResult: %v", err)
	}
	execution := StatusExecution{Batches: []BatchExecution{
		{RequestRef: plan.Batches[0].Request.RequestRef, RequestSHA256: plan.Batches[0].WireSHA256,
			State: BatchAccepted, ProviderCalls: 1, TransportAttempts: 1},
		{RequestRef: plan.Batches[1].Request.RequestRef, RequestSHA256: plan.Batches[1].WireSHA256,
			State: BatchCanceled, ProviderCalls: 1, TransportAttempts: 0},
	}}
	status, err := BuildStatus(factsRaw, candidatesRaw, resultRaw, execution)
	if err != nil {
		t.Fatalf("BuildStatus: %v", err)
	}
	if status.State != StatusPartial || status.AcceptedBatchCount != 1 || status.FailedBatchCount != 1 ||
		status.ProviderCallCount != 2 || status.TransportAttemptCount != 1 ||
		status.UnattemptedBatchCount != len(plan.Batches)-2 {
		t.Fatalf("partial status = %+v", status)
	}
	if _, err := BuildStatus(factsRaw, candidatesRaw, resultRaw, StatusExecution{Batches: execution.Batches[:1]}); err == nil {
		t.Fatal("unclosed attempted prefix was accepted")
	}
	emptyCandidatesRaw, err := EncodeCandidates(factsRaw, nil)
	if err != nil {
		t.Fatalf("EncodeCandidates empty: %v", err)
	}
	emptyResultRaw, err := EncodeResult(factsRaw, emptyCandidatesRaw)
	if err != nil {
		t.Fatalf("EncodeResult empty: %v", err)
	}
	failed, err := BuildStatus(factsRaw, emptyCandidatesRaw, emptyResultRaw, StatusExecution{Batches: []BatchExecution{{
		RequestRef: plan.Batches[0].Request.RequestRef, RequestSHA256: plan.Batches[0].WireSHA256,
		State: BatchConfigurationFailed, ProviderCalls: 0, TransportAttempts: 0,
	}}})
	if err != nil {
		t.Fatalf("BuildStatus first-batch failure: %v", err)
	}
	if failed.State != StatusFailed || failed.FailedBatchCount != 1 ||
		failed.UnattemptedBatchCount != len(plan.Batches)-1 {
		t.Fatalf("failed status = %+v", failed)
	}
}

func TestStatusRejectsStateSpecificBatchAccountingContradictions(t *testing.T) {
	compilation, _ := compileChainBatch(t)
	plan, err := PlanRequestBatches(compilation)
	if err != nil || len(plan.Batches) != 1 {
		t.Fatalf("PlanRequestBatches: batches=%d err=%v", len(plan.Batches), err)
	}
	factsRaw, err := EncodeFacts(compilation, plan)
	if err != nil {
		t.Fatalf("EncodeFacts: %v", err)
	}
	responseRaw, err := MockResponse(plan.Batches[0])
	if err != nil {
		t.Fatalf("MockResponse: %v", err)
	}
	candidate, err := ParseBatchCandidate(compilation, plan.Batches[0], responseRaw)
	if err != nil {
		t.Fatalf("ParseBatchCandidate: %v", err)
	}
	acceptedCandidatesRaw, err := EncodeCandidates(factsRaw, []BatchCandidate{candidate})
	if err != nil {
		t.Fatalf("EncodeCandidates accepted: %v", err)
	}
	acceptedResultRaw, err := EncodeResult(factsRaw, acceptedCandidatesRaw)
	if err != nil {
		t.Fatalf("EncodeResult accepted: %v", err)
	}
	emptyCandidatesRaw, err := EncodeCandidates(factsRaw, nil)
	if err != nil {
		t.Fatalf("EncodeCandidates empty: %v", err)
	}
	emptyResultRaw, err := EncodeResult(factsRaw, emptyCandidatesRaw)
	if err != nil {
		t.Fatalf("EncodeResult empty: %v", err)
	}

	tests := []struct {
		name               string
		state              BatchExecutionState
		providerCalls      int
		transportAttempts  int
		acceptedCandidates bool
		wantError          bool
	}{
		{name: "accepted", state: BatchAccepted, providerCalls: 1, transportAttempts: 1, acceptedCandidates: true},
		{name: "accepted without transport", state: BatchAccepted, providerCalls: 1, acceptedCandidates: true, wantError: true},
		{name: "accepted without provider call", state: BatchAccepted, transportAttempts: 1, acceptedCandidates: true, wantError: true},
		{name: "response invalid", state: BatchResponseInvalid, providerCalls: 1, transportAttempts: 1},
		{name: "response invalid without transport", state: BatchResponseInvalid, providerCalls: 1, wantError: true},
		{name: "output limit", state: BatchOutputLimit, providerCalls: 1, transportAttempts: 1},
		{name: "output limit without provider call", state: BatchOutputLimit, transportAttempts: 1, wantError: true},
		{name: "provider failed", state: BatchProviderFailed, providerCalls: 1, transportAttempts: 1},
		{name: "provider failed without transport", state: BatchProviderFailed, providerCalls: 1, wantError: true},
		{name: "configuration failed", state: BatchConfigurationFailed},
		{name: "configuration failure after provider call", state: BatchConfigurationFailed, providerCalls: 1, transportAttempts: 1, wantError: true},
		{name: "pre-call canceled", state: BatchCanceled},
		{name: "started call canceled before transport", state: BatchCanceled, providerCalls: 1},
		{name: "started call canceled after transport", state: BatchCanceled, providerCalls: 1, transportAttempts: 1},
		{name: "pre-call cancellation with transport", state: BatchCanceled, transportAttempts: 1, wantError: true},
		{name: "too many provider calls", state: BatchCanceled, providerCalls: 2, wantError: true},
		{name: "too many transport attempts", state: BatchProviderFailed, providerCalls: 1, transportAttempts: 9, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidatesRaw, resultRaw := emptyCandidatesRaw, emptyResultRaw
			if test.acceptedCandidates {
				candidatesRaw, resultRaw = acceptedCandidatesRaw, acceptedResultRaw
			}
			_, err := BuildStatus(factsRaw, candidatesRaw, resultRaw, StatusExecution{Batches: []BatchExecution{{
				RequestRef:        plan.Batches[0].Request.RequestRef,
				RequestSHA256:     plan.Batches[0].WireSHA256,
				State:             test.state,
				ProviderCalls:     test.providerCalls,
				TransportAttempts: test.transportAttempts,
			}}})
			if (err != nil) != test.wantError {
				t.Fatalf("BuildStatus error = %v, wantError=%v", err, test.wantError)
			}
		})
	}
}

func TestZeroEligiblePlanBuildsCompletePreparedFamily(t *testing.T) {
	index := buildChainIndex(t)
	root := requireNodeBySymbol(t, index, "example.com/mechanism.entry")
	compilation, err := CompileContexts([]ExactContext{{
		Label: "Unresolved", Question: "What can be prepared?",
		Readings: []ExactReading{{Label: "Wrong", Path: root.Declaration.Path, Line: root.Declaration.Line, Symbol: "wrong.symbol"}},
	}}, index, repositoryBinding())
	if err != nil {
		t.Fatalf("CompileContexts: %v", err)
	}
	plan, err := PlanRequestBatches(compilation)
	if err != nil || len(plan.Batches) != 0 {
		t.Fatalf("zero eligible plan = %+v err=%v", plan, err)
	}
	factsRaw, err := EncodeFacts(compilation, plan)
	if err != nil {
		t.Fatalf("EncodeFacts: %v", err)
	}
	candidatesRaw, err := EncodeCandidates(factsRaw, nil)
	if err != nil {
		t.Fatalf("EncodeCandidates: %v", err)
	}
	resultRaw, err := EncodeResult(factsRaw, candidatesRaw)
	if err != nil {
		t.Fatalf("EncodeResult: %v", err)
	}
	status, err := BuildStatus(factsRaw, candidatesRaw, resultRaw, StatusExecution{})
	if err != nil {
		t.Fatalf("BuildStatus: %v", err)
	}
	if status.State != StatusComplete || status.PreparedCardCount != 1 || status.ProviderCallCount != 0 ||
		status.PlannedBatchCount != 0 {
		t.Fatalf("zero-call status = %+v", status)
	}
}

func TestArtifactsRejectCanonicalDigestResultCredentialAndSizeTamper(t *testing.T) {
	compilation, _ := compileChainBatch(t)
	plan, _ := PlanRequestBatches(compilation)
	factsRaw, err := EncodeFacts(compilation, plan)
	if err != nil {
		t.Fatalf("EncodeFacts: %v", err)
	}
	response, _ := MockResponse(plan.Batches[0])
	candidate, err := ParseBatchCandidate(compilation, plan.Batches[0], response)
	if err != nil {
		t.Fatalf("ParseBatchCandidate: %v", err)
	}
	candidatesRaw, _ := EncodeCandidates(factsRaw, []BatchCandidate{candidate})
	resultRaw, _ := EncodeResult(factsRaw, candidatesRaw)
	statusRaw, _ := EncodeStatus(factsRaw, candidatesRaw, resultRaw, StatusExecution{Batches: []BatchExecution{{
		RequestRef: plan.Batches[0].Request.RequestRef, RequestSHA256: plan.Batches[0].WireSHA256,
		State: BatchAccepted, ProviderCalls: 1, TransportAttempts: 1,
	}}})

	var facts factsArtifact
	json.Unmarshal(factsRaw, &facts)
	facts.RequestBatches[0].WireSHA256 = strings.Repeat("f", 64)
	if _, err := DecodeFacts(marshalJSON(t, facts)); err == nil {
		t.Fatal("facts request digest tamper was accepted")
	}
	if _, err := DecodeFacts(addUnknownField(factsRaw)); err == nil {
		t.Fatal("facts unknown field was accepted")
	}
	if _, err := DecodeFacts(append(append([]byte(nil), factsRaw...), '\n')); err == nil {
		t.Fatal("noncanonical facts were accepted")
	}
	json.Unmarshal(factsRaw, &facts)
	facts.Version++
	if _, err := DecodeFacts(marshalJSON(t, facts)); err == nil {
		t.Fatal("facts version tamper was accepted")
	}
	json.Unmarshal(factsRaw, &facts)
	facts.RequestBatches[0].RequestRef = "q999"
	if _, err := DecodeFacts(marshalJSON(t, facts)); err == nil {
		t.Fatal("facts request ref tamper was accepted")
	}

	var candidates candidatesArtifact
	json.Unmarshal(candidatesRaw, &candidates)
	candidates.Batches[0].ResponseSHA256 = strings.Repeat("e", 64)
	if _, err := DecodeCandidates(factsRaw, marshalJSON(t, candidates)); err == nil {
		t.Fatal("candidate response digest tamper was accepted")
	}
	json.Unmarshal(candidatesRaw, &candidates)
	candidates.Version++
	if _, err := DecodeCandidates(factsRaw, marshalJSON(t, candidates)); err == nil {
		t.Fatal("candidates version tamper was accepted")
	}
	json.Unmarshal(candidatesRaw, &candidates)
	candidates.Batches[0].RequestRef = "q999"
	if _, err := DecodeCandidates(factsRaw, marshalJSON(t, candidates)); err == nil {
		t.Fatal("candidates request ref tamper was accepted")
	}
	if _, err := DecodeCandidates(factsRaw, addUnknownField(candidatesRaw)); err == nil {
		t.Fatal("candidates unknown field was accepted")
	}
	var result resultArtifact
	json.Unmarshal(resultRaw, &result)
	result.Cards[0].State = OutcomePrepared
	if _, err := DecodeResult(factsRaw, candidatesRaw, marshalJSON(t, result)); err == nil {
		t.Fatal("non-replayed result tamper was accepted")
	}
	json.Unmarshal(resultRaw, &result)
	result.Version++
	if _, err := DecodeResult(factsRaw, candidatesRaw, marshalJSON(t, result)); err == nil {
		t.Fatal("result version tamper was accepted")
	}
	json.Unmarshal(resultRaw, &result)
	result.Cards[0].CardRef = "t999"
	if _, err := DecodeResult(factsRaw, candidatesRaw, marshalJSON(t, result)); err == nil {
		t.Fatal("result card ref tamper was accepted")
	}
	if _, err := DecodeResult(factsRaw, candidatesRaw, addUnknownField(resultRaw)); err == nil {
		t.Fatal("result unknown field was accepted")
	}
	var status Status
	json.Unmarshal(statusRaw, &status)
	status.PreparedCardCount++
	if _, err := DecodeStatus(factsRaw, candidatesRaw, resultRaw, marshalJSON(t, status)); err == nil {
		t.Fatal("derived status count tamper was accepted")
	}
	json.Unmarshal(statusRaw, &status)
	status.Version++
	if _, err := DecodeStatus(factsRaw, candidatesRaw, resultRaw, marshalJSON(t, status)); err == nil {
		t.Fatal("status version tamper was accepted")
	}
	json.Unmarshal(statusRaw, &status)
	status.Batches[0].RequestRef = "q999"
	if _, err := DecodeStatus(factsRaw, candidatesRaw, resultRaw, marshalJSON(t, status)); err == nil {
		t.Fatal("status batch ref tamper was accepted")
	}
	if _, err := DecodeStatus(factsRaw, candidatesRaw, resultRaw, addUnknownField(statusRaw)); err == nil {
		t.Fatal("status unknown field was accepted")
	}

	secret := "sk-1234567890abcdef1234567890abcdef"
	secretArtifact := []byte(`{"api_key":"` + secret + `"}`)
	credentialTests := []struct {
		name string
		call func() error
	}{
		{"facts", func() error { _, err := DecodeFacts(secretArtifact); return err }},
		{"candidates", func() error { _, err := DecodeCandidates(factsRaw, secretArtifact); return err }},
		{"result", func() error { _, err := DecodeResult(factsRaw, candidatesRaw, secretArtifact); return err }},
		{"status", func() error { _, err := DecodeStatus(factsRaw, candidatesRaw, resultRaw, secretArtifact); return err }},
	}
	for _, test := range credentialTests {
		t.Run(test.name+" credential", func(t *testing.T) {
			err := test.call()
			if err == nil || strings.Contains(err.Error(), secret) {
				t.Fatalf("credential-safe rejection = %v", err)
			}
		})
	}
	sizeTests := []struct {
		name string
		size int
		call func([]byte) error
	}{
		{"facts", MaxFactsArtifactBytes + 1, func(raw []byte) error { _, err := DecodeFacts(raw); return err }},
		{"candidates", MaxCandidatesArtifactBytes + 1, func(raw []byte) error { _, err := DecodeCandidates(factsRaw, raw); return err }},
		{"result", MaxResultArtifactBytes + 1, func(raw []byte) error { _, err := DecodeResult(factsRaw, candidatesRaw, raw); return err }},
		{"status", MaxStatusArtifactBytes + 1, func(raw []byte) error { _, err := DecodeStatus(factsRaw, candidatesRaw, resultRaw, raw); return err }},
	}
	for _, test := range sizeTests {
		t.Run(test.name+" size", func(t *testing.T) {
			if err := test.call(bytes.Repeat([]byte{'x'}, test.size)); err == nil {
				t.Fatal("oversized artifact was accepted")
			}
		})
	}
}

func compileManyContextCompilation(t *testing.T) (*Compilation, RequestPlan) {
	t.Helper()
	index := buildChainIndex(t)
	root := requireNodeBySymbol(t, index, "example.com/mechanism.entry")
	contexts := make([]ExactContext, MaxCards)
	for position := range contexts {
		contexts[position] = ExactContext{
			Label: "Context " + string(rune('A'+position)), Question: "What exact work follows?",
			Readings: []ExactReading{{Label: "Entry", Path: root.Declaration.Path, Line: root.Declaration.Line, Symbol: root.Symbol.ID}},
		}
	}
	compilation, err := CompileContexts(contexts, index, repositoryBinding())
	if err != nil {
		t.Fatalf("CompileContexts: %v", err)
	}
	plan, err := PlanRequestBatches(compilation)
	if err != nil {
		t.Fatalf("PlanRequestBatches: %v", err)
	}
	return compilation, plan
}

func addUnknownField(raw []byte) []byte {
	return append([]byte(`{"unexpected":true,`), raw[1:]...)
}

func TestArtifactFilenamesAndLimitsArePinned(t *testing.T) {
	want := []string{
		"study_investigation_facts.v1.json",
		"study_investigation_candidates.v1.json",
		"study_investigation_result.v1.json",
		"study_investigation_status.v1.json",
	}
	if !reflect.DeepEqual(ArtifactFilenames, want) || MaxFactsArtifactBytes != 2<<20 ||
		MaxCandidatesArtifactBytes != 320<<10 || MaxResultArtifactBytes != 256<<10 ||
		MaxStatusArtifactBytes != 64<<10 {
		t.Fatalf("artifact identity drift: files=%v", ArtifactFilenames)
	}
}
