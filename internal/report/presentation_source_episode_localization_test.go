package report

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/localization"
)

func TestPresentationTextInventoryIncludesApprovedSourceEpisodeProse(t *testing.T) {
	t.Parallel()

	raw, episode := readSourceEpisodeFixture(
		t,
		filepath.Join(
			"..",
			"..",
			"experiments",
			"source-episode",
			"etcd-put",
			"episode.json",
		),
	)
	canonical := authorizedSourceEpisodeReport(episode)
	if err := AttachSourceEpisodePresentation(canonical, raw); err != nil {
		t.Fatal(err)
	}
	prepared, err := PreparePresentationLocalization(
		canonical,
		localization.LocaleRussian,
	)
	if err != nil {
		t.Fatal(err)
	}
	inputByID := make(map[string]localization.InputField, len(prepared.Input.Fields))
	inputTexts := make([]string, 0, len(prepared.Input.Fields))
	for _, field := range prepared.Input.Fields {
		inputByID[field.ID] = field
		inputTexts = append(inputTexts, field.Text)
	}
	for _, prose := range []string{
		episode.Question,
		episode.Claims[0].Title,
		episode.Claims[0].Statement,
		episode.Uncertainties[0].Statement,
	} {
		fieldID := ""
		for _, field := range prepared.Canonical.Fields {
			if field.Text == prose {
				fieldID = field.ID
				break
			}
		}
		input, found := inputByID[fieldID]
		if fieldID == "" || !found || input.Text == "" {
			t.Fatalf(
				"source-episode prose is absent from one translation request: %q",
				prose,
			)
		}
	}
	joinedInputTexts := strings.Join(inputTexts, "\n")
	for _, opaque := range []string{
		episode.EpisodeID,
		episode.Repository.Revision,
		episode.Anchors[0].Path,
	} {
		if strings.Contains(joinedInputTexts, opaque) {
			t.Fatalf("opaque source-episode value reached translation prose: %q", opaque)
		}
	}

	projected, result, err := ApplyPresentationLocalization(
		canonical,
		prepared,
		russianPresentationProjection(prepared),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 {
		t.Fatalf("source-episode projection was rejected: %#v", result)
	}
	html, err := RenderHTMLWithSourceEpisode(projected, raw)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(html, []byte(episode.Question)) ||
		!bytes.Contains(html, []byte("Русский текст")) {
		t.Fatalf("source-episode terminal prose was not projected atomically")
	}
	for _, opaque := range []string{
		episode.Repository.Revision,
		episode.Anchors[0].Path,
	} {
		if !bytes.Contains(html, []byte(opaque)) {
			t.Fatalf("opaque source-episode value changed or disappeared: %q", opaque)
		}
	}
}
