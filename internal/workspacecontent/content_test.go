package workspacecontent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
)

func TestReadAuthorizedExactRangePreservesPhysicalText(t *testing.T) {
	t.Parallel()

	content := "package café\r\n\r\nfunc Run() {\r\n\tprintln(\"привет\")\r\n}\r\n"
	service := testService(t, testScope{
		files: map[string][]byte{"service/main.go": []byte(content)},
	})
	result, err := service.Read(context.Background(), Request{
		Path:  "service/main.go",
		Range: Range{StartLine: 2, EndLine: 4, FocusLine: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantLines := []Line{
		{Number: 1, Text: "package café"},
		{Number: 2, Text: ""},
		{Number: 3, Text: "func Run() {"},
		{Number: 4, Text: "\tprintln(\"привет\")"},
		{Number: 5, Text: "}"},
	}
	if result.Path != "service/main.go" || result.StartLine != 1 || result.EndLine != 5 ||
		result.FocusLine != 3 || result.TotalLines != 5 ||
		!reflect.DeepEqual(result.Lines, wantLines) {
		t.Fatalf("Read() = %#v", result)
	}
	if result.Text != strings.TrimSuffix(content, "\n") ||
		result.Truncated || result.StopReason != StopEndOfFile {
		t.Fatalf("text/range metadata = %#v", result)
	}
	if len(result.ContentSHA256) != 64 || filepath.IsAbs(result.Path) {
		t.Fatalf("unsafe result authority = %#v", result)
	}
	if cap(result.Lines) != len(result.Lines) {
		t.Fatalf("line capacity = %d, want %d", cap(result.Lines), len(result.Lines))
	}
}

func TestSelectRangePreservesSourceContextWindows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		request   Range
		lineCount int
		wantStart int
		wantEnd   int
		wantFocus int
		wantOK    bool
	}{
		{
			name: "short range gains ten lines of context", request: Range{41, 45, 43},
			lineCount: 120, wantStart: 31, wantEnd: 55, wantFocus: 43, wantOK: true,
		},
		{
			name: "wide range is centered on focus", request: Range{10, 100, 80},
			lineCount: 200, wantStart: 60, wantEnd: 139, wantFocus: 80, wantOK: true,
		},
		{
			name: "out of range focus falls back to start", request: Range{10, 100, 150},
			lineCount: 200, wantStart: 1, wantEnd: 80, wantFocus: 10, wantOK: true,
		},
		{
			name: "range clips at end of file", request: Range{110, 140, 120},
			lineCount: 120, wantStart: 100, wantEnd: 120, wantFocus: 120, wantOK: true,
		},
		{
			name: "saved range starts beyond file", request: Range{121, 130, 121},
			lineCount: 120, wantOK: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			start, end, focus, _, ok := selectRange(test.request, test.lineCount, maxLines)
			if ok != test.wantOK || start != test.wantStart || end != test.wantEnd ||
				focus != test.wantFocus {
				t.Fatalf(
					"selectRange() = (%d,%d,%d,%t), want (%d,%d,%d,%t)",
					start, end, focus, ok,
					test.wantStart, test.wantEnd, test.wantFocus, test.wantOK,
				)
			}
			if ok && end-start+1 > maxLines {
				t.Fatalf("selected %d lines, maximum %d", end-start+1, maxLines)
			}
		})
	}
}

func TestReadLineEndingsFinalLineAndControls(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		content     string
		endLine     int
		wantLines   []Line
		wantRawText string
		wantTotal   int
		wantStop    StopReason
		wantErrKind ErrorKind
	}{
		{
			name: "LF final newline", content: "one\ntwo\n", endLine: 2,
			wantLines:   []Line{{Number: 1, Text: "one"}, {Number: 2, Text: "two"}},
			wantRawText: "one\ntwo", wantTotal: 2, wantStop: StopEndOfFile,
		},
		{
			name: "no final newline", content: "one\ntwo", endLine: 2,
			wantLines:   []Line{{Number: 1, Text: "one"}, {Number: 2, Text: "two"}},
			wantRawText: "one\ntwo", wantTotal: 2, wantStop: StopEndOfFile,
		},
		{
			name: "empty final physical line remains", content: "one\n\n", endLine: 2,
			wantLines:   []Line{{Number: 1, Text: "one"}, {Number: 2, Text: ""}},
			wantRawText: "one\n", wantTotal: 2, wantStop: StopEndOfFile,
		},
		{
			name: "valid UTF-8 controls remain exact", content: "a\x00b\tc\n", endLine: 1,
			wantLines:   []Line{{Number: 1, Text: "a\x00b\tc"}},
			wantRawText: "a\x00b\tc", wantTotal: 1, wantStop: StopEndOfFile,
		},
		{
			name: "empty file has no location", content: "", endLine: 1,
			wantErrKind: ErrorUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service := testService(t, testScope{
				files: map[string][]byte{"text.txt": []byte(test.content)},
			})
			result, err := service.Read(context.Background(), Request{
				Path: "text.txt", Range: Range{StartLine: 1, EndLine: test.endLine, FocusLine: 1},
			})
			if got := ErrorKindOf(err); got != test.wantErrKind {
				t.Fatalf("ErrorKindOf(Read()) = %q, want %q (err=%v)", got, test.wantErrKind, err)
			}
			if test.wantErrKind != "" {
				return
			}
			if !reflect.DeepEqual(result.Lines, test.wantLines) ||
				result.Text != test.wantRawText || result.TotalLines != test.wantTotal ||
				result.StopReason != test.wantStop {
				t.Fatalf("Read() = %#v", result)
			}
		})
	}
}

func TestReadRejectsInvalidOrUnauthorizedRequestsBeforeRead(t *testing.T) {
	t.Parallel()

	service := testService(t, testScope{
		files: map[string][]byte{"safe.txt": []byte("safe\n")},
	})
	tests := []struct {
		name    string
		request Request
		kind    ErrorKind
	}{
		{name: "absolute", request: Request{Path: "/safe.txt", Range: Range{1, 1, 1}}, kind: ErrorInvalidRequest},
		{name: "traversal", request: Request{Path: "../safe.txt", Range: Range{1, 1, 1}}, kind: ErrorInvalidRequest},
		{name: "backslash", request: Request{Path: `dir\safe.txt`, Range: Range{1, 1, 1}}, kind: ErrorInvalidRequest},
		{name: "dot alias", request: Request{Path: "./safe.txt", Range: Range{1, 1, 1}}, kind: ErrorInvalidRequest},
		{name: "control path", request: Request{Path: "safe\x00.txt", Range: Range{1, 1, 1}}, kind: ErrorInvalidRequest},
		{name: "oversized path", request: Request{Path: strings.Repeat("a", maxPathBytes+1), Range: Range{1, 1, 1}}, kind: ErrorInvalidRequest},
		{name: "unauthorized", request: Request{Path: "other.txt", Range: Range{1, 1, 1}}, kind: ErrorUnauthorized},
		{name: "zero start", request: Request{Path: "safe.txt", Range: Range{0, 1, 0}}, kind: ErrorInvalidRequest},
		{name: "reversed", request: Request{Path: "safe.txt", Range: Range{2, 1, 1}}, kind: ErrorInvalidRequest},
		{name: "overflow-like end", request: Request{Path: "safe.txt", Range: Range{1, maxLineValue + 1, 1}}, kind: ErrorInvalidRequest},
		{name: "negative focus", request: Request{Path: "safe.txt", Range: Range{1, 1, -1}}, kind: ErrorInvalidRequest},
		{name: "invalid limit", request: Request{
			Path: "safe.txt", Range: Range{1, 1, 1}, Limits: Limits{MaxLines: maxLines + 1},
		}, kind: ErrorInvalidRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result, err := service.Read(context.Background(), test.request)
			if got := ErrorKindOf(err); got != test.kind {
				t.Fatalf("ErrorKindOf(Read()) = %q, want %q (err=%v)", got, test.kind, err)
			}
			if !reflect.DeepEqual(result, Result{}) {
				t.Fatalf("failed result = %#v", result)
			}
			if err != nil && (strings.Contains(err.Error(), service.catalog.AnalysisRoot()) ||
				strings.Contains(err.Error(), test.request.Path)) {
				t.Fatalf("error leaked request or root: %q", err)
			}
		})
	}
}

func TestReadFailsClosedForChangedMissingSymlinkAndUnsupportedContent(t *testing.T) {
	t.Parallel()

	t.Run("changed bytes", func(t *testing.T) {
		t.Parallel()
		scope := testScope{files: map[string][]byte{"source.go": []byte("package original\n")}}
		service := testService(t, scope)
		if err := os.WriteFile(
			filepath.Join(service.catalog.AnalysisRoot(), "source.go"),
			[]byte("package changed\nDO_NOT_LEAK\n"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		_, err := service.Read(context.Background(), Request{
			Path: "source.go", Range: Range{1, 1, 1},
		})
		if ErrorKindOf(err) != ErrorSourceChanged ||
			strings.Contains(err.Error(), "DO_NOT_LEAK") ||
			strings.Contains(err.Error(), service.catalog.AnalysisRoot()) {
			t.Fatalf("changed-source error = %q", err)
		}
	})

	t.Run("captured SHA mismatch", func(t *testing.T) {
		t.Parallel()
		service := testService(t, testScope{
			files:        map[string][]byte{"source.go": []byte("package source\n")},
			capturedHash: map[string]string{"source.go": strings.Repeat("a", 64)},
		})
		_, err := service.Read(context.Background(), Request{
			Path: "source.go", Range: Range{1, 1, 1},
		})
		if ErrorKindOf(err) != ErrorSourceChanged {
			t.Fatalf("captured mismatch error = %v", err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		service := testService(t, testScope{
			files: map[string][]byte{"source.go": []byte("package source\n")},
		})
		if err := os.Remove(filepath.Join(service.catalog.AnalysisRoot(), "source.go")); err != nil {
			t.Fatal(err)
		}
		_, err := service.Read(context.Background(), Request{
			Path: "source.go", Range: Range{1, 1, 1},
		})
		if ErrorKindOf(err) != ErrorUnavailable {
			t.Fatalf("missing error = %v", err)
		}
	})

	t.Run("invalid UTF-8 outside selected line", func(t *testing.T) {
		t.Parallel()
		service := testService(t, testScope{
			files: map[string][]byte{"source.go": {'o', 'k', '\n', 0xff, '\n'}},
		})
		_, err := service.Read(context.Background(), Request{
			Path: "source.go", Range: Range{1, 1, 1},
		})
		if ErrorKindOf(err) != ErrorUnsupportedText {
			t.Fatalf("invalid UTF-8 error = %v", err)
		}
	})

	t.Run("final symlink replacement", func(t *testing.T) {
		t.Parallel()
		service := testService(t, testScope{
			files: map[string][]byte{
				"source.go": []byte("package source\n"),
				"other.go":  []byte("package source\n"),
			},
		})
		sourcePath := filepath.Join(service.catalog.AnalysisRoot(), "source.go")
		if err := os.Remove(sourcePath); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("other.go", sourcePath); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		_, err := service.Read(context.Background(), Request{
			Path: "source.go", Range: Range{1, 1, 1},
		})
		if ErrorKindOf(err) != ErrorUnavailable {
			t.Fatalf("symlink replacement error = %v", err)
		}
	})

	t.Run("intermediate symlink", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		realDir := filepath.Join(root, "real")
		if err := os.Mkdir(realDir, 0o700); err != nil {
			t.Fatal(err)
		}
		raw := []byte("content\n")
		if err := os.WriteFile(filepath.Join(realDir, "text.txt"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("real", filepath.Join(root, "alias")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		catalog := testCatalog(t, root, root, map[string][]byte{"alias/text.txt": raw}, nil)
		service, err := New(catalog)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = service.Close() })
		_, err = service.Read(context.Background(), Request{
			Path: "alias/text.txt", Range: Range{1, 1, 1},
		})
		if ErrorKindOf(err) != ErrorUnavailable {
			t.Fatalf("intermediate symlink error = %v", err)
		}
	})
}

func TestReadEnforcesEveryOperationalLimitBeforeOutputGrowth(t *testing.T) {
	t.Parallel()

	var manyLines strings.Builder
	for line := 1; line <= 100; line++ {
		fmt.Fprintf(&manyLines, "line-%03d\n", line)
	}
	service := testService(t, testScope{
		files: map[string][]byte{
			"many.txt": []byte(manyLines.String()),
			"long.txt": []byte("abcdef\n"),
		},
	})

	result, err := service.Read(context.Background(), Request{
		Path: "many.txt", Range: Range{StartLine: 1, EndLine: 100, FocusLine: 50},
		Limits: Limits{MaxLines: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 3 || cap(result.Lines) != 3 || !result.Truncated ||
		result.StopReason != StopLineLimit || result.FocusLine < result.StartLine ||
		result.FocusLine > result.EndLine {
		t.Fatalf("line-limited result = %#v", result)
	}

	tests := []struct {
		name  string
		limit Limits
		want  LimitKind
	}{
		{name: "file bytes", limit: Limits{MaxFileBytes: 5}, want: LimitFile},
		{name: "selected bytes", limit: Limits{MaxBytes: 5}, want: LimitText},
		{name: "physical line bytes", limit: Limits{MaxLineBytes: 5}, want: LimitLine},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, readErr := service.Read(context.Background(), Request{
				Path: "long.txt", Range: Range{1, 1, 1}, Limits: test.limit,
			})
			if ErrorKindOf(readErr) != ErrorLimitExceeded || LimitKindOf(readErr) != test.want {
				t.Fatalf("limit error = %v kind=%q limit=%q", readErr, ErrorKindOf(readErr), LimitKindOf(readErr))
			}
		})
	}

	exact, err := service.Read(context.Background(), Request{
		Path: "long.txt", Range: Range{1, 1, 1}, Limits: Limits{MaxFileBytes: 7},
	})
	if err != nil || exact.Text != "abcdef" {
		t.Fatalf("exact file bound result=%#v err=%v", exact, err)
	}
}

func TestReadIsCancellationAwareAndReturnsDefensiveResults(t *testing.T) {
	t.Parallel()

	service := testService(t, testScope{
		files: map[string][]byte{"safe.txt": []byte("one\ntwo\n")},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Read(ctx, Request{
		Path: "safe.txt", Range: Range{1, 2, 1},
	}); ErrorKindOf(err) != ErrorCanceled {
		t.Fatalf("canceled error = %v", err)
	}
	if _, err := service.Read(nil, Request{
		Path: "safe.txt", Range: Range{1, 2, 1},
	}); ErrorKindOf(err) != ErrorInvalidRequest {
		t.Fatalf("nil context error = %v", err)
	}

	first, err := service.Read(context.Background(), Request{
		Path: "safe.txt", Range: Range{1, 2, 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	first.Lines[0].Text = "mutated"
	first.Path = "/absolute"
	second, err := service.Read(context.Background(), Request{
		Path: "safe.txt", Range: Range{1, 2, 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Path != "safe.txt" || second.Lines[0].Text != "one" {
		t.Fatalf("result was not defensive: %#v", second)
	}
}

func TestServiceKeepsDescriptorBoundRootAfterPathReplacement(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	root := filepath.Join(parent, "repo")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte("original\n")
	if err := os.WriteFile(filepath.Join(root, "text.txt"), original, 0o600); err != nil {
		t.Fatal(err)
	}
	catalog := testCatalog(t, root, root, map[string][]byte{"text.txt": original}, nil)
	service, err := New(catalog)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })

	heldRoot := filepath.Join(parent, "held")
	if err := os.Rename(root, heldRoot); err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(parent, "replacement")
	if err := os.Mkdir(replacement, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(replacement, "text.txt"), original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("replacement", root); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	result, err := service.Read(context.Background(), Request{
		Path: "text.txt", Range: Range{1, 1, 1},
	})
	if err != nil || result.Text != "original" {
		t.Fatalf("descriptor-bound read = %#v, %v", result, err)
	}
}

func TestNewRejectsMissingOrEmptyCatalogWithoutLeakingRoot(t *testing.T) {
	t.Parallel()

	if _, err := New(sourcecatalog.Catalog{}); ErrorKindOf(err) != ErrorInvalidRequest {
		t.Fatalf("empty catalog error = %v", err)
	}
	root := filepath.Join(t.TempDir(), "missing-root")
	catalog := testCatalog(t, root, root, map[string][]byte{"text.txt": []byte("x\n")}, nil)
	if _, err := New(catalog); ErrorKindOf(err) != ErrorUnavailable ||
		strings.Contains(err.Error(), root) {
		t.Fatalf("missing-root error = %q", err)
	}
}

type testScope struct {
	files        map[string][]byte
	capturedHash map[string]string
}

func testService(t *testing.T, scope testScope) *Service {
	t.Helper()
	root := t.TempDir()
	for name, content := range scope.files {
		filePath := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filePath, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	catalog := testCatalog(t, root, root, scope.files, scope.capturedHash)
	service, err := New(catalog)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Error(err)
		}
	})
	return service
}

func testCatalog(
	t *testing.T,
	repositoryRoot, analysisRoot string,
	files map[string][]byte,
	capturedHash map[string]string,
) sourcecatalog.Catalog {
	t.Helper()
	allowed := make([]string, 0, len(files))
	inputs := make([]freshness.CapturedInput, 0, len(files))
	for sourcePath, content := range files {
		allowed = append(allowed, sourcePath)
		repositoryPath, err := filepath.Rel(repositoryRoot, filepath.Join(analysisRoot, filepath.FromSlash(sourcePath)))
		if err != nil {
			t.Fatal(err)
		}
		repositoryPath = filepath.ToSlash(repositoryPath)
		contentHash := fmt.Sprintf("%x", sha256.Sum256(content))
		if capturedHash[sourcePath] != "" {
			contentHash = capturedHash[sourcePath]
		}
		id := sha256.Sum256([]byte("workspacecontent-test\x00" + repositoryPath))
		inputs = append(inputs, freshness.CapturedInput{
			Version:       freshness.CapturedInputVersion,
			ID:            fmt.Sprintf("%x", id[:]),
			Path:          repositoryPath,
			Kind:          freshness.FileRegular,
			Mode:          "100644",
			ContentSHA256: contentHash,
			Stages:        []string{"report_evidence"},
		})
	}
	catalog, err := sourcecatalog.New(sourcecatalog.Input{
		RepositoryRoot: repositoryRoot,
		AnalysisRoot:   analysisRoot,
		AllowedPaths:   allowed,
		CapturedInputs: inputs,
	})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}
