package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

// TestAtlasStudyReadableTargetReferenceDerivesQualifiedSymbols verifies the
// D211 HOLD-repair contract: a fully-qualified symbol yields the exact
// package.Symbol segment after the last '/', bare identifiers qualify
// directly, natural labels fall back, and generic labels stay private.
func TestAtlasStudyReadableTargetReferenceDerivesQualifiedSymbols(t *testing.T) {
	cases := []struct {
		name    string
		symbol  string
		label   string
		wantRef string
		wantSym bool
	}{
		{
			name:    "qualified go function yields package.Symbol",
			symbol:  "github.com/casdoor/casdoor/object.InitDb",
			label:   "Repository function",
			wantRef: "object.InitDb", wantSym: true,
		},
		{
			name:    "qualified method yields package.Symbol",
			symbol:  "github.com/casdoor/casdoor/service.Start",
			label:   "Repository function",
			wantRef: "service.Start", wantSym: true,
		},
		{
			name:    "qualified main yields package.Symbol",
			symbol:  "github.com/casdoor/casdoor.main",
			label:   "main",
			wantRef: "casdoor.main", wantSym: true,
		},
		{
			name:    "external qualified call derives the bare identifier",
			symbol:  "http.ListenAndServe",
			label:   "Qualified Go call site",
			wantRef: "ListenAndServe", wantSym: true,
		},
		{
			name:    "bare identifier qualifies directly",
			symbol:  "ObtainCertificateGoDaddy",
			label:   "Function ObtainCertificateGoDaddy",
			wantRef: "ObtainCertificateGoDaddy", wantSym: true,
		},
		{
			name:    "empty symbol falls back to natural label",
			symbol:  "",
			label:   "main",
			wantRef: "main", wantSym: false,
		},
		{
			name:    "generic label stays private",
			symbol:  "",
			label:   "Repository function",
			wantRef: "", wantSym: false,
		},
		{
			name:    "trailing slash has no derivable segment",
			symbol:  "github.com/casdoor/casdoor/",
			label:   "Repository function",
			wantRef: "", wantSym: false,
		},
		{
			name:    "control characters inside the segment are rejected",
			symbol:  "pkg.Init\nDb",
			label:   "Repository function",
			wantRef: "", wantSym: false,
		},
		{
			name:    "spaces inside the segment are rejected",
			symbol:  "pkg.Init Db",
			label:   "Repository function",
			wantRef: "", wantSym: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref, symbol := atlasStudyReadableTargetReference(atlasstudy.ReadingTarget{
				Symbol: tc.symbol, Label: tc.label,
			})
			if ref != tc.wantRef || symbol != tc.wantSym {
				t.Fatalf("reference(symbol %q, label %q) = (%q, %v), want (%q, %v)",
					tc.symbol, tc.label, ref, symbol, tc.wantRef, tc.wantSym)
			}
		})
	}
}

// TestAtlasStudyRouteSpansTargetSpecificQuestions verifies that system-path
// and focused questions carry the exact derived package.Symbol for both
// languages and never leak the import path, generic labels or generic wording.
func TestAtlasStudyRouteSpansTargetSpecificQuestions(t *testing.T) {
	targets := []atlasstudy.ReadingTarget{
		{
			ID: "target-main", Kind: "symbol", Label: "main",
			Symbol:    "github.com/casdoor/casdoor.main",
			Location:  evidence.Location{Path: "main.go", Line: 36},
			Authority: repositoryatlas.AuthorityInferred,
		},
		{
			ID: "target-initdb", Kind: "symbol", Label: "Repository function",
			Symbol:    "github.com/casdoor/casdoor/object.InitDb",
			Location:  evidence.Location{Path: "object/init.go", Line: 27},
			Authority: repositoryatlas.AuthorityInferred,
		},
		{
			ID: "target-godaddy", Kind: "symbol", Label: "Function ObtainCertificateGoDaddy",
			Symbol:    "ObtainCertificateGoDaddy",
			Location:  evidence.Location{Path: "certificate/dns.go", Line: 135},
			Authority: repositoryatlas.AuthorityInferred,
		},
	}
	supports := []atlasstudy.ReadingSupport{
		{ID: "support-entry", TargetID: "target-main", Role: atlasstudy.SupportProcessEntry, Authority: repositoryatlas.AuthorityInferred},
		{ID: "support-callee", TargetID: "target-initdb", Role: atlasstudy.SupportEntryHandoff, Authority: repositoryatlas.AuthorityInferred},
		{ID: "support-godaddy", TargetID: "target-godaddy", Role: atlasstudy.SupportEntryHandoff, Authority: repositoryatlas.AuthorityInferred},
	}
	relations := []atlasstudy.RouteProducerRelation{{
		ID: "rel-1", Kind: atlasstudy.RouteRelationEntryHandoff,
		FromSupportID: "support-entry", ToSupportID: "support-callee",
		FromTargetID: "target-main", ToTargetID: "target-initdb",
	}}

	spans := atlasStudyRouteSpans(targets, supports, relations)
	// Every support yields one focused span and every relation one
	// system-path span: 3 + 1.
	if len(spans) != 4 {
		t.Fatalf("route spans = %d, want 4", len(spans))
	}
	byID := make(map[string]atlasstudy.RouteSpan, len(spans))
	for _, span := range spans {
		byID[span.ID] = span
	}

	systemPath := byID["route-span-"+atlasStudyDigest("system_path\x00rel-1")]
	if systemPath.Kind != atlasstudy.RouteSpanSystemPath {
		t.Fatalf("system path span kind = %q", systemPath.Kind)
	}
	for _, forbidden := range []string{
		"github.com/casdoor", "Repository function", "repository-local callee", "process entry to this",
	} {
		if strings.Contains(systemPath.QuestionEnglish, forbidden) || strings.Contains(systemPath.QuestionRussian, forbidden) {
			t.Fatalf("system path question leaks %q:\nEN %q\nRU %q",
				forbidden, systemPath.QuestionEnglish, systemPath.QuestionRussian)
		}
	}
	for _, want := range []string{"`casdoor.main`", "`object.InitDb`"} {
		if !strings.Contains(systemPath.QuestionEnglish, want) || !strings.Contains(systemPath.QuestionRussian, want) {
			t.Fatalf("system path question missing %s:\nEN %q\nRU %q",
				want, systemPath.QuestionEnglish, systemPath.QuestionRussian)
		}
	}
	if !strings.HasSuffix(systemPath.QuestionEnglish, "?") || !strings.HasSuffix(systemPath.QuestionRussian, "?") {
		t.Fatalf("system path questions must end with '?': %q / %q",
			systemPath.QuestionEnglish, systemPath.QuestionRussian)
	}

	focused := byID["route-span-"+atlasStudyDigest("focused\x00support-godaddy")]
	if focused.Kind != atlasstudy.RouteSpanFocused {
		t.Fatalf("focused span kind = %q", focused.Kind)
	}
	for _, want := range []string{"`ObtainCertificateGoDaddy`"} {
		if !strings.Contains(focused.QuestionEnglish, want) || !strings.Contains(focused.QuestionRussian, want) {
			t.Fatalf("focused question missing %s:\nEN %q\nRU %q",
				want, focused.QuestionEnglish, focused.QuestionRussian)
		}
	}

	entryFocused := byID["route-span-"+atlasStudyDigest("focused\x00support-callee")]
	if entryFocused.Kind != atlasstudy.RouteSpanFocused {
		t.Fatalf("entry focused span kind = %q", entryFocused.Kind)
	}
	if !strings.Contains(entryFocused.QuestionEnglish, "`object.InitDb`") ||
		!strings.Contains(entryFocused.QuestionRussian, "`object.InitDb`") {
		t.Fatalf("entry focused question missing `object.InitDb`:\nEN %q\nRU %q",
			entryFocused.QuestionEnglish, entryFocused.QuestionRussian)
	}
	if strings.Contains(entryFocused.QuestionEnglish, "What repository code is called directly from the process entry?") {
		t.Fatalf("entry focused question still generic: %q", entryFocused.QuestionEnglish)
	}
}

// TestAtlasStudyRouteSpanIDsIgnoreQuestionWording verifies the span identity
// invariant: span IDs derive from support/relation IDs only, so question
// compilation changes never alter the accepted frontier or replayed result.
func TestAtlasStudyRouteSpanIDsIgnoreQuestionWording(t *testing.T) {
	makeTargets := func(symbol string) []atlasstudy.ReadingTarget {
		return []atlasstudy.ReadingTarget{
			{
				ID: "target-main", Kind: "symbol", Label: "main", Symbol: symbol,
				Location:  evidence.Location{Path: "main.go", Line: 36},
				Authority: repositoryatlas.AuthorityInferred,
			},
			{
				ID: "target-callee", Kind: "symbol", Label: "Repository function",
				Symbol:    symbol + "/object.InitDb",
				Location:  evidence.Location{Path: "object/init.go", Line: 27},
				Authority: repositoryatlas.AuthorityInferred,
			},
		}
	}
	supports := []atlasstudy.ReadingSupport{
		{ID: "support-entry", TargetID: "target-main", Role: atlasstudy.SupportProcessEntry, Authority: repositoryatlas.AuthorityInferred},
		{ID: "support-callee", TargetID: "target-callee", Role: atlasstudy.SupportEntryHandoff, Authority: repositoryatlas.AuthorityInferred},
	}
	relations := []atlasstudy.RouteProducerRelation{{
		ID: "rel-stable", Kind: atlasstudy.RouteRelationEntryHandoff,
		FromSupportID: "support-entry", ToSupportID: "support-callee",
		FromTargetID: "target-main", ToTargetID: "target-callee",
	}}

	before := atlasStudyRouteSpans(makeTargets("example.com/repo"), supports, relations)
	after := atlasStudyRouteSpans(makeTargets("changed.example.org/other"), supports, relations)
	if len(before) != len(after) {
		t.Fatalf("span count changed: %d -> %d", len(before), len(after))
	}
	for index := range before {
		if before[index].ID != after[index].ID {
			t.Fatalf("span ID changed with question wording: %q -> %q",
				before[index].ID, after[index].ID)
		}
		if before[index].Kind != after[index].Kind {
			t.Fatalf("span kind changed with question wording: %q -> %q",
				before[index].Kind, after[index].Kind)
		}
	}
}
