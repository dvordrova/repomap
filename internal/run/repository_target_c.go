package run

import (
	"context"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/cproject"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/targetoutcome"
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

// The file hypotheses a C program offers the target portfolio. A linked
// program offers the files only it compiles; a program the build does not
// link offers its main file; a directory's unlinked files offer themselves.
// A linked program whose every file is shared offers its link line's file.
const (
	cLinkedFileHypothesis  = "compiled into exactly one C program that the build links"
	cMainFileHypothesis    = "contains an exact non-static C main function, in a file no build link line links"
	cLibraryFileHypothesis = "C source file that no build link line links and that defines no main"
	cLinkFileHypothesis    = "C build description linking programs whose every file another program links too"
)

func newCRepositoryTypedTarget(program cproject.Program) (repositoryTypedTarget, error) {
	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		return repositoryTypedTarget{}, err
	}
	scope := targetoutcome.ScopeLibrary
	if program.Kind == cproject.ProgramExecutable {
		scope = targetoutcome.ScopeExecutable
	}
	return newRepositoryTypedTarget(registry, repositoryTargetKey{Adapter: repositoryTargetAdapterC, Ref: program.Ref}, program.Selector, program.Name, scope, program)
}

func cRepositoryTargetAdapterDescriptor() repositoryTargetAdapterDescriptor {
	return repositoryTargetAdapterDescriptor{
		Key: repositoryTargetAdapterC, Rank: 4, Label: "C", AllowedLanguages: []string{"c"}, SelectorPrefixes: []string{"c:"},
		Discover: discoverCRepositoryTargets,
		PrepareDispatchPlan: func(_ repositoryTargetPlan, ordered []repositoryTypedTarget) (any, error) {
			return newCRepositoryDispatchPlan(ordered), nil
		},
		PrepareDispatchTarget: func(ctx context.Context, options repositoryTargetDispatchOptions, target repositoryTypedTarget, state any) (repositoryTargetDispatchBinding, error) {
			native, ok := target.native.(cproject.Program)
			plan, planOK := state.(*cRepositoryDispatchPlan)
			if !ok || !planOK {
				return repositoryTargetDispatchBinding{}, fmt.Errorf("invalid C target")
			}
			parsed, err := cproject.Parse(ctx, options.Repo, options.Corpus, native, plan.store)
			if err != nil {
				plan.done(native.Ref)
				return repositoryTargetDispatchBinding{}, err
			}
			if len(parsed.Outside) > 0 && options.Output != nil {
				// The platform view: included sources an #if kept out on this
				// host (another platform's backend).
				options.Output.State("C program", "parsed", "program: "+native.Selector,
					"outside this platform's build: "+strings.Join(parsed.Outside, ", "))
			}
			return repositoryTargetDispatchBinding{
				Target: target, ProgramFacts: &cRepositoryProgramFacts{Parsed: parsed, plan: plan}, ProgramFactsBound: true,
				CPlatform: cPlatformView(parsed),
			}, nil
		},
		ValidateNative: func(target repositoryTypedTarget) error {
			native, ok := target.native.(cproject.Program)
			if !ok {
				return fmt.Errorf("invalid C target")
			}
			if target.Key.Ref != native.Ref || target.Selector != native.Selector {
				return fmt.Errorf("C target identity mismatch")
			}
			return native.Validate()
		},
		MatchProgramTarget: func(target repositoryTypedTarget, program programindex.Target) bool {
			native, ok := target.native.(cproject.Program)
			return ok && program.Selector == native.Selector && program.Name == native.Name && program.AnchorFileRef == native.AnchorFileRef
		},
		ValidatePlanAuthority: func(authority any, _ repositoryTypedTarget) error {
			if authority != nil {
				return fmt.Errorf("unexpected C plan authority")
			}
			return nil
		},
		BuildProgramInput: func(request repositoryProgramBuildRequest) (programindex.Input, error) {
			facts, err := cRepositoryFacts(request.Target, request.Facts)
			if err != nil {
				return programindex.Input{}, err
			}
			result, err := cproject.Index(request.Corpus, facts.Parsed)
			// The projection is the last reader of the program's units.
			if facts.plan != nil {
				facts.plan.done(facts.Parsed.Program.Ref)
			}
			if err != nil {
				return programindex.Input{}, err
			}
			if result.Program.Ref != facts.Parsed.Program.Ref {
				return programindex.Input{}, fmt.Errorf("C projection of another program")
			}
			facts.result = result
			return result.Input, nil
		},
		BuildDependencies: func(request repositoryDependencyBuildRequest) (dependencies.Catalog, error) {
			facts, err := cRepositoryFacts(request.Target, request.Facts)
			if err != nil {
				return dependencies.Catalog{}, err
			}
			if facts.result == nil {
				return dependencies.Catalog{}, fmt.Errorf("C dependencies requested before the program projection")
			}
			return facts.result.Dependencies, nil
		},
	}
}

// cPlatformView is the platform view a program was parsed in (owner decision
// D3): clang's version and target, the flags every unit gets after its own,
// each unit's kept and dropped build flags, and the included sources this
// platform's build leaves out. The run records it in the target's metadata
// and prints clang and the outside sources; the page shows none of it.
func cPlatformView(parsed *cproject.Parsed) *debugdump.CPlatform {
	tool := parsed.Toolchain
	view := &debugdump.CPlatform{
		Clang: tool.Version, Target: tool.Target, Sysroot: tool.Sysroot,
		Overrides:  slices.Clone(tool.Overrides),
		BuildError: parsed.Program.BuildErr,
		Outside:    slices.Clone(parsed.Outside),
	}
	for _, unit := range parsed.Units {
		view.Units = append(view.Units, debugdump.CPlatformUnit{
			Path: unit.Path, Built: unit.Built, Kept: slices.Clone(unit.Args), Dropped: slices.Clone(unit.Dropped),
		})
	}
	return view
}

// cRepositoryDispatchPlan is the C lane's state for one plan: one parse store,
// so a unit several programs link is parsed once, and the planned programs
// not yet projected. Once a program is projected (or fails to parse), the
// store releases the units no remaining program needs, instead of holding
// every decoded unit through the model work of every page.
type cRepositoryDispatchPlan struct {
	store *cproject.Store

	mu      sync.Mutex
	pending []cproject.Program
}

func newCRepositoryDispatchPlan(ordered []repositoryTypedTarget) *cRepositoryDispatchPlan {
	plan := &cRepositoryDispatchPlan{store: cproject.NewStore()}
	for _, target := range ordered {
		if native, ok := target.native.(cproject.Program); ok && target.Key.Adapter == repositoryTargetAdapterC {
			plan.pending = append(plan.pending, native)
		}
	}
	return plan
}

// done records that the program no longer reads its units.
func (plan *cRepositoryDispatchPlan) done(ref string) {
	plan.mu.Lock()
	plan.pending = slices.DeleteFunc(plan.pending, func(program cproject.Program) bool { return program.Ref == ref })
	remaining := slices.Clone(plan.pending)
	plan.mu.Unlock()
	plan.store.Keep(remaining)
}

// cRepositoryProgramFacts is one C program's native handoff: the units target
// preparation parsed, and the projection BuildProgramInput makes of them,
// whose dependency catalog BuildDependencies returns.
type cRepositoryProgramFacts struct {
	Parsed *cproject.Parsed
	result *cproject.Result
	plan   *cRepositoryDispatchPlan
}

func cRepositoryFacts(target repositoryTypedTarget, value any) (*cRepositoryProgramFacts, error) {
	facts, ok := value.(*cRepositoryProgramFacts)
	if !ok || facts == nil || facts.Parsed == nil || target.Key.Ref != facts.Parsed.Program.Ref {
		return nil, fmt.Errorf("C native fact binding mismatch")
	}
	return facts, nil
}

// cRepositoryHasSource reports a corpus with a .c file outside the tooling
// directories: only then does C discovery read the build and probe clang. A
// testdata fixture's C files are inputs of the tests around them, not a
// program of the repository.
func cRepositoryHasSource(repository *corpus.Corpus) bool {
	for _, entry := range repository.Entries() {
		if path.Ext(entry.Path) == ".c" && !corpus.ToolingPath(entry.Path) {
			return true
		}
	}
	return false
}

// cProgramRoot is the directory a program belongs to: its link output's,
// its main file's, or the library directory itself.
func cProgramRoot(program cproject.Program) string {
	if program.Kind == cproject.ProgramLibrary {
		return program.Name
	}
	return path.Dir(program.Name)
}

func discoverCRepositoryTargets(ctx context.Context, options repositoryTargetRuntimeOptions) (repositoryTargetAdapterDiscovery, bool, error) {
	// An explicit --target another adapter owns names no C program: the dry
	// run and the clang probes would be work for nothing.
	if options.Repository == nil || !cRepositoryHasSource(options.Repository) ||
		explicitTargetsOwnedElsewhere(options.TargetOverride, repositoryTargetAdapterC) {
		return repositoryTargetAdapterDiscovery{}, false, nil
	}
	if strings.TrimSpace(options.Root) == "" {
		return repositoryTargetAdapterDiscovery{}, false, fmt.Errorf("discover C targets: the repository root is unavailable")
	}
	project, err := cproject.Discover(ctx, options.Root, options.Repository)
	if err != nil {
		return repositoryTargetAdapterDiscovery{}, false, fmt.Errorf("discover C targets: %w", err)
	}
	if project == nil {
		return repositoryTargetAdapterDiscovery{}, false, nil
	}
	reportCProject(options.Output, *project)
	if len(project.Programs) == 0 {
		return repositoryTargetAdapterDiscovery{}, false, nil
	}
	programs := project.Programs

	// Every file a program compiles, and its link line's file, restores it.
	owners := map[corpus.FileID][]int{}
	linkedBy := map[string]map[int]bool{}
	own := func(ref corpus.FileID, index int) {
		for _, known := range owners[ref] {
			if known == index {
				return
			}
		}
		owners[ref] = append(owners[ref], index)
	}
	for index, program := range programs {
		own(corpus.FileID(program.AnchorFileRef), index)
		for _, unit := range program.Units {
			own(corpus.FileID(unit.FileRef), index)
			if linkedBy[unit.Path] == nil {
				linkedBy[unit.Path] = map[int]bool{}
			}
			linkedBy[unit.Path][index] = true
		}
	}
	var candidates []analysistarget.FileCandidate
	offered := map[corpus.FileID]bool{}
	offer := func(ref corpus.FileID, hypothesis string) {
		if !offered[ref] {
			offered[ref] = true
			candidates = append(candidates, analysistarget.FileCandidate{FileRef: ref, Hypotheses: []string{hypothesis}})
		}
	}
	for _, program := range programs {
		offers := 0
		for _, unit := range program.Units {
			if len(linkedBy[unit.Path]) != 1 {
				continue
			}
			hypothesis := cLinkedFileHypothesis
			switch {
			case program.Kind == cproject.ProgramLibrary:
				hypothesis = cLibraryFileHypothesis
			case program.Closure:
				hypothesis = cMainFileHypothesis
			}
			offer(corpus.FileID(unit.FileRef), hypothesis)
			offers++
		}
		if offers == 0 {
			offer(corpus.FileID(program.AnchorFileRef), cLinkFileHypothesis)
		}
	}
	resolve := func(ref corpus.FileID) []int { return owners[ref] }
	refs := make([]string, len(programs))
	for index, program := range programs {
		refs[index] = program.Ref
	}
	required, err := canonicalNativeTargetFileRefs("C", candidates, refs, func(ref corpus.FileID) ([]string, error) {
		var out []string
		for _, index := range resolve(ref) {
			out = append(out, programs[index].Ref)
		}
		return out, nil
	})
	if err != nil {
		return repositoryTargetAdapterDiscovery{}, false, err
	}
	discovery := repositoryTargetAdapterDiscovery{Key: repositoryTargetAdapterC, Candidates: candidates, RequiredFileRefs: required}
	discovery.ResolvesFile = func(ref corpus.FileID) bool { return len(owners[ref]) > 0 }
	discovery.RestoreFiles = func(fileRefs []corpus.FileID) ([]repositoryTargetFileRestoration, error) {
		if len(fileRefs) == 0 {
			return nil, nil
		}
		targets := map[repositoryTargetKey]repositoryTypedTarget{}
		files := map[repositoryTargetKey][]corpus.FileID{}
		for _, ref := range fileRefs {
			indexes := resolve(ref)
			if len(indexes) == 0 {
				return nil, fmt.Errorf("restored C file_ref %q belongs to no C program", ref)
			}
			for _, index := range indexes {
				target, err := newCRepositoryTypedTarget(programs[index])
				if err != nil {
					return nil, err
				}
				targets[target.Key] = target
				files[target.Key] = append(files[target.Key], ref)
			}
		}
		return repositoryTargetRestorations(targets, files), nil
	}
	discovery.ResolveExplicit = func(_ *corpus.Corpus, selector string) ([]repositoryTypedTarget, error) {
		for _, program := range programs {
			if program.Selector == selector || program.Ref == selector {
				target, err := newCRepositoryTypedTarget(program)
				if err != nil {
					return nil, err
				}
				return []repositoryTypedTarget{target}, nil
			}
		}
		return nil, nil
	}
	discovery.NativeEvidence = func(target repositoryTypedTarget) (repositoryNativeEvidence, error) {
		native, ok := target.native.(cproject.Program)
		if !ok {
			return repositoryNativeEvidence{}, fmt.Errorf("invalid C target")
		}
		evidence := repositoryNativeEvidence{Root: cProgramRoot(native)}
		for _, observation := range native.Evidence {
			evidence.Observations = append(evidence.Observations, targetportfolio.Observation{
				Kind: observation.Kind, Path: observation.Path, Line: observation.Line,
				Fields: observation.Fields, Values: observation.Values,
			})
		}
		return evidence, nil
	}
	discovery.ChoiceGroup = func() (targetPortfolioChoiceGroup, error) {
		choices := make([]string, len(programs))
		for index, program := range programs {
			choices[index] = program.Selector
		}
		sort.Strings(choices)
		return targetPortfolioChoiceGroup{Language: "C", Choices: strings.Join(choices, ", ")}, nil
	}
	discovery.SnapshotAuthority = func() (any, error) { return nil, nil }
	return discovery, true, nil
}

// reportCProject prints where the build description came from, the platform
// view every unit is parsed in, and what discovery could not use.
func reportCProject(output *runOutput, project cproject.Project) {
	if output == nil {
		return
	}
	details := []string{fmt.Sprintf("programs: %d; units: %d", len(project.Programs), len(project.Units))}
	switch project.Build.Kind {
	case cproject.BuildNone:
		details = append(details, "build: clang defaults for every .c file")
	default:
		details = append(details, "build: "+strings.Join(project.Build.Command, " "))
		if project.Build.Path != "" && len(project.Build.Command) == 0 {
			details[len(details)-1] = "build: " + project.Build.Path
		}
	}
	if tool := project.Toolchain; tool.Err == "" {
		details = append(details, "clang: "+tool.Version+" for "+tool.Target)
	}
	for _, observation := range project.Observations {
		line := observation.Kind
		if observation.Path != "" {
			line += " " + observation.Path
			if observation.Line > 0 {
				line += fmt.Sprintf(":%d", observation.Line)
			}
		}
		keys := make([]string, 0, len(observation.Fields))
		for key := range observation.Fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			line += "; " + key + ": " + observation.Fields[key]
		}
		details = append(details, line)
	}
	output.State("C build description", string(project.Build.Kind), details...)
}
