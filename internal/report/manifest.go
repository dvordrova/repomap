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
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"unicode"

	"github.com/dvordrova/repomap/internal/activityentrypoint"
	"github.com/dvordrova/repomap/internal/activitypath"
	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/coremap"
	"github.com/dvordrova/repomap/internal/cubemap"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/dependencydeclaration"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/integrationusage"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/pythondeclareddependencies"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/runtimeportfolio"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/targetoutcome"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

const (
	CurrentRunManifestVersion = 35
	RunManifestFilename       = "run_manifest.json"

	maxRunManifestBytes             = 4 * 1024 * 1024
	maxManifestReportBytes          = MaxReportJSONBytes
	maxManifestSnapshotBytes        = 64 * 1024 * 1024
	maxManifestOpenablePaths        = 4096
	maxManifestIdentifierBytes      = 256
	maxManifestPathBytes            = 4096
	maxManifestRepositoryDirtyFiles = 20_000
)

// RunManifest is the local authority boundary for report source actions. It
// commits to the exact report, artifacts, target, and captured source inputs.
type RunManifest struct {
	Version               int                        `json:"version"`
	RepositoryState       freshness.RepositoryState  `json:"repository_state"`
	AnalysisRoot          string                     `json:"analysis_root"`
	StandaloneSource      *StandaloneSourceAuthority `json:"standalone_source,omitempty"`
	RepositoryStateSHA256 string                     `json:"repository_state_sha256"`
	SnapshotSHA256        string                     `json:"snapshot_sha256"`
	ReportSHA256          string                     `json:"report_sha256"`
	ReportFormatVersion   int                        `json:"report_format_version"`
	OpenablePaths         []string                   `json:"openable_paths"`
	CapturedInputs        []freshness.CapturedInput  `json:"captured_inputs,omitempty"`
	CapturedInputsSHA256  string                     `json:"captured_inputs_sha256,omitempty"`
	MaterialInputs        MaterialInputs             `json:"material_inputs"`
}

// StandaloneSourceAuthority is the manifest-owned external routing authority
// for one static report. The captured revision and repository-relative path
// prefix are deliberately derived from the surrounding manifest instead of
// being independently repeated in HTML.
type StandaloneSourceAuthority struct {
	Host          string `json:"host"`
	RepositoryURL string `json:"repository_url"`
}

func (authority *StandaloneSourceAuthority) validate() error {
	if authority == nil {
		return nil
	}
	var (
		normalized string
		err        error
	)
	switch authority.Host {
	case "GitHub":
		normalized, err = NormalizeGitHubRepositoryURL(authority.RepositoryURL)
	case "GitLab":
		normalized, err = NormalizeGitLabRepositoryURL(authority.RepositoryURL)
	default:
		return fmt.Errorf("report manifest: standalone source host is invalid")
	}
	if err != nil || normalized == "" || normalized != authority.RepositoryURL {
		return fmt.Errorf("report manifest: standalone source repository URL is not canonical")
	}
	return nil
}

type MaterialInputs struct {
	SelectedRevision              string `json:"selected_revision"`
	AnalysisTargetRef             string `json:"analysis_target_ref,omitempty"`
	AnalysisTargetSHA256          string `json:"analysis_target_sha256,omitempty"`
	ProgramTargetID               string `json:"program_target_id"`
	ProgramTargetSHA256           string `json:"program_target_sha256"`
	ProgramIndexSetSHA256         string `json:"program_index_set_sha256"`
	CubeMapSHA256                 string `json:"cube_map_sha256,omitempty"`
	CoreMapSHA256                 string `json:"core_map_sha256,omitempty"`
	ActivityEntrypointsSHA256     string `json:"activity_entrypoints_sha256,omitempty"`
	PythonTargetCatalogSHA256     string `json:"python_target_catalog_sha256,omitempty"`
	DeclaredDependenciesSHA256    string `json:"declared_dependencies_sha256,omitempty"`
	DependencyCatalogSHA256       string `json:"dependency_catalog_sha256,omitempty"`
	IntegrationDependenciesSHA256 string `json:"integration_dependencies_sha256,omitempty"`
	IntegrationUsageSHA256        string `json:"integration_usage_sha256,omitempty"`
	ActivityPathsSHA256           string `json:"activity_paths_sha256,omitempty"`
	JSTSProjectSHA256             string `json:"js_ts_project_sha256,omitempty"`
	ReadmeFileRolesSHA256         string `json:"readme_file_roles_sha256,omitempty"`
	TargetRunContainerSHA256      string `json:"target_run_container_sha256,omitempty"`
	TargetPagePortfolioSHA256     string `json:"target_page_portfolio_sha256,omitempty"`
	ProgramPagePortfolioSHA256    string `json:"program_page_portfolio_sha256,omitempty"`
	RuntimePortfolioSHA256        string `json:"runtime_portfolio_sha256,omitempty"`
	TargetOutcomePortfolioSHA256  string `json:"target_outcome_portfolio_sha256,omitempty"`
	InputPolicyVersion            string `json:"input_policy_version"`
	ReportContract                int    `json:"report_contract"`
}

// RunAuthority binds one analysis to its captured repository identity and
// exact material inputs. It deliberately carries no later-state comparison.
type RunAuthority struct {
	analysisRoot string
	repository   freshness.RepositoryState
	inputs       []freshness.CapturedInput
	confirmed    bool
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
	if err := m.StandaloneSource.validate(); err != nil {
		return err
	}
	repositoryDigest, err := m.RepositoryState.Digest()
	if err != nil {
		return fmt.Errorf("report manifest: repository digest: %w", err)
	}
	if !validManifestSHA256(m.RepositoryStateSHA256) || m.RepositoryStateSHA256 != repositoryDigest {
		return fmt.Errorf("report manifest: repository state sha256 mismatch")
	}
	if !validManifestSHA256(m.SnapshotSHA256) {
		return fmt.Errorf("report manifest: snapshot sha256 is invalid")
	}
	if !validManifestSHA256(m.ReportSHA256) {
		return fmt.Errorf("report manifest: report sha256 is invalid")
	}
	if m.ReportFormatVersion != CurrentFormatVersion {
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
	if m.MaterialInputs.SelectedRevision != m.RepositoryState.Head ||
		!validManifestLabel(m.MaterialInputs.InputPolicyVersion) ||
		m.MaterialInputs.ReportContract != m.ReportFormatVersion {
		return fmt.Errorf("report manifest: material inputs are invalid")
	}
	hasAnalysisTargetRef := m.MaterialInputs.AnalysisTargetRef != ""
	hasAnalysisTargetSHA256 := m.MaterialInputs.AnalysisTargetSHA256 != ""
	if hasAnalysisTargetRef != hasAnalysisTargetSHA256 {
		return fmt.Errorf("report manifest: analysis target ref and sha256 must both be present or absent")
	}
	if hasAnalysisTargetRef && !validManifestLabel(m.MaterialInputs.AnalysisTargetRef) {
		return fmt.Errorf("report manifest: analysis target ref is invalid")
	}
	if hasAnalysisTargetSHA256 && !validManifestSHA256(m.MaterialInputs.AnalysisTargetSHA256) {
		return fmt.Errorf("report manifest: analysis target sha256 is invalid")
	}
	hasProgramTargetID := m.MaterialInputs.ProgramTargetID != ""
	hasProgramTargetSHA256 := m.MaterialInputs.ProgramTargetSHA256 != ""
	if !hasProgramTargetID || !hasProgramTargetSHA256 {
		return fmt.Errorf("report manifest: program target id and sha256 are required")
	}
	if hasProgramTargetID && !validManifestLabel(m.MaterialInputs.ProgramTargetID) {
		return fmt.Errorf("report manifest: program target id is invalid")
	}
	if hasProgramTargetSHA256 && !validManifestSHA256(m.MaterialInputs.ProgramTargetSHA256) {
		return fmt.Errorf("report manifest: program target sha256 is invalid")
	}
	if !validManifestSHA256(m.MaterialInputs.ProgramIndexSetSHA256) {
		return fmt.Errorf("report manifest: program index set sha256 is invalid")
	}
	hasCubeMap := m.MaterialInputs.CubeMapSHA256 != ""
	if hasCubeMap && !validManifestSHA256(m.MaterialInputs.CubeMapSHA256) {
		return fmt.Errorf("report manifest: cube map sha256 is invalid")
	}
	if hasCubeMap && (!hasAnalysisTargetRef || !hasProgramTargetID) {
		return fmt.Errorf("report manifest: cube map requires exact analysis and program targets")
	}
	hasCoreMap := m.MaterialInputs.CoreMapSHA256 != ""
	if hasCoreMap && !validManifestSHA256(m.MaterialInputs.CoreMapSHA256) {
		return fmt.Errorf("report manifest: core map sha256 is invalid")
	}
	if hasCoreMap && (!hasProgramTargetID || hasCubeMap) {
		return fmt.Errorf("report manifest: core map requires one exact program target and no legacy cube map")
	}
	hasActivityEntrypoints := m.MaterialInputs.ActivityEntrypointsSHA256 != ""
	if hasActivityEntrypoints && !validManifestSHA256(m.MaterialInputs.ActivityEntrypointsSHA256) {
		return fmt.Errorf("report manifest: activity entrypoints sha256 is invalid")
	}
	if hasActivityEntrypoints != hasCoreMap {
		return fmt.Errorf("report manifest: core map and activity entrypoints must be bound together")
	}
	hasPythonTargetCatalog := m.MaterialInputs.PythonTargetCatalogSHA256 != ""
	if hasPythonTargetCatalog && !validManifestSHA256(m.MaterialInputs.PythonTargetCatalogSHA256) {
		return fmt.Errorf("report manifest: Python target catalog sha256 is invalid")
	}
	hasDeclaredDependencies := m.MaterialInputs.DeclaredDependenciesSHA256 != ""
	if hasDeclaredDependencies && !validManifestSHA256(m.MaterialInputs.DeclaredDependenciesSHA256) {
		return fmt.Errorf("report manifest: declared dependencies sha256 is invalid")
	}
	if hasDeclaredDependencies && (!hasCoreMap || !hasPythonTargetCatalog) {
		return fmt.Errorf("report manifest: declared dependencies require a core map and Python target catalog")
	}
	hasIntegrationDependencies := m.MaterialInputs.IntegrationDependenciesSHA256 != ""
	if hasIntegrationDependencies && !validManifestSHA256(m.MaterialInputs.IntegrationDependenciesSHA256) {
		return fmt.Errorf("report manifest: integration dependencies sha256 is invalid")
	}
	hasDependencyCatalog := m.MaterialInputs.DependencyCatalogSHA256 != ""
	if hasDependencyCatalog && !validManifestSHA256(m.MaterialInputs.DependencyCatalogSHA256) {
		return fmt.Errorf("report manifest: dependency catalog sha256 is invalid")
	}
	hasIntegrationUsage := m.MaterialInputs.IntegrationUsageSHA256 != ""
	if hasIntegrationUsage && !validManifestSHA256(m.MaterialInputs.IntegrationUsageSHA256) {
		return fmt.Errorf("report manifest: integration usage sha256 is invalid")
	}
	if hasIntegrationDependencies != hasCoreMap || hasDependencyCatalog != hasCoreMap ||
		hasIntegrationUsage != hasCoreMap {
		return fmt.Errorf("report manifest: core map, dependency catalog, integration dependencies, and integration usage must be bound together")
	}
	hasActivityPaths := m.MaterialInputs.ActivityPathsSHA256 != ""
	if hasActivityPaths && !validManifestSHA256(m.MaterialInputs.ActivityPathsSHA256) {
		return fmt.Errorf("report manifest: activity paths sha256 is invalid")
	}
	if hasActivityPaths != hasCoreMap {
		return fmt.Errorf("report manifest: semantic authority requires a bound activity path artifact")
	}
	if m.MaterialInputs.JSTSProjectSHA256 != "" &&
		!validManifestSHA256(m.MaterialInputs.JSTSProjectSHA256) {
		return fmt.Errorf("report manifest: JavaScript/TypeScript project sha256 is invalid")
	}
	if m.MaterialInputs.JSTSProjectSHA256 != "" && !hasCoreMap {
		return fmt.Errorf("report manifest: JavaScript/TypeScript project requires complete semantic authority")
	}
	if m.MaterialInputs.ReadmeFileRolesSHA256 != "" &&
		!validManifestSHA256(m.MaterialInputs.ReadmeFileRolesSHA256) {
		return fmt.Errorf("report manifest: README file-role sha256 is invalid")
	}
	hasTargetRunContainer := m.MaterialInputs.TargetRunContainerSHA256 != ""
	hasTargetPagePortfolio := m.MaterialInputs.TargetPagePortfolioSHA256 != ""
	hasProgramPagePortfolio := m.MaterialInputs.ProgramPagePortfolioSHA256 != ""
	hasRuntimePortfolio := m.MaterialInputs.RuntimePortfolioSHA256 != ""
	hasTargetOutcomePortfolio := m.MaterialInputs.TargetOutcomePortfolioSHA256 != ""
	if hasTargetRunContainer && !hasAnalysisTargetRef {
		return fmt.Errorf("report manifest: target run container requires one analysis target")
	}
	if hasTargetRunContainer && !validManifestSHA256(m.MaterialInputs.TargetRunContainerSHA256) {
		return fmt.Errorf("report manifest: target run container sha256 is invalid")
	}
	if hasTargetPagePortfolio && (!hasTargetRunContainer ||
		!validManifestSHA256(m.MaterialInputs.TargetPagePortfolioSHA256)) {
		return fmt.Errorf("report manifest: target page portfolio binding is invalid")
	}
	if hasProgramPagePortfolio && !validManifestSHA256(m.MaterialInputs.ProgramPagePortfolioSHA256) {
		return fmt.Errorf("report manifest: program page portfolio sha256 is invalid")
	}
	if hasProgramPagePortfolio && (hasTargetRunContainer || hasTargetPagePortfolio) {
		return fmt.Errorf("report manifest: program and Go target page authority are mutually exclusive")
	}
	if hasTargetOutcomePortfolio != hasProgramPagePortfolio ||
		hasTargetOutcomePortfolio && !validManifestSHA256(m.MaterialInputs.TargetOutcomePortfolioSHA256) {
		return fmt.Errorf("report manifest: target outcome portfolio binding is invalid")
	}
	if hasRuntimePortfolio != (hasTargetPagePortfolio || hasProgramPagePortfolio) ||
		hasRuntimePortfolio && !validManifestSHA256(m.MaterialInputs.RuntimePortfolioSHA256) {
		return fmt.Errorf("report manifest: runtime portfolio binding is invalid")
	}
	previousPath := ""
	for index, value := range m.OpenablePaths {
		if err := validateManifestPath(value); err != nil {
			return fmt.Errorf("report manifest: openable path %d: %w", index, err)
		}
		if previousPath != "" && value <= previousPath {
			return fmt.Errorf("report manifest: openable paths must be uniquely sorted")
		}
		previousPath = value
	}
	return nil
}

// ConfirmRunAuthorityScoped binds report source authority to exact captured
// inputs without recapturing or comparing the repository after analysis.
func ConfirmRunAuthorityScoped(
	ctx context.Context,
	analysisRoot string,
	initial freshness.RepositoryState,
	paths []string,
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
	repository := initial
	repository.Dirty = append([]freshness.DirtyFile(nil), initial.Dirty...)
	repository.Submodules = append([]freshness.SubmoduleState(nil), initial.Submodules...)
	return RunAuthority{
		analysisRoot: root, repository: repository, inputs: inputs,
		confirmed: true,
	}, nil
}

// ExtendRunAuthority adds report-relative evidence paths to an already
// confirmed authority using the original captured RepositoryState. Clean
// inputs are read from the captured commit and dirty inputs retain the hashes
// recorded at initial capture; this is not a later-state freshness gate.
func ExtendRunAuthority(
	ctx context.Context,
	authority RunAuthority,
	paths []string,
) (RunAuthority, error) {
	if err := authority.validate(); err != nil {
		return RunAuthority{}, err
	}
	repositoryPaths, err := repositoryRelativeInputPaths(
		authority.repository.Identity,
		authority.analysisRoot,
		paths,
	)
	if err != nil {
		return RunAuthority{}, err
	}
	additional, err := freshness.CaptureInputs(ctx, authority.repository, repositoryPaths)
	if err != nil {
		return RunAuthority{}, fmt.Errorf("report manifest: extend captured inputs: %w", err)
	}
	byPath := make(map[string]freshness.CapturedInput, len(authority.inputs)+len(additional))
	for _, input := range authority.inputs {
		byPath[input.Path] = input
	}
	for _, input := range additional {
		if existing, present := byPath[input.Path]; present && !reflect.DeepEqual(existing, input) {
			return RunAuthority{}, fmt.Errorf("report manifest: extended captured input %q changed identity", input.Path)
		}
		byPath[input.Path] = input
	}
	inputs := make([]freshness.CapturedInput, 0, len(byPath))
	for _, input := range byPath {
		inputs = append(inputs, input)
	}
	sort.Slice(inputs, func(left, right int) bool { return inputs[left].Path < inputs[right].Path })
	if _, err := freshness.CapturedInputsDigest(inputs); err != nil {
		return RunAuthority{}, fmt.Errorf("report manifest: extended captured inputs: %w", err)
	}
	result := authority
	result.inputs = inputs
	return result, nil
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
func CapturedInputPaths(data *ReportData) ([]string, error) {
	if data == nil {
		return nil, fmt.Errorf("report: captured input paths require report data")
	}
	if data.ProgramPortfolio == nil {
		return nil, fmt.Errorf("report: captured input paths require a program portfolio")
	}
	defaultEntry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return nil, fmt.Errorf("report: captured input paths: %w", err)
	}
	paths := append([]string(nil), data.OpenablePaths...)
	paths = append(paths, data.materialInputPaths...)
	goInputs := strings.EqualFold(defaultEntry.Target.Language, "go") ||
		strings.EqualFold(defaultEntry.Target.Language, "golang")
	if goInputs {
		paths = append(paths, "go.work", "go.work.sum")
	}
	sort.Strings(paths)
	compacted := paths[:0]
	for _, value := range paths {
		if value == "" || (len(compacted) > 0 && compacted[len(compacted)-1] == value) {
			continue
		}
		compacted = append(compacted, value)
	}
	return compacted, nil
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
	report, err := decodeStrictReportJSON(reportJSON)
	if err != nil {
		return fmt.Errorf("report manifest: %w", err)
	}
	return m.verifyReportData(report)
}

func (m RunManifest) verifyReportData(report ReportData) error {
	if err := validateProgramPresentation(&report); err != nil {
		return fmt.Errorf("report manifest: persisted report: %w", err)
	}
	if report.FormatVersion != m.ReportFormatVersion {
		return fmt.Errorf("report manifest: report format version mismatch")
	}
	if report.CapturedRevision == "" || report.CapturedRevision != m.RepositoryState.Head {
		return fmt.Errorf("report manifest: captured revision does not match repository authority")
	}
	if report.CapturedInputCount != len(m.CapturedInputs) {
		return fmt.Errorf("report manifest: captured input count does not match manifest authority")
	}
	if !reflect.DeepEqual(report.OpenablePaths, m.OpenablePaths) {
		return fmt.Errorf("report manifest: source authority does not match report")
	}
	material := m.MaterialInputs
	targetRef, targetSHA256, err := reportAnalysisTargetMaterial(report.AnalysisTarget)
	if err != nil {
		return fmt.Errorf("report manifest: report analysis target: %w", err)
	}
	if targetRef != material.AnalysisTargetRef || targetSHA256 != material.AnalysisTargetSHA256 {
		return fmt.Errorf("report manifest: analysis target identity does not match report")
	}
	if report.ProgramPortfolio == nil {
		return fmt.Errorf("report manifest: report program portfolio is missing")
	}
	defaultEntry, err := report.ProgramPortfolio.defaultEntry()
	if err != nil {
		return fmt.Errorf("report manifest: report program portfolio: %w", err)
	}
	programTargetID, programTargetSHA256, err := reportProgramTargetMaterial(&defaultEntry.Target)
	if err != nil {
		return fmt.Errorf("report manifest: report program target: %w", err)
	}
	if programTargetID != material.ProgramTargetID || programTargetSHA256 != material.ProgramTargetSHA256 {
		return fmt.Errorf("report manifest: program target identity does not match report")
	}
	if err := validateProgramSemanticPresentation(
		report.ProgramPortfolio,
		report.AnalysisTarget,
		report.CubeMapView,
		report.CoreMapView,
		report.ActivityEntrypointView,
		report.IntegrationUsageView,
		report.ActivityPathView,
		jstsSemanticPresentation{report.JSTSSurfaceCatalogView, report.CrossSurfacePathView},
	); err != nil {
		return fmt.Errorf("report manifest: %w", err)
	}
	hasCubeMap := material.CubeMapSHA256 != ""
	if (report.CubeMapView != nil) != hasCubeMap {
		return fmt.Errorf("report manifest: cube map identity does not match report projection")
	}
	if report.CubeMapView != nil {
		if report.AnalysisTarget == nil {
			return fmt.Errorf("report manifest: cube map projection lacks exact target authority")
		}
		if err := report.CubeMapView.Validate(); err != nil {
			return fmt.Errorf("report manifest: cube map view: %w", err)
		}
		if report.CubeMapView.Target.Ref != report.AnalysisTarget.Ref {
			return fmt.Errorf("report manifest: cube map target does not match report target")
		}
		if report.CubeMapView.ProgramTargetID != defaultEntry.Target.ID {
			return fmt.Errorf("report manifest: cube map does not bind the report program target")
		}
		if err := validateCubeMapProgramTarget(*report.AnalysisTarget, defaultEntry.Target); err != nil {
			return fmt.Errorf("report manifest: %w", err)
		}
	}
	hasCoreMap := material.CoreMapSHA256 != ""
	if (report.CoreMapView != nil) != hasCoreMap {
		return fmt.Errorf("report manifest: core map identity does not match report projection")
	}
	if report.CoreMapView != nil {
		if err := report.CoreMapView.Validate(); err != nil {
			return fmt.Errorf("report manifest: core map view: %w", err)
		}
		if report.CoreMapView.ProgramTargetID != defaultEntry.Target.ID ||
			report.CoreMapView.ProgramIndexSHA256 != defaultEntry.View.IndexSHA256 ||
			report.CoreMapView.IntegrationUsageSHA256 != material.IntegrationUsageSHA256 {
			return fmt.Errorf("report manifest: core map does not bind the report program and semantic inputs")
		}
	}
	hasActivityEntrypoints := material.ActivityEntrypointsSHA256 != ""
	if (report.ActivityEntrypointView != nil) != hasActivityEntrypoints {
		return fmt.Errorf("report manifest: activity entrypoint identity does not match report projection")
	}
	if report.ActivityEntrypointView != nil {
		if err := report.ActivityEntrypointView.Validate(); err != nil {
			return fmt.Errorf("report manifest: activity entrypoint view: %w", err)
		}
		if report.ActivityEntrypointView.ProgramTargetID != defaultEntry.Target.ID ||
			report.ActivityEntrypointView.ProgramIndexSHA256 != defaultEntry.View.IndexSHA256 {
			return fmt.Errorf("report manifest: activity entrypoints do not bind the report program target and index")
		}
	}
	hasIntegrationUsage := material.IntegrationUsageSHA256 != ""
	if (report.IntegrationUsageView != nil) != hasIntegrationUsage {
		return fmt.Errorf("report manifest: integration usage identity does not match report projection")
	}
	if report.IntegrationUsageView != nil {
		if err := report.IntegrationUsageView.Validate(); err != nil {
			return fmt.Errorf("report manifest: integration usage view: %w", err)
		}
		if report.IntegrationUsageView.ProgramTargetID != defaultEntry.Target.ID ||
			report.IntegrationUsageView.ProgramIndexSHA256 != defaultEntry.View.IndexSHA256 ||
			report.IntegrationUsageView.DependencyCatalogSHA256 != material.DependencyCatalogSHA256 ||
			report.IntegrationUsageView.IntegrationDependenciesSHA256 != material.IntegrationDependenciesSHA256 ||
			report.IntegrationUsageView.IntegrationUsageSHA256 != material.IntegrationUsageSHA256 {
			return fmt.Errorf("report manifest: integration usage does not bind the report program and semantic inputs")
		}
	}
	hasActivityPaths := material.ActivityPathsSHA256 != ""
	if (report.ActivityPathView != nil) != hasActivityPaths {
		return fmt.Errorf("report manifest: activity path identity does not match report projection")
	}
	if report.ActivityPathView != nil {
		if err := report.ActivityPathView.Validate(); err != nil {
			return fmt.Errorf("report manifest: activity path view: %w", err)
		}
		if report.ActivityPathView.ProgramTargetID != defaultEntry.Target.ID ||
			report.ActivityPathView.ProgramIndexSHA256 != defaultEntry.View.IndexSHA256 ||
			report.ActivityPathView.ActivityEntrypointsSHA256 != material.ActivityEntrypointsSHA256 ||
			report.ActivityPathView.IntegrationDependenciesSHA256 != material.IntegrationDependenciesSHA256 ||
			report.ActivityPathView.IntegrationUsageSHA256 != material.IntegrationUsageSHA256 ||
			report.ActivityPathView.ActivityPathsSHA256 != material.ActivityPathsSHA256 {
			return fmt.Errorf("report manifest: activity path does not bind the report program and semantic inputs")
		}
		if err := report.ActivityPathView.ValidateReportJoins(
			report.ActivityEntrypointView, report.IntegrationUsageView,
		); err != nil {
			return fmt.Errorf("report manifest: activity path report joins: %w", err)
		}
	}
	hasJSTSProject := material.JSTSProjectSHA256 != ""
	if (report.JSTSSurfaceCatalogView != nil) != hasJSTSProject ||
		(report.CrossSurfacePathView != nil) != hasJSTSProject {
		return fmt.Errorf("report manifest: JavaScript/TypeScript project identity does not match report projections")
	}
	if report.JSTSSurfaceCatalogView != nil {
		if err := report.JSTSSurfaceCatalogView.Validate(); err != nil {
			return fmt.Errorf("report manifest: JavaScript/TypeScript surface catalog view: %w", err)
		}
		if err := report.CrossSurfacePathView.Validate(); err != nil {
			return fmt.Errorf("report manifest: cross-surface path view: %w", err)
		}
		if report.JSTSSurfaceCatalogView.ProgramTargetID != defaultEntry.Target.ID ||
			report.JSTSSurfaceCatalogView.ProgramIndexSHA256 != defaultEntry.View.IndexSHA256 ||
			report.CrossSurfacePathView.ProgramTargetID != defaultEntry.Target.ID ||
			report.CrossSurfacePathView.ProgramIndexSHA256 != defaultEntry.View.IndexSHA256 ||
			report.JSTSSurfaceCatalogView.JSTSProjectSHA256 != report.CrossSurfacePathView.JSTSProjectSHA256 {
			return fmt.Errorf("report manifest: JavaScript/TypeScript views do not bind the report program and producer authority")
		}
	}
	hasRuntimePortfolio := material.RuntimePortfolioSHA256 != ""
	if (report.RuntimePortfolio != nil) != hasRuntimePortfolio {
		return fmt.Errorf("report manifest: runtime portfolio identity does not match report projection")
	}
	if report.RuntimePortfolio != nil {
		if err := report.RuntimePortfolio.Validate(); err != nil {
			return fmt.Errorf("report manifest: runtime portfolio view: %w", err)
		}
	}
	hasTargetOutcomePortfolio := material.TargetOutcomePortfolioSHA256 != ""
	if (report.TargetOutcomePortfolio != nil) != hasTargetOutcomePortfolio {
		return fmt.Errorf("report manifest: target outcome portfolio identity does not match report projection")
	}
	if report.TargetOutcomePortfolio != nil {
		if err := report.TargetOutcomePortfolio.Validate(); err != nil {
			return fmt.Errorf("report manifest: target outcome portfolio view: %w", err)
		}
	}
	return nil
}

func decodeStrictReportJSON(reportJSON []byte) (ReportData, error) {
	decoder := json.NewDecoder(bytes.NewReader(reportJSON))
	decoder.DisallowUnknownFields()
	var report ReportData
	if err := decoder.Decode(&report); err != nil {
		return ReportData{}, fmt.Errorf("decode report: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return ReportData{}, fmt.Errorf("report has multiple json values")
		}
		return ReportData{}, fmt.Errorf("report has trailing data: %w", err)
	}
	return report, nil
}

// manifestVerificationSuite owns one immutable verification view of a run.
// Every bounded file and decoded authority is memoized for the lifetime of the
// suite so the complete manifest verifier never re-reads or re-decodes a
// canonical artifact while checking its independent projections.
type manifestVerificationSuite struct {
	manifest   RunManifest
	runDir     string
	root       *os.Root
	reportJSON []byte

	files         map[string]*manifestVerificationFile
	decoded       map[string]any
	readCounts    map[string]int
	decodeCounts  map[string]int
	inventory     []string
	inventoryRead bool
}

type manifestVerificationFile struct {
	presenceKnown bool
	present       bool
	info          fs.FileInfo
	inspectErr    error
	read          bool
	released      bool
	data          []byte
	readErr       error
}

type manifestVerificationStats struct {
	FileReads map[string]int
	Decodes   map[string]int
}

func newManifestVerificationSuite(manifest RunManifest, runDir string) (*manifestVerificationSuite, error) {
	return newManifestVerificationSuiteWithValidation(manifest, runDir, true)
}

func newManifestVerificationSuiteWithValidation(
	manifest RunManifest,
	runDir string,
	validateComplete bool,
) (*manifestVerificationSuite, error) {
	if validateComplete {
		if err := manifest.Validate(); err != nil {
			return nil, err
		}
	} else if manifest.Version != CurrentRunManifestVersion {
		return nil, fmt.Errorf("report manifest: unsupported version %d", manifest.Version)
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return nil, fmt.Errorf("report manifest: open run directory: %w", err)
	}
	return &manifestVerificationSuite{
		manifest:     manifest,
		runDir:       runDir,
		root:         root,
		files:        make(map[string]*manifestVerificationFile),
		decoded:      make(map[string]any),
		readCounts:   make(map[string]int),
		decodeCounts: make(map[string]int),
	}, nil
}

func (suite *manifestVerificationSuite) Close() error {
	if suite == nil || suite.root == nil {
		return nil
	}
	err := suite.root.Close()
	suite.root = nil
	return err
}

func (suite *manifestVerificationSuite) stats() manifestVerificationStats {
	result := manifestVerificationStats{
		FileReads: make(map[string]int, len(suite.readCounts)),
		Decodes:   make(map[string]int, len(suite.decodeCounts)),
	}
	for name, count := range suite.readCounts {
		result.FileReads[name] = count
	}
	for name, count := range suite.decodeCounts {
		result.Decodes[name] = count
	}
	return result
}

func (suite *manifestVerificationSuite) cachedFile(name string) *manifestVerificationFile {
	file := suite.files[name]
	if file == nil {
		file = &manifestVerificationFile{}
		suite.files[name] = file
	}
	return file
}

func (suite *manifestVerificationSuite) inspectFile(name string) (*manifestVerificationFile, error) {
	file := suite.cachedFile(name)
	if file.presenceKnown {
		return file, file.inspectErr
	}
	file.presenceKnown = true
	info, err := suite.root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		file.present = false
		return file, nil
	}
	if err != nil {
		file.inspectErr = fmt.Errorf("report manifest: inspect %s: %w", name, err)
		return file, file.inspectErr
	}
	file.present = true
	file.info = info
	return file, nil
}

func (suite *manifestVerificationSuite) artifactPresent(name string) (bool, error) {
	file, err := suite.inspectFile(name)
	if err != nil {
		return false, err
	}
	return file.present, nil
}

func (suite *manifestVerificationSuite) readFile(name string, limit int) ([]byte, error) {
	file, err := suite.inspectFile(name)
	if err != nil {
		return nil, err
	}
	if !file.present {
		return nil, fmt.Errorf("report manifest: inspect %s: %w", name, fs.ErrNotExist)
	}
	if file.read {
		if file.readErr != nil {
			return nil, file.readErr
		}
		if file.released {
			return nil, fmt.Errorf("report manifest: internal released authority was read again: %s", name)
		}
		if len(file.data) > limit {
			return nil, fmt.Errorf("report manifest: %s exceeds %d bytes", name, limit)
		}
		return file.data, nil
	}
	file.read = true
	suite.readCounts[name]++
	if !file.info.Mode().IsRegular() {
		file.readErr = fmt.Errorf("report manifest: %s is not a regular file", name)
		return nil, file.readErr
	}
	if file.info.Size() < 0 || file.info.Size() > int64(limit) {
		file.readErr = fmt.Errorf("report manifest: %s exceeds %d bytes", name, limit)
		return nil, file.readErr
	}
	handle, err := suite.root.Open(name)
	if err != nil {
		file.readErr = fmt.Errorf("report manifest: open %s: %w", name, err)
		return nil, file.readErr
	}
	data, readErr := io.ReadAll(io.LimitReader(handle, int64(limit)+1))
	closeErr := handle.Close()
	if readErr != nil {
		file.readErr = fmt.Errorf("report manifest: read %s: %w", name, readErr)
		return nil, file.readErr
	}
	if closeErr != nil {
		file.readErr = fmt.Errorf("report manifest: close %s: %w", name, closeErr)
		return nil, file.readErr
	}
	if len(data) > limit {
		file.readErr = fmt.Errorf("report manifest: %s exceeds %d bytes", name, limit)
		return nil, file.readErr
	}
	file.data = data
	return data, nil
}

func (suite *manifestVerificationSuite) releaseFile(name string) {
	file := suite.files[name]
	if file == nil || !file.read || file.readErr != nil {
		return
	}
	file.data = nil
	file.released = true
}

func (suite *manifestVerificationSuite) programIndexInventory() ([]string, error) {
	if suite.inventoryRead {
		return append([]string(nil), suite.inventory...), nil
	}
	suite.inventoryRead = true
	entries, err := os.ReadDir(suite.runDir)
	if err != nil {
		return nil, fmt.Errorf("report manifest: list program index artifacts: %w", err)
	}
	names := make([]string, 0)
	for _, entry := range entries {
		name := entry.Name()
		if name == programindex.ArtifactSetFilename || name == programindex.ArtifactFilename ||
			name == jstsproject.ProgramIndexFilename ||
			strings.HasPrefix(name, "program-index.") && strings.HasSuffix(name, ".json") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	suite.inventory = names
	return append([]string(nil), names...), nil
}

func manifestDecodeCached[T any](
	suite *manifestVerificationSuite,
	key string,
	decode func() (T, error),
) (T, error) {
	if cached, ok := suite.decoded[key]; ok {
		value, valid := cached.(T)
		if !valid {
			var zero T
			return zero, fmt.Errorf("report manifest: internal decoded authority type mismatch for %s", key)
		}
		return value, nil
	}
	value, err := decode()
	if err != nil {
		var zero T
		return zero, err
	}
	suite.decodeCounts[key]++
	suite.decoded[key] = value
	return value, nil
}

func manifestDecodeBound[T any](
	suite *manifestVerificationSuite,
	key string,
	name string,
	limit int,
	wantSHA256 string,
	label string,
	decode func([]byte) (T, error),
) (T, bool, error) {
	var zero T
	present, err := suite.artifactPresent(name)
	if err != nil {
		return zero, false, err
	}
	if wantSHA256 == "" {
		if present {
			return zero, false, fmt.Errorf("report manifest: unbound %s artifact is present", label)
		}
		return zero, false, nil
	}
	if !present {
		return zero, false, fmt.Errorf("report manifest: bound %s artifact is missing", label)
	}
	value, err := manifestDecodeCached(suite, key, func() (T, error) {
		raw, readErr := suite.readFile(name, limit)
		if readErr != nil {
			return zero, readErr
		}
		if manifestSHA256(raw) != wantSHA256 {
			return zero, fmt.Errorf("report manifest: %s sha256 mismatch", label)
		}
		decoded, decodeErr := decode(raw)
		if decodeErr != nil {
			return zero, fmt.Errorf("report manifest: %s artifact: %w", label, decodeErr)
		}
		suite.releaseFile(name)
		return decoded, nil
	})
	if err != nil {
		return zero, false, err
	}
	return value, true, nil
}

func (suite *manifestVerificationSuite) reportBytes(reportJSON []byte) ([]byte, error) {
	if reportJSON != nil {
		if suite.reportJSON != nil && !bytes.Equal(suite.reportJSON, reportJSON) {
			return nil, fmt.Errorf("report manifest: verification suite received different report bytes")
		}
		suite.reportJSON = reportJSON
		return suite.reportJSON, nil
	}
	if suite.reportJSON != nil {
		return suite.reportJSON, nil
	}
	raw, err := suite.readFile("report.json", maxManifestReportBytes)
	if err != nil {
		return nil, err
	}
	suite.reportJSON = raw
	return raw, nil
}

func (suite *manifestVerificationSuite) report(reportJSON []byte) (ReportData, error) {
	return manifestDecodeCached(suite, "report.json", func() (ReportData, error) {
		var err error
		reportJSON, err = suite.reportBytes(reportJSON)
		if err != nil {
			return ReportData{}, err
		}
		if len(reportJSON) > maxManifestReportBytes {
			return ReportData{}, fmt.Errorf("report manifest: report exceeds %d bytes", maxManifestReportBytes)
		}
		report, err := decodeStrictReportJSON(reportJSON)
		if err != nil {
			return ReportData{}, fmt.Errorf("report manifest: %w", err)
		}
		suite.releaseFile("report.json")
		suite.reportJSON = nil
		return report, nil
	})
}

func (suite *manifestVerificationSuite) verifyReport(reportJSON []byte) error {
	reportJSON, err := suite.reportBytes(reportJSON)
	if err != nil {
		return err
	}
	if manifestSHA256(reportJSON) != suite.manifest.ReportSHA256 {
		return fmt.Errorf("report manifest: report sha256 mismatch")
	}
	report, err := suite.report(reportJSON)
	if err != nil {
		return err
	}
	return suite.manifest.verifyReportData(report)
}

type manifestProgramIndexes struct {
	set          programindex.ArtifactSet
	indexes      []programindex.Index
	byFile       map[string]programindex.Index
	defaultIndex programindex.Index
}

func (suite *manifestVerificationSuite) programIndexes() (manifestProgramIndexes, error) {
	return manifestDecodeCached(suite, "program-index-authority", func() (manifestProgramIndexes, error) {
		presentArtifacts, err := suite.programIndexInventory()
		if err != nil {
			return manifestProgramIndexes{}, err
		}
		setRaw, err := suite.readFile(programindex.ArtifactSetFilename, programindex.MaxArtifactSetBytes)
		if err != nil {
			return manifestProgramIndexes{}, err
		}
		if manifestSHA256(setRaw) != suite.manifest.MaterialInputs.ProgramIndexSetSHA256 {
			return manifestProgramIndexes{}, fmt.Errorf("report manifest: program index set sha256 mismatch")
		}
		set, err := programindex.DecodeArtifactSet(setRaw)
		if err != nil {
			return manifestProgramIndexes{}, fmt.Errorf("report manifest: program index set: %w", err)
		}
		suite.releaseFile(programindex.ArtifactSetFilename)
		if set.DefaultTargetID != suite.manifest.MaterialInputs.ProgramTargetID {
			return manifestProgramIndexes{}, fmt.Errorf("report manifest: program index set default target does not match report target")
		}
		allowedArtifacts := map[string]struct{}{programindex.ArtifactSetFilename: {}}
		for _, entry := range set.Entries {
			allowedArtifacts[entry.Filename] = struct{}{}
		}
		for _, name := range presentArtifacts {
			if _, allowed := allowedArtifacts[name]; !allowed {
				return manifestProgramIndexes{}, fmt.Errorf(
					"report manifest: program index artifact %s is not bound by the artifact set", name,
				)
			}
		}

		result := manifestProgramIndexes{
			set:     set,
			indexes: make([]programindex.Index, 0, len(set.Entries)),
			byFile:  make(map[string]programindex.Index, len(set.Entries)),
		}
		for _, entry := range set.Entries {
			index, alreadyVerified := result.byFile[entry.Filename]
			if !alreadyVerified {
				indexRaw, readErr := suite.readFile(entry.Filename, programindex.MaxIndexBytes)
				if readErr != nil {
					return manifestProgramIndexes{}, readErr
				}
				index, err = programindex.Decode(indexRaw)
				if err != nil {
					return manifestProgramIndexes{}, fmt.Errorf(
						"report manifest: program index %s: %w", entry.Filename, err,
					)
				}
				suite.releaseFile(entry.Filename)
				suite.decodeCounts["program-index:"+entry.Filename]++
				result.byFile[entry.Filename] = index
			}
			if index.SHA256 != entry.IndexSHA256 {
				return manifestProgramIndexes{}, fmt.Errorf(
					"report manifest: program index %s sha256 mismatch", entry.Filename,
				)
			}
			if index.Target.ID != entry.TargetID {
				return manifestProgramIndexes{}, fmt.Errorf(
					"report manifest: program index %s target id mismatch", entry.Filename,
				)
			}
			result.indexes = append(result.indexes, index)
			if entry.TargetID != set.DefaultTargetID {
				continue
			}
			targetID, targetSHA256, targetErr := reportProgramTargetMaterial(&index.Target)
			if targetErr != nil {
				return manifestProgramIndexes{}, fmt.Errorf("report manifest: default program target: %w", targetErr)
			}
			if targetID != suite.manifest.MaterialInputs.ProgramTargetID ||
				targetSHA256 != suite.manifest.MaterialInputs.ProgramTargetSHA256 {
				return manifestProgramIndexes{}, fmt.Errorf("report manifest: default program target identity mismatch")
			}
			result.defaultIndex = index
		}
		if result.defaultIndex.Target.ID == "" {
			return manifestProgramIndexes{}, fmt.Errorf("report manifest: core map default ProgramIndex is missing")
		}
		return result, nil
	})
}

// VerifyProgramIndexArtifacts binds the exact artifact-set bytes and every
// ProgramIndex selected by that set. The set is the only filename inventory:
// entries must resolve to bounded regular files, and each decoded index must
// retain both the advertised semantic seal and target identity.
func (m RunManifest) VerifyProgramIndexArtifacts(runDir string) error {
	suite, err := newManifestVerificationSuite(m, runDir)
	if err != nil {
		return err
	}
	defer suite.Close()
	_, err = suite.programIndexes()
	return err
}

// VerifyProgramPortfolioProjection re-derives the complete browser-facing
// ProgramPortfolio from every manifest-bound ProgramIndex. The report cannot
// drop a selected target, substitute another default, or independently join
// a seed to another object while retaining a valid report hash.
func (m RunManifest) VerifyProgramPortfolioProjection(runDir string, reportJSON []byte) error {
	suite, err := newManifestVerificationSuite(m, runDir)
	if err != nil {
		return err
	}
	defer suite.Close()
	return suite.verifyProgramPortfolioProjection(reportJSON)
}

func (suite *manifestVerificationSuite) verifyProgramPortfolioProjection(reportJSON []byte) error {
	report, err := suite.report(reportJSON)
	if err != nil {
		return fmt.Errorf("report manifest: program portfolio: %w", err)
	}
	if report.ProgramPortfolio == nil {
		return fmt.Errorf("report manifest: bound program portfolio is incomplete")
	}
	program, err := suite.programIndexes()
	if err != nil {
		return err
	}
	expected, err := NewProgramPortfolio(program.set.DefaultTargetID, program.indexes)
	if err != nil {
		return fmt.Errorf("report manifest: rederive program portfolio: %w", err)
	}
	if !reflect.DeepEqual(report.ProgramPortfolio, expected) {
		return fmt.Errorf("report manifest: program portfolio does not match the selected indexes")
	}
	return nil
}

// VerifyRuntimePortfolioProjection binds the repository-level semantic
// artifact, its target-page portfolio input, and the reduced report/browser
// projection. Artifact absence is valid only before target-page publication.
func (m RunManifest) VerifyRuntimePortfolioProjection(runDir string, reportJSON []byte) error {
	suite, err := newManifestVerificationSuite(m, runDir)
	if err != nil {
		return err
	}
	defer suite.Close()
	return suite.verifyRuntimePortfolioProjection(reportJSON)
}

func (suite *manifestVerificationSuite) verifyRuntimePortfolioProjection(reportJSON []byte) error {
	report, err := suite.report(reportJSON)
	if err != nil {
		return fmt.Errorf("report manifest: runtime portfolio view: %w", err)
	}
	wantSHA256 := suite.manifest.MaterialInputs.RuntimePortfolioSHA256
	present, err := suite.artifactPresent(runtimeportfolio.ArtifactFilename)
	if err != nil {
		return err
	}
	if wantSHA256 == "" {
		if present || report.RuntimePortfolio != nil {
			return fmt.Errorf("report manifest: unbound runtime portfolio artifact or projection is present")
		}
		return nil
	}
	if !present || report.RuntimePortfolio == nil {
		return fmt.Errorf("report manifest: bound runtime portfolio artifact or projection is missing")
	}
	result, _, err := manifestDecodeBound(
		suite,
		"runtime-portfolio",
		runtimeportfolio.ArtifactFilename,
		runtimeportfolio.MaxArtifactBytes,
		wantSHA256,
		"runtime portfolio",
		runtimeportfolio.Decode,
	)
	if err != nil {
		return err
	}
	if suite.manifest.MaterialInputs.ProgramPagePortfolioSHA256 != "" {
		portfolio, _, decodeErr := manifestDecodeBound(
			suite,
			"program-page-portfolio",
			programpage.ArtifactFilename,
			programpage.MaxArtifactBytes,
			suite.manifest.MaterialInputs.ProgramPagePortfolioSHA256,
			"program page portfolio",
			programpage.Decode,
		)
		if decodeErr != nil {
			return fmt.Errorf("report manifest: runtime portfolio program-page binding: %w", decodeErr)
		}
		if err := validateRuntimeProgramPageCoverage(result, portfolio); err != nil {
			return fmt.Errorf("report manifest: %w", err)
		}
	} else {
		portfolio, _, decodeErr := manifestDecodeBound(
			suite,
			"target-page-portfolio",
			snapshot.TargetPagePortfolioArtifactFilename,
			snapshot.MaxTargetPagePortfolioBytes,
			suite.manifest.MaterialInputs.TargetPagePortfolioSHA256,
			"target page portfolio",
			snapshot.DecodeTargetPagePortfolio,
		)
		if decodeErr != nil {
			return fmt.Errorf("report manifest: runtime portfolio target-page binding: %w", decodeErr)
		}
		if result.TargetPagePortfolioSHA256 != portfolio.SHA256 {
			return fmt.Errorf("report manifest: runtime portfolio target-page binding mismatch")
		}
		if len(result.Targets) != len(portfolio.Targets) {
			return fmt.Errorf("report manifest: runtime portfolio target coverage does not match target pages")
		}
	}
	if report.ProgramPortfolio == nil {
		return fmt.Errorf("report manifest: runtime portfolio current ProgramTarget is missing")
	}
	defaultEntry, err := report.ProgramPortfolio.defaultEntry()
	if err != nil {
		return fmt.Errorf("report manifest: runtime portfolio current ProgramTarget: %w", err)
	}
	if err := validateRuntimePortfolioCurrentTarget(result, defaultEntry.Target); err != nil {
		return fmt.Errorf("report manifest: %w", err)
	}
	expected, err := NewRuntimePortfolioView(result)
	if err != nil {
		return fmt.Errorf("report manifest: project runtime portfolio: %w", err)
	}
	if !reflect.DeepEqual(report.RuntimePortfolio, expected) {
		return fmt.Errorf("report manifest: runtime portfolio projection does not match artifact")
	}
	openable := make(map[string]struct{}, len(report.OpenablePaths))
	for _, sourcePath := range report.OpenablePaths {
		openable[sourcePath] = struct{}{}
	}
	for _, role := range expected.Roles {
		for _, evidence := range role.Evidence {
			if _, allowed := openable[evidence.Location.Path]; !allowed {
				return fmt.Errorf("report manifest: runtime portfolio evidence is outside source authority")
			}
		}
	}
	return nil
}

// VerifyTargetOutcomePortfolioProjection binds the complete selected-target
// outcome artifact, the exact set of analyzed program pages, and the reduced
// report/browser projection. The artifact is present exactly when neutral
// multi-selected page authority is present.
func (m RunManifest) VerifyTargetOutcomePortfolioProjection(runDir string, reportJSON []byte) error {
	suite, err := newManifestVerificationSuite(m, runDir)
	if err != nil {
		return err
	}
	defer suite.Close()
	return suite.verifyTargetOutcomePortfolioProjection(reportJSON)
}

func (suite *manifestVerificationSuite) verifyTargetOutcomePortfolioProjection(reportJSON []byte) error {
	report, err := suite.report(reportJSON)
	if err != nil {
		return fmt.Errorf("report manifest: target outcome portfolio view: %w", err)
	}
	wantSHA256 := suite.manifest.MaterialInputs.TargetOutcomePortfolioSHA256
	present, err := suite.artifactPresent(targetoutcome.ArtifactFilename)
	if err != nil {
		return err
	}
	if wantSHA256 == "" {
		if present || report.TargetOutcomePortfolio != nil {
			return fmt.Errorf("report manifest: unbound target outcome portfolio artifact or projection is present")
		}
		return nil
	}
	if !present || report.TargetOutcomePortfolio == nil {
		return fmt.Errorf("report manifest: bound target outcome portfolio artifact or projection is missing")
	}
	outcomes, _, err := manifestDecodeBound(
		suite,
		"target-outcome-portfolio",
		targetoutcome.ArtifactFilename,
		targetoutcome.MaxArtifactBytes,
		wantSHA256,
		"target outcome portfolio",
		targetoutcome.Decode,
	)
	if err != nil {
		return err
	}
	pages, _, err := manifestDecodeBound(
		suite,
		"program-page-portfolio",
		programpage.ArtifactFilename,
		programpage.MaxArtifactBytes,
		suite.manifest.MaterialInputs.ProgramPagePortfolioSHA256,
		"program page portfolio",
		programpage.Decode,
	)
	if err != nil {
		return fmt.Errorf("report manifest: target outcome program-page binding: %w", err)
	}
	expected, err := NewTargetOutcomePortfolioView(outcomes, pages)
	if err != nil {
		return fmt.Errorf("report manifest: project target outcome portfolio: %w", err)
	}
	if !reflect.DeepEqual(report.TargetOutcomePortfolio, expected) {
		return fmt.Errorf("report manifest: target outcome portfolio projection does not match artifacts")
	}
	return nil
}

func validateRuntimeProgramPageCoverage(
	result runtimeportfolio.Result,
	portfolio programpage.Portfolio,
) error {
	if result.TargetPagePortfolioSHA256 != portfolio.SHA256 ||
		len(result.Targets) != len(portfolio.Pages) {
		return fmt.Errorf("runtime portfolio program-page binding mismatch")
	}
	pages := make(map[string]programpage.Page, len(portfolio.Pages))
	for _, page := range portfolio.Pages {
		pages[page.Target.ID] = page
	}
	for _, target := range result.Targets {
		page, ok := pages[target.ProgramTargetID]
		if !ok || target.DisplayName != page.Target.Name ||
			target.Language != page.Target.Language || target.Kind != page.Target.Kind ||
			target.Selector != page.Target.Selector ||
			target.Default != (page.Target.ID == portfolio.DefaultTargetID) {
			return fmt.Errorf("runtime portfolio target coverage does not match program pages")
		}
	}
	return nil
}

// VerifyCubeMapProjection binds the complete saved semantic artifact and
// re-derives the browser projection. A missing artifact is a valid explicit
// absence; a present but unbound, malformed, or differently projected
// artifact is never ignored.
func (m RunManifest) VerifyCubeMapProjection(runDir string, reportJSON []byte) error {
	suite, err := newManifestVerificationSuite(m, runDir)
	if err != nil {
		return err
	}
	defer suite.Close()
	return suite.verifyCubeMapProjection(reportJSON)
}

func (suite *manifestVerificationSuite) verifyCubeMapProjection(reportJSON []byte) error {
	report, err := suite.report(reportJSON)
	if err != nil {
		return fmt.Errorf("report manifest: cube map view: %w", err)
	}
	wantSHA256 := suite.manifest.MaterialInputs.CubeMapSHA256
	value, present, err := manifestDecodeBound(
		suite,
		"cube-map",
		cubemap.ArtifactFilename,
		maxManifestReportBytes,
		wantSHA256,
		"cube map",
		cubemap.Decode,
	)
	if err != nil {
		return err
	}
	if wantSHA256 == "" {
		if present {
			return fmt.Errorf("report manifest: unbound cube map artifact is present")
		}
		if report.CubeMapView != nil {
			return fmt.Errorf("report manifest: unbound cube map view is present")
		}
		return nil
	}
	if !present {
		return fmt.Errorf("report manifest: bound cube map artifact is missing")
	}
	if report.CubeMapView == nil {
		return fmt.Errorf("report manifest: bound cube map view is missing")
	}
	if report.ProgramPortfolio == nil {
		return fmt.Errorf("report manifest: bound cube map lacks a program portfolio")
	}
	defaultEntry, err := report.ProgramPortfolio.defaultEntry()
	if err != nil {
		return fmt.Errorf("report manifest: bound cube map default target: %w", err)
	}
	expected, err := NewCubeMapView(value, defaultEntry.Target, defaultEntry.View.IndexSHA256)
	if err != nil {
		return fmt.Errorf("report manifest: rederive cube map view: %w", err)
	}
	if !reflect.DeepEqual(report.CubeMapView, expected) {
		return fmt.Errorf("report manifest: cube map view does not match cube map artifact")
	}
	return nil
}

// VerifyActivityEntrypointProjection binds the complete selected-callable
// artifact to the exact default ProgramIndex and re-derives the report view.
// Missing authority is valid only for targets whose semantic capability does
// not declare this separate cube; a present unbound artifact is never ignored.
func (m RunManifest) VerifyActivityEntrypointProjection(runDir string, reportJSON []byte) error {
	suite, err := newManifestVerificationSuite(m, runDir)
	if err != nil {
		return err
	}
	defer suite.Close()
	return suite.verifyActivityEntrypointProjection(reportJSON)
}

func (suite *manifestVerificationSuite) activityEntrypoints() (activityentrypoint.Result, bool, error) {
	wantSHA256 := suite.manifest.MaterialInputs.ActivityEntrypointsSHA256
	program, err := suite.programIndexes()
	if err != nil {
		return activityentrypoint.Result{}, false, err
	}
	return manifestDecodeBound(
		suite,
		"activity-entrypoints",
		activityentrypoint.ArtifactFilename,
		activityentrypoint.MaxArtifactBytes,
		wantSHA256,
		"activity entrypoint",
		func(raw []byte) (activityentrypoint.Result, error) {
			return activityentrypoint.Decode(raw, program.defaultIndex)
		},
	)
}

type manifestPythonSemantics struct {
	declarations dependencydeclaration.Result
	integrations integrationdependency.Result
	usage        integrationusage.Result
}

func (suite *manifestVerificationSuite) declaredDependencies() (dependencydeclaration.Result, error) {
	return manifestDecodeCached(suite, "declared-dependency-authority", func() (dependencydeclaration.Result, error) {
		declarationPresent, err := suite.artifactPresent(dependencydeclaration.ArtifactFilename)
		if err != nil {
			return dependencydeclaration.Result{}, err
		}
		targetPresent, err := suite.artifactPresent(pythontarget.ArtifactFilename)
		if err != nil {
			return dependencydeclaration.Result{}, err
		}
		wantTargetSHA256 := suite.manifest.MaterialInputs.PythonTargetCatalogSHA256
		wantDeclarationSHA256 := suite.manifest.MaterialInputs.DeclaredDependenciesSHA256
		if wantTargetSHA256 == "" {
			if targetPresent || declarationPresent || wantDeclarationSHA256 != "" {
				return dependencydeclaration.Result{}, fmt.Errorf(
					"report manifest: unbound Python target or declared dependency artifact is present",
				)
			}
			return dependencydeclaration.Result{}, nil
		}
		if !targetPresent {
			return dependencydeclaration.Result{}, fmt.Errorf(
				"report manifest: bound Python target catalog is missing",
			)
		}
		targets, _, err := manifestDecodeBound(
			suite,
			"python-target-catalog",
			pythontarget.ArtifactFilename,
			pythontarget.MaxArtifactBytes,
			wantTargetSHA256,
			"Python target catalog",
			pythontarget.DecodeCatalog,
		)
		if err != nil {
			return dependencydeclaration.Result{}, err
		}
		if wantDeclarationSHA256 == "" {
			if declarationPresent {
				return dependencydeclaration.Result{}, fmt.Errorf(
					"report manifest: unbound declared dependency artifact is present",
				)
			}
			return dependencydeclaration.Result{}, nil
		}
		if !declarationPresent {
			return dependencydeclaration.Result{}, fmt.Errorf(
				"report manifest: bound declared dependency artifact is missing",
			)
		}
		program, err := suite.programIndexes()
		if err != nil {
			return dependencydeclaration.Result{}, err
		}
		return manifestDecodeCached(suite, "declared-dependencies", func() (dependencydeclaration.Result, error) {
			raw, readErr := suite.readFile(
				dependencydeclaration.ArtifactFilename, dependencydeclaration.MaxArtifactBytes,
			)
			if readErr != nil {
				return dependencydeclaration.Result{}, readErr
			}
			if manifestSHA256(raw) != wantDeclarationSHA256 {
				return dependencydeclaration.Result{}, fmt.Errorf(
					"report manifest: declared dependencies sha256 mismatch",
				)
			}
			declarations, decodeErr := pythondeclareddependencies.DecodeTargetAuthority(
				raw, targets, program.defaultIndex,
			)
			if decodeErr != nil {
				return dependencydeclaration.Result{}, fmt.Errorf(
					"report manifest: declared dependency artifact: %w", decodeErr,
				)
			}
			suite.releaseFile(dependencydeclaration.ArtifactFilename)
			return declarations, nil
		})
	})
}

func (suite *manifestVerificationSuite) pythonSemantics() (manifestPythonSemantics, error) {
	return manifestDecodeCached(suite, "python-semantic-authority", func() (manifestPythonSemantics, error) {
		declarations, err := suite.declaredDependencies()
		if err != nil {
			return manifestPythonSemantics{}, err
		}
		catalogPresent, err := suite.artifactPresent(dependencies.ArtifactFilename)
		if err != nil {
			return manifestPythonSemantics{}, err
		}
		integrationPresent, err := suite.artifactPresent(integrationdependency.ArtifactFilename)
		if err != nil {
			return manifestPythonSemantics{}, err
		}
		usagePresent, err := suite.artifactPresent(integrationusage.ArtifactFilename)
		if err != nil {
			return manifestPythonSemantics{}, err
		}
		wantCatalogSHA256 := suite.manifest.MaterialInputs.DependencyCatalogSHA256
		wantIntegrationSHA256 := suite.manifest.MaterialInputs.IntegrationDependenciesSHA256
		wantUsageSHA256 := suite.manifest.MaterialInputs.IntegrationUsageSHA256
		if wantCatalogSHA256 == "" && wantIntegrationSHA256 == "" && wantUsageSHA256 == "" {
			if catalogPresent || integrationPresent || usagePresent {
				return manifestPythonSemantics{}, fmt.Errorf(
					"report manifest: unbound Python semantic artifact is present",
				)
			}
			return manifestPythonSemantics{declarations: declarations}, nil
		}
		if !catalogPresent || !integrationPresent || !usagePresent {
			return manifestPythonSemantics{}, fmt.Errorf(
				"report manifest: bound Python semantic artifact is missing",
			)
		}
		catalog, _, err := manifestDecodeBound(
			suite,
			"dependency-catalog",
			dependencies.ArtifactFilename,
			dependencies.MaxArtifactBytes,
			wantCatalogSHA256,
			"dependency catalog",
			dependencies.Decode,
		)
		if err != nil {
			return manifestPythonSemantics{}, err
		}
		integrations, _, err := manifestDecodeBound(
			suite,
			"integration-dependencies",
			integrationdependency.ArtifactFilename,
			integrationdependency.MaxArtifactBytes,
			wantIntegrationSHA256,
			"integration dependencies",
			integrationdependency.Decode,
		)
		if err != nil {
			return manifestPythonSemantics{}, err
		}
		program, err := suite.programIndexes()
		if err != nil {
			return manifestPythonSemantics{}, err
		}
		if program.defaultIndex.Target.Language == "python" {
			err = integrations.ValidateAgainstDeclarations(catalog, declarations, program.defaultIndex.Target)
		} else {
			err = integrations.ValidateAgainst(catalog)
		}
		if err != nil {
			return manifestPythonSemantics{}, fmt.Errorf(
				"report manifest: integration dependencies authority: %w", err,
			)
		}
		usage, _, err := manifestDecodeBound(
			suite,
			"integration-usage",
			integrationusage.ArtifactFilename,
			integrationusage.MaxArtifactBytes,
			wantUsageSHA256,
			"integration usage",
			integrationusage.Decode,
		)
		if err != nil {
			return manifestPythonSemantics{}, err
		}
		return manifestPythonSemantics{
			declarations: declarations,
			integrations: integrations,
			usage:        usage,
		}, nil
	})
}

func (suite *manifestVerificationSuite) verifyActivityEntrypointProjection(reportJSON []byte) error {
	report, err := suite.report(reportJSON)
	if err != nil {
		return fmt.Errorf("report manifest: activity entrypoint view: %w", err)
	}
	wantSHA256 := suite.manifest.MaterialInputs.ActivityEntrypointsSHA256
	result, present, err := suite.activityEntrypoints()
	if err != nil {
		return err
	}
	if wantSHA256 == "" {
		if present {
			return fmt.Errorf("report manifest: unbound activity entrypoint artifact is present")
		}
		if report.ActivityEntrypointView != nil {
			return fmt.Errorf("report manifest: unbound activity entrypoint view is present")
		}
		return nil
	}
	if !present {
		return fmt.Errorf("report manifest: bound activity entrypoint artifact is missing")
	}
	if report.ActivityEntrypointView == nil || report.ProgramPortfolio == nil {
		return fmt.Errorf("report manifest: bound activity entrypoint view or portfolio is missing")
	}
	defaultEntry, err := report.ProgramPortfolio.defaultEntry()
	if err != nil {
		return fmt.Errorf("report manifest: bound activity entrypoint default target: %w", err)
	}
	program, err := suite.programIndexes()
	if err != nil {
		return err
	}
	defaultIndex := program.defaultIndex
	if defaultIndex.Target.ID != defaultEntry.Target.ID ||
		defaultIndex.SHA256 != defaultEntry.View.IndexSHA256 {
		return fmt.Errorf("report manifest: activity entrypoint default ProgramIndex differs from report portfolio")
	}
	expected, err := NewActivityEntrypointView(result, defaultIndex)
	if err != nil {
		return fmt.Errorf("report manifest: rederive activity entrypoint view: %w", err)
	}
	if !reflect.DeepEqual(report.ActivityEntrypointView, expected) {
		return fmt.Errorf("report manifest: activity entrypoint view does not match activity artifact")
	}
	return nil
}

// VerifyActivityPathProjection binds the deterministic activity-path artifact
// to its exact ProgramIndex, ActivityEntrypoint, IntegrationDependency and
// IntegrationUsage inputs, then re-derives the compact report projection.
func (m RunManifest) VerifyActivityPathProjection(runDir string, reportJSON []byte) error {
	suite, err := newManifestVerificationSuite(m, runDir)
	if err != nil {
		return err
	}
	defer suite.Close()
	return suite.verifyActivityPathProjection(reportJSON)
}

func (suite *manifestVerificationSuite) activityPaths() (activitypath.Result, bool, error) {
	wantSHA256 := suite.manifest.MaterialInputs.ActivityPathsSHA256
	program, err := suite.programIndexes()
	if err != nil {
		return activitypath.Result{}, false, err
	}
	activities, _, err := suite.activityEntrypoints()
	if err != nil {
		return activitypath.Result{}, false, err
	}
	python, err := suite.pythonSemantics()
	if err != nil {
		return activitypath.Result{}, false, err
	}
	return manifestDecodeBound(
		suite,
		"activity-paths",
		activitypath.ArtifactFilename,
		activitypath.MaxArtifactBytes,
		wantSHA256,
		"activity path",
		func(raw []byte) (activitypath.Result, error) {
			return activitypath.Decode(
				raw,
				program.defaultIndex,
				activities,
				python.integrations,
				python.usage,
			)
		},
	)
}

func (suite *manifestVerificationSuite) verifyActivityPathProjection(reportJSON []byte) error {
	report, err := suite.report(reportJSON)
	if err != nil {
		return fmt.Errorf("report manifest: activity path view: %w", err)
	}
	wantSHA256 := suite.manifest.MaterialInputs.ActivityPathsSHA256
	result, present, err := suite.activityPaths()
	if err != nil {
		return err
	}
	if wantSHA256 == "" {
		if present {
			return fmt.Errorf("report manifest: unbound activity path artifact is present")
		}
		if report.ActivityPathView != nil {
			return fmt.Errorf("report manifest: unbound activity path view is present")
		}
		return nil
	}
	if !present {
		return fmt.Errorf("report manifest: bound activity path artifact is missing")
	}
	if report.ActivityPathView == nil || report.ActivityEntrypointView == nil ||
		report.IntegrationUsageView == nil || report.ProgramPortfolio == nil {
		return fmt.Errorf("report manifest: bound activity path report authority is incomplete")
	}
	defaultEntry, err := report.ProgramPortfolio.defaultEntry()
	if err != nil {
		return fmt.Errorf("report manifest: bound activity path default target: %w", err)
	}
	program, err := suite.programIndexes()
	if err != nil {
		return err
	}
	index := program.defaultIndex
	if index.Target.ID != defaultEntry.Target.ID || index.SHA256 != defaultEntry.View.IndexSHA256 {
		return fmt.Errorf("report manifest: activity path default ProgramIndex differs from report portfolio")
	}
	activities, _, err := suite.activityEntrypoints()
	if err != nil {
		return err
	}
	python, err := suite.pythonSemantics()
	if err != nil {
		return err
	}
	expected, err := NewActivityPathView(
		result, index, activities, python.integrations, python.usage,
	)
	if err != nil {
		return fmt.Errorf("report manifest: rederive activity path view: %w", err)
	}
	if err := expected.ValidateReportJoins(
		report.ActivityEntrypointView, report.IntegrationUsageView,
	); err != nil {
		return fmt.Errorf("report manifest: rederive activity path joins: %w", err)
	}
	if !reflect.DeepEqual(report.ActivityPathView, expected) {
		return fmt.Errorf("report manifest: activity path view does not match activity path artifact")
	}
	return nil
}

// VerifyJSTSProjection binds the single sealed deterministic JS/TS project
// artifact to both report projections and the exact default ProgramIndex.
// Either both views re-derive byte-for-byte from that authority or publication
// fails; the browser cannot fill a missing surface or path projection.
func (m RunManifest) VerifyJSTSProjection(runDir string, reportJSON []byte) error {
	suite, err := newManifestVerificationSuite(m, runDir)
	if err != nil {
		return err
	}
	defer suite.Close()
	return suite.verifyJSTSProjection(reportJSON)
}

func (suite *manifestVerificationSuite) jstsProject() (jstsproject.Result, bool, error) {
	return manifestDecodeBound(
		suite,
		"js-ts-project",
		jstsproject.ArtifactFilename,
		jstsproject.MaxArtifactBytes,
		suite.manifest.MaterialInputs.JSTSProjectSHA256,
		"JavaScript/TypeScript project",
		jstsproject.Decode,
	)
}

func (suite *manifestVerificationSuite) verifyJSTSProjection(reportJSON []byte) error {
	report, err := suite.report(reportJSON)
	if err != nil {
		return fmt.Errorf("report manifest: JavaScript/TypeScript views: %w", err)
	}
	result, present, err := suite.jstsProject()
	if err != nil {
		return err
	}
	wantSHA256 := suite.manifest.MaterialInputs.JSTSProjectSHA256
	if wantSHA256 == "" {
		if present {
			return fmt.Errorf("report manifest: unbound JavaScript/TypeScript project artifact is present")
		}
		if report.JSTSSurfaceCatalogView != nil || report.CrossSurfacePathView != nil {
			return fmt.Errorf("report manifest: unbound JavaScript/TypeScript report projection is present")
		}
		return nil
	}
	if !present {
		return fmt.Errorf("report manifest: bound JavaScript/TypeScript project artifact is missing")
	}
	if report.JSTSSurfaceCatalogView == nil || report.CrossSurfacePathView == nil || report.ProgramPortfolio == nil {
		return fmt.Errorf("report manifest: bound JavaScript/TypeScript report authority is incomplete")
	}
	defaultEntry, err := report.ProgramPortfolio.defaultEntry()
	if err != nil {
		return fmt.Errorf("report manifest: JavaScript/TypeScript default target: %w", err)
	}
	program, err := suite.programIndexes()
	if err != nil {
		return err
	}
	index := program.defaultIndex
	if index.Target.ID != defaultEntry.Target.ID || index.SHA256 != defaultEntry.View.IndexSHA256 {
		return fmt.Errorf("report manifest: JavaScript/TypeScript default ProgramIndex differs from report portfolio")
	}
	expectedSurfaces, err := NewJSTSSurfaceCatalogView(result, index)
	if err != nil {
		return fmt.Errorf("report manifest: rederive JavaScript/TypeScript surface catalog: %w", err)
	}
	expectedPaths, err := NewCrossSurfacePathView(result, index)
	if err != nil {
		return fmt.Errorf("report manifest: rederive cross-surface paths: %w", err)
	}
	if err := expectedPaths.ValidateSurfaceJoins(expectedSurfaces); err != nil {
		return fmt.Errorf("report manifest: rederive cross-surface joins: %w", err)
	}
	if !reflect.DeepEqual(report.JSTSSurfaceCatalogView, expectedSurfaces) ||
		!reflect.DeepEqual(report.CrossSurfacePathView, expectedPaths) {
		return fmt.Errorf("report manifest: JavaScript/TypeScript views do not match exact project artifact")
	}
	return nil
}

// VerifyCoreMapProjection binds the complete ProgramIndex-backed CoreMap, its
// optional README file-role authority, and re-derives the browser projection.
// Missing CoreMap authority is accepted only when neither the artifact nor the
// report view exists; the Python capability contract independently makes that
// absence terminal for a Python default.
func (m RunManifest) VerifyCoreMapProjection(runDir string, reportJSON []byte) error {
	suite, err := newManifestVerificationSuite(m, runDir)
	if err != nil {
		return err
	}
	defer suite.Close()
	return suite.verifyCoreMapProjection(reportJSON)
}

func (suite *manifestVerificationSuite) readmeFileRoles() (map[string]string, error) {
	return manifestDecodeCached(suite, "readme-file-roles", func() (map[string]string, error) {
		present, err := suite.artifactPresent(readmetargetscout.ArtifactFilename)
		if err != nil {
			return nil, err
		}
		wantSHA256 := suite.manifest.MaterialInputs.ReadmeFileRolesSHA256
		if wantSHA256 == "" {
			if present {
				return nil, fmt.Errorf("report manifest: unbound README file-role artifact is present")
			}
			return map[string]string{}, nil
		}
		if !present {
			return nil, fmt.Errorf("report manifest: bound README file-role artifact is missing")
		}
		raw, err := suite.readFile(
			readmetargetscout.ArtifactFilename, maxReadmeFileRoleArtifactBytes,
		)
		if err != nil {
			return nil, err
		}
		if manifestSHA256(raw) != wantSHA256 {
			return nil, fmt.Errorf("report manifest: README file-role sha256 mismatch")
		}
		files, err := decodeReadmeFileRoleAuthority(raw)
		if err != nil {
			return nil, fmt.Errorf("report manifest: %w", err)
		}
		suite.releaseFile(readmetargetscout.ArtifactFilename)
		return files, nil
	})
}

func (suite *manifestVerificationSuite) coreMap() (coremap.Result, bool, error) {
	return manifestDecodeBound(
		suite,
		"core-map",
		coremap.ArtifactFilename,
		coremap.MaxArtifactBytes,
		suite.manifest.MaterialInputs.CoreMapSHA256,
		"core map",
		coremap.Decode,
	)
}

func (suite *manifestVerificationSuite) verifyCoreMapProjection(reportJSON []byte) error {
	report, err := suite.report(reportJSON)
	if err != nil {
		return fmt.Errorf("report manifest: core map view: %w", err)
	}
	python, err := suite.pythonSemantics()
	if err != nil {
		return err
	}
	readmeFiles, err := suite.readmeFileRoles()
	if err != nil {
		return err
	}
	value, present, err := suite.coreMap()
	if err != nil {
		return err
	}
	wantSHA256 := suite.manifest.MaterialInputs.CoreMapSHA256
	if wantSHA256 == "" {
		if present {
			return fmt.Errorf("report manifest: unbound core map artifact is present")
		}
		if report.CoreMapView != nil {
			return fmt.Errorf("report manifest: unbound core map view is present")
		}
		return nil
	}
	if !present {
		return fmt.Errorf("report manifest: bound core map artifact is missing")
	}
	if report.CoreMapView == nil || report.ProgramPortfolio == nil {
		return fmt.Errorf("report manifest: bound core map view or portfolio is missing")
	}
	defaultEntry, err := report.ProgramPortfolio.defaultEntry()
	if err != nil {
		return fmt.Errorf("report manifest: bound core map default target: %w", err)
	}
	integrationUsageSHA256, err := python.usage.ArtifactSHA256()
	if err != nil {
		return fmt.Errorf("report manifest: integration usage identity: %w", err)
	}
	if value.IntegrationUsageSHA256 == "" ||
		value.IntegrationUsageSHA256 != integrationUsageSHA256 {
		return fmt.Errorf("report manifest: core map integration usage authority mismatch")
	}
	if report.CoreMapView.IntegrationUsageSHA256 != integrationUsageSHA256 {
		return fmt.Errorf("report manifest: core map view integration usage authority mismatch")
	}
	program, err := suite.programIndexes()
	if err != nil {
		return err
	}
	defaultIndex := program.defaultIndex
	if defaultIndex.Target.ID != defaultEntry.Target.ID || defaultIndex.SHA256 != defaultEntry.View.IndexSHA256 {
		return fmt.Errorf("report manifest: core map default ProgramIndex differs from report portfolio")
	}
	if err := python.usage.ValidateAgainst(defaultIndex, python.integrations); err != nil {
		return fmt.Errorf("report manifest: integration usage authority: %w", err)
	}
	expectedIntegrationUsage, err := NewIntegrationUsageView(
		python.usage, defaultIndex, python.integrations,
	)
	if err != nil {
		return fmt.Errorf("report manifest: rederive integration usage view: %w", err)
	}
	if !reflect.DeepEqual(report.IntegrationUsageView, expectedIntegrationUsage) {
		return fmt.Errorf("report manifest: integration usage view does not match integration usage artifact")
	}
	expected, err := NewCoreMapView(value, defaultIndex, readmeFiles)
	if err != nil {
		return fmt.Errorf("report manifest: rederive core map view: %w", err)
	}
	if !reflect.DeepEqual(report.CoreMapView, expected) {
		return fmt.Errorf("report manifest: core map view does not match core map artifact")
	}
	return nil
}

func readManifestDefaultProgramIndex(root *os.Root, setSHA256 string) (programindex.Index, error) {
	setRaw, err := readManifestFile(root, programindex.ArtifactSetFilename, programindex.MaxArtifactSetBytes)
	if err != nil {
		return programindex.Index{}, err
	}
	if manifestSHA256(setRaw) != setSHA256 {
		return programindex.Index{}, fmt.Errorf("report manifest: core map program index set sha256 mismatch")
	}
	set, err := programindex.DecodeArtifactSet(setRaw)
	if err != nil {
		return programindex.Index{}, fmt.Errorf("report manifest: core map program index set: %w", err)
	}
	for _, entry := range set.Entries {
		if entry.TargetID != set.DefaultTargetID {
			continue
		}
		raw, readErr := readManifestFile(root, entry.Filename, programindex.MaxIndexBytes)
		if readErr != nil {
			return programindex.Index{}, readErr
		}
		index, decodeErr := programindex.Decode(raw)
		if decodeErr != nil {
			return programindex.Index{}, fmt.Errorf("report manifest: core map default ProgramIndex: %w", decodeErr)
		}
		if index.Target.ID != entry.TargetID || index.SHA256 != entry.IndexSHA256 {
			return programindex.Index{}, fmt.Errorf("report manifest: core map default ProgramIndex binding mismatch")
		}
		return index, nil
	}
	return programindex.Index{}, fmt.Errorf("report manifest: core map default ProgramIndex is missing")
}

// VerifySnapshotArtifact binds the exact saved snapshot bytes used to derive
// the report and its private replay inputs. It must run before any verifier
// that rehydrates semantic authority from snapshot.json.
func (m RunManifest) VerifySnapshotArtifact(runDir string) error {
	suite, err := newManifestVerificationSuite(m, runDir)
	if err != nil {
		return err
	}
	defer suite.Close()
	return suite.verifySnapshotArtifact()
}

func (suite *manifestVerificationSuite) verifySnapshotArtifact() error {
	snapshotJSON, err := suite.readFile("snapshot.json", maxManifestSnapshotBytes)
	if err != nil {
		return err
	}
	if manifestSHA256(snapshotJSON) != suite.manifest.SnapshotSHA256 {
		return fmt.Errorf("report manifest: snapshot sha256 mismatch")
	}
	suite.releaseFile("snapshot.json")
	return nil
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

// VerifiedRunReceipt is a small transaction-local proof that one exact run
// directory passed the complete canonical manifest verification suite. It
// deliberately retains no decoded report or semantic artifact and must not be
// persisted as a replacement authority.
type VerifiedRunReceipt struct {
	runDir   string
	manifest RunManifest
}

// ReadVerifiedRunManifest verifies one run once and returns a compact receipt
// that downstream work in the same transaction can pass instead of invoking
// the semantic verifier again.
func ReadVerifiedRunManifest(runDir string) (VerifiedRunReceipt, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return VerifiedRunReceipt{}, fmt.Errorf("report manifest: open run directory: %w", err)
	}
	defer root.Close()

	manifestJSON, err := readManifestFile(root, RunManifestFilename, maxRunManifestBytes)
	if err != nil {
		return VerifiedRunReceipt{}, err
	}
	manifest, err := DecodeRunManifest(manifestJSON)
	if err != nil {
		return VerifiedRunReceipt{}, err
	}
	if _, err := verifyCompleteRunManifest(runDir, manifest, nil); err != nil {
		return VerifiedRunReceipt{}, err
	}
	absoluteRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return VerifiedRunReceipt{}, fmt.Errorf("report manifest: resolve verified run directory: %w", err)
	}
	return VerifiedRunReceipt{
		runDir:   filepath.Clean(absoluteRunDir),
		manifest: cloneRunManifest(manifest),
	}, nil
}

// ReadRunManifest reads and verifies run_manifest.json and report.json from a
// run directory. Both files must be bounded, regular files (not symlinks).
func ReadRunManifest(runDir string) (RunManifest, error) {
	receipt, err := ReadVerifiedRunManifest(runDir)
	if err != nil {
		return RunManifest{}, err
	}
	return receipt.Manifest(), nil
}

// Manifest returns an isolated copy of the verified manifest authority.
func (receipt VerifiedRunReceipt) Manifest() RunManifest {
	return cloneRunManifest(receipt.manifest)
}

// RunDir returns the clean absolute run directory bound to the receipt.
func (receipt VerifiedRunReceipt) RunDir() string {
	return receipt.runDir
}

// ReportSHA256 returns the exact canonical report identity already verified.
func (receipt VerifiedRunReceipt) ReportSHA256() string {
	return receipt.manifest.ReportSHA256
}

// ProgramPagePortfolioSHA256 returns the exact neutral page authority already
// verified for the run, or an empty string for a pre-portfolio run.
func (receipt VerifiedRunReceipt) ProgramPagePortfolioSHA256() string {
	return receipt.manifest.MaterialInputs.ProgramPagePortfolioSHA256
}

// ProgramTargetID returns the exact target identity bound to the verified run.
func (receipt VerifiedRunReceipt) ProgramTargetID() string {
	return receipt.manifest.MaterialInputs.ProgramTargetID
}

func cloneRunManifest(manifest RunManifest) RunManifest {
	result := manifest
	result.RepositoryState.Dirty = append(
		[]freshness.DirtyFile(nil), manifest.RepositoryState.Dirty...,
	)
	result.RepositoryState.Submodules = append(
		[]freshness.SubmoduleState(nil), manifest.RepositoryState.Submodules...,
	)
	if manifest.StandaloneSource != nil {
		source := *manifest.StandaloneSource
		result.StandaloneSource = &source
	}
	result.OpenablePaths = append([]string(nil), manifest.OpenablePaths...)
	result.CapturedInputs = append([]freshness.CapturedInput(nil), manifest.CapturedInputs...)
	for index := range result.CapturedInputs {
		result.CapturedInputs[index].Stages = append(
			[]string(nil), manifest.CapturedInputs[index].Stages...,
		)
	}
	return result
}

func verifyCompleteRunManifest(
	runDir string,
	manifest RunManifest,
	reportJSON []byte,
) (manifestVerificationStats, error) {
	suite, err := newManifestVerificationSuite(manifest, runDir)
	if err != nil {
		return manifestVerificationStats{}, err
	}
	defer suite.Close()
	if err := suite.verifyReport(reportJSON); err != nil {
		return suite.stats(), err
	}
	if _, err := suite.programIndexes(); err != nil {
		return suite.stats(), err
	}
	if err := suite.verifyProgramPortfolioProjection(reportJSON); err != nil {
		return suite.stats(), err
	}
	if err := suite.verifyRuntimePortfolioProjection(reportJSON); err != nil {
		return suite.stats(), err
	}
	if err := suite.verifyTargetOutcomePortfolioProjection(reportJSON); err != nil {
		return suite.stats(), err
	}
	if err := suite.verifyCubeMapProjection(reportJSON); err != nil {
		return suite.stats(), err
	}
	if err := suite.verifyActivityEntrypointProjection(reportJSON); err != nil {
		return suite.stats(), err
	}
	if err := suite.verifyActivityPathProjection(reportJSON); err != nil {
		return suite.stats(), err
	}
	if err := suite.verifyJSTSProjection(reportJSON); err != nil {
		return suite.stats(), err
	}
	if err := suite.verifyCoreMapProjection(reportJSON); err != nil {
		return suite.stats(), err
	}
	if err := suite.verifySnapshotArtifact(); err != nil {
		return suite.stats(), err
	}
	if err := suite.verifyTargetPagePortfolioArtifacts(); err != nil {
		return suite.stats(), err
	}
	if err := suite.verifyProgramPagePortfolioArtifact(); err != nil {
		return suite.stats(), err
	}
	return suite.stats(), nil
}

// VerifyTargetPagePortfolioArtifacts binds the exact selected-target
// container and, when publication has completed, the shared sibling-page
// portfolio. A container-only manifest is valid for the initial/default or
// single-target publication; a portfolio can never exist without it.
func (m RunManifest) VerifyTargetPagePortfolioArtifacts(runDir string) error {
	suite, err := newManifestVerificationSuiteWithValidation(m, runDir, false)
	if err != nil {
		return err
	}
	defer suite.Close()
	return suite.verifyTargetPagePortfolioArtifacts()
}

func (suite *manifestVerificationSuite) verifyTargetPagePortfolioArtifacts() error {
	containerPresent, err := suite.artifactPresent(snapshot.TargetRunContainerArtifactFilename)
	if err != nil {
		return err
	}
	portfolioPresent, err := suite.artifactPresent(snapshot.TargetPagePortfolioArtifactFilename)
	if err != nil {
		return err
	}
	if suite.manifest.MaterialInputs.TargetRunContainerSHA256 == "" && containerPresent {
		return fmt.Errorf(
			"report manifest: unbound target page artifact %s is present",
			snapshot.TargetRunContainerArtifactFilename,
		)
	}
	if suite.manifest.MaterialInputs.TargetPagePortfolioSHA256 == "" && portfolioPresent {
		return fmt.Errorf(
			"report manifest: unbound target page artifact %s is present",
			snapshot.TargetPagePortfolioArtifactFilename,
		)
	}
	if suite.manifest.MaterialInputs.TargetRunContainerSHA256 != "" && !containerPresent {
		return fmt.Errorf(
			"report manifest: target page artifact %s sha256 mismatch",
			snapshot.TargetRunContainerArtifactFilename,
		)
	}
	if suite.manifest.MaterialInputs.TargetPagePortfolioSHA256 != "" && !portfolioPresent {
		return fmt.Errorf(
			"report manifest: target page artifact %s sha256 mismatch",
			snapshot.TargetPagePortfolioArtifactFilename,
		)
	}
	container, containerPresent, err := manifestDecodeBound(
		suite,
		"target-run-container",
		snapshot.TargetRunContainerArtifactFilename,
		snapshot.MaxTargetRunContainerBytes,
		suite.manifest.MaterialInputs.TargetRunContainerSHA256,
		"target page artifact "+snapshot.TargetRunContainerArtifactFilename,
		snapshot.DecodeTargetRunContainer,
	)
	if err != nil {
		return err
	}
	portfolio, portfolioPresent, err := manifestDecodeBound(
		suite,
		"target-page-portfolio",
		snapshot.TargetPagePortfolioArtifactFilename,
		snapshot.MaxTargetPagePortfolioBytes,
		suite.manifest.MaterialInputs.TargetPagePortfolioSHA256,
		"target page artifact "+snapshot.TargetPagePortfolioArtifactFilename,
		snapshot.DecodeTargetPagePortfolio,
	)
	if err != nil {
		return err
	}
	if !containerPresent {
		if portfolioPresent {
			return fmt.Errorf("report manifest: target page portfolio lacks container authority")
		}
		return nil
	}
	if !portfolioPresent {
		return nil
	}
	if err := portfolio.ValidateAgainstContainer(container); err != nil {
		return fmt.Errorf("report manifest: target page portfolio: %w", err)
	}
	for _, page := range portfolio.Targets {
		if page.TargetRef == suite.manifest.MaterialInputs.AnalysisTargetRef &&
			page.RunID == filepath.Base(suite.runDir) {
			return nil
		}
	}
	return fmt.Errorf("report manifest: analysis target has no exact published target page")
}

// VerifyProgramPagePortfolioArtifact binds the language-neutral page
// portfolio and proves that this exact run publishes the manifest's current
// ProgramTarget. The neutral authority is mutually exclusive with the legacy
// Go target container/page pair in MaterialInputs.Validate.
func (m RunManifest) VerifyProgramPagePortfolioArtifact(runDir string) error {
	suite, err := newManifestVerificationSuiteWithValidation(m, runDir, false)
	if err != nil {
		return err
	}
	defer suite.Close()
	return suite.verifyProgramPagePortfolioArtifact()
}

func (suite *manifestVerificationSuite) verifyProgramPagePortfolioArtifact() error {
	wantSHA256 := suite.manifest.MaterialInputs.ProgramPagePortfolioSHA256
	if wantSHA256 == "" {
		present, err := suite.artifactPresent(programpage.ArtifactFilename)
		if err != nil {
			return err
		}
		if present {
			return fmt.Errorf("report manifest: unbound program page portfolio artifact is present")
		}
		return nil
	}
	portfolio, _, err := manifestDecodeBound(
		suite,
		"program-page-portfolio",
		programpage.ArtifactFilename,
		programpage.MaxArtifactBytes,
		wantSHA256,
		"program page portfolio",
		programpage.Decode,
	)
	if err != nil {
		return err
	}
	runID := filepath.Base(suite.runDir)
	for _, page := range portfolio.Pages {
		if page.Target.ID == suite.manifest.MaterialInputs.ProgramTargetID && page.RunID == runID {
			return nil
		}
	}
	return fmt.Errorf("report manifest: ProgramTarget has no exact published program page")
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
	return nil
}

func prepareAuthorizedRunManifest(
	runDir string,
	data *ReportData,
	reportJSON []byte,
	authority RunAuthority,
	standaloneSource *standaloneSourceConfig,
) (RunManifest, error) {
	if err := authority.validate(); err != nil {
		return RunManifest{}, err
	}
	repositoryDigest, err := authority.repository.Digest()
	if err != nil {
		return RunManifest{}, fmt.Errorf("report manifest: digest repository state: %w", err)
	}
	snapshotJSON, err := readRunSnapshot(runDir)
	if err != nil {
		return RunManifest{}, err
	}
	analysisTargetRef, analysisTargetDigest, err := reportAnalysisTargetMaterial(data.AnalysisTarget)
	if err != nil {
		return RunManifest{}, fmt.Errorf("report manifest: analysis target: %w", err)
	}
	if data.ProgramPortfolio == nil {
		return RunManifest{}, fmt.Errorf("report manifest: program portfolio is missing")
	}
	defaultEntry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return RunManifest{}, fmt.Errorf("report manifest: program portfolio: %w", err)
	}
	programTargetID, programTargetDigest, err := reportProgramTargetMaterial(&defaultEntry.Target)
	if err != nil {
		return RunManifest{}, fmt.Errorf("report manifest: program target: %w", err)
	}
	programIndexSetDigest, err := savedProgramIndexSetSHA256(runDir)
	if err != nil {
		return RunManifest{}, err
	}
	cubeMapDigest, err := savedCubeMapSHA256(runDir)
	if err != nil {
		return RunManifest{}, err
	}
	coreMapDigest, err := savedCoreMapSHA256(runDir)
	if err != nil {
		return RunManifest{}, err
	}
	activityEntrypointsDigest, err := savedActivityEntrypointsSHA256(runDir, programIndexSetDigest)
	if err != nil {
		return RunManifest{}, err
	}
	pythonTargetCatalogDigest, declaredDependenciesDigest, err :=
		savedDeclaredDependencyArtifactsSHA256(runDir, programIndexSetDigest)
	if err != nil {
		return RunManifest{}, err
	}
	dependencyCatalogDigest, integrationDependenciesDigest, integrationUsageDigest, err :=
		savedPythonSemanticArtifactsSHA256(runDir, programIndexSetDigest)
	if err != nil {
		return RunManifest{}, err
	}
	activityPathsDigest, err := savedActivityPathsSHA256(runDir, programIndexSetDigest)
	if err != nil {
		return RunManifest{}, err
	}
	jstsProjectDigest, err := savedJSTSProjectSHA256(runDir, programIndexSetDigest)
	if err != nil {
		return RunManifest{}, err
	}
	readmeFileRolesDigest, err := savedReadmeFileRolesSHA256(runDir)
	if err != nil {
		return RunManifest{}, err
	}
	runtimePortfolioDigest, err := savedRuntimePortfolioSHA256(runDir)
	if err != nil {
		return RunManifest{}, err
	}
	inputs := append([]freshness.CapturedInput(nil), authority.inputs...)
	if inputs == nil {
		capturedPaths, pathErr := CapturedInputPaths(data)
		if pathErr != nil {
			return RunManifest{}, pathErr
		}
		paths, err := repositoryRelativeInputPaths(
			authority.repository.Identity,
			authority.analysisRoot,
			capturedPaths,
		)
		if err != nil {
			return RunManifest{}, err
		}
		inputs, err = freshness.CaptureInputs(context.Background(), authority.repository, paths)
		if err != nil {
			return RunManifest{}, err
		}
	}
	inputsDigest, err := freshness.CapturedInputsDigest(inputs)
	if err != nil {
		return RunManifest{}, err
	}
	var sourceAuthority *StandaloneSourceAuthority
	if standaloneSource != nil {
		sourceAuthority = &StandaloneSourceAuthority{
			Host:          standaloneSource.hostName,
			RepositoryURL: standaloneSource.repositoryURL,
		}
		if err := sourceAuthority.validate(); err != nil {
			return RunManifest{}, err
		}
	}
	manifest := RunManifest{
		Version:               CurrentRunManifestVersion,
		RepositoryState:       authority.repository,
		AnalysisRoot:          authority.analysisRoot,
		StandaloneSource:      sourceAuthority,
		RepositoryStateSHA256: repositoryDigest,
		SnapshotSHA256:        manifestSHA256(snapshotJSON),
		ReportSHA256:          manifestSHA256(reportJSON),
		ReportFormatVersion:   data.FormatVersion,
		OpenablePaths:         append([]string(nil), data.OpenablePaths...),
		CapturedInputs:        inputs,
		CapturedInputsSHA256:  inputsDigest,
		MaterialInputs: MaterialInputs{
			SelectedRevision:              authority.repository.Head,
			AnalysisTargetRef:             analysisTargetRef,
			AnalysisTargetSHA256:          analysisTargetDigest,
			ProgramTargetID:               programTargetID,
			ProgramTargetSHA256:           programTargetDigest,
			ProgramIndexSetSHA256:         programIndexSetDigest,
			CubeMapSHA256:                 cubeMapDigest,
			CoreMapSHA256:                 coreMapDigest,
			ActivityEntrypointsSHA256:     activityEntrypointsDigest,
			PythonTargetCatalogSHA256:     pythonTargetCatalogDigest,
			DeclaredDependenciesSHA256:    declaredDependenciesDigest,
			DependencyCatalogSHA256:       dependencyCatalogDigest,
			IntegrationDependenciesSHA256: integrationDependenciesDigest,
			IntegrationUsageSHA256:        integrationUsageDigest,
			ActivityPathsSHA256:           activityPathsDigest,
			JSTSProjectSHA256:             jstsProjectDigest,
			ReadmeFileRolesSHA256:         readmeFileRolesDigest,
			TargetRunContainerSHA256:      savedArtifactSHA256(runDir, snapshot.TargetRunContainerArtifactFilename),
			TargetPagePortfolioSHA256:     savedArtifactSHA256(runDir, snapshot.TargetPagePortfolioArtifactFilename),
			ProgramPagePortfolioSHA256:    savedArtifactSHA256(runDir, programpage.ArtifactFilename),
			RuntimePortfolioSHA256:        runtimePortfolioDigest,
			TargetOutcomePortfolioSHA256:  savedArtifactSHA256(runDir, targetoutcome.ArtifactFilename),
			InputPolicyVersion:            "captured-inputs-v1",
			ReportContract:                data.FormatVersion,
		},
	}
	if _, err := verifyCompleteRunManifest(runDir, manifest, reportJSON); err != nil {
		return RunManifest{}, err
	}
	return manifest, nil
}

func reportAnalysisTargetBinding(target *analysistarget.Target) (string, string, error) {
	if target == nil {
		return "", "", fmt.Errorf("analysis target is missing")
	}
	canonical, err := target.CanonicalJSON()
	if err != nil {
		return "", "", err
	}
	return target.Ref, manifestSHA256(canonical), nil
}

func reportAnalysisTargetMaterial(target *analysistarget.Target) (string, string, error) {
	if target != nil {
		return reportAnalysisTargetBinding(target)
	}
	return "", "", nil
}

func reportProgramTargetMaterial(target *programindex.Target) (string, string, error) {
	if target == nil {
		return "", "", nil
	}
	if err := target.Validate(); err != nil {
		return "", "", err
	}
	canonical, err := json.Marshal(target.Snapshot())
	if err != nil {
		return "", "", fmt.Errorf("encode program target: %w", err)
	}
	return target.ID, manifestSHA256(canonical), nil
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

func savedRuntimePortfolioSHA256(runDir string) (string, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return "", fmt.Errorf("report manifest: open runtime portfolio run: %w", err)
	}
	defer root.Close()
	_, runtimeErr := root.Lstat(runtimeportfolio.ArtifactFilename)
	_, targetPortfolioErr := root.Lstat(snapshot.TargetPagePortfolioArtifactFilename)
	_, programPortfolioErr := root.Lstat(programpage.ArtifactFilename)
	runtimePresent := runtimeErr == nil
	targetPortfolioPresent := targetPortfolioErr == nil
	programPortfolioPresent := programPortfolioErr == nil
	if runtimeErr != nil && !errors.Is(runtimeErr, fs.ErrNotExist) {
		return "", fmt.Errorf("report manifest: inspect runtime portfolio: %w", runtimeErr)
	}
	if targetPortfolioErr != nil && !errors.Is(targetPortfolioErr, fs.ErrNotExist) {
		return "", fmt.Errorf("report manifest: inspect target page portfolio for runtime portfolio: %w", targetPortfolioErr)
	}
	if programPortfolioErr != nil && !errors.Is(programPortfolioErr, fs.ErrNotExist) {
		return "", fmt.Errorf("report manifest: inspect program page portfolio for runtime portfolio: %w", programPortfolioErr)
	}
	if targetPortfolioPresent && programPortfolioPresent {
		return "", fmt.Errorf("report manifest: program and Go target page authority are mutually exclusive")
	}
	pagePortfolioPresent := targetPortfolioPresent || programPortfolioPresent
	if runtimePresent != pagePortfolioPresent {
		return "", fmt.Errorf("report manifest: page and runtime portfolio authority must be published together")
	}
	if !runtimePresent {
		return "", nil
	}
	raw, err := readManifestFile(root, runtimeportfolio.ArtifactFilename, runtimeportfolio.MaxArtifactBytes)
	if err != nil {
		return "", err
	}
	result, err := runtimeportfolio.Decode(raw)
	if err != nil {
		return "", fmt.Errorf("report manifest: runtime portfolio artifact: %w", err)
	}
	if programPortfolioPresent {
		portfolioRaw, readErr := readManifestFile(
			root, programpage.ArtifactFilename, programpage.MaxArtifactBytes,
		)
		if readErr != nil {
			return "", readErr
		}
		portfolio, decodeErr := programpage.Decode(portfolioRaw)
		if decodeErr != nil {
			return "", fmt.Errorf("report manifest: runtime program page portfolio: %w", decodeErr)
		}
		if err := validateRuntimeProgramPageCoverage(result, portfolio); err != nil {
			return "", fmt.Errorf("report manifest: %w", err)
		}
	} else {
		portfolioRaw, readErr := readManifestFile(
			root,
			snapshot.TargetPagePortfolioArtifactFilename,
			snapshot.MaxTargetPagePortfolioBytes,
		)
		if readErr != nil {
			return "", readErr
		}
		portfolio, decodeErr := snapshot.DecodeTargetPagePortfolio(portfolioRaw)
		if decodeErr != nil {
			return "", fmt.Errorf("report manifest: runtime target page portfolio: %w", decodeErr)
		}
		if result.TargetPagePortfolioSHA256 != portfolio.SHA256 {
			return "", fmt.Errorf("report manifest: runtime portfolio target-page binding mismatch")
		}
	}
	return manifestSHA256(raw), nil
}

func savedCubeMapSHA256(runDir string) (string, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return "", fmt.Errorf("report manifest: open cube map run: %w", err)
	}
	defer root.Close()
	if _, err := root.Lstat(cubemap.ArtifactFilename); errors.Is(err, fs.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("report manifest: inspect cube map: %w", err)
	}
	raw, err := readManifestFile(root, cubemap.ArtifactFilename, maxManifestReportBytes)
	if err != nil {
		return "", err
	}
	if _, err := cubemap.Decode(raw); err != nil {
		return "", fmt.Errorf("report manifest: cube map artifact: %w", err)
	}
	return manifestSHA256(raw), nil
}

func savedCoreMapSHA256(runDir string) (string, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return "", fmt.Errorf("report manifest: open core map run: %w", err)
	}
	defer root.Close()
	if _, err := root.Lstat(coremap.ArtifactFilename); errors.Is(err, fs.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("report manifest: inspect core map: %w", err)
	}
	raw, err := readManifestFile(root, coremap.ArtifactFilename, coremap.MaxArtifactBytes)
	if err != nil {
		return "", err
	}
	if _, err := coremap.Decode(raw); err != nil {
		return "", fmt.Errorf("report manifest: core map artifact: %w", err)
	}
	return manifestSHA256(raw), nil
}

func savedActivityEntrypointsSHA256(
	runDir string,
	programIndexSetSHA256 string,
) (string, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return "", fmt.Errorf("report manifest: open activity entrypoint run: %w", err)
	}
	defer root.Close()
	present, err := manifestArtifactPresent(root, activityentrypoint.ArtifactFilename)
	if err != nil {
		return "", err
	}
	if !present {
		return "", nil
	}
	defaultIndex, err := readManifestDefaultProgramIndex(root, programIndexSetSHA256)
	if err != nil {
		return "", err
	}
	raw, err := readManifestFile(
		root, activityentrypoint.ArtifactFilename, activityentrypoint.MaxArtifactBytes,
	)
	if err != nil {
		return "", err
	}
	if _, err := activityentrypoint.Decode(raw, defaultIndex); err != nil {
		return "", fmt.Errorf("report manifest: activity entrypoint artifact: %w", err)
	}
	return manifestSHA256(raw), nil
}

func savedDeclaredDependencyArtifactsSHA256(
	runDir string,
	programIndexSetSHA256 string,
) (string, string, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return "", "", fmt.Errorf("report manifest: open declared dependency run: %w", err)
	}
	defer root.Close()
	declarationPresent, err := manifestArtifactPresent(root, dependencydeclaration.ArtifactFilename)
	if err != nil {
		return "", "", err
	}
	targetPresent, err := manifestArtifactPresent(root, pythontarget.ArtifactFilename)
	if err != nil {
		return "", "", err
	}
	if !targetPresent {
		if !declarationPresent {
			return "", "", nil
		}
		return "", "", fmt.Errorf("report manifest: declared dependencies require the Python target catalog")
	}
	targetRaw, err := readManifestFile(root, pythontarget.ArtifactFilename, pythontarget.MaxArtifactBytes)
	if err != nil {
		return "", "", err
	}
	targets, err := pythontarget.DecodeCatalog(targetRaw)
	if err != nil {
		return "", "", fmt.Errorf("report manifest: Python target catalog artifact: %w", err)
	}
	targetSHA256 := manifestSHA256(targetRaw)
	if !declarationPresent {
		return targetSHA256, "", nil
	}
	declarationRaw, err := readManifestFile(
		root, dependencydeclaration.ArtifactFilename, dependencydeclaration.MaxArtifactBytes,
	)
	if err != nil {
		return "", "", err
	}
	index, err := readManifestDefaultProgramIndex(root, programIndexSetSHA256)
	if err != nil {
		return "", "", err
	}
	if _, err := pythondeclareddependencies.DecodeTargetAuthority(declarationRaw, targets, index); err != nil {
		return "", "", fmt.Errorf("report manifest: declared dependency artifact: %w", err)
	}
	return targetSHA256, manifestSHA256(declarationRaw), nil
}

func readSavedDeclaredDependencyAuthority(
	root *os.Root,
	index programindex.Index,
) (dependencydeclaration.Result, error) {
	targetRaw, err := readManifestFile(root, pythontarget.ArtifactFilename, pythontarget.MaxArtifactBytes)
	if err != nil {
		return dependencydeclaration.Result{}, err
	}
	targets, err := pythontarget.DecodeCatalog(targetRaw)
	if err != nil {
		return dependencydeclaration.Result{}, fmt.Errorf("report manifest: Python target catalog artifact: %w", err)
	}
	declarationRaw, err := readManifestFile(
		root, dependencydeclaration.ArtifactFilename, dependencydeclaration.MaxArtifactBytes,
	)
	if err != nil {
		return dependencydeclaration.Result{}, err
	}
	declarations, err := pythondeclareddependencies.DecodeTargetAuthority(declarationRaw, targets, index)
	if err != nil {
		return dependencydeclaration.Result{}, fmt.Errorf("report manifest: declared dependency artifact: %w", err)
	}
	return declarations, nil
}

func savedPythonSemanticArtifactsSHA256(
	runDir string,
	programIndexSetSHA256 string,
) (string, string, string, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return "", "", "", fmt.Errorf("report manifest: open Python semantic run: %w", err)
	}
	defer root.Close()
	catalogPresent, err := manifestArtifactPresent(root, dependencies.ArtifactFilename)
	if err != nil {
		return "", "", "", err
	}
	integrationPresent, err := manifestArtifactPresent(root, integrationdependency.ArtifactFilename)
	if err != nil {
		return "", "", "", err
	}
	usagePresent, err := manifestArtifactPresent(root, integrationusage.ArtifactFilename)
	if err != nil {
		return "", "", "", err
	}
	if !catalogPresent && !integrationPresent && !usagePresent {
		return "", "", "", nil
	}
	if !catalogPresent || !integrationPresent || !usagePresent {
		return "", "", "", fmt.Errorf(
			"report manifest: Python dependency catalog, integration result, and integration usage must all be present",
		)
	}
	catalogRaw, err := readManifestFile(root, dependencies.ArtifactFilename, dependencies.MaxArtifactBytes)
	if err != nil {
		return "", "", "", err
	}
	catalog, err := dependencies.Decode(catalogRaw)
	if err != nil {
		return "", "", "", fmt.Errorf("report manifest: dependency catalog artifact: %w", err)
	}
	defaultIndex, err := readManifestDefaultProgramIndex(root, programIndexSetSHA256)
	if err != nil {
		return "", "", "", err
	}
	integrationRaw, err := readManifestFile(
		root, integrationdependency.ArtifactFilename, integrationdependency.MaxArtifactBytes,
	)
	if err != nil {
		return "", "", "", err
	}
	integration, err := integrationdependency.Decode(integrationRaw)
	if err != nil {
		return "", "", "", fmt.Errorf("report manifest: integration dependencies artifact: %w", err)
	}
	var integrationAuthorityErr error
	if defaultIndex.Target.Language == "python" {
		declarations, declarationErr := readSavedDeclaredDependencyAuthority(root, defaultIndex)
		if declarationErr != nil {
			return "", "", "", declarationErr
		}
		integrationAuthorityErr = integration.ValidateAgainstDeclarations(catalog, declarations, defaultIndex.Target)
	} else {
		integrationAuthorityErr = integration.ValidateAgainst(catalog)
	}
	if integrationAuthorityErr != nil {
		return "", "", "", fmt.Errorf("report manifest: integration dependencies authority: %w", integrationAuthorityErr)
	}
	usageRaw, err := readManifestFile(root, integrationusage.ArtifactFilename, integrationusage.MaxArtifactBytes)
	if err != nil {
		return "", "", "", err
	}
	usage, err := integrationusage.Decode(usageRaw)
	if err != nil {
		return "", "", "", fmt.Errorf("report manifest: integration usage artifact: %w", err)
	}
	if err := usage.ValidateAgainst(defaultIndex, integration); err != nil {
		return "", "", "", fmt.Errorf("report manifest: integration usage authority: %w", err)
	}
	return manifestSHA256(catalogRaw), manifestSHA256(integrationRaw), manifestSHA256(usageRaw), nil
}

func savedActivityPathsSHA256(
	runDir string,
	programIndexSetSHA256 string,
) (string, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return "", fmt.Errorf("report manifest: open activity path run: %w", err)
	}
	defer root.Close()
	present, err := manifestArtifactPresent(root, activitypath.ArtifactFilename)
	if err != nil {
		return "", err
	}
	if !present {
		return "", nil
	}
	index, err := readManifestDefaultProgramIndex(root, programIndexSetSHA256)
	if err != nil {
		return "", err
	}
	activityRaw, err := readManifestFile(
		root, activityentrypoint.ArtifactFilename, activityentrypoint.MaxArtifactBytes,
	)
	if err != nil {
		return "", err
	}
	activities, err := activityentrypoint.Decode(activityRaw, index)
	if err != nil {
		return "", fmt.Errorf("report manifest: activity path activity entrypoints: %w", err)
	}
	catalogRaw, err := readManifestFile(root, dependencies.ArtifactFilename, dependencies.MaxArtifactBytes)
	if err != nil {
		return "", err
	}
	catalog, err := dependencies.Decode(catalogRaw)
	if err != nil {
		return "", fmt.Errorf("report manifest: activity path dependency catalog: %w", err)
	}
	integrationRaw, err := readManifestFile(
		root, integrationdependency.ArtifactFilename, integrationdependency.MaxArtifactBytes,
	)
	if err != nil {
		return "", err
	}
	integrations, err := integrationdependency.Decode(integrationRaw)
	if err != nil {
		return "", fmt.Errorf("report manifest: activity path integration dependencies: %w", err)
	}
	var integrationAuthorityErr error
	if index.Target.Language == "python" {
		declarations, declarationErr := readSavedDeclaredDependencyAuthority(root, index)
		if declarationErr != nil {
			return "", declarationErr
		}
		integrationAuthorityErr = integrations.ValidateAgainstDeclarations(catalog, declarations, index.Target)
	} else {
		integrationAuthorityErr = integrations.ValidateAgainst(catalog)
	}
	if integrationAuthorityErr != nil {
		return "", fmt.Errorf("report manifest: activity path integration dependency authority: %w", integrationAuthorityErr)
	}
	usageRaw, err := readManifestFile(root, integrationusage.ArtifactFilename, integrationusage.MaxArtifactBytes)
	if err != nil {
		return "", err
	}
	usage, err := integrationusage.Decode(usageRaw)
	if err != nil {
		return "", fmt.Errorf("report manifest: activity path integration usage: %w", err)
	}
	pathRaw, err := readManifestFile(root, activitypath.ArtifactFilename, activitypath.MaxArtifactBytes)
	if err != nil {
		return "", err
	}
	if _, err := activitypath.Decode(pathRaw, index, activities, integrations, usage); err != nil {
		return "", fmt.Errorf("report manifest: activity path artifact: %w", err)
	}
	return manifestSHA256(pathRaw), nil
}

func savedJSTSProjectSHA256(runDir, programIndexSetSHA256 string) (string, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return "", fmt.Errorf("report manifest: open JavaScript/TypeScript project run: %w", err)
	}
	defer root.Close()
	present, err := manifestArtifactPresent(root, jstsproject.ArtifactFilename)
	if err != nil {
		return "", err
	}
	if !present {
		return "", nil
	}
	index, err := readManifestDefaultProgramIndex(root, programIndexSetSHA256)
	if err != nil {
		return "", err
	}
	raw, err := readManifestFile(root, jstsproject.ArtifactFilename, jstsproject.MaxArtifactBytes)
	if err != nil {
		return "", err
	}
	result, err := jstsproject.Decode(raw)
	if err != nil {
		return "", fmt.Errorf("report manifest: JavaScript/TypeScript project artifact: %w", err)
	}
	if err := validateJSTSProgramIndexBinding(result, index); err != nil {
		return "", fmt.Errorf("report manifest: JavaScript/TypeScript project binding: %w", err)
	}
	return manifestSHA256(raw), nil
}

func manifestArtifactPresent(root *os.Root, name string) (bool, error) {
	if _, err := root.Lstat(name); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("report manifest: inspect %s: %w", name, err)
	}
	return true, nil
}

func savedReadmeFileRolesSHA256(runDir string) (string, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return "", fmt.Errorf("report manifest: open README file-role run: %w", err)
	}
	defer root.Close()
	if _, err := root.Lstat(readmetargetscout.ArtifactFilename); errors.Is(err, fs.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("report manifest: inspect README file-role artifact: %w", err)
	}
	raw, err := readManifestFile(root, readmetargetscout.ArtifactFilename, maxReadmeFileRoleArtifactBytes)
	if err != nil {
		return "", err
	}
	if _, err := decodeReadmeFileRoleAuthority(raw); err != nil {
		return "", err
	}
	return manifestSHA256(raw), nil
}

func savedProgramIndexSetSHA256(runDir string) (string, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return "", fmt.Errorf("report manifest: open program index run: %w", err)
	}
	defer root.Close()
	if _, err := root.Lstat(programindex.ArtifactSetFilename); errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("report manifest: required program index set is missing")
	} else if err != nil {
		return "", fmt.Errorf("report manifest: inspect %s: %w", programindex.ArtifactSetFilename, err)
	}
	raw, err := readManifestFile(root, programindex.ArtifactSetFilename, programindex.MaxArtifactSetBytes)
	if err != nil {
		return "", err
	}
	return manifestSHA256(raw), nil
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

func readRunSnapshot(runDir string) ([]byte, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return nil, fmt.Errorf("report manifest: open snapshot run: %w", err)
	}
	defer root.Close()
	return readManifestFile(root, "snapshot.json", maxManifestSnapshotBytes)
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
