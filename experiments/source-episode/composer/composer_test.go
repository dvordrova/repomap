package composer

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

var (
	localAbsolutePathPattern = regexp.MustCompile(`(?:^|[\s"'=(\[])(?:/Users/|/home/|[A-Za-z]:[\\/])`)
	htmlCommentPattern       = regexp.MustCompile(`(?s)<!--.*?-->`)
	pinnedSourcePattern      = regexp.MustCompile(`https://github\.com/etcd-io/etcd/blob/([0-9a-f]{40})/`)
)

func TestComposeMatchesReviewedArtifact(t *testing.T) {
	episodeJSON := readFixture(t, "../etcd-put/episode.json")
	projectionJSON := readFixture(t, "../etcd-put/where-to-change.projection.json")
	want := readFixture(t, "../etcd-put/where-to-change.md")

	got, err := Compose(episodeJSON, projectionJSON)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	again, err := Compose(episodeJSON, projectionJSON)
	if err != nil {
		t.Fatalf("second Compose() error = %v", err)
	}
	if !bytes.Equal(got, again) {
		t.Fatal("Compose() is not deterministic")
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Compose() differs from reviewed artifact at byte %d\n--- got ---\n%s\n--- want ---\n%s", firstDifference(got, want), got, want)
	}
}

func TestComposePreservesWeakSignals(t *testing.T) {
	got := composeFixtures(t)

	for _, required := range []string{
		"## Short answer — INFERRED",
		"## Where to change — INFERRED",
		"## What behavior changes — INFERRED",
		"committed_consistent_index >= applied_entry_index",
		"**UNKNOWN** — Client acknowledgment and this Ready loop's WAL Save completion are not ordered here.",
		"shared work, not a per-Put callback",
		"design seam, not a ready-made edit",
	} {
		if !bytes.Contains(got, []byte(required)) {
			t.Errorf("rendered projection is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"## Short answer — CORROBORATED",
		"## What behavior changes — CORROBORATED",
		"move `s.w.Trigger` into",
		"make the Put durable",
		"Evidence IDs:",
	} {
		if bytes.Contains(got, []byte(forbidden)) {
			t.Errorf("rendered projection contains forbidden wording %q", forbidden)
		}
	}

	readerVisible := htmlCommentPattern.ReplaceAll(got, nil)
	for _, internalID := range []string{"claim-", "fact-", "flow-node-", "anchor-"} {
		if bytes.Contains(readerVisible, []byte(internalID)) {
			t.Errorf("reader-visible output leaks internal ID prefix %q", internalID)
		}
	}
}

func TestComposeRejectsSemanticDrift(t *testing.T) {
	episodeJSON := readFixture(t, "../etcd-put/episode.json")
	projectionJSON := readFixture(t, "../etcd-put/where-to-change.projection.json")

	tests := []struct {
		name        string
		old         string
		replacement string
		wantError   string
	}{
		{
			name:        "promoted design",
			old:         `"state": "inferred"`,
			replacement: `"state": "corroborated"`,
			wantError:   "must remain inferred",
		},
		{
			name:        "promoted behavior change",
			old:         `"behavior_change_state": "inferred"`,
			replacement: `"behavior_change_state": "corroborated"`,
			wantError:   "must remain inferred",
		},
		{
			name:        "missing watermark correlation",
			old:         `"committed_consistent_index >= applied_entry_index"`,
			replacement: `"request_id == committed_batch_id"`,
			wantError:   "correlation predicate",
		},
		{
			name:        "precommit mistaken for postcommit",
			old:         `"post_commit_anchor_id": "anchor-backend-bbolt-commit"`,
			replacement: `"post_commit_anchor_id": "anchor-backend-saves-consistent-index"`,
			wantError:   "two-ended design",
		},
		{
			name:        "unknown claim",
			old:         `"claim-success-waits-for-local-apply"`,
			replacement: `"claim-does-not-exist"`,
			wantError:   "unknown ID",
		},
		{
			name:        "unknown fact",
			old:         `"fact-apply-result-triggers-waiter"`,
			replacement: `"fact-does-not-exist"`,
			wantError:   "unknown ID",
		},
		{
			name:        "unknown flow node",
			old:         `"flow-node-mvcc-visible"`,
			replacement: `"flow-node-does-not-exist"`,
			wantError:   "unknown ID",
		},
		{
			name:        "unknown anchor",
			old:         `"anchor-mvcc-revision-publication"`,
			replacement: `"anchor-does-not-exist"`,
			wantError:   "unknown ID",
		},
		{
			name:        "known fact outside selected claim support",
			old:         `"fact-backend-commits-periodically"`,
			replacement: `"fact-unstable-entries-feed-wal"`,
			wantError:   "outside selected claim support",
		},
		{
			name:        "unknown limit promoted",
			old:         "\"remaining_limits\": {\n    \"state\": \"unknown\"",
			replacement: "\"remaining_limits\": {\n    \"state\": \"inferred\"",
			wantError:   "must remain unknown",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := replaceOnce(t, projectionJSON, test.old, test.replacement)
			_, err := Compose(episodeJSON, mutated)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("Compose() error = %v, want substring %q", err, test.wantError)
			}
		})
	}
}

func TestComposeRejectsInvalidInputBoundary(t *testing.T) {
	episodeJSON := readFixture(t, "../etcd-put/episode.json")
	projectionJSON := readFixture(t, "../etcd-put/where-to-change.projection.json")

	t.Run("episode drift", func(t *testing.T) {
		drifted := append(append([]byte(nil), episodeJSON...), '\n')
		_, err := Compose(drifted, projectionJSON)
		if err == nil || !strings.Contains(err.Error(), "episode SHA-256") {
			t.Fatalf("Compose() error = %v, want episode SHA-256 mismatch", err)
		}
	})

	t.Run("projection unknown field", func(t *testing.T) {
		mutated := replaceOnce(
			t,
			projectionJSON,
			`"artifact_version": "1",`,
			`"artifact_version": "1",`+"\n  "+`"source_path": "/tmp/not-allowed",`,
		)
		_, err := Compose(episodeJSON, mutated)
		if err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("Compose() error = %v, want unknown field rejection", err)
		}
	})

	t.Run("projection trailing JSON", func(t *testing.T) {
		mutated := append(append([]byte(nil), projectionJSON...), []byte("{}")...)
		_, err := Compose(episodeJSON, mutated)
		if err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
			t.Fatalf("Compose() error = %v, want trailing JSON rejection", err)
		}
	})

	t.Run("oversized episode rejected before decode", func(t *testing.T) {
		_, err := Compose(bytes.Repeat([]byte("x"), maxEpisodeBytes+1), projectionJSON)
		if err == nil || !strings.Contains(err.Error(), "episode input exceeds") {
			t.Fatalf("Compose() error = %v, want episode byte budget rejection", err)
		}
	})

	t.Run("oversized projection rejected before decode", func(t *testing.T) {
		_, err := Compose(episodeJSON, bytes.Repeat([]byte("x"), maxProjectionBytes+1))
		if err == nil || !strings.Contains(err.Error(), "projection input exceeds") {
			t.Fatalf("Compose() error = %v, want projection byte budget rejection", err)
		}
	})
}

func TestComposeUsesOnlyPinnedEpisodeLocations(t *testing.T) {
	got := composeFixtures(t)

	if bytes.Contains(got, []byte("file://")) {
		t.Fatal("rendered projection contains a file URL")
	}
	if localAbsolutePathPattern.Match(got) {
		t.Fatalf("rendered projection contains a local absolute path: %s", localAbsolutePathPattern.Find(got))
	}

	matches := pinnedSourcePattern.FindAllSubmatch(got, -1)
	if len(matches) == 0 {
		t.Fatal("rendered projection has no pinned source links")
	}
	for _, match := range matches {
		if string(match[1]) != "58f45a9ff1c083130830eb02b0cc7d9783609095" {
			t.Errorf("rendered projection contains unpinned or wrong revision %q", match[1])
		}
	}

	projectionJSON := readFixture(t, "../etcd-put/where-to-change.projection.json")
	for _, forbiddenKey := range []string{`"path"`, `"url"`, `"source"`} {
		if bytes.Contains(projectionJSON, []byte(forbiddenKey)) {
			t.Errorf("question projection supplies forbidden location field %s", forbiddenKey)
		}
	}
}

func composeFixtures(t *testing.T) []byte {
	t.Helper()
	got, err := Compose(
		readFixture(t, "../etcd-put/episode.json"),
		readFixture(t, "../etcd-put/where-to-change.projection.json"),
	)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}
	return got
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return raw
}

func replaceOnce(t *testing.T, raw []byte, old, replacement string) []byte {
	t.Helper()
	if bytes.Count(raw, []byte(old)) == 0 {
		t.Fatalf("fixture does not contain %q", old)
	}
	return bytes.Replace(raw, []byte(old), []byte(replacement), 1)
}

func firstDifference(left, right []byte) int {
	limit := min(len(left), len(right))
	for i := 0; i < limit; i++ {
		if left[i] != right[i] {
			return i
		}
	}
	return limit
}
