package reportserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
	"github.com/dvordrova/repomap/internal/testevidence"
	"github.com/dvordrova/repomap/internal/workspaceopen"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

const (
	maxArtifactBytes   = 32 * 1024 * 1024
	maxSourceHashBytes = workspaceopen.MaxHashBytes
	capabilityBytes    = 32
)

type OpenFileFunc func(ctx context.Context, absolutePath string, line, column int) error

type Options struct {
	RunsDir             string
	InitialRunID        string
	Port                int
	Capability          string
	ExpectedHost        string
	OpenFile            OpenFileFunc
	LocationResolver    analysis.LocationResolver
	ExactSymbolAnalyzer analysis.ExactSymbolAnalyzer
	ReferenceFinder     testevidence.ReferenceFinder
	CaptureRepository   CaptureRepositoryFunc
	CaptureFactContext  CaptureFactContextFunc
	AnalysisTimeout     time.Duration
	// SourceEpisodeJSON is one optional, approved source-episode artifact for
	// InitialRunID. It is copied into memory, never persisted, and cannot affect
	// any other saved run served by this handler.
	SourceEpisodeJSON []byte
	Logf              func(string, ...any)
	OnReady           func(url string) error
}

type RunSummary struct {
	ID                string `json:"id"`
	RepoName          string `json:"repo_name,omitempty"`
	CreatedAt         string `json:"created_at,omitempty"`
	ReportLanguage    string `json:"report_language"`
	CacheMode         string `json:"cache_mode"`
	ShortID           string `json:"short_id"`
	AnalysisAvailable bool   `json:"analysis_available"`
}

type runRecord struct {
	RunSummary
	RepoPath           string
	Manifest           *report.RunManifest
	WorkspaceSnapshot  *workspacesnapshot.Snapshot
	Report             *report.ReportData
	ReportSHA256       string // Adapter-only binding for opaque browser IDs.
	SourceCatalog      *sourcecatalog.Catalog
	Sources            map[string]sourceTarget
	SourceContexts     map[string]sourceContextTarget
	ArtifactsSignature string
	RequestedLocale    string
}

func (run runRecord) verifyRepositoryState(current freshness.RepositoryState) error {
	if run.WorkspaceSnapshot != nil {
		return run.WorkspaceSnapshot.Verify(current)
	}
	return fmt.Errorf("workspace authority unavailable")
}

func (run runRecord) workspaceAnalysisRoot() string {
	if run.WorkspaceSnapshot != nil {
		return run.WorkspaceSnapshot.AnalysisRoot()
	}
	return run.RepoPath
}

func (run runRecord) workspaceRepositoryDigest() string {
	if run.WorkspaceSnapshot != nil {
		return run.WorkspaceSnapshot.RepositoryDigest()
	}
	if run.Manifest != nil {
		return run.Manifest.RepositoryStateSHA256
	}
	return ""
}

type runIndex struct {
	runs []runRecord
	byID map[string]runRecord
}

type handler struct {
	runsDir            string
	initialRunID       string
	urlPrefix          string
	openFile           OpenFileFunc
	openSlot           chan struct{}
	sourceSlot         chan struct{}
	analysis           *symbolAnalysis
	captureRepo        CaptureRepositoryFunc
	sourceEpisodeRunID string
	sourceEpisodeJSON  []byte
	logf               func(string, ...any)
	runsMu             sync.RWMutex
	reloadMu           sync.Mutex
	runIndex           *runIndex
}

type metadata struct {
	RepoName         string `json:"repo_name"`
	RepoPath         string `json:"repo_path"`
	CreatedAt        string `json:"created_at"`
	EffectiveOptions struct {
		ReportLanguage string `json:"report_language"`
		NoCache        bool   `json:"no_cache"`
	} `json:"effective_options"`
}

type openRequest struct {
	RunID    string `json:"run_id"`
	SourceID string `json:"source_id"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
}

type sourceTarget struct {
	relativePath   string
	capturedSHA256 string
}

func Serve(ctx context.Context, opts Options) error {
	started := time.Now()
	if ctx == nil {
		return fmt.Errorf("report server: context is required")
	}
	if opts.Port < 0 || opts.Port > 65535 {
		return fmt.Errorf("report server: port must be between 0 and 65535")
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", opts.Port))
	if err != nil {
		return fmt.Errorf("report server: listen: %w", err)
	}

	address := listener.Addr().(*net.TCPAddr)
	opts.Capability = strings.TrimSpace(opts.Capability)
	if opts.Capability == "" {
		opts.Capability, err = generateCapability()
		if err != nil {
			_ = listener.Close()
			return fmt.Errorf("report server: generate capability: %w", err)
		}
	}
	opts.ExpectedHost = net.JoinHostPort("127.0.0.1", strconv.Itoa(address.Port))
	h, err := NewHandler(opts)
	if err != nil {
		_ = listener.Close()
		return err
	}
	if opts.Logf != nil {
		opts.Logf("report server ready in %d ms", time.Since(started).Milliseconds())
	}

	urlPrefix := capabilityURLPrefix(opts.Capability)
	url := fmt.Sprintf("http://127.0.0.1:%d%s/", address.Port, urlPrefix)
	if opts.InitialRunID != "" {
		url += "runs/" + opts.InitialRunID + "/report.html"
	}
	if opts.OnReady != nil {
		if err := opts.OnReady(url); err != nil {
			_ = listener.Close()
			return fmt.Errorf("report server: ready callback: %w", err)
		}
	}

	server := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-serveCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	err = server.Serve(listener)
	if !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("report server: serve: %w", err)
	}
	<-shutdownDone
	return nil
}

func NewHandler(opts Options) (http.Handler, error) {
	if strings.TrimSpace(opts.RunsDir) == "" {
		return nil, fmt.Errorf("report server: runs directory is required")
	}
	var sourceEpisodeJSON []byte
	if len(opts.SourceEpisodeJSON) > 0 {
		if strings.TrimSpace(opts.InitialRunID) == "" {
			return nil, fmt.Errorf("report server: source episode requires an initial run")
		}
		if len(opts.SourceEpisodeJSON) > report.MaxSourceEpisodeBytes {
			return nil, fmt.Errorf("report server: source episode exceeds %d bytes", report.MaxSourceEpisodeBytes)
		}
		sourceEpisodeJSON = append([]byte(nil), opts.SourceEpisodeJSON...)
	}
	runsDir, err := filepath.Abs(opts.RunsDir)
	if err != nil {
		return nil, fmt.Errorf("report server: resolve runs directory: %w", err)
	}
	runsInfo, err := os.Stat(runsDir)
	if err != nil || !runsInfo.IsDir() {
		return nil, fmt.Errorf("report server: runs directory is unavailable: %s", runsDir)
	}
	openFile := opts.OpenFile
	if openFile == nil {
		openFile, err = NewVSCodeLauncher(opts.Logf)
		if err != nil {
			openFile = unavailableEditorLauncher(err)
		}
	}
	capability := strings.TrimSpace(opts.Capability)
	if capability == "" {
		capability, err = generateCapability()
		if err != nil {
			return nil, fmt.Errorf("report server: generate capability: %w", err)
		}
	}
	if !validCapability(capability) {
		return nil, fmt.Errorf("report server: invalid capability")
	}
	if opts.ExpectedHost != "" && !validExpectedHost(opts.ExpectedHost) {
		return nil, fmt.Errorf("report server: invalid expected host")
	}
	urlPrefix := capabilityURLPrefix(capability)
	captureRepo := opts.CaptureRepository
	if captureRepo == nil {
		captureRepo = freshness.CaptureRepository
	}
	h := &handler{
		runsDir:      runsDir,
		initialRunID: opts.InitialRunID,
		urlPrefix:    urlPrefix,
		openFile:     openFile,
		openSlot:     make(chan struct{}, 1),
		sourceSlot:   make(chan struct{}, 1),
		analysis:     newSymbolAnalysis(opts),
		captureRepo:  captureRepo,
		logf:         opts.Logf,
	}
	if err := h.reloadRuns(); err != nil {
		return nil, fmt.Errorf("report server: list runs: %w", err)
	}
	if opts.InitialRunID != "" {
		if !validRunID(opts.InitialRunID) {
			return nil, fmt.Errorf("report server: invalid initial run id")
		}
		initialRun, findErr := h.findRunCached(opts.InitialRunID)
		if findErr != nil {
			return nil, fmt.Errorf("report server: initial run not found: %s", opts.InitialRunID)
		}
		if len(sourceEpisodeJSON) > 0 {
			if initialRun.Manifest == nil || initialRun.Report == nil {
				return nil, fmt.Errorf("report server: source episode requires a verified initial run")
			}
			if _, renderErr := report.RenderHTMLWithSourceEpisode(initialRun.Report, sourceEpisodeJSON); renderErr != nil {
				return nil, fmt.Errorf("report server: validate source episode for initial run: %w", renderErr)
			}
			h.sourceEpisodeRunID = opts.InitialRunID
			h.sourceEpisodeJSON = sourceEpisodeJSON
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+urlPrefix+"/{$}", h.serveRoot)
	mux.HandleFunc("GET "+urlPrefix+"/api/runs", h.serveRuns)
	mux.HandleFunc("POST "+urlPrefix+"/api/open", h.serveOpen)
	mux.HandleFunc("POST "+urlPrefix+"/api/source-context", h.serveSourceContext)
	mux.HandleFunc("POST "+urlPrefix+"/api/symbols", h.serveSymbols)
	mux.HandleFunc("POST "+urlPrefix+"/api/symbol", h.serveInspectSymbol)
	mux.HandleFunc("POST "+urlPrefix+"/api/investigation/latest", h.serveLatestInvestigation)
	mux.HandleFunc("POST "+urlPrefix+"/api/investigation/target-tests", h.serveTargetTestReferences)
	mux.HandleFunc("GET "+urlPrefix+"/runs/{runID}/report.html", h.serveReport)
	routes := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && !knownBrowserGETPath(urlPrefix, r.URL.Path) {
			h.serveUnknownBrowserRoute(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
	return securityHeaders(routes, opts.ExpectedHost), nil
}

func (h *handler) serveUnknownBrowserRoute(w http.ResponseWriter, r *http.Request) {
	h.writeBrowserError(
		w,
		http.StatusNotFound,
		"en",
		report.BrowserErrorReportNotFound,
		"unknown browser route %q",
		r.URL.Path,
	)
}

func knownBrowserGETPath(urlPrefix, requestPath string) bool {
	switch requestPath {
	case urlPrefix, urlPrefix + "/", urlPrefix + "/api/runs":
		return true
	}
	const reportSuffix = "/report.html"
	runPrefix := urlPrefix + "/runs/"
	if !strings.HasPrefix(requestPath, runPrefix) ||
		!strings.HasSuffix(requestPath, reportSuffix) {
		return false
	}
	runID := strings.TrimSuffix(strings.TrimPrefix(requestPath, runPrefix), reportSuffix)
	return runID != "" && !strings.Contains(runID, "/")
}

func (h *handler) serveRoot(w http.ResponseWriter, r *http.Request) {
	requestedLocale := "en"
	cachedRuns := h.runsSnapshot()
	cachedRunID := h.initialRunID
	if !containsRun(cachedRuns, cachedRunID) && len(cachedRuns) > 0 {
		cachedRunID = cachedRuns[0].ID
	}
	if cached, err := h.findRunCached(cachedRunID); err == nil {
		requestedLocale = cached.RequestedLocale
	}
	if err := h.reloadRuns(); err != nil {
		h.writeBrowserError(
			w,
			http.StatusInternalServerError,
			requestedLocale,
			report.BrowserErrorReportUnavailable,
			"report root could not refresh saved reports: %v",
			err,
		)
		return
	}
	runs := h.runsSnapshot()
	runID := h.initialRunID
	if !containsRun(runs, runID) {
		if len(runs) == 0 {
			h.writeBrowserError(
				w,
				http.StatusNotFound,
				"en",
				report.BrowserErrorNoSavedReports,
				"report root has no saved reports",
			)
			return
		}
		runID = runs[0].ID
	}
	http.Redirect(w, r, h.urlPrefix+"/runs/"+runID+"/report.html", http.StatusFound)
}

func (h *handler) serveRuns(w http.ResponseWriter, _ *http.Request) {
	_ = h.reloadRuns()
	runs := h.runsSnapshot()
	summaries := make([]RunSummary, 0, len(runs))
	for _, run := range runs {
		summaries = append(summaries, run.RunSummary)
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": summaries})
}

func (h *handler) serveReport(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	runID := r.PathValue("runID")
	if !validRunID(runID) {
		h.writeBrowserError(
			w,
			http.StatusNotFound,
			"en",
			report.BrowserErrorReportNotFound,
			"report request has an invalid run id",
		)
		return
	}
	requestedLocale := "en"
	if cached, err := h.findRunCached(runID); err == nil {
		requestedLocale = cached.RequestedLocale
	}
	if err := h.reloadRuns(); err != nil {
		h.writeBrowserError(
			w,
			http.StatusInternalServerError,
			requestedLocale,
			report.BrowserErrorReportUnavailable,
			"report %s could not refresh saved reports: %v",
			runID,
			err,
		)
		return
	}
	run, err := h.findRunCached(runID)
	if err != nil {
		h.writeBrowserError(
			w,
			http.StatusNotFound,
			requestedLocale,
			report.BrowserErrorReportNotFound,
			"report %s was not found after refreshing saved reports",
			runID,
		)
		return
	}
	requestedLocale = run.RequestedLocale
	if run.Manifest == nil || run.Report == nil {
		h.writeBrowserError(
			w,
			http.StatusConflict,
			requestedLocale,
			report.BrowserErrorAuthorityUnavailable,
			"report %s cannot be served without verified local authority",
			runID,
		)
		return
	}
	reportData := *run.Report
	run.Report = &reportData
	h.refreshRunFreshness(r.Context(), &run)
	if runID == h.sourceEpisodeRunID && len(h.sourceEpisodeJSON) > 0 {
		if err := report.AttachSourceEpisodePresentation(
			run.Report,
			h.sourceEpisodeJSON,
		); err != nil {
			h.writeBrowserError(
				w,
				http.StatusInternalServerError,
				requestedLocale,
				report.BrowserErrorReportUnavailable,
				"report %s could not bind approved source episode: %v",
				runID,
				err,
			)
			return
		}
	}
	localizedReport, localizationStatus := report.LoadPresentationLocalization(
		filepath.Join(h.runsDir, run.ID),
		run.Report,
		run.RequestedLocale,
	)
	run.Report = localizedReport
	if localizationStatus.State == report.PresentationLocalizationFailed {
		h.log(
			"report %s localization failed reason=%s; serving Russian product UI with canonical English model prose",
			run.ID,
			localizationStatus.ReasonCode,
		)
	}
	renderStarted := time.Now()
	var rendered []byte
	if runID == h.sourceEpisodeRunID && len(h.sourceEpisodeJSON) > 0 {
		rendered, err = report.RenderHTMLWithSourceEpisode(run.Report, h.sourceEpisodeJSON)
	} else {
		rendered, err = report.RenderHTML(run.Report)
	}
	if err != nil {
		h.writeBrowserError(
			w,
			http.StatusInternalServerError,
			requestedLocale,
			report.BrowserErrorReportUnavailable,
			"report %s could not render: %v",
			runID,
			err,
		)
		return
	}
	if len(rendered) > maxArtifactBytes {
		h.writeBrowserError(
			w,
			http.StatusUnprocessableEntity,
			requestedLocale,
			report.BrowserErrorInvalidArtifact,
			"report %s rendered artifact exceeds the byte limit: bytes=%d limit=%d",
			runID,
			len(rendered),
			maxArtifactBytes,
		)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "report.html", time.Time{}, bytes.NewReader(rendered))
	h.log("served report %s: load+freshness=%d ms render=%d ms total=%d ms bytes=%d",
		runID,
		renderStarted.Sub(started).Milliseconds(),
		time.Since(renderStarted).Milliseconds(),
		time.Since(started).Milliseconds(),
		len(rendered),
	)
}

func (h *handler) writeBrowserError(
	w http.ResponseWriter,
	status int,
	locale string,
	kind report.BrowserErrorKind,
	diagnostic string,
	args ...any,
) {
	if diagnostic != "" {
		h.log(diagnostic, args...)
	}
	writeBrowserErrorResponse(w, status, locale, kind, h.log)
}

func writeBrowserErrorResponse(
	w http.ResponseWriter,
	status int,
	locale string,
	kind report.BrowserErrorKind,
	logf func(string, ...any),
) {
	rendered, err := report.RenderBrowserErrorHTML(locale, kind)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		if logf != nil {
			logf("browser error page render failed kind=%s status=%d: %v", kind, status, err)
		}
		w.WriteHeader(status)
		return
	}
	w.WriteHeader(status)
	if _, err := w.Write(rendered); err != nil {
		if logf != nil {
			logf("browser error page write failed kind=%s status=%d: %v", kind, status, err)
		}
	}
}

func (h *handler) serveOpen(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	if r.Header.Get("X-Repomap-Action") != "open-file" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing repomap action header"})
		return
	}
	defer r.Body.Close()
	var request openRequest
	if err := decodeJSONBody(w, r, &request, 4096); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid open-file request"})
		return
	}
	if request.Line < 0 || request.Line > 10_000_000 || request.Column < 0 || request.Column > 10_000_000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid source location"})
		return
	}
	resolveRunStarted := time.Now()
	run, err := h.findRun(request.RunID)
	resolveRunMS := time.Since(resolveRunStarted).Milliseconds()
	if err != nil {
		h.logSourceOpen(request.RunID, request.SourceID, "run_not_found", resolveRunMS, 0, 0, 0, started)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "report run not found"})
		return
	}
	authorizeStarted := time.Now()
	if run.Manifest == nil || !filepath.IsAbs(run.RepoPath) ||
		run.WorkspaceSnapshot == nil || run.SourceCatalog == nil {
		h.logSourceOpen(run.ID, request.SourceID, "view_only", resolveRunMS, time.Since(authorizeStarted).Milliseconds(), 0, 0, started)
		writeJSON(w, http.StatusConflict, map[string]string{"error": "this report is view-only; regenerate it to enable editor actions"})
		return
	}
	target, ok := run.Sources[strings.TrimSpace(request.SourceID)]
	authorizeMS := time.Since(authorizeStarted).Milliseconds()
	if !ok || request.SourceID == "" {
		h.logSourceOpen(run.ID, request.SourceID, "source_unauthorized", resolveRunMS, authorizeMS, 0, 0, started)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "source is not authorized by this report"})
		return
	}
	resolveTargetStarted := time.Now()
	// Target resolution and hashing historically ignored request cancellation;
	// the launcher below still receives the original request context.
	absolutePath, sourceChanged, resolveErr := resolveOpenTarget(
		context.WithoutCancel(r.Context()),
		run,
		target,
	)
	if resolveErr != nil {
		resolveTargetMS := time.Since(resolveTargetStarted).Milliseconds()
		h.logSourceOpen(run.ID, request.SourceID, "source_unavailable", resolveRunMS, authorizeMS, resolveTargetMS, 0, started)
		writeJSON(w, http.StatusConflict, map[string]string{"error": "authorized source is unavailable", "code": "source_unavailable"})
		return
	}
	resolveTargetMS := time.Since(resolveTargetStarted).Milliseconds()
	select {
	case h.openSlot <- struct{}{}:
		defer func() { <-h.openSlot }()
	default:
		h.logSourceOpen(run.ID, request.SourceID, "editor_busy", resolveRunMS, authorizeMS, resolveTargetMS, 0, started)
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "another editor action is still running", "code": "editor_busy"})
		return
	}
	spawnStarted := time.Now()
	if err := h.openFile(r.Context(), absolutePath, request.Line, request.Column); err != nil {
		status := http.StatusBadGateway
		code := "editor_launch_failed"
		if errors.Is(err, ErrEditorUnavailable) {
			status = http.StatusServiceUnavailable
			code = "editor_unavailable"
		}
		h.logSourceOpen(run.ID, request.SourceID, code, resolveRunMS, authorizeMS, resolveTargetMS, time.Since(spawnStarted).Milliseconds(), started)
		writeJSON(w, status, map[string]string{"error": "could not open file in VS Code", "code": code})
		return
	}
	spawnMS := time.Since(spawnStarted).Milliseconds()
	writeJSON(w, http.StatusOK, map[string]any{"status": "opened", "source_changed": sourceChanged})
	h.logSourceOpen(run.ID, request.SourceID, "opened", resolveRunMS, authorizeMS, resolveTargetMS, spawnMS, started)
}

func (h *handler) findRun(runID string) (runRecord, error) {
	if !validRunID(runID) {
		return runRecord{}, fmt.Errorf("invalid run id")
	}
	run, err := h.findRunCached(runID)
	if err == nil && !h.runArtifactsChanged(run) {
		return run, nil
	}
	if reloadErr := h.reloadRuns(); reloadErr != nil {
		return runRecord{}, reloadErr
	}
	return h.findRunCached(runID)
}

func manifestSourceID(runID, reportSHA256, relativePath string) string {
	digest := sha256.Sum256([]byte("repomap-source-v1\x00" + runID + "\x00" + reportSHA256 + "\x00" + relativePath))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func (h *handler) reloadRuns() error {
	h.reloadMu.Lock()
	defer h.reloadMu.Unlock()

	runs, err := h.loadRuns()
	if err != nil {
		return err
	}
	byID := make(map[string]runRecord, len(runs))
	for index := range runs {
		run := &runs[index]
		if run.WorkspaceSnapshot != nil {
			catalog := run.WorkspaceSnapshot.Catalog()
			run.SourceCatalog = &catalog
		}
		preparedReport, preparationErr := report.PrepareRunPresentation(
			filepath.Join(h.runsDir, run.ID),
			run.Report,
			nil,
		)
		if preparedReport != nil {
			run.Report = preparedReport
		}
		if preparationErr != nil {
			h.log("report %s presentation preparation unavailable: %v", run.ID, preparationErr)
		}
		if run.SourceCatalog != nil {
			if searchErr := report.AttachExactWorkspaceSearch(run.Report, *run.SourceCatalog); searchErr != nil {
				h.log("report %s exact workspace search unavailable: %v", run.ID, searchErr)
			}
		}
		if run.Manifest != nil && run.Report != nil {
			switch {
			case run.SourceCatalog != nil:
				run.Sources, run.Report.SourceIDs = catalogSourceTargets(
					run.ID,
					run.ReportSHA256,
					*run.SourceCatalog,
				)
			default:
				run.Sources = make(map[string]sourceTarget)
				run.Report.SourceIDs = make(map[string]string)
			}
			run.SourceContexts = make(map[string]sourceContextTarget)
			run.Report.SourceContextIDs = make(map[string]string)
			if run.SourceCatalog != nil {
				for _, snippet := range reportSourceSnippets(run.Report) {
					if err := snippet.Validate(); err != nil {
						continue
					}
					source, ok := run.SourceCatalog.Lookup(snippet.Path)
					if !ok || source.Kind != freshness.FileRegular || source.ContentSHA256 == "" {
						continue
					}
					contextID := manifestSourceContextID(run.ID, run.ReportSHA256, snippet.PresentationSHA256)
					focusLine := snippet.StartLine
					if len(snippet.HighlightRanges) > 0 {
						focusLine = snippet.HighlightRanges[0].StartLine
					}
					run.SourceContexts[contextID] = sourceContextTarget{
						relativePath: snippet.Path, capturedSHA256: source.ContentSHA256,
						startLine: snippet.StartLine, endLine: snippet.EndLine, focusLine: focusLine,
					}
					run.Report.SourceContextIDs[snippet.PresentationSHA256] = contextID
				}
			}
		}
		byID[run.ID] = *run
	}
	next := &runIndex{runs: runs, byID: byID}
	h.runsMu.Lock()
	h.runIndex = next
	h.runsMu.Unlock()
	return nil
}

func (h *handler) runsSnapshot() []runRecord {
	h.runsMu.RLock()
	defer h.runsMu.RUnlock()
	if h.runIndex == nil {
		return nil
	}
	return h.runIndex.runs
}

func (h *handler) findRunCached(runID string) (runRecord, error) {
	h.runsMu.RLock()
	defer h.runsMu.RUnlock()
	if h.runIndex != nil {
		if run, ok := h.runIndex.byID[runID]; ok {
			return run, nil
		}
	}
	return runRecord{}, fmt.Errorf("run not found")
}

func (h *handler) runArtifactsChanged(run runRecord) bool {
	signature, err := runArtifactSignature(filepath.Join(h.runsDir, run.ID))
	return err != nil || signature != run.ArtifactsSignature
}

func runArtifactSignature(runDir string) (string, error) {
	parts := make([]string, 0, 8)
	for index, name := range []string{
		"report.json", report.RunManifestFilename,
		"task_investigation_bundle.json", "task_investigation_attempt.json",
		"task_investigation_pack.json", "task_investigation_status.json",
		report.PresentationLocalizationStatusFile,
	} {
		optionalPresentationArtifact := index >= 6
		info, err := os.Lstat(filepath.Join(runDir, name))
		if os.IsNotExist(err) && index > 0 {
			parts = append(parts, name+":missing")
			continue
		}
		if err != nil {
			if optionalPresentationArtifact {
				parts = append(parts, name+":unavailable")
				continue
			}
			return "", fmt.Errorf("run artifact is unavailable")
		}
		if !info.Mode().IsRegular() {
			if optionalPresentationArtifact {
				parts = append(parts, fmt.Sprintf(
					"%s:invalid:%s:%d:%d",
					name,
					info.Mode().String(),
					info.Size(),
					info.ModTime().UnixNano(),
				))
				continue
			}
			return "", fmt.Errorf("run artifact is unavailable")
		}
		parts = append(parts, fmt.Sprintf("%s:%d:%d", name, info.Size(), info.ModTime().UnixNano()))
	}
	return strings.Join(parts, "|"), nil
}

func catalogSourceTargets(
	runID, reportSHA256 string,
	catalog sourcecatalog.Catalog,
) (map[string]sourceTarget, map[string]string) {
	paths := catalog.Paths()
	targets := make(map[string]sourceTarget, len(paths))
	sourceIDs := make(map[string]string, len(paths))
	for _, relativePath := range paths {
		source, ok := catalog.Lookup(relativePath)
		if !ok {
			continue
		}
		sourceID := manifestSourceID(runID, reportSHA256, relativePath)
		targets[sourceID] = sourceTarget{
			relativePath:   source.Path,
			capturedSHA256: source.ContentSHA256,
		}
		sourceIDs[relativePath] = sourceID
	}
	return targets, sourceIDs
}

func resolveOpenTarget(
	ctx context.Context,
	run runRecord,
	target sourceTarget,
) (string, bool, error) {
	if run.WorkspaceSnapshot != nil {
		service, err := workspaceopen.New(*run.WorkspaceSnapshot)
		if err != nil {
			return "", false, err
		}
		resolved, err := service.Resolve(ctx, workspaceopen.Request{
			Path:         target.relativePath,
			MaxHashBytes: maxSourceHashBytes,
		})
		if err != nil {
			return "", false, err
		}
		return resolved.AbsolutePath, resolved.SourceChanged, nil
	}
	return "", false, fmt.Errorf("workspace snapshot unavailable")
}

func (h *handler) logSourceOpen(
	runID, sourceID, outcome string,
	resolveRunMS, authorizeMS, resolveTargetMS, spawnMS int64,
	started time.Time,
) {
	h.log("source open run=%s source=%s outcome=%s source_open.resolve_run_ms=%d source_open.authorize_ms=%d source_open.resolve_target_ms=%d source_open.spawn_ms=%d source_open.response_ms=%d",
		runID, sourceID, outcome, resolveRunMS, authorizeMS, resolveTargetMS, spawnMS, time.Since(started).Milliseconds())
}

func (h *handler) loadRuns() ([]runRecord, error) {
	started := time.Now()
	entries, err := os.ReadDir(h.runsDir)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(h.runsDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	var runs []runRecord
	for _, entry := range entries {
		if !entry.IsDir() || !validRunID(entry.Name()) {
			continue
		}
		artifactSignature, signatureErr := runArtifactSignature(filepath.Join(h.runsDir, entry.Name()))
		if signatureErr != nil {
			continue
		}
		info, err := root.Lstat(path.Join(entry.Name(), "report.html"))
		if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxArtifactBytes {
			continue
		}
		reportJSON, err := readRootFile(root, path.Join(entry.Name(), "report.json"), maxArtifactBytes)
		if err != nil {
			continue
		}
		var reportData report.ReportData
		if json.Unmarshal(reportJSON, &reportData) != nil || reportData.FormatVersion != report.CurrentFormatVersion {
			continue
		}

		var meta metadata
		if data, err := readRootFile(root, path.Join(entry.Name(), "metadata.json"), 1024*1024); err == nil {
			_ = json.Unmarshal(data, &meta)
		}
		repoName := strings.TrimSpace(meta.RepoName)
		if repoName == "" {
			repoName = reportData.RepoName
		}
		run := runRecord{
			RunSummary: RunSummary{
				ID:             entry.Name(),
				RepoName:       repoName,
				CreatedAt:      meta.CreatedAt,
				ReportLanguage: runReportLanguage(meta.EffectiveOptions.ReportLanguage),
				CacheMode:      runCacheMode(meta.EffectiveOptions.NoCache),
				ShortID:        shortRunID(entry.Name()),
			},
			Report:             &reportData,
			ArtifactsSignature: artifactSignature,
			RequestedLocale:    meta.EffectiveOptions.ReportLanguage,
		}
		manifestJSON, manifestErr := readRootFile(root, path.Join(entry.Name(), report.RunManifestFilename), maxArtifactBytes)
		if manifestErr == nil {
			manifest, decodeErr := report.DecodeRunManifest(manifestJSON)
			if decodeErr == nil && manifest.VerifyReportJSON(reportJSON) == nil &&
				manifest.VerifyTaskInvestigationArtifacts(filepath.Join(h.runsDir, entry.Name())) == nil &&
				manifest.VerifyOrientationContextSelectionArtifact(filepath.Join(h.runsDir, entry.Name())) == nil {
				analysisRoot, rootErr := manifest.ResolveAnalysisRoot()
				if rootErr == nil {
					run.Manifest = &manifest
					run.RepoPath = analysisRoot
					run.ReportSHA256 = manifest.ReportSHA256
					snapshot, catalog, snapshotErr := workspaceSnapshotForManifest(manifest, analysisRoot)
					if snapshotErr == nil {
						run.WorkspaceSnapshot = snapshot
						run.SourceCatalog = catalog
						run.RepoPath = snapshot.AnalysisRoot()
					} else {
						h.log("report %s source catalog unavailable; local analysis disabled", run.ID)
					}
					run.AnalysisAvailable = run.WorkspaceSnapshot != nil &&
						run.SourceCatalog != nil &&
						h.analysisAvailable(manifest)
				}
			}
		}
		// Task Lens is an evidence-backed workspace, not a legacy view-only
		// report. Never expose it when its exact report/manifest/artifact chain
		// cannot be verified.
		if reportData.TaskInvestigation != nil && run.Manifest == nil {
			continue
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].ID > runs[j].ID })
	h.log("loaded %d saved report(s) in %d ms", len(runs), time.Since(started).Milliseconds())
	return runs, nil
}

func runReportLanguage(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "ru") {
		return "ru"
	}
	return "en"
}

func runCacheMode(noCache bool) string {
	if noCache {
		return "no-cache"
	}
	return "cache"
}

func shortRunID(runID string) string {
	value := runID
	if separator := strings.LastIndexByte(runID, '-'); separator >= 0 &&
		separator+1 < len(runID) {
		value = runID[separator+1:]
	}
	const maxShortRunIDBytes = 12
	if len(value) > maxShortRunIDBytes {
		value = value[len(value)-maxShortRunIDBytes:]
	}
	return value
}

func (h *handler) refreshRunFreshness(ctx context.Context, run *runRecord) {
	if run == nil || run.Manifest == nil || run.Report == nil || run.Manifest.Version != report.CurrentRunManifestVersion ||
		h.captureRepo == nil {
		return
	}
	started := time.Now()
	repositoryRoot := run.Manifest.RepositoryState.Identity
	if run.WorkspaceSnapshot != nil {
		repositoryRoot = run.WorkspaceSnapshot.RepositoryRoot()
	}
	current, err := h.captureRepo(ctx, repositoryRoot)
	if err != nil {
		result := freshness.NewFreshnessResult(freshness.FreshnessUnavailable)
		result.Diagnostics = []string{"current analyzed-input freshness could not be checked"}
		run.Report.Freshness = &result
		h.log("report %s freshness unavailable after %d ms", run.ID, time.Since(started).Milliseconds())
		return
	}
	result := run.Manifest.CurrentFreshness(current)
	if run.WorkspaceSnapshot != nil {
		result = run.WorkspaceSnapshot.Assess(current)
	}
	run.Report.Freshness = &result
	h.log("report %s freshness=%s in %d ms affected_inputs=%d affected_submodules=%d",
		run.ID,
		result.State,
		time.Since(started).Milliseconds(),
		len(result.AffectedInputIDs),
		len(result.AffectedSubmodules),
	)
}

func workspaceSnapshotForManifest(
	manifest report.RunManifest,
	resolvedRoot string,
) (*workspacesnapshot.Snapshot, *sourcecatalog.Catalog, error) {
	if manifest.Version != report.CurrentRunManifestVersion {
		return nil, nil, fmt.Errorf("workspace snapshot unavailable")
	}
	snapshot, err := manifest.WorkspaceSnapshot()
	if err != nil {
		return nil, nil, fmt.Errorf("workspace snapshot unavailable")
	}
	catalog := snapshot.Catalog()
	if snapshot.AnalysisRoot() != resolvedRoot || catalog.AnalysisRoot() != resolvedRoot {
		return nil, nil, fmt.Errorf("workspace snapshot root mismatch")
	}
	return &snapshot, &catalog, nil
}

func (h *handler) log(format string, args ...any) {
	if h.logf != nil {
		h.logf(format, args...)
	}
}

func readRootFile(root *os.Root, name string, maxBytes int64) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxBytes {
		return nil, fmt.Errorf("artifact is not a bounded regular file")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("artifact exceeds %d bytes", maxBytes)
	}
	return data, nil
}

func resolveRepoFile(repoPath, relativePath string) (string, error) {
	relativePath = filepath.FromSlash(strings.TrimSpace(relativePath))
	if !filepath.IsLocal(relativePath) || relativePath == "." {
		return "", fmt.Errorf("file path must stay inside the repository")
	}
	repoRoot, err := filepath.EvalSymlinks(repoPath)
	if err != nil {
		return "", fmt.Errorf("repository is unavailable")
	}
	unresolvedCandidate := filepath.Join(repoRoot, relativePath)
	entryInfo, err := os.Lstat(unresolvedCandidate)
	if err != nil {
		return "", fmt.Errorf("file does not exist")
	}
	if entryInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("symbolic-link files cannot be opened from a report")
	}
	candidate, err := filepath.EvalSymlinks(unresolvedCandidate)
	if err != nil {
		return "", fmt.Errorf("file does not exist")
	}
	rel, err := filepath.Rel(repoRoot, candidate)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("file path escapes the repository")
	}
	info, err := os.Stat(candidate)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("path is not a regular file")
	}
	return candidate, nil
}

func containsRun(runs []runRecord, id string) bool {
	for _, run := range runs {
		if run.ID == id {
			return true
		}
	}
	return false
}

func validRunID(id string) bool {
	if id == "" || id == "." || !filepath.IsLocal(id) || filepath.Base(id) != id {
		return false
	}
	for _, char := range id {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func securityHeaders(next http.Handler, expectedHost string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHost(r.Host, expectedHost) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				writeBrowserErrorResponse(
					w,
					http.StatusForbidden,
					"en",
					report.BrowserErrorReportUnavailable,
					nil,
				)
				return
			}
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; img-src data:; object-src 'none'; frame-ancestors 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.Method == http.MethodOptions {
			http.Error(w, "OPTIONS is not supported", http.StatusMethodNotAllowed)
			return
		}
		if r.Method == http.MethodPost {
			if !hasJSONContentType(r.Header.Get("Content-Type")) {
				http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
			if !sameOrigin(r.Header.Get("Origin"), r.Host) {
				http.Error(w, "invalid origin", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func allowedHost(value, expectedHost string) bool {
	if expectedHost != "" {
		return value == expectedHost
	}
	host := value
	if parsed, _, err := net.SplitHostPort(value); err == nil {
		host = parsed
	}
	host = strings.Trim(host, "[]")
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func hasJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && mediaType == "application/json"
}

func sameOrigin(origin, host string) bool {
	return origin == "http://"+host
}

func (h *handler) analysisAvailable(manifest report.RunManifest) bool {
	if h.analysis == nil || h.analysis.exact == nil {
		return false
	}
	for _, component := range manifest.Components {
		for _, anchor := range component.Anchors {
			if anchor.CanListSymbols {
				return true
			}
		}
	}
	return false
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any, maxBytes int64) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple json values")
		}
		return err
	}
	return nil
}

func generateCapability() (string, error) {
	buffer := make([]byte, capabilityBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func capabilityURLPrefix(capability string) string {
	return "/_repomap/" + capability
}

func validCapability(capability string) bool {
	if capability == "" || len(capability) > 256 {
		return false
	}
	for _, char := range capability {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func validExpectedHost(expectedHost string) bool {
	host, port, err := net.SplitHostPort(expectedHost)
	if err != nil || port == "" {
		return false
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return false
	}
	host = strings.Trim(host, "[]")
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
