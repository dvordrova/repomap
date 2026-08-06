package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
)

// TestArchive8EtcdReplayD7Salvage replays the Archive 8 etcd saved synthesis
// response through the CURRENT D7 item-scope salvage code. The Archive 8
// run was generated before the D7 repair commits; this proves the etcd
// proposal (13 records incl. one unknown anchor ref) now publishes as
// accepted_partial instead of whole-rejection.
func TestArchive8EtcdReplayD7Salvage(t *testing.T) {
	runDir := os.Getenv("ARCHIVE8_ETCD_RUN")
	clone := os.Getenv("ARCHIVE8_ETCD_CLONE")
	if runDir == "" || clone == "" {
		t.Skip("ARCHIVE8_ETCD_RUN and ARCHIVE8_ETCD_CLONE not set")
	}
	seed, err := report.ReadRunManifestAuthoritySeed(runDir)
	if err != nil {
		t.Fatalf("read authority seed: %v", err)
	}
	ctx := context.Background()
	before, err := freshness.CaptureRepository(ctx, clone)
	if err != nil {
		t.Fatalf("capture repo before: %v", err)
	}
	after, err := freshness.CaptureRepository(ctx, clone)
	if err != nil {
		t.Fatalf("capture repo after: %v", err)
	}
	authority, err := report.ConfirmRunAuthorityScoped(
		ctx, clone, before, after, seed.CapturedInputPaths, true,
	)
	if err != nil {
		t.Fatalf("confirm authority: %v", err)
	}
	data, err := report.ReadRunDirForAuthorizedArchitecture(runDir, authority)
	if err != nil {
		t.Fatalf("read run dir: %v", err)
	}
	bundle, err := report.BuildArchitectureCanvasInput(data)
	if err != nil {
		t.Fatalf("build input: %v", err)
	}
	revision := "etcd-replay"
	if data.ModelResearch != nil {
		ctx := data.ModelResearch.Repository
		revision = ctx.Revision + ":" + "etcd-replay"
	}
	exchanges := filepath.Join(runDir, "semantic_exchanges")
	entries, err := os.ReadDir(exchanges)
	if err != nil {
		t.Fatalf("read exchanges: %v", err)
	}
	var saved []byte
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		exPath := filepath.Join(exchanges, entry.Name(), "exchange.v1.json")
		raw, err := os.ReadFile(exPath)
		if err != nil {
			continue
		}
		var ex struct {
			Kind  string `json:"kind"`
			Stage string `json:"stage"`
		}
		if json.Unmarshal(raw, &ex) == nil && (ex.Kind == "architecture_synthesis" || ex.Stage == "architecture_synthesis") {
			saved, err = os.ReadFile(filepath.Join(exchanges, entry.Name(), "response.json"))
			if err != nil {
				t.Fatalf("read saved response: %v", err)
			}
			// The response.json stores a storage envelope in Archive 8
			// runs; unwrap to the raw record bytes when present.
			var envelope struct {
				OriginalBytes int    `json:"original_bytes"`
				Storage       string `json:"storage"`
			}
			if json.Unmarshal(saved, &envelope) == nil && envelope.Storage == "raw_content" && envelope.OriginalBytes > 0 {
				file := filepath.Join(exchanges, entry.Name(), "response.json")
				if raw, err := os.ReadFile(file); err == nil {
					var candidate json.RawMessage
					if json.Unmarshal(raw, &candidate) == nil && !json.Valid(candidate) {
						saved = raw
					}
				}
			}
			break
		}
	}
	if len(saved) == 0 {
		t.Fatal("no saved architecture synthesis response found")
	}
	result, err := componentmap.RecordSynthesisResponse(bundle.CandidateBundle, revision, "etcd-replay", "deepseek", 0, saved)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	t.Logf("outcome=%s fallback=%v fallbackReason=%s subsystems=%d memberships=%d",
		result.Landscape.ValidationOutcome, result.Landscape.Fallback,
		result.Landscape.FallbackReason, len(result.Landscape.Subsystems),
		len(result.Landscape.ConceptualMemberships))
	for _, d := range result.Landscape.Diagnostics {
		if strings.HasPrefix(d.Code, "proposal.") {
			t.Logf("  diagnostic: %s severity=%s", d.Code, d.Severity)
		}
	}
	if result.Landscape.ValidationOutcome != componentmap.ValidationAcceptedPartial {
		t.Errorf("outcome = %v, want accepted_partial (D7 item-scope salvage)", result.Landscape.ValidationOutcome)
	}
	if result.Landscape.Fallback {
		t.Errorf("fallback fired: %v", result.Landscape.FallbackReason)
	}
}
