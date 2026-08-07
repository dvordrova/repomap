package tasklens

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Refreshes the reportserver task-warning fixture after ANY Task Lens prompt
// wording change: the saved attempt prompt SHA and the status artifact SHAs
// must match the current stable prompt bytes. Run with -update.
var updateFixtureFlag = flag.Bool("update", false, "update fixture files")

func TestRegenerateWarningFixture(t *testing.T) {
	if !*updateFixtureFlag {
		t.Skip("fixture refresh is explicit (run with -update)")
	}
	base := filepath.Join("..", "reportserver", "testdata", "task-warning")

	attemptPath := filepath.Join(base, AttemptFile)
	attemptRaw, err := os.ReadFile(attemptPath)
	if err != nil {
		t.Fatal(err)
	}
	var attempt Attempt
	if err := json.Unmarshal(attemptRaw, &attempt); err != nil {
		t.Fatal(err)
	}
	bundleRaw, err := os.ReadFile(filepath.Join(base, BundleFile))
	if err != nil {
		t.Fatal(err)
	}
	var bundle Bundle
	if err := json.Unmarshal(bundleRaw, &bundle); err != nil {
		t.Fatal(err)
	}
	stable, err := StablePromptJSON(bundle)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(stable)
	bundleHash, err := BundleHash(bundle)
	if err != nil {
		t.Fatal(err)
	}
	attempt.PromptSHA256 = fmt.Sprintf("%x", digest)
	attempt.PromptVersion = PromptVersion
	attemptRaw, err = json.Marshal(attempt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(attemptPath, attemptRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("rewrote %s (prompt sha %x)", AttemptFile, digest)

	statusPath := filepath.Join(base, StatusFile)
	statusRaw, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	var status Status
	if err := json.Unmarshal(statusRaw, &status); err != nil {
		t.Fatal(err)
	}
	attemptFileSHA := sha256.Sum256(attemptRaw)
	status.AttemptSHA256 = fmt.Sprintf("%x", attemptFileSHA)
	status.BundleSHA256 = bundleHash
	statusRaw, err = json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statusPath, statusRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("rewrote %s", StatusFile)
}
