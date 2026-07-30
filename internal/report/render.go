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
	"slices"
	"sort"
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
	persisted := reportDataForPersistence(data)
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
	if err := data.GitLabSourceLinks.validate(); err != nil {
		return nil, err
	}
	rendered := reportDataForRendering(data)
	css := styleCSS
	js := scriptJS
	if episode == nil {
		css = withoutSourceEpisodeAssetBlocks(css)
		js = withoutSourceEpisodeAssetBlocks(js)
	}
	var payload any
	if episode == nil {
		payload = rendered
	} else {
		payload = struct {
			*ReportData
			SourceEpisode *sourceEpisodeProjection `json:"source_episode"`
		}{
			ReportData:    rendered,
			SourceEpisode: episode,
		}
	}
	dataJSON, err := marshalHTMLPayload(payload, rendered.GitLabSourceLinks != nil)
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
		"Language":              normalizedReportLanguage(data.ReportLanguage),
		"CSS":                   template.CSS(css),
		"HasArchitectureCanvas": data.ArchitectureCanvas != nil,
		"ArchitectureCanvasCSS": template.CSS(architectureCanvasCSS),
		"ELKJS":                 template.JS(elkJSBundledJS),
		"ArchitectureCanvasJS":  template.JS(architectureCanvasJS),
		"HasDiscoveredSurfaces": data.DiscoveredSurfaces != nil,
		"SurfaceCatalogCSS":     template.CSS(surfaceCatalogCSS),
		"SurfaceCatalogJS":      template.JS(surfaceCatalogJS),
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
	rendered.SemanticSearch = nil
	rendered.SemanticSearchDisabled = false
	return &rendered
}

func reportDataForPersistence(data *ReportData) *ReportData {
	rendered := *data
	rendered.SemanticSearch = nil
	rendered.SemanticSearchDisabled = false
	// Static source routing belongs only to the generated standalone HTML.
	// Canonical report.json and its manifest binding remain host-neutral.
	rendered.GitLabSourceLinks = nil
	return &rendered
}

func Generate(runDir string) error {
	return generate(runDir, nil, nil, "")
}

// GenerateAuthorized renders a report and binds its exact generated JSON to
// repository authority confirmed stable across orientation.
func GenerateAuthorized(runDir string, authority RunAuthority) error {
	return generate(runDir, &authority, nil, "")
}

// GenerateAuthorizedGitLab emits the ordinary persisted report and manifest
// plus one standalone HTML report whose source actions target the exact
// captured revision on the supplied GitLab project.
func GenerateAuthorizedGitLab(
	runDir string,
	authority RunAuthority,
	repositoryURL string,
) error {
	normalizedURL, err := NormalizeGitLabRepositoryURL(repositoryURL)
	if err != nil {
		return err
	}
	if normalizedURL == "" {
		return fmt.Errorf("report: GitLab repository URL is required")
	}
	if err := validateGitLabAuthority(authority); err != nil {
		return err
	}
	return generate(runDir, &authority, nil, normalizedURL)
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
	return generate(runDir, &authority, episodeJSON, "")
}

func generate(
	runDir string,
	authority *RunAuthority,
	sourceEpisodeJSON []byte,
	gitLabRepositoryURL string,
) error {
	if gitLabRepositoryURL != "" && sourceEpisodeJSON != nil {
		return fmt.Errorf("report: standalone GitLab reports cannot embed a source episode")
	}
	if gitLabRepositoryURL != "" {
		if authority == nil {
			return fmt.Errorf("report: standalone GitLab report requires confirmed repository authority")
		}
		if err := validateGitLabAuthority(*authority); err != nil {
			return err
		}
	}
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
	var gitLabSourceLinks *GitLabSourceLinks
	if authority != nil {
		freshnessResult := authority.freshness
		data.Freshness = &freshnessResult
		data.CapturedRevision = authority.repository.Head
		if gitLabRepositoryURL != "" {
			pathPrefix, err := gitLabSourcePathPrefix(authority.repository.Identity, authority.analysisRoot)
			if err != nil {
				return err
			}
			gitLabSourceLinks, err = newGitLabSourceLinks(
				gitLabRepositoryURL,
				data.CapturedRevision,
				pathPrefix,
			)
			if err != nil {
				return err
			}
			gitLabSourceLinks.WorkingTreeDirty = len(authority.repository.Dirty) != 0
			gitLabSourceLinks.WorkingTreePaths = gitLabWorkingTreePaths(
				pathPrefix,
				authority.repository.Dirty,
				data.OpenablePaths,
			)
			data.standaloneLocalRoots = []string{
				data.ArtifactsDir,
				authority.analysisRoot,
				authority.repository.Identity,
			}
		}
		bindOperationalRevision(data.Operations, data.CapturedRevision)
		if err := bindTaskInvestigationAuthority(data.TaskInvestigation, authority.repository); err != nil {
			return err
		}
		data.CapturedInputCount = len(authority.inputs)
		data.RepositorySubmodules = append([]freshness.SubmoduleState(nil), authority.repository.Submodules...)
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
	data.GitLabSourceLinks = gitLabSourceLinks
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

func gitLabSourcePathPrefix(repositoryRoot, analysisRoot string) (string, error) {
	relative, err := filepath.Rel(repositoryRoot, analysisRoot)
	if err != nil || filepath.IsAbs(relative) || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("report: GitLab source analysis root is outside repository")
	}
	if relative == "." {
		return "", nil
	}
	prefix := filepath.ToSlash(relative)
	if err := validateManifestPath(prefix); err != nil {
		return "", fmt.Errorf("report: GitLab source analysis path is invalid")
	}
	return prefix, nil
}

func gitLabWorkingTreePaths(
	pathPrefix string,
	dirty []freshness.DirtyFile,
	openablePaths []string,
) []string {
	dirtyPaths := make(map[string]struct{}, len(dirty)*2)
	for _, file := range dirty {
		dirtyPaths[file.Path] = struct{}{}
		if file.FromPath != "" {
			dirtyPaths[file.FromPath] = struct{}{}
		}
	}
	result := make([]string, 0, min(len(openablePaths), len(dirtyPaths)))
	for _, openablePath := range openablePaths {
		repositoryPath := openablePath
		if pathPrefix != "" {
			repositoryPath = path.Join(pathPrefix, openablePath)
		}
		if _, exists := dirtyPaths[repositoryPath]; exists {
			result = append(result, openablePath)
		}
	}
	sort.Strings(result)
	return slices.Compact(result)
}

func marshalHTMLPayload(payload any, standalone bool) ([]byte, error) {
	var localRoots []string
	if reportData, ok := payload.(*ReportData); ok && reportData != nil {
		localRoots = reportData.standaloneLocalRoots
	}
	data, err := json.Marshal(payload)
	if err != nil || !standalone {
		return data, err
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, fmt.Errorf("report: decode standalone HTML projection: %w", err)
	}
	stripStandaloneSourceContent(decoded)
	stripStandaloneLocalPaths(decoded, localRoots)
	data, err = json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("report: encode standalone HTML projection: %w", err)
	}
	return data, nil
}

func stripStandaloneSourceContent(value any) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			stripStandaloneSourceContent(item)
		}
	case map[string]any:
		// These fields are useful only to the local debug/server experience and
		// would either leak a workstation path or retain exact source bytes.
		delete(typed, "model_research")
		delete(typed, "artifacts_dir")
		delete(typed, "feedback_path")
		delete(typed, "source_ids")
		delete(typed, "source_context_ids")
		if _, hasPath := typed["path"]; hasPath {
			// SourceSignal snippets are exact scanner excerpts. They are useful
			// in the localhost report, but the standalone artifact keeps only
			// their repository location and explanation.
			delete(typed, "snippet")
			if _, isSnippet := typed["content"]; isSnippet {
				delete(typed, "content")
				delete(typed, "lines")
				delete(typed, "full_function_lines")
				delete(typed, "full_function_start_line")
				delete(typed, "full_function_end_line")
				delete(typed, "content_sha256")
				delete(typed, "presentation_sha256")
			}
		}
		if _, codeBearing := typed["code_bearing"]; codeBearing {
			delete(typed, "lines")
		}
		for _, child := range typed {
			stripStandaloneSourceContent(child)
		}
	}
}

func stripStandaloneLocalPaths(value any, roots []string) {
	normalized := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		root = filepath.Clean(root)
		if !filepath.IsAbs(root) || root == string(filepath.Separator) {
			continue
		}
		if _, duplicate := seen[root]; duplicate {
			continue
		}
		seen[root] = struct{}{}
		normalized = append(normalized, root)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return len(normalized[i]) > len(normalized[j])
	})
	if len(normalized) == 0 {
		return
	}

	var scrub func(any)
	scrub = func(current any) {
		switch typed := current.(type) {
		case []any:
			for index, child := range typed {
				if text, ok := child.(string); ok {
					for _, root := range normalized {
						text = strings.ReplaceAll(text, root, "[local path]")
					}
					typed[index] = text
					continue
				}
				scrub(child)
			}
		case map[string]any:
			for key, child := range typed {
				if text, ok := child.(string); ok {
					for _, root := range normalized {
						text = strings.ReplaceAll(text, root, "[local path]")
					}
					typed[key] = text
					continue
				}
				scrub(child)
			}
		}
	}
	scrub(value)
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
