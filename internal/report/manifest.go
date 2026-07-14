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

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/freshness"
)

const (
	CurrentRunManifestVersion = 3
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
	SelectedRevision     string `json:"selected_revision"`
	ModelBundleSHA256    string `json:"model_bundle_sha256,omitempty"`
	InputPolicyVersion   string `json:"input_policy_version"`
	ArchitectureContract int    `json:"architecture_contract"`
	ReportContract       int    `json:"report_contract"`
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
	if m.Version != 2 && m.Version != CurrentRunManifestVersion {
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
	if m.Version >= 3 {
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
		if m.MaterialInputs.ModelBundleSHA256 != "" && !validManifestSHA256(m.MaterialInputs.ModelBundleSHA256) {
			return fmt.Errorf("report manifest: model bundle sha256 is invalid")
		}
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
		FormatVersion int         `json:"format_version"`
		OpenablePaths []string    `json:"openable_paths"`
		Components    []Component `json:"components"`
	}
	if err := json.Unmarshal(reportJSON, &report); err != nil {
		return fmt.Errorf("report manifest: decode report: %w", err)
	}
	if report.FormatVersion != m.ReportFormatVersion {
		return fmt.Errorf("report manifest: report format version mismatch")
	}
	authority, err := componentAuthority(report.Components)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(report.OpenablePaths, m.OpenablePaths) || !reflect.DeepEqual(authority, m.Components) {
		return fmt.Errorf("report manifest: authority does not match report")
	}
	return nil
}

// VerifyRepositoryState lets a caller compare a freshly captured repository
// state with the one authorized by this manifest without re-reading artifacts.
func (m RunManifest) VerifyRepositoryState(current freshness.RepositoryState) error {
	if err := m.Validate(); err != nil {
		return err
	}
	if m.Version < 3 {
		digest, err := current.Digest()
		if err != nil {
			return fmt.Errorf("report manifest: current repository state: %w", err)
		}
		if digest != m.RepositoryStateSHA256 {
			return fmt.Errorf("report manifest: legacy repository state changed")
		}
		return nil
	}
	result := freshness.AssessInputs(context.Background(), m.RepositoryState, current, m.CapturedInputs)
	if result.State == freshness.FreshnessPartiallyStale || result.State == freshness.FreshnessMixedSnapshot ||
		result.State == freshness.FreshnessUnavailable {
		return fmt.Errorf("report manifest: analyzed inputs are %s", result.State)
	}
	return nil
}

func (m RunManifest) CurrentFreshness(current freshness.RepositoryState) freshness.FreshnessResult {
	if m.Version < 3 {
		return freshness.NewFreshnessResult(freshness.FreshnessLegacyUnknown)
	}
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
			SelectedRevision: authority.repository.Head, ModelBundleSHA256: savedArtifactSHA256(runDir, "llm_bundle.json"),
			InputPolicyVersion: "captured-inputs-v1", ArchitectureContract: componentmap.ContractVersion,
			ReportContract: data.FormatVersion,
		},
	}
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
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
