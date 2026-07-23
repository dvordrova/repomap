package reportserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/reporead"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/secretscan"
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
	if run.Manifest == nil || run.Manifest.Version < report.CurrentRunManifestVersion || run.Report == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "this report is view-only"})
		return
	}
	target, ok := run.SourceContexts[request.ContextID]
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "source context is not authorized by this report"})
		return
	}
	analysisRoot, err := run.Manifest.ResolveAnalysisRoot()
	if err != nil || analysisRoot != run.RepoPath {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "authorized source is unavailable", "code": "source_unavailable"})
		return
	}
	input, ok := capturedSourceInput(*run.Manifest, target.relativePath)
	if !ok || input.Kind != freshness.FileRegular || input.ContentSHA256 == "" ||
		input.ContentSHA256 != target.capturedSHA256 {
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

	reader, err := reporead.New(analysisRoot)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "authorized source is unavailable", "code": "source_unavailable"})
		return
	}
	defer reader.Close()
	current, err := reader.ReadFile(target.relativePath, maxSourceContextFileBytes)
	if err != nil || current.Truncated || !utf8.Valid(current.Bytes) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "authorized source is unavailable", "code": "source_unavailable"})
		return
	}
	digest := sha256.Sum256(current.Bytes)
	if fmt.Sprintf("%x", digest[:]) != target.capturedSHA256 {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "source changed since this report was generated", "code": "source_changed",
		})
		return
	}

	lines := strings.Split(string(current.Bytes), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	startLine, endLine, ok := boundedSourceContextRange(target, len(lines))
	if !ok {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "authorized source location is unavailable", "code": "source_unavailable"})
		return
	}
	selected := lines[startLine-1 : endLine]
	text := strings.Join(selected, "\n")
	if len(text) > maxSourceContextTextBytes {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "bounded source context is too large"})
		return
	}
	if _, containsSecret := secretscan.Detect(text); containsSecret {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "bounded source context is unavailable"})
		return
	}
	response := sourceContextResponse{
		Status: "ok",
		Source: publicSourceContext{
			Path: target.relativePath, StartLine: startLine, EndLine: endLine,
			Lines: make([]publicSourceLine, 0, len(selected)),
		},
	}
	for index, line := range selected {
		response.Source.Lines = append(response.Source.Lines, publicSourceLine{
			Line: startLine + index, Text: strings.TrimSuffix(line, "\r"),
		})
	}
	writeBoundedSourceContextJSON(w, response)
}

func boundedSourceContextRange(target sourceContextTarget, lineCount int) (int, int, bool) {
	if lineCount <= 0 || target.startLine <= 0 || target.endLine < target.startLine || target.startLine > lineCount {
		return 0, 0, false
	}
	start := target.startLine
	end := min(target.endLine, lineCount)
	if end-start+1 <= 60 {
		start = max(1, start-10)
		end = min(lineCount, end+10)
	} else {
		focus := target.focusLine
		if focus < target.startLine || focus > target.endLine {
			focus = target.startLine
		}
		start = max(1, focus-20)
		end = min(lineCount, start+maxSourceContextLines-1)
	}
	if end-start+1 > maxSourceContextLines {
		end = start + maxSourceContextLines - 1
	}
	return start, end, start > 0 && end >= start
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

func capturedSourceInput(manifest report.RunManifest, relativePath string) (freshness.CapturedInput, bool) {
	repositoryRelative := relativePath
	if analysisRelative, err := filepath.Rel(manifest.RepositoryState.Identity, manifest.AnalysisRoot); err == nil && analysisRelative != "." {
		repositoryRelative = filepath.ToSlash(filepath.Join(analysisRelative, filepath.FromSlash(relativePath)))
	}
	for _, input := range manifest.CapturedInputs {
		if input.Path == repositoryRelative || input.Path == relativePath {
			return input, true
		}
	}
	return freshness.CapturedInput{}, false
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
