package run

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The page says the run until its publication; the command's time is that
// and the publication. Headscale's metadata said 185,602 ms of a 781 s
// command, its 596 s publication nowhere. The publication is its own stage
// in the Time block, and the saved timing keeps the page's wall_ms beside
// publication_ms and command_ms, every other field as written.
func TestPublicationIsItsOwnStageAndTheCommandItsWholeTime(t *testing.T) {
	runDir := t.TempDir()
	saved := `{"repo_name":"headscale","run_id":"r1","timing":{"wall_ms":185602,"stages":[{"stage":"glossary","live":13,"cached":0,"provider_ms":22122,"slowest_ms":10777}]}}`
	if err := os.WriteFile(filepath.Join(runDir, "metadata.json"), []byte(saved), 0o644); err != nil {
		t.Fatal(err)
	}
	var console bytes.Buffer
	output := newRunOutput(&console)
	start := time.Date(2026, 10, 2, 15, 24, 59, 0, time.UTC)
	output.started, output.now = start, func() time.Time { return start.Add(781 * time.Second) }
	output.Wall(publicationStage, 596*time.Second)
	if err := recordCommandTiming(runDir, output); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		RepoName string `json:"repo_name"`
		RunID    string `json:"run_id"`
		Timing   struct {
			WallMS        int64 `json:"wall_ms"`
			PublicationMS int64 `json:"publication_ms"`
			CommandMS     int64 `json:"command_ms"`
			Stages        []struct {
				Stage string `json:"stage"`
				Live  int    `json:"live"`
			} `json:"stages"`
		} `json:"timing"`
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	timing := metadata.Timing
	if metadata.RepoName != "headscale" || metadata.RunID != "r1" || timing.WallMS != 185602 ||
		timing.PublicationMS != 596000 || timing.CommandMS != 781000 || len(timing.Stages) != 1 || timing.Stages[0].Live != 13 {
		t.Fatalf("saved timing = %s", raw)
	}
	output.Timing()
	if !strings.Contains(console.String(), "report publication: 9m56s, no model") {
		t.Fatalf("the Time block does not name the publication: %s", console.String())
	}
}
