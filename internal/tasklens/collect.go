package tasklens

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/reporead"
)

const (
	maxCollectedFileBytes    = 64 << 10
	maxCompleteSymbolBytes   = 32 << 10
	maxCompleteFileBytes     = 64 << 10
	maxManifestFileBytes     = 16 << 10
	maxGrepOutputBytes       = 256 << 10
	maxGrepSignalPaths       = 256
	maxCandidatePathsPerTerm = 24
	maxTerms                 = 32
	maxGrepTerms             = 12
	maxAnchorsPerFile        = 8
	maxFileLexicalNeighbors  = 2
	maxASTLinkedZeroTargets  = MaxReadFiles * maxAnchorsPerFile
	maxExcerptLines          = 36
	maxFragmentExcerptLines  = 4096
	fragmentContextLines     = 2
)

var (
	markdownHeadingPattern = regexp.MustCompile(`(?m)^#{1,6}\s+(.+?)\s*$`)
	shellFunctionPattern   = regexp.MustCompile(`^\s*(?:function\s+)?[A-Za-z_][A-Za-z0-9_.-]*\s*(?:\(\s*\))?\s*\{`)
	makeTargetPattern      = regexp.MustCompile(`^[^\s#][^:=]*:(?:\s|$)`)
	configSectionPattern   = regexp.MustCompile(`^\s*\[[^]]+\]\s*$`)
	configKeyPattern       = regexp.MustCompile(`^[A-Za-z0-9_.-]+\s*:\s*(?:$|[|>])`)
	wordPattern            = regexp.MustCompile(`[\pL\pN_][\pL\pN_.:/-]*`)
	backtickPattern        = regexp.MustCompile("`([^`\\r\\n]{1,160})`")
	modulePattern          = regexp.MustCompile(`(?m)^\s*module\s+([^\s]+)\s*$`)
	manifestNamePattern    = regexp.MustCompile(`(?m)^\s*(?:name\s*=|\"name\"\s*:)\s*[\"']([^\"']+)[\"']`)
)

var taskStopWords = map[string]struct{}{
	"about": {}, "after": {}, "also": {}, "and": {}, "before": {}, "behavior": {}, "both": {},
	"can": {}, "code": {}, "could": {}, "create": {}, "determine": {}, "does": {}, "exact": {},
	"explain": {}, "fails": {},
	"file": {}, "find": {}, "from": {}, "have": {}, "into": {}, "likely": {}, "minimal": {},
	"identify": {}, "inspect": {}, "more": {}, "only": {}, "other": {}, "paths": {}, "produce": {}, "propose": {}, "repository": {},
	"should": {}, "similar": {}, "smallest": {}, "source": {}, "such": {}, "task": {}, "that": {}, "their": {}, "then": {},
	"the": {}, "this": {}, "through": {}, "using": {}, "when": {}, "where": {}, "which": {}, "while": {},
	"whose": {}, "with": {}, "without": {}, "would": {}, "verify": {}, "verification": {}, "user": {},
}

type CollectOptions struct {
	RepositoryPath string
	TaskText       string
	DisplayName    string
	Now            func() time.Time
}

type CollectResult struct {
	Bundle Bundle
	Trace  RetrievalTrace
}

type candidate struct {
	path       string
	score      int
	termHits   map[string]struct{}
	grepLines  []int
	pathHit    bool
	isTest     bool
	isDocument bool
}

type collectedFile struct {
	candidate        candidate
	content          []byte
	lines            []string
	truncated        bool
	truncationReason string
	// sourceTotalLines is exact for a complete bounded read and zero when the
	// file extends beyond that read. A partial scope must never turn the visible
	// prefix into a fabricated whole-file line count.
	sourceTotalLines int
}

type anchorCandidate struct {
	anchor Anchor
	score  int
	terms  []string
	stage  RetrievalStage
}

type goAnchorLedgerItem struct {
	candidate anchorCandidate
	taskScore int
}

type goAnchorReference struct {
	name       string
	directCall bool
}

type moduleCollection struct {
	modules           []Module
	filesFound        int
	filesRead         int
	bytesRead         int
	manifestFilesRead int
	manifestBytesRead int
}

type manifestBudgetReader struct {
	reader          *reporead.Reader
	files           int
	bytes           int
	sourceScanBytes int
	cache           map[string]reporead.Content
}

var taskLensStagesSkipped = []string{
	"architecture_synthesis", "generic_orientation", "guided_tour", "mechanism_opportunity",
	"paved_paths", "repository_study_map", "runtime_surface_discovery",
}

// CanonicalStagesSkipped returns the exact ordered set of generic stages that
// the dedicated Task Lens path bypasses.
func CanonicalStagesSkipped() []string {
	return append([]string(nil), taskLensStagesSkipped...)
}

func (reader *manifestBudgetReader) ReadFile(
	filePath string,
	requestedBytes int,
) (reporead.Content, bool, error) {
	if reader == nil || reader.reader == nil {
		return reporead.Content{}, false, fmt.Errorf("task lens: repository reader is unavailable")
	}
	if content, ok := reader.cache[filePath]; ok {
		return content, true, nil
	}
	if reader.files >= MaxReadFiles || reader.bytes >= MaxReadBytes {
		return reporead.Content{}, false, fmt.Errorf("task lens: repository read budget exhausted")
	}
	limit := min(requestedBytes, MaxReadBytes-reader.bytes)
	content, err := reader.reader.ReadFile(filePath, int64(limit))
	if err != nil {
		return reporead.Content{}, false, err
	}
	reader.files++
	reader.bytes += len(content.Bytes)
	reader.cache[filePath] = content
	return content, false, nil
}

// ParseTaskText extracts the prompt-safe task section from a benchmark-style
// packet. Ordinary task files without that heading are preserved as-is.
func ParseTaskText(raw []byte) (string, error) {
	if len(raw) == 0 || len(raw) > maxTaskBytes {
		return "", fmt.Errorf("task lens: task file is outside bounds")
	}
	if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
		return "", fmt.Errorf("task lens: task file must be valid UTF-8 text")
	}
	text := strings.TrimSpace(strings.ReplaceAll(string(raw), "\r\n", "\n"))
	const heading = "## Prompt-safe task"
	if start := strings.Index(text, heading); start >= 0 {
		body := text[start+len(heading):]
		if next := strings.Index(body, "\n## "); next >= 0 {
			body = body[:next]
		}
		text = strings.TrimSpace(body)
	}
	if text == "" {
		return "", fmt.Errorf("task lens: task text is empty")
	}
	return text, nil
}

// Collect builds one deterministic, bounded facts bundle. It does not invoke
// a model, execute repository code, or build a global call graph.
func Collect(ctx context.Context, opts CollectOptions) (Bundle, error) {
	started := time.Now()
	if opts.Now != nil {
		started = opts.Now()
	}
	if strings.TrimSpace(opts.RepositoryPath) == "" {
		return Bundle{}, fmt.Errorf("task lens: repository path is required")
	}
	taskText := strings.TrimSpace(opts.TaskText)
	if len(taskText) == 0 || len(taskText) > maxTaskBytes || !utf8.ValidString(taskText) {
		return Bundle{}, fmt.Errorf("task lens: task text is outside bounds")
	}
	absRepo, err := filepath.Abs(opts.RepositoryPath)
	if err != nil {
		return Bundle{}, fmt.Errorf("task lens: resolve repository path: %w", err)
	}
	absRepo, err = filepath.EvalSymlinks(absRepo)
	if err != nil {
		return Bundle{}, fmt.Errorf("task lens: resolve repository root: %w", err)
	}
	localCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	topLevel, err := gitText(localCtx, absRepo, "rev-parse", "--show-toplevel")
	if err != nil {
		return Bundle{}, err
	}
	topLevel, err = filepath.EvalSymlinks(topLevel)
	if err != nil {
		return Bundle{}, fmt.Errorf("task lens: resolve worktree root: %w", err)
	}
	if filepath.Clean(topLevel) != filepath.Clean(absRepo) {
		return Bundle{}, fmt.Errorf("task lens: repository path must be the worktree root")
	}

	tracked, err := gitfiles.List(absRepo)
	if err != nil {
		return Bundle{}, err
	}
	sort.Strings(tracked)
	tracked = regularTrackedFiles(absRepo, tracked)
	reader, err := reporead.New(absRepo)
	if err != nil {
		return Bundle{}, err
	}
	defer reader.Close()

	repository, readBudget, err := collectRepository(
		localCtx, absRepo, tracked, reader, opts.DisplayName,
	)
	if err != nil {
		return Bundle{}, err
	}
	terms := extractTerms(taskText)
	kindHint := classifyTaskKind(taskText)
	profile := DeriveTaskProfile(taskText, kindHint)
	roleContract, err := DefaultRoleContract(profile)
	if err != nil {
		return Bundle{}, err
	}
	candidates, candidateItemsFound, grepQueries, err := collectCandidates(
		localCtx, absRepo, tracked, terms, taskText,
	)
	if err != nil {
		return Bundle{}, err
	}
	selectedFiles := selectFilesForProfile(candidates, terms, kindHint, profile)
	moduleFacts := collectModules(tracked, readBudget, selectedFiles)
	repository = finalizeRepositoryIdentity(repository, tracked, readBudget, moduleFacts.modules)
	moduleFacts.manifestFilesRead = readBudget.files
	moduleFacts.manifestBytesRead = readBudget.bytes
	files, evidenceFilesRead, readFiles, readBytes, sourceScanBytes,
		fileLimitBound, byteLimitBound, sourceScanLimitBound, err := readCandidateFiles(
		readBudget, selectedFiles,
	)
	if err != nil {
		return Bundle{}, err
	}
	anchorCandidates, astParses := extractAnchors(files, terms, kindHint, taskText)
	anchors := selectAnchorsForRoleContract(anchorCandidates, terms, roleContract)
	anchors, frontierExpansions := completeMissingRoleAnchors(
		anchors,
		anchorCandidates,
		terms,
		roleContract,
	)
	anchors = completeVerificationAnchor(anchors, anchorCandidates, terms, roleContract)
	anchors, retainedSourceBytes, retainedByteLimitBound := boundAnchorsBySourceBytes(anchors)

	evidence := []Evidence{{
		ID: OpaqueID("evidence", "task", SHA256([]byte(taskText))), Kind: EvidenceTaskProvided,
		Summary: "Symptom or requested outcome supplied by the task; not repository truth.",
	}}
	for index := range anchors {
		kind := EvidenceRepositoryFact
		if isDocumentPath(anchors[index].Path) {
			kind = EvidenceDocumentClaim
		}
		evidenceID := OpaqueID(
			"evidence", repository.StateSHA256, anchors[index].Path,
			strconv.Itoa(anchors[index].StartLine), strconv.Itoa(anchors[index].EndLine),
			SourceExcerptSHA256(anchors[index].Excerpt),
		)
		anchors[index].EvidenceIDs = []string{evidenceID}
		evidence = append(evidence, Evidence{
			ID: evidenceID, Kind: kind, Path: anchors[index].Path,
			StartLine: anchors[index].StartLine, EndLine: anchors[index].EndLine,
			AnchorID: anchors[index].ID,
			Summary:  anchorEvidenceSummary(anchors[index], kind),
		})
	}
	groundTerms(terms, anchors)
	relations := collectRelations(anchors, terms)
	roleCoverage, err := EvaluateRoleCoverage(roleContract, anchors)
	if err != nil {
		return Bundle{}, err
	}
	decisiveRelation, decisiveRelationFound := selectDecisiveRelation(
		relations,
		anchors,
		terms,
		profile,
	)
	verification := buildVerificationFrontier(anchors, relations, decisiveRelation, terms)
	decisiveRelationID := ""
	if decisiveRelationFound {
		decisiveRelationID = decisiveRelation.ID
	}
	cheapExit := EvaluateCheapExit(CheapExitInput{
		AreaIDs: cheapExitAreaIDs(
			decisiveRelation,
			roleCoverage,
			anchors,
			relations,
			verification,
		),
		MissingKeyRoles:         roleCoverage.MissingKeyRoles(),
		DecisiveRelationKind:    RelationKind(decisiveRelation.Kind),
		DecisiveRelationSupport: decisiveRelation.SupportType,
		Verification:            verification,
		UnresolvedCompetingHypotheses: unresolvedCompetingHypotheses(
			roleCoverage,
			anchors,
			relations,
			decisiveRelation,
			verification,
		),
	})
	allowedPaths := make([]string, 0, len(anchors)+len(moduleFacts.modules)+1)
	for _, anchor := range anchors {
		allowedPaths = append(allowedPaths, anchor.Path)
	}
	for _, module := range moduleFacts.modules {
		allowedPaths = append(allowedPaths, module.SourcePath)
	}
	if repository.IdentitySourcePath != "" {
		allowedPaths = append(allowedPaths, repository.IdentitySourcePath)
	}
	sort.Strings(allowedPaths)
	allowedPaths = slices.Compact(allowedPaths)

	now := time.Now()
	if opts.Now != nil {
		now = opts.Now()
	}
	wall := now.Sub(started).Milliseconds()
	if wall < 0 {
		wall = 0
	}
	locality := classifyLocality(taskText, terms, anchors, relations)
	bundle := Bundle{
		Version:    BundleVersion,
		ID:         OpaqueID("task", repository.Identity, repository.Revision, repository.StateSHA256, SHA256([]byte(taskText))),
		Repository: repository,
		Task:       Task{Text: taskText, EvidenceID: evidence[0].ID},
		KindHint:   kindHint, Profile: profile,
		ObservableHint: taskObservable(taskText), Locality: locality,
		Terms: terms, Modules: moduleFacts.modules, Anchors: anchors, Evidence: evidence, Relations: relations,
		RoleContract: roleContract, RoleCoverage: roleCoverage,
		Verification: verification, DecisiveRelationID: decisiveRelationID, CheapExit: cheapExit,
		AllowedPaths:  allowedPaths,
		StagesSkipped: CanonicalStagesSkipped(),
		Budgets: Budgets{
			InitialCandidates: len(candidates), CandidateItemsFound: candidateItemsFound,
			RetainedAnchors: len(anchors), AnchorItemsFound: len(anchorCandidates),
			EvidenceFilesConsidered: len(selectedFiles), ReadFiles: readFiles,
			ReadBytes: readBytes, SourceScanBytes: sourceScanBytes,
			RetainedSourceBytes: retainedSourceBytes,
			GoplsQueries:        0, FrontierExpansions: frontierExpansions, LocalWallMillis: wall,
			CandidateLimitBound:    candidateItemsFound > MaxInitialCandidates,
			AnchorLimitBound:       len(anchorCandidates) > MaxRetainedAnchors,
			FileLimitBound:         fileLimitBound,
			ByteLimitBound:         byteLimitBound,
			SourceScanLimitBound:   sourceScanLimitBound,
			RetainedByteLimitBound: retainedByteLimitBound,
			TimeLimitBound:         localCtx.Err() == context.DeadlineExceeded,
		},
		Metrics: RetrievalMetrics{
			TrackedFiles: len(tracked), GitGrepQueries: grepQueries,
			ASTParses: astParses, RelationsRetained: len(relations),
			EvidenceFilesRead: evidenceFilesRead,
			ModuleFilesFound:  moduleFacts.filesFound,
			ModuleFilesRead:   moduleFacts.filesRead,
			ModuleBytesRead:   moduleFacts.bytesRead,
			ManifestFilesRead: moduleFacts.manifestFilesRead,
			ManifestBytesRead: moduleFacts.manifestBytesRead,
		},
	}
	bundle.LocalTrace = buildRetrievalTrace(bundle, anchorCandidates)
	if err := bundle.LocalTrace.Validate(); err != nil {
		return Bundle{}, fmt.Errorf("task lens: validate retrieval trace: %w", err)
	}
	endingState, err := freshness.CaptureRepository(localCtx, absRepo)
	if err != nil {
		return Bundle{}, fmt.Errorf("task lens: recapture repository after bounded retrieval: %w", err)
	}
	endingSHA, err := RepositoryStateSHA(endingState)
	if err != nil {
		return Bundle{}, err
	}
	if endingState.Head != repository.Revision || endingSHA != repository.StateSHA256 {
		return Bundle{}, fmt.Errorf("task lens: repository changed during bounded retrieval")
	}
	if err := VerifyBundleSources(absRepo, bundle); err != nil {
		return Bundle{}, fmt.Errorf("task lens: verify retained source snapshot: %w", err)
	}
	if err := bundle.Validate(); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

// CollectWithTrace returns the same canonical bundle as Collect together with
// the independent, replayable account of pre-ranking candidates and losses.
func CollectWithTrace(ctx context.Context, opts CollectOptions) (CollectResult, error) {
	bundle, err := Collect(ctx, opts)
	if err != nil {
		return CollectResult{}, err
	}
	return CollectResult{Bundle: bundle, Trace: bundle.LocalTrace}, nil
}

// VerifyBundleSources confirms that every retained excerpt still matches the
// exact repository-relative source lines under a symlink-free path. It is run
// after repository-state recapture and again before an authorized report is
// sealed.
func VerifyBundleSources(repositoryPath string, bundle Bundle) error {
	if err := bundle.Validate(); err != nil {
		return err
	}
	reader, err := reporead.New(repositoryPath)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, module := range bundle.Modules {
		if !regularPathWithoutSymlinkComponents(repositoryPath, module.SourcePath) {
			return fmt.Errorf("module source %q is not a symlink-free regular file", module.SourcePath)
		}
		content, readErr := reader.ReadFile(module.SourcePath, maxManifestFileBytes)
		if readErr != nil {
			return fmt.Errorf("read module source %q: %w", module.SourcePath, readErr)
		}
		match := modulePattern.FindSubmatch(content.Bytes)
		if content.Truncated || len(match) != 2 || string(match[1]) != module.Path {
			return fmt.Errorf("module fact no longer matches %s", module.SourcePath)
		}
	}
	if bundle.Repository.IdentitySource == "manifest" {
		sourcePath := bundle.Repository.IdentitySourcePath
		if !regularPathWithoutSymlinkComponents(repositoryPath, sourcePath) {
			return fmt.Errorf("identity source %q is not a symlink-free regular file", sourcePath)
		}
		content, readErr := reader.ReadFile(sourcePath, maxManifestFileBytes)
		if readErr != nil {
			return fmt.Errorf("read identity source %q: %w", sourcePath, readErr)
		}
		match := manifestNamePattern.FindSubmatch(content.Bytes)
		if content.Truncated || len(match) != 2 || string(match[1]) != bundle.Repository.Identity {
			return fmt.Errorf("repository identity no longer matches %s", sourcePath)
		}
	}
	anchorsByPath := make(map[string][]Anchor, len(bundle.AllowedPaths))
	for _, anchor := range bundle.Anchors {
		anchorsByPath[anchor.Path] = append(anchorsByPath[anchor.Path], anchor)
	}
	for sourcePath, anchors := range anchorsByPath {
		if !regularPathWithoutSymlinkComponents(repositoryPath, sourcePath) {
			return fmt.Errorf("retained path %q is not a symlink-free regular file", sourcePath)
		}
		windows := make([]reporead.LineWindow, 0, len(anchors))
		for _, anchor := range anchors {
			// Fragment scopes may intentionally omit distant intervening lines.
			// Re-read only the exact retained lines so verification stays within
			// the same retained-source budget even when scope_start/scope_end span
			// a large source unit.
			for _, sourceLine := range anchor.Excerpt {
				windows = append(windows, reporead.LineWindow{
					Start: sourceLine.Line,
					End:   sourceLine.Line,
				})
			}
		}
		content, readErr := reader.ReadLineWindows(sourcePath, reporead.WindowOptions{
			ScanBytes:   MaxSourceScanBytes,
			RetainBytes: MaxRetainedSourceBytes,
			Windows:     windows,
		})
		if readErr != nil {
			return fmt.Errorf("read retained path %q: %w", sourcePath, readErr)
		}
		lines := make(map[int]string, len(content.Lines))
		for _, sourceLine := range content.Lines {
			if !utf8.ValidString(sourceLine.Text) || strings.IndexByte(sourceLine.Text, 0) >= 0 {
				return fmt.Errorf("retained path %q is not bounded UTF-8 text", sourcePath)
			}
			lines[sourceLine.Number] = sourceLine.Text
		}
		for _, anchor := range anchors {
			for _, sourceLine := range anchor.Excerpt {
				actual, exists := lines[sourceLine.Line]
				if !exists || actual != sourceLine.Text {
					return fmt.Errorf("retained excerpt no longer matches %s:%d", sourcePath, sourceLine.Line)
				}
			}
		}
	}
	return nil
}

// regularTrackedFiles excludes tracked symlink blobs and other non-regular
// entries from model-visible retrieval. reporead intentionally permits a
// symlink whose target stays in the repository, but Task Lens is authoritative
// only over the tracked path's own snapshot content and must not substitute
// bytes from an ignored or untracked target.
func regularTrackedFiles(repo string, tracked []string) []string {
	regular := make([]string, 0, len(tracked))
	for _, filePath := range tracked {
		if !regularPathWithoutSymlinkComponents(repo, filePath) {
			continue
		}
		regular = append(regular, filePath)
	}
	return regular
}

func regularPathWithoutSymlinkComponents(repo, filePath string) bool {
	localPath := filepath.FromSlash(filePath)
	if localPath == "" || filepath.IsAbs(localPath) || !filepath.IsLocal(localPath) ||
		filepath.Clean(localPath) != localPath {
		return false
	}
	current := repo
	parts := strings.Split(localPath, string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if index < len(parts)-1 && !info.IsDir() {
			return false
		}
		if index == len(parts)-1 && !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

func collectRepository(
	ctx context.Context,
	repo string,
	tracked []string,
	reader *reporead.Reader,
	displayName string,
) (Repository, *manifestBudgetReader, error) {
	revision, err := gitText(ctx, repo, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return Repository{}, nil, err
	}
	treeHash, err := gitText(ctx, repo, "rev-parse", "--verify", "HEAD^{tree}")
	if err != nil {
		return Repository{}, nil, err
	}
	state, err := freshness.CaptureRepository(ctx, repo)
	if err != nil {
		return Repository{}, nil, err
	}
	if state.Head != revision {
		return Repository{}, nil, fmt.Errorf("task lens: repository revision changed during capture")
	}
	stateSHA, err := RepositoryStateSHA(state)
	if err != nil {
		return Repository{}, nil, err
	}
	manifestReader := &manifestBudgetReader{
		reader: reader,
		cache:  make(map[string]reporead.Content),
	}
	identity, source := "local-repository", "neutral_fallback"
	if remote, remoteErr := gitText(ctx, repo, "config", "--local", "--get", "remote.origin.url"); remoteErr == nil {
		if normalized := normalizeRemote(remote); normalized != "" {
			identity, source = normalized, "remote"
		}
	}
	if !safeRepositoryIdentity(identity) {
		identity, source = "local-repository", "neutral_fallback"
	}
	if displayName == "" {
		displayName = filepath.Base(repo)
	}
	return Repository{
		Identity: identity, DisplayName: displayName, Revision: revision, TreeHash: treeHash,
		StateSHA256: stateSHA, IdentitySource: source,
	}, manifestReader, nil
}

func finalizeRepositoryIdentity(
	repository Repository,
	tracked []string,
	reader *manifestBudgetReader,
	modules []Module,
) Repository {
	if rootModule, ok := rootModuleIdentity(modules); ok {
		repository.Identity = rootModule.Path
		repository.IdentitySource = "root_module"
		repository.IdentitySourcePath = rootModule.SourcePath
	} else if repository.IdentitySource == "neutral_fallback" {
		if manifest, sourcePath := manifestIdentity(tracked, reader); manifest != "" {
			repository.Identity = manifest
			repository.IdentitySource = "manifest"
			repository.IdentitySourcePath = sourcePath
		}
	}
	if !safeRepositoryIdentity(repository.Identity) {
		repository.Identity = "local-repository"
		repository.IdentitySource = "neutral_fallback"
		repository.IdentitySourcePath = ""
	}
	return repository
}

func rootModuleIdentity(modules []Module) (Module, bool) {
	for _, module := range modules {
		if module.Dir == "." {
			return module, true
		}
	}
	return Module{}, false
}

func collectModules(tracked []string, reader *manifestBudgetReader, preferred []candidate) moduleCollection {
	moduleFiles := make([]string, 0, min(len(tracked), MaxModules))
	for _, filePath := range tracked {
		if path.Base(filePath) == "go.mod" && !strings.Contains(filePath, "/vendor/") &&
			!strings.HasPrefix(filePath, "vendor/") {
			moduleFiles = append(moduleFiles, filePath)
		}
	}
	moduleFiles = prioritizedModuleFiles(moduleFiles, preferred)
	result := moduleCollection{filesFound: len(moduleFiles)}
	if len(moduleFiles) > MaxManifestFiles {
		moduleFiles = moduleFiles[:MaxManifestFiles]
	}

	for _, filePath := range moduleFiles {
		content, _, err := reader.ReadFile(filePath, maxManifestFileBytes)
		if err != nil {
			continue
		}
		result.filesRead++
		result.bytesRead += len(content.Bytes)
		match := modulePattern.FindSubmatch(content.Bytes)
		if len(match) != 2 {
			continue
		}
		dir := path.Dir(filePath)
		if dir == "." {
			dir = "."
		}
		modulePath := string(match[1])
		result.modules = append(result.modules, Module{
			ID: OpaqueID("module", modulePath, dir), Path: modulePath, Dir: dir,
			SourcePath: filePath,
		})
	}
	sort.Slice(result.modules, func(i, j int) bool {
		if result.modules[i].Dir == "." {
			return true
		}
		if result.modules[j].Dir == "." {
			return false
		}
		if strings.Count(result.modules[i].Dir, "/") != strings.Count(result.modules[j].Dir, "/") {
			return strings.Count(result.modules[i].Dir, "/") < strings.Count(result.modules[j].Dir, "/")
		}
		return result.modules[i].Dir < result.modules[j].Dir
	})
	return result
}

func preferredModuleDirs(moduleFiles []string, preferred []candidate) map[string]struct{} {
	relevant := make(map[string]struct{})
	for _, item := range preferred {
		best := ""
		for _, moduleFile := range moduleFiles {
			directory := path.Dir(moduleFile)
			if directory != "." && !strings.HasPrefix(item.path, directory+"/") {
				continue
			}
			if best == "" || len(directory) > len(best) {
				best = directory
			}
		}
		if best != "" {
			relevant[best] = struct{}{}
		}
	}
	return relevant
}

func prioritizedModuleFiles(moduleFiles []string, preferred []candidate) []string {
	relevantDirs := preferredModuleDirs(moduleFiles, preferred)
	ranked := append([]string(nil), moduleFiles...)
	sort.Slice(ranked, func(i, j int) bool {
		leftAffinity := moduleTaskAffinity(ranked[i], preferred, relevantDirs)
		rightAffinity := moduleTaskAffinity(ranked[j], preferred, relevantDirs)
		if leftAffinity != rightAffinity {
			return leftAffinity > rightAffinity
		}
		left, right := path.Dir(ranked[i]), path.Dir(ranked[j])
		if strings.Count(left, "/") != strings.Count(right, "/") {
			return strings.Count(left, "/") < strings.Count(right, "/")
		}
		return left < right
	})
	result := make([]string, 0, len(ranked))
	chosen := make(map[string]struct{})
	add := func(filePath string) {
		if _, exists := chosen[filePath]; exists {
			return
		}
		chosen[filePath] = struct{}{}
		result = append(result, filePath)
	}
	for _, filePath := range ranked {
		if path.Dir(filePath) == "." {
			add(filePath)
			break
		}
	}
	selectedRelevant := make([]string, 0, 2)
	for _, filePath := range ranked {
		if _, exact := relevantDirs[path.Dir(filePath)]; !exact || path.Dir(filePath) == "." {
			continue
		}
		add(filePath)
		selectedRelevant = append(selectedRelevant, path.Dir(filePath))
		if len(selectedRelevant) == 2 {
			break
		}
	}
	for _, relevant := range selectedRelevant {
		for _, filePath := range ranked {
			directory := path.Dir(filePath)
			if directory != relevant && path.Dir(directory) == path.Dir(relevant) {
				add(filePath)
				break
			}
		}
		if len(result) >= MaxManifestFiles {
			break
		}
	}
	for _, filePath := range ranked {
		add(filePath)
	}
	return result
}

func moduleTaskAffinity(filePath string, preferred []candidate, relevantDirs map[string]struct{}) int {
	directory := path.Dir(filePath)
	if directory == "." {
		return 1_000
	}
	score := 0
	if _, exact := relevantDirs[directory]; exact {
		score += 500
	}
	for relevant := range relevantDirs {
		if path.Dir(relevant) == path.Dir(directory) {
			score += 100
		}
	}
	for _, item := range preferred {
		if directory == "." || strings.HasPrefix(item.path, directory+"/") {
			score += item.score * 2
		}
		if item.path == filePath {
			score += item.score * 2
		}
	}
	return score
}

// RepositoryStateSHA is the location-independent repository-state identity
// stored in Task Lens artifacts. Dirty content hashes and submodule state are
// preserved while checkout paths remain display-only local data.
func RepositoryStateSHA(state freshness.RepositoryState) (string, error) {
	if err := state.Validate(); err != nil {
		return "", err
	}
	canonical := struct {
		Version    int                        `json:"version"`
		Head       string                     `json:"head"`
		Dirty      []freshness.DirtyFile      `json:"dirty"`
		Submodules []freshness.SubmoduleState `json:"submodules,omitempty"`
	}{
		Version: state.Version, Head: state.Head,
		Dirty:      append([]freshness.DirtyFile{}, state.Dirty...),
		Submodules: state.Submodules,
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("task lens: encode repository state: %w", err)
	}
	return SHA256(raw), nil
}

func manifestIdentity(tracked []string, reader *manifestBudgetReader) (string, string) {
	for _, name := range []string{"pyproject.toml", "package.json", "Cargo.toml"} {
		if _, found := slices.BinarySearch(tracked, name); !found {
			continue
		}
		content, _, err := reader.ReadFile(name, maxManifestFileBytes)
		if err != nil {
			continue
		}
		if match := manifestNamePattern.FindSubmatch(content.Bytes); len(match) == 2 {
			return string(match[1]), name
		}
	}
	return "", ""
}

func normalizeRemote(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil || parsed.Hostname() == "" {
			return ""
		}
		cleanPath := strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/")
		if cleanPath == "" {
			return ""
		}
		return parsed.Hostname() + "/" + cleanPath
	}
	if userHost, remotePath, found := strings.Cut(value, ":"); found && strings.Contains(userHost, "@") {
		host := userHost[strings.LastIndex(userHost, "@")+1:]
		remotePath = strings.Trim(strings.TrimSuffix(remotePath, ".git"), "/")
		if host != "" && remotePath != "" {
			return host + "/" + remotePath
		}
	}
	return ""
}

func extractTerms(task string) []Term {
	weights := make(map[string]int)
	display := make(map[string]string)
	occurrences := make(map[string]int)
	firstSeen := make(map[string]int)
	sequence := 0
	record := func(raw string, weight int) {
		normalized := addTerm(weights, display, raw, weight)
		if normalized == "" {
			return
		}
		occurrences[normalized]++
		if _, exists := firstSeen[normalized]; !exists {
			firstSeen[normalized] = sequence
			sequence++
		}
	}
	for _, match := range backtickPattern.FindAllStringSubmatch(task, -1) {
		record(match[1], 16)
		for _, part := range splitIdentifier(match[1]) {
			record(part, 8)
		}
	}
	for _, raw := range wordPattern.FindAllString(task, -1) {
		clean := strings.Trim(strings.TrimSpace(raw), "`'\".,;:()[]{}<>")
		weight := 2
		if hasUpperTransition(clean) || strings.ContainsAny(clean, "_./") {
			weight = 10
		} else if startsLikeExportedIdentifier(clean) {
			// Plain sentence capitalization is weak evidence of an exported Go
			// identifier. Keep it searchable, but do not let words such as
			// "The" or "Create" outrank repeated task-domain vocabulary.
			weight = 4
		}
		record(clean, weight)
		for _, part := range splitIdentifier(clean) {
			partWeight := 3
			// A punctuation-delimited fragment is often the searchable source
			// identifier hidden inside a stack-frame or type-expression spelling
			// (for example, a receiver-qualified method or a generic argument).
			// Keep it below an exact backtick term, but above ordinary prose so
			// syntactic punctuation cannot push every useful grep query out of the
			// bounded plan.
			if strings.ContainsAny(clean, "_./:[](){}<>*-") &&
				(len([]rune(part)) >= 5 || hasUpperTransition(part) || startsLikeExportedIdentifier(part)) {
				partWeight = 8
			}
			record(part, partWeight)
		}
	}
	for normalized, count := range occurrences {
		if count <= 1 || weights[normalized] >= 8 {
			continue
		}
		// Repetition is a repository-independent signal that a concept names
		// the task's mechanism rather than incidental prose. Cap the boost so
		// repeated ordinary words cannot displace explicit code-like terms.
		increment := 2
		if weights[normalized] >= 4 {
			// Repeated exported-style vocabulary is much less likely to be
			// accidental sentence capitalization (for example, Enabled named
			// twice in a configuration task).
			increment = 4
		}
		weights[normalized] = min(8, weights[normalized]+(count-1)*increment)
	}
	type weightedTerm struct {
		normalized  string
		text        string
		weight      int
		occurrences int
		firstSeen   int
	}
	values := make([]weightedTerm, 0, len(weights))
	for normalized, weight := range weights {
		values = append(values, weightedTerm{
			normalized:  normalized,
			text:        display[normalized],
			weight:      weight,
			occurrences: occurrences[normalized],
			firstSeen:   firstSeen[normalized],
		})
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].weight != values[j].weight {
			return values[i].weight > values[j].weight
		}
		if values[i].occurrences != values[j].occurrences {
			return values[i].occurrences > values[j].occurrences
		}
		if values[i].firstSeen != values[j].firstSeen {
			return values[i].firstSeen < values[j].firstSeen
		}
		return values[i].normalized < values[j].normalized
	})
	if len(values) > maxTerms {
		values = values[:maxTerms]
	}
	result := make([]Term, 0, len(values))
	for _, value := range values {
		result = append(result, Term{
			ID: OpaqueID("term", value.normalized), Text: value.text,
			Normalized: value.normalized, Weight: value.weight,
		})
	}
	return result
}

func addTerm(weights map[string]int, display map[string]string, raw string, weight int) string {
	raw = strings.Trim(strings.TrimSpace(raw), "`'\".,;:()[]{}<>")
	normalized := strings.ToLower(raw)
	if len(normalized) < 3 || len(normalized) > 96 || strings.ContainsAny(normalized, "\r\n") {
		return ""
	}
	if _, stopped := taskStopWords[normalized]; stopped {
		return ""
	}
	if previous := weights[normalized]; weight > previous {
		weights[normalized] = weight
		display[normalized] = raw
	}
	return normalized
}

func splitIdentifier(value string) []string {
	chunks := strings.FieldsFunc(value, func(current rune) bool {
		return !unicode.IsLetter(current) && !unicode.IsDigit(current)
	})
	result := make([]string, 0, len(chunks))
	seen := make(map[string]struct{})
	add := func(part string) {
		if part == "" || part == value {
			return
		}
		normalized := strings.ToLower(part)
		if _, duplicate := seen[normalized]; duplicate {
			return
		}
		seen[normalized] = struct{}{}
		result = append(result, part)
	}
	for _, chunk := range chunks {
		add(chunk)
	}
	return result
}

func hasUpperTransition(value string) bool {
	runes := []rune(value)
	for index := 1; index < len(runes); index++ {
		if unicode.IsUpper(runes[index]) && unicode.IsLower(runes[index-1]) {
			return true
		}
	}
	return false
}

func startsLikeExportedIdentifier(value string) bool {
	runes := []rune(value)
	if len(runes) < 3 || !unicode.IsUpper(runes[0]) {
		return false
	}
	// Keep short proper-name identifiers such as Foo without promoting terse
	// all-caps vocabulary such as API into high-priority repository terms.
	for _, r := range runes[1:] {
		if unicode.IsLower(r) {
			return true
		}
	}
	return false
}

func collectCandidates(
	ctx context.Context,
	repo string,
	tracked []string,
	terms []Term,
	task string,
) ([]candidate, int, int, error) {
	profile := DeriveTaskProfile(task, classifyTaskKind(task))
	candidates := make(map[string]*candidate)
	trackedSet := make(map[string]struct{}, len(tracked))
	for _, filePath := range tracked {
		trackedSet[filePath] = struct{}{}
		if !isEvidenceFile(filePath) {
			continue
		}
		lowerPath := strings.ToLower(filePath)
		score := 0
		hits := make(map[string]struct{})
		for _, term := range terms {
			if strings.Contains(lowerPath, term.Normalized) {
				score += term.Weight + 5
				hits[term.ID] = struct{}{}
			}
		}
		if score > 0 {
			candidate := &candidate{
				path: filePath, score: score, termHits: hits, pathHit: true,
				isTest: isTestPath(filePath), isDocument: isDocumentPath(filePath),
			}
			candidates[filePath] = candidate
		}
	}

	queries := 0
	for _, term := range terms {
		if queries >= maxGrepTerms || !usableGrepTerm(term) {
			continue
		}
		matches, err := gitGrep(ctx, repo, term.Text)
		queries++
		if err != nil {
			return nil, 0, queries, err
		}
		rankGrepMatches(matches, term.Normalized)
		pathBonuses := grepCandidatePathBonuses(matches, term.Normalized)
		termPaths := make(map[string]struct{})
		for _, match := range matches {
			if _, ok := trackedSet[match.path]; !ok || !isEvidenceFile(match.path) {
				continue
			}
			if _, seen := termPaths[match.path]; !seen && len(termPaths) >= maxCandidatePathsPerTerm {
				continue
			}
			termPaths[match.path] = struct{}{}
			item := candidates[match.path]
			if item == nil {
				item = &candidate{
					path: match.path, termHits: make(map[string]struct{}),
					isTest: isTestPath(match.path), isDocument: isDocumentPath(match.path),
				}
				candidates[match.path] = item
			}
			if _, seen := item.termHits[term.ID]; !seen {
				item.score += term.Weight + 8 + pathBonuses[match.path]
				item.termHits[term.ID] = struct{}{}
			}
			if len(item.grepLines) < 12 && match.line > 0 {
				item.grepLines = append(item.grepLines, match.line)
			}
		}
	}

	kind := classifyTaskKind(task)
	for _, filePath := range tracked {
		if !isEvidenceFile(filePath) {
			continue
		}
		bonus := candidateRoleBonus(filePath, kind, task) + candidateProfilePathScore(filePath, profile)
		if bonus == 0 {
			continue
		}
		item := candidates[filePath]
		if item == nil {
			item = &candidate{
				path: filePath, termHits: make(map[string]struct{}),
				isTest: isTestPath(filePath), isDocument: isDocumentPath(filePath),
			}
			candidates[filePath] = item
		}
		item.score += bonus
	}

	result := make([]candidate, 0, len(candidates))
	for _, item := range candidates {
		sort.Ints(item.grepLines)
		item.grepLines = slices.Compact(item.grepLines)
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].score != result[j].score {
			return result[i].score > result[j].score
		}
		if len(result[i].termHits) != len(result[j].termHits) {
			return len(result[i].termHits) > len(result[j].termHits)
		}
		return result[i].path < result[j].path
	})
	found := len(result)
	if found > MaxInitialCandidates {
		result = boundInitialCandidates(result, profile)
	}
	return result, found, queries, nil
}

func boundInitialCandidates(candidates []candidate, profile TaskProfile) []candidate {
	if len(candidates) <= MaxInitialCandidates {
		return append([]candidate(nil), candidates...)
	}
	selected := make([]candidate, 0, MaxInitialCandidates)
	seen := make(map[string]struct{}, MaxInitialCandidates)
	add := func(item candidate) {
		if len(selected) >= MaxInitialCandidates {
			return
		}
		if _, duplicate := seen[item.path]; duplicate {
			return
		}
		seen[item.path] = struct{}{}
		selected = append(selected, item)
	}
	for bucket := 0; bucket < profileFileBucketCount(profile); bucket++ {
		for _, item := range candidates {
			if candidateMatchesProfileFileBucket(item, profile, bucket) {
				add(item)
				break
			}
		}
	}
	for _, item := range candidates {
		add(item)
	}
	return selected
}

func grepCandidatePathBonuses(matches []grepMatch, term string) map[string]int {
	counts := make(map[string]int)
	declarations := make(map[string]struct{})
	for _, match := range matches {
		counts[match.path]++
		if !sourceDeclarationLine(strings.TrimSpace(match.text)) {
			continue
		}
		if _, exactWord := identifierWordSet(match.text)[strings.ToLower(term)]; exactWord {
			declarations[match.path] = struct{}{}
		}
	}
	bonuses := make(map[string]int, len(counts))
	for filePath, count := range counts {
		bonuses[filePath] = min(count, 6) * 2
		if _, exactDeclaration := declarations[filePath]; exactDeclaration {
			bonuses[filePath] += 12
		}
	}
	return bonuses
}

type grepMatch struct {
	path string
	line int
	text string
}

func rankGrepMatches(matches []grepMatch, term string) {
	pathMatches := make(map[string]int)
	for _, match := range matches {
		pathMatches[match.path]++
	}
	sort.SliceStable(matches, func(i, j int) bool {
		left := grepMatchScore(matches[i], term, pathMatches[matches[i].path])
		right := grepMatchScore(matches[j], term, pathMatches[matches[j].path])
		if left != right {
			return left > right
		}
		if matches[i].path != matches[j].path {
			return matches[i].path < matches[j].path
		}
		return matches[i].line < matches[j].line
	})
}

func grepMatchScore(match grepMatch, term string, pathMatches int) int {
	score := grepPathScore(match.path, term) + min(pathMatches, 8)*3
	trimmed := strings.TrimSpace(match.text)
	if sourceDeclarationLine(trimmed) {
		if _, exactWord := identifierWordSet(trimmed)[strings.ToLower(term)]; exactWord {
			score += 24
		}
	}
	if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
		score -= 4
	}
	return score
}

func sourceDeclarationLine(line string) bool {
	return strings.HasPrefix(line, "func ") || strings.HasPrefix(line, "type ") ||
		strings.HasPrefix(line, "var ") || strings.HasPrefix(line, "const ")
}

func grepPathScore(filePath, term string) int {
	lower := strings.ToLower(filePath)
	score := 40 - strings.Count(filePath, "/")*4
	if strings.Contains(lower, term) {
		score += 40
	}
	if isTestPath(filePath) {
		score += 2
	}
	return score
}

func gitGrep(ctx context.Context, repo, term string) ([]grepMatch, error) {
	pathCommand := isolatedGitCommand(
		ctx, repo, "grep", "-l", "-I", "--ignore-case", "--full-name",
		"--fixed-strings", "-e", term, "--",
	)
	var pathOutput limitedBuffer
	pathOutput.limit = maxGrepOutputBytes
	var pathStderr bytes.Buffer
	pathCommand.Stdout = &pathOutput
	pathCommand.Stderr = &pathStderr
	err := pathCommand.Run()
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return nil, nil
	}
	if err != nil && !pathOutput.exceeded {
		return nil, fmt.Errorf("task lens: git grep paths: %w: %s", err, strings.TrimSpace(pathStderr.String()))
	}
	var paths []string
	for _, line := range splitLines(pathOutput.bytes) {
		filePath := filepath.ToSlash(strings.TrimSpace(line))
		if filePath != "" {
			paths = append(paths, filePath)
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		left := grepPathScore(paths[i], strings.ToLower(term))
		right := grepPathScore(paths[j], strings.ToLower(term))
		if left != right {
			return left > right
		}
		return paths[i] < paths[j]
	})
	if len(paths) > maxGrepSignalPaths {
		paths = paths[:maxGrepSignalPaths]
	}
	if len(paths) == 0 {
		return nil, nil
	}

	args := []string{
		"grep", "-n", "-I", "--ignore-case", "--max-count=4", "--full-name",
		"--fixed-strings", "-e", term, "--",
	}
	args = append(args, paths...)
	command := isolatedGitCommand(ctx, repo, args...)
	var stdout limitedBuffer
	stdout.limit = maxGrepOutputBytes
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return nil, nil
	}
	if err != nil && !stdout.exceeded {
		return nil, fmt.Errorf("task lens: git grep: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var result []grepMatch
	// ReadBytes has no token-size ceiling (unlike bufio.Scanner) — a git
	// grep hit inside a generated single-line file (statik-style payloads)
	// must not fail the whole stage.
	for _, line := range splitLines(stdout.bytes) {
		filePath, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		lineNumber, text, found := strings.Cut(rest, ":")
		if !found {
			continue
		}
		number, numberErr := strconv.Atoi(lineNumber)
		if numberErr != nil {
			continue
		}
		result = append(result, grepMatch{
			path: filepath.ToSlash(filePath), line: number, text: text,
		})
	}
	return result, nil
}

// splitLines splits arbitrary-length text on newlines without a scanner
// token ceiling. A trailing newline does not add a phantom empty line.
func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

type limitedBuffer struct {
	bytes    []byte
	limit    int
	exceeded bool
}

func (buffer *limitedBuffer) Write(data []byte) (int, error) {
	remaining := buffer.limit - len(buffer.bytes)
	if remaining > 0 {
		if len(data) < remaining {
			remaining = len(data)
		}
		buffer.bytes = append(buffer.bytes, data[:remaining]...)
	}
	if len(data) > remaining {
		buffer.exceeded = true
	}
	return len(data), nil
}

func selectFiles(candidates []candidate, terms []Term, kind TaskKind) []candidate {
	return selectFilesForProfile(candidates, terms, kind, TaskProfileUnknown)
}

func selectFilesForProfile(
	candidates []candidate,
	terms []Term,
	kind TaskKind,
	profile TaskProfile,
) []candidate {
	selected := make([]candidate, 0, MaxReadFiles)
	chosen := make(map[string]struct{})
	add := func(item candidate) {
		if len(selected) >= MaxReadFiles {
			return
		}
		if _, exists := chosen[item.path]; exists {
			return
		}
		selected = append(selected, item)
		chosen[item.path] = struct{}{}
	}
	termWeights := make(map[string]int, len(terms))
	covered := make(map[string]struct{}, len(terms))
	for _, term := range terms {
		termWeights[term.ID] = term.Weight
	}
	addBest := func(accept func(candidate) bool, limit int, preferAffinity bool) {
		for added := 0; added < limit && len(selected) < MaxReadFiles; added++ {
			bestIndex, bestPathTaskFit, bestProfileFit := -1, -1, -1
			bestCoverage, bestAffinity, bestScore := -1, -1, -1
			for index, item := range candidates {
				if _, exists := chosen[item.path]; exists || !accept(item) {
					continue
				}
				coverage := 0
				for termID := range item.termHits {
					if _, alreadyCovered := covered[termID]; !alreadyCovered {
						coverage += termWeights[termID]
					}
				}
				affinity := candidateSelectionAffinity(item, selected, kind)
				pathTaskFit := candidatePathTaskFit(item.path, terms)
				profileFit := candidateProfilePathScore(item.path, profile)
				selectionScore := item.score
				if !preferAffinity {
					// Once task-term coverage ties, prefer the shallower owning
					// subsystem over a coincidental nested adapter/example match.
					// Strong novel coverage still wins before this tie-breaker.
					selectionScore -= strings.Count(item.path, "/") * 24
				}
				better := false
				if preferAffinity {
					if profile == TaskProfileDataTagTransformation {
						// Inside a profile bucket, a strong task term in the file name is
						// better evidence of the requested data path than matching several
						// broad profile words. Novel task coverage then keeps the parser
						// beside the owning entry and the focused test beside both.
						better = pathTaskFit > bestPathTaskFit ||
							pathTaskFit == bestPathTaskFit && coverage > bestCoverage ||
							pathTaskFit == bestPathTaskFit && coverage == bestCoverage && affinity > bestAffinity ||
							pathTaskFit == bestPathTaskFit && coverage == bestCoverage && affinity == bestAffinity && profileFit > bestProfileFit ||
							pathTaskFit == bestPathTaskFit && coverage == bestCoverage && affinity == bestAffinity && profileFit == bestProfileFit && selectionScore > bestScore
					} else {
						better = profileFit > bestProfileFit ||
							profileFit == bestProfileFit && affinity > bestAffinity ||
							profileFit == bestProfileFit && affinity == bestAffinity && coverage > bestCoverage ||
							profileFit == bestProfileFit && affinity == bestAffinity && coverage == bestCoverage && selectionScore > bestScore
					}
				} else {
					better = coverage > bestCoverage ||
						coverage == bestCoverage && selectionScore > bestScore ||
						coverage == bestCoverage && selectionScore == bestScore && affinity > bestAffinity
				}
				if better {
					bestIndex, bestPathTaskFit, bestProfileFit = index, pathTaskFit, profileFit
					bestCoverage, bestAffinity, bestScore = coverage, affinity, selectionScore
				}
			}
			if bestIndex < 0 {
				break
			}
			add(candidates[bestIndex])
			for termID := range candidates[bestIndex].termHits {
				covered[termID] = struct{}{}
			}
		}
	}
	// Reserve one file for each evidence-derived profile bucket before broad
	// term coverage fills the bounded read set. This is path/category scoring,
	// not an episode-specific path list: it keeps the unsafe operation, mapper,
	// fixture, or operational source from being displaced by adjacent helpers.
	for bucket := 0; bucket < profileFileBucketCount(profile); bucket++ {
		currentBucket := bucket
		addBest(func(item candidate) bool {
			return candidateMatchesProfileFileBucket(item, profile, currentBucket)
		}, 1, true)
	}
	if kind == TaskExtension {
		// Named sibling/package paths carrying multiple strong task terms are
		// higher-signal than another root file containing ubiquitous words.
		addBest(func(item candidate) bool {
			return strings.EqualFold(path.Ext(item.path), ".go") && !item.isTest &&
				candidatePathTermWeight(item.path, terms) >= 16
		}, 2, false)
	}

	// Source declarations and executable verification examples are the most
	// compact way to answer a code task. Reserve both before large generated
	// snapshots can consume the byte budget. Coverage remains the first ranking
	// key, so a lower-scoring file carrying a distinct stack symbol or concept
	// wins over another near-duplicate integration file.
	productionSlots, testSlots := 3, 3
	if profile == TaskProfileOperationalRelease {
		productionSlots, testSlots = 0, 0
	}
	addBest(func(item candidate) bool {
		return strings.EqualFold(path.Ext(item.path), ".go") && !item.isTest
	}, productionSlots, false)
	addBest(func(item candidate) bool {
		return strings.HasSuffix(strings.ToLower(item.path), "_test.go")
	}, testSlots, true)
	if profile == TaskProfileOperationalRelease {
		for bucket := 0; bucket < profileFileBucketCount(profile); bucket++ {
			currentBucket := bucket
			addBest(func(item candidate) bool {
				return candidateMatchesProfileFileBucket(item, profile, currentBucket)
			}, 1, true)
		}
	}

	for len(selected) < 8 {
		bestIndex, bestCoverage, bestAffinity, bestScore := -1, 0, -1, -1
		for index, item := range candidates {
			if _, exists := chosen[item.path]; exists {
				continue
			}
			coverage := 0
			for termID := range item.termHits {
				if _, alreadyCovered := covered[termID]; !alreadyCovered {
					coverage += termWeights[termID]
				}
			}
			affinity := candidateSelectionAffinity(item, selected, kind)
			if coverage > bestCoverage ||
				(coverage == bestCoverage && affinity > bestAffinity) ||
				(coverage == bestCoverage && affinity == bestAffinity && item.score > bestScore) {
				bestIndex, bestCoverage, bestAffinity, bestScore = index, coverage, affinity, item.score
			}
		}
		if bestIndex < 0 || bestCoverage == 0 {
			break
		}
		add(candidates[bestIndex])
		for termID := range candidates[bestIndex].termHits {
			covered[termID] = struct{}{}
		}
	}
	if kind == TaskExtension || kind == TaskCompatibility {
		for _, item := range candidates {
			base := path.Base(item.path)
			if base == "go.work" || base == "go.mod" || strings.HasPrefix(item.path, "examples/") ||
				strings.HasPrefix(item.path, "example/") {
				add(item)
			}
			if len(selected) >= 7 {
				break
			}
		}
	}
	for _, item := range candidates {
		if !lowSignalGeneratedArtifact(item.path) {
			add(item)
		}
	}
	for _, item := range candidates {
		add(item)
	}
	return selected
}

func profileFileBucketCount(profile TaskProfile) int {
	switch profile {
	case TaskProfileDataTagTransformation:
		return 4
	case TaskProfileErrorStatusMapping,
		TaskProfileNilPanic,
		TaskProfileErrorNormalizationPrivacy:
		return 3
	case TaskProfileConfigurationPropagation:
		return 2
	case TaskProfileOperationalRelease,
		TaskProfileExtensionContribution:
		return 4
	default:
		return 0
	}
}

func candidateMatchesProfileFileBucket(
	item candidate,
	profile TaskProfile,
	bucket int,
) bool {
	lower := strings.ToLower(item.path)
	base := path.Base(lower)
	contains := func(parts ...string) bool {
		for _, part := range parts {
			if strings.Contains(lower, part) {
				return true
			}
		}
		return false
	}
	isProductionGo := strings.EqualFold(path.Ext(lower), ".go") && !item.isTest
	isTestGo := strings.HasSuffix(lower, "_test.go")
	isFixture := contains("testdata/", "golden", "snapshot", "generated") &&
		(path.Ext(lower) == ".json" || path.Ext(lower) == ".yaml" || path.Ext(lower) == ".yml")
	switch profile {
	case TaskProfileDataTagTransformation:
		switch bucket {
		case 0:
			return isProductionGo && contains("openapi", "schema", "parameter", "route")
		case 1:
			return isProductionGo && contains("parse", "option", "transform", "custom")
		case 2:
			return isTestGo && contains("openapi", "schema", "parameter", "option")
		case 3:
			return isFixture
		}
	case TaskProfileErrorStatusMapping:
		switch bucket {
		case 0:
			return isProductionGo && contains("serial", "negotiat", "response", "accept", "send")
		case 1:
			return isProductionGo && contains("error", "status")
		case 2:
			return isTestGo && contains("serial", "negotiat", "accept", "error", "status")
		}
	case TaskProfileNilPanic:
		switch bucket {
		case 0:
			return isProductionGo && contains("validat", "guard", "check")
		case 1:
			return isProductionGo && contains("body", "context", "ctx", "decode", "deserial")
		case 2:
			return isTestGo && contains("validat", "body", "context", "ctx", "decode", "deserial")
		}
	case TaskProfileConfigurationPropagation:
		switch bucket {
		case 0:
			return isProductionGo && contains("config", "option", "engine", "server", "setting")
		case 1:
			return isTestGo && contains("config", "option", "engine", "server", "setting")
		}
	case TaskProfileErrorNormalizationPrivacy:
		switch bucket {
		case 0:
			return isProductionGo && contains("error", "handler", "normaliz")
		case 1:
			return isProductionGo && contains("serial", "response", "public")
		case 2:
			return isTestGo && contains("error", "handler", "normaliz", "serial")
		}
	case TaskProfileOperationalRelease:
		switch bucket {
		case 0:
			return path.Ext(lower) == ".sh" || contains("scripts/")
		case 1:
			return base == "makefile" || base == "taskfile" || base == "taskfile.yml" || base == "taskfile.yaml"
		case 2:
			return item.isDocument && contains("contributing", "release", "maintain", "readme")
		case 3:
			return base == "go.work" || base == "go.mod"
		}
	case TaskProfileExtensionContribution:
		switch bucket {
		case 0:
			return isProductionGo && contains("interface", "port", "adapter", "adaptor")
		case 1:
			return isProductionGo && contains("extra/", "integration", "adapter", "adaptor")
		case 2:
			return contains("example", "examples/")
		case 3:
			return base == "go.work" || base == "go.mod" || base == "makefile"
		}
	}
	return false
}

func candidateProfilePathScore(filePath string, profile TaskProfile) int {
	item := candidate{
		path:       filePath,
		isTest:     isTestPath(filePath),
		isDocument: isDocumentPath(filePath),
	}
	score := 0
	for bucket := 0; bucket < profileFileBucketCount(profile); bucket++ {
		if candidateMatchesProfileFileBucket(item, profile, bucket) {
			score += 32
		}
	}
	lower := strings.ToLower(filePath)
	switch profile {
	case TaskProfileDataTagTransformation:
		if strings.Contains(lower, "parameter") || strings.Contains(lower, "schema") || strings.Contains(lower, "openapi") {
			score += 64
		}
	case TaskProfileErrorStatusMapping:
		if strings.Contains(lower, "serial") || strings.Contains(lower, "accept") || strings.Contains(lower, "negotiat") {
			score += 112
		} else if strings.Contains(lower, "response") || strings.Contains(lower, "error") || strings.Contains(lower, "status") {
			score += 48
		}
	case TaskProfileNilPanic:
		if strings.Contains(lower, "validat") {
			score += 112
		} else if strings.Contains(lower, "body") || strings.Contains(lower, "context") ||
			strings.Contains(lower, "ctx") || strings.Contains(lower, "deserial") {
			score += 72
		}
	case TaskProfileConfigurationPropagation:
		if strings.Contains(lower, "config") || strings.Contains(lower, "engine") {
			score += 96
		} else if strings.Contains(lower, "server") || strings.Contains(lower, "option") {
			score += 64
		}
	case TaskProfileErrorNormalizationPrivacy:
		if strings.Contains(lower, "error") || strings.Contains(lower, "normaliz") || strings.Contains(lower, "handler") {
			score += 96
		}
	}
	if profile == TaskProfileOperationalRelease {
		base := path.Base(lower)
		switch {
		case base == "go.work":
			score += 128
		case base == "go.mod" && path.Dir(lower) == ".":
			score += 80
		case base == "go.mod":
			score += max(0, 32-strings.Count(lower, "/")*8)
		case strings.Contains(base, "contributing") || strings.Contains(base, "release"):
			score += 96
		case base == "readme.md":
			score += 8
		}
	}
	return min(score, 192)
}

func candidatePathTermWeight(filePath string, terms []Term) int {
	lower := strings.ToLower(filePath)
	weight := 0
	for _, term := range terms {
		if term.Weight >= 8 && strings.Contains(lower, term.Normalized) {
			weight += term.Weight
		}
	}
	return weight
}

func candidatePathTaskFit(filePath string, terms []Term) int {
	weight := candidatePathTermWeight(filePath, terms)
	base := strings.TrimSuffix(
		strings.ToLower(path.Base(filePath)),
		strings.ToLower(path.Ext(filePath)),
	)
	for _, term := range terms {
		if term.Weight >= 8 && base == term.Normalized {
			weight += term.Weight * 2
		}
	}
	return weight
}

func candidateSelectionAffinity(item candidate, selected []candidate, kind TaskKind) int {
	affinity := 0
	lower := strings.ToLower(item.path)
	if strings.EqualFold(path.Ext(item.path), ".go") {
		affinity += 8
	}
	if item.pathHit {
		affinity += 16
	}
	if strings.HasSuffix(lower, "_test.go") {
		affinity += 4
		stem := strings.TrimSuffix(path.Base(lower), "_test.go")
		for _, existing := range selected {
			existingStem := strings.TrimSuffix(path.Base(strings.ToLower(existing.path)), ".go")
			if path.Dir(existing.path) == path.Dir(item.path) &&
				(existingStem == stem || strings.HasPrefix(stem, existingStem+"_")) {
				affinity += 12
				break
			}
		}
	}
	if item.isDocument && kind == TaskCompatibility {
		affinity += 10
	}
	if lowSignalGeneratedArtifact(item.path) {
		affinity -= 24
	}
	return affinity
}

func lowSignalGeneratedArtifact(filePath string) bool {
	lower := strings.ToLower(filePath)
	if path.Ext(lower) != ".json" && path.Ext(lower) != ".yaml" && path.Ext(lower) != ".yml" {
		return false
	}
	base := path.Base(lower)
	return strings.Contains(lower, "/testdata/") || strings.HasPrefix(lower, "testdata/") ||
		strings.Contains(base, "golden") || strings.Contains(base, "snapshot")
}

func readCandidateFiles(
	reader *manifestBudgetReader,
	candidates []candidate,
) ([]collectedFile, int, int, int, int, bool, bool, bool, error) {
	files := make([]collectedFile, 0, len(candidates))
	evidenceFilesRead := 0
	fileLimitBound := false
	byteLimitBound := false
	sourceScanLimitBound := false
	for _, item := range candidates {
		if _, cached := reader.cache[item.path]; !cached {
			if reader.files >= MaxReadFiles {
				fileLimitBound = true
				continue
			}
			if reader.bytes >= MaxReadBytes {
				byteLimitBound = true
				continue
			}
		}
		content, _, err := reader.ReadFile(item.path, maxCollectedFileBytes)
		if err != nil {
			continue
		}
		evidenceFilesRead++
		if reader.bytes == MaxReadBytes && content.Truncated {
			byteLimitBound = true
		}
		retained := content.Bytes
		truncated := content.Truncated
		truncationReason := ""
		totalLines := 0
		if content.Truncated && reader.sourceScanBytes < MaxSourceScanBytes {
			available := MaxSourceScanBytes - reader.sourceScanBytes
			window, scanErr := reader.reader.ReadLineWindows(item.path, reporead.WindowOptions{
				ScanBytes:   int64(available),
				RetainBytes: int64(available),
				Windows:     []reporead.LineWindow{{Start: 1, End: 1 << 30}},
			})
			if scanErr == nil {
				reader.sourceScanBytes += int(window.ScannedBytes)
				if window.ScanTruncated || window.RetainTruncated {
					sourceScanLimitBound = true
				}
			}
			if scanErr == nil && len(window.Lines) > 0 {
				lines := make([]string, 0, len(window.Lines))
				for _, line := range window.Lines {
					lines = append(lines, line.Text)
				}
				retained = []byte(strings.Join(lines, "\n"))
				truncated = window.ScanTruncated || window.RetainTruncated
				if window.SourceTotalLinesExact {
					totalLines = window.SourceTotalLines
				}
				if truncated {
					truncationReason = "bounded_source_scan_limit"
				}
			}
		}
		if content.Truncated && reader.sourceScanBytes >= MaxSourceScanBytes && truncated {
			sourceScanLimitBound = true
			if truncationReason == "" {
				truncationReason = "bounded_source_scan_limit"
			}
		}
		if truncated && totalLines == 0 && bytes.Equal(retained, content.Bytes) {
			lastNewline := bytes.LastIndexByte(retained, '\n')
			if lastNewline < 0 {
				continue
			}
			retained = retained[:lastNewline]
			if truncationReason == "" {
				truncationReason = "file_read_byte_limit"
			}
		}
		if !utf8.Valid(retained) || bytes.IndexByte(retained, 0) >= 0 {
			continue
		}
		lines := splitSourceLines(retained)
		if !truncated && totalLines == 0 {
			totalLines = len(lines)
		}
		if truncated {
			totalLines = 0
		}
		files = append(files, collectedFile{
			candidate: item, content: retained, lines: lines, truncated: truncated,
			truncationReason: truncationReason,
			sourceTotalLines: totalLines,
		})
	}
	return files, evidenceFilesRead, reader.files, reader.bytes, reader.sourceScanBytes,
		fileLimitBound, byteLimitBound, sourceScanLimitBound, nil
}

// splitSourceLines uses physical one-based source lines. strings.Split would
// otherwise invent an extra empty line after a terminal newline, allowing an
// EOF excerpt to claim a line that tools such as git and editors do not expose.
func splitSourceLines(raw []byte) []string {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if text == "" {
		return nil
	}
	text = strings.TrimSuffix(text, "\n")
	return strings.Split(text, "\n")
}

func extractAnchors(
	files []collectedFile,
	terms []Term,
	kind TaskKind,
	taskText string,
) ([]anchorCandidate, int) {
	result := make([]anchorCandidate, 0)
	var goLedger []goAnchorLedgerItem
	astParses := 0
	for _, file := range files {
		var extracted []anchorCandidate
		if strings.HasSuffix(strings.ToLower(file.candidate.path), ".go") && !file.truncated {
			items := goAnchorLedger(file, terms)
			goLedger = append(goLedger, items...)
			for _, item := range items {
				if item.taskScore > 0 {
					extracted = append(extracted, item.candidate)
				}
			}
			astParses++
		}
		if len(extracted) == 0 && file.candidate.isDocument {
			extracted = documentAnchors(file, terms)
		}
		if len(extracted) == 0 {
			extracted = lineAnchors(file, terms)
		}
		if fallback, ok := profileRoleOnlyFileAnchor(file, kind, taskText, terms); ok {
			hasCompleteFile := false
			for _, existing := range extracted {
				if existing.anchor.Scope.ScopeKind == SourceScopeCompleteFile {
					hasCompleteFile = true
					break
				}
			}
			if !hasCompleteFile {
				extracted = append(extracted, fallback)
			}
		}
		// Assign the immutable v0.1 roles before any anchor ranking. Retaining
		// the complete bounded candidate set prevents a same-file helper cap
		// from deleting the only candidate for a missing key role.
		for index := range extracted {
			extracted[index].anchor.RoleHints = deterministicRoleHints(
				extracted[index].anchor,
				kind,
				taskText,
			)
		}
		result = append(result, extracted...)
	}
	seen := make(map[string]struct{}, len(result))
	for _, item := range result {
		seen[item.anchor.ID] = struct{}{}
	}
	for _, linked := range linkedZeroTaskGoAnchors(goLedger) {
		if _, exists := seen[linked.anchor.ID]; exists {
			continue
		}
		linked.anchor.RoleHints = deterministicRoleHints(linked.anchor, kind, taskText)
		result = append(result, linked)
		seen[linked.anchor.ID] = struct{}{}
	}
	// Keep a tiny deterministic filler reserve for reducer/replay auditing.
	// Exact linked zero-score targets above always enter first; these fillers
	// cannot increase the two-expansion completion budget and can be replaced
	// when a key role or stronger relation is missing.
	fillers := make([]anchorCandidate, 0, MaxFrontierExpansions)
	for _, item := range goLedger {
		if item.taskScore != 0 {
			continue
		}
		if _, exists := seen[item.candidate.anchor.ID]; exists {
			continue
		}
		fillers = append(fillers, item.candidate)
	}
	sort.Slice(fillers, func(left, right int) bool {
		if fillers[left].score != fillers[right].score {
			return fillers[left].score > fillers[right].score
		}
		if fillers[left].anchor.Path != fillers[right].anchor.Path {
			return fillers[left].anchor.Path < fillers[right].anchor.Path
		}
		return fillers[left].anchor.StartLine < fillers[right].anchor.StartLine
	})
	if len(fillers) > MaxFrontierExpansions {
		fillers = fillers[:MaxFrontierExpansions]
	}
	for _, filler := range fillers {
		filler.anchor.RoleHints = deterministicRoleHints(filler.anchor, kind, taskText)
		result = append(result, filler)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].score != result[j].score {
			return result[i].score > result[j].score
		}
		if result[i].anchor.Path != result[j].anchor.Path {
			return result[i].anchor.Path < result[j].anchor.Path
		}
		return result[i].anchor.StartLine < result[j].anchor.StartLine
	})
	return result, astParses
}

func profileRoleOnlyFileAnchor(
	file collectedFile,
	kind TaskKind,
	taskText string,
	terms []Term,
) (anchorCandidate, bool) {
	profile := DeriveTaskProfile(taskText, kind)
	if profile != TaskProfileOperationalRelease && profile != TaskProfileExtensionContribution {
		return anchorCandidate{}, false
	}
	if file.truncated || len(file.lines) == 0 || !isFileScopeAnchorPath(file.candidate.path) ||
		sourceRangeBytes(file.lines, 1, len(file.lines)) > maxCompleteFileBytes {
		return anchorCandidate{}, false
	}
	matchedBucket := false
	for bucket := 0; bucket < profileFileBucketCount(profile); bucket++ {
		if candidateMatchesProfileFileBucket(file.candidate, profile, bucket) {
			matchedBucket = true
			break
		}
	}
	if !matchedBucket {
		return anchorCandidate{}, false
	}
	symbol := path.Base(file.candidate.path)
	score, hits := scoreLines(file.lines, 1, len(file.lines), terms)
	anchor := makeAnchor(
		file.candidate.path,
		"",
		symbol,
		"",
		1,
		len(file.lines),
		file.lines,
		roleHints(symbol, file.candidate.path),
		max(1, score+file.candidate.score/3),
	)
	anchor.Scope = sourceScopeForAnchor(
		file,
		1,
		len(file.lines),
		1,
		len(file.lines),
		true,
		SourceScopeCompleteFile,
		terms,
	)
	anchor.RoleHints = deterministicRoleHints(anchor, kind, taskText)
	return anchorCandidate{anchor: anchor, score: anchor.Score, terms: hits}, true
}

func selectFileAnchors(candidates []anchorCandidate, terms []Term) []anchorCandidate {
	if len(candidates) <= maxAnchorsPerFile {
		return append([]anchorCandidate(nil), candidates...)
	}
	chosen := make(map[int]struct{}, maxAnchorsPerFile)
	covered := make(map[string]struct{}, len(terms))
	termWeights := make(map[string]int, len(terms))
	promotedScore := candidates[0].score + 1
	add := func(index int, promote bool) {
		if len(chosen) >= maxAnchorsPerFile {
			return
		}
		if _, exists := chosen[index]; exists {
			return
		}
		if promote && candidates[index].score < promotedScore {
			candidates[index].score = promotedScore
			candidates[index].anchor.Score = promotedScore
		}
		chosen[index] = struct{}{}
		for _, termID := range candidates[index].terms {
			covered[termID] = struct{}{}
		}
	}
	for _, term := range terms {
		termWeights[term.ID] = term.Weight
		if term.Weight < 10 {
			continue
		}
		for index, item := range candidates {
			symbol := strings.TrimPrefix(item.anchor.Symbol, "*")
			if strings.EqualFold(symbol, term.Text) ||
				strings.HasSuffix(strings.ToLower(symbol), "."+term.Normalized) {
				add(index, false)
				break
			}
		}
	}
	termFrequency := make(map[string]int, len(terms))
	for _, item := range candidates {
		for _, termID := range item.terms {
			termFrequency[termID]++
		}
	}
	// Raw anchor scores are easily dominated by a repository-wide task word
	// (for example, a framework or protocol name). Reserve a few declarations
	// whose task terms are comparatively rare within this file before taking
	// the raw leaders. This favors the local mechanism while remaining wholly
	// task- and repository-derived.
	for range 3 {
		bestIndex, bestDiscrimination, bestScore := -1, 0, -1
		for index, item := range candidates {
			if _, exists := chosen[index]; exists {
				continue
			}
			discrimination := 0
			for _, termID := range item.terms {
				weight := termWeights[termID]
				frequency := termFrequency[termID]
				if weight < 3 || frequency == 0 {
					continue
				}
				discrimination += weight * 100 / frequency
			}
			if discrimination > bestDiscrimination ||
				discrimination == bestDiscrimination && discrimination > 0 && item.score > bestScore {
				bestIndex, bestDiscrimination, bestScore = index, discrimination, item.score
			}
		}
		if bestIndex < 0 || bestDiscrimination == 0 {
			break
		}
		add(bestIndex, true)
	}
	for index := 0; index < len(candidates) && index < 2; index++ {
		add(index, false)
	}

	// Retain declarations that explain distinct task vocabulary instead of
	// letting one ubiquitous repository term fill every slot in a large file.
	for len(chosen) < 6 {
		bestIndex, bestCoverage, bestScore := -1, 0, -1
		for index, item := range candidates {
			if _, exists := chosen[index]; exists {
				continue
			}
			coverage := 0
			for _, termID := range item.terms {
				if _, exists := covered[termID]; !exists {
					coverage += termWeights[termID]
				}
			}
			if coverage > bestCoverage || coverage == bestCoverage && coverage > 0 && item.score > bestScore {
				bestIndex, bestCoverage, bestScore = index, coverage, item.score
			}
		}
		if bestIndex < 0 || bestCoverage == 0 {
			break
		}
		add(bestIndex, true)
	}

	// The full declaration set is already in memory. Two bounded lexical
	// neighbor rounds keep locally referenced helpers without another file read,
	// symbol query, or repository-wide graph. This is selection inside the
	// retained file, not an expansion of the retrieval frontier budget.
	for range maxFileLexicalNeighbors {
		bestSource, bestSourceRelevance := -1, -1
		for sourceIndex := range chosen {
			hasTarget := false
			for targetIndex, target := range candidates {
				if _, exists := chosen[targetIndex]; exists {
					continue
				}
				targetSymbol := baseSymbol(target.anchor.Symbol)
				if len(targetSymbol) >= 4 && !genericGrepTerm(strings.ToLower(targetSymbol)) &&
					anchorContainsExact(candidates[sourceIndex].anchor, strings.ToLower(targetSymbol)) {
					hasTarget = true
					break
				}
			}
			if !hasTarget {
				continue
			}
			relevance := 0
			for _, termID := range candidates[sourceIndex].terms {
				relevance += termWeights[termID]
			}
			if relevance > bestSourceRelevance || relevance == bestSourceRelevance &&
				(bestSource < 0 || sourceIndex < bestSource) {
				bestSource, bestSourceRelevance = sourceIndex, relevance
			}
		}
		bestIndex, bestCoverage, bestScore := -1, -1, -1
		if bestSource >= 0 {
			for index, target := range candidates {
				if _, exists := chosen[index]; exists {
					continue
				}
				targetSymbol := baseSymbol(target.anchor.Symbol)
				if len(targetSymbol) < 4 || genericGrepTerm(strings.ToLower(targetSymbol)) ||
					!anchorContainsExact(candidates[bestSource].anchor, strings.ToLower(targetSymbol)) {
					continue
				}
				coverage := 0
				for _, termID := range target.terms {
					if _, exists := covered[termID]; !exists {
						coverage += termWeights[termID]
					}
				}
				if coverage > bestCoverage || coverage == bestCoverage && target.score > bestScore {
					bestIndex, bestCoverage, bestScore = index, coverage, target.score
				}
			}
		}
		if bestIndex < 0 {
			break
		}
		add(bestIndex, true)
	}
	for index := range candidates {
		add(index, false)
		if len(chosen) >= maxAnchorsPerFile {
			break
		}
	}
	selected := make([]anchorCandidate, 0, len(chosen))
	for index, item := range candidates {
		if _, exists := chosen[index]; exists {
			selected = append(selected, item)
		}
	}
	return selected
}

func goAnchors(file collectedFile, terms []Term) []anchorCandidate {
	ledger := goAnchorLedger(file, terms)
	result := make([]anchorCandidate, 0, len(ledger))
	for _, item := range ledger {
		if item.taskScore > 0 {
			result = append(result, item.candidate)
		}
	}
	result = append(result, linkedZeroTaskGoAnchors(ledger)...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].score != result[j].score {
			return result[i].score > result[j].score
		}
		return result[i].anchor.StartLine < result[j].anchor.StartLine
	})
	return result
}

func goAnchorLedger(file collectedFile, terms []Term) []goAnchorLedgerItem {
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, file.candidate.path, file.content, parser.ParseComments)
	if err != nil {
		return nil
	}
	type declaration struct {
		name       string
		start, end int
		roles      []AnchorRole
		function   *ast.FuncDecl
	}
	var declarations []declaration
	for _, item := range parsed.Decls {
		switch value := item.(type) {
		case *ast.FuncDecl:
			name := value.Name.Name
			if value.Recv != nil && len(value.Recv.List) > 0 {
				name = receiverName(value.Recv.List[0].Type) + "." + name
			}
			declarations = append(declarations, declaration{
				name: name, start: fileSet.Position(value.Pos()).Line, end: fileSet.Position(value.End()).Line,
				roles: roleHints(name, file.candidate.path), function: value,
			})
		case *ast.GenDecl:
			for _, spec := range value.Specs {
				name := ""
				switch typed := spec.(type) {
				case *ast.TypeSpec:
					name = typed.Name.Name
				case *ast.ValueSpec:
					if len(typed.Names) > 0 {
						name = typed.Names[0].Name
					}
				}
				if name == "" {
					continue
				}
				declarations = append(declarations, declaration{
					name: name, start: fileSet.Position(spec.Pos()).Line, end: fileSet.Position(spec.End()).Line,
					roles: roleHints(name, file.candidate.path),
				})
			}
		}
	}
	var result []goAnchorLedgerItem
	for _, declaration := range declarations {
		start, end, complete := completeOrBoundedRange(
			declaration.start,
			declaration.end,
			file.lines,
			terms,
			maxCompleteSymbolBytes,
		)
		anchor := Anchor{}
		score, hits := 0, []string{}
		if complete || declaration.function == nil {
			score, hits = scoreLines(file.lines, start, end, terms)
			anchor = makeAnchor(
				file.candidate.path,
				parsed.Name.Name,
				declaration.name,
				"",
				start,
				end,
				file.lines,
				declaration.roles,
				score,
			)
			anchor.Scope = sourceScopeForAnchor(
				file,
				start,
				end,
				declaration.start,
				declaration.end,
				complete,
				SourceScopeCompleteEnclosingSymbol,
				terms,
			)
		} else {
			anchor, score, hits = goFunctionFragmentAnchor(
				file,
				fileSet,
				parsed.Name.Name,
				declaration.name,
				declaration.roles,
				declaration.function,
				terms,
			)
		}
		hitSet := make(map[string]struct{}, len(hits))
		for _, hit := range hits {
			hitSet[hit] = struct{}{}
		}
		for _, term := range terms {
			if strings.EqualFold(term.Text, declaration.name) || strings.Contains(strings.ToLower(declaration.name), term.Normalized) {
				score += term.Weight + 18
				if _, exists := hitSet[term.ID]; !exists {
					hits = append(hits, term.ID)
					hitSet[term.ID] = struct{}{}
				}
			}
		}
		taskScore := score
		sort.Strings(hits)
		score += file.candidate.score / 3
		anchor.Score = score
		result = append(result, goAnchorLedgerItem{
			candidate: anchorCandidate{anchor: anchor, score: score, terms: hits},
			taskScore: taskScore,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].candidate.score != result[j].candidate.score {
			return result[i].candidate.score > result[j].candidate.score
		}
		return result[i].candidate.anchor.StartLine < result[j].candidate.anchor.StartLine
	})
	return result
}

type sourceLineRange struct {
	start int
	end   int
}

type fragmentLinePlan struct {
	priorities map[int]int
	taskLines  map[int]struct{}
}

func newFragmentLinePlan() fragmentLinePlan {
	return fragmentLinePlan{
		priorities: make(map[int]int),
		taskLines:  make(map[int]struct{}),
	}
}

func (plan fragmentLinePlan) addLine(line, priority, lineCount int) {
	if line < 1 || line > lineCount {
		return
	}
	if previous, exists := plan.priorities[line]; !exists || priority < previous {
		plan.priorities[line] = priority
	}
}

func (plan fragmentLinePlan) addRange(start, end, priority, lineCount int) {
	start, end = clampLines(start, end, lineCount)
	for line := start; line <= end; line++ {
		plan.addLine(line, priority, lineCount)
	}
}

func (plan fragmentLinePlan) addTaskLine(line, lineCount int) {
	plan.addLine(line, 0, lineCount)
	if line >= 1 && line <= lineCount {
		plan.taskLines[line] = struct{}{}
	}
}

func (plan fragmentLinePlan) selectLines(lines []string, maxBytes int) ([]int, bool, bool) {
	type rankedLine struct {
		line     int
		priority int
	}
	ranked := make([]rankedLine, 0, len(plan.priorities))
	for line, priority := range plan.priorities {
		ranked = append(ranked, rankedLine{line: line, priority: priority})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].priority != ranked[j].priority {
			return ranked[i].priority < ranked[j].priority
		}
		return ranked[i].line < ranked[j].line
	})

	selected := make(map[int]struct{}, min(len(ranked), maxFragmentExcerptLines))
	retainedBytes := 0
	clipped := false
	for _, item := range ranked {
		size := len(lines[item.line-1]) + 1
		if len(selected) >= maxFragmentExcerptLines || size > maxBytes-retainedBytes {
			clipped = true
			continue
		}
		selected[item.line] = struct{}{}
		retainedBytes += size
	}

	allTaskLines := true
	for line := range plan.taskLines {
		if _, retained := selected[line]; !retained {
			allTaskLines = false
			break
		}
	}
	result := make([]int, 0, len(selected))
	for line := range selected {
		result = append(result, line)
	}
	sort.Ints(result)
	return result, clipped, allTaskLines
}

func goFunctionFragmentAnchor(
	file collectedFile,
	fileSet *token.FileSet,
	packageName string,
	symbol string,
	roles []AnchorRole,
	function *ast.FuncDecl,
	terms []Term,
) (Anchor, int, []string) {
	start := fileSet.Position(function.Pos()).Line
	end := fileSet.Position(function.End()).Line
	plan := newFragmentLinePlan()
	matchingLines := taskMatchingLines(file.lines, start, end, terms)
	for _, line := range matchingLines {
		plan.addTaskLine(line, len(file.lines))
		plan.addRange(
			line-fragmentContextLines,
			line+fragmentContextLines,
			3,
			len(file.lines),
		)
	}

	if function.Body != nil {
		bodyStart := fileSet.Position(function.Body.Lbrace).Line
		bodyEnd := fileSet.Position(function.Body.Rbrace).Line
		plan.addRange(start, bodyStart, 2, len(file.lines))
		plan.addLine(bodyEnd, 2, len(file.lines))
		addGoStatementNeighborhoods(plan, fileSet, function.Body, matchingLines, len(file.lines))
	} else {
		plan.addRange(start, end, 2, len(file.lines))
	}

	selected, clipped, allTaskLines := plan.selectLines(file.lines, maxCompleteSymbolBytes)
	anchor := makeFragmentAnchor(
		file,
		packageName,
		symbol,
		"",
		roles,
		selected,
		terms,
		"oversized_symbol_fragment_retention",
		clipped,
		allTaskLines,
	)
	score, hits := scoreSourceLines(anchor.Excerpt, terms)
	return anchor, score, hits
}

func addGoStatementNeighborhoods(
	plan fragmentLinePlan,
	fileSet *token.FileSet,
	body *ast.BlockStmt,
	matchingLines []int,
	lineCount int,
) {
	for _, matchLine := range matchingLines {
		var smallest ast.Stmt
		smallestSpan := int(^uint(0) >> 1)
		ast.Inspect(body, func(node ast.Node) bool {
			if node == nil {
				return true
			}
			nodeStart := fileSet.Position(node.Pos()).Line
			nodeEnd := fileSet.Position(node.End()).Line
			if matchLine < nodeStart || matchLine > nodeEnd {
				return true
			}
			if statement, ok := node.(ast.Stmt); ok {
				if _, block := statement.(*ast.BlockStmt); !block && nodeEnd-nodeStart < smallestSpan {
					smallest = statement
					smallestSpan = nodeEnd - nodeStart
				}
			}
			if isGoBranchNode(node) {
				plan.addLine(nodeStart, 2, lineCount)
			}
			return true
		})
		if smallest == nil {
			continue
		}
		statementStart := fileSet.Position(smallest.Pos()).Line
		statementEnd := fileSet.Position(smallest.End()).Line
		plan.addRange(statementStart, statementEnd, 1, lineCount)
		plan.addRange(
			statementStart-fragmentContextLines,
			statementEnd+fragmentContextLines,
			3,
			lineCount,
		)
	}
}

func isGoBranchNode(node ast.Node) bool {
	switch node.(type) {
	case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt,
		*ast.TypeSwitchStmt, *ast.SelectStmt, *ast.CaseClause, *ast.CommClause:
		return true
	default:
		return false
	}
}

func taskMatchingLines(lines []string, start, end int, terms []Term) []int {
	start, end = clampLines(start, end, len(lines))
	result := make([]int, 0)
	for line := start; line <= end; line++ {
		score, _ := scoreText(lines[line-1], terms)
		if score > 0 {
			result = append(result, line)
		}
	}
	return result
}

// linkedZeroTaskGoAnchors retains exact package-level references from
// task-scored declarations to otherwise unscored declarations in the already
// parsed file set. The admission cap is independent from the two completion
// expansions: it bounds the candidate ledger while leaving completion free to
// choose the missing role or strongest exact relation.
func linkedZeroTaskGoAnchors(ledger []goAnchorLedgerItem) []anchorCandidate {
	type linkedTarget struct {
		candidate   anchorCandidate
		directCall  bool
		sourceScore int
	}

	linked := make(map[string]linkedTarget)
	for _, source := range ledger {
		if source.taskScore == 0 {
			continue
		}
		references := exactUnqualifiedGoReferences(source.candidate.anchor)
		if len(references) == 0 {
			continue
		}
		for _, target := range ledger {
			if target.taskScore != 0 || strings.Contains(target.candidate.anchor.Symbol, ".") ||
				!sameGoPackage(source.candidate.anchor, target.candidate.anchor) {
				continue
			}
			name := baseSymbol(target.candidate.anchor.Symbol)
			reference, exists := references[name]
			if !exists {
				continue
			}
			current, exists := linked[target.candidate.anchor.ID]
			if exists && (current.directCall && !reference.directCall ||
				current.directCall == reference.directCall && current.sourceScore >= source.taskScore) {
				continue
			}
			linked[target.candidate.anchor.ID] = linkedTarget{
				candidate:   target.candidate,
				directCall:  reference.directCall,
				sourceScore: source.taskScore,
			}
		}
	}

	ranked := make([]linkedTarget, 0, len(linked))
	for _, target := range linked {
		ranked = append(ranked, target)
	}
	sort.Slice(ranked, func(left, right int) bool {
		if ranked[left].directCall != ranked[right].directCall {
			return ranked[left].directCall
		}
		if ranked[left].sourceScore != ranked[right].sourceScore {
			return ranked[left].sourceScore > ranked[right].sourceScore
		}
		if ranked[left].candidate.anchor.Path != ranked[right].candidate.anchor.Path {
			return ranked[left].candidate.anchor.Path < ranked[right].candidate.anchor.Path
		}
		return ranked[left].candidate.anchor.StartLine < ranked[right].candidate.anchor.StartLine
	})
	if len(ranked) > maxASTLinkedZeroTargets {
		ranked = ranked[:maxASTLinkedZeroTargets]
	}
	result := make([]anchorCandidate, 0, len(ranked))
	for _, target := range ranked {
		result = append(result, target.candidate)
	}
	return result
}

func exactUnqualifiedGoReferences(anchor Anchor) map[string]goAnchorReference {
	parsed, err := parseGoAnchor(anchor)
	if err != nil {
		return nil
	}
	selectorIdentifiers := make(map[*ast.Ident]struct{})
	directCalls := make(map[*ast.Ident]struct{})
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.SelectorExpr:
			selectorIdentifiers[value.Sel] = struct{}{}
			if qualifier, ok := value.X.(*ast.Ident); ok {
				selectorIdentifiers[qualifier] = struct{}{}
			}
		case *ast.CallExpr:
			if function, ok := value.Fun.(*ast.Ident); ok {
				directCalls[function] = struct{}{}
			}
		}
		return true
	})
	references := make(map[string]goAnchorReference)
	ast.Inspect(parsed, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || identifier.Obj != nil || identifier == parsed.Name {
			return true
		}
		if _, selectorPart := selectorIdentifiers[identifier]; selectorPart {
			return true
		}
		_, directCall := directCalls[identifier]
		current := references[identifier.Name]
		current.name = identifier.Name
		current.directCall = current.directCall || directCall
		references[identifier.Name] = current
		return true
	})
	return references
}

func receiverName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return receiverName(value.X)
	case *ast.IndexExpr:
		return receiverName(value.X)
	case *ast.IndexListExpr:
		return receiverName(value.X)
	default:
		return "receiver"
	}
}

func boundedDeclarationLines(start, end int, lines []string, terms []Term) (int, int) {
	if end-start+1 <= maxExcerptLines {
		return clampLines(start, end, len(lines))
	}
	best := start
	bestScore := -1
	for line := start; line <= end && line <= len(lines); line++ {
		score, _ := scoreText(lines[line-1], terms)
		if score > bestScore {
			best, bestScore = line, score
		}
	}
	windowStart := best - maxExcerptLines/2
	if windowStart < start {
		windowStart = start
	}
	windowEnd := windowStart + maxExcerptLines - 1
	if windowEnd > end {
		windowEnd = end
		windowStart = end - maxExcerptLines + 1
	}
	return clampLines(windowStart, windowEnd, len(lines))
}

func completeOrBoundedRange(
	start, end int,
	lines []string,
	terms []Term,
	maxBytes int,
) (int, int, bool) {
	start, end = clampLines(start, end, len(lines))
	if sourceRangeBytes(lines, start, end) <= maxBytes {
		return start, end, true
	}
	boundedStart, boundedEnd := boundedDeclarationLines(start, end, lines, terms)
	return boundedStart, boundedEnd, false
}

func sourceRangeBytes(lines []string, start, end int) int {
	start, end = clampLines(start, end, len(lines))
	total := 0
	for line := start; line <= end; line++ {
		total += len(lines[line-1]) + 1
	}
	return total
}

func documentAnchors(file collectedFile, terms []Term) []anchorCandidate {
	if oversizedFileScope(file) {
		if anchor, ok := fileFragmentAnchor(file, terms); ok {
			return []anchorCandidate{anchor}
		}
		return nil
	}
	headings := markdownHeadingPattern.FindAllStringSubmatchIndex(string(file.content), -1)
	if len(headings) == 0 {
		return lineAnchors(file, terms)
	}
	lineOffsets := byteLineOffsets(file.content)
	var result []anchorCandidate
	for index, match := range headings {
		sectionStart := lineForOffset(lineOffsets, match[0])
		sectionEnd := len(file.lines)
		if index+1 < len(headings) {
			sectionEnd = lineForOffset(lineOffsets, headings[index+1][0]) - 1
		}
		section := strings.TrimSpace(string(file.content[match[2]:match[3]]))
		start, end, complete := completeOrBoundedRange(
			sectionStart,
			sectionEnd,
			file.lines,
			terms,
			maxCompleteSymbolBytes,
		)
		score, hits := scoreLines(file.lines, start, end, terms)
		if score == 0 {
			continue
		}
		score += file.candidate.score / 3
		anchor := makeAnchor(file.candidate.path, "", section, section, start, end, file.lines, []AnchorRole{RoleDocumentationContract}, score)
		anchor.Scope = sourceScopeForAnchor(
			file,
			start,
			end,
			sectionStart,
			sectionEnd,
			complete,
			SourceScopeCompleteDocumentSection,
			terms,
		)
		result = append(result, anchorCandidate{anchor: anchor, score: score, terms: hits})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].score != result[j].score {
			return result[i].score > result[j].score
		}
		return result[i].anchor.StartLine < result[j].anchor.StartLine
	})
	return result
}

func lineAnchors(file collectedFile, terms []Term) []anchorCandidate {
	if oversizedFileScope(file) {
		if anchor, ok := fileFragmentAnchor(file, terms); ok {
			return []anchorCandidate{anchor}
		}
		return nil
	}
	if isFileScopeAnchorPath(file.candidate.path) && !file.truncated &&
		sourceRangeBytes(file.lines, 1, len(file.lines)) <= maxCompleteFileBytes {
		score, hits := scoreLines(file.lines, 1, len(file.lines), terms)
		if score > 0 {
			symbol := path.Base(file.candidate.path)
			anchor := makeAnchor(
				file.candidate.path,
				"",
				symbol,
				"",
				1,
				len(file.lines),
				file.lines,
				roleHints(symbol, file.candidate.path),
				score+file.candidate.score/3,
			)
			anchor.Scope = sourceScopeForAnchor(
				file,
				1,
				len(file.lines),
				1,
				len(file.lines),
				true,
				SourceScopeCompleteFile,
				terms,
			)
			return []anchorCandidate{{
				anchor: anchor,
				score:  score + file.candidate.score/3,
				terms:  hits,
			}}
		}
	}
	type scoredLine struct {
		line  int
		score int
	}
	var matches []scoredLine
	for index, line := range file.lines {
		score, _ := scoreText(line, terms)
		if score > 0 {
			matches = append(matches, scoredLine{line: index + 1, score: score})
		}
	}
	if len(matches) == 0 {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].line < matches[j].line
	})
	var result []anchorCandidate
	for _, match := range matches {
		start, end := clampLines(match.line-8, match.line+10, len(file.lines))
		overlaps := false
		for _, existing := range result {
			if existing.anchor.StartLine <= end && start <= existing.anchor.EndLine {
				overlaps = true
				break
			}
		}
		if overlaps {
			continue
		}
		score, hits := scoreLines(file.lines, start, end, terms)
		symbol := path.Base(file.candidate.path) + ":" + strconv.Itoa(match.line)
		roles := roleHints(symbol, file.candidate.path)
		anchor := makeAnchor(file.candidate.path, "", symbol, "", start, end, file.lines, roles, score+file.candidate.score/3)
		anchor.Scope = sourceScopeForAnchor(
			file,
			start,
			end,
			1,
			max(len(file.lines), start),
			false,
			SourceScopePartialWindow,
			terms,
		)
		result = append(result, anchorCandidate{anchor: anchor, score: score + file.candidate.score/3, terms: hits})
		if len(result) >= maxAnchorsPerFile {
			break
		}
	}
	return result
}

func oversizedFileScope(file collectedFile) bool {
	return isFileScopeAnchorPath(file.candidate.path) &&
		(file.truncated || sourceRangeBytes(file.lines, 1, len(file.lines)) > maxCompleteFileBytes)
}

func fileFragmentAnchor(file collectedFile, terms []Term) (anchorCandidate, bool) {
	matchingLines := taskMatchingLines(file.lines, 1, len(file.lines), terms)
	if len(matchingLines) == 0 {
		return anchorCandidate{}, false
	}

	plan := newFragmentLinePlan()
	for _, line := range matchingLines {
		plan.addTaskLine(line, len(file.lines))
		plan.addRange(
			line-fragmentContextLines,
			line+fragmentContextLines,
			3,
			len(file.lines),
		)
	}
	structuralStarts := fileStructuralStarts(file.candidate.path, file.lines)
	for _, line := range structuralStarts {
		plan.addLine(line, 2, len(file.lines))
	}
	for _, matchingLine := range matchingLines {
		block, found := enclosingStructuralBlock(structuralStarts, matchingLine, len(file.lines))
		if !found {
			continue
		}
		plan.addRange(block.start, block.end, 1, len(file.lines))
	}

	selected, clipped, allTaskLines := plan.selectLines(file.lines, maxCompleteFileBytes)
	symbol := path.Base(file.candidate.path)
	anchor := makeFragmentAnchor(
		file,
		"",
		symbol,
		"",
		roleHints(symbol, file.candidate.path),
		selected,
		terms,
		"oversized_file_fragment_retention",
		clipped,
		allTaskLines,
	)
	score, hits := scoreSourceLines(anchor.Excerpt, terms)
	score += file.candidate.score / 3
	anchor.Score = score
	return anchorCandidate{anchor: anchor, score: score, terms: hits}, true
}

func fileStructuralStarts(filePath string, lines []string) []int {
	lower := strings.ToLower(filePath)
	base := path.Base(lower)
	result := make([]int, 0)
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		structural := false
		switch {
		case isDocumentPath(lower):
			structural = strings.HasPrefix(trimmed, "#")
		case path.Ext(lower) == ".sh":
			structural = shellFunctionPattern.MatchString(line)
		case base == "makefile":
			structural = makeTargetPattern.MatchString(line)
		case path.Ext(lower) == ".toml":
			structural = configSectionPattern.MatchString(line)
		case path.Ext(lower) == ".yaml" || path.Ext(lower) == ".yml":
			structural = line == strings.TrimLeft(line, " \t") && configKeyPattern.MatchString(line)
		case path.Ext(lower) == ".json":
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			structural = indent <= 4 && strings.HasPrefix(trimmed, "\"") && strings.Contains(trimmed, "\":")
		case base == "dockerfile":
			fields := strings.Fields(trimmed)
			structural = len(fields) > 0 && fields[0] == strings.ToUpper(fields[0])
		case base == "go.mod" || base == "go.work":
			structural = strings.HasSuffix(trimmed, "(") || strings.HasPrefix(trimmed, "module ") ||
				strings.HasPrefix(trimmed, "go ") || strings.HasPrefix(trimmed, "toolchain ")
		}
		if structural {
			result = append(result, index+1)
		}
	}
	return result
}

func enclosingStructuralBlock(starts []int, line, lineCount int) (sourceLineRange, bool) {
	if len(starts) == 0 {
		return sourceLineRange{}, false
	}
	index := -1
	for candidate, start := range starts {
		if start > line {
			break
		}
		index = candidate
	}
	if index < 0 {
		return sourceLineRange{}, false
	}
	end := lineCount
	if index+1 < len(starts) {
		end = starts[index+1] - 1
	}
	return sourceLineRange{start: starts[index], end: end}, true
}

func isFileScopeAnchorPath(filePath string) bool {
	lower := strings.ToLower(filePath)
	base := path.Base(lower)
	if isDocumentPath(lower) {
		return true
	}
	switch path.Ext(lower) {
	case ".sh", ".json", ".yaml", ".yml", ".toml":
		return true
	default:
		return base == "makefile" || base == "dockerfile" || base == "go.mod" || base == "go.work"
	}
}

func sourceScopeForAnchor(
	file collectedFile,
	start, end int,
	sourceStart, sourceEnd int,
	complete bool,
	completeKind SourceScopeKind,
	terms []Term,
) SourceScope {
	outside := taskMatchesOutsideRange(
		file.lines,
		sourceStart,
		sourceEnd,
		start,
		end,
		terms,
	)
	for _, line := range file.candidate.grepLines {
		if line < start || line > end {
			outside = true
			break
		}
	}
	if complete && !file.truncated {
		return SourceScope{
			ScopeKind:                completeKind,
			ScopeStart:               start,
			ScopeEnd:                 end,
			SourceTotalLines:         file.sourceTotalLines,
			Truncated:                false,
			TruncationReason:         "",
			TaskMatchesOutsideWindow: false,
			NegativeClaimsAllowed:    true,
			NegativeEvidenceBasis:    NegativeEvidenceCompleteScope,
		}
	}
	reason := "per_anchor_byte_or_line_limit"
	if file.truncated {
		reason = file.truncationReason
		if reason == "" {
			reason = "file_read_byte_limit"
		}
	}
	return SourceScope{
		ScopeKind:                SourceScopePartialWindow,
		ScopeStart:               start,
		ScopeEnd:                 end,
		SourceTotalLines:         file.sourceTotalLines,
		Truncated:                true,
		TruncationReason:         reason,
		TaskMatchesOutsideWindow: outside,
		NegativeClaimsAllowed:    false,
		NegativeEvidenceBasis:    NegativeEvidenceNone,
	}
}

func makeFragmentAnchor(
	file collectedFile,
	packageName string,
	symbol string,
	section string,
	roles []AnchorRole,
	selectedLines []int,
	terms []Term,
	retentionReason string,
	clipped bool,
	allTaskLines bool,
) Anchor {
	if len(selectedLines) == 0 {
		return Anchor{}
	}
	excerpt := make([]SourceLine, 0, len(selectedLines))
	retained := make(map[int]struct{}, len(selectedLines))
	for _, line := range selectedLines {
		if line < 1 || line > len(file.lines) {
			continue
		}
		text := file.lines[line-1]
		excerpt = append(excerpt, SourceLine{
			Line:      line,
			Text:      text,
			Highlight: lineMatchesTask(text, terms),
		})
		retained[line] = struct{}{}
	}
	if len(excerpt) == 0 {
		return Anchor{}
	}

	start := excerpt[0].Line
	end := excerpt[len(excerpt)-1].Line
	knownMatchOutside := !allTaskLines
	for _, line := range file.candidate.grepLines {
		if _, exists := retained[line]; !exists {
			knownMatchOutside = true
			break
		}
	}
	scopeKind := SourceScopeMatchedFragments
	reason := retentionReason
	if file.truncated || clipped || !allTaskLines {
		scopeKind = SourceScopePartialWindow
		reason = "per_anchor_fragment_byte_or_line_limit"
		if file.truncated {
			reason = file.truncationReason
			if reason == "" {
				reason = "bounded_source_scan_limit"
			}
		}
	}
	anchor := Anchor{
		Path:      file.candidate.path,
		Symbol:    symbol,
		Section:   section,
		Package:   packageName,
		StartLine: start,
		EndLine:   end,
		Excerpt:   excerpt,
		Scope: SourceScope{
			ScopeKind:                scopeKind,
			ScopeStart:               start,
			ScopeEnd:                 end,
			SourceTotalLines:         file.sourceTotalLines,
			Truncated:                true,
			TruncationReason:         reason,
			TaskMatchesOutsideWindow: knownMatchOutside,
			NegativeClaimsAllowed:    false,
			NegativeEvidenceBasis:    NegativeEvidenceNone,
		},
		RoleHints:   roles,
		EvidenceIDs: []string{"pending"},
	}
	anchor.ID = OpaqueID(
		"anchor",
		anchor.Path,
		anchor.Symbol,
		strconv.Itoa(anchor.StartLine),
		strconv.Itoa(anchor.EndLine),
		SourceExcerptSHA256(anchor.Excerpt),
	)
	return anchor
}

func lineMatchesTask(line string, terms []Term) bool {
	score, _ := scoreText(line, terms)
	return score > 0
}

func scoreSourceLines(lines []SourceLine, terms []Term) (int, []string) {
	hits := make(map[string]struct{})
	for _, line := range lines {
		_, lineHits := scoreText(line.Text, terms)
		for _, hit := range lineHits {
			hits[hit] = struct{}{}
		}
	}
	score := 0
	for _, term := range terms {
		if _, exists := hits[term.ID]; exists {
			score += term.Weight
		}
	}
	result := make([]string, 0, len(hits))
	for hit := range hits {
		result = append(result, hit)
	}
	sort.Strings(result)
	return score, result
}

func taskMatchesOutsideRange(
	lines []string,
	sourceStart, sourceEnd int,
	retainedStart, retainedEnd int,
	terms []Term,
) bool {
	sourceStart, sourceEnd = clampLines(sourceStart, sourceEnd, len(lines))
	for line := sourceStart; line <= sourceEnd; line++ {
		if line >= retainedStart && line <= retainedEnd {
			continue
		}
		score, _ := scoreText(lines[line-1], terms)
		if score > 0 {
			return true
		}
	}
	return false
}

func makeAnchor(
	filePath, packageName, symbol, section string,
	start, end int,
	lines []string,
	roles []AnchorRole,
	score int,
) Anchor {
	excerpt := make([]SourceLine, 0, end-start+1)
	for line := start; line <= end; line++ {
		excerpt = append(excerpt, SourceLine{Line: line, Text: lines[line-1]})
	}
	return Anchor{
		ID: OpaqueID(
			"anchor", filePath, symbol, strconv.Itoa(start), strconv.Itoa(end),
			SourceExcerptSHA256(excerpt),
		),
		Path: filePath, Symbol: symbol, Section: section, Package: packageName,
		StartLine: start, EndLine: end, Excerpt: excerpt, RoleHints: roles, Score: score,
		EvidenceIDs: []string{"pending"},
	}
}

// SourceExcerptSHA256 binds exact retained source-line content to anchor and
// evidence identities.
func SourceExcerptSHA256(excerpt []SourceLine) string {
	raw, _ := json.Marshal(excerpt)
	return SHA256(raw)
}

func selectAnchors(candidates []anchorCandidate, terms []Term) []Anchor {
	chosenIndexes := make(map[int]struct{}, min(MaxRetainedAnchors, len(candidates)))
	chosenOrder := make([]int, 0, min(MaxRetainedAnchors, len(candidates)))
	seen := make(map[string]struct{})
	perFile := make(map[string]int)
	add := func(index, fileLimit int) {
		if len(chosenIndexes) >= MaxRetainedAnchors {
			return
		}
		item := candidates[index]
		key := item.anchor.Path + "\x00" + item.anchor.Symbol + "\x00" + strconv.Itoa(item.anchor.StartLine)
		if _, duplicate := seen[key]; duplicate || perFile[item.anchor.Path] >= fileLimit {
			return
		}
		chosenIndexes[index] = struct{}{}
		chosenOrder = append(chosenOrder, index)
		seen[key] = struct{}{}
		perFile[item.anchor.Path]++
	}
	for _, term := range terms {
		if term.Weight < 10 {
			continue
		}
		for index, item := range candidates {
			symbol := strings.TrimPrefix(item.anchor.Symbol, "*")
			if strings.EqualFold(symbol, term.Text) || strings.HasSuffix(strings.ToLower(symbol), "."+term.Normalized) {
				add(index, 4)
				break
			} else if strings.Contains(strings.ToLower(symbol), term.Normalized) {
				add(index, 4)
				break
			}
		}
	}
	reservedTests := make(map[string]struct{})
	for index, item := range candidates {
		if !isTestPath(item.anchor.Path) || len(reservedTests) >= 3 {
			continue
		}
		if _, exists := reservedTests[item.anchor.Path]; exists {
			continue
		}
		add(index, 4)
		reservedTests[item.anchor.Path] = struct{}{}
	}
	if len(candidates) > 0 {
		for index, item := range candidates {
			if isDocumentPath(item.anchor.Path) && item.score*2 >= candidates[0].score {
				add(index, 4)
				break
			}
		}
	}
	representedFiles := make(map[string]struct{})
	for index := range chosenIndexes {
		// chosenIndexes is populated only from indexes produced by ranging candidates.
		//nolint:nilaway
		representedFiles[candidates[index].anchor.Path] = struct{}{}
	}
	for index, item := range candidates {
		if len(representedFiles) >= 6 {
			break
		}
		if _, represented := representedFiles[item.anchor.Path]; represented {
			continue
		}
		before := len(chosenIndexes)
		add(index, 4)
		if len(chosenIndexes) > before {
			representedFiles[item.anchor.Path] = struct{}{}
		}
	}
	for index := range candidates {
		add(index, 6)
		if len(chosenIndexes) >= MaxRetainedAnchors {
			break
		}
	}
	for index := range candidates {
		add(index, maxAnchorsPerFile)
		if len(chosenIndexes) >= MaxRetainedAnchors {
			break
		}
	}
	selected := make([]Anchor, 0, len(chosenOrder))
	for _, index := range chosenOrder {
		selected = append(selected, candidates[index].anchor)
	}
	return selected
}

func boundAnchorsBySourceBytes(anchors []Anchor) ([]Anchor, int, bool) {
	selected := make([]Anchor, 0, len(anchors))
	retainedBytes := 0
	limitBound := false
	for _, anchor := range anchors {
		size := anchorExcerptBytes(anchor)
		if size > MaxRetainedSourceBytes-retainedBytes {
			limitBound = true
			continue
		}
		selected = append(selected, anchor)
		retainedBytes += size
	}
	return selected, retainedBytes, limitBound
}

func anchorExcerptBytes(anchor Anchor) int {
	total := 0
	for _, line := range anchor.Excerpt {
		total += len(line.Text) + 1
	}
	return total
}

func retainedAnchorSourceBytes(anchors []Anchor) int {
	total := 0
	for _, anchor := range anchors {
		total += anchorExcerptBytes(anchor)
	}
	return total
}

// RetainedSourceByteCount reports the exact source-line bytes exposed by the
// retained anchor set, including one line separator per saved line.
func RetainedSourceByteCount(anchors []Anchor) int {
	return retainedAnchorSourceBytes(anchors)
}

func groundTerms(terms []Term, anchors []Anchor) {
	for index := range terms {
		for _, anchor := range anchors {
			if anchorContainsExact(anchor, terms[index].Normalized) {
				terms[index].EvidenceIDs = append(terms[index].EvidenceIDs, anchor.EvidenceIDs...)
			}
		}
		terms[index].EvidenceIDs = uniqueSorted(terms[index].EvidenceIDs)
		terms[index].Found = len(terms[index].EvidenceIDs) > 0
	}
}

// GroundedTaskTerms deterministically derives the exact task-term projection
// used by bundle replay and artifact fixtures.
func GroundedTaskTerms(taskText string, anchors []Anchor) []Term {
	terms := extractTerms(taskText)
	groundTerms(terms, anchors)
	return terms
}

// GroundedTaskClassification returns the deterministic task kind and
// task-provided observable saved in a bundle.
func GroundedTaskClassification(taskText string) (TaskKind, string) {
	return classifyTaskKind(taskText), taskObservable(taskText)
}

// GroundedRelations returns the complete ordered local relation set for the
// retained anchors and grounded task terms.
func GroundedRelations(anchors []Anchor, terms []Term) []Relation {
	return collectRelations(anchors, terms)
}

// GroundedLocality returns the deterministic locality classification for a
// completed retained evidence set.
func GroundedLocality(
	taskText string,
	terms []Term,
	anchors []Anchor,
	relations []Relation,
) Locality {
	return classifyLocality(taskText, terms, anchors, relations)
}

func collectRelations(anchors []Anchor, terms []Term) []Relation {
	type rankedRelation struct {
		relation Relation
		score    int
	}
	var ranked []rankedRelation
	seen := make(map[string]struct{})
	for left := range anchors {
		for right := left + 1; right < len(anchors); right++ {
			kind := ""
			sharedWeight := 0
			relationLeft, relationRight := left, right
			leftSymbol := baseSymbol(anchors[left].Symbol)
			rightSymbol := baseSymbol(anchors[right].Symbol)
			if uniqueCallableTarget(anchors, right) && anchorCalls(anchors[left], anchors[right]) {
				kind = relationKindDirectCall
			} else if uniqueCallableTarget(anchors, left) && anchorCalls(anchors[right], anchors[left]) {
				kind = relationKindDirectCall
				relationLeft, relationRight = right, left
			}
			if kind == "" && isTestPath(anchors[left].Path) != isTestPath(anchors[right].Path) {
				production, test := anchors[left], anchors[right]
				if isTestPath(production.Path) {
					production, test = test, production
				}
				if testReferencesProductionAnchor(test, production) {
					kind = relationKindTestReference
				}
			}
			if kind == "" {
				if anchorReferencesFixture(anchors[left], anchors[right]) {
					kind = string(RelationFixtureRecords)
				} else if anchorReferencesFixture(anchors[right], anchors[left]) {
					kind = string(RelationFixtureRecords)
					relationLeft, relationRight = right, left
				}
			}
			if kind == "" && leftSymbol != rightSymbol && len(rightSymbol) >= 3 &&
				anchorExcerptContainsExact(anchors[left], strings.ToLower(rightSymbol)) {
				kind = classifyReferencedRelation(anchors[left], anchors[right], terms)
			} else if kind == "" && leftSymbol != rightSymbol && len(leftSymbol) >= 3 &&
				anchorExcerptContainsExact(anchors[right], strings.ToLower(leftSymbol)) {
				kind = classifyReferencedRelation(anchors[right], anchors[left], terms)
				relationLeft, relationRight = right, left
			}
			if kind == "" {
				for _, term := range terms {
					if term.Weight >= 8 && anchorContainsExact(anchors[left], term.Normalized) &&
						anchorContainsExact(anchors[right], term.Normalized) {
						kind = classifySharedTermRelation(anchors[left], anchors[right], term.Normalized, terms)
						sharedWeight = max(sharedWeight, term.Weight)
					}
				}
			}
			if kind == "" {
				continue
			}
			key := anchors[relationLeft].ID + "\x00" + anchors[relationRight].ID + "\x00" + kind
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			relation := Relation{
				ID: OpaqueID("relation", key), LeftID: anchors[relationLeft].ID, RightID: anchors[relationRight].ID,
				Kind: kind, SupportType: SupportLocallyObserved,
				EvidenceIDs: uniqueSorted(append(append([]string{}, anchors[left].EvidenceIDs...), anchors[right].EvidenceIDs...)),
				Scope:       relationScope(kind),
			}
			score := anchors[left].Score + anchors[right].Score
			switch kind {
			case relationKindDirectCall:
				score += 4_000
			case relationKindTestReference:
				score += 3_000
			case string(RelationConfigApplied), string(RelationErrorMapped),
				string(RelationErrorExposed), string(RelationValueTransformed),
				string(RelationFieldCopy), string(RelationFieldRead),
				string(RelationFieldWrite), string(RelationScriptInvokes),
				string(RelationDocumentedUses), string(RelationFixtureRecords):
				score += 2_000
			case relationKindSharedTaskTerm:
				score += 1_000 + sharedWeight*10
			}
			ranked = append(ranked, rankedRelation{relation: relation, score: score})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].relation.ID < ranked[j].relation.ID
	})
	if len(ranked) > 24 {
		ranked = ranked[:24]
	}
	relations := make([]Relation, 0, len(ranked))
	for _, item := range ranked {
		relations = append(relations, item.relation)
	}
	return relations
}

func classifyReferencedRelation(source, target Anchor, terms []Term) string {
	if isTestPath(source.Path) && !isTestPath(target.Path) {
		if testReferencesProductionAnchor(source, target) {
			return string(RelationTestExercises)
		}
		return relationKindExactIdentifier
	}
	if isDocumentPath(source.Path) && source.Scope.isComplete() {
		return string(RelationDocumentedUses)
	}
	if isOperationalSourcePath(source.Path) && source.Scope.ScopeKind == SourceScopeCompleteFile &&
		isOperationalRelationTarget(target) && anchorNamesOperationalTarget(source, target) {
		return string(RelationScriptInvokes)
	}
	if anchorHasAnyRole(source, RoleConfigurationCopy) &&
		anchorHasAnyRole(target, RoleConfigurationSource, RoleEffectiveDestination) &&
		anchorHasExactSelectorCopy(source, terms) {
		return string(RelationConfigApplied)
	}
	if anchorHasAnyRole(source, RoleErrorNormalizer, RoleErrorMapping) &&
		anchorHasAnyRole(target, RolePublicErrorType, RoleErrorCreation, RoleErrorMapping) &&
		anchorHasExactErrorMappingSyntax(source) && anchorReferencesTypedTarget(source, target) {
		return string(RelationErrorMapped)
	}
	if anchorHasAnyRole(source, RolePublicErrorExposure, RoleIntegrationBoundary) &&
		anchorHasAnyRole(target, RolePublicErrorType, RoleErrorNormalizer, RoleErrorCreation) &&
		anchorHasExactExposureSyntax(source) {
		return string(RelationErrorExposed)
	}
	return relationKindExactIdentifier
}

func testReferencesProductionAnchor(test, production Anchor) bool {
	if !isTestPath(test.Path) || isTestPath(production.Path) {
		return false
	}
	target := baseSymbol(production.Symbol)
	if len(target) < 3 || strings.Contains(production.Symbol, ".") {
		return false
	}
	parsed, err := parseGoAnchor(test)
	if err != nil {
		return false
	}
	if sameGoPackage(test, production) {
		found := false
		selectorIdentifiers := make(map[*ast.Ident]struct{})
		ast.Inspect(parsed, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			selectorIdentifiers[selector.Sel] = struct{}{}
			if qualifier, ok := selector.X.(*ast.Ident); ok {
				selectorIdentifiers[qualifier] = struct{}{}
			}
			return true
		})
		ast.Inspect(parsed, func(node ast.Node) bool {
			if found {
				return false
			}
			identifier, ok := node.(*ast.Ident)
			if !ok || identifier.Name != target || identifier.Obj != nil {
				return true
			}
			_, selectorPart := selectorIdentifiers[identifier]
			found = !selectorPart
			return !found
		})
		return found
	}
	if production.Package == "" {
		return false
	}
	found := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		if found {
			return false
		}
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != target {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		found = ok && qualifier.Name == production.Package && qualifier.Obj == nil
		return !found
	})
	return found
}

func anchorReferencesTypedTarget(source, target Anchor) bool {
	symbol := strings.Trim(strings.TrimPrefix(target.Symbol, "*"), "()")
	dot := strings.LastIndex(symbol, ".")
	if dot < 0 {
		return true
	}
	receiver := strings.Trim(symbol[:dot], "()*")
	if packageDot := strings.LastIndex(receiver, "."); packageDot >= 0 {
		receiver = receiver[packageDot+1:]
	}
	if receiver == "" {
		return false
	}
	return anchorExcerptContainsExact(source, strings.ToLower(receiver))
}

func isOperationalRelationTarget(anchor Anchor) bool {
	lower := strings.ToLower(anchor.Path)
	base := path.Base(lower)
	return path.Ext(lower) == ".sh" || base == "makefile" || base == "taskfile" ||
		base == "go.mod" || base == "go.work" || strings.HasPrefix(lower, "cmd/")
}

func anchorNamesOperationalTarget(source, target Anchor) bool {
	lower := strings.ToLower(anchorText(source))
	return strings.Contains(lower, strings.ToLower(target.Path)) ||
		strings.Contains(lower, strings.ToLower(path.Base(target.Path)))
}

func classifySharedTermRelation(left, right Anchor, term string, terms []Term) string {
	if !taskNamesPrimaryConfigurationField(terms, term) {
		return relationKindSharedTaskTerm
	}
	if anchorHasAnyRole(left, RoleEffectiveDestination) &&
		anchorHasAnyRole(right, RoleConfigurationSource, RoleConfigurationCopy) &&
		anchorReadsSelector(left, term) {
		return string(RelationFieldRead)
	}
	if anchorHasAnyRole(right, RoleEffectiveDestination) &&
		anchorHasAnyRole(left, RoleConfigurationSource, RoleConfigurationCopy) &&
		anchorReadsSelector(right, term) {
		return string(RelationFieldRead)
	}
	return relationKindSharedTaskTerm
}

func taskNamesPrimaryConfigurationField(terms []Term, field string) bool {
	bestNormalized := ""
	bestWeight, bestLength := -1, -1
	for _, term := range terms {
		if term.Weight < 4 {
			continue
		}
		normalized := term.Normalized
		if normalized == "config" || normalized == "configuration" || normalized == "option" ||
			normalized == "options" || normalized == "settings" || normalized == "engine" ||
			strings.HasSuffix(normalized, "config") || strings.HasSuffix(normalized, "configuration") ||
			strings.HasSuffix(normalized, "options") {
			continue
		}
		if term.Weight > bestWeight || term.Weight == bestWeight && len(normalized) > bestLength ||
			term.Weight == bestWeight && len(normalized) == bestLength && normalized < bestNormalized {
			bestNormalized, bestWeight, bestLength = normalized, term.Weight, len(normalized)
		}
	}
	return bestNormalized != "" && strings.EqualFold(bestNormalized, field)
}

func anchorReferencesFixture(source, fixture Anchor) bool {
	if !strings.HasSuffix(strings.ToLower(source.Path), "_test.go") || !isGeneratedFixturePath(fixture.Path) {
		return false
	}
	text := anchorText(source)
	return strings.Contains(text, fixture.Path) || strings.Contains(text, path.Base(fixture.Path))
}

func isGeneratedFixturePath(filePath string) bool {
	lower := strings.ToLower(filePath)
	return (strings.Contains(lower, "testdata/") || strings.Contains(lower, "golden") ||
		strings.Contains(lower, "snapshot") || strings.Contains(lower, "generated")) &&
		(path.Ext(lower) == ".json" || path.Ext(lower) == ".yaml" || path.Ext(lower) == ".yml")
}

func anchorHasExactSelectorCopy(anchor Anchor, terms []Term) bool {
	parsed, err := parseGoAnchor(anchor)
	if err != nil {
		return false
	}
	found := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		if found {
			return false
		}
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, left := range assignment.Lhs {
			leftSelector, leftOK := left.(*ast.SelectorExpr)
			if !leftOK {
				continue
			}
			for _, right := range assignment.Rhs {
				rightSelector, rightOK := right.(*ast.SelectorExpr)
				if rightOK && leftSelector.Sel.Name == rightSelector.Sel.Name &&
					taskNamesField(terms, leftSelector.Sel.Name) {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

func taskNamesField(terms []Term, field string) bool {
	lowerField := strings.ToLower(field)
	for _, term := range terms {
		if term.Weight < 4 {
			continue
		}
		if term.Normalized == lowerField {
			return true
		}
	}
	return false
}

func anchorHasExactErrorMappingSyntax(anchor Anchor) bool {
	parsed, err := parseGoAnchor(anchor)
	if err != nil {
		return false
	}
	found := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch function := call.Fun.(type) {
		case *ast.SelectorExpr:
			if function.Sel.Name == "As" || function.Sel.Name == "StatusCode" {
				found = true
			}
		case *ast.Ident:
			if strings.Contains(strings.ToLower(function.Name), "error") {
				found = true
			}
		}
		return !found
	})
	return found
}

func anchorHasExactExposureSyntax(anchor Anchor) bool {
	lower := strings.ToLower(anchorText(anchor))
	return strings.Contains(lower, "publicerror") || strings.Contains(lower, "writeheader") ||
		strings.Contains(lower, "json") || strings.Contains(lower, "xml") ||
		strings.Contains(lower, "yaml") || strings.Contains(lower, "marshal")
}

func anchorReadsSelector(anchor Anchor, wanted string) bool {
	parsed, err := parseGoAnchor(anchor)
	if err != nil {
		return false
	}
	found := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok && strings.EqualFold(selector.Sel.Name, wanted) {
			found = true
			return false
		}
		return !found
	})
	return found
}

func parseGoAnchor(anchor Anchor) (*ast.File, error) {
	if !strings.HasSuffix(strings.ToLower(anchor.Path), ".go") || len(anchor.Excerpt) == 0 {
		return nil, fmt.Errorf("not a retained Go declaration")
	}
	var source strings.Builder
	source.WriteString("package tasklens_anchor\n")
	for _, line := range anchor.Excerpt {
		source.WriteString(line.Text)
		source.WriteByte('\n')
	}
	return parser.ParseFile(token.NewFileSet(), anchor.Path, source.String(), 0)
}

func anchorHasAnyRole(anchor Anchor, roles ...AnchorRole) bool {
	for _, role := range roles {
		if slices.Contains(anchor.RoleHints, role) {
			return true
		}
	}
	return false
}

func isOperationalSourcePath(filePath string) bool {
	lower := strings.ToLower(filePath)
	return path.Ext(lower) == ".sh" || path.Base(lower) == "makefile"
}

func baseSymbol(symbol string) string {
	if dot := strings.LastIndex(symbol, "."); dot >= 0 {
		symbol = symbol[dot+1:]
	}
	if bracket := strings.Index(symbol, "["); bracket >= 0 {
		symbol = symbol[:bracket]
	}
	return strings.TrimPrefix(symbol, "*")
}

func uniqueCallableTarget(anchors []Anchor, targetIndex int) bool {
	target := anchors[targetIndex]
	if strings.Contains(target.Symbol, ".") || !anchorIsFunction(target) {
		return false
	}
	count := 0
	for _, candidate := range anchors {
		if sameGoPackage(candidate, target) && candidate.Symbol == target.Symbol && anchorIsFunction(candidate) {
			count++
		}
	}
	return count == 1
}

func anchorIsFunction(anchor Anchor) bool {
	return len(anchor.Excerpt) > 0 && strings.HasPrefix(strings.TrimSpace(anchor.Excerpt[0].Text), "func ")
}

func anchorCalls(anchor, target Anchor) bool {
	found, _ := parsedAnchorCalls(anchor, target)
	return found
}

func parsedAnchorCalls(anchor, target Anchor) (bool, error) {
	callee := target.Symbol
	if len(callee) < 3 || !sameGoPackage(anchor, target) ||
		strings.Contains(callee, ".") || !anchorIsFunction(target) {
		return false, nil
	}
	var source strings.Builder
	source.WriteString("package tasklens_anchor\n")
	for _, line := range anchor.Excerpt {
		source.WriteString(line.Text)
		source.WriteByte('\n')
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), anchor.Path, source.String(), 0)
	if err != nil {
		// A clipped declaration is not sufficient authority for a syntactic
		// call relation. Exact identifier relations can still represent it.
		return false, err
	}
	found := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		if found {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		function, ok := call.Fun.(*ast.Ident)
		// A resolved object here is local to the retained declaration (for
		// example, a parameter or variable that shadows the package function),
		// so it is not evidence of a call-name match to the target anchor.
		found = ok && function.Name == callee && function.Obj == nil
		return !found
	})
	return found, nil
}

func sameGoPackage(left, right Anchor) bool {
	return left.Package != "" && left.Package == right.Package && path.Dir(left.Path) == path.Dir(right.Path)
}

func relationScope(kind string) string {
	switch RelationKind(kind) {
	case RelationDirectCall:
		return "A direct call expression is present in the retained caller excerpt; this does not prove runtime reachability, order, or callee behavior."
	case RelationConfigApplied, RelationFieldCopy, RelationFieldRead, RelationFieldWrite:
		return "The retained source contains the named field reference or assignment; this does not prove that every runtime construction path reaches it."
	case RelationErrorCreated, RelationErrorMapped, RelationErrorExposed:
		return "The retained source contains the named error construction, mapping, or exposure syntax; endpoint reachability and runtime ordering are not implied."
	case RelationValueTransformed, RelationTypeNameGenerated:
		return "The retained source contains the named transformation reference; the relation does not establish all input variants or runtime outcomes."
	case RelationTestExercises, RelationFixtureRecords:
		return "The retained test or fixture names exact production evidence; it is not evidence that the test was executed for this investigation."
	case RelationScriptInvokes, RelationDocumentedUses:
		return "The retained script, target, or document contains the exact invocation or usage reference; command execution is outside this static investigation."
	}
	return "Exact retained source references only; this does not prove runtime order, reachability, or causality."
}

func scoreLines(lines []string, start, end int, terms []Term) (int, []string) {
	hits := make(map[string]struct{})
	for line := start; line <= end && line <= len(lines); line++ {
		_, lineHits := scoreText(lines[line-1], terms)
		for _, hit := range lineHits {
			hits[hit] = struct{}{}
		}
	}
	score := 0
	for _, term := range terms {
		if _, ok := hits[term.ID]; ok {
			score += term.Weight
		}
	}
	result := make([]string, 0, len(hits))
	for hit := range hits {
		result = append(result, hit)
	}
	sort.Strings(result)
	return score, result
}

func scoreText(text string, terms []Term) (int, []string) {
	lower := strings.ToLower(text)
	score := 0
	var hits []string
	for _, term := range terms {
		if strings.Contains(lower, term.Normalized) {
			score += term.Weight
			hits = append(hits, term.ID)
		}
	}
	return score, hits
}

func anchorContainsExact(anchor Anchor, normalized string) bool {
	if containsExactTerm(anchor.Symbol, normalized) || containsExactTerm(anchor.Section, normalized) {
		return true
	}
	for _, line := range anchor.Excerpt {
		if containsExactTerm(line.Text, normalized) {
			return true
		}
	}
	return false
}

func anchorExcerptContainsExact(anchor Anchor, normalized string) bool {
	for _, line := range anchor.Excerpt {
		if containsExactTerm(line.Text, normalized) {
			return true
		}
	}
	return false
}

func containsExactTerm(text, term string) bool {
	return containsIdentifierTerm(text, term)
}

func roleHints(symbol, filePath string) []AnchorRole {
	return deterministicRoleHints(Anchor{Symbol: symbol, Path: filePath}, TaskUnknown, "")
}

func deterministicRoleHints(anchor Anchor, kind TaskKind, taskText string) []AnchorRole {
	words := identifierWordSet(anchor.Symbol + " " + anchor.Path)
	excerpt := make([]string, 0, len(anchor.Excerpt))
	for _, line := range anchor.Excerpt {
		excerpt = append(excerpt, line.Text)
	}
	excerptText := strings.Join(excerpt, "\n")
	hasWord := func(values ...string) bool {
		for _, value := range values {
			if _, ok := words[value]; ok {
				return true
			}
		}
		return false
	}
	var roles []AnchorRole
	add := func(role AnchorRole) {
		if !slices.Contains(roles, role) {
			roles = append(roles, role)
		}
	}
	if isDocumentPath(anchor.Path) {
		add(RoleDocumentationContract)
	}
	if isTestPath(anchor.Path) {
		add(RoleVerificationAnchor)
		for _, role := range additionalRoleHints(anchor, kind, taskText) {
			add(role)
		}
		return roles
	}
	if hasWord("config", "configuration", "option", "options", "setting", "settings") {
		add(RoleConfigurationSource)
	}
	if hasWord("copy", "clone", "merge", "apply") &&
		hasWord("config", "configuration", "option", "options", "setting", "settings") {
		add(RoleConfigurationCopy)
	}
	if hasWord("new", "register", "serve", "main", "command", "cli") ||
		strings.HasPrefix(strings.ToLower(anchor.Path), "cmd/") {
		add(RolePublicOrCLIEntry)
	}
	if hasWord("error", "err") ||
		strings.Contains(excerptText, "errors.New(") || strings.Contains(excerptText, "fmt.Errorf(") {
		add(RoleErrorCreation)
	}
	if hasWord("map", "mapping", "normalize", "normalization", "translate", "convert", "wrap", "status") &&
		(hasWord("error", "err") || strings.Contains(strings.ToLower(excerptText), "error")) {
		add(RoleErrorMapping)
	}
	if hasWord(
		"serial", "serialize", "serializer", "adapter", "adaptor", "handler",
		"middleware", "transport", "boundary",
	) {
		add(RoleIntegrationBoundary)
	}
	if strings.Contains(excerptText, "type "+anchor.Symbol+" struct") ||
		strings.Contains(excerptText, "type "+anchor.Symbol+" interface") ||
		hasWord("store", "state", "registry", "cache") {
		add(RoleStateOwner)
	}
	if hasWord("set", "update", "append", "add", "delete", "remove", "register", "copy", "apply", "merge") {
		add(RoleStateMutation)
	}
	if hasWord("generated", "codegen") || strings.Contains(excerptText, "Code generated") {
		add(RoleGeneratedOutput)
	}
	if kind == TaskBug || kind == TaskConfiguration || kind == TaskCompatibility {
		for _, term := range extractTerms(taskText) {
			if term.Weight >= 10 && !genericGrepTerm(term.Normalized) &&
				(containsExactTerm(anchor.Symbol, term.Normalized) || containsExactTerm(anchor.Section, term.Normalized)) {
				add(RoleSymptomSite)
				break
			}
		}
	}
	if len(roles) == 0 {
		add(RoleRepresentativeImplementation)
	}
	for _, role := range additionalRoleHints(anchor, kind, taskText) {
		add(role)
	}
	return roles
}

// GroundedAnchorRoles recomputes the closed, deterministic role hints exposed
// by a retained anchor. Saved bundles use this during replay validation.
func GroundedAnchorRoles(anchor Anchor, kind TaskKind, taskText string) []AnchorRole {
	return deterministicRoleHints(anchor, kind, taskText)
}

func identifierWordSet(value string) map[string]struct{} {
	words := make(map[string]struct{})
	start := -1
	flush := func(end int) {
		if start >= 0 && end > start {
			words[strings.ToLower(value[start:end])] = struct{}{}
		}
		start = -1
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		letterOrDigit := character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' || character >= '0' && character <= '9'
		if !letterOrDigit {
			flush(index)
			continue
		}
		if start < 0 {
			start = index
			continue
		}
		previous := value[index-1]
		if character >= 'A' && character <= 'Z' && previous >= 'a' && previous <= 'z' {
			flush(index)
			start = index
		}
	}
	flush(len(value))
	return words
}

func candidateRoleBonus(filePath string, kind TaskKind, task string) int {
	lower := strings.ToLower(filePath)
	base := path.Base(lower)
	bonus := 0
	if isDocumentPath(filePath) && (kind == TaskCompatibility || strings.Contains(strings.ToLower(task), "documentation")) {
		bonus += 9
	}
	if kind == TaskExtension {
		if base == "go.work" {
			bonus += 8
		}
		if strings.HasPrefix(lower, "examples/") || strings.Contains(lower, "/adapter") || strings.Contains(lower, "/adaptor") || strings.Contains(lower, "/integration") {
			bonus += 7
		}
	}
	return bonus
}

func classifyTaskKind(task string) TaskKind {
	words := classificationWords(task)
	has := func(values ...string) bool {
		for _, value := range values {
			if slices.Contains(words, value) {
				return true
			}
		}
		return false
	}
	hasPhrase := func(values ...string) bool {
		for _, value := range values {
			if containsWordSequence(words, strings.Fields(value)) {
				return true
			}
		}
		return false
	}
	hasPrefix := func(prefix string) bool {
		for _, word := range words {
			if strings.HasPrefix(word, prefix) {
				return true
			}
		}
		return false
	}
	switch {
	case hasPhrase("add support", "new integration") || has("adapter", "adaptor", "contribution", "extension"):
		return TaskExtension
	case hasPrefix("compatib") || hasPhrase("version mismatch", "older specification", "dependency import", "no longer renders"):
		return TaskCompatibility
	case has("configuration", "setting", "option") || hasPhrase("config field"):
		return TaskConfiguration
	case has("panic", "bug", "fails", "failure", "nil") ||
		hasPhrase("wrong status", "incorrect status", "silently ignored"):
		return TaskBug
	case has("release", "deploy", "operational", "script"):
		return TaskOperational
	case has("implement", "feature"):
		return TaskFeature
	default:
		return TaskUnknown
	}
}

func classificationWords(value string) []string {
	fields := strings.FieldsFunc(strings.ToLower(value), func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '_'
	})
	return fields
}

func containsWordSequence(words, sequence []string) bool {
	if len(sequence) == 0 || len(sequence) > len(words) {
		return false
	}
	for start := 0; start+len(sequence) <= len(words); start++ {
		if slices.Equal(words[start:start+len(sequence)], sequence) {
			return true
		}
	}
	return false
}

func classifyLocality(task string, terms []Term, anchors []Anchor, relations []Relation) Locality {
	if len(anchors) == 0 {
		return LocalityBroadDynamic
	}
	kind := classifyTaskKind(task)
	if kind == TaskExtension {
		return LocalityExtension
	}
	foundStrong := 0
	for _, term := range terms {
		if term.Found && term.Weight >= 8 {
			foundStrong++
		}
	}
	words := classificationWords(task)
	// Retained anchor count measures the breadth of the bounded retrieval, not
	// whether its best evidence identifies a local contract. Configuration and
	// documentation compatibility tasks have a cheap exit when an exact strong
	// task term is witnessed at the role that owns that contract. Requiring the
	// role and the exact term on the same anchor avoids promoting a broad result
	// merely because a repository-wide name was found somewhere in the bundle.
	if kind == TaskConfiguration && hasRoleGroundedStrongTerm(terms, anchors,
		RoleConfigurationSource, RoleConfigurationCopy) {
		return LocalityLocalExact
	}
	if kind == TaskCompatibility && hasRoleGroundedStrongTerm(terms, anchors,
		RoleDocumentationContract) {
		return LocalityLocalExact
	}
	localCue := slices.Contains(words, "config") || slices.Contains(words, "configuration") ||
		slices.Contains(words, "nil") || slices.Contains(words, "panic") ||
		slices.Contains(words, "dependency") || slices.Contains(words, "documentation") ||
		containsWordSequence(words, []string{"wrong", "status"}) ||
		containsWordSequence(words, []string{"incorrect", "status"}) ||
		containsWordSequence(words, []string{"error", "type"})
	if foundStrong >= 1 && len(anchors) <= 8 && localCue {
		return LocalityLocalExact
	}
	if len(anchors) >= PreferredMinVisibleAnchors && (foundStrong >= 1 || len(relations) > 0) {
		return LocalityBoundedCrossFile
	}
	return LocalityBroadDynamic
}

func hasRoleGroundedStrongTerm(terms []Term, anchors []Anchor, roles ...AnchorRole) bool {
	for _, anchor := range anchors {
		hasRole := false
		for _, role := range roles {
			if slices.Contains(anchor.RoleHints, role) {
				hasRole = true
				break
			}
		}
		if !hasRole {
			continue
		}
		for _, term := range terms {
			if term.Found && term.Weight >= 8 && anchorContainsExact(anchor, term.Normalized) {
				return true
			}
		}
	}
	return false
}

func taskObservable(task string) string {
	paragraph := task
	if separator := strings.Index(paragraph, "\n\n"); separator >= 0 {
		paragraph = paragraph[:separator]
	}
	paragraph = strings.Join(strings.Fields(paragraph), " ")
	return truncateUTF8(paragraph, 900)
}

func anchorEvidenceSummary(anchor Anchor, kind EvidenceKind) string {
	if kind == EvidenceDocumentClaim {
		return fmt.Sprintf("Exact repository document excerpt for %s at %s:%d-%d.", anchor.Symbol, anchor.Path, anchor.StartLine, anchor.EndLine)
	}
	return fmt.Sprintf("Exact repository source excerpt for %s at %s:%d-%d.", anchor.Symbol, anchor.Path, anchor.StartLine, anchor.EndLine)
}

func isEvidenceFile(filePath string) bool {
	lower := strings.ToLower(filePath)
	base := path.Base(lower)
	if strings.HasPrefix(lower, "vendor/") || strings.Contains(lower, "/vendor/") ||
		strings.HasPrefix(lower, ".git/") || strings.Contains(lower, "node_modules/") ||
		base == "package-lock.json" || base == "npm-shrinkwrap.json" {
		return false
	}
	ext := strings.ToLower(path.Ext(lower))
	switch ext {
	case ".go", ".md", ".mdx", ".rst", ".txt", ".json", ".yaml", ".yml", ".toml", ".sh":
		return true
	}
	return base == "makefile" || base == "dockerfile" || base == "go.mod" || base == "go.work"
}

func isDocumentPath(filePath string) bool {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".md", ".mdx", ".rst", ".txt":
		return true
	default:
		return false
	}
}

func isTestPath(filePath string) bool {
	lower := strings.ToLower(filePath)
	return strings.HasSuffix(lower, "_test.go") || strings.Contains(lower, "/testdata/") ||
		strings.HasPrefix(lower, "testdata/") || strings.Contains(lower, "/examples/") || strings.HasPrefix(lower, "examples/") ||
		strings.Contains(lower, "/example/") || strings.HasPrefix(lower, "example/")
}

// IsTestOrExamplePath is the canonical repository-owned verification path
// predicate used by collection, pack validation, and report replay.
func IsTestOrExamplePath(filePath string) bool {
	return isTestPath(filePath)
}

func clampLines(start, end, count int) (int, int) {
	if count <= 0 {
		return 1, 1
	}
	if start < 1 {
		start = 1
	}
	if end < start {
		end = start
	}
	if end > count {
		end = count
	}
	if start > end {
		start = end
	}
	return start, end
}

func byteLineOffsets(content []byte) []int {
	offsets := []int{0}
	for index, value := range content {
		if value == '\n' {
			offsets = append(offsets, index+1)
		}
	}
	return offsets
}

func lineForOffset(offsets []int, offset int) int {
	index, _ := slices.BinarySearch(offsets, offset)
	if index < len(offsets) && offsets[index] > offset {
		index--
	}
	return index + 1
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func genericGrepTerm(value string) bool {
	switch value {
	case "write", "read", "request", "response", "context", "error", "handler", "result", "value", "exists", "package", "type":
		return true
	default:
		return false
	}
}

func usableGrepTerm(term Term) bool {
	if len(term.Normalized) < 4 || term.Weight < 3 || genericGrepTerm(term.Normalized) {
		return false
	}
	query := strings.TrimSpace(term.Text)
	if query == "" || strings.ContainsAny(query, "[](){}<>*") ||
		strings.HasPrefix(query, "_") || strings.HasSuffix(query, "_") {
		return false
	}
	// Slash-delimited prose concepts are better searched through the retained
	// lexical fragments. A dotted import/module spelling remains useful as an
	// exact query, while strings such as "generic/slice" otherwise consume a
	// scarce grep slot without resembling source syntax.
	if strings.Contains(query, "/") && !strings.Contains(query, ".") {
		return false
	}
	return true
}

func gitText(ctx context.Context, repo string, args ...string) (string, error) {
	command := isolatedGitCommand(ctx, repo, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("task lens: git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func isolatedGitCommand(ctx context.Context, repo string, args ...string) *exec.Cmd {
	commandArgs := []string{
		"--no-pager",
		"-c", "core.fsmonitor=false",
		"-c", "core.hooksPath=" + os.DevNull,
		"-C", repo,
	}
	commandArgs = append(commandArgs, args...)
	command := exec.CommandContext(ctx, "git", commandArgs...)
	command.Env = isolatedGitEnvironment(os.Environ())
	return command
}

func isolatedGitEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment)+4)
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR",
			"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
			"GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS",
			"GIT_CONFIG_SYSTEM", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM",
			"GIT_EXTERNAL_DIFF", "GIT_PAGER", "PAGER":
			continue
		}
		if strings.HasPrefix(name, "GIT_CONFIG_KEY_") || strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			continue
		}
		result = append(result, entry)
	}
	return append(result,
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_PAGER=cat",
		"PAGER=cat",
	)
}
