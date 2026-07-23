package reporead

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReaderReadFileBoundsContent(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := New(repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	})

	tests := []struct {
		name              string
		limit             int64
		expected          string
		expectedTruncated bool
	}{
		{name: "prefix", limit: 3, expected: "abc", expectedTruncated: true},
		{name: "exact bound", limit: 6, expected: "abcdef", expectedTruncated: false},
		{name: "larger bound", limit: 10, expected: "abcdef", expectedTruncated: false},
		{name: "zero bound", limit: 0, expected: "", expectedTruncated: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			content, err := reader.ReadFile("README.md", test.limit)
			if err != nil {
				t.Fatal(err)
			}
			if string(content.Bytes) != test.expected {
				t.Fatalf("bytes = %q, want %q", content.Bytes, test.expected)
			}
			if content.Truncated != test.expectedTruncated {
				t.Fatalf("truncated = %v, want %v", content.Truncated, test.expectedTruncated)
			}
			if cap(content.Bytes) != len(content.Bytes) {
				t.Fatalf("byte capacity = %d, want %d", cap(content.Bytes), len(content.Bytes))
			}
		})
	}
}

func TestReaderReadFileRejectsUnsafeOrNonRegularPaths(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("readme"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := New(repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	})

	tests := []struct {
		name string
		path string
	}{
		{name: "empty", path: ""},
		{name: "root", path: "."},
		{name: "absolute", path: filepath.Join(repo, "README.md")},
		{name: "traversal", path: "../outside"},
		{name: "nested traversal", path: "dir/../../outside"},
		{name: "contained traversal", path: "dir/../README.md"},
		{name: "dot prefix", path: "./README.md"},
		{name: "directory", path: "dir"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := reader.ReadFile(test.path, 32); err == nil {
				t.Fatal("ReadFile() error = nil")
			}
		})
	}
}

func TestReaderReadFileRejectsSymlinkEscapeAndAllowsContainedSymlink(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	outsideDir := filepath.Join(parent, "outside")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outsideDir, 0o700); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(repo, "inside.go")
	if err := os.WriteFile(inside, []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(outsideDir, "outside.go")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("inside.go", filepath.Join(repo, "contained.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink("../outside/outside.go", filepath.Join(repo, "escape.go")); err != nil {
		t.Fatal(err)
	}

	reader, err := New(repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	})
	content, err := reader.ReadFile("contained.go", 32)
	if err != nil {
		t.Fatalf("contained symlink: %v", err)
	}
	if string(content.Bytes) != "inside" {
		t.Fatalf("contained symlink bytes = %q", content.Bytes)
	}
	if _, err := reader.ReadFile("escape.go", 32); err == nil {
		t.Fatal("ReadFile() accepted symlink escape")
	}
}

func TestNewRejectsFileAsRepositoryRoot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "repo")
	if err := os.WriteFile(path, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(path); err == nil {
		t.Fatal("New() error = nil")
	}
}

func TestReaderReadLineWindowsFindsMergedWindowsBeyond64KiB(t *testing.T) {
	t.Parallel()

	const totalLines = 8_000
	var source strings.Builder
	for line := 1; line <= totalLines; line++ {
		fmt.Fprintf(&source, "line-%05d payload\n", line)
	}
	raw := []byte(source.String())
	if len(raw) <= 64<<10 {
		t.Fatalf("fixture size = %d, want greater than 64 KiB", len(raw))
	}

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "large.txt"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	reader := newTestReader(t, repo)

	content, err := reader.ReadLineWindows("large.txt", WindowOptions{
		ScanBytes:   int64(len(raw)),
		RetainBytes: 1 << 10,
		Windows: []LineWindow{
			{Start: 7_003, End: 7_004},
			{Start: 7_000, End: 7_001},
			{Start: 7_001, End: 7_003},
			{Start: 7_002, End: 7_002},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	wantLines := make([]SourceLine, 0, 5)
	var wantRetained int64
	for line := 7_000; line <= 7_004; line++ {
		text := fmt.Sprintf("line-%05d payload", line)
		wantLines = append(wantLines, SourceLine{Number: line, Text: text})
		wantRetained += int64(len(text) + 1)
	}
	if !reflect.DeepEqual(content.Lines, wantLines) {
		t.Fatalf("lines = %#v, want %#v", content.Lines, wantLines)
	}
	if content.ScannedBytes != int64(len(raw)) {
		t.Fatalf("scanned bytes = %d, want %d", content.ScannedBytes, len(raw))
	}
	if content.RetainedBytes != wantRetained {
		t.Fatalf("retained bytes = %d, want %d", content.RetainedBytes, wantRetained)
	}
	if content.ScanTruncated || content.RetainTruncated {
		t.Fatalf(
			"unexpected truncation: scan=%v retain=%v",
			content.ScanTruncated,
			content.RetainTruncated,
		)
	}
	if !content.SourceTotalLinesExact || content.SourceTotalLines != totalLines {
		t.Fatalf(
			"source total = %d exact=%v, want %d exact=true",
			content.SourceTotalLines,
			content.SourceTotalLinesExact,
			totalLines,
		)
	}
}

func TestReaderReadLineWindowsDoesNotReturnLineCutByScanLimit(t *testing.T) {
	t.Parallel()

	const source = "one\ntwo\nthree\n"
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	reader := newTestReader(t, repo)

	content, err := reader.ReadLineWindows("source.txt", WindowOptions{
		ScanBytes:   int64(len("one\ntw")),
		RetainBytes: 64,
		Windows:     []LineWindow{{Start: 1, End: 3}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []SourceLine{{Number: 1, Text: "one"}}
	if !reflect.DeepEqual(content.Lines, want) {
		t.Fatalf("lines = %#v, want %#v", content.Lines, want)
	}
	if content.ScannedBytes != int64(len("one\ntw")) {
		t.Fatalf("scanned bytes = %d", content.ScannedBytes)
	}
	if !content.ScanTruncated {
		t.Fatal("scan truncated = false")
	}
	if content.RetainTruncated {
		t.Fatal("retain truncated = true for a scan-clipped line")
	}
	if content.SourceTotalLinesExact || content.SourceTotalLines != 0 {
		t.Fatalf(
			"source total = %d exact=%v, want 0 exact=false",
			content.SourceTotalLines,
			content.SourceTotalLinesExact,
		)
	}
}

func TestReaderReadLineWindowsRetainsOnlyWholeLinesWithinByteLimit(t *testing.T) {
	t.Parallel()

	const source = "oversized\nx\nokay"
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	reader := newTestReader(t, repo)

	content, err := reader.ReadLineWindows("source.txt", WindowOptions{
		ScanBytes:   int64(len(source)),
		RetainBytes: 6,
		Windows:     []LineWindow{{Start: 1, End: 3}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []SourceLine{
		{Number: 2, Text: "x"},
		{Number: 3, Text: "okay"},
	}
	if !reflect.DeepEqual(content.Lines, want) {
		t.Fatalf("lines = %#v, want %#v", content.Lines, want)
	}
	if content.RetainedBytes != 6 {
		t.Fatalf("retained bytes = %d, want 6", content.RetainedBytes)
	}
	if !content.RetainTruncated {
		t.Fatal("retain truncated = false")
	}
	if content.ScanTruncated {
		t.Fatal("scan truncated = true")
	}
	if !content.SourceTotalLinesExact || content.SourceTotalLines != 3 {
		t.Fatalf(
			"source total = %d exact=%v, want 3 exact=true",
			content.SourceTotalLines,
			content.SourceTotalLinesExact,
		)
	}
}

func TestReaderReadLineWindowsUsesPhysicalLinesAndCRLFText(t *testing.T) {
	t.Parallel()

	const source = "\nalpha\r\nomega"
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	reader := newTestReader(t, repo)

	content, err := reader.ReadLineWindows("source.txt", WindowOptions{
		ScanBytes:   int64(len(source)),
		RetainBytes: int64(len(source)),
		Windows:     []LineWindow{{Start: 1, End: 4}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []SourceLine{
		{Number: 1, Text: ""},
		{Number: 2, Text: "alpha"},
		{Number: 3, Text: "omega"},
	}
	if !reflect.DeepEqual(content.Lines, want) {
		t.Fatalf("lines = %#v, want %#v", content.Lines, want)
	}
	if content.RetainedBytes != int64(len(source)) {
		t.Fatalf("retained bytes = %d, want %d", content.RetainedBytes, len(source))
	}
	if !content.SourceTotalLinesExact || content.SourceTotalLines != 3 {
		t.Fatalf(
			"source total = %d exact=%v, want 3 exact=true",
			content.SourceTotalLines,
			content.SourceTotalLinesExact,
		)
	}
}

func TestReaderReadLineWindowsRejectsUnsafePaths(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	outsideDir := filepath.Join(parent, "outside")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outsideDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "inside.go"), []byte("inside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "outside.go"), []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(repo, "dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("inside.go", filepath.Join(repo, "contained.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink("../outside/outside.go", filepath.Join(repo, "escape.go")); err != nil {
		t.Fatal(err)
	}
	reader := newTestReader(t, repo)
	opts := WindowOptions{
		ScanBytes:   32,
		RetainBytes: 32,
		Windows:     []LineWindow{{Start: 1, End: 1}},
	}

	content, err := reader.ReadLineWindows("contained.go", opts)
	if err != nil {
		t.Fatalf("contained symlink: %v", err)
	}
	want := []SourceLine{{Number: 1, Text: "inside"}}
	if !reflect.DeepEqual(content.Lines, want) {
		t.Fatalf("contained symlink lines = %#v, want %#v", content.Lines, want)
	}
	for _, unsafePath := range []string{
		"", ".", "../outside", "./inside.go", "dir", "escape.go",
	} {
		if _, err := reader.ReadLineWindows(unsafePath, opts); err == nil {
			t.Errorf("ReadLineWindows(%q) error = nil", unsafePath)
		}
	}
}

func TestReaderReadLineWindowsRejectsInvalidBounds(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "source.txt"), []byte("source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader := newTestReader(t, repo)

	tests := []struct {
		name string
		opts WindowOptions
	}{
		{
			name: "negative scan limit",
			opts: WindowOptions{ScanBytes: -1, RetainBytes: 1},
		},
		{
			name: "overflowing scan limit",
			opts: WindowOptions{ScanBytes: maxReadBytes, RetainBytes: 1},
		},
		{
			name: "negative retained limit",
			opts: WindowOptions{ScanBytes: 1, RetainBytes: -1},
		},
		{
			name: "overflowing retained limit",
			opts: WindowOptions{ScanBytes: 1, RetainBytes: maxReadBytes},
		},
		{
			name: "zero start line",
			opts: WindowOptions{
				ScanBytes: 1, RetainBytes: 1,
				Windows: []LineWindow{{Start: 0, End: 1}},
			},
		},
		{
			name: "descending window",
			opts: WindowOptions{
				ScanBytes: 1, RetainBytes: 1,
				Windows: []LineWindow{{Start: 2, End: 1}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := reader.ReadLineWindows("source.txt", test.opts); err == nil {
				t.Fatal("ReadLineWindows() error = nil")
			}
		})
	}
}

func newTestReader(t *testing.T, repo string) *Reader {
	t.Helper()

	reader, err := New(repo)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reader.Close(); err != nil {
			t.Error(err)
		}
	})
	return reader
}
