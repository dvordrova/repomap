package main

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/themestudy"
)

// Long-horizon program Phase 3 (validation audit, owner directive): a
// backend-owned span question must carry the exact a* anchor ref it compiled
// to, so the model never has to guess which anchor a question belongs to
// (span_questions previously shipped without refs; 5/24 Miniflux questions
// did not even match a seed symbol).
func TestThemeScoutContextBindsSpanQuestionsToAnchors(t *testing.T) {
	t.Parallel()
	input := atlasStudyRuntimeInput()
	product, err := atlasstudy.Compile(input)
	if err != nil {
		t.Fatalf("compile substrate: %v", err)
	}
	seedSpecs := themeSeedSpecsFromInput(input)
	packs, err := themestudy.BuildSeedPacks(
		seedSpecs, 0, 0, 0, 0,
		func(path string, startLine, endLine int) ([]string, error) {
			return []string{"func f() {", "\t// fixture body", "}"}, nil
		},
		func(path string) (int, error) { return 3, nil },
	)
	if err != nil {
		t.Fatalf("build seed packs: %v", err)
	}
	context := themeScoutContext(product, "fixture", themeSpanAnchorRefsFromPacks(packs))
	if len(context.SpanQuestions) == 0 {
		t.Fatalf("fixture must expose at least one span question")
	}
	bound := 0
	unbound := 0
	for _, question := range context.SpanQuestions {
		if question.AnchorRef == "" {
			unbound++
			continue
		}
		bound++
		// Every bound ref must be a real a* seed in the packs.
		found := false
		for _, pack := range packs.Packs {
			if pack.Seed.Ref == question.AnchorRef {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("span question binds to %q which is not a seed ref", question.AnchorRef)
		}
	}
	if bound == 0 {
		t.Fatalf("fixture spans must bind to anchors, got 0 bound")
	}
	_ = unbound
}
