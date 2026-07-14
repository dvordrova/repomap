package reportserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	goplsanalyzer "github.com/dvordrova/repomap/internal/analyzer/golang/gopls"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/investigation"
	"github.com/dvordrova/repomap/internal/memory"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/symbol"
	"github.com/dvordrova/repomap/internal/testevidence"
)

const (
	investigationDirectory  = "investigation"
	maxPublicTestReferences = 5
)

var (
	errInvestigationNotFound = errors.New("saved investigation not found")
	errInvestigationStale    = errors.New("saved investigation is stale")
	errInvestigationBinding  = errors.New("saved investigation authority mismatch")
)

// CaptureFactContextFunc records the exact local tool/build/collector inputs
// needed to decide whether a browser checkpoint can be reused after restart.
type CaptureFactContextFunc func(context.Context, freshness.RepositoryState, string) (freshness.FactContext, error)

type investigationRequest struct {
	RunID string `json:"run_id"`
}

type publicTestReference struct {
	Path      string                    `json:"path"`
	Line      int                       `json:"line"`
	Column    int                       `json:"column,omitempty"`
	Kind      testevidence.EvidenceKind `json:"kind"`
	Certainty string                    `json:"certainty"`
}

type loadedInvestigation struct {
	root   *os.Root
	run    runRecord
	facts  freshness.FactContext
	record memory.Record
}

func browserSymbolOptions() symbol.Options {
	return symbol.Options{
		MaxCandidates:        1,
		MaxIncomingCalls:     5,
		MaxOutgoingCalls:     5,
		MaxProvenancePerFact: 1,
	}
}

func browserSourceLimits() sourcecard.Limits {
	return sourcecard.Limits{
		MaxFileBytes:   1024 * 1024,
		MaxWindowLines: 20,
		MaxWindowBytes: 12 * 1024,
		MaxLineBytes:   4 * 1024,
	}
}

func browserTestOptions() testevidence.Options {
	return testevidence.Options{
		MaxSearches:            1,
		MaxReferencesPerSearch: maxPublicTestReferences,
	}
}

func captureBrowserFactContext(
	ctx context.Context,
	repository freshness.RepositoryState,
	analysisRoot string,
) (freshness.FactContext, error) {
	analyzerOptions, err := json.Marshal(struct {
		MaxSymbols   int `json:"max_symbols"`
		MaxCallRoots int `json:"max_call_roots"`
		MaxCallers   int `json:"max_callers"`
		MaxCallees   int `json:"max_callees"`
	}{
		MaxSymbols:   20,
		MaxCallRoots: 3,
		MaxCallers:   30,
		MaxCallees:   30,
	})
	if err != nil {
		return freshness.FactContext{}, err
	}
	collectorOptions, err := json.Marshal(struct {
		AnalysisRoot string               `json:"analysis_root"`
		Symbol       symbol.Options       `json:"symbol"`
		Source       sourcecard.Limits    `json:"source"`
		Tests        testevidence.Options `json:"tests"`
	}{
		AnalysisRoot: analysisRoot,
		Symbol:       browserSymbolOptions(),
		Source:       browserSourceLimits(),
		Tests:        browserTestOptions(),
	})
	if err != nil {
		return freshness.FactContext{}, err
	}
	return freshness.CaptureGoFactContext(ctx, repository, freshness.GoOptions{
		Collector: "browser-investigation",
		CollectorVersion: fmt.Sprintf(
			"gopls-%d.symbol-%d.source-%d.tests-%d.session-%d",
			goplsanalyzer.CollectorVersion,
			symbol.BundleVersion,
			sourcecard.Version,
			testevidence.BundleVersion,
			investigation.SessionVersion,
		),
		AnalyzerOptions:  string(analyzerOptions),
		CollectorOptions: string(collectorOptions),
	})
}

func (h *handler) serveLatestInvestigation(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeInvestigationRequest(w, r, "resume-investigation")
	if !ok {
		return
	}
	if h.analysis == nil || h.analysis.captureFacts == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "saved investigations are unavailable"})
		return
	}
	if !h.acquireAnalysisSlot(w) {
		return
	}
	defer h.releaseAnalysisSlot()

	ctx, cancel := context.WithTimeout(r.Context(), h.analysis.timeout)
	defer cancel()
	loaded, err := h.loadBrowserInvestigation(ctx, request.RunID)
	if err != nil {
		if errors.Is(err, errInvestigationNotFound) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.writeInvestigationError(w, ctx, err)
		return
	}
	defer loaded.root.Close()
	response, err := buildInvestigationResponse(loaded.record.Session)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "saved investigation is unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *handler) serveTargetTestReferences(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeInvestigationRequest(w, r, "find-test-references")
	if !ok {
		return
	}
	if h.analysis == nil || h.analysis.captureFacts == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "local test-reference analysis is unavailable"})
		return
	}
	if !h.acquireAnalysisSlot(w) {
		return
	}
	defer h.releaseAnalysisSlot()

	ctx, cancel := context.WithTimeout(r.Context(), h.analysis.timeout)
	defer cancel()
	loaded, err := h.loadBrowserInvestigation(ctx, request.RunID)
	if err != nil {
		h.writeInvestigationError(w, ctx, err)
		return
	}
	defer loaded.root.Close()

	session := loaded.record.Session
	if session.State == investigation.StateCompleted && session.Tests != nil {
		response, err := buildInvestigationResponse(session)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "saved investigation is unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, response)
		return
	}
	if h.analysis.references == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "local test-reference analysis is unavailable"})
		return
	}
	if session.State != investigation.StateFindingTestReferences || len(session.Next) != 1 ||
		session.Next[0].Kind != investigation.ActionFindTestReferences {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "test references are not available for this investigation"})
		return
	}
	runner := investigation.Runner{
		ReferenceFinder: h.analysis.references,
		TestOptions:     browserTestOptions(),
	}
	execution, err := runner.Execute(ctx, session, session.Next[0])
	if err != nil || execution.DiagnosticError != nil {
		h.writeAnalysisError(w, ctx, "could not find target test references")
		return
	}
	session, _, err = investigation.Reduce(session, execution.Event)
	if err != nil || session.State != investigation.StateCompleted || session.Tests == nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "target test references did not match the saved investigation"})
		return
	}
	after, err := h.analysis.capture(ctx, loaded.run.RepoPath)
	if err != nil {
		h.writeAnalysisError(w, ctx, "could not recheck repository state")
		return
	}
	if err := loaded.run.Manifest.VerifyRepositoryState(after); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "repository changed during analysis; regenerate the report"})
		return
	}
	if err := memory.SaveRoot(loaded.root, memory.Input{
		Session:    session,
		Repository: loaded.run.Manifest.RepositoryState,
		Facts:      &loaded.facts,
	}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "could not save the investigation"})
		return
	}
	response, err := buildInvestigationResponse(session)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "could not build the investigation result"})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func decodeInvestigationRequest(w http.ResponseWriter, r *http.Request, action string) (investigationRequest, bool) {
	if r.Header.Get("X-Repomap-Action") != action {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing repomap action header"})
		return investigationRequest{}, false
	}
	defer r.Body.Close()
	var request investigationRequest
	if err := decodeJSONBody(w, r, &request, 4096); err != nil || !validRunID(request.RunID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid investigation request"})
		return investigationRequest{}, false
	}
	return request, true
}

func (h *handler) acquireAnalysisSlot(w http.ResponseWriter) bool {
	select {
	case h.analysis.slot <- struct{}{}:
		return true
	default:
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "another local analysis is still running"})
		return false
	}
}

func (h *handler) releaseAnalysisSlot() {
	<-h.analysis.slot
}

func (h *handler) writeInvestigationError(w http.ResponseWriter, ctx context.Context, err error) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "local investigation loading timed out"})
		return
	}
	switch {
	case errors.Is(err, errInvestigationNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "saved investigation not found"})
	case errors.Is(err, errInvestigationStale), errors.Is(err, errInvestigationBinding):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "saved investigation no longer matches this report"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "could not load the saved investigation"})
	}
}

func (h *handler) loadBrowserInvestigation(ctx context.Context, runID string) (loadedInvestigation, error) {
	root, run, err := h.openInvestigationStore(runID, false)
	if err != nil {
		return loadedInvestigation{}, err
	}
	fail := func(err error) (loadedInvestigation, error) {
		_ = root.Close()
		return loadedInvestigation{}, err
	}
	if _, err := root.Lstat(memory.SessionFileName); err != nil {
		if os.IsNotExist(err) {
			return fail(errInvestigationNotFound)
		}
		return fail(err)
	}
	repository, err := h.analysis.capture(ctx, run.RepoPath)
	if err != nil {
		return fail(err)
	}
	if err := run.Manifest.VerifyRepositoryState(repository); err != nil {
		return fail(errInvestigationStale)
	}
	facts, err := h.analysis.captureFacts(ctx, repository, run.RepoPath)
	if err != nil {
		return fail(err)
	}
	facts.Repository = run.Manifest.RepositoryState
	record, err := memory.LoadRoot(root, memory.Current{Repository: run.Manifest.RepositoryState, Facts: &facts})
	if err != nil {
		return fail(err)
	}
	if len(record.Changes) != 0 {
		return fail(errInvestigationStale)
	}
	if err := verifyInvestigationBinding(run, record); err != nil {
		return fail(err)
	}
	return loadedInvestigation{
		root:   root,
		run:    run,
		facts:  facts,
		record: record,
	}, nil
}

func (h *handler) openInvestigationStore(runID string, create bool) (*os.Root, runRecord, error) {
	if !validRunID(runID) {
		return nil, runRecord{}, errInvestigationNotFound
	}
	runsRoot, err := os.OpenRoot(h.runsDir)
	if err != nil {
		return nil, runRecord{}, err
	}
	defer runsRoot.Close()
	runInfo, err := runsRoot.Lstat(runID)
	if err != nil || !runInfo.Mode().IsDir() {
		return nil, runRecord{}, errInvestigationNotFound
	}
	runRoot, err := runsRoot.OpenRoot(runID)
	if err != nil {
		return nil, runRecord{}, err
	}
	defer runRoot.Close()
	run, err := h.readAuthorizedRun(runID, runRoot)
	if err != nil {
		return nil, runRecord{}, errInvestigationBinding
	}
	info, err := runRoot.Lstat(investigationDirectory)
	if os.IsNotExist(err) && create {
		if err := runRoot.Mkdir(investigationDirectory, 0o700); err != nil && !os.IsExist(err) {
			return nil, runRecord{}, err
		}
		info, err = runRoot.Lstat(investigationDirectory)
	}
	if os.IsNotExist(err) {
		return nil, runRecord{}, errInvestigationNotFound
	}
	if err != nil {
		return nil, runRecord{}, err
	}
	if !info.Mode().IsDir() {
		return nil, runRecord{}, errInvestigationBinding
	}
	root, err := os.OpenRoot(filepath.Join(h.runsDir, runID, investigationDirectory))
	if err != nil {
		return nil, runRecord{}, err
	}
	return root, run, nil
}

func (h *handler) readAuthorizedRun(runID string, root *os.Root) (runRecord, error) {
	reportJSON, err := readRootFile(root, "report.json", maxArtifactBytes)
	if err != nil {
		return runRecord{}, err
	}
	var reportData report.ReportData
	if err := json.Unmarshal(reportJSON, &reportData); err != nil || reportData.FormatVersion != report.CurrentFormatVersion {
		return runRecord{}, fmt.Errorf("invalid report")
	}
	manifestJSON, err := readRootFile(root, report.RunManifestFilename, maxArtifactBytes)
	if err != nil {
		return runRecord{}, err
	}
	manifest, err := report.DecodeRunManifest(manifestJSON)
	if err != nil {
		return runRecord{}, err
	}
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
		return runRecord{}, err
	}
	analysisRoot, err := manifest.ResolveAnalysisRoot()
	if err != nil {
		return runRecord{}, err
	}
	return runRecord{
		RunSummary: RunSummary{ID: runID, RepoName: reportData.RepoName},
		RepoPath:   analysisRoot,
		Manifest:   &manifest,
		Report:     &reportData,
	}, nil
}

func verifyInvestigationBinding(run runRecord, record memory.Record) error {
	if run.Manifest == nil || run.Report == nil || record.Facts == nil || record.Claims != nil || len(record.Changes) != 0 {
		return errInvestigationBinding
	}
	if run.Manifest.VerifyRepositoryState(record.Repository) != nil ||
		run.Manifest.VerifyRepositoryState(record.Facts.Repository) != nil {
		return errInvestigationBinding
	}
	session := record.Session
	if session.Repository.Path != run.Manifest.AnalysisRoot || session.Repository.Path != run.RepoPath ||
		session.Repository.Revision != run.Manifest.RepositoryStateSHA256 || session.Origin == nil ||
		session.Origin.Kind != investigation.OriginOrientationComponent ||
		session.Origin.Status != investigation.OriginCandidate ||
		session.Origin.ReportSHA256 != run.Manifest.ReportSHA256 ||
		session.Origin.AcceptedRevision != run.Manifest.RepositoryStateSHA256 ||
		session.Origin.RepoName != run.Report.RepoName || session.Origin.FlowID != "" || session.Origin.FlowName != "" ||
		session.Focus.Entity == nil || session.Focus.Entity.Location == nil || session.Symbol == nil || session.Source == nil ||
		session.Assessment != nil || session.SourceReport != nil {
		return errInvestigationBinding
	}
	anchor, ok := authorizedOriginAnchor(*run.Manifest, session.Origin.ComponentID, session.Origin.AnchorID)
	if !ok || !anchor.CanListSymbols || session.Focus.Entity.Location.Path != anchor.Path ||
		session.Symbol.Target.Entity.Location == nil || session.Symbol.Target.Entity.Location.Path != anchor.Path {
		return errInvestigationBinding
	}
	switch session.State {
	case investigation.StateFindingTestReferences:
		if session.Tests != nil || len(session.Next) != 1 || session.Next[0].Kind != investigation.ActionFindTestReferences {
			return errInvestigationBinding
		}
	case investigation.StateCompleted:
		if session.Tests == nil || len(session.Next) != 0 {
			return errInvestigationBinding
		}
	default:
		return errInvestigationBinding
	}
	return nil
}

func authorizedOriginAnchor(
	manifest report.RunManifest,
	componentID string,
	anchorID string,
) (report.AnchorAuthority, bool) {
	for _, component := range manifest.Components {
		if component.ID != componentID {
			continue
		}
		for _, anchor := range component.Anchors {
			if anchor.ID == anchorID {
				return anchor, true
			}
		}
		return report.AnchorAuthority{}, false
	}
	return report.AnchorAuthority{}, false
}

func publicTestReferences(bundle *testevidence.Bundle) ([]publicTestReference, []string) {
	if bundle == nil {
		return nil, nil
	}
	references := make([]publicTestReference, 0, min(len(bundle.References), maxPublicTestReferences))
	for _, reference := range bundle.References {
		if len(references) == maxPublicTestReferences {
			break
		}
		if !fs.ValidPath(reference.Path) || strings.ContainsRune(reference.Path, '\\') ||
			!strings.HasSuffix(reference.Path, "_test.go") || reference.Line <= 0 || reference.Column <= 0 {
			continue
		}
		references = append(references, publicTestReference{
			Path:      reference.Path,
			Line:      reference.Line,
			Column:    reference.Column,
			Kind:      reference.Kind,
			Certainty: string(reference.Certainty),
		})
	}
	warnings := make([]string, 0, len(bundle.Warnings))
	for _, warning := range bundle.Warnings {
		var message string
		switch warning.Code {
		case "support.reference_only":
			message = "test references are navigation evidence only; no test behavior was assessed"
		case "searches.truncated":
			message = "some lower-priority reference searches were omitted"
		case "references.truncated":
			message = "additional test references were omitted"
		case "references.failed":
			message = "some test references could not be collected"
		case "references.invalid":
			message = "some invalid test-reference evidence was ignored"
		}
		if message != "" && !containsPublicWarning(warnings, message) {
			warnings = append(warnings, message)
		}
	}
	return references, warnings
}

func containsPublicWarning(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
