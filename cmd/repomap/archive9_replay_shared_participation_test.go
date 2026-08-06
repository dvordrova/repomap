package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/navigator"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
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

// stageV2NavigatorStatus rewrites the staged navigator_status.v1.json with a
// freshly derived v2 status compiled from the run's own repository Atlas
// (Decision 232). The Archive 9 navigator records are v1 and fail closed
// under the v2 contract; the staged read needs an identity-consistent local
// navigator row (unavailable/prepared — no provider call) so the
// architecture synthesis replay is unaffected.
func stageV2NavigatorStatus(runDir string) error {
	atlasRaw, err := os.ReadFile(filepath.Join(runDir, "repository_atlas.v1.json"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // no Atlas: no navigator row either
		}
		return err
	}
	atlas, err := repositoryatlas.DecodeCanonicalJSON(atlasRaw)
	if err != nil {
		return fmt.Errorf("decode repository atlas: %w", err)
	}
	product, err := navigator.CompileProduct(navigator.ProductInput{
		Atlas: atlas,
		Limits: navigator.Limits{
			MaxWireBytes: 128 << 10, MaxResponseBytes: 128 << 10,
			MaxUnitLabelBytes: 512,
			MaxSeeds:          32, MaxDirectTrails: 64,
			MaxIntersections: 32, MaxEvidence: 128, MaxGaps: 32, MaxActions: 32,
		},
	})
	if err != nil {
		return fmt.Errorf("compile navigator product: %w", err)
	}
	var status navigator.Status
	if product.Empty() {
		status = product.PreparedStatus()
		// Empty local state requires its exact empty result artifact
		// (report read discipline), so stage it from the compiled product.
		record, err := product.EmptyRecord()
		if err != nil {
			return fmt.Errorf("navigator empty record: %w", err)
		}
		encodedResult, err := navigator.EncodeRecommendationRecord(record)
		if err != nil {
			return fmt.Errorf("encode navigator result: %w", err)
		}
		if err := os.WriteFile(filepath.Join(runDir, "navigator_result.v1.json"), encodedResult, 0o600); err != nil {
			return err
		}
	} else {
		status, err = product.UnavailableStatus(navigator.UnavailableOffline)
		if err != nil {
			return fmt.Errorf("navigator unavailable status: %w", err)
		}
		// Unavailable local state requires its exact request artifact and
		// must NOT carry a recommendation artifact (stale v1 result from
		// the archived run is dropped).
		_ = os.Remove(filepath.Join(runDir, "navigator_result.v1.json"))
		request, err := product.RequestRecord()
		if err != nil {
			return fmt.Errorf("navigator request record: %w", err)
		}
		encodedRequest, err := navigator.EncodeRequestRecord(request)
		if err != nil {
			return fmt.Errorf("encode navigator request: %w", err)
		}
		if err := os.WriteFile(filepath.Join(runDir, "navigator_request.v1.json"), encodedRequest, 0o600); err != nil {
			return err
		}
	}
	encoded, err := navigator.EncodeStatus(status)
	if err != nil {
		return fmt.Errorf("encode navigator status: %w", err)
	}
	return os.WriteFile(filepath.Join(runDir, "navigator_status.v1.json"), encoded, 0o600)
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
			// Decision 232 (Archive 9): Archive 9 navigator records are v1
			// and fail closed under the v2 contract (versioned-identity
			// invariant). The replay under test is the ARCHITECTURE
			// synthesis path, so the staged read replaces the v1 navigator
			// status with a freshly generated v2 status derived from the
			// SAME repository Atlas (identity-consistent: the navigator
			// row is an honest local state under the current contract, and
			// the architecture replay is unaffected). The v1 navigator
			// request/result artifacts are dropped — the v2 status has no
			// provider call.
			if err := stageV2NavigatorStatus(readDir); err != nil {
				t.Fatalf("stage v2 navigator status: %v", err)
			}
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
