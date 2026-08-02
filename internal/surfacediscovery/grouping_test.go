package surfacediscovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGroupingReplayReferencesOnlySuppliedIDs(t *testing.T) {
	result := analyzeFixture(t, "direct")
	bundle := BuildGroupingBundle(result)
	raw, err := os.ReadFile(filepath.Join("testdata", "grouping_response.json"))
	if err != nil {
		t.Fatal(err)
	}
	response, validation, err := ParseGroupingResponse(raw, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Valid || len(response.Recommendations) != 1 {
		t.Fatalf("response/validation = %#v/%#v", response, validation)
	}
	directory := t.TempDir()
	if err := WriteGroupingReplayArtifacts(directory, bundle, raw); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"bundle.json", "request.redacted.json", "response.raw.txt",
		"normalized.json", "validation.json", "comparison.md",
	} {
		if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
	}
}

func TestGroupingReplayRejectsInventedTrigger(t *testing.T) {
	result := analyzeFixture(t, "direct")
	bundle := BuildGroupingBundle(result)
	raw := []byte(`{"version":1,"groups":[],"recommendations":[{"trigger_id":"invented","name":"made up","reason":"unsupported"}]}`)
	_, validation, err := ParseGroupingResponse(raw, bundle)
	if err == nil || !strings.Contains(err.Error(), "unknown trigger id") || validation.Valid {
		t.Fatalf("error/validation = %v/%#v", err, validation)
	}
}
