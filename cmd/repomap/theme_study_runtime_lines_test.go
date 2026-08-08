package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
