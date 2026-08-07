package themestudy

import (
	"strings"
	"testing"
)

// buildTestScoutRequest compiles a deterministic Scout request over a small
// synthetic substrate (names-only vocabulary + bounded seed packs).
func buildTestScoutRequest(t *testing.T) ScoutRequest {
	t.Helper()
	files := map[string][]string{
		"main.go":          {"package main", "func main() {", "  svc.Start()", "}"},
		"svc.go":           {"package svc", "func Start() {", "  go serve()", "}"},
		"routers/route.go": {"package routers", "func Handle() {", "  auth.Check()", "}"},
		"cred/argon2id.go": {"package cred", "func Hash() {", "  return", "}"},
	}
	reader, total := fakeSource(files)
	vocab := BuildFileVocabulary([]string{
		"main.go", "svc.go", "routers/route.go", "cred/argon2id.go", "README.md",
	}, 0, nil)
	seeds := []SeedSpec{
		{Ref: "a1", Path: "main.go", Line: 2, Symbol: "main", Provenance: "d211_span_reading_target", Kind: "focused", Role: RoleProductionSource},
		{Ref: "a2", Path: "svc.go", Line: 2, Symbol: "Start", Provenance: "d211_span_reading_target", Kind: "focused", Role: RoleProductionSource},
		{Ref: "a3", Path: "routers/route.go", Line: 2, Symbol: "Handle", Provenance: "surface", Kind: "focused", Role: RoleProductionSource},
		{Ref: "a4", Path: "cred/argon2id.go", Line: 2, Symbol: "Hash", Provenance: "d211_span_reading_target", Kind: "focused", Role: RoleProductionSource},
		{Ref: "a5", Path: "main.go", Line: 3, Symbol: "main", Kind: "system_path",
			CallerSymbol: "main", CallerLine: 2, CallLine: 3, CalleeSymbol: "Start", CalleeLine: 2,
			Provenance: "d211_span_reading_target", Role: RoleProductionSource, CanonicalSpanID: "span-1"},
	}
	packs, err := BuildSeedPacks(seeds, 0, 0, 0, 0, reader, total)
	if err != nil {
		t.Fatalf("BuildSeedPacks: %v", err)
	}
	request, err := CompileScout(LanguageEnglish, vocab, packs, ScoutContext{
		RepositoryName: "fixture",
		SpanQuestions: []ScoutSpanQuestion{
			{Kind: "system_path", Question: "How does the process entry reach Start?"},
			{Kind: "focused", Question: "What does Hash do?"},
		},
	}, "test-revision")
	if err != nil {
		t.Fatalf("CompileScout: %v", err)
	}
	return request
}

func TestCompileScoutRequestIsBoundedAndIdentified(t *testing.T) {
	request := buildTestScoutRequest(t)
	if request.Version != ScoutRequestVersion || request.PromptVersion != ScoutPromptVersion {
		t.Fatalf("bad identity: %+v", request)
	}
	if request.WireSHA256 == "" || request.CatalogSHA256 == "" || request.WireJSON == "" {
		t.Fatalf("missing digests or wire")
	}
	if !strings.Contains(request.WireJSON, "\"f1\"") || !strings.Contains(request.WireJSON, "\"a1\"") {
		t.Fatalf("wire must carry typed f*/a* refs")
	}
	if !strings.Contains(request.WireJSON, "func main") {
		t.Fatalf("wire must carry the bounded seed-pack source (provider evidence, contract B)")
	}
	if len(request.AnchorRefs()) != 5 || len(request.FileRefs()) != 5 {
		t.Fatalf("ref sets: anchors=%d files=%d", len(request.AnchorRefs()), len(request.FileRefs()))
	}
	encoded, err := EncodeScoutRequest(request)
	if err != nil {
		t.Fatalf("EncodeScoutRequest: %v", err)
	}
	decoded, err := DecodeScoutRequest(encoded)
	if err != nil {
		t.Fatalf("DecodeScoutRequest: %v", err)
	}
	if decoded.CatalogSHA256 != request.CatalogSHA256 || decoded.WireSHA256 != request.WireSHA256 {
		t.Fatalf("decoded identity does not round-trip")
	}
}

func TestMockAndReplayScoutRoundTrip(t *testing.T) {
	request := buildTestScoutRequest(t)
	mock, err := MockScoutResponse(request)
	if err != nil {
		t.Fatalf("MockScoutResponse: %v", err)
	}
	result, status, err := ReplayScoutResponse(request, mock)
	if err != nil {
		t.Fatalf("ReplayScoutResponse: %v", err)
	}
	if result.State != "accepted" || status.State != "accepted" {
		t.Fatalf("mock fixture must replay as accepted, got %q/%q", result.State, status.State)
	}
	if len(result.Candidates) < MinScoutCandidates {
		t.Fatalf("candidate count %d below valid minimum", len(result.Candidates))
	}
	if result.Status.Accepted != len(result.Candidates) {
		t.Fatalf("status accepted %d != candidates %d", result.Status.Accepted, len(result.Candidates))
	}
	for _, candidate := range result.Candidates {
		if candidate.Ref == "" || !strings.HasPrefix(candidate.Ref, "t") {
			t.Fatalf("candidate missing t* ref: %+v", candidate)
		}
		for _, anchor := range candidate.AnchorRefs {
			if _, ok := request.AnchorRefs()[anchor]; !ok {
				t.Fatalf("candidate anchors unknown a* ref %q", anchor)
			}
		}
	}
	// An unknown a* ref must reject item-locally: the offending candidate is
	// dropped while valid siblings survive as accepted_partial, never silently
	// accepted and never a whole-response failure.
	broken := strings.Replace(string(mock), "\"a1\"", "\"a99\"", 1)
	partialResult, partialStatus, replayErr := ReplayScoutResponse(request, []byte(broken))
	if replayErr != nil {
		t.Fatalf("item-local rejection must not fail the whole response: %v", replayErr)
	}
	if partialResult.State != "accepted_partial" || partialStatus.State != "accepted_partial" {
		t.Fatalf("expected accepted_partial after item-local rejection, got %q/%q",
			partialResult.State, partialStatus.State)
	}
	if partialResult.Status.Rejected != 1 {
		t.Fatalf("expected exactly 1 rejected candidate, got %d", partialResult.Status.Rejected)
	}
	for _, candidate := range partialResult.Candidates {
		for _, anchor := range candidate.AnchorRefs {
			if _, ok := request.AnchorRefs()[anchor]; !ok {
				t.Fatalf("surviving candidate carries unknown a* ref %q", anchor)
			}
		}
	}
}

func TestMockAndReplayAdjudicationRoundTrip(t *testing.T) {
	request := buildTestScoutRequest(t)
	scoutMock, err := MockScoutResponse(request)
	if err != nil {
		t.Fatalf("MockScoutResponse: %v", err)
	}
	scoutResult, _, err := ReplayScoutResponse(request, scoutMock)
	if err != nil {
		t.Fatalf("ReplayScoutResponse: %v", err)
	}
	// Local source expansion over the requested f* refs.
	expansionRefs := RefsForExpansion(scoutResult.Candidates)
	expansion, err := ExpandFiles(
		ExpansionFilesForRefs(request.Vocabulary, expansionRefs),
		func(path string, start, end int) ([]string, error) {
			return []string{"line1", "line2"}, nil
		},
		func(path string) (int, error) { return 2, nil },
	)
	if err != nil {
		t.Fatalf("ExpandFiles: %v", err)
	}
	anchors := anchorInfoFromPacks(request.SeedPacks)
	adjRequest, err := CompileAdjudication(LanguageEnglish, scoutResult.Candidates, expansion, anchors, request.SeedPacks.Packs)
	if err != nil {
		t.Fatalf("CompileAdjudication: %v", err)
	}
	adjMock, err := MockAdjudicationResponse(adjRequest)
	if err != nil {
		t.Fatalf("MockAdjudicationResponse: %v", err)
	}
	adjResult, adjStatus, err := ReplayAdjudicationResponse(adjRequest, adjMock)
	if err != nil {
		t.Fatalf("ReplayAdjudicationResponse: %v", err)
	}
	if adjResult.State != "accepted" || adjStatus.State != "accepted" {
		t.Fatalf("adjudication mock must replay as accepted, got %q/%q", adjResult.State, adjStatus.State)
	}
	if adjResult.Status.Accepted != len(adjResult.Themes) || adjResult.Status.Accepted == 0 {
		t.Fatalf("adjudication accepted count mismatch")
	}
	for _, theme := range adjResult.Themes {
		if _, ok := candidateByRef(scoutResult.Candidates)[theme.CandidateRef]; !ok {
			t.Fatalf("theme references unknown candidate %q", theme.CandidateRef)
		}
		hasDirect := false
		for _, assessment := range theme.AnchorAssessments {
			if assessment.Fit == FitDirect {
				hasDirect = true
			}
		}
		if !hasDirect {
			t.Fatalf("theme %q has no direct anchor", theme.CandidateRef)
		}
	}
	// Reducer publishes clean cards with zero source bytes.
	reduction, err := ReduceAccepted(scoutResult.Candidates, adjResult.Themes, anchors)
	if err != nil {
		t.Fatalf("ReduceAccepted: %v", err)
	}
	if len(reduction.Cards) == 0 {
		t.Fatalf("reducer published no cards")
	}
	encoded, err := EncodeStudyThemes(StudyThemes{
		// Decision 235 (v11): themes artifact v3.
		Version: "v3", ScoutSHA256: request.CatalogSHA256,
		AdjSHA256: adjRequest.CatalogSHA256, Cards: reduction.Cards,
		Omitted: reduction.Omitted, Partial: reduction.Partial, Diagnostics: reduction.Diagnostics,
	})
	if err != nil {
		t.Fatalf("EncodeStudyThemes: %v", err)
	}
	decoded, err := DecodeStudyThemes(encoded)
	if err != nil {
		t.Fatalf("DecodeStudyThemes: %v", err)
	}
	if len(decoded.Cards) != len(reduction.Cards) {
		t.Fatalf("study themes round-trip lost cards")
	}
	for _, card := range decoded.Cards {
		for _, reading := range card.Readings {
			if strings.Contains(reading.Symbol, "func ") || reading.Path == "" {
				t.Fatalf("card reading carries source bytes or no path: %+v", reading)
			}
		}
	}
}

func TestReplayScoutRefusesCorruptRequest(t *testing.T) {
	request := buildTestScoutRequest(t)
	mock, err := MockScoutResponse(request)
	if err != nil {
		t.Fatalf("MockScoutResponse: %v", err)
	}
	tampered := request
	tampered.CatalogSHA256 = "0"
	if _, _, err := ReplayScoutResponse(tampered, mock); err == nil {
		t.Fatalf("tampered request identity must reject")
	}
}

func TestCompileScoutRejectsEmptyLayers(t *testing.T) {
	vocab := BuildFileVocabulary([]string{"main.go"}, 0, nil)
	packs, err := BuildSeedPacks([]SeedSpec{
		{Ref: "a1", Path: "main.go", Line: 1, Symbol: "main", Provenance: "p", Kind: "focused", Role: RoleProductionSource},
	}, 0, 0, 0, 0, func(path string, s, e int) ([]string, error) { return []string{"x"}, nil }, func(path string) (int, error) { return 1, nil })
	if err != nil {
		t.Fatalf("BuildSeedPacks: %v", err)
	}
	if _, err := CompileScout(LanguageEnglish, vocab, packs, ScoutContext{}, ""); err != nil {
		t.Fatalf("valid compile must pass: %v", err)
	}
	if _, err := CompileScout(LanguageEnglish, Vocabulary{}, packs, ScoutContext{}, ""); err == nil {
		t.Fatalf("empty vocabulary must reject")
	}
	if _, err := CompileScout(LanguageEnglish, vocab, SeedPackResult{}, ScoutContext{}, ""); err == nil {
		t.Fatalf("empty seed packs must reject")
	}
}
