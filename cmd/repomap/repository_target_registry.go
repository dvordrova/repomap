package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

// repositoryTargetAdapter is a stable command-layer adapter key. It is not a
// source language: one adapter may deliberately produce several ProgramIndex
// languages (for example JavaScript and TypeScript).
type repositoryTargetAdapter string

const (
	repositoryTargetAdapterGo     repositoryTargetAdapter = "go"
	repositoryTargetAdapterPython repositoryTargetAdapter = "python"
	repositoryTargetAdapterJSTS   repositoryTargetAdapter = "jsts"
)

// repositoryTargetKey is the collision-safe planning identity. Native refs
// stay adapter-private and never become repository-wide map keys on their own.
type repositoryTargetKey struct {
	Adapter repositoryTargetAdapter
	Ref     string
}

func (key repositoryTargetKey) String() string {
	return string(key.Adapter) + ":" + key.Ref
}

// repositoryTargetAdapterDescriptor is the one command-layer declaration an
// adapter adds. Generic planning knows only this descriptor and the neutral
// repositoryTypedTarget below. Adapter-native values remain opaque and are
// inspected only by these callbacks.
//
// BuildProgramInput is the atomic adapter seam: an adapter translates one
// compiler/interpreter snapshot into one complete programindex.Input, and the
// generic dispatcher sends the sealed result through the shared page path.
// Existing adapters still enter through their mature page builders; a new
// adapter must not expose five independently sampled getters.
type repositoryTargetAdapterDescriptor struct {
	Key              repositoryTargetAdapter
	Rank             int
	Label            string
	AllowedLanguages []string
	SelectorPrefixes []string

	Discover              func(context.Context, repositoryTargetRuntimeOptions) (repositoryTargetAdapterDiscovery, bool, error)
	PrepareDispatchPlan   func(repositoryTargetPlan, []repositoryTypedTarget) (any, error)
	PrepareDispatchTarget func(context.Context, repositoryTargetDispatchOptions, repositoryTypedTarget, any) (repositoryTargetDispatchBinding, error)
	ValidateNative        func(repositoryTypedTarget) error
	MatchProgramTarget    func(repositoryTypedTarget, programindex.Target) bool
	ValidatePlanAuthority func(any, repositoryTypedTarget) error
	BuildProgramInput     func(repositoryProgramBuildRequest) (programindex.Input, error)
	BuildDependencies     func(repositoryDependencyBuildRequest) (dependencies.Catalog, error)
}

// repositoryProgramBuildRequest is one immutable adapter-owned compiler fact
// snapshot. Generic orchestration passes it atomically to BuildProgramInput;
// an adapter never exposes independently sampled GetObjects/GetRelations/etc.
type repositoryProgramBuildRequest struct {
	Context context.Context
	Corpus  *corpus.Corpus
	Target  repositoryTypedTarget
	Facts   any
}

type repositoryDependencyBuildRequest struct {
	Target       repositoryTypedTarget
	ProgramIndex programindex.Index
	Facts        any
}

type repositoryProgramPageAuthority struct {
	ProgramIndex programindex.Index
	Dependencies dependencies.Catalog
	Label        string
}

type repositoryTargetFileRestoration struct {
	Target   repositoryTypedTarget
	FileRefs []corpus.FileID
}

// repositoryTargetAdapterDiscovery is the adapter's complete deterministic
// planning snapshot. Generic selection only merges Candidates and delegates
// exact restoration through these callbacks; it never switches on a language.
type repositoryTargetAdapterDiscovery struct {
	Key              repositoryTargetAdapter
	Candidates       []analysistarget.FileCandidate
	RequiredFileRefs []corpus.FileID
	Authority        any

	ResolvesFile      func(corpus.FileID) bool
	RestoreFiles      func([]corpus.FileID) ([]repositoryTargetFileRestoration, error)
	ResolveExplicit   func(*corpus.Corpus, string) ([]repositoryTypedTarget, error)
	ChoiceGroup       func() (targetPortfolioChoiceGroup, error)
	SnapshotAuthority func() (any, error)
}

func (discovery repositoryTargetAdapterDiscovery) validate(
	registry repositoryTargetAdapterRegistry,
) error {
	descriptor, ok := registry.descriptor(discovery.Key)
	if !ok || descriptor.Key != discovery.Key || discovery.ResolvesFile == nil ||
		discovery.RestoreFiles == nil || discovery.ResolveExplicit == nil ||
		discovery.ChoiceGroup == nil || discovery.SnapshotAuthority == nil {
		return fmt.Errorf("repository target adapter %q returned an incomplete discovery snapshot", discovery.Key)
	}
	seenRequired := make(map[corpus.FileID]struct{}, len(discovery.RequiredFileRefs))
	for _, fileRef := range discovery.RequiredFileRefs {
		if strings.TrimSpace(string(fileRef)) == "" {
			return fmt.Errorf("repository target adapter %q has an invalid required file ref", discovery.Key)
		}
		if _, duplicate := seenRequired[fileRef]; duplicate {
			return fmt.Errorf("repository target adapter %q has duplicate required file refs", discovery.Key)
		}
		seenRequired[fileRef] = struct{}{}
	}
	return nil
}

type repositoryTargetAdapterRegistry struct {
	ordered []repositoryTargetAdapterDescriptor
	byKey   map[repositoryTargetAdapter]repositoryTargetAdapterDescriptor
}

func newRepositoryTargetAdapterRegistry(
	descriptors ...repositoryTargetAdapterDescriptor,
) (repositoryTargetAdapterRegistry, error) {
	if len(descriptors) == 0 {
		return repositoryTargetAdapterRegistry{}, fmt.Errorf("repository target adapter registry is empty")
	}
	ordered := append([]repositoryTargetAdapterDescriptor(nil), descriptors...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Rank < ordered[j].Rank })
	byKey := make(map[repositoryTargetAdapter]repositoryTargetAdapterDescriptor, len(ordered))
	seenRank := make(map[int]struct{}, len(ordered))
	type selectorPrefixOwner struct {
		prefix string
		key    repositoryTargetAdapter
	}
	seenPrefixes := make([]selectorPrefixOwner, 0)
	for index := range ordered {
		descriptor := &ordered[index]
		if strings.TrimSpace(string(descriptor.Key)) == "" ||
			string(descriptor.Key) != strings.TrimSpace(string(descriptor.Key)) ||
			strings.TrimSpace(descriptor.Label) == "" || descriptor.Label != strings.TrimSpace(descriptor.Label) ||
			descriptor.Rank < 0 || len(descriptor.AllowedLanguages) == 0 ||
			descriptor.Discover == nil || descriptor.PrepareDispatchPlan == nil ||
			descriptor.PrepareDispatchTarget == nil ||
			descriptor.ValidateNative == nil || descriptor.MatchProgramTarget == nil ||
			descriptor.ValidatePlanAuthority == nil ||
			descriptor.BuildProgramInput == nil || descriptor.BuildDependencies == nil {
			return repositoryTargetAdapterRegistry{}, fmt.Errorf(
				"repository target adapter descriptor %d is incomplete", index,
			)
		}
		if _, duplicate := byKey[descriptor.Key]; duplicate {
			return repositoryTargetAdapterRegistry{}, fmt.Errorf(
				"repository target adapter key %q is duplicated", descriptor.Key,
			)
		}
		if _, duplicate := seenRank[descriptor.Rank]; duplicate {
			return repositoryTargetAdapterRegistry{}, fmt.Errorf(
				"repository target adapter rank %d is duplicated", descriptor.Rank,
			)
		}
		seenRank[descriptor.Rank] = struct{}{}
		if err := validateRepositoryTargetLanguages(descriptor.AllowedLanguages); err != nil {
			return repositoryTargetAdapterRegistry{}, fmt.Errorf(
				"repository target adapter %q: %w", descriptor.Key, err,
			)
		}
		prefixes := append([]string(nil), descriptor.SelectorPrefixes...)
		for prefixIndex, prefix := range prefixes {
			if strings.TrimSpace(prefix) == "" || prefix != strings.TrimSpace(prefix) ||
				!strings.HasSuffix(prefix, ":") {
				return repositoryTargetAdapterRegistry{}, fmt.Errorf(
					"repository target adapter %q has invalid selector prefix at index %d",
					descriptor.Key, prefixIndex,
				)
			}
			if prefixIndex > 0 && prefixes[prefixIndex-1] >= prefix {
				return repositoryTargetAdapterRegistry{}, fmt.Errorf(
					"repository target adapter %q selector prefixes are not canonical", descriptor.Key,
				)
			}
			for _, owner := range seenPrefixes {
				if strings.HasPrefix(prefix, owner.prefix) || strings.HasPrefix(owner.prefix, prefix) {
					return repositoryTargetAdapterRegistry{}, fmt.Errorf(
						"repository target selector prefixes %q and %q overlap for adapters %q and %q",
						owner.prefix, prefix, owner.key, descriptor.Key,
					)
				}
			}
			seenPrefixes = append(seenPrefixes, selectorPrefixOwner{prefix: prefix, key: descriptor.Key})
		}
		descriptor.AllowedLanguages = append([]string(nil), descriptor.AllowedLanguages...)
		descriptor.SelectorPrefixes = prefixes
		byKey[descriptor.Key] = *descriptor
	}
	return repositoryTargetAdapterRegistry{ordered: ordered, byKey: byKey}, nil
}

func validateRepositoryTargetLanguages(languages []string) error {
	for index, language := range languages {
		if strings.TrimSpace(language) == "" || language != strings.TrimSpace(language) {
			return fmt.Errorf("invalid allowed language at index %d", index)
		}
		if index > 0 && languages[index-1] >= language {
			return fmt.Errorf("allowed languages are not canonical")
		}
	}
	return nil
}

func equalRepositoryTargetLanguages(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameRepositoryPlannedTarget(left, right repositoryTypedTarget) bool {
	// Display may be refreshed from a materialized manifest after discovery;
	// selector, scope, language family, and file authority may not drift.
	if left.Key != right.Key || left.Selector != right.Selector || left.Scope != right.Scope ||
		!equalRepositoryTargetLanguages(left.AllowedLanguages, right.AllowedLanguages) ||
		len(left.FileRefs) != len(right.FileRefs) {
		return false
	}
	for index := range left.FileRefs {
		if left.FileRefs[index] != right.FileRefs[index] {
			return false
		}
	}
	return true
}

func (registry repositoryTargetAdapterRegistry) descriptor(
	key repositoryTargetAdapter,
) (repositoryTargetAdapterDescriptor, bool) {
	descriptor, ok := registry.byKey[key]
	return descriptor, ok
}

// ordinaryRepositoryTargetAdapterRegistry is deliberately an explicit list.
// There is no init-time self-registration, so supported adapters and their
// deterministic order remain visible in one reviewable place.
func ordinaryRepositoryTargetAdapterRegistry() (repositoryTargetAdapterRegistry, error) {
	return newRepositoryTargetAdapterRegistry(
		goRepositoryTargetAdapterDescriptor(),
		pythonRepositoryTargetAdapterDescriptor(),
		jstsRepositoryTargetAdapterDescriptor(),
	)
}

// repositoryTypedTarget is the neutral, complete selected-target snapshot.
// The fields needed by generic orchestration are first-class; native is an
// opaque adapter handle and cannot leak into cubes, reports, or browser data.
type repositoryTypedTarget struct {
	Key              repositoryTargetKey
	Selector         string
	Display          string
	Scope            targetoutcome.ScopeKind
	AllowedLanguages []string
	FileRefs         []corpus.FileID

	native any
}

func newRepositoryTypedTarget(
	registry repositoryTargetAdapterRegistry,
	key repositoryTargetKey,
	selector string,
	display string,
	scope targetoutcome.ScopeKind,
	native any,
) (repositoryTypedTarget, error) {
	descriptor, ok := registry.descriptor(key.Adapter)
	if !ok {
		return repositoryTypedTarget{}, fmt.Errorf(
			"repository target %q: adapter is not registered", key,
		)
	}
	target := repositoryTypedTarget{
		Key: key, Selector: selector, Display: display, Scope: scope,
		AllowedLanguages: append([]string(nil), descriptor.AllowedLanguages...),
		FileRefs:         []corpus.FileID{},
		native:           native,
	}
	if err := target.validateWith(registry); err != nil {
		return repositoryTypedTarget{}, err
	}
	return target, nil
}

func (target repositoryTypedTarget) Validate() error {
	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		return err
	}
	return target.validateWith(registry)
}

func (target repositoryTypedTarget) validateWith(
	registry repositoryTargetAdapterRegistry,
) error {
	if strings.TrimSpace(target.Key.Ref) == "" || target.Key.Ref != strings.TrimSpace(target.Key.Ref) ||
		strings.TrimSpace(target.Selector) == "" || target.Selector != strings.TrimSpace(target.Selector) ||
		strings.TrimSpace(target.Display) == "" || target.Display != strings.TrimSpace(target.Display) ||
		!target.Scope.Valid() || target.native == nil {
		return fmt.Errorf("repository target: invalid neutral identity")
	}
	descriptor, ok := registry.descriptor(target.Key.Adapter)
	if !ok {
		return fmt.Errorf("repository target %q: adapter is not registered", target.Key)
	}
	if err := validateRepositoryTargetLanguages(target.AllowedLanguages); err != nil {
		return fmt.Errorf("repository target %q: %w", target.Key, err)
	}
	if !equalRepositoryTargetLanguages(target.AllowedLanguages, descriptor.AllowedLanguages) {
		return fmt.Errorf("repository target %q: allowed languages do not match its adapter", target.Key)
	}
	for index, fileRef := range target.FileRefs {
		if strings.TrimSpace(string(fileRef)) == "" || string(fileRef) != strings.TrimSpace(string(fileRef)) {
			return fmt.Errorf("repository target %q: invalid file_ref at index %d", target.Key, index)
		}
		if index > 0 && target.FileRefs[index-1] >= fileRef {
			return fmt.Errorf("repository target %q: file refs are not canonical", target.Key)
		}
	}
	if err := descriptor.ValidateNative(target); err != nil {
		return fmt.Errorf("repository target %q: native authority: %w", target.Key, err)
	}
	return nil
}

func repositoryTypedTargetDisplay(target repositoryTypedTarget) string {
	if err := target.Validate(); err != nil {
		return target.Key.String()
	}
	return target.Display
}

func repositoryTypedTargetMatchesProgramTarget(
	target repositoryTypedTarget,
	programTarget programindex.Target,
) bool {
	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		return false
	}
	return repositoryTypedTargetMatchesProgramTargetWithRegistry(registry, target, programTarget)
}

func repositoryTypedTargetMatchesProgramTargetWithRegistry(
	registry repositoryTargetAdapterRegistry,
	target repositoryTypedTarget,
	programTarget programindex.Target,
) bool {
	if err := target.validateWith(registry); err != nil || programTarget.Validate() != nil {
		return false
	}
	descriptor, ok := registry.descriptor(target.Key.Adapter)
	if !ok || !repositoryTargetAllowsLanguage(target, programTarget.Language) {
		return false
	}
	return descriptor.MatchProgramTarget(target, programTarget)
}

func repositoryTargetAllowsLanguage(target repositoryTypedTarget, language string) bool {
	position := sort.SearchStrings(target.AllowedLanguages, language)
	return position < len(target.AllowedLanguages) && target.AllowedLanguages[position] == language
}

// buildRepositoryProgramPageAuthority is the generic adapter seam for a new
// compiler integration. It accepts one immutable fact snapshot, invokes one
// atomic ProgramInput callback, lets the common builder seal it, and binds the
// result to portable dependency authority for the shared page pipeline.
func buildRepositoryProgramPageAuthority(
	registry repositoryTargetAdapterRegistry,
	request repositoryProgramBuildRequest,
) (repositoryProgramPageAuthority, error) {
	if err := request.Target.validateWith(registry); err != nil {
		return repositoryProgramPageAuthority{}, fmt.Errorf("build program input: %w", err)
	}
	descriptor, ok := registry.descriptor(request.Target.Key.Adapter)
	if !ok || descriptor.BuildProgramInput == nil {
		return repositoryProgramPageAuthority{}, fmt.Errorf(
			"build program input: adapter %q has no atomic ProgramIndex builder",
			request.Target.Key.Adapter,
		)
	}
	input, err := descriptor.BuildProgramInput(request)
	if err != nil {
		return repositoryProgramPageAuthority{}, fmt.Errorf(
			"build program input: adapter %q: %w", request.Target.Key.Adapter, err,
		)
	}
	index, err := programindex.New(input)
	if err != nil {
		return repositoryProgramPageAuthority{}, fmt.Errorf(
			"build program input: adapter %q returned invalid common facts: %w",
			request.Target.Key.Adapter, err,
		)
	}
	if !repositoryTypedTargetMatchesProgramTargetWithRegistry(registry, request.Target, index.Target) {
		return repositoryProgramPageAuthority{}, fmt.Errorf(
			"build program input: adapter %q returned a different ProgramTarget",
			request.Target.Key.Adapter,
		)
	}
	catalog, err := descriptor.BuildDependencies(repositoryDependencyBuildRequest{
		Target: request.Target, ProgramIndex: index.Snapshot(), Facts: request.Facts,
	})
	if err != nil {
		return repositoryProgramPageAuthority{}, fmt.Errorf(
			"build dependency authority: adapter %q: %w", request.Target.Key.Adapter, err,
		)
	}
	if err := validateRepositoryDependencyLanguages(catalog, index.Target.Language); err != nil {
		return repositoryProgramPageAuthority{}, fmt.Errorf(
			"build dependency authority: adapter %q: %w", request.Target.Key.Adapter, err,
		)
	}
	return repositoryProgramPageAuthority{
		ProgramIndex: index, Dependencies: catalog, Label: descriptor.Label,
	}, nil
}

// ownRepositoryProgramPageAuthority validates and snapshots the already-built
// page boundary before the child run persists it. No adapter-native compiler
// facts survive this point.
func ownRepositoryProgramPageAuthority(
	registry repositoryTargetAdapterRegistry,
	target repositoryTypedTarget,
	page repositoryProgramPageAuthority,
) (repositoryProgramPageAuthority, error) {
	if err := target.validateWith(registry); err != nil {
		return repositoryProgramPageAuthority{}, err
	}
	descriptor, ok := registry.descriptor(target.Key.Adapter)
	if !ok {
		return repositoryProgramPageAuthority{}, fmt.Errorf(
			"repository target adapter %q is not registered", target.Key.Adapter,
		)
	}
	if page.Label != descriptor.Label {
		return repositoryProgramPageAuthority{}, fmt.Errorf(
			"repository target page label %q does not match adapter %q",
			page.Label, descriptor.Label,
		)
	}
	if err := page.ProgramIndex.Validate(); err != nil {
		return repositoryProgramPageAuthority{}, fmt.Errorf("validate ProgramIndex: %w", err)
	}
	if !repositoryTypedTargetMatchesProgramTargetWithRegistry(
		registry, target, page.ProgramIndex.Target,
	) {
		return repositoryProgramPageAuthority{}, fmt.Errorf(
			"ProgramIndex target does not match exact selected target",
		)
	}
	if err := validateRepositoryDependencyLanguages(
		page.Dependencies, page.ProgramIndex.Target.Language,
	); err != nil {
		return repositoryProgramPageAuthority{}, fmt.Errorf("validate dependencies: %w", err)
	}
	ownedDependencies, err := dependencies.BuildWithOmissions(
		page.Dependencies.Importers,
		page.Dependencies.Dependencies,
		page.Dependencies.Coverage.Omissions,
	)
	if err != nil {
		return repositoryProgramPageAuthority{}, fmt.Errorf("own dependencies: %w", err)
	}
	return repositoryProgramPageAuthority{
		ProgramIndex: page.ProgramIndex.Snapshot(),
		Dependencies: ownedDependencies,
		Label:        page.Label,
	}, nil
}

func validateRepositoryDependencyLanguages(catalog dependencies.Catalog, language string) error {
	if err := catalog.Validate(); err != nil {
		return err
	}
	if catalog.Coverage.State != dependencies.CoverageComplete {
		return fmt.Errorf("dependency coverage is incomplete")
	}
	for _, importer := range catalog.Importers {
		if importer.Language != language {
			return fmt.Errorf("importer language %q does not match ProgramTarget %q", importer.Language, language)
		}
	}
	for _, dependency := range catalog.Dependencies {
		if dependency.Language != language {
			return fmt.Errorf("dependency language %q does not match ProgramTarget %q", dependency.Language, language)
		}
	}
	return nil
}

func repositoryTypedTargetLess(left, right repositoryTypedTarget) bool {
	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		return left.Key.String() < right.Key.String()
	}
	leftDescriptor, leftKnown := registry.descriptor(left.Key.Adapter)
	rightDescriptor, rightKnown := registry.descriptor(right.Key.Adapter)
	if leftKnown != rightKnown {
		return leftKnown
	}
	if leftKnown && leftDescriptor.Rank != rightDescriptor.Rank {
		return leftDescriptor.Rank < rightDescriptor.Rank
	}
	if left.Selector != right.Selector {
		return left.Selector < right.Selector
	}
	return left.Key.Ref < right.Key.Ref
}

func explicitNonGoRepositoryTargetSelector(value string) bool {
	value = strings.TrimSpace(value)
	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		return false
	}
	for _, descriptor := range registry.ordered {
		for _, prefix := range descriptor.SelectorPrefixes {
			if strings.HasPrefix(value, prefix) {
				return true
			}
		}
	}
	return false
}

func newGoRepositoryTypedTarget(
	target analysistarget.Target,
	selector string,
) (repositoryTypedTarget, error) {
	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		return repositoryTypedTarget{}, err
	}
	scope := targetoutcome.ScopeLibrary
	if target.Kind == analysistarget.KindExecutablePackage {
		scope = targetoutcome.ScopeExecutable
	}
	return newRepositoryTypedTarget(
		registry,
		repositoryTargetKey{Adapter: repositoryTargetAdapterGo, Ref: target.Ref},
		selector,
		target.DisplayPath(),
		scope,
		target.Snapshot(),
	)
}

func newPythonRepositoryTypedTarget(target pythontarget.Target) (repositoryTypedTarget, error) {
	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		return repositoryTypedTarget{}, err
	}
	scope := targetoutcome.ScopeLibrary
	if target.Kind == pythontarget.KindExecutable {
		scope = targetoutcome.ScopeExecutable
	}
	return newRepositoryTypedTarget(
		registry,
		repositoryTargetKey{Adapter: repositoryTargetAdapterPython, Ref: target.Ref},
		target.Selector,
		target.DisplayName,
		scope,
		target,
	)
}

func newJSTSRepositoryTypedTarget(target jstsproject.Target) (repositoryTypedTarget, error) {
	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		return repositoryTypedTarget{}, err
	}
	return newRepositoryTypedTarget(
		registry,
		repositoryTargetKey{Adapter: repositoryTargetAdapterJSTS, Ref: target.Ref},
		target.Selector,
		target.Name,
		targetoutcome.ScopePackage,
		target,
	)
}

func repositoryGoTarget(target repositoryTypedTarget) (analysistarget.Target, bool) {
	value, ok := target.native.(analysistarget.Target)
	return value, ok
}

func repositoryPythonTarget(target repositoryTypedTarget) (pythontarget.Target, bool) {
	value, ok := target.native.(pythontarget.Target)
	return value, ok
}

func repositoryJSTSTarget(target repositoryTypedTarget) (jstsproject.Target, bool) {
	value, ok := target.native.(jstsproject.Target)
	return value, ok
}

func goRepositoryTargetAdapterDescriptor() repositoryTargetAdapterDescriptor {
	return repositoryTargetAdapterDescriptor{
		Key: repositoryTargetAdapterGo, Rank: 0, Label: "Go",
		AllowedLanguages:      []string{"go"},
		Discover:              discoverGoRepositoryTargets,
		PrepareDispatchPlan:   prepareGoRepositoryDispatchPlan,
		PrepareDispatchTarget: prepareGoRepositoryDispatchTarget,
		ValidateNative: func(target repositoryTypedTarget) error {
			value, ok := repositoryGoTarget(target)
			if !ok {
				return fmt.Errorf("expected Go target handle")
			}
			if err := value.Validate(); err != nil {
				return err
			}
			if target.Key.Ref != value.Ref {
				return fmt.Errorf("exact ref mismatch")
			}
			return nil
		},
		MatchProgramTarget: func(target repositoryTypedTarget, programTarget programindex.Target) bool {
			value, ok := repositoryGoTarget(target)
			if !ok {
				return false
			}
			name := value.PackagePath
			kind := "executable"
			if value.Kind == analysistarget.KindModuleLibrary {
				name = value.ModulePath
				kind = "library"
			}
			return programTarget.Name == name && programTarget.Selector == name && programTarget.Kind == kind
		},
		ValidatePlanAuthority: func(authority any, target repositoryTypedTarget) error {
			source, ok := authority.(snapshot.Snapshot)
			if !ok || source.AnalysisTarget != nil || source.GoFacts == nil || source.TargetCatalog == nil {
				return fmt.Errorf("missing unscoped Go source authority")
			}
			if err := source.TargetCatalog.Validate(); err != nil {
				return err
			}
			rebuilt, err := analysistarget.BuildCatalog(*source.GoFacts)
			if err != nil || rebuilt.Ref != source.TargetCatalog.Ref {
				return fmt.Errorf("source target catalog does not match Go facts")
			}
			entry, found := targetCatalogEntryByRef(*source.TargetCatalog, target.Key.Ref)
			value, valueOK := repositoryGoTarget(target)
			if !found || !valueOK || entry.Candidate.Key != target.Selector ||
				entry.Candidate.Target.Ref != value.Ref {
				return fmt.Errorf("target is outside exact source authority")
			}
			return nil
		},
		BuildProgramInput: buildGoRepositoryProgramInput,
		BuildDependencies: buildGoRepositoryDependencies,
	}
}

func pythonRepositoryTargetAdapterDescriptor() repositoryTargetAdapterDescriptor {
	return repositoryTargetAdapterDescriptor{
		Key: repositoryTargetAdapterPython, Rank: 1, Label: "Python",
		AllowedLanguages: []string{"python"}, SelectorPrefixes: []string{"python:"},
		Discover:              discoverPythonRepositoryTargets,
		PrepareDispatchPlan:   preparePythonRepositoryDispatchPlan,
		PrepareDispatchTarget: preparePythonRepositoryDispatchTarget,
		ValidateNative: func(target repositoryTypedTarget) error {
			value, ok := repositoryPythonTarget(target)
			if !ok {
				return fmt.Errorf("expected Python target handle")
			}
			if err := value.Validate(); err != nil {
				return err
			}
			if target.Key.Ref != value.Ref || target.Selector != value.Selector {
				return fmt.Errorf("exact identity mismatch")
			}
			return nil
		},
		MatchProgramTarget: func(target repositoryTypedTarget, programTarget programindex.Target) bool {
			value, ok := repositoryPythonTarget(target)
			return ok && programTarget.Selector == value.Selector && programTarget.Name == value.DisplayName
		},
		ValidatePlanAuthority: func(authority any, target repositoryTypedTarget) error {
			catalog, ok := authority.(pythontarget.Catalog)
			value, valueOK := repositoryPythonTarget(target)
			if !ok || !valueOK || catalog.Validate() != nil || !catalog.OwnsTarget(value) {
				return fmt.Errorf("target is outside exact catalog authority")
			}
			return nil
		},
		BuildProgramInput: buildPythonRepositoryProgramInput,
		BuildDependencies: buildPythonRepositoryDependencies,
	}
}

func jstsRepositoryTargetAdapterDescriptor() repositoryTargetAdapterDescriptor {
	return repositoryTargetAdapterDescriptor{
		Key: repositoryTargetAdapterJSTS, Rank: 2, Label: "JavaScript/TypeScript",
		AllowedLanguages:      []string{"javascript", "typescript"},
		SelectorPrefixes:      []string{"jsts:"},
		Discover:              discoverJSTSRepositoryTargets,
		PrepareDispatchPlan:   prepareJSTSRepositoryDispatchPlan,
		PrepareDispatchTarget: prepareJSTSRepositoryDispatchTarget,
		ValidateNative: func(target repositoryTypedTarget) error {
			value, ok := repositoryJSTSTarget(target)
			if !ok {
				return fmt.Errorf("expected JavaScript/TypeScript target handle")
			}
			if err := value.Validate(); err != nil {
				return err
			}
			if target.Key.Ref != value.Ref || target.Selector != value.Selector {
				return fmt.Errorf("exact identity mismatch")
			}
			return nil
		},
		MatchProgramTarget: func(target repositoryTypedTarget, programTarget programindex.Target) bool {
			value, ok := repositoryJSTSTarget(target)
			return ok && programTarget.Selector == value.Selector && programTarget.Name == value.Name &&
				programTarget.AnchorFileRef == value.ManifestFileRef
		},
		ValidatePlanAuthority: func(authority any, _ repositoryTypedTarget) error {
			if authority != nil {
				return fmt.Errorf("unexpected retained plan authority")
			}
			return nil
		},
		BuildProgramInput: buildJSTSRepositoryProgramInput,
		BuildDependencies: buildJSTSRepositoryDependencies,
	}
}
