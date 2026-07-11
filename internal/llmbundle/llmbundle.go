package llmbundle

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/snapshot"
)

type Bundle struct {
	RepoName               string                 `json:"repo_name"`
	ReadmeExcerpt          string                 `json:"readme_excerpt"`
	TopLevelDirectoryStats map[string]int         `json:"top_level_directory_stats"`
	LanguageHints          []snapshot.LanguageHint `json:"language_hints"`
	Go                     goSection              `json:"go"`
	KnownDocs              []string               `json:"known_docs"`
	CandidateFileIndex     []fileIndexEntry       `json:"candidate_file_index"`
	AllowedPaths           []string               `json:"allowed_paths"`
	Warnings               []string               `json:"warnings,omitempty"`
}

type fileIndexEntry struct {
	Path    string   `json:"path"`
	Kind    string   `json:"kind"`
	Signals []string `json:"signals"`
	Score   int      `json:"score"`
	Reasons []string `json:"reasons"`
}

type goSection struct {
	ModulesCount          int                            `json:"modules_count"`
	PackagesCount         int                            `json:"packages_count"`
	ModuleSummaries       []moduleSummaryCompact         `json:"module_summaries"`
	Entrypoints           []entrypointCompact            `json:"entrypoints"`
	OrientationCandidates []gofacts.OrientationCandidate `json:"orientation_candidates"`
	ImportantEdges        []gofacts.Edge                 `json:"important_edges"`
}

type moduleSummaryCompact struct {
	ModulePath              string              `json:"module_path"`
	ModuleDir               string              `json:"module_dir"`
	PackagesCount           int                 `json:"packages_count"`
	EntrypointsCount        int                 `json:"entrypoints_count"`
	RoleGuess               string              `json:"role_guess"`
	TopImportedInternalPkgs []string            `json:"top_imported_internal_packages"`
	TopExternalImports      []gofacts.ExtImport `json:"top_external_imports"`
}

type entrypointCompact struct {
	Kind        string   `json:"kind"`
	ImportPath  string   `json:"import_path"`
	PackageDir  string   `json:"package_dir"`
	OpenFiles   []string `json:"open_files"`
	GoFiles     []string `json:"go_files"`
}

type Options struct {
	MaxReadmeBytes int
	MaxModules     int
	MaxEntrypoints int
	MaxFiles       int
	MaxEdges       int
}

func defaults(opts Options) Options {
	if opts.MaxReadmeBytes <= 0 {
		opts.MaxReadmeBytes = 6000
	}
	if opts.MaxModules <= 0 {
		opts.MaxModules = 20
	}
	if opts.MaxEntrypoints <= 0 {
		opts.MaxEntrypoints = 20
	}
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = 150
	}
	if opts.MaxEdges <= 0 {
		opts.MaxEdges = 120
	}
	return opts
}

func Build(s snapshot.Snapshot, fileList []string, opts Options) Bundle {
	opts = defaults(opts)

	b := Bundle{
		RepoName:               s.RepoName,
		ReadmeExcerpt:          truncateStr(s.Readme, opts.MaxReadmeBytes),
		TopLevelDirectoryStats: s.TopLevelStats,
		LanguageHints:          s.LanguageHints,
		KnownDocs:              findKnownDocs(fileList),
	}

	if s.GoFacts != nil {
		f := s.GoFacts

		modSummaries := make([]moduleSummaryCompact, 0, len(f.ModuleSummaries))
		for _, ms := range f.ModuleSummaries {
			modSummaries = append(modSummaries, moduleSummaryCompact{
				ModulePath:              ms.ModulePath,
				ModuleDir:               ms.ModuleDir,
				PackagesCount:           ms.PackagesCount,
				EntrypointsCount:        ms.EntrypointsCount,
				RoleGuess:               ms.RoleGuess,
				TopImportedInternalPkgs: ms.TopImportedInternalPkgs,
				TopExternalImports:      ms.TopExternalImports,
			})
		}
		if len(modSummaries) > opts.MaxModules {
			b.Warnings = append(b.Warnings, "truncated module summaries")
			modSummaries = modSummaries[:opts.MaxModules]
		}

		eps := make([]entrypointCompact, 0, len(f.EntrypointPackages))
		for _, ep := range f.EntrypointPackages {
			openFiles := make([]string, 0, len(ep.GoFiles))
			for _, gf := range ep.GoFiles {
				if ep.PackageDir == "." || ep.PackageDir == "" {
					openFiles = append(openFiles, gf)
				} else {
					openFiles = append(openFiles, ep.PackageDir+"/"+gf)
				}
			}
			eps = append(eps, entrypointCompact{
				Kind:       ep.Kind,
				ImportPath: ep.ImportPath,
				PackageDir: ep.PackageDir,
				OpenFiles:  openFiles,
				GoFiles:    ep.GoFiles,
			})
		}
		if len(eps) > opts.MaxEntrypoints {
			b.Warnings = append(b.Warnings, "truncated entrypoints")
			eps = eps[:opts.MaxEntrypoints]
		}

		candidates := f.OrientationCandidates
		if len(candidates) > opts.MaxFiles {
			b.Warnings = append(b.Warnings, "truncated orientation candidates")
			candidates = candidates[:opts.MaxFiles]
		}

		edges := f.InternalEdges
		if len(edges) > opts.MaxEdges {
			b.Warnings = append(b.Warnings, "truncated important edges")
			edges = edges[:opts.MaxEdges]
		}

		b.Go = goSection{
			ModulesCount:          len(f.Modules),
			PackagesCount:         f.PackagesCount,
			ModuleSummaries:       modSummaries,
			Entrypoints:           eps,
			OrientationCandidates: candidates,
			ImportantEdges:        edges,
		}

		fileIndex := buildFileIndex(fileList, s.GoFacts, b.KnownDocs)
		if len(fileIndex) > opts.MaxFiles {
			b.Warnings = append(b.Warnings, "truncated candidate_file_index")
			fileIndex = fileIndex[:opts.MaxFiles]
		}

		b.CandidateFileIndex = fileIndex
		b.AllowedPaths = buildAllowedPaths(fileIndex)
	}

	return b
}

func buildFileIndex(fileList []string, facts *gofacts.Facts, knownDocs []string) []fileIndexEntry {
	seen := make(map[string]struct{})
	var entries []fileIndexEntry

	entrypointPaths := make(map[string]struct{})
	if facts != nil {
		for _, ep := range facts.EntrypointPackages {
			for _, gf := range ep.GoFiles {
				p := gf
				if ep.PackageDir != "." && ep.PackageDir != "" {
					p = ep.PackageDir + "/" + gf
				}
				entrypointPaths[p] = struct{}{}
			}
		}
		for _, oc := range facts.OrientationCandidates {
			for _, of := range oc.OpenFiles {
				entrypointPaths[of] = struct{}{}
			}
		}
	}

	knownDocSet := make(map[string]struct{})
	for _, d := range knownDocs {
		knownDocSet[d] = struct{}{}
	}

	for _, f := range fileList {
		seen[f] = struct{}{}
		kind := detectFileKind(f)
		score, signals, reasons := scoreFile(f, kind, entrypointPaths, knownDocSet)

		if score <= 0 {
			continue
		}

		entries = append(entries, fileIndexEntry{
			Path:    f,
			Kind:    kind,
			Signals: signals,
			Score:   score,
			Reasons: reasons,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Score == entries[j].Score {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Score > entries[j].Score
	})

	return entries
}

func detectFileKind(path string) string {
	lower := strings.ToLower(path)
	base := strings.ToLower(filepath.Base(path))

	if strings.HasSuffix(base, "_test.go") {
		return "test"
	}
	if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".drawio") {
		return "doc"
	}
	if strings.HasSuffix(lower, ".proto") {
		return "proto"
	}
	if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") ||
		strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".toml") ||
		strings.HasSuffix(lower, ".conf") || strings.HasSuffix(lower, ".sample") {
		return "config"
	}
	if strings.HasSuffix(base, ".pb.go") {
		return "generated"
	}
	if strings.HasSuffix(lower, ".go") {
		return "source"
	}
	return "unknown"
}

func scoreFile(path string, kind string, entrypointPaths, knownDocSet map[string]struct{}) (int, []string, []string) {
	score := 0
	var signals []string
	var reasons []string
	lower := strings.ToLower(path)

	addSignal := func(s string, sScore int, reason string) {
		signals = append(signals, s)
		reasons = append(reasons, reason)
		score += sScore
	}

	if _, ok := entrypointPaths[path]; ok {
		addSignal("entrypoint", 100, "entrypoint source file")
	}

	for _, word := range []string{"server", "etcdserver"} {
		if strings.Contains(lower, word) {
			addSignal(word, 70, word+" path")
			break
		}
	}

	for _, word := range []string{"v3rpc", "lease", "mvcc", "wal", "backend", "rafthttp"} {
		if strings.Contains(lower, word) {
			addSignal(word, 65, word+" component")
			break
		}
	}

	if strings.Contains(lower, "api/") && kind == "proto" {
		addSignal("api-proto", 70, "API proto file")
	}
	if strings.Contains(lower, "etcdserverpb") {
		addSignal("etcdserverpb", 65, "server proto")
	}

	for _, word := range []string{"client", "kv", "watch", "lease", "txn"} {
		if strings.Contains(lower, "client/") && strings.Contains(lower, word) {
			addSignal("client-"+word, 70, "client "+word+" file")
			break
		}
	}

	if strings.Contains(lower, "etcdctl/") && (strings.Contains(lower, "command") || strings.Contains(lower, "main")) {
		addSignal("etcdctl-cmd", 70, "etcdctl command file")
	}

	if _, ok := knownDocSet[path]; ok {
		addSignal("known-doc", 60, "documentation file")

		for _, docWord := range []string{"internals", "workflow", "architecture", "api", "raft", "watch", "write", "read"} {
			if strings.Contains(lower, docWord) {
				addSignal("doc-"+docWord, 10, "documentation about "+docWord)
				break
			}
		}
	}

	if kind == "test" {
		addSignal("test", 40, "test file near selected packages")
	}

	if kind == "proto" {
		addSignal("proto", 50, "protocol buffer file")
	}

	if kind == "source" {
		addSignal("source", 30, "Go source file")
	}

	if kind == "generated" {
		addSignal("generated", 10, "generated file (low priority)")
	}

	if kind == "config" {
		addSignal("config", 20, "configuration file")
	}

	if score <= 10 {
		return 0, nil, nil
	}

	return score, signals, reasons
}

func buildAllowedPaths(index []fileIndexEntry) []string {
	paths := make([]string, len(index))
	for i, e := range index {
		paths[i] = e.Path
	}
	sort.Strings(paths)
	return paths
}

func findKnownDocs(files []string) []string {
	interestingPatterns := []string{
		"Documentation/",
		"docs/",
		"doc/",
		"README",
	}

	interestingWords := []string{
		"architecture", "workflow", "internals", "design",
		"overview", "contributing", "changelog",
	}

	seen := make(map[string]struct{})
	var docs []string

	for _, f := range files {
		lower := strings.ToLower(f)

		matched := false
		for _, pat := range interestingPatterns {
			if strings.Contains(lower, strings.ToLower(pat)) {
				matched = true
				break
			}
		}
		if !matched {
			ext := strings.ToLower(f)
			if strings.HasSuffix(ext, ".md") || strings.HasSuffix(ext, ".drawio") {
				for _, w := range interestingWords {
					if strings.Contains(lower, w) {
						matched = true
						break
					}
				}
			}
		}
		if !matched {
			continue
		}

		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		docs = append(docs, f)
	}

	sort.Strings(docs)

	if len(docs) > 30 {
		docs = docs[:30]
	}

	return docs
}

func truncateStr(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 {
		if cut >= len(s) {
			break
		}
		if s[cut]&0xC0 != 0x80 {
			break
		}
		cut--
	}
	if cut == 0 {
		return ""
	}
	return s[:cut] + "\n...[truncated]"
}
