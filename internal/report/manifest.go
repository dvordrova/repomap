package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// RunManifest records where a run came from: which repository at which
// revision, which directory of it was analysed, which target the run is for,
// and where its source links point when the page is published standalone.
//
// It used to be an authority boundary two thousand lines long — every
// artifact digested, every digest re-verified before a page was served, an
// allow-list of every path the page might open — and none of it ever changed
// what a page said. The owner's rule is that the tool is not a security
// boundary, so the manifest is a record now and nothing checks it twice.
type RunManifest struct {
	Version             int                        `json:"version"`
	RepositoryState     freshness.RepositoryState  `json:"repository_state"`
	AnalysisRoot        string                     `json:"analysis_root"`
	StandaloneSource    *StandaloneSourceAuthority `json:"standalone_source,omitempty"`
	ReportFormatVersion int                        `json:"report_format_version"`
	ProgramTargetID     string                     `json:"program_target_id"`
	Display             *DisplayPublication        `json:"display,omitempty"`
}

const (
	CurrentRunManifestVersion = 40
	RunManifestFilename       = "run_manifest.json"
	maxManifestPathBytes      = 4096
)

// StandaloneSourceAuthority says which host a standalone page links its
// sources to, so a reader of a page far from the repository still lands on
// the right file at the right revision.
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

// Validate is the shape of the record: a version this code reads, a
// repository state that names a root and a revision, an analysis root inside
// that repository, and a target.
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
	if m.ReportFormatVersion != CurrentFormatVersion {
		return fmt.Errorf("report manifest: unsupported report format version %d", m.ReportFormatVersion)
	}
	if m.ProgramTargetID == "" {
		return fmt.Errorf("report manifest: program target id is required")
	}
	return m.Display.validate()
}

// RunSource is what a report is generated from: the repository as it was
// captured and the directory of it that was analysed. The group graph is
// bound to it when a multi-target page is finalized, because the graph then
// spans runs and no single run directory holds it.
type RunSource struct {
	AnalysisRoot string
	Repository   freshness.RepositoryState
	GroupGraph   []groupindex.Index
}

// NewRunSource names the repository and the analysed directory. The root is
// resolved to its canonical absolute form once, here, so every path written
// later is relative to the same thing.
func NewRunSource(analysisRoot string, repository freshness.RepositoryState) (RunSource, error) {
	root, err := canonicalManifestDirectory("analysis root", analysisRoot)
	if err != nil {
		return RunSource{}, fmt.Errorf("report manifest: %w", err)
	}
	if err := repository.Validate(); err != nil {
		return RunSource{}, fmt.Errorf("report manifest: repository state: %w", err)
	}
	if err := validateAnalysisRoot(repository.Identity, root); err != nil {
		return RunSource{}, fmt.Errorf("report manifest: analysis root: %w", err)
	}
	return RunSource{AnalysisRoot: root, Repository: repository}, nil
}

// WithGroupGraph binds the matched graph of every target to this source.
func (source RunSource) WithGroupGraph(indexes []groupindex.Index) (RunSource, error) {
	if err := groupindex.ValidateSet(indexes); err != nil {
		return RunSource{}, fmt.Errorf("report manifest: group graph: %w", err)
	}
	cloned := make([]groupindex.Index, len(indexes))
	for position, index := range indexes {
		cloned[position] = index.Snapshot()
	}
	source.GroupGraph = cloned
	return source, nil
}

func (source RunSource) validate() error {
	if err := source.Repository.Validate(); err != nil {
		return fmt.Errorf("report manifest: repository state: %w", err)
	}
	if err := validateAnalysisRoot(source.Repository.Identity, source.AnalysisRoot); err != nil {
		return fmt.Errorf("report manifest: analysis root: %w", err)
	}
	return nil
}

// RunReceipt is one generated run as later stages refer to it: where it is,
// what it recorded, which target page it is, and what the repository is
// called. It is read from the run directory, never verified against it.
type RunReceipt struct {
	runDir         string
	manifest       RunManifest
	programPage    TargetNavigationPage
	repositoryName string
	data           *ReportData
	renderOptions  RenderOptions
}

func newRunReceipt(runDir string, manifest RunManifest, data *ReportData) (RunReceipt, error) {
	absoluteRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return RunReceipt{}, fmt.Errorf("report manifest: resolve run directory: %w", err)
	}
	page, err := PreparedTargetNavigationPage(absoluteRunDir, data)
	if err != nil {
		return RunReceipt{}, err
	}
	return RunReceipt{
		runDir: filepath.Clean(absoluteRunDir), manifest: manifest,
		programPage: page, repositoryName: data.RepoName, data: data,
	}, nil
}

// ReadRunReceipt restores a receipt from a run directory on disk.
func ReadRunReceipt(runDir string) (RunReceipt, error) {
	manifest, err := ReadRunManifest(runDir)
	if err != nil {
		return RunReceipt{}, err
	}
	absoluteRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return RunReceipt{}, fmt.Errorf("report manifest: resolve run directory: %w", err)
	}
	raw, err := os.ReadFile(filepath.Join(absoluteRunDir, "report.json"))
	if err != nil {
		return RunReceipt{}, err
	}
	data, err := decodeStrictReportJSON(raw)
	if err != nil {
		return RunReceipt{}, err
	}
	data.ArtifactsDir = absoluteRunDir
	data.defaultProgramIndexArtifactFilename = programindex.ArtifactFilename
	receipt, err := newRunReceipt(absoluteRunDir, manifest, &data)
	if err != nil {
		return RunReceipt{}, err
	}
	receipt.renderOptions, err = loadDisplayOptions(absoluteRunDir, manifest.Display)
	if err != nil {
		return RunReceipt{}, err
	}
	return receipt, nil
}

// Data returns the immutable in-memory publication result, if available.
// A server makes a shallow copy before attaching its session source links.
func (receipt RunReceipt) Data() *ReportData { return receipt.data }

func (receipt RunReceipt) Manifest() RunManifest   { return receipt.manifest }
func (receipt RunReceipt) RunDir() string          { return receipt.runDir }
func (receipt RunReceipt) RepositoryName() string  { return receipt.repositoryName }
func (receipt RunReceipt) ProgramTargetID() string { return receipt.manifest.ProgramTargetID }
func (receipt RunReceipt) ProgramPage() TargetNavigationPage {
	page := receipt.programPage
	page.ProgramTarget = receipt.programPage.ProgramTarget.Snapshot()
	return page
}

// DecodeRunManifest reads one manifest and checks its shape.
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

// ReadRunManifest reads the manifest a run directory holds.
func ReadRunManifest(runDir string) (RunManifest, error) {
	data, err := os.ReadFile(filepath.Join(runDir, RunManifestFilename))
	if err != nil {
		return RunManifest{}, fmt.Errorf("report manifest: read: %w", err)
	}
	return DecodeRunManifest(data)
}

// RemoveRunManifest forgets a run's manifest, which is how a run that failed
// to publish stops looking like one that did.
func RemoveRunManifest(runDir string) error {
	manifestPath := filepath.Join(runDir, RunManifestFilename)
	if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("report manifest: remove stale manifest: %w", err)
	}
	return nil
}

// ResolveAnalysisRoot is the analysed directory as an absolute path.
func (m RunManifest) ResolveAnalysisRoot() (string, error) {
	root, err := canonicalManifestDirectory("analysis root", m.AnalysisRoot)
	if err != nil {
		return "", fmt.Errorf("report manifest: %w", err)
	}
	return root, nil
}

func prepareRunManifest(
	data *ReportData,
	source RunSource,
	standaloneSource *standaloneSourceConfig,
) (RunManifest, error) {
	if err := source.validate(); err != nil {
		return RunManifest{}, err
	}
	if data.ProgramPortfolio == nil {
		return RunManifest{}, fmt.Errorf("report manifest: program portfolio is missing")
	}
	defaultEntry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return RunManifest{}, fmt.Errorf("report manifest: program portfolio: %w", err)
	}
	var sourceAuthority *StandaloneSourceAuthority
	if standaloneSource != nil {
		sourceAuthority = &StandaloneSourceAuthority{
			Host:          standaloneSource.hostName,
			RepositoryURL: standaloneSource.repositoryURL,
		}
	}
	manifest := RunManifest{
		Version:             CurrentRunManifestVersion,
		RepositoryState:     source.Repository,
		AnalysisRoot:        source.AnalysisRoot,
		StandaloneSource:    sourceAuthority,
		ReportFormatVersion: data.FormatVersion,
		ProgramTargetID:     defaultEntry.Target.ID,
	}
	if err := manifest.Validate(); err != nil {
		return RunManifest{}, err
	}
	return manifest, nil
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
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("report manifest: write temporary file: %w", err)
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
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", label, err)
	}
	return filepath.Clean(absolute), nil
}

func validateAnalysisRoot(repositoryRoot, analysisRoot string) error {
	if !filepath.IsAbs(analysisRoot) || filepath.Clean(analysisRoot) != analysisRoot {
		return fmt.Errorf("analysis root must be an absolute clean path")
	}
	relative, err := filepath.Rel(repositoryRoot, analysisRoot)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("analysis root is outside the repository")
	}
	return nil
}

func validateManifestPath(value string) error {
	if value == "" || len(value) > maxManifestPathBytes || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\\') || filepath.ToSlash(filepath.Clean(value)) != value ||
		strings.HasPrefix(value, "/") || strings.HasPrefix(value, "../") || value == ".." || value == "." {
		return fmt.Errorf("path is not a clean relative slash path")
	}
	return nil
}
