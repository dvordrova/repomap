package themestudy

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// fakeSource builds a SourceReader + TotalLines over an in-memory file map.
func fakeSource(files map[string][]string) (SourceReader, TotalLines) {
	reader := func(path string, start, end int) ([]string, error) {
		content := files[path]
		if start < 1 {
			return nil, nil
		}
		if end > len(content) {
			end = len(content)
		}
		if start > end {
			return nil, nil
		}
		return append([]string(nil), content[start-1:end]...), nil
	}
	total := func(path string) (int, error) { return len(files[path]), nil }
	return reader, total
}

func TestBuildFileVocabularyComplete(t *testing.T) {
	paths := []string{"main.go", "routers/router.go", "cred/argon2id.go", "README.md"}
	vocab := BuildFileVocabulary(paths, 0, nil)
	if !vocab.Complete {
		t.Fatalf("small list should be complete")
	}
	if vocab.Considered != 4 || vocab.Advertised != 4 {
		t.Fatalf("considered/advertised = %d/%d, want 4/4", vocab.Considered, vocab.Advertised)
	}
	seen := map[string]bool{}
	for _, f := range vocab.Files {
		if strings.HasPrefix(f.Ref, "f") == false || len(f.Ref) < 2 {
			t.Fatalf("bad ref %q", f.Ref)
		}
		if seen[f.Path] {
			t.Fatalf("duplicate path %q in vocabulary", f.Path)
		}
		seen[f.Path] = true
		if !f.Role.Valid() {
			t.Fatalf("bad role %q for %s", f.Role, f.Path)
		}
	}
	if vocab.CandidateSHA256 == "" {
		t.Fatalf("missing candidate digest")
	}
}

func TestBuildFileVocabularyBudgetTruncation(t *testing.T) {
	paths := []string{"a.go", "b.go", "c.go", "d.go", "e.go"}
	vocab := BuildFileVocabulary(paths, 12, nil) // force budget truncation
	if vocab.Complete {
		t.Fatalf("expected incomplete vocabulary under tight budget")
	}
	if vocab.Considered != 5 {
		t.Fatalf("considered = %d, want 5", vocab.Considered)
	}
	omitted := 0
	hasBudget := false
	for _, o := range vocab.Omissions {
		if o.Reason == "vocabulary_budget" {
			hasBudget = true
			omitted = o.Count
		}
	}
	if !hasBudget {
		t.Fatalf("expected vocabulary_budget omission")
	}
	if vocab.Advertised+omitted != vocab.Considered {
		t.Fatalf("advertised(%d)+omitted(%d) != considered(%d)", vocab.Advertised, omitted, vocab.Considered)
	}
}

func TestBuildFileVocabularyNotAdvertisable(t *testing.T) {
	paths := []string{"a.go", "secret.env"}
	vocab := BuildFileVocabulary(paths, 0, func(p string) bool { return p != "secret.env" })
	for _, o := range vocab.Omissions {
		if o.Reason == "eligible_not_advertisable" && o.Count != 1 {
			t.Fatalf("expected 1 not-advertisable omission, got %d", o.Count)
		}
	}
}

func TestSeedPacksFocusedAndSystemPath(t *testing.T) {
	files := map[string][]string{
		"main.go": {"package main", "func main() {", "  svc.Start()", "}"},
		"svc.go":  {"package svc", "func Start() {", "  go serve()", "}"},
	}
	reader, total := fakeSource(files)
	seeds := []SeedSpec{
		{Ref: "a1", Path: "svc.go", Line: 2, Symbol: "Start", Provenance: "d211_span", Kind: "focused", Role: RoleProductionSource},
		{Ref: "a2", Path: "main.go", Line: 3, Symbol: "main", Kind: "system_path",
			CallerSymbol: "main", CallerLine: 1, CallLine: 3, CalleeSymbol: "Start", CalleeLine: 2,
			Provenance: "d211_span", Role: RoleProductionSource},
	}
	res, err := BuildSeedPacks(seeds, 0, 0, 0, 0, reader, total)
	if err != nil {
		t.Fatalf("BuildSeedPacks: %v", err)
	}
	if len(res.Packs) != 2 {
		t.Fatalf("want 2 packs, got %d", len(res.Packs))
	}
	if len(res.Packs[0].Objects) != 1 || res.Packs[0].Objects[0].Role != SourceRoleDeclaration {
		t.Fatalf("focused seed must emit 1 declaration object")
	}
	if len(res.Packs[1].Objects) != 3 {
		t.Fatalf("system_path seed must emit 3 objects, got %d", len(res.Packs[1].Objects))
	}
	roles := map[SourceRole]int{}
	for _, o := range res.Packs[1].Objects {
		roles[o.Role]++
	}
	if roles[SourceRoleCaller] != 1 || roles[SourceRoleCallsite] != 1 || roles[SourceRoleCallee] != 1 {
		t.Fatalf("want caller/callsite/callee each 1, got %v", roles)
	}
	if res.Packs[1].Objects[1].Line != 3 {
		t.Fatalf("callsite must be at exact call line 3, got %d", res.Packs[1].Objects[1].Line)
	}
	for _, pack := range res.Packs {
		for _, o := range pack.Objects {
			if o.ContentSHA256 == "" {
				t.Fatalf("missing content sha on object")
			}
		}
	}
}

func TestSeedPackPartialAndOmittedRanges(t *testing.T) {
	long := []string{}
	for i := 0; i < 500; i++ {
		long = append(long, "func f() {}")
	}
	files := map[string][]string{"big.go": long}
	reader, total := fakeSource(files)
	seeds := []SeedSpec{{Ref: "a1", Path: "big.go", Line: 1, Symbol: "f", Kind: "focused", Role: RoleProductionSource}}
	maxObjectLines := 100
	res, err := BuildSeedPacks(seeds, 0, 0, maxObjectLines, MaxSourceObjectBytes, reader, total)
	if err != nil {
		t.Fatalf("BuildSeedPacks: %v", err)
	}
	obj := res.Packs[0].Objects[0]
	if !obj.Partial || obj.FullBody {
		t.Fatalf("large file must be partial & not full body")
	}
	if len(obj.Omitted) == 0 || obj.Omitted[0].StartLine != maxObjectLines+1 {
		t.Fatalf("expected explicit omitted range starting at %d", maxObjectLines+1)
	}
	if len(obj.Lines) != maxObjectLines {
		t.Fatalf("bounded object must cap at %d lines", maxObjectLines)
	}
}

func TestScoutItemLocalRejection(t *testing.T) {
	anchorRefs := map[string]struct{}{"a1": {}, "a2": {}, "a3": {}}
	fileRefs := map[string]struct{}{"f1": {}}
	good := `{"themes":[{"title":"Validation & hashing","question":"How do validation and hashing support signup?","theme_kind":"user_journey","anchor_refs":["a1","a2"],"why_it_matters":"joins signup with handling","expected_learning":"locate validation","relation_claim":"editorial_only"}]}`
	accepted, status, err := ValidateScout([]byte(good), anchorRefs, fileRefs, "d")
	if err != nil || status.Accepted != 1 || len(accepted) != 1 || status.State != "accepted" {
		t.Fatalf("good scout rejected or wrong state: accepted=%d state=%s err=%v", status.Accepted, status.State, err)
	}

	// Phase 3 validation audit: relation_claim is backend-owned — the
	// design rule says a model may never create runtime facts, so its
	// value is ALWAYS editorial_only and we assign it ourselves. A stray
	// value in this unrequested field must NOT poison a sibling (and must
	// not reject the theme itself): it is ignored and overwritten.
	mixed := `{"themes":[` +
		`{"title":"t1","question":"q1?","theme_kind":"user_journey","anchor_refs":["a1"],"why_it_matters":"w","expected_learning":"l","relation_claim":"runtime_proven"},` +
		`{"title":"t2","question":"q2?","theme_kind":"shared_domain_responsibility","anchor_refs":["a2","a3"],"why_it_matters":"w","expected_learning":"l","relation_claim":"editorial_only"}` +
		`]}`
	accepted, status, err = ValidateScout([]byte(mixed), anchorRefs, fileRefs, "d")
	if err != nil {
		t.Fatalf("mixed scout err: %v", err)
	}
	if status.Accepted != 2 || status.Rejected != 0 || len(accepted) != 2 || status.State != "accepted" {
		t.Fatalf("stray relation_claim must not reject: want 2 accepted / 0 rejected / accepted, got %d/%d/%s", status.Accepted, status.Rejected, status.State)
	}
	for _, issue := range status.Issues {
		if issue.Code == ScoutIssueInvalidRelationClaim {
			t.Fatalf("relation_claim must never produce an issue (backend-owned)")
		}
	}
	if accepted[0].RelationClaim != RelationClaimEditorialOnly || accepted[1].RelationClaim != RelationClaimEditorialOnly {
		t.Fatalf("relation_claim must be assigned editorial_only by the backend, got %q and %q", accepted[0].RelationClaim, accepted[1].RelationClaim)
	}

	// wrong-kind f* in anchor_refs → item-local failure of the whole candidate set.
	wrongkind := `{"themes":[{"title":"t","question":"q?","theme_kind":"user_journey","anchor_refs":["f1"],"why_it_matters":"w","expected_learning":"l","relation_claim":"editorial_only"}]}`
	_, status, err = ValidateScout([]byte(wrongkind), anchorRefs, fileRefs, "d")
	if err != nil || status.Accepted != 0 || status.State != "failed" {
		t.Fatalf("wrong-kind should fail closed: accepted=%d state=%s err=%v", status.Accepted, status.State, err)
	}

	// Unknown top-level key → unrequested output error.
	unreq := `{"themes":[],"extra":1}`
	if _, _, err := ValidateScout([]byte(unreq), anchorRefs, fileRefs, "d"); err == nil {
		t.Fatalf("expected unrequested-output error")
	}
}

func TestAdjudicationItemLocalRejection(t *testing.T) {
	candidates := map[string]*ScoutCandidate{
		"t1": {Title: "t1", Question: "q1?", ThemeKind: KindUserJourney, AnchorRefs: []string{"a1", "a2", "a3"}, WhyItMatters: "w", ExpectedLearning: "l", RelationClaim: RelationClaimEditorialOnly},
		"t2": {Title: "t2", Question: "q2?", ThemeKind: KindSharedDomainResponsibility, AnchorRefs: []string{"a2", "a3"}, WhyItMatters: "w", ExpectedLearning: "l", RelationClaim: RelationClaimEditorialOnly},
	}
	good := `{"themes":[{"candidate_ref":"t1","final_title":"validated signup","final_question":"how does signup validate?","anchor_assessments":[{"anchor_ref":"a1","fit":"direct","supported_observation":"signup coordinates account creation"},{"anchor_ref":"a2","fit":"supporting","supported_observation":"validation filter guards input"}],"reading_order":["a1","a2"],"unknowns":["no runtime order proven"]}]}`
	accepted, status, err := ValidateAdjudication([]byte(good), candidates)
	if err != nil || status.Accepted != 1 || len(accepted) != 1 {
		t.Fatalf("good adjudication rejected: accepted=%d err=%v", status.Accepted, err)
	}

	// reading_order containing an unknown ref → theme rejected item-locally.
	badOrder := `{"themes":[{"candidate_ref":"t1","final_title":"x","final_question":"y?","anchor_assessments":[{"anchor_ref":"a1","fit":"direct","supported_observation":"o"}],"reading_order":["a99"]}]}`
	_, status, err = ValidateAdjudication([]byte(badOrder), candidates)
	if err != nil || status.Accepted != 0 || status.State != "failed" {
		t.Fatalf("unknown reading ref should fail: accepted=%d err=%v", status.Accepted, err)
	}

	// No direct anchor → no_direct reject.
	noDirect := `{"themes":[{"candidate_ref":"t1","final_title":"x","final_question":"y?","anchor_assessments":[{"anchor_ref":"a1","fit":"supporting","supported_observation":"o"}],"reading_order":["a1"]}]}`
	_, status, err = ValidateAdjudication([]byte(noDirect), candidates)
	if err != nil || status.Accepted != 0 {
		t.Fatalf("no-direct theme should be rejected: accepted=%d err=%v", status.Accepted, err)
	}
}

func reducerFixture() ReducerInput {
	return ReducerInput{
		Themes: []AdjudicatedTheme{
			{
				CandidateRef: "t1", FinalTitle: "signup validation", FinalQuestion: "how does validation support signup?",
				AnchorAssessments: []AnchorAssessment{
					{AnchorRef: "a1", Fit: FitDirect, SupportedObservation: "controller coordinates signup"},
					{AnchorRef: "a2", Fit: FitSupporting, SupportedObservation: "filter guards input"},
					{AnchorRef: "a3", Fit: FitWeak, SupportedObservation: "unrelated helper"},
				},
				ReadingOrder: []string{"a1", "a2", "a3"},
			},
		},
		Candidates: map[string]*ScoutCandidate{
			"t1": {Title: "signup validation", Question: "how does validation support signup?", ThemeKind: KindUserJourney,
				AnchorRefs: []string{"a1", "a2", "a3"}, WhyItMatters: "w", ExpectedLearning: "l", RelationClaim: RelationClaimEditorialOnly},
		},
		Anchors: map[string]AnchorInfo{
			"a1": {Path: "controllers/account.go", Symbol: "Signup", Line: 40},
			"a2": {Path: "routers/field_validation_filter.go", Symbol: "Filter", Line: 12},
			"a3": {Path: "util/helper.go", Symbol: "Helper", Line: 7},
		},
	}
}

func TestReducerPublishFilterAndZeroSource(t *testing.T) {
	reduction, err := Reduce(reducerFixture())
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}
	if len(reduction.Cards) != 1 {
		t.Fatalf("want 1 card, got %d", len(reduction.Cards))
	}
	card := reduction.Cards[0]
	// weak/irrelevant anchors never publish: readings are exactly a1,a2.
	if len(card.Readings) != 2 {
		t.Fatalf("weak anchor must be excluded; readings=%d", len(card.Readings))
	}
	if card.Readings[0].Symbol != "Signup" || card.Readings[0].Path != "controllers/account.go" || card.Readings[0].Line != 40 {
		t.Fatalf("reading must carry exact anchor identity, got %+v", card.Readings[0])
	}
	if card.DirectCount != 1 {
		t.Fatalf("want 1 direct, got %d", card.DirectCount)
	}
	// Zero source bytes on the card.
	data, _ := json.Marshal(card)
	cardJSON := string(data)
	if strings.Contains(cardJSON, "func ") || strings.Contains(cardJSON, "\"lines\"") || strings.Contains(cardJSON, "content_sha256") {
		t.Fatalf("card must carry zero source bytes, got %s", cardJSON)
	}
}

func TestReducerCanonicalIdentityStable(t *testing.T) {
	base := reducerFixture()
	// changed prose only: identity must stay identical.
	changed := reducerFixture()
	changed.Themes[0].FinalTitle = "signup credential handling"
	changed.Themes[0].FinalQuestion = "how does signup validation and hashing work together?"
	r1, _ := Reduce(base)
	r2, _ := Reduce(changed)
	if len(r1.Cards) != 1 || len(r2.Cards) != 1 {
		t.Fatalf("expected 1 card each")
	}
	if r1.Cards[0].CanonicalID != r2.Cards[0].CanonicalID {
		t.Fatalf("identity must be stable across prose changes: %s vs %s", r1.Cards[0].CanonicalID, r2.Cards[0].CanonicalID)
	}
	// changed anchor set: identity must differ.
	anchored := reducerFixture()
	anchored.Themes[0].AnchorAssessments = append(anchored.Themes[0].AnchorAssessments,
		AnchorAssessment{AnchorRef: "a3", Fit: FitSupporting, SupportedObservation: "helper participates"})
	anchored.Themes[0].ReadingOrder = []string{"a1", "a2", "a3"}
	r3, _ := Reduce(anchored)
	if len(r3.Cards) != 1 {
		t.Fatalf("expected 1 card")
	}
	if r1.Cards[0].CanonicalID == r3.Cards[0].CanonicalID {
		t.Fatalf("identity must change when anchors change")
	}
}

func TestReducerDedupeAndBalanceCap(t *testing.T) {
	// Two themes share a root anchor; catalog has enough alternatives.
	candidates := map[string]*ScoutCandidate{}
	anchors := map[string]AnchorInfo{}
	themes := []AdjudicatedTheme{}
	for i := 0; i < 6; i++ {
		tref := "t" + string(rune('1'+i))
		candidates[tref] = &ScoutCandidate{Title: tref, Question: "q?", ThemeKind: KindUserJourney,
			AnchorRefs: []string{"a0", "a" + string(rune('1'+i))}, WhyItMatters: "w", ExpectedLearning: "l", RelationClaim: RelationClaimEditorialOnly}
		themes = append(themes, AdjudicatedTheme{
			CandidateRef: tref, FinalTitle: tref, FinalQuestion: "q" + string(rune('1'+i)) + "?",
			AnchorAssessments: []AnchorAssessment{
				{AnchorRef: "a0", Fit: FitDirect, SupportedObservation: "root"},
				{AnchorRef: "a" + string(rune('1'+i)), Fit: FitDirect, SupportedObservation: "leaf"},
			},
			ReadingOrder: []string{"a0", "a" + string(rune('1'+i))},
		})
	}
	for i := 0; i < 7; i++ {
		anchors["a"+string(rune('0'+i))] = AnchorInfo{Path: "f" + string(rune('0'+i)) + ".go", Symbol: "S", Line: 1}
	}
	input := ReducerInput{Themes: themes, Candidates: candidates, Anchors: anchors}
	reduction, err := Reduce(input)
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}
	if len(reduction.Cards) == 0 {
		t.Fatalf("balance cap must keep ≥1 published theme, got 0")
	}
	rootAnchor := "a0"
	for _, card := range reduction.Cards {
		for _, r := range card.Readings {
			if r.Path == "f0.go" {
				t.Fatalf("root anchor a0 must not dominate >half; still present on card %d", card.Ordinal)
			}
			_ = rootAnchor
		}
	}
}

// TestScoutNormalizesOverlongProseInsteadOfRejecting covers Decision 224
// (D219 A): a structurally valid candidate with overlong provisional prose
// is accepted with deterministic whole-rune truncation and a typed
// normalization count — never erased as prose_too_long.
func TestScoutNormalizesOverlongProseInsteadOfRejecting(t *testing.T) {
	anchorRefs := map[string]struct{}{"a1": {}, "a2": {}}
	fileRefs := map[string]struct{}{"f1": {}}
	longTitle := strings.Repeat("т", MaxTitleRunes+20)
	longQuestion := strings.Repeat("в", MaxQuestionRunes+15)
	longWhy := strings.Repeat("п", MaxEditorialRunes+30)
	longExpected := strings.Repeat("о", MaxEditorialRunes+40)
	raw := `{"themes":[{"title":"` + longTitle + `","question":"` + longQuestion +
		`","theme_kind":"user_journey","anchor_refs":["a1","a2"],"why_it_matters":"` + longWhy +
		`","expected_learning":"` + longExpected + `","relation_claim":"editorial_only"}]}`
	accepted, status, err := ValidateScout([]byte(raw), anchorRefs, fileRefs, "d")
	if err != nil {
		t.Fatalf("ValidateScout: %v", err)
	}
	if status.Accepted != 1 || status.Rejected != 0 || len(accepted) != 1 {
		t.Fatalf("want 1 accepted / 0 rejected, got %d/%d", status.Accepted, status.Rejected)
	}
	if utf8.RuneCountInString(accepted[0].Title) > MaxTitleRunes ||
		utf8.RuneCountInString(accepted[0].Question) > MaxQuestionRunes ||
		utf8.RuneCountInString(accepted[0].WhyItMatters) > MaxEditorialRunes ||
		utf8.RuneCountInString(accepted[0].ExpectedLearning) > MaxEditorialRunes {
		t.Fatalf("normalized candidate still exceeds a bound")
	}
	if status.Normalized["title"] != 1 || status.Normalized["question"] != 1 ||
		status.Normalized["why_it_matters"] != 1 || status.Normalized["expected_learning"] != 1 {
		t.Fatalf("typed normalization counts = %v, want 1 per field", status.Normalized)
	}
}

// TestAdjudicationSplitIssueVocabulary covers Decision 224 (D219 B): the
// conflated empty_observation is split into empty / too_long / unknown_too_long
// / too_many_unknowns, and overlong populated observations are normalized —
// never rejected as empty.
func TestAdjudicationSplitIssueVocabulary(t *testing.T) {
	candidates := map[string]*ScoutCandidate{
		"t1": {Title: "t1", Question: "q1?", ThemeKind: KindUserJourney, AnchorRefs: []string{"a1", "a2"}, WhyItMatters: "w", ExpectedLearning: "l", RelationClaim: RelationClaimEditorialOnly},
	}
	// Overlong observation: accepted, normalized, counted.
	overlongObs := strings.Repeat("н", MaxEditorialRunes+50)
	good := `{"themes":[{"candidate_ref":"t1","final_title":"x","final_question":"y?",` +
		`"anchor_assessments":[{"anchor_ref":"a1","fit":"direct","supported_observation":"` + overlongObs + `"}],` +
		`"reading_order":["a1"]}]}`
	accepted, status, err := ValidateAdjudication([]byte(good), candidates)
	if err != nil {
		t.Fatalf("ValidateAdjudication: %v", err)
	}
	if status.Accepted != 1 || len(accepted) != 1 {
		t.Fatalf("overlong observation was rejected: accepted=%d", status.Accepted)
	}
	if utf8.RuneCountInString(accepted[0].AnchorAssessments[0].SupportedObservation) > MaxEditorialRunes {
		t.Fatalf("observation not truncated to limit")
	}
	if status.Normalized["observation"] != 1 {
		t.Fatalf("observation normalization count = %v, want 1", status.Normalized)
	}

	// Empty observation: still a hard rejection with the distinct code.
	empty := `{"themes":[{"candidate_ref":"t1","final_title":"x","final_question":"y?",` +
		`"anchor_assessments":[{"anchor_ref":"a1","fit":"direct","supported_observation":""}],"reading_order":["a1"]}]}`
	_, status, err = ValidateAdjudication([]byte(empty), candidates)
	if err != nil || status.Accepted != 0 || status.Rejected != 1 {
		t.Fatalf("empty observation must reject: accepted=%d err=%v", status.Accepted, err)
	}
	found := false
	for _, issue := range status.Issues {
		if issue.Code == AdjIssueEmptyObservation {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected empty_observation issue, got %v", status.Issues)
	}

	// Too many unknowns: distinct code.
	tooMany := `{"themes":[{"candidate_ref":"t1","final_title":"x","final_question":"y?",` +
		`"anchor_assessments":[{"anchor_ref":"a1","fit":"direct","supported_observation":"o"}],` +
		`"reading_order":["a1"],"unknowns":["u1","u2","u3","u4","u5"]}]}`
	_, status, err = ValidateAdjudication([]byte(tooMany), candidates)
	if err != nil || status.Accepted != 0 {
		t.Fatalf("too-many-unknowns must reject: accepted=%d err=%v", status.Accepted, err)
	}
	found = false
	for _, issue := range status.Issues {
		if issue.Code == AdjIssueTooManyUnknowns {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected too_many_unknowns issue, got %v", status.Issues)
	}
}

// TestReducerDeduplicatesByExactSourceIdentity covers Decision 224 (D219 E):
// two anchors resolving to the same (path,line,symbol) publish one reading.
func TestReducerDeduplicatesByExactSourceIdentity(t *testing.T) {
	// a1 and a2 both resolve to controllers/account.go:40 Signup — the
	// Casdoor webhook defect class (sendWebhook twice).
	anchors := map[string]AnchorInfo{
		"a1": {Path: "controllers/account.go", Symbol: "Signup", Line: 40},
		"a2": {Path: "controllers/account.go", Symbol: "Signup", Line: 40},
	}
	input := ReducerInput{
		Themes: []AdjudicatedTheme{{
			CandidateRef: "t1", FinalTitle: "signup validation", FinalQuestion: "how does signup validate?",
			AnchorAssessments: []AnchorAssessment{
				{AnchorRef: "a1", Fit: FitDirect, SupportedObservation: "signup coordinates account creation"},
				{AnchorRef: "a2", Fit: FitDirect, SupportedObservation: "signup coordinates account creation"},
			},
			ReadingOrder: []string{"a1", "a2"},
		}},
		Candidates: map[string]*ScoutCandidate{
			"t1": {Title: "signup validation", Question: "how does signup validate?", ThemeKind: KindUserJourney,
				AnchorRefs: []string{"a1", "a2"}, WhyItMatters: "w", ExpectedLearning: "l", RelationClaim: RelationClaimEditorialOnly},
		},
		Anchors: anchors,
	}
	reduction, err := Reduce(input)
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(reduction.Cards) != 1 {
		t.Fatalf("cards = %d, want 1", len(reduction.Cards))
	}
	if len(reduction.Cards[0].Readings) != 1 {
		t.Fatalf("readings = %d, want 1 (exact (path,line,symbol) dedup)", len(reduction.Cards[0].Readings))
	}
}

// TestReducerIdentityCollisionPrefersDirectReading covers D224 R2: when two
// anchors share the exact public identity (path,line,symbol) but differ in
// fit, a supporting anchor listed FIRST in ReadingOrder must not cause the
// theme's only direct reading to be dropped (which would silently omit the
// whole theme). The direct winner replaces the supporting incumbent.
func TestReducerIdentityCollisionPrefersDirectReading(t *testing.T) {
	anchors := map[string]AnchorInfo{
		"supporting-first": {Path: "webhooks.go", Symbol: "SendWebhook", Line: 42},
		"direct-second":    {Path: "webhooks.go", Symbol: "SendWebhook", Line: 42},
	}
	input := ReducerInput{
		Themes: []AdjudicatedTheme{{
			CandidateRef: "t1", FinalTitle: "webhook dispatch", FinalQuestion: "how are webhooks dispatched?",
			AnchorAssessments: []AnchorAssessment{
				{AnchorRef: "supporting-first", Fit: FitSupporting, SupportedObservation: "webhook send is referenced"},
				{AnchorRef: "direct-second", Fit: FitDirect, SupportedObservation: "webhook send is implemented here"},
			},
			ReadingOrder: []string{"supporting-first", "direct-second"},
		}},
		Candidates: map[string]*ScoutCandidate{
			"t1": {Title: "webhook dispatch", Question: "how are webhooks dispatched?", ThemeKind: KindUserJourney,
				AnchorRefs: []string{"supporting-first", "direct-second"}, WhyItMatters: "w", ExpectedLearning: "l", RelationClaim: RelationClaimEditorialOnly},
		},
		Anchors: anchors,
	}
	reduction, err := Reduce(input)
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(reduction.Cards) != 1 {
		t.Fatalf("cards = %d, want 1 (theme must not be silently omitted)", len(reduction.Cards))
	}
	card := reduction.Cards[0]
	if len(card.Readings) != 1 {
		t.Fatalf("readings = %d, want 1", len(card.Readings))
	}
	// The direct reading must have won the identity collision.
	if card.Readings[0].Fit != FitDirect {
		t.Fatalf("winning reading fit = %q, want direct (direct beats supporting)", card.Readings[0].Fit)
	}
	if card.Readings[0].SupportedObservation != "webhook send is implemented here" {
		t.Fatalf("winning reading observation = %q, want the direct reading's observation", card.Readings[0].SupportedObservation)
	}
	if card.Badge != "editorial_source_backed" {
		t.Fatalf("badge = %q, want editorial_source_backed (only direct reading, no unknowns)", card.Badge)
	}
}

// TestReducerHonestCoverageBadge covers Decision 224 (D219 D): a theme with
// a supporting-only facet or unresolved unknowns is partial, never fully
// supported; a theme whose readings are all direct and unknowns empty is
// fully supported.
func TestReducerHonestCoverageBadge(t *testing.T) {
	anchors := map[string]AnchorInfo{
		"a1": {Path: "a.go", Symbol: "A", Line: 1},
		"a2": {Path: "b.go", Symbol: "B", Line: 2},
	}
	candidates := map[string]*ScoutCandidate{
		"t1": {Title: "t1", Question: "q?", ThemeKind: KindUserJourney, AnchorRefs: []string{"a1", "a2"}, WhyItMatters: "w", ExpectedLearning: "l", RelationClaim: RelationClaimEditorialOnly},
	}
	// Supporting facet present → partial.
	input := ReducerInput{
		Themes: []AdjudicatedTheme{{
			CandidateRef: "t1", FinalTitle: "x", FinalQuestion: "y?",
			AnchorAssessments: []AnchorAssessment{
				{AnchorRef: "a1", Fit: FitDirect, SupportedObservation: "o1"},
				{AnchorRef: "a2", Fit: FitSupporting, SupportedObservation: "o2"},
			},
			ReadingOrder: []string{"a1", "a2"},
		}},
		Candidates: candidates, Anchors: anchors,
	}
	reduction, err := Reduce(input)
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(reduction.Cards) != 1 || reduction.Cards[0].Badge != "partial" {
		t.Fatalf("supporting facet must be partial: %#v", reduction.Cards)
	}

	// All direct, no unknowns → full support.
	input.Themes[0].AnchorAssessments = []AnchorAssessment{
		{AnchorRef: "a1", Fit: FitDirect, SupportedObservation: "o1"},
		{AnchorRef: "a2", Fit: FitDirect, SupportedObservation: "o2"},
	}
	input.Themes[0].Unknowns = nil
	reduction, err = Reduce(input)
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(reduction.Cards) != 1 || reduction.Cards[0].Badge != "editorial_source_backed" {
		t.Fatalf("all-direct theme must be fully supported: %#v", reduction.Cards)
	}

	// Unknowns present → partial even when all direct.
	input.Themes[0].Unknowns = []string{"runtime order not proven"}
	reduction, err = Reduce(input)
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if reduction.Cards[0].Badge != "partial" {
		t.Fatalf("unknowns must force partial: %#v", reduction.Cards[0])
	}
}

// TestReducerPreservesModelComparativeOrder covers Phase 2 prompt cleanup:
// Scout returns themes ordered by decreasing usefulness, and the backend
// preserves that order. theme_kind is presentation metadata only — unstable
// model classification, never evidence identity, never default prominence.
func TestReducerPreservesModelComparativeOrder(t *testing.T) {
	// Distinct exact readings per theme so ordering is exercised
	// independently of the identical-reading-set co-projection (Phase 3:
	// kind is model classification, never evidence identity — themes over
	// the same exact reading collapse into one card regardless of kind).
	anchors := map[string]AnchorInfo{
		"a1": {Path: "a.go", Symbol: "A", Line: 1},
		"a2": {Path: "b.go", Symbol: "B", Line: 1},
		"a3": {Path: "c.go", Symbol: "C", Line: 1},
	}
	mk := func(tref, anchor, kind string) AdjudicatedTheme {
		return AdjudicatedTheme{
			CandidateRef: tref, FinalTitle: tref, FinalQuestion: tref + "?",
			AnchorAssessments: []AnchorAssessment{{AnchorRef: anchor, Fit: FitDirect, SupportedObservation: "o"}},
			ReadingOrder:      []string{anchor},
		}
	}
	input := ReducerInput{
		// Deliberately NOT core-first: integration_family comes first in
		// model order. The reducer must preserve it — kind never re-ranks.
		Themes: []AdjudicatedTheme{
			mk("t1", "a1", string(KindIntegrationFamily)),
			mk("t2", "a2", string(KindUserJourney)),
			mk("t3", "a3", string(KindLifecycleConcern)),
		},
		Candidates: map[string]*ScoutCandidate{
			"t1": {Title: "t1", Question: "t1?", ThemeKind: KindIntegrationFamily, AnchorRefs: []string{"a1"}, WhyItMatters: "w", ExpectedLearning: "l", RelationClaim: RelationClaimEditorialOnly},
			"t2": {Title: "t2", Question: "t2?", ThemeKind: KindUserJourney, AnchorRefs: []string{"a2"}, WhyItMatters: "w", ExpectedLearning: "l", RelationClaim: RelationClaimEditorialOnly},
			"t3": {Title: "t3", Question: "t3?", ThemeKind: KindLifecycleConcern, AnchorRefs: []string{"a3"}, WhyItMatters: "w", ExpectedLearning: "l", RelationClaim: RelationClaimEditorialOnly},
		},
		Anchors: anchors,
	}
	reduction, err := Reduce(input)
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(reduction.Cards) != 3 {
		t.Fatalf("cards = %d, want 3", len(reduction.Cards))
	}
	// Model comparative order survives: t1 (integration_family) stays first
	// even though user_journey/lifecycle appear later — kind never re-ranks.
	if reduction.Cards[0].FinalTitle != "t1" || reduction.Cards[1].FinalTitle != "t2" || reduction.Cards[2].FinalTitle != "t3" {
		t.Fatalf("model comparative order not preserved: %s, %s, %s",
			reduction.Cards[0].FinalTitle, reduction.Cards[1].FinalTitle, reduction.Cards[2].FinalTitle)
	}
}
