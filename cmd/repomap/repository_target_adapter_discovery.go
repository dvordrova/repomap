package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/snapshot"
)

func discoverGoRepositoryTargets(
	_ context.Context,
	options repositoryTargetRuntimeOptions,
) (repositoryTargetAdapterDiscovery, bool, error) {
	if options.GoSnapshot == nil {
		return repositoryTargetAdapterDiscovery{}, false, nil
	}
	owned, err := snapshot.OwnSnapshot(*options.GoSnapshot)
	if err != nil {
		return repositoryTargetAdapterDiscovery{}, false, fmt.Errorf("own prepared Go snapshot: %w", err)
	}
	if owned.AnalysisTarget != nil || owned.GoFacts == nil || owned.TargetCatalog == nil ||
		len(owned.TargetCatalog.Entries) == 0 {
		return repositoryTargetAdapterDiscovery{}, false,
			fmt.Errorf("prepared Go snapshot must be unscoped and contain exact facts plus target catalog")
	}
	if err := owned.TargetCatalog.Validate(); err != nil {
		return repositoryTargetAdapterDiscovery{}, false, fmt.Errorf("validate prepared Go target catalog: %w", err)
	}
	rebuilt, err := analysistarget.BuildCatalog(*owned.GoFacts)
	if err != nil || rebuilt.Ref != owned.TargetCatalog.Ref {
		return repositoryTargetAdapterDiscovery{}, false,
			fmt.Errorf("prepared Go target catalog does not match its facts")
	}
	candidates, resolver, err := analysistarget.DiscoverGoTargetFilesWithResolver(
		options.Repository, *owned.GoFacts, *owned.TargetCatalog,
	)
	if err != nil {
		return repositoryTargetAdapterDiscovery{}, false, fmt.Errorf("discover Go target files: %w", err)
	}
	targetRefs := make([]string, len(owned.TargetCatalog.Entries))
	for index, entry := range owned.TargetCatalog.Entries {
		targetRefs[index] = entry.Candidate.Target.Ref
	}
	required, err := canonicalNativeTargetFileRefs(
		"Go", candidates, targetRefs,
		func(fileRef corpus.FileID) ([]string, error) {
			return resolver.Resolve([]corpus.FileID{fileRef})
		},
	)
	if err != nil {
		return repositoryTargetAdapterDiscovery{}, false, err
	}
	discovery := repositoryTargetAdapterDiscovery{
		Key: repositoryTargetAdapterGo, Candidates: candidates,
		RequiredFileRefs: required, Authority: owned,
		ResolvesFile: resolver.ResolvesOne,
	}
	discovery.RestoreFiles = func(fileRefs []corpus.FileID) ([]repositoryTargetFileRestoration, error) {
		refs, resolveErr := resolver.Resolve(fileRefs)
		if resolveErr != nil {
			return nil, fmt.Errorf("restore selected Go targets: %w", resolveErr)
		}
		targets := make(map[repositoryTargetKey]repositoryTypedTarget, len(refs))
		files := make(map[repositoryTargetKey][]corpus.FileID, len(refs))
		for _, ref := range refs {
			entry, found := targetCatalogEntryByRef(*owned.TargetCatalog, ref)
			if !found {
				return nil, fmt.Errorf("restored Go target %q is outside exact catalog", ref)
			}
			target, targetErr := newGoRepositoryTypedTarget(entry.Candidate.Target, entry.Candidate.Key)
			if targetErr != nil {
				return nil, targetErr
			}
			targets[target.Key] = target
		}
		for _, fileRef := range fileRefs {
			resolved, resolveErr := resolver.Resolve([]corpus.FileID{fileRef})
			if resolveErr != nil {
				return nil, fmt.Errorf("restore Go file_ref %q: %w", fileRef, resolveErr)
			}
			for _, ref := range resolved {
				key := repositoryTargetKey{Adapter: repositoryTargetAdapterGo, Ref: ref}
				files[key] = append(files[key], fileRef)
			}
		}
		return repositoryTargetRestorations(targets, files), nil
	}
	discovery.ResolveExplicit = func(_ *corpus.Corpus, override string) ([]repositoryTypedTarget, error) {
		matches := []repositoryTypedTarget{}
		for _, entry := range owned.TargetCatalog.Entries {
			if override != entry.Candidate.Key && override != entry.Candidate.Target.Ref {
				continue
			}
			target, targetErr := newGoRepositoryTypedTarget(entry.Candidate.Target, entry.Candidate.Key)
			if targetErr != nil {
				return nil, targetErr
			}
			matches = append(matches, target)
		}
		return matches, nil
	}
	discovery.ChoiceGroup = func() (targetPortfolioChoiceGroup, error) {
		return targetPortfolioChoiceGroup{
			Language: "Go", Choices: targetPortfolioChoices(*owned.TargetCatalog),
		}, nil
	}
	discovery.SnapshotAuthority = func() (any, error) {
		return snapshot.OwnSnapshot(owned)
	}
	return discovery, true, nil
}

func discoverPythonRepositoryTargets(
	ctx context.Context,
	options repositoryTargetRuntimeOptions,
) (repositoryTargetAdapterDiscovery, bool, error) {
	if !options.DiscoverPython {
		return repositoryTargetAdapterDiscovery{}, false, nil
	}
	catalog, err := pythontarget.Discover(ctx, options.Repository)
	if err != nil {
		return repositoryTargetAdapterDiscovery{}, false, fmt.Errorf("discover Python targets: %w", err)
	}
	if err := catalog.Validate(); err != nil {
		return repositoryTargetAdapterDiscovery{}, false, fmt.Errorf("validate Python target catalog: %w", err)
	}
	catalog = catalog.Snapshot()
	reportPythonTargetCatalogScaleWarnings(options.Output, catalog)
	candidates, resolver, err := pythontarget.FileCandidatesWithResolver(options.Repository, catalog)
	if err != nil {
		return repositoryTargetAdapterDiscovery{}, false,
			fmt.Errorf("project Python targets into repository portfolio: %w", err)
	}
	targetRefs := make([]string, len(catalog.Entries))
	for index, target := range catalog.Entries {
		targetRefs[index] = target.Ref
	}
	required, err := canonicalNativeTargetFileRefs(
		"Python", candidates, targetRefs,
		func(fileRef corpus.FileID) ([]string, error) {
			targets, resolveErr := resolver.Resolve([]corpus.FileID{fileRef})
			if resolveErr != nil {
				return nil, resolveErr
			}
			refs := make([]string, len(targets))
			for index, target := range targets {
				refs[index] = target.Ref
			}
			return refs, nil
		},
	)
	if err != nil {
		return repositoryTargetAdapterDiscovery{}, false, err
	}
	discovery := repositoryTargetAdapterDiscovery{
		Key: repositoryTargetAdapterPython, Candidates: candidates,
		RequiredFileRefs: required, Authority: catalog,
		ResolvesFile: resolver.Resolves,
	}
	discovery.RestoreFiles = func(fileRefs []corpus.FileID) ([]repositoryTargetFileRestoration, error) {
		resolved, resolveErr := resolver.Resolve(fileRefs)
		if resolveErr != nil {
			return nil, fmt.Errorf("restore selected Python targets: %w", resolveErr)
		}
		targets := make(map[repositoryTargetKey]repositoryTypedTarget, len(resolved))
		files := make(map[repositoryTargetKey][]corpus.FileID, len(resolved))
		for _, value := range resolved {
			if !catalog.OwnsTarget(value) {
				return nil, fmt.Errorf("restored Python target %q is outside exact catalog", value.Ref)
			}
			target, targetErr := newPythonRepositoryTypedTarget(value)
			if targetErr != nil {
				return nil, targetErr
			}
			targets[target.Key] = target
		}
		for _, fileRef := range fileRefs {
			values, resolveErr := resolver.Resolve([]corpus.FileID{fileRef})
			if resolveErr != nil {
				return nil, fmt.Errorf("restore Python file_ref %q: %w", fileRef, resolveErr)
			}
			for _, value := range values {
				key := repositoryTargetKey{Adapter: repositoryTargetAdapterPython, Ref: value.Ref}
				files[key] = append(files[key], fileRef)
			}
		}
		return repositoryTargetRestorations(targets, files), nil
	}
	discovery.ResolveExplicit = func(repository *corpus.Corpus, override string) ([]repositoryTypedTarget, error) {
		matches := make(map[repositoryTargetKey]repositoryTypedTarget)
		nativeAnchorMatched := false
		if repository != nil {
			if fileRef, known := repository.ID(override); known {
				for _, entry := range catalog.Entries {
					if entry.AnchorFileRef != fileRef {
						continue
					}
					nativeAnchorMatched = true
					target, targetErr := newPythonRepositoryTypedTarget(entry)
					if targetErr != nil {
						return nil, targetErr
					}
					matches[target.Key] = target
				}
			}
		}
		for _, entry := range catalog.Entries {
			if override != entry.Selector && override != entry.Ref && override != entry.IdentityRef {
				continue
			}
			target, targetErr := newPythonRepositoryTypedTarget(entry)
			if targetErr != nil {
				return nil, targetErr
			}
			matches[target.Key] = target
		}
		if !nativeAnchorMatched {
			derived, found, resolveErr := resolver.ResolveSelector(override)
			if resolveErr != nil {
				return nil, fmt.Errorf("resolve exact Python selector: %w", resolveErr)
			}
			if found {
				target, targetErr := newPythonRepositoryTypedTarget(derived)
				if targetErr != nil {
					return nil, targetErr
				}
				matches[target.Key] = target
			}
		}
		return repositoryTargetMapValues(matches), nil
	}
	discovery.ChoiceGroup = func() (targetPortfolioChoiceGroup, error) {
		choices, choiceErr := pythonExactTargetChoices(catalog, resolver)
		if choiceErr != nil {
			return targetPortfolioChoiceGroup{}, choiceErr
		}
		return targetPortfolioChoiceGroup{Language: "Python", Choices: choices}, nil
	}
	discovery.SnapshotAuthority = func() (any, error) { return catalog.Snapshot(), nil }
	return discovery, true, nil
}

func discoverJSTSRepositoryTargets(
	ctx context.Context,
	options repositoryTargetRuntimeOptions,
) (repositoryTargetAdapterDiscovery, bool, error) {
	if !options.DiscoverJSTS {
		return repositoryTargetAdapterDiscovery{}, false, nil
	}
	scout := options.ScoutJSTSFn
	if scout == nil {
		scout = jstsproject.ScoutTargets
	}
	targets, err := scout(ctx, options.Repository, exactJSTSManifestSelector(options.TargetOverride))
	if err != nil {
		return repositoryTargetAdapterDiscovery{}, false,
			fmt.Errorf("scout JavaScript/TypeScript package target: %w", err)
	}
	if len(targets) == 0 {
		return repositoryTargetAdapterDiscovery{}, false,
			fmt.Errorf("JavaScript/TypeScript target scout returned no exact package targets")
	}
	byManifest := make(map[corpus.FileID]jstsproject.Target, len(targets))
	candidates := make([]analysistarget.FileCandidate, 0, len(targets))
	required := make([]corpus.FileID, 0, len(targets))
	seenRefs := make(map[string]struct{}, len(targets))
	for index, target := range targets {
		if err := target.ValidateAgainst(options.Repository); err != nil {
			return repositoryTargetAdapterDiscovery{}, false,
				fmt.Errorf("bind JavaScript/TypeScript scout target %d to the current repository: %w", index, err)
		}
		if index > 0 && targets[index-1].Selector >= target.Selector {
			return repositoryTargetAdapterDiscovery{}, false,
				fmt.Errorf("JavaScript/TypeScript scout targets are not canonical")
		}
		if _, duplicate := seenRefs[target.Ref]; duplicate {
			return repositoryTargetAdapterDiscovery{}, false,
				fmt.Errorf("JavaScript/TypeScript scout targets share ref %q", target.Ref)
		}
		seenRefs[target.Ref] = struct{}{}
		manifest := corpus.FileID(target.ManifestFileRef)
		if _, duplicate := byManifest[manifest]; duplicate {
			return repositoryTargetAdapterDiscovery{}, false,
				fmt.Errorf("JavaScript/TypeScript scout targets share manifest file_ref %q", manifest)
		}
		byManifest[manifest] = target
		required = append(required, manifest)
		candidates = append(candidates, analysistarget.FileCandidate{
			FileRef: manifest,
			Hypotheses: []string{
				"JavaScript/TypeScript package project with an exact tracked manifest and owned source-file evidence",
			},
		})
	}
	sort.Slice(required, func(i, j int) bool { return required[i] < required[j] })
	discovery := repositoryTargetAdapterDiscovery{
		Key: repositoryTargetAdapterJSTS, Candidates: candidates,
		RequiredFileRefs: required, Authority: nil,
		ResolvesFile: func(fileRef corpus.FileID) bool {
			_, ok := byManifest[fileRef]
			return ok
		},
	}
	discovery.RestoreFiles = func(fileRefs []corpus.FileID) ([]repositoryTargetFileRestoration, error) {
		result := make([]repositoryTargetFileRestoration, 0, len(fileRefs))
		for _, fileRef := range fileRefs {
			value, ok := byManifest[fileRef]
			if !ok {
				return nil, fmt.Errorf(
					"restored JavaScript/TypeScript file_ref %q is not an exact package target manifest", fileRef,
				)
			}
			target, targetErr := newJSTSRepositoryTypedTarget(value)
			if targetErr != nil {
				return nil, targetErr
			}
			result = append(result, repositoryTargetFileRestoration{Target: target, FileRefs: []corpus.FileID{fileRef}})
		}
		return result, nil
	}
	discovery.ResolveExplicit = func(_ *corpus.Corpus, override string) ([]repositoryTypedTarget, error) {
		matches := []repositoryTypedTarget{}
		for _, value := range targets {
			if override != value.Selector && override != value.Ref {
				continue
			}
			target, targetErr := newJSTSRepositoryTypedTarget(value)
			if targetErr != nil {
				return nil, targetErr
			}
			matches = append(matches, target)
		}
		return matches, nil
	}
	discovery.ChoiceGroup = func() (targetPortfolioChoiceGroup, error) {
		choices := make([]string, len(targets))
		for index, target := range targets {
			choices[index] = target.Selector + " (" + target.ManifestPath + ")"
		}
		return targetPortfolioChoiceGroup{
			Language: "JavaScript/TypeScript", Choices: strings.Join(choices, ", "),
		}, nil
	}
	discovery.SnapshotAuthority = func() (any, error) { return nil, nil }
	return discovery, true, nil
}

func repositoryTargetRestorations(
	targets map[repositoryTargetKey]repositoryTypedTarget,
	files map[repositoryTargetKey][]corpus.FileID,
) []repositoryTargetFileRestoration {
	result := make([]repositoryTargetFileRestoration, 0, len(targets))
	for key, target := range targets {
		refs := append([]corpus.FileID(nil), files[key]...)
		sort.Slice(refs, func(i, j int) bool { return refs[i] < refs[j] })
		result = append(result, repositoryTargetFileRestoration{Target: target, FileRefs: refs})
	}
	sort.Slice(result, func(i, j int) bool {
		return repositoryTypedTargetLess(result[i].Target, result[j].Target)
	})
	return result
}

func repositoryTargetMapValues(
	values map[repositoryTargetKey]repositoryTypedTarget,
) []repositoryTypedTarget {
	result := make([]repositoryTypedTarget, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return repositoryTypedTargetLess(result[i], result[j]) })
	return result
}
