package reportserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	reportpkg "github.com/dvordrova/repomap/internal/report"
)

func TestReportBrowserRoutesUseTypedErrorPresentation(t *testing.T) {
	t.Parallel()

	t.Run("empty root", func(t *testing.T) {
		t.Parallel()
		h := &handler{runsDir: t.TempDir()}
		response := httptest.NewRecorder()
		h.serveRoot(response, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil))
		assertBrowserErrorResponse(
			t,
			response,
			http.StatusNotFound,
			"en",
			"main.browser_error.no_saved_reports",
		)
	})

	t.Run("root refresh failure", func(t *testing.T) {
		t.Parallel()
		var logs []string
		h := &handler{
			runsDir: filepath.Join(t.TempDir(), "missing"),
			logf: func(format string, args ...any) {
				logs = append(logs, fmt.Sprintf(format, args...))
			},
		}
		response := httptest.NewRecorder()
		h.serveRoot(response, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil))
		assertBrowserErrorResponse(
			t,
			response,
			http.StatusInternalServerError,
			"en",
			"main.browser_error.report_temporarily_unavailable",
		)
		if len(logs) == 0 || !strings.Contains(logs[0], "could not refresh saved reports") {
			t.Fatalf("root refresh diagnostics = %q", logs)
		}
	})

	t.Run("root refresh failure keeps known Russian locale", func(t *testing.T) {
		t.Parallel()
		repository := t.TempDir()
		runsDir := t.TempDir()
		const runID = "20260731-155900-russian-cached"
		writeRun(t, runsDir, runID, repository, "saved HTML")
		metadataPath := filepath.Join(runsDir, runID, "metadata.json")
		metadataJSON, err := os.ReadFile(metadataPath)
		if err != nil {
			t.Fatal(err)
		}
		var meta metadata
		if err := json.Unmarshal(metadataJSON, &meta); err != nil {
			t.Fatal(err)
		}
		meta.EffectiveOptions.ReportLanguage = "ru"
		metadataJSON, err = json.Marshal(meta)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(metadataPath, metadataJSON, 0o600); err != nil {
			t.Fatal(err)
		}
		h := &handler{runsDir: runsDir, initialRunID: runID}
		if err := h.reloadRuns(); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(runsDir, runsDir+"-moved"); err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		h.serveRoot(response, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil))
		assertBrowserErrorResponse(
			t,
			response,
			http.StatusInternalServerError,
			"ru",
			"main.browser_error.report_temporarily_unavailable",
		)
	})

	t.Run("invalid report id", func(t *testing.T) {
		t.Parallel()
		h := &handler{runsDir: t.TempDir()}
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/report.html", nil)
		request.SetPathValue("runID", "../invalid")
		response := httptest.NewRecorder()
		h.serveReport(response, request)
		assertBrowserErrorResponse(
			t,
			response,
			http.StatusNotFound,
			"en",
			"main.browser_error.report_not_found",
		)
	})

	t.Run("known Russian report has no verified authority", func(t *testing.T) {
		t.Parallel()
		repository := t.TempDir()
		runsDir := t.TempDir()
		const runID = "20260731-160000-russian-unverified"
		writeRun(t, runsDir, runID, repository, "untrusted saved HTML")
		runDir := filepath.Join(runsDir, runID)
		if err := os.Remove(filepath.Join(runDir, reportpkg.RunManifestFilename)); err != nil {
			t.Fatal(err)
		}
		metadataPath := filepath.Join(runDir, "metadata.json")
		metadataJSON, err := os.ReadFile(metadataPath)
		if err != nil {
			t.Fatal(err)
		}
		var meta metadata
		if err := json.Unmarshal(metadataJSON, &meta); err != nil {
			t.Fatal(err)
		}
		meta.EffectiveOptions.ReportLanguage = "ru"
		metadataJSON, err = json.Marshal(meta)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(metadataPath, metadataJSON, 0o600); err != nil {
			t.Fatal(err)
		}

		h := &handler{runsDir: runsDir}
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/report.html", nil)
		request.SetPathValue("runID", runID)
		response := httptest.NewRecorder()
		h.serveReport(response, request)
		assertBrowserErrorResponse(
			t,
			response,
			http.StatusConflict,
			"ru",
			"main.browser_error.report_authority_unavailable",
		)
		if strings.Contains(response.Body.String(), "untrusted saved HTML") {
			t.Fatal("browser error response exposed the unverified saved artifact")
		}
	})
}

func TestSecurityHeadersUseTypedErrorForBrowserHostRejection(t *testing.T) {
	t.Parallel()

	handler := securityHeaders(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("rejected browser request reached the report handler")
		}),
		"127.0.0.1:41000",
	)
	request := httptest.NewRequest(http.MethodGet, "http://invalid.example/report.html", nil)
	request.Host = "invalid.example"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assertBrowserErrorResponse(
		t,
		response,
		http.StatusForbidden,
		"en",
		"main.browser_error.report_temporarily_unavailable",
	)
	if strings.Contains(response.Body.String(), "invalid host") {
		t.Fatal("browser host rejection exposed the raw security diagnostic")
	}
}

func TestUnknownBrowserGETUsesTypedErrorPresentation(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(Options{
		RunsDir:    t.TempDir(),
		Capability: testCapability,
		OpenFile: func(context.Context, string, int, int) error {
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodGet,
		"http://127.0.0.1/not-a-repomap-route",
		nil,
	)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assertBrowserErrorResponse(
		t,
		response,
		http.StatusNotFound,
		"en",
		"main.browser_error.report_not_found",
	)
	if strings.Contains(response.Body.String(), "404 page not found") {
		t.Fatal("unknown browser route exposed the raw ServeMux response")
	}
}

func TestBrowserGETWithoutTrailingCapabilitySlashKeepsMuxRedirect(t *testing.T) {
	t.Parallel()

	handler, err := NewHandler(Options{
		RunsDir:    t.TempDir(),
		Capability: testCapability,
		OpenFile: func(context.Context, string, int, int) error {
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodGet,
		"http://127.0.0.1"+capabilityURLPrefix(testCapability),
		nil,
	)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusTemporaryRedirect)
	}
	if got, want := response.Header().Get("Location"), capabilityURLPrefix(testCapability)+"/"; got != want {
		t.Fatalf("location = %q, want %q", got, want)
	}
}

func assertBrowserErrorResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
	wantStatus int,
	wantLocale,
	wantMessageID string,
) {
	t.Helper()
	if response.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, wantStatus, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache control = %q", got)
	}
	for _, want := range []string{
		`<html lang="` + wantLocale + `">`,
		`data-rm-message="` + wantMessageID + `"`,
		"window.RepomapUI.message",
	} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("browser error response is missing %q", want)
		}
	}
}
