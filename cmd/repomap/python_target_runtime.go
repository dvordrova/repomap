package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/targetviewchoice"
)

// pythonTargetRunSelection is the main-owned bridge between the shared
// file-addressed portfolio and the Python adapter. Language-specific target
// values stay local; the next durable handoff is programindex.Index.
type pythonTargetRunSelection struct {
	Catalog pythontarget.Catalog
	Default pythontarget.Target
	Targets []pythontarget.Target
	Outcome targetPortfolioRunOutcome
}

type repositoryLanguageEvidence struct {
	Go     bool
	Python bool
}

// repositoryLanguages is a cheap routing fact over the one shared corpus. A
// lone .go file is not Go-project authority: Python repositories commonly
// carry generated helpers and examples. The Go main path still handles a pure
// Go tree without go.mod; for a multi-language dispatch we require an explicit
// Go module boundary before spending the shared portfolio on Go candidates.
// Language adapters still own exact target discovery.
func repositoryLanguages(repository *corpus.Corpus) repositoryLanguageEvidence {
	if repository == nil {
		return repositoryLanguageEvidence{}
	}
	var evidence repositoryLanguageEvidence
	for _, entry := range repository.Entries() {
		switch {
		case entry.Path == "go.mod" || strings.HasSuffix(entry.Path, "/go.mod"):
			evidence.Go = true
		case path.Ext(entry.Path) == ".py" || pythonManifestPath(entry.Path):
			evidence.Python = true
		}
	}
	return evidence
}

func pythonManifestPath(filePath string) bool {
	base := path.Base(filePath)
	if strings.HasPrefix(base, "requirements") && strings.HasSuffix(base, ".txt") {
		return true
	}
	switch base {
	case "pyproject.toml", "setup.cfg", "setup.py", "requirements.txt", "Pipfile", "tox.ini":
		return true
	default:
		return false
	}
}

// selectPythonTargetsForRun runs the same first-layer shape as the Go path:
// local language discovery and the README classifier are parallel, their
// FileCandidates are dumb-merged, and the shared portfolio sees only file
// refs. Target identities are restored locally after the model decision.
func selectPythonTargetsForRun(
	ctx context.Context,
	repoName string,
	repository *corpus.Corpus,
	override string,
	output *runOutput,
	providers targetPortfolioProviderFactory,
	executor llm.Executor,
) (pythonTargetRunSelection, error) {
	if repository == nil {
		return pythonTargetRunSelection{}, fmt.Errorf("Python target selection: repository corpus is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	parallelContext, cancelParallel := context.WithCancel(ctx)
	defer cancelParallel()

	type discoveryResult struct {
		catalog pythontarget.Catalog
		err     error
	}
	discovery := make(chan discoveryResult, 1)
	go func() {
		started := time.Now()
		catalog, err := pythontarget.Discover(parallelContext, repository)
		if err == nil && output != nil {
			reportPythonTargetDiscovery(output, catalog, time.Since(started))
		}
		discovery <- discoveryResult{catalog: catalog, err: err}
	}()

	readmeResult := make(chan struct {
		roles readmetargetscout.Result
		err   error
	}, 1)
	go func() {
		if !readmetargetscout.HasGuidanceFiles(repository) {
			readmeResult <- struct {
				roles readmetargetscout.Result
				err   error
			}{roles: readmetargetscout.Result{}}
			return
		}
		roles, err := discoverReadmeFileRoles(
			parallelContext, repoName, repository, output, providers, executor,
		)
		readmeResult <- struct {
			roles readmetargetscout.Result
			err   error
		}{roles: roles, err: err}
	}()

	discovered := <-discovery
	if discovered.err != nil {
		cancelParallel()
		<-readmeResult
		return pythonTargetRunSelection{}, fmt.Errorf("discover Python targets: %w", discovered.err)
	}
	if len(discovered.catalog.Entries) == 0 && len(discovered.catalog.ModuleScopes) == 0 {
		cancelParallel()
		<-readmeResult
		if len(discovered.catalog.Omissions) > 0 {
			return pythonTargetRunSelection{}, fmt.Errorf(
				"no eligible Python analysis targets; discovery omissions: %s",
				pythonTargetOmissionSummary(discovered.catalog.Omissions),
			)
		}
		return pythonTargetRunSelection{}, fmt.Errorf("no eligible Python analysis targets")
	}
	resolver, err := pythontarget.NewFileTargetResolver(repository, discovered.catalog)
	if err != nil {
		cancelParallel()
		<-readmeResult
		return pythonTargetRunSelection{}, fmt.Errorf("bind Python target resolver: %w", err)
	}

	override = strings.TrimSpace(override)
	if override != "" {
		readme := <-readmeResult
		if readme.err != nil {
			return pythonTargetRunSelection{}, readme.err
		}
		target, err := resolvePythonTargetOverride(discovered.catalog, resolver, override)
		if err != nil {
			return pythonTargetRunSelection{}, err
		}
		outcome := pythonSelectionOutcome(target, []pythontarget.Target{target})
		outcome.ReadmeRoles = compileReadmeRoleLog(repository, readme.roles)
		if output != nil {
			output.State(
				"Target hypothesis merge", "not needed",
				"reason: explicit --target bypasses candidate merging",
			)
			output.State(
				"Analysis target", "selected",
				"source: explicit --target",
				"selected: "+target.DisplayName,
			)
			output.State(
				"Target view", "not needed",
				"reason: explicit --target resolves one exact view",
			)
		}
		return pythonTargetRunSelection{
			Catalog: discovered.catalog.Snapshot(), Default: target,
			Targets: []pythontarget.Target{target}, Outcome: outcome,
		}, nil
	}

	readme := <-readmeResult
	if readme.err != nil {
		return pythonTargetRunSelection{}, readme.err
	}
	projectionStarted := time.Now()
	nativeCandidates, err := pythontarget.FileCandidates(repository, discovered.catalog)
	if err != nil {
		return pythonTargetRunSelection{}, err
	}
	readmeCandidates, unsupported := pythonResolvableReadmeTargetCandidates(
		readme.roles.TargetCandidates(), resolver,
	)
	if output != nil {
		output.State(
			"Python target projection", "ready",
			fmt.Sprintf("native file hypotheses: %d", len(nativeCandidates)),
			fmt.Sprintf("resolvable guidance hypotheses: %d", len(readmeCandidates)),
			formatRunOutputWallDuration(time.Since(projectionStarted)),
		)
	}
	if output != nil && unsupported > 0 {
		output.Stage(
			"Repository guidance classifier",
			fmt.Sprintf(
				"kept %d target hypotheses for the Python adapter; retained %d unsupported target roles only in diagnostics",
				len(readmeCandidates), unsupported,
			),
		)
	}
	mergeStarted := time.Now()
	merged, err := analysistarget.MergeFileCandidates(
		repository.Snapshot(), nativeCandidates, readmeCandidates,
	)
	if err != nil {
		return pythonTargetRunSelection{}, fmt.Errorf("merge Python target hypotheses: %w", err)
	}
	if output != nil {
		output.State(
			"Target hypothesis merge", "complete",
			fmt.Sprintf("native hypotheses: %d", len(nativeCandidates)),
			fmt.Sprintf("guidance hypotheses: %d", len(readmeCandidates)),
			fmt.Sprintf("merged hypotheses: %d", len(merged)),
			formatRunOutputWallDuration(time.Since(mergeStarted)),
		)
	}
	if len(merged) == 0 {
		return pythonTargetRunSelection{}, fmt.Errorf("Python target discovery returned no file hypotheses")
	}

	portfolio, outcome, err := selectTargetPortfolioForRun(
		ctx, repository.Snapshot(), merged, output, providers, executor,
	)
	outcome.ReadmeRoles = compileReadmeRoleLog(repository, readme.roles)
	if err != nil {
		return pythonTargetRunSelection{}, withTargetPortfolioChoices(
			err,
			targetPortfolioChoiceGroup{Language: "Python", Choices: pythonTargetChoices(discovered.catalog)},
		)
	}

	selectedFileRefs := make([]corpus.FileID, 0, len(portfolio.Targets))
	for _, candidate := range portfolio.Targets {
		selectedFileRefs = append(selectedFileRefs, candidate.FileRef)
	}
	selectedTargets, err := resolver.Resolve(selectedFileRefs)
	if err != nil {
		return pythonTargetRunSelection{}, fmt.Errorf(
			"restore selected Python targets from file portfolio: %w; choose one exact Python target with --target TARGET; choices: %s",
			err, pythonTargetChoices(discovered.catalog),
		)
	}
	selectedByRef := make(map[string]pythontarget.Target, len(selectedTargets))
	for _, target := range selectedTargets {
		selectedByRef[target.Ref] = target
	}
	if portfolio.Default == nil {
		return pythonTargetRunSelection{}, fmt.Errorf("Python target portfolio accepted targets without a default")
	}
	defaultTargets, err := resolver.Resolve([]corpus.FileID{portfolio.Default.FileRef})
	if err != nil {
		return pythonTargetRunSelection{}, fmt.Errorf(
			"restore default Python target views from file portfolio: %w; choose one exact Python target with --target TARGET; choices: %s",
			err, pythonTargetChoices(discovered.catalog),
		)
	}
	defaultTarget, err := choosePythonDefaultTargetView(
		ctx, repository, defaultTargets, portfolio.Default.Hypotheses, output, providers, executor,
	)
	if err != nil {
		return pythonTargetRunSelection{}, fmt.Errorf(
			"choose default Python target view: %w; choose one exact Python target with --target TARGET; choices: %s",
			err, pythonTargetChoices(discovered.catalog),
		)
	}
	selectedByRef[defaultTarget.Ref] = defaultTarget

	targets := make([]pythontarget.Target, 0, len(selectedByRef))
	for _, selected := range selectedByRef {
		if !discovered.catalog.OwnsTarget(selected) {
			return pythonTargetRunSelection{}, fmt.Errorf("restored Python target is outside the sealed discovery catalog")
		}
		targets = append(targets, selected)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].Selector < targets[j].Selector })
	outcome.SelectedRef = defaultTarget.Ref
	outcome.SelectedTargets = len(targets)
	outcome.SelectedTargetRefs = pythonTargetRefs(targets)
	outcome.SelectedFileRefs = len(portfolio.Targets)
	outcome.UnclassifiedFiles = len(portfolio.Unclassified)
	return pythonTargetRunSelection{
		Catalog: discovered.catalog.Snapshot(), Default: defaultTarget,
		Targets: targets, Outcome: outcome,
	}, nil
}

// discoverPythonTargetCatalogForRun is the mixed-repository adapter entry. It
// performs the same exact local discovery as the Python-only selection path,
// without spending a model call to choose a default that the Go report cannot
// use. Discovery errors are fatal; typed omissions remain in the sealed
// catalog and console diagnostics but do not erase independently proven exact
// targets or become guessed targets.
func discoverPythonTargetCatalogForRun(
	ctx context.Context,
	repository *corpus.Corpus,
	output *runOutput,
) (pythontarget.Catalog, error) {
	started := time.Now()
	catalog, err := pythontarget.Discover(ctx, repository)
	if err != nil {
		return pythontarget.Catalog{}, fmt.Errorf("discover Python targets: %w", err)
	}
	if output != nil {
		reportPythonTargetDiscovery(output, catalog, time.Since(started))
	}
	return catalog, nil
}

func reportPythonTargetDiscovery(
	output *runOutput,
	catalog pythontarget.Catalog,
	elapsed time.Duration,
) {
	if output == nil {
		return
	}
	state := "complete"
	details := []string{
		fmt.Sprintf("exact native targets: %d", len(catalog.Entries)),
		fmt.Sprintf("sealed module scopes: %d", len(catalog.ModuleScopes)),
		formatRunOutputWallDuration(elapsed),
	}
	if catalog.Coverage == pythontarget.CoveragePartial {
		state = "partial"
		details = append(
			details,
			fmt.Sprintf("typed omissions: %d", len(catalog.Omissions)),
			"first omission: "+pythonTargetOmissionSummary(catalog.Omissions),
			"continuing only with independently sealed exact targets; omissions remain diagnostics",
		)
	}
	output.State("Python target discovery", state, details...)
}

func pythonResolvableReadmeTargetCandidates(
	candidates []analysistarget.FileCandidate,
	resolver pythontarget.FileTargetResolver,
) ([]analysistarget.FileCandidate, int) {
	result := make([]analysistarget.FileCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if resolver.Resolves(candidate.FileRef) {
			result = append(result, candidate)
		}
	}
	if result == nil {
		result = []analysistarget.FileCandidate{}
	}
	return result, len(candidates) - len(result)
}

func choosePythonDefaultTargetView(
	ctx context.Context,
	repository *corpus.Corpus,
	targets []pythontarget.Target,
	selectedFileHypotheses []string,
	output *runOutput,
	providers targetPortfolioProviderFactory,
	executor llm.Executor,
) (pythontarget.Target, error) {
	if len(targets) == 0 {
		return pythontarget.Target{}, fmt.Errorf("selected file has no exact target views")
	}
	if len(targets) == 1 {
		if output != nil {
			output.State(
				"Target view", "not needed",
				"reason: selected file has one exact view",
				"selected: "+targets[0].DisplayName,
			)
		}
		return targets[0], nil
	}
	started := time.Now()
	if repository == nil {
		return pythontarget.Target{}, fmt.Errorf("repository corpus is unavailable")
	}
	views := make([]targetviewchoice.View, len(targets))
	targetsBySelector := make(map[string]pythontarget.Target, len(targets))
	for position, target := range targets {
		view, err := pythonTargetChoiceView(repository, target)
		if err != nil {
			return pythontarget.Target{}, err
		}
		views[position] = view
		key := view.Language + "\x00" + view.Selector
		if _, duplicate := targetsBySelector[key]; duplicate {
			return pythontarget.Target{}, fmt.Errorf("duplicate Python target selector %q", view.Selector)
		}
		targetsBySelector[key] = target
	}
	cube, err := targetviewchoice.Compile(views, selectedFileHypotheses)
	if err != nil {
		return pythontarget.Target{}, err
	}
	if providers == nil {
		return pythontarget.Target{}, fmt.Errorf("model provider is unavailable")
	}
	provider, err := providers()
	if err != nil {
		return pythontarget.Target{}, fmt.Errorf("configure model provider: %w", err)
	}
	if provider == nil {
		return pythontarget.Target{}, fmt.Errorf("configured model provider is unavailable")
	}
	if output != nil {
		output.Stage(
			"Target view",
			fmt.Sprintf("asking the model to choose one default among %d exact views for the selected file", len(targets)),
		)
	}
	selection, err := targetviewchoice.Run(
		ctx,
		debugdump.BindStage(executor, debugdump.SemanticStageTargetViewChoice),
		provider,
		cube,
	)
	if err != nil {
		return pythontarget.Target{}, err
	}
	key := selection.DefaultView.Language + "\x00" + selection.DefaultView.Selector
	selected, ok := targetsBySelector[key]
	if !ok {
		return pythontarget.Target{}, fmt.Errorf("model-selected view is outside local target authority")
	}
	if output != nil {
		output.State(
			"Target view", "selected",
			"selected: "+selected.DisplayName,
			"selector: "+selected.Selector,
			formatRunOutputWallDuration(time.Since(started)),
		)
	}
	return selected, nil
}

func pythonTargetChoiceView(repository *corpus.Corpus, target pythontarget.Target) (targetviewchoice.View, error) {
	if err := target.Validate(); err != nil {
		return targetviewchoice.View{}, fmt.Errorf("validate Python target view: %w", err)
	}
	anchor, ok := repository.Info(target.AnchorFileRef)
	if !ok {
		return targetviewchoice.View{}, fmt.Errorf("Python target anchor %q is outside repository corpus", target.AnchorFileRef)
	}
	roots := make([]string, len(target.Roots))
	for position, root := range target.Roots {
		rootName := root.Module
		if root.Qualname != "" {
			rootName += ":" + root.Qualname
		}
		roots[position] = fmt.Sprintf("%s | %s | %s:%d", root.Kind, rootName, root.Path, root.Line)
	}
	basis := make([]string, len(target.Basis))
	for position, evidence := range target.Basis {
		location := evidence.Path
		if evidence.Line > 0 {
			location += fmt.Sprintf(":%d", evidence.Line)
		}
		basis[position] = fmt.Sprintf("%s | %s", evidence.Kind, location)
		if evidence.Label != "" {
			basis[position] += " | " + evidence.Label
		}
	}
	return targetviewchoice.View{
		Language: "python", Kind: string(target.Kind), DisplayName: target.DisplayName,
		Selector: target.Selector, AnchorPath: anchor.Entry.Path,
		RootSummaries: roots, BasisSummaries: basis,
	}, nil
}

func resolvePythonTargetOverride(
	catalog pythontarget.Catalog,
	resolver pythontarget.FileTargetResolver,
	override string,
) (pythontarget.Target, error) {
	matches := make(map[string]pythontarget.Target)
	for _, target := range catalog.Entries {
		if pythonTargetMatchesOverride(target, override) {
			matches[target.Ref] = target
		}
	}
	if len(matches) == 1 {
		for _, target := range matches {
			return target, nil
		}
	}
	if len(matches) == 0 {
		if derived, ok, err := resolver.ResolveSelector(override); err != nil {
			return pythontarget.Target{}, err
		} else if ok {
			return derived, nil
		}
		choices, choicesErr := pythonExactTargetChoices(catalog, resolver)
		if choicesErr != nil {
			return pythontarget.Target{}, choicesErr
		}
		return pythontarget.Target{}, fmt.Errorf(
			"--target %q is not an eligible exact Python target; use one exact selector: %s",
			override, choices,
		)
	}
	refs := make([]string, 0, len(matches))
	for _, target := range matches {
		refs = append(refs, target.Selector)
	}
	sort.Strings(refs)
	return pythontarget.Target{}, fmt.Errorf(
		"--target %q is ambiguous; use one exact Python selector: %s",
		override, strings.Join(refs, ", "),
	)
}

func pythonExactTargetChoices(
	catalog pythontarget.Catalog,
	resolver pythontarget.FileTargetResolver,
) (string, error) {
	const moduleLimit = 12
	parts := make([]string, 0, 2)
	if native := pythonTargetChoices(catalog); native != "" {
		parts = append(parts, "native "+native)
	}
	modules, total, err := resolver.ModuleExecutionChoices(moduleLimit)
	if err != nil {
		return "", err
	}
	if len(modules) > 0 {
		choices := make([]string, 0, len(modules)+1)
		for _, choice := range modules {
			choices = append(choices, fmt.Sprintf("%s (%s)", choice.Selector, choice.Path))
		}
		if total > len(modules) {
			choices = append(choices, fmt.Sprintf("... and %d more module selectors", total-len(modules)))
		}
		parts = append(parts, "module "+strings.Join(choices, ", "))
	}
	if len(parts) == 0 {
		return "no exact Python selectors were discovered", nil
	}
	return strings.Join(parts, "; "), nil
}

// resolveMixedPythonTargetOverride accepts only an adapter-owned exact key.
// Human aliases and paths can also name Go targets, and the legacy Go catalog
// is not available until orient loads it. Rejecting those aliases is the
// fail-closed union rule: one language must never claim an ambiguous --target
// before the other language has advertised its choices.
func resolveMixedPythonTargetOverride(
	catalog pythontarget.Catalog,
	repository *corpus.Corpus,
	override string,
) (*pythontarget.Target, error) {
	override = strings.TrimSpace(override)
	if override == "" {
		return nil, nil
	}
	exact := make(map[string]pythontarget.Target)
	broad := make(map[string]pythontarget.Target)
	for _, target := range catalog.Entries {
		if override == target.Ref || override == target.IdentityRef || override == target.Selector {
			exact[target.Ref] = target
		}
		if pythonTargetMatchesOverride(target, override) {
			broad[target.Ref] = target
		}
	}
	if len(exact) == 1 {
		for _, target := range exact {
			owned := target
			return &owned, nil
		}
	}
	if len(exact) > 1 {
		return nil, fmt.Errorf("--target %q matches more than one exact Python key", override)
	}
	resolver, err := pythontarget.NewFileTargetResolver(repository, catalog)
	if err != nil {
		return nil, fmt.Errorf("bind mixed Python target resolver: %w", err)
	}
	if derived, ok, err := resolver.ResolveSelector(override); err != nil {
		return nil, err
	} else if ok {
		return &derived, nil
	}
	if len(broad) == 0 {
		return nil, nil
	}
	selectors := make([]string, 0, len(broad))
	for _, target := range broad {
		selectors = append(selectors, target.Selector)
	}
	sort.Strings(selectors)
	return nil, fmt.Errorf(
		"--target %q is a non-exact Python alias in a mixed Go/Python repository; use one exact Python selector (%s) or an exact advertised Go target",
		override, strings.Join(selectors, ", "),
	)
}

func pythonTargetMatchesOverride(target pythontarget.Target, override string) bool {
	return override == target.Ref || override == target.IdentityRef || override == target.Selector ||
		override == target.DisplayName || override == target.ProjectDir ||
		pythonTargetHasPath(target, override)
}

func pythonTargetHasPath(target pythontarget.Target, wanted string) bool {
	for _, root := range target.Roots {
		if root.Path == wanted {
			return true
		}
	}
	return false
}

func pythonTargetChoices(catalog pythontarget.Catalog) string {
	const limit = 12
	choices := make([]string, 0, min(len(catalog.Entries), limit)+1)
	for index, target := range catalog.Entries {
		if index == limit {
			choices = append(choices, fmt.Sprintf("... and %d more", len(catalog.Entries)-limit))
			break
		}
		choices = append(choices, fmt.Sprintf("%s (%s; %s)", target.DisplayName, target.Kind, target.Selector))
	}
	return strings.Join(choices, ", ")
}

func pythonTargetOmissionSummary(omissions []pythontarget.Omission) string {
	if len(omissions) == 0 {
		return "coverage is partial without a classified omission"
	}
	first := omissions[0]
	location := first.Path
	if first.Line > 0 {
		location += fmt.Sprintf(":%d", first.Line)
	}
	summary := fmt.Sprintf("%s at %s", first.Kind, location)
	if first.Label != "" {
		summary += " (" + first.Label + ")"
	}
	if len(omissions) > 1 {
		summary += fmt.Sprintf(" and %d more", len(omissions)-1)
	}
	return summary
}

func pythonSelectionOutcome(
	defaultTarget pythontarget.Target,
	targets []pythontarget.Target,
) targetPortfolioRunOutcome {
	return targetPortfolioRunOutcome{
		SelectedRef:     defaultTarget.Ref,
		SelectedTargets: len(targets), SelectedTargetRefs: pythonTargetRefs(targets),
		SelectedFileRefs: 1,
	}
}

func pythonTargetRefs(targets []pythontarget.Target) []string {
	refs := make([]string, len(targets))
	for index, target := range targets {
		refs[index] = target.Ref
	}
	return refs
}

// pythonProgramRepresentatives returns every selected target view with the
// default first. BuildMany itself shares one AST parse across identical module
// inventories, so retaining all views does not repeat source analysis.
func pythonProgramRepresentatives(selection pythonTargetRunSelection) ([]pythontarget.Target, error) {
	ordered := make([]pythontarget.Target, 0, len(selection.Targets)+1)
	ordered = append(ordered, selection.Default)
	for _, target := range selection.Targets {
		if target.Ref != selection.Default.Ref {
			ordered = append(ordered, target)
		}
	}
	seen := make(map[string]struct{}, len(ordered))
	representatives := make([]pythontarget.Target, 0, len(ordered))
	for _, target := range ordered {
		if err := target.Validate(); err != nil {
			return nil, fmt.Errorf("validate selected Python target %q: %w", target.Ref, err)
		}
		if _, duplicate := seen[target.Ref]; duplicate {
			continue
		}
		seen[target.Ref] = struct{}{}
		representatives = append(representatives, target)
	}
	if len(representatives) == 0 {
		return nil, fmt.Errorf("selected Python target portfolio has no program scopes")
	}
	return representatives, nil
}

func pythonProgramScopeCount(targets []pythontarget.Target) (int, error) {
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		wire, err := json.Marshal(target.Modules)
		if err != nil {
			return 0, fmt.Errorf("group Python program scope %q: %w", target.Ref, err)
		}
		seen[string(wire)] = struct{}{}
	}
	return len(seen), nil
}
