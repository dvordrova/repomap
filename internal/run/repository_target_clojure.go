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
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

const repositoryTargetAdapterClojure repositoryTargetAdapter = "clojure"

func newClojureRepositoryTypedTarget(target clojureproject.Target) (repositoryTypedTarget, error) {
	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		return repositoryTypedTarget{}, err
	}
	// A shadow-cljs build is a program a person runs; the JVM project is a
	// package, whatever its aliases start.
	scope := targetoutcome.ScopePackage
	if target.Platform == "cljs" {
		scope = targetoutcome.ScopeExecutable
	}
	return newRepositoryTypedTarget(registry, repositoryTargetKey{Adapter: repositoryTargetAdapterClojure, Ref: target.Ref}, target.Selector, target.Name, scope, target)
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

// clojureTargetHypothesis is what a Clojure manifest offers the target
// portfolio: the JVM project it describes, or the ClojureScript programs its
// shadow-cljs builds start.
func clojureTargetHypothesis(target clojureproject.Target) string {
	if target.Platform != "cljs" {
		return "Clojure JVM project with a native dependency manifest and its complete owned .clj/.cljc source scope"
	}
	return "shadow-cljs build description whose builds are ClojureScript programs over the owned .cljs/.cljc sources, each started from the function or namespace its build names"
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
	builds, err := clojureproject.ScoutShadow(options.Repository)
	if err != nil {
		return repositoryTargetAdapterDiscovery{}, false, err
	}
	targets = append(targets, builds...)
	if len(targets) == 0 {
		return repositoryTargetAdapterDiscovery{}, false, nil
	}
	// A shadow-cljs.edn restores every build it describes; each build is a
	// target of its own.
	byManifest := map[corpus.FileID][]clojureproject.Target{}
	discovery := repositoryTargetAdapterDiscovery{Key: repositoryTargetAdapterClojure}
	for _, target := range targets {
		ref := corpus.FileID(target.ManifestFileRef)
		if len(byManifest[ref]) == 0 {
			discovery.RequiredFileRefs = append(discovery.RequiredFileRefs, ref)
			discovery.Candidates = append(discovery.Candidates, analysistarget.FileCandidate{FileRef: ref, Hypotheses: []string{clojureTargetHypothesis(target)}})
		}
		byManifest[ref] = append(byManifest[ref], target)
	}
	sort.Slice(discovery.RequiredFileRefs, func(i, j int) bool { return discovery.RequiredFileRefs[i] < discovery.RequiredFileRefs[j] })
	discovery.ResolvesFile = func(ref corpus.FileID) bool { return len(byManifest[ref]) > 0 }
	discovery.RestoreFiles = func(refs []corpus.FileID) ([]repositoryTargetFileRestoration, error) {
		var out []repositoryTargetFileRestoration
		for _, ref := range refs {
			natives := byManifest[ref]
			if len(natives) == 0 {
				return nil, fmt.Errorf("unknown Clojure manifest %s", ref)
			}
			for _, native := range natives {
				target, err := newClojureRepositoryTypedTarget(native)
				if err != nil {
					return nil, err
				}
				out = append(out, repositoryTargetFileRestoration{Target: target, FileRefs: []corpus.FileID{ref}})
			}
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
		evidence := repositoryNativeEvidence{Root: native.ProjectDir}
		// A build's entries are what the shadow-cljs.edn writes it starts from.
		for _, entry := range native.Entries {
			evidence.Observations = append(evidence.Observations, targetportfolio.Observation{
				Kind: "shadow_cljs_entry", Path: native.ManifestPath, Line: entry.Line,
				Fields: map[string]string{"build": native.Build, "key": entry.Key}, Values: []string{entry.Symbol},
			})
		}
		return evidence, nil
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
