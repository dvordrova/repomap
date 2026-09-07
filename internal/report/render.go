package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"embed"
	"io/fs"
)

// The report is one static page, assembled from the files under templates/.
// Every .html file there is one region of the page; every .css and .js file
// is one layer, concatenated in filename order, which is why they carry a
// numeric prefix. Adding a region or a layer is a new file and no Go change,
// so working on the page does not mean working on this package.
//
//go:embed templates
var reportTemplateFS embed.FS

const reportPageEntryTemplate = "page.html"

func encodeReportJSON(data *ReportData, maxBytes int) ([]byte, error) {
	if data == nil {
		return nil, fmt.Errorf("report: data is required")
	}
	if err := validateProgramPresentation(data); err != nil {
		return nil, err
	}
	persisted := reportDataForPersistence(data)
	// SourceIDs are issued by the local report server after manifest
	// verification. They are session navigation IDs, not persistent evidence.
	persisted.SourceIDs = nil
	b, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return nil, err
	}
	b = append(b, '\n')
	if maxBytes > 0 && len(b) > maxBytes {
		return nil, &ReportResourceLimitError{
			LimitBytes:  maxBytes,
			ActualBytes: len(b),
		}
	}
	return b, nil
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

// RenderHTMLWithOptions renders one ReportData target page plus optional
// caller-authorized presentation navigation to sibling target pages.
func RenderHTMLWithOptions(data *ReportData, options RenderOptions) ([]byte, error) {
	rendered, _, err := renderHTMLWithOptionsDiagnostics(data, options)
	return rendered, err
}

func renderHTMLWithOptionsDiagnostics(
	data *ReportData,
	options RenderOptions,
) ([]byte, GenerationDiagnostics, error) {
	if data == nil {
		return nil, GenerationDiagnostics{}, fmt.Errorf("report: data is required")
	}
	if err := validateProgramPresentation(data); err != nil {
		return nil, GenerationDiagnostics{}, err
	}
	if err := validateTargetNavigation(data, options.TargetNavigation); err != nil {
		return nil, GenerationDiagnostics{}, err
	}
	if data.ProgramPortfolio == nil {
		return nil, GenerationDiagnostics{}, fmt.Errorf("report: HTML publication requires a complete program portfolio")
	}
	rendered, diagnostics, err := buildHTMLWithOptionsDiagnostics(data, options)
	if err != nil {
		return nil, diagnostics, err
	}
	return rendered, diagnostics, nil
}

func buildHTMLWithOptions(data *ReportData, options RenderOptions) ([]byte, error) {
	rendered, _, err := buildHTMLWithOptionsDiagnostics(data, options)
	return rendered, err
}

func buildHTMLWithOptionsDiagnostics(
	data *ReportData,
	options RenderOptions,
) ([]byte, GenerationDiagnostics, error) {
	if err := data.GitLabSourceLinks.validate(); err != nil {
		return nil, GenerationDiagnostics{}, err
	}
	if err := data.GitHubSourceLinks.validate(); err != nil {
		return nil, GenerationDiagnostics{}, err
	}
	if data.GitLabSourceLinks != nil && data.GitHubSourceLinks != nil {
		return nil, GenerationDiagnostics{}, fmt.Errorf("report: multiple external source hosts are not allowed")
	}
	if (data.GitLabSourceLinks != nil && data.GitLabSourceLinks.Revision != data.CapturedRevision) ||
		(data.GitHubSourceLinks != nil && data.GitHubSourceLinks.Revision != data.CapturedRevision) {
		return nil, GenerationDiagnostics{}, fmt.Errorf("report: external source revision does not match captured report authority")
	}
	if err := validateBrowserSourceIDs(data); err != nil {
		return nil, GenerationDiagnostics{}, err
	}
	if data.ProgramPortfolio == nil {
		return nil, GenerationDiagnostics{}, fmt.Errorf("report: HTML publication requires a complete program portfolio")
	}
	return buildProgramHTMLWithOptionsDiagnostics(data, options)
}

func buildProgramHTMLWithOptions(data *ReportData, options RenderOptions) ([]byte, error) {
	rendered, _, err := buildProgramHTMLWithOptionsDiagnostics(data, options)
	return rendered, err
}

func buildProgramHTMLWithOptionsDiagnostics(
	data *ReportData,
	options RenderOptions,
) ([]byte, GenerationDiagnostics, error) {
	return buildProgramHTMLWithOptionsDiagnosticsAndHooks(data, options)
}

func buildProgramHTMLWithOptionsDiagnosticsAndHooks(
	data *ReportData,
	options RenderOptions,
) ([]byte, GenerationDiagnostics, error) {
	if data.ProgramPortfolio == nil {
		return nil, GenerationDiagnostics{}, fmt.Errorf("report: program shell requires a complete program portfolio")
	}
	diagnostics := GenerationDiagnostics{}
	localRoots := renderPayloadLocalRoots(data, options.LocalRoots)
	rendered, err := executeProgramReport(data, options, localRoots)
	if err != nil {
		return nil, diagnostics, err
	}
	return rendered, diagnostics, nil
}

// executeProgramReport renders the one static page from the already validated
// report data. The page carries no analysis payload of its own: everything it
// shows is projected here, in Go, from the same artifacts report.json binds.
func executeProgramReport(data *ReportData, options RenderOptions, localRoots []string) ([]byte, error) {
	options.LocalRoots = localRoots
	page, err := PreparePage(data, options)
	if err != nil {
		return nil, err
	}
	if err := page.applyDisplay(options); err != nil {
		return nil, err
	}
	view := page.view
	vocabulary, err := uiVocabulary(view.Language)
	if err != nil {
		return nil, err
	}
	encodedVocabulary, err := json.Marshal(vocabulary)
	if err != nil {
		return nil, err
	}
	view.UIVocabularyJSON = template.JS(encodedVocabulary)
	styles, err := bundledTemplateAssets("templates/css")
	if err != nil {
		return nil, err
	}
	scripts, err := bundledTemplateAssets("templates/js")
	if err != nil {
		return nil, err
	}
	view.CSS = template.CSS(styles)
	view.JS = template.JS(scripts)
	pageTemplate, err := template.New("report").Funcs(template.FuncMap{"t": func(key string, params ...any) (string, error) { return uiText(view.Language, key, params...) }}).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		return nil, fmt.Errorf("report: parse embedded page templates: %w", err)
	}
	var buffer bytes.Buffer
	if err := pageTemplate.ExecuteTemplate(&buffer, reportPageEntryTemplate, view); err != nil {
		return nil, fmt.Errorf("report: render page: %w", err)
	}
	return buffer.Bytes(), nil
}

// bundledTemplateAssets concatenates one asset directory in filename order.
// That order is the cascade, so the numeric prefixes are the contract and
// nothing else decides which layer wins.
func bundledTemplateAssets(directory string) (string, error) {
	entries, err := fs.ReadDir(reportTemplateFS, directory)
	if err != nil {
		return "", fmt.Errorf("report: read %s: %w", directory, err)
	}
	var bundle strings.Builder
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := fs.ReadFile(reportTemplateFS, directory+"/"+entry.Name())
		if err != nil {
			return "", fmt.Errorf("report: read %s/%s: %w", directory, entry.Name(), err)
		}
		if bundle.Len() > 0 {
			bundle.WriteByte('\n')
		}
		bundle.Write(content)
	}
	return strings.TrimRight(bundle.String(), "\n"), nil
}

// browserValueContainsLocalPath walks the typed browser contract before JSON
// escaping can hide host separators or HTML-sensitive path characters. It is
// a persistence guard only; it neither rewrites fields nor supplies semantics.
func browserValueContainsLocalPath(value any, roots []string) bool {
	return browserReflectValueContainsLocalPath(reflect.ValueOf(value), roots)
}

func browserReflectValueContainsLocalPath(value reflect.Value, roots []string) bool {
	if !value.IsValid() {
		return false
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		return !value.IsNil() && browserReflectValueContainsLocalPath(value.Elem(), roots)
	case reflect.String:
		return browserTextContainsLocalPath(value.String(), roots)
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			if browserReflectValueContainsLocalPath(value.Field(index), roots) {
				return true
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if browserReflectValueContainsLocalPath(value.Index(index), roots) {
				return true
			}
		}
	case reflect.Map:
		iterator := value.MapRange()
		for iterator.Next() {
			if browserReflectValueContainsLocalPath(iterator.Key(), roots) ||
				browserReflectValueContainsLocalPath(iterator.Value(), roots) {
				return true
			}
		}
	}
	return false
}

func scrubBrowserLocalPaths(text string, roots []string) string {
	for _, root := range normalizedBrowserLocalRoots(roots) {
		for searchFrom := 0; searchFrom <= len(text)-len(root); {
			relative := strings.Index(text[searchFrom:], root)
			if relative < 0 {
				break
			}
			start := searchFrom + relative
			end := start + len(root)
			if browserLocalPathBoundary(text, start, end) {
				text = text[:start] + "[local path]" + text[end:]
				searchFrom = start + len("[local path]")
				continue
			}
			searchFrom = end
		}
	}
	return text
}

func browserTextContainsLocalPath(text string, roots []string) bool {
	for _, root := range normalizedBrowserLocalRoots(roots) {
		for searchFrom := 0; searchFrom <= len(text)-len(root); {
			relative := strings.Index(text[searchFrom:], root)
			if relative < 0 {
				break
			}
			start := searchFrom + relative
			end := start + len(root)
			if browserLocalPathBoundary(text, start, end) {
				return true
			}
			searchFrom = end
		}
	}
	return false
}

func normalizedBrowserLocalRoots(roots []string) []string {
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
	sort.Slice(normalized, func(i, j int) bool { return len(normalized[i]) > len(normalized[j]) })
	return normalized
}

func browserLocalPathBoundary(text string, start, end int) bool {
	if start > 0 && browserPathSegmentByte(text[start-1]) {
		return false
	}
	return end == len(text) || !browserPathSegmentByte(text[end])
}

func browserPathSegmentByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' || value == '_' || value == '-' || value == '.'
}

func reportDataForPersistence(data *ReportData) *ReportData {
	rendered := *data
	// Static source routing belongs only to the generated standalone HTML.
	// Canonical report.json remains host-neutral; the run manifest separately
	// binds the exact external host and repository URL used by that HTML.
	rendered.GitLabSourceLinks = nil
	rendered.GitHubSourceLinks = nil
	return &rendered
}

func validateProgramPresentation(data *ReportData) error {
	if data == nil {
		return fmt.Errorf("report: data is required")
	}
	if data.ProgramPortfolio == nil {
		return fmt.Errorf("report: publication requires a complete program portfolio")
	}
	if data.FormatVersion != CurrentFormatVersion {
		return fmt.Errorf("report: unsupported format version %d", data.FormatVersion)
	}
	if data.RepoName == "" || strings.TrimSpace(data.RepoName) != data.RepoName {
		return fmt.Errorf("report: repository name must be exact and non-empty")
	}
	if !validGitRevision(data.CapturedRevision) || data.CapturedRevision != strings.ToLower(data.CapturedRevision) {
		return fmt.Errorf("report: captured revision must be a canonical lowercase 40- or 64-character hex revision")
	}
	previousPath := ""
	for index, sourcePath := range data.OpenablePaths {
		if err := validateManifestPath(sourcePath); err != nil {
			return fmt.Errorf("report: openable path %d: %w", index, err)
		}
		if previousPath != "" && previousPath >= sourcePath {
			return fmt.Errorf("report: openable paths must be uniquely sorted")
		}
		previousPath = sourcePath
	}
	for index, warning := range data.Warnings {
		if strings.TrimSpace(warning) == "" {
			return fmt.Errorf("report: warning %d must be non-empty", index)
		}
	}
	defaultEntry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return fmt.Errorf("report: %w", err)
	}
	if data.GroupGraph == nil {
		return fmt.Errorf("report: publication requires the final group graph")
	}
	if err := validateSelectedGroupGraphBinding(
		data.GroupGraph, defaultEntry.Target, defaultEntry.View.IndexSHA256,
	); err != nil {
		return fmt.Errorf("report: group graph view: %w", err)
	}
	if data.TargetOutcomePortfolio == nil {
		return fmt.Errorf("report: publication requires the exhaustive target outcome portfolio")
	}
	if err := data.TargetOutcomePortfolio.Validate(); err != nil {
		return fmt.Errorf("report: target outcome portfolio view: %w", err)
	}
	return nil
}

func validateBrowserSourceIDs(data *ReportData) error {
	if data == nil || len(data.SourceIDs) == 0 {
		return nil
	}
	if data.GitHubSourceLinks != nil || data.GitLabSourceLinks != nil {
		return fmt.Errorf("report: static and served source authorities cannot be mixed")
	}
	if len(data.SourceIDs) != len(data.OpenablePaths) {
		return fmt.Errorf("report: served source authority does not cover every openable path")
	}
	openable := make(map[string]struct{}, len(data.OpenablePaths))
	for _, sourcePath := range data.OpenablePaths {
		openable[sourcePath] = struct{}{}
	}
	seenIDs := make(map[string]struct{}, len(data.SourceIDs))
	for sourcePath, sourceID := range data.SourceIDs {
		if _, allowed := openable[sourcePath]; !allowed || !validBrowserSourceID(sourceID) {
			return fmt.Errorf("report: served source authority is invalid")
		}
		if _, duplicate := seenIDs[sourceID]; duplicate {
			return fmt.Errorf("report: served source authority contains a duplicate source ID")
		}
		seenIDs[sourceID] = struct{}{}
	}
	return nil
}

func validBrowserSourceID(value string) bool {
	if len(value) != 43 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

// GenerateAuthorized renders a report and binds its exact generated JSON to
// repository authority confirmed stable across orientation.
type standaloneSourceConfig struct {
	hostName      string
	repositoryURL string
}

// GenerateOptions is how a report is to be generated: linked to a source host
// or not, rendered with sibling navigation or not, published as HTML or kept
// as page data for a multi-target page to publish.
type GenerateOptions struct {
	// Data is the current process's result. Nil restores saved stage artifacts.
	Data        *ReportData
	GitHubURL   string
	GitLabURL   string
	Render      RenderOptions
	PublishHTML bool
}

// Generate writes report.json, the manifest and, when asked, report.html
// into the run directory, and returns the run as later stages refer to it.
// Twenty named variants used to do this — authorized, verified, with
// diagnostics, for GitLab, for GitHub, page data only — each a thin wrapper
// over the one below. This is that one.
func Generate(runDir string, source RunSource, options GenerateOptions) (RunReceipt, error) {
	var standalone *standaloneSourceConfig
	switch {
	case options.GitLabURL != "" && options.GitHubURL != "":
		return RunReceipt{}, fmt.Errorf("report: multiple external source hosts are not allowed")
	case options.GitLabURL != "":
		normalized, err := NormalizeGitLabRepositoryURL(options.GitLabURL)
		if err != nil {
			return RunReceipt{}, err
		}
		if normalized == "" {
			return RunReceipt{}, fmt.Errorf("report: GitLab repository URL is required")
		}
		standalone = &standaloneSourceConfig{hostName: "GitLab", repositoryURL: normalized}
	case options.GitHubURL != "":
		normalized, err := NormalizeGitHubRepositoryURL(options.GitHubURL)
		if err != nil {
			return RunReceipt{}, err
		}
		if normalized == "" {
			return RunReceipt{}, fmt.Errorf("report: GitHub repository URL is required")
		}
		standalone = &standaloneSourceConfig{hostName: "GitHub", repositoryURL: normalized}
	}
	return generate(runDir, source, standalone, options.Render, options.PublishHTML, options.Data)
}

func generate(
	runDir string,
	source RunSource,
	standaloneSource *standaloneSourceConfig,
	renderOptions RenderOptions,
	publishHTML bool,
	data *ReportData,
) (RunReceipt, error) {
	if err := source.validate(); err != nil {
		return RunReceipt{}, err
	}
	// report.json and report.html look like a finished product when opened
	// directly, so every final name is removed before regeneration and the
	// manifest is installed last, after the complete replacement is written.
	if err := removePublishedReportArtifacts(runDir); err != nil {
		return RunReceipt{}, err
	}
	var err error
	if data == nil {
		data, err = readRunDir(runDir)
		if err != nil {
			return RunReceipt{}, err
		}
	} else {
		copy := *data
		data = &copy
		absDir, err := filepath.Abs(runDir)
		if err != nil {
			return RunReceipt{}, err
		}
		if data.ArtifactsDir != absDir {
			return RunReceipt{}, fmt.Errorf("report: in-memory data belongs to another run")
		}
	}
	if len(source.GroupGraph) > 0 {
		if err := BindGroupGraphView(data, source.GroupGraph); err != nil {
			return RunReceipt{}, fmt.Errorf("report: bind group graph: %w", err)
		}
	}
	if err := collectOpenablePaths(data); err != nil {
		return RunReceipt{}, err
	}
	var gitLabSourceLinks *GitLabSourceLinks
	var gitHubSourceLinks *GitHubSourceLinks
	data.CapturedRevision = source.Repository.Head
	if standaloneSource != nil && publishHTML {
		pathPrefix, err := standaloneSourcePathPrefix(source.Repository.Identity, source.AnalysisRoot)
		if err != nil {
			return RunReceipt{}, err
		}
		switch standaloneSource.hostName {
		case "GitLab":
			gitLabSourceLinks, err = newGitLabSourceLinks(
				standaloneSource.repositoryURL,
				data.CapturedRevision,
				pathPrefix,
			)
			if err != nil {
				return RunReceipt{}, err
			}
		case "GitHub":
			gitHubSourceLinks, err = newGitHubSourceLinks(
				standaloneSource.repositoryURL,
				data.CapturedRevision,
				pathPrefix,
			)
			if err != nil {
				return RunReceipt{}, err
			}
		default:
			return RunReceipt{}, fmt.Errorf("report: unsupported external source host %q", standaloneSource.hostName)
		}
		data.standaloneLocalRoots = []string{
			data.ArtifactsDir,
			source.AnalysisRoot,
			source.Repository.Identity,
		}
	}

	reportJSON, err := encodeReportJSON(data, 0)
	if err != nil {
		return RunReceipt{}, err
	}
	// The final canonical report JSON exists at this boundary. Preserve its
	// advisory measurement even when manifest preparation or any later
	// publication step fails.
	manifest, err := prepareRunManifest(data, source, standaloneSource)
	if err != nil {
		return RunReceipt{}, err
	}
	// The module display name may end in a language version suffix (chi/v5).
	// Name the file after the selected repository, independently of its modules.
	display, translationsJSON, err := prepareDisplayPublication(filepath.Base(source.Repository.Identity), renderOptions)
	if err != nil {
		return RunReceipt{}, err
	}
	manifest.Display = display
	receipt, err := newRunReceipt(runDir, manifest, data)
	if err != nil {
		return RunReceipt{}, err
	}
	receipt.renderOptions = renderOptions
	if !publishHTML {
		if display != nil {
			return RunReceipt{}, fmt.Errorf("report: translated publication requires HTML")
		}
		if err := installAuthorizedReport(runDir, reportJSON, nil, manifest, nil); err != nil {
			return RunReceipt{}, err
		}
		return receipt, nil
	}
	renderData := *data
	renderData.GitLabSourceLinks = gitLabSourceLinks
	renderData.GitHubSourceLinks = gitHubSourceLinks
	// The page is stamped with the digest of the exact report.json bytes it
	// was rendered from, so publication can prove the pair belongs together.
	digest := sha256.Sum256(reportJSON)
	renderOptions.ReportSHA256 = hex.EncodeToString(digest[:])
	reportHTML, err := RenderHTMLWithOptions(&renderData, renderOptions)
	if err != nil {
		return RunReceipt{}, err
	}
	if err := installAuthorizedReport(runDir, reportJSON, reportHTML, manifest, translationsJSON); err != nil {
		return RunReceipt{}, err
	}
	return receipt, nil
}

// installAuthorizedReport stages the canonical report data and, when non-nil,
// its browser artifact. It writes the already-validated manifest last as the
// sole readiness boundary. A nil HTML payload is a backing page, not an empty
// report. Any returned error removes every final product name.
func installAuthorizedReport(
	runDir string,
	reportJSON []byte,
	reportHTML []byte,
	manifest RunManifest,
	translationsJSON []byte,
) (resultErr error) {
	jsonStage, err := stageReportArtifact(runDir, ".report-json-*.tmp", reportJSON)
	if err != nil {
		return err
	}
	htmlStage := ""
	translationsStage := ""
	installed := false
	defer func() {
		cleanupErr := errors.Join(removeIfPresent(jsonStage), removeIfPresent(htmlStage), removeIfPresent(translationsStage))
		if !installed {
			cleanupErr = errors.Join(cleanupErr, removePublishedReportArtifacts(runDir))
		}
		resultErr = errors.Join(resultErr, cleanupErr)
	}()
	if err := manifest.Display.validate(); err != nil {
		return err
	}
	if (manifest.Display == nil) != (translationsJSON == nil) {
		return fmt.Errorf("report: translated artifact and publication must be supplied together")
	}
	if translationsJSON != nil {
		translationsStage, err = stageReportArtifact(runDir, ".report-translations-*.tmp", translationsJSON)
		if err != nil {
			return err
		}
	}

	if reportHTML != nil {
		htmlStage, err = stageReportArtifact(runDir, ".report-html-*.tmp", reportHTML)
		if err != nil {
			return err
		}
	}
	if reportHTML == nil {
		if err := removeIfPresent(filepath.Join(runDir, "report.html")); err != nil {
			return fmt.Errorf("report: remove target-local report.html: %w", err)
		}
	}
	jsonPath := filepath.Join(runDir, "report.json")
	if err := os.Rename(jsonStage, jsonPath); err != nil {
		return fmt.Errorf("report: install report.json: %w", err)
	}
	jsonStage = ""
	if reportHTML != nil {
		htmlFilename := "report.html"
		if manifest.Display != nil {
			htmlFilename = manifest.Display.HTMLFilename
		}
		htmlPath := filepath.Join(runDir, htmlFilename)
		if err := os.Rename(htmlStage, htmlPath); err != nil {
			return fmt.Errorf("report: install HTML: %w", err)
		}
		htmlStage = ""
	}
	if translationsStage != "" {
		if err := os.Rename(translationsStage, filepath.Join(runDir, manifest.Display.TranslationsFilename)); err != nil {
			return fmt.Errorf("report: install translations: %w", err)
		}
		translationsStage = ""
	}
	if err := writeRunManifestAtomic(runDir, manifest); err != nil {
		return err
	}
	installed = true
	return nil
}

func stageReportArtifact(runDir string, pattern string, data []byte) (string, error) {
	file, err := os.CreateTemp(runDir, pattern)
	if err != nil {
		return "", fmt.Errorf("report: create staged artifact: %w", err)
	}
	name := file.Name()
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(name)
		}
	}()
	if err := file.Chmod(0o644); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("report: set staged artifact permissions: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("report: write staged artifact: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("report: sync staged artifact: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("report: close staged artifact: %w", err)
	}
	remove = false
	return name, nil
}

func removePublishedReportArtifacts(runDir string) error {
	var result error
	names := []string{RunManifestFilename, "report.json", "report.html"}
	entries, err := os.ReadDir(runDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "report.") && strings.HasSuffix(name, ".html") ||
			strings.HasPrefix(name, "report-translations.") && strings.HasSuffix(name, ".json") {
			names = append(names, name)
		}
	}
	for _, name := range names {
		if err := removeIfPresent(filepath.Join(runDir, name)); err != nil {
			result = errors.Join(result, fmt.Errorf("report: remove incomplete %s: %w", name, err))
		}
	}
	return result
}

func removeIfPresent(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
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

func marshalHTMLPayloadWithLocalRoots(
	payload any,
	localRoots []string,
) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, fmt.Errorf("report: decode browser projection: %w", err)
	}
	scrubRenderLocalPaths(decoded, localRoots)
	data, err = json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("report: encode browser projection: %w", err)
	}
	return data, nil
}

func renderPayloadLocalRoots(data *ReportData, extra []string) []string {
	roots := append([]string(nil), extra...)
	if data == nil {
		return roots
	}
	roots = append(roots, data.standaloneLocalRoots...)
	if data.ArtifactsDir != "" {
		roots = append(roots, data.ArtifactsDir)
	}
	return roots
}

func scrubRenderLocalPaths(value any, roots []string) {
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

// GenerationDiagnostics is what report generation has to say besides its
// result. It used to carry advisory scale warnings; it carries nothing now
// and stays only so the generation functions keep their shape.
type GenerationDiagnostics struct{}

// decodeStrictReportJSON reads report.json exactly as it was written: no
// unknown fields, one value, the format this code renders.
func decodeStrictReportJSON(reportJSON []byte) (ReportData, error) {
	decoder := json.NewDecoder(bytes.NewReader(reportJSON))
	decoder.DisallowUnknownFields()
	var data ReportData
	if err := decoder.Decode(&data); err != nil {
		return ReportData{}, fmt.Errorf("report: decode report.json: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return ReportData{}, fmt.Errorf("report: report.json contains multiple values")
		}
		return ReportData{}, fmt.Errorf("report: report.json has trailing data: %w", err)
	}
	if data.FormatVersion != CurrentFormatVersion {
		return ReportData{}, fmt.Errorf("report: unsupported report format version %d", data.FormatVersion)
	}
	return data, nil
}
