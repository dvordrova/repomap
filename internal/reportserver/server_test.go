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
	"time"

	"github.com/dvordrova/repomap/internal/freshness"
	reportpkg "github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
)

const testCapability = "test-capability"

func TestCatalogSourceTargetsUseOnlyCatalogAuthority(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "offline-root")
	contentSHA256 := strings.Repeat("c", 64)
	inputID := sha256.Sum256([]byte("catalog-target-test"))
	catalog, err := sourcecatalog.New(sourcecatalog.Input{
		RepositoryRoot: root,
		AnalysisRoot:   root,
		AllowedPaths:   []string{"batch.go"},
		CapturedInputs: []freshness.CapturedInput{{
			Version:       freshness.CapturedInputVersion,
			ID:            fmt.Sprintf("%x", inputID[:]),
			Path:          "batch.go",
			Kind:          freshness.FileRegular,
			Mode:          "100644",
			ContentSHA256: contentSHA256,
			Stages:        []string{"report_evidence"},
		}},
	})
	if err != nil {
		t.Fatalf("sourcecatalog.New: %v", err)
	}
	const runID = "20260724-120000-catalog"
	reportSHA256 := strings.Repeat("a", 64)
	targets, sourceIDs := catalogSourceTargets(runID, reportSHA256, catalog)
	sourceID := manifestSourceID(runID, reportSHA256, "batch.go")
	target, ok := targets[sourceID]
	if !ok || target.relativePath != "batch.go" || target.capturedSHA256 != contentSHA256 {
		t.Fatalf("catalog target = %#v, %t", target, ok)
	}
	if len(targets) != 1 || len(sourceIDs) != 1 || sourceIDs["batch.go"] != sourceID {
		t.Fatalf("targets=%#v source IDs=%#v", targets, sourceIDs)
	}
}

func TestOpaqueSourceIDFormulasRemainReportserverBound(t *testing.T) {
	t.Parallel()

	const runID = "20260724-120000-slice1"
	reportSHA256 := strings.Repeat("a", 64)
	if got, want := manifestSourceID(runID, reportSHA256, "service/main.go"),
		"w_Gd_C_vYaqc3HhzJxDZkjG4BBCGQH28UURTLzOACH8"; got != want {
		t.Fatalf("manifestSourceID() = %q, want %q", got, want)
	}
	if got, want := manifestSourceContextID(runID, reportSHA256, strings.Repeat("b", 64)),
		"Te4jr1_reQBquPAGzhMukVNYR9gQWH9NYYzhcggcOUk"; got != want {
		t.Fatalf("manifestSourceContextID() = %q, want %q", got, want)
	}
}

func TestHandlerServesSourceEpisodeOnlyForInitialRun(t *testing.T) {
	tests := []struct {
		name               string
		path               string
		changeAfterCapture bool
	}{
		{
			name:               "etcd changed workspace",
			path:               filepath.Join("..", "..", "experiments", "source-episode", "etcd-put", "episode.json"),
			changeAfterCapture: true,
		},
		{
			name: "django",
			path: filepath.Join("..", "..", "experiments", "source-episode", "django-atomic", "episode.json"),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			raw, fixture := readServerSourceEpisodeFixture(t, test.path)
			anchor := firstServerSourceEpisodeAnchor(t, fixture)
			repo := t.TempDir()
			runsDir := t.TempDir()
			const selectedRunID = "20260727-120000-selected"
			const otherRunID = "20260727-120001-other"
			writeServerSourceEpisodeRun(t, runsDir, selectedRunID, repo, fixture.Repository.Revision, anchor)
			writeServerSourceEpisodeRun(t, runsDir, otherRunID, repo, fixture.Repository.Revision, anchor)
			repositoryState := readServerRunManifest(t, runsDir, selectedRunID).RepositoryState

			var openedPath string
			var openedLine int
			handler, err := NewHandler(Options{
				RunsDir:           runsDir,
				InitialRunID:      selectedRunID,
				Capability:        testCapability,
				SourceEpisodeJSON: raw,
				OpenFile: func(_ context.Context, path string, line, _ int) error {
					openedPath = path
					openedLine = line
					return nil
				},
				CaptureRepository: func(context.Context, string) (freshness.RepositoryState, error) {
					return repositoryState, nil
				},
			})
			if err != nil {
				t.Fatalf("NewHandler with approved source episode: %v", err)
			}
			raw[0] ^= 0xff // The handler must retain its validated private copy.
			if test.changeAfterCapture {
				sourcePath := filepath.Join(repo, filepath.FromSlash(anchor.Path))
				if err := os.WriteFile(sourcePath, []byte("changed source\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			server := httptest.NewServer(handler)
			defer server.Close()
			baseURL := server.URL + capabilityURLPrefix(testCapability)

			response, err := server.Client().Get(baseURL + "/runs/" + selectedRunID + "/report.html")
			if err != nil {
				t.Fatal(err)
			}
			selectedHTML, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			if response.StatusCode != http.StatusOK {
				t.Fatalf("selected report status = %d, want %d: %s", response.StatusCode, http.StatusOK, selectedHTML)
			}
			for _, required := range []string{
				`"source_episode":`,
				fixture.EpisodeID,
				"renderSourceEpisodeSources",
			} {
				if !bytes.Contains(selectedHTML, []byte(required)) {
					t.Fatalf("selected report is missing %q", required)
				}
			}
			pathJSON, err := json.Marshal(anchor.Path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(selectedHTML, []byte(`"sources":[{"path":`+string(pathJSON))) {
				t.Fatalf("selected report did not project authorized source %q", anchor.Path)
			}
			sourceID := testSourceID(t, runsDir, selectedRunID, anchor.Path)
			sourceIDsJSON, err := json.Marshal(map[string]string{anchor.Path: sourceID})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(selectedHTML, []byte(`"source_ids":`+string(sourceIDsJSON))) {
				t.Fatalf("selected report is missing opaque source authority for %q", anchor.Path)
			}

			openResponse := postOpen(t, baseURL, openRequest{
				RunID: selectedRunID, SourceID: sourceID, Line: anchor.StartLine,
			}, true)
			var openPayload map[string]any
			if err := json.NewDecoder(openResponse.Body).Decode(&openPayload); err != nil {
				openResponse.Body.Close()
				t.Fatal(err)
			}
			openResponse.Body.Close()
			wantOpenedPath, err := filepath.EvalSymlinks(filepath.Join(repo, filepath.FromSlash(anchor.Path)))
			if err != nil {
				t.Fatal(err)
			}
			if openResponse.StatusCode != http.StatusOK || openedPath != wantOpenedPath ||
				openedLine != anchor.StartLine ||
				openPayload["source_changed"] != test.changeAfterCapture {
				t.Fatalf(
					"episode source action status=%d path=%q line=%d changed=%v, want status=%d path=%q line=%d changed=%t",
					openResponse.StatusCode, openedPath, openedLine, openPayload["source_changed"],
					http.StatusOK, wantOpenedPath, anchor.StartLine, test.changeAfterCapture,
				)
			}

			response, err = server.Client().Get(baseURL + "/runs/" + otherRunID + "/report.html")
			if err != nil {
				t.Fatal(err)
			}
			otherHTML, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			if response.StatusCode != http.StatusOK {
				t.Fatalf("nonselected report status = %d, want %d: %s", response.StatusCode, http.StatusOK, otherHTML)
			}
			for _, forbidden := range []string{`"source_episode":`, fixture.EpisodeID, "renderSourceEpisode"} {
				if bytes.Contains(otherHTML, []byte(forbidden)) {
					t.Fatalf("nonselected report gained source episode token %q", forbidden)
				}
			}
		})
	}
}

func TestHandlerRejectsInvalidSourceEpisodeBinding(t *testing.T) {
	t.Run("initial run is required", func(t *testing.T) {
		if _, err := NewHandler(Options{
			RunsDir: t.TempDir(), SourceEpisodeJSON: []byte(`{}`),
		}); err == nil {
			t.Fatal("source episode without InitialRunID was accepted")
		}
	})

	t.Run("byte budget", func(t *testing.T) {
		if _, err := NewHandler(Options{
			RunsDir:           t.TempDir(),
			InitialRunID:      "20260727-120000-oversized",
			SourceEpisodeJSON: bytes.Repeat([]byte("x"), reportpkg.MaxSourceEpisodeBytes+1),
		}); err == nil {
			t.Fatal("oversized source episode was accepted")
		}
	})

	t.Run("cross revision", func(t *testing.T) {
		raw, fixture := readServerSourceEpisodeFixture(
			t,
			filepath.Join("..", "..", "experiments", "source-episode", "etcd-put", "episode.json"),
		)
		anchor := firstServerSourceEpisodeAnchor(t, fixture)
		repo := t.TempDir()
		runsDir := t.TempDir()
		const runID = "20260727-120000-mismatch"
		writeServerSourceEpisodeRun(t, runsDir, runID, repo, strings.Repeat("f", 40), anchor)
		if _, err := NewHandler(Options{
			RunsDir:           runsDir,
			InitialRunID:      runID,
			Capability:        testCapability,
			SourceEpisodeJSON: raw,
		}); err == nil {
			t.Fatal("source episode was accepted for a different report revision")
		}
	})
}

func TestLegacyManifestSourceTargetsPreserveV2V3Behavior(t *testing.T) {
	t.Parallel()

	reportSHA256 := strings.Repeat("a", 64)
	contentSHA256 := strings.Repeat("b", 64)
	manifest := reportpkg.RunManifest{
		RepositoryState: freshness.RepositoryState{Identity: "/repo"},
		AnalysisRoot:    "/repo/service",
		ReportSHA256:    reportSHA256,
		OpenablePaths:   []string{"batch.go"},
		CapturedInputs: []freshness.CapturedInput{{
			Path: "service/batch.go", ContentSHA256: contentSHA256,
		}},
	}
	const runID = "20260724-120000-legacy"
	sourceID := manifestSourceID(runID, reportSHA256, "batch.go")
	for _, test := range []struct {
		version    int
		wantSHA256 string
	}{
		{version: 2, wantSHA256: ""},
		{version: 3, wantSHA256: contentSHA256},
	} {
		manifest.Version = test.version
		if test.version == 2 {
			manifest.CapturedInputs = nil
		} else {
			manifest.CapturedInputs = []freshness.CapturedInput{{
				Path: "service/batch.go", ContentSHA256: contentSHA256,
			}}
		}
		targets, sourceIDs := legacyManifestSourceTargets(runID, reportSHA256, manifest)
		target, ok := targets[sourceID]
		if !ok || target.relativePath != "batch.go" ||
			target.capturedSHA256 != test.wantSHA256 || sourceIDs["batch.go"] != sourceID {
			t.Fatalf("v%d targets=%#v source IDs=%#v", test.version, targets, sourceIDs)
		}
	}
}

func TestHandlerListsReportsServesLatestAndOpensValidatedFile(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	filePath := filepath.Join(repo, "batch.go")
	if err := os.WriteFile(filePath, []byte("package pebble\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	writeRun(t, runsDir, "20260711-190000-pebble", repo, "older")
	const unverifiedHTML = `<script>window.__unverified_saved_report_executed__ = true</script>`
	writeRun(t, runsDir, "20260711-200000-pebble", repo, unverifiedHTML)
	const legacyHTML = `<script>window.__legacy_saved_report_executed__ = true</script>`
	writeRun(t, runsDir, "20260711-210000-legacy", ".", legacyHTML)

	var openedPath string
	var openedLine int
	captureCalls := 0
	handler, err := NewHandler(Options{
		RunsDir:      runsDir,
		InitialRunID: "20260711-200000-pebble",
		Capability:   testCapability,
		OpenFile: func(_ context.Context, path string, line, _ int) error {
			openedPath = path
			openedLine = line
			return nil
		},
		CaptureRepository: func(context.Context, string) (freshness.RepositoryState, error) {
			captureCalls++
			return freshness.RepositoryState{
				Version: freshness.RepositoryStateVersion, Identity: filepath.Clean(repo), Head: strings.Repeat("0", 40),
			}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	baseURL := server.URL + capabilityURLPrefix(testCapability)
	response, err := client.Get(baseURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusFound || response.Header.Get("Location") != capabilityURLPrefix(testCapability)+"/runs/20260711-200000-pebble/report.html" {
		t.Fatalf("root response = %d location=%q", response.StatusCode, response.Header.Get("Location"))
	}

	response, err = server.Client().Get(baseURL + "/api/runs")
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Runs []RunSummary `json:"runs"`
	}
	if err := json.NewDecoder(response.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if len(list.Runs) != 3 || list.Runs[0].ID != "20260711-210000-legacy" {
		t.Fatalf("runs = %#v", list.Runs)
	}
	if captureCalls != 0 {
		t.Fatalf("run listing performed %d live freshness captures", captureCalls)
	}

	response, err = server.Client().Get(baseURL + "/runs/20260711-200000-pebble/report.html")
	if err != nil {
		t.Fatal(err)
	}
	report, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !bytes.Contains(report, []byte(`id="rm-report-data"`)) {
		t.Fatalf("trusted report response = %d body=%q", response.StatusCode, report)
	}
	if captureCalls != 1 {
		t.Fatalf("report request performed %d live freshness captures, want 1", captureCalls)
	}
	if bytes.Contains(report, []byte(unverifiedHTML)) {
		t.Fatal("server executed the saved report HTML instead of rendering verified report data")
	}
	if response.Header.Get("Content-Security-Policy") == "" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("trusted report response missing security headers: %#v", response.Header)
	}

	response, err = server.Client().Get(baseURL + "/runs/20260711-210000-legacy/report.html")
	if err != nil {
		t.Fatal(err)
	}
	legacyResponse, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("legacy report status = %d, want %d", response.StatusCode, http.StatusConflict)
	}
	if bytes.Contains(legacyResponse, []byte(legacyHTML)) {
		t.Fatal("server returned unverified legacy HTML on the capability origin")
	}

	response = postOpen(t, baseURL, openRequest{
		RunID:    "20260711-200000-pebble",
		SourceID: testSourceID(t, runsDir, "20260711-200000-pebble", "batch.go"),
		Line:     288,
	}, true)
	response.Body.Close()
	canonicalFilePath, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || openedPath != canonicalFilePath || openedLine != 288 {
		t.Fatalf("open response=%d path=%q line=%d", response.StatusCode, openedPath, openedLine)
	}
	if captureCalls != 1 {
		t.Fatalf("source open performed freshness capture; calls=%d", captureCalls)
	}
}

func TestOpenEndpointUsesStartupAuthorizationIndexAndRejectsRawPath(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "batch.go"), []byte("package p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	const runID = "20260711-200000-cached"
	writeRun(t, runsDir, runID, repo, "report")
	sourceID := testSourceID(t, runsDir, runID, "batch.go")
	launches := 0
	openedLine, openedColumn := 0, 0
	var logs []string
	handler, err := NewHandler(Options{
		RunsDir: runsDir, Capability: testCapability,
		OpenFile: func(_ context.Context, _ string, line, column int) error {
			launches++
			openedLine, openedColumn = line, column
			return nil
		},
		CaptureRepository: func(context.Context, string) (freshness.RepositoryState, error) {
			t.Fatal("source open must not check freshness")
			return freshness.RepositoryState{}, nil
		},
		Logf: func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) },
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	baseURL := server.URL + capabilityURLPrefix(testCapability)
	response := postOpen(t, baseURL, openRequest{RunID: runID, SourceID: sourceID, Line: 3, Column: 2}, true)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || launches != 1 || openedLine != 3 || openedColumn != 2 {
		t.Fatalf("cached open status=%d launches=%d location=%d:%d", response.StatusCode, launches, openedLine, openedColumn)
	}
	loadedLogs := 0
	for _, log := range logs {
		if strings.HasPrefix(log, "loaded ") {
			loadedLogs++
		}
	}
	if loadedLogs != 1 {
		t.Fatalf("source open reloaded saved reports; load logs=%d logs=%v", loadedLogs, logs)
	}

	body := strings.NewReader(`{"run_id":"` + runID + `","path":"batch.go","line":3}`)
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/open", body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", server.URL)
	request.Header.Set("X-Repomap-Action", "open-file")
	response, err = server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest || launches != 1 {
		t.Fatalf("raw path status=%d launches=%d", response.StatusCode, launches)
	}
}

func TestOpenEndpointReportsSelectedFileChangedWithoutRepositoryReconciliation(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	filePath := filepath.Join(repo, "batch.go")
	if err := os.WriteFile(filePath, []byte("package original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	const runID = "20260711-200000-stale-source"
	writeRun(t, runsDir, runID, repo, "report")
	sourceID := testSourceID(t, runsDir, runID, "batch.go")
	launches := 0
	handler, err := NewHandler(Options{
		RunsDir: runsDir, Capability: testCapability,
		OpenFile: func(context.Context, string, int, int) error {
			launches++
			return nil
		},
		CaptureRepository: func(context.Context, string) (freshness.RepositoryState, error) {
			t.Fatal("source open must not reconcile repository freshness")
			return freshness.RepositoryState{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("package changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	response := postOpen(t, server.URL+capabilityURLPrefix(testCapability), openRequest{RunID: runID, SourceID: sourceID}, true)
	defer response.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || payload["source_changed"] != true || launches != 1 {
		t.Fatalf("status=%d payload=%v launches=%d", response.StatusCode, payload, launches)
	}
}

func TestOpenEndpointRechecksSymlinksAfterHandlerStartup(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	filePath := filepath.Join(repo, "batch.go")
	if err := os.WriteFile(filePath, []byte("package original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	const runID = "20260711-200000-replaced-source"
	writeRun(t, runsDir, runID, repo, "report")
	launches := 0
	handler, err := NewHandler(Options{
		RunsDir: runsDir, Capability: testCapability,
		OpenFile: func(context.Context, string, int, int) error {
			launches++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filePath); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	response := postOpen(t, server.URL+capabilityURLPrefix(testCapability), openRequest{
		RunID: runID, SourceID: testSourceID(t, runsDir, runID, "batch.go"),
	}, true)
	defer response.Body.Close()
	var payload map[string]string
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusConflict || payload["code"] != "source_unavailable" || launches != 0 {
		t.Fatalf("status=%d payload=%v launches=%d", response.StatusCode, payload, launches)
	}
}

func TestOpenEndpointRejectsSourceRemovedAfterHandlerStartup(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	filePath := filepath.Join(repo, "batch.go")
	if err := os.WriteFile(filePath, []byte("package original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	const runID = "20260711-200000-removed-source"
	writeRun(t, runsDir, runID, repo, "report")
	sourceID := testSourceID(t, runsDir, runID, "batch.go")
	launches := 0
	handler, err := NewHandler(Options{
		RunsDir: runsDir, Capability: testCapability,
		OpenFile: func(context.Context, string, int, int) error {
			launches++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filePath); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	response := postOpen(t, server.URL+capabilityURLPrefix(testCapability), openRequest{
		RunID: runID, SourceID: sourceID,
	}, true)
	defer response.Body.Close()
	var payload map[string]string
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusConflict || payload["code"] != "source_unavailable" || launches != 0 {
		t.Fatalf("status=%d payload=%v launches=%d", response.StatusCode, payload, launches)
	}
}

func TestOpenEndpointInvalidatesReplacedRunAuthority(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "batch.go"), []byte("package p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	const runID = "20260711-200000-replaced-run"
	writeRun(t, runsDir, runID, repo, "first report")
	oldSourceID := testSourceID(t, runsDir, runID, "batch.go")
	launches := 0
	handler, err := NewHandler(Options{
		RunsDir: runsDir, Capability: testCapability,
		OpenFile: func(context.Context, string, int, int) error {
			launches++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rewriteRunReportName(t, runsDir, runID, "replacement-report")
	newSourceID := testSourceID(t, runsDir, runID, "batch.go")
	if oldSourceID == newSourceID {
		t.Fatal("replacement report retained its old source id")
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	baseURL := server.URL + capabilityURLPrefix(testCapability)
	response := postOpen(t, baseURL, openRequest{RunID: runID, SourceID: oldSourceID}, true)
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden || launches != 0 {
		t.Fatalf("old source status=%d launches=%d", response.StatusCode, launches)
	}
	response = postOpen(t, baseURL, openRequest{RunID: runID, SourceID: newSourceID}, true)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || launches != 1 {
		t.Fatalf("new source status=%d launches=%d", response.StatusCode, launches)
	}
}

func TestOpenEndpointReturnsTypedEditorUnavailable(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "batch.go"), []byte("package p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	const runID = "20260711-200000-unavailable"
	writeRun(t, runsDir, runID, repo, "report")
	handler, err := NewHandler(Options{
		RunsDir: runsDir, Capability: testCapability,
		OpenFile: unavailableEditorLauncher(ErrEditorUnavailable),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	response := postOpen(t, server.URL+capabilityURLPrefix(testCapability), openRequest{
		RunID: runID, SourceID: testSourceID(t, runsDir, runID, "batch.go"),
	}, true)
	defer response.Body.Close()
	var payload map[string]string
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusServiceUnavailable || payload["code"] != "editor_unavailable" {
		t.Fatalf("status=%d payload=%v", response.StatusCode, payload)
	}
}

func TestOpenEndpointResolvesPathsFromManifestAnalysisRoot(t *testing.T) {
	t.Parallel()

	repository := t.TempDir()
	analysisRoot := filepath.Join(repository, "service")
	if err := os.Mkdir(analysisRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(analysisRoot, "batch.go")
	if err := os.WriteFile(filePath, []byte("package service\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	writeScopedRun(t, runsDir, "20260711-200000-service", repository, analysisRoot, "report")

	var openedPath string
	handler, err := NewHandler(Options{
		RunsDir:    runsDir,
		Capability: testCapability,
		OpenFile: func(_ context.Context, path string, _, _ int) error {
			openedPath = path
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	response := postOpen(t, server.URL+capabilityURLPrefix(testCapability), openRequest{
		RunID:    "20260711-200000-service",
		SourceID: testSourceID(t, runsDir, "20260711-200000-service", "batch.go"),
		Line:     1,
	}, true)
	response.Body.Close()
	want, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || openedPath != want {
		t.Fatalf("open response=%d path=%q, want %q", response.StatusCode, openedPath, want)
	}
}

func TestOpenEndpointRejectsCrossOriginShapeAndRepositoryEscape(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	runsDir := t.TempDir()
	escapePath := filepath.Join(repo, "escape.go")
	if err := os.WriteFile(escapePath, []byte("package placeholder\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeRun(t, runsDir, "20260711-200000-project", repo, "report")
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(escapePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, escapePath); err != nil {
		t.Fatal(err)
	}

	openCalls := 0
	handler, err := NewHandler(Options{
		RunsDir:    runsDir,
		Capability: testCapability,
		OpenFile: func(context.Context, string, int, int) error {
			openCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	baseURL := server.URL + capabilityURLPrefix(testCapability)
	response := postOpen(t, baseURL, openRequest{
		RunID:    "20260711-200000-project",
		SourceID: "not-authorized",
	}, true)
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("traversal status = %d", response.StatusCode)
	}

	response = postOpen(t, baseURL, openRequest{
		RunID:    "20260711-200000-project",
		SourceID: testSourceID(t, runsDir, "20260711-200000-project", "escape.go"),
	}, true)
	response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("symlink escape status = %d", response.StatusCode)
	}

	response = postOpen(t, baseURL, openRequest{
		RunID:    "20260711-200000-project",
		SourceID: "not-authorized",
	}, false)
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("missing action header status = %d", response.StatusCode)
	}

	body := strings.NewReader(`{"run_id":"20260711-200000-project","path":"missing.go"}`)
	hostRequest, err := http.NewRequest(http.MethodPost, baseURL+"/api/open", body)
	if err != nil {
		t.Fatal(err)
	}
	hostRequest.Host = "attacker.example"
	hostRequest.Header.Set("Content-Type", "application/json")
	hostRequest.Header.Set("Origin", "http://attacker.example")
	hostRequest.Header.Set("X-Repomap-Action", "open-file")
	response, err = http.DefaultClient.Do(hostRequest)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign host status = %d", response.StatusCode)
	}
	if openCalls != 0 {
		t.Fatalf("editor called %d times for rejected requests", openCalls)
	}
}

func TestOpenEndpointDoesNotExposeEditorCommandErrors(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "batch.go"), []byte("package pebble\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	writeRun(t, runsDir, "20260711-200000-project", repo, "saved report")

	handler, err := NewHandler(Options{
		RunsDir:    runsDir,
		Capability: testCapability,
		OpenFile: func(context.Context, string, int, int) error {
			return fmt.Errorf("editor output leaked /Users/example/private.sock")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	response := postOpen(t, server.URL+capabilityURLPrefix(testCapability), openRequest{
		RunID:    "20260711-200000-project",
		SourceID: testSourceID(t, runsDir, "20260711-200000-project", "batch.go"),
		Line:     1,
	}, true)
	defer response.Body.Close()
	var payload map[string]string
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusBadGateway)
	}
	if payload["error"] != "could not open file in VS Code" {
		t.Fatalf("public editor error = %q", payload["error"])
	}
}

func TestHandlerTransportRejectionMatrix(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "batch.go"), []byte("package pebble\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	writeRun(t, runsDir, "20260711-200000-project", repo, "report")

	openCalls := 0
	const expectedHost = "127.0.0.1:4321"
	handler, err := NewHandler(Options{
		RunsDir:      runsDir,
		Capability:   testCapability,
		ExpectedHost: expectedHost,
		OpenFile: func(context.Context, string, int, int) error {
			openCalls++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(openRequest{
		RunID:    "20260711-200000-project",
		SourceID: testSourceID(t, runsDir, "20260711-200000-project", "batch.go"),
		Line:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	prefix := capabilityURLPrefix(testCapability)
	tests := []struct {
		name       string
		method     string
		path       string
		host       string
		origin     string
		mediaType  string
		action     string
		wantStatus int
	}{
		{
			name:       "valid HTTP same origin",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "http://" + expectedHost,
			mediaType:  "application/json",
			action:     "open-file",
			wantStatus: http.StatusOK,
		},
		{
			name:       "HTTPS origin is rejected by the HTTP-only server",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "https://" + expectedHost,
			mediaType:  "application/json; charset=utf-8",
			action:     "open-file",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "missing capability",
			method:     http.MethodPost,
			path:       "/api/open",
			host:       expectedHost,
			origin:     "http://" + expectedHost,
			mediaType:  "application/json",
			action:     "open-file",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "wrong capability",
			method:     http.MethodPost,
			path:       capabilityURLPrefix("wrong-capability") + "/api/open",
			host:       expectedHost,
			origin:     "http://" + expectedHost,
			mediaType:  "application/json",
			action:     "open-file",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "wrong host",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       "localhost:4321",
			origin:     "http://localhost:4321",
			mediaType:  "application/json",
			action:     "open-file",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "missing origin",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			mediaType:  "application/json",
			action:     "open-file",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "null origin",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "null",
			mediaType:  "application/json",
			action:     "open-file",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "cross origin",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "http://attacker.example",
			mediaType:  "application/json",
			action:     "open-file",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "origin with path",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "http://" + expectedHost + "/unexpected",
			mediaType:  "application/json",
			action:     "open-file",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "missing content type",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "http://" + expectedHost,
			action:     "open-file",
			wantStatus: http.StatusUnsupportedMediaType,
		},
		{
			name:       "wrong content type",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "http://" + expectedHost,
			mediaType:  "text/plain",
			action:     "open-file",
			wantStatus: http.StatusUnsupportedMediaType,
		},
		{
			name:       "missing action header",
			method:     http.MethodPost,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "http://" + expectedHost,
			mediaType:  "application/json",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "OPTIONS preflight",
			method:     http.MethodOptions,
			path:       prefix + "/api/open",
			host:       expectedHost,
			origin:     "http://" + expectedHost,
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "http://"+expectedHost+test.path, bytes.NewReader(body))
			request.Host = test.host
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.mediaType != "" {
				request.Header.Set("Content-Type", test.mediaType)
			}
			if test.action != "" {
				request.Header.Set("X-Repomap-Action", test.action)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%q", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if recorder.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatalf("unexpected CORS response header")
			}
		})
	}
	if openCalls != 1 {
		t.Fatalf("editor called %d times, want 1 accepted request", openCalls)
	}
}

func TestServeUsesRandomLoopbackPortAndStopsWithContext(t *testing.T) {
	t.Parallel()

	runsDir := t.TempDir()
	writeRun(t, runsDir, "20260711-200000-project", t.TempDir(), "report")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, Options{
			RunsDir:      runsDir,
			InitialRunID: "20260711-200000-project",
			OnReady: func(url string) error {
				ready <- url
				return nil
			},
		})
	}()

	var url string
	select {
	case url = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("report server did not become ready")
	}
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatalf("server URL = %q", url)
	}
	parsedURL, err := neturl.Parse(url)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(parsedURL.Path, "/_repomap/") || !strings.HasSuffix(parsedURL.Path, "/runs/20260711-200000-project/report.html") {
		t.Fatalf("server URL does not contain capability prefix and report path: %q", url)
	}
	response, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	wrongHostRequest, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	wrongHostRequest.Host = "localhost:" + parsedURL.Port()
	response, err = http.DefaultClient.Do(wrongHostRequest)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("alternate loopback host status = %d, want %d", response.StatusCode, http.StatusForbidden)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("report server did not stop after cancellation")
	}
}

func TestServeShutdownDoesNotWaitForFakeEditor(t *testing.T) {
	if os.Getenv("REPOMAP_SLOW_EDITOR_HELPER") == "1" {
		if err := os.WriteFile(os.Getenv("REPOMAP_SLOW_EDITOR_STARTED"), []byte("started"), 0o600); err != nil {
			os.Exit(2)
		}
		time.Sleep(2 * time.Second)
		if err := os.WriteFile(os.Getenv("REPOMAP_SLOW_EDITOR_EXITED"), []byte("exited"), 0o600); err != nil {
			os.Exit(3)
		}
		os.Exit(0)
	}

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "batch.go"), []byte("package p\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runsDir := t.TempDir()
	const runID = "20260711-200000-slow-editor"
	writeRun(t, runsDir, runID, repo, "report")
	startedFile := filepath.Join(t.TempDir(), "started")
	exitedFile := filepath.Join(t.TempDir(), "exited")
	t.Setenv("REPOMAP_SLOW_EDITOR_HELPER", "1")
	t.Setenv("REPOMAP_SLOW_EDITOR_STARTED", startedFile)
	t.Setenv("REPOMAP_SLOW_EDITOR_EXITED", exitedFile)
	launcher := editorLauncher(os.Args[0], []string{"-test.run=TestServeShutdownDoesNotWaitForFakeEditor", "--", "--goto"})

	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- Serve(ctx, Options{
			RunsDir: runsDir, InitialRunID: runID, Port: 0, Capability: testCapability,
			OpenFile: launcher,
			OnReady: func(url string) error {
				ready <- url
				return nil
			},
		})
	}()
	reportURL := <-ready
	parsed, err := neturl.Parse(reportURL)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		response, requestErr := http.Get(reportURL)
		if requestErr == nil {
			response.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not become ready: %v", requestErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	baseURL := parsed.Scheme + "://" + parsed.Host + capabilityURLPrefix(testCapability)
	requestStarted := time.Now()
	response := postOpen(t, baseURL, openRequest{
		RunID: runID, SourceID: testSourceID(t, runsDir, runID, "batch.go"), Line: 9, Column: 3,
	}, true)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || time.Since(requestStarted) > 500*time.Millisecond {
		t.Fatalf("source open status=%d elapsed=%v", response.StatusCode, time.Since(requestStarted))
	}
	waitForFile(t, startedFile, time.Second)
	cancel()
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(750 * time.Millisecond):
		t.Fatal("server shutdown waited for fake editor")
	}
	waitForFile(t, exitedFile, 3*time.Second)
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", filepath.Base(path))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func postOpen(t *testing.T, serverURL string, request openRequest, withHeader bool) *http.Response {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest, err := http.NewRequest(http.MethodPost, serverURL+"/api/open", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	parsedURL, err := neturl.Parse(serverURL)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest.Header.Set("Origin", parsedURL.Scheme+"://"+parsedURL.Host)
	if withHeader {
		httpRequest.Header.Set("X-Repomap-Action", "open-file")
	}
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

type serverSourceEpisodeFixture struct {
	EpisodeID  string `json:"episode_id"`
	Repository struct {
		Revision string `json:"revision"`
	} `json:"repository"`
	Anchors []serverSourceEpisodeAnchor `json:"anchors"`
	Claims  []struct {
		AnchorIDs []string `json:"anchor_ids"`
	} `json:"claims"`
}

type serverSourceEpisodeAnchor struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

func readServerSourceEpisodeFixture(t *testing.T, fixturePath string) ([]byte, serverSourceEpisodeFixture) {
	t.Helper()
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var fixture serverSourceEpisodeFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return raw, fixture
}

func firstServerSourceEpisodeAnchor(
	t *testing.T,
	fixture serverSourceEpisodeFixture,
) serverSourceEpisodeAnchor {
	t.Helper()
	if len(fixture.Claims) == 0 || len(fixture.Claims[0].AnchorIDs) == 0 {
		t.Fatal("source episode fixture has no first claim anchor")
	}
	wantID := fixture.Claims[0].AnchorIDs[0]
	for _, anchor := range fixture.Anchors {
		if anchor.ID == wantID {
			return anchor
		}
	}
	t.Fatalf("source episode fixture is missing anchor %q", wantID)
	return serverSourceEpisodeAnchor{}
}

func writeServerSourceEpisodeRun(
	t *testing.T,
	runsDir,
	runID,
	repoPath,
	revision string,
	anchor serverSourceEpisodeAnchor,
) {
	t.Helper()
	sourcePath := anchor.Path
	sourceContent := []byte(strings.Repeat("// source episode fixture\n", max(anchor.EndLine, 1)))
	absoluteSourcePath := filepath.Join(repoPath, filepath.FromSlash(sourcePath))
	if err := os.MkdirAll(filepath.Dir(absoluteSourcePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absoluteSourcePath, sourceContent, 0o600); err != nil {
		t.Fatal(err)
	}
	writeRun(t, runsDir, runID, repoPath, "saved report")

	runDir := filepath.Join(runsDir, runID)
	reportPath := filepath.Join(runDir, "report.json")
	reportJSON, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var reportData reportpkg.ReportData
	if err := json.Unmarshal(reportJSON, &reportData); err != nil {
		t.Fatal(err)
	}
	reportData.CapturedRevision = revision
	reportData.OpenablePaths = []string{sourcePath}
	reportJSON, err = json.Marshal(reportData)
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
	manifest.RepositoryState.Head = revision
	manifest.RepositoryStateSHA256, err = manifest.RepositoryState.Digest()
	if err != nil {
		t.Fatal(err)
	}
	input := freshness.CapturedInput{
		Version:       freshness.CapturedInputVersion,
		ID:            fmt.Sprintf("%x", sha256.Sum256([]byte("source-episode-input\x00"+sourcePath))),
		Path:          sourcePath,
		Kind:          freshness.FileRegular,
		Mode:          string(freshness.FileRegular),
		ContentSHA256: fmt.Sprintf("%x", sha256.Sum256(sourceContent)),
		Stages:        []string{"report_evidence"},
	}
	manifest.ReportSHA256 = fmt.Sprintf("%x", sha256.Sum256(reportJSON))
	manifest.OpenablePaths = []string{sourcePath}
	manifest.CapturedInputs = []freshness.CapturedInput{input}
	manifest.CapturedInputsSHA256, err = freshness.CapturedInputsDigest(manifest.CapturedInputs)
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.SelectedRevision = revision
	manifestJSON, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestJSON, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readServerRunManifest(t *testing.T, runsDir, runID string) reportpkg.RunManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runsDir, runID, reportpkg.RunManifestFilename))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := reportpkg.DecodeRunManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func testSourceID(t *testing.T, runsDir, runID, relativePath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runsDir, runID, reportpkg.RunManifestFilename))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := reportpkg.DecodeRunManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	return manifestSourceID(runID, manifest.ReportSHA256, relativePath)
}

func rewriteRunReportName(t *testing.T, runsDir, runID, repoName string) {
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
	data.RepoName = repoName
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
}

func writeRun(t *testing.T, runsDir, runID, repoPath, report string) {
	t.Helper()
	writeScopedRun(t, runsDir, runID, repoPath, repoPath, report)
}

func writeScopedRun(t *testing.T, runsDir, runID, repositoryPath, analysisRoot, report string) {
	t.Helper()
	runDir := filepath.Join(runsDir, runID)
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.html"), []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	openablePaths := make([]string, 0, 2)
	for _, candidate := range []string{"batch.go", "escape.go"} {
		info, statErr := os.Lstat(filepath.Join(analysisRoot, filepath.FromSlash(candidate)))
		if statErr == nil && info.Mode().IsRegular() {
			openablePaths = append(openablePaths, candidate)
		}
	}
	reportData := reportpkg.ReportData{
		FormatVersion: reportpkg.CurrentFormatVersion,
		RepoName:      filepath.Base(analysisRoot),
		OpenablePaths: openablePaths,
	}
	reportJSON, err := json.Marshal(reportData)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), reportJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	metadataJSON, err := json.Marshal(metadata{RepoName: filepath.Base(analysisRoot), RepoPath: analysisRoot, CreatedAt: runID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "metadata.json"), metadataJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(repositoryPath) || !filepath.IsAbs(analysisRoot) {
		return
	}
	canonicalRepoPath, err := filepath.EvalSymlinks(repositoryPath)
	if err != nil {
		t.Fatal(err)
	}
	canonicalRepoPath, err = filepath.Abs(canonicalRepoPath)
	if err != nil {
		t.Fatal(err)
	}
	canonicalRepoPath = filepath.Clean(canonicalRepoPath)
	canonicalAnalysisRoot, err := filepath.EvalSymlinks(analysisRoot)
	if err != nil {
		t.Fatal(err)
	}
	canonicalAnalysisRoot, err = filepath.Abs(canonicalAnalysisRoot)
	if err != nil {
		t.Fatal(err)
	}
	canonicalAnalysisRoot = filepath.Clean(canonicalAnalysisRoot)
	analysisRelative, err := filepath.Rel(canonicalRepoPath, canonicalAnalysisRoot)
	if err != nil {
		t.Fatal(err)
	}
	state := freshness.RepositoryState{
		Version:  freshness.RepositoryStateVersion,
		Identity: canonicalRepoPath,
		Head:     strings.Repeat("0", 40),
	}
	stateDigest, err := state.Digest()
	if err != nil {
		t.Fatal(err)
	}
	inputs := make([]freshness.CapturedInput, 0, len(reportData.OpenablePaths))
	for _, inputPath := range reportData.OpenablePaths {
		repositoryInputPath := inputPath
		if analysisRelative != "." {
			repositoryInputPath = filepath.ToSlash(filepath.Join(analysisRelative, filepath.FromSlash(inputPath)))
		}
		input := freshness.CapturedInput{
			Version: freshness.CapturedInputVersion, ID: fmt.Sprintf("%x", sha256.Sum256([]byte("input\x00"+repositoryInputPath))),
			Path: repositoryInputPath, Kind: freshness.FileMissing, Stages: []string{"report_evidence"},
		}
		if data, readErr := os.ReadFile(filepath.Join(analysisRoot, filepath.FromSlash(inputPath))); readErr == nil {
			input.Kind = freshness.FileRegular
			input.Mode = "regular"
			input.ContentSHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
		}
		inputs = append(inputs, input)
	}
	inputsDigest, err := freshness.CapturedInputsDigest(inputs)
	if err != nil {
		t.Fatal(err)
	}
	manifest := reportpkg.RunManifest{
		Version:               reportpkg.CurrentRunManifestVersion,
		RepositoryState:       state,
		AnalysisRoot:          canonicalAnalysisRoot,
		RepositoryStateSHA256: stateDigest,
		ReportSHA256:          fmt.Sprintf("%x", sha256.Sum256(reportJSON)),
		ReportFormatVersion:   reportpkg.CurrentFormatVersion,
		OpenablePaths:         append([]string(nil), reportData.OpenablePaths...),
		CapturedInputs:        inputs,
		CapturedInputsSHA256:  inputsDigest,
		Freshness:             freshness.NewFreshnessResult(freshness.FreshnessFresh),
		MaterialInputs: reportpkg.MaterialInputs{
			SelectedRevision: state.Head, InputPolicyVersion: "captured-inputs-v1",
			ArchitectureContract: 1, ReportContract: reportpkg.CurrentFormatVersion,
		},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, reportpkg.RunManifestFilename), manifestJSON, 0o600); err != nil {
		t.Fatal(err)
	}
}
