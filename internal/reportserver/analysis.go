package reportserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/inspection"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
	"github.com/dvordrova/repomap/internal/testevidence"
)

const (
	defaultAnalysisTimeout = 45 * time.Second
	maxCandidateSets       = 32
	maxRankTerms           = 16
)

// CaptureRepositoryFunc is injected so request-bound freshness checks can be
// tested without shelling out to git. Production uses freshness.CaptureRepository.
type CaptureRepositoryFunc func(context.Context, string) (freshness.RepositoryState, error)

type symbolAnalysis struct {
	resolver     analysis.LocationResolver
	exact        analysis.ExactSymbolAnalyzer
	references   testevidence.ReferenceFinder
	capture      CaptureRepositoryFunc
	captureFacts CaptureFactContextFunc
	timeout      time.Duration
	slot         chan struct{}

	mu    sync.Mutex
	sets  map[string]candidateSet
	order []string
}

type symbolsRequest struct {
	RunID       string `json:"run_id"`
	ComponentID string `json:"component_id"`
	AnchorID    string `json:"anchor_id"`
	Line        int    `json:"line"`
}

type symbolsResponse struct {
	Status         string                    `json:"status"`
	CandidateSetID string                    `json:"candidate_set_id"`
	Candidates     []publicLocationCandidate `json:"candidates"`
	Warnings       []string                  `json:"warnings,omitempty"`
	Provenance     publicProvenance          `json:"provenance"`
}

type publicLocationCandidate struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Kind        evidence.EntityKind `json:"kind"`
	Path        string              `json:"path"`
	Line        int                 `json:"line"`
	Column      int                 `json:"column,omitempty"`
	Match       string              `json:"match"`
	Certainty   evidence.Certainty  `json:"certainty"`
	Distance    int                 `json:"distance_lines"`
	RankReasons []string            `json:"rank_reasons,omitempty"`
}

type publicProvenance struct {
	Provider  string `json:"provider"`
	Version   string `json:"version,omitempty"`
	Operation string `json:"operation"`
}

type candidateSet struct {
	RunID          string
	ComponentID    string
	AnchorID       string
	AnchorLine     int
	ReportSHA256   string
	RepositoryHash string
	AnalysisRoot   string
	Candidates     map[string]inspection.Candidate
	CreatedAt      time.Time
}

type authorizedAnchor struct {
	run       runRecord
	component report.Component
	anchor    report.AnchorGroup
}

func newSymbolAnalysis(opts Options) *symbolAnalysis {
	if opts.LocationResolver == nil {
		return nil
	}
	capture := opts.CaptureRepository
	if capture == nil {
		capture = freshness.CaptureRepository
	}
	timeout := opts.AnalysisTimeout
	if timeout <= 0 {
		timeout = defaultAnalysisTimeout
	}
	exact := opts.ExactSymbolAnalyzer
	if exact == nil {
		exact, _ = opts.LocationResolver.(analysis.ExactSymbolAnalyzer)
	}
	references := opts.ReferenceFinder
	if references == nil {
		references, _ = opts.LocationResolver.(testevidence.ReferenceFinder)
	}
	captureFacts := opts.CaptureFactContext
	if captureFacts == nil {
		captureFacts = captureBrowserFactContext
	}
	return &symbolAnalysis{
		resolver:     opts.LocationResolver,
		exact:        exact,
		references:   references,
		capture:      capture,
		captureFacts: captureFacts,
		timeout:      timeout,
		slot:         make(chan struct{}, 1),
		sets:         make(map[string]candidateSet),
	}
}

func (a *symbolAnalysis) inspectionService(catalog *sourcecatalog.Catalog) (*inspection.Service, error) {
	if a == nil || catalog == nil {
		return nil, fmt.Errorf("inspection authority unavailable")
	}
	return inspection.New(*catalog, inspection.Dependencies{
		Resolver:        a.resolver,
		ExactAnalyzer:   a.exact,
		ReferenceFinder: a.references,
	})
}

func (h *handler) serveSymbols(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Repomap-Action") != "list-symbols" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing repomap action header"})
		return
	}
	if h.analysis == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "local Go analysis is unavailable"})
		return
	}
	defer r.Body.Close()
	var request symbolsRequest
	if err := decodeJSONBody(w, r, &request, 4096); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid symbol request"})
		return
	}
	if request.Line < 0 || request.Line > 10_000_000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid source line"})
		return
	}
	authorized, err := h.authorizeAnchor(request)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "component anchor is not authorized for local analysis"})
		return
	}

	select {
	case h.analysis.slot <- struct{}{}:
		defer func() { <-h.analysis.slot }()
	default:
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "another local analysis is still running"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.analysis.timeout)
	defer cancel()
	before, err := h.analysis.capture(ctx, authorized.run.RepoPath)
	if err != nil {
		h.writeAnalysisError(w, ctx, "could not verify repository state")
		return
	}
	if err := authorized.run.verifyRepositoryState(before); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "report is stale; regenerate it before analyzing symbols"})
		return
	}

	line := request.Line
	if line == 0 {
		line = 1
	}
	service, err := h.analysis.inspectionService(authorized.run.SourceCatalog)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "local Go analysis is unavailable"})
		return
	}
	resolution, err := service.Resolve(ctx, inspection.ResolveRequest{
		Location:  evidence.Location{Path: authorized.anchor.Path, Line: line},
		RankTerms: componentRankTerms(authorized.component, authorized.anchor),
	})
	if err != nil {
		if inspection.ErrorKindOf(err) == inspection.ErrorSourceChanged {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "report is stale; regenerate it before analyzing symbols"})
			return
		}
		if inspection.ErrorKindOf(err) == inspection.ErrorNotFound {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "no callable Go declarations were found near this anchor"})
			return
		}
		h.writeAnalysisError(w, ctx, "could not resolve Go symbols for this anchor")
		return
	}
	after, err := h.analysis.capture(ctx, authorized.run.RepoPath)
	if err != nil {
		h.writeAnalysisError(w, ctx, "could not recheck repository state")
		return
	}
	if err := authorized.run.verifyRepositoryState(after); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "repository changed during analysis; regenerate the report"})
		return
	}

	response, err := h.analysis.rememberCandidates(authorized, request.Line, resolution)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "could not prepare symbol candidates"})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *handler) writeAnalysisError(w http.ResponseWriter, ctx context.Context, message string) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "local Go analysis timed out"})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": message})
}

func (h *handler) authorizeAnchor(request symbolsRequest) (authorizedAnchor, error) {
	run, err := h.findRun(request.RunID)
	if err != nil || run.Manifest == nil || run.WorkspaceSnapshot == nil ||
		run.Report == nil || run.SourceCatalog == nil || h.analysis == nil {
		return authorizedAnchor{}, fmt.Errorf("analysis authority unavailable")
	}
	var componentAuthority *report.ComponentAuthority
	for index := range run.Manifest.Components {
		if run.Manifest.Components[index].ID == request.ComponentID {
			componentAuthority = &run.Manifest.Components[index]
			break
		}
	}
	if componentAuthority == nil {
		return authorizedAnchor{}, fmt.Errorf("component is not authorized")
	}
	var anchorAuthority *report.AnchorAuthority
	for index := range componentAuthority.Anchors {
		if componentAuthority.Anchors[index].ID == request.AnchorID {
			anchorAuthority = &componentAuthority.Anchors[index]
			break
		}
	}
	if anchorAuthority == nil || !anchorAuthority.CanListSymbols || !allowedAnchorLine(*anchorAuthority, request.Line) {
		return authorizedAnchor{}, fmt.Errorf("anchor is not authorized")
	}
	var component *report.Component
	for index := range run.Report.Components {
		if run.Report.Components[index].ID == request.ComponentID {
			component = &run.Report.Components[index]
			break
		}
	}
	if component == nil {
		return authorizedAnchor{}, fmt.Errorf("component presentation is unavailable")
	}
	var anchor *report.AnchorGroup
	for index := range component.AnchorGroups {
		if component.AnchorGroups[index].ID == request.AnchorID {
			anchor = &component.AnchorGroups[index]
			break
		}
	}
	if anchor == nil || anchor.Path != anchorAuthority.Path || !anchor.CanListSymbols {
		return authorizedAnchor{}, fmt.Errorf("anchor presentation does not match authority")
	}
	return authorizedAnchor{run: run, component: *component, anchor: *anchor}, nil
}

func allowedAnchorLine(anchor report.AnchorAuthority, line int) bool {
	if len(anchor.AllowedLines) == 0 {
		return line == 0
	}
	if line <= 0 {
		return false
	}
	index := sort.SearchInts(anchor.AllowedLines, line)
	return index < len(anchor.AllowedLines) && anchor.AllowedLines[index] == line
}

func componentRankTerms(component report.Component, anchor report.AnchorGroup) []string {
	values := []string{component.Name, component.ModelPurpose, filepath.Base(anchor.Path)}
	values = append(values, anchor.ModelNotes...)
	seen := make(map[string]struct{})
	terms := make([]string, 0, maxRankTerms)
	for _, value := range values {
		fields := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		})
		for _, field := range fields {
			if len(terms) == maxRankTerms {
				return terms
			}
			if len(field) < 2 || len(field) > 64 {
				continue
			}
			if _, duplicate := seen[field]; duplicate {
				continue
			}
			seen[field] = struct{}{}
			terms = append(terms, field)
		}
	}
	return terms
}

func (a *symbolAnalysis) rememberCandidates(
	authorized authorizedAnchor,
	anchorLine int,
	resolution inspection.ResolveResult,
) (symbolsResponse, error) {
	setID, err := generateCapability()
	if err != nil {
		return symbolsResponse{}, err
	}
	set := candidateSet{
		RunID:          authorized.run.ID,
		ComponentID:    authorized.component.ID,
		AnchorID:       authorized.anchor.ID,
		AnchorLine:     anchorLine,
		ReportSHA256:   authorized.run.Manifest.ReportSHA256,
		RepositoryHash: authorized.run.workspaceRepositoryDigest(),
		AnalysisRoot:   authorized.run.workspaceAnalysisRoot(),
		Candidates:     make(map[string]inspection.Candidate, len(resolution.Candidates)),
		CreatedAt:      time.Now(),
	}
	publicCandidates := make([]publicLocationCandidate, 0, len(resolution.Candidates))
	for _, candidate := range resolution.Candidates {
		candidateID, err := generateCapability()
		if err != nil {
			return symbolsResponse{}, err
		}
		set.Candidates[candidateID] = candidate
		location := candidate.Entity.Location
		publicCandidates = append(publicCandidates, publicLocationCandidate{
			ID:          candidateID,
			Name:        candidate.Entity.Name,
			Kind:        candidate.Entity.Kind,
			Path:        location.Path,
			Line:        location.Line,
			Column:      location.Column,
			Match:       candidate.Match,
			Certainty:   candidate.Certainty,
			Distance:    candidate.Distance,
			RankReasons: append([]string(nil), candidate.RankReasons...),
		})
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	for len(a.order) >= maxCandidateSets {
		delete(a.sets, a.order[0])
		a.order = a.order[1:]
	}
	a.sets[setID] = set
	a.order = append(a.order, setID)

	warnings := append([]string(nil), resolution.Warnings...)
	if len(warnings) > 8 {
		warnings = warnings[:8]
	}
	return symbolsResponse{
		Status:         "ok",
		CandidateSetID: setID,
		Candidates:     publicCandidates,
		Warnings:       warnings,
		Provenance: publicProvenance{
			Provider:  resolution.Provenance.Provider,
			Version:   resolution.Provenance.Version,
			Operation: resolution.Provenance.Operation,
		},
	}, nil
}
