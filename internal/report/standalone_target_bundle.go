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
	"github.com/dvordrova/repomap/internal/snapshot"
)

const (
	StandaloneTargetBundleVersion     = 1
	StandaloneTargetNavigationVersion = 3

	// One GiB is a terminal aggregate payload bound, not a prefix-clipping
	// budget. Every ready target is either present in full or publication fails.
	MaxStandaloneTargetBundlePayloadBytes int64 = 1 << 30

	maxPreparedStandaloneTargetHTMLBytes = int64(256 << 20)
	maxStandaloneTargetBundleHeaderBytes = int64(8 << 20)
	maxStandaloneTargetBundleSealBytes   = int64(512)
	maxStandaloneTargetBundleOverhead    = int64(64 << 20)

	standaloneTargetBundlePlaceholder  = "REPOMAP_STANDALONE_TARGET_BUNDLE_PLACEHOLDER_3D909014"
	standaloneTargetBundleMarkerPrefix = "<!-- repomap-standalone-target-bundle:v1:"
	standaloneTargetBundleMarkerSuffix = " -->"
	standaloneTargetBundleSealPrefix   = "<!-- repomap-standalone-target-bundle-seal:v1:"
	standaloneTargetBundleSealSuffix   = " -->\n"

	reportDataScriptOpen  = `<script type="application/json" id="rm-report-data">`
	reportDataScriptClose = `</script>`
)

//go:embed templates/standalone_target_bootstrap.js
var standaloneTargetBootstrapJS string

// PreparedStandaloneTarget is an opaque, fully scrubbed hosted target
// payload. Values can only be produced by the authorized GitHub/GitLab
// generation seams below; callers cannot manufacture an unavailable slot or
// alter target/source authority.
type PreparedStandaloneTarget struct {
	prepared *preparedStandaloneTarget
}

type preparedStandaloneTarget struct {
	target            analysistarget.Target
	payload           []byte
	host              string
	repositoryURL     string
	revision          string
	language          string
	localizationState string
	repoName          string
	projectGuess      string
	hasCanvas         bool
	hasSurfaces       bool
	localRoots        []string
}

// StandaloneTargetBundleIdentity is the small public identity returned by the
// bounded streaming inspector. It binds the exact container and terminal page
// portfolio without exposing sibling run IDs or filesystem locations.
type StandaloneTargetBundleIdentity struct {
	Version                   int    `json:"version"`
	TargetRunContainerSHA256  string `json:"target_run_container_sha256"`
	TargetPagePortfolioSHA256 string `json:"target_page_portfolio_sha256"`
	DefaultTargetIndex        int    `json:"default_target_index"`
	TargetCount               int    `json:"target_count"`
	ReadyTargetCount          int    `json:"ready_target_count"`
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
// GitHub generation (including canonical JSON, report.html and Manifest 18),
// then returns an opaque scrubbed payload for a later all-targets bundle.
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
	prepared, err := prepareStandaloneTargetFromHTML(htmlBytes, localRoots)
	if err != nil {
		return PreparedStandaloneTarget{}, err
	}
	if prepared.host != wantHost || prepared.revision != strings.ToLower(authority.repository.Head) {
		return PreparedStandaloneTarget{}, fmt.Errorf("report: prepared standalone target source authority mismatch")
	}
	return PreparedStandaloneTarget{prepared: prepared}, nil
}

func prepareStandaloneTargetFromHTML(
	htmlBytes []byte,
	localRoots []string,
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
	stripStandaloneSourceContent(decoded)
	stripStandaloneLocalPaths(decoded, localRoots)
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("report: encode prepared standalone target payload: %w", err)
	}

	var envelope struct {
		FormatVersion      int                    `json:"format_version"`
		AnalysisTarget     *analysistarget.Target `json:"analysis_target"`
		ReportLanguage     string                 `json:"report_language"`
		RepoName           string                 `json:"repo_name"`
		ProjectGuess       string                 `json:"project_guess"`
		GitHubSourceLinks  *GitHubSourceLinks     `json:"github_source_links"`
		GitLabSourceLinks  *GitLabSourceLinks     `json:"gitlab_source_links"`
		ArchitectureCanvas json.RawMessage        `json:"architecture_canvas"`
		DiscoveredSurfaces json.RawMessage        `json:"discovered_surfaces"`
	}
	if err := json.Unmarshal(canonical, &envelope); err != nil {
		return nil, fmt.Errorf("report: inspect prepared standalone target payload: %w", err)
	}
	if envelope.FormatVersion != CurrentFormatVersion || envelope.AnalysisTarget == nil {
		return nil, fmt.Errorf("report: prepared standalone target has incompatible report authority")
	}
	if err := envelope.AnalysisTarget.Validate(); err != nil {
		return nil, fmt.Errorf("report: prepared standalone target analysis target: %w", err)
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

	localizationState, err := standaloneLocalizationState(htmlBytes)
	if err != nil {
		return nil, err
	}
	return &preparedStandaloneTarget{
		target:            envelope.AnalysisTarget.Snapshot(),
		payload:           canonical,
		host:              host,
		repositoryURL:     repositoryURL,
		revision:          revision,
		language:          normalizedReportLanguage(envelope.ReportLanguage),
		localizationState: localizationState,
		repoName:          envelope.RepoName,
		projectGuess:      envelope.ProjectGuess,
		hasCanvas:         rawJSONPresent(envelope.ArchitectureCanvas),
		hasSurfaces:       rawJSONPresent(envelope.DiscoveredSurfaces),
		localRoots:        normalizedStandaloneLocalRoots(localRoots),
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

func rawJSONPresent(value json.RawMessage) bool {
	trimmed := bytes.TrimSpace(value)
	return len(trimmed) != 0 && !bytes.Equal(trimmed, []byte("null"))
}

func standaloneLocalizationState(htmlBytes []byte) (string, error) {
	const marker = `rm-localization-status--`
	start := bytes.Index(htmlBytes, []byte(marker))
	if start < 0 {
		return "", nil
	}
	if bytes.LastIndex(htmlBytes, []byte(marker)) != start {
		return "", fmt.Errorf("report: prepared standalone target has duplicate localization state")
	}
	start += len(marker)
	end := bytes.IndexByte(htmlBytes[start:], '"')
	if end < 0 {
		return "", fmt.Errorf("report: prepared standalone target localization state is unterminated")
	}
	state := string(htmlBytes[start : start+end])
	if state != PresentationLocalizationSucceeded && state != PresentationLocalizationFailed &&
		state != presentationLocalizationStageOwned {
		return "", fmt.Errorf("report: prepared standalone target localization state is invalid")
	}
	return state, nil
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
	TargetRef   string              `json:"target_ref"`
	Kind        analysistarget.Kind `json:"kind"`
	ModulePath  string              `json:"module_path"`
	ModuleDir   string              `json:"module_dir"`
	DisplayPath string              `json:"display_path"`
	Available   bool                `json:"available"`
	Href        string              `json:"href,omitempty"`
	Payload     json.RawMessage     `json:"payload,omitempty"`
}

type validatedStandaloneTargetBundle struct {
	identity      StandaloneTargetBundleIdentity
	defaultTarget *preparedStandaloneTarget
	targets       []standaloneTargetBundleItem
	hasCanvas     bool
	hasSurfaces   bool
}

// WriteStandaloneTargetBundleAtomic replaces runDir/report.html only after
// the complete canonical bundle has been validated and written. The caller
// supplies opaque payloads for ready targets only; canonical order, default
// ownership and unavailable slots come exclusively from container+portfolio.
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
	if len(ready) == 0 {
		return nil, fmt.Errorf("report: standalone target bundle has no prepared targets")
	}

	preparedByRef := make(map[string]*preparedStandaloneTarget, len(ready))
	for index, opaque := range ready {
		prepared := opaque.prepared
		if prepared == nil || len(prepared.payload) == 0 {
			return nil, fmt.Errorf("report: standalone target bundle prepared target %d is invalid", index)
		}
		if _, duplicate := preparedByRef[prepared.target.Ref]; duplicate {
			return nil, fmt.Errorf("report: standalone target bundle contains duplicate prepared target")
		}
		preparedByRef[prepared.target.Ref] = prepared
	}

	defaultIndex := -1
	readyCount := 0
	aggregate := int64(0)
	targets := make([]standaloneTargetBundleItem, 0, len(container.Targets))
	var first *preparedStandaloneTarget
	var defaultPrepared *preparedStandaloneTarget
	hasCanvas := false
	hasSurfaces := false
	for index, projection := range container.Targets {
		page := portfolio.Targets[index]
		if projection.Target.Ref == container.DefaultTargetRef {
			defaultIndex = index
		}
		item := standaloneTargetBundleItem{
			TargetRef:   projection.Target.Ref,
			Kind:        projection.Target.Kind,
			ModulePath:  projection.Target.ModulePath,
			ModuleDir:   projection.Target.ModuleDir,
			DisplayPath: projection.DisplayPath,
			Available:   page.State == snapshot.TargetPageReady,
		}
		prepared, found := preparedByRef[projection.Target.Ref]
		if !item.Available {
			if found {
				return nil, fmt.Errorf("report: standalone target bundle prepared an unavailable target")
			}
			targets = append(targets, item)
			continue
		}
		if !found {
			return nil, fmt.Errorf("report: standalone target bundle is missing one ready target")
		}
		if !reflect.DeepEqual(prepared.target, projection.Target) {
			return nil, fmt.Errorf("report: standalone target bundle target authority mismatch")
		}
		if first == nil {
			first = prepared
		} else if prepared.host != first.host || prepared.repositoryURL != first.repositoryURL ||
			prepared.revision != first.revision || prepared.language != first.language ||
			prepared.localizationState != first.localizationState {
			return nil, fmt.Errorf("report: standalone target bundle source or language authority mismatch")
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
		item.Href = fmt.Sprintf("?target=%d#canvas", index)
		item.Payload = json.RawMessage(prepared.payload)
		targets = append(targets, item)
		readyCount++
		hasCanvas = hasCanvas || prepared.hasCanvas
		hasSurfaces = hasSurfaces || prepared.hasSurfaces
		delete(preparedByRef, projection.Target.Ref)
		if index == defaultIndex {
			defaultPrepared = prepared
		}
	}
	if len(preparedByRef) != 0 {
		return nil, fmt.Errorf("report: standalone target bundle contains a foreign prepared target")
	}
	if defaultIndex < 0 || defaultPrepared == nil {
		return nil, fmt.Errorf("report: standalone target bundle default target is unavailable")
	}
	return &validatedStandaloneTargetBundle{
		identity: StandaloneTargetBundleIdentity{
			Version:                   StandaloneTargetBundleVersion,
			TargetRunContainerSHA256:  container.SHA256,
			TargetPagePortfolioSHA256: portfolio.SHA256,
			DefaultTargetIndex:        defaultIndex,
			TargetCount:               len(container.Targets),
			ReadyTargetCount:          readyCount,
		},
		defaultTarget: defaultPrepared,
		targets:       targets,
		hasCanvas:     hasCanvas,
		hasSurfaces:   hasSurfaces,
	}, nil
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
	if prepared.projectGuess != "" {
		title += " — " + prepared.projectGuess
	}
	var buffer bytes.Buffer
	err := reportTmpl.Execute(&buffer, map[string]any{
		"Title":                  title,
		"Language":               prepared.language,
		"CSS":                    template.CSS(withoutSourceEpisodeAssetBlocks(styleCSS)),
		"HasArchitectureCanvas":  validated.hasCanvas,
		"ArchitectureCanvasCSS":  template.CSS(architectureCanvasCSS),
		"ELKJS":                  template.JS(elkJSBundledJS),
		"ArchitectureCanvasJS":   template.JS(architectureCanvasJS),
		"HasDiscoveredSurfaces":  validated.hasSurfaces,
		"SurfaceCatalogCSS":      template.CSS(surfaceCatalogCSS),
		"SurfaceCatalogJS":       template.JS(surfaceCatalogJS),
		"LocalizationState":      prepared.localizationState,
		"StandaloneTargetBundle": template.HTML(standaloneTargetBundlePlaceholder),
		"UIMessagesJS":           template.JS(uiMessagesJS),
		"JS":                     template.JS(withoutSourceEpisodeAssetBlocks(scriptJS)),
	})
	if err != nil {
		return nil, fmt.Errorf("report: render standalone target bundle shell: %w", err)
	}
	return buffer.Bytes(), nil
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

func validStandaloneTargetBundleIdentity(identity StandaloneTargetBundleIdentity) bool {
	return identity.Version == StandaloneTargetBundleVersion &&
		validStandaloneTargetBundleDigest(identity.TargetRunContainerSHA256) &&
		validStandaloneTargetBundleDigest(identity.TargetPagePortfolioSHA256) &&
		identity.TargetCount > 1 && identity.ReadyTargetCount > 0 &&
		identity.ReadyTargetCount <= identity.TargetCount &&
		identity.DefaultTargetIndex >= 0 && identity.DefaultTargetIndex < identity.TargetCount
}

func validStandaloneTargetBundleDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
