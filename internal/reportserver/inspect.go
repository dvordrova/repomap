package reportserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	analysis "github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/investigation"
	"github.com/dvordrova/repomap/internal/memory"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/symbol"
)

const candidateSetTTL = 15 * time.Minute

type inspectSymbolRequest struct {
	RunID          string `json:"run_id"`
	CandidateSetID string `json:"candidate_set_id"`
	CandidateID    string `json:"candidate_id"`
}

type inspectSymbolResponse struct {
	Status                string                `json:"status"`
	ComponentID           string                `json:"component_id"`
	AnchorID              string                `json:"anchor_id"`
	InvestigationStatus   string                `json:"investigation_status"`
	CanFindTestReferences bool                  `json:"can_find_test_references"`
	EvidenceLevel         evidence.Certainty    `json:"evidence_level"`
	Target                publicEntity          `json:"target"`
	Source                publicSource          `json:"source"`
	IncomingCalls         []publicCall          `json:"incoming_calls"`
	OutgoingCalls         []publicCall          `json:"outgoing_calls"`
	TestReferences        []publicTestReference `json:"test_references,omitempty"`
	TestWarnings          []string              `json:"test_warnings,omitempty"`
	Warnings              []string              `json:"warnings,omitempty"`
	Truncated             map[string]int        `json:"truncated,omitempty"`
}

type publicEntity struct {
	Name   string              `json:"name"`
	Kind   evidence.EntityKind `json:"kind"`
	Path   string              `json:"path"`
	Line   int                 `json:"line"`
	Column int                 `json:"column,omitempty"`
}

type publicCall struct {
	Symbol    publicEntity       `json:"symbol"`
	Callsite  *publicLocation    `json:"callsite,omitempty"`
	Certainty evidence.Certainty `json:"certainty"`
}

type publicLocation struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column,omitempty"`
}

type publicSource struct {
	Path       string                `json:"path"`
	StartLine  int                   `json:"start_line"`
	EndLine    int                   `json:"end_line"`
	StopReason sourcecard.StopReason `json:"stop_reason"`
	Truncated  bool                  `json:"truncated"`
	Lines      []publicSourceLine    `json:"lines"`
}

type publicSourceLine struct {
	Line      int    `json:"line"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated,omitempty"`
}

func (h *handler) serveInspectSymbol(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Repomap-Action") != "inspect-symbol" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing repomap action header"})
		return
	}
	if h.analysis == nil || h.analysis.exact == nil || h.analysis.captureFacts == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "exact Go symbol analysis is unavailable"})
		return
	}
	defer r.Body.Close()
	var request inspectSymbolRequest
	if err := decodeJSONBody(w, r, &request, 4096); err != nil ||
		!validRunID(request.RunID) || !validCapability(request.CandidateSetID) || !validCapability(request.CandidateID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid inspect-symbol request"})
		return
	}
	set, candidate, ok := h.analysis.candidateForInspection(request.CandidateSetID, request.CandidateID)
	if !ok || set.RunID != request.RunID {
		writeJSON(w, http.StatusGone, map[string]string{"error": "symbol candidates expired; find Go symbols again"})
		return
	}
	storeRoot, run, err := h.openInvestigationStore(request.RunID, true)
	if err != nil {
		if errors.Is(err, errInvestigationNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "report run not found"})
		} else if errors.Is(err, errInvestigationBinding) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "report authority changed; find Go symbols again"})
		} else {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "saved investigation storage is unavailable"})
		}
		return
	}
	defer storeRoot.Close()
	if run.Manifest == nil || run.Manifest.ReportSHA256 != set.ReportSHA256 ||
		run.Manifest.RepositoryStateSHA256 != set.RepositoryHash || run.RepoPath == "" || run.RepoPath != set.AnalysisRoot {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "report authority changed; find Go symbols again"})
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
	before, err := h.analysis.capture(ctx, run.RepoPath)
	if err != nil {
		h.writeAnalysisError(w, ctx, "could not verify repository state")
		return
	}
	if err := run.Manifest.VerifyRepositoryState(before); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "report is stale; regenerate it before inspecting symbols"})
		return
	}

	session, _, err := investigation.Reduce(investigation.Session{}, investigation.Event{
		Kind: investigation.EventStarted,
		Start: &investigation.StartInput{
			Goal:       investigation.Goal{Text: "Understand " + candidate.Entity.Name},
			Repository: investigation.Repository{Path: run.RepoPath, Revision: set.RepositoryHash},
			Focus: investigation.Focus{
				Kind:   investigation.FocusSymbol,
				Symbol: candidate.Entity.Name,
				Entity: &candidate.Entity,
			},
			Origin: &investigation.Origin{
				Kind:             investigation.OriginOrientationComponent,
				Status:           investigation.OriginCandidate,
				ReportSHA256:     set.ReportSHA256,
				RepoName:         run.Report.RepoName,
				ComponentID:      set.ComponentID,
				AnchorID:         set.AnchorID,
				AcceptedRevision: set.RepositoryHash,
			},
		},
	})
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "could not start the exact symbol investigation"})
		return
	}
	runner := investigation.Runner{
		ExactAnalyzer: h.analysis.exact,
		SymbolOptions: browserSymbolOptions(),
		SourceLimits:  browserSourceLimits(),
	}
	execution, err := runner.Execute(ctx, session, session.Next[0])
	if err != nil || execution.DiagnosticError != nil {
		h.writeAnalysisError(w, ctx, "could not inspect the selected Go symbol")
		return
	}
	session, _, err = investigation.Reduce(session, execution.Event)
	if err != nil || session.Symbol == nil || session.State != investigation.StateReadingSource {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "exact symbol evidence did not match the selected declaration"})
		return
	}
	sourceExecution, err := runner.Execute(ctx, session, session.Next[0])
	if err != nil || sourceExecution.DiagnosticError != nil || sourceExecution.Event.Source == nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "could not read a bounded source window for this declaration"})
		return
	}
	session, _, err = investigation.Reduce(session, sourceExecution.Event)
	if err != nil || session.State != investigation.StateFindingTestReferences || session.Source == nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "source evidence did not match the selected declaration"})
		return
	}
	facts, err := h.analysis.captureFacts(ctx, before, run.RepoPath)
	if err != nil {
		h.writeAnalysisError(w, ctx, "could not capture local analysis context")
		return
	}
	after, err := h.analysis.capture(ctx, run.RepoPath)
	if err != nil {
		h.writeAnalysisError(w, ctx, "could not recheck repository state")
		return
	}
	if err := run.Manifest.VerifyRepositoryState(after); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "repository changed during analysis; regenerate the report"})
		return
	}
	facts.Repository = run.Manifest.RepositoryState
	record := memory.Record{Session: session, Repository: run.Manifest.RepositoryState, Facts: &facts}
	if err := verifyInvestigationBinding(run, record); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "investigation no longer matches this report"})
		return
	}
	if err := memory.SaveRoot(storeRoot, memory.Input{
		Session:    session,
		Repository: run.Manifest.RepositoryState,
		Facts:      &facts,
	}); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "could not save the investigation"})
		return
	}

	response, err := buildInvestigationResponse(session)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "could not build the symbol evidence card"})
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *symbolAnalysis) candidateForInspection(setID, candidateID string) (candidateSet, analysis.LocationCandidate, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	set, ok := a.sets[setID]
	if !ok {
		return candidateSet{}, analysis.LocationCandidate{}, false
	}
	if time.Since(set.CreatedAt) > candidateSetTTL {
		delete(a.sets, setID)
		for index, id := range a.order {
			if id == setID {
				a.order = append(a.order[:index], a.order[index+1:]...)
				break
			}
		}
		return candidateSet{}, analysis.LocationCandidate{}, false
	}
	candidate, ok := set.Candidates[candidateID]
	return set, candidate, ok
}

func buildInspectSymbolResponse(bundle symbol.Bundle, card sourcecard.Card) (inspectSymbolResponse, error) {
	target, ok := publicEntityFrom(bundle.Target.Entity)
	if !ok {
		return inspectSymbolResponse{}, fmt.Errorf("target has no repository location")
	}
	response := inspectSymbolResponse{
		Status:        "ok",
		EvidenceLevel: evidence.CertaintyStatic,
		Target:        target,
		Source: publicSource{
			Path:       card.Target.Path,
			StartLine:  card.Window.StartLine,
			EndLine:    card.Window.EndLine,
			StopReason: card.Window.StopReason,
			Truncated:  card.Window.Truncated,
			Lines:      make([]publicSourceLine, 0, len(card.Lines)),
		},
		Truncated: cloneTruncation(bundle.Truncated),
	}
	for _, line := range card.Lines {
		response.Source.Lines = append(response.Source.Lines, publicSourceLine{
			Line:      line.Line,
			Text:      line.Text,
			Truncated: line.Truncated,
		})
	}
	for _, call := range bundle.IncomingCalls {
		if public, ok := publicCallFrom(call.Caller, call.Callsite, call.Certainty); ok {
			response.IncomingCalls = append(response.IncomingCalls, public)
		}
	}
	for _, call := range bundle.OutgoingCalls {
		if public, ok := publicCallFrom(call.Callee, call.Callsite, call.Certainty); ok {
			response.OutgoingCalls = append(response.OutgoingCalls, public)
		}
	}
	response.Warnings = append(response.Warnings, bundle.Warnings...)
	for _, warning := range card.Warnings {
		response.Warnings = append(response.Warnings, warning.Message)
	}
	if len(response.Warnings) > 8 {
		response.Warnings = response.Warnings[:8]
	}
	return response, nil
}

func buildInvestigationResponse(session investigation.Session) (inspectSymbolResponse, error) {
	if session.Symbol == nil || session.Source == nil || session.Origin == nil ||
		session.Origin.Kind != investigation.OriginOrientationComponent {
		return inspectSymbolResponse{}, fmt.Errorf("investigation response is incomplete")
	}
	response, err := buildInspectSymbolResponse(*session.Symbol, *session.Source)
	if err != nil {
		return inspectSymbolResponse{}, err
	}
	response.ComponentID = session.Origin.ComponentID
	response.AnchorID = session.Origin.AnchorID
	switch session.State {
	case investigation.StateFindingTestReferences:
		response.InvestigationStatus = "source_ready"
		response.CanFindTestReferences = true
	case investigation.StateCompleted:
		if session.Tests == nil {
			return inspectSymbolResponse{}, fmt.Errorf("completed investigation has no test references")
		}
		response.InvestigationStatus = "tests_ready"
		response.CanFindTestReferences = false
		response.TestReferences, response.TestWarnings = publicTestReferences(session.Tests)
	default:
		return inspectSymbolResponse{}, fmt.Errorf("investigation state %q is not public", session.State)
	}
	return response, nil
}

func publicCallFrom(entity evidence.Entity, callsite *evidence.Location, certainty evidence.Certainty) (publicCall, bool) {
	public, ok := publicEntityFrom(entity)
	if !ok {
		return publicCall{}, false
	}
	result := publicCall{Symbol: public, Certainty: certainty}
	if callsite != nil && callsite.Path != "" && callsite.Line > 0 {
		result.Callsite = &publicLocation{Path: callsite.Path, Line: callsite.Line, Column: callsite.Column}
	}
	return result, true
}

func publicEntityFrom(entity evidence.Entity) (publicEntity, bool) {
	if entity.Location == nil || entity.Location.Path == "" || entity.Location.Line <= 0 {
		return publicEntity{}, false
	}
	return publicEntity{
		Name:   entity.Name,
		Kind:   entity.Kind,
		Path:   entity.Location.Path,
		Line:   entity.Location.Line,
		Column: entity.Location.Column,
	}, true
}

func cloneTruncation(input map[string]int) map[string]int {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]int, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
