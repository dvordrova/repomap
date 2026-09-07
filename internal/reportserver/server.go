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
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/repoconfig"
	"github.com/dvordrova/repomap/internal/report"
)

// The server does two things: it serves one repository report with its target
// sections, and it opens a source file of the page in the editor at a
// line. It used to verify every run against its manifest, every manifest
// against every artifact, and every path a page might open against an
// allow-list captured at analysis time, with the page keyed by capability
// token and locked to one host. The owner's rule is that the tool is not a
// security boundary: a run directory is read, rendered and served, and a
// path the page names is opened under the analysed root.
const (
	capabilityBytes           = 32
	maxCapabilityTokenBytes   = 256
	MaxTCPPort                = 65535
	maxConcurrentOpenRequests = 1
	maxOpenLocationCoordinate = 10_000_000
	reportReadHeaderTimeout   = 5 * time.Second
	reportIdleTimeout         = 30 * time.Second
	reportShutdownTimeout     = 3 * time.Second
)

type OpenFileFunc func(ctx context.Context, absolutePath string, line, column int) error

// Options is the server configuration. Runs, when given, are the runs the
// current process just generated; otherwise every run is read from disk by
// following the initial run's navigation.
type Options struct {
	Config       *repoconfig.Config
	RunsDir      string
	InitialRunID string
	Port         int
	Capability   string
	Runs         []report.RunReceipt
	OpenFile     OpenFileFunc
	Logf         func(string, ...any)
	OnReady      func(url string) error
}

type runRecord struct {
	id           string
	runDir       string
	analysisRoot string
	rendered     []byte
	// sources maps the ids the page carries to the paths they name.
	sources map[string]string
}

type handler struct {
	urlPrefix  string
	initialRun string
	runs       map[string]runRecord
	openFile   OpenFileFunc
	openSlot   chan struct{}
	logf       func(string, ...any)
}

type openRequest struct {
	RunID    string `json:"run_id"`
	SourceID string `json:"source_id"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
}

// Serve listens on the loopback interface and serves until ctx is done.
func Serve(ctx context.Context, opts Options) error {
	started := time.Now()
	if ctx == nil {
		return fmt.Errorf("report server: context is required")
	}
	if opts.Port < 0 || opts.Port > MaxTCPPort {
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
	serverHandler, err := NewHandler(opts)
	if err != nil {
		_ = listener.Close()
		return err
	}
	url := fmt.Sprintf(
		"http://127.0.0.1:%d%s/runs/%s/report.html#/repository",
		address.Port, capabilityURLPrefix(opts.Capability), opts.InitialRunID,
	)
	if opts.OnReady != nil {
		if err := opts.OnReady(url); err != nil {
			_ = listener.Close()
			return fmt.Errorf("report server: ready callback: %w", err)
		}
	}
	if opts.Logf != nil {
		opts.Logf("report server ready in %d ms", time.Since(started).Milliseconds())
	}
	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	server := &http.Server{
		Handler:           serverHandler,
		ReadHeaderTimeout: reportReadHeaderTimeout,
		IdleTimeout:       reportIdleTimeout,
		BaseContext:       func(net.Listener) context.Context { return serveCtx },
	}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-serveCtx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), reportShutdownTimeout)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	err = server.Serve(listener)
	if !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("report server: serve: %w", err)
	}
	<-shutdownDone
	return nil
}

// NewHandler loads and renders every run the initial run's navigation names,
// and serves them.
func NewHandler(opts Options) (http.Handler, error) {
	if strings.TrimSpace(opts.RunsDir) == "" {
		return nil, fmt.Errorf("report server: runs directory is required")
	}
	if !validRunID(opts.InitialRunID) {
		return nil, fmt.Errorf("report server: valid initial run id is required")
	}
	runsDir, err := filepath.Abs(opts.RunsDir)
	if err != nil {
		return nil, fmt.Errorf("report server: resolve runs directory: %w", err)
	}
	if info, err := os.Stat(runsDir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("report server: runs directory is unavailable: %s", runsDir)
	}
	capability := strings.TrimSpace(opts.Capability)
	if capability == "" {
		if capability, err = generateCapability(); err != nil {
			return nil, fmt.Errorf("report server: generate capability: %w", err)
		}
	}
	if !validCapability(capability) {
		return nil, fmt.Errorf("report server: invalid capability")
	}
	runs, err := loadRuns(runsDir, opts.InitialRunID, opts.Runs)
	if err != nil {
		return nil, err
	}
	openFile := opts.OpenFile
	if openFile == nil {
		openFile = configuredEditor(runs[opts.InitialRunID].analysisRoot, opts.Config, opts.Logf)
	}
	h := &handler{
		urlPrefix:  capabilityURLPrefix(capability),
		initialRun: opts.InitialRunID,
		runs:       runs,
		openFile:   openFile,
		openSlot:   make(chan struct{}, maxConcurrentOpenRequests),
		logf:       opts.Logf,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+h.urlPrefix+"/{$}", h.serveRoot)
	mux.HandleFunc("GET "+h.urlPrefix+"/runs/{runID}/report.html", h.serveReport)
	mux.HandleFunc("POST "+h.urlPrefix+"/api/open", h.serveOpen)
	return mux, nil
}

// loadRuns renders the initial run and every sibling its navigation links
// to. Runs handed over by the generating process are loaded the same way:
// what they add is only which run directories to read.
func loadRuns(runsDir, initialRunID string, given []report.RunReceipt) (map[string]runRecord, error) {
	for _, receipt := range given {
		if filepath.Base(receipt.RunDir()) == initialRunID && receipt.Data() != nil {
			run, err := renderRun(initialRunID, receipt)
			if err != nil {
				return nil, err
			}
			return map[string]runRecord{initialRunID: run}, nil
		}
	}
	initial, _, err := loadRun(runsDir, initialRunID)
	if err != nil {
		return nil, fmt.Errorf("report server: load initial run %s: %w", initialRunID, err)
	}
	return map[string]runRecord{initialRunID: initial}, nil
}

func loadRun(runsDir, runID string) (runRecord, *report.TargetNavigationPortfolio, error) {
	runDir := filepath.Join(runsDir, runID)
	receipt, err := report.ReadRunReceipt(runDir)
	if err != nil {
		return runRecord{}, nil, err
	}
	run, err := renderRun(runID, receipt)
	return run, nil, err
}

func renderRun(runID string, receipt report.RunReceipt) (runRecord, error) {
	runDir := receipt.RunDir()
	reportData := *receipt.Data()
	manifest := receipt.Manifest()
	analysisRoot, err := manifest.ResolveAnalysisRoot()
	if err != nil {
		return runRecord{}, fmt.Errorf("resolve analysis root: %w", err)
	}
	sources := make(map[string]string, len(reportData.OpenablePaths))
	sourceIDs := make(map[string]string, len(reportData.OpenablePaths))
	for _, relativePath := range reportData.OpenablePaths {
		id := sourceID(runID, relativePath)
		sources[id] = relativePath
		sourceIDs[relativePath] = id
	}
	reportData.SourceIDs = sourceIDs
	rendered, err := report.RenderHTMLWithOptions(&reportData, report.RenderOptions{
		LocalRoots: []string{runDir, analysisRoot, manifest.RepositoryState.Identity},
	})
	if err != nil {
		return runRecord{}, fmt.Errorf("render report: %w", err)
	}
	return runRecord{
		id: runID, runDir: runDir, analysisRoot: analysisRoot,
		rendered: rendered, sources: sources,
	}, nil
}

func decodeReportJSON(encoded []byte) (report.ReportData, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var data report.ReportData
	if err := decoder.Decode(&data); err != nil {
		return report.ReportData{}, fmt.Errorf("report.json is invalid: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return report.ReportData{}, fmt.Errorf("report.json contains multiple values")
		}
		return report.ReportData{}, fmt.Errorf("report.json has trailing data: %w", err)
	}
	if data.FormatVersion != report.CurrentFormatVersion {
		return report.ReportData{}, fmt.Errorf("report.json has format version %d", data.FormatVersion)
	}
	return data, nil
}

// navigationRunID reads the sibling run id back out of a navigation link.
func navigationRunID(
	initialRunID, currentTargetID string,
	item report.TargetNavigationItem,
) (string, error) {
	if item.TargetID == currentTargetID {
		return initialRunID, nil
	}
	parsed, err := url.Parse(item.Href)
	if err != nil {
		return "", fmt.Errorf("sibling target route is invalid")
	}
	const prefix = "../"
	const suffix = "/report.html"
	if !strings.HasPrefix(parsed.Path, prefix) || !strings.HasSuffix(parsed.Path, suffix) {
		return "", fmt.Errorf("sibling target route is invalid")
	}
	runID := strings.TrimSuffix(strings.TrimPrefix(parsed.Path, prefix), suffix)
	if !validRunID(runID) {
		return "", fmt.Errorf("sibling target run id is invalid")
	}
	return runID, nil
}

func (h *handler) serveRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, h.urlPrefix+"/runs/"+h.initialRun+"/report.html#/repository", http.StatusFound)
}

func (h *handler) serveReport(w http.ResponseWriter, r *http.Request) {
	run, ok := h.runs[r.PathValue("runID")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "report.html", time.Time{}, bytes.NewReader(run.rendered))
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
	run, ok := h.runs[request.RunID]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "report run not found"})
		return
	}
	if request.Line < 0 || request.Line > maxOpenLocationCoordinate ||
		request.Column < 0 || request.Column > maxOpenLocationCoordinate {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid source location"})
		return
	}
	relativePath, known := run.sources[request.SourceID]
	if !known {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "source is not on this page"})
		return
	}
	absolutePath, err := resolveOpenTarget(run.analysisRoot, relativePath)
	if err != nil {
		h.log("source open run=%s source=%s outcome=source_unavailable response_ms=%d",
			request.RunID, request.SourceID, time.Since(started).Milliseconds())
		writeJSON(w, http.StatusConflict, map[string]string{"error": "source is unavailable"})
		return
	}
	select {
	case h.openSlot <- struct{}{}:
		defer func() { <-h.openSlot }()
	default:
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "another editor action is still running"})
		return
	}
	if err := h.openFile(r.Context(), absolutePath, request.Line, request.Column); err != nil {
		h.log("source open failed: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not open your editor. Check editor in .repomap.conf."})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "opened"})
	h.log("source open run=%s source=%s outcome=opened response_ms=%d",
		request.RunID, request.SourceID, time.Since(started).Milliseconds())
}

// resolveOpenTarget is the file a page path names, under the analysed root.
func resolveOpenTarget(analysisRoot, relativePath string) (string, error) {
	local := filepath.FromSlash(relativePath)
	if !filepath.IsLocal(local) || local == "." {
		return "", fmt.Errorf("path is not inside the analysed root")
	}
	absolutePath := filepath.Join(analysisRoot, local)
	info, err := os.Stat(absolutePath)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("file is unavailable")
	}
	return absolutePath, nil
}

func sourceID(runID, relativePath string) string {
	digest := sha256.Sum256([]byte("repomap-source-v1\x00" + runID + "\x00" + relativePath))
	return base64.RawURLEncoding.EncodeToString(digest[:])
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
	if capability == "" || len(capability) > maxCapabilityTokenBytes {
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (h *handler) log(format string, args ...any) {
	if h.logf != nil {
		h.logf(format, args...)
	}
}

var _ = strconv.Itoa
