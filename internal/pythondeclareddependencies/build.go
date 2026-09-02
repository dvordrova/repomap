// Package pythondeclareddependencies extracts exact Python package-manager
// declarations from one selected target scope. It never executes repository
// code and never promotes a declaration into observed import or call evidence.
package pythondeclareddependencies

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencydeclaration"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

const AdvisorySourceBytes = int64(2 << 20)

const (
	formatPyproject    = "pyproject_toml"
	formatRequirements = "requirements"
	formatSetupCFG     = "setup_cfg"
	formatSetupPY      = "setup_py"
	formatPipfile      = "pipfile"
)

type sourceDraft struct {
	key     string
	entry   corpus.Entry
	format  string
	state   dependencydeclaration.SourceState
	content []byte
	digest  string
}

type requirementWork struct {
	sourceKey string
	kind      dependencydeclaration.StatementKind
}

type builder struct {
	ctx                        context.Context
	repository                 *corpus.Corpus
	projectDir                 string
	excludedDirs               []string
	sources                    map[string]*sourceDraft
	maxLogicalRequirementBytes int
	statements                 []dependencydeclaration.StatementInput
	includes                   []dependencydeclaration.IncludeInput
	frontiers                  []dependencydeclaration.FrontierInput
	queue                      []requirementWork
	parsed                     map[string]struct{}
	includeSeen                map[string]struct{}
	frontierSeen               map[string]struct{}
}

type buildDiagnostics struct {
	maxLogicalRequirementBytes int
}

const (
	ScaleWarningPythonSourceBytes dependencydeclaration.ScaleWarningKind = "python_source_bytes"
	ScaleWarningLogicalLineBytes  dependencydeclaration.ScaleWarningKind = "logical_requirement_bytes"
)

// Build reads the complete available bytes of declaration sources through the
// sealed RepositoryCorpus, parses the supported closed grammars, and returns
// a fully sealed language-neutral artifact.
func Build(
	ctx context.Context,
	repository *corpus.Corpus,
	targets pythontarget.Catalog,
	selected pythontarget.Target,
	index programindex.Index,
) (dependencydeclaration.Result, error) {
	return build(ctx, repository, targets, selected, index, nil)
}

// BuildWithDiagnostics returns the same exact result plus warning-only size
// observations for ordinary console output. Diagnostics never participate in
// artifact identity or acceptance.
func BuildWithDiagnostics(
	ctx context.Context,
	repository *corpus.Corpus,
	targets pythontarget.Catalog,
	selected pythontarget.Target,
	index programindex.Index,
) (dependencydeclaration.Result, []dependencydeclaration.ScaleWarning, error) {
	diagnostics := &buildDiagnostics{}
	result, err := build(ctx, repository, targets, selected, index, diagnostics)
	if err != nil {
		return dependencydeclaration.Result{}, nil, err
	}
	warnings := dependencydeclaration.ScaleWarnings(result)
	maxSourceBytes := 0
	for _, source := range result.Sources {
		if source.ByteCount > maxSourceBytes {
			maxSourceBytes = source.ByteCount
		}
	}
	if int64(maxSourceBytes) > AdvisorySourceBytes {
		warnings = append(warnings, dependencydeclaration.ScaleWarning{
			Kind: ScaleWarningPythonSourceBytes, Retained: int64(maxSourceBytes),
			AdvisorySize: AdvisorySourceBytes,
		})
	}
	if diagnostics.maxLogicalRequirementBytes > AdvisoryLogicalRequirementBytes {
		warnings = append(warnings, dependencydeclaration.ScaleWarning{
			Kind:         ScaleWarningLogicalLineBytes,
			Retained:     int64(diagnostics.maxLogicalRequirementBytes),
			AdvisorySize: AdvisoryLogicalRequirementBytes,
		})
	}
	return result, warnings, nil
}

func build(
	ctx context.Context,
	repository *corpus.Corpus,
	targets pythontarget.Catalog,
	selected pythontarget.Target,
	index programindex.Index,
	diagnostics *buildDiagnostics,
) (dependencydeclaration.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return dependencydeclaration.Result{}, err
	}
	if repository == nil {
		return dependencydeclaration.Result{}, fmt.Errorf("python declared dependencies: repository corpus is required")
	}
	snapshot := repository.Snapshot()
	if err := snapshot.Validate(); err != nil {
		return dependencydeclaration.Result{}, fmt.Errorf("python declared dependencies: corpus: %w", err)
	}
	if err := validateCatalogCorpusScopes(snapshot, targets); err != nil {
		return dependencydeclaration.Result{}, err
	}
	scope, excludedDirs, err := ScopeForTarget(targets, selected, index)
	if err != nil {
		return dependencydeclaration.Result{}, err
	}
	projectDir := selected.ProjectDir
	if projectDir == "." {
		projectDir = ""
	}

	state := &builder{
		ctx: ctx, repository: repository, projectDir: projectDir,
		excludedDirs: excludedDirs, sources: make(map[string]*sourceDraft),
		statements: []dependencydeclaration.StatementInput{},
		includes:   []dependencydeclaration.IncludeInput{},
		frontiers:  []dependencydeclaration.FrontierInput{},
		parsed:     make(map[string]struct{}), includeSeen: make(map[string]struct{}),
		frontierSeen: make(map[string]struct{}),
	}
	if err := state.discoverRootSources(); err != nil {
		return dependencydeclaration.Result{}, err
	}
	if err := state.parseSources(); err != nil {
		return dependencydeclaration.Result{}, err
	}

	input := dependencydeclaration.Input{
		CorpusSHA256: snapshot.SHA256, ProgramIndexSHA256: index.SHA256, TargetID: index.Target.ID,
		Scope:   scope,
		Sources: state.sourceInputs(), Statements: state.statements,
		Includes: state.includes, Frontiers: state.frontiers,
	}
	result, err := dependencydeclaration.Build(input)
	if err != nil {
		return dependencydeclaration.Result{}, fmt.Errorf("python declared dependencies: build artifact: %w", err)
	}
	if err := result.ValidateAgainst(snapshot, index); err != nil {
		return dependencydeclaration.Result{}, err
	}
	if err := ValidateAgainst(result, snapshot, targets, selected, index); err != nil {
		return dependencydeclaration.Result{}, err
	}
	if diagnostics != nil {
		diagnostics.maxLogicalRequirementBytes = state.maxLogicalRequirementBytes
	}
	return result, nil
}

// ValidateAgainst adds the adapter-owned Python target scope proof to the
// generic corpus and ProgramIndex validation.
func ValidateAgainst(
	result dependencydeclaration.Result,
	snapshot corpus.Snapshot,
	targets pythontarget.Catalog,
	selected pythontarget.Target,
	index programindex.Index,
) error {
	if err := result.ValidateAgainst(snapshot, index); err != nil {
		return err
	}
	if err := validateCatalogCorpusScopes(snapshot, targets); err != nil {
		return err
	}
	wantScope, _, err := ScopeForTarget(targets, selected, index)
	if err != nil {
		return err
	}
	if result.Scope != wantScope {
		return fmt.Errorf("python declared dependencies: target scope authority mismatch")
	}
	return nil
}

// ValidateTargetAuthority re-establishes the adapter-owned Python target
// scope when a consumer has the sealed target catalog and ProgramIndex but no
// longer has the complete live RepositoryCorpus. This is intentionally
// narrower than ValidateAgainst: source byte identities remain owned by the
// declaration artifact and its canonical seal.
func ValidateTargetAuthority(
	result dependencydeclaration.Result,
	targets pythontarget.Catalog,
	index programindex.Index,
) error {
	if err := result.Validate(); err != nil {
		return err
	}
	if err := targets.Validate(); err != nil {
		return fmt.Errorf("python declared dependencies: target catalog: %w", err)
	}
	ownedIndex := index.Snapshot()
	if err := ownedIndex.Validate(); err != nil {
		return fmt.Errorf("python declared dependencies: ProgramIndex: %w", err)
	}
	selected, found, err := targets.ResolveSelector(ownedIndex.Target.Selector)
	if err != nil {
		return fmt.Errorf("python declared dependencies: resolve ProgramIndex target: %w", err)
	}
	if !found || validateTargetProjection(selected, ownedIndex.Target) != nil {
		return fmt.Errorf("python declared dependencies: ProgramIndex has no exact Python target authority")
	}
	wantScope, _, err := ScopeForTarget(targets, selected, ownedIndex)
	if err != nil {
		return err
	}
	if result.ProgramIndexSHA256 != ownedIndex.SHA256 || result.TargetID != ownedIndex.Target.ID ||
		result.Scope != wantScope {
		return fmt.Errorf("python declared dependencies: target authority mismatch")
	}
	return nil
}

// ScopeForTarget derives the exact declaration scope seal and the nested
// project directories excluded from one adapter-owned Python target. It is the
// same constructor used by Build, so other local pipeline components never
// reproduce the scope hashing or project-exclusion contract.
func ScopeForTarget(
	targets pythontarget.Catalog,
	selected pythontarget.Target,
	index programindex.Index,
) (dependencydeclaration.Scope, []string, error) {
	authoritySHA256, excludedDirs, err := validateAuthority(targets, selected, index)
	if err != nil {
		return dependencydeclaration.Scope{}, nil, err
	}
	repositoryPath := selected.ProjectDir
	if repositoryPath == "." {
		repositoryPath = ""
	}
	return dependencydeclaration.Scope{
		Language: "python", Ecosystem: "pypi", RepositoryPath: repositoryPath,
		AuthoritySHA256: authoritySHA256,
	}, excludedDirs, nil
}

func validateCatalogCorpusScopes(snapshot corpus.Snapshot, targets pythontarget.Catalog) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("python declared dependencies: corpus: %w", err)
	}
	if err := targets.Validate(); err != nil {
		return fmt.Errorf("python declared dependencies: target catalog: %w", err)
	}
	entries := make(map[corpus.FileID]corpus.Entry, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		entries[entry.ID] = entry
	}
	for _, target := range targets.Entries {
		paths := make(map[corpus.FileID]string, len(target.Modules)+len(target.Basis))
		for _, module := range target.Modules {
			paths[module.FileID] = module.Path
		}
		for _, basis := range target.Basis {
			if previous, exists := paths[basis.FileID]; exists && previous != basis.Path {
				return fmt.Errorf("python declared dependencies: target catalog has conflicting corpus path")
			}
			paths[basis.FileID] = basis.Path
		}
		anchorPath := paths[target.AnchorFileRef]
		anchor, ok := entries[target.AnchorFileRef]
		if anchorPath == "" || !ok || anchor.Path != anchorPath {
			return fmt.Errorf("python declared dependencies: target %q anchor does not match corpus", target.Selector)
		}
	}
	for _, scope := range targets.ModuleScopes {
		for _, module := range scope.Modules {
			entry, ok := entries[module.FileID]
			if !ok || entry.Path != module.Path {
				return fmt.Errorf(
					"python declared dependencies: module scope %q does not match corpus",
					scope.Ref,
				)
			}
		}
	}
	return nil
}

func validateAuthority(
	targets pythontarget.Catalog,
	selected pythontarget.Target,
	index programindex.Index,
) (string, []string, error) {
	if err := targets.Validate(); err != nil {
		return "", nil, fmt.Errorf("python declared dependencies: target catalog: %w", err)
	}
	if err := selected.Validate(); err != nil {
		return "", nil, fmt.Errorf("python declared dependencies: selected target: %w", err)
	}
	if !targets.OwnsTarget(selected) {
		return "", nil, fmt.Errorf("python declared dependencies: selected target is absent from catalog")
	}
	ownedIndex := index.Snapshot()
	if err := ownedIndex.Validate(); err != nil {
		return "", nil, fmt.Errorf("python declared dependencies: ProgramIndex: %w", err)
	}
	if err := validateTargetProjection(selected, ownedIndex.Target); err != nil {
		return "", nil, err
	}
	excludedSet := make(map[string]struct{})
	for _, target := range targets.Entries {
		if target.ProjectDir == selected.ProjectDir || !strictlyWithin(selected.ProjectDir, target.ProjectDir) {
			continue
		}
		excludedSet[target.ProjectDir] = struct{}{}
	}
	for _, scope := range targets.ModuleScopes {
		if scope.ProjectDir == selected.ProjectDir || !strictlyWithin(selected.ProjectDir, scope.ProjectDir) {
			continue
		}
		excludedSet[scope.ProjectDir] = struct{}{}
	}
	excluded := make([]string, 0, len(excludedSet))
	for directory := range excludedSet {
		excluded = append(excluded, directory)
	}
	sort.Strings(excluded)
	payload := struct {
		Version           int      `json:"version"`
		TargetCatalogRef  string   `json:"target_catalog_ref"`
		TargetRef         string   `json:"target_ref"`
		TargetIdentityRef string   `json:"target_identity_ref"`
		ProjectDir        string   `json:"project_dir"`
		ExcludedDirs      []string `json:"excluded_dirs"`
	}{1, targets.Ref, selected.Ref, selected.IdentityRef, selected.ProjectDir, excluded}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", nil, fmt.Errorf("python declared dependencies: encode scope authority: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), excluded, nil
}

func validateTargetProjection(selected pythontarget.Target, target programindex.Target) error {
	wantKind := "executable"
	if selected.Kind == pythontarget.KindLibrary {
		wantKind = "library"
	}
	if target.Language != "python" || target.Kind != wantKind || target.Name != selected.DisplayName ||
		target.Selector != selected.Selector || target.AnchorFileRef != string(selected.AnchorFileRef) {
		return fmt.Errorf("python declared dependencies: ProgramIndex target does not match selected Python target")
	}
	pathsByRef := make(map[string]string, len(selected.Modules)+len(selected.Basis))
	for _, module := range selected.Modules {
		pathsByRef[string(module.FileID)] = module.Path
	}
	for _, basis := range selected.Basis {
		if previous, exists := pathsByRef[string(basis.FileID)]; exists && previous != basis.Path {
			return fmt.Errorf("python declared dependencies: conflicting target source binding")
		}
		pathsByRef[string(basis.FileID)] = basis.Path
	}
	selectedRefs := make(map[string]struct{}, len(selected.SourceRefs)+len(selected.Basis))
	for _, ref := range selected.SourceRefs {
		selectedRefs[string(ref)] = struct{}{}
	}
	for _, basis := range selected.Basis {
		selectedRefs[string(basis.FileID)] = struct{}{}
	}
	wantSources := make([]programindex.TargetSource, 0, len(selectedRefs))
	for ref := range selectedRefs {
		if pathsByRef[ref] == "" {
			return fmt.Errorf("python declared dependencies: selected target source has no path")
		}
		wantSources = append(wantSources, programindex.TargetSource{FileRef: ref, Path: pathsByRef[ref]})
	}
	sort.Slice(wantSources, func(i, j int) bool {
		if wantSources[i].FileRef != wantSources[j].FileRef {
			return wantSources[i].FileRef < wantSources[j].FileRef
		}
		return wantSources[i].Path < wantSources[j].Path
	})
	if !reflect.DeepEqual(target.Sources, wantSources) {
		return fmt.Errorf("python declared dependencies: ProgramIndex source scope does not match selected Python target")
	}
	return nil
}

func (state *builder) discoverRootSources() error {
	entries := state.repository.Entries()
	candidates := make([]corpus.Entry, 0)
	for _, entry := range entries {
		if err := state.ctx.Err(); err != nil {
			return err
		}
		base := path.Base(entry.Path)
		atProjectRoot := repositoryDir(entry.Path) == state.projectDir
		if (atProjectRoot && (base == "pyproject.toml" || base == "setup.cfg" ||
			base == "setup.py" || base == "Pipfile")) || state.requirementsCandidate(entry.Path) {
			candidates = append(candidates, entry)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Path < candidates[j].Path })
	for _, entry := range candidates {
		var format string
		var sourceState dependencydeclaration.SourceState
		switch path.Base(entry.Path) {
		case "pyproject.toml":
			format, sourceState = formatPyproject, dependencydeclaration.SourceParsed
		case "setup.cfg":
			format, sourceState = formatSetupCFG, dependencydeclaration.SourceFrontier
		case "setup.py":
			format, sourceState = formatSetupPY, dependencydeclaration.SourceFrontier
		case "Pipfile":
			format, sourceState = formatPipfile, dependencydeclaration.SourceFrontier
		default:
			format, sourceState = formatRequirements, dependencydeclaration.SourceParsed
		}
		source, err := state.addSource(entry, format, sourceState)
		if err != nil {
			return err
		}
		switch format {
		case formatRequirements:
			state.queue = append(state.queue, requirementWork{sourceKey: source.key, kind: dependencydeclaration.StatementRequirement})
		case formatSetupCFG:
			state.addSourceFrontier(source, dependencydeclaration.FrontierUnsupportedSetupCFG)
		case formatSetupPY:
			state.addSourceFrontier(source, dependencydeclaration.FrontierUnsupportedSetupPY)
		case formatPipfile:
			state.addSourceFrontier(source, dependencydeclaration.FrontierUnsupportedPipfile)
		}
	}
	return nil
}

func (state *builder) parseSources() error {
	pyprojects := make([]*sourceDraft, 0)
	for _, source := range state.sources {
		if source.format == formatPyproject {
			pyprojects = append(pyprojects, source)
		}
	}
	sort.Slice(pyprojects, func(i, j int) bool { return pyprojects[i].entry.Path < pyprojects[j].entry.Path })
	for _, source := range pyprojects {
		if err := state.ctx.Err(); err != nil {
			return err
		}
		if err := state.parsePyproject(source); err != nil {
			return err
		}
	}
	for len(state.queue) > 0 {
		work := state.queue[0]
		state.queue = state.queue[1:]
		parseKey := work.sourceKey + ":" + string(work.kind)
		if _, done := state.parsed[parseKey]; done {
			continue
		}
		state.parsed[parseKey] = struct{}{}
		source := state.sources[work.sourceKey]
		if source == nil {
			return fmt.Errorf("python declared dependencies: requirement work has unknown source")
		}
		if err := state.parseRequirements(source, work.kind); err != nil {
			return err
		}
	}
	return nil
}

func (state *builder) addSource(
	entry corpus.Entry,
	format string,
	sourceState dependencydeclaration.SourceState,
) (*sourceDraft, error) {
	key := sourceKey(entry.ID, format)
	if existing := state.sources[key]; existing != nil {
		if existing.entry != entry || existing.state != sourceState {
			return nil, fmt.Errorf("python declared dependencies: conflicting source projection %q", entry.Path)
		}
		return existing, nil
	}
	content, err := state.repository.ReadFileAll(entry.ID)
	if err != nil {
		return nil, fmt.Errorf("python declared dependencies: read %q: %w", entry.Path, err)
	}
	digest := sha256.Sum256(content.Bytes)
	source := &sourceDraft{
		key: key, entry: entry, format: format, state: sourceState,
		content: append([]byte(nil), content.Bytes...), digest: hex.EncodeToString(digest[:]),
	}
	state.sources[key] = source
	return source, nil
}

func (state *builder) addSourceFrontier(source *sourceDraft, reason dependencydeclaration.FrontierReason) {
	key := source.key + ":source:" + string(reason)
	if _, duplicate := state.frontierSeen[key]; duplicate {
		return
	}
	state.frontierSeen[key] = struct{}{}
	state.frontiers = append(state.frontiers, dependencydeclaration.FrontierInput{
		SourceKey: source.key, Kind: dependencydeclaration.FrontierSource,
		Reason: reason, ExpressionSHA256: source.digest,
	})
}

func (state *builder) sourceInputs() []dependencydeclaration.SourceInput {
	result := make([]dependencydeclaration.SourceInput, 0, len(state.sources))
	for _, source := range state.sources {
		result = append(result, dependencydeclaration.SourceInput{
			Key: source.key, FileRef: source.entry.ID, Path: source.entry.Path,
			Format: source.format, State: source.state, ContentSHA256: source.digest,
			ByteCount: len(source.content),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		return result[i].Format < result[j].Format
	})
	return result
}

func sourceKey(fileRef corpus.FileID, format string) string {
	return "source-" + string(fileRef) + "-" + format
}

func requirementsFilename(base string) bool {
	return strings.HasPrefix(base, "requirements") && strings.HasSuffix(base, ".txt")
}

func (state *builder) requirementsCandidate(filePath string) bool {
	if !state.inScope(filePath) || path.Ext(filePath) != ".txt" {
		return false
	}
	if requirementsFilename(path.Base(filePath)) {
		return true
	}
	relative := filePath
	if state.projectDir != "" {
		relative = strings.TrimPrefix(filePath, state.projectDir+"/")
	}
	parts := strings.Split(relative, "/")
	return len(parts) >= 2 && parts[0] == "requirements"
}

func repositoryDir(filePath string) string {
	directory := path.Dir(filePath)
	if directory == "." {
		return ""
	}
	return directory
}

func strictlyWithin(root, candidate string) bool {
	return candidate != root && (root == "" || strings.HasPrefix(candidate, root+"/"))
}

func (state *builder) inScope(candidate string) bool {
	if candidate == "" || path.IsAbs(candidate) || path.Clean(candidate) != candidate ||
		candidate == "." || candidate == ".." || strings.HasPrefix(candidate, "../") ||
		strings.Contains(candidate, "\\") {
		return false
	}
	if state.projectDir != "" && candidate != state.projectDir && !strings.HasPrefix(candidate, state.projectDir+"/") {
		return false
	}
	for _, directory := range state.excludedDirs {
		if candidate == directory || strings.HasPrefix(candidate, directory+"/") {
			return false
		}
	}
	return true
}
