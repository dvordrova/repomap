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
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
	"github.com/dvordrova/repomap/internal/workspaceopen"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

const (
	maxReportJSONBytes = report.MaxReportJSONBytes
	maxReportHTMLBytes = report.MaxOrdinaryReportHTMLBytes
	capabilityBytes    = 32
)

type OpenFileFunc func(ctx context.Context, absolutePath string, line, column int) error

// Options contains the ordinary-run report server configuration.
type Options struct {
	RunsDir      string
	InitialRunID string
	Port         int
	Capability   string
	ExpectedHost string
	OpenFile     OpenFileFunc
	Logf         func(string, ...any)
	OnReady      func(url string) error
}

type runRecord struct {
	id                string
	manifest          report.RunManifest
	targetNavigation  *report.TargetNavigationPortfolio
	workspaceSnapshot workspacesnapshot.Snapshot
	sources           map[string]sourceTarget
	rendered          []byte
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

type sourceTarget struct {
	relativePath string
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
	serverHandler, err := NewHandler(opts)
	if err != nil {
		_ = listener.Close()
		return err
	}

	url := fmt.Sprintf(
		"http://127.0.0.1:%d%s/runs/%s/report.html#/program",
		address.Port,
		capabilityURLPrefix(opts.Capability),
		opts.InitialRunID,
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
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return serveCtx
		},
	}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-serveCtx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
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
	runsInfo, err := os.Stat(runsDir)
	if err != nil || !runsInfo.IsDir() {
		return nil, fmt.Errorf("report server: runs directory is unavailable: %s", runsDir)
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

	openFile := opts.OpenFile
	if openFile == nil {
		openFile, err = NewVSCodeLauncher(opts.Logf)
		if err != nil {
			return nil, fmt.Errorf("report server: VS Code launcher: %w", err)
		}
	}
	run, err := loadRun(runsDir, opts.InitialRunID)
	if err != nil {
		return nil, fmt.Errorf("report server: load initial run %s: %w", opts.InitialRunID, err)
	}
	runs, err := loadAuthorizedRuns(runsDir, run)
	if err != nil {
		return nil, fmt.Errorf("report server: load authorized target pages: %w", err)
	}

	urlPrefix := capabilityURLPrefix(capability)
	h := &handler{
		urlPrefix:  urlPrefix,
		initialRun: opts.InitialRunID,
		runs:       runs,
		openFile:   openFile,
		openSlot:   make(chan struct{}, 1),
		logf:       opts.Logf,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+urlPrefix+"/{$}", h.serveRoot)
	mux.HandleFunc("GET "+urlPrefix+"/runs/{runID}/report.html", h.serveReport)
	mux.HandleFunc("POST "+urlPrefix+"/api/open", h.serveOpen)
	return securityHeaders(mux, opts.ExpectedHost), nil
}

func loadRun(runsDir, runID string) (runRecord, error) {
	runDir := filepath.Join(runsDir, runID)
	runInfo, err := os.Lstat(runDir)
	if err != nil || runInfo.Mode()&os.ModeSymlink != 0 || !runInfo.IsDir() {
		return runRecord{}, fmt.Errorf("run directory is unavailable")
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return runRecord{}, fmt.Errorf("open run directory: %w", err)
	}
	defer root.Close()
	if info, err := root.Lstat("report.html"); err != nil || !info.Mode().IsRegular() ||
		info.Size() < 0 || info.Size() > maxReportHTMLBytes {
		return runRecord{}, fmt.Errorf("report.html is unavailable")
	}
	reportJSON, err := readRootFile(root, "report.json", maxReportJSONBytes)
	if err != nil {
		return runRecord{}, fmt.Errorf("read report.json: %w", err)
	}
	var reportData report.ReportData
	if err := json.Unmarshal(reportJSON, &reportData); err != nil ||
		reportData.FormatVersion != report.CurrentFormatVersion {
		return runRecord{}, fmt.Errorf("report.json is invalid")
	}

	manifest, err := report.ReadRunManifest(runDir)
	if err != nil {
		return runRecord{}, fmt.Errorf("read manifest: %w", err)
	}
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
		return runRecord{}, fmt.Errorf("verify report.json: %w", err)
	}
	analysisRoot, err := manifest.ResolveAnalysisRoot()
	if err != nil {
		return runRecord{}, fmt.Errorf("resolve analysis root: %w", err)
	}
	workspaceSnapshot, catalog, err := workspaceSnapshotForManifest(manifest, analysisRoot)
	if err != nil {
		return runRecord{}, err
	}
	targetNavigation, err := loadTargetNavigation(runDir, manifest)
	if err != nil {
		return runRecord{}, err
	}
	sources, sourceIDs := catalogSourceTargets(runID, manifest.ReportSHA256, catalog)
	reportData.SourceIDs = sourceIDs
	rendered, err := report.RenderHTMLWithOptions(
		&reportData,
		report.RenderOptions{
			TargetNavigation: targetNavigation,
			LocalRoots: []string{
				runDir,
				analysisRoot,
				manifest.RepositoryState.Identity,
			},
		},
	)
	if err != nil {
		return runRecord{}, fmt.Errorf("render report: %w", err)
	}
	if len(rendered) > maxReportHTMLBytes {
		return runRecord{}, fmt.Errorf("rendered report exceeds %d bytes", maxReportHTMLBytes)
	}
	return runRecord{
		id:                runID,
		manifest:          manifest,
		targetNavigation:  targetNavigation,
		workspaceSnapshot: workspaceSnapshot,
		sources:           sources,
		rendered:          rendered,
	}, nil
}

func loadTargetNavigation(
	runDir string,
	manifest report.RunManifest,
) (*report.TargetNavigationPortfolio, error) {
	if manifest.MaterialInputs.TargetPagePortfolioSHA256 == "" {
		return nil, nil
	}
	navigation, err := report.LoadManifestTargetNavigation(runDir, manifest)
	if err != nil {
		return nil, fmt.Errorf("load target navigation: %w", err)
	}
	return navigation, nil
}

func loadAuthorizedRuns(runsDir string, initial runRecord) (map[string]runRecord, error) {
	runs := map[string]runRecord{initial.id: initial}
	navigation := initial.targetNavigation
	if navigation == nil {
		return runs, nil
	}
	if initial.manifest.MaterialInputs.ProgramTargetID != navigation.CurrentTargetID {
		return nil, fmt.Errorf("initial target identity mismatch")
	}
	for _, item := range navigation.Targets {
		runID, err := navigationRunID(initial.id, navigation.CurrentTargetID, item)
		if err != nil {
			return nil, err
		}
		if runID == initial.id {
			continue
		}
		if _, duplicate := runs[runID]; duplicate {
			return nil, fmt.Errorf("duplicate authorized target run")
		}
		sibling, err := loadRun(runsDir, runID)
		if err != nil {
			return nil, fmt.Errorf("load sibling run %s: %w", runID, err)
		}
		if err := authorizeSiblingRun(initial, sibling, item.TargetID); err != nil {
			return nil, fmt.Errorf("authorize sibling run %s: %w", runID, err)
		}
		runs[runID] = sibling
	}
	return runs, nil
}

func navigationRunID(
	initialRunID, currentTargetID string,
	item report.TargetNavigationItem,
) (string, error) {
	if item.TargetID == currentTargetID {
		if item.Href != "#/program" {
			return "", fmt.Errorf("current target route is invalid")
		}
		return initialRunID, nil
	}
	parsed, err := url.Parse(item.Href)
	if err != nil || parsed.RawQuery != "" || parsed.Fragment != "/program" {
		return "", fmt.Errorf("sibling target route is invalid")
	}
	const prefix = "../"
	const suffix = "/report.html"
	if !strings.HasPrefix(parsed.Path, prefix) || !strings.HasSuffix(parsed.Path, suffix) {
		return "", fmt.Errorf("sibling target route is invalid")
	}
	runID := strings.TrimSuffix(strings.TrimPrefix(parsed.Path, prefix), suffix)
	if !validRunID(runID) || runID == initialRunID {
		return "", fmt.Errorf("sibling target run id is invalid")
	}
	return runID, nil
}

func authorizeSiblingRun(initial, sibling runRecord, targetID string) error {
	initialMaterial := initial.manifest.MaterialInputs
	siblingMaterial := sibling.manifest.MaterialInputs
	if siblingMaterial.ProgramTargetID != targetID ||
		siblingMaterial.TargetRunContainerSHA256 == "" ||
		siblingMaterial.TargetRunContainerSHA256 != initialMaterial.TargetRunContainerSHA256 ||
		siblingMaterial.TargetPagePortfolioSHA256 == "" ||
		siblingMaterial.TargetPagePortfolioSHA256 != initialMaterial.TargetPagePortfolioSHA256 ||
		sibling.manifest.RepositoryState.Identity != initial.manifest.RepositoryState.Identity ||
		sibling.targetNavigation == nil ||
		sibling.targetNavigation.CurrentTargetID != targetID ||
		sibling.targetNavigation.DefaultTargetID != initial.targetNavigation.DefaultTargetID {
		return fmt.Errorf("manifest binding mismatch")
	}
	// AnalysisTargetRef is backend-specific outer-page authority. When both
	// Go page manifests carry it, a sibling must not reuse the initial page's
	// outer identity; its exact ProgramTargetID remains the navigation join.
	if initialMaterial.AnalysisTargetRef != "" &&
		siblingMaterial.AnalysisTargetRef != "" &&
		initialMaterial.AnalysisTargetRef == siblingMaterial.AnalysisTargetRef {
		return fmt.Errorf("manifest binding mismatch")
	}
	return nil
}

func (h *handler) serveRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(
		w,
		r,
		h.urlPrefix+"/runs/"+h.initialRun+"/report.html#/program",
		http.StatusFound,
	)
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
	if request.Line < 0 || request.Line > 10_000_000 ||
		request.Column < 0 || request.Column > 10_000_000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid source location"})
		return
	}
	sourceID := strings.TrimSpace(request.SourceID)
	if sourceID == "" || sourceID != request.SourceID || !validCapability(sourceID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "source is not authorized by this report"})
		return
	}
	target, ok := run.sources[sourceID]
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "source is not authorized by this report"})
		return
	}

	absolutePath, err := resolveOpenTarget(r.Context(), run, target)
	if err != nil {
		h.log("source open run=%s source=%s outcome=source_unavailable response_ms=%d",
			run.id, sourceID, time.Since(started).Milliseconds())
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "authorized source is unavailable",
		})
		return
	}
	select {
	case h.openSlot <- struct{}{}:
		defer func() { <-h.openSlot }()
	default:
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "another editor action is still running",
		})
		return
	}
	if err := h.openFile(r.Context(), absolutePath, request.Line, request.Column); err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, ErrEditorUnavailable) {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, map[string]string{
			"error": "could not open file in VS Code",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "opened",
	})
	h.log("source open run=%s source=%s outcome=opened response_ms=%d",
		run.id, sourceID, time.Since(started).Milliseconds())
}

func manifestSourceID(runID, reportSHA256, relativePath string) string {
	digest := sha256.Sum256([]byte(
		"repomap-source-v1\x00" + runID + "\x00" + reportSHA256 + "\x00" + relativePath,
	))
	return base64.RawURLEncoding.EncodeToString(digest[:])
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
		sourceID := manifestSourceID(runID, reportSHA256, source.Path)
		targets[sourceID] = sourceTarget{relativePath: source.Path}
		sourceIDs[source.Path] = sourceID
	}
	return targets, sourceIDs
}

func resolveOpenTarget(
	ctx context.Context,
	run runRecord,
	target sourceTarget,
) (string, error) {
	service, err := workspaceopen.New(run.workspaceSnapshot)
	if err != nil {
		return "", err
	}
	resolved, err := service.Resolve(ctx, workspaceopen.Request{
		Path: target.relativePath,
	})
	if err != nil {
		return "", err
	}
	return resolved.AbsolutePath, nil
}

func workspaceSnapshotForManifest(
	manifest report.RunManifest,
	resolvedRoot string,
) (workspacesnapshot.Snapshot, sourcecatalog.Catalog, error) {
	if manifest.Version != report.CurrentRunManifestVersion {
		return workspacesnapshot.Snapshot{}, sourcecatalog.Catalog{},
			fmt.Errorf("workspace snapshot unavailable")
	}
	snapshot, err := manifest.WorkspaceSnapshot()
	if err != nil {
		return workspacesnapshot.Snapshot{}, sourcecatalog.Catalog{},
			fmt.Errorf("workspace snapshot unavailable")
	}
	catalog := snapshot.Catalog()
	if snapshot.AnalysisRoot() != resolvedRoot || catalog.AnalysisRoot() != resolvedRoot {
		return workspacesnapshot.Snapshot{}, sourcecatalog.Catalog{},
			fmt.Errorf("workspace snapshot root mismatch")
	}
	return snapshot, catalog, nil
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
		w.Header().Set(
			"Content-Security-Policy",
			"default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; img-src data:; object-src 'none'; frame-ancestors 'none'",
		)
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

func (h *handler) log(format string, args ...any) {
	if h.logf != nil {
		h.logf(format, args...)
	}
}
