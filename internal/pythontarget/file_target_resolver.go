package pythontarget

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
)

// FileTargetResolver is the local bridge from corpus FileIDs selected by a
// language-neutral portfolio back to existing sealed Python targets. The
// resolver owns a snapshot of the catalog so later caller mutations cannot
// change its authority.
//
// Native targets keep their sealed SourceRefs as authority. Separately, every
// exact module in a sealed catalog scope can yield one framework-neutral
// module-execution view when another cube selects that file. This resolver-only
// projection is not a native hypothesis and never restores unrelated project
// views. ResolveOne is the strict helper for callers requiring one view.
type FileTargetResolver struct {
	initialized                      bool
	corpusFiles                      map[corpus.FileID]struct{}
	targets                          []Target
	authoritativeTargetIndexesByFile map[corpus.FileID][]int
	moduleScopes                     []ModuleScope
	moduleByFile                     map[corpus.FileID]moduleScopeLocation
}

type moduleScopeLocation struct {
	scopeIndex  int
	moduleIndex int
}

// ModuleExecutionChoice is display-only corrective guidance for an exact
// resolver-owned selector. Path is evidence for the human; only Selector is
// accepted as --target authority.
type ModuleExecutionChoice struct {
	Selector string
	Path     string
}

// NewFileTargetResolver binds one validated Python target catalog to the exact
// repository corpus that supplied its module, basis, source, and anchor refs.
// It indexes native SourceRefs and cryptographically sealed module scopes. It
// never infers framework ownership, copies model prose, or turns module scope
// membership into an advertised target hypothesis.
func NewFileTargetResolver(
	repository *corpus.Corpus,
	catalog Catalog,
) (FileTargetResolver, error) {
	corpusSnapshot, err := validateFileProjectionInputs(repository, catalog)
	if err != nil {
		return FileTargetResolver{}, err
	}
	return fileTargetResolverFromValidated(catalog, corpusSnapshot)
}

func fileTargetResolverFromValidated(
	catalog Catalog,
	corpusSnapshot corpus.Snapshot,
) (FileTargetResolver, error) {
	ownedTargets := make([]Target, len(catalog.Entries))
	for index, target := range catalog.Entries {
		ownedTargets[index] = cloneFileResolverTarget(target)
	}
	resolver := FileTargetResolver{
		initialized:                      true,
		corpusFiles:                      make(map[corpus.FileID]struct{}, len(corpusSnapshot.Entries)),
		targets:                          ownedTargets,
		authoritativeTargetIndexesByFile: make(map[corpus.FileID][]int),
		moduleScopes:                     cloneModuleScopes(catalog.ModuleScopes),
		moduleByFile:                     make(map[corpus.FileID]moduleScopeLocation),
	}
	for _, entry := range corpusSnapshot.Entries {
		resolver.corpusFiles[entry.ID] = struct{}{}
	}

	for targetIndex, target := range resolver.targets {
		for _, fileRef := range target.SourceRefs {
			if err := resolver.addBoundAuthoritativeFile(targetIndex, fileRef); err != nil {
				return FileTargetResolver{}, err
			}
		}
	}
	for scopeIndex, scope := range resolver.moduleScopes {
		for moduleIndex, module := range scope.Modules {
			if _, known := resolver.corpusFiles[module.FileID]; !known {
				return FileTargetResolver{}, fmt.Errorf(
					"python file target resolver: module scope %q cites file_ref %q outside the repository corpus",
					scope.Ref, module.FileID,
				)
			}
			if previous, duplicate := resolver.moduleByFile[module.FileID]; duplicate {
				previousScope := resolver.moduleScopes[previous.scopeIndex]
				return FileTargetResolver{}, fmt.Errorf(
					"python file target resolver: file_ref %q belongs to conflicting scopes %q and %q",
					module.FileID, previousScope.Ref, scope.Ref,
				)
			}
			resolver.moduleByFile[module.FileID] = moduleScopeLocation{
				scopeIndex: scopeIndex, moduleIndex: moduleIndex,
			}
		}
	}

	return resolver, nil
}

// ResolvesOne reports whether fileRef names exactly one existing target in the
// bound catalog. Unknown, unrelated, and ambiguous corpus files all return
// false.
func (resolver FileTargetResolver) ResolvesOne(fileRef corpus.FileID) bool {
	if !resolver.initialized {
		return false
	}
	if len(resolver.targetIndexes(fileRef)) > 0 {
		return len(resolver.targetIndexes(fileRef)) == 1
	}
	_, exists := resolver.moduleByFile[fileRef]
	return exists
}

// Resolves reports whether fileRef has at least one exact target association.
// Unlike ResolvesOne, legitimate multi-target ownership is accepted. This is
// the right predicate for default portfolio discovery and README hypotheses.
func (resolver FileTargetResolver) Resolves(fileRef corpus.FileID) bool {
	if !resolver.initialized {
		return false
	}
	if len(resolver.targetIndexes(fileRef)) > 0 {
		return true
	}
	_, exists := resolver.moduleByFile[fileRef]
	return exists
}

// Resolve returns every exact target associated with the selected FileIDs in
// canonical catalog order. Duplicate files and targets are de-duplicated. A
// file selected by the semantic portfolio must still be known and associated
// with at least one locally established target; unknown or unsupported refs
// fail closed.
func (resolver FileTargetResolver) Resolve(fileRefs []corpus.FileID) ([]Target, error) {
	if !resolver.initialized {
		return nil, fmt.Errorf("python file target resolver: resolver is not initialized")
	}
	if len(fileRefs) == 0 {
		return nil, fmt.Errorf("python file target resolver: selected file set is empty")
	}

	selected := make(map[int]struct{})
	derived := make(map[string]Target)
	for _, fileRef := range fileRefs {
		if _, known := resolver.corpusFiles[fileRef]; !known {
			return nil, fmt.Errorf(
				"python file target resolver: unknown file_ref %q", fileRef,
			)
		}
		targetIndexes := resolver.targetIndexes(fileRef)
		if len(targetIndexes) == 0 {
			target, ok, err := resolver.moduleExecutionTarget(fileRef)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf(
					"python file target resolver: file_ref %q has no exact Python target", fileRef,
				)
			}
			derived[target.Ref] = target
			continue
		}
		for _, targetIndex := range targetIndexes {
			selected[targetIndex] = struct{}{}
		}
	}

	result := make([]Target, 0, len(selected))
	for targetIndex, target := range resolver.targets {
		if _, ok := selected[targetIndex]; ok {
			result = append(result, cloneFileResolverTarget(target))
		}
	}
	for _, target := range derived {
		result = append(result, cloneFileResolverTarget(target))
	}
	sort.Slice(result, func(i, j int) bool { return targetLess(result[i], result[j]) })
	return result, nil
}

// ResolveSelector restores one resolver-owned module-execution view from its
// exact advertised selector. It accepts no path, display-name, or module-name
// aliases: those values are useful evidence but are not closed target keys.
func (resolver FileTargetResolver) ResolveSelector(selector string) (Target, bool, error) {
	if !resolver.initialized {
		return Target{}, false, fmt.Errorf("python file target resolver: resolver is not initialized")
	}
	const prefix = "python:module-execution:"
	if !strings.HasPrefix(selector, prefix) {
		return Target{}, false, nil
	}
	remainder := strings.TrimPrefix(selector, prefix)
	scopeKey, fileKey, ok := strings.Cut(remainder, ":")
	if !ok || scopeKey == "" || fileKey == "" {
		return Target{}, false, nil
	}
	fileRef := corpus.FileID(fileKey)
	location, exists := resolver.moduleByFile[fileRef]
	if !exists {
		return Target{}, false, nil
	}
	scope := resolver.moduleScopes[location.scopeIndex]
	if strings.TrimPrefix(scope.Ref, "pys-") != scopeKey {
		return Target{}, false, nil
	}
	target, exists, err := resolver.moduleExecutionTarget(fileRef)
	if err != nil || !exists {
		return Target{}, false, err
	}
	if target.Selector != selector {
		return Target{}, false, nil
	}
	return target, true, nil
}

// ModuleExecutionChoices returns a deterministic bounded prefix plus the
// complete observed count. It does not materialize or advertise these views to
// TargetPortfolio; callers use it only after an invalid explicit --target.
func (resolver FileTargetResolver) ModuleExecutionChoices(limit int) ([]ModuleExecutionChoice, int, error) {
	if !resolver.initialized {
		return nil, 0, fmt.Errorf("python file target resolver: resolver is not initialized")
	}
	if limit < 0 {
		return nil, 0, fmt.Errorf("python file target resolver: negative choice limit")
	}
	all := make([]ModuleExecutionChoice, 0, len(resolver.moduleByFile))
	for _, scope := range resolver.moduleScopes {
		for _, module := range scope.Modules {
			all = append(all, ModuleExecutionChoice{
				Selector: moduleExecutionSelector(scope, module), Path: module.Path,
			})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Path != all[j].Path {
			return all[i].Path < all[j].Path
		}
		return all[i].Selector < all[j].Selector
	})
	total := len(all)
	if limit < len(all) {
		all = all[:limit]
	}
	return all, total, nil
}

func (resolver FileTargetResolver) moduleExecutionTarget(fileRef corpus.FileID) (Target, bool, error) {
	location, exists := resolver.moduleByFile[fileRef]
	if !exists {
		return Target{}, false, nil
	}
	scope := resolver.moduleScopes[location.scopeIndex]
	module := scope.Modules[location.moduleIndex]
	sealed, err := newModuleExecutionTarget(scope, module)
	if err != nil {
		return Target{}, false, fmt.Errorf(
			"python file target resolver: seal module-execution target for %q: %w", module.Path, err,
		)
	}
	if !catalogScopeOwnsTarget(resolver.moduleScopes, sealed) {
		return Target{}, false, fmt.Errorf(
			"python file target resolver: module-execution target for %q escaped catalog scope", module.Path,
		)
	}
	return sealed, true, nil
}

func newModuleExecutionTarget(scope ModuleScope, module Module) (Target, error) {
	label := module.Name
	if !validLabel(label) {
		label = ""
	}
	displayName := module.Name
	if !validLabel(displayName) {
		displayName = "Python module " + string(module.FileID)
	}
	target := Target{
		Version: TargetVersion, Kind: KindExecutable,
		Selector: moduleExecutionSelector(scope, module), DisplayName: displayName,
		ProjectDir: scope.ProjectDir, ScopeRef: scope.Ref,
		SourceRoots: cloneStrings(scope.SourceRoots), Modules: cloneModules(scope.Modules),
		Roots: []Root{{
			Kind: RootModuleExecution, Module: module.Name, Path: module.Path, Line: 1,
		}},
		Basis: []Basis{{
			FileID: module.FileID, Kind: BasisModuleExecutionView, Path: module.Path, Line: 1, Label: label,
		}},
	}
	canonicalizeTarget(&target)
	sealed, err := sealTarget(target)
	if err != nil {
		return Target{}, err
	}
	return sealed, nil
}

func moduleExecutionSelector(scope ModuleScope, module Module) string {
	return "python:module-execution:" + strings.TrimPrefix(scope.Ref, "pys-") + ":" + string(module.FileID)
}

// ResolveOne restores exactly one existing sealed target for fileRef. The
// returned value is independently owned and byte-for-byte equivalent to its
// catalog entry; no target identity or source fact is synthesized here.
func (resolver FileTargetResolver) ResolveOne(fileRef corpus.FileID) (Target, error) {
	targets, err := resolver.Resolve([]corpus.FileID{fileRef})
	if err != nil {
		return Target{}, err
	}
	if len(targets) != 1 {
		return Target{}, fmt.Errorf(
			"python file target resolver: file_ref %q maps to %d Python targets",
			fileRef, len(targets),
		)
	}
	return targets[0], nil
}

func (resolver FileTargetResolver) targetIndexes(fileRef corpus.FileID) []int {
	return resolver.authoritativeTargetIndexesByFile[fileRef]
}

func (resolver *FileTargetResolver) addBoundAuthoritativeFile(targetIndex int, fileRef corpus.FileID) error {
	return resolver.addBoundTargetFile(
		resolver.authoritativeTargetIndexesByFile, targetIndex, fileRef,
	)
}

func (resolver *FileTargetResolver) addBoundTargetFile(
	index map[corpus.FileID][]int,
	targetIndex int,
	fileRef corpus.FileID,
) error {
	if _, known := resolver.corpusFiles[fileRef]; !known {
		return fmt.Errorf(
			"python file target resolver: target %q cites file_ref %q outside the repository corpus",
			resolver.targets[targetIndex].Ref, fileRef,
		)
	}
	indexes := index[fileRef]
	for _, existing := range indexes {
		if existing == targetIndex {
			return nil
		}
	}
	index[fileRef] = append(indexes, targetIndex)
	return nil
}

func cloneFileResolverTarget(target Target) Target {
	target.SourceRoots = cloneFileResolverSlice(target.SourceRoots)
	target.SourceRefs = cloneFileResolverSlice(target.SourceRefs)
	target.Modules = cloneFileResolverSlice(target.Modules)
	target.Roots = cloneFileResolverSlice(target.Roots)
	target.Packages = cloneFileResolverSlice(target.Packages)
	target.Basis = cloneFileResolverSlice(target.Basis)
	return target
}

func cloneFileResolverSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	result := make([]T, len(values))
	copy(result, values)
	return result
}
