package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	_ "embed"
)

//go:embed templates/report_app.css
var reportAppCSS string

//go:embed templates/report_app.js
var reportAppJS string

//go:embed templates/report_loader.js
var reportLoaderJS string

// The System canvas is split into deterministic browser modules so graph
// projection, interaction policy, geometry, and DOM/SVG rendering can evolve
// without turning report_app.js back into their shared mutable owner.
//
//go:embed templates/system_canvas_graph.js
var systemCanvasGraphJS string

//go:embed templates/system_canvas_interaction.js
var systemCanvasInteractionJS string

//go:embed templates/system_canvas_geometry.js
var systemCanvasGeometryJS string

//go:embed templates/system_canvas_renderer.js
var systemCanvasRendererJS string

//go:embed templates/program_report.html
var programTemplateHTML string

func encodeReportJSON(data *ReportData, maxBytes int) ([]byte, error) {
	if data == nil {
		return nil, fmt.Errorf("report: data is required")
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("report: positive artifact byte limit is required")
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
	if len(b) > maxBytes {
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
	if data == nil {
		return nil, fmt.Errorf("report: data is required")
	}
	if err := validateProgramPresentation(data); err != nil {
		return nil, err
	}
	if err := validateTargetNavigation(data, options.TargetNavigation); err != nil {
		return nil, err
	}
	if data.ProgramPortfolio == nil {
		return nil, fmt.Errorf("report: HTML publication requires a complete program portfolio")
	}
	rendered, err := buildHTMLWithOptions(data, options)
	if err != nil {
		return nil, err
	}
	if len(rendered) > MaxOrdinaryReportHTMLBytes {
		return nil, &ReportResourceLimitError{
			LimitBytes: MaxOrdinaryReportHTMLBytes, ActualBytes: len(rendered),
		}
	}
	return rendered, nil
}

func buildHTMLWithOptions(data *ReportData, options RenderOptions) ([]byte, error) {
	if err := data.GitLabSourceLinks.validate(); err != nil {
		return nil, err
	}
	if err := data.GitHubSourceLinks.validate(); err != nil {
		return nil, err
	}
	if data.GitLabSourceLinks != nil && data.GitHubSourceLinks != nil {
		return nil, fmt.Errorf("report: multiple external source hosts are not allowed")
	}
	if (data.GitLabSourceLinks != nil && data.GitLabSourceLinks.Revision != data.CapturedRevision) ||
		(data.GitHubSourceLinks != nil && data.GitHubSourceLinks.Revision != data.CapturedRevision) {
		return nil, fmt.Errorf("report: external source revision does not match captured report authority")
	}
	if err := validateBrowserSourceIDs(data); err != nil {
		return nil, err
	}
	if data.ProgramPortfolio == nil {
		return nil, fmt.Errorf("report: HTML publication requires a complete program portfolio")
	}
	return buildProgramHTMLWithOptions(data, options)
}

func buildProgramHTMLWithOptions(data *ReportData, options RenderOptions) ([]byte, error) {
	if data.ProgramPortfolio == nil {
		return nil, fmt.Errorf("report: program shell requires a complete program portfolio")
	}
	transport, err := buildOrdinaryBrowserTransportV4(
		data, options.TargetNavigation, renderPayloadLocalRoots(data, options.LocalRoots),
	)
	if err != nil {
		return nil, err
	}
	section, err := standaloneBundleTransportHTMLSectionV4(transport)
	if err != nil {
		return nil, err
	}
	return executeProgramReport(programReportTemplateData{
		Title:            data.RepoName,
		BrowserTransport: template.HTML(section),
	})
}

type programReportTemplateData struct {
	Title            string
	BrowserTransport template.HTML
	// StandaloneTargetBundle is a construction alias until the standalone
	// writer switches its skeleton call to BrowserTransport.
	StandaloneTargetBundle template.HTML
}

func executeProgramReport(data programReportTemplateData) ([]byte, error) {
	programReportTmpl, err := template.New("program-report").Parse(programTemplateHTML)
	if err != nil {
		return nil, fmt.Errorf("report: parse embedded program template: %w", err)
	}
	browserTransport := data.BrowserTransport
	if browserTransport == "" {
		browserTransport = data.StandaloneTargetBundle
	}
	if browserTransport == "" {
		return nil, fmt.Errorf("report: browser transport section is required")
	}
	var buffer bytes.Buffer
	err = programReportTmpl.Execute(&buffer, map[string]any{
		"Title":                     data.Title,
		"ReportAppCSS":              template.CSS(reportAppCSS),
		"ReportLoaderJS":            template.JS(reportLoaderJS),
		"SystemCanvasGraphJS":       template.JS(systemCanvasGraphJS),
		"SystemCanvasInteractionJS": template.JS(systemCanvasInteractionJS),
		"SystemCanvasGeometryJS":    template.JS(systemCanvasGeometryJS),
		"SystemCanvasRendererJS":    template.JS(systemCanvasRendererJS),
		"ReportAppJS":               template.JS(reportAppJS),
		"BrowserTransport":          browserTransport,
	})
	if err != nil {
		return nil, fmt.Errorf("report: render program shell: %w", err)
	}
	return buffer.Bytes(), nil
}

func buildOrdinaryBrowserTransportV4(
	data *ReportData,
	navigation *TargetNavigationPortfolio,
	localRoots []string,
) (standaloneBundleTransportV4, error) {
	repository, err := ProjectBrowserRepositoryPayload(data, navigation)
	if err != nil {
		return standaloneBundleTransportV4{}, err
	}
	target, err := ProjectBrowserTargetPayload(data)
	if err != nil {
		return standaloneBundleTransportV4{}, err
	}
	repositoryRaw, err := encodeBrowserRepositoryPayloadForHTML(repository, localRoots)
	if err != nil {
		return standaloneBundleTransportV4{}, err
	}
	targetRaw, err := encodeBrowserTargetPayloadForHTML(target, localRoots)
	if err != nil {
		return standaloneBundleTransportV4{}, err
	}
	selectedTargetID := ""
	for _, row := range repository.Targets {
		if row.State != "analyzed" || row.ProgramTargetID != target.Target.ID {
			continue
		}
		if selectedTargetID != "" {
			return standaloneBundleTransportV4{}, fmt.Errorf("report: current browser target binding is ambiguous")
		}
		selectedTargetID = row.SelectedTargetID
	}
	if selectedTargetID == "" {
		return standaloneBundleTransportV4{}, fmt.Errorf("report: current browser target is absent from repository index")
	}
	return prepareStandaloneBundleTransportV4(standaloneBundleTransportInputV4{
		RepositoryPayload:      repositoryRaw,
		LogicalDefaultTargetID: repository.LogicalDefaultSelectedTargetID,
		Targets: []standaloneBundleTransportTargetInputV4{{
			TargetID: selectedTargetID, ProgramTargetID: target.Target.ID,
			State: standaloneBundleTransportTargetAnalyzed, Payload: targetRaw,
		}},
	})
}

func encodeBrowserRepositoryPayloadForHTML(
	payload BrowserRepositoryPayload,
	localRoots []string,
) ([]byte, error) {
	raw, err := EncodeBrowserRepositoryPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("report: encode repository browser projection: %w", err)
	}
	restored, err := DecodeBrowserRepositoryPayload(raw)
	if err != nil {
		return nil, fmt.Errorf("report: round-trip repository browser projection: %w", err)
	}
	for index := range restored.Warnings {
		restored.Warnings[index] = scrubBrowserLocalPaths(restored.Warnings[index], localRoots)
	}
	if restored.Runtime != nil {
		for index := range restored.Runtime.Roles {
			role := &restored.Runtime.Roles[index]
			role.Name = scrubBrowserLocalPaths(role.Name, localRoots)
			role.Purpose = scrubBrowserLocalPaths(role.Purpose, localRoots)
			for implementationIndex := range role.Implementations {
				implementation := &role.Implementations[implementationIndex]
				implementation.Mode = scrubBrowserLocalPaths(implementation.Mode, localRoots)
			}
		}
		for index := range restored.Runtime.UnclassifiedTargets {
			target := &restored.Runtime.UnclassifiedTargets[index]
			target.Reason = scrubBrowserLocalPaths(target.Reason, localRoots)
		}
	}
	if browserValueContainsLocalPath(restored, localRoots) {
		return nil, fmt.Errorf("report: repository browser projection retained a local path")
	}
	return EncodeBrowserRepositoryPayload(restored)
}

func encodeBrowserTargetPayloadForHTML(
	payload BrowserTargetPayload,
	localRoots []string,
) ([]byte, error) {
	if browserValueContainsLocalPath(payload, localRoots) {
		return nil, fmt.Errorf("report: target browser projection retained a local path")
	}
	raw, err := EncodeBrowserTargetPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("report: encode target browser projection: %w", err)
	}
	restored, err := DecodeBrowserTargetPayload(raw)
	if err != nil {
		return nil, fmt.Errorf("report: round-trip target browser projection: %w", err)
	}
	return EncodeBrowserTargetPayload(restored)
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
	if data.CapturedInputCount < 0 {
		return fmt.Errorf("report: captured input count cannot be negative")
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
	if err := validateProgramSemanticPresentation(
		data.ProgramPortfolio, data.AnalysisTarget, data.CubeMapView, data.CoreMapView,
		data.ActivityEntrypointView, data.IntegrationUsageView, data.ActivityPathView,
		jstsSemanticPresentation{data.JSTSSurfaceCatalogView, data.CrossSurfacePathView},
	); err != nil {
		return err
	}
	if data.CubeMapView != nil {
		if data.AnalysisTarget == nil {
			return fmt.Errorf("report: cube map view requires an exact analysis target")
		}
		if err := data.AnalysisTarget.Validate(); err != nil {
			return fmt.Errorf("report: cube map analysis target: %w", err)
		}
		if err := data.CubeMapView.Validate(); err != nil {
			return fmt.Errorf("report: cube map view: %w", err)
		}
		if data.CubeMapView.Target.Ref != data.AnalysisTarget.Ref {
			return fmt.Errorf("report: cube map view target does not match analysis target")
		}
		if data.CubeMapView.ProgramTargetID != defaultEntry.Target.ID {
			return fmt.Errorf("report: cube map view does not bind the default program target")
		}
		if err := validateCubeMapProgramTarget(*data.AnalysisTarget, defaultEntry.Target); err != nil {
			return fmt.Errorf("report: %w", err)
		}
	}
	if data.CoreMapView != nil {
		if err := data.CoreMapView.Validate(); err != nil {
			return fmt.Errorf("report: core map view: %w", err)
		}
		if data.CoreMapView.ProgramTargetID != defaultEntry.Target.ID ||
			data.CoreMapView.ProgramIndexSHA256 != defaultEntry.View.IndexSHA256 {
			return fmt.Errorf("report: core map view does not bind the default program target and index")
		}
	}
	if data.ActivityEntrypointView != nil {
		if err := data.ActivityEntrypointView.Validate(); err != nil {
			return fmt.Errorf("report: activity entrypoint view: %w", err)
		}
		if data.ActivityEntrypointView.ProgramTargetID != defaultEntry.Target.ID ||
			data.ActivityEntrypointView.ProgramIndexSHA256 != defaultEntry.View.IndexSHA256 {
			return fmt.Errorf("report: activity entrypoint view does not bind the default program target and index")
		}
	}
	if data.IntegrationUsageView != nil {
		if err := data.IntegrationUsageView.Validate(); err != nil {
			return fmt.Errorf("report: integration usage view: %w", err)
		}
		if data.IntegrationUsageView.ProgramTargetID != defaultEntry.Target.ID ||
			data.IntegrationUsageView.ProgramIndexSHA256 != defaultEntry.View.IndexSHA256 {
			return fmt.Errorf("report: integration usage view does not bind the default program target and index")
		}
	}
	if data.ActivityPathView != nil {
		if err := data.ActivityPathView.Validate(); err != nil {
			return fmt.Errorf("report: activity path view: %w", err)
		}
		if data.ActivityPathView.ProgramTargetID != defaultEntry.Target.ID ||
			data.ActivityPathView.ProgramIndexSHA256 != defaultEntry.View.IndexSHA256 {
			return fmt.Errorf("report: activity path view does not bind the default program target and index")
		}
		if err := data.ActivityPathView.ValidateReportJoins(
			data.ActivityEntrypointView, data.IntegrationUsageView,
		); err != nil {
			return fmt.Errorf("report: activity path report joins: %w", err)
		}
	}
	if data.JSTSSurfaceCatalogView != nil {
		if err := data.JSTSSurfaceCatalogView.Validate(); err != nil {
			return fmt.Errorf("report: JavaScript/TypeScript surface catalog view: %w", err)
		}
		if data.JSTSSurfaceCatalogView.ProgramTargetID != defaultEntry.Target.ID ||
			data.JSTSSurfaceCatalogView.ProgramIndexSHA256 != defaultEntry.View.IndexSHA256 {
			return fmt.Errorf("report: JavaScript/TypeScript surface catalog does not bind the default program target and index")
		}
	}
	if data.CrossSurfacePathView != nil {
		if err := data.CrossSurfacePathView.Validate(); err != nil {
			return fmt.Errorf("report: cross-surface path view: %w", err)
		}
		if data.CrossSurfacePathView.ProgramTargetID != defaultEntry.Target.ID ||
			data.CrossSurfacePathView.ProgramIndexSHA256 != defaultEntry.View.IndexSHA256 {
			return fmt.Errorf("report: cross-surface paths do not bind the default program target and index")
		}
		if data.JSTSSurfaceCatalogView == nil ||
			data.CrossSurfacePathView.JSTSProjectSHA256 != data.JSTSSurfaceCatalogView.JSTSProjectSHA256 {
			return fmt.Errorf("report: cross-surface paths do not bind the exact JavaScript/TypeScript surface authority")
		}
		if err := data.CrossSurfacePathView.ValidateSurfaceJoins(data.JSTSSurfaceCatalogView); err != nil {
			return fmt.Errorf("report: cross-surface path report joins: %w", err)
		}
	}
	if data.RuntimePortfolio != nil {
		if err := data.RuntimePortfolio.Validate(); err != nil {
			return fmt.Errorf("report: runtime portfolio view: %w", err)
		}
	}
	if data.TargetOutcomePortfolio != nil {
		if err := data.TargetOutcomePortfolio.Validate(); err != nil {
			return fmt.Errorf("report: target outcome portfolio view: %w", err)
		}
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
	return generate(runDir, authority, nil, options, true)
}

// GenerateAuthorizedPageData finalizes one manifest-bound backing page
// without publishing a browser HTML artifact. Multi-target publication uses
// these exact report.json/manifest authorities to build its sole owner HTML.
func GenerateAuthorizedPageData(runDir string, authority RunAuthority) error {
	return generate(runDir, authority, nil, RenderOptions{}, false)
}

// GenerateAuthorizedPageDataVerified finalizes one backing page and returns
// its compact current-transaction receipt. The receipt is created only after
// the already verified report and manifest have been atomically installed.
func GenerateAuthorizedPageDataVerified(
	runDir string,
	authority RunAuthority,
) (VerifiedRunReceipt, error) {
	return generateVerified(runDir, authority, nil, RenderOptions{}, false)
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
	return generate(runDir, authority, &standaloneSourceConfig{
		hostName:      "GitLab",
		repositoryURL: normalizedURL,
	}, options, true)
}

// GenerateAuthorizedGitLabPageData is the backing-page equivalent of
// GenerateAuthorizedGitLab. It retains exact static source authority in the
// manifest while deliberately publishing no target-local HTML.
func GenerateAuthorizedGitLabPageData(
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
	return generate(runDir, authority, &standaloneSourceConfig{
		hostName:      "GitLab",
		repositoryURL: normalizedURL,
	}, RenderOptions{}, false)
}

// GenerateAuthorizedGitLabPageDataVerified is the transaction-receipt
// equivalent of GenerateAuthorizedGitLabPageData.
func GenerateAuthorizedGitLabPageDataVerified(
	runDir string,
	authority RunAuthority,
	repositoryURL string,
) (VerifiedRunReceipt, error) {
	normalizedURL, err := NormalizeGitLabRepositoryURL(repositoryURL)
	if err != nil {
		return VerifiedRunReceipt{}, err
	}
	if normalizedURL == "" {
		return VerifiedRunReceipt{}, fmt.Errorf("report: GitLab repository URL is required")
	}
	if err := validateGitLabAuthority(authority); err != nil {
		return VerifiedRunReceipt{}, err
	}
	return generateVerified(runDir, authority, &standaloneSourceConfig{
		hostName:      "GitLab",
		repositoryURL: normalizedURL,
	}, RenderOptions{}, false)
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
	return generate(runDir, authority, &standaloneSourceConfig{
		hostName:      "GitHub",
		repositoryURL: normalizedURL,
	}, options, true)
}

// GenerateAuthorizedGitHubPageData is the backing-page equivalent of
// GenerateAuthorizedGitHub. It retains exact static source authority in the
// manifest while deliberately publishing no target-local HTML.
func GenerateAuthorizedGitHubPageData(
	runDir string,
	authority RunAuthority,
	repositoryURL string,
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
	return generate(runDir, authority, &standaloneSourceConfig{
		hostName:      "GitHub",
		repositoryURL: normalizedURL,
	}, RenderOptions{}, false)
}

// GenerateAuthorizedGitHubPageDataVerified is the transaction-receipt
// equivalent of GenerateAuthorizedGitHubPageData.
func GenerateAuthorizedGitHubPageDataVerified(
	runDir string,
	authority RunAuthority,
	repositoryURL string,
) (VerifiedRunReceipt, error) {
	normalizedURL, err := NormalizeGitHubRepositoryURL(repositoryURL)
	if err != nil {
		return VerifiedRunReceipt{}, err
	}
	if normalizedURL == "" {
		return VerifiedRunReceipt{}, fmt.Errorf("report: GitHub repository URL is required")
	}
	if err := validateStandaloneSourceAuthority(authority, "GitHub"); err != nil {
		return VerifiedRunReceipt{}, err
	}
	return generateVerified(runDir, authority, &standaloneSourceConfig{
		hostName:      "GitHub",
		repositoryURL: normalizedURL,
	}, RenderOptions{}, false)
}

func generate(
	runDir string,
	authority RunAuthority,
	standaloneSource *standaloneSourceConfig,
	renderOptions RenderOptions,
	publishHTML bool,
) error {
	return generateWithReceipt(
		runDir, authority, standaloneSource, renderOptions, publishHTML, nil,
	)
}

func generateVerified(
	runDir string,
	authority RunAuthority,
	standaloneSource *standaloneSourceConfig,
	renderOptions RenderOptions,
	publishHTML bool,
) (VerifiedRunReceipt, error) {
	var receipt VerifiedRunReceipt
	err := generateWithReceipt(
		runDir, authority, standaloneSource, renderOptions, publishHTML, &receipt,
	)
	return receipt, err
}

func generateWithReceipt(
	runDir string,
	authority RunAuthority,
	standaloneSource *standaloneSourceConfig,
	renderOptions RenderOptions,
	publishHTML bool,
	receiptOut *VerifiedRunReceipt,
) error {
	if standaloneSource != nil {
		if err := validateStandaloneSourceAuthority(authority, standaloneSource.hostName); err != nil {
			return err
		}
	}
	if err := authority.validate(); err != nil {
		return err
	}
	// report.json and report.html are not publication authority without the
	// manifest, but they still look like a finished product when opened
	// directly. Invalidate all final names before any regeneration and install
	// the manifest last only after the complete replacement has validated.
	if err := removePublishedReportArtifacts(runDir); err != nil {
		return err
	}
	data, err := readRunDir(runDir)
	if err != nil {
		return err
	}
	var gitLabSourceLinks *GitLabSourceLinks
	var gitHubSourceLinks *GitHubSourceLinks
	data.CapturedRevision = authority.repository.Head
	if standaloneSource != nil && publishHTML {
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
		case "GitHub":
			gitHubSourceLinks, err = newGitHubSourceLinks(
				standaloneSource.repositoryURL,
				data.CapturedRevision,
				pathPrefix,
			)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("report: unsupported external source host %q", standaloneSource.hostName)
		}
		data.standaloneLocalRoots = []string{
			data.ArtifactsDir,
			authority.analysisRoot,
			authority.repository.Identity,
		}
	}
	data.CapturedInputCount = len(authority.inputs)

	reportJSON, err := encodeReportJSON(data, maxManifestReportBytes)
	if err != nil {
		return err
	}
	manifest, err := prepareAuthorizedRunManifest(
		runDir, data, reportJSON, authority, standaloneSource,
	)
	if err != nil {
		return err
	}
	var preparedReceipt VerifiedRunReceipt
	if receiptOut != nil {
		preparedReceipt, err = newVerifiedRunReceiptFromReportData(runDir, manifest, data)
		if err != nil {
			return err
		}
	}
	if !publishHTML {
		if err := installAuthorizedReport(runDir, reportJSON, nil, manifest); err != nil {
			return err
		}
		retainVerifiedRunReceipt(receiptOut, preparedReceipt)
		return nil
	}
	renderData := *data
	renderData.GitLabSourceLinks = gitLabSourceLinks
	renderData.GitHubSourceLinks = gitHubSourceLinks
	reportHTML, err := RenderHTMLWithOptions(&renderData, renderOptions)
	if err != nil {
		return err
	}
	if err := VerifyOrdinaryReportHTMLPayload(
		reportHTML,
		reportJSON,
		OrdinaryReportHTMLAuthority{
			TargetNavigation: renderOptions.TargetNavigation,
			StandaloneSource: manifest.StandaloneSource,
			ArtifactsDir:     data.ArtifactsDir,
			AnalysisRoot:     manifest.AnalysisRoot,
			RepositoryRoot:   manifest.RepositoryState.Identity,
		},
	); err != nil {
		return fmt.Errorf("report: verify generated html before publication: %w", err)
	}
	if err := installAuthorizedReport(runDir, reportJSON, reportHTML, manifest); err != nil {
		return err
	}
	retainVerifiedRunReceipt(receiptOut, preparedReceipt)
	return nil
}

func retainVerifiedRunReceipt(
	receiptOut *VerifiedRunReceipt,
	receipt VerifiedRunReceipt,
) {
	if receiptOut == nil {
		return
	}
	*receiptOut = receipt
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
) (resultErr error) {
	jsonStage, err := stageReportArtifact(runDir, ".report-json-*.tmp", reportJSON)
	if err != nil {
		return err
	}
	htmlStage := ""
	installed := false
	defer func() {
		cleanupErr := errors.Join(removeIfPresent(jsonStage), removeIfPresent(htmlStage))
		if !installed {
			cleanupErr = errors.Join(cleanupErr, removePublishedReportArtifacts(runDir))
		}
		resultErr = errors.Join(resultErr, cleanupErr)
	}()

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
		htmlPath := filepath.Join(runDir, "report.html")
		if err := os.Rename(htmlStage, htmlPath); err != nil {
			return fmt.Errorf("report: install report.html: %w", err)
		}
		htmlStage = ""
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
	for _, name := range []string{RunManifestFilename, "report.json", "report.html"} {
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
