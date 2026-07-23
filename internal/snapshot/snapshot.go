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

	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/reporead"
)

type Options struct {
	RepoPath            string
	MaxReadmeBytes      int
	MaxTreeLines        int
	MaxInterestingFiles int
	MaxGoPkgs           int
	MaxGoEdges          int
}

type Snapshot struct {
	// RepoName is a semantic repository identity derived from repository
	// metadata. It must not depend on the local checkout directory name.
	RepoName string `json:"repo_name"`
	// DisplayName is local presentation copy only. Provider bundles deliberately
	// omit it because temporary checkout names can contain task labels.
	DisplayName        string         `json:"display_name,omitempty"`
	Readme             string         `json:"readme"`
	FileTree           []string       `json:"file_tree"`
	TopLevelStats      map[string]int `json:"top_level_directory_stats"`
	LanguageHints      []LanguageHint `json:"detected_language_hints"`
	InterestingFiles   []string       `json:"interesting_files"`
	Go                 GoHints        `json:"go_hints"`
	GoFacts            *gofacts.Facts `json:"go_facts,omitempty"`
	FilesConsidered    int            `json:"files_considered"`
	FilesSkipped       int            `json:"files_skipped"`
	SkippedPathSamples []string       `json:"skipped_path_samples"`
	FilteredFiles      []string       `json:"-"`
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
	if opts.MaxReadmeBytes <= 0 {
		opts.MaxReadmeBytes = 20000
	}
	if opts.MaxTreeLines <= 0 {
		opts.MaxTreeLines = 400
	}
	if opts.MaxInterestingFiles <= 0 {
		opts.MaxInterestingFiles = 200
	}
	if opts.MaxGoPkgs <= 0 {
		opts.MaxGoPkgs = 300
	}
	if opts.MaxGoEdges <= 0 {
		opts.MaxGoEdges = 500
	}

	files, err := gitfiles.List(opts.RepoPath)
	if err != nil {
		return Snapshot{}, err
	}

	filtered := make([]string, 0, len(files))
	skippedSamples := make([]string, 0, 20)
	for _, f := range files {
		if shouldSkipPath(f) {
			if len(skippedSamples) < 20 {
				skippedSamples = append(skippedSamples, f)
			}
			continue
		}
		filtered = append(filtered, f)
	}

	sort.Strings(filtered)
	goMetadata := goHints(opts.RepoPath, filtered)
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
		FilteredFiles:      filtered,
		Go:                 goMetadata,
	}

	s.Readme = readReadme(opts.RepoPath, filtered, opts.MaxReadmeBytes)

	if s.Go.GoModExists || hasGoFiles(filtered) {
		facts, err := gofacts.Load(context.Background(), opts.RepoPath, filtered, opts.MaxGoPkgs, opts.MaxGoEdges)
		if err != nil {
			s.GoFacts = &gofacts.Facts{
				Warnings: []string{fmt.Sprintf("go facts load failed: %v", err)},
			}
		} else {
			s.GoFacts = facts
		}
	}

	return s, nil
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
