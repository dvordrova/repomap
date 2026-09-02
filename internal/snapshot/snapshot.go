package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/gotarget"
)

type Options struct {
	RepoPath string
	GoTarget string
	// BuildTags is one canonical run-wide Go build selection shared with the
	// later typed-program load.
	BuildTags []string
	// GoModuleDir is a pre-load routing hint derived only from an exact typed
	// --target candidate key. ScopeAnalysisTarget still resolves that complete
	// key against the resulting catalog before it becomes authority.
	GoModuleDir string
	// RepositoryCorpus is the one run-local tracked-file namespace shared by
	// every initial discovery cube. Ordinary main constructs it once and every
	// caller must pass that exact instance; BuildContext never inventories a
	// second namespace behind the caller's back.
	RepositoryCorpus *corpus.Corpus
	// SkipGoFacts is an explicit language-routing decision for a Python-only
	// ordinary run. It prevents incidental Go files without a Go module from
	// activating the Go adapter. A requested Go load is otherwise mandatory and
	// fail-closed.
	SkipGoFacts bool
	// AutoGoTarget allows the exhaustive tracked-file platform preflight to
	// replace the caller's host target with one unique strong production
	// alternative before Go facts are loaded. Callers must leave this false
	// whenever --force-platform or either standard Go target environment
	// dimension was supplied explicitly.
	AutoGoTarget bool
}

type Snapshot struct {
	// RepoName is a semantic repository identity derived from repository
	// metadata. It must not depend on the local checkout directory name.
	RepoName string `json:"repo_name"`
	// DisplayName is local presentation copy only. Provider bundles deliberately
	// omit it because temporary checkout names can contain task labels.
	DisplayName            string                        `json:"display_name,omitempty"`
	GoFacts                *gofacts.Facts                `json:"go_facts,omitempty"`
	AnalysisTarget         *analysistarget.Target        `json:"analysis_target,omitempty"`
	FilesConsidered        int                           `json:"files_considered"`
	FilteredFiles          []string                      `json:"-"`
	GoTargetAdvisory       *GoTargetAdvisory             `json:"-"`
	GoTargetSelection      *GoTargetSelection            `json:"-"`
	TargetCatalog          *analysistarget.TargetCatalog `json:"-"`
	repositoryScaleMetrics repositoryScaleMetrics
}

var skipDirPrefixes = []string{
	".git/",
	".github/",
	"vendor/",
	"node_modules/",
	"dist/",
	"build/",
	"coverage/",
}

var skipFileExt = map[string]struct{}{
	".png":   {},
	".jpg":   {},
	".jpeg":  {},
	".gif":   {},
	".webp":  {},
	".ico":   {},
	".bmp":   {},
	".svg":   {},
	".zip":   {},
	".tar":   {},
	".gz":    {},
	".tgz":   {},
	".7z":    {},
	".rar":   {},
	".jar":   {},
	".pdf":   {},
	".mp4":   {},
	".mp3":   {},
	".wav":   {},
	".mov":   {},
	".ttf":   {},
	".woff":  {},
	".woff2": {},
	".eot":   {},
	".o":     {},
	".a":     {},
	".so":    {},
	".dylib": {},
	".dll":   {},
	".exe":   {},
	".bin":   {},
	".class": {},
}

func BuildContext(ctx context.Context, opts Options) (Snapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	repositoryCorpus := opts.RepositoryCorpus
	if repositoryCorpus == nil {
		return Snapshot{}, fmt.Errorf("repository corpus is required")
	}
	corpusSnapshot := repositoryCorpus.Snapshot()
	if err := corpusSnapshot.Validate(); err != nil {
		return Snapshot{}, fmt.Errorf("repository corpus: %w", err)
	}
	entries := repositoryCorpus.Entries()
	files := repositoryCorpus.VisiblePaths()
	regular := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		regular[entry.Path] = struct{}{}
	}

	filtered := make([]string, 0, len(files))
	analysisFiles := make([]string, 0, len(files))
	for _, f := range files {
		if shouldSkipPath(f) {
			continue
		}
		filtered = append(filtered, f)
		if _, ok := regular[f]; ok {
			analysisFiles = append(analysisFiles, f)
		}
	}

	sort.Strings(filtered)
	sort.Strings(analysisFiles)
	if strings.TrimSpace(opts.GoTarget) == "" {
		return Snapshot{}, fmt.Errorf("resolved Go target is required")
	}
	currentTarget, err := gotarget.Parse(opts.GoTarget)
	if err != nil {
		return Snapshot{}, fmt.Errorf("restore resolved Go target: %w", err)
	}
	goModExists, goModuleName, goModuleBytes := goModuleMetadata(repositoryCorpus, analysisFiles)
	advisory, scaleMetrics := detectGoTargetAdvisory(opts.RepoPath, analysisFiles, currentTarget)
	repoName, manifestBytes := repositoryIdentity(opts.RepoPath, repositoryCorpus, goModuleName)
	scaleMetrics.goModuleBytes = goModuleBytes
	scaleMetrics.manifestBytes = manifestBytes
	var goTargetSelection *GoTargetSelection
	if opts.AutoGoTarget && advisory != nil {
		selected, selectErr := newAutomaticGoTargetSelection(currentTarget, *advisory)
		if selectErr != nil {
			return Snapshot{}, selectErr
		}
		goTargetSelection = &selected
		opts.GoTarget = selected.Target
	}
	s := Snapshot{
		RepoName:               repoName,
		DisplayName:            repositoryDisplayName(opts.RepoPath),
		FilesConsidered:        len(filtered),
		FilteredFiles:          analysisFiles,
		GoTargetAdvisory:       advisory,
		GoTargetSelection:      goTargetSelection,
		repositoryScaleMetrics: scaleMetrics,
	}

	if !opts.SkipGoFacts && (goModExists || hasGoFiles(analysisFiles)) {
		facts, err := gofacts.LoadWithOptions(
			ctx,
			opts.RepoPath,
			analysisFiles,
			gofacts.LoadOptions{
				GoTarget: opts.GoTarget, BuildTags: opts.BuildTags,
				ModuleDir: opts.GoModuleDir,
			},
		)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Snapshot{}, ctxErr
			}
			return Snapshot{}, fmt.Errorf("load exact Go facts: %w", err)
		}
		s.GoFacts = facts
		if s.GoFacts != nil {
			catalog, catalogErr := analysistarget.BuildCatalog(*s.GoFacts)
			if catalogErr != nil {
				return Snapshot{}, fmt.Errorf("build analysis target catalog: %w", catalogErr)
			}
			ownedCatalog := catalog.Snapshot()
			s.TargetCatalog = &ownedCatalog
		}
	}

	return s, nil
}

// ScopeAnalysisTarget applies one exact target from a fresh unselected catalog.
// Callers provide only the self-sealed target ref; arbitrary Target values are
// never accepted as authority. A successful application consumes the catalog
// seam and restores the ordinary single-target snapshot contract.
func ScopeAnalysisTarget(s Snapshot, targetRef string) (Snapshot, error) {
	if targetRef == "" || targetRef != strings.TrimSpace(targetRef) {
		return Snapshot{}, fmt.Errorf("scope analysis target: target ref must be exact and non-empty")
	}
	if s.TargetCatalog == nil {
		return Snapshot{}, fmt.Errorf("scope analysis target: live target catalog is unavailable")
	}
	if err := s.TargetCatalog.Validate(); err != nil {
		return Snapshot{}, fmt.Errorf("scope analysis target: validate live target catalog: %w", err)
	}
	if s.GoFacts == nil {
		return Snapshot{}, fmt.Errorf("scope analysis target: complete Go facts are unavailable")
	}

	var selected *analysistarget.Target
	for _, entry := range s.TargetCatalog.Entries {
		if entry.Candidate.Target.Ref != targetRef {
			continue
		}
		target := entry.Candidate.Target.Snapshot()
		selected = &target
		break
	}
	if selected == nil {
		return Snapshot{}, fmt.Errorf("scope analysis target: unknown target ref %q", targetRef)
	}

	scoped, err := analysistarget.ScopeGoFacts(*s.GoFacts, *selected)
	if err != nil {
		return Snapshot{}, fmt.Errorf("scope analysis target: %w", err)
	}
	files := analysisTargetFiles(scoped, s.FilteredFiles)
	s.AnalysisTarget = selected
	s.GoFacts = &scoped
	s.FilteredFiles = files
	s.TargetCatalog = nil
	return s, nil
}

func analysisTargetFiles(facts gofacts.Facts, repositoryFiles []string) []string {
	allowed := make(map[string]struct{})
	for _, pkg := range facts.Packages {
		for _, file := range pkg.Files {
			allowed[filepath.ToSlash(filepath.Clean(file))] = struct{}{}
		}
	}
	for _, module := range facts.Modules {
		if module.GoMod != "" {
			allowed[filepath.ToSlash(filepath.Clean(module.GoMod))] = struct{}{}
		}
	}
	// Root documentation is shared product context, not executable source.
	for _, name := range []string{"README.md", "README", "readme.md", "Readme.md"} {
		allowed[name] = struct{}{}
	}
	result := make([]string, 0, len(allowed))
	for _, file := range repositoryFiles {
		if _, keep := allowed[file]; keep {
			result = append(result, file)
		}
	}
	return result
}

func (s Snapshot) JSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

func goModuleMetadata(repositoryCorpus *corpus.Corpus, files []string) (bool, string, int) {
	for _, f := range files {
		if f == "go.mod" {
			fileID, ok := repositoryCorpus.ID(f)
			if !ok {
				return false, "", 0
			}
			content, readErr := repositoryCorpus.ReadFileAll(fileID)
			if readErr == nil {
				return true, parseModuleName(content.Bytes), len(content.Bytes)
			}
			return false, "", 0
		}
	}
	return false, "", 0
}

func parseModuleName(goMod []byte) string {
	for _, rawLine := range strings.Split(string(goMod), "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func shouldSkipPath(path string) bool {
	p := filepath.ToSlash(path)
	l := strings.ToLower(p)

	if strings.HasPrefix(l, ".env") || strings.Contains(l, "/.env") {
		return true
	}
	for _, prefix := range skipDirPrefixes {
		if strings.HasPrefix(l, prefix) || strings.Contains(l, "/"+prefix) {
			return true
		}
	}
	ext := strings.ToLower(filepath.Ext(l))
	if _, ok := skipFileExt[ext]; ok {
		return true
	}
	if strings.HasSuffix(l, ".key") || strings.HasSuffix(l, ".pem") || strings.HasSuffix(l, ".crt") || strings.HasSuffix(l, ".cer") {
		return true
	}
	return false
}

func hasGoFiles(files []string) bool {
	for _, f := range files {
		if strings.HasSuffix(strings.ToLower(f), ".go") {
			return true
		}
	}
	return false
}
