package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/dvordrova/repomap/internal/freshness"

	_ "embed"
)

//go:embed templates/report.html
var templateHTML string

//go:embed templates/style.css
var styleCSS string

//go:embed templates/architecture_canvas.css
var architectureCanvasCSS string

//go:embed templates/architecture_canvas.js
var architectureCanvasJS string

//go:embed templates/surface_catalog.css
var surfaceCatalogCSS string

//go:embed templates/surface_catalog.js
var surfaceCatalogJS string

//go:embed templates/semantic_search.css
var semanticSearchCSS string

//go:embed templates/semantic_search.js
var semanticSearchJS string

//go:embed templates/script.js
var scriptJS string

var reportTmpl *template.Template

// MaxSourceEpisodeBytes is the maximum approved source-episode artifact that
// the transient report renderer will inspect.
const MaxSourceEpisodeBytes = maxSourceEpisodeBytes

func init() {
	reportTmpl = template.Must(template.New("report").Parse(templateHTML))
}

func WriteReportJSON(data *ReportData, path string) error {
	if data == nil {
		return fmt.Errorf("report: data is required")
	}
	persisted := reportDataForRendering(data)
	// SourceIDs are issued by the local report server after manifest
	// verification. They are session navigation IDs, not persistent evidence.
	persisted.SourceIDs = nil
	persisted.SourceContextIDs = nil
	b, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func WriteReportHTML(data *ReportData, path string) error {
	html, err := RenderHTML(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, html, 0o644)
}

// RenderHTML renders report data with repomap's embedded trusted template and
// assets. Servers should use this instead of executing a saved HTML artifact.
func RenderHTML(data *ReportData) ([]byte, error) {
	if data == nil {
		return nil, fmt.Errorf("report: data is required")
	}
	return buildHTML(data)
}

// RenderHTMLWithSourceEpisode adds one approved, SHA-pinned source episode to
// the rendered Study surface. The projection exists only in this HTML
// response: it is not added to ReportData or persisted in report.json.
func RenderHTMLWithSourceEpisode(data *ReportData, episodeJSON []byte) ([]byte, error) {
	if data == nil {
		return nil, fmt.Errorf("report: data is required")
	}
	episode, err := projectApprovedSourceEpisode(data, episodeJSON)
	if err != nil {
		return nil, err
	}
	return buildHTMLWithSourceEpisode(data, episode)
}

// ValidateSourceEpisodeForRevision rejects an unapproved artifact or revision
// mismatch before a caller spends time on repository orientation. It performs
// the same bounded validation as rendering without persisting the input.
func ValidateSourceEpisodeForRevision(episodeJSON []byte, revision string) error {
	_, err := projectApprovedSourceEpisode(
		&ReportData{CapturedRevision: revision},
		episodeJSON,
	)
	return err
}

func buildHTML(data *ReportData) ([]byte, error) {
	return buildHTMLWithSourceEpisode(data, nil)
}

func buildHTMLWithSourceEpisode(data *ReportData, episode *sourceEpisodeProjection) ([]byte, error) {
	rendered := reportDataForRendering(data)
	if episode != nil {
		// The source-first answer is the complete destination for this bounded
		// experiment. Keep the legacy search index persisted in report.json,
		// but do not serialize or ship its UI/assets in the projected HTML.
		rendered.SemanticSearch = nil
	}
	css := styleCSS
	js := scriptJS
	if episode == nil {
		css = withoutSourceEpisodeAssetBlocks(css)
		js = withoutSourceEpisodeAssetBlocks(js)
	}
	var dataJSON []byte
	var err error
	if episode == nil {
		dataJSON, err = json.Marshal(rendered)
	} else {
		dataJSON, err = json.Marshal(struct {
			*ReportData
			SourceEpisode *sourceEpisodeProjection `json:"source_episode"`
		}{
			ReportData:    rendered,
			SourceEpisode: episode,
		})
	}
	if err != nil {
		return nil, err
	}

	title := data.RepoName
	if data.ProjectGuess != "" {
		title = title + " — " + data.ProjectGuess
	}

	var buf bytes.Buffer
	err = reportTmpl.Execute(&buf, map[string]any{
		"Title":                 title,
		"CSS":                   template.CSS(css),
		"HasArchitectureCanvas": data.ArchitectureCanvas != nil,
		"ArchitectureCanvasCSS": template.CSS(architectureCanvasCSS),
		"ELKJS":                 template.JS(elkJSBundledJS),
		"ArchitectureCanvasJS":  template.JS(architectureCanvasJS),
		"HasDiscoveredSurfaces": data.DiscoveredSurfaces != nil,
		"SurfaceCatalogCSS":     template.CSS(surfaceCatalogCSS),
		"SurfaceCatalogJS":      template.JS(surfaceCatalogJS),
		"HasSemanticSearch":     rendered.SemanticSearch != nil && !rendered.SemanticSearchDisabled,
		"SemanticSearchCSS":     template.CSS(semanticSearchCSS),
		"SemanticSearchJS":      template.JS(semanticSearchJS),
		"DataJSON":              template.JS(dataJSON),
		"JS":                    template.JS(js),
	})
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func withoutSourceEpisodeAssetBlocks(asset string) string {
	const (
		startMarker = "repomap-source-episode:start"
		endMarker   = "repomap-source-episode:end"
	)
	for {
		start := strings.Index(asset, startMarker)
		if start < 0 {
			return asset
		}
		lineStart := strings.LastIndex(asset[:start], "\n")
		if lineStart < 0 {
			lineStart = 0
		}
		endOffset := strings.Index(asset[start:], endMarker)
		if endOffset < 0 {
			return asset
		}
		lineEnd := strings.Index(asset[start+endOffset:], "\n")
		if lineEnd < 0 {
			asset = asset[:lineStart]
			continue
		}
		lineEnd += start + endOffset
		left := asset[:lineStart]
		right := asset[lineEnd+1:]
		separator := "\n"
		if strings.HasSuffix(left, "\n") || strings.HasPrefix(right, "\n") {
			separator = ""
		}
		asset = left + separator + right
	}
}

func reportDataForRendering(data *ReportData) *ReportData {
	rendered := *data
	if rendered.SemanticSearchDisabled {
		rendered.SemanticSearch = nil
		return &rendered
	}
	if rendered.SemanticSearch != nil {
		if err := rendered.SemanticSearch.Validate(&rendered); err == nil {
			return &rendered
		}
	}
	rendered.SemanticSearch = nil
	_ = attachSemanticSearchIndex(&rendered)
	return &rendered
}

func Generate(runDir string) error {
	return generate(runDir, nil, nil)
}

// GenerateAuthorized renders a report and binds its exact generated JSON to
// repository authority confirmed stable across orientation.
func GenerateAuthorized(runDir string, authority RunAuthority) error {
	return generate(runDir, &authority, nil)
}

// GenerateAuthorizedWithSourceEpisode generates the same persisted report and
// manifest as GenerateAuthorized, while placing one approved, SHA-pinned
// source episode only in the static report.html review surface.
func GenerateAuthorizedWithSourceEpisode(
	runDir string,
	authority RunAuthority,
	episodeJSON []byte,
) error {
	if err := authority.validate(); err != nil {
		return err
	}
	if len(episodeJSON) == 0 || len(episodeJSON) > MaxSourceEpisodeBytes {
		return fmt.Errorf("report: source episode input is outside the byte budget")
	}
	episodeJSON = append([]byte(nil), episodeJSON...)
	if err := ValidateSourceEpisodeForRevision(episodeJSON, authority.repository.Head); err != nil {
		return err
	}
	return generate(runDir, &authority, episodeJSON)
}

func generate(runDir string, authority *RunAuthority, sourceEpisodeJSON []byte) error {
	if err := RemoveRunManifest(runDir); err != nil {
		return err
	}
	deferredSourceAuthority := authority != nil && authority.inputs == nil
	if authority != nil {
		if err := authority.validate(); err != nil {
			return err
		}
		if sourceAuthorityNeedsLiteralGitMode(authority.inputs) {
			// Clean captured-input mode lookup is pathspec-sensitive. Mark the
			// ambiguous path unavailable before any exact workspace adapters run
			// so both the report and manifest remain view-only.
			viewOnlyAuthority := *authority
			viewOnlyAuthority.inputs = append([]freshness.CapturedInput(nil), authority.inputs...)
			for index := range viewOnlyAuthority.inputs {
				if sourcePathNeedsLiteralGitMode(viewOnlyAuthority.inputs[index].Path) {
					viewOnlyAuthority.inputs[index].Kind = freshness.FileMissing
					viewOnlyAuthority.inputs[index].Mode = ""
					viewOnlyAuthority.inputs[index].ContentSHA256 = ""
				}
			}
			authority = &viewOnlyAuthority
		}
	}
	studyDocumentSourceRoot := ""
	if authority != nil {
		studyDocumentSourceRoot = authority.analysisRoot
	}
	data, err := readRunDir(runDir, studyDocumentSourceRoot, authority)
	if err != nil {
		return err
	}
	if sourceEpisodeJSON != nil && authority != nil {
		if err := retainSourceEpisodeRegularOpenablePaths(data, *authority); err != nil {
			return err
		}
	}
	if err := writePavedPathPublicationDiagnostics(runDir); err != nil {
		return err
	}
	if deferredSourceAuthority {
		repositoryPaths, pathErr := repositoryRelativeInputPaths(
			authority.repository.Identity,
			authority.analysisRoot,
			data.OpenablePaths,
		)
		if pathErr != nil {
			return pathErr
		}
		if sourcePathsNeedLiteralGitMode(repositoryPaths) {
			viewOnlyAuthority := *authority
			viewOnlyAuthority.inputs = missingSourceAuthority(repositoryPaths)
			authority = &viewOnlyAuthority
		}
	}
	feedbackPath := runDir + "/onboarding-feedback.md"
	if err := ensureFeedbackTemplate(data, feedbackPath); err != nil {
		return err
	}
	data.FeedbackPath = feedbackPath
	if authority != nil {
		freshnessResult := authority.freshness
		data.Freshness = &freshnessResult
		data.CapturedRevision = authority.repository.Head
		bindOperationalRevision(data.Operations, data.CapturedRevision)
		if err := bindTaskInvestigationAuthority(data.TaskInvestigation, authority.repository); err != nil {
			return err
		}
		data.CapturedInputCount = len(authority.inputs)
		data.RepositorySubmodules = append([]freshness.SubmoduleState(nil), authority.repository.Submodules...)
		catalog, available, catalogErr := authorizedExactSearchCatalog(data, *authority)
		// Source-backed actions are optional capabilities. A manifest may still
		// bind a coherent view-only report when its captured scope cannot form a
		// regular-file catalog; reportserver will reconstruct the same failure
		// and withhold source IDs and local analysis.
		if catalogErr == nil && available {
			if err := AttachExactWorkspaceSearch(data, catalog); err != nil {
				return err
			}
		}
	}

	jsonPath := runDir + "/report.json"
	if err := WriteReportJSON(data, jsonPath); err != nil {
		return err
	}
	var reportJSON []byte
	if authority != nil {
		reportJSON, err = os.ReadFile(jsonPath)
		if err != nil {
			return fmt.Errorf("read generated report json: %w", err)
		}
	}

	htmlPath := runDir + "/report.html"
	if sourceEpisodeJSON == nil {
		if err := WriteReportHTML(data, htmlPath); err != nil {
			return err
		}
	} else {
		html, err := RenderHTMLWithSourceEpisode(data, sourceEpisodeJSON)
		if err != nil {
			return err
		}
		if err := os.WriteFile(htmlPath, html, 0o644); err != nil {
			return err
		}
	}
	if authority != nil {
		if err := writeAuthorizedRunManifest(runDir, data, reportJSON, *authority); err != nil {
			return err
		}
	}
	return nil
}

func retainSourceEpisodeRegularOpenablePaths(data *ReportData, authority RunAuthority) error {
	if data == nil || authority.inputs == nil {
		return nil
	}
	analysisRelative, err := filepath.Rel(authority.repository.Identity, authority.analysisRoot)
	if err != nil || analysisRelative == ".." ||
		strings.HasPrefix(analysisRelative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("report: source episode analysis root is outside repository")
	}
	analysisPrefix := ""
	if analysisRelative != "." {
		analysisPrefix = filepath.ToSlash(analysisRelative)
	}
	capturedByPath := make(map[string]freshness.CapturedInput, len(authority.inputs))
	for _, input := range authority.inputs {
		if _, duplicate := capturedByPath[input.Path]; duplicate {
			return fmt.Errorf("report: source episode captured authority has duplicate paths")
		}
		capturedByPath[input.Path] = input
	}
	retained := make([]string, 0, min(len(data.OpenablePaths), maxManifestOpenablePaths))
	for _, sourcePath := range data.OpenablePaths {
		if err := validateManifestPath(sourcePath); err != nil {
			return err
		}
		repositoryPath := sourcePath
		if analysisPrefix != "" {
			repositoryPath = path.Join(analysisPrefix, sourcePath)
		}
		input, ok := capturedByPath[repositoryPath]
		if ok && input.Kind == freshness.FileSymlink {
			continue
		}
		retained = append(retained, sourcePath)
	}
	data.OpenablePaths = retained
	return nil
}

func sourcePathNeedsLiteralGitMode(sourcePath string) bool {
	return strings.HasPrefix(sourcePath, ":") || strings.ContainsAny(sourcePath, "*?[")
}

func sourcePathsNeedLiteralGitMode(paths []string) bool {
	for _, sourcePath := range paths {
		if sourcePathNeedsLiteralGitMode(sourcePath) {
			return true
		}
	}
	return false
}

func sourceAuthorityNeedsLiteralGitMode(inputs []freshness.CapturedInput) bool {
	for _, input := range inputs {
		if sourcePathNeedsLiteralGitMode(input.Path) {
			return true
		}
	}
	return false
}

func missingSourceAuthority(paths []string) []freshness.CapturedInput {
	inputs := make([]freshness.CapturedInput, 0, len(paths))
	for _, sourcePath := range paths {
		id := sha256.Sum256([]byte("captured-input-v1\x00" + sourcePath))
		inputs = append(inputs, freshness.CapturedInput{
			Version: freshness.CapturedInputVersion,
			ID:      fmt.Sprintf("%x", id),
			Path:    sourcePath,
			Kind:    freshness.FileMissing,
			Stages:  []string{"report_evidence"},
		})
	}
	return inputs
}

func ensureFeedbackTemplate(data *ReportData, path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create onboarding feedback template: %w", err)
	}
	defer file.Close()

	content := fmt.Sprintf(`# repomap onboarding feedback

Repository: %s
Direction followed:
First useful file:
Time to useful orientation:

## Correct

-

## Missing

-

## Misleading

-
`, data.RepoName)
	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("write onboarding feedback template: %w", err)
	}
	return nil
}
