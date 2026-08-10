package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/gotarget"
	"github.com/dvordrova/repomap/internal/reporead"
)

type Options struct {
	RepoPath                      string
	GoTarget                      string
	MaxReadmeBytes                int
	MaxTreeLines                  int
	MaxInterestingFiles           int
	MaxGoPkgs                     int
	MaxGoEdges                    int
	AnalysisTargetOverride        string
	DeferAnalysisTargetResolution bool
}

type Snapshot struct {
	// RepoName is a semantic repository identity derived from repository
	// metadata. It must not depend on the local checkout directory name.
	RepoName string `json:"repo_name"`
	// DisplayName is local presentation copy only. Provider bundles deliberately
	// omit it because temporary checkout names can contain task labels.
	DisplayName        string                        `json:"display_name,omitempty"`
	Readme             string                        `json:"readme"`
	FileTree           []string                      `json:"file_tree"`
	TopLevelStats      map[string]int                `json:"top_level_directory_stats"`
	LanguageHints      []LanguageHint                `json:"detected_language_hints"`
	InterestingFiles   []string                      `json:"interesting_files"`
	Go                 GoHints                       `json:"go_hints"`
	GoFacts            *gofacts.Facts                `json:"go_facts,omitempty"`
	AnalysisTarget     *analysistarget.Target        `json:"analysis_target,omitempty"`
	FilesConsidered    int                           `json:"files_considered"`
	FilesSkipped       int                           `json:"files_skipped"`
	SkippedPathSamples []string                      `json:"skipped_path_samples"`
	FilteredFiles      []string                      `json:"-"`
	GoTargetAdvisory   *GoTargetAdvisory             `json:"-"`
	TargetCatalog      *analysistarget.TargetCatalog `json:"-"`
}

type LanguageHint struct {
	Language string `json:"language"`
	Count    int    `json:"count"`
}

type GoHints struct {
	GoModExists       bool     `json:"go_mod_exists"`
	ModuleName        string   `json:"module_name,omitempty"`
	LikelyEntrypoints []string `json:"likely_entrypoints"`
	ImportantGoFiles  []string `json:"important_go_files"`
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

var interestingWords = []string{
	"server", "handler", "grpc", "http", "cli", "cobra",
	"storage", "store", "db", "database", "repository", "repo", "migration",
	"consumer", "producer", "kafka", "queue", "pubsub", "event",
	"config", "env", "flag", "viper",
	"worker", "scheduler", "cron", "job",
}

func Build(opts Options) (Snapshot, error) {
	return BuildContext(context.Background(), opts)
}

func BuildContext(ctx context.Context, opts Options) (Snapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if opts.DeferAnalysisTargetResolution && strings.TrimSpace(opts.AnalysisTargetOverride) != "" {
		return Snapshot{}, fmt.Errorf("analysis target resolution cannot be deferred with explicit override %q", opts.AnalysisTargetOverride)
	}
	if opts.MaxReadmeBytes <= 0 {
		opts.MaxReadmeBytes = 20000
	}
	if opts.MaxTreeLines <= 0 {
		opts.MaxTreeLines = 400
	}
	if opts.MaxInterestingFiles <= 0 {
		opts.MaxInterestingFiles = 200
	}
	listing, err := gitfiles.ListWithModesContext(ctx, opts.RepoPath)
	if err != nil {
		return Snapshot{}, err
	}
	files := listing.Paths
	regular := make(map[string]struct{}, len(listing.RegularPaths))
	for _, filePath := range listing.RegularPaths {
		regular[filePath] = struct{}{}
	}

	filtered := make([]string, 0, len(files))
	analysisFiles := make([]string, 0, len(listing.RegularPaths))
	skippedSamples := make([]string, 0, 20)
	for _, f := range files {
		if shouldSkipPath(f) {
			if len(skippedSamples) < 20 {
				skippedSamples = append(skippedSamples, f)
			}
			continue
		}
		filtered = append(filtered, f)
		if _, ok := regular[f]; ok {
			analysisFiles = append(analysisFiles, f)
		}
	}

	sort.Strings(filtered)
	sort.Strings(analysisFiles)
	currentTarget := gotarget.Host()
	if opts.GoTarget != "" {
		if parsed, parseErr := gotarget.Parse(opts.GoTarget); parseErr == nil {
			currentTarget = parsed
		}
	}
	goMetadata := goHints(opts.RepoPath, analysisFiles)
	s := Snapshot{
		RepoName:           repositoryIdentity(opts.RepoPath, filtered, goMetadata),
		DisplayName:        repositoryDisplayName(opts.RepoPath),
		FileTree:           takeFirst(filtered, opts.MaxTreeLines),
		TopLevelStats:      topLevelStats(filtered),
		LanguageHints:      detectLanguages(filtered),
		InterestingFiles:   findInterestingFiles(filtered, opts.MaxInterestingFiles),
		FilesConsidered:    len(filtered),
		FilesSkipped:       len(files) - len(filtered),
		SkippedPathSamples: skippedSamples,
		FilteredFiles:      analysisFiles,
		GoTargetAdvisory:   detectGoTargetAdvisory(opts.RepoPath, analysisFiles, currentTarget),
		Go:                 goMetadata,
	}

	s.Readme = readReadme(opts.RepoPath, analysisFiles, opts.MaxReadmeBytes)

	if s.Go.GoModExists || hasGoFiles(analysisFiles) {
		facts, err := gofacts.LoadWithOptions(
			ctx,
			opts.RepoPath,
			analysisFiles,
			opts.MaxGoPkgs,
			opts.MaxGoEdges,
			gofacts.LoadOptions{GoTarget: opts.GoTarget},
		)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Snapshot{}, ctxErr
			}
			s.GoFacts = &gofacts.Facts{
				Coverage: gofacts.Coverage{State: gofacts.CoverageUnavailable},
				Warnings: []string{fmt.Sprintf("go facts load failed: %v", err)},
			}
		} else {
			s.GoFacts = facts
		}
		if s.GoFacts != nil && len(s.GoFacts.Packages) > 0 && opts.DeferAnalysisTargetResolution {
			catalog, catalogErr := analysistarget.BuildCatalog(*s.GoFacts)
			if catalogErr != nil {
				return Snapshot{}, fmt.Errorf("build analysis target catalog: %w", catalogErr)
			}
			ownedCatalog := catalog.Snapshot()
			s.TargetCatalog = &ownedCatalog
		} else if s.GoFacts != nil && len(s.GoFacts.Packages) > 0 {
			resolution, resolveErr := analysistarget.Resolve(*s.GoFacts, analysistarget.Options{Override: opts.AnalysisTargetOverride})
			if resolveErr != nil {
				return Snapshot{}, fmt.Errorf("resolve analysis target: %w", resolveErr)
			}
			switch resolution.State {
			case analysistarget.ResolutionSelected:
				scoped, scopeErr := analysistarget.ScopeGoFacts(*s.GoFacts, *resolution.Selected)
				if scopeErr != nil {
					return Snapshot{}, scopeErr
				}
				target := resolution.Selected.Snapshot()
				s.AnalysisTarget = &target
				s.GoFacts = &scoped
				s.FilteredFiles = analysisTargetFiles(scoped, analysisFiles)
			case analysistarget.ResolutionAmbiguous:
				return Snapshot{}, fmt.Errorf("analysis target is ambiguous; choose one target with --target: %s", analysisTargetCandidateKeys(resolution.Candidates))
			}
		}
		if strings.TrimSpace(opts.AnalysisTargetOverride) != "" && s.AnalysisTarget == nil {
			return Snapshot{}, fmt.Errorf("resolve analysis target: explicit override %q requires available exact Go target facts", opts.AnalysisTargetOverride)
		}
	}

	return s, nil
}

// ScopeAnalysisTarget applies one exact target from a live deferred catalog.
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

func analysisTargetCandidateKeys(candidates []analysistarget.Candidate) string {
	const maxKeys = 12
	displayed := candidates
	executables := make([]analysistarget.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.MainModule && candidate.Target.Kind == analysistarget.KindExecutablePackage {
			executables = append(executables, candidate)
		}
	}
	if len(executables) > 0 {
		displayed = executables
	}
	keys := make([]string, 0, min(len(displayed), maxKeys))
	for _, candidate := range displayed {
		if len(keys) == maxKeys {
			break
		}
		key := candidate.Target.DisplayPath()
		if key == "." {
			if candidate.Target.Kind == analysistarget.KindModuleLibrary {
				key = candidate.Target.ModulePath
			} else {
				key = candidate.Target.PackagePath
			}
		}
		keys = append(keys, key)
	}
	if len(displayed) > len(keys) {
		keys = append(keys, fmt.Sprintf("... and %d more", len(displayed)-len(keys)))
	}
	return strings.Join(keys, ", ")
}

func (s Snapshot) JSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

func readReadme(repoPath string, trackedFiles []string, maxBytes int) string {
	reader, err := reporead.New(repoPath)
	if err != nil {
		return ""
	}
	defer reader.Close()

	tracked := make(map[string]struct{}, len(trackedFiles))
	for _, path := range trackedFiles {
		tracked[path] = struct{}{}
	}

	candidates := []string{"README.md", "README", "readme.md", "Readme.md"}
	for _, name := range candidates {
		if _, ok := tracked[name]; !ok {
			continue
		}
		content, err := reader.ReadFile(name, int64(maxBytes))
		if err != nil {
			continue
		}
		truncated, invalidBoundary := truncateUTF8Bytes(string(content.Bytes), maxBytes)
		if content.Truncated || invalidBoundary {
			return truncated + "\n...[truncated]"
		}
		return truncated
	}
	return ""
}

func truncateUTF8Bytes(s string, maxBytes int) (string, bool) {
	if maxBytes <= 0 {
		return "", len(s) > 0
	}
	if len(s) <= maxBytes && utf8.ValidString(s) {
		return s, false
	}
	cut := maxBytes
	if len(s) < cut {
		cut = len(s)
	}
	for cut > 0 && !utf8.ValidString(s[:cut]) {
		cut--
	}
	if cut == 0 {
		return "", true
	}
	return s[:cut], true
}

func topLevelStats(files []string) map[string]int {
	stats := map[string]int{}
	for _, f := range files {
		p := strings.SplitN(f, "/", 2)
		key := "."
		if len(p) > 1 {
			key = p[0]
		}
		stats[key]++
	}
	return stats
}

func detectLanguages(files []string) []LanguageHint {
	extToLang := map[string]string{
		".go":    "Go",
		".py":    "Python",
		".js":    "JavaScript",
		".ts":    "TypeScript",
		".tsx":   "TypeScript",
		".jsx":   "JavaScript",
		".java":  "Java",
		".rs":    "Rust",
		".c":     "C",
		".h":     "C/C++ Header",
		".cpp":   "C++",
		".cc":    "C++",
		".rb":    "Ruby",
		".php":   "PHP",
		".sh":    "Shell",
		".yaml":  "YAML",
		".yml":   "YAML",
		".json":  "JSON",
		".md":    "Markdown",
		".proto": "Protocol Buffers",
	}

	counts := map[string]int{}
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f))
		if lang, ok := extToLang[ext]; ok {
			counts[lang]++
		}
	}

	hints := make([]LanguageHint, 0, len(counts))
	for lang, count := range counts {
		hints = append(hints, LanguageHint{Language: lang, Count: count})
	}
	sort.Slice(hints, func(i, j int) bool {
		if hints[i].Count == hints[j].Count {
			return hints[i].Language < hints[j].Language
		}
		return hints[i].Count > hints[j].Count
	})
	return hints
}

func findInterestingFiles(files []string, max int) []string {
	seen := map[string]struct{}{}
	add := func(dst []string, path string) []string {
		if len(dst) >= max {
			return dst
		}
		if _, ok := seen[path]; ok {
			return dst
		}
		seen[path] = struct{}{}
		return append(dst, path)
	}

	out := make([]string, 0, max)
	priorityNames := []string{
		"README.md", "go.mod", "pyproject.toml", "setup.py", "setup.cfg",
		"requirements.txt", "Makefile", "Dockerfile", ".gitignore",
	}

	for _, p := range files {
		base := filepath.Base(p)
		for _, n := range priorityNames {
			if base == n {
				out = add(out, p)
			}
		}
	}

	for _, p := range files {
		l := strings.ToLower(p)
		if strings.HasPrefix(l, "cmd/") || strings.Contains(l, "/cmd/") ||
			strings.HasPrefix(l, "internal/") || strings.HasPrefix(l, "pkg/") {
			out = add(out, p)
		}
	}

	for _, p := range files {
		l := strings.ToLower(filepath.Base(p))
		for _, w := range interestingWords {
			if strings.Contains(l, w) {
				out = add(out, preferProtoFile(p, files))
				break
			}
		}
	}
	return out
}

func goHints(repoPath string, files []string) GoHints {
	h := GoHints{}
	for _, f := range files {
		if f == "go.mod" {
			reader, err := reporead.New(repoPath)
			if err != nil {
				break
			}
			content, readErr := reader.ReadFile("go.mod", 1024*1024)
			_ = reader.Close()
			if readErr == nil && !content.Truncated {
				h.GoModExists = true
				h.ModuleName = parseModuleName(content.Bytes)
			}
			break
		}
	}

	entrySet := map[string]struct{}{}
	important := make([]string, 0, 32)

	for _, f := range files {
		if !strings.HasSuffix(strings.ToLower(f), ".go") {
			continue
		}
		l := strings.ToLower(f)
		if strings.HasPrefix(l, "cmd/") && filepath.Base(l) == "main.go" {
			entrySet[f] = struct{}{}
		}
		base := strings.ToLower(filepath.Base(f))
		for _, w := range interestingWords {
			if strings.Contains(base, w) {
				important = append(important, f)
				break
			}
		}
	}

	h.LikelyEntrypoints = sortedSet(entrySet)
	sort.Strings(important)
	if len(important) > 200 {
		important = important[:200]
	}
	h.ImportantGoFiles = important
	return h
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

func takeFirst(items []string, n int) []string {
	if n <= 0 || len(items) == 0 {
		return nil
	}
	if len(items) <= n {
		out := make([]string, len(items))
		copy(out, items)
		return out
	}
	out := make([]string, n+1)
	copy(out, items[:n])
	out[n] = fmt.Sprintf("... (%s more)", strconv.Itoa(len(items)-n))
	return out
}

func sortedSet(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func hasGoFiles(files []string) bool {
	for _, f := range files {
		if strings.HasSuffix(strings.ToLower(f), ".go") {
			return true
		}
	}
	return false
}

func preferProtoFile(path string, files []string) string {
	if !strings.HasSuffix(path, ".pb.go") {
		return path
	}
	proto := path[:len(path)-6] + ".proto"
	for _, f := range files {
		if f == proto {
			return proto
		}
	}
	return path
}
