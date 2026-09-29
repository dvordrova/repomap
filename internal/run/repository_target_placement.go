package run

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

type repositoryPlacement struct {
	Target   string `json:"target"`
	Selector string `json:"selector"`
	targetportfolio.Placement
}

func applyRepositoryPlacements(plan repositoryTargetPlan, native []repositoryNativeCandidate, placements []targetportfolio.Placement) (repositoryTargetPlan, error) {
	if len(native) != len(placements) {
		return repositoryTargetPlan{}, fmt.Errorf("incomplete native placement result")
	}
	keys := make(map[string]repositoryTargetKey)
	refs := make(map[repositoryTargetKey]string)
	positions := make(map[repositoryTargetKey]int)
	for _, candidate := range native {
		keys[candidate.Row.Ref] = candidate.Target.Key
		refs[candidate.Target.Key] = candidate.Row.Ref
	}
	for i, target := range plan.Targets {
		positions[target.Key] = i
	}
	for _, placement := range placements {
		key, known := keys[placement.Candidate.Ref]
		if !known {
			return repositoryTargetPlan{}, fmt.Errorf("placement outside native authority")
		}
		position, found := positions[key]
		if !found {
			return repositoryTargetPlan{}, fmt.Errorf("native target missing before placement")
		}
		plan.Targets[position].Placement = placement.Decision
		plan.Outcome.Placements = append(plan.Outcome.Placements, repositoryPlacement{Target: key.String(), Selector: plan.Targets[position].Selector, Placement: placement})
	}
	for i := range plan.Targets {
		target := &plan.Targets[i]
		if ownerRef, seed := strings.CutPrefix(target.Placement, "seed_of:"); seed {
			ownerKey, ok := keys[ownerRef]
			if !ok {
				return repositoryTargetPlan{}, fmt.Errorf("seed owner missing")
			}
			ownerPosition, ok := positions[ownerKey]
			if !ok || plan.Targets[ownerPosition].Placement != "standalone" {
				return repositoryTargetPlan{}, fmt.Errorf("seed owner is not standalone")
			}
			plan.Targets[ownerPosition].Seeds = append(plan.Targets[ownerPosition].Seeds, *target)
			if plan.Default == target.Key {
				plan.Default = ownerKey
			}
		}
	}
	candidates := make(map[repositoryTargetKey]repositoryNativeCandidate, len(native))
	for _, candidate := range native {
		candidates[candidate.Target.Key] = candidate
	}
	foldPythonFacets(&plan, refs, candidates)
	for _, candidate := range native {
		position := positions[candidate.Target.Key]
		if plan.Targets[position].Placement != "shared_code" {
			continue
		}
		// A Go module library whose only consumer is one standalone
		// executable is that executable's own code: cmd/app beside internal/
		// and pkg/ packages is one program, not a program and a library. The
		// model's shared_code decision stays in the journal; the page is the
		// code's to compose. Two or more consumers keep the shared library.
		if owner, folds := soleStandaloneConsumer(plan, positions, candidate); folds {
			if library, isGo := repositoryGoTarget(candidate.Target); isGo && library.Kind == analysistarget.KindModuleLibrary {
				ownerRef := refs[plan.Targets[owner].Key]
				plan.Targets[owner].Absorbed = append(plan.Targets[owner].Absorbed, candidate.Target.Key)
				plan.Targets[owner].AbsorbedRoot = library.ModuleDir
				plan.Targets[position].Placement = "folded_into:" + ownerRef
				if plan.Default == candidate.Target.Key {
					plan.Default = plan.Targets[owner].Key
				}
				plan.Outcome.Placements = append(plan.Outcome.Placements, repositoryPlacement{
					Target: candidate.Target.Key.String(), Selector: plan.Targets[position].Selector,
					Placement: targetportfolio.Placement{Candidate: candidate.Row, Decision: "folded_into:" + ownerRef,
						Reason: "a module library whose only consumer is one standalone executable is that executable's own code"},
				})
				continue
			}
		}
		for _, consumer := range candidate.Consumers {
			if position, ok := positions[consumer]; ok {
				plan.Targets[position].SharedCode = append(plan.Targets[position].SharedCode, candidate.Target.Key)
			}
		}
	}
	retained := make([]repositoryTypedTarget, 0, len(plan.Targets))
	for _, target := range plan.Targets {
		if !strings.HasPrefix(target.Placement, "seed_of:") && !strings.HasPrefix(target.Placement, "folded_into:") {
			retained = append(retained, target)
		}
	}
	plan.Targets = retained
	for i := range plan.Targets {
		if plan.Targets[i].Placement == "" {
			plan.Targets[i].Placement = "standalone"
		}
	}
	// A retained default remains a UI choice. If it is auxiliary, prefer the
	// first accepted standalone target in the same canonical order.
	if selected, ok := plan.DefaultTarget(); ok && selected.Placement != "standalone" {
		for _, target := range plan.Targets {
			if target.Placement == "standalone" {
				plan.Default = target.Key
				break
			}
		}
	}
	plan.Outcome.SelectedRef = defaultRef(plan.Default)
	plan.Outcome.SelectedTargets = len(plan.Targets)
	plan.Outcome.SelectedTargetRefs = repositoryTargetRefs(plan.Targets)
	return plan, plan.Validate()
}

// soleStandaloneConsumer returns the position of the one standalone target
// that consumes this library, when there is exactly one.
func soleStandaloneConsumer(plan repositoryTargetPlan, positions map[repositoryTargetKey]int, candidate repositoryNativeCandidate) (int, bool) {
	if len(candidate.Consumers) != 1 {
		return 0, false
	}
	position, ok := positions[candidate.Consumers[0]]
	if !ok || plan.Targets[position].Placement != "standalone" {
		return 0, false
	}
	return position, true
}

func persistRepositoryPlacements(runDir string, placements []repositoryPlacement) error {
	if len(placements) == 0 {
		return nil
	}
	raw, err := json.MarshalIndent(struct {
		Version   int                   `json:"version"`
		Decisions []repositoryPlacement `json:"decisions"`
	}{1, placements}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runDir, "target-placements.json"), append(raw, '\n'), 0600)
}

// foldPythonFacets merges, after the model's placements, the Python targets
// that are one component under two names. Each fold is journaled beside the
// model's own decision; the folded target's code stays in its owner's page,
// whose ProgramIndex already holds the whole project (the Python index is
// project-wide), so no fact is dropped.
//
//   - A script-file launch (an author shebang with no __main__ guard and no
//     launch call) of the file that defines a standalone program's launch
//     callable is that program's launch form: its seed, keeping its anchor
//     (freqtrade_client/ft_client.py beside freqtrade-client =
//     freqtrade_client.ft_client:main). Exactly one such program owns it.
//   - A library whose declared packages are all under the root of one
//     standalone program built from them (its launch file is a declared
//     module) is that program's library facet: the program also claims the
//     library's root and names the facet (Libraries, its declared
//     top-level packages). ft_client's library held only its tests beside
//     freqtrade-client. A library with code of its own outside the program's
//     root, one seeding a guard, or one two programs qualify for stays. The
//     program launches as the distribution's command: a console or GUI
//     script or a `python -m` package launch, itself or as its seed. A
//     declared guard alone was offered to the model as the library's seed,
//     and a shebang alone runs a file, not the distribution.
func foldPythonFacets(plan *repositoryTargetPlan, refs map[repositoryTargetKey]string, candidates map[repositoryTargetKey]repositoryNativeCandidate) {
	journal := func(folded, owner int, decision, reason string) {
		target := plan.Targets[folded]
		plan.Outcome.Placements = append(plan.Outcome.Placements, repositoryPlacement{
			Target: target.Key.String(), Selector: target.Selector,
			Placement: targetportfolio.Placement{Candidate: candidates[target.Key].Row, Decision: decision, Reason: reason},
		})
		if plan.Default == target.Key {
			plan.Default = plan.Targets[owner].Key
		}
	}
	retained := func(target repositoryTypedTarget) bool {
		return !strings.HasPrefix(target.Placement, "seed_of:") && !strings.HasPrefix(target.Placement, "folded_into:")
	}
	for i := range plan.Targets {
		script, ok := repositoryPythonTarget(plan.Targets[i])
		if !ok || !retained(plan.Targets[i]) || len(plan.Targets[i].Seeds) != 0 {
			continue
		}
		owner := -1
		for j := range plan.Targets {
			program, ok := repositoryPythonTarget(plan.Targets[j])
			if j == i || !ok || plan.Targets[j].Placement != "standalone" || !pythontarget.ScriptFileOf(program, script) {
				continue
			}
			if owner >= 0 {
				owner = -2
				break
			}
			owner = j
		}
		if owner < 0 {
			continue
		}
		decision := "seed_of:" + refs[plan.Targets[owner].Key]
		plan.Targets[i].Placement = decision
		plan.Targets[owner].Seeds = append(plan.Targets[owner].Seeds, plan.Targets[i])
		journal(i, owner, decision, "a script-file launch of the file defining the program's launch callable is that program's launch form")
	}
	for i := range plan.Targets {
		library, ok := repositoryPythonTarget(plan.Targets[i])
		if !ok || library.Kind != pythontarget.KindLibrary || len(plan.Targets[i].Seeds) != 0 ||
			plan.Targets[i].Placement != "standalone" && plan.Targets[i].Placement != "shared_code" {
			continue
		}
		var declared []pythontarget.Module
		for _, module := range library.Modules {
			if library.DeclaresModule(module) {
				declared = append(declared, module)
			}
		}
		if len(declared) == 0 {
			continue
		}
		owner := -1
		for j := range plan.Targets {
			program, ok := repositoryPythonTarget(plan.Targets[j])
			if j == i || !ok || plan.Targets[j].Placement != "standalone" || plan.Targets[j].AbsorbedRoot != "" ||
				program.Kind != pythontarget.KindExecutable || program.ProjectDir != library.ProjectDir || len(program.Roots) == 0 ||
				!distributionLaunch(plan.Targets[j]) {
				continue
			}
			launch, root := program.Roots[0].Path, path.Dir(program.Roots[0].Path)
			built, within := false, true
			for _, module := range declared {
				built = built || module.Path == launch
				within = within && (root == "." || strings.HasPrefix(module.Path, root+"/"))
			}
			if !built || !within {
				continue
			}
			if owner >= 0 {
				owner = -2
				break
			}
			owner = j
		}
		if owner < 0 {
			continue
		}
		absorbed := library.ProjectDir
		for _, basis := range library.Basis {
			if basis.Kind == pythontarget.BasisImportPackage && basis.Path != "" {
				absorbed = path.Dir(basis.Path)
				break
			}
		}
		var names []string
		for _, module := range declared {
			top, _, _ := strings.Cut(module.Name, ".")
			names = append(names, top)
		}
		slices.Sort(names)
		program := &plan.Targets[owner]
		program.Absorbed = append(program.Absorbed, plan.Targets[i].Key)
		program.AbsorbedRoot = absorbed
		program.Libraries = append(program.Libraries, names...)
		slices.Sort(program.Libraries)
		program.Libraries = slices.Compact(program.Libraries)
		decision := "folded_into:" + refs[program.Key]
		plan.Targets[i].Placement = decision
		journal(i, owner, decision, "a library whose declared packages are all the code of the one program built from them is that program's library facet")
	}
}

// distributionLaunch says a Python program launches as its distribution's
// command, itself or through a seed: a console or GUI script (a callable
// root) or a `python -m` package launch.
func distributionLaunch(target repositoryTypedTarget) bool {
	for _, form := range append([]repositoryTypedTarget{target}, target.Seeds...) {
		if value, ok := repositoryPythonTarget(form); ok && len(value.Roots) == 1 &&
			(value.Roots[0].Kind == pythontarget.RootCallable || value.Roots[0].Kind == pythontarget.RootModule) {
			return true
		}
	}
	return false
}
