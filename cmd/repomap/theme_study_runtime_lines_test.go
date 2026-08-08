package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/themestudy"
)

// Regression: statik-style generated files contain one multi-megabyte line
// (embedded base64 payload). countFileLines and readFileLines must handle
// them — a bufio.Scanner token-size ceiling must never surface as a run
// failure (live ghz run: "expand .../statik.go: total lines: bufio.Scanner:
// token too long").
func TestCountAndReadFileLinesHandleHugeSingleLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "statik.go")

	// One line far beyond the old Scanner buffer ceiling (1 MiB).
	payload := strings.Repeat("AQIDBA==", 256*1024) // 2 MiB single line
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}

	count, err := countFileLines(path)
	if err != nil {
		t.Fatalf("countFileLines: %v", err)
	}
	if count != 1 {
		t.Fatalf("countFileLines = %d, want 1 (single huge line)", count)
	}

	lines, err := readFileLines(path, 1, 1)
	if err != nil {
		t.Fatalf("readFileLines: %v", err)
	}
	if len(lines) != 1 || lines[0] != payload {
		t.Fatalf("readFileLines returned %d lines or wrong content", len(lines))
	}
}

// countFileLines must agree with the physical line count for normal files.
func TestCountFileLinesMatchesPhysicalLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plain.go")
	content := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	count, err := countFileLines(path)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("countFileLines = %d, want 3", count)
	}

	// Windowed read: lines 2..3 (empty line and func line).
	lines, err := readFileLines(path, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0] != "" || lines[1] != "func main() {}" {
		t.Fatalf("readFileLines(2,3) = %#v", lines)
	}
}

func TestPrepareThemeSourceExpansionClosesHugeOrUnreadableSiblingAndContinues(t *testing.T) {
	root := t.TempDir()
	acceptedPath := filepath.Join(root, "accepted.go")
	if err := os.WriteFile(acceptedPath, []byte("package accepted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hugePayload := strings.Repeat("AQIDBA==", 256*1024) // 2 MiB, one physical line.
	hugePath := filepath.Join(root, "huge.go")
	if err := os.WriteFile(hugePath, []byte(hugePayload), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name       string
		problem    string
		wantReason string
	}{
		{name: "huge single line", problem: "huge.go", wantReason: themestudy.ExpansionClosedReasonOversized},
		{name: "unreadable file", problem: "missing.go", wantReason: themestudy.ExpansionClosedReasonUnreadable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runDir := t.TempDir()
			writer, err := debugdump.OpenWriter(runDir, false)
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Close()

			var console bytes.Buffer
			expansion, err := prepareThemeSourceExpansionForRun(
				[]themestudy.FileRef{
					{Ref: "f1", Path: tc.problem},
					{Ref: "f2", Path: "accepted.go"},
				},
				[]string{"f1", "f2"},
				themeExpansionSourceReader(root), themeTotalLines(root), writer, newRunOutput(&console),
			)
			if err != nil {
				t.Fatalf("item-local source closure terminated the report pipeline: %v", err)
			}
			if len(expansion.Files) != 2 {
				t.Fatalf("files = %#v, want one closed source plus one accepted sibling", expansion.Files)
			}
			closed, accepted := expansion.Files[0], expansion.Files[1]
			if !closed.Closed || closed.ClosedReason != tc.wantReason ||
				len(closed.Objects) != 0 || closed.ExpandedLines != 0 {
				t.Fatalf("closed source = %#v", closed)
			}
			if accepted.Closed || accepted.Ref != "f2" || len(accepted.Objects) != 1 ||
				len(accepted.Objects[0].Lines) != 1 || accepted.Objects[0].Lines[0] != "package accepted" {
				t.Fatalf("accepted sibling = %#v", accepted)
			}
			if len(expansion.OmittedRefs) != 1 || expansion.OmittedRefs[0] != "f1" {
				t.Fatalf("omitted refs = %#v, want [f1]", expansion.OmittedRefs)
			}

			persisted, err := os.ReadFile(filepath.Join(runDir, themestudy.ExpansionArtifactFilename))
			if err != nil {
				t.Fatalf("read persisted expansion: %v", err)
			}
			if _, err := themestudy.DecodeExpansion(persisted); err != nil {
				t.Fatalf("persisted expansion integrity: %v", err)
			}
			gotConsole := console.String()
			if strings.Count(gotConsole, "WARN") != 1 ||
				!strings.Contains(gotConsole, "source files skipped: 1") ||
				!strings.Contains(gotConsole, "remaining exact source context continues") {
				t.Fatalf("console warning = %q", gotConsole)
			}
			if strings.Contains(gotConsole, tc.problem) || strings.Contains(gotConsole, hugePayload[:64]) {
				t.Fatalf("console warning leaked path or source prose: %q", gotConsole)
			}
		})
	}
}

// A trailing newline must not add a phantom empty last line.
func TestCountFileLinesTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trailing.go")
	if err := os.WriteFile(path, []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	count, err := countFileLines(path)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("countFileLines = %d, want 2", count)
	}
}
