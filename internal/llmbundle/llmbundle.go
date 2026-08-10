package llmbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/artifactrole"
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

const (
	maxKnownDocsForBundle     = 30
	maxCommandTracesForBundle = 8
	minByteFitCandidateFiles  = 8
	maxByteFitSearchAttempts  = 64
	byteFitWarning            = "provider bundle fitted to request-byte context budget"
)

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
	bundle, _ := BuildWithTrace(s, fileList, opts)
	return bundle
}

// BuildWithTrace returns the exact ordinary Bundle plus observability captured
// inside the same selection and byte-fit path.
func BuildWithTrace(s snapshot.Snapshot, fileList []string, opts Options) (Bundle, BuildTrace) {
	opts = defaults(opts)
	maxBytes := opts.MaxBytes
	opts.MaxBytes = 0
	configuredCaps := selectionCapsFromOptions(opts, maxBytes)
	if maxBytes <= 0 {
		candidate, trace := buildWithTrace(s, fileList, opts)
		encoded, _ := json.Marshal(candidate)
		trace.ConfiguredCaps = configuredCaps
		trace.EffectiveCaps = effectiveSelectionCaps(opts, opts, configuredCaps, maxBytes)
		trace.ByteFit = ByteFitTrace{
			Attempts: 1, Fit: true, InitialBytes: len(encoded), FittedBytes: len(encoded),
		}
		return candidate, trace
	}

	baseline, baselineTrace := buildWithTrace(s, fileList, opts)
	baselineJSON, baselineErr := json.Marshal(baseline)
	initialBytes := len(baselineJSON)
	if baselineErr == nil && initialBytes <= maxBytes {
		baselineTrace.ConfiguredCaps = configuredCaps
		baselineTrace.EffectiveCaps = effectiveSelectionCaps(opts, opts, configuredCaps, maxBytes)
		baselineTrace.ByteFit = ByteFitTrace{
			Attempts: 1, Fit: true, InitialBytes: initialBytes, FittedBytes: initialBytes,
		}
		return baseline, baselineTrace
	}

	// The canonical candidate rows are sorted best-first. Search that exact
	// prefix for the largest count whose complete returned bundle, including
	// its fit warning, stays within the byte budget.
	upper := min(opts.MaxFiles, baselineTrace.Counts.Candidates.Before)
	if upper <= minByteFitCandidateFiles {
		return failedByteFitResult(
			baseline,
			baselineTrace,
			opts,
			opts,
			configuredCaps,
			maxBytes,
			1,
			initialBytes,
		)
	}

	attempts := 1
	var bestBundle Bundle
	var bestTrace BuildTrace
	var bestOptions Options
	bestBytes := 0
	var minimumBundle Bundle
	var minimumTrace BuildTrace
	minimumCaptured := false
	low, high := minByteFitCandidateFiles, upper-1
	for low <= high && attempts < maxByteFitSearchAttempts {
		candidateLimit := low + (high-low)/2
		fitOptions := opts
		fitOptions.MaxFiles = candidateLimit
		candidate, trace := buildWithByteFitTrace(s, fileList, fitOptions, opts.MaxFiles)
		candidate.Warnings = append(candidate.Warnings, byteFitWarning)
		if candidateLimit == minByteFitCandidateFiles {
			minimumBundle = candidate
			minimumBundle.Warnings = append([]string(nil), candidate.Warnings...)
			minimumTrace = trace
			minimumCaptured = true
		}
		encoded, err := json.Marshal(candidate)
		attempts++
		if err == nil && len(encoded) <= maxBytes {
			bestBundle = candidate
			bestTrace = trace
			bestOptions = fitOptions
			bestBytes = len(encoded)
			low = candidateLimit + 1
			continue
		}
		high = candidateLimit - 1
	}
	if bestBytes > 0 {
		bestTrace.ConfiguredCaps = configuredCaps
		bestTrace.EffectiveCaps = effectiveSelectionCaps(opts, bestOptions, configuredCaps, maxBytes)
		bestTrace.ByteFit = ByteFitTrace{
			Attempts: attempts, Applied: true, Fit: true,
			InitialBytes: initialBytes, FittedBytes: bestBytes,
		}
		appendSelectionCutoff(&bestTrace, "bundle_bytes", "byte_fit", "request_byte_budget", initialBytes, bestBytes, nil)
		return bestBundle, bestTrace
	}

	minimumOptions := opts
	minimumOptions.MaxFiles = minByteFitCandidateFiles
	if !minimumCaptured {
		minimumBundle, minimumTrace = buildWithByteFitTrace(s, fileList, minimumOptions, opts.MaxFiles)
		attempts++
	}
	return failedByteFitResult(
		minimumBundle,
		minimumTrace,
		opts,
		minimumOptions,
		configuredCaps,
		maxBytes,
		attempts,
		initialBytes,
	)
}

func failedByteFitResult(
	bundle Bundle,
	trace BuildTrace,
	configuredOptions Options,
	effectiveOptions Options,
	configuredCaps SelectionCaps,
	maxBytes int,
	attempts int,
	initialBytes int,
) (Bundle, BuildTrace) {
	bundle.Warnings = append(bundle.Warnings, "provider bundle exceeds configured context-byte budget")
	encoded, _ := json.Marshal(bundle)
	trace.ConfiguredCaps = configuredCaps
	trace.EffectiveCaps = effectiveSelectionCaps(configuredOptions, effectiveOptions, configuredCaps, maxBytes)
	trace.ByteFit = ByteFitTrace{
		Attempts: attempts, Applied: attempts > 1, Fit: false,
		InitialBytes: initialBytes, FittedBytes: len(encoded),
	}
	if trace.ByteFit.Applied && initialBytes > len(encoded) {
		appendSelectionCutoff(&trace, "bundle_bytes", "byte_fit", "request_byte_budget_exhausted", initialBytes, len(encoded), nil)
	}
	return bundle, trace
}

func build(s snapshot.Snapshot, fileList []string, opts Options) Bundle {
	bundle, _ := buildWithTrace(s, fileList, opts)
	return bundle
}

func buildWithTrace(s snapshot.Snapshot, fileList []string, opts Options) (Bundle, BuildTrace) {
	return buildWithFileIndexSelection(
		s,
		fileList,
		opts,
		opts.MaxFiles,
		countBoundFileIndexSelection,
	)
}

type fileIndexSelection int

const (
	countBoundFileIndexSelection fileIndexSelection = iota
	byteFitFileIndexSelection
)

func buildWithByteFitTrace(
	s snapshot.Snapshot,
	fileList []string,
	opts Options,
	orientationCandidateLimit int,
) (Bundle, BuildTrace) {
	return buildWithFileIndexSelection(
		s,
		fileList,
		opts,
		orientationCandidateLimit,
		byteFitFileIndexSelection,
	)
}

func buildWithFileIndexSelection(
	s snapshot.Snapshot,
	fileList []string,
	opts Options,
	orientationCandidateLimit int,
	selection fileIndexSelection,
) (Bundle, BuildTrace) {
	var trace BuildTrace
	allKnownDocs := findKnownDocsUnbounded(fileList)
	knownDocs := allKnownDocs
	if len(knownDocs) > maxKnownDocsForBundle {
		knownDocs = knownDocs[:maxKnownDocsForBundle]
		appendSelectionCutoff(&trace, "known_docs", "selection", "max_known_docs", len(allKnownDocs), len(knownDocs), allKnownDocs[len(knownDocs):])
	}
	readmeExcerpt, readmeRetainedBytes := truncateStrWithRetainedBytes(s.Readme, opts.MaxReadmeBytes)

	b := Bundle{
		RepoName:               s.RepoName,
		ReadmeExcerpt:          readmeExcerpt,
		TopLevelDirectoryStats: s.TopLevelStats,
		LanguageHints:          s.LanguageHints,
		KnownDocs:              knownDocs,
		PolicyVersion:          opts.PolicyVersion,
		LocalAuthorizedFiles:   len(fileList),
	}
	trace.Counts.ReadmeBytes.Before = len(s.Readme)
	trace.Counts.ReadmeBytes.After = readmeRetainedBytes
	trace.Counts.KnownDocs.Before = len(allKnownDocs)
	if len(s.Readme) > readmeRetainedBytes {
		appendSelectionCutoff(&trace, "readme_bytes", "selection", "max_readme_bytes", len(s.Readme), readmeRetainedBytes, nil)
	}

	if s.GoFacts != nil {
		f := s.GoFacts
		trace.Counts.Modules.Before = len(f.ModuleSummaries)
		trace.Counts.Entrypoints.Before = len(f.EntrypointPackages)
		trace.Counts.Edges.Before = len(f.InternalEdges)
		trace.Counts.OrientationCandidates.Before = len(f.OrientationCandidates)
		trace.Counts.CommandTraces.Before = len(f.CommandTraces)

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
		sort.SliceStable(modSummaries, func(i, j int) bool {
			left := moduleSummaryRole(modSummaries[i])
			right := moduleSummaryRole(modSummaries[j])
			if artifactrole.SelectionPriority(left) != artifactrole.SelectionPriority(right) {
				return artifactrole.SelectionPriority(left) > artifactrole.SelectionPriority(right)
			}
			return artifactrole.LessPath(
				modSummaries[i].ModuleDir,
				modSummaries[j].ModuleDir,
				left,
			)
		})
		if len(modSummaries) > opts.MaxModules {
			b.Warnings = append(b.Warnings, "truncated module summaries")
			omitted := moduleSummarySamples(modSummaries[opts.MaxModules:])
			appendSelectionCutoff(&trace, "modules", "selection", "max_modules", len(modSummaries), opts.MaxModules, omitted)
			modSummaries = modSummaries[:opts.MaxModules]
		}

		selectedEntrypoints := selectOrientationEntrypoints(f.EntrypointPackages)
		if len(selectedEntrypoints) < len(f.EntrypointPackages) {
			appendSelectionCutoff(&trace, "entrypoints", "eligibility", "orientation_entrypoint_policy", len(f.EntrypointPackages), len(selectedEntrypoints), entrypointSamplesDifference(f.EntrypointPackages, selectedEntrypoints))
		}
		if len(selectedEntrypoints) > opts.MaxEntrypoints {
			b.Warnings = append(b.Warnings, "truncated entrypoints")
			appendSelectionCutoff(&trace, "entrypoints", "selection", "max_entrypoints", len(selectedEntrypoints), opts.MaxEntrypoints, entrypointSamples(selectedEntrypoints[opts.MaxEntrypoints:]))
			selectedEntrypoints = selectedEntrypoints[:opts.MaxEntrypoints]
		}
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
		candidates := make([]gofacts.OrientationCandidate, 0, len(f.OrientationCandidates))
		for _, candidate := range f.OrientationCandidates {
			_, selectedEntrypoint := selectedEntrypointImports[candidate.EntrypointPackage]
			if candidate.Kind == gofacts.OrientationKindSignalFlow || selectedEntrypoint {
				candidates = append(candidates, candidate)
			}
		}
		if len(candidates) < len(f.OrientationCandidates) {
			appendSelectionCutoff(&trace, "orientation_candidates", "eligibility", "selected_entrypoint_or_signal_flow", len(f.OrientationCandidates), len(candidates), orientationCandidateSamplesDifference(f.OrientationCandidates, candidates))
		}
		if len(candidates) > orientationCandidateLimit {
			b.Warnings = append(b.Warnings, "truncated orientation candidates")
			appendSelectionCutoff(
				&trace,
				"orientation_candidates",
				"selection",
				"max_files",
				len(candidates),
				orientationCandidateLimit,
				orientationCandidateSamples(candidates[orientationCandidateLimit:]),
			)
			candidates = candidates[:orientationCandidateLimit]
		}

		edges := selectImportantEdges(f.InternalEdges, selectedEntrypoints, f.Modules, opts.MaxEdges)
		if len(f.InternalEdges) > opts.MaxEdges {
			b.Warnings = append(b.Warnings, "truncated important edges")
		}
		if len(edges) < len(f.InternalEdges) {
			appendSelectionCutoff(&trace, "edges", "selection", "max_edges", len(f.InternalEdges), len(edges), edgeSamplesDifference(f.InternalEdges, edges))
		}
		commandTraces := selectCommandTraces(f.CommandTraces, b.ReadmeExcerpt, maxCommandTracesForBundle)
		if len(commandTraces) < len(f.CommandTraces) {
			appendSelectionCutoff(&trace, "command_traces", "selection", "max_command_traces", len(f.CommandTraces), len(commandTraces), commandTraceSamplesDifference(f.CommandTraces, commandTraces))
		}

		b.Go = goSection{
			ModulesCount:          len(f.Modules),
			PackagesCount:         f.PackagesCount,
			ModuleSummaries:       modSummaries,
			Entrypoints:           eps,
			CommandTraces:         commandTraces,
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
	trace.Counts.Candidates.Before = len(fileIndex)
	trace.Counts.SourceSignals.Before = len(fileSignals)
	if len(fileIndex) > opts.MaxFiles {
		b.Warnings = append(b.Warnings, "truncated candidate_file_index")
		var selected []fileIndexEntry
		cutoffStage := "selection"
		cutoffReason := "max_files"
		if selection == byteFitFileIndexSelection {
			selected = append([]fileIndexEntry(nil), fileIndex[:opts.MaxFiles]...)
			cutoffStage = "byte_fit"
			cutoffReason = "request_byte_budget"
		} else {
			selected = selectFileIndexWithPins(
				fileIndex,
				opts.MaxFiles,
				selectedCommandTracePaths(b.Go.CommandTraces),
			)
		}
		omittedRows := omittedCandidateRows(fileIndex, selected)
		trace.OmittedCandidateSamples = boundedCandidateRows(omittedRows, maxSelectionCutoffSamples)
		appendSelectionCutoff(
			&trace,
			"candidates",
			cutoffStage,
			cutoffReason,
			len(fileIndex),
			len(selected),
			candidatePaths(omittedRows),
		)
		fileIndex = selected
	}
	b.CandidateFileIndex = fileIndex
	b.ProviderAllowedPaths = buildAllowedPaths(fileIndex)
	allowedSet := makePathSet(b.ProviderAllowedPaths)
	filteredKnownDocs := filterPaths(b.KnownDocs, allowedSet)
	appendSelectionCutoff(&trace, "known_docs", "allowlist", "candidate_file_index", len(b.KnownDocs), len(filteredKnownDocs), stringDifference(b.KnownDocs, filteredKnownDocs))
	b.KnownDocs = filteredKnownDocs
	filteredSignals := filterSourceSignals(b.SourceSignals, allowedSet)
	appendSelectionCutoff(&trace, "source_signals", "allowlist", "candidate_file_index", len(b.SourceSignals), len(filteredSignals), sourceSignalSamplesDifference(b.SourceSignals, filteredSignals))
	b.SourceSignals = filteredSignals
	filteredEntrypoints := filterEntrypoints(b.Go.Entrypoints, allowedSet)
	appendSelectionCutoff(&trace, "entrypoints", "allowlist", "candidate_file_index", len(b.Go.Entrypoints), len(filteredEntrypoints), nil)
	b.Go.Entrypoints = filteredEntrypoints
	filteredCommandTraces := filterCommandTraces(b.Go.CommandTraces, allowedSet)
	appendSelectionCutoff(&trace, "command_traces", "allowlist", "candidate_file_index", len(b.Go.CommandTraces), len(filteredCommandTraces), nil)
	b.Go.CommandTraces = filteredCommandTraces
	filteredOrientationCandidates := filterOrientationCandidates(
		b.Go.OrientationCandidates,
		allowedSet,
	)
	appendSelectionCutoff(&trace, "orientation_candidates", "allowlist", "candidate_file_index", len(b.Go.OrientationCandidates), len(filteredOrientationCandidates), orientationCandidateSamplesDifference(b.Go.OrientationCandidates, filteredOrientationCandidates))
	b.Go.OrientationCandidates = filteredOrientationCandidates

	trace.Counts.Candidates.After = len(b.CandidateFileIndex)
	trace.Counts.Entrypoints.After = len(b.Go.Entrypoints)
	trace.Counts.Modules.After = len(b.Go.ModuleSummaries)
	trace.Counts.Edges.After = len(b.Go.ImportantEdges)
	trace.Counts.SourceSignals.After = len(b.SourceSignals)
	trace.Counts.OrientationCandidates.After = len(b.Go.OrientationCandidates)
	trace.Counts.KnownDocs.After = len(b.KnownDocs)
	trace.Counts.CommandTraces.After = len(b.Go.CommandTraces)
	_ = completeSelectionCounts(&trace.Counts)
	trace.SelectedCandidates = candidateSelectionRows(b.CandidateFileIndex)
	return b, trace
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
		role := artifactrole.Classify(f, artifactrole.Hints{
			PrimaryEntry:  containsPath(entrypointPaths, f),
			Documentation: kind == "doc",
			Generated:     kind == "generated",
			Test:          kind == "test",
		})
		score += fileRoleScore(role)
		signals = append(signals, "role:"+string(role))
		reasons = append(reasons, "bounded context role: "+string(role))

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
			leftRole := fileIndexEntryRole(entries[i])
			rightRole := fileIndexEntryRole(entries[j])
			if leftRole == rightRole {
				return artifactrole.LessPath(entries[i].Path, entries[j].Path, leftRole)
			}
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
		sortEntrypointsByRole(selected)
		production := selected[:0]
		for _, entrypoint := range selected {
			if artifactrole.IsProduction(entrypointRole(entrypoint)) {
				production = append(production, entrypoint)
			}
		}
		if len(production) > 0 {
			return production
		}
		return selected
	}

	const fallbackLimit = 3
	fallback := append([]gofacts.Entrypoint(nil), entrypoints...)
	sortEntrypointsByRole(fallback)
	if len(fallback) > fallbackLimit {
		return fallback[:fallbackLimit]
	}
	return fallback
}

func sortEntrypointsByRole(entrypoints []gofacts.Entrypoint) {
	sort.SliceStable(entrypoints, func(i, j int) bool {
		left := entrypointRole(entrypoints[i])
		right := entrypointRole(entrypoints[j])
		if artifactrole.SelectionPriority(left) != artifactrole.SelectionPriority(right) {
			return artifactrole.SelectionPriority(left) > artifactrole.SelectionPriority(right)
		}
		return artifactrole.LessPath(
			entrypointArtifactPath(entrypoints[i]),
			entrypointArtifactPath(entrypoints[j]),
			left,
		)
	})
}

func entrypointRole(entrypoint gofacts.Entrypoint) artifactrole.Role {
	return artifactrole.Classify(entrypointArtifactPath(entrypoint), artifactrole.Hints{
		PrimaryEntry: entrypoint.Kind == "primary_binary" || entrypoint.Kind == "cli",
		Test:         entrypoint.Kind == "test_binary",
	})
}

func entrypointArtifactPath(entrypoint gofacts.Entrypoint) string {
	entryPath := strings.Trim(strings.ReplaceAll(entrypoint.PackageDir, "\\", "/"), "/")
	if len(entrypoint.Anchors) > 0 && entrypoint.Anchors[0].Path != "" {
		entryPath = entrypoint.Anchors[0].Path
	} else if entryPath != "" && entryPath != "." {
		entryPath += "/main.go"
	} else {
		entryPath = "main.go"
	}
	return entryPath
}

func moduleSummaryRole(summary moduleSummaryCompact) artifactrole.Role {
	modulePath := strings.Trim(strings.ReplaceAll(summary.ModuleDir, "\\", "/"), "/")
	if modulePath == "" || modulePath == "." {
		modulePath = "go.mod"
	} else {
		modulePath += "/go.mod"
	}
	hints := artifactrole.Hints{PrimaryEntry: summary.EntrypointsCount > 0}
	switch summary.RoleGuess {
	case "api_definitions", "client_library":
		hints.PublicAPI = true
	case "server_runtime":
		hints.EffectBoundary = true
	case "tests":
		hints.Test = true
	}
	return artifactrole.Classify(modulePath, hints)
}

func containsPath(paths map[string]struct{}, target string) bool {
	_, ok := paths[target]
	return ok
}

func fileRoleScore(role artifactrole.Role) int {
	switch role {
	case artifactrole.RolePrimaryProductionEntry:
		return 60
	case artifactrole.RoleEffectBoundary:
		return 35
	case artifactrole.RolePublicAPI:
		return 30
	case artifactrole.RoleProductionCore:
		return 20
	case artifactrole.RoleCurrentDocumentation:
		return 15
	case artifactrole.RoleExperimental:
		return -5
	case artifactrole.RoleTooling:
		return -10
	case artifactrole.RoleExample:
		return -15
	case artifactrole.RoleHistoricalDocumentation:
		return -20
	case artifactrole.RolePlayground:
		return -30
	case artifactrole.RoleTest:
		return -40
	case artifactrole.RoleFixture, artifactrole.RoleGenerated:
		return -50
	default:
		return 0
	}
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
	productionEntrypoints := 0
	for i := range entries {
		if selectedCount == maxFiles {
			break
		}
		if selected[i] || !containsFileSignal(entries[i].Signals, "entrypoint") ||
			!fileIndexEntryIsProduction(entries[i]) {
			continue
		}
		selectEntry(i)
		productionEntrypoints++
	}
	if productionEntrypoints == 0 {
		for i := range entries {
			if selectedCount == maxFiles {
				break
			}
			if selected[i] || !containsFileSignal(entries[i].Signals, "entrypoint") {
				continue
			}
			selectEntry(i)
		}
	}
	for _, target := range targets {
		remaining := target.count - selectedByGroup[target.group]
		if target.group == "source" {
			const maxInitialSourcesPerDir = 4
			for _, productionOnly := range []bool{true, false} {
				for i := range entries {
					if remaining <= 0 || selectedCount == maxFiles {
						break
					}
					if selected[i] || fileIndexGroup(entries[i].Kind) != target.group {
						continue
					}
					if productionOnly != fileIndexEntryIsProduction(entries[i]) {
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
		}
		for _, productionOnly := range []bool{true, false} {
			for i := range entries {
				if remaining <= 0 || selectedCount == maxFiles {
					break
				}
				if selected[i] || fileIndexGroup(entries[i].Kind) != target.group {
					continue
				}
				if target.group == "source" && productionOnly != fileIndexEntryIsProduction(entries[i]) {
					continue
				}
				selected[i] = true
				selectedCount++
				remaining--
			}
			if target.group != "source" {
				break
			}
		}
	}

	for _, productionOnly := range []bool{true, false} {
		for i := range entries {
			if selectedCount == maxFiles {
				break
			}
			if selected[i] || productionOnly != fileIndexEntryIsProduction(entries[i]) {
				continue
			}
			selected[i] = true
			selectedCount++
		}
	}

	result := make([]fileIndexEntry, 0, maxFiles)
	for i := range entries {
		if selected[i] {
			result = append(result, entries[i])
		}
	}
	return result
}

func fileIndexEntryIsProduction(entry fileIndexEntry) bool {
	return artifactrole.IsProduction(fileIndexEntryRole(entry))
}

func fileIndexEntryRole(entry fileIndexEntry) artifactrole.Role {
	for _, signal := range entry.Signals {
		const prefix = "role:"
		if !strings.HasPrefix(signal, prefix) {
			continue
		}
		return artifactrole.Role(strings.TrimPrefix(signal, prefix))
	}
	return artifactrole.Classify(entry.Path, artifactrole.Hints{})
}

func selectedCommandTracePaths(traces []gofacts.CommandTrace) map[string]struct{} {
	productionOnly := false
	for _, trace := range traces {
		if artifactrole.IsProduction(commandTraceRole(trace)) {
			productionOnly = true
			break
		}
	}
	paths := make(map[string]struct{})
	for _, trace := range traces {
		if productionOnly && !artifactrole.IsProduction(commandTraceRole(trace)) {
			continue
		}
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

type importantParentEdge struct {
	edge  gofacts.Edge
	depth int
}

func selectImportantEdges(
	edges []gofacts.Edge,
	entrypoints []gofacts.Entrypoint,
	modules []gofacts.ModuleFact,
	limit int,
) []gofacts.Edge {
	if limit <= 0 || len(edges) == 0 {
		return nil
	}

	ordered := append([]gofacts.Edge(nil), edges...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].From != ordered[j].From {
			return ordered[i].From < ordered[j].From
		}
		return ordered[i].To < ordered[j].To
	})
	if len(ordered) <= limit {
		return ordered
	}

	roots := make([]string, 0, len(entrypoints))
	for _, entrypoint := range entrypoints {
		if entrypointRole(entrypoint) == artifactrole.RolePrimaryProductionEntry {
			roots = append(roots, entrypoint.ImportPath)
		}
	}
	sort.Strings(roots)

	parents := make(map[string]importantParentEdge)
	depths := make(map[string]int, len(roots))
	queue := append([]string(nil), roots...)
	for _, root := range roots {
		depths[root] = 0
	}
	adjacency := make(map[string][]gofacts.Edge)
	for _, edge := range ordered {
		adjacency[edge.From] = append(adjacency[edge.From], edge)
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		depth := depths[current]
		for _, edge := range adjacency[current] {
			if _, seen := depths[edge.To]; seen {
				continue
			}
			depths[edge.To] = depth + 1
			parents[edge.To] = importantParentEdge{edge: edge, depth: depth + 1}
			queue = append(queue, edge.To)
		}
	}

	targets := make([]string, 0, len(parents))
	for target := range parents {
		role := importArtifactRole(target, modules)
		if artifactrole.IsProduction(role) {
			targets = append(targets, target)
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		leftRole := importArtifactRole(targets[i], modules)
		rightRole := importArtifactRole(targets[j], modules)
		if artifactrole.SelectionPriority(leftRole) != artifactrole.SelectionPriority(rightRole) {
			return artifactrole.SelectionPriority(leftRole) > artifactrole.SelectionPriority(rightRole)
		}
		if depths[targets[i]] != depths[targets[j]] {
			return depths[targets[i]] > depths[targets[j]]
		}
		return targets[i] < targets[j]
	})

	selected := make([]gofacts.Edge, 0, min(limit, len(ordered)))
	seen := make(map[gofacts.Edge]struct{}, limit)
	connectivityLimit := limit / 2
	if connectivityLimit < 4 {
		connectivityLimit = limit
	}
	for _, target := range targets {
		pathEdges := connectivityPath(target, parents)
		newCount := 0
		for _, edge := range pathEdges {
			if _, exists := seen[edge]; !exists {
				newCount++
			}
		}
		pathLimit := connectivityLimit
		if len(selected) == 0 {
			pathLimit = limit
		}
		if len(selected)+newCount > pathLimit {
			continue
		}
		for _, edge := range pathEdges {
			if _, exists := seen[edge]; exists {
				continue
			}
			seen[edge] = struct{}{}
			selected = append(selected, edge)
		}
		if len(selected) == connectivityLimit {
			break
		}
	}

	type rankedEdge struct {
		edge  gofacts.Edge
		score int
	}
	ranked := make([]rankedEdge, 0, len(ordered))
	for _, edge := range ordered {
		toRole := importArtifactRole(edge.To, modules)
		fromRole := importArtifactRole(edge.From, modules)
		score := artifactrole.SelectionPriority(toRole)*10 + artifactrole.SelectionPriority(fromRole)
		if depth, reachable := depths[edge.From]; reachable {
			score += 10_000 - depth*100
			if !artifactrole.IsProduction(toRole) {
				score -= 5_000
			}
		}
		ranked = append(ranked, rankedEdge{edge: edge, score: score})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if ranked[i].edge.From != ranked[j].edge.From {
			return ranked[i].edge.From < ranked[j].edge.From
		}
		return ranked[i].edge.To < ranked[j].edge.To
	})
	for _, item := range ranked {
		if len(selected) == limit {
			break
		}
		if _, exists := seen[item.edge]; exists {
			continue
		}
		seen[item.edge] = struct{}{}
		selected = append(selected, item.edge)
	}
	return selected
}

func connectivityPath(target string, parents map[string]importantParentEdge) []gofacts.Edge {
	var reversed []gofacts.Edge
	for {
		parent, ok := parents[target]
		if !ok {
			break
		}
		reversed = append(reversed, parent.edge)
		target = parent.edge.From
	}
	result := make([]gofacts.Edge, len(reversed))
	for index := range reversed {
		result[index] = reversed[len(reversed)-1-index]
	}
	return result
}

func importArtifactRole(importPath string, modules []gofacts.ModuleFact) artifactrole.Role {
	if directory, ok := repositoryDirForImport(importPath, modules); ok {
		return artifactrole.Classify(path.Join(directory, "package.go"), artifactrole.Hints{})
	}
	return artifactrole.Classify(importPath+"/package.go", artifactrole.Hints{})
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
	if artifactrole.IsSourcePath(path) {
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
		role     artifactrole.Role
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
		ranked = append(ranked, rankedTrace{
			trace: trace, score: score, role: commandTraceRole(trace), position: position,
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		leftRole := artifactrole.SelectionPriority(ranked[i].role)
		rightRole := artifactrole.SelectionPriority(ranked[j].role)
		if leftRole != rightRole {
			return leftRole > rightRole
		}
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

func commandTraceRole(trace gofacts.CommandTrace) artifactrole.Role {
	for _, step := range trace.Steps {
		if step.TargetLocation.Path != "" {
			return artifactrole.Classify(step.TargetLocation.Path, artifactrole.Hints{PrimaryEntry: true})
		}
		if step.CallsiteLocation != nil && step.CallsiteLocation.Path != "" {
			return artifactrole.Classify(step.CallsiteLocation.Path, artifactrole.Hints{PrimaryEntry: true})
		}
	}
	for _, call := range trace.HandlerCalls {
		if call.Path != "" {
			return artifactrole.Classify(call.Path, artifactrole.Hints{PrimaryEntry: true})
		}
		if call.TargetPath != "" {
			return artifactrole.Classify(call.TargetPath, artifactrole.Hints{PrimaryEntry: true})
		}
	}
	return artifactrole.Classify(trace.EntrypointPackage+"/main.go", artifactrole.Hints{PrimaryEntry: true})
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
	docs := findKnownDocsUnbounded(files)
	if len(docs) > maxKnownDocsForBundle {
		docs = docs[:maxKnownDocsForBundle]
	}
	return docs
}

func findKnownDocsUnbounded(files []string) []string {
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
	docs := make([]string, 0)

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

	sort.Slice(docs, func(i, j int) bool {
		left := artifactrole.Classify(docs[i], artifactrole.Hints{Documentation: true})
		right := artifactrole.Classify(docs[j], artifactrole.Hints{Documentation: true})
		if artifactrole.SelectionPriority(left) != artifactrole.SelectionPriority(right) {
			return artifactrole.SelectionPriority(left) > artifactrole.SelectionPriority(right)
		}
		return artifactrole.LessPath(docs[i], docs[j], left)
	})

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
	result, _ := truncateStrWithRetainedBytes(s, maxBytes)
	return result
}

func truncateStrWithRetainedBytes(s string, maxBytes int) (string, int) {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s, len(s)
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
		return "", 0
	}
	return s[:cut] + "\n...[truncated]", cut
}
