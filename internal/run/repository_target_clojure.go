package run

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/clojureproject"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

const repositoryTargetAdapterClojure repositoryTargetAdapter = "clojure"

func newClojureRepositoryTypedTarget(target clojureproject.Target) (repositoryTypedTarget, error) {
	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		return repositoryTypedTarget{}, err
	}
	return newRepositoryTypedTarget(registry, repositoryTargetKey{Adapter: repositoryTargetAdapterClojure, Ref: target.Ref}, target.Selector, target.Name, targetoutcome.ScopePackage, target)
}

func clojureRepositoryTargetAdapterDescriptor() repositoryTargetAdapterDescriptor {
	return repositoryTargetAdapterDescriptor{
		Key: repositoryTargetAdapterClojure, Rank: 3, Label: "Clojure", AllowedLanguages: []string{"clojure"}, SelectorPrefixes: []string{"clojure:"},
		Discover:            discoverClojureRepositoryTargets,
		PrepareDispatchPlan: func(repositoryTargetPlan, []repositoryTypedTarget) (any, error) { return nil, nil },
		PrepareDispatchTarget: func(ctx context.Context, options repositoryTargetDispatchOptions, target repositoryTypedTarget, _ any) (repositoryTargetDispatchBinding, error) {
			native, ok := target.native.(clojureproject.Target)
			if !ok {
				return repositoryTargetDispatchBinding{}, fmt.Errorf("invalid Clojure target")
			}
			result, err := clojureproject.Build(ctx, options.Repo, options.Corpus, native)
			if err != nil {
				return repositoryTargetDispatchBinding{}, err
			}
			return repositoryTargetDispatchBinding{Target: target, ProgramFacts: result, ProgramFactsBound: true}, nil
		},
		ValidateNative: func(target repositoryTypedTarget) error {
			native, ok := target.native.(clojureproject.Target)
			if !ok {
				return fmt.Errorf("invalid Clojure target")
			}
			if target.Key.Ref != native.Ref || target.Selector != native.Selector {
				return fmt.Errorf("Clojure target identity mismatch")
			}
			return native.Validate()
		},
		MatchProgramTarget: func(target repositoryTypedTarget, program programindex.Target) bool {
			native, ok := target.native.(clojureproject.Target)
			return ok && program.Selector == native.Selector && program.Name == native.Name && program.AnchorFileRef == native.ManifestFileRef
		},
		ValidatePlanAuthority: func(authority any, _ repositoryTypedTarget) error {
			if authority != nil {
				return fmt.Errorf("unexpected Clojure plan authority")
			}
			return nil
		},
		BuildProgramInput: func(request repositoryProgramBuildRequest) (programindex.Input, error) {
			result, err := clojureRepositoryFacts(request.Target, request.Facts)
			if err != nil {
				return programindex.Input{}, err
			}
			if err := result.Target.ValidateAgainst(request.Corpus); err != nil {
				return programindex.Input{}, err
			}
			return result.Input, nil
		},
		BuildDependencies: func(request repositoryDependencyBuildRequest) (dependencies.Catalog, error) {
			result, err := clojureRepositoryFacts(request.Target, request.Facts)
			if err != nil {
				return dependencies.Catalog{}, err
			}
			return result.Dependencies, nil
		},
	}
}
func clojureRepositoryFacts(target repositoryTypedTarget, facts any) (*clojureproject.Result, error) {
	result, ok := facts.(*clojureproject.Result)
	if !ok || result == nil || target.Key.Ref != result.Target.Ref {
		return nil, fmt.Errorf("Clojure native fact binding mismatch")
	}
	return result, nil
}

func discoverClojureRepositoryTargets(_ context.Context, options repositoryTargetRuntimeOptions) (repositoryTargetAdapterDiscovery, bool, error) {
	name := options.RepoName
	if name == "" {
		name = "Clojure project"
	}
	targets, err := clojureproject.Scout(options.Repository, name)
	if err != nil {
		return repositoryTargetAdapterDiscovery{}, false, err
	}
	if len(targets) == 0 {
		return repositoryTargetAdapterDiscovery{}, false, nil
	}
	byManifest := map[corpus.FileID]clojureproject.Target{}
	discovery := repositoryTargetAdapterDiscovery{Key: repositoryTargetAdapterClojure}
	for _, target := range targets {
		ref := corpus.FileID(target.ManifestFileRef)
		byManifest[ref] = target
		discovery.RequiredFileRefs = append(discovery.RequiredFileRefs, ref)
		discovery.Candidates = append(discovery.Candidates, analysistarget.FileCandidate{FileRef: ref, Hypotheses: []string{"Clojure JVM project with a native dependency manifest and its complete owned .clj/.cljc source scope"}})
	}
	sort.Slice(discovery.RequiredFileRefs, func(i, j int) bool { return discovery.RequiredFileRefs[i] < discovery.RequiredFileRefs[j] })
	discovery.ResolvesFile = func(ref corpus.FileID) bool { _, ok := byManifest[ref]; return ok }
	discovery.RestoreFiles = func(refs []corpus.FileID) ([]repositoryTargetFileRestoration, error) {
		var out []repositoryTargetFileRestoration
		for _, ref := range refs {
			native, ok := byManifest[ref]
			if !ok {
				return nil, fmt.Errorf("unknown Clojure manifest %s", ref)
			}
			target, err := newClojureRepositoryTypedTarget(native)
			if err != nil {
				return nil, err
			}
			out = append(out, repositoryTargetFileRestoration{Target: target, FileRefs: []corpus.FileID{ref}})
		}
		return out, nil
	}
	discovery.ResolveExplicit = func(_ *corpus.Corpus, selector string) ([]repositoryTypedTarget, error) {
		for _, native := range targets {
			if native.Selector == selector || native.Ref == selector {
				target, err := newClojureRepositoryTypedTarget(native)
				if err != nil {
					return nil, err
				}
				return []repositoryTypedTarget{target}, nil
			}
		}
		return nil, nil
	}
	discovery.NativeEvidence = func(target repositoryTypedTarget) (repositoryNativeEvidence, error) {
		native, ok := target.native.(clojureproject.Target)
		if !ok {
			return repositoryNativeEvidence{}, fmt.Errorf("invalid Clojure target")
		}
		return repositoryNativeEvidence{Root: native.ProjectDir}, nil
	}
	discovery.ChoiceGroup = func() (targetPortfolioChoiceGroup, error) {
		var choices []string
		for _, target := range targets {
			choices = append(choices, target.Selector)
		}
		return targetPortfolioChoiceGroup{Language: "Clojure", Choices: strings.Join(choices, ", ")}, nil
	}
	discovery.SnapshotAuthority = func() (any, error) { return nil, nil }
	return discovery, true, nil
}
