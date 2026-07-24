package reportserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/workspacecontent"
)

const (
	maxSourceContextFileBytes     = 8 << 20
	maxSourceContextLines         = 80
	maxSourceContextTextBytes     = 32 << 10
	maxSourceContextResponseBytes = 256 << 10
)

type sourceContextRequest struct {
	RunID     string `json:"run_id"`
	ContextID string `json:"context_id"`
}

type sourceContextTarget struct {
	relativePath   string
	capturedSHA256 string
	startLine      int
	endLine        int
	focusLine      int
}

type sourceContextResponse struct {
	Status string              `json:"status"`
	Source publicSourceContext `json:"source"`
}

type publicSourceContext struct {
	Path           string             `json:"path"`
	StartLine      int                `json:"start_line"`
	EndLine        int                `json:"end_line"`
	Lines          []publicSourceLine `json:"lines"`
	SourceComplete bool               `json:"source_complete"`
}

func (h *handler) serveSourceContext(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Repomap-Action") != "read-source-context" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "missing repomap action header"})
		return
	}
	defer r.Body.Close()
	var request sourceContextRequest
	if err := decodeJSONBody(w, r, &request, 4096); err != nil ||
		!validRunID(request.RunID) || !validCapability(request.ContextID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid source-context request"})
		return
	}
	run, err := h.findRun(request.RunID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "report run not found"})
		return
	}
	if run.Manifest == nil || run.Manifest.Version < report.CurrentRunManifestVersion ||
		run.Report == nil || run.SourceCatalog == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "this report is view-only"})
		return
	}
	target, ok := run.SourceContexts[request.ContextID]
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "source context is not authorized by this report"})
		return
	}
	analysisRoot := run.SourceCatalog.AnalysisRoot()
	resolvedRoot, err := run.Manifest.ResolveAnalysisRoot()
	if err != nil || resolvedRoot != analysisRoot || analysisRoot != run.RepoPath {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "authorized source is unavailable", "code": "source_unavailable"})
		return
	}
	source, ok := run.SourceCatalog.Lookup(target.relativePath)
	if !ok || source.Kind != freshness.FileRegular || source.ContentSHA256 == "" ||
		source.ContentSHA256 != target.capturedSHA256 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "authorized source snapshot is unavailable", "code": "source_unavailable"})
		return
	}
	select {
	case h.sourceSlot <- struct{}{}:
		defer func() { <-h.sourceSlot }()
	default:
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "another source read is still running", "code": "source_busy"})
		return
	}

	contentService, err := workspacecontent.New(*run.SourceCatalog)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "authorized source is unavailable", "code": "source_unavailable"})
		return
	}
	defer contentService.Close()
	content, err := contentService.Read(r.Context(), workspacecontent.Request{
		Path: target.relativePath,
		Range: workspacecontent.Range{
			StartLine: target.startLine,
			EndLine:   target.endLine,
			FocusLine: target.focusLine,
		},
		Limits: workspacecontent.Limits{
			MaxFileBytes: maxSourceContextFileBytes,
			MaxLines:     maxSourceContextLines,
			MaxBytes:     maxSourceContextTextBytes,
			MaxLineBytes: maxSourceContextTextBytes,
		},
	})
	if err != nil {
		writeSourceContextReadError(w, err)
		return
	}
	if _, containsSecret := secretscan.Detect(content.Text); containsSecret {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "bounded source context is unavailable"})
		return
	}
	response := sourceContextResponse{
		Status: "ok",
		Source: publicSourceContext{
			Path: content.Path, StartLine: content.StartLine, EndLine: content.EndLine,
			Lines: make([]publicSourceLine, 0, len(content.Lines)),
		},
	}
	for _, line := range content.Lines {
		response.Source.Lines = append(response.Source.Lines, publicSourceLine{
			Line: line.Number, Text: line.Text, Truncated: line.Truncated,
		})
	}
	writeBoundedSourceContextJSON(w, response)
}

func writeSourceContextReadError(w http.ResponseWriter, err error) {
	switch {
	case workspacecontent.ErrorKindOf(err) == workspacecontent.ErrorSourceChanged:
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "source changed since this report was generated", "code": "source_changed",
		})
	case workspacecontent.ErrorKindOf(err) == workspacecontent.ErrorLimitExceeded &&
		workspacecontent.LimitKindOf(err) != workspacecontent.LimitFile:
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
			"error": "bounded source context is too large",
		})
	case workspacecontent.FailureStageOf(err) == workspacecontent.StageRange:
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "authorized source location is unavailable", "code": "source_unavailable",
		})
	default:
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "authorized source is unavailable", "code": "source_unavailable",
		})
	}
}

func manifestSourceContextID(runID, reportSHA256, presentationSHA256 string) string {
	digest := sha256.Sum256([]byte(
		"repomap-source-context-v1\x00" + runID + "\x00" + reportSHA256 + "\x00" + presentationSHA256,
	))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func reportSourceSnippets(data *report.ReportData) []report.SourceSnippet {
	if data == nil {
		return nil
	}
	result := append([]report.SourceSnippet(nil), data.UserSources...)
	for _, mechanism := range data.UserMechanisms {
		for _, step := range mechanism.Steps {
			result = append(result, step.Sources...)
		}
	}
	if data.StudyMap != nil {
		for _, area := range data.StudyMap.Shape {
			if area.Source != nil {
				result = append(result, *area.Source)
			}
		}
		for _, direction := range data.StudyMap.Directions {
			for _, reading := range direction.ReadingAnchors {
				result = append(result, reading.Source)
			}
			for _, document := range direction.Documents {
				if document.Source != nil {
					result = append(result, *document.Source)
				}
			}
		}
	}
	if data.Operations != nil {
		appendReference := func(reference report.OperationalReference) {
			if !reference.Redacted {
				result = append(result, reference.Source)
			}
		}
		for _, pavedPath := range data.Operations.Paths {
			for _, reference := range pavedPath.Prerequisites {
				appendReference(reference)
			}
			for _, action := range pavedPath.Actions {
				appendReference(action.Reference)
			}
			for _, reference := range pavedPath.Expected {
				appendReference(reference)
			}
			for _, reference := range pavedPath.Troubleshooting {
				appendReference(reference)
			}
		}
		for _, landmark := range data.Operations.Landmarks {
			appendReference(landmark.Reference)
		}
	}
	if data.TaskInvestigation != nil {
		for _, anchor := range data.TaskInvestigation.Anchors {
			result = append(result, anchor.Source)
		}
	}
	return result
}

func writeBoundedSourceContextJSON(w http.ResponseWriter, response sourceContextResponse) {
	var buffer bytes.Buffer
	if err := json.NewEncoder(&buffer).Encode(response); err != nil || buffer.Len() > maxSourceContextResponseBytes {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "bounded source context is too large"})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buffer.Bytes())
}
