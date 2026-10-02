package report

import (
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/targetoutcome"
)

// A target the run could not read says why in the failure's own words:
// under the programs on the home reading, on its card in Component details,
// and in the reading of the map's "Not analysed" note, which shows those
// cards. litestream's src had read only "analysis failed".
func TestATargetNotReadSaysWhyWhereItAppears(t *testing.T) {
	data := reportProgramShellDataFixture(t, "litestream")
	const why = "no build line compiles src/litestream-vfs.c; parsed with clang's defaults (exit status 1): src/litestream-vfs.c:1:10: fatal error: 'litestream-vfs.h' file not found"
	view := *data.TargetOutcomePortfolio
	view.Outcomes = append(append([]TargetOutcomeView(nil), view.Outcomes...), TargetOutcomeView{
		SelectedTargetID: "t9", Language: targetoutcome.LanguageGroup("c"), AllowedProgramLanguages: []string{"c"},
		ScopeKind: targetoutcome.ScopeLibrary, DisplayName: "src", Selector: "c:src/", State: targetoutcome.StateNotAnalyzed,
		FailureStage: targetoutcome.StageTargetPreparation, FailureReason: targetoutcome.ReasonAnalysisFailed, FailureDetail: why,
	})
	if err := view.Validate(); err != nil {
		t.Fatal(err)
	}
	data.TargetOutcomePortfolio = &view
	page, err := RenderHTMLWithOptions(&data, reportSingleTargetRenderOptionsFixture(t, &data))
	if err != nil {
		t.Fatal(err)
	}
	text := html.UnescapeString(string(page))
	home := regexp.MustCompile(`(?s)<article><strong>src</strong><p class="meta">analysis failed</p><p class="unread-why">([^<]*)</p></article>`).FindStringSubmatch(text)
	if home == nil || home[1] != why {
		t.Fatal("the home reading does not say why src was not read")
	}
	card := regexp.MustCompile(`(?s)<div id="targets-not-read" class="cards"><div class="card muted"><h3>src</h3>.*?<dd>analysis failed \(target preparation\)<p class="unread-why">([^<]*)</p></dd>`).FindStringSubmatch(text)
	if card == nil || card[1] != why {
		t.Fatal("src's card in Component details does not say why it was not read")
	}
	note := regexp.MustCompile(`id="system-unread"[^>]*data-details-id="([^"]*)"`).FindStringSubmatch(text)
	if note == nil || note[1] != "targets-not-read" {
		t.Fatal("the map's Not analysed note does not read the cards that say why")
	}
	if strings.Contains(text, "/Users/") {
		t.Fatal("a host path reached the page")
	}
}
