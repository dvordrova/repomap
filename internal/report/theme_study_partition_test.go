package report

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/themestudy"
)

func TestValidateThemeStudyThemesBindsCoProjectedPartition(t *testing.T) {
	scoutRequest := themestudy.ScoutRequest{CatalogSHA256: "scout-digest"}
	result := themestudy.AdjudicationResult{
		CatalogSHA256: "adjudication-digest",
		Themes:        make([]themestudy.AdjudicatedTheme, 4),
	}
	themes := themestudy.StudyThemes{
		ScoutSHA256: scoutRequest.CatalogSHA256,
		AdjSHA256:   result.CatalogSHA256,
		Cards:       make([]themestudy.ThemeCard, 1),
		Omitted:     1,
		CoProjected: 2,
	}
	if err := validateThemeStudyThemes(themes, result, scoutRequest); err != nil {
		t.Fatalf("valid cards + omitted + co_projected partition rejected: %v", err)
	}

	tampered := themes
	tampered.CoProjected--
	err := validateThemeStudyThemes(tampered, result, scoutRequest)
	if err == nil {
		t.Fatal("tampered co_projected count passed report hydration")
	}
	if !strings.Contains(err.Error(), "does not partition accepted themes") {
		t.Fatalf("tampered partition error = %q", err)
	}
}
