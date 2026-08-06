package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
)

// copyRunDirForReplay stages a run directory into a temp dir so the test can
// drop saved synthesis artifacts before reading (see TestArchive9ReplaySharedParticipation).
func copyRunDirForReplay(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}

// TestArchive9ReplaySharedParticipation replays all five Archive 9 saved
// architecture synthesis responses through the CURRENT Decision 231 shared
// participation code. Archive 9 (runs 20260806-1633xx, built on the
// intermediate D230 binary) whole-rejected Telebot and Chatto and weakened
// Restic because repeated broad package units were intersected with symbol
// anchors. The owner objective: useful model roles survive as distinct
// anchor-backed shared participation; no whole model fallback merely
// because unit/anchor kinds differ.
//
// Env (all required):
//
//	ARCHIVE9_CASDOOR_RUN / ARCHIVE9_CASDOOR_CLONE
//	ARCHIVE9_ETCD_RUN / ARCHIVE9_ETCD_CLONE
//	ARCHIVE9_TELEBOT_RUN / ARCHIVE9_TELEBOT_CLONE
//	ARCHIVE9_CHATTO_RUN / ARCHIVE9_CHATTO_CLONE
//	ARCHIVE9_RESTIC_RUN / ARCHIVE9_RESTIC_CLONE
func TestArchive9ReplaySharedParticipation(t *testing.T) {
	repos := []struct {
		name  string
		run   string
		clone string
	}{
		{"casdoor", os.Getenv("ARCHIVE9_CASDOOR_RUN"), os.Getenv("ARCHIVE9_CASDOOR_CLONE")},
		{"etcd", os.Getenv("ARCHIVE9_ETCD_RUN"), os.Getenv("ARCHIVE9_ETCD_CLONE")},
		{"telebot", os.Getenv("ARCHIVE9_TELEBOT_RUN"), os.Getenv("ARCHIVE9_TELEBOT_CLONE")},
		{"chatto", os.Getenv("ARCHIVE9_CHATTO_RUN"), os.Getenv("ARCHIVE9_CHATTO_CLONE")},
		{"restic", os.Getenv("ARCHIVE9_RESTIC_RUN"), os.Getenv("ARCHIVE9_RESTIC_CLONE")},
	}
	for _, repo := range repos {
		repo := repo
		t.Run(repo.name, func(t *testing.T) {
			if repo.run == "" || repo.clone == "" {
				t.Skipf("ARCHIVE9_%s_RUN/CLONE not set", strings.ToUpper(repo.name))
			}
			seed, err := report.ReadRunManifestAuthoritySeed(repo.run)
			if err != nil {
				t.Fatalf("read authority seed: %v", err)
			}
			ctx := context.Background()
			before, err := freshness.CaptureRepository(ctx, repo.clone)
			if err != nil {
				t.Fatalf("capture repo before: %v", err)
			}
			after, err := freshness.CaptureRepository(ctx, repo.clone)
			if err != nil {
				t.Fatalf("capture repo after: %v", err)
			}
			authority, err := report.ConfirmRunAuthorityScoped(
				ctx, repo.clone, before, after, seed.CapturedInputPaths, true,
			)
			if err != nil {
				t.Fatalf("confirm authority: %v", err)
			}
			// Archive 9 records were saved with the D230 prompt v18 bytes,
			// while this HEAD deliberately restores v17 (48dd3f3). The
			// saved synthesis record therefore fails replay closed —
			// exactly the versioned-identity invariant. For a RAW
			// response replay we read the run directory WITHOUT the saved
			// synthesis projection: copy to a temp dir and drop the
			// synthesis artifacts so readRunDir skips replay.
			readDir := t.TempDir()
			if err := copyRunDirForReplay(repo.run, readDir); err != nil {
				t.Fatalf("stage run dir: %v", err)
			}
			_ = os.Remove(filepath.Join(readDir, "architecture_synthesis.json"))
			_ = os.Remove(filepath.Join(readDir, "architecture_synthesis_status.json"))
			data, err := report.ReadRunDirForAuthorizedArchitecture(readDir, authority)
			if err != nil {
				t.Fatalf("read run dir: %v", err)
			}
			bundle, err := report.BuildArchitectureCanvasInput(data)
			if err != nil {
				t.Fatalf("build input: %v", err)
			}
			revision := "archive9-" + repo.name + "-replay"
			if data.ModelResearch != nil {
				revision = data.ModelResearch.Repository.Revision + ":" + repo.name + "-replay"
			}
			exchanges := filepath.Join(repo.run, "semantic_exchanges")
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
					var envelope struct {
						OriginalBytes int    `json:"original_bytes"`
						Storage       string `json:"storage"`
					}
					if json.Unmarshal(saved, &envelope) == nil && envelope.Storage == "raw_content" && envelope.OriginalBytes > 0 {
						var candidate json.RawMessage
						if json.Unmarshal(saved, &candidate) == nil && !json.Valid(candidate) {
							saved = raw
						}
					}
					break
				}
			}
			if len(saved) == 0 {
				t.Fatal("no saved architecture synthesis response found")
			}
			result, err := componentmap.RecordSynthesisResponse(bundle.CandidateBundle, revision, repo.name+"-replay", "deepseek", 0, saved)
			if err != nil {
				t.Fatalf("replay: %v", err)
			}
			t.Logf("outcome=%s fallback=%v subsystems=%d memberships=%d",
				result.Landscape.ValidationOutcome, result.Landscape.Fallback,
				len(result.Landscape.Subsystems), len(result.Landscape.ConceptualMemberships))
			for _, d := range result.Landscape.Diagnostics {
				if strings.HasPrefix(d.Code, "proposal.") {
					t.Logf("  diagnostic: %s severity=%s", d.Code, d.Severity)
				}
			}
			if result.Landscape.Fallback {
				t.Errorf("whole model fallback fired; roles must survive as shared participation: %v", result.Landscape.FallbackReason)
			}
			for _, subsystem := range result.Landscape.Subsystems {
				for _, c := range subsystem.Components {
					t.Logf("  component: %s members=%d shared=%d sharedUnits=%v anchors=%d",
						c.Name, len(c.Members), len(c.SharedMemberIDs), c.SharedUnitRefs, len(c.AnchorIDs))
				}
			}
		})
	}
}
