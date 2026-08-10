package report

import (
	"bytes"
	"context"
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

	_ "embed"
	"github.com/dvordrova/repomap/internal/freshness"
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

//go:embed templates/ui_messages.js
var uiMessagesJS string

var reportTmpl *template.Template

// MaxSourceEpisodeBytes is the maximum approved source-episode artifact that
// the transient report renderer will inspect.
const MaxSourceEpisodeBytes = maxSourceEpisodeBytes

func init() {
	reportTmpl = template.Must(template.New("report").Parse(templateHTML))
}

func WriteReportJSON(data *ReportData, path string) error {
	return writeReportJSON(data, path, maxManifestReportBytes)
}

func writeReportJSON(data *ReportData, path string, maxBytes int) error {
	if data == nil {
		return fmt.Errorf("report: data is required")
	}
	if maxBytes <= 0 {
		return fmt.Errorf("report: positive artifact byte limit is required")
	}
	if err := ensureArchitectureComponentNavigation(data); err != nil {
		return err
	}
	if err := ensureArchitectureAssociations(data); err != nil {
		return err
	}
	if err := ensureEntrypointHandoffGroups(data); err != nil {
		return err
	}
	if err := validateLibraryAPIProjection(data.AnalysisTarget, data.LibraryAPI, data.OpenablePaths); err != nil {
		return err
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
	b = append(b, '\n')
	if len(b) > maxBytes {
		return &ReportResourceLimitError{
			LimitBytes:  maxBytes,
			ActualBytes: len(b),
		}
	}
	return os.WriteFile(path, b, 0o644)
}

// ReportResourceLimitError is a terminal report-publication resource outcome.
// It deliberately exposes only byte counts, never report or source content.
type ReportResourceLimitError struct {
	LimitBytes  int
	ActualBytes int
}

func (err *ReportResourceLimitError) Error() string {
	if err == nil {
		return "report: resource limit exceeded"
	}
	return fmt.Sprintf("report: exact artifact requires %d bytes; limit is %d bytes",
		err.ActualBytes, err.LimitBytes)
}

func WriteReportHTML(data *ReportData, path string) error {
	return WriteReportHTMLWithOptions(data, path, RenderOptions{})
}

// WriteReportHTMLWithOptions writes one canonical target page with optional
// render-only sibling navigation. The options are never persisted to
// report.json.
func WriteReportHTMLWithOptions(data *ReportData, path string, options RenderOptions) error {
	html, err := RenderHTMLWithOptions(data, options)
	if err != nil {
		return err
	}
	return os.WriteFile(path, html, 0o644)
}

// RenderHTML renders report data with repomap's embedded trusted template and
// assets. Servers should use this instead of executing a saved HTML artifact.
func RenderHTML(data *ReportData) ([]byte, error) {
	return RenderHTMLWithOptions(data, RenderOptions{})
}

// RenderHTMLWithOptions renders one ReportData target page plus optional
// caller-authorized presentation navigation to sibling target pages.
func RenderHTMLWithOptions(data *ReportData, options RenderOptions) ([]byte, error) {
	if data == nil {
		return nil, fmt.Errorf("report: data is required")
	}
	if err := validateTargetNavigation(data, options.TargetNavigation); err != nil {
		return nil, err
	}
	if err := ensureArchitectureComponentNavigation(data); err != nil {
		return nil, err
	}
	if err := ensureArchitectureAssociations(data); err != nil {
		return nil, err
	}
	if err := ensureEntrypointHandoffGroups(data); err != nil {
		return nil, err
	}
	return buildHTMLWithOptions(data, options)
}

// RenderHTMLWithSourceEpisode adds one approved, SHA-pinned source episode to
// the rendered Study surface. The projection exists only in this HTML
// response: it is not added to ReportData or persisted in report.json.
func RenderHTMLWithSourceEpisode(data *ReportData, episodeJSON []byte) ([]byte, error) {
	return RenderHTMLWithSourceEpisodeAndOptions(data, episodeJSON, RenderOptions{})
}

// RenderHTMLWithSourceEpisodeAndOptions is the source-episode equivalent of
// RenderHTMLWithOptions. Both inputs remain transient HTML presentation state.
func RenderHTMLWithSourceEpisodeAndOptions(
	data *ReportData,
	episodeJSON []byte,
	options RenderOptions,
) ([]byte, error) {
	if data == nil {
		return nil, fmt.Errorf("report: data is required")
	}
	if err := validateTargetNavigation(data, options.TargetNavigation); err != nil {
		return nil, err
	}
	if err := ensureArchitectureComponentNavigation(data); err != nil {
		return nil, err
	}
	if err := ensureArchitectureAssociations(data); err != nil {
		return nil, err
	}
	if err := ensureEntrypointHandoffGroups(data); err != nil {
		return nil, err
	}
	canonicalEpisode, err := projectApprovedSourceEpisode(data, episodeJSON)
	if err != nil {
		return nil, err
	}
	episode := canonicalEpisode
	if data.presentationSourceEpisode != nil {
		if !sameSourceEpisodePresentationShape(
			canonicalEpisode,
			data.presentationSourceEpisode,
		) {
			return nil, fmt.Errorf(
				"report: localized source episode does not match approved input",
			)
		}
		episode = data.presentationSourceEpisode
	}
	return buildHTMLWithSourceEpisode(data, episode, options)
}

func sameSourceEpisodePresentationShape(
	left,
	right *sourceEpisodeProjection,
) bool {
	if left == nil || right == nil ||
		left.EpisodeID != right.EpisodeID ||
		left.Repository != right.Repository ||
		left.Revision != right.Revision ||
		len(left.Claims) != len(right.Claims) ||
		len(left.Uncertainties) != len(right.Uncertainties) {
		return false
	}
	for index := range left.Claims {
		if left.Claims[index].ID != right.Claims[index].ID ||
			left.Claims[index].State != right.Claims[index].State ||
			left.Claims[index].Strength != right.Claims[index].Strength ||
			!sourceEpisodeSourcesEqual(
				left.Claims[index].Sources,
				right.Claims[index].Sources,
			) {
			return false
		}
	}
	for index := range left.Uncertainties {
		if left.Uncertainties[index].ID != right.Uncertainties[index].ID ||
			left.Uncertainties[index].State != right.Uncertainties[index].State ||
			!sourceEpisodeSourcesEqual(
				left.Uncertainties[index].Sources,
				right.Uncertainties[index].Sources,
			) {
			return false
		}
	}
	return true
}

func sourceEpisodeSourcesEqual(
	left,
	right []sourceEpisodeSource,
) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
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
	return buildHTMLWithOptions(data, RenderOptions{})
}

func buildHTMLWithOptions(data *ReportData, options RenderOptions) ([]byte, error) {
	return buildHTMLWithSourceEpisode(data, nil, options)
}

func buildHTMLWithSourceEpisode(
	data *ReportData,
	episode *sourceEpisodeProjection,
	options RenderOptions,
) ([]byte, error) {
	if err := data.GitLabSourceLinks.validate(); err != nil {
		return nil, err
	}
	if err := data.GitHubSourceLinks.validate(); err != nil {
		return nil, err
	}
	if data.GitLabSourceLinks != nil && data.GitHubSourceLinks != nil {
		return nil, fmt.Errorf("report: multiple external source hosts are not allowed")
	}
	rendered := reportDataForRendering(data)
	css := styleCSS
	js := scriptJS
	if episode == nil {
		css = withoutSourceEpisodeAssetBlocks(css)
		js = withoutSourceEpisodeAssetBlocks(js)
	}
	payload := struct {
		*ReportData
		ArchitectureDebugPresentation map[string]string          `json:"architecture_debug_presentation,omitempty"`
		SourceEpisode                 *sourceEpisodeProjection   `json:"source_episode,omitempty"`
		TargetNavigation              *TargetNavigationPortfolio `json:"target_navigation,omitempty"`
	}{
		ReportData:                    rendered,
		ArchitectureDebugPresentation: rendered.architectureDebugPresentation,
		SourceEpisode:                 episode,
		TargetNavigation:              options.TargetNavigation,
	}
	dataJSON, err := marshalHTMLPayload(
		payload,
		rendered.GitLabSourceLinks != nil || rendered.GitHubSourceLinks != nil,
	)
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
		"LocalizationState":     data.presentationLocalizationState,
		"DataJSON":              template.JS(dataJSON),
		"UIMessagesJS":          template.JS(uiMessagesJS),
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
	rendered.PresentationWarningMessages = runPresentationWarnings(data)
	if data.TaskInvestigation != nil {
		workspace := *data.TaskInvestigation
		workspace.PresentationWarnings = taskInvestigationPresentationWarnings(
			workspace.warningDiagnostics,
		)
		rendered.TaskInvestigation = &workspace
	}
	return &rendered
}

func reportDataForPersistence(data *ReportData) *ReportData {
	rendered := *data
	// Canonical report JSON is always English. Requested locale and any
	// translated presentation live in separately validated render sidecars.
	rendered.ReportLanguage = ""
	rendered.SemanticSearch = nil
	rendered.SemanticSearchDisabled = false
	rendered.PresentationWarnings = nil
	rendered.PresentationWarningKinds = nil
	rendered.PresentationWarningMessages = nil
	// Static source routing belongs only to the generated standalone HTML.
	// Canonical report.json and its manifest binding remain host-neutral.
	rendered.GitLabSourceLinks = nil
	rendered.GitHubSourceLinks = nil
	if data.TaskInvestigation != nil {
		workspace := *data.TaskInvestigation
		workspace.PresentationWarnings = nil
		rendered.TaskInvestigation = &workspace
	}
	return &rendered
}

func Generate(runDir string) error {
	return GenerateWithOptions(runDir, RenderOptions{})
}

// GenerateWithOptions is the unauthenticated generation seam for transient
// presentation options. Existing callers retain the exact zero-options path.
func GenerateWithOptions(runDir string, options RenderOptions) error {
	return generate(runDir, nil, nil, nil, options)
}

// GenerateAuthorized renders a report and binds its exact generated JSON to
// repository authority confirmed stable across orientation.
func GenerateAuthorized(runDir string, authority RunAuthority) error {
	return GenerateAuthorizedWithOptions(runDir, authority, RenderOptions{})
}

// GenerateAuthorizedWithOptions preserves ordinary source and manifest
// authority while adding only transient target-page navigation to report.html.
func GenerateAuthorizedWithOptions(
	runDir string,
	authority RunAuthority,
	options RenderOptions,
) error {
	return generate(runDir, &authority, nil, nil, options)
}

type standaloneSourceConfig struct {
	hostName      string
	repositoryURL string
}

// GenerateAuthorizedGitLab emits the ordinary persisted report and manifest
// plus one standalone HTML report whose source actions target the exact
// captured revision on the supplied GitLab project.
func GenerateAuthorizedGitLab(
	runDir string,
	authority RunAuthority,
	repositoryURL string,
) error {
	return GenerateAuthorizedGitLabWithOptions(
		runDir, authority, repositoryURL, RenderOptions{},
	)
}

func GenerateAuthorizedGitLabWithOptions(
	runDir string,
	authority RunAuthority,
	repositoryURL string,
	options RenderOptions,
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
	return generate(runDir, &authority, nil, &standaloneSourceConfig{
		hostName:      "GitLab",
		repositoryURL: normalizedURL,
	}, options)
}

// GenerateAuthorizedGitHub emits the ordinary persisted report and manifest
// plus one standalone HTML report whose source actions target the exact
// captured revision on the supplied GitHub repository.
func GenerateAuthorizedGitHub(
	runDir string,
	authority RunAuthority,
	repositoryURL string,
) error {
	return GenerateAuthorizedGitHubWithOptions(
		runDir, authority, repositoryURL, RenderOptions{},
	)
}

func GenerateAuthorizedGitHubWithOptions(
	runDir string,
	authority RunAuthority,
	repositoryURL string,
	options RenderOptions,
) error {
	normalizedURL, err := NormalizeGitHubRepositoryURL(repositoryURL)
	if err != nil {
		return err
	}
	if normalizedURL == "" {
		return fmt.Errorf("report: GitHub repository URL is required")
	}
	if err := validateStandaloneSourceAuthority(authority, "GitHub"); err != nil {
		return err
	}
	return generate(runDir, &authority, nil, &standaloneSourceConfig{
		hostName:      "GitHub",
		repositoryURL: normalizedURL,
	}, options)
}

// GenerateAuthorizedWithSourceEpisode generates the same persisted report and
// manifest as GenerateAuthorized, while placing one approved, SHA-pinned
// source episode only in the static report.html review surface.
func GenerateAuthorizedWithSourceEpisode(
	runDir string,
	authority RunAuthority,
	episodeJSON []byte,
) error {
	return GenerateAuthorizedWithSourceEpisodeAndOptions(
		runDir, authority, episodeJSON, RenderOptions{},
	)
}

func GenerateAuthorizedWithSourceEpisodeAndOptions(
	runDir string,
	authority RunAuthority,
	episodeJSON []byte,
	options RenderOptions,
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
	return generate(runDir, &authority, episodeJSON, nil, options)
}

func generate(
	runDir string,
	authority *RunAuthority,
	sourceEpisodeJSON []byte,
	standaloneSource *standaloneSourceConfig,
	renderOptions RenderOptions,
) error {
	if standaloneSource != nil && sourceEpisodeJSON != nil {
		return fmt.Errorf("report: standalone external-source reports cannot embed a source episode")
	}
	if standaloneSource != nil {
		if authority == nil {
			return fmt.Errorf("report: standalone external-source report requires confirmed repository authority")
		}
		if err := validateStandaloneSourceAuthority(*authority, standaloneSource.hostName); err != nil {
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
	data, err := readRunDir(runDir, studyDocumentSourceRoot, authority, nil)
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
	var gitHubSourceLinks *GitHubSourceLinks
	if authority != nil {
		freshnessResult := authority.freshness
		data.Freshness = &freshnessResult
		data.CapturedRevision = authority.repository.Head
		if standaloneSource != nil {
			pathPrefix, err := standaloneSourcePathPrefix(authority.repository.Identity, authority.analysisRoot)
			if err != nil {
				return err
			}
			switch standaloneSource.hostName {
			case "GitLab":
				gitLabSourceLinks, err = newGitLabSourceLinks(
					standaloneSource.repositoryURL,
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
			case "GitHub":
				gitHubSourceLinks, err = newGitHubSourceLinks(
					standaloneSource.repositoryURL,
					data.CapturedRevision,
					pathPrefix,
				)
				if err != nil {
					return err
				}
				gitHubSourceLinks.WorkingTreeDirty = len(authority.repository.Dirty) != 0
				gitHubSourceLinks.WorkingTreePaths = gitLabWorkingTreePaths(
					pathPrefix,
					authority.repository.Dirty,
					data.OpenablePaths,
				)
			default:
				return fmt.Errorf("report: unsupported external source host %q", standaloneSource.hostName)
			}
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
	if sourceEpisodeJSON != nil {
		if err := AttachSourceEpisodePresentation(data, sourceEpisodeJSON); err != nil {
			return err
		}
	}
	if authority != nil {
		if err := PrepareAuthorizedSourceCoverage(context.Background(), data, authority); err != nil {
			return err
		}
		data.CapturedInputCount = len(authority.inputs)
		if data.RepositoryAtlas != nil {
			data.AtlasStudy, data.StudyMap, err = readAtlasStudyReportProduct(runDir, data)
			if err != nil {
				return err
			}
			applyCanonicalStudyPublication(data)
			prepareReplayedPresentationMetadata(data)
		}
	}

	jsonPath := runDir + "/report.json"
	if err := WriteReportJSON(data, jsonPath); err != nil {
		return err
	}
	canonicalData := data
	var reportJSON []byte
	if authority != nil {
		reportJSON, err = os.ReadFile(jsonPath)
		if err != nil {
			return fmt.Errorf("read generated report json: %w", err)
		}
	}

	htmlPath := runDir + "/report.html"
	preparedRenderData, _ := PrepareRunPresentation(
		runDir,
		canonicalData,
		sourceEpisodeJSON,
	)
	if preparedRenderData == nil {
		preparedRenderData = canonicalData
	}
	renderData, _ := LoadPresentationLocalization(
		runDir,
		preparedRenderData,
		canonicalData.requestedPresentationLocale,
	)
	renderData.GitLabSourceLinks = gitLabSourceLinks
	renderData.GitHubSourceLinks = gitHubSourceLinks
	if sourceEpisodeJSON == nil {
		if err := WriteReportHTMLWithOptions(renderData, htmlPath, renderOptions); err != nil {
			return err
		}
	} else {
		html, err := RenderHTMLWithSourceEpisodeAndOptions(
			renderData,
			sourceEpisodeJSON,
			renderOptions,
		)
		if err != nil {
			return err
		}
		if err := os.WriteFile(htmlPath, html, 0o644); err != nil {
			return err
		}
	}
	if authority != nil {
		if err := writeAuthorizedRunManifest(runDir, canonicalData, reportJSON, *authority); err != nil {
			return err
		}
	}
	return nil
}

func standaloneSourcePathPrefix(repositoryRoot, analysisRoot string) (string, error) {
	relative, err := filepath.Rel(repositoryRoot, analysisRoot)
	if err != nil || filepath.IsAbs(relative) || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("report: external source analysis root is outside repository")
	}
	if relative == "." {
		return "", nil
	}
	prefix := filepath.ToSlash(relative)
	if err := validateManifestPath(prefix); err != nil {
		return "", fmt.Errorf("report: external source analysis path is invalid")
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
