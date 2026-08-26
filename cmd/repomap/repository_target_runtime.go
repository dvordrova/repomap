package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/jstsproject"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
	"github.com/dvordrova/repomap/internal/snapshot"
	"github.com/dvordrova/repomap/internal/targetportfolio"
)

// repositoryTargetAdapter is the command-owned dispatch key for one native
// language adapter. JavaScript and TypeScript deliberately share one adapter:
// the sealed selected package project retains its exact producer language.
type repositoryTargetAdapter string

const (
	repositoryTargetAdapterGo     repositoryTargetAdapter = "go"
	repositoryTargetAdapterPython repositoryTargetAdapter = "python"
	repositoryTargetAdapterJSTS   repositoryTargetAdapter = "jsts"
)

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

// repositoryTargetKey is the collision-safe identity used by the execution
// plan. Native refs remain untouched, but never become cross-adapter map keys
// on their own.
type repositoryTargetKey struct {
	Adapter repositoryTargetAdapter
	Ref     string
}

func (key repositoryTargetKey) String() string {
	return string(key.Adapter) + ":" + key.Ref
}

// repositoryTypedTarget carries exactly one adapter-native payload. Selector
// is the exact user-facing spelling that can restore this target without a
// semantic request. FileRefs records only the closed portfolio choices that
// restored the payload; it is empty for an explicit selector bypass.
type repositoryTypedTarget struct {
	Key      repositoryTargetKey
	Selector string
	FileRefs []corpus.FileID

	Go     *analysistarget.Target
	Python *pythontarget.Target
	JSTS   *jstsproject.Target
}

func (target repositoryTypedTarget) Validate() error {
	if strings.TrimSpace(target.Key.Ref) == "" || target.Key.Ref != strings.TrimSpace(target.Key.Ref) ||
		strings.TrimSpace(target.Selector) == "" || target.Selector != strings.TrimSpace(target.Selector) {
		return fmt.Errorf("repository target: invalid exact identity")
	}
	payloads := boolCount(target.Go != nil) + boolCount(target.Python != nil) + boolCount(target.JSTS != nil)
	if payloads != 1 {
		return fmt.Errorf("repository target %q: expected exactly one native payload, got %d", target.Key, payloads)
	}
	for index, fileRef := range target.FileRefs {
		if strings.TrimSpace(string(fileRef)) == "" || string(fileRef) != strings.TrimSpace(string(fileRef)) {
			return fmt.Errorf("repository target %q: invalid file_ref at index %d", target.Key, index)
		}
		if index > 0 && target.FileRefs[index-1] >= fileRef {
			return fmt.Errorf("repository target %q: file refs are not canonical", target.Key)
		}
	}

	switch target.Key.Adapter {
	case repositoryTargetAdapterGo:
		if target.Go == nil || target.Python != nil || target.JSTS != nil {
			return fmt.Errorf("repository target %q: Go adapter/payload mismatch", target.Key)
		}
		if err := target.Go.Validate(); err != nil {
			return fmt.Errorf("repository target %q: Go payload: %w", target.Key, err)
		}
		if target.Key.Ref != target.Go.Ref {
			return fmt.Errorf("repository target %q: Go ref mismatch", target.Key)
		}
	case repositoryTargetAdapterPython:
		if target.Python == nil || target.Go != nil || target.JSTS != nil {
			return fmt.Errorf("repository target %q: Python adapter/payload mismatch", target.Key)
		}
		if err := target.Python.Validate(); err != nil {
			return fmt.Errorf("repository target %q: Python payload: %w", target.Key, err)
		}
		if target.Key.Ref != target.Python.Ref || target.Selector != target.Python.Selector {
			return fmt.Errorf("repository target %q: Python exact identity mismatch", target.Key)
		}
	case repositoryTargetAdapterJSTS:
		if target.JSTS == nil || target.Go != nil || target.Python != nil {
			return fmt.Errorf("repository target %q: JavaScript/TypeScript adapter/payload mismatch", target.Key)
		}
		if err := target.JSTS.Validate(); err != nil {
			return fmt.Errorf("repository target %q: JavaScript/TypeScript scout target: %w", target.Key, err)
		}
		if target.Key.Ref != target.JSTS.Ref || target.Selector != target.JSTS.Selector {
			return fmt.Errorf("repository target %q: JavaScript/TypeScript exact identity mismatch", target.Key)
		}
	default:
		return fmt.Errorf("repository target %q: unsupported adapter", target.Key)
	}
	return nil
}

// repositoryTargetPlan is the exact, typed output of one repository-wide
// target decision. GoSource is the one unscoped snapshot from which a caller
// may derive a target-local snapshot with snapshot.ScopeAnalysisTarget.
// PythonCatalog retains ownership of native and resolver-derived Python views.
type repositoryTargetPlan struct {
	Targets       []repositoryTypedTarget
	Default       repositoryTargetKey
	Explicit      bool
	GoSource      *snapshot.Snapshot
	PythonCatalog *pythontarget.Catalog
	Outcome       targetPortfolioRunOutcome
}

func (plan repositoryTargetPlan) DefaultTarget() (repositoryTypedTarget, bool) {
	for _, target := range plan.Targets {
		if target.Key == plan.Default {
			return target, true
		}
	}
	return repositoryTypedTarget{}, false
}

func (plan repositoryTargetPlan) Validate() error {
	if len(plan.Targets) == 0 {
		return fmt.Errorf("repository target plan: target set is empty")
	}
	if plan.GoSource != nil {
		if plan.GoSource.AnalysisTarget != nil || plan.GoSource.GoFacts == nil || plan.GoSource.TargetCatalog == nil {
			return fmt.Errorf("repository target plan: Go source is not an unscoped catalog snapshot")
		}
		if err := plan.GoSource.TargetCatalog.Validate(); err != nil {
			return fmt.Errorf("repository target plan: Go source catalog: %w", err)
		}
		rebuilt, err := analysistarget.BuildCatalog(*plan.GoSource.GoFacts)
		if err != nil {
			return fmt.Errorf("repository target plan: Go source facts: %w", err)
		}
		if rebuilt.Ref != plan.GoSource.TargetCatalog.Ref {
			return fmt.Errorf("repository target plan: Go source catalog does not match its facts")
		}
	}
	if plan.PythonCatalog != nil {
		if err := plan.PythonCatalog.Validate(); err != nil {
			return fmt.Errorf("repository target plan: Python catalog: %w", err)
		}
	}

	seen := make(map[repositoryTargetKey]struct{}, len(plan.Targets))
	defaultCount := 0
	for index, target := range plan.Targets {
		if err := target.Validate(); err != nil {
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

		switch target.Key.Adapter {
		case repositoryTargetAdapterGo:
			if plan.GoSource == nil {
				return fmt.Errorf("repository target plan: Go target has no source snapshot")
			}
			entry, ok := targetCatalogEntryByRef(*plan.GoSource.TargetCatalog, target.Key.Ref)
			if !ok || entry.Candidate.Key != target.Selector || entry.Candidate.Target.Ref != target.Go.Ref {
				return fmt.Errorf("repository target plan: Go target %q is outside exact source authority", target.Key)
			}
		case repositoryTargetAdapterPython:
			if plan.PythonCatalog == nil || !plan.PythonCatalog.OwnsTarget(*target.Python) {
				return fmt.Errorf("repository target plan: Python target %q is outside exact catalog authority", target.Key)
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

type repositoryTargetDiscovery struct {
	goSource     *snapshot.Snapshot
	goCandidates []analysistarget.FileCandidate
	goResolver   analysistarget.GoFileTargetResolver

	pythonCatalog    *pythontarget.Catalog
	pythonCandidates []analysistarget.FileCandidate
	pythonResolver   pythontarget.FileTargetResolver

	jstsTargets    []jstsproject.Target
	jstsByManifest map[corpus.FileID]jstsproject.Target
	jstsCandidates []analysistarget.FileCandidate

	readme readmetargetscout.Result
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
		plan, resolveErr := resolveExplicitRepositoryTarget(discovery, override, readmeRows)
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
	nativeCandidates, err := analysistarget.MergeFileCandidates(
		options.Repository.Snapshot(),
		discovery.goCandidates,
		discovery.pythonCandidates,
		discovery.jstsCandidates,
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

	if discovery.goSource != nil && discovery.goSource.TargetCatalog != nil {
		targetRefs := make([]string, len(discovery.goSource.TargetCatalog.Entries))
		for index, entry := range discovery.goSource.TargetCatalog.Entries {
			targetRefs[index] = entry.Candidate.Target.Ref
		}
		refs, err := canonicalNativeTargetFileRefs(
			"Go",
			discovery.goCandidates,
			targetRefs,
			func(fileRef corpus.FileID) ([]string, error) {
				return discovery.goResolver.Resolve([]corpus.FileID{fileRef})
			},
		)
		if err != nil {
			return nil, err
		}
		add(refs)
	}
	if discovery.pythonCatalog != nil {
		targetRefs := make([]string, len(discovery.pythonCatalog.Entries))
		for index, target := range discovery.pythonCatalog.Entries {
			targetRefs[index] = target.Ref
		}
		refs, err := canonicalNativeTargetFileRefs(
			"Python",
			discovery.pythonCandidates,
			targetRefs,
			func(fileRef corpus.FileID) ([]string, error) {
				targets, resolveErr := discovery.pythonResolver.Resolve([]corpus.FileID{fileRef})
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
			return nil, err
		}
		add(refs)
	}
	for _, target := range discovery.jstsTargets {
		selected[corpus.FileID(target.ManifestFileRef)] = struct{}{}
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
	if options.GoSnapshot != nil {
		owned, err := snapshot.OwnSnapshot(*options.GoSnapshot)
		if err != nil {
			return result, fmt.Errorf("own prepared Go snapshot: %w", err)
		}
		if owned.AnalysisTarget != nil || owned.GoFacts == nil || owned.TargetCatalog == nil {
			return result, fmt.Errorf("prepared Go snapshot must be unscoped and contain exact facts plus target catalog")
		}
		if err := owned.TargetCatalog.Validate(); err != nil {
			return result, fmt.Errorf("validate prepared Go target catalog: %w", err)
		}
		rebuilt, err := analysistarget.BuildCatalog(*owned.GoFacts)
		if err != nil {
			return result, fmt.Errorf("validate prepared Go facts: %w", err)
		}
		if rebuilt.Ref != owned.TargetCatalog.Ref {
			return result, fmt.Errorf("prepared Go target catalog does not match its facts")
		}
		result.goSource = &owned
	}
	if result.goSource == nil && !options.DiscoverPython && !options.DiscoverJSTS {
		return result, fmt.Errorf("repository target selection has no enabled language adapter")
	}

	parallelContext, cancelParallel := context.WithCancel(ctx)
	defer cancelParallel()
	type pythonResult struct {
		catalog pythontarget.Catalog
		err     error
	}
	pythonChannel := make(chan pythonResult, 1)
	go func() {
		if !options.DiscoverPython {
			pythonChannel <- pythonResult{}
			return
		}
		catalog, err := pythontarget.Discover(parallelContext, options.Repository)
		pythonChannel <- pythonResult{catalog: catalog, err: err}
	}()

	type jstsResult struct {
		targets []jstsproject.Target
		err     error
	}
	jstsChannel := make(chan jstsResult, 1)
	go func() {
		if !options.DiscoverJSTS {
			jstsChannel <- jstsResult{}
			return
		}
		scout := options.ScoutJSTSFn
		if scout == nil {
			scout = jstsproject.ScoutTargets
		}
		targets, err := scout(
			parallelContext,
			options.Repository,
			exactJSTSManifestSelector(options.TargetOverride),
		)
		jstsChannel <- jstsResult{targets: targets, err: err}
	}()

	type readmeResult struct {
		roles readmetargetscout.Result
		err   error
	}
	readmeChannel := make(chan readmeResult, 1)
	go func() {
		if !readmetargetscout.HasGuidanceFiles(options.Repository) {
			readmeChannel <- readmeResult{roles: readmetargetscout.Result{}}
			return
		}
		roles, err := discoverReadmeFileRoles(
			parallelContext,
			options.RepoName,
			options.Repository,
			options.Output,
			options.Providers,
			options.Executor,
		)
		readmeChannel <- readmeResult{roles: roles, err: err}
	}()

	var goErr error
	if result.goSource != nil && len(result.goSource.TargetCatalog.Entries) > 0 {
		result.goCandidates, result.goResolver, goErr = analysistarget.DiscoverGoTargetFilesWithResolver(
			options.Repository,
			*result.goSource.GoFacts,
			*result.goSource.TargetCatalog,
		)
	}
	python := <-pythonChannel
	jsts := <-jstsChannel
	readme := <-readmeChannel
	if goErr != nil {
		return result, fmt.Errorf("discover Go target files: %w", goErr)
	}
	if python.err != nil {
		return result, fmt.Errorf("discover Python targets: %w", python.err)
	}
	if jsts.err != nil {
		return result, fmt.Errorf("scout JavaScript/TypeScript package target: %w", jsts.err)
	}
	if readme.err != nil {
		return result, readme.err
	}
	result.readme = readme.roles

	if options.DiscoverPython {
		if err := python.catalog.Validate(); err != nil {
			return result, fmt.Errorf("validate Python target catalog: %w", err)
		}
		catalog := python.catalog.Snapshot()
		candidates, resolver, err := pythontarget.FileCandidatesWithResolver(options.Repository, catalog)
		if err != nil {
			return result, fmt.Errorf("project Python targets into repository portfolio: %w", err)
		}
		result.pythonCatalog = &catalog
		result.pythonCandidates = candidates
		result.pythonResolver = resolver
	}
	if options.DiscoverJSTS {
		if len(jsts.targets) == 0 {
			return result, fmt.Errorf("JavaScript/TypeScript target scout returned no exact package targets")
		}
		result.jstsTargets = append([]jstsproject.Target(nil), jsts.targets...)
		result.jstsByManifest = make(map[corpus.FileID]jstsproject.Target, len(jsts.targets))
		result.jstsCandidates = make([]analysistarget.FileCandidate, 0, len(jsts.targets))
		seenRefs := make(map[string]struct{}, len(jsts.targets))
		for index, target := range result.jstsTargets {
			if err := target.ValidateAgainst(options.Repository); err != nil {
				return result, fmt.Errorf("bind JavaScript/TypeScript scout target %d to the current repository: %w", index, err)
			}
			if index > 0 && (result.jstsTargets[index-1].Selector >= target.Selector) {
				return result, fmt.Errorf("JavaScript/TypeScript scout targets are not canonical")
			}
			if _, duplicate := seenRefs[target.Ref]; duplicate {
				return result, fmt.Errorf("JavaScript/TypeScript scout targets share ref %q", target.Ref)
			}
			seenRefs[target.Ref] = struct{}{}
			manifest := corpus.FileID(target.ManifestFileRef)
			if _, duplicate := result.jstsByManifest[manifest]; duplicate {
				return result, fmt.Errorf("JavaScript/TypeScript scout targets share manifest file_ref %q", manifest)
			}
			result.jstsByManifest[manifest] = target
			result.jstsCandidates = append(result.jstsCandidates, analysistarget.FileCandidate{
				FileRef: manifest,
				Hypotheses: []string{
					"JavaScript/TypeScript package project with an exact tracked manifest and owned source-file evidence",
				},
			})
		}
	}
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
	matches := make([]repositoryTargetAdapter, 0, 3)
	if discovery.goSource != nil && discovery.goResolver.ResolvesOne(fileRef) {
		matches = append(matches, repositoryTargetAdapterGo)
	}
	if discovery.pythonCatalog != nil && discovery.pythonResolver.Resolves(fileRef) {
		matches = append(matches, repositoryTargetAdapterPython)
	}
	if _, ok := discovery.jstsByManifest[fileRef]; ok {
		matches = append(matches, repositoryTargetAdapterJSTS)
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
	discovery repositoryTargetDiscovery,
	override string,
	readmeRows []readmeRoleLogRow,
) (repositoryTargetPlan, error) {
	matches := make(map[repositoryTargetKey]repositoryTypedTarget)
	if discovery.goSource != nil {
		for _, entry := range discovery.goSource.TargetCatalog.Entries {
			if override != entry.Candidate.Key && override != entry.Candidate.Target.Ref {
				continue
			}
			target := entry.Candidate.Target.Snapshot()
			key := repositoryTargetKey{Adapter: repositoryTargetAdapterGo, Ref: target.Ref}
			matches[key] = repositoryTypedTarget{Key: key, Selector: entry.Candidate.Key, Go: &target}
		}
	}
	if discovery.pythonCatalog != nil {
		for _, entry := range discovery.pythonCatalog.Entries {
			if override != entry.Selector && override != entry.Ref && override != entry.IdentityRef {
				continue
			}
			target := entry
			key := repositoryTargetKey{Adapter: repositoryTargetAdapterPython, Ref: target.Ref}
			matches[key] = repositoryTypedTarget{Key: key, Selector: target.Selector, Python: &target}
		}
		if derived, ok, err := discovery.pythonResolver.ResolveSelector(override); err != nil {
			return repositoryTargetPlan{}, fmt.Errorf("resolve exact Python selector: %w", err)
		} else if ok {
			target := derived
			key := repositoryTargetKey{Adapter: repositoryTargetAdapterPython, Ref: target.Ref}
			matches[key] = repositoryTypedTarget{Key: key, Selector: target.Selector, Python: &target}
		}
	}
	for _, value := range discovery.jstsTargets {
		if override != value.Selector && override != value.Ref {
			continue
		}
		target := value
		key := repositoryTargetKey{Adapter: repositoryTargetAdapterJSTS, Ref: target.Ref}
		matches[key] = repositoryTypedTarget{Key: key, Selector: target.Selector, JSTS: &target}
	}
	if len(matches) != 1 {
		groups, err := repositoryTargetChoiceGroups(discovery)
		if err != nil {
			return repositoryTargetPlan{}, err
		}
		base := fmt.Errorf("--target %q is not one unambiguous exact repository target selector", override)
		if len(matches) > 1 {
			base = fmt.Errorf("--target %q matches more than one exact repository target", override)
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
	selectedFiles := map[repositoryTargetAdapter][]corpus.FileID{
		repositoryTargetAdapterGo:     {},
		repositoryTargetAdapterPython: {},
		repositoryTargetAdapterJSTS:   {},
	}
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

	goRefs := []string{}
	if len(selectedFiles[repositoryTargetAdapterGo]) > 0 {
		goRefs, err = discovery.goResolver.Resolve(selectedFiles[repositoryTargetAdapterGo])
		if err != nil {
			return repositoryTargetPlan{}, fmt.Errorf("restore selected Go targets: %w", err)
		}
		for _, ref := range goRefs {
			entry, ok := targetCatalogEntryByRef(*discovery.goSource.TargetCatalog, ref)
			if !ok {
				return repositoryTargetPlan{}, fmt.Errorf("restored Go target %q is outside exact catalog", ref)
			}
			target := entry.Candidate.Target.Snapshot()
			key := repositoryTargetKey{Adapter: repositoryTargetAdapterGo, Ref: ref}
			if err := addTarget(repositoryTypedTarget{Key: key, Selector: entry.Candidate.Key, Go: &target}); err != nil {
				return repositoryTargetPlan{}, err
			}
		}
		for _, fileRef := range selectedFiles[repositoryTargetAdapterGo] {
			refs, resolveErr := discovery.goResolver.Resolve([]corpus.FileID{fileRef})
			if resolveErr != nil {
				return repositoryTargetPlan{}, fmt.Errorf("restore Go file_ref %q: %w", fileRef, resolveErr)
			}
			for _, ref := range refs {
				if err := addFile(repositoryTargetKey{Adapter: repositoryTargetAdapterGo, Ref: ref}, fileRef); err != nil {
					return repositoryTargetPlan{}, err
				}
			}
		}
	}

	if len(selectedFiles[repositoryTargetAdapterPython]) > 0 {
		pythonTargets, resolveErr := discovery.pythonResolver.Resolve(selectedFiles[repositoryTargetAdapterPython])
		if resolveErr != nil {
			return repositoryTargetPlan{}, fmt.Errorf("restore selected Python targets: %w", resolveErr)
		}
		for _, value := range pythonTargets {
			if !discovery.pythonCatalog.OwnsTarget(value) {
				return repositoryTargetPlan{}, fmt.Errorf("restored Python target %q is outside exact catalog", value.Ref)
			}
			target := value
			key := repositoryTargetKey{Adapter: repositoryTargetAdapterPython, Ref: target.Ref}
			if err := addTarget(repositoryTypedTarget{Key: key, Selector: target.Selector, Python: &target}); err != nil {
				return repositoryTargetPlan{}, err
			}
		}
		for _, fileRef := range selectedFiles[repositoryTargetAdapterPython] {
			targets, resolveErr := discovery.pythonResolver.Resolve([]corpus.FileID{fileRef})
			if resolveErr != nil {
				return repositoryTargetPlan{}, fmt.Errorf("restore Python file_ref %q: %w", fileRef, resolveErr)
			}
			for _, target := range targets {
				if err := addFile(repositoryTargetKey{Adapter: repositoryTargetAdapterPython, Ref: target.Ref}, fileRef); err != nil {
					return repositoryTargetPlan{}, err
				}
			}
		}
	}

	if len(selectedFiles[repositoryTargetAdapterJSTS]) > 0 {
		for _, fileRef := range selectedFiles[repositoryTargetAdapterJSTS] {
			value, ok := discovery.jstsByManifest[fileRef]
			if !ok {
				return repositoryTargetPlan{}, fmt.Errorf("restored JavaScript/TypeScript file_ref %q is not an exact package target manifest", fileRef)
			}
			target := value
			key := repositoryTargetKey{Adapter: repositoryTargetAdapterJSTS, Ref: target.Ref}
			if err := addTarget(repositoryTypedTarget{Key: key, Selector: target.Selector, JSTS: &target}); err != nil {
				return repositoryTargetPlan{}, err
			}
			if err := addFile(key, fileRef); err != nil {
				return repositoryTargetPlan{}, err
			}
		}
	}

	var defaultKey repositoryTargetKey
	switch defaultAdapter {
	case repositoryTargetAdapterGo:
		ref, resolveErr := discovery.goResolver.ResolveOne(portfolio.Default.FileRef)
		if resolveErr != nil {
			return repositoryTargetPlan{}, fmt.Errorf("restore default Go target: %w", resolveErr)
		}
		defaultKey = repositoryTargetKey{Adapter: repositoryTargetAdapterGo, Ref: ref}
	case repositoryTargetAdapterPython:
		target, resolveErr := discovery.pythonResolver.ResolveOne(portfolio.Default.FileRef)
		if resolveErr != nil {
			return repositoryTargetPlan{}, fmt.Errorf(
				"restore one default Python target view: %w; use one exact Python selector with --target",
				resolveErr,
			)
		}
		defaultKey = repositoryTargetKey{Adapter: repositoryTargetAdapterPython, Ref: target.Ref}
	case repositoryTargetAdapterJSTS:
		target, ok := discovery.jstsByManifest[portfolio.Default.FileRef]
		if !ok {
			return repositoryTargetPlan{}, fmt.Errorf("restore default JavaScript/TypeScript target: package manifest is outside exact scout authority")
		}
		defaultKey = repositoryTargetKey{Adapter: repositoryTargetAdapterJSTS, Ref: target.Ref}
	default:
		return repositoryTargetPlan{}, fmt.Errorf("repository target portfolio has no supported default adapter")
	}
	if _, ok := builders[defaultKey]; !ok {
		return repositoryTargetPlan{}, fmt.Errorf("repository target portfolio default %q is outside restored selected targets", defaultKey)
	}

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
	plan := repositoryTargetPlan{
		Targets: append([]repositoryTypedTarget(nil), targets...),
		Default: defaultKey, Explicit: explicit, Outcome: outcome,
	}
	if discovery.goSource != nil {
		owned, err := snapshot.OwnSnapshot(*discovery.goSource)
		if err != nil {
			return repositoryTargetPlan{}, fmt.Errorf("own repository plan Go source: %w", err)
		}
		plan.GoSource = &owned
	}
	if discovery.pythonCatalog != nil {
		catalog := discovery.pythonCatalog.Snapshot()
		plan.PythonCatalog = &catalog
	}
	if err := plan.Validate(); err != nil {
		return repositoryTargetPlan{}, err
	}
	return plan, nil
}

func repositoryTargetChoiceGroups(
	discovery repositoryTargetDiscovery,
) ([]targetPortfolioChoiceGroup, error) {
	groups := make([]targetPortfolioChoiceGroup, 0, 3)
	if discovery.goSource != nil && len(discovery.goSource.TargetCatalog.Entries) > 0 {
		groups = append(groups, targetPortfolioChoiceGroup{
			Language: "Go", Choices: targetPortfolioChoices(*discovery.goSource.TargetCatalog),
		})
	}
	if discovery.pythonCatalog != nil {
		choices, err := pythonExactTargetChoices(*discovery.pythonCatalog, discovery.pythonResolver)
		if err != nil {
			return nil, err
		}
		groups = append(groups, targetPortfolioChoiceGroup{Language: "Python", Choices: choices})
	}
	if len(discovery.jstsTargets) > 0 {
		choices := make([]string, len(discovery.jstsTargets))
		for index, target := range discovery.jstsTargets {
			choices[index] = target.Selector + " (" + target.ManifestPath + ")"
		}
		groups = append(groups, targetPortfolioChoiceGroup{
			Language: "JavaScript/TypeScript",
			Choices:  strings.Join(choices, ", "),
		})
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("no exact repository target choices were discovered")
	}
	return groups, nil
}

func repositoryTypedTargetLess(left, right repositoryTypedTarget) bool {
	leftRank := repositoryTargetAdapterRank(left.Key.Adapter)
	rightRank := repositoryTargetAdapterRank(right.Key.Adapter)
	if leftRank != rightRank {
		return leftRank < rightRank
	}
	if left.Selector != right.Selector {
		return left.Selector < right.Selector
	}
	return left.Key.Ref < right.Key.Ref
}

func repositoryTargetAdapterRank(adapter repositoryTargetAdapter) int {
	switch adapter {
	case repositoryTargetAdapterGo:
		return 0
	case repositoryTargetAdapterPython:
		return 1
	case repositoryTargetAdapterJSTS:
		return 2
	default:
		return 3
	}
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
