package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

// repositoryTargetPlan is the exact, typed output of one repository-wide
// target decision. GoSource is the one unscoped snapshot from which a caller
// may derive a target-local snapshot with snapshot.ScopeAnalysisTarget.
// PythonCatalog retains ownership of native and resolver-derived Python views.
type repositoryTargetPlan struct {
	Targets     []repositoryTypedTarget
	Default     repositoryTargetKey
	Explicit    bool
	Authorities map[repositoryTargetAdapter]any
	Outcome     targetPortfolioRunOutcome
	guidance    readmetargetscout.GuidanceSnapshot
}

func (plan repositoryTargetPlan) DefaultTarget() (repositoryTypedTarget, bool) {
	for _, target := range plan.Targets {
		if target.Key == plan.Default {
			return target, true
		}
	}
	return repositoryTypedTarget{}, false
}

func repositoryPlanGoSource(plan repositoryTargetPlan) (*snapshot.Snapshot, bool) {
	value, ok := plan.Authorities[repositoryTargetAdapterGo].(snapshot.Snapshot)
	if !ok {
		return nil, false
	}
	owned, err := snapshot.OwnSnapshot(value)
	if err != nil {
		return nil, false
	}
	return &owned, true
}

func repositoryPlanPythonCatalog(plan repositoryTargetPlan) (*pythontarget.Catalog, bool) {
	value, ok := plan.Authorities[repositoryTargetAdapterPython].(pythontarget.Catalog)
	if !ok {
		return nil, false
	}
	owned := value.Snapshot()
	return &owned, true
}

func (plan repositoryTargetPlan) Validate() error {
	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		return fmt.Errorf("repository target plan: adapter registry: %w", err)
	}
	return plan.validateWith(registry)
}

func (plan repositoryTargetPlan) validateWith(registry repositoryTargetAdapterRegistry) error {
	if len(plan.Targets) == 0 {
		return fmt.Errorf("repository target plan: target set is empty")
	}
	if err := plan.guidance.Validate(); err != nil {
		return fmt.Errorf("repository target plan: repository guidance: %w", err)
	}
	for key := range plan.Authorities {
		if _, known := registry.descriptor(key); !known {
			return fmt.Errorf("repository target plan: authority belongs to unregistered adapter %q", key)
		}
	}

	seen := make(map[repositoryTargetKey]struct{}, len(plan.Targets))
	defaultCount := 0
	for index, target := range plan.Targets {
		if err := target.validateWith(registry); err != nil {
			return fmt.Errorf("repository target plan: target %d: %w", index, err)
		}
		if index > 0 && !repositoryTypedTargetLess(plan.Targets[index-1], target) {
			return fmt.Errorf("repository target plan: targets are not canonical")
		}
		if _, duplicate := seen[target.Key]; duplicate {
			return fmt.Errorf("repository target plan: duplicate target %q", target.Key)
		}
		seen[target.Key] = struct{}{}
		if target.Key == plan.Default {
			defaultCount++
		}

		descriptor, known := registry.descriptor(target.Key.Adapter)
		if !known {
			return fmt.Errorf("repository target plan: target %q uses an unregistered adapter", target.Key)
		}
		if descriptor.ValidatePlanAuthority != nil {
			if err := descriptor.ValidatePlanAuthority(plan.Authorities[target.Key.Adapter], target); err != nil {
				return fmt.Errorf("repository target plan: target %q: %w", target.Key, err)
			}
		}
	}
	if defaultCount != 1 {
		return fmt.Errorf("repository target plan: expected one typed default, got %d", defaultCount)
	}
	if plan.Outcome.SelectedRef != plan.Default.String() || plan.Outcome.SelectedTargets != len(plan.Targets) {
		return fmt.Errorf("repository target plan: outcome identity does not match typed targets")
	}
	wantRefs := repositoryTargetRefs(plan.Targets)
	if !sameRepositoryStrings(plan.Outcome.SelectedTargetRefs, wantRefs) {
		return fmt.Errorf("repository target plan: outcome target refs do not match typed targets")
	}
	return nil
}

// repositoryTargetRuntimeOptions keeps discovery policy explicit. GoSnapshot
// is optional but, when supplied, must already contain the complete unscoped
// Go facts/catalog pair. Python and JSTS discovery are independently optional.
type repositoryTargetRuntimeOptions struct {
	RepoName   string
	Repository *corpus.Corpus

	GoSnapshot     *snapshot.Snapshot
	DiscoverPython bool
	DiscoverJSTS   bool
	TargetOverride string

	Output      *runOutput
	Providers   targetPortfolioProviderFactory
	Executor    llm.Executor
	ScoutJSTSFn jsTSTargetScout
}

// prepareRepositoryPlanningGoSource keeps initial language prerequisites
// scoped to the adapter that can own an explicit namespaced selector. JSTS and
// Python selectors are closed, disjoint namespaces, so neither needs Go facts
// in order to prove its exact identity. Adapter-neutral aliases and ordinary
// discovery still prepare Go whenever the corpus contains Go evidence.
func prepareRepositoryPlanningGoSource(
	hasGoEvidence bool,
	targetOverride string,
	prepare func() (snapshot.Snapshot, error),
) (*snapshot.Snapshot, error) {
	if !hasGoEvidence || explicitNonGoRepositoryTargetSelector(targetOverride) {
		return nil, nil
	}
	if prepare == nil {
		return nil, fmt.Errorf("prepare repository planning Go source: builder is unavailable")
	}
	prepared, err := prepare()
	if err != nil {
		return nil, err
	}
	return &prepared, nil
}

type repositoryTargetDiscovery struct {
	adapters []repositoryTargetAdapterDiscovery
	byKey    map[repositoryTargetAdapter]repositoryTargetAdapterDiscovery
	readme   readmetargetscout.Result
	guidance readmetargetscout.GuidanceSnapshot
}

// selectRepositoryTargetPlanForRun runs all enabled deterministic adapters
// beside exactly one README scout and then spends one repository-wide
// TargetPortfolio request. The model sees only file refs. Every accepted ref is
// restored through one exact local adapter before the typed plan is returned.
// An exact TargetOverride still runs enabled local discovery plus the one
// README scout, but bypasses TargetPortfolio.
func selectRepositoryTargetPlanForRun(
	ctx context.Context,
	options repositoryTargetRuntimeOptions,
) (repositoryTargetPlan, error) {
	if options.Repository == nil {
		return repositoryTargetPlan{}, fmt.Errorf("repository target selection: repository corpus is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return repositoryTargetPlan{}, err
	}
	discovery, err := discoverRepositoryTargets(ctx, options)
	if err != nil {
		return repositoryTargetPlan{}, err
	}
	readmeRows := compileReadmeRoleLog(options.Repository, discovery.readme)

	override := strings.TrimSpace(options.TargetOverride)
	if override != "" {
		plan, resolveErr := resolveExplicitRepositoryTarget(
			options.Repository, discovery, override, readmeRows,
		)
		if resolveErr != nil {
			return repositoryTargetPlan{}, resolveErr
		}
		if options.Output != nil {
			options.Output.State(
				"Target hypothesis merge", "not needed",
				"reason: explicit --target bypasses candidate merging",
			)
			options.Output.State(
				"Repository target plan", "ready",
				"source: explicit --target",
				"selected: "+plan.Default.String(),
			)
		}
		return plan, nil
	}

	readmeCandidates, unsupported := repositoryResolvableReadmeCandidates(
		discovery, discovery.readme.TargetCandidates(),
	)
	if options.Output != nil && unsupported > 0 {
		options.Output.Stage(
			"Repository guidance classifier",
			fmt.Sprintf(
				"kept %d target hypotheses resolvable by one enabled language adapter; retained %d unsupported or ambiguous target roles only in diagnostics",
				len(readmeCandidates), unsupported,
			),
		)
	}
	adapterCandidates := make([][]analysistarget.FileCandidate, 0, len(discovery.adapters))
	for _, adapter := range discovery.adapters {
		adapterCandidates = append(adapterCandidates, adapter.Candidates)
	}
	nativeCandidates, err := analysistarget.MergeFileCandidates(
		options.Repository.Snapshot(), adapterCandidates...,
	)
	if err != nil {
		return repositoryTargetPlan{}, fmt.Errorf("merge native repository target hypotheses: %w", err)
	}
	merged, err := analysistarget.MergeFileCandidates(
		options.Repository.Snapshot(),
		nativeCandidates,
		readmeCandidates,
	)
	if err != nil {
		return repositoryTargetPlan{}, fmt.Errorf("merge repository target hypotheses: %w", err)
	}
	if len(merged) == 0 {
		return repositoryTargetPlan{}, fmt.Errorf("repository target discovery returned no file hypotheses")
	}
	requiredTargetFileRefs, err := repositoryRequiredTargetFileRefs(discovery, nativeCandidates)
	if err != nil {
		return repositoryTargetPlan{}, fmt.Errorf("bind exact repository target authority: %w", err)
	}

	portfolio, outcome, err := selectTargetPortfolioForRun(
		ctx,
		options.Repository.Snapshot(),
		merged,
		nil,
		&requiredTargetFileRefs,
		options.Output,
		options.Providers,
		options.Executor,
	)
	outcome.ReadmeRoles = cloneReadmeRoleLog(readmeRows)
	if err != nil {
		groups, choiceErr := repositoryTargetChoiceGroups(discovery)
		if choiceErr != nil {
			return repositoryTargetPlan{Outcome: outcome}, fmt.Errorf("%w; build exact target choices: %v", err, choiceErr)
		}
		return repositoryTargetPlan{Outcome: outcome}, withTargetPortfolioChoices(err, groups...)
	}
	plan, err := restoreRepositoryTargetPortfolio(discovery, portfolio, outcome)
	if err != nil {
		return repositoryTargetPlan{Outcome: outcome}, err
	}
	if options.Output != nil {
		options.Output.State(
			"Repository target plan", "ready",
			fmt.Sprintf("typed targets: %d", len(plan.Targets)),
			"default: "+plan.Default.String(),
		)
	}
	return plan, nil
}

// repositoryRequiredTargetFileRefs projects exactly one canonical
// representative file for every deterministic native target. Language scouts
// may advertise several useful files for one target; those alternatives remain
// in the shared candidate catalog but do not become duplicate mandatory refs or
// duplicate pages. The returned list is language-neutral and follows the
// canonical merged candidate order.
func repositoryRequiredTargetFileRefs(
	discovery repositoryTargetDiscovery,
	nativeCandidates []analysistarget.FileCandidate,
) ([]corpus.FileID, error) {
	selected := make(map[corpus.FileID]struct{})
	add := func(refs []corpus.FileID) {
		for _, fileRef := range refs {
			selected[fileRef] = struct{}{}
		}
	}

	for _, adapter := range discovery.adapters {
		add(adapter.RequiredFileRefs)
	}

	result := make([]corpus.FileID, 0, len(selected))
	for _, candidate := range nativeCandidates {
		if _, required := selected[candidate.FileRef]; required {
			result = append(result, candidate.FileRef)
			delete(selected, candidate.FileRef)
		}
	}
	if len(selected) != 0 {
		return nil, fmt.Errorf("canonical target representative is outside the merged native candidates")
	}
	if result == nil {
		return []corpus.FileID{}, nil
	}
	return result, nil
}

func canonicalNativeTargetFileRefs(
	language string,
	candidates []analysistarget.FileCandidate,
	targetRefs []string,
	resolve func(corpus.FileID) ([]string, error),
) ([]corpus.FileID, error) {
	knownTargets := make(map[string]struct{}, len(targetRefs))
	for _, targetRef := range targetRefs {
		if targetRef == "" {
			return nil, fmt.Errorf("%s target authority contains an empty target ref", language)
		}
		if _, duplicate := knownTargets[targetRef]; duplicate {
			return nil, fmt.Errorf("%s target authority contains duplicate target %q", language, targetRef)
		}
		knownTargets[targetRef] = struct{}{}
	}

	firstFileByTarget := make(map[string]corpus.FileID, len(targetRefs))
	for _, candidate := range candidates {
		resolved, err := resolve(candidate.FileRef)
		if err != nil {
			return nil, fmt.Errorf("resolve %s candidate %q: %w", language, candidate.FileRef, err)
		}
		if len(resolved) == 0 {
			return nil, fmt.Errorf("%s candidate %q restores no exact target", language, candidate.FileRef)
		}
		for _, targetRef := range resolved {
			if _, known := knownTargets[targetRef]; !known {
				return nil, fmt.Errorf("%s candidate %q restores unknown target %q", language, candidate.FileRef, targetRef)
			}
			if _, represented := firstFileByTarget[targetRef]; !represented {
				firstFileByTarget[targetRef] = candidate.FileRef
			}
		}
	}

	selected := make(map[corpus.FileID]struct{}, len(targetRefs))
	for _, targetRef := range targetRefs {
		fileRef, represented := firstFileByTarget[targetRef]
		if !represented {
			return nil, fmt.Errorf("%s exact target %q has no file representative", language, targetRef)
		}
		selected[fileRef] = struct{}{}
	}
	result := make([]corpus.FileID, 0, len(selected))
	for _, candidate := range candidates {
		if _, ok := selected[candidate.FileRef]; ok {
			result = append(result, candidate.FileRef)
			delete(selected, candidate.FileRef)
		}
	}
	if len(selected) != 0 {
		return nil, fmt.Errorf("%s canonical target representative is outside its candidate catalog", language)
	}
	if result == nil {
		return []corpus.FileID{}, nil
	}
	return result, nil
}

func discoverRepositoryTargets(
	ctx context.Context,
	options repositoryTargetRuntimeOptions,
) (repositoryTargetDiscovery, error) {
	var result repositoryTargetDiscovery
	registry, err := ordinaryRepositoryTargetAdapterRegistry()
	if err != nil {
		return result, err
	}
	parallelContext, cancelParallel := context.WithCancel(ctx)
	defer cancelParallel()
	type adapterResult struct {
		key       repositoryTargetAdapter
		discovery repositoryTargetAdapterDiscovery
		enabled   bool
		err       error
	}
	adapterChannel := make(chan adapterResult, len(registry.ordered))
	for _, descriptor := range registry.ordered {
		descriptor := descriptor
		go func() {
			discovery, enabled, discoverErr := descriptor.Discover(parallelContext, options)
			adapterChannel <- adapterResult{
				key: descriptor.Key, discovery: discovery, enabled: enabled, err: discoverErr,
			}
		}()
	}

	type readmeResult struct {
		discovery readmeFileRoleDiscovery
		err       error
	}
	readmeChannel := make(chan readmeResult, 1)
	go func() {
		if !readmetargetscout.HasGuidanceFiles(options.Repository) {
			readmeChannel <- readmeResult{discovery: readmeFileRoleDiscovery{
				Roles: readmetargetscout.Result{},
			}}
			return
		}
		discovery, err := discoverReadmeFileRolesWithGuidance(
			parallelContext,
			options.RepoName,
			options.Repository,
			options.Output,
			options.Providers,
			options.Executor,
		)
		readmeChannel <- readmeResult{discovery: discovery, err: err}
	}()

	discoveredByKey := make(map[repositoryTargetAdapter]repositoryTargetAdapterDiscovery)
	for range registry.ordered {
		adapter := <-adapterChannel
		if adapter.err != nil {
			cancelParallel()
			return result, adapter.err
		}
		if !adapter.enabled {
			continue
		}
		if adapter.discovery.Key != adapter.key {
			return result, fmt.Errorf(
				"repository target adapter %q returned discovery for %q", adapter.key, adapter.discovery.Key,
			)
		}
		if err := adapter.discovery.validate(registry); err != nil {
			return result, err
		}
		discoveredByKey[adapter.key] = adapter.discovery
	}
	readme := <-readmeChannel
	if readme.err != nil {
		return result, readme.err
	}
	if len(discoveredByKey) == 0 {
		return result, fmt.Errorf("repository target selection has no enabled language adapter")
	}
	result.byKey = make(map[repositoryTargetAdapter]repositoryTargetAdapterDiscovery, len(discoveredByKey))
	for _, descriptor := range registry.ordered {
		discovery, ok := discoveredByKey[descriptor.Key]
		if !ok {
			continue
		}
		result.adapters = append(result.adapters, discovery)
		result.byKey[descriptor.Key] = discovery
	}
	result.readme = readme.discovery.Roles
	result.guidance = readme.discovery.Guidance
	return result, nil
}

func repositoryResolvableReadmeCandidates(
	discovery repositoryTargetDiscovery,
	candidates []analysistarget.FileCandidate,
) ([]analysistarget.FileCandidate, int) {
	result := make([]analysistarget.FileCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		_, matched, err := discovery.adapterForFile(candidate.FileRef)
		if err != nil || !matched {
			continue
		}
		result = append(result, candidate)
	}
	if result == nil {
		result = []analysistarget.FileCandidate{}
	}
	return result, len(candidates) - len(result)
}

func (discovery repositoryTargetDiscovery) adapterForFile(
	fileRef corpus.FileID,
) (repositoryTargetAdapter, bool, error) {
	matches := make([]repositoryTargetAdapter, 0, len(discovery.adapters))
	for _, adapter := range discovery.adapters {
		if adapter.ResolvesFile(fileRef) {
			matches = append(matches, adapter.Key)
		}
	}
	if len(matches) == 0 {
		return "", false, nil
	}
	if len(matches) != 1 {
		values := make([]string, len(matches))
		for index, match := range matches {
			values[index] = string(match)
		}
		return "", false, fmt.Errorf(
			"file_ref %q is ambiguous between exact adapters: %s",
			fileRef, strings.Join(values, ", "),
		)
	}
	return matches[0], true, nil
}

func resolveExplicitRepositoryTarget(
	repository *corpus.Corpus,
	discovery repositoryTargetDiscovery,
	override string,
	readmeRows []readmeRoleLogRow,
) (repositoryTargetPlan, error) {
	matches := make(map[repositoryTargetKey]repositoryTypedTarget)
	for _, adapter := range discovery.adapters {
		resolved, err := adapter.ResolveExplicit(repository, override)
		if err != nil {
			return repositoryTargetPlan{}, fmt.Errorf(
				"resolve exact target through adapter %q: %w", adapter.Key, err,
			)
		}
		for _, target := range resolved {
			if target.Key.Adapter != adapter.Key {
				return repositoryTargetPlan{}, fmt.Errorf(
					"adapter %q restored target owned by %q", adapter.Key, target.Key.Adapter,
				)
			}
			matches[target.Key] = target
		}
	}
	if len(matches) != 1 {
		groups, err := repositoryTargetChoiceGroups(discovery)
		if err != nil {
			return repositoryTargetPlan{}, err
		}
		base := fmt.Errorf("--target %q is not one unambiguous exact repository target selector", override)
		if len(matches) > 1 {
			matchedSelectors := make([]string, 0, len(matches))
			for _, target := range matches {
				matchedSelectors = append(matchedSelectors, target.Selector)
			}
			sort.Strings(matchedSelectors)
			base = fmt.Errorf(
				"--target %q matches more than one exact repository target; matching exact selectors: %s",
				override,
				strings.Join(matchedSelectors, ", "),
			)
		}
		return repositoryTargetPlan{}, withTargetPortfolioChoices(base, groups...)
	}
	var selected repositoryTypedTarget
	for _, target := range matches {
		selected = target
	}
	selected.FileRefs = []corpus.FileID{}
	outcome := targetPortfolioRunOutcome{
		SelectedRef: selected.Key.String(), SelectedTargets: 1,
		SelectedTargetRefs: []string{selected.Key.String()},
		ReadmeRoles:        cloneReadmeRoleLog(readmeRows),
	}
	plan, err := repositoryTargetPlanFromDiscovery(
		discovery, []repositoryTypedTarget{selected}, selected.Key, true, outcome,
	)
	if err != nil {
		return repositoryTargetPlan{}, err
	}
	return plan, nil
}

type repositoryTargetBuilder struct {
	target repositoryTypedTarget
	files  map[corpus.FileID]struct{}
}

func restoreRepositoryTargetPortfolio(
	discovery repositoryTargetDiscovery,
	portfolio targetportfolio.Selection,
	outcome targetPortfolioRunOutcome,
) (repositoryTargetPlan, error) {
	if portfolio.Default == nil {
		return repositoryTargetPlan{}, fmt.Errorf("repository target portfolio accepted targets without a default")
	}
	selectedFiles := make(map[repositoryTargetAdapter][]corpus.FileID, len(discovery.adapters))
	for _, candidate := range portfolio.Targets {
		adapter, matched, err := discovery.adapterForFile(candidate.FileRef)
		if err != nil {
			return repositoryTargetPlan{}, fmt.Errorf("restore selected file %q: %w", candidate.Path, err)
		}
		if !matched {
			return repositoryTargetPlan{}, fmt.Errorf(
				"restore selected file %q: file is outside every enabled exact adapter", candidate.Path,
			)
		}
		selectedFiles[adapter] = append(selectedFiles[adapter], candidate.FileRef)
	}
	defaultAdapter, matched, err := discovery.adapterForFile(portfolio.Default.FileRef)
	if err != nil {
		return repositoryTargetPlan{}, fmt.Errorf("restore default file %q: %w", portfolio.Default.Path, err)
	}
	if !matched {
		return repositoryTargetPlan{}, fmt.Errorf(
			"restore default file %q: file is outside every enabled exact adapter", portfolio.Default.Path,
		)
	}

	builders := make(map[repositoryTargetKey]*repositoryTargetBuilder)
	addTarget := func(target repositoryTypedTarget) error {
		if err := target.Validate(); err != nil {
			return err
		}
		if existing, ok := builders[target.Key]; ok {
			if existing.target.Selector != target.Selector || existing.target.Key.Adapter != target.Key.Adapter {
				return fmt.Errorf("restored target %q has conflicting exact identities", target.Key)
			}
			return nil
		}
		builders[target.Key] = &repositoryTargetBuilder{
			target: target, files: make(map[corpus.FileID]struct{}),
		}
		return nil
	}
	addFile := func(key repositoryTargetKey, fileRef corpus.FileID) error {
		builder, ok := builders[key]
		if !ok {
			return fmt.Errorf("restored file_ref %q cites missing target %q", fileRef, key)
		}
		builder.files[fileRef] = struct{}{}
		return nil
	}

	for _, adapter := range discovery.adapters {
		fileRefs := selectedFiles[adapter.Key]
		if len(fileRefs) == 0 {
			continue
		}
		restored, restoreErr := adapter.RestoreFiles(fileRefs)
		if restoreErr != nil {
			return repositoryTargetPlan{}, fmt.Errorf(
				"restore selected targets through adapter %q: %w", adapter.Key, restoreErr,
			)
		}
		for _, value := range restored {
			if value.Target.Key.Adapter != adapter.Key {
				return repositoryTargetPlan{}, fmt.Errorf(
					"adapter %q restored target owned by %q", adapter.Key, value.Target.Key.Adapter,
				)
			}
			if err := addTarget(value.Target); err != nil {
				return repositoryTargetPlan{}, err
			}
			for _, fileRef := range value.FileRefs {
				if err := addFile(value.Target.Key, fileRef); err != nil {
					return repositoryTargetPlan{}, err
				}
			}
		}
	}

	// TargetPortfolio chooses a default file, not one semantic view of that
	// file. A shared exact representative may legitimately restore several
	// targets; all of them remain in the plan. The landing-page default is the
	// first owner in the same canonical target order used by the plan itself,
	// so this presentation choice needs no second model gate or adapter-specific
	// ResolveOne restriction.
	defaultOwners := make([]repositoryTypedTarget, 0)
	for _, builder := range builders {
		if builder.target.Key.Adapter != defaultAdapter {
			continue
		}
		if _, ownsDefaultFile := builder.files[portfolio.Default.FileRef]; ownsDefaultFile {
			defaultOwners = append(defaultOwners, builder.target)
		}
	}
	if len(defaultOwners) == 0 {
		return repositoryTargetPlan{}, fmt.Errorf(
			"repository target portfolio default file %q is outside restored selected targets",
			portfolio.Default.Path,
		)
	}
	sort.Slice(defaultOwners, func(i, j int) bool {
		return repositoryTypedTargetLess(defaultOwners[i], defaultOwners[j])
	})
	defaultKey := defaultOwners[0].Key

	targets := make([]repositoryTypedTarget, 0, len(builders))
	for _, builder := range builders {
		builder.target.FileRefs = make([]corpus.FileID, 0, len(builder.files))
		for fileRef := range builder.files {
			builder.target.FileRefs = append(builder.target.FileRefs, fileRef)
		}
		sort.Slice(builder.target.FileRefs, func(i, j int) bool {
			return builder.target.FileRefs[i] < builder.target.FileRefs[j]
		})
		targets = append(targets, builder.target)
	}
	sort.Slice(targets, func(i, j int) bool { return repositoryTypedTargetLess(targets[i], targets[j]) })
	outcome.SelectedRef = defaultKey.String()
	outcome.SelectedTargets = len(targets)
	outcome.SelectedTargetRefs = repositoryTargetRefs(targets)
	outcome.SelectedFileRefs = len(portfolio.Targets)
	outcome.UnclassifiedFiles = len(portfolio.Unclassified)
	return repositoryTargetPlanFromDiscovery(discovery, targets, defaultKey, false, outcome)
}

func repositoryTargetPlanFromDiscovery(
	discovery repositoryTargetDiscovery,
	targets []repositoryTypedTarget,
	defaultKey repositoryTargetKey,
	explicit bool,
	outcome targetPortfolioRunOutcome,
) (repositoryTargetPlan, error) {
	guidance, err := discovery.guidance.Snapshot()
	if err != nil {
		return repositoryTargetPlan{}, fmt.Errorf("own repository guidance: %w", err)
	}
	plan := repositoryTargetPlan{
		Targets: append([]repositoryTypedTarget(nil), targets...),
		Default: defaultKey, Explicit: explicit, Outcome: outcome, guidance: guidance,
		Authorities: make(map[repositoryTargetAdapter]any),
	}
	for _, adapter := range discovery.adapters {
		authority, authorityErr := adapter.SnapshotAuthority()
		if authorityErr != nil {
			return repositoryTargetPlan{}, fmt.Errorf(
				"own repository plan authority for adapter %q: %w", adapter.Key, authorityErr,
			)
		}
		if authority != nil {
			plan.Authorities[adapter.Key] = authority
		}
	}
	if err := plan.Validate(); err != nil {
		return repositoryTargetPlan{}, err
	}
	return plan, nil
}

func repositoryTargetChoiceGroups(
	discovery repositoryTargetDiscovery,
) ([]targetPortfolioChoiceGroup, error) {
	groups := make([]targetPortfolioChoiceGroup, 0, len(discovery.adapters))
	for _, adapter := range discovery.adapters {
		group, err := adapter.ChoiceGroup()
		if err != nil {
			return nil, fmt.Errorf("build choices for adapter %q: %w", adapter.Key, err)
		}
		groups = append(groups, group)
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("no exact repository target choices were discovered")
	}
	return groups, nil
}

func repositoryTargetRefs(targets []repositoryTypedTarget) []string {
	refs := make([]string, len(targets))
	for index, target := range targets {
		refs[index] = target.Key.String()
	}
	sort.Strings(refs)
	return refs
}

func sameRepositoryStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftCopy := append([]string(nil), left...)
	rightCopy := append([]string(nil), right...)
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	for index := range leftCopy {
		if leftCopy[index] != rightCopy[index] {
			return false
		}
	}
	return true
}
