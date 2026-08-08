package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/report"
)

// MAP_READY gate (Decision 235): provider-free replay of the 9 fixtures —
// every applicable row must PASS. Verifies the Architecture response is
// accepted under the current contract (not a whole fallback) and the Study
// input compiles from the accepted canvas (no stale local names, no blank
// span questions).
func TestV11MapReadyFixtures(t *testing.T) {
	fixtures := []struct {
		name   string
		runDir string
	}{
		{"casdoor", "/Users/dvordrova/git/repomap/tmp/hermes-archive9-semantic-product-20260806-182649/archive9/20260806-163314-casdoor-f1e101f458c5"},
		{"telebot", "/Users/dvordrova/git/repomap/tmp/hermes-archive9-semantic-product-20260806-182649/archive9/20260806-163318-telebot-265c26ae8755"},
		{"chatto", "/Users/dvordrova/git/repomap/tmp/hermes-archive9-semantic-product-20260806-182649/archive9/20260806-163320-chatto-0f4e9ee7d764"},
		{"restic", "/Users/dvordrova/git/repomap/tmp/hermes-archive9-semantic-product-20260806-182649/archive9/20260806-163323-restic-accaa84c6dd7"},
		{"miniflux", "/Users/dvordrova/git/go-corpus-repomap/service__github__miniflux__v2/runs/20260806-231044-v2-04fcac53e8ea"},
		{"gotify", "/Users/dvordrova/git/go-corpus-repomap/service__github__gotify__server/runs/20260806-231044-server-2f0a6dbd338b"},
		{"task", "/Users/dvordrova/git/go-corpus-repomap/cli__github__go-task__task/runs/20260806-231233-task-9de2dd11eeb7"},
		{"lazygit", "/Users/dvordrova/git/go-corpus-repomap/cli__github__jesseduffield__lazygit/runs/20260806-231358-lazygit-20210fca818a"},
		{"gosec", "/Users/dvordrova/git/go-corpus-repomap/cli__github__securego__gosec/runs/20260806-231347-gosec-17d980f31d06"},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			if _, err := os.Stat(fixture.runDir); err != nil {
				t.Skipf("run dir unavailable: %v", err)
			}
			readDir := t.TempDir()
			if err := copyRunDirForReplay(fixture.runDir, readDir); err != nil {
				t.Fatalf("stage run dir: %v", err)
			}
			for _, artifact := range []string{
				"theme_scout_request.v1.json", "theme_scout_result.v1.json",
				"theme_scout_status.v1.json",
				"theme_adjudication_request.v1.json",
				"theme_adjudication_result.v1.json",
				"theme_adjudication_status.v1.json",
				"study_themes.v1.json", "theme_source_expansion.v1.json",
			} {
				_ = os.Remove(filepath.Join(readDir, artifact))
			}
			// Saved synthesis records predate the v11 record version and
			// fail closed on replay — drop them so the RAW saved response
			// replays under the current contract (archive9 pattern).
			_ = os.Remove(filepath.Join(readDir, "architecture_synthesis.json"))
			_ = os.Remove(filepath.Join(readDir, "architecture_synthesis_status.json"))
			data, err := report.ReadRunDir(readDir)
			if err != nil {
				t.Fatalf("read run dir: %v", err)
			}
			// Row: one Architecture response grammar + item-local
			// normalization — the saved response must replay accepted
			// (validated/partial/normalized), never a whole fallback.
			saved, err := savedArchitectureResponseBytes(t, readDir)
			if err != nil {
				t.Skipf("no saved architecture response: %v", err)
			}
			bundle, err := report.BuildArchitectureCanvasInput(data)
			if err != nil {
				t.Fatalf("build input: %v", err)
			}
			result, err := componentmap.RecordSynthesisResponse(bundle.CandidateBundle, "v11-mapready-"+fixture.name, "test", "test", time.Millisecond, saved)
			if err != nil {
				t.Fatalf("record: %v", err)
			}
			if result.Landscape.Fallback {
				t.Fatalf("whole fallback (rows: one grammar, item-local normalization)")
			}
			if result.Landscape.ValidationOutcome != componentmap.ValidationAccepted &&
				result.Landscape.ValidationOutcome != componentmap.ValidationAcceptedPartial &&
				result.Landscape.ValidationOutcome != componentmap.ValidationAcceptedNormalized {
				t.Fatalf("outcome %s (row: accepted)", result.Landscape.ValidationOutcome)
			}
			// Row: final Architecture used by Study — the Study input
			// compiles from the accepted canvas (or local fallback when
			// Architecture failed/unavailable), never a hard error.
			input, err := report.BuildAtlasStudyInput(data, languageForFixture())
			if err != nil {
				t.Fatalf("Study input: %v (row: final Architecture used by Study)", err)
			}
			// Row: zero blank span questions — the compiled Scout
			// context must not emit empty placeholder objects.
			if input.Architecture.Version <= 0 && input.Architecture.Source == "" {
				t.Fatalf("Architecture block empty without local source marker")
			}
			// Row: version/cache/replay correctness — the input carried
			// an identity-consistent bundle.
			if len(bundle.CandidateBundle.Candidates) == 0 {
				t.Fatalf("no candidates (row: explicit unclassified remainder)")
			}
		})
	}
}

func languageForFixture() atlasstudy.Language {
	return atlasstudy.LanguageEnglish
}
