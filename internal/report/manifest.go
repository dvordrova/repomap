package report

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/llmbundle"
	"github.com/dvordrova/repomap/internal/navigator"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/sourcecatalog"
	"github.com/dvordrova/repomap/internal/tasklens"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

const (
	CurrentRunManifestVersion = 11
	RunManifestFilename       = "run_manifest.json"

	maxRunManifestBytes             = 4 * 1024 * 1024
	maxManifestReportBytes          = 32 * 1024 * 1024
	maxManifestOpenablePaths        = 4096
	maxManifestComponents           = 512
	maxManifestAnchors              = 4096
	maxManifestAnchorsPerComponent  = 256
	maxManifestLinesPerAnchor       = 128
	maxManifestRelatedFlows         = 256
	maxManifestIdentifierBytes      = 256
	maxManifestPathBytes            = 4096
	maxManifestRepositoryDirtyFiles = 20_000
)

// RunManifest is the local authority boundary for interactive report actions.
// It commits to the exact report bytes and repository state used to derive the
// bounded component and anchor choices below.
type RunManifest struct {
	Version               int                       `json:"version"`
	RepositoryState       freshness.RepositoryState `json:"repository_state"`
	AnalysisRoot          string                    `json:"analysis_root"`
	RepositoryStateSHA256 string                    `json:"repository_state_sha256"`
	ReportSHA256          string                    `json:"report_sha256"`
	ReportFormatVersion   int                       `json:"report_format_version"`
	OpenablePaths         []string                  `json:"openable_paths"`
	Components            []ComponentAuthority      `json:"components,omitempty"`
	CapturedInputs        []freshness.CapturedInput `json:"captured_inputs,omitempty"`
	CapturedInputsSHA256  string                    `json:"captured_inputs_sha256,omitempty"`
	Freshness             freshness.FreshnessResult `json:"freshness"`
	MaterialInputs        MaterialInputs            `json:"material_inputs"`
}

type MaterialInputs struct {
	SelectedRevision                  string `json:"selected_revision"`
	ModelBundleSHA256                 string `json:"model_bundle_sha256,omitempty"`
	OrientationContextSelectionSHA256 string `json:"orientation_context_selection_sha256,omitempty"`
	RepositoryAtlasSHA256             string `json:"repository_atlas_sha256,omitempty"`
	NavigatorRequestSHA256            string `json:"navigator_request_sha256,omitempty"`
	NavigatorResultSHA256             string `json:"navigator_result_sha256,omitempty"`
	NavigatorStatusSHA256             string `json:"navigator_status_sha256,omitempty"`
	AtlasStudyRequestSHA256           string `json:"atlas_study_request_sha256,omitempty"`
	AtlasStudyResultSHA256            string `json:"atlas_study_result_sha256,omitempty"`
	AtlasStudyStatusSHA256            string `json:"atlas_study_status_sha256,omitempty"`
	TaskBundleSHA256                  string `json:"task_bundle_sha256,omitempty"`
	TaskAttemptSHA256                 string `json:"task_attempt_sha256,omitempty"`
	TaskPackSHA256                    string `json:"task_pack_sha256,omitempty"`
	TaskStatusSHA256                  string `json:"task_status_sha256,omitempty"`
	TaskRetrievalTraceSHA256          string `json:"task_retrieval_trace_sha256,omitempty"`
	TaskRetrievalTraceMarkdownSHA256  string `json:"task_retrieval_trace_markdown_sha256,omitempty"`
	InputPolicyVersion                string `json:"input_policy_version"`
	ArchitectureContract              int    `json:"architecture_contract"`
	ReportContract                    int    `json:"report_contract"`
}

// RunAuthority is a repository state that was captured before repository
// analysis and confirmed unchanged after the analysis completed. Its fields
// are intentionally private so callers cannot manufacture authority without
// going through ConfirmRunAuthority.
type RunAuthority struct {
	analysisRoot string
	repository   freshness.RepositoryState
	inputs       []freshness.CapturedInput
	freshness    freshness.FreshnessResult
	confirmed    bool
}

func (authority RunAuthority) Freshness() freshness.FreshnessResult {
	return authority.freshness
}

// ComponentAuthority names the flows and repository anchors that a component
// may use as the starting point for a focused local investigation.
type ComponentAuthority struct {
	ID             string            `json:"id"`
	RelatedFlowIDs []string          `json:"related_flow_ids,omitempty"`
	Anchors        []AnchorAuthority `json:"anchors,omitempty"`
}

// AnchorAuthority is deliberately narrower than the presentation anchor. Only
// exact model-grounded lines are retained; prose and nearby source context are
// not authorization inputs.
type AnchorAuthority struct {
	ID             string `json:"id"`
	Path           string `json:"path"`
	AllowedLines   []int  `json:"allowed_lines,omitempty"`
	CanListSymbols bool   `json:"can_list_symbols"`
}

// Validate checks that a manifest is bounded, canonical, and internally
// consistent before it is used as an authority source.
func (m RunManifest) Validate() error {
	if m.Version != CurrentRunManifestVersion {
		return fmt.Errorf("report manifest: unsupported version %d", m.Version)
	}
	if err := m.RepositoryState.Validate(); err != nil {
		return fmt.Errorf("report manifest: repository state: %w", err)
	}
	if len(m.RepositoryState.Dirty) > maxManifestRepositoryDirtyFiles {
		return fmt.Errorf("report manifest: repository state has more than %d dirty files", maxManifestRepositoryDirtyFiles)
	}
	if err := validateAnalysisRoot(m.RepositoryState.Identity, m.AnalysisRoot); err != nil {
		return fmt.Errorf("report manifest: analysis root: %w", err)
	}
	repositoryDigest, err := m.RepositoryState.Digest()
	if err != nil {
		return fmt.Errorf("report manifest: repository digest: %w", err)
	}
	if !validManifestSHA256(m.RepositoryStateSHA256) || m.RepositoryStateSHA256 != repositoryDigest {
		return fmt.Errorf("report manifest: repository state sha256 mismatch")
	}
	if !validManifestSHA256(m.ReportSHA256) {
		return fmt.Errorf("report manifest: report sha256 is invalid")
	}
	if m.ReportFormatVersion <= 0 || m.ReportFormatVersion > CurrentFormatVersion {
		return fmt.Errorf("report manifest: unsupported report format version %d", m.ReportFormatVersion)
	}
	if len(m.OpenablePaths) > maxManifestOpenablePaths {
		return fmt.Errorf("report manifest: more than %d openable paths", maxManifestOpenablePaths)
	}
	inputsDigest, err := freshness.CapturedInputsDigest(m.CapturedInputs)
	if err != nil {
		return fmt.Errorf("report manifest: captured inputs: %w", err)
	}
	if !validManifestSHA256(m.CapturedInputsSHA256) || inputsDigest != m.CapturedInputsSHA256 {
		return fmt.Errorf("report manifest: captured inputs sha256 mismatch")
	}
	if err := m.Freshness.Validate(); err != nil {
		return fmt.Errorf("report manifest: freshness: %w", err)
	}
	if m.MaterialInputs.SelectedRevision != m.RepositoryState.Head ||
		!validManifestLabel(m.MaterialInputs.InputPolicyVersion) ||
		m.MaterialInputs.ArchitectureContract <= 0 || m.MaterialInputs.ReportContract != m.ReportFormatVersion {
		return fmt.Errorf("report manifest: material inputs are invalid")
	}
	hasModelBundle := m.MaterialInputs.ModelBundleSHA256 != ""
	hasOrientationSelection := m.MaterialInputs.OrientationContextSelectionSHA256 != ""
	if hasModelBundle != hasOrientationSelection {
		return fmt.Errorf("report manifest: Orientation model bundle and context selection identity must both be present or absent")
	}
	if hasModelBundle && !validManifestSHA256(m.MaterialInputs.ModelBundleSHA256) {
		return fmt.Errorf("report manifest: model bundle sha256 is invalid")
	}
	if hasOrientationSelection && !validManifestSHA256(m.MaterialInputs.OrientationContextSelectionSHA256) {
		return fmt.Errorf("report manifest: orientation context selection sha256 is invalid")
	}
	if m.MaterialInputs.RepositoryAtlasSHA256 != "" &&
		!validManifestSHA256(m.MaterialInputs.RepositoryAtlasSHA256) {
		return fmt.Errorf("report manifest: repository Atlas sha256 is invalid")
	}
	navigatorDigests := []string{
		m.MaterialInputs.NavigatorRequestSHA256,
		m.MaterialInputs.NavigatorResultSHA256,
		m.MaterialInputs.NavigatorStatusSHA256,
	}
	navigatorDigestCount := 0
	for _, digest := range navigatorDigests {
		if digest == "" {
			continue
		}
		navigatorDigestCount++
		if !validManifestSHA256(digest) {
			return fmt.Errorf("report manifest: Navigator artifact sha256 is invalid")
		}
	}
	if navigatorDigestCount != 0 && m.MaterialInputs.NavigatorStatusSHA256 == "" {
		return fmt.Errorf("report manifest: Navigator artifact identity requires status")
	}
	if navigatorDigestCount != 0 && m.MaterialInputs.RepositoryAtlasSHA256 == "" {
		return fmt.Errorf("report manifest: Navigator artifacts require repository Atlas authority")
	}
	atlasStudyDigests := []string{
		m.MaterialInputs.AtlasStudyRequestSHA256,
		m.MaterialInputs.AtlasStudyResultSHA256,
		m.MaterialInputs.AtlasStudyStatusSHA256,
	}
	atlasStudyDigestCount := 0
	for _, digest := range atlasStudyDigests {
		if digest == "" {
			continue
		}
		atlasStudyDigestCount++
		if !validManifestSHA256(digest) {
			return fmt.Errorf("report manifest: Atlas Study artifact sha256 is invalid")
		}
	}
	if atlasStudyDigestCount != 0 &&
		(m.MaterialInputs.AtlasStudyRequestSHA256 == "" ||
			m.MaterialInputs.AtlasStudyStatusSHA256 == "") {
		return fmt.Errorf("report manifest: Atlas Study artifact identity requires request and status")
	}
	if atlasStudyDigestCount != 0 && m.MaterialInputs.RepositoryAtlasSHA256 == "" {
		return fmt.Errorf("report manifest: Atlas Study artifacts require repository Atlas authority")
	}
	taskDigests := []string{
		m.MaterialInputs.TaskBundleSHA256,
		m.MaterialInputs.TaskAttemptSHA256,
		m.MaterialInputs.TaskPackSHA256,
		m.MaterialInputs.TaskStatusSHA256,
		m.MaterialInputs.TaskRetrievalTraceSHA256,
		m.MaterialInputs.TaskRetrievalTraceMarkdownSHA256,
	}
	taskDigestCount := 0
	for _, digest := range taskDigests {
		if digest != "" {
			taskDigestCount++
			if !validManifestSHA256(digest) {
				return fmt.Errorf("report manifest: Task Lens artifact sha256 is invalid")
			}
		}
	}
	if taskDigestCount != 0 && taskDigestCount != len(taskDigests) {
		return fmt.Errorf("report manifest: Task Lens artifact identity is incomplete")
	}
	openable := make(map[string]struct{}, len(m.OpenablePaths))
	previousPath := ""
	for index, value := range m.OpenablePaths {
		if err := validateManifestPath(value); err != nil {
			return fmt.Errorf("report manifest: openable path %d: %w", index, err)
		}
		if previousPath != "" && value <= previousPath {
			return fmt.Errorf("report manifest: openable paths must be uniquely sorted")
		}
		previousPath = value
		openable[value] = struct{}{}
	}
	if len(m.Components) > maxManifestComponents {
		return fmt.Errorf("report manifest: more than %d components", maxManifestComponents)
	}
	componentIDs := make(map[string]struct{}, len(m.Components))
	anchorIDs := make(map[string]struct{})
	totalAnchors := 0
	for componentIndex, component := range m.Components {
		if err := validateManifestIdentifier(component.ID); err != nil {
			return fmt.Errorf("report manifest: component %d id: %w", componentIndex, err)
		}
		if _, duplicate := componentIDs[component.ID]; duplicate {
			return fmt.Errorf("report manifest: duplicate component id %q", component.ID)
		}
		componentIDs[component.ID] = struct{}{}
		if len(component.RelatedFlowIDs) > maxManifestRelatedFlows {
			return fmt.Errorf("report manifest: component %q has more than %d related flows", component.ID, maxManifestRelatedFlows)
		}
		flowIDs := make(map[string]struct{}, len(component.RelatedFlowIDs))
		for flowIndex, flowID := range component.RelatedFlowIDs {
			if err := validateManifestIdentifier(flowID); err != nil {
				return fmt.Errorf("report manifest: component %q related flow %d: %w", component.ID, flowIndex, err)
			}
			if _, duplicate := flowIDs[flowID]; duplicate {
				return fmt.Errorf("report manifest: component %q has duplicate related flow id %q", component.ID, flowID)
			}
			flowIDs[flowID] = struct{}{}
		}
		if len(component.Anchors) > maxManifestAnchorsPerComponent {
			return fmt.Errorf("report manifest: component %q has more than %d anchors", component.ID, maxManifestAnchorsPerComponent)
		}
		totalAnchors += len(component.Anchors)
		if totalAnchors > maxManifestAnchors {
			return fmt.Errorf("report manifest: more than %d anchors", maxManifestAnchors)
		}
		componentPaths := make(map[string]struct{}, len(component.Anchors))
		for anchorIndex, anchor := range component.Anchors {
			if err := validateManifestIdentifier(anchor.ID); err != nil {
				return fmt.Errorf("report manifest: component %q anchor %d id: %w", component.ID, anchorIndex, err)
			}
			if _, duplicate := anchorIDs[anchor.ID]; duplicate {
				return fmt.Errorf("report manifest: duplicate anchor id %q", anchor.ID)
			}
			anchorIDs[anchor.ID] = struct{}{}
			if err := validateManifestPath(anchor.Path); err != nil {
				return fmt.Errorf("report manifest: anchor %q path: %w", anchor.ID, err)
			}
			if _, ok := openable[anchor.Path]; !ok {
				return fmt.Errorf("report manifest: anchor %q path is not openable", anchor.ID)
			}
			if _, duplicate := componentPaths[anchor.Path]; duplicate {
				return fmt.Errorf("report manifest: component %q has duplicate anchor path %q", component.ID, anchor.Path)
			}
			componentPaths[anchor.Path] = struct{}{}
			if anchor.CanListSymbols && (!strings.HasSuffix(anchor.Path, ".go") || strings.HasSuffix(anchor.Path, "_test.go")) {
				return fmt.Errorf("report manifest: anchor %q cannot list symbols for this path", anchor.ID)
			}
			if len(anchor.AllowedLines) > maxManifestLinesPerAnchor {
				return fmt.Errorf("report manifest: anchor %q has more than %d allowed lines", anchor.ID, maxManifestLinesPerAnchor)
			}
			previousLine := 0
			for _, line := range anchor.AllowedLines {
				if line <= previousLine {
					return fmt.Errorf("report manifest: anchor %q allowed lines must be positive, unique, and sorted", anchor.ID)
				}
				previousLine = line
			}
		}
	}
	return nil
}

// ConfirmRunAuthority records the initial repository state only when a second
// capture proves that repository analysis did not observe a moving checkout.
func ConfirmRunAuthority(
	analysisRoot string,
	initial freshness.RepositoryState,
	current freshness.RepositoryState,
) (RunAuthority, error) {
	initialDigest, err := initial.Digest()
	if err != nil {
		return RunAuthority{}, fmt.Errorf("report manifest: initial repository state: %w", err)
	}
	currentDigest, err := current.Digest()
	if err != nil {
		return RunAuthority{}, fmt.Errorf("report manifest: current repository state: %w", err)
	}
	if initialDigest != currentDigest {
		return RunAuthority{}, fmt.Errorf("report manifest: repository changed during orientation")
	}
	root, err := canonicalManifestDirectory("analysis root", analysisRoot)
	if err != nil {
		return RunAuthority{}, err
	}
	repositoryRoot, err := canonicalManifestDirectory("repository identity", initial.Identity)
	if err != nil {
		return RunAuthority{}, err
	}
	if repositoryRoot != initial.Identity {
		return RunAuthority{}, fmt.Errorf("report manifest: repository identity is not canonical")
	}
	if err := validateAnalysisRoot(repositoryRoot, root); err != nil {
		return RunAuthority{}, fmt.Errorf("report manifest: analysis root: %w", err)
	}

	repository := initial
	repository.Dirty = append([]freshness.DirtyFile(nil), initial.Dirty...)
	return RunAuthority{
		analysisRoot: root,
		repository:   repository,
		freshness:    freshness.NewFreshnessResult(freshness.FreshnessFresh),
		confirmed:    true,
	}, nil
}

// ConfirmRunAuthorityScoped binds report freshness to exact captured inputs.
// Unrelated repository changes remain authorized; strict mode rejects stale or
// unavailable analyzed inputs.
func ConfirmRunAuthorityScoped(
	ctx context.Context,
	analysisRoot string,
	initial freshness.RepositoryState,
	current freshness.RepositoryState,
	paths []string,
	strict bool,
) (RunAuthority, error) {
	root, repositoryRoot, err := validateAuthorityRoots(analysisRoot, initial)
	if err != nil {
		return RunAuthority{}, err
	}
	repositoryPaths, err := repositoryRelativeInputPaths(repositoryRoot, root, paths)
	if err != nil {
		return RunAuthority{}, err
	}
	inputs, err := freshness.CaptureInputs(ctx, initial, repositoryPaths)
	if err != nil {
		return RunAuthority{}, fmt.Errorf("report manifest: capture analyzed inputs: %w", err)
	}
	result := freshness.AssessInputs(ctx, initial, current, inputs)
	result.AffectedPaths = analysisRelativeAffectedPaths(repositoryRoot, root, result.AffectedPaths)
	if strict && result.State != freshness.FreshnessFresh && result.State != freshness.FreshnessUnrelatedChanges {
		return RunAuthority{}, fmt.Errorf("report manifest: strict snapshot is %s", result.State)
	}
	repository := initial
	repository.Dirty = append([]freshness.DirtyFile(nil), initial.Dirty...)
	repository.Submodules = append([]freshness.SubmoduleState(nil), initial.Submodules...)
	return RunAuthority{
		analysisRoot: root, repository: repository, inputs: inputs,
		freshness: result, confirmed: true,
	}, nil
}

func analysisRelativeAffectedPaths(repositoryRoot, analysisRoot string, values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		relative, err := filepath.Rel(analysisRoot, filepath.Join(repositoryRoot, filepath.FromSlash(value)))
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		result = append(result, filepath.ToSlash(relative))
	}
	sort.Strings(result)
	return result
}

func validateAuthorityRoots(analysisRoot string, initial freshness.RepositoryState) (string, string, error) {
	root, err := canonicalManifestDirectory("analysis root", analysisRoot)
	if err != nil {
		return "", "", err
	}
	repositoryRoot, err := canonicalManifestDirectory("repository identity", initial.Identity)
	if err != nil {
		return "", "", err
	}
	if repositoryRoot != initial.Identity {
		return "", "", fmt.Errorf("report manifest: repository identity is not canonical")
	}
	if err := validateAnalysisRoot(repositoryRoot, root); err != nil {
		return "", "", fmt.Errorf("report manifest: analysis root: %w", err)
	}
	return root, repositoryRoot, nil
}

func repositoryRelativeInputPaths(repositoryRoot, analysisRoot string, paths []string) ([]string, error) {
	analysisRelative, err := filepath.Rel(repositoryRoot, analysisRoot)
	if err != nil || analysisRelative == ".." || strings.HasPrefix(analysisRelative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("report manifest: analysis root is outside repository")
	}
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if err := validateManifestPath(path); err != nil {
			return nil, err
		}
		joined := filepath.ToSlash(filepath.Clean(filepath.Join(analysisRelative, filepath.FromSlash(path))))
		if joined == "." || joined == ".." || strings.HasPrefix(joined, "../") {
			return nil, fmt.Errorf("report manifest: captured input path escapes repository")
		}
		result = append(result, joined)
	}
	sort.Strings(result)
	compacted := result[:0]
	for _, path := range result {
		if len(compacted) == 0 || compacted[len(compacted)-1] != path {
			compacted = append(compacted, path)
		}
	}
	return compacted, nil
}

// CapturedInputPaths returns report-relative files and build metadata that
// materially informed the saved report. It never expands into ignored or
// untracked directories.
func CapturedInputPaths(data *ReportData) []string {
	if data == nil {
		return nil
	}
	paths := append([]string(nil), data.OpenablePaths...)
	if data.TaskInvestigation != nil {
		paths = append(paths, data.TaskInvestigation.MaterialPaths...)
	}
	paths = append(paths, "go.work", "go.work.sum")
	if data.RepositoryGraph != nil {
		for _, module := range data.RepositoryGraph.Modules {
			dir := filepath.ToSlash(filepath.Clean(filepath.FromSlash(module.Dir)))
			if dir == "." || dir == "" {
				paths = append(paths, "go.mod", "go.sum")
				continue
			}
			paths = append(paths, path.Join(dir, "go.mod"), path.Join(dir, "go.sum"))
		}
		for _, pkg := range data.RepositoryGraph.Packages {
			paths = append(paths, pkg.Files...)
		}
	}
	sort.Strings(paths)
	compacted := paths[:0]
	for _, value := range paths {
		if value == "" || (len(compacted) > 0 && compacted[len(compacted)-1] == value) {
			continue
		}
		compacted = append(compacted, value)
	}
	return compacted
}

// ResolveAnalysisRoot returns the canonical directory against which all
// repository-relative report paths must be resolved. It rechecks the stored
// paths so a directory replaced by a symlink cannot widen report authority.
func (m RunManifest) ResolveAnalysisRoot() (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	repositoryRoot, err := canonicalManifestDirectory("repository identity", m.RepositoryState.Identity)
	if err != nil {
		return "", err
	}
	if repositoryRoot != m.RepositoryState.Identity {
		return "", fmt.Errorf("report manifest: repository identity is no longer canonical")
	}
	analysisRoot, err := canonicalManifestDirectory("analysis root", m.AnalysisRoot)
	if err != nil {
		return "", err
	}
	if analysisRoot != m.AnalysisRoot {
		return "", fmt.Errorf("report manifest: analysis root is no longer canonical")
	}
	if err := validateAnalysisRoot(repositoryRoot, analysisRoot); err != nil {
		return "", fmt.Errorf("report manifest: analysis root: %w", err)
	}
	return analysisRoot, nil
}

// SourceCatalog adapts the source scope of an already validated manifest into
// a presentation-neutral catalog. Report/run binding and all other manifest
// authority remain outside the catalog.
func (m RunManifest) SourceCatalog() (sourcecatalog.Catalog, error) {
	if err := m.Validate(); err != nil {
		return sourcecatalog.Catalog{}, err
	}
	catalog, err := sourcecatalog.New(sourcecatalog.Input{
		RepositoryRoot: m.RepositoryState.Identity,
		AnalysisRoot:   m.AnalysisRoot,
		AllowedPaths:   m.OpenablePaths,
		CapturedInputs: m.CapturedInputs,
	})
	if err != nil {
		return sourcecatalog.Catalog{}, fmt.Errorf("report manifest: source catalog: %w", err)
	}
	return catalog, nil
}

// WorkspaceSnapshot adapts current captured-input authority into one
// presentation-neutral immutable value. Live filesystem canonicality remains
// the responsibility of ResolveAnalysisRoot.
func (m RunManifest) WorkspaceSnapshot() (workspacesnapshot.Snapshot, error) {
	if err := m.Validate(); err != nil {
		return workspacesnapshot.Snapshot{}, err
	}
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot:   m.AnalysisRoot,
		Repository:     m.RepositoryState,
		CapturedInputs: m.CapturedInputs,
		AllowedPaths:   m.OpenablePaths,
	})
	if err != nil {
		return workspacesnapshot.Snapshot{}, fmt.Errorf("report manifest: workspace snapshot: %w", err)
	}
	return snapshot, nil
}

// VerifyReportJSON verifies both the exact report bytes and the authority
// projection carried by those bytes.
func (m RunManifest) VerifyReportJSON(reportJSON []byte) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if len(reportJSON) > maxManifestReportBytes {
		return fmt.Errorf("report manifest: report exceeds %d bytes", maxManifestReportBytes)
	}
	if manifestSHA256(reportJSON) != m.ReportSHA256 {
		return fmt.Errorf("report manifest: report sha256 mismatch")
	}
	var report struct {
		FormatVersion                   int                                        `json:"format_version"`
		OpenablePaths                   []string                                   `json:"openable_paths"`
		Components                      []Component                                `json:"components"`
		RepositoryAtlas                 *repositoryatlas.Atlas                     `json:"repository_atlas"`
		Navigator                       *NavigatorReportProduct                    `json:"navigator"`
		ArchitectureCanvas              *ArchitectureCanvas                        `json:"architecture_canvas"`
		ArchitectureComponentNavigation *ArchitectureComponentNavigationProjection `json:"architecture_component_navigation"`
		Architecture                    *ArchitectureSynthesisStatus               `json:"architecture_synthesis"`
		AtlasStudy                      *AtlasStudyReportStatus                    `json:"atlas_study"`
		StudyMap                        *RepositoryStudyMap                        `json:"study_map"`
		TaskInvestigation               *struct {
			BundleSHA256                 string `json:"bundle_sha256"`
			AttemptSHA256                string `json:"attempt_sha256"`
			PackSHA256                   string `json:"pack_sha256"`
			StatusSHA256                 string `json:"status_sha256"`
			RetrievalTraceSHA256         string `json:"retrieval_trace_sha256"`
			RetrievalTraceMarkdownSHA256 string `json:"retrieval_trace_markdown_sha256"`
		} `json:"task_investigation"`
	}
	if err := json.Unmarshal(reportJSON, &report); err != nil {
		return fmt.Errorf("report manifest: decode report: %w", err)
	}
	if report.FormatVersion != m.ReportFormatVersion {
		return fmt.Errorf("report manifest: report format version mismatch")
	}
	if report.ArchitectureCanvas != nil && report.ArchitectureCanvas.Version != ArchitectureCanvasVersion {
		return fmt.Errorf(
			"report manifest: unsupported architecture canvas version %d, want %d",
			report.ArchitectureCanvas.Version,
			ArchitectureCanvasVersion,
		)
	}
	if err := ValidateArchitectureComponentNavigation(
		report.ArchitectureCanvas,
		report.OpenablePaths,
		report.ArchitectureComponentNavigation,
	); err != nil {
		return fmt.Errorf("report manifest: %w", err)
	}
	if report.Architecture != nil {
		if err := report.Architecture.Validate(); err != nil {
			return fmt.Errorf("report manifest: architecture synthesis status: %w", err)
		}
	}
	hasRepositoryAtlas := m.MaterialInputs.RepositoryAtlasSHA256 != ""
	if (report.RepositoryAtlas != nil) != hasRepositoryAtlas {
		return fmt.Errorf("report manifest: repository Atlas identity does not match report")
	}
	if report.RepositoryAtlas != nil {
		if _, err := repositoryatlas.CanonicalJSON(*report.RepositoryAtlas); err != nil {
			return fmt.Errorf("report manifest: repository Atlas: %w", err)
		}
	}
	hasNavigatorResult := m.MaterialInputs.NavigatorResultSHA256 != ""
	hasNavigatorStatus := m.MaterialInputs.NavigatorStatusSHA256 != ""
	if (report.Navigator != nil) != hasNavigatorStatus ||
		hasRepositoryAtlas != hasNavigatorStatus {
		return fmt.Errorf("report manifest: Navigator artifact identity does not match report")
	}
	if report.Navigator != nil {
		hasNavigatorRequest := m.MaterialInputs.NavigatorRequestSHA256 != ""
		switch report.Navigator.State {
		case navigator.ProductStateEmpty:
			if hasNavigatorRequest || !hasNavigatorResult ||
				report.Navigator.UnavailableCode != "" || report.Navigator.FailureCode != "" ||
				report.Navigator.Recommendation != nil {
				return fmt.Errorf("report manifest: empty Navigator report projection is invalid")
			}
		case navigator.ProductStateSelected:
			if !hasNavigatorRequest || !hasNavigatorResult ||
				report.Navigator.UnavailableCode != "" || report.Navigator.FailureCode != "" ||
				report.Navigator.Recommendation == nil {
				return fmt.Errorf("report manifest: selected Navigator report projection is incomplete")
			}
		case navigator.ProductStateUnavailable:
			if !hasNavigatorRequest || hasNavigatorResult ||
				report.Navigator.UnavailableCode != navigator.UnavailableOffline ||
				report.Navigator.FailureCode != "" ||
				report.Navigator.Recommendation != nil {
				return fmt.Errorf("report manifest: unavailable Navigator report projection is invalid")
			}
		case navigator.ProductStateFailed:
			if !hasNavigatorRequest || hasNavigatorResult ||
				report.Navigator.UnavailableCode != "" ||
				!publishableNavigatorFailure(report.Navigator.FailureCode) ||
				report.Navigator.Recommendation != nil {
				return fmt.Errorf("report manifest: failed Navigator report projection is invalid")
			}
		default:
			return fmt.Errorf("report manifest: unsupported Navigator report state %q", report.Navigator.State)
		}
		if report.Navigator.Version != navigator.ProductVersion {
			return fmt.Errorf("report manifest: unsupported Navigator report version %d", report.Navigator.Version)
		}
	}
	hasAtlasStudyRequest := m.MaterialInputs.AtlasStudyRequestSHA256 != ""
	hasAtlasStudyResult := m.MaterialInputs.AtlasStudyResultSHA256 != ""
	hasAtlasStudyStatus := m.MaterialInputs.AtlasStudyStatusSHA256 != ""
	if report.AtlasStudy == nil && (hasAtlasStudyRequest || hasAtlasStudyResult || hasAtlasStudyStatus) {
		return fmt.Errorf("report manifest: Atlas Study artifact identity does not match report")
	}
	if report.AtlasStudy != nil {
		if !hasRepositoryAtlas || report.AtlasStudy.Version != atlasstudy.ResultVersion ||
			report.AtlasStudy.ProjectionVersion != AtlasStudyReportProjectionVersion {
			return fmt.Errorf("report manifest: Atlas Study report projection is incomplete")
		}
		if err := validateAtlasStudyReportProjection(
			report.AtlasStudy,
			report.StudyMap,
			hasAtlasStudyRequest,
			hasAtlasStudyResult,
			hasAtlasStudyStatus,
		); err != nil {
			return fmt.Errorf("report manifest: %w", err)
		}
	} else if report.StudyMap != nil && hasRepositoryAtlas {
		return fmt.Errorf("report manifest: Atlas-first Study map lacks Atlas Study state")
	}
	authority, err := componentAuthority(report.Components)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(report.OpenablePaths, m.OpenablePaths) || !reflect.DeepEqual(authority, m.Components) {
		return fmt.Errorf("report manifest: authority does not match report")
	}
	material := m.MaterialInputs
	if report.TaskInvestigation == nil {
		if material.TaskBundleSHA256 != "" || material.TaskAttemptSHA256 != "" ||
			material.TaskPackSHA256 != "" || material.TaskStatusSHA256 != "" ||
			material.TaskRetrievalTraceSHA256 != "" || material.TaskRetrievalTraceMarkdownSHA256 != "" {
			return fmt.Errorf("report manifest: Task Lens artifacts are not present in report")
		}
	} else if report.TaskInvestigation.BundleSHA256 != material.TaskBundleSHA256 ||
		report.TaskInvestigation.AttemptSHA256 != material.TaskAttemptSHA256 ||
		report.TaskInvestigation.PackSHA256 != material.TaskPackSHA256 ||
		report.TaskInvestigation.StatusSHA256 != material.TaskStatusSHA256 ||
		!validManifestSHA256(report.TaskInvestigation.BundleSHA256) ||
		!validManifestSHA256(report.TaskInvestigation.AttemptSHA256) ||
		!validManifestSHA256(report.TaskInvestigation.PackSHA256) ||
		!validManifestSHA256(report.TaskInvestigation.StatusSHA256) ||
		report.TaskInvestigation.RetrievalTraceSHA256 != material.TaskRetrievalTraceSHA256 ||
		report.TaskInvestigation.RetrievalTraceMarkdownSHA256 != material.TaskRetrievalTraceMarkdownSHA256 ||
		!validManifestSHA256(report.TaskInvestigation.RetrievalTraceSHA256) ||
		!validManifestSHA256(report.TaskInvestigation.RetrievalTraceMarkdownSHA256) {
		return fmt.Errorf("report manifest: Task Lens artifact identity does not match report")
	}
	return nil
}

func validateAtlasStudyReportProjection(
	status *AtlasStudyReportStatus,
	studyMap *RepositoryStudyMap,
	hasRequest bool,
	hasResult bool,
	hasStatus bool,
) error {
	if status == nil {
		return fmt.Errorf("Atlas Study projection is absent")
	}
	zeroSpanCoverage := status.RequestedSpanCount == 0 && status.CoveredSpanCount == 0 &&
		status.UncoveredSpanCount == 0 && !status.CoverageComplete
	switch status.State {
	case atlasstudy.ProductStateAccepted, atlasstudy.ProductStateAcceptedPartial:
		if !hasRequest || !hasResult || !hasStatus || studyMap == nil ||
			status.UnavailableCode != "" || status.FailureCode != "" ||
			status.CandidateCoverage == nil || status.DirectionCount < 1 ||
			status.PublishedDirectionCount < 1 ||
			status.PublishedDirectionCount != len(studyMap.Directions) ||
			status.HiddenDirectionCount != len(studyMap.HiddenDirections) ||
			status.DirectionCount != status.PublishedDirectionCount+status.HiddenDirectionCount ||
			status.CoveredSpanCount != status.DirectionCount ||
			status.RequestedSpanCount != status.CoveredSpanCount+status.UncoveredSpanCount {
			return fmt.Errorf("accepted Atlas Study projection is invalid")
		}
		if err := status.CandidateCoverage.validate(); err != nil {
			return err
		}
		if status.CandidateCoverage.SpansSelected != status.RequestedSpanCount {
			return fmt.Errorf("accepted Atlas Study candidate/span counts do not match")
		}
		if status.State == atlasstudy.ProductStateAccepted {
			if !status.CoverageComplete || status.UncoveredSpanCount != 0 {
				return fmt.Errorf("complete Atlas Study projection is invalid")
			}
		} else if status.CoverageComplete || status.UncoveredSpanCount <= 0 {
			return fmt.Errorf("partial Atlas Study projection is invalid")
		}
	case atlasstudy.ProductStateUnavailable:
		if hasRequest || hasResult || hasStatus || studyMap != nil ||
			(status.UnavailableCode != AtlasStudyUnavailableOffline &&
				status.UnavailableCode != AtlasStudyUnavailableInsufficientCatalog) ||
			status.FailureCode != "" || status.CandidateCoverage != nil ||
			status.DirectionCount != 0 || status.PublishedDirectionCount != 0 ||
			status.HiddenDirectionCount != 0 || !zeroSpanCoverage {
			return fmt.Errorf("unavailable Atlas Study projection is invalid")
		}
	case atlasstudy.ProductStateFailed:
		if !hasRequest || hasResult || !hasStatus || studyMap != nil ||
			status.UnavailableCode != "" || status.CandidateCoverage == nil ||
			!status.FailureCode.Valid() || status.FailureCode == atlasstudy.FailureResource ||
			status.FailureCode == atlasstudy.FailureCanceled || status.DirectionCount != 0 ||
			status.PublishedDirectionCount != 0 || status.HiddenDirectionCount != 0 || !zeroSpanCoverage {
			return fmt.Errorf("failed Atlas Study projection is invalid")
		}
		if err := status.CandidateCoverage.validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported Atlas Study report state %q", status.State)
	}
	return nil
}

// VerifyTaskInvestigationArtifacts binds the canonical Task Lens files
// to the exact report/manifest pair before any saved workspace is exposed.
func (m RunManifest) VerifyTaskInvestigationArtifacts(runDir string) error {
	material := m.MaterialInputs
	if material.TaskBundleSHA256 == "" {
		return nil
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("report manifest: open Task Lens run: %w", err)
	}
	defer root.Close()
	artifacts := []struct {
		name string
		want string
	}{
		{tasklens.BundleFile, material.TaskBundleSHA256},
		{tasklens.AttemptFile, material.TaskAttemptSHA256},
		{tasklens.PackFile, material.TaskPackSHA256},
		{tasklens.StatusFile, material.TaskStatusSHA256},
	}
	artifacts = append(artifacts,
		struct{ name, want string }{tasklens.TraceJSONFile, material.TaskRetrievalTraceSHA256},
		struct{ name, want string }{tasklens.TraceMarkdownFile, material.TaskRetrievalTraceMarkdownSHA256},
	)
	for _, artifact := range artifacts {
		data, readErr := readManifestFile(root, artifact.name, maxRunManifestBytes)
		if readErr != nil || manifestSHA256(data) != artifact.want {
			return fmt.Errorf("report manifest: Task Lens artifact %s sha256 mismatch", artifact.name)
		}
	}
	return nil
}

// VerifyOrientationContextSelectionArtifact binds the safe selection trace to
// the authorized run before callers trust it as an explanation of model input.
func (m RunManifest) VerifyOrientationContextSelectionArtifact(runDir string) error {
	want := m.MaterialInputs.OrientationContextSelectionSHA256
	if want == "" {
		return nil
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("report manifest: open orientation context selection run: %w", err)
	}
	defer root.Close()
	data, err := readManifestFile(
		root,
		llmbundle.OrientationContextSelectionFilename,
		llmbundle.MaxOrientationContextSelectionBytes,
	)
	if err != nil || manifestSHA256(data) != want {
		return fmt.Errorf("report manifest: orientation context selection sha256 mismatch")
	}
	selection, err := llmbundle.DecodeOrientationContextSelection(data)
	if err != nil {
		return fmt.Errorf("report manifest: orientation context selection: %w", err)
	}
	modelBundle, err := readManifestFile(root, "llm_bundle.json", maxManifestReportBytes)
	if err != nil || manifestSHA256(modelBundle) != m.MaterialInputs.ModelBundleSHA256 {
		return fmt.Errorf("report manifest: model bundle sha256 mismatch")
	}
	if !orientationSelectionMatchesModelBundle(selection, modelBundle) {
		return fmt.Errorf("report manifest: orientation context selection model bundle identity mismatch")
	}
	return nil
}

// VerifyRepositoryAtlasArtifact binds the exact canonical Atlas file to the
// Atlas value embedded in the authorized report. Neither copy is accepted as
// an independent source of truth.
func (m RunManifest) VerifyRepositoryAtlasArtifact(runDir string, reportJSON []byte) error {
	want := m.MaterialInputs.RepositoryAtlasSHA256
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("report manifest: open repository Atlas run: %w", err)
	}
	defer root.Close()
	if want == "" {
		if _, statErr := root.Lstat(repositoryatlas.ArtifactFilename); statErr == nil {
			return fmt.Errorf("report manifest: unbound repository Atlas artifact is present")
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return fmt.Errorf("report manifest: inspect repository Atlas artifact: %w", statErr)
		}
		return nil
	}
	encoded, err := readManifestFile(root, repositoryatlas.ArtifactFilename, repositoryatlas.MaxArtifactBytes)
	if err != nil || manifestSHA256(encoded) != want {
		return fmt.Errorf("report manifest: repository Atlas artifact sha256 mismatch")
	}
	atlas, err := repositoryatlas.DecodeCanonicalJSON(encoded)
	if err != nil {
		return fmt.Errorf("report manifest: repository Atlas artifact: %w", err)
	}
	var persisted struct {
		RepositoryAtlas *repositoryatlas.Atlas `json:"repository_atlas"`
	}
	if err := json.Unmarshal(reportJSON, &persisted); err != nil {
		return fmt.Errorf("report manifest: decode report repository Atlas: %w", err)
	}
	if persisted.RepositoryAtlas == nil || !reflect.DeepEqual(atlas, *persisted.RepositoryAtlas) {
		return fmt.Errorf("report manifest: repository Atlas artifact does not match report")
	}
	return nil
}

// VerifyNavigatorArtifacts binds the exact Navigator request/result/status
// files to the Atlas and the deliberately small recommendation projection in
// the authorized report. Selected runs require all three files; a local empty
// result requires result and status; an offline run requires its compiled
// request and status and forbids a result artifact.
func (m RunManifest) VerifyNavigatorArtifacts(runDir string, reportJSON []byte) error {
	artifacts := []struct {
		name  string
		want  string
		limit int
	}{
		{navigator.RequestArtifactFilename, m.MaterialInputs.NavigatorRequestSHA256, repositoryatlas.MaxArtifactBytes},
		{navigator.RecordArtifactFilename, m.MaterialInputs.NavigatorResultSHA256, repositoryatlas.MaxArtifactBytes},
		{navigator.StatusArtifactFilename, m.MaterialInputs.NavigatorStatusSHA256, navigator.MaxStatusArtifactBytes},
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("report manifest: open Navigator run: %w", err)
	}
	defer root.Close()
	for _, artifact := range artifacts {
		if artifact.want == "" {
			if _, statErr := root.Lstat(artifact.name); statErr == nil {
				return fmt.Errorf("report manifest: unbound Navigator artifact %s is present", artifact.name)
			} else if !errors.Is(statErr, fs.ErrNotExist) {
				return fmt.Errorf("report manifest: inspect Navigator artifact %s: %w", artifact.name, statErr)
			}
			continue
		}
		encoded, readErr := readManifestFile(root, artifact.name, artifact.limit)
		if readErr != nil || manifestSHA256(encoded) != artifact.want {
			return fmt.Errorf("report manifest: Navigator artifact %s sha256 mismatch", artifact.name)
		}
	}

	var persisted struct {
		RepositoryAtlas *repositoryatlas.Atlas  `json:"repository_atlas"`
		Navigator       *NavigatorReportProduct `json:"navigator"`
	}
	if err := json.Unmarshal(reportJSON, &persisted); err != nil {
		return fmt.Errorf("report manifest: decode report Navigator projection: %w", err)
	}
	projected, err := readNavigatorReportProduct(runDir, persisted.RepositoryAtlas)
	if err != nil {
		return fmt.Errorf("report manifest: Navigator artifacts: %w", err)
	}
	if !reflect.DeepEqual(projected, persisted.Navigator) {
		return fmt.Errorf("report manifest: Navigator artifacts do not match report")
	}
	return nil
}

// VerifyAtlasStudyArtifacts binds the exact Atlas Study request/result/status
// files to the authorized report. Semantic decoding and projection equality
// are enforced by the shared Atlas Study report reader after the byte identity
// checks below succeed.
func (m RunManifest) VerifyAtlasStudyArtifacts(runDir string, reportJSON []byte) error {
	if m.Version != CurrentRunManifestVersion {
		return fmt.Errorf("report manifest: unsupported version %d", m.Version)
	}
	artifacts := []struct {
		name  string
		want  string
		limit int
	}{
		{atlasstudy.RequestArtifactFilename, m.MaterialInputs.AtlasStudyRequestSHA256, atlasstudy.MaxRequestArtifactBytes},
		{atlasstudy.ResultArtifactFilename, m.MaterialInputs.AtlasStudyResultSHA256, atlasstudy.MaxResultArtifactBytes},
		{atlasstudy.StatusArtifactFilename, m.MaterialInputs.AtlasStudyStatusSHA256, atlasstudy.MaxStatusArtifactBytes},
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("report manifest: open Atlas Study run: %w", err)
	}
	defer root.Close()
	for _, artifact := range artifacts {
		if artifact.want == "" {
			if _, statErr := root.Lstat(artifact.name); statErr == nil {
				return fmt.Errorf("report manifest: unbound Atlas Study artifact %s is present", artifact.name)
			} else if !errors.Is(statErr, fs.ErrNotExist) {
				return fmt.Errorf("report manifest: inspect Atlas Study artifact %s: %w", artifact.name, statErr)
			}
			continue
		}
		encoded, readErr := readManifestFile(root, artifact.name, artifact.limit)
		if readErr != nil || manifestSHA256(encoded) != artifact.want {
			return fmt.Errorf("report manifest: Atlas Study artifact %s sha256 mismatch", artifact.name)
		}
	}
	var persisted ReportData
	if err := json.Unmarshal(reportJSON, &persisted); err != nil {
		return fmt.Errorf("report manifest: decode Atlas Study report projection: %w", err)
	}
	if persisted.AtlasStudy == nil &&
		m.MaterialInputs.AtlasStudyRequestSHA256 == "" &&
		m.MaterialInputs.AtlasStudyResultSHA256 == "" &&
		m.MaterialInputs.AtlasStudyStatusSHA256 == "" {
		return nil
	}
	status, studyMap, err := readAtlasStudyReportProduct(runDir, &persisted)
	if err != nil {
		return fmt.Errorf("report manifest: Atlas Study artifacts: %w", err)
	}
	if !reflect.DeepEqual(status, persisted.AtlasStudy) ||
		!reflect.DeepEqual(studyMap, persisted.StudyMap) {
		return fmt.Errorf("report manifest: Atlas Study artifacts do not match report")
	}
	return nil
}

func orientationSelectionMatchesModelBundle(
	selection llmbundle.OrientationContextSelection,
	modelBundle []byte,
) bool {
	return selection.PersistedBundleBytes == len(modelBundle) &&
		selection.PersistedBundleSHA256 == manifestSHA256(modelBundle)
}

// VerifyRepositoryState lets a caller compare a freshly captured repository
// state with the one authorized by this manifest without re-reading artifacts.
func (m RunManifest) VerifyRepositoryState(current freshness.RepositoryState) error {
	if err := m.Validate(); err != nil {
		return err
	}
	result := freshness.AssessInputs(context.Background(), m.RepositoryState, current, m.CapturedInputs)
	if result.State == freshness.FreshnessPartiallyStale || result.State == freshness.FreshnessMixedSnapshot ||
		result.State == freshness.FreshnessUnavailable {
		return fmt.Errorf("report manifest: analyzed inputs are %s", result.State)
	}
	return nil
}

func (m RunManifest) CurrentFreshness(current freshness.RepositoryState) freshness.FreshnessResult {
	return freshness.AssessInputs(context.Background(), m.RepositoryState, current, m.CapturedInputs)
}

// DecodeRunManifest strictly decodes one bounded run_manifest.json payload.
func DecodeRunManifest(data []byte) (RunManifest, error) {
	if len(data) == 0 || len(data) > maxRunManifestBytes {
		return RunManifest{}, fmt.Errorf("report manifest: size must be between 1 and %d bytes", maxRunManifestBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest RunManifest
	if err := decoder.Decode(&manifest); err != nil {
		return RunManifest{}, fmt.Errorf("report manifest: decode: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return RunManifest{}, fmt.Errorf("report manifest: multiple json values")
		}
		return RunManifest{}, fmt.Errorf("report manifest: trailing data: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return RunManifest{}, err
	}
	return manifest, nil
}

// RunManifestAuthoritySeed exposes only the exact repository identity and
// captured input paths needed to re-confirm authority before an authorized
// provider-free report render. It is not a verified interactive manifest:
// callers must capture the repository again and publish a new manifest.
type RunManifestAuthoritySeed struct {
	RepositoryIdentity string
	AnalysisRoot       string
	SelectedRevision   string
	// CapturedInputPaths are analysis-root-relative, matching the input
	// contract of ConfirmRunAuthorityScoped. The manifest itself stores these
	// paths relative to RepositoryIdentity.
	CapturedInputPaths []string
}

// ReadRunManifestAuthoritySeed strictly reads the existing bounded manifest
// without interpreting its report projection under the current contract. This
// narrow bootstrap seam lets a copied historical run recover the exact input
// set needed by ConfirmRunAuthorityScoped; the resulting render still replays
// every saved product and writes a fresh current manifest fail-closed.
func ReadRunManifestAuthoritySeed(runDir string) (RunManifestAuthoritySeed, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return RunManifestAuthoritySeed{}, fmt.Errorf("report manifest seed: open run directory: %w", err)
	}
	defer root.Close()
	raw, err := readManifestFile(root, RunManifestFilename, maxRunManifestBytes)
	if err != nil {
		return RunManifestAuthoritySeed{}, fmt.Errorf("report manifest seed: %w", err)
	}
	manifest, err := DecodeRunManifest(raw)
	if err != nil {
		return RunManifestAuthoritySeed{}, err
	}
	paths, err := analysisRelativeManifestInputPaths(manifest)
	if err != nil {
		return RunManifestAuthoritySeed{}, err
	}
	return RunManifestAuthoritySeed{
		RepositoryIdentity: manifest.RepositoryState.Identity,
		AnalysisRoot:       manifest.AnalysisRoot,
		SelectedRevision:   manifest.MaterialInputs.SelectedRevision,
		CapturedInputPaths: paths,
	}, nil
}

func analysisRelativeManifestInputPaths(manifest RunManifest) ([]string, error) {
	paths := make([]string, 0, len(manifest.CapturedInputs))
	for _, input := range manifest.CapturedInputs {
		repositoryPath := filepath.Join(manifest.RepositoryState.Identity, filepath.FromSlash(input.Path))
		relative, err := filepath.Rel(manifest.AnalysisRoot, repositoryPath)
		if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("report manifest seed: captured input %q is outside analysis root", input.Path)
		}
		path := filepath.ToSlash(relative)
		if err := validateManifestPath(path); err != nil {
			return nil, fmt.Errorf("report manifest seed: captured input %q: %w", input.Path, err)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// ReadRunManifest reads and verifies run_manifest.json and report.json from a
// run directory. Both files must be bounded, regular files (not symlinks).
func ReadRunManifest(runDir string) (RunManifest, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return RunManifest{}, fmt.Errorf("report manifest: open run directory: %w", err)
	}
	defer root.Close()

	manifestJSON, err := readManifestFile(root, RunManifestFilename, maxRunManifestBytes)
	if err != nil {
		return RunManifest{}, err
	}
	manifest, err := DecodeRunManifest(manifestJSON)
	if err != nil {
		return RunManifest{}, err
	}
	reportJSON, err := readManifestFile(root, "report.json", maxManifestReportBytes)
	if err != nil {
		return RunManifest{}, err
	}
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
		return RunManifest{}, err
	}
	if err := manifest.VerifyTaskInvestigationArtifacts(runDir); err != nil {
		return RunManifest{}, err
	}
	if err := manifest.VerifyOrientationContextSelectionArtifact(runDir); err != nil {
		return RunManifest{}, err
	}
	if err := manifest.VerifyRepositoryAtlasArtifact(runDir, reportJSON); err != nil {
		return RunManifest{}, err
	}
	if err := manifest.VerifyNavigatorArtifacts(runDir, reportJSON); err != nil {
		return RunManifest{}, err
	}
	if err := manifest.VerifyAtlasStudyArtifacts(runDir, reportJSON); err != nil {
		return RunManifest{}, err
	}
	return manifest, nil
}

// RemoveRunManifest invalidates any previous interactive authority for a run.
// Callers should do this before starting work that may reuse a run directory.
func RemoveRunManifest(runDir string) error {
	manifestPath := filepath.Join(runDir, RunManifestFilename)
	if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("report manifest: remove stale manifest: %w", err)
	}
	return nil
}

func (authority RunAuthority) validate() error {
	if !authority.confirmed {
		return fmt.Errorf("report manifest: run authority is not confirmed")
	}
	if err := authority.repository.Validate(); err != nil {
		return fmt.Errorf("report manifest: authorized repository state: %w", err)
	}
	if err := validateAnalysisRoot(authority.repository.Identity, authority.analysisRoot); err != nil {
		return fmt.Errorf("report manifest: analysis root: %w", err)
	}
	if err := authority.freshness.Validate(); err != nil {
		return fmt.Errorf("report manifest: authorized freshness: %w", err)
	}
	return nil
}

func writeAuthorizedRunManifest(runDir string, data *ReportData, reportJSON []byte, authority RunAuthority) error {
	if err := authority.validate(); err != nil {
		return err
	}
	repositoryDigest, err := authority.repository.Digest()
	if err != nil {
		return fmt.Errorf("report manifest: digest repository state: %w", err)
	}
	components, err := componentAuthority(data.Components)
	if err != nil {
		return err
	}
	inputs := append([]freshness.CapturedInput(nil), authority.inputs...)
	if inputs == nil {
		paths, err := repositoryRelativeInputPaths(authority.repository.Identity, authority.analysisRoot, data.OpenablePaths)
		if err != nil {
			return err
		}
		inputs, err = freshness.CaptureInputs(context.Background(), authority.repository, paths)
		if err != nil {
			return err
		}
	}
	annotateCapturedInputOwnership(inputs, data, authority.repository.Identity, authority.analysisRoot)
	inputsDigest, err := freshness.CapturedInputsDigest(inputs)
	if err != nil {
		return err
	}
	orientationSelectionDigest, err := savedOrientationContextSelectionSHA256(runDir)
	if err != nil {
		return err
	}
	modelBundleDigest, err := savedModelBundleSHA256(runDir)
	if err != nil {
		return err
	}
	repositoryAtlasDigest, err := savedRepositoryAtlasSHA256(runDir)
	if err != nil {
		return err
	}
	navigatorRequestDigest := savedArtifactSHA256(runDir, navigator.RequestArtifactFilename)
	navigatorResultDigest := savedArtifactSHA256(runDir, navigator.RecordArtifactFilename)
	navigatorStatusDigest := savedArtifactSHA256(runDir, navigator.StatusArtifactFilename)
	atlasStudyRequestDigest := savedArtifactSHA256(runDir, atlasstudy.RequestArtifactFilename)
	atlasStudyResultDigest := savedArtifactSHA256(runDir, atlasstudy.ResultArtifactFilename)
	atlasStudyStatusDigest := savedArtifactSHA256(runDir, atlasstudy.StatusArtifactFilename)
	manifest := RunManifest{
		Version:               CurrentRunManifestVersion,
		RepositoryState:       authority.repository,
		AnalysisRoot:          authority.analysisRoot,
		RepositoryStateSHA256: repositoryDigest,
		ReportSHA256:          manifestSHA256(reportJSON),
		ReportFormatVersion:   data.FormatVersion,
		OpenablePaths:         append([]string(nil), data.OpenablePaths...),
		Components:            components,
		CapturedInputs:        inputs,
		CapturedInputsSHA256:  inputsDigest,
		Freshness:             authority.freshness,
		MaterialInputs: MaterialInputs{
			SelectedRevision:                  authority.repository.Head,
			ModelBundleSHA256:                 modelBundleDigest,
			OrientationContextSelectionSHA256: orientationSelectionDigest,
			RepositoryAtlasSHA256:             repositoryAtlasDigest,
			NavigatorRequestSHA256:            navigatorRequestDigest,
			NavigatorResultSHA256:             navigatorResultDigest,
			NavigatorStatusSHA256:             navigatorStatusDigest,
			AtlasStudyRequestSHA256:           atlasStudyRequestDigest,
			AtlasStudyResultSHA256:            atlasStudyResultDigest,
			AtlasStudyStatusSHA256:            atlasStudyStatusDigest,
			TaskBundleSHA256:                  savedArtifactSHA256(runDir, tasklens.BundleFile),
			TaskAttemptSHA256:                 savedArtifactSHA256(runDir, tasklens.AttemptFile),
			TaskPackSHA256:                    savedArtifactSHA256(runDir, tasklens.PackFile),
			TaskStatusSHA256:                  savedArtifactSHA256(runDir, tasklens.StatusFile),
			TaskRetrievalTraceSHA256:          savedArtifactSHA256(runDir, tasklens.TraceJSONFile),
			TaskRetrievalTraceMarkdownSHA256:  savedArtifactSHA256(runDir, tasklens.TraceMarkdownFile),
			InputPolicyVersion:                "captured-inputs-v1", ArchitectureContract: componentmap.ContractVersion,
			ReportContract: data.FormatVersion,
		},
	}
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
		return err
	}
	if err := manifest.VerifyOrientationContextSelectionArtifact(runDir); err != nil {
		return err
	}
	if err := manifest.VerifyRepositoryAtlasArtifact(runDir, reportJSON); err != nil {
		return err
	}
	if err := manifest.VerifyNavigatorArtifacts(runDir, reportJSON); err != nil {
		return err
	}
	if err := manifest.VerifyAtlasStudyArtifacts(runDir, reportJSON); err != nil {
		return err
	}
	return writeRunManifestAtomic(runDir, manifest)
}

func annotateCapturedInputOwnership(inputs []freshness.CapturedInput, data *ReportData, repositoryRoot, analysisRoot string) {
	if data == nil || data.RepositoryGraph == nil {
		return
	}
	for index := range inputs {
		relative, err := filepath.Rel(analysisRoot, filepath.Join(repositoryRoot, filepath.FromSlash(inputs[index].Path)))
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		dir := path.Dir(filepath.ToSlash(relative))
		if dir == "." {
			dir = ""
		}
		for _, pkg := range data.RepositoryGraph.Packages {
			if pkg.Dir == dir {
				inputs[index].OwningModuleID = pkg.ModuleID
				inputs[index].OwningPackage = pkg.CanonicalPath
				break
			}
		}
	}
}

func savedArtifactSHA256(runDir, name string) string {
	info, err := os.Lstat(filepath.Join(runDir, name))
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxManifestReportBytes {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(runDir, name))
	if err != nil {
		return ""
	}
	return manifestSHA256(data)
}

func savedOrientationContextSelectionSHA256(runDir string) (string, error) {
	path := filepath.Join(runDir, llmbundle.OrientationContextSelectionFilename)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("report manifest: inspect orientation context selection: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > llmbundle.MaxOrientationContextSelectionBytes {
		return "", fmt.Errorf("report manifest: orientation context selection must be a bounded regular file")
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return "", fmt.Errorf("report manifest: open orientation context selection run: %w", err)
	}
	defer root.Close()
	data, err := readManifestFile(root, llmbundle.OrientationContextSelectionFilename, llmbundle.MaxOrientationContextSelectionBytes)
	if err != nil {
		return "", fmt.Errorf("report manifest: read orientation context selection: %w", err)
	}
	if _, err := llmbundle.DecodeOrientationContextSelection(data); err != nil {
		return "", fmt.Errorf("report manifest: orientation context selection: %w", err)
	}
	return manifestSHA256(data), nil
}

func savedModelBundleSHA256(runDir string) (string, error) {
	path := filepath.Join(runDir, "llm_bundle.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("report manifest: inspect model bundle: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxManifestReportBytes {
		return "", fmt.Errorf("report manifest: model bundle must be a bounded regular file")
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return "", fmt.Errorf("report manifest: open model bundle run: %w", err)
	}
	defer root.Close()
	data, err := readManifestFile(root, "llm_bundle.json", maxManifestReportBytes)
	if err != nil {
		return "", fmt.Errorf("report manifest: read model bundle: %w", err)
	}
	return manifestSHA256(data), nil
}

func savedRepositoryAtlasSHA256(runDir string) (string, error) {
	path := filepath.Join(runDir, repositoryatlas.ArtifactFilename)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("report manifest: inspect repository Atlas: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > repositoryatlas.MaxArtifactBytes {
		return "", fmt.Errorf("report manifest: repository Atlas must be a bounded regular file")
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return "", fmt.Errorf("report manifest: open repository Atlas run: %w", err)
	}
	defer root.Close()
	data, err := readManifestFile(root, repositoryatlas.ArtifactFilename, repositoryatlas.MaxArtifactBytes)
	if err != nil {
		return "", fmt.Errorf("report manifest: read repository Atlas: %w", err)
	}
	if _, err := repositoryatlas.DecodeCanonicalJSON(data); err != nil {
		return "", fmt.Errorf("report manifest: repository Atlas: %w", err)
	}
	return manifestSHA256(data), nil
}

func validManifestLabel(value string) bool {
	if value == "" || len(value) > maxManifestIdentifierBytes {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func componentAuthority(components []Component) ([]ComponentAuthority, error) {
	if len(components) == 0 {
		return nil, nil
	}
	authority := make([]ComponentAuthority, 0, len(components))
	for _, component := range components {
		item := ComponentAuthority{
			ID:             component.ID,
			RelatedFlowIDs: append([]string(nil), component.RelatedFlowIDs...),
		}
		for _, anchor := range component.AnchorGroups {
			var lines []int
			for _, location := range anchor.Locations {
				if location.Path != anchor.Path || location.Line <= 0 {
					return nil, fmt.Errorf("report manifest: anchor %q has an invalid exact location", anchor.ID)
				}
				lines = append(lines, location.Line)
			}
			sort.Ints(lines)
			item.Anchors = append(item.Anchors, AnchorAuthority{
				ID:             anchor.ID,
				Path:           anchor.Path,
				AllowedLines:   lines,
				CanListSymbols: anchor.CanListSymbols,
			})
		}
		authority = append(authority, item)
	}
	return authority, nil
}

func writeRunManifestAtomic(runDir string, manifest RunManifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("report manifest: encode: %w", err)
	}
	data = append(data, '\n')
	if len(data) > maxRunManifestBytes {
		return fmt.Errorf("report manifest: encoded manifest exceeds %d bytes", maxRunManifestBytes)
	}
	temporary, err := os.CreateTemp(runDir, ".run-manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("report manifest: create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("report manifest: set temporary file permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("report manifest: write temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("report manifest: sync temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("report manifest: close temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, filepath.Join(runDir, RunManifestFilename)); err != nil {
		return fmt.Errorf("report manifest: install: %w", err)
	}
	removeTemporary = false
	return nil
}

func canonicalManifestDirectory(label, value string) (string, error) {
	if !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return "", fmt.Errorf("report manifest: %s must be a clean absolute path", label)
	}
	resolved, err := filepath.EvalSymlinks(value)
	if err != nil {
		return "", fmt.Errorf("report manifest: resolve %s: %w", label, err)
	}
	absolute, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("report manifest: resolve %s: %w", label, err)
	}
	root := filepath.Clean(absolute)
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("report manifest: inspect %s: %w", label, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("report manifest: %s is not a directory", label)
	}
	return root, nil
}

func validateAnalysisRoot(repositoryRoot, analysisRoot string) error {
	if len(analysisRoot) > maxManifestPathBytes || !filepath.IsAbs(analysisRoot) || filepath.Clean(analysisRoot) != analysisRoot {
		return fmt.Errorf("must be a clean absolute path")
	}
	for _, r := range analysisRoot {
		if unicode.IsControl(r) {
			return fmt.Errorf("must not contain control characters")
		}
	}
	relative, err := filepath.Rel(repositoryRoot, analysisRoot)
	if err != nil {
		return fmt.Errorf("must be inside repository identity: %w", err)
	}
	if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("must be inside repository identity")
	}
	return nil
}

func readManifestFile(root *os.Root, name string, limit int) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("report manifest: inspect %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("report manifest: %s is not a regular file", name)
	}
	if info.Size() < 0 || info.Size() > int64(limit) {
		return nil, fmt.Errorf("report manifest: %s exceeds %d bytes", name, limit)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, fmt.Errorf("report manifest: open %s: %w", name, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("report manifest: read %s: %w", name, err)
	}
	if len(data) > limit {
		return nil, fmt.Errorf("report manifest: %s exceeds %d bytes", name, limit)
	}
	return data, nil
}

func validateManifestPath(value string) error {
	if len(value) > maxManifestPathBytes || !fs.ValidPath(value) || value == "." || strings.ContainsRune(value, '\\') {
		return fmt.Errorf("path must be a clean repository-relative slash path")
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("path must not contain control characters")
		}
	}
	return nil
}

func validateManifestIdentifier(value string) error {
	if strings.TrimSpace(value) != value || value == "" || len(value) > maxManifestIdentifierBytes {
		return fmt.Errorf("must be non-empty and at most %d bytes", maxManifestIdentifierBytes)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("must not contain control characters")
		}
	}
	return nil
}

func validManifestSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func manifestSHA256(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
