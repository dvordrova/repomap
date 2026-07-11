package deepseektest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSourceFixtureManifestHashes(t *testing.T) {
	t.Parallel()

	var manifest struct {
		SHA256 map[string]string `json:"sha256"`
	}
	if err := json.Unmarshal(SourceManifestJSON, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	fixtures := map[string][]byte{
		"source_card.json":     SourceCardJSON,
		"source_bundle.json":   SourceBundleJSON,
		"source_response.json": SourceResponseJSON,
	}
	for name, data := range fixtures {
		if !json.Valid(data) {
			t.Fatalf("%s is not valid JSON", name)
		}
		got := fmt.Sprintf("%x", sha256.Sum256(data))
		if got != manifest.SHA256[name] {
			t.Fatalf("%s hash = %s, want %s", name, got, manifest.SHA256[name])
		}
		lower := strings.ToLower(string(data))
		if strings.Contains(lower, "authorization") || strings.Contains(lower, "bearer sk-") {
			t.Fatalf("%s contains authorization material", name)
		}
	}
}
