package cubemap

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/activitysurface"
	"github.com/dvordrova/repomap/internal/coremap"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/entrycall"
	"github.com/dvordrova/repomap/internal/gocoreobject"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

const (
	maxEntrypointCandidates       = 512
	maxIntegrationUsageCandidates = 1024

	maxRequestBytes  = 1024 * 1024
	maxResponseBytes = 128 * 1024
	maxOutputTokens  = 4096
)

//go:embed prompts/entrypoints.md
var entrypointPrompt string

//go:embed prompts/integration-symbols.md
var integrationSymbolsPrompt string

type entrypointWireRow struct {
	Ref               string   `json:"ref"`
	Path              string   `json:"path"`
	Line              int      `json:"line"`
	Package           string   `json:"package"`
	Symbol            string   `json:"symbol"`
	Incoming          int      `json:"incoming"`
	Outgoing          int      `json:"outgoing"`
	GoroutineOutgoing int      `json:"goroutine_outgoing"`
	ActivitySurfaces  int      `json:"accepted_activity_surfaces"`
	Signals           []string `json:"signals"`
}

type entrypointRequest struct {
	Observed int                 `json:"observed"`
	Omitted  int                 `json:"omitted"`
	Catalog  []entrypointWireRow `json:"catalog"`
}

type entrypointResponse struct {
	EntrypointRefs []string `json:"entrypoint_refs"`
}

type usageWireDependency struct {
	Ref         string               `json:"ref"`
	Kind        dependencies.Kind    `json:"kind"`
	Name        string               `json:"name"`
	PackagePath string               `json:"package_path"`
	Operations  []usageWireOperation `json:"operations"`
}

type usageWireOperation struct {
	Ref              string                                `json:"ref"`
	Receiver         string                                `json:"receiver,omitempty"`
	Name             string                                `json:"name"`
	Dispatch         surfacediscovery.ExternalCallDispatch `json:"dispatch"`
	Invocation       surfacediscovery.DirectCallInvocation `json:"invocation"`
	WitnessCount     int                                   `json:"witness_count"`
	Callsites        []Location                            `json:"callsites"`
	CallsitesOmitted int                                   `json:"callsites_omitted"`
	ActivitySurfaces []usageWireSurfaceContext             `json:"activity_surfaces"`
}

type usageWireSurfaceContext struct {
	Kind         string   `json:"kind"`
	Role         string   `json:"role"`
	Registration Location `json:"registration"`
	Identity     string   `json:"identity,omitempty"`
	Method       string   `json:"method,omitempty"`
	Path         string   `json:"path,omitempty"`
}

type integrationUsageWireRow struct {
	Ref          string                `json:"ref"`
	Path         string                `json:"path"`
	Line         int                   `json:"line"`
	Package      string                `json:"package"`
	Symbol       string                `json:"symbol"`
	Dependencies []usageWireDependency `json:"dependencies"`
}

type integrationUsageRequest struct {
	Observed int                       `json:"observed"`
	Omitted  int                       `json:"omitted"`
	Catalog  []integrationUsageWireRow `json:"catalog"`
}

type integrationUsageResponse struct {
	IntegrationOperationRefs []string `json:"integration_operation_refs"`
}

type entrypointCandidate struct {
	ref              string
	node             surfacediscovery.DirectCallNode
	incoming         int
	outgoing         int
	goCalls          int
	activitySurfaces int
	signals          []string
	rank             int
}

type dependencyCandidate struct {
	ref       string
	value     dependencies.Dependency
	importers []dependencies.Importer
}

func run(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	coreCompilation coremap.Compilation,
	coreObjectIndex gocoreobject.Index,
	index surfacediscovery.DirectCallIndex,
	externalIndex surfacediscovery.ExternalCallIndex,
	activitySubstrate entrycall.Substrate,
	catalog dependencies.Catalog,
) (Map, error) {
	if err := index.Validate(); err != nil {
		return Map{}, fmt.Errorf("cubemap: direct-call index: %w", err)
	}
	if index.State != surfacediscovery.DirectCallIndexReady {
		return Map{}, fmt.Errorf("cubemap: direct-call authority is unavailable")
	}
	if len(index.Nodes) > maxEntrypointCandidates {
		return Map{}, fmt.Errorf(
			"cubemap: %d entrypoint candidates exceed the complete request bound %d; choose a narrower target with --target instead of publishing a truncated entrypoint map",
			len(index.Nodes), maxEntrypointCandidates,
		)
	}
	index = index.Snapshot()
	if err := catalog.Validate(); err != nil {
		return Map{}, fmt.Errorf("cubemap: dependency catalog: %w", err)
	}
	if catalog.Coverage.State != dependencies.CoverageComplete {
		return Map{}, fmt.Errorf(
			"cubemap: dependency authority is incomplete: %s",
			dependencyOmissionSummary(catalog.Coverage.Omissions),
		)
	}
	if err := externalIndex.Validate(); err != nil {
		return Map{}, fmt.Errorf("cubemap: external-call index: %w", err)
	}
	if !sameScenario(index.Scenario, externalIndex.Scenario) {
		return Map{}, fmt.Errorf("cubemap: call indexes describe different build scenarios")
	}
	core, err := coremap.Run(ctx, executor, provider, coreCompilation)
	if err != nil {
		return Map{}, fmt.Errorf("cubemap: core-map cube: %w", err)
	}
	coreObjects, err := compileCoreObjectProjection(core, coreObjectIndex)
	if err != nil {
		return Map{}, fmt.Errorf("cubemap: core-object projection: %w", err)
	}
	catalogJSON, err := json.Marshal(catalog)
	if err != nil {
		return Map{}, fmt.Errorf("cubemap: dependency identity: %w", err)
	}
	activities, err := activitysurface.Run(
		ctx, executorForStage(executor, StageActivitySurfaces), provider, activitySubstrate,
	)
	if err != nil {
		return Map{}, fmt.Errorf("cubemap: activity-surface cube: %w", err)
	}
	if activities.State == entrycall.StateUnavailable &&
		activities.ClosedReason != entrycall.ClosedNoEntrypoints {
		return Map{}, fmt.Errorf(
			"cubemap: activity-surface authority is unavailable (%s)",
			activities.ClosedReason,
		)
	}
	result := Map{
		Version:                 Version,
		SourceIndexSHA256:       index.SHA256,
		ExternalCallIndexSHA256: externalIndex.SHA256,
		DependencyCatalogSHA256: sha256Hex(catalogJSON),
		Core:                    core,
		CoreObjects:             coreObjects,
		ActivitySurfaces:        activities,
		Entrypoints:             []Symbol{},
		IntegrationDependencies: []IntegrationDependency{},
		IntegrationSymbols:      []IntegrationSymbol{},
		Paths:                   []Path{},
	}
	result.Coverage.DependencyCatalog = dependencyCatalogCoverage(catalog.Coverage)
	result.Coverage.ExternalCallFamiliesObserved = len(externalIndex.Families)
	result.Coverage.ExternalCalls = externalIndex.Coverage
	finish := func() (Map, error) {
		bindingCompilation, compileErr := compileSurfaceCoreEffectBinder(
			result.Core, result.CoreObjects, result.ActivitySurfaces, result.IntegrationDependencies,
			result.IntegrationSymbols, index,
		)
		if compileErr != nil {
			return Map{}, compileErr
		}
		bindings, bindingErr := runSurfaceCoreEffectBinder(ctx, executor, provider, bindingCompilation)
		if bindingErr != nil {
			return Map{}, bindingErr
		}
		result.SurfaceCoreEffects = &bindings
		if validateErr := Validate(result); validateErr != nil {
			return Map{}, validateErr
		}
		return result, nil
	}

	activityRoots, err := exactActivityRoots(index, activities)
	if err != nil {
		return Map{}, err
	}
	entryCandidates := buildEntrypointCandidates(index, activityRoots)
	result.Coverage.Entrypoints = candidateCoverage(len(index.Nodes), len(entryCandidates), len(entryCandidates) > 0)
	if len(entryCandidates) > 0 {
		selected, selectErr := selectEntrypoints(ctx, executor, provider, entryCandidates)
		if selectErr != nil {
			return Map{}, selectErr
		}
		for _, candidate := range selected {
			result.Entrypoints = append(result.Entrypoints, symbolFromNode(candidate.node))
		}
	}
	sort.Slice(result.Entrypoints, func(i, j int) bool { return symbolLess(result.Entrypoints[i], result.Entrypoints[j]) })

	classifiedDependencies, selectErr := integrationdependency.Run(
		ctx, executorForStage(executor, StageIntegrationDependencies), provider, catalog,
	)
	if selectErr != nil {
		return Map{}, fmt.Errorf("cubemap: integration dependency cube: %w", selectErr)
	}
	result.Coverage.IntegrationDependencies = CandidateCoverage{
		Observed:    classifiedDependencies.Coverage.Observed,
		Advertised:  classifiedDependencies.Coverage.Advertised,
		Omitted:     classifiedDependencies.Coverage.Omitted,
		ModelCalled: classifiedDependencies.Coverage.ModelCalled,
	}
	selectedDependencies := dependencyCandidatesFromClassification(classifiedDependencies)
	for _, candidate := range selectedDependencies {
		result.IntegrationDependencies = append(result.IntegrationDependencies, restoreDependency(candidate))
	}
	sort.Slice(result.IntegrationDependencies, func(i, j int) bool {
		return integrationDependencyLess(result.IntegrationDependencies[i], result.IntegrationDependencies[j])
	})
	if len(selectedDependencies) == 0 {
		return finish()
	}

	usageCandidates, matchedFamilies, usageBuildErr := discoverExternalUsages(index, externalIndex, selectedDependencies)
	if usageBuildErr != nil {
		return Map{}, usageBuildErr
	}
	result.Coverage.ExternalCallFamiliesMatched = matchedFamilies
	if len(usageCandidates) > maxIntegrationUsageCandidates {
		return Map{}, fmt.Errorf(
			"cubemap: %d integration-usage candidates exceed the complete request bound %d; choose a narrower target with --target instead of publishing a truncated integration map",
			len(usageCandidates), maxIntegrationUsageCandidates,
		)
	}
	result.Coverage.IntegrationSymbols = candidateCoverage(len(usageCandidates), len(usageCandidates), len(usageCandidates) > 0)
	if len(usageCandidates) > 0 {
		selectedSymbols, usageErr := selectIntegrationSymbols(
			ctx, executor, provider, usageCandidates, selectedDependencies, activities,
		)
		if usageErr != nil {
			return Map{}, usageErr
		}
		for _, candidate := range selectedSymbols {
			result.IntegrationSymbols = append(result.IntegrationSymbols, restoreIntegrationSymbol(candidate, selectedDependencies))
		}
		sort.Slice(result.IntegrationSymbols, func(i, j int) bool {
			return integrationSymbolLess(result.IntegrationSymbols[i], result.IntegrationSymbols[j])
		})
	}
	result.Paths = buildShortestPaths(index, result.Entrypoints, result.IntegrationSymbols)
	result.Coverage.UnconnectedSymbols = len(result.IntegrationSymbols) - len(result.Paths)
	return finish()
}

func dependencyCatalogCoverage(value dependencies.Coverage) DependencyCatalogCoverage {
	counts := make(map[dependencies.OmissionReason]int)
	for _, omission := range value.Omissions {
		counts[omission.Reason]++
	}
	reasons := make([]DependencyOmissionCount, 0, len(counts))
	for reason, count := range counts {
		reasons = append(reasons, DependencyOmissionCount{Reason: reason, Count: count})
	}
	sort.Slice(reasons, func(i, j int) bool { return reasons[i].Reason < reasons[j].Reason })
	return DependencyCatalogCoverage{
		State: value.State, ImportsObserved: value.ImportsObserved, ImportsRetained: value.ImportsRetained,
		Omissions: len(value.Omissions), Reasons: reasons,
	}
}

func dependencyOmissionSummary(omissions []dependencies.Omission) string {
	if len(omissions) == 0 {
		return "partial coverage has no classified omission"
	}
	first := omissions[0]
	summary := fmt.Sprintf("%s for %s", first.Reason, first.PackagePath)
	if len(omissions) > 1 {
		summary += fmt.Sprintf(" and %d more", len(omissions)-1)
	}
	return summary
}

func exactActivityRoots(
	index surfacediscovery.DirectCallIndex,
	activities activitysurface.Result,
) (map[string]int, error) {
	result := make(map[string]int)
	for _, surface := range activities.Surfaces {
		if _, exists := index.Node(surface.RootNodeID); !exists {
			return nil, fmt.Errorf("cubemap: activity surface %q has no exact graph root", surface.ID)
		}
		result[surface.RootNodeID]++
	}
	return result, nil
}

func buildEntrypointCandidates(
	index surfacediscovery.DirectCallIndex,
	activityRoots map[string]int,
) []entrypointCandidate {
	values := make([]entrypointCandidate, 0, len(index.Nodes))
	targetPackages := make(map[string]struct{}, len(index.Scope.TargetPackages))
	for _, packagePath := range index.Scope.TargetPackages {
		targetPackages[packagePath] = struct{}{}
	}
	for _, node := range index.Nodes {
		incoming := index.Incoming(node.ID)
		outgoing := index.Outgoing(node.ID)
		candidate := entrypointCandidate{
			node: node, incoming: len(incoming), outgoing: len(outgoing),
			activitySurfaces: activityRoots[node.ID],
		}
		if candidate.activitySurfaces > 0 {
			candidate.signals = append(candidate.signals, "accepted_activity_surface")
			candidate.rank += 1024
		}
		if _, ok := targetPackages[node.Package]; ok {
			candidate.signals = append(candidate.signals, "target_package")
			candidate.rank += 32
		}
		if len(incoming) == 0 {
			candidate.signals = append(candidate.signals, "no_incoming_exact_edge")
			candidate.rank += 16
		}
		if len(outgoing) > 0 {
			candidate.signals = append(candidate.signals, "calls_repository_symbol")
			candidate.rank += 4
		}
		for _, edge := range outgoing {
			if edge.Invocation == surfacediscovery.DirectCallGoroutine {
				candidate.goCalls++
			}
		}
		if candidate.goCalls > 0 {
			candidate.signals = append(candidate.signals, "starts_goroutine")
			candidate.rank += 24
		}
		values = append(values, candidate)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].rank != values[j].rank {
			return values[i].rank > values[j].rank
		}
		return symbolKey(symbolFromNode(values[i].node)) < symbolKey(symbolFromNode(values[j].node))
	})
	for position := range values {
		values[position].ref = fmt.Sprintf("e%d", position+1)
	}
	return values
}

func selectEntrypoints(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	candidates []entrypointCandidate,
) ([]entrypointCandidate, error) {
	rows := make([]entrypointWireRow, 0, len(candidates))
	allowed := make(map[string]entrypointCandidate, len(candidates))
	for _, candidate := range candidates {
		row := entrypointWireRow{
			Ref: candidate.ref, Path: candidate.node.Declaration.Path, Line: candidate.node.Declaration.Line,
			Package: candidate.node.Package, Symbol: candidate.node.Symbol.Name,
			Incoming: candidate.incoming, Outgoing: candidate.outgoing,
			GoroutineOutgoing: candidate.goCalls, ActivitySurfaces: candidate.activitySurfaces,
			Signals: append([]string(nil), candidate.signals...),
		}
		rows = append(rows, row)
		allowed[candidate.ref] = candidate
	}
	payload := entrypointRequest{Observed: len(rows), Omitted: 0, Catalog: rows}
	user, err := promptPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("cubemap: entrypoint request: %w", err)
	}
	authority := make([]struct {
		Ref    string `json:"ref"`
		NodeID string `json:"node_id"`
	}, 0, len(candidates))
	for _, candidate := range candidates {
		authority = append(authority, struct {
			Ref    string `json:"ref"`
			NodeID string `json:"node_id"`
		}{Ref: candidate.ref, NodeID: candidate.node.ID})
	}
	state, err := cubeState("entrypoints", entrypointPrompt, authority)
	if err != nil {
		return nil, fmt.Errorf("cubemap: entrypoint state: %w", err)
	}
	var response entrypointResponse
	outcome, err := llm.ExecuteJSON(ctx, executorForStage(executor, StageEntrypoints), provider, llm.Call[entrypointResponse]{
		State:  state,
		Prompt: llm.Prompt{System: strings.TrimSpace(entrypointPrompt), User: user, ResponseFormatJSON: true},
		Limits: cubeLimits(),
		Validate: func(value entrypointResponse) error {
			return validateSelectedRefs(value.EntrypointRefs, allowedKeys(allowed), 0, 32)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("cubemap: entrypoint cube: %w", err)
	}
	response = outcome.Value
	selectedRefs := stringSet(response.EntrypointRefs)
	selected := make([]entrypointCandidate, 0, len(selectedRefs))
	for _, candidate := range candidates {
		if _, ok := selectedRefs[candidate.ref]; ok {
			selected = append(selected, candidate)
		}
	}
	return selected, nil
}

func dependencyCandidatesFromClassification(result integrationdependency.Result) []dependencyCandidate {
	values := make([]dependencyCandidate, len(result.Dependencies))
	for position, selected := range result.Dependencies {
		values[position] = dependencyCandidate{
			ref:       fmt.Sprintf("d%d", position+1),
			value:     selected.Dependency,
			importers: append([]dependencies.Importer(nil), selected.Importers...),
		}
	}
	return values
}

func selectIntegrationSymbols(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	candidates []usageCandidate,
	dependencies []dependencyCandidate,
	activities activitysurface.Result,
) ([]usageCandidate, error) {
	dependencyByRef := make(map[string]dependencyCandidate, len(dependencies))
	for _, candidate := range dependencies {
		dependencyByRef[candidate.ref] = candidate
	}
	rows := make([]integrationUsageWireRow, 0, len(candidates))
	type operationAuthority struct {
		CallerRef     string
		DependencyRef string
		FamilyID      string
	}
	allowedOperations := make(map[string]operationAuthority)
	operationOrdinal := 0
	for position := range candidates {
		candidate := candidates[position]
		candidate.ref = fmt.Sprintf("u%d", position+1)
		candidates[position].ref = candidate.ref
		row := integrationUsageWireRow{
			Ref: candidate.ref, Path: candidate.node.Declaration.Path, Line: candidate.node.Declaration.Line,
			Package: candidate.node.Package, Symbol: candidate.node.Symbol.Name,
		}
		for _, dependencyRef := range candidate.dependencyRefs {
			dependency := dependencyByRef[dependencyRef]
			wireDependency := usageWireDependency{
				Ref: dependencyRef, Kind: dependency.value.Kind,
				Name: dependency.value.Name, PackagePath: dependency.value.PackagePath,
				Operations: []usageWireOperation{},
			}
			for _, family := range candidate.familiesByDependency[dependencyRef] {
				operationOrdinal++
				operationRef := fmt.Sprintf("o%d", operationOrdinal)
				operation := usageWireOperation{
					Ref:      operationRef,
					Receiver: family.Target.Receiver, Name: family.Target.Name,
					Dispatch: family.Dispatch, Invocation: family.Invocation, WitnessCount: family.WitnessCount,
					CallsitesOmitted: family.CallsitesOmitted,
				}
				for _, callsite := range family.Callsites {
					operation.Callsites = append(operation.Callsites, locationFromSurface(callsite))
				}
				operation.ActivitySurfaces = activityContextsForCallsites(activities, family.Callsites)
				wireDependency.Operations = append(wireDependency.Operations, operation)
				allowedOperations[operationRef] = operationAuthority{
					CallerRef: candidate.ref, DependencyRef: dependencyRef, FamilyID: family.ID,
				}
			}
			row.Dependencies = append(row.Dependencies, wireDependency)
		}
		rows = append(rows, row)
	}
	payload := integrationUsageRequest{Observed: len(rows), Omitted: 0, Catalog: rows}
	user, err := promptPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("cubemap: integration symbol request: %w", err)
	}
	type usageAuthorityRow struct {
		Ref            string `json:"ref"`
		CallerRef      string `json:"caller_ref"`
		NodeID         string `json:"node_id"`
		DependencyID   string `json:"dependency_id"`
		ExternalCallID string `json:"external_call_family_id"`
	}
	authority := make([]usageAuthorityRow, 0, len(allowedOperations))
	operationRefs := make([]string, 0, len(allowedOperations))
	for ref := range allowedOperations {
		operationRefs = append(operationRefs, ref)
	}
	sort.Slice(operationRefs, func(i, j int) bool {
		return numericRefLess(operationRefs[i], operationRefs[j])
	})
	callerByRef := make(map[string]usageCandidate, len(candidates))
	for _, candidate := range candidates {
		callerByRef[candidate.ref] = candidate
	}
	for _, operationRef := range operationRefs {
		exact := allowedOperations[operationRef]
		authority = append(authority, usageAuthorityRow{
			Ref: operationRef, CallerRef: exact.CallerRef,
			NodeID:         callerByRef[exact.CallerRef].node.ID,
			DependencyID:   dependencyByRef[exact.DependencyRef].value.ID,
			ExternalCallID: exact.FamilyID,
		})
	}
	activityWire, err := json.Marshal(activities)
	if err != nil {
		return nil, fmt.Errorf("cubemap: integration symbol activity authority: %w", err)
	}
	state, err := cubeState("integration-symbols", integrationSymbolsPrompt, struct {
		Operations             []usageAuthorityRow `json:"operations"`
		ActivitySurfacesSHA256 string              `json:"activity_surfaces_sha256"`
	}{Operations: authority, ActivitySurfacesSHA256: sha256Hex(activityWire)})
	if err != nil {
		return nil, fmt.Errorf("cubemap: integration symbol state: %w", err)
	}
	outcome, err := llm.ExecuteJSON(ctx, executorForStage(executor, StageIntegrationSymbols), provider, llm.Call[integrationUsageResponse]{
		State:  state,
		Prompt: llm.Prompt{System: strings.TrimSpace(integrationSymbolsPrompt), User: user, ResponseFormatJSON: true},
		Limits: cubeLimits(),
		Validate: func(value integrationUsageResponse) error {
			return validateSelectedRefs(value.IntegrationOperationRefs, allowedKeys(allowedOperations), 0, 256)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("cubemap: integration symbol cube: %w", err)
	}
	selectedRefs := stringSet(outcome.Value.IntegrationOperationRefs)
	selectedFamilyIDs := make(map[string]struct{}, len(selectedRefs))
	for ref := range selectedRefs {
		selectedFamilyIDs[allowedOperations[ref].FamilyID] = struct{}{}
	}
	selected := make([]usageCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		filtered := usageCandidate{
			ref: candidate.ref, node: candidate.node,
			familiesByDependency: make(map[string][]surfacediscovery.ExternalCallFamily),
		}
		for _, dependencyRef := range candidate.dependencyRefs {
			for _, family := range candidate.familiesByDependency[dependencyRef] {
				if _, ok := selectedFamilyIDs[family.ID]; !ok {
					continue
				}
				filtered.familiesByDependency[dependencyRef] = append(
					filtered.familiesByDependency[dependencyRef], family,
				)
			}
			if len(filtered.familiesByDependency[dependencyRef]) > 0 {
				filtered.dependencyRefs = append(filtered.dependencyRefs, dependencyRef)
			}
		}
		if len(filtered.dependencyRefs) > 0 {
			selected = append(selected, filtered)
		}
	}
	return selected, nil
}

func activityContextsForCallsites(
	activities activitysurface.Result,
	callsites []surfacediscovery.Location,
) []usageWireSurfaceContext {
	contexts := make([]usageWireSurfaceContext, 0)
	for _, surface := range activities.Surfaces {
		matched := false
		for _, callsite := range callsites {
			if callsite.Path == surface.Registration.Path &&
				callsite.Line == surface.Registration.Line &&
				callsite.Column == surface.Registration.Column {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		contexts = append(contexts, usageWireSurfaceContext{
			Kind: surface.Kind, Role: surface.Role,
			Registration: locationFromEntryCall(surface.Registration),
			Identity:     surfaceValueText(surface.Identity), Method: surfaceValueText(surface.Method),
			Path: surfaceValueText(surface.Path),
		})
	}
	return contexts
}

func numericRefLess(left, right string) bool {
	var leftNumber, rightNumber int
	_, leftErr := fmt.Sscanf(left, "o%d", &leftNumber)
	_, rightErr := fmt.Sscanf(right, "o%d", &rightNumber)
	if leftErr == nil && rightErr == nil && leftNumber != rightNumber {
		return leftNumber < rightNumber
	}
	return left < right
}

func restoreDependency(candidate dependencyCandidate) IntegrationDependency {
	result := IntegrationDependency{
		ID: candidate.value.ID, Kind: candidate.value.Kind, Name: candidate.value.Name, ModulePath: candidate.value.ModulePath,
		ModuleVersion: candidate.value.ModuleVersion, PackagePath: candidate.value.PackagePath,
		Importers: make([]Importer, 0, len(candidate.importers)),
	}
	for _, importer := range candidate.importers {
		result.Importers = append(result.Importers, Importer{
			PackagePath: importer.PackagePath, RepositoryPath: importer.RepositoryPath,
		})
	}
	sort.Slice(result.Importers, func(i, j int) bool { return importerKey(result.Importers[i]) < importerKey(result.Importers[j]) })
	return result
}

func restoreIntegrationSymbol(candidate usageCandidate, selectedDependencies []dependencyCandidate) IntegrationSymbol {
	dependencyIDs := make([]string, 0, len(candidate.dependencyRefs))
	operations := make([]IntegrationOperation, 0)
	byRef := make(map[string]dependencies.Dependency, len(selectedDependencies))
	for _, dependency := range selectedDependencies {
		byRef[dependency.ref] = dependency.value
	}
	for _, ref := range candidate.dependencyRefs {
		dependency := byRef[ref]
		dependencyIDs = append(dependencyIDs, dependency.ID)
		for _, family := range candidate.familiesByDependency[ref] {
			operation := IntegrationOperation{
				ExternalCallFamilyID: family.ID, DependencyID: dependency.ID,
				PackagePath: family.Target.PackagePath, Receiver: family.Target.Receiver,
				Name: family.Target.Name, Dispatch: family.Dispatch, Invocation: family.Invocation,
				WitnessCount: family.WitnessCount, CallsitesOmitted: family.CallsitesOmitted,
			}
			for _, callsite := range family.Callsites {
				operation.Callsites = append(operation.Callsites, locationFromSurface(callsite))
			}
			operations = append(operations, operation)
		}
	}
	sort.Strings(dependencyIDs)
	dependencyIDs = compactStrings(dependencyIDs)
	sort.Slice(operations, func(i, j int) bool {
		return integrationOperationKey(operations[i]) < integrationOperationKey(operations[j])
	})
	return IntegrationSymbol{Symbol: symbolFromNode(candidate.node), DependencyIDs: dependencyIDs, Operations: operations}
}

func symbolFromNode(node surfacediscovery.DirectCallNode) Symbol {
	return Symbol{
		NodeID: node.ID, Package: node.Package, Name: node.Symbol.Name,
		Path: node.Declaration.Path, Line: node.Declaration.Line, Column: node.Declaration.Column,
	}
}

func candidateCoverage(observed, advertised int, called bool) CandidateCoverage {
	return CandidateCoverage{Observed: observed, Advertised: advertised, Omitted: observed - advertised, ModelCalled: called}
}

func cubeLimits() llm.Limits {
	return llm.Limits{MaxRequestBytes: maxRequestBytes, MaxResponseBytes: maxResponseBytes, MaxOutputTokens: maxOutputTokens}
}

func cubeState(name, prompt string, authority any) ([]byte, error) {
	authorityJSON, err := json.Marshal(authority)
	if err != nil {
		return nil, err
	}
	state, err := json.Marshal(struct {
		Contract        string `json:"contract"`
		Preparation     int    `json:"preparation"`
		ResponseSchema  int    `json:"response_schema"`
		Cube            string `json:"cube"`
		PromptSHA256    string `json:"prompt_sha256"`
		AuthoritySHA256 string `json:"authority_sha256"`
	}{
		Contract: "repomap.cubemap.v1", Preparation: 1, ResponseSchema: 1, Cube: name,
		PromptSHA256: sha256Hex([]byte(strings.TrimSpace(prompt))), AuthoritySHA256: sha256Hex(authorityJSON),
	})
	if err != nil {
		return nil, err
	}
	return state, nil
}

func promptPayload(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func validateSelectedRefs(refs []string, allowed map[string]struct{}, minimum, maximum int) error {
	if refs == nil {
		return fmt.Errorf("cubemap: selected refs must be a JSON array")
	}
	if len(refs) < minimum || len(refs) > maximum {
		return fmt.Errorf("cubemap: selected ref count is outside bounds")
	}
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if _, exists := allowed[ref]; !exists {
			return fmt.Errorf("cubemap: unknown request-local ref %q", ref)
		}
		if _, exists := seen[ref]; exists {
			return fmt.Errorf("cubemap: duplicate request-local ref %q", ref)
		}
		seen[ref] = struct{}{}
	}
	return nil
}

func allowedKeys[T any](values map[string]T) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for key := range values {
		result[key] = struct{}{}
	}
	return result
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func compactStrings(values []string) []string {
	write := 0
	for _, value := range values {
		if write > 0 && values[write-1] == value {
			continue
		}
		values[write] = value
		write++
	}
	return values[:write]
}

func sha256Hex(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}
