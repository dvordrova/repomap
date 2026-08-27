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

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/snapshot"
)

const (
	StandaloneTargetBundleVersion     = 4
	StandaloneTargetNavigationVersion = 5

	// One GiB is a terminal aggregate payload bound, not a prefix-clipping
	// budget. Every ready target is either present in full or publication fails.
	MaxStandaloneTargetBundlePayloadBytes int64 = 1 << 30

	maxPreparedStandaloneTargetHTMLBytes = int64(256 << 20)
	maxStandaloneTargetBundleHeaderBytes = int64(8 << 20)
	maxStandaloneTargetBundleSealBytes   = int64(512)
	// Base64 can add one third to incompressible gzip target bytes. Keep the
	// inspector's outer file bound above the one-GiB raw aggregate contract
	// without changing that raw authority limit.
	maxStandaloneTargetBundleOverhead = MaxStandaloneTargetBundlePayloadBytes/2 + int64(64<<20)

	standaloneTargetBundlePlaceholder  = "REPOMAP_STANDALONE_TARGET_BUNDLE_PLACEHOLDER_4D909014"
	standaloneTargetBundleMarkerPrefix = "<!-- repomap-standalone-target-bundle:v4:"
	standaloneTargetBundleMarkerSuffix = " -->"
	standaloneTargetBundleSealPrefix   = "<!-- repomap-standalone-target-bundle-seal:v4:"
	standaloneTargetBundleSealSuffix   = " -->\n"

	standaloneBundleScriptClose = `</script>`
)

// PreparedStandaloneTarget is an opaque, fully scrubbed hosted target
// payload. Values can only be produced by the authorized GitHub/GitLab
// generation seams below; callers cannot alter target/source authority.
type PreparedStandaloneTarget struct {
	prepared *preparedStandaloneTarget
}

type preparedStandaloneTarget struct {
	analysisTarget    *analysistarget.Target
	programPage       TargetNavigationPage
	repository        BrowserRepositoryPayload
	repositoryPayload []byte
	target            BrowserTargetPayload
	targetPayload     []byte
	selectedTargetID  string
	reportData        *ReportData
	host              string
	repositoryURL     string
	revision          string
	repoName          string
	localRoots        []string
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
	if err := verifyStandalonePreparationAuthority(runDir); err != nil {
		return PreparedStandaloneTarget{}, err
	}
	page, err := LoadTargetNavigationPage(runDir, filepath.Base(filepath.Clean(runDir)))
	if err != nil {
		return PreparedStandaloneTarget{}, err
	}
	analysisTarget, err := preparedStandaloneAnalysisTarget(runDir)
	if err != nil {
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
	prepared, err := prepareStandaloneTargetFromHTMLV4(
		htmlBytes, localRoots, page.ProgramTarget, analysisTarget,
	)
	if err != nil {
		return PreparedStandaloneTarget{}, err
	}
	if prepared.host != wantHost || prepared.revision != strings.ToLower(authority.repository.Head) {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: prepared standalone target source authority mismatch")
	}
	prepared.programPage = page
	return PreparedStandaloneTarget{prepared: prepared}, nil
}

// PrepareStandaloneProgramPage restores one opaque standalone payload from an
// already-generated, manifest-verified program page. Unlike the legacy Go
// target seam, its cross-page authority is only the exact ProgramTarget+run
// binding in programpage.Portfolio.
func PrepareStandaloneProgramPage(runDir string) (PreparedStandaloneTarget, error) {
	if err := verifyStandalonePreparationAuthority(runDir); err != nil {
		return PreparedStandaloneTarget{}, err
	}
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
	page, err := LoadTargetNavigationPage(runDir, runID)
	if err != nil {
		return PreparedStandaloneTarget{}, err
	}
	if !reflect.DeepEqual(page.ProgramTarget, binding.Target) {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: standalone program page target authority mismatch")
	}
	prepared, err := prepareStandaloneTargetFromHTMLV4(htmlBytes, []string{
		absoluteRunDir, manifest.AnalysisRoot, manifest.RepositoryState.Identity,
	}, binding.Target, nil)
	if err != nil {
		return PreparedStandaloneTarget{}, err
	}
	if prepared.host != manifest.StandaloneSource.Host ||
		prepared.repositoryURL != manifest.StandaloneSource.RepositoryURL ||
		prepared.revision != strings.ToLower(manifest.MaterialInputs.SelectedRevision) {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: standalone program page source authority mismatch")
	}
	prepared.programPage = page
	return PreparedStandaloneTarget{prepared: prepared}, nil
}

func verifyStandalonePreparationAuthority(runDir string) error {
	assessment, err := AssessRunPublication(runDir)
	if err != nil {
		return fmt.Errorf("report: verify prepared standalone publication: %w", err)
	}
	if assessment.Status != PublicationReady {
		return fmt.Errorf("report: prepared standalone publication is not ready")
	}
	return nil
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

func prepareStandaloneTargetFromHTMLV4(
	htmlBytes []byte,
	localRoots []string,
	programTarget programindex.Target,
	authoritativeTarget *analysistarget.Target,
) (*preparedStandaloneTarget, error) {
	if err := programTarget.Validate(); err != nil {
		return nil, fmt.Errorf("report: prepared standalone target ProgramTarget: %w", err)
	}
	if authoritativeTarget != nil {
		if err := authoritativeTarget.Validate(); err != nil {
			return nil, fmt.Errorf("report: prepared standalone target analysis target: %w", err)
		}
	}
	transport, err := extractStandaloneBundleTransportV4HTML(htmlBytes)
	if err != nil {
		return nil, fmt.Errorf("report: extract prepared standalone target v4: %w", err)
	}
	repository, err := DecodeBrowserRepositoryPayload(transport.RepositoryPayload)
	if err != nil {
		return nil, fmt.Errorf("report: prepared standalone repository payload: %w", err)
	}
	canonicalRepository, err := EncodeBrowserRepositoryPayload(repository)
	if err != nil || !bytes.Equal(canonicalRepository, transport.RepositoryPayload) {
		return nil, fmt.Errorf("report: prepared standalone repository payload is not canonical")
	}
	var indexRow *standaloneBundleTargetIndexV4
	selectedTargetID := ""
	for position := range transport.Index.Targets {
		row := &transport.Index.Targets[position]
		if row.ProgramTargetID != programTarget.ID {
			continue
		}
		if indexRow != nil {
			return nil, fmt.Errorf("report: prepared standalone ProgramTarget binding is ambiguous")
		}
		indexRow = row
		selectedTargetID = row.TargetID
	}
	if indexRow == nil || indexRow.Chunk == nil {
		return nil, fmt.Errorf("report: prepared standalone ProgramTarget chunk is missing")
	}
	var encoded *standaloneBundleEncodedTargetChunkV4
	for position := range transport.TargetChunks {
		chunk := &transport.TargetChunks[position]
		if chunk.TargetID == selectedTargetID {
			encoded = chunk
			break
		}
	}
	if encoded == nil {
		return nil, fmt.Errorf("report: prepared standalone target chunk is missing")
	}
	targetRaw, err := decodeStandaloneBundleTargetChunkV4(encoded.Ref, encoded.Base64)
	if err != nil {
		return nil, fmt.Errorf("report: prepared standalone target chunk: %w", err)
	}
	target, err := DecodeBrowserTargetPayload(targetRaw)
	if err != nil {
		return nil, fmt.Errorf("report: prepared standalone target payload: %w", err)
	}
	canonicalTarget, err := EncodeBrowserTargetPayload(target)
	if err != nil || !bytes.Equal(canonicalTarget, targetRaw) {
		return nil, fmt.Errorf("report: prepared standalone target payload is not canonical")
	}
	if target.Target.ID != programTarget.ID || target.Target.Language != programTarget.Language ||
		target.Target.Kind != programTarget.Kind || target.Target.Name != programTarget.Name ||
		target.Target.Selector != programTarget.Selector {
		return nil, fmt.Errorf("report: prepared standalone target payload does not match ProgramTarget")
	}
	host := ""
	switch repository.Source.Kind {
	case "github":
		host = "GitHub"
	case "gitlab":
		host = "GitLab"
	default:
		return nil, fmt.Errorf("report: prepared standalone target requires hosted source authority")
	}
	var analysisTarget *analysistarget.Target
	if authoritativeTarget != nil {
		owned := authoritativeTarget.Snapshot()
		analysisTarget = &owned
	}
	return &preparedStandaloneTarget{
		analysisTarget:    analysisTarget,
		programPage:       TargetNavigationPage{ProgramTarget: programTarget.Snapshot()},
		repository:        repository,
		repositoryPayload: append([]byte(nil), transport.RepositoryPayload...),
		target:            target,
		targetPayload:     append([]byte(nil), targetRaw...),
		selectedTargetID:  selectedTargetID,
		host:              host,
		repositoryURL:     repository.Source.RepositoryURL,
		revision:          repository.Repository.CapturedRevision,
		repoName:          repository.Repository.Name,
		localRoots:        normalizedStandaloneLocalRoots(localRoots),
	}, nil
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

const standaloneBundleIndexElementID = "rm-bundle-index"

// standaloneBundleTransportHTMLSectionV4 is the shared ordinary/standalone
// transport seam. It writes only the feature-blind index, one repository
// payload, and the indexed opaque target chunks. Marker, seal, application
// bootstrap, and canonical report data are deliberately outside this helper.
func standaloneBundleTransportHTMLSectionV4(
	transport standaloneBundleTransportV4,
) ([]byte, error) {
	var section bytes.Buffer
	if err := writeStandaloneBundleTransportHTMLSectionV4(&section, transport); err != nil {
		return nil, err
	}
	return section.Bytes(), nil
}

func writeStandaloneBundleTransportHTMLSectionV4(
	w io.Writer,
	transport standaloneBundleTransportV4,
) error {
	if err := validateStandaloneBundleTransportForHTMLV4(transport); err != nil {
		return err
	}
	if _, err := io.WriteString(w, `  <script type="application/json" id="rm-bundle-index">`); err != nil {
		return err
	}
	if _, err := w.Write(transport.IndexJSON); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "</script>\n"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, `  <script type="application/json" id="rm-repository-payload">`); err != nil {
		return err
	}
	if _, err := w.Write(transport.RepositoryPayload); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "</script>\n"); err != nil {
		return err
	}
	for _, chunk := range transport.TargetChunks {
		if _, err := io.WriteString(w, `  <script type="application/octet-stream" id="`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, chunk.Ref.ElementID); err != nil {
			return err
		}
		if _, err := io.WriteString(w, `">`); err != nil {
			return err
		}
		if _, err := io.WriteString(w, chunk.Base64); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "</script>\n"); err != nil {
			return err
		}
	}
	return nil
}

func validateStandaloneBundleTransportForHTMLV4(transport standaloneBundleTransportV4) error {
	index, err := decodeStandaloneBundleIndexV4(transport.IndexJSON)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(index, transport.Index) {
		return fmt.Errorf("report: standalone bundle v4 index bytes do not match transport index")
	}
	if err := verifyStandaloneBundleIdentityPayloadV4(index.Repository, transport.RepositoryPayload); err != nil {
		return fmt.Errorf("report: standalone bundle v4 repository payload: %w", err)
	}
	if bytes.Contains(bytes.ToLower(transport.RepositoryPayload), []byte("</script")) {
		return fmt.Errorf("report: standalone bundle v4 repository payload cannot be embedded safely")
	}
	chunkIndex := 0
	for position, row := range index.Targets {
		if row.State != standaloneBundleTransportTargetAnalyzed {
			continue
		}
		if chunkIndex >= len(transport.TargetChunks) {
			return fmt.Errorf("report: standalone bundle v4 target %d chunk is missing", position)
		}
		chunk := transport.TargetChunks[chunkIndex]
		if row.Chunk == nil || chunk.TargetID != row.TargetID || chunk.Ref != *row.Chunk {
			return fmt.Errorf("report: standalone bundle v4 target %d chunk binding mismatch", position)
		}
		if _, err := decodeStandaloneBundleTargetChunkV4(chunk.Ref, chunk.Base64); err != nil {
			return fmt.Errorf("report: standalone bundle v4 target %d chunk: %w", position, err)
		}
		chunkIndex++
	}
	if chunkIndex != len(transport.TargetChunks) {
		return fmt.Errorf("report: standalone bundle v4 contains an unindexed target chunk")
	}
	return nil
}

// extractStandaloneBundleTransportV4HTML restores the v4 transport without
// knowing any browser feature schema. Every indexed element must occur exactly
// once; repository and target integrity are checked before bytes are returned.
func extractStandaloneBundleTransportV4HTML(htmlBytes []byte) (standaloneBundleTransportV4, error) {
	indexRaw, err := extractExactStandaloneBundleScriptV4(
		htmlBytes, standaloneBundleIndexElementID, "application/json",
	)
	if err != nil {
		return standaloneBundleTransportV4{}, err
	}
	index, err := decodeStandaloneBundleIndexV4(indexRaw)
	if err != nil {
		return standaloneBundleTransportV4{}, err
	}
	repositoryRaw, err := extractExactStandaloneBundleScriptV4(
		htmlBytes, index.Repository.ElementID, "application/json",
	)
	if err != nil {
		return standaloneBundleTransportV4{}, err
	}
	if err := verifyStandaloneBundleIdentityPayloadV4(index.Repository, repositoryRaw); err != nil {
		return standaloneBundleTransportV4{}, fmt.Errorf("report: standalone bundle v4 repository payload: %w", err)
	}
	chunks := make([]standaloneBundleEncodedTargetChunkV4, 0, len(index.Targets))
	for position, row := range index.Targets {
		if row.State != standaloneBundleTransportTargetAnalyzed {
			continue
		}
		if row.Chunk == nil {
			return standaloneBundleTransportV4{}, fmt.Errorf("report: standalone bundle v4 target %d chunk is missing", position)
		}
		encodedRaw, extractErr := extractExactStandaloneBundleScriptV4(
			htmlBytes, row.Chunk.ElementID, "application/octet-stream",
		)
		if extractErr != nil {
			return standaloneBundleTransportV4{}, extractErr
		}
		encoded := string(encodedRaw)
		if _, decodeErr := decodeStandaloneBundleTargetChunkV4(*row.Chunk, encoded); decodeErr != nil {
			return standaloneBundleTransportV4{}, fmt.Errorf(
				"report: standalone bundle v4 target %d chunk: %w", position, decodeErr,
			)
		}
		chunks = append(chunks, standaloneBundleEncodedTargetChunkV4{
			TargetID: row.TargetID, Ref: *row.Chunk, Base64: encoded,
		})
	}
	if bytes.Count(htmlBytes, []byte(`id="`+standaloneBundleTargetElementPrefix)) != len(chunks) {
		return standaloneBundleTransportV4{}, fmt.Errorf("report: standalone bundle v4 contains an unindexed or duplicate target chunk")
	}
	transport := standaloneBundleTransportV4{
		Index:             index,
		IndexJSON:         append([]byte(nil), indexRaw...),
		RepositoryPayload: append([]byte(nil), repositoryRaw...),
		TargetChunks:      chunks,
	}
	if err := validateStandaloneBundleTransportForHTMLV4(transport); err != nil {
		return standaloneBundleTransportV4{}, err
	}
	return transport, nil
}

func extractExactStandaloneBundleScriptV4(
	htmlBytes []byte,
	elementID string,
	mimeType string,
) ([]byte, error) {
	idToken := []byte(`id="` + elementID + `"`)
	if bytes.Count(htmlBytes, idToken) != 1 {
		return nil, fmt.Errorf("report: standalone bundle v4 element %q must occur exactly once", elementID)
	}
	open := []byte(`<script type="` + mimeType + `" id="` + elementID + `">`)
	start := bytes.Index(htmlBytes, open)
	if start < 0 {
		return nil, fmt.Errorf("report: standalone bundle v4 element %q has invalid script metadata", elementID)
	}
	start += len(open)
	endOffset := bytes.Index(htmlBytes[start:], []byte(standaloneBundleScriptClose))
	if endOffset < 0 {
		return nil, fmt.Errorf("report: standalone bundle v4 element %q is unterminated", elementID)
	}
	return htmlBytes[start : start+endOffset], nil
}

type validatedStandaloneTargetBundle struct {
	identity      StandaloneTargetBundleIdentity
	defaultTarget *preparedStandaloneTarget
	transport     standaloneBundleTransportV4
	spool         *standaloneBundleSpoolV4
}

type standaloneBundleSpoolChunkV4 struct {
	TargetID string
	Ref      standaloneBundlePayloadRefV4
	Offset   int64
}

type standaloneBundleSpoolV4 struct {
	Index             standaloneBundleIndexV4
	IndexJSON         []byte
	RepositoryPayload []byte
	TargetChunks      []standaloneBundleSpoolChunkV4
	file              *os.File
	path              string
}

type standaloneBundleSpoolTargetLoaderV4 func(
	position int,
	row BrowserTargetIndexItem,
) ([]byte, error)

func prepareStandaloneBundleSpoolV4(
	repository BrowserRepositoryPayload,
	loadTarget standaloneBundleSpoolTargetLoaderV4,
) (result *standaloneBundleSpoolV4, resultErr error) {
	if loadTarget == nil {
		return nil, fmt.Errorf("report: standalone bundle v4 spool target loader is missing")
	}
	boundRepository, repositoryRaw, err := bindStandaloneBrowserRepositoryPayloadV4(repository)
	if err != nil {
		return nil, err
	}
	aggregate, err := standaloneTargetAggregateBytes(0, int64(len(repositoryRaw)))
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp("", "repomap-report-bundle-v4-*")
	if err != nil {
		return nil, fmt.Errorf("report: create standalone bundle v4 spool: %w", err)
	}
	spool := &standaloneBundleSpoolV4{file: file, path: file.Name()}
	installed := false
	defer func() {
		if !installed {
			if cleanupErr := spool.closeAndRemove(); cleanupErr != nil {
				resultErr = errors.Join(resultErr, cleanupErr)
			}
		}
	}()
	spool.Index = standaloneBundleIndexV4{
		Version: standaloneBundleTransportVersion,
		Repository: standaloneBundleIdentityPayloadRefV4(
			standaloneBundleRepositoryElementID, standaloneBundleRepositoryEncoding, repositoryRaw,
		),
		LogicalDefaultTargetID: boundRepository.LogicalDefaultSelectedTargetID,
		Targets:                make([]standaloneBundleTargetIndexV4, 0, len(boundRepository.Targets)),
	}
	spool.RepositoryPayload = repositoryRaw
	offset := int64(0)
	for position, target := range boundRepository.Targets {
		row := standaloneBundleTargetIndexV4{
			TargetID: target.SelectedTargetID,
			State:    standaloneBundleTransportTargetState(target.State),
		}
		switch target.State {
		case "analyzed":
			raw, loadErr := loadTarget(position, target)
			if loadErr != nil {
				return nil, fmt.Errorf("report: standalone bundle v4 target %d: %w", position, loadErr)
			}
			aggregate, err = standaloneTargetAggregateBytes(aggregate, int64(len(raw)))
			if err != nil {
				return nil, err
			}
			elementID := fmt.Sprintf("%s%d", standaloneBundleTargetElementPrefix, position)
			ref, encodeErr := encodeStandaloneBundleTargetChunkToV4(
				target.SelectedTargetID, elementID, raw, file,
			)
			if encodeErr != nil {
				return nil, fmt.Errorf("report: standalone bundle v4 target %d: %w", position, encodeErr)
			}
			if roundTripErr := verifyStandaloneBundleTargetChunkStreamV4(
				ref, io.NewSectionReader(file, offset, ref.CompressedByteLength), raw,
			); roundTripErr != nil {
				return nil, fmt.Errorf(
					"report: standalone bundle v4 target %d generation round-trip: %w",
					position, roundTripErr,
				)
			}
			row.ProgramTargetID = target.ProgramTargetID
			row.Chunk = &ref
			spool.TargetChunks = append(spool.TargetChunks, standaloneBundleSpoolChunkV4{
				TargetID: target.SelectedTargetID, Ref: ref, Offset: offset,
			})
			offset += ref.CompressedByteLength
		case "not_analyzed":
			// Failed outcomes deliberately have no target loader call and no chunk.
		default:
			return nil, fmt.Errorf("report: standalone bundle v4 target %d state is unsupported", position)
		}
		spool.Index.Targets = append(spool.Index.Targets, row)
	}
	_ = aggregate
	if err := validateStandaloneBundleIndexV4(spool.Index); err != nil {
		return nil, err
	}
	spool.IndexJSON, err = json.Marshal(spool.Index)
	if err != nil {
		return nil, fmt.Errorf("report: encode standalone bundle v4 spool index: %w", err)
	}
	if _, err := decodeStandaloneBundleIndexV4(spool.IndexJSON); err != nil {
		return nil, fmt.Errorf("report: verify standalone bundle v4 spool index: %w", err)
	}
	if err := file.Sync(); err != nil {
		return nil, fmt.Errorf("report: sync standalone bundle v4 spool: %w", err)
	}
	if err := validateStandaloneBundleSpoolV4(spool); err != nil {
		return nil, err
	}
	installed = true
	return spool, nil
}

func validateStandaloneBundleSpoolV4(spool *standaloneBundleSpoolV4) error {
	if spool == nil || spool.file == nil || spool.path == "" {
		return fmt.Errorf("report: standalone bundle v4 spool is incomplete")
	}
	index, err := decodeStandaloneBundleIndexV4(spool.IndexJSON)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(index, spool.Index) {
		return fmt.Errorf("report: standalone bundle v4 spool index bytes do not match index")
	}
	if err := verifyStandaloneBundleIdentityPayloadV4(index.Repository, spool.RepositoryPayload); err != nil {
		return fmt.Errorf("report: standalone bundle v4 spool repository payload: %w", err)
	}
	chunkIndex := 0
	offset := int64(0)
	for position, row := range index.Targets {
		if row.State != standaloneBundleTransportTargetAnalyzed {
			continue
		}
		if chunkIndex >= len(spool.TargetChunks) || row.Chunk == nil {
			return fmt.Errorf("report: standalone bundle v4 spool target %d chunk is missing", position)
		}
		chunk := spool.TargetChunks[chunkIndex]
		if chunk.TargetID != row.TargetID || chunk.Ref != *row.Chunk || chunk.Offset != offset {
			return fmt.Errorf("report: standalone bundle v4 spool target %d chunk binding mismatch", position)
		}
		if err := verifyStandaloneBundleTargetChunkDigestStreamV4(
			chunk.Ref,
			io.NewSectionReader(spool.file, chunk.Offset, chunk.Ref.CompressedByteLength),
		); err != nil {
			return fmt.Errorf("report: standalone bundle v4 spool target %d content: %w", position, err)
		}
		offset += chunk.Ref.CompressedByteLength
		chunkIndex++
	}
	if chunkIndex != len(spool.TargetChunks) {
		return fmt.Errorf("report: standalone bundle v4 spool contains an unindexed target chunk")
	}
	info, err := spool.file.Stat()
	if err != nil {
		return fmt.Errorf("report: inspect standalone bundle v4 spool: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != offset {
		return fmt.Errorf("report: standalone bundle v4 spool length mismatch")
	}
	return nil
}

func writeStandaloneBundleSpoolHTMLSectionV4(w io.Writer, spool *standaloneBundleSpoolV4) error {
	if w == nil {
		return fmt.Errorf("report: standalone bundle v4 spool writer is missing")
	}
	if err := validateStandaloneBundleSpoolV4(spool); err != nil {
		return err
	}
	if _, err := io.WriteString(w, `  <script type="application/json" id="rm-bundle-index">`); err != nil {
		return err
	}
	if _, err := w.Write(spool.IndexJSON); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "</script>\n  <script type=\"application/json\" id=\"rm-repository-payload\">"); err != nil {
		return err
	}
	if _, err := w.Write(spool.RepositoryPayload); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "</script>\n"); err != nil {
		return err
	}
	for _, chunk := range spool.TargetChunks {
		if _, err := io.WriteString(w, `  <script type="application/octet-stream" id="`+chunk.Ref.ElementID+`">`); err != nil {
			return err
		}
		encoder := base64.NewEncoder(base64.StdEncoding, w)
		written, copyErr := io.Copy(
			encoder,
			io.NewSectionReader(spool.file, chunk.Offset, chunk.Ref.CompressedByteLength),
		)
		closeErr := encoder.Close()
		if copyErr != nil {
			return fmt.Errorf("report: encode standalone bundle v4 spooled target: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("report: finish standalone bundle v4 spooled target: %w", closeErr)
		}
		if written != chunk.Ref.CompressedByteLength {
			return fmt.Errorf("report: standalone bundle v4 spooled target length mismatch")
		}
		if _, err := io.WriteString(w, "</script>\n"); err != nil {
			return err
		}
	}
	return nil
}

func (spool *standaloneBundleSpoolV4) closeAndRemove() error {
	if spool == nil {
		return nil
	}
	var closeErr error
	if spool.file != nil {
		closeErr = spool.file.Close()
		spool.file = nil
	}
	removeErr := error(nil)
	if spool.path != "" {
		removeErr = os.Remove(spool.path)
		if removeErr == nil || errors.Is(removeErr, os.ErrNotExist) {
			spool.path = ""
		}
	}
	failures := make([]error, 0, 2)
	if closeErr != nil {
		failures = append(failures, fmt.Errorf("report: close private standalone bundle spool"))
	}
	if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		failures = append(failures, fmt.Errorf("report: remove private standalone bundle spool"))
	}
	return errors.Join(failures...)
}

func finishStandaloneBundleSpoolV4(spool *standaloneBundleSpoolV4, primary error) error {
	cleanupErr := spool.closeAndRemove()
	if primary == nil {
		return cleanupErr
	}
	if cleanupErr == nil {
		return primary
	}
	return errors.Join(primary, cleanupErr)
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
// HTML for a neutral ProgramPagePortfolio. Callers that need the publication
// readiness result should use PublishProgramPageBundleFromArtifactsAtomic.
func WriteProgramPageBundleFromArtifactsAtomic(
	runDir string,
	portfolio programpage.Portfolio,
) error {
	_, err := PublishProgramPageBundleFromArtifactsAtomic(runDir, portfolio)
	return err
}

// PublishProgramPageBundleFromArtifactsAtomic publishes and assesses the sole
// user-facing HTML for a neutral ProgramPagePortfolio. The analyzed-page
// portfolio can contain one page when every other selected target failed
// before producing a page. Every payload is projected directly from its
// manifest-verified report.json and page-local artifacts; target-local HTML is
// neither required nor merged. READY is returned only after the closed
// temporary HTML has been compared byte-for-byte with the live, validated v4
// spool, the spool has been removed, and that exact temporary file has been
// atomically installed. Later and recovery verification must continue to use
// AssessRunPublication.
func PublishProgramPageBundleFromArtifactsAtomic(
	runDir string,
	portfolio programpage.Portfolio,
) (PublicationAssessment, error) {
	if err := writeProgramPageBundleFromArtifactsAtomic(
		runDir, portfolio, standaloneArtifactPublicationHooks{},
	); err != nil {
		return FailedPublicationAssessment(), err
	}
	return PublicationAssessment{Status: PublicationReady}, nil
}

// PublishProgramPageBundleFromVerifiedRunsAtomic publishes the sole
// user-facing HTML from transaction-local receipts produced while the exact
// backing runs were written. A receipt is authority only inside that current
// publication transaction: this path never persists it and the independent
// AssessRunPublication recovery seam remains artifact-derived.
//
// Every report is decoded exactly once. The owner report supplies both the
// repository shell and its target chunk. Non-owner receipts supply the small
// navigation descriptors needed for that shell, then their reports are loaded
// once while the canonical v4 spool is built.
func PublishProgramPageBundleFromVerifiedRunsAtomic(
	runDir string,
	portfolio programpage.Portfolio,
	receipts []VerifiedRunReceipt,
) (PublicationAssessment, error) {
	if err := writeProgramPageBundleFromVerifiedRunsAtomic(
		runDir, portfolio, receipts, standaloneVerifiedPublicationHooks{},
	); err != nil {
		return FailedPublicationAssessment(), err
	}
	return PublicationAssessment{Status: PublicationReady}, nil
}

type standaloneVerifiedPublicationHooks struct {
	afterTargetLoad func(index int)
}

type verifiedStandaloneProgramPage struct {
	receipt    VerifiedRunReceipt
	manifest   RunManifest
	descriptor *preparedStandaloneTarget
}

func writeProgramPageBundleFromVerifiedRunsAtomic(
	runDir string,
	portfolio programpage.Portfolio,
	receipts []VerifiedRunReceipt,
	hooks standaloneVerifiedPublicationHooks,
) error {
	pages, defaultIndex, err := bindVerifiedStandaloneProgramPages(
		runDir, portfolio, receipts,
	)
	if err != nil {
		return err
	}

	preparedTargets := make([]*preparedStandaloneTarget, len(pages))
	for index := range pages {
		preparedTargets[index] = pages[index].descriptor
	}
	defaultPrepared, err := expectedStandaloneTargetFromVerifiedRunV4(pages[defaultIndex])
	if err != nil {
		return fmt.Errorf("report: verified program page bundle owner: %w", err)
	}
	if hooks.afterTargetLoad != nil {
		hooks.afterTargetLoad(defaultIndex)
	}
	if err := matchVerifiedStandaloneDescriptor(defaultPrepared, pages[defaultIndex].descriptor); err != nil {
		return fmt.Errorf("report: verified program page bundle owner: %w", err)
	}
	preparedTargets[defaultIndex] = defaultPrepared
	if err := projectStandaloneArtifactRepositoryV4(
		defaultPrepared, preparedTargets, portfolio.DefaultTargetID,
	); err != nil {
		return err
	}
	repository := defaultPrepared.repository

	// Keep the owner's already projected target bytes for the spool. The large
	// canonical report and repository values are no longer needed after the
	// repository shell has been projected.
	defaultPrepared.reportData = nil
	defaultPrepared.repository = BrowserRepositoryPayload{}
	defaultPrepared.repositoryPayload = nil

	spool, err := prepareStandaloneArtifactSpoolV4(
		repository,
		preparedTargets,
		func(index int) (*preparedStandaloneTarget, error) {
			if index == defaultIndex {
				return defaultPrepared, nil
			}
			prepared, loadErr := expectedStandaloneTargetFromVerifiedRunV4(pages[index])
			if loadErr != nil {
				return nil, loadErr
			}
			if hooks.afterTargetLoad != nil {
				hooks.afterTargetLoad(index)
			}
			return prepared, nil
		},
	)
	if err != nil {
		return err
	}
	validated := &validatedStandaloneTargetBundle{
		identity: StandaloneTargetBundleIdentity{
			Version:                    StandaloneTargetBundleVersion,
			ProgramPagePortfolioSHA256: portfolio.SHA256,
			DefaultTargetIndex:         defaultIndex,
			TargetCount:                len(portfolio.Pages),
		},
		defaultTarget: defaultPrepared,
		spool:         spool,
	}
	publicationErr := writeValidatedStandaloneTargetBundleAtomicWithHooks(
		runDir,
		validated,
		standaloneTargetBundleAtomicHooks{beforeInstall: spool.closeAndRemove},
	)
	return finishStandaloneBundleSpoolV4(spool, publicationErr)
}

func bindVerifiedStandaloneProgramPages(
	runDir string,
	portfolio programpage.Portfolio,
	receipts []VerifiedRunReceipt,
) ([]verifiedStandaloneProgramPage, int, error) {
	if err := portfolio.Validate(); err != nil {
		return nil, -1, fmt.Errorf("report: verified program page bundle authority: %w", err)
	}
	if len(receipts) != len(portfolio.Pages) {
		return nil, -1, fmt.Errorf("report: verified program page bundle receipt count mismatch")
	}
	portfolioRaw, err := portfolio.CanonicalJSON()
	if err != nil {
		return nil, -1, fmt.Errorf("report: verified program page bundle portfolio: %w", err)
	}
	portfolioArtifactSHA256 := manifestSHA256(portfolioRaw)
	absoluteOwnerDir, err := filepath.Abs(runDir)
	if err != nil {
		return nil, -1, fmt.Errorf("report: resolve verified program page bundle owner: %w", err)
	}
	absoluteOwnerDir = filepath.Clean(absoluteOwnerDir)
	runsDir := filepath.Dir(absoluteOwnerDir)
	pages := make([]verifiedStandaloneProgramPage, len(portfolio.Pages))
	defaultIndex := -1
	var defaultManifest RunManifest
	defaultRepositoryName := ""

	for index, binding := range portfolio.Pages {
		receipt := receipts[index]
		expectedRunDir := filepath.Join(runsDir, binding.RunID)
		if err := receipt.ValidateRunIdentity(expectedRunDir); err != nil {
			return nil, -1, fmt.Errorf(
				"report: verified program page bundle receipt %d run directory mismatch: %w", index, err,
			)
		}
		manifest := receipt.Manifest()
		if err := manifest.Validate(); err != nil {
			return nil, -1, fmt.Errorf(
				"report: verified program page bundle receipt %d manifest: %w", index, err,
			)
		}
		page := receipt.ProgramPage()
		if page.RunID != binding.RunID ||
			!reflect.DeepEqual(page.ProgramTarget, binding.Target) ||
			receipt.ProgramTargetID() != binding.Target.ID ||
			manifest.MaterialInputs.ProgramTargetID != binding.Target.ID {
			return nil, -1, fmt.Errorf(
				"report: verified program page bundle receipt %d target binding mismatch", index,
			)
		}
		if receipt.ProgramPagePortfolioSHA256() != portfolioArtifactSHA256 ||
			manifest.MaterialInputs.ProgramPagePortfolioSHA256 != portfolioArtifactSHA256 {
			return nil, -1, fmt.Errorf(
				"report: verified program page bundle receipt %d portfolio binding mismatch", index,
			)
		}
		if receipt.RepositoryName() == "" {
			return nil, -1, fmt.Errorf(
				"report: verified program page bundle receipt %d repository identity is incomplete", index,
			)
		}
		host := ""
		repositoryURL := ""
		if manifest.StandaloneSource != nil {
			host = manifest.StandaloneSource.Host
			repositoryURL = manifest.StandaloneSource.RepositoryURL
		}
		descriptor := &preparedStandaloneTarget{
			programPage:   page,
			host:          host,
			repositoryURL: repositoryURL,
			revision:      manifest.MaterialInputs.SelectedRevision,
			repoName:      receipt.RepositoryName(),
			localRoots: normalizedStandaloneLocalRoots([]string{
				receipt.RunDir(), manifest.AnalysisRoot, manifest.RepositoryState.Identity,
			}),
		}
		pages[index] = verifiedStandaloneProgramPage{
			receipt: receipt, manifest: manifest, descriptor: descriptor,
		}
		if binding.Target.ID == portfolio.DefaultTargetID {
			defaultIndex = index
			defaultManifest = manifest
			defaultRepositoryName = receipt.RepositoryName()
			if receipt.RunDir() != absoluteOwnerDir {
				return nil, -1, fmt.Errorf("report: verified program page bundle destination is not the default run")
			}
		}
	}
	if defaultIndex < 0 {
		return nil, -1, fmt.Errorf("report: verified program page bundle default target is missing")
	}
	for index := range pages {
		manifest := pages[index].manifest
		if manifest.AnalysisRoot != defaultManifest.AnalysisRoot ||
			manifest.RepositoryStateSHA256 != defaultManifest.RepositoryStateSHA256 ||
			manifest.MaterialInputs.SelectedRevision != defaultManifest.MaterialInputs.SelectedRevision ||
			manifest.MaterialInputs.ProgramPagePortfolioSHA256 != defaultManifest.MaterialInputs.ProgramPagePortfolioSHA256 ||
			manifest.MaterialInputs.RuntimePortfolioSHA256 != defaultManifest.MaterialInputs.RuntimePortfolioSHA256 ||
			manifest.MaterialInputs.TargetOutcomePortfolioSHA256 != defaultManifest.MaterialInputs.TargetOutcomePortfolioSHA256 ||
			!reflect.DeepEqual(manifest.StandaloneSource, defaultManifest.StandaloneSource) ||
			pages[index].descriptor.repoName != defaultRepositoryName {
			return nil, -1, fmt.Errorf("report: verified program page bundle repository identity mismatch")
		}
		if index == defaultIndex {
			continue
		}
		htmlPath := filepath.Join(pages[index].receipt.RunDir(), "report.html")
		if _, htmlErr := os.Lstat(htmlPath); htmlErr == nil {
			return nil, -1, fmt.Errorf("report: verified backing child unexpectedly publishes report.html")
		} else if !errors.Is(htmlErr, os.ErrNotExist) {
			return nil, -1, fmt.Errorf("report: inspect verified backing child report.html: %w", htmlErr)
		}
	}
	return pages, defaultIndex, nil
}

func expectedStandaloneTargetFromVerifiedRunV4(
	verified verifiedStandaloneProgramPage,
) (*preparedStandaloneTarget, error) {
	descriptor := verified.descriptor
	if descriptor == nil {
		return nil, fmt.Errorf("verified target descriptor is missing")
	}
	root, err := os.OpenRoot(verified.receipt.RunDir())
	if err != nil {
		return nil, err
	}
	defer root.Close()
	reportJSON, err := readManifestFile(root, "report.json", maxManifestReportBytes)
	if err != nil {
		return nil, err
	}
	data, err := decodeStrictReportJSON(reportJSON)
	if err != nil {
		return nil, err
	}
	if data.ProgramPortfolio == nil {
		return nil, fmt.Errorf("child report ProgramPortfolio authority is incomplete")
	}
	defaultEntry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(defaultEntry.Target, descriptor.programPage.ProgramTarget) {
		return nil, fmt.Errorf("child report ProgramTarget does not match verified receipt")
	}
	if data.RepoName != descriptor.repoName || data.CapturedRevision != descriptor.revision {
		return nil, fmt.Errorf("child report repository identity does not match verified receipt")
	}
	sourceAuthority := OrdinaryReportHTMLAuthority{
		StandaloneSource: verified.manifest.StandaloneSource,
		AnalysisRoot:     verified.manifest.AnalysisRoot,
		RepositoryRoot:   verified.manifest.RepositoryState.Identity,
	}
	githubLinks, gitlabLinks, err := ordinaryHTMLSourceLinks(data.CapturedRevision, sourceAuthority)
	if err != nil {
		return nil, err
	}
	data.GitHubSourceLinks = githubLinks
	data.GitLabSourceLinks = gitlabLinks
	target, err := ProjectBrowserTargetPayload(&data)
	if err != nil {
		return nil, err
	}
	targetRaw, err := encodeBrowserTargetPayloadForHTML(target, descriptor.localRoots)
	if err != nil {
		return nil, err
	}
	target, err = DecodeBrowserTargetPayload(targetRaw)
	if err != nil {
		return nil, err
	}
	ownedData := data
	prepared := &preparedStandaloneTarget{
		programPage:   descriptor.programPage,
		target:        target,
		targetPayload: targetRaw,
		reportData:    &ownedData,
		host:          descriptor.host,
		repositoryURL: descriptor.repositoryURL,
		revision:      descriptor.revision,
		repoName:      descriptor.repoName,
		localRoots:    append([]string(nil), descriptor.localRoots...),
	}
	if err := matchVerifiedStandaloneDescriptor(prepared, descriptor); err != nil {
		return nil, err
	}
	return prepared, nil
}

func matchVerifiedStandaloneDescriptor(
	prepared *preparedStandaloneTarget,
	descriptor *preparedStandaloneTarget,
) error {
	if prepared == nil || descriptor == nil ||
		!reflect.DeepEqual(prepared.programPage, descriptor.programPage) ||
		prepared.host != descriptor.host || prepared.repositoryURL != descriptor.repositoryURL ||
		prepared.revision != descriptor.revision || prepared.repoName != descriptor.repoName ||
		prepared.target.Target.ID != descriptor.programPage.ProgramTarget.ID ||
		len(prepared.targetPayload) == 0 {
		return fmt.Errorf("loaded target does not match its verified receipt")
	}
	return nil
}

type standaloneArtifactPublicationHooks struct {
	afterTargetLoad     func(index int)
	afterSpoolPrepared  func(path string)
	afterTemporaryClose func(path string) error
}

func writeProgramPageBundleFromArtifactsAtomic(
	runDir string,
	portfolio programpage.Portfolio,
	hooks standaloneArtifactPublicationHooks,
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
	loadTarget := func(index int) (*preparedStandaloneTarget, error) {
		if hooks.afterTargetLoad != nil {
			hooks.afterTargetLoad(index)
		}
		return expectedStandaloneProgramPageBundleTarget(
			runsDir, portfolio.Pages[index], defaultManifest,
		)
	}
	preparedTargets := make([]*preparedStandaloneTarget, 0, len(portfolio.Pages))
	var defaultPrepared *preparedStandaloneTarget
	for index := range portfolio.Pages {
		prepared, itemErr := loadTarget(index)
		if itemErr != nil {
			return fmt.Errorf("report: program page bundle target %d: %w", index, itemErr)
		}
		if len(preparedTargets) != 0 && (prepared.repoName != preparedTargets[0].repoName ||
			prepared.host != preparedTargets[0].host ||
			prepared.repositoryURL != preparedTargets[0].repositoryURL ||
			prepared.revision != preparedTargets[0].revision) {
			return fmt.Errorf("report: program page bundle repository identity mismatch")
		}
		preparedTargets = append(preparedTargets, prepared)
		if index == defaultIndex {
			defaultPrepared = prepared
		} else {
			prepared.reportData = nil
		}
		prepared.target = BrowserTargetPayload{}
		prepared.targetPayload = nil
	}
	if defaultPrepared == nil || defaultIndex < 0 {
		return fmt.Errorf("report: program page bundle default target is missing")
	}
	if err := projectStandaloneArtifactRepositoryV4(
		defaultPrepared, preparedTargets, portfolio.DefaultTargetID,
	); err != nil {
		return err
	}
	repository := defaultPrepared.repository
	releaseArtifactPreparedTargetData(preparedTargets)
	spool, err := prepareStandaloneArtifactSpoolV4(
		repository,
		preparedTargets,
		loadTarget,
	)
	if err != nil {
		return err
	}
	if hooks.afterSpoolPrepared != nil {
		hooks.afterSpoolPrepared(spool.path)
	}
	validated := &validatedStandaloneTargetBundle{
		identity: StandaloneTargetBundleIdentity{
			Version:                    StandaloneTargetBundleVersion,
			ProgramPagePortfolioSHA256: portfolio.SHA256,
			DefaultTargetIndex:         defaultIndex,
			TargetCount:                len(portfolio.Pages),
		},
		defaultTarget: defaultPrepared,
		spool:         spool,
	}
	publicationErr := writeValidatedStandaloneTargetBundleAtomicWithHooks(
		runDir, validated, standaloneTargetBundleAtomicHooks{
			afterTemporaryClose: hooks.afterTemporaryClose,
			beforeInstall:       spool.closeAndRemove,
		},
	)
	return finishStandaloneBundleSpoolV4(spool, publicationErr)
}

func writeValidatedStandaloneTargetBundleAtomic(
	runDir string,
	validated *validatedStandaloneTargetBundle,
) error {
	return writeValidatedStandaloneTargetBundleAtomicBeforeInstall(runDir, validated, nil)
}

func writeValidatedStandaloneTargetBundleAtomicBeforeInstall(
	runDir string,
	validated *validatedStandaloneTargetBundle,
	beforeInstall func() error,
) error {
	return writeValidatedStandaloneTargetBundleAtomicWithHooks(
		runDir, validated, standaloneTargetBundleAtomicHooks{beforeInstall: beforeInstall},
	)
}

type standaloneTargetBundleAtomicHooks struct {
	afterTemporaryClose func(path string) error
	beforeInstall       func() error
}

func writeValidatedStandaloneTargetBundleAtomicWithHooks(
	runDir string,
	validated *validatedStandaloneTargetBundle,
	hooks standaloneTargetBundleAtomicHooks,
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
	if hooks.afterTemporaryClose != nil {
		if err := hooks.afterTemporaryClose(temporaryPath); err != nil {
			return fmt.Errorf("report: inspect closed standalone target bundle: %w", err)
		}
	}
	if err := verifyExactStandaloneTargetBundleProjection(temporaryPath, validated); err != nil {
		return fmt.Errorf("report: verify closed standalone target bundle: %w", err)
	}
	if hooks.beforeInstall != nil {
		if err := hooks.beforeInstall(); err != nil {
			return err
		}
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
		if prepared == nil || len(prepared.targetPayload) == 0 || len(prepared.repositoryPayload) == 0 {
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
	ordered := make([]*preparedStandaloneTarget, 0, len(portfolio.Pages))
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
			if bytes.Contains(prepared.targetPayload, []byte(root)) {
				return nil, fmt.Errorf("report: standalone program page bundle retained a local path")
			}
		}
		for _, sibling := range portfolio.Pages {
			if bytes.Contains(prepared.targetPayload, []byte(sibling.RunID)) {
				return nil, fmt.Errorf("report: standalone program page bundle retained a sibling run id")
			}
		}
		ordered = append(ordered, prepared)
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
	transport, err := prepareStandaloneTransportFromTargetsV4(defaultPrepared.repository, ordered)
	if err != nil {
		return nil, err
	}
	return &validatedStandaloneTargetBundle{
		identity: StandaloneTargetBundleIdentity{
			Version:                    StandaloneTargetBundleVersion,
			ProgramPagePortfolioSHA256: portfolio.SHA256,
			DefaultTargetIndex:         defaultIndex,
			TargetCount:                len(portfolio.Pages),
		},
		defaultTarget: defaultPrepared,
		transport:     transport,
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
		if prepared == nil || prepared.analysisTarget == nil ||
			len(prepared.targetPayload) == 0 || len(prepared.repositoryPayload) == 0 {
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
	ordered := make([]*preparedStandaloneTarget, 0, len(container.Targets))
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
		if _, duplicate := seenProgramTargetIDs[prepared.programPage.ProgramTarget.ID]; duplicate {
			return nil, fmt.Errorf("report: standalone target bundle contains duplicate program target identity")
		}
		seenProgramTargetIDs[prepared.programPage.ProgramTarget.ID] = struct{}{}
		if first == nil {
			first = prepared
		} else if prepared.host != first.host || prepared.repositoryURL != first.repositoryURL ||
			prepared.revision != first.revision {
			return nil, fmt.Errorf("report: standalone target bundle source authority mismatch")
		}
		for _, root := range prepared.localRoots {
			if bytes.Contains(prepared.targetPayload, []byte(root)) {
				return nil, fmt.Errorf("report: standalone target bundle retained a local path")
			}
		}
		for _, sibling := range portfolio.Targets {
			if sibling.RunID != "" && bytes.Contains(prepared.targetPayload, []byte(sibling.RunID)) {
				return nil, fmt.Errorf("report: standalone target bundle retained a sibling run id")
			}
		}
		ordered = append(ordered, prepared)
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
	transport, err := prepareStandaloneTransportFromTargetsV4(defaultPrepared.repository, ordered)
	if err != nil {
		return nil, err
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
		transport:     transport,
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

func bindStandaloneBrowserRepositoryPayloadV4(
	payload BrowserRepositoryPayload,
) (BrowserRepositoryPayload, []byte, error) {
	payload.Targets = append([]BrowserTargetIndexItem(nil), payload.Targets...)
	for position := range payload.Targets {
		row := &payload.Targets[position]
		switch row.State {
		case "analyzed":
			row.Href = standaloneTargetHref(position)
		case "not_analyzed":
			row.Href = ""
		default:
			return BrowserRepositoryPayload{}, nil, fmt.Errorf(
				"report: standalone bundle repository target %d has unsupported state", position,
			)
		}
	}
	encoded, err := EncodeBrowserRepositoryPayload(payload)
	if err != nil {
		return BrowserRepositoryPayload{}, nil, fmt.Errorf(
			"report: bind standalone bundle repository routes: %w", err,
		)
	}
	return payload, encoded, nil
}

func prepareStandaloneTransportFromTargetsV4(
	repository BrowserRepositoryPayload,
	preparedTargets []*preparedStandaloneTarget,
) (standaloneBundleTransportV4, error) {
	boundRepository, repositoryRaw, err := bindStandaloneBrowserRepositoryPayloadV4(repository)
	if err != nil {
		return standaloneBundleTransportV4{}, err
	}
	preparedByProgramTarget := make(map[string]*preparedStandaloneTarget, len(preparedTargets))
	localRoots := make([]string, 0)
	for position, prepared := range preparedTargets {
		if prepared == nil || len(prepared.targetPayload) == 0 {
			return standaloneBundleTransportV4{}, fmt.Errorf(
				"report: standalone bundle prepared target %d is invalid", position,
			)
		}
		if _, duplicate := preparedByProgramTarget[prepared.target.Target.ID]; duplicate {
			return standaloneBundleTransportV4{}, fmt.Errorf(
				"report: standalone bundle contains duplicate prepared ProgramTarget identity",
			)
		}
		preparedByProgramTarget[prepared.target.Target.ID] = prepared
		localRoots = append(localRoots, prepared.localRoots...)
	}
	localRoots = normalizedBrowserLocalRoots(localRoots)
	input := standaloneBundleTransportInputV4{
		RepositoryPayload:      repositoryRaw,
		LogicalDefaultTargetID: boundRepository.LogicalDefaultSelectedTargetID,
		Targets:                make([]standaloneBundleTransportTargetInputV4, 0, len(boundRepository.Targets)),
	}
	for position, row := range boundRepository.Targets {
		entry := standaloneBundleTransportTargetInputV4{TargetID: row.SelectedTargetID}
		switch row.State {
		case "analyzed":
			prepared, ok := preparedByProgramTarget[row.ProgramTargetID]
			if !ok || prepared.selectedTargetID != row.SelectedTargetID ||
				prepared.target.Target.ID != row.ProgramTargetID {
				return standaloneBundleTransportV4{}, fmt.Errorf(
					"report: standalone bundle target %d lacks its exact prepared payload", position,
				)
			}
			canonical, encodeErr := EncodeBrowserTargetPayload(prepared.target)
			if encodeErr != nil || !bytes.Equal(canonical, prepared.targetPayload) {
				return standaloneBundleTransportV4{}, fmt.Errorf(
					"report: standalone bundle target %d payload is not canonical", position,
				)
			}
			if browserValueContainsLocalPath(prepared.target, localRoots) ||
				browserValueContainsLocalPath(boundRepository, localRoots) {
				return standaloneBundleTransportV4{}, fmt.Errorf(
					"report: standalone bundle retained a local path",
				)
			}
			entry.ProgramTargetID = row.ProgramTargetID
			entry.State = standaloneBundleTransportTargetAnalyzed
			entry.Payload = prepared.targetPayload
			delete(preparedByProgramTarget, row.ProgramTargetID)
		case "not_analyzed":
			entry.State = standaloneBundleTransportTargetNotAnalyzed
		default:
			return standaloneBundleTransportV4{}, fmt.Errorf(
				"report: standalone bundle target %d has unsupported state", position,
			)
		}
		input.Targets = append(input.Targets, entry)
	}
	if len(preparedByProgramTarget) != 0 {
		return standaloneBundleTransportV4{}, fmt.Errorf(
			"report: standalone bundle contains a foreign prepared target",
		)
	}
	return prepareStandaloneBundleTransportV4(input)
}

func projectStandaloneArtifactRepositoryV4(
	owner *preparedStandaloneTarget,
	preparedTargets []*preparedStandaloneTarget,
	defaultProgramTargetID string,
) error {
	if owner == nil || owner.reportData == nil || len(preparedTargets) == 0 {
		return fmt.Errorf("report: standalone artifact repository projection is incomplete")
	}
	pages := make([]TargetNavigationPage, 0, len(preparedTargets))
	for position, prepared := range preparedTargets {
		if prepared == nil {
			return fmt.Errorf("report: standalone artifact target %d is incomplete", position)
		}
		pages = append(pages, prepared.programPage)
	}
	navigation, err := BuildTargetNavigation(
		pages, defaultProgramTargetID, owner.programPage.ProgramTarget.ID,
	)
	if err != nil {
		return err
	}
	repository, err := ProjectBrowserRepositoryPayload(owner.reportData, navigation)
	if err != nil {
		return err
	}
	localRoots := make([]string, 0)
	for _, prepared := range preparedTargets {
		localRoots = append(localRoots, prepared.localRoots...)
	}
	repositoryRaw, err := encodeBrowserRepositoryPayloadForHTML(repository, localRoots)
	if err != nil {
		return err
	}
	repository, err = DecodeBrowserRepositoryPayload(repositoryRaw)
	if err != nil {
		return err
	}
	selectedByProgramTarget := make(map[string]string, len(repository.Targets))
	for _, row := range repository.Targets {
		if row.State == "analyzed" {
			selectedByProgramTarget[row.ProgramTargetID] = row.SelectedTargetID
		}
	}
	for position, prepared := range preparedTargets {
		selectedTargetID := selectedByProgramTarget[prepared.programPage.ProgramTarget.ID]
		if selectedTargetID == "" {
			return fmt.Errorf(
				"report: standalone artifact target %d is absent from exhaustive repository outcomes", position,
			)
		}
		prepared.selectedTargetID = selectedTargetID
	}
	owner.repository = repository
	owner.repositoryPayload = repositoryRaw
	return nil
}

func releaseArtifactPreparedTargetData(preparedTargets []*preparedStandaloneTarget) {
	for _, prepared := range preparedTargets {
		if prepared == nil {
			continue
		}
		prepared.reportData = nil
		prepared.repository = BrowserRepositoryPayload{}
		prepared.repositoryPayload = nil
		prepared.target = BrowserTargetPayload{}
		prepared.targetPayload = nil
	}
}

type standaloneArtifactTargetReloaderV4 func(index int) (*preparedStandaloneTarget, error)

func prepareStandaloneArtifactSpoolV4(
	repository BrowserRepositoryPayload,
	descriptors []*preparedStandaloneTarget,
	reload standaloneArtifactTargetReloaderV4,
) (*standaloneBundleSpoolV4, error) {
	if len(descriptors) == 0 || reload == nil {
		return nil, fmt.Errorf("report: standalone artifact spool inputs are incomplete")
	}
	byProgramTarget := make(map[string]int, len(descriptors))
	localRoots := make([]string, 0)
	for index, descriptor := range descriptors {
		if descriptor == nil {
			return nil, fmt.Errorf("report: standalone artifact descriptor %d is missing", index)
		}
		programTargetID := descriptor.programPage.ProgramTarget.ID
		if _, duplicate := byProgramTarget[programTargetID]; duplicate {
			return nil, fmt.Errorf("report: standalone artifact descriptors repeat a ProgramTarget")
		}
		byProgramTarget[programTargetID] = index
		localRoots = append(localRoots, descriptor.localRoots...)
	}
	localRoots = normalizedBrowserLocalRoots(localRoots)
	if browserValueContainsLocalPath(repository, localRoots) {
		return nil, fmt.Errorf("report: standalone artifact repository retained a local path")
	}
	loaded := make([]bool, len(descriptors))
	spool, err := prepareStandaloneBundleSpoolV4(
		repository,
		func(_ int, row BrowserTargetIndexItem) ([]byte, error) {
			index, ok := byProgramTarget[row.ProgramTargetID]
			if !ok || loaded[index] {
				return nil, fmt.Errorf("analyzed ProgramTarget has no unique artifact descriptor")
			}
			fresh, reloadErr := reload(index)
			if reloadErr != nil {
				return nil, reloadErr
			}
			descriptor := descriptors[index]
			if fresh == nil || !reflect.DeepEqual(fresh.programPage, descriptor.programPage) ||
				fresh.host != descriptor.host || fresh.repositoryURL != descriptor.repositoryURL ||
				fresh.revision != descriptor.revision || fresh.repoName != descriptor.repoName ||
				fresh.target.Target.ID != row.ProgramTargetID || len(fresh.targetPayload) == 0 {
				return nil, fmt.Errorf("reloaded artifact target does not match its first-pass authority")
			}
			if browserValueContainsLocalPath(fresh.target, localRoots) {
				return nil, fmt.Errorf("reloaded artifact target retained a local path")
			}
			loaded[index] = true
			return fresh.targetPayload, nil
		},
	)
	if err != nil {
		return nil, err
	}
	for index, wasLoaded := range loaded {
		if !wasLoaded {
			primary := fmt.Errorf("report: standalone artifact descriptor %d has no analyzed outcome", index)
			return nil, finishStandaloneBundleSpoolV4(spool, primary)
		}
	}
	return spool, nil
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
	if _, err := io.WriteString(w, marker+"\n"); err != nil {
		return fmt.Errorf("report: write standalone target bundle marker: %w", err)
	}
	if validated.spool != nil {
		if err := writeStandaloneBundleSpoolHTMLSectionV4(w, validated.spool); err != nil {
			return fmt.Errorf("report: write standalone target bundle v4 spool: %w", err)
		}
		return nil
	}
	if err := writeStandaloneBundleTransportHTMLSectionV4(w, validated.transport); err != nil {
		return fmt.Errorf("report: write standalone target bundle v4 transport: %w", err)
	}
	return nil
}

func standaloneTargetBundleSeal(digest hash.Hash) string {
	return standaloneTargetBundleSealPrefix + hex.EncodeToString(digest.Sum(nil)) +
		standaloneTargetBundleSealSuffix
}

// InspectStandaloneTargetBundleHTML detects and verifies one v4 bundle
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
	preparedTargets := make([]*preparedStandaloneTarget, 0, len(container.Targets))
	var defaultPrepared *preparedStandaloneTarget
	for index := range container.Targets {
		prepared, prepareErr := expectedStandaloneTargetBundleTarget(
			runsDir, container.Targets[index], portfolio.Targets[index], manifest,
		)
		if prepareErr != nil {
			return StandaloneTargetBundleIdentity{}, fmt.Errorf(
				"report: standalone target bundle payload %d: %w", index, prepareErr,
			)
		}
		preparedTargets = append(preparedTargets, prepared)
		if index == defaultIndex {
			defaultPrepared = prepared
		} else {
			prepared.reportData = nil
		}
		prepared.target = BrowserTargetPayload{}
		prepared.targetPayload = nil
	}
	if defaultPrepared == nil {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: standalone target bundle default payload is missing")
	}
	if err := projectStandaloneArtifactRepositoryV4(
		defaultPrepared, preparedTargets, defaultPrepared.programPage.ProgramTarget.ID,
	); err != nil {
		return StandaloneTargetBundleIdentity{}, err
	}
	repository := defaultPrepared.repository
	releaseArtifactPreparedTargetData(preparedTargets)
	spool, err := prepareStandaloneArtifactSpoolV4(
		repository,
		preparedTargets,
		func(index int) (*preparedStandaloneTarget, error) {
			return expectedStandaloneTargetBundleTarget(
				runsDir, container.Targets[index], portfolio.Targets[index], manifest,
			)
		},
	)
	if err != nil {
		return StandaloneTargetBundleIdentity{}, err
	}
	verifyErr := verifyExactStandaloneTargetBundleProjection(absolutePath, &validatedStandaloneTargetBundle{
		identity: identity, defaultTarget: defaultPrepared, spool: spool,
	})
	if err := finishStandaloneBundleSpoolV4(spool, verifyErr); err != nil {
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
	preparedTargets := make([]*preparedStandaloneTarget, 0, len(portfolio.Pages))
	var defaultPrepared *preparedStandaloneTarget
	for index := range portfolio.Pages {
		prepared, prepareErr := expectedStandaloneProgramPageBundleTarget(
			runsDir, portfolio.Pages[index], manifest,
		)
		if prepareErr != nil {
			return StandaloneTargetBundleIdentity{}, fmt.Errorf(
				"report: standalone program page bundle payload %d: %w", index, prepareErr,
			)
		}
		preparedTargets = append(preparedTargets, prepared)
		if index == defaultIndex {
			defaultPrepared = prepared
		} else {
			prepared.reportData = nil
		}
		prepared.target = BrowserTargetPayload{}
		prepared.targetPayload = nil
	}
	if defaultPrepared == nil {
		return StandaloneTargetBundleIdentity{}, fmt.Errorf("report: standalone program page bundle default payload is missing")
	}
	if err := projectStandaloneArtifactRepositoryV4(
		defaultPrepared, preparedTargets, portfolio.DefaultTargetID,
	); err != nil {
		return StandaloneTargetBundleIdentity{}, err
	}
	repository := defaultPrepared.repository
	releaseArtifactPreparedTargetData(preparedTargets)
	spool, err := prepareStandaloneArtifactSpoolV4(
		repository,
		preparedTargets,
		func(index int) (*preparedStandaloneTarget, error) {
			return expectedStandaloneProgramPageBundleTarget(
				runsDir, portfolio.Pages[index], manifest,
			)
		},
	)
	if err != nil {
		return StandaloneTargetBundleIdentity{}, err
	}
	verifyErr := verifyExactStandaloneTargetBundleProjection(absolutePath, &validatedStandaloneTargetBundle{
		identity: identity, defaultTarget: defaultPrepared, spool: spool,
	})
	if err := finishStandaloneBundleSpoolV4(spool, verifyErr); err != nil {
		return StandaloneTargetBundleIdentity{}, err
	}
	return identity, nil
}

func verifyExactStandaloneTargetBundleProjection(
	path string,
	validated *validatedStandaloneTargetBundle,
) error {
	if validated == nil || !validStandaloneTargetBundleIdentity(validated.identity) ||
		validated.defaultTarget == nil || validated.defaultTarget.repoName == "" {
		return fmt.Errorf("report: standalone target bundle expected projection is invalid")
	}
	skeleton, err := standaloneTargetBundleSkeleton(validated)
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
	identityJSON, err := json.Marshal(validated.identity)
	if err != nil {
		return err
	}
	marker := standaloneTargetBundleMarkerPrefix +
		base64.RawURLEncoding.EncodeToString(identityJSON) + standaloneTargetBundleMarkerSuffix
	if err := comparator.compare([]byte(marker+"\n"), "bundle marker"); err != nil {
		return err
	}
	expectedWriter := exactStandaloneBundleExpectedWriter{
		comparator: &comparator,
		label:      "bundle v4 transport",
	}
	if validated.spool != nil {
		if err := writeStandaloneBundleSpoolHTMLSectionV4(&expectedWriter, validated.spool); err != nil {
			return err
		}
	} else if err := writeStandaloneBundleTransportHTMLSectionV4(&expectedWriter, validated.transport); err != nil {
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

func projectPreparedStandaloneTargetV4(
	data *ReportData,
	navigation *TargetNavigationPortfolio,
	programTarget programindex.Target,
	analysisTarget *analysistarget.Target,
	localRoots []string,
) (*preparedStandaloneTarget, error) {
	repository, err := ProjectBrowserRepositoryPayload(data, navigation)
	if err != nil {
		return nil, err
	}
	target, err := ProjectBrowserTargetPayload(data)
	if err != nil {
		return nil, err
	}
	if target.Target.ID != programTarget.ID || target.Target.Language != programTarget.Language ||
		target.Target.Kind != programTarget.Kind || target.Target.Name != programTarget.Name ||
		target.Target.Selector != programTarget.Selector {
		return nil, fmt.Errorf("report: browser target payload does not match ProgramTarget authority")
	}
	repositoryRaw, err := encodeBrowserRepositoryPayloadForHTML(repository, localRoots)
	if err != nil {
		return nil, err
	}
	repository, err = DecodeBrowserRepositoryPayload(repositoryRaw)
	if err != nil {
		return nil, err
	}
	targetRaw, err := encodeBrowserTargetPayloadForHTML(target, localRoots)
	if err != nil {
		return nil, err
	}
	target, err = DecodeBrowserTargetPayload(targetRaw)
	if err != nil {
		return nil, err
	}
	selectedTargetID := ""
	for _, row := range repository.Targets {
		if row.ProgramTargetID != programTarget.ID {
			continue
		}
		if selectedTargetID != "" {
			return nil, fmt.Errorf("report: browser repository ProgramTarget binding is ambiguous")
		}
		selectedTargetID = row.SelectedTargetID
	}
	if selectedTargetID == "" {
		return nil, fmt.Errorf("report: browser repository lacks current ProgramTarget binding")
	}
	host := ""
	switch repository.Source.Kind {
	case "github":
		host = "GitHub"
	case "gitlab":
		host = "GitLab"
	default:
		return nil, fmt.Errorf("report: standalone bundle requires hosted source authority")
	}
	var ownedAnalysis *analysistarget.Target
	if analysisTarget != nil {
		owned := analysisTarget.Snapshot()
		ownedAnalysis = &owned
	}
	return &preparedStandaloneTarget{
		analysisTarget:    ownedAnalysis,
		programPage:       TargetNavigationPage{ProgramTarget: programTarget.Snapshot()},
		repository:        repository,
		repositoryPayload: repositoryRaw,
		target:            target,
		targetPayload:     targetRaw,
		selectedTargetID:  selectedTargetID,
		host:              host,
		repositoryURL:     repository.Source.RepositoryURL,
		revision:          repository.Repository.CapturedRevision,
		repoName:          repository.Repository.Name,
		localRoots:        normalizedStandaloneLocalRoots(localRoots),
	}, nil
}

func expectedStandaloneProgramPageBundleTarget(
	runsDir string,
	binding programpage.Page,
	defaultManifest RunManifest,
) (*preparedStandaloneTarget, error) {
	runDir := filepath.Join(runsDir, binding.RunID)
	info, err := os.Lstat(runDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("child run directory is unavailable")
	}
	manifest, err := ReadRunManifest(runDir)
	if err != nil {
		return nil, fmt.Errorf("child manifest: %w", err)
	}
	if binding.Target.ID != defaultManifest.MaterialInputs.ProgramTargetID {
		htmlPath := filepath.Join(runDir, "report.html")
		if _, htmlErr := os.Lstat(htmlPath); htmlErr == nil {
			return nil, fmt.Errorf("backing child unexpectedly publishes report.html")
		} else if !errors.Is(htmlErr, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect backing child report.html: %w", htmlErr)
		}
	}
	if manifest.MaterialInputs.ProgramTargetID != binding.Target.ID ||
		manifest.AnalysisRoot != defaultManifest.AnalysisRoot ||
		manifest.RepositoryStateSHA256 != defaultManifest.RepositoryStateSHA256 ||
		manifest.MaterialInputs.SelectedRevision != defaultManifest.MaterialInputs.SelectedRevision ||
		manifest.MaterialInputs.ProgramPagePortfolioSHA256 != defaultManifest.MaterialInputs.ProgramPagePortfolioSHA256 ||
		manifest.MaterialInputs.RuntimePortfolioSHA256 != defaultManifest.MaterialInputs.RuntimePortfolioSHA256 ||
		manifest.MaterialInputs.TargetOutcomePortfolioSHA256 != defaultManifest.MaterialInputs.TargetOutcomePortfolioSHA256 ||
		!reflect.DeepEqual(manifest.StandaloneSource, defaultManifest.StandaloneSource) {
		return nil, fmt.Errorf("child manifest authority mismatch")
	}
	return expectedStandaloneTargetFromManifestV4(runDir, binding.RunID, manifest, binding.Target, nil)
}

func expectedStandaloneTargetBundleTarget(
	runsDir string,
	projection snapshot.TargetRunProjection,
	page snapshot.TargetPage,
	defaultManifest RunManifest,
) (*preparedStandaloneTarget, error) {
	if page.TargetRef != projection.Target.Ref {
		return nil, fmt.Errorf("target order does not match container")
	}
	runDir := filepath.Join(runsDir, page.RunID)
	info, err := os.Lstat(runDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("child run directory is unavailable")
	}
	manifest, err := ReadRunManifest(runDir)
	if err != nil {
		return nil, fmt.Errorf("child manifest: %w", err)
	}
	if manifest.MaterialInputs.AnalysisTargetRef != projection.Target.Ref ||
		manifest.AnalysisRoot != defaultManifest.AnalysisRoot ||
		manifest.RepositoryStateSHA256 != defaultManifest.RepositoryStateSHA256 ||
		manifest.MaterialInputs.SelectedRevision != defaultManifest.MaterialInputs.SelectedRevision ||
		manifest.MaterialInputs.TargetRunContainerSHA256 != defaultManifest.MaterialInputs.TargetRunContainerSHA256 ||
		manifest.MaterialInputs.TargetPagePortfolioSHA256 != defaultManifest.MaterialInputs.TargetPagePortfolioSHA256 ||
		manifest.MaterialInputs.RuntimePortfolioSHA256 != defaultManifest.MaterialInputs.RuntimePortfolioSHA256 ||
		!reflect.DeepEqual(manifest.StandaloneSource, defaultManifest.StandaloneSource) {
		return nil, fmt.Errorf("child manifest authority mismatch")
	}
	prepared, err := expectedStandaloneTargetFromManifestV4(
		runDir, page.RunID, manifest, programindex.Target{}, &projection.Target,
	)
	if err != nil {
		return nil, err
	}
	return prepared, nil
}

func expectedStandaloneTargetFromManifestV4(
	runDir string,
	runID string,
	manifest RunManifest,
	wantProgramTarget programindex.Target,
	wantAnalysisTarget *analysistarget.Target,
) (*preparedStandaloneTarget, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	reportJSON, err := readManifestFile(root, "report.json", maxManifestReportBytes)
	if err != nil {
		return nil, err
	}
	if err := manifest.VerifyReportJSON(reportJSON); err != nil {
		return nil, err
	}
	data, err := decodeStrictReportJSON(reportJSON)
	if err != nil {
		return nil, err
	}
	if data.ProgramPortfolio == nil {
		return nil, fmt.Errorf("child report ProgramPortfolio authority is incomplete")
	}
	if wantAnalysisTarget != nil {
		if data.AnalysisTarget == nil || !reflect.DeepEqual(*data.AnalysisTarget, *wantAnalysisTarget) {
			return nil, fmt.Errorf("child report target does not match container")
		}
	}
	defaultEntry, err := data.ProgramPortfolio.defaultEntry()
	if err != nil {
		return nil, err
	}
	if wantProgramTarget.ID != "" && !reflect.DeepEqual(defaultEntry.Target, wantProgramTarget) {
		return nil, fmt.Errorf("child report ProgramTarget does not match portfolio")
	}
	if wantAnalysisTarget != nil {
		if err := validateCubeMapProgramTarget(*wantAnalysisTarget, defaultEntry.Target); err != nil {
			return nil, err
		}
	}
	setRaw, err := readManifestFile(root, programindex.ArtifactSetFilename, programindex.MaxArtifactSetBytes)
	if err != nil {
		return nil, err
	}
	if manifestSHA256(setRaw) != manifest.MaterialInputs.ProgramIndexSetSHA256 {
		return nil, fmt.Errorf("child ProgramIndex set authority mismatch")
	}
	set, err := programindex.DecodeArtifactSet(setRaw)
	if err != nil {
		return nil, err
	}
	if set.DefaultTargetID != defaultEntry.Target.ID {
		return nil, fmt.Errorf("child ProgramIndex default target mismatch")
	}
	artifactFilename := ""
	for _, entry := range set.Entries {
		if entry.TargetID == set.DefaultTargetID {
			artifactFilename = entry.Filename
			break
		}
	}
	if artifactFilename == "" {
		return nil, fmt.Errorf("child ProgramIndex artifact is missing")
	}
	sourceAuthority := OrdinaryReportHTMLAuthority{
		StandaloneSource: manifest.StandaloneSource,
		AnalysisRoot:     manifest.AnalysisRoot,
		RepositoryRoot:   manifest.RepositoryState.Identity,
	}
	githubLinks, gitlabLinks, err := ordinaryHTMLSourceLinks(data.CapturedRevision, sourceAuthority)
	if err != nil {
		return nil, err
	}
	data.GitHubSourceLinks = githubLinks
	data.GitLabSourceLinks = gitlabLinks
	absoluteRunDir, err := filepath.Abs(runDir)
	if err != nil {
		return nil, err
	}
	localRoots := normalizedStandaloneLocalRoots([]string{
		absoluteRunDir, manifest.AnalysisRoot, manifest.RepositoryState.Identity,
	})
	target, err := ProjectBrowserTargetPayload(&data)
	if err != nil {
		return nil, err
	}
	targetRaw, err := encodeBrowserTargetPayloadForHTML(target, localRoots)
	if err != nil {
		return nil, err
	}
	target, err = DecodeBrowserTargetPayload(targetRaw)
	if err != nil {
		return nil, err
	}
	host := ""
	repositoryURL := ""
	if githubLinks != nil {
		host = "GitHub"
		repositoryURL = githubLinks.RepositoryURL
	} else if gitlabLinks != nil {
		host = "GitLab"
		repositoryURL = gitlabLinks.RepositoryURL
	}
	ownedData := data
	return &preparedStandaloneTarget{
		analysisTarget: wantAnalysisTarget,
		programPage: TargetNavigationPage{
			RunID: runID, ProgramTarget: defaultEntry.Target.Snapshot(), ArtifactFilename: artifactFilename,
		},
		target:        target,
		targetPayload: targetRaw,
		reportData:    &ownedData,
		host:          host,
		repositoryURL: repositoryURL,
		revision:      data.CapturedRevision,
		repoName:      data.RepoName,
		localRoots:    localRoots,
	}, nil
}

type exactStandaloneBundleComparator struct {
	reader io.Reader
	digest hash.Hash
	buffer []byte
}

type exactStandaloneBundleExpectedWriter struct {
	comparator *exactStandaloneBundleComparator
	label      string
}

func (writer *exactStandaloneBundleExpectedWriter) Write(expected []byte) (int, error) {
	if writer == nil || writer.comparator == nil {
		return 0, fmt.Errorf("report: standalone target bundle comparator is unavailable")
	}
	if err := writer.comparator.compare(expected, writer.label); err != nil {
		return 0, err
	}
	return len(expected), nil
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
