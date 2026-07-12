package reportserver

import (
	"bytes"
	"context"
	"crypto/rand"
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
	"time"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/testevidence"
)

const (
	maxArtifactBytes = 32 * 1024 * 1024
	capabilityBytes  = 32
)

var (
	errRunViewOnly      = errors.New("report run has no verified local authority")
	errPathUnauthorized = errors.New("path is not authorized by this report")
)

type OpenFileFunc func(ctx context.Context, absolutePath string, line int) error

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
	Logf                func(string, ...any)
	OnReady             func(url string) error
}

type RunSummary struct {
	ID                string `json:"id"`
	RepoName          string `json:"repo_name,omitempty"`
	CreatedAt         string `json:"created_at,omitempty"`
	AnalysisAvailable bool   `json:"analysis_available"`
}

type runRecord struct {
	RunSummary
	RepoPath string
	Manifest *report.RunManifest
	Report   *report.ReportData
}

type handler struct {
	runsDir      string
	initialRunID string
	urlPrefix    string
	openFile     OpenFileFunc
	openSlot     chan struct{}
	analysis     *symbolAnalysis
	captureRepo  CaptureRepositoryFunc
	logf         func(string, ...any)
}

type metadata struct {
	RepoName  string `json:"repo_name"`
	RepoPath  string `json:"repo_path"`
	CreatedAt string `json:"created_at"`
}

type openRequest struct {
	RunID string `json:"run_id"`
	Path  string `json:"path"`
	Line  int    `json:"line,omitempty"`
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
		openFile = OpenInVSCode
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
		analysis:     newSymbolAnalysis(opts),
		captureRepo:  captureRepo,
		logf:         opts.Logf,
	}
	if opts.InitialRunID != "" {
		if !validRunID(opts.InitialRunID) {
			return nil, fmt.Errorf("report server: invalid initial run id")
		}
		runs, err := h.loadRuns()
		if err != nil {
			return nil, fmt.Errorf("report server: list runs: %w", err)
		}
		if !containsRun(runs, opts.InitialRunID) {
			return nil, fmt.Errorf("report server: initial run not found: %s", opts.InitialRunID)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET "+urlPrefix+"/{$}", h.serveRoot)
	mux.HandleFunc("GET "+urlPrefix+"/api/runs", h.serveRuns)
	mux.HandleFunc("POST "+urlPrefix+"/api/open", h.serveOpen)
	mux.HandleFunc("POST "+urlPrefix+"/api/symbols", h.serveSymbols)
	mux.HandleFunc("POST "+urlPrefix+"/api/symbol", h.serveInspectSymbol)
	mux.HandleFunc("POST "+urlPrefix+"/api/investigation/latest", h.serveLatestInvestigation)
	mux.HandleFunc("POST "+urlPrefix+"/api/investigation/target-tests", h.serveTargetTestReferences)
	mux.HandleFunc("GET "+urlPrefix+"/runs/{runID}/report.html", h.serveReport)
	return securityHeaders(mux, opts.ExpectedHost), nil
}

func (h *handler) serveRoot(w http.ResponseWriter, r *http.Request) {
	runs, err := h.loadRuns()
	if err != nil {
		http.Error(w, "could not list reports", http.StatusInternalServerError)
		return
	}
	runID := h.initialRunID
	if !containsRun(runs, runID) {
		if len(runs) == 0 {
			http.Error(w, "no saved reports", http.StatusNotFound)
			return
		}
		runID = runs[0].ID
	}
	http.Redirect(w, r, h.urlPrefix+"/runs/"+runID+"/report.html", http.StatusFound)
}

func (h *handler) serveRuns(w http.ResponseWriter, _ *http.Request) {
	runs, err := h.loadRuns()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not list reports"})
		return
	}
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
		http.NotFound(w, r)
		return
	}
	run, err := h.findRun(runID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if run.Manifest == nil || run.Report == nil {
		http.Error(w, "saved report cannot be served without verified local authority", http.StatusConflict)
		return
	}
	h.refreshRunFreshness(r.Context(), &run)
	renderStarted := time.Now()
	rendered, err := report.RenderHTML(run.Report)
	if err != nil {
		http.Error(w, "could not render report", http.StatusInternalServerError)
		return
	}
	if len(rendered) > maxArtifactBytes {
		http.Error(w, "invalid report artifact", http.StatusUnprocessableEntity)
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

func (h *handler) serveOpen(w http.ResponseWriter, r *http.Request) {
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
	if request.Line < 0 || request.Line > 10_000_000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid line number"})
		return
	}
	run, err := h.findAuthorizedRun(request.RunID, request.Path)
	if err != nil {
		switch {
		case errors.Is(err, errRunViewOnly):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "this report is view-only; regenerate it to enable editor actions"})
		case errors.Is(err, errPathUnauthorized):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "file is not authorized by this report"})
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "report run not found"})
		}
		return
	}
	absolutePath, err := resolveRepoFile(run.RepoPath, request.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	select {
	case h.openSlot <- struct{}{}:
		defer func() { <-h.openSlot }()
	default:
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "another editor action is still running"})
		return
	}
	if err := h.openFile(r.Context(), absolutePath, request.Line); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not open file in VS Code"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "opened"})
}

func (h *handler) findRun(runID string) (runRecord, error) {
	if !validRunID(runID) {
		return runRecord{}, fmt.Errorf("invalid run id")
	}
	runs, err := h.loadRuns()
	if err != nil {
		return runRecord{}, err
	}
	for _, run := range runs {
		if run.ID == runID {
			return run, nil
		}
	}
	return runRecord{}, fmt.Errorf("run not found")
}

func (h *handler) findAuthorizedRun(runID, requestedPath string) (runRecord, error) {
	run, err := h.findRun(runID)
	if err != nil {
		return runRecord{}, err
	}
	if run.Manifest == nil || !filepath.IsAbs(run.RepoPath) {
		return runRecord{}, errRunViewOnly
	}
	requestedPath = filepath.ToSlash(strings.TrimSpace(requestedPath))
	index := sort.SearchStrings(run.Manifest.OpenablePaths, requestedPath)
	if index >= len(run.Manifest.OpenablePaths) || run.Manifest.OpenablePaths[index] != requestedPath {
		return runRecord{}, errPathUnauthorized
	}
	return run, nil
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
			RunSummary: RunSummary{ID: entry.Name(), RepoName: repoName, CreatedAt: meta.CreatedAt},
			Report:     &reportData,
		}
		manifestJSON, manifestErr := readRootFile(root, path.Join(entry.Name(), report.RunManifestFilename), maxArtifactBytes)
		if manifestErr == nil {
			manifest, decodeErr := report.DecodeRunManifest(manifestJSON)
			if decodeErr == nil && manifest.VerifyReportJSON(reportJSON) == nil {
				analysisRoot, rootErr := manifest.ResolveAnalysisRoot()
				if rootErr == nil {
					run.Manifest = &manifest
					run.RepoPath = analysisRoot
					run.AnalysisAvailable = h.analysisAvailable(manifest)
					if manifest.Version < report.CurrentRunManifestVersion {
						legacy := freshness.NewFreshnessResult(freshness.FreshnessLegacyUnknown)
						run.Report.Freshness = &legacy
					}
				}
			}
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].ID > runs[j].ID })
	h.log("loaded %d saved report(s) in %d ms", len(runs), time.Since(started).Milliseconds())
	return runs, nil
}

func (h *handler) refreshRunFreshness(ctx context.Context, run *runRecord) {
	if run == nil || run.Manifest == nil || run.Report == nil || run.Manifest.Version < report.CurrentRunManifestVersion ||
		h.captureRepo == nil {
		return
	}
	started := time.Now()
	current, err := h.captureRepo(ctx, run.Manifest.RepositoryState.Identity)
	if err != nil {
		result := freshness.NewFreshnessResult(freshness.FreshnessUnavailable)
		result.Diagnostics = []string{"current analyzed-input freshness could not be checked"}
		run.Report.Freshness = &result
		h.log("report %s freshness unavailable after %d ms", run.ID, time.Since(started).Milliseconds())
		return
	}
	result := run.Manifest.CurrentFreshness(current)
	run.Report.Freshness = &result
	h.log("report %s freshness=%s in %d ms affected_inputs=%d affected_submodules=%d",
		run.ID,
		result.State,
		time.Since(started).Milliseconds(),
		len(result.AffectedInputIDs),
		len(result.AffectedSubmodules),
	)
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
