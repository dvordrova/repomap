package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusCountsEntryHandoffGroupsWithoutMechanismFallback(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	reportJSON := []byte(`{
  "captured_revision":"abc123",
  "discovered_surfaces":{"version":1,"total_count":17},
  "architecture_associations":{"version":2,"total":23},
  "architecture_canvas":{
    "entry_handoff_groups":[
      {"entry_handoffs":[{},{}]},
      {"entry_handoffs":[{}]}
    ],
    "mechanism_fragments":[{},{},{},{},{}]
  }
}`)
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), reportJSON, 0o600); err != nil {
		t.Fatal(err)
	}

	facts := collectCorpusRunFacts(corpusRepo{Repository: "example/repo"}, runDir)
	if facts.EntryHandoffGroups != 2 || facts.EntryHandoffs != 3 {
		t.Fatalf("entry handoff counts = groups %d handoffs %d, want 2/3", facts.EntryHandoffGroups, facts.EntryHandoffs)
	}
	if facts.Surfaces != 17 || facts.Associations != 23 {
		t.Fatalf("current report object counts = surfaces %d associations %d, want 17/23", facts.Surfaces, facts.Associations)
	}

	var output bytes.Buffer
	printCorpusFact(&output, facts)
	if !strings.Contains(output.String(), "entry_groups=2") ||
		!strings.Contains(output.String(), "entry_handoffs=3") ||
		strings.Contains(output.String(), "mech=") {
		t.Fatalf("corpus output retained ambiguous mechanism accounting: %s", output.String())
	}
}
