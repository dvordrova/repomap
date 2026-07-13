package llmbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

type Bundle struct {
	RepoName               string                  `json:"repo_name"`
	ReadmeExcerpt          string                  `json:"readme_excerpt"`
	TopLevelDirectoryStats map[string]int          `json:"top_level_directory_stats"`
	LanguageHints          []snapshot.LanguageHint `json:"language_hints"`
	Go                     goSection               `json:"go"`
	KnownDocs              []string                `json:"known_docs"`
	CandidateFileIndex     []fileIndexEntry        `json:"candidate_file_index"`
	// ProviderAllowedPaths is the closed path set for this orientation request.
	// It is not local repository authorization and must not bound local proof or
	// later focused retrieval.
	ProviderAllowedPaths []string               `json:"allowed_paths"`
	SourceSignals        []sourcesignals.Signal `json:"source_signals,omitempty"`
	Warnings             []string               `json:"warnings,omitempty"`
	PolicyVersion        string                 `json:"research_policy_version,omitempty"`
	LocalAuthorizedFiles int                    `json:"local_authorized_file_count,omitempty"`
}

type fileIndexEntry struct {
	ID      string   `json:"id"`
	Path    string   `json:"path"`
	Kind    string   `json:"kind"`
	Signals []string `json:"signals"`
	Score   int      `json:"score"`
	Reasons []string `json:"reasons"`
}

var packageAnchorRoles = map[string]struct{}{
	"client": {}, "controller": {}, "engine": {}, "handler": {},
	"manager": {}, "repository": {}, "runner": {}, "server": {},
	"service": {}, "store": {}, "worker": {},
}

type goSection struct {
	ModulesCount          int                            `json:"modules_count"`
	PackagesCount         int                            `json:"packages_count"`
	ModuleSummaries       []moduleSummaryCompact         `json:"module_summaries"`
	Entrypoints           []entrypointCompact            `json:"entrypoints"`
	CommandTraces         []gofacts.CommandTrace         `json:"command_traces,omitempty"`
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
	Kind       string                     `json:"kind"`
	ImportPath string                     `json:"import_path"`
	PackageDir string                     `json:"package_dir"`
	Anchors    []gofacts.EntrypointAnchor `json:"anchors,omitempty"`
	OpenFiles  []string                   `json:"open_files"`
}

type Options struct {
	MaxReadmeBytes   int
	MaxModules       int
	MaxEntrypoints   int
	MaxFiles         int
	MaxEdges         int
	MaxSignalTotal   int
	MaxSignalPerFile int
	RepoPath         string
	SourceSignals    []sourcesignals.Signal
	MaxBytes         int
	PolicyVersion    string
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
	if opts.MaxSignalTotal <= 0 {
		opts.MaxSignalTotal = 200
	}
	if opts.MaxSignalPerFile <= 0 {
		opts.MaxSignalPerFile = 5
	}
	return opts
}

func Build(s snapshot.Snapshot, fileList []string, opts Options) Bundle {
	opts = defaults(opts)
	maxBytes := opts.MaxBytes
	opts.MaxBytes = 0
	if maxBytes <= 0 {
		return build(s, fileList, opts)
	}

	fit := opts
	var candidate Bundle
	for attempt := 0; attempt < 24; attempt++ {
		candidate = build(s, fileList, fit)
		encoded, err := json.Marshal(candidate)
		if err == nil && len(encoded) <= maxBytes {
			if fit.MaxFiles < opts.MaxFiles || fit.MaxEdges < opts.MaxEdges ||
				fit.MaxSignalTotal < opts.MaxSignalTotal {
				candidate.Warnings = append(candidate.Warnings, "provider bundle fitted to request-byte context budget")
			}
			return candidate
		}
		if !shrinkForByteBudget(&fit) {
			break
		}
	}
	candidate.Warnings = append(candidate.Warnings, "provider bundle exceeds configured context-byte budget")
	return candidate
}

func build(s snapshot.Snapshot, fileList []string, opts Options) Bundle {

	b := Bundle{
		RepoName:               s.RepoName,
		ReadmeExcerpt:          truncateStr(s.Readme, opts.MaxReadmeBytes),
		TopLevelDirectoryStats: s.TopLevelStats,
		LanguageHints:          s.LanguageHints,
		KnownDocs:              findKnownDocs(fileList),
		PolicyVersion:          opts.PolicyVersion,
		LocalAuthorizedFiles:   len(fileList),
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

		selectedEntrypoints := selectOrientationEntrypoints(f.EntrypointPackages)
		selectedEntrypointImports := make(map[string]struct{}, len(selectedEntrypoints))
		eps := make([]entrypointCompact, 0, len(selectedEntrypoints))
		for _, ep := range selectedEntrypoints {
			selectedEntrypointImports[ep.ImportPath] = struct{}{}
			openFiles := make([]string, 0, len(ep.Anchors)+len(ep.GoFiles))
			seenOpenFiles := make(map[string]struct{}, cap(openFiles))
			for _, anchor := range ep.Anchors {
				if _, seen := seenOpenFiles[anchor.Path]; seen {
					continue
				}
				seenOpenFiles[anchor.Path] = struct{}{}
				openFiles = append(openFiles, anchor.Path)
			}
			for _, gf := range ep.GoFiles {
				var openFile string
				if ep.PackageDir == "." || ep.PackageDir == "" {
					openFile = filepath.ToSlash(gf)
				} else {
					openFile = filepath.ToSlash(ep.PackageDir) + "/" + filepath.ToSlash(gf)
				}
				if _, seen := seenOpenFiles[openFile]; seen {
					continue
				}
				seenOpenFiles[openFile] = struct{}{}
				openFiles = append(openFiles, openFile)
			}
			eps = append(eps, entrypointCompact{
				Kind:       ep.Kind,
				ImportPath: ep.ImportPath,
				PackageDir: ep.PackageDir,
				Anchors:    append([]gofacts.EntrypointAnchor(nil), ep.Anchors...),
				OpenFiles:  openFiles,
			})
		}
		if len(eps) > opts.MaxEntrypoints {
			b.Warnings = append(b.Warnings, "truncated entrypoints")
			eps = eps[:opts.MaxEntrypoints]
		}

		candidates := make([]gofacts.OrientationCandidate, 0, len(f.OrientationCandidates))
		for _, candidate := range f.OrientationCandidates {
			_, selectedEntrypoint := selectedEntrypointImports[candidate.EntrypointPackage]
			if candidate.Kind == gofacts.OrientationKindSignalFlow || selectedEntrypoint {
				candidates = append(candidates, candidate)
			}
		}
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
			CommandTraces:         selectCommandTraces(f.CommandTraces, b.ReadmeExcerpt, 8),
			OrientationCandidates: candidates,
			ImportantEdges:        edges,
		}

	}

	// The bounded file index is language-neutral. Go facts enrich its ranking
	// when present, but Python and other repositories still need a closed
	// allowed_paths set for grounded orientation.
	fileSignals := append([]sourcesignals.Signal(nil), opts.SourceSignals...)
	if opts.SourceSignals == nil && opts.RepoPath != "" {
		fileSignals = sourcesignals.ScanFiles(fileList, opts.RepoPath, sourcesignals.ScanOptions{
			MaxPerFile: opts.MaxSignalPerFile,
			MaxTotal:   opts.MaxSignalTotal,
		})
	}
	b.SourceSignals = fileSignals
	fileIndex := buildFileIndex(fileList, s.GoFacts, b.KnownDocs, fileSignals)
	if len(fileIndex) > opts.MaxFiles {
		b.Warnings = append(b.Warnings, "truncated candidate_file_index")
		fileIndex = selectFileIndexWithPins(fileIndex, opts.MaxFiles, selectedCommandTracePaths(b.Go.CommandTraces))
	}
	b.CandidateFileIndex = fileIndex
	b.ProviderAllowedPaths = buildAllowedPaths(fileIndex)
	allowedSet := makePathSet(b.ProviderAllowedPaths)
	b.KnownDocs = filterPaths(b.KnownDocs, allowedSet)
	b.SourceSignals = filterSourceSignals(b.SourceSignals, allowedSet)
	b.Go.Entrypoints = filterEntrypoints(b.Go.Entrypoints, allowedSet)
	b.Go.CommandTraces = filterCommandTraces(b.Go.CommandTraces, allowedSet)
	b.Go.OrientationCandidates = filterOrientationCandidates(
		b.Go.OrientationCandidates,
		allowedSet,
	)

	return b
}

func shrinkForByteBudget(opts *Options) bool {
	changed := false
	shrink := func(value *int, minimum int) {
		if *value <= minimum {
			return
		}
		next := *value * 4 / 5
		if next < minimum {
			next = minimum
		}
		if next == *value {
			next--
		}
		*value = next
		changed = true
	}
	shrink(&opts.MaxFiles, 8)
	shrink(&opts.MaxEdges, 16)
	shrink(&opts.MaxSignalTotal, 12)
	shrink(&opts.MaxEntrypoints, 8)
	shrink(&opts.MaxModules, 8)
	shrink(&opts.MaxReadmeBytes, 2<<10)
	return changed
}

func buildFileIndex(fileList []string, facts *gofacts.Facts, knownDocs []string, fileSignals []sourcesignals.Signal) []fileIndexEntry {
	seen := make(map[string]struct{})
	var entries []fileIndexEntry

	signalMap := sourcesignals.BuildFileSignalMap(fileSignals)

	entrypointPaths := make(map[string]struct{})
	commandTracePaths := make(map[string]struct{})
	entrypointDependencyDirs := make(map[string]struct{})
	entrypointSecondHopDirs := make(map[string]struct{})
	entrypointThirdHopDirs := make(map[string]struct{})
	if facts != nil {
		for _, trace := range facts.CommandTraces {
			for _, step := range trace.Steps {
				commandTracePaths[step.TargetLocation.Path] = struct{}{}
				if step.CallsiteLocation != nil {
					commandTracePaths[step.CallsiteLocation.Path] = struct{}{}
				}
			}
			for _, call := range trace.HandlerCalls {
				if call.TargetPath != "" {
					commandTracePaths[call.TargetPath] = struct{}{}
				}
			}
		}
		selectedEntrypoints := selectOrientationEntrypoints(facts.EntrypointPackages)
		entrypointImports := make(map[string]struct{}, len(selectedEntrypoints))
		verifiedEntrypointImports := make(map[string]struct{}, len(selectedEntrypoints))
		entrypointDependencies := make(map[string]struct{})
		entrypointSecondHopDependencies := make(map[string]struct{})
		for _, ep := range selectedEntrypoints {
			entrypointImports[ep.ImportPath] = struct{}{}
			if len(ep.Anchors) > 0 {
				verifiedEntrypointImports[ep.ImportPath] = struct{}{}
				for _, anchor := range ep.Anchors {
					entrypointPaths[anchor.Path] = struct{}{}
				}
			} else {
				for _, gf := range ep.GoFiles {
					p := filepath.ToSlash(gf)
					if ep.PackageDir != "." && ep.PackageDir != "" {
						p = filepath.ToSlash(ep.PackageDir) + "/" + filepath.ToSlash(gf)
					}
					entrypointPaths[p] = struct{}{}
				}
			}
		}
		for _, oc := range facts.OrientationCandidates {
			if _, ok := entrypointImports[oc.EntrypointPackage]; !ok {
				continue
			}
			if _, verified := verifiedEntrypointImports[oc.EntrypointPackage]; verified {
				continue
			}
			for _, of := range oc.OpenFiles {
				entrypointPaths[of] = struct{}{}
			}
		}
		for _, edge := range facts.InternalEdges {
			if _, ok := entrypointImports[edge.From]; !ok {
				continue
			}
			entrypointDependencies[edge.To] = struct{}{}
			if dir, ok := repositoryDirForImport(edge.To, facts.Modules); ok {
				entrypointDependencyDirs[dir] = struct{}{}
			}
		}
		for _, edge := range facts.InternalEdges {
			if _, ok := entrypointDependencies[edge.From]; !ok {
				continue
			}
			entrypointSecondHopDependencies[edge.To] = struct{}{}
			if dir, ok := repositoryDirForImport(edge.To, facts.Modules); ok {
				if _, isDirect := entrypointDependencyDirs[dir]; !isDirect {
					entrypointSecondHopDirs[dir] = struct{}{}
				}
			}
		}
		for _, edge := range facts.InternalEdges {
			if _, ok := entrypointSecondHopDependencies[edge.From]; !ok {
				continue
			}
			if dir, ok := repositoryDirForImport(edge.To, facts.Modules); ok {
				if _, isDirect := entrypointDependencyDirs[dir]; isDirect {
					continue
				}
				if _, isSecondHop := entrypointSecondHopDirs[dir]; !isSecondHop {
					entrypointThirdHopDirs[dir] = struct{}{}
				}
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
		score, signals, reasons := scoreFile(
			f,
			kind,
			entrypointPaths,
			entrypointDependencyDirs,
			entrypointSecondHopDirs,
			entrypointThirdHopDirs,
			knownDocSet,
		)
		if _, ok := commandTracePaths[f]; ok {
			score += 90
			signals = append(signals, "command-trace")
			reasons = append(reasons, "exact declaration in a bounded CLI dispatch trace")
		}

		// Enrich with source signal categories
		if fileSignalsForFile, ok := signalMap[f]; ok {
			for _, sig := range fileSignalsForFile {
				signals = append(signals, "src:"+sig.Category)
				reasons = append(reasons, sig.Reason)
				score += sig.Weight / 2
			}
		}

		if score <= 0 {
			continue
		}

		entries = append(entries, fileIndexEntry{
			ID:      providerFileID(f),
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

func providerFileID(path string) string {
	digest := sha256.Sum256([]byte("provider-file-v1\x00" + path))
	return "file-" + hex.EncodeToString(digest[:8])
}

func selectOrientationEntrypoints(entrypoints []gofacts.Entrypoint) []gofacts.Entrypoint {
	selected := make([]gofacts.Entrypoint, 0, len(entrypoints))
	for _, entrypoint := range entrypoints {
		packageDir := strings.Trim(strings.ReplaceAll(entrypoint.PackageDir, "\\", "/"), "/")
		userFacingKind := entrypoint.Kind == "primary_binary" || entrypoint.Kind == "cli"
		rootOrCmdUnknown := entrypoint.Kind == "unknown" &&
			(packageDir == "" || packageDir == "." || packageDir == "cmd" || strings.HasPrefix(packageDir, "cmd/"))
		if userFacingKind || rootOrCmdUnknown {
			selected = append(selected, entrypoint)
		}
	}
	if len(selected) > 0 {
		return selected
	}

	const fallbackLimit = 3
	fallback := append([]gofacts.Entrypoint(nil), entrypoints...)
	sort.Slice(fallback, func(i, j int) bool {
		return fallback[i].ImportPath < fallback[j].ImportPath
	})
	if len(fallback) > fallbackLimit {
		return fallback[:fallbackLimit]
	}
	return fallback
}

func selectFileIndex(entries []fileIndexEntry, maxFiles int) []fileIndexEntry {
	return selectFileIndexWithPins(entries, maxFiles, nil)
}

func selectFileIndexWithPins(entries []fileIndexEntry, maxFiles int, pinned map[string]struct{}) []fileIndexEntry {
	if len(entries) <= maxFiles {
		return entries
	}

	testTarget := maxFiles / 10
	docTarget := maxFiles / 15
	flexSlots := maxFiles / 10
	targets := []struct {
		group string
		count int
	}{
		{group: "source", count: maxFiles - testTarget - docTarget - flexSlots},
		{group: "test", count: testTarget},
		{group: "doc", count: docTarget},
	}

	selected := make([]bool, len(entries))
	selectedCount := 0
	selectedByGroup := make(map[string]int)
	selectedSourcesByDir := make(map[string]int)
	selectEntry := func(index int) {
		selected[index] = true
		selectedCount++
		group := fileIndexGroup(entries[index].Kind)
		selectedByGroup[group]++
		if group == "source" {
			selectedSourcesByDir[path.Dir(entries[index].Path)]++
		}
	}
	for i := range entries {
		if selectedCount == maxFiles {
			break
		}
		if _, ok := pinned[entries[i].Path]; !ok {
			continue
		}
		selectEntry(i)
	}
	for i := range entries {
		if selectedCount == maxFiles {
			break
		}
		if selected[i] || !containsFileSignal(entries[i].Signals, "entrypoint") {
			continue
		}
		selectEntry(i)
	}
	for _, target := range targets {
		remaining := target.count - selectedByGroup[target.group]
		if target.group == "source" {
			const maxInitialSourcesPerDir = 4
			for i := range entries {
				if remaining <= 0 || selectedCount == maxFiles {
					break
				}
				if selected[i] || fileIndexGroup(entries[i].Kind) != target.group {
					continue
				}
				dir := path.Dir(entries[i].Path)
				if selectedSourcesByDir[dir] >= maxInitialSourcesPerDir {
					continue
				}
				selected[i] = true
				selectedCount++
				remaining--
				selectedSourcesByDir[dir]++
			}
		}
		for i := range entries {
			if remaining <= 0 || selectedCount == maxFiles {
				break
			}
			if selected[i] || fileIndexGroup(entries[i].Kind) != target.group {
				continue
			}
			selected[i] = true
			selectedCount++
			remaining--
		}
	}

	for i := range entries {
		if selectedCount == maxFiles {
			break
		}
		if selected[i] {
			continue
		}
		selected[i] = true
		selectedCount++
	}

	result := make([]fileIndexEntry, 0, maxFiles)
	for i := range entries {
		if selected[i] {
			result = append(result, entries[i])
		}
	}
	return result
}

func selectedCommandTracePaths(traces []gofacts.CommandTrace) map[string]struct{} {
	paths := make(map[string]struct{})
	for _, trace := range traces {
		for _, step := range trace.Steps {
			paths[step.TargetLocation.Path] = struct{}{}
			if step.CallsiteLocation != nil {
				paths[step.CallsiteLocation.Path] = struct{}{}
			}
		}
		for _, call := range trace.HandlerCalls {
			paths[call.Path] = struct{}{}
			if call.TargetPath != "" {
				paths[call.TargetPath] = struct{}{}
			}
		}
	}
	return paths
}

func containsFileSignal(signals []string, target string) bool {
	for _, signal := range signals {
		if signal == target {
			return true
		}
	}
	return false
}

func fileIndexGroup(kind string) string {
	switch kind {
	case "source":
		return "source"
	case "test":
		return "test"
	case "doc":
		return "doc"
	default:
		return "support"
	}
}

func repositoryDirForImport(importPath string, modules []gofacts.ModuleFact) (string, bool) {
	var best *gofacts.ModuleFact
	for i := range modules {
		modulePath := strings.TrimSuffix(modules[i].ModulePath, "/")
		if modulePath == "" || (importPath != modulePath && !strings.HasPrefix(importPath, modulePath+"/")) {
			continue
		}
		if best == nil || len(modulePath) > len(strings.TrimSuffix(best.ModulePath, "/")) {
			best = &modules[i]
		}
	}
	if best == nil {
		return "", false
	}

	modulePath := strings.TrimSuffix(best.ModulePath, "/")
	relative := strings.TrimPrefix(importPath, modulePath)
	relative = strings.TrimPrefix(relative, "/")
	moduleDir := strings.Trim(strings.ReplaceAll(best.ModuleDir, "\\", "/"), "/")
	if moduleDir == "" || moduleDir == "." {
		if relative == "" {
			return ".", true
		}
		return path.Clean(relative), true
	}
	if relative == "" {
		return path.Clean(moduleDir), true
	}
	return path.Join(moduleDir, relative), true
}

func detectFileKind(path string) string {
	lower := strings.ToLower(path)
	base := strings.ToLower(filepath.Base(path))

	if strings.HasSuffix(base, "_test.go") ||
		(strings.HasSuffix(base, ".py") && (strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py"))) {
		return "test"
	}
	if isDocumentationFile(path) {
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
	if strings.HasSuffix(lower, ".go") || strings.HasSuffix(lower, ".py") || strings.HasSuffix(lower, ".pyi") {
		return "source"
	}
	return "unknown"
}

func scoreFile(
	filePath string,
	kind string,
	entrypointPaths map[string]struct{},
	entrypointDependencyDirs map[string]struct{},
	entrypointSecondHopDirs map[string]struct{},
	entrypointThirdHopDirs map[string]struct{},
	knownDocSet map[string]struct{},
) (int, []string, []string) {
	score := 0
	var signals []string
	var reasons []string
	lower := strings.ToLower(filePath)

	addSignal := func(s string, sScore int, reason string) {
		signals = append(signals, s)
		reasons = append(reasons, reason)
		score += sScore
	}

	if _, ok := entrypointPaths[filePath]; ok {
		addSignal("entrypoint", 100, "entrypoint source file")
	}
	if kind == "source" {
		base := strings.ToLower(path.Base(filePath))
		switch base {
		case "__main__.py", "main.py", "cli.py", "app.py", "manage.py":
			addSignal("entrypoint", 90, "conventional Python entrypoint filename")
		}
		directory := path.Dir(filePath)
		if _, ok := entrypointDependencyDirs[directory]; ok {
			addSignal("entrypoint-dependency", 80, "package imported directly by an entrypoint")
		} else if _, ok := entrypointSecondHopDirs[directory]; ok {
			addSignal("entrypoint-second-hop", 50, "package imported by a direct entrypoint dependency")
		} else if _, ok := entrypointThirdHopDirs[directory]; ok {
			addSignal("entrypoint-third-hop", 30, "package imported within three hops of an entrypoint")
		}
		if isDirectoryNamedSource(filePath) {
			addSignal("directory-anchor", 20, "source file named after its directory")
		} else if isPackageAnchorSource(filePath) {
			addSignal("package-anchor", 50, "source file names the containing package role")
		}
	}

	for _, word := range []string{"server", "etcdserver"} {
		if strings.Contains(lower, word) {
			addSignal(word, 70, word+" path")
			break
		}
	}

	for _, word := range []string{"v3rpc", "lease", "mvcc", "wal", "backend", "rafthttp", "raft"} {
		if hasPathToken(lower, word) {
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
		if strings.Contains(lower, "client/") && hasPathToken(lower, word) {
			addSignal("client-"+word, 70, "client "+word+" file")
			break
		}
	}

	if strings.Contains(lower, "etcdctl/") && (strings.Contains(lower, "command") || strings.Contains(lower, "main")) {
		addSignal("etcdctl-cmd", 70, "etcdctl command file")
	}

	if _, ok := knownDocSet[filePath]; ok {
		addSignal("known-doc", 60, "documentation file")

		for _, docWord := range []string{"internals", "workflow", "architecture", "api", "raft", "watch", "write", "read"} {
			if strings.Contains(lower, docWord) {
				addSignal("doc-"+docWord, 10, "documentation about "+docWord)
				break
			}
		}
	}

	if kind == "test" {
		addSignal("test", 15, "test file")
	}

	if kind == "proto" {
		addSignal("proto", 50, "protocol buffer file")
	}

	if kind == "source" {
		addSignal("source", 30, "source file")
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

func isDirectoryNamedSource(filePath string) bool {
	directory := path.Dir(filePath)
	if directory == "." || directory == "/" {
		return false
	}
	base := strings.TrimSuffix(path.Base(filePath), path.Ext(filePath))
	return strings.EqualFold(base, path.Base(directory))
}

func isPackageAnchorSource(filePath string) bool {
	directory := path.Dir(filePath)
	if directory == "." || directory == "/" {
		return false
	}
	base := strings.ToLower(strings.TrimSuffix(path.Base(filePath), path.Ext(filePath)))
	if _, ok := packageAnchorRoles[base]; !ok {
		return false
	}
	return strings.HasSuffix(strings.ToLower(path.Base(directory)), base)
}

func hasPathToken(value, token string) bool {
	for offset := 0; offset < len(value); {
		index := strings.Index(value[offset:], token)
		if index < 0 {
			return false
		}
		start := offset + index
		end := start + len(token)
		if (start == 0 || isPathTokenBoundary(value[start-1])) &&
			(end == len(value) || isPathTokenBoundary(value[end])) {
			return true
		}
		offset = start + 1
	}
	return false
}

func isPathTokenBoundary(value byte) bool {
	switch value {
	case '/', '\\', '.', '_', '-':
		return true
	default:
		return false
	}
}

func buildAllowedPaths(index []fileIndexEntry) []string {
	paths := make([]string, len(index))
	for i, e := range index {
		paths[i] = e.Path
	}
	sort.Strings(paths)
	return paths
}

func makePathSet(paths []string) map[string]struct{} {
	result := make(map[string]struct{}, len(paths))
	for _, filePath := range paths {
		result[filePath] = struct{}{}
	}
	return result
}

func filterPaths(paths []string, allowed map[string]struct{}) []string {
	result := make([]string, 0, len(paths))
	for _, filePath := range paths {
		if _, ok := allowed[filePath]; ok {
			result = append(result, filePath)
		}
	}
	return result
}

func filterSourceSignals(signals []sourcesignals.Signal, allowed map[string]struct{}) []sourcesignals.Signal {
	result := make([]sourcesignals.Signal, 0, len(signals))
	for _, signal := range signals {
		if _, ok := allowed[signal.Path]; ok {
			result = append(result, signal)
		}
	}
	return result
}

func filterEntrypoints(entrypoints []entrypointCompact, allowed map[string]struct{}) []entrypointCompact {
	result := make([]entrypointCompact, 0, len(entrypoints))
	for _, entrypoint := range entrypoints {
		hadAnchors := len(entrypoint.Anchors) > 0
		anchors := make([]gofacts.EntrypointAnchor, 0, len(entrypoint.Anchors))
		for _, anchor := range entrypoint.Anchors {
			if _, ok := allowed[anchor.Path]; !ok {
				continue
			}
			anchors = append(anchors, anchor)
		}
		if hadAnchors && len(anchors) == 0 {
			continue
		}

		openFiles := make([]string, 0, len(entrypoint.OpenFiles))
		for _, openFile := range entrypoint.OpenFiles {
			if _, ok := allowed[openFile]; !ok {
				continue
			}
			openFiles = append(openFiles, openFile)
		}
		if len(openFiles) == 0 {
			continue
		}
		entrypoint.Anchors = anchors
		entrypoint.OpenFiles = openFiles
		result = append(result, entrypoint)
	}
	return result
}

func selectCommandTraces(traces []gofacts.CommandTrace, readme string, limit int) []gofacts.CommandTrace {
	if limit <= 0 || len(traces) == 0 {
		return nil
	}
	type rankedTrace struct {
		trace    gofacts.CommandTrace
		score    int
		position int
	}
	lowerReadme := strings.ToLower(readme)
	ranked := make([]rankedTrace, 0, len(traces))
	for position, trace := range traces {
		score := 0
		command := strings.ToLower(strings.TrimSpace(trace.Command))
		if command != "" && containsWord(lowerReadme, command) {
			score += 100
		}
		if trace.Complete {
			score += 10
		}
		ranked = append(ranked, rankedTrace{trace: trace, score: score, position: position})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].position < ranked[j].position
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	selected := make([]gofacts.CommandTrace, len(ranked))
	for index := range ranked {
		selected[index] = ranked[index].trace
	}
	return selected
}

func containsWord(text, word string) bool {
	for offset := 0; offset < len(text); {
		index := strings.Index(text[offset:], word)
		if index < 0 {
			return false
		}
		start := offset + index
		end := start + len(word)
		if (start == 0 || !isWordByte(text[start-1])) && (end == len(text) || !isWordByte(text[end])) {
			return true
		}
		offset = start + 1
	}
	return false
}

func isWordByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '_'
}

func filterCommandTraces(traces []gofacts.CommandTrace, allowed map[string]struct{}) []gofacts.CommandTrace {
	filtered := make([]gofacts.CommandTrace, 0, len(traces))
	for _, trace := range traces {
		steps := make([]gofacts.CommandTraceStep, 0, len(trace.Steps))
		for _, step := range trace.Steps {
			if _, ok := allowed[step.TargetLocation.Path]; !ok {
				continue
			}
			if step.CallsiteLocation != nil {
				if _, ok := allowed[step.CallsiteLocation.Path]; !ok {
					continue
				}
			}
			steps = append(steps, step)
		}
		if len(steps) != len(trace.Steps) {
			continue
		}
		calls := make([]gofacts.CommandTraceCall, 0, len(trace.HandlerCalls))
		for _, call := range trace.HandlerCalls {
			if _, ok := allowed[call.Path]; !ok {
				continue
			}
			if call.TargetPath != "" {
				if _, ok := allowed[call.TargetPath]; !ok {
					call.TargetPath = ""
					call.TargetLine = 0
					call.Resolved = false
				}
			}
			calls = append(calls, call)
		}
		trace.Steps = steps
		trace.HandlerCalls = calls
		filtered = append(filtered, trace)
	}
	return filtered
}

func filterOrientationCandidates(
	candidates []gofacts.OrientationCandidate,
	allowed map[string]struct{},
) []gofacts.OrientationCandidate {
	result := make([]gofacts.OrientationCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.OpenFiles = filterPaths(candidate.OpenFiles, allowed)
		if len(candidate.OpenFiles) > 0 {
			result = append(result, candidate)
		}
	}
	return result
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
		if !isDocumentationFile(f) {
			continue
		}
		lower := strings.ToLower(f)

		matched := false
		for _, pat := range interestingPatterns {
			if strings.Contains(lower, strings.ToLower(pat)) {
				matched = true
				break
			}
		}
		if !matched {
			for _, w := range interestingWords {
				if strings.Contains(lower, w) {
					matched = true
					break
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

func isDocumentationFile(filePath string) bool {
	base := strings.ToLower(filepath.Base(filePath))
	switch strings.ToLower(filepath.Ext(base)) {
	case ".md", ".markdown", ".mdx", ".rst", ".adoc", ".asciidoc", ".drawio":
		return true
	}
	switch base {
	case "readme", "contributing", "changelog", "architecture", "design":
		return true
	default:
		return false
	}
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
