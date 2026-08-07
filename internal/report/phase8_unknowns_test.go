package report

import (
	"testing"

	"github.com/dvordrova/repomap/internal/themestudy"
)

// Phase 8 reviewer finding (product usefulness): adjudication unknowns must
// reach the published report card, not only drive the partial badge.
func TestThemeLimitationUsesReviewedAnchorDenominator(t *testing.T) {
	t.Parallel()
	themes := themestudy.StudyThemes{Partial: true}
	// ReviewedAnchorCount is the exact denominator: 5 anchors were reviewed,
	// 4 passed (direct+supporting), so the limitation must say 4 of 5.
	card := themestudy.ThemeCard{
		DirectCount:         3,
		SupportingCount:     1,
		ReviewedAnchorCount: 5,
	}
	got := themeLimitation(themes, card)
	want := "partial — 4 of 5 anchors passed source review"
	if got != want {
		t.Fatalf("themeLimitation = %q, want %q", got, want)
	}
	// Regression: the pre-fix N/N form is never produced.
	if got == "partial — 4 of 4 anchors passed source review" {
		t.Fatalf("themeLimitation still renders N/N (denominator bug)")
	}
}

// Phase 8 reviewer finding: a card with unknowns and no passed readings
// still renders a limitation (the generic partial form).
func TestThemeLimitationZeroPassedReadings(t *testing.T) {
	t.Parallel()
	themes := themestudy.StudyThemes{Partial: true}
	card := themestudy.ThemeCard{Unknowns: []string{"unclear"}}
	if got := themeLimitation(themes, card); got != "partial" {
		t.Fatalf("themeLimitation = %q, want %q", got, "partial")
	}
}
