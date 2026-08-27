package report

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	_ "embed"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/snapshot"
)

const (
	StandaloneTargetBundleVersion     = 3
	StandaloneTargetNavigationVersion = 5

	// One GiB is a terminal aggregate payload bound, not a prefix-clipping
	// budget. Every ready target is either present in full or publication fails.
	MaxStandaloneTargetBundlePayloadBytes int64 = 1 << 30

	maxPreparedStandaloneTargetHTMLBytes = int64(256 << 20)
	maxStandaloneTargetBundleHeaderBytes = int64(8 << 20)
	maxStandaloneTargetBundleSealBytes   = int64(512)
	maxStandaloneTargetBundleOverhead    = int64(64 << 20)

	standaloneTargetBundlePlaceholder  = "REPOMAP_STANDALONE_TARGET_BUNDLE_PLACEHOLDER_3D909014"
	standaloneTargetBundleMarkerPrefix = "<!-- repomap-standalone-target-bundle:v3:"
	standaloneTargetBundleMarkerSuffix = " -->"
	standaloneTargetBundleSealPrefix   = "<!-- repomap-standalone-target-bundle-seal:v3:"
	standaloneTargetBundleSealSuffix   = " -->\n"

	reportDataScriptOpen  = `<script type="application/json" id="rm-report-data">`
	reportDataScriptClose = `</script>`
)

//go:embed templates/standalone_target_bootstrap.js
var standaloneTargetBootstrapJS string

// PreparedStandaloneTarget is an opaque, fully scrubbed hosted target
// payload. Values can only be produced by the authorized GitHub/GitLab
// generation seams below; callers cannot alter target/source authority.
type PreparedStandaloneTarget struct {
	prepared *preparedStandaloneTarget
}

type preparedStandaloneTarget struct {
	analysisTarget *analysistarget.Target
	programPage    TargetNavigationPage
	payload        []byte
	host           string
	repositoryURL  string
	revision       string
	repoName       string
	localRoots     []string
}

// StandaloneTargetBundleIdentity is the small public identity returned by the
// bounded streaming inspector. It binds either the legacy Go container/page
// pair or one language-neutral program-page portfolio, never both, without
// exposing sibling run IDs or filesystem locations.
type StandaloneTargetBundleIdentity struct {
	Version                    int    `json:"version"`
	TargetRunContainerSHA256   string `json:"target_run_container_sha256,omitempty"`
	TargetPagePortfolioSHA256  string `json:"target_page_portfolio_sha256,omitempty"`
	ProgramPagePortfolioSHA256 string `json:"program_page_portfolio_sha256,omitempty"`
	DefaultTargetIndex         int    `json:"default_target_index"`
	TargetCount                int    `json:"target_count"`
}

// StandaloneTargetBundleResourceLimitError is a terminal publication result.
// The bundle never truncates or silently omits a ready target.
type StandaloneTargetBundleResourceLimitError struct {
	LimitBytes  int64
	ActualBytes int64
}

func (err *StandaloneTargetBundleResourceLimitError) Error() string {
	if err == nil {
		return "report: standalone target bundle resource limit exceeded"
	}
	return fmt.Sprintf(
		"report: standalone target payloads require %d bytes; limit is %d bytes",
		err.ActualBytes,
		err.LimitBytes,
	)
}

// GenerateAuthorizedGitHubPreparedWithOptions performs the normal authorized
// GitHub generation (including canonical JSON, report.html and its manifest),
// then returns an opaque scrubbed payload for the selected-target bundle.
func GenerateAuthorizedGitHubPreparedWithOptions(
	runDir string,
	authority RunAuthority,
	repositoryURL string,
	options RenderOptions,
) (PreparedStandaloneTarget, error) {
	if err := GenerateAuthorizedGitHubWithOptions(runDir, authority, repositoryURL, options); err != nil {
		return PreparedStandaloneTarget{}, err
	}
	return prepareGeneratedStandaloneTarget(runDir, authority, "GitHub")
}

// GenerateAuthorizedGitLabPreparedWithOptions is the GitLab equivalent of
// GenerateAuthorizedGitHubPreparedWithOptions.
func GenerateAuthorizedGitLabPreparedWithOptions(
	runDir string,
	authority RunAuthority,
	repositoryURL string,
	options RenderOptions,
) (PreparedStandaloneTarget, error) {
	if err := GenerateAuthorizedGitLabWithOptions(runDir, authority, repositoryURL, options); err != nil {
		return PreparedStandaloneTarget{}, err
	}
	return prepareGeneratedStandaloneTarget(runDir, authority, "GitLab")
}

func prepareGeneratedStandaloneTarget(
	runDir string,
	authority RunAuthority,
	wantHost string,
) (PreparedStandaloneTarget, error) {
	if err := authority.validate(); err != nil {
		return PreparedStandaloneTarget{}, err
	}
	htmlPath := filepath.Join(runDir, "report.html")
	info, err := os.Lstat(htmlPath)
	if err != nil {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: inspect prepared standalone target: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxPreparedStandaloneTargetHTMLBytes {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: prepared standalone target HTML is outside the byte budget")
	}
	htmlBytes, err := os.ReadFile(htmlPath)
	if err != nil {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: read prepared standalone target: %w", err)
	}
	absoluteRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: resolve prepared standalone target run: %w", err)
	}
	localRoots := []string{absoluteRunDir, authority.analysisRoot, authority.repository.Identity}
	analysisTarget, err := preparedStandaloneAnalysisTarget(runDir)
	if err != nil {
		return PreparedStandaloneTarget{}, err
	}
	prepared, err := prepareStandaloneTargetFromHTMLWithAuthority(
		htmlBytes, localRoots, analysisTarget,
	)
	if err != nil {
		return PreparedStandaloneTarget{}, err
	}
	if prepared.host != wantHost || prepared.revision != strings.ToLower(authority.repository.Head) {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: prepared standalone target source authority mismatch")
	}
	page, err := LoadTargetNavigationPage(runDir, filepath.Base(runDir))
	if err != nil {
		return PreparedStandaloneTarget{}, err
	}
	if prepared.programPage.ProgramTarget.ID != page.ProgramTarget.ID {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: prepared standalone target program authority mismatch")
	}
	prepared.programPage = page
	return PreparedStandaloneTarget{prepared: prepared}, nil
}

// PrepareStandaloneProgramPage restores one opaque standalone payload from an
// already-generated, manifest-verified program page. Unlike the legacy Go
// target seam, its cross-page authority is only the exact ProgramTarget+run
// binding in programpage.Portfolio.
func PrepareStandaloneProgramPage(runDir string) (PreparedStandaloneTarget, error) {
	manifest, err := ReadRunManifest(runDir)
	if err != nil {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: prepare standalone program page manifest: %w", err)
	}
	if manifest.StandaloneSource == nil || manifest.MaterialInputs.ProgramPagePortfolioSHA256 == "" {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: standalone program page lacks hosted portfolio authority")
	}
	portfolio, err := manifestStandaloneProgramPageAuthority(runDir, manifest)
	if err != nil {
		return PreparedStandaloneTarget{}, err
	}
	runID := filepath.Base(filepath.Clean(runDir))
	var binding *programpage.Page
	for position := range portfolio.Pages {
		candidate := &portfolio.Pages[position]
		if candidate.RunID != runID || candidate.Target.ID != manifest.MaterialInputs.ProgramTargetID {
			continue
		}
		if binding != nil {
			return PreparedStandaloneTarget{}, fmt.Errorf("report: standalone program page binding is ambiguous")
		}
		binding = candidate
	}
	if binding == nil {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: standalone program page is absent from portfolio")
	}

	htmlPath := filepath.Join(runDir, "report.html")
	info, err := os.Lstat(htmlPath)
	if err != nil {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: inspect standalone program page: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxPreparedStandaloneTargetHTMLBytes {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: standalone program page HTML is outside the byte budget")
	}
	htmlBytes, err := os.ReadFile(htmlPath)
	if err != nil {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: read standalone program page: %w", err)
	}
	absoluteRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: resolve standalone program page run: %w", err)
	}
	prepared, err := prepareStandaloneTargetFromHTML(htmlBytes, []string{
		absoluteRunDir, manifest.AnalysisRoot, manifest.RepositoryState.Identity,
	})
	if err != nil {
		return PreparedStandaloneTarget{}, err
	}
	page, err := LoadTargetNavigationPage(runDir, runID)
	if err != nil {
		return PreparedStandaloneTarget{}, err
	}
	if !reflect.DeepEqual(page.ProgramTarget, binding.Target) ||
		!reflect.DeepEqual(prepared.programPage.ProgramTarget, binding.Target) {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: standalone program page target authority mismatch")
	}
	if prepared.host != manifest.StandaloneSource.Host ||
		prepared.repositoryURL != manifest.StandaloneSource.RepositoryURL ||
		prepared.revision != strings.ToLower(manifest.MaterialInputs.SelectedRevision) {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: standalone program page source authority mismatch")
	}
	prepared.programPage = page
	return PreparedStandaloneTarget{prepared: prepared}, nil
}

// preparedStandaloneAnalysisTarget reads the outer target from the canonical,
// manifest-verified report. Ordinary language-neutral browser payloads omit
// this Go-only authority, while the multi-target bundle needs it to bind each
// prepared payload to the selected TargetRunContainer projection.
func preparedStandaloneAnalysisTarget(runDir string) (*analysistarget.Target, error) {
	manifest, err := ReadRunManifest(runDir)
	if err != nil {
		return nil, fmt.Errorf("report: prepared standalone target manifest: %w", err)
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return nil, fmt.Errorf("report: open prepared standalone target run: %w", err)
	}
	defer root.Close()
	reportJSON, err := readManifestFile(root, "report.json", maxManifestReportBytes)
	if err != nil {
		return nil, fmt.Errorf("report: read prepared standalone target report: %w", err)
	}
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
		return nil, fmt.Errorf("report: verify prepared standalone target report: %w", err)
	}
	envelope, err := decodeStrictReportJSON(reportJSON)
	if err != nil {
		return nil, fmt.Errorf("report: decode prepared standalone target report: %w", err)
	}
	if envelope.AnalysisTarget == nil {
		return nil, fmt.Errorf("report: prepared standalone target report lacks target authority")
	}
	owned := envelope.AnalysisTarget.Snapshot()
	return &owned, nil
}

func prepareStandaloneTargetFromHTML(
	htmlBytes []byte,
	localRoots []string,
) (*preparedStandaloneTarget, error) {
	return prepareStandaloneTargetFromHTMLWithAuthority(htmlBytes, localRoots, nil)
}

func prepareStandaloneTargetFromHTMLWithAuthority(
	htmlBytes []byte,
	localRoots []string,
	authoritativeTarget *analysistarget.Target,
) (*preparedStandaloneTarget, error) {
	start := bytes.Index(htmlBytes, []byte(reportDataScriptOpen))
	if start < 0 || bytes.LastIndex(htmlBytes, []byte(reportDataScriptOpen)) != start {
		return nil, fmt.Errorf("report: prepared standalone target has invalid report payload marker")
	}
	start += len(reportDataScriptOpen)
	endOffset := bytes.Index(htmlBytes[start:], []byte(reportDataScriptClose))
	if endOffset < 0 {
		return nil, fmt.Errorf("report: prepared standalone target payload is unterminated")
	}
	payload := htmlBytes[start : start+endOffset]
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var decoded map[string]any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("report: decode prepared standalone target payload: %w", err)
	}
	if err := ensureJSONDecoderEOF(decoder); err != nil {
		return nil, fmt.Errorf("report: decode prepared standalone target payload: %w", err)
	}
	delete(decoded, "target_navigation")
	scrubRenderLocalPaths(decoded, localRoots)
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("report: encode prepared standalone target payload: %w", err)
	}

	envelope, err := decodeStrictReportJSON(canonical)
	if err != nil {
		return nil, fmt.Errorf("report: inspect prepared standalone target payload: %w", err)
	}
	if envelope.FormatVersion != CurrentFormatVersion {
		return nil, fmt.Errorf("report: prepared standalone target has incompatible report authority")
	}
	if authoritativeTarget != nil {
		if err := authoritativeTarget.Validate(); err != nil {
			return nil, fmt.Errorf("report: prepared standalone target canonical analysis target: %w", err)
		}
		if envelope.AnalysisTarget != nil && !reflect.DeepEqual(*envelope.AnalysisTarget, *authoritativeTarget) {
			return nil, fmt.Errorf("report: prepared standalone target analysis target authority mismatch")
		}
		decoded["analysis_target"] = authoritativeTarget.Snapshot()
		// Re-run the complete browser JSON round-trip after injecting typed
		// authority. This gives the restored nested target the same canonical
		// map-key ordering as the manifest-derived verification projection.
		canonical, err = marshalHTMLPayloadWithLocalRoots(decoded, nil)
		if err != nil {
			return nil, fmt.Errorf("report: encode prepared standalone target authority: %w", err)
		}
		envelope, err = decodeStrictReportJSON(canonical)
		if err != nil {
			return nil, fmt.Errorf("report: inspect prepared standalone target authority: %w", err)
		}
	}
	var analysisTarget *analysistarget.Target
	if envelope.AnalysisTarget != nil {
		if err := envelope.AnalysisTarget.Validate(); err != nil {
			return nil, fmt.Errorf("report: prepared standalone target analysis target: %w", err)
		}
		owned := envelope.AnalysisTarget.Snapshot()
		analysisTarget = &owned
	}
	if envelope.ProgramPortfolio == nil {
		return nil, fmt.Errorf("report: prepared standalone target requires a complete program portfolio")
	}
	if err := envelope.ProgramPortfolio.Validate(); err != nil {
		return nil, fmt.Errorf("report: prepared standalone target program portfolio: %w", err)
	}
	defaultEntry, err := envelope.ProgramPortfolio.defaultEntry()
	if err != nil {
		return nil, fmt.Errorf("report: prepared standalone target default program target: %w", err)
	}
	if envelope.RepoName == "" || envelope.RepoName != strings.TrimSpace(envelope.RepoName) {
		return nil, fmt.Errorf("report: prepared standalone target repository name is invalid")
	}

	host := ""
	repositoryURL := ""
	revision := ""
	switch {
	case envelope.GitHubSourceLinks != nil && envelope.GitLabSourceLinks == nil:
		if err := envelope.GitHubSourceLinks.validate(); err != nil {
			return nil, err
		}
		host = "GitHub"
		repositoryURL = envelope.GitHubSourceLinks.RepositoryURL
		revision = envelope.GitHubSourceLinks.Revision
	case envelope.GitLabSourceLinks != nil && envelope.GitHubSourceLinks == nil:
		if err := envelope.GitLabSourceLinks.validate(); err != nil {
			return nil, err
		}
		host = "GitLab"
		repositoryURL = envelope.GitLabSourceLinks.RepositoryURL
		revision = envelope.GitLabSourceLinks.Revision
	default:
		return nil, fmt.Errorf("report: prepared standalone target requires exactly one external source host")
	}
	if envelope.CapturedRevision != revision {
		return nil, fmt.Errorf("report: prepared standalone target captured revision does not match source authority")
	}
	if envelope.CapturedInputCount < 0 {
		return nil, fmt.Errorf("report: prepared standalone target captured input count is invalid")
	}
	previousPath := ""
	for index, sourcePath := range envelope.OpenablePaths {
		if err := validateManifestPath(sourcePath); err != nil {
			return nil, fmt.Errorf("report: prepared standalone target openable path %d: %w", index, err)
		}
		if previousPath != "" && previousPath >= sourcePath {
			return nil, fmt.Errorf("report: prepared standalone target openable paths are not uniquely sorted")
		}
		previousPath = sourcePath
	}

	return &preparedStandaloneTarget{
		analysisTarget: analysisTarget,
		programPage: TargetNavigationPage{
			ProgramTarget: defaultEntry.Target.Snapshot(),
		},
		payload:       canonical,
		host:          host,
		repositoryURL: repositoryURL,
		revision:      revision,
		repoName:      envelope.RepoName,
		localRoots:    normalizedStandaloneLocalRoots(localRoots),
	}, nil
}

func ensureJSONDecoderEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func normalizedStandaloneLocalRoots(roots []string) []string {
	result := make([]string, 0, len(roots))
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
		result = append(result, root)
	}
	return result
}

type standaloneTargetBundleWire struct {
	Version            int                          `json:"version"`
	DefaultTargetIndex int                          `json:"default_target_index"`
	Targets            []standaloneTargetBundleItem `json:"targets"`
}

type standaloneTargetBundleItem struct {
	TargetID    string          `json:"target_id"`
	Language    string          `json:"language"`
	Kind        string          `json:"kind"`
	DisplayName string          `json:"display_name"`
	Href        string          `json:"href"`
	Payload     json.RawMessage `json:"payload"`
}

type validatedStandaloneTargetBundle struct {
	identity      StandaloneTargetBundleIdentity
	defaultTarget *preparedStandaloneTarget
	targets       []standaloneTargetBundleItem
}

// WriteStandaloneTargetBundleAtomic replaces runDir/report.html only after
// the complete canonical bundle has been validated and written. The caller
// supplies one opaque payload per selected target; canonical order and default
// ownership come exclusively from container+portfolio.
func WriteStandaloneTargetBundleAtomic(
	runDir string,
	container snapshot.TargetRunContainer,
	portfolio snapshot.TargetPagePortfolio,
	ready []PreparedStandaloneTarget,
) error {
	validated, err := validateStandaloneTargetBundle(container, portfolio, ready)
	if err != nil {
		return err
	}
	return writeValidatedStandaloneTargetBundleAtomic(runDir, validated)
}

// WriteStandaloneProgramPageBundleAtomic publishes one self-contained
// --no-serve document from a complete language-neutral analyzed-page
// portfolio. The portfolio may contain one page when other selected targets
// failed. Canonical order, default ownership, and sibling run bindings come
// only from the sealed programpage.Portfolio.
func WriteStandaloneProgramPageBundleAtomic(
	runDir string,
	portfolio programpage.Portfolio,
	ready []PreparedStandaloneTarget,
) error {
	validated, err := validateStandaloneProgramPageBundle(portfolio, ready)
	if err != nil {
		return err
	}
	defaultRunID := ""
	for _, binding := range portfolio.Pages {
		if binding.Target.ID == portfolio.DefaultTargetID {
			defaultRunID = binding.RunID
			break
		}
	}
	if defaultRunID == "" || filepath.Base(filepath.Clean(runDir)) != defaultRunID {
		return fmt.Errorf("report: standalone program page bundle destination is not the default run")
	}
	return writeValidatedStandaloneTargetBundleAtomic(runDir, validated)
}

// WriteProgramPageBundleFromArtifactsAtomic publishes the sole user-facing
// HTML for a neutral ProgramPagePortfolio. The analyzed-page portfolio can
// contain one page when every other selected target failed before producing a
// page. Every payload is projected directly from its manifest-verified
// report.json and page-local artifacts; target-local HTML is neither required
// nor merged.
func WriteProgramPageBundleFromArtifactsAtomic(
	runDir string,
	portfolio programpage.Portfolio,
) error {
	if err := portfolio.Validate(); err != nil {
		return fmt.Errorf("report: program page bundle authority: %w", err)
	}
	defaultRunID := ""
	defaultIndex := -1
	for index, binding := range portfolio.Pages {
		if binding.Target.ID == portfolio.DefaultTargetID {
			defaultRunID = binding.RunID
			defaultIndex = index
			break
		}
	}
	if defaultRunID == "" || filepath.Base(filepath.Clean(runDir)) != defaultRunID {
		return fmt.Errorf("report: program page bundle destination is not the default run")
	}
	defaultManifest, err := ReadRunManifest(runDir)
	if err != nil {
		return fmt.Errorf("report: program page bundle default manifest: %w", err)
	}
	manifestPortfolio, err := manifestStandaloneProgramPageAuthority(runDir, defaultManifest)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(manifestPortfolio, portfolio) {
		return fmt.Errorf("report: program page bundle portfolio does not match the default manifest")
	}

	runsDir := filepath.Dir(filepath.Clean(runDir))
	targets := make([]standaloneTargetBundleItem, 0, len(portfolio.Pages))
	aggregate := int64(0)
	repoName := ""
	for index, binding := range portfolio.Pages {
		item, itemRepoName, itemErr := expectedStandaloneProgramPageBundleItem(
			runsDir, binding, index, defaultManifest,
		)
		if itemErr != nil {
			return fmt.Errorf("report: program page bundle target %d: %w", index, itemErr)
		}
		if repoName == "" {
			repoName = itemRepoName
		} else if itemRepoName != repoName {
			return fmt.Errorf("report: program page bundle repository identity mismatch")
		}
		nextAggregate, aggregateErr := standaloneTargetAggregateBytes(
			aggregate, int64(len(item.Payload)),
		)
		if aggregateErr != nil {
			return aggregateErr
		}
		aggregate = nextAggregate
		targets = append(targets, item)
	}
	if repoName == "" || defaultIndex < 0 {
		return fmt.Errorf("report: program page bundle default target is missing")
	}
	validated := &validatedStandaloneTargetBundle{
		identity: StandaloneTargetBundleIdentity{
			Version:                    StandaloneTargetBundleVersion,
			ProgramPagePortfolioSHA256: portfolio.SHA256,
			DefaultTargetIndex:         defaultIndex,
			TargetCount:                len(portfolio.Pages),
		},
		defaultTarget: &preparedStandaloneTarget{repoName: repoName},
		targets:       targets,
	}
	return writeValidatedStandaloneTargetBundleAtomic(runDir, validated)
}

func writeValidatedStandaloneTargetBundleAtomic(
	runDir string,
	validated *validatedStandaloneTargetBundle,
) error {
	skeleton, err := standaloneTargetBundleSkeleton(validated)
	if err != nil {
		return err
	}
	placeholder := []byte(standaloneTargetBundlePlaceholder)
	if bytes.Count(skeleton, placeholder) != 1 {
		return fmt.Errorf("report: standalone target bundle template seam is invalid")
	}
	cut := bytes.Index(skeleton, placeholder)

	info, err := os.Stat(runDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("report: standalone target bundle run directory is unavailable")
	}
	temporary, err := os.CreateTemp(runDir, ".report.html.bundle-*")
	if err != nil {
		return fmt.Errorf("report: create standalone target bundle: %w", err)
	}
	temporaryPath := temporary.Name()
	installed := false
	defer func() {
		if !installed {
			_ = temporary.Close()
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return fmt.Errorf("report: set standalone target bundle mode: %w", err)
	}

	digest := sha256.New()
	writer := io.MultiWriter(temporary, digest)
	if _, err := writer.Write(skeleton[:cut]); err != nil {
		return fmt.Errorf("report: write standalone target bundle prefix: %w", err)
	}
	if err := writeStandaloneTargetBundleSection(writer, validated); err != nil {
		return err
	}
	if _, err := writer.Write(skeleton[cut+len(placeholder):]); err != nil {
		return fmt.Errorf("report: write standalone target bundle suffix: %w", err)
	}
	seal := standaloneTargetBundleSeal(digest)
	if _, err := temporary.WriteString(seal); err != nil {
		return fmt.Errorf("report: write standalone target bundle seal: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("report: sync standalone target bundle: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("report: close standalone target bundle: %w", err)
	}
	finalPath := filepath.Join(runDir, "report.html")
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return fmt.Errorf("report: install standalone target bundle: %w", err)
	}
	installed = true
	return nil
}

func validateStandaloneProgramPageBundle(
	portfolio programpage.Portfolio,
	ready []PreparedStandaloneTarget,
) (*validatedStandaloneTargetBundle, error) {
	if err := portfolio.Validate(); err != nil {
		return nil, fmt.Errorf("report: standalone program page bundle authority: %w", err)
	}
	if len(ready) != len(portfolio.Pages) {
		return nil, fmt.Errorf("report: standalone program page bundle requires one prepared page per target")
	}

	preparedByTargetID := make(map[string]*preparedStandaloneTarget, len(ready))
	for index, opaque := range ready {
		prepared := opaque.prepared
		if prepared == nil || len(prepared.payload) == 0 {
			return nil, fmt.Errorf("report: standalone program page bundle prepared page %d is invalid", index)
		}
		if err := validateTargetNavigationPage(prepared.programPage); err != nil {
			return nil, fmt.Errorf("report: standalone program page bundle prepared page %d: %w", index, err)
		}
		targetID := prepared.programPage.ProgramTarget.ID
		if _, duplicate := preparedByTargetID[targetID]; duplicate {
			return nil, fmt.Errorf("report: standalone program page bundle contains duplicate prepared target")
		}
		preparedByTargetID[targetID] = prepared
	}

	defaultIndex := -1
	aggregate := int64(0)
	targets := make([]standaloneTargetBundleItem, 0, len(portfolio.Pages))
	var first *preparedStandaloneTarget
	var defaultPrepared *preparedStandaloneTarget
	for index, binding := range portfolio.Pages {
		prepared, found := preparedByTargetID[binding.Target.ID]
		if !found {
			return nil, fmt.Errorf("report: standalone program page bundle is missing one portfolio target")
		}
		if prepared.programPage.RunID != binding.RunID ||
			!reflect.DeepEqual(prepared.programPage.ProgramTarget, binding.Target) {
			return nil, fmt.Errorf("report: standalone program page bundle target binding mismatch")
		}
		if first == nil {
			first = prepared
		} else if prepared.host != first.host || prepared.repositoryURL != first.repositoryURL ||
			prepared.revision != first.revision {
			return nil, fmt.Errorf("report: standalone program page bundle source authority mismatch")
		}
		for _, root := range prepared.localRoots {
			if bytes.Contains(prepared.payload, []byte(root)) {
				return nil, fmt.Errorf("report: standalone program page bundle retained a local path")
			}
		}
		for _, sibling := range portfolio.Pages {
			if bytes.Contains(prepared.payload, []byte(sibling.RunID)) {
				return nil, fmt.Errorf("report: standalone program page bundle retained a sibling run id")
			}
		}
		nextAggregate, err := standaloneTargetAggregateBytes(aggregate, int64(len(prepared.payload)))
		if err != nil {
			return nil, err
		}
		aggregate = nextAggregate
		targets = append(targets, standaloneTargetBundleItem{
			TargetID:    binding.Target.ID,
			Language:    binding.Target.Language,
			Kind:        binding.Target.Kind,
			DisplayName: binding.Target.Name,
			Href:        standaloneTargetHref(index),
			Payload:     json.RawMessage(prepared.payload),
		})
		delete(preparedByTargetID, binding.Target.ID)
		if binding.Target.ID == portfolio.DefaultTargetID {
			defaultIndex = index
			defaultPrepared = prepared
		}
	}
	if len(preparedByTargetID) != 0 {
		return nil, fmt.Errorf("report: standalone program page bundle contains a foreign prepared target")
	}
	if defaultIndex < 0 || defaultPrepared == nil {
		return nil, fmt.Errorf("report: standalone program page bundle default target is missing")
	}
	return &validatedStandaloneTargetBundle{
		identity: StandaloneTargetBundleIdentity{
			Version:                    StandaloneTargetBundleVersion,
			ProgramPagePortfolioSHA256: portfolio.SHA256,
			DefaultTargetIndex:         defaultIndex,
			TargetCount:                len(portfolio.Pages),
		},
		defaultTarget: defaultPrepared,
		targets:       targets,
	}, nil
}

func validateStandaloneTargetBundle(
	container snapshot.TargetRunContainer,
	portfolio snapshot.TargetPagePortfolio,
	ready []PreparedStandaloneTarget,
) (*validatedStandaloneTargetBundle, error) {
	if err := portfolio.ValidateAgainstContainer(container); err != nil {
		return nil, fmt.Errorf("report: standalone target bundle authority: %w", err)
	}
	if len(container.Targets) <= 1 {
		return nil, fmt.Errorf("report: standalone target bundle requires multiple selected targets")
	}
	if len(ready) != len(container.Targets) {
		return nil, fmt.Errorf("report: standalone target bundle requires one prepared ProgramPortfolio page per selected target")
	}

	preparedByRef := make(map[string]*preparedStandaloneTarget, len(ready))
	for index, opaque := range ready {
		prepared := opaque.prepared
		if prepared == nil || prepared.analysisTarget == nil || len(prepared.payload) == 0 {
			return nil, fmt.Errorf("report: standalone target bundle prepared target %d is invalid", index)
		}
		if err := validateTargetNavigationPage(prepared.programPage); err != nil {
			return nil, fmt.Errorf("report: standalone target bundle prepared target %d: %w", index, err)
		}
		if _, duplicate := preparedByRef[prepared.analysisTarget.Ref]; duplicate {
			return nil, fmt.Errorf("report: standalone target bundle contains duplicate prepared target")
		}
		preparedByRef[prepared.analysisTarget.Ref] = prepared
	}

	defaultIndex := -1
	aggregate := int64(0)
	targets := make([]standaloneTargetBundleItem, 0, len(container.Targets))
	seenProgramTargetIDs := make(map[string]struct{}, len(container.Targets))
	var first *preparedStandaloneTarget
	var defaultPrepared *preparedStandaloneTarget
	for index, projection := range container.Targets {
		if projection.Target.Ref == container.DefaultTargetRef {
			defaultIndex = index
		}
		prepared, found := preparedByRef[projection.Target.Ref]
		if !found {
			return nil, fmt.Errorf("report: standalone target bundle is missing one selected target")
		}
		if prepared.programPage.RunID != portfolio.Targets[index].RunID {
			return nil, fmt.Errorf("report: standalone target bundle program page run mismatch")
		}
		if !reflect.DeepEqual(*prepared.analysisTarget, projection.Target) {
			return nil, fmt.Errorf("report: standalone target bundle target authority mismatch")
		}
		item := standaloneTargetBundleItem{
			TargetID:    prepared.programPage.ProgramTarget.ID,
			Language:    prepared.programPage.ProgramTarget.Language,
			Kind:        prepared.programPage.ProgramTarget.Kind,
			DisplayName: prepared.programPage.ProgramTarget.Name,
		}
		if _, duplicate := seenProgramTargetIDs[item.TargetID]; duplicate {
			return nil, fmt.Errorf("report: standalone target bundle contains duplicate program target identity")
		}
		seenProgramTargetIDs[item.TargetID] = struct{}{}
		if first == nil {
			first = prepared
		} else if prepared.host != first.host || prepared.repositoryURL != first.repositoryURL ||
			prepared.revision != first.revision {
			return nil, fmt.Errorf("report: standalone target bundle source authority mismatch")
		}
		for _, root := range prepared.localRoots {
			if bytes.Contains(prepared.payload, []byte(root)) {
				return nil, fmt.Errorf("report: standalone target bundle retained a local path")
			}
		}
		for _, sibling := range portfolio.Targets {
			if sibling.RunID != "" && bytes.Contains(prepared.payload, []byte(sibling.RunID)) {
				return nil, fmt.Errorf("report: standalone target bundle retained a sibling run id")
			}
		}
		nextAggregate, aggregateErr := standaloneTargetAggregateBytes(
			aggregate,
			int64(len(prepared.payload)),
		)
		if aggregateErr != nil {
			return nil, aggregateErr
		}
		aggregate = nextAggregate
		item.Href = standaloneTargetHref(index)
		item.Payload = json.RawMessage(prepared.payload)
		targets = append(targets, item)
		delete(preparedByRef, projection.Target.Ref)
		if index == defaultIndex {
			defaultPrepared = prepared
		}
	}
	if len(preparedByRef) != 0 {
		return nil, fmt.Errorf("report: standalone target bundle contains a foreign prepared target")
	}
	if defaultIndex < 0 || defaultPrepared == nil {
		return nil, fmt.Errorf("report: standalone target bundle default target is missing")
	}
	return &validatedStandaloneTargetBundle{
		identity: StandaloneTargetBundleIdentity{
			Version:                   StandaloneTargetBundleVersion,
			TargetRunContainerSHA256:  container.SHA256,
			TargetPagePortfolioSHA256: portfolio.SHA256,
			DefaultTargetIndex:        defaultIndex,
			TargetCount:               len(container.Targets),
		},
		defaultTarget: defaultPrepared,
		targets:       targets,
	}, nil
}

func standaloneTargetHref(index int) string {
	return fmt.Sprintf("?target=%d#/program", index)
}

func standaloneTargetAggregateBytes(current, next int64) (int64, error) {
	if current < 0 || next < 0 || current > MaxStandaloneTargetBundlePayloadBytes-next {
		actual := current + next
		if actual < 0 {
			actual = MaxStandaloneTargetBundlePayloadBytes + 1
		}
		return 0, &StandaloneTargetBundleResourceLimitError{
			LimitBytes: MaxStandaloneTargetBundlePayloadBytes, ActualBytes: actual,
		}
	}
	return current + next, nil
}

func standaloneTargetBundleSkeleton(validated *validatedStandaloneTargetBundle) ([]byte, error) {
	prepared := validated.defaultTarget
	title := prepared.repoName
	return executeProgramReport(programReportTemplateData{
		Title:                  title,
		StandaloneTargetBundle: template.HTML(standaloneTargetBundlePlaceholder),
	})
}

func writeStandaloneTargetBundleSection(
	w io.Writer,
	validated *validatedStandaloneTargetBundle,
) error {
	identityJSON, err := json.Marshal(validated.identity)
	if err != nil {
		return fmt.Errorf("report: encode standalone target bundle identity: %w", err)
	}
	marker := standaloneTargetBundleMarkerPrefix +
		base64.RawURLEncoding.EncodeToString(identityJSON) + standaloneTargetBundleMarkerSuffix
	if _, err := io.WriteString(w, marker+"\n  <script type=\"application/json\" id=\"rm-standalone-target-bundle\">"); err != nil {
		return fmt.Errorf("report: write standalone target bundle marker: %w", err)
	}
	wire := standaloneTargetBundleWire{
		Version: StandaloneTargetBundleVersion, DefaultTargetIndex: validated.identity.DefaultTargetIndex,
		Targets: validated.targets,
	}
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(wire); err != nil {
		return fmt.Errorf("report: write standalone target bundle payloads: %w", err)
	}
	if _, err := io.WriteString(w,
		"  </script>\n  <script type=\"application/json\" id=\"rm-report-data\"></script>\n"+
			"  <script id=\"rm-standalone-target-bootstrap\">"+standaloneTargetBootstrapJS+"</script>",
	); err != nil {
		return fmt.Errorf("report: write standalone target bootstrap: %w", err)
	}
	return nil
}

func standaloneTargetBundleSeal(digest hash.Hash) string {
	return standaloneTargetBundleSealPrefix + hex.EncodeToString(digest.Sum(nil)) +
		standaloneTargetBundleSealSuffix
}

// InspectStandaloneTargetBundleHTML detects and verifies one D286 bundle
// without loading the potentially one-GiB artifact into memory. Ordinary
// pre-bundle report HTML returns found=false so recovery can rebuild it.
func InspectStandaloneTargetBundleHTML(
	path string,
) (StandaloneTargetBundleIdentity, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return StandaloneTargetBundleIdentity{}, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return StandaloneTargetBundleIdentity{}, false, err
	}
	maxFileBytes := MaxStandaloneTargetBundlePayloadBytes + maxStandaloneTargetBundleOverhead
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxFileBytes {
		return StandaloneTargetBundleIdentity{}, false, fmt.Errorf("report: standalone target bundle file is outside the byte budget")
	}

	headerBytes := min(info.Size(), maxStandaloneTargetBundleHeaderBytes)
	header := make([]byte, int(headerBytes))
	if _, err := io.ReadFull(file, header); err != nil {
		return StandaloneTargetBundleIdentity{}, false, err
	}
	prefix := []byte(standaloneTargetBundleMarkerPrefix)
	markerStart := bytes.Index(header, prefix)
	if markerStart < 0 {
		return StandaloneTargetBundleIdentity{}, false, nil
	}
	if bytes.LastIndex(header, prefix) != markerStart {
		return StandaloneTargetBundleIdentity{}, true, fmt.Errorf("report: duplicate standalone target bundle marker")
	}
	encodedStart := markerStart + len(prefix)
	encodedEndOffset := bytes.Index(header[encodedStart:], []byte(standaloneTargetBundleMarkerSuffix))
	if encodedEndOffset < 0 {
		return StandaloneTargetBundleIdentity{}, true, fmt.Errorf("report: incomplete standalone target bundle marker")
	}
	identityJSON, err := base64.RawURLEncoding.DecodeString(
		string(header[encodedStart : encodedStart+encodedEndOffset]),
	)
	if err != nil {
		return StandaloneTargetBundleIdentity{}, true, fmt.Errorf("report: decode standalone target bundle marker: %w", err)
	}
	var identity StandaloneTargetBundleIdentity
	if err := json.Unmarshal(identityJSON, &identity); err != nil {
		return StandaloneTargetBundleIdentity{}, true, fmt.Errorf("report: decode standalone target bundle identity: %w", err)
	}
	canonicalIdentity, err := json.Marshal(identity)
	if err != nil || !bytes.Equal(canonicalIdentity, identityJSON) || !validStandaloneTargetBundleIdentity(identity) {
		return StandaloneTargetBundleIdentity{}, true, fmt.Errorf("report: standalone target bundle identity is invalid")
	}

	tailBytes := min(info.Size(), maxStandaloneTargetBundleSealBytes)
	if _, err := file.Seek(-tailBytes, io.SeekEnd); err != nil {
		return StandaloneTargetBundleIdentity{}, true, err
	}
	tail := make([]byte, int(tailBytes))
	if _, err := io.ReadFull(file, tail); err != nil {
		return StandaloneTargetBundleIdentity{}, true, err
	}
	sealPrefix := []byte(standaloneTargetBundleSealPrefix)
	sealStartInTail := bytes.LastIndex(tail, sealPrefix)
	if sealStartInTail < 0 || !bytes.HasSuffix(tail, []byte(standaloneTargetBundleSealSuffix)) {
		return StandaloneTargetBundleIdentity{}, true, fmt.Errorf("report: standalone target bundle seal is missing")
	}
	sealDigestStart := sealStartInTail + len(sealPrefix)
	sealDigestEnd := len(tail) - len(standaloneTargetBundleSealSuffix)
	sealDigest := tail[sealDigestStart:sealDigestEnd]
	if len(sealDigest) != sha256.Size*2 {
		return StandaloneTargetBundleIdentity{}, true, fmt.Errorf("report: standalone target bundle seal is invalid")
	}
	if _, err := hex.DecodeString(string(sealDigest)); err != nil {
		return StandaloneTargetBundleIdentity{}, true, fmt.Errorf("report: standalone target bundle seal is invalid")
	}
	sealAbsoluteOffset := info.Size() - tailBytes + int64(sealStartInTail)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return StandaloneTargetBundleIdentity{}, true, err
	}
	digest := sha256.New()
	if _, err := io.CopyN(digest, bufio.NewReaderSize(file, 64<<10), sealAbsoluteOffset); err != nil {
		return StandaloneTargetBundleIdentity{}, true, fmt.Errorf("report: verify standalone target bundle: %w", err)
	}
	if !bytes.Equal([]byte(hex.EncodeToString(digest.Sum(nil))), sealDigest) {
		return StandaloneTargetBundleIdentity{}, true, fmt.Errorf("report: standalone target bundle seal mismatch")
	}
	return identity, true, nil
}

// VerifyStandaloneTargetBundleHTML treats the seal as corruption detection
// only. Publication authority comes from the default manifest's exact target
// container/portfolio plus every sibling's independently validated manifest,
// report and ProgramIndex set. The complete HTML is compared byte-for-byte to
// the canonical projection rederived from those authorities, one child page at
// a time, so a rewritten and re-sealed blob cannot certify itself.
func VerifyStandaloneTargetBundleHTML(
	path string,
	runDir string,
	manifest RunManifest,
) (StandaloneTargetBundleIdentity, error) {
	if err := manifest.Validate(); err != nil {
		return StandaloneTargetBundleIdentity{}, err
	}
	if manifest.StandaloneSource == nil {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: standalone target bundle lacks manifest source authority")
	}
	identity, found, err := InspectStandaloneTargetBundleHTML(path)
	if err != nil {
		return StandaloneTargetBundleIdentity{}, err
	}
	if !found {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: standalone target bundle marker is missing")
	}

	absoluteRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: resolve standalone target bundle run: %w", err)
	}
	absoluteRunDir = filepath.Clean(absoluteRunDir)
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: resolve standalone target bundle path: %w", err)
	}
	if filepath.Clean(absolutePath) != filepath.Join(absoluteRunDir, "report.html") {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: standalone target bundle path is not the manifest report")
	}
	container, portfolio, err := manifestStandaloneTargetAuthority(absoluteRunDir, manifest)
	if err != nil {
		return StandaloneTargetBundleIdentity{}, err
	}
	if len(container.Targets) <= 1 {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: standalone target bundle authority is not multi-target")
	}
	defaultIndex := -1
	for index, projection := range container.Targets {
		if projection.Target.Ref == container.DefaultTargetRef {
			defaultIndex = index
			break
		}
	}
	if defaultIndex < 0 ||
		portfolio.Targets[defaultIndex].RunID != filepath.Base(absoluteRunDir) ||
		manifest.MaterialInputs.AnalysisTargetRef != container.DefaultTargetRef {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: standalone target bundle default run authority mismatch")
	}
	if identity.TargetRunContainerSHA256 != container.SHA256 ||
		identity.TargetPagePortfolioSHA256 != portfolio.SHA256 ||
		identity.DefaultTargetIndex != defaultIndex ||
		identity.TargetCount != len(container.Targets) {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: standalone target bundle identity does not match manifest authority")
	}

	runsDir := filepath.Dir(absoluteRunDir)
	defaultItem, repoName, err := expectedStandaloneTargetBundleItem(
		runsDir,
		container.Targets[defaultIndex],
		portfolio.Targets[defaultIndex],
		defaultIndex,
		manifest,
	)
	if err != nil {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: standalone target bundle default payload: %w", err)
	}
	itemAt := func(index int) (standaloneTargetBundleItem, error) {
		if index == defaultIndex {
			return defaultItem, nil
		}
		item, itemRepoName, itemErr := expectedStandaloneTargetBundleItem(
			runsDir, container.Targets[index], portfolio.Targets[index], index, manifest,
		)
		if itemErr != nil {
			return standaloneTargetBundleItem{}, fmt.Errorf(
				"report: standalone target bundle payload %d: %w", index, itemErr,
			)
		}
		if itemRepoName != repoName {
			return standaloneTargetBundleItem{}, fmt.Errorf("report: standalone target bundle repository identity mismatch")
		}
		return item, nil
	}
	if err := verifyExactStandaloneTargetBundleProjection(
		absolutePath, identity, repoName, itemAt,
	); err != nil {
		return StandaloneTargetBundleIdentity{}, err
	}
	return identity, nil
}

// VerifyStandaloneProgramPageBundleHTML re-derives a language-neutral bundle
// from the default manifest, the sealed programpage portfolio, and every
// sibling's independently verified report. The HTML seal detects corruption;
// it is never treated as publication authority by itself.
func VerifyStandaloneProgramPageBundleHTML(
	path string,
	runDir string,
	manifest RunManifest,
) (StandaloneTargetBundleIdentity, error) {
	if err := manifest.Validate(); err != nil {
		return StandaloneTargetBundleIdentity{}, err
	}
	identity, found, err := InspectStandaloneTargetBundleHTML(path)
	if err != nil {
		return StandaloneTargetBundleIdentity{}, err
	}
	if !found {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: standalone program page bundle marker is missing")
	}

	absoluteRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: resolve standalone program page bundle run: %w", err)
	}
	absoluteRunDir = filepath.Clean(absoluteRunDir)
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: resolve standalone program page bundle path: %w", err)
	}
	if filepath.Clean(absolutePath) != filepath.Join(absoluteRunDir, "report.html") {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: standalone program page bundle path is not the manifest report")
	}
	portfolio, err := manifestStandaloneProgramPageAuthority(absoluteRunDir, manifest)
	if err != nil {
		return StandaloneTargetBundleIdentity{}, err
	}
	defaultIndex := -1
	for index, binding := range portfolio.Pages {
		if binding.Target.ID == portfolio.DefaultTargetID {
			defaultIndex = index
			break
		}
	}
	if defaultIndex < 0 || portfolio.Pages[defaultIndex].RunID != filepath.Base(absoluteRunDir) ||
		manifest.MaterialInputs.ProgramTargetID != portfolio.DefaultTargetID {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: standalone program page bundle default run authority mismatch")
	}
	if identity.ProgramPagePortfolioSHA256 != portfolio.SHA256 ||
		identity.DefaultTargetIndex != defaultIndex || identity.TargetCount != len(portfolio.Pages) {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: standalone program page bundle identity does not match manifest authority")
	}

	runsDir := filepath.Dir(absoluteRunDir)
	defaultItem, repoName, err := expectedStandaloneProgramPageBundleItem(
		runsDir, portfolio.Pages[defaultIndex], defaultIndex, manifest,
	)
	if err != nil {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: standalone program page bundle default payload: %w", err)
	}
	itemAt := func(index int) (standaloneTargetBundleItem, error) {
		if index == defaultIndex {
			return defaultItem, nil
		}
		item, itemRepoName, itemErr := expectedStandaloneProgramPageBundleItem(
			runsDir, portfolio.Pages[index], index, manifest,
		)
		if itemErr != nil {
			return standaloneTargetBundleItem{}, fmt.Errorf(
				"report: standalone program page bundle payload %d: %w", index, itemErr,
			)
		}
		if itemRepoName != repoName {
			return standaloneTargetBundleItem{}, fmt.Errorf("report: standalone program page bundle repository identity mismatch")
		}
		return item, nil
	}
	if err := verifyExactStandaloneTargetBundleProjection(absolutePath, identity, repoName, itemAt); err != nil {
		return StandaloneTargetBundleIdentity{}, err
	}
	return identity, nil
}

func verifyExactStandaloneTargetBundleProjection(
	path string,
	identity StandaloneTargetBundleIdentity,
	repoName string,
	itemAt func(int) (standaloneTargetBundleItem, error),
) error {
	if !validStandaloneTargetBundleIdentity(identity) || repoName == "" || itemAt == nil {
		return fmt.Errorf("report: standalone target bundle expected projection is invalid")
	}
	skeleton, err := standaloneTargetBundleSkeleton(&validatedStandaloneTargetBundle{
		defaultTarget: &preparedStandaloneTarget{repoName: repoName},
	})
	if err != nil {
		return err
	}
	placeholder := []byte(standaloneTargetBundlePlaceholder)
	if bytes.Count(skeleton, placeholder) != 1 {
		return fmt.Errorf("report: standalone target bundle verification seam is invalid")
	}
	cut := bytes.Index(skeleton, placeholder)

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	digest := sha256.New()
	comparator := exactStandaloneBundleComparator{
		reader: file,
		digest: digest,
		buffer: make([]byte, 64<<10),
	}
	if err := comparator.compare(skeleton[:cut], "application shell prefix"); err != nil {
		return err
	}
	identityJSON, err := json.Marshal(identity)
	if err != nil {
		return err
	}
	marker := standaloneTargetBundleMarkerPrefix +
		base64.RawURLEncoding.EncodeToString(identityJSON) + standaloneTargetBundleMarkerSuffix
	if err := comparator.compare(
		[]byte(marker+"\n  <script type=\"application/json\" id=\"rm-standalone-target-bundle\">"),
		"bundle marker",
	); err != nil {
		return err
	}
	header := fmt.Sprintf(
		`{"version":%d,"default_target_index":%d,"targets":[`,
		StandaloneTargetBundleVersion,
		identity.DefaultTargetIndex,
	)
	if err := comparator.compare([]byte(header), "bundle header"); err != nil {
		return err
	}
	for index := 0; index < identity.TargetCount; index++ {
		item, err := itemAt(index)
		if err != nil {
			return err
		}
		if index != 0 {
			if err := comparator.compare([]byte(","), "bundle target separator"); err != nil {
				return err
			}
		}
		itemJSON, marshalErr := json.Marshal(item)
		if marshalErr != nil {
			return fmt.Errorf("report: encode expected standalone target %d: %w", index, marshalErr)
		}
		if err := comparator.compare(itemJSON, fmt.Sprintf("bundle target %d", index)); err != nil {
			return err
		}
	}
	if err := comparator.compare(
		[]byte("]}\n  </script>\n  <script type=\"application/json\" id=\"rm-report-data\"></script>\n"+
			"  <script id=\"rm-standalone-target-bootstrap\">"+standaloneTargetBootstrapJS+"</script>"),
		"bundle payload suffix",
	); err != nil {
		return err
	}
	if err := comparator.compare(skeleton[cut+len(placeholder):], "application shell suffix"); err != nil {
		return err
	}
	expectedSeal := standaloneTargetBundleSeal(digest)
	if err := compareStandaloneBundleBytes(file, nil, []byte(expectedSeal), comparator.buffer, "bundle authority seal"); err != nil {
		return err
	}
	var trailing [1]byte
	if count, readErr := file.Read(trailing[:]); readErr != io.EOF || count != 0 {
		if readErr == nil {
			return fmt.Errorf("report: standalone target bundle has trailing bytes")
		}
		return fmt.Errorf("report: read standalone target bundle end: %w", readErr)
	}
	return nil
}

func manifestStandaloneTargetAuthority(
	runDir string,
	manifest RunManifest,
) (snapshot.TargetRunContainer, snapshot.TargetPagePortfolio, error) {
	if err := manifest.VerifyTargetPagePortfolioArtifacts(runDir); err != nil {
		return snapshot.TargetRunContainer{}, snapshot.TargetPagePortfolio{}, err
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return snapshot.TargetRunContainer{}, snapshot.TargetPagePortfolio{}, err
	}
	defer root.Close()
	containerRaw, err := readManifestFile(
		root, snapshot.TargetRunContainerArtifactFilename, snapshot.MaxTargetRunContainerBytes,
	)
	if err != nil || manifestSHA256(containerRaw) != manifest.MaterialInputs.TargetRunContainerSHA256 {
		return snapshot.TargetRunContainer{}, snapshot.TargetPagePortfolio{}, fmt.Errorf("report: standalone target bundle container authority mismatch")
	}
	portfolioRaw, err := readManifestFile(
		root, snapshot.TargetPagePortfolioArtifactFilename, snapshot.MaxTargetPagePortfolioBytes,
	)
	if err != nil || manifestSHA256(portfolioRaw) != manifest.MaterialInputs.TargetPagePortfolioSHA256 {
		return snapshot.TargetRunContainer{}, snapshot.TargetPagePortfolio{}, fmt.Errorf("report: standalone target bundle portfolio authority mismatch")
	}
	container, err := snapshot.DecodeTargetRunContainer(containerRaw)
	if err != nil {
		return snapshot.TargetRunContainer{}, snapshot.TargetPagePortfolio{}, err
	}
	portfolio, err := snapshot.DecodeTargetPagePortfolio(portfolioRaw)
	if err != nil {
		return snapshot.TargetRunContainer{}, snapshot.TargetPagePortfolio{}, err
	}
	if err := portfolio.ValidateAgainstContainer(container); err != nil {
		return snapshot.TargetRunContainer{}, snapshot.TargetPagePortfolio{}, err
	}
	return container, portfolio, nil
}

func manifestStandaloneProgramPageAuthority(
	runDir string,
	manifest RunManifest,
) (programpage.Portfolio, error) {
	if err := manifest.VerifyProgramPagePortfolioArtifact(runDir); err != nil {
		return programpage.Portfolio{}, err
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return programpage.Portfolio{}, err
	}
	defer root.Close()
	raw, err := readManifestFile(root, programpage.ArtifactFilename, programpage.MaxArtifactBytes)
	if err != nil || manifestSHA256(raw) != manifest.MaterialInputs.ProgramPagePortfolioSHA256 {
		return programpage.Portfolio{}, fmt.Errorf("report: standalone program page bundle portfolio authority mismatch")
	}
	portfolio, err := programpage.Decode(raw)
	if err != nil {
		return programpage.Portfolio{}, err
	}
	return portfolio, nil
}

func expectedStandaloneProgramPageBundleItem(
	runsDir string,
	binding programpage.Page,
	index int,
	defaultManifest RunManifest,
) (standaloneTargetBundleItem, string, error) {
	runDir := filepath.Join(runsDir, binding.RunID)
	info, err := os.Lstat(runDir)
	if err != nil || !info.IsDir() {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("child run directory is unavailable")
	}
	manifest, err := ReadRunManifest(runDir)
	if err != nil {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("child manifest: %w", err)
	}
	if binding.Target.ID != defaultManifest.MaterialInputs.ProgramTargetID {
		htmlPath := filepath.Join(runDir, "report.html")
		if _, htmlErr := os.Lstat(htmlPath); htmlErr == nil {
			return standaloneTargetBundleItem{}, "", fmt.Errorf("backing child unexpectedly publishes report.html")
		} else if !errors.Is(htmlErr, os.ErrNotExist) {
			return standaloneTargetBundleItem{}, "", fmt.Errorf("inspect backing child report.html: %w", htmlErr)
		}
	}
	if manifest.MaterialInputs.ProgramTargetID != binding.Target.ID ||
		manifest.RepositoryStateSHA256 != defaultManifest.RepositoryStateSHA256 ||
		manifest.MaterialInputs.SelectedRevision != defaultManifest.MaterialInputs.SelectedRevision ||
		manifest.MaterialInputs.ProgramPagePortfolioSHA256 != defaultManifest.MaterialInputs.ProgramPagePortfolioSHA256 ||
		manifest.MaterialInputs.RuntimePortfolioSHA256 != defaultManifest.MaterialInputs.RuntimePortfolioSHA256 ||
		manifest.MaterialInputs.TargetOutcomePortfolioSHA256 != defaultManifest.MaterialInputs.TargetOutcomePortfolioSHA256 ||
		!reflect.DeepEqual(manifest.StandaloneSource, defaultManifest.StandaloneSource) {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("child manifest authority mismatch")
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	defer root.Close()
	reportJSON, err := readManifestFile(root, "report.json", maxManifestReportBytes)
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	data, err := decodeStrictReportJSON(reportJSON)
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	if data.ProgramPortfolio == nil {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("child report ProgramPortfolio authority is incomplete")
	}
	defaultEntry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	if !reflect.DeepEqual(defaultEntry.Target, binding.Target) {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("child report ProgramTarget does not match portfolio")
	}
	setRaw, err := readManifestFile(root, programindex.ArtifactSetFilename, programindex.MaxArtifactSetBytes)
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	if manifestSHA256(setRaw) != manifest.MaterialInputs.ProgramIndexSetSHA256 {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("child ProgramIndex set authority mismatch")
	}
	set, err := programindex.DecodeArtifactSet(setRaw)
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	if set.DefaultTargetID != binding.Target.ID {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("child ProgramIndex default target mismatch")
	}
	artifactFilename := ""
	for _, entry := range set.Entries {
		if entry.TargetID == set.DefaultTargetID {
			artifactFilename = entry.Filename
			break
		}
	}
	if artifactFilename == "" {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("child ProgramIndex artifact is missing")
	}
	sourceAuthority := OrdinaryReportHTMLAuthority{
		StandaloneSource: manifest.StandaloneSource,
		AnalysisRoot:     manifest.AnalysisRoot,
		RepositoryRoot:   manifest.RepositoryState.Identity,
	}
	githubLinks, gitlabLinks, err := ordinaryHTMLSourceLinks(data.CapturedRevision, sourceAuthority)
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	data.GitHubSourceLinks = githubLinks
	data.GitLabSourceLinks = gitlabLinks
	absoluteRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	payload, err := marshalHTMLPayloadWithLocalRoots(
		programShellPayloadForReport(&data, nil),
		[]string{absoluteRunDir, manifest.AnalysisRoot, manifest.RepositoryState.Identity},
	)
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	return standaloneTargetBundleItem{
		TargetID:    binding.Target.ID,
		Language:    binding.Target.Language,
		Kind:        binding.Target.Kind,
		DisplayName: binding.Target.Name,
		Href:        standaloneTargetHref(index),
		Payload:     json.RawMessage(payload),
	}, data.RepoName, nil
}

func expectedStandaloneTargetBundleItem(
	runsDir string,
	projection snapshot.TargetRunProjection,
	page snapshot.TargetPage,
	index int,
	defaultManifest RunManifest,
) (standaloneTargetBundleItem, string, error) {
	if page.TargetRef != projection.Target.Ref {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("target order does not match container")
	}
	runDir := filepath.Join(runsDir, page.RunID)
	info, err := os.Lstat(runDir)
	if err != nil || !info.IsDir() {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("child run directory is unavailable")
	}
	manifest, err := ReadRunManifest(runDir)
	if err != nil {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("child manifest: %w", err)
	}
	if manifest.MaterialInputs.AnalysisTargetRef != projection.Target.Ref ||
		manifest.RepositoryStateSHA256 != defaultManifest.RepositoryStateSHA256 ||
		manifest.MaterialInputs.SelectedRevision != defaultManifest.MaterialInputs.SelectedRevision ||
		manifest.MaterialInputs.TargetRunContainerSHA256 != defaultManifest.MaterialInputs.TargetRunContainerSHA256 ||
		manifest.MaterialInputs.TargetPagePortfolioSHA256 != defaultManifest.MaterialInputs.TargetPagePortfolioSHA256 ||
		manifest.MaterialInputs.RuntimePortfolioSHA256 != defaultManifest.MaterialInputs.RuntimePortfolioSHA256 ||
		!reflect.DeepEqual(manifest.StandaloneSource, defaultManifest.StandaloneSource) {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("child manifest authority mismatch")
	}
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	defer root.Close()
	reportJSON, err := readManifestFile(root, "report.json", maxManifestReportBytes)
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	data, err := decodeStrictReportJSON(reportJSON)
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	if data.ProgramPortfolio == nil || data.AnalysisTarget == nil {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("child report authority is incomplete")
	}
	if !reflect.DeepEqual(*data.AnalysisTarget, projection.Target) {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("child report target does not match container")
	}
	defaultEntry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	if err := validateCubeMapProgramTarget(projection.Target, defaultEntry.Target); err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	setRaw, err := readManifestFile(root, programindex.ArtifactSetFilename, programindex.MaxArtifactSetBytes)
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	if manifestSHA256(setRaw) != manifest.MaterialInputs.ProgramIndexSetSHA256 {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("child ProgramIndex set authority mismatch")
	}
	set, err := programindex.DecodeArtifactSet(setRaw)
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	if set.DefaultTargetID != defaultEntry.Target.ID {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("child ProgramIndex default target mismatch")
	}
	artifactFilename := ""
	for _, entry := range set.Entries {
		if entry.TargetID == set.DefaultTargetID {
			artifactFilename = entry.Filename
			break
		}
	}
	if artifactFilename == "" {
		return standaloneTargetBundleItem{}, "", fmt.Errorf("child ProgramIndex artifact is missing")
	}
	sourceAuthority := OrdinaryReportHTMLAuthority{
		StandaloneSource: manifest.StandaloneSource,
		AnalysisRoot:     manifest.AnalysisRoot,
		RepositoryRoot:   manifest.RepositoryState.Identity,
	}
	githubLinks, gitlabLinks, err := ordinaryHTMLSourceLinks(data.CapturedRevision, sourceAuthority)
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	data.GitHubSourceLinks = githubLinks
	data.GitLabSourceLinks = gitlabLinks
	absoluteRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	payload, err := marshalHTMLPayloadWithLocalRoots(
		manifestStandaloneProgramShellPayload(&data, projection.Target),
		[]string{absoluteRunDir, manifest.AnalysisRoot, manifest.RepositoryState.Identity},
	)
	if err != nil {
		return standaloneTargetBundleItem{}, "", err
	}
	return standaloneTargetBundleItem{
		TargetID:    defaultEntry.Target.ID,
		Language:    defaultEntry.Target.Language,
		Kind:        defaultEntry.Target.Kind,
		DisplayName: defaultEntry.Target.Name,
		Href:        standaloneTargetHref(index),
		Payload:     json.RawMessage(payload),
	}, data.RepoName, nil
}

// manifestStandaloneProgramShellPayload mirrors the multi-target writer's
// restoration of outer target authority. Ordinary CoreMap browser payloads
// omit this Go-only field, but a standalone bundle carries it so each embedded
// page remains bound to its exact TargetRunContainer projection.
func manifestStandaloneProgramShellPayload(
	data *ReportData,
	target analysistarget.Target,
) programShellPayload {
	payload := programShellPayloadForReport(data, nil)
	owned := target.Snapshot()
	payload.AnalysisTarget = &owned
	return payload
}

type exactStandaloneBundleComparator struct {
	reader io.Reader
	digest hash.Hash
	buffer []byte
}

func (comparator *exactStandaloneBundleComparator) compare(expected []byte, label string) error {
	return compareStandaloneBundleBytes(
		comparator.reader, comparator.digest, expected, comparator.buffer, label,
	)
}

func compareStandaloneBundleBytes(
	reader io.Reader,
	digest hash.Hash,
	expected []byte,
	buffer []byte,
	label string,
) error {
	for len(expected) > 0 {
		count := min(len(expected), len(buffer))
		actual := buffer[:count]
		if _, err := io.ReadFull(reader, actual); err != nil {
			return fmt.Errorf("report: standalone target bundle %s is incomplete: %w", label, err)
		}
		if !bytes.Equal(actual, expected[:count]) {
			return fmt.Errorf("report: standalone target bundle %s does not match manifest-derived projection", label)
		}
		if digest != nil {
			_, _ = digest.Write(actual)
		}
		expected = expected[count:]
	}
	return nil
}

func validStandaloneTargetBundleIdentity(identity StandaloneTargetBundleIdentity) bool {
	legacyAuthority := validStandaloneTargetBundleDigest(identity.TargetRunContainerSHA256) &&
		validStandaloneTargetBundleDigest(identity.TargetPagePortfolioSHA256) &&
		identity.ProgramPagePortfolioSHA256 == ""
	programPageAuthority := identity.TargetRunContainerSHA256 == "" &&
		identity.TargetPagePortfolioSHA256 == "" &&
		validStandaloneTargetBundleDigest(identity.ProgramPagePortfolioSHA256)
	validTargetCount := legacyAuthority && identity.TargetCount > 1 ||
		programPageAuthority && identity.TargetCount > 0
	return identity.Version == StandaloneTargetBundleVersion &&
		(legacyAuthority || programPageAuthority) && validTargetCount &&
		identity.DefaultTargetIndex >= 0 && identity.DefaultTargetIndex < identity.TargetCount
}

func validStandaloneTargetBundleDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
