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

	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/orientation"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/targetoutcome"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

const (
	CurrentRunManifestVersion = 39
	RunManifestFilename       = "run_manifest.json"

	advisoryRunManifestBytes      = 4 * 1024 * 1024
	maxRunManifestBytes           = 0
	maxManifestReportBytes        = 0
	advisoryManifestSnapshotBytes = 64 * 1024 * 1024
	// Snapshot artifacts are local exact authority. Zero means read the whole
	// regular file; scale diagnostics, not validation, own the former 64 MiB
	// observation threshold.
	maxManifestSnapshotBytes = 0
	// maxManifestOpenablePaths is advisory only. Complete source authority is
	// retained; unusually large path sets are reported through scale warnings.
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
	SelectedRevision             string `json:"selected_revision"`
	ProgramTargetID              string `json:"program_target_id"`
	ProgramTargetSHA256          string `json:"program_target_sha256"`
	ProgramIndexSetSHA256        string `json:"program_index_set_sha256"`
	DependencyCatalogSHA256      string `json:"dependency_catalog_sha256"`
	ReducedDocumentationSHA256   string `json:"reduced_documentation_sha256"`
	GroupsIndexSHA256            string `json:"groups_index_sha256"`
	ReadmeFileRolesSHA256        string `json:"readme_file_roles_sha256,omitempty"`
	ProgramPagePortfolioSHA256   string `json:"program_page_portfolio_sha256"`
	TargetOutcomePortfolioSHA256 string `json:"target_outcome_portfolio_sha256"`
	// The first-day artifacts are optional: an empty digest means the run
	// directory carries no such file and report.json carries no such value.
	FactsSHA256        string `json:"facts_sha256,omitempty"`
	ClaimsSHA256       string `json:"claims_sha256,omitempty"`
	OrientationSHA256  string `json:"orientation_sha256,omitempty"`
	InputPolicyVersion string `json:"input_policy_version"`
	ReportContract     int    `json:"report_contract"`
}

// RunAuthority binds one analysis to its captured repository identity and
// exact material inputs. It deliberately carries no later-state comparison.
type RunAuthority struct {
	analysisRoot      string
	repository        freshness.RepositoryState
	inputs            []freshness.CapturedInput
	confirmed         bool
	groupGraphIndexes []groupindex.Index
	groupGraphBound   bool
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
	if !validManifestSHA256(m.MaterialInputs.DependencyCatalogSHA256) {
		return fmt.Errorf("report manifest: dependency catalog sha256 is invalid")
	}
	if !validManifestSHA256(m.MaterialInputs.GroupsIndexSHA256) {
		return fmt.Errorf("report manifest: groups index sha256 is invalid")
	}
	if !validManifestSHA256(m.MaterialInputs.ReducedDocumentationSHA256) {
		return fmt.Errorf("report manifest: reduced documentation sha256 is invalid")
	}
	if m.MaterialInputs.ReadmeFileRolesSHA256 != "" &&
		!validManifestSHA256(m.MaterialInputs.ReadmeFileRolesSHA256) {
		return fmt.Errorf("report manifest: README file-role sha256 is invalid")
	}
	if !validManifestSHA256(m.MaterialInputs.ProgramPagePortfolioSHA256) {
		return fmt.Errorf("report manifest: program page portfolio sha256 is invalid")
	}
	if !validManifestSHA256(m.MaterialInputs.TargetOutcomePortfolioSHA256) {
		return fmt.Errorf("report manifest: target outcome portfolio sha256 is invalid")
	}
	for label, digest := range map[string]string{
		"facts":       m.MaterialInputs.FactsSHA256,
		"claims":      m.MaterialInputs.ClaimsSHA256,
		"orientation": m.MaterialInputs.OrientationSHA256,
	} {
		if digest != "" && !validManifestSHA256(digest) {
			return fmt.Errorf("report manifest: %s sha256 is invalid", label)
		}
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

// BindRunAuthorityGroupGraph attaches the complete matched graph set only
// after every source path carried by that set has entered captured-input
// authority. The set is transaction-local; canonical report.json receives the
// direct GroupGraph projection during generation.
func BindRunAuthorityGroupGraph(
	authority RunAuthority,
	indexes []groupindex.Index,
) (RunAuthority, error) {
	if err := authority.validate(); err != nil {
		return RunAuthority{}, err
	}
	if len(indexes) == 0 {
		return RunAuthority{}, fmt.Errorf("report manifest: group graph set is empty")
	}
	view, err := NewGroupGraphView(indexes, indexes[0].Target.ID)
	if err != nil {
		return RunAuthority{}, fmt.Errorf("report manifest: group graph: %w", err)
	}
	paths, err := view.SourcePaths()
	if err != nil {
		return RunAuthority{}, fmt.Errorf("report manifest: group graph source paths: %w", err)
	}
	wantPaths, err := repositoryRelativeInputPaths(
		authority.repository.Identity, authority.analysisRoot, paths,
	)
	if err != nil {
		return RunAuthority{}, err
	}
	captured := make(map[string]struct{}, len(authority.inputs))
	for _, input := range authority.inputs {
		captured[input.Path] = struct{}{}
	}
	for _, sourcePath := range wantPaths {
		if _, ok := captured[sourcePath]; !ok {
			return RunAuthority{}, fmt.Errorf(
				"report manifest: group graph path %q is not captured", sourcePath,
			)
		}
	}
	result := authority
	result.groupGraphIndexes = view.Indexes
	result.groupGraphBound = true
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
	_, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return nil, fmt.Errorf("report: captured input paths: %w", err)
	}
	paths := append([]string(nil), data.OpenablePaths...)
	paths = append(paths, data.materialInputPaths...)
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
	if maxManifestReportBytes > 0 && len(reportJSON) > maxManifestReportBytes {
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
	if report.ProgramPortfolio == nil {
		return fmt.Errorf("report manifest: report program portfolio is missing")
	}
	defaultEntry, err := report.ProgramPortfolio.defaultEntry()
	if err != nil {
		return fmt.Errorf("report manifest: report program portfolio: %w", err)
	}
	if report.GroupGraph == nil {
		return fmt.Errorf("report manifest: final group graph is missing")
	}
	programTargetID, programTargetSHA256, err := reportProgramTargetMaterial(&defaultEntry.Target)
	if err != nil {
		return fmt.Errorf("report manifest: report program target: %w", err)
	}
	if programTargetID != material.ProgramTargetID || programTargetSHA256 != material.ProgramTargetSHA256 {
		return fmt.Errorf("report manifest: program target identity does not match report")
	}
	if report.TargetOutcomePortfolio == nil {
		return fmt.Errorf("report manifest: target outcome portfolio projection is missing")
	}
	if err := report.TargetOutcomePortfolio.Validate(); err != nil {
		return fmt.Errorf("report manifest: target outcome portfolio view: %w", err)
	}
	return m.verifyFirstDayReportValues(report)
}

// verifyFirstDayReportValues proves that report.json carries a first-day
// artifact exactly when the manifest binds one, and that the sealed values
// validate. The artifact bytes themselves are compared by the verification
// suite, which has the run directory.
func (m RunManifest) verifyFirstDayReportValues(report ReportData) error {
	material := m.MaterialInputs
	if (material.FactsSHA256 == "") != (report.Facts == nil) {
		return fmt.Errorf("report manifest: facts binding does not match report")
	}
	if report.Facts != nil {
		if err := report.Facts.Validate(); err != nil {
			return fmt.Errorf("report manifest: report facts: %w", err)
		}
	}
	if (material.ClaimsSHA256 == "") != (report.Claims == nil) {
		return fmt.Errorf("report manifest: claims binding does not match report")
	}
	if report.Claims != nil {
		if err := report.Claims.Validate(); err != nil {
			return fmt.Errorf("report manifest: report claims: %w", err)
		}
	}
	if (material.OrientationSHA256 == "") != (report.Orientation == nil) {
		return fmt.Errorf("report manifest: orientation binding does not match report")
	}
	if report.Orientation != nil {
		if err := report.Orientation.Validate(); err != nil {
			return fmt.Errorf("report manifest: report orientation: %w", err)
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
		if limit > 0 && len(file.data) > limit {
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
	if file.info.Size() < 0 || (limit > 0 && file.info.Size() > int64(limit)) {
		file.readErr = fmt.Errorf("report manifest: %s exceeds %d bytes", name, limit)
		return nil, file.readErr
	}
	handle, err := suite.root.Open(name)
	if err != nil {
		file.readErr = fmt.Errorf("report manifest: open %s: %w", name, err)
		return nil, file.readErr
	}
	var reader io.Reader = handle
	if limit > 0 {
		reader = io.LimitReader(handle, int64(limit)+1)
	}
	data, readErr := io.ReadAll(reader)
	closeErr := handle.Close()
	if readErr != nil {
		file.readErr = fmt.Errorf("report manifest: read %s: %w", name, readErr)
		return nil, file.readErr
	}
	if closeErr != nil {
		file.readErr = fmt.Errorf("report manifest: close %s: %w", name, closeErr)
		return nil, file.readErr
	}
	if limit > 0 && len(data) > limit {
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
		if name == programindex.ArtifactSetFilename || name == programindex.ArtifactFilename {
			names = append(names, name)
			continue
		}
		if strings.HasPrefix(name, "program-index") {
			return nil, fmt.Errorf(
				"report manifest: noncanonical ProgramIndex artifact %q is not accepted", name,
			)
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
		if maxManifestReportBytes > 0 && len(reportJSON) > maxManifestReportBytes {
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
			return manifestProgramIndexes{}, fmt.Errorf("report manifest: default ProgramIndex is missing")
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

// VerifyTargetOutcomePortfolioProjection binds the complete selected-target
// outcome artifact, the exact set of analyzed program pages, and the reduced
// report/browser projection. Both artifacts and the projection are mandatory
// for every published page, including a one-target portfolio.
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

func readManifestDefaultProgramIndex(root *os.Root, setSHA256 string) (programindex.Index, error) {
	setRaw, err := readManifestFile(root, programindex.ArtifactSetFilename, programindex.MaxArtifactSetBytes)
	if err != nil {
		return programindex.Index{}, err
	}
	if manifestSHA256(setRaw) != setSHA256 {
		return programindex.Index{}, fmt.Errorf("report manifest: program index set sha256 mismatch")
	}
	set, err := programindex.DecodeArtifactSet(setRaw)
	if err != nil {
		return programindex.Index{}, fmt.Errorf("report manifest: program index set: %w", err)
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
			return programindex.Index{}, fmt.Errorf("report manifest: default ProgramIndex: %w", decodeErr)
		}
		if index.Target.ID != entry.TargetID || index.SHA256 != entry.IndexSHA256 {
			return programindex.Index{}, fmt.Errorf("report manifest: default ProgramIndex binding mismatch")
		}
		return index, nil
	}
	return programindex.Index{}, fmt.Errorf("report manifest: default ProgramIndex is missing")
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
	if len(data) == 0 {
		return RunManifest{}, fmt.Errorf("report manifest: artifact is empty")
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
	verified       bool
	runDir         string
	manifest       RunManifest
	programPage    TargetNavigationPage
	repositoryName string
}

type verifiedRunIdentity struct {
	programPage    TargetNavigationPage
	repositoryName string
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
	_, identity, err := verifyCompleteRunManifestWithIdentity(runDir, manifest, nil)
	if err != nil {
		return VerifiedRunReceipt{}, err
	}
	return newVerifiedRunReceipt(runDir, manifest, identity)
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

// ProgramPage returns the small exact page identity retained by this
// transaction. It does not restore report or ProgramIndex authority again.
func (receipt VerifiedRunReceipt) ProgramPage() TargetNavigationPage {
	page := receipt.programPage
	page.ProgramTarget = receipt.programPage.ProgramTarget.Snapshot()
	return page
}

// RepositoryName returns the canonical report repository name retained by
// this transaction-local receipt.
func (receipt VerifiedRunReceipt) RepositoryName() string {
	return receipt.repositoryName
}

// ValidateRunIdentity proves only that the opaque receipt belongs to the
// supplied run directory. Current-run receipts intentionally do not re-read
// process-owned artifacts; independent later reads use ReadRunManifest.
func (receipt VerifiedRunReceipt) ValidateRunIdentity(runDir string) error {
	if !receipt.verified {
		return fmt.Errorf("report manifest: verified run receipt is empty")
	}
	absoluteRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return fmt.Errorf("report manifest: resolve verified run directory: %w", err)
	}
	if filepath.Clean(absoluteRunDir) != receipt.runDir {
		return fmt.Errorf("report manifest: verified run receipt directory mismatch")
	}
	return nil
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

func newVerifiedRunReceipt(
	runDir string,
	manifest RunManifest,
	identity verifiedRunIdentity,
) (VerifiedRunReceipt, error) {
	absoluteRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return VerifiedRunReceipt{}, fmt.Errorf("report manifest: resolve verified run directory: %w", err)
	}
	page := identity.programPage
	if err := validateTargetNavigationPage(page); err != nil {
		return VerifiedRunReceipt{}, fmt.Errorf("report manifest: verified program page: %w", err)
	}
	if page.RunID != filepath.Base(filepath.Clean(absoluteRunDir)) ||
		page.ProgramTarget.ID != manifest.MaterialInputs.ProgramTargetID ||
		identity.repositoryName == "" {
		return VerifiedRunReceipt{}, fmt.Errorf("report manifest: verified run receipt identity mismatch")
	}
	return VerifiedRunReceipt{
		verified:       true,
		runDir:         filepath.Clean(absoluteRunDir),
		manifest:       cloneRunManifest(manifest),
		programPage:    page,
		repositoryName: identity.repositoryName,
	}, nil
}

func newVerifiedRunReceiptFromReportData(
	runDir string,
	manifest RunManifest,
	data *ReportData,
) (VerifiedRunReceipt, error) {
	page, err := PreparedTargetNavigationPage(runDir, data)
	if err != nil {
		return VerifiedRunReceipt{}, fmt.Errorf("report manifest: verified report program page: %w", err)
	}
	return newVerifiedRunReceipt(runDir, manifest, verifiedRunIdentity{
		programPage: page, repositoryName: data.RepoName,
	})
}

func verifyCompleteRunManifest(
	runDir string,
	manifest RunManifest,
	reportJSON []byte,
) (manifestVerificationStats, error) {
	stats, _, err := verifyCompleteRunManifestWithIdentity(runDir, manifest, reportJSON)
	return stats, err
}

func verifyCompleteRunManifestWithIdentity(
	runDir string,
	manifest RunManifest,
	reportJSON []byte,
) (manifestVerificationStats, verifiedRunIdentity, error) {
	suite, err := newManifestVerificationSuite(manifest, runDir)
	if err != nil {
		return manifestVerificationStats{}, verifiedRunIdentity{}, err
	}
	defer suite.Close()
	if err := suite.verifyReport(reportJSON); err != nil {
		return suite.stats(), verifiedRunIdentity{}, err
	}
	program, err := suite.programIndexes()
	if err != nil {
		return suite.stats(), verifiedRunIdentity{}, err
	}
	if err := suite.verifyDependencyCatalogArtifact(); err != nil {
		return suite.stats(), verifiedRunIdentity{}, err
	}
	if err := suite.verifyReducedDocumentationArtifact(program); err != nil {
		return suite.stats(), verifiedRunIdentity{}, err
	}
	if err := suite.verifyGroupsIndexArtifact(program, reportJSON); err != nil {
		return suite.stats(), verifiedRunIdentity{}, err
	}
	if err := suite.verifyProgramPortfolioProjection(reportJSON); err != nil {
		return suite.stats(), verifiedRunIdentity{}, err
	}
	if err := suite.verifyTargetOutcomePortfolioProjection(reportJSON); err != nil {
		return suite.stats(), verifiedRunIdentity{}, err
	}
	if err := suite.verifySnapshotArtifact(); err != nil {
		return suite.stats(), verifiedRunIdentity{}, err
	}
	if err := suite.verifyProgramPagePortfolioArtifact(); err != nil {
		return suite.stats(), verifiedRunIdentity{}, err
	}
	if err := suite.verifyFirstDayArtifacts(reportJSON); err != nil {
		return suite.stats(), verifiedRunIdentity{}, err
	}
	report, err := suite.report(reportJSON)
	if err != nil {
		return suite.stats(), verifiedRunIdentity{}, err
	}
	artifactFilename := ""
	for _, entry := range program.set.Entries {
		if entry.TargetID == program.set.DefaultTargetID {
			artifactFilename = entry.Filename
			break
		}
	}
	identity := verifiedRunIdentity{
		programPage: TargetNavigationPage{
			RunID:            filepath.Base(filepath.Clean(runDir)),
			ProgramTarget:    program.defaultIndex.Target.Snapshot(),
			ArtifactFilename: artifactFilename,
		},
		repositoryName: report.RepoName,
	}
	return suite.stats(), identity, nil
}

func (suite *manifestVerificationSuite) verifyReducedDocumentationArtifact(
	program manifestProgramIndexes,
) error {
	present, err := suite.artifactPresent(documentationreduce.ArtifactFilename)
	if err != nil {
		return err
	}
	digest := suite.manifest.MaterialInputs.ReducedDocumentationSHA256
	required := program.defaultIndex.Categorization != nil
	if digest == "" {
		if present {
			return fmt.Errorf("report manifest: unbound reduced documentation is present")
		}
		if required {
			return fmt.Errorf("report manifest: required reduced documentation is missing")
		}
		return nil
	}
	if !present || !required {
		return fmt.Errorf("report manifest: reduced documentation sha256 mismatch")
	}
	raw, err := suite.readFile(documentationreduce.ArtifactFilename, 0)
	if err != nil {
		return err
	}
	if manifestSHA256(raw) != digest {
		return fmt.Errorf("report manifest: reduced documentation sha256 mismatch")
	}
	reduced, err := documentationreduce.Decode(raw)
	if err != nil {
		return fmt.Errorf("report manifest: reduced documentation artifact: %w", err)
	}
	suite.releaseFile(documentationreduce.ArtifactFilename)
	if reduced.ReductionSHA256 != program.defaultIndex.Categorization.ReducedDocumentationSHA256 {
		return fmt.Errorf("report manifest: reduced documentation does not bind categorized ProgramIndex")
	}
	return nil
}

func (suite *manifestVerificationSuite) verifyDependencyCatalogArtifact() error {
	raw, err := suite.readFile(dependencies.ArtifactFilename, dependencies.MaxArtifactBytes)
	if err != nil {
		return err
	}
	if manifestSHA256(raw) != suite.manifest.MaterialInputs.DependencyCatalogSHA256 {
		return fmt.Errorf("report manifest: dependency catalog sha256 mismatch")
	}
	if _, err := dependencies.Decode(raw); err != nil {
		return fmt.Errorf("report manifest: dependency catalog artifact: %w", err)
	}
	suite.releaseFile(dependencies.ArtifactFilename)
	return nil
}

func (suite *manifestVerificationSuite) verifyGroupsIndexArtifact(
	program manifestProgramIndexes,
	reportJSON []byte,
) error {
	present, err := suite.artifactPresent(groupindex.ArtifactFilename)
	if err != nil {
		return err
	}
	digest := suite.manifest.MaterialInputs.GroupsIndexSHA256
	if digest == "" {
		if present {
			return fmt.Errorf("report manifest: unbound groups index is present")
		}
		return nil
	}
	if !present {
		return fmt.Errorf("report manifest: groups index sha256 mismatch")
	}
	raw, err := suite.readFile(groupindex.ArtifactFilename, 0)
	if err != nil {
		return err
	}
	if manifestSHA256(raw) != digest {
		return fmt.Errorf("report manifest: groups index sha256 mismatch")
	}
	local, err := groupindex.Decode(raw)
	if err != nil {
		return fmt.Errorf("report manifest: groups index artifact: %w", err)
	}
	suite.releaseFile(groupindex.ArtifactFilename)
	if local.Target.ID != program.defaultIndex.Target.ID ||
		local.ProgramIndexSHA256 != program.defaultIndex.SHA256 ||
		!reflect.DeepEqual(local.Target, program.defaultIndex.Target) {
		return fmt.Errorf("report manifest: groups index does not bind the default ProgramIndex")
	}
	report, err := suite.report(reportJSON)
	if err != nil {
		return err
	}
	if report.GroupGraph == nil {
		return fmt.Errorf("report manifest: groups index lacks report projection")
	}
	var selected *groupindex.Index
	for index := range report.GroupGraph.Indexes {
		if report.GroupGraph.Indexes[index].Target.ID == local.Target.ID {
			selected = &report.GroupGraph.Indexes[index]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("report manifest: groups index projection omits the selected target")
	}
	if err := validateLocalGroupIndexExtension(local, *selected); err != nil {
		return fmt.Errorf("report manifest: %w", err)
	}
	return nil
}

// VerifyProgramPagePortfolioArtifact binds the language-neutral page
// portfolio and proves that this exact run publishes the manifest's current
// ProgramTarget.
func (m RunManifest) VerifyProgramPagePortfolioArtifact(runDir string) error {
	suite, err := newManifestVerificationSuiteWithValidation(m, runDir, false)
	if err != nil {
		return err
	}
	defer suite.Close()
	return suite.verifyProgramPagePortfolioArtifact()
}

// verifyFirstDayArtifacts binds facts.json, claims.json and orientation.json
// to the manifest digests and proves report.json carries the same sealed
// values. Absent artifacts must be unbound and absent from the report.
func (suite *manifestVerificationSuite) verifyFirstDayArtifacts(reportJSON []byte) error {
	report, err := suite.report(reportJSON)
	if err != nil {
		return fmt.Errorf("report manifest: first-day artifacts: %w", err)
	}
	material := suite.manifest.MaterialInputs
	if err := verifyFirstDayArtifact(
		suite, "facts", facts.ArtifactFilename, material.FactsSHA256, facts.Decode, report.Facts,
	); err != nil {
		return err
	}
	if err := verifyFirstDayArtifact(
		suite, "claims", claims.ArtifactFilename, material.ClaimsSHA256, claims.Decode, report.Claims,
	); err != nil {
		return err
	}
	return verifyFirstDayArtifact(
		suite, "orientation", orientation.ArtifactFilename, material.OrientationSHA256,
		orientation.Decode, report.Orientation,
	)
}

func verifyFirstDayArtifact[T any](
	suite *manifestVerificationSuite,
	label string,
	name string,
	wantSHA256 string,
	decode func([]byte) (T, error),
	reported *T,
) error {
	decoded, present, err := manifestDecodeBound(suite, label, name, 0, wantSHA256, label, decode)
	if err != nil {
		return err
	}
	if present != (reported != nil) {
		return fmt.Errorf("report manifest: %s artifact and report value disagree", label)
	}
	if present && !reflect.DeepEqual(decoded, *reported) {
		return fmt.Errorf("report manifest: report %s does not match the bound artifact", label)
	}
	return nil
}

func (suite *manifestVerificationSuite) verifyProgramPagePortfolioArtifact() error {
	wantSHA256 := suite.manifest.MaterialInputs.ProgramPagePortfolioSHA256
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
	if authority.groupGraphBound {
		if err := groupindex.ValidateSet(authority.groupGraphIndexes); err != nil {
			return fmt.Errorf("report manifest: group graph: %w", err)
		}
	} else if len(authority.groupGraphIndexes) != 0 {
		return fmt.Errorf("report manifest: unbound group graph")
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
	dependencyCatalogDigest, err := savedDependencyCatalogSHA256(runDir)
	if err != nil {
		return RunManifest{}, err
	}
	reducedDocumentationDigest, err := savedReducedDocumentationSHA256(runDir, data.defaultProgramIndex)
	if err != nil {
		return RunManifest{}, err
	}
	groupsIndexDigest, err := savedGroupsIndexSHA256(runDir, data.GroupGraph != nil)
	if err != nil {
		return RunManifest{}, err
	}
	readmeFileRolesDigest, err := savedReadmeFileRolesSHA256(runDir)
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
			SelectedRevision:             authority.repository.Head,
			ProgramTargetID:              programTargetID,
			ProgramTargetSHA256:          programTargetDigest,
			ProgramIndexSetSHA256:        programIndexSetDigest,
			DependencyCatalogSHA256:      dependencyCatalogDigest,
			ReducedDocumentationSHA256:   reducedDocumentationDigest,
			GroupsIndexSHA256:            groupsIndexDigest,
			ReadmeFileRolesSHA256:        readmeFileRolesDigest,
			ProgramPagePortfolioSHA256:   savedArtifactSHA256(runDir, programpage.ArtifactFilename),
			TargetOutcomePortfolioSHA256: savedArtifactSHA256(runDir, targetoutcome.ArtifactFilename),
			FactsSHA256:                  savedArtifactSHA256(runDir, facts.ArtifactFilename),
			ClaimsSHA256:                 savedArtifactSHA256(runDir, claims.ArtifactFilename),
			OrientationSHA256:            savedArtifactSHA256(runDir, orientation.ArtifactFilename),
			InputPolicyVersion:           "captured-inputs-v1",
			ReportContract:               data.FormatVersion,
		},
	}
	if err := verifyPreparedRunManifest(manifest, data, reportJSON); err != nil {
		return RunManifest{}, err
	}
	return manifest, nil
}

// verifyPreparedRunManifest closes the current in-process publication
// transaction. readRunDir already restored and semantically validated every
// producer artifact used to construct data, and the manifest digests above
// were derived from those process-owned files. Re-reading and re-projecting
// the complete run here would only validate our own immediately preceding
// work a second time. Independent later consumers still use
// verifyCompleteRunManifest and rebuild authority from disk.
func verifyPreparedRunManifest(
	manifest RunManifest,
	data *ReportData,
	reportJSON []byte,
) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	if maxManifestReportBytes > 0 && len(reportJSON) > maxManifestReportBytes || manifestSHA256(reportJSON) != manifest.ReportSHA256 {
		return fmt.Errorf("report manifest: prepared report bytes do not match manifest authority")
	}
	persisted := reportDataForPersistence(data)
	if persisted == nil {
		return fmt.Errorf("report manifest: prepared report data is missing")
	}
	if err := manifest.verifyReportData(*persisted); err != nil {
		return err
	}
	return nil
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
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 ||
		(maxManifestReportBytes > 0 && info.Size() > int64(maxManifestReportBytes)) {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(runDir, name))
	if err != nil {
		return ""
	}
	return manifestSHA256(data)
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
	raw, err := readManifestFile(root, readmetargetscout.ArtifactFilename, 0)
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

func savedDependencyCatalogSHA256(runDir string) (string, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return "", fmt.Errorf("report manifest: open dependency catalog run: %w", err)
	}
	defer root.Close()
	if _, err := root.Lstat(dependencies.ArtifactFilename); errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("report manifest: required dependency catalog is missing")
	} else if err != nil {
		return "", fmt.Errorf("report manifest: inspect %s: %w", dependencies.ArtifactFilename, err)
	}
	raw, err := readManifestFile(root, dependencies.ArtifactFilename, dependencies.MaxArtifactBytes)
	if err != nil {
		return "", err
	}
	if _, err := dependencies.Decode(raw); err != nil {
		return "", fmt.Errorf("report manifest: dependency catalog artifact: %w", err)
	}
	return manifestSHA256(raw), nil
}

func savedGroupsIndexSHA256(runDir string, required bool) (string, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return "", fmt.Errorf("report manifest: open groups index run: %w", err)
	}
	defer root.Close()
	if _, err := root.Lstat(groupindex.ArtifactFilename); errors.Is(err, fs.ErrNotExist) {
		if required {
			return "", fmt.Errorf("report manifest: required groups index is missing")
		}
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("report manifest: inspect %s: %w", groupindex.ArtifactFilename, err)
	}
	if !required {
		return "", fmt.Errorf("report manifest: unbound groups index is present")
	}
	raw, err := readManifestFile(root, groupindex.ArtifactFilename, 0)
	if err != nil {
		return "", err
	}
	if _, err := groupindex.Decode(raw); err != nil {
		return "", fmt.Errorf("report manifest: groups index artifact: %w", err)
	}
	return manifestSHA256(raw), nil
}

func savedReducedDocumentationSHA256(
	runDir string,
	index *programindex.Index,
) (string, error) {
	required := index != nil && index.Categorization != nil
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return "", fmt.Errorf("report manifest: open reduced documentation run: %w", err)
	}
	defer root.Close()
	if _, err := root.Lstat(documentationreduce.ArtifactFilename); errors.Is(err, fs.ErrNotExist) {
		if required {
			return "", fmt.Errorf("report manifest: required reduced documentation is missing")
		}
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf(
			"report manifest: inspect %s: %w", documentationreduce.ArtifactFilename, err,
		)
	}
	if !required {
		return "", fmt.Errorf("report manifest: unbound reduced documentation is present")
	}
	raw, err := readManifestFile(root, documentationreduce.ArtifactFilename, 0)
	if err != nil {
		return "", err
	}
	reduced, err := documentationreduce.Decode(raw)
	if err != nil {
		return "", fmt.Errorf("report manifest: reduced documentation artifact: %w", err)
	}
	if reduced.ReductionSHA256 != index.Categorization.ReducedDocumentationSHA256 {
		return "", fmt.Errorf("report manifest: reduced documentation does not bind categorized ProgramIndex")
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
	if maxRunManifestBytes > 0 && len(data) > maxRunManifestBytes {
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
	if info.Size() < 0 || limit > 0 && info.Size() > int64(limit) {
		return nil, fmt.Errorf("report manifest: %s exceeds %d bytes", name, limit)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, fmt.Errorf("report manifest: open %s: %w", name, err)
	}
	defer file.Close()
	var reader io.Reader = file
	if limit > 0 {
		reader = io.LimitReader(file, int64(limit)+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("report manifest: read %s: %w", name, err)
	}
	if limit > 0 && len(data) > limit {
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
