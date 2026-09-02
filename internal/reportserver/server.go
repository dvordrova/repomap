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
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
	"github.com/dvordrova/repomap/internal/workspaceopen"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

const (
	maxReportJSONBytes        = 0
	maxReportHTMLBytes        = 0
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

// Options contains the ordinary-run report server configuration.
type Options struct {
	RunsDir      string
	InitialRunID string
	Port         int
	Capability   string
	ExpectedHost string
	// VerifiedRuns is ordered current-transaction authority for lazily served
	// pages. An empty slice preserves the independent artifact recovery path.
	VerifiedRuns []report.VerifiedRunReceipt
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

type lazyRunRecord struct {
	mu               sync.Mutex
	id               string
	runDir           string
	reportSHA256     string
	repositoryName   string
	manifest         report.RunManifest
	targetNavigation *report.TargetNavigationPortfolio

	loadComplete      bool
	loadErr           error
	workspaceSnapshot workspacesnapshot.Snapshot
	sources           map[string]sourceTarget
	rendered          []byte
}

type handler struct {
	urlPrefix    string
	initialRun   string
	runs         map[string]runRecord
	verifiedRuns map[string]*lazyRunRecord
	openFile     OpenFileFunc
	openSlot     chan struct{}
	logf         func(string, ...any)
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
	opts.ExpectedHost = net.JoinHostPort("127.0.0.1", strconv.Itoa(address.Port))
	serverHandler, err := NewHandler(opts)
	if err != nil {
		_ = listener.Close()
		return err
	}

	url := fmt.Sprintf(
		"http://127.0.0.1:%d%s/runs/%s/report.html#/repository",
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
		ReadHeaderTimeout: reportReadHeaderTimeout,
		IdleTimeout:       reportIdleTimeout,
		BaseContext: func(net.Listener) context.Context {
			return serveCtx
		},
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
	var runs map[string]runRecord
	var verifiedRuns map[string]*lazyRunRecord
	if len(opts.VerifiedRuns) > 0 {
		verifiedRuns, err = bindVerifiedRuns(runsDir, opts.InitialRunID, opts.VerifiedRuns)
		if err != nil {
			return nil, fmt.Errorf("report server: bind verified target pages: %w", err)
		}
	} else {
		if err := requireOwnerHTML(filepath.Join(runsDir, opts.InitialRunID)); err != nil {
			return nil, fmt.Errorf("report server: verify initial report: %w", err)
		}
		run, loadErr := loadRun(runsDir, opts.InitialRunID)
		if loadErr != nil {
			return nil, fmt.Errorf("report server: load initial run %s: %w", opts.InitialRunID, loadErr)
		}
		runs, err = loadAuthorizedRuns(runsDir, run)
		if err != nil {
			return nil, fmt.Errorf("report server: load authorized target pages: %w", err)
		}
	}

	urlPrefix := capabilityURLPrefix(capability)
	h := &handler{
		urlPrefix:    urlPrefix,
		initialRun:   opts.InitialRunID,
		runs:         runs,
		verifiedRuns: verifiedRuns,
		openFile:     openFile,
		openSlot:     make(chan struct{}, maxConcurrentOpenRequests),
		logf:         opts.Logf,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+urlPrefix+"/{$}", h.serveRoot)
	mux.HandleFunc("GET "+urlPrefix+"/runs/{runID}/report.html", h.serveReport)
	mux.HandleFunc("POST "+urlPrefix+"/api/open", h.serveOpen)
	return securityHeaders(mux, opts.ExpectedHost), nil
}

// bindVerifiedRuns consumes only opaque, transaction-local receipt authority.
// It performs no report or manifest reads and does not render any page.
func bindVerifiedRuns(
	runsDir string,
	initialRunID string,
	receipts []report.VerifiedRunReceipt,
) (map[string]*lazyRunRecord, error) {
	pages := make([]report.TargetNavigationPage, 0, len(receipts))
	records := make(map[string]*lazyRunRecord, len(receipts))
	initialTargetID := ""

	for index, receipt := range receipts {
		page := receipt.ProgramPage()
		if !validRunID(page.RunID) {
			return nil, fmt.Errorf("verified run %d has an invalid page run id", index)
		}
		runDir := filepath.Join(runsDir, page.RunID)
		if err := receipt.ValidateRunIdentity(runDir); err != nil {
			return nil, fmt.Errorf("verified run %d identity: %w", index, err)
		}
		manifest := receipt.Manifest()
		if receipt.ProgramTargetID() != page.ProgramTarget.ID ||
			manifest.MaterialInputs.ProgramTargetID != page.ProgramTarget.ID ||
			receipt.ReportSHA256() != manifest.ReportSHA256 ||
			receipt.ProgramPagePortfolioSHA256() != manifest.MaterialInputs.ProgramPagePortfolioSHA256 ||
			receipt.RepositoryName() == "" {
			return nil, fmt.Errorf("verified run %d page authority mismatch", index)
		}
		if _, duplicate := records[page.RunID]; duplicate {
			return nil, fmt.Errorf("verified runs contain a duplicate run id")
		}
		records[page.RunID] = &lazyRunRecord{
			id:             page.RunID,
			runDir:         receipt.RunDir(),
			reportSHA256:   receipt.ReportSHA256(),
			repositoryName: receipt.RepositoryName(),
			manifest:       manifest,
		}
		pages = append(pages, page)
		if page.RunID == initialRunID {
			initialTargetID = page.ProgramTarget.ID
		}
	}
	if initialTargetID == "" {
		return nil, fmt.Errorf("verified runs do not contain the initial run")
	}

	initial := records[initialRunID]
	initialManifest := initial.manifest
	initialRepositoryName := initial.repositoryName
	if len(records) > 1 &&
		(initialManifest.MaterialInputs.ProgramPagePortfolioSHA256 == "" ||
			initialManifest.MaterialInputs.TargetOutcomePortfolioSHA256 == "") {
		return nil, fmt.Errorf("verified multi-run pages lack neutral portfolio authority")
	}
	for index, page := range pages {
		record := records[page.RunID]
		manifest := record.manifest
		if record.repositoryName != initialRepositoryName ||
			manifest.AnalysisRoot != initialManifest.AnalysisRoot ||
			manifest.RepositoryState.Identity != initialManifest.RepositoryState.Identity ||
			manifest.RepositoryStateSHA256 != initialManifest.RepositoryStateSHA256 ||
			manifest.MaterialInputs.SelectedRevision != initialManifest.MaterialInputs.SelectedRevision ||
			manifest.MaterialInputs.ProgramPagePortfolioSHA256 != initialManifest.MaterialInputs.ProgramPagePortfolioSHA256 ||
			manifest.MaterialInputs.TargetOutcomePortfolioSHA256 != initialManifest.MaterialInputs.TargetOutcomePortfolioSHA256 ||
			!reflect.DeepEqual(manifest.StandaloneSource, initialManifest.StandaloneSource) {
			return nil, fmt.Errorf("verified run %d repository authority mismatch", index)
		}
		navigation, err := report.BuildTargetNavigation(pages, initialTargetID, page.ProgramTarget.ID)
		if err != nil {
			return nil, fmt.Errorf("verified run %d navigation: %w", index, err)
		}
		record.targetNavigation = navigation
	}
	return records, nil
}

func requireOwnerHTML(runDir string) error {
	info, err := os.Lstat(filepath.Join(runDir, "report.html"))
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 {
		return fmt.Errorf("report.html is unavailable")
	}
	return nil
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
	if maxReportHTMLBytes > 0 && len(rendered) > maxReportHTMLBytes {
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

func (run *lazyRunRecord) renderedPage() ([]byte, error) {
	run.mu.Lock()
	defer run.mu.Unlock()
	if run.loadComplete {
		return run.rendered, run.loadErr
	}
	run.loadComplete = true
	run.workspaceSnapshot, run.sources, run.rendered, run.loadErr = loadVerifiedRunPage(run)
	return run.rendered, run.loadErr
}

func (run *lazyRunRecord) openAuthority(
	sourceID string,
) (workspacesnapshot.Snapshot, sourceTarget, bool) {
	run.mu.Lock()
	defer run.mu.Unlock()
	if !run.loadComplete || run.loadErr != nil {
		return workspacesnapshot.Snapshot{}, sourceTarget{}, false
	}
	target, ok := run.sources[sourceID]
	return run.workspaceSnapshot, target, ok
}

func loadVerifiedRunPage(
	run *lazyRunRecord,
) (workspacesnapshot.Snapshot, map[string]sourceTarget, []byte, error) {
	if run == nil || run.id == "" || run.runDir == "" || run.targetNavigation == nil {
		return workspacesnapshot.Snapshot{}, nil, nil,
			fmt.Errorf("verified run page authority is incomplete")
	}
	root, err := os.OpenRoot(run.runDir)
	if err != nil {
		return workspacesnapshot.Snapshot{}, nil, nil, fmt.Errorf("open run directory: %w", err)
	}
	defer root.Close()
	reportJSON, err := readRootFile(root, "report.json", maxReportJSONBytes)
	if err != nil {
		return workspacesnapshot.Snapshot{}, nil, nil, fmt.Errorf("read report.json: %w", err)
	}
	digest := sha256.Sum256(reportJSON)
	if fmt.Sprintf("%x", digest) != run.reportSHA256 {
		return workspacesnapshot.Snapshot{}, nil, nil,
			fmt.Errorf("report.json does not match verified receipt")
	}
	reportData, err := decodeVerifiedReportJSON(reportJSON)
	if err != nil {
		return workspacesnapshot.Snapshot{}, nil, nil, err
	}
	analysisRoot, err := run.manifest.ResolveAnalysisRoot()
	if err != nil {
		return workspacesnapshot.Snapshot{}, nil, nil, fmt.Errorf("resolve analysis root: %w", err)
	}
	workspaceSnapshot, catalog, err := workspaceSnapshotForManifest(run.manifest, analysisRoot)
	if err != nil {
		return workspacesnapshot.Snapshot{}, nil, nil, err
	}
	sources, sourceIDs := catalogSourceTargets(run.id, run.reportSHA256, catalog)
	reportData.SourceIDs = sourceIDs
	rendered, err := report.RenderHTMLWithOptions(
		&reportData,
		report.RenderOptions{
			TargetNavigation: run.targetNavigation,
			LocalRoots: []string{
				run.runDir,
				analysisRoot,
				run.manifest.RepositoryState.Identity,
			},
		},
	)
	if err != nil {
		return workspacesnapshot.Snapshot{}, nil, nil, fmt.Errorf("render report: %w", err)
	}
	if maxReportHTMLBytes > 0 && len(rendered) > maxReportHTMLBytes {
		return workspacesnapshot.Snapshot{}, nil, nil,
			fmt.Errorf("rendered report exceeds %d bytes", maxReportHTMLBytes)
	}
	return workspaceSnapshot, sources, rendered, nil
}

func decodeVerifiedReportJSON(encoded []byte) (report.ReportData, error) {
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
		return report.ReportData{}, fmt.Errorf("report.json is invalid")
	}
	return data, nil
}

func loadTargetNavigation(
	runDir string,
	manifest report.RunManifest,
) (*report.TargetNavigationPortfolio, error) {
	if manifest.MaterialInputs.ProgramPagePortfolioSHA256 == "" {
		return nil, fmt.Errorf("load target navigation: neutral program page authority is missing")
	}
	if manifest.MaterialInputs.TargetOutcomePortfolioSHA256 == "" {
		return nil, fmt.Errorf("load target navigation: neutral target outcome authority is missing")
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
	programPageAuthority := initialMaterial.ProgramPagePortfolioSHA256 != "" &&
		siblingMaterial.ProgramPagePortfolioSHA256 == initialMaterial.ProgramPagePortfolioSHA256 &&
		initialMaterial.TargetOutcomePortfolioSHA256 != "" &&
		siblingMaterial.TargetOutcomePortfolioSHA256 == initialMaterial.TargetOutcomePortfolioSHA256
	if siblingMaterial.ProgramTargetID != targetID ||
		!programPageAuthority ||
		sibling.manifest.RepositoryState.Identity != initial.manifest.RepositoryState.Identity ||
		sibling.targetNavigation == nil ||
		sibling.targetNavigation.CurrentTargetID != targetID ||
		sibling.targetNavigation.DefaultTargetID != initial.targetNavigation.DefaultTargetID {
		return fmt.Errorf("manifest binding mismatch")
	}
	return nil
}

func (h *handler) serveRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(
		w,
		r,
		h.urlPrefix+"/runs/"+h.initialRun+"/report.html#/repository",
		http.StatusFound,
	)
}

func (h *handler) serveReport(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	var rendered []byte
	if verified, ok := h.verifiedRuns[runID]; ok {
		var err error
		rendered, err = verified.renderedPage()
		if err != nil {
			h.log("report load run=%s outcome=failed", runID)
			http.Error(w, "report is unavailable", http.StatusInternalServerError)
			return
		}
	} else {
		run, ok := h.runs[runID]
		if !ok {
			http.NotFound(w, r)
			return
		}
		rendered = run.rendered
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "report.html", time.Time{}, bytes.NewReader(rendered))
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
	verified, verifiedOK := h.verifiedRuns[request.RunID]
	eager, eagerOK := h.runs[request.RunID]
	if !verifiedOK && !eagerOK {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "report run not found"})
		return
	}
	if request.Line < 0 || request.Line > maxOpenLocationCoordinate ||
		request.Column < 0 || request.Column > maxOpenLocationCoordinate {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid source location"})
		return
	}
	sourceID := strings.TrimSpace(request.SourceID)
	if sourceID == "" || sourceID != request.SourceID || !validCapability(sourceID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "source is not authorized by this report"})
		return
	}
	var workspaceSnapshot workspacesnapshot.Snapshot
	var target sourceTarget
	var authorized bool
	if verifiedOK {
		workspaceSnapshot, target, authorized = verified.openAuthority(sourceID)
	} else {
		workspaceSnapshot = eager.workspaceSnapshot
		target, authorized = eager.sources[sourceID]
	}
	if !authorized {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "source is not authorized by this report"})
		return
	}

	absolutePath, err := resolveOpenTargetWithSnapshot(r.Context(), workspaceSnapshot, target)
	if err != nil {
		h.log("source open run=%s source=%s outcome=source_unavailable response_ms=%d",
			request.RunID, sourceID, time.Since(started).Milliseconds())
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
		request.RunID, sourceID, time.Since(started).Milliseconds())
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
	return resolveOpenTargetWithSnapshot(ctx, run.workspaceSnapshot, target)
}

func resolveOpenTargetWithSnapshot(
	ctx context.Context,
	snapshot workspacesnapshot.Snapshot,
	target sourceTarget,
) (string, error) {
	service, err := workspaceopen.New(snapshot)
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
	if !info.Mode().IsRegular() || info.Size() < 0 || (maxBytes > 0 && info.Size() > maxBytes) {
		return nil, fmt.Errorf("artifact is not a bounded regular file")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var reader io.Reader = file
	if maxBytes > 0 {
		reader = io.LimitReader(file, maxBytes+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
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

func validExpectedHost(expectedHost string) bool {
	host, port, err := net.SplitHostPort(expectedHost)
	if err != nil || port == "" {
		return false
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > MaxTCPPort {
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
