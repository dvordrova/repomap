package reportserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	reportpkg "github.com/dvordrova/repomap/internal/report"
)

func TestSourceContextReturnsCapturedBoundedSource(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	sourceLines := numberedSourceLines(120)
	if err := os.WriteFile(
		filepath.Join(repo, "batch.go"),
		[]byte(strings.Join(sourceLines, "\n")+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	const runID = "20260716-100000-source-context"
	writeRun(t, runsDir, runID, repo, "saved report")
	snippet := sourceContextTestSnippet("batch.go", 41, sourceLines[40:45])
	manifest := rewriteRunSourceSnippets(t, runsDir, runID, []reportpkg.SourceSnippet{snippet})
	server, baseURL := newSourceContextTestServer(t, runsDir, runID)
	defer server.Close()

	response := postSourceContext(t, baseURL, sourceContextRequest{
		RunID:     runID,
		ContextID: manifestSourceContextID(runID, manifest.ReportSHA256, snippet.PresentationSHA256),
	})
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("source context status = %d body=%q", response.StatusCode, body)
	}
	if got := response.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	var payload sourceContextResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "ok" || payload.Source.Path != "batch.go" ||
		payload.Source.StartLine != 31 || payload.Source.EndLine != 55 {
		t.Fatalf("source context = %#v", payload)
	}
	if payload.Source.SourceComplete {
		t.Fatal("bounded working-tree context was marked source-complete")
	}
	expectedLines := sourceLines[30:55]
	if len(payload.Source.Lines) != len(expectedLines) || len(payload.Source.Lines) > maxSourceContextLines {
		t.Fatalf("returned %d lines, want %d (maximum %d)", len(payload.Source.Lines), len(expectedLines), maxSourceContextLines)
	}
	for index, expected := range expectedLines {
		line := payload.Source.Lines[index]
		if line.Line != index+31 || line.Text != expected {
			t.Fatalf("source line %d = %#v, want line=%d text=%q", index, line, index+31, expected)
		}
	}
}

func TestReportSourceSnippetsIncludesStudyMapReadingAnchors(t *testing.T) {
	t.Parallel()

	snippet := sourceContextTestSnippet("study.go", 12, []string{
		"func study() {",
		"\tinspect()",
		"}",
	})
	data := &reportpkg.ReportData{StudyMap: &reportpkg.RepositoryStudyMap{
		Shape: []reportpkg.RepositoryStudyArea{{ID: "area-core", Source: &snippet}},
		Directions: []reportpkg.StudyDirection{{
			ID:             "study-core",
			ReadingAnchors: []reportpkg.StudyReadingAnchor{{Source: snippet}},
			Documents: []reportpkg.StudyDocumentReference{{
				Label: "Guide", Location: reportpkg.UserCodeLocation{Path: "study.go"}, Source: &snippet,
			}},
		}},
	}}
	got := reportSourceSnippets(data)
	if len(got) != 3 || got[0].PresentationSHA256 != snippet.PresentationSHA256 || got[0].Path != "study.go" {
		t.Fatalf("Study Map source contexts = %#v", got)
	}
}

func TestReportSourceSnippetsIncludesOperationsButExcludesRedactedReferences(t *testing.T) {
	t.Parallel()

	normal := sourceContextTestSnippet("README.md", 12, []string{"go run ./cmd/server"})
	redacted := sourceContextTestSnippet("secrets.env", 3, []string{"TOKEN=[redacted]"})
	normalReference := reportpkg.OperationalReference{Label: "Run", Source: normal}
	redactedReference := reportpkg.OperationalReference{
		Label: "Environment", Redacted: true, Source: redacted,
	}
	data := &reportpkg.ReportData{Operations: &reportpkg.RepositoryOperations{
		Paths: []reportpkg.RepositoryPavedPath{{
			ID:              "operate-server",
			Prerequisites:   []reportpkg.OperationalReference{normalReference, redactedReference},
			Actions:         []reportpkg.OperationalAction{{Reference: normalReference}},
			Expected:        []reportpkg.OperationalReference{redactedReference},
			Troubleshooting: []reportpkg.OperationalReference{normalReference},
		}},
		Landmarks: []reportpkg.OperationalLandmark{
			{ID: "normal", Reference: normalReference},
			{ID: "redacted", Reference: redactedReference},
		},
	}}

	got := reportSourceSnippets(data)
	if len(got) != 4 {
		t.Fatalf("operation source contexts = %d, want 4 non-redacted references: %#v", len(got), got)
	}
	for _, snippet := range got {
		if snippet.Path == "secrets.env" {
			t.Fatal("redacted operation source was authorized for source context")
		}
	}
}

func TestReportSourceSnippetsIncludesTaskInvestigationAnchors(t *testing.T) {
	t.Parallel()

	first := sourceContextTestSnippet("api.go", 10, []string{"func Handle() {}"})
	second := sourceContextTestSnippet("validate.go", 20, []string{"func Validate() error { return nil }"})
	data := &reportpkg.ReportData{TaskInvestigation: &reportpkg.TaskInvestigationWorkspace{
		Anchors: []reportpkg.TaskInvestigationAnchor{
			{Path: first.Path, Source: first},
			{Path: second.Path, Source: second},
		},
	}}

	got := reportSourceSnippets(data)
	if len(got) != 2 || got[0].Path != "api.go" || got[1].Path != "validate.go" {
		t.Fatalf("Task Lens source contexts = %#v", got)
	}
}

func TestSourceContextRejectsUnboundInputs(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	sourceLines := numberedSourceLines(40)
	if err := os.WriteFile(
		filepath.Join(repo, "batch.go"),
		[]byte(strings.Join(sourceLines, "\n")+"\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	const runID = "20260716-101000-source-context"
	writeRun(t, runsDir, runID, repo, "saved report")
	validSnippet := sourceContextTestSnippet("batch.go", 10, sourceLines[9:14])
	missingSnippet := sourceContextTestSnippet("missing.go", 1, []string{"package missing"})
	escapingSnippet := sourceContextTestSnippet("../outside.go", 1, []string{"outside root"})
	manifest := rewriteRunSourceSnippets(t, runsDir, runID, []reportpkg.SourceSnippet{
		validSnippet,
		missingSnippet,
		escapingSnippet,
	})
	server, baseURL := newSourceContextTestServer(t, runsDir, runID)
	defer server.Close()

	validResponse := postSourceContext(t, baseURL, sourceContextRequest{
		RunID:     runID,
		ContextID: manifestSourceContextID(runID, manifest.ReportSHA256, validSnippet.PresentationSHA256),
	})
	validResponse.Body.Close()
	if validResponse.StatusCode != http.StatusOK {
		t.Fatalf("valid root-bound source status = %d, want %d", validResponse.StatusCode, http.StatusOK)
	}

	tests := []struct {
		name string
		body any
		want int
	}{
		{
			name: "unknown opaque id",
			body: sourceContextRequest{RunID: runID, ContextID: "unknown-context"},
			want: http.StatusForbidden,
		},
		{
			name: "captured missing file has no authority",
			body: sourceContextRequest{
				RunID: runID,
				ContextID: manifestSourceContextID(
					runID, manifest.ReportSHA256, missingSnippet.PresentationSHA256,
				),
			},
			want: http.StatusForbidden,
		},
		{
			name: "escaping snippet has no authority",
			body: sourceContextRequest{
				RunID: runID,
				ContextID: manifestSourceContextID(
					runID, manifest.ReportSHA256, escapingSnippet.PresentationSHA256,
				),
			},
			want: http.StatusForbidden,
		},
		{
			name: "raw path is not accepted",
			body: map[string]any{
				"run_id":     runID,
				"context_id": manifestSourceContextID(runID, manifest.ReportSHA256, validSnippet.PresentationSHA256),
				"path":       "batch.go",
				"start_line": 1,
				"end_line":   40,
			},
			want: http.StatusBadRequest,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := postSourceContextValue(t, baseURL, test.body)
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != test.want {
				t.Fatalf("status = %d body=%q, want %d", response.StatusCode, body, test.want)
			}
			if bytes.Contains(body, []byte(sourceLines[9])) {
				t.Fatalf("rejected response leaked source bytes: %q", body)
			}
		})
	}
}

func TestSourceContextRejectsModifiedWorkingTreeWithoutLeakingBytes(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	filePath := filepath.Join(repo, "batch.go")
	originalLines := numberedSourceLines(30)
	if err := os.WriteFile(filePath, []byte(strings.Join(originalLines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	const runID = "20260716-102000-source-context"
	writeRun(t, runsDir, runID, repo, "saved report")
	snippet := sourceContextTestSnippet("batch.go", 8, originalLines[7:12])
	manifest := rewriteRunSourceSnippets(t, runsDir, runID, []reportpkg.SourceSnippet{snippet})
	server, baseURL := newSourceContextTestServer(t, runsDir, runID)
	defer server.Close()

	const changedSource = "package changed\nconst workingTreeOnly = \"DO_NOT_LEAK_CHANGED_SOURCE\"\n"
	if err := os.WriteFile(filePath, []byte(changedSource), 0o600); err != nil {
		t.Fatal(err)
	}
	response := postSourceContext(t, baseURL, sourceContextRequest{
		RunID:     runID,
		ContextID: manifestSourceContextID(runID, manifest.ReportSHA256, snippet.PresentationSHA256),
	})
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("source context status = %d body=%q, want %d", response.StatusCode, body, http.StatusConflict)
	}
	var diagnostic map[string]string
	if err := json.Unmarshal(body, &diagnostic); err != nil {
		t.Fatal(err)
	}
	if diagnostic["code"] != "source_changed" {
		t.Fatalf("diagnostic = %#v, want source_changed", diagnostic)
	}
	if bytes.Contains(body, []byte("DO_NOT_LEAK_CHANGED_SOURCE")) || bytes.Contains(body, []byte("workingTreeOnly")) {
		t.Fatalf("changed working-tree bytes leaked in response: %q", body)
	}
	if !strings.Contains(snippet.Content, originalLines[7]) || strings.Contains(snippet.Content, "workingTreeOnly") {
		t.Fatalf("saved snippet was unexpectedly replaced: %q", snippet.Content)
	}
	reportResponse, err := server.Client().Get(baseURL + "/runs/" + runID + "/report.html")
	if err != nil {
		t.Fatal(err)
	}
	defer reportResponse.Body.Close()
	renderedReport, err := io.ReadAll(reportResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	if reportResponse.StatusCode != http.StatusOK {
		t.Fatalf("saved report status = %d body=%q", reportResponse.StatusCode, renderedReport)
	}
	if !bytes.Contains(renderedReport, []byte(originalLines[7])) {
		t.Fatalf("saved report no longer contains captured source %q", originalLines[7])
	}
	if bytes.Contains(renderedReport, []byte("DO_NOT_LEAK_CHANGED_SOURCE")) ||
		bytes.Contains(renderedReport, []byte("workingTreeOnly")) {
		t.Fatal("saved report silently replaced its snapshot with modified working-tree bytes")
	}
}

func TestBoundedSourceContextRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		target    sourceContextTarget
		lineCount int
		wantStart int
		wantEnd   int
		wantOK    bool
	}{
		{
			name:      "short range gains ten lines of context",
			target:    sourceContextTarget{startLine: 41, endLine: 45, focusLine: 43},
			lineCount: 120,
			wantStart: 31,
			wantEnd:   55,
			wantOK:    true,
		},
		{
			name:      "wide range is centered on focus and bounded",
			target:    sourceContextTarget{startLine: 10, endLine: 100, focusLine: 80},
			lineCount: 200,
			wantStart: 60,
			wantEnd:   139,
			wantOK:    true,
		},
		{
			name:      "out of range focus falls back to saved start",
			target:    sourceContextTarget{startLine: 10, endLine: 100, focusLine: 150},
			lineCount: 200,
			wantStart: 1,
			wantEnd:   80,
			wantOK:    true,
		},
		{
			name:      "range clips at end of file",
			target:    sourceContextTarget{startLine: 110, endLine: 140, focusLine: 120},
			lineCount: 120,
			wantStart: 100,
			wantEnd:   120,
			wantOK:    true,
		},
		{
			name:      "saved range starts beyond file",
			target:    sourceContextTarget{startLine: 121, endLine: 130, focusLine: 121},
			lineCount: 120,
			wantOK:    false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start, end, ok := boundedSourceContextRange(test.target, test.lineCount)
			if ok != test.wantOK || start != test.wantStart || end != test.wantEnd {
				t.Fatalf("range = (%d, %d, %t), want (%d, %d, %t)",
					start, end, ok, test.wantStart, test.wantEnd, test.wantOK)
			}
			if ok && end-start+1 > maxSourceContextLines {
				t.Fatalf("range contains %d lines, maximum is %d", end-start+1, maxSourceContextLines)
			}
		})
	}
}

func numberedSourceLines(count int) []string {
	lines := make([]string, 0, count)
	for line := 1; line <= count; line++ {
		lines = append(lines, fmt.Sprintf("// exact captured source line %03d", line))
	}
	return lines
}

func sourceContextTestSnippet(sourcePath string, startLine int, lines []string) reportpkg.SourceSnippet {
	snippetLines := make([]reportpkg.SourceSnippetLine, 0, len(lines))
	for index, line := range lines {
		snippetLines = append(snippetLines, reportpkg.SourceSnippetLine{
			Line:      startLine + index,
			Text:      line,
			Highlight: index == 0,
		})
	}
	contentJSON, _ := json.Marshal(lines)
	contentDigest := sha256.Sum256(contentJSON)
	snippet := reportpkg.SourceSnippet{
		Path:            sourcePath,
		Language:        "go",
		EnclosingSymbol: "example.Source",
		StartLine:       startLine,
		EndLine:         startLine + len(lines) - 1,
		HighlightRanges: []reportpkg.SourceHighlight{{StartLine: startLine, EndLine: startLine}},
		Content:         strings.Join(lines, "\n"),
		Lines:           snippetLines,
		ContentSHA256:   fmt.Sprintf("%x", contentDigest),
		Role:            "primary",
		SourceComplete:  true,
	}
	presentationJSON, _ := json.Marshal(snippet)
	presentationDigest := sha256.Sum256(presentationJSON)
	snippet.PresentationSHA256 = fmt.Sprintf("%x", presentationDigest)
	return snippet
}

func rewriteRunSourceSnippets(
	t *testing.T,
	runsDir, runID string,
	snippets []reportpkg.SourceSnippet,
) reportpkg.RunManifest {
	t.Helper()
	runDir := filepath.Join(runsDir, runID)
	reportPath := filepath.Join(runDir, "report.json")
	reportJSON, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var data reportpkg.ReportData
	if err := json.Unmarshal(reportJSON, &data); err != nil {
		t.Fatal(err)
	}
	data.UserSources = snippets
	reportJSON, err = json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reportPath, reportJSON, 0o600); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(runDir, reportpkg.RunManifestFilename)
	manifestJSON, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := reportpkg.DecodeRunManifest(manifestJSON)
	if err != nil {
		t.Fatal(err)
	}
	manifest.ReportSHA256 = fmt.Sprintf("%x", sha256.Sum256(reportJSON))
	manifestJSON, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func newSourceContextTestServer(t *testing.T, runsDir, runID string) (*httptest.Server, string) {
	t.Helper()
	handler, err := NewHandler(Options{
		RunsDir:      runsDir,
		InitialRunID: runID,
		Capability:   testCapability,
		OpenFile:     func(context.Context, string, int, int) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	return server, server.URL + capabilityURLPrefix(testCapability)
}

func postSourceContext(t *testing.T, serverURL string, request sourceContextRequest) *http.Response {
	t.Helper()
	return postSourceContextValue(t, serverURL, request)
}

func postSourceContextValue(t *testing.T, serverURL string, value any) *http.Response {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest, err := http.NewRequest(http.MethodPost, serverURL+"/api/source-context", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-Repomap-Action", "read-source-context")
	parsedURL, err := neturl.Parse(serverURL)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest.Header.Set("Origin", parsedURL.Scheme+"://"+parsedURL.Host)
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
