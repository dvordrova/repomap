// Package activitypath deterministically connects model-selected activity
// starts to exact callers of selected integration uses through one sealed,
// language-neutral ProgramIndex. It never invokes a model or invents a graph
// edge when ProgramIndex authority is unresolved.
package activitypath

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/activityentrypoint"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/integrationusage"
	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	Version          = 2
	ArtifactFilename = "activity-paths.json"

	MaxRoutes                  = integrationusage.MaxSelectedUses
	MaxOutcomes                = integrationusage.MaxSelectedUses
	MaxProjectedTraversalEdges = 1_048_576
	MaxRouteSteps              = 256
	MaxTotalPathSteps          = 32_768
)

// Status is the complete closed outcome for one selected integration caller.
type Status string

const (
	// StatusExact has a zero-hop activity/caller identity or a path composed
	// entirely of exact calls and executes relations.
	StatusExact Status = "exact"
	// StatusPossible has the best retained path, but at least one alternatives
	// relation or callback handoff prevents exact runtime authority.
	StatusPossible Status = "possible"
	// StatusFrontier has no retained path and reports either global ProgramIndex
	// omissions or an exact non-traversable relation endpoint joining the
	// selected activity region to this caller. It never transfers a boundary
	// reached elsewhere onto an unrelated caller.
	StatusFrontier Status = "frontier"
	// StatusUnconnected has no retained path or relevant authority boundary. It
	// never claims that a runtime path is impossible.
	StatusUnconnected Status = "unconnected"
)

func (status Status) Valid() bool {
	return status == StatusExact || status == StatusPossible || status == StatusFrontier || status == StatusUnconnected
}

// EdgeAuthority separates exact local execution edges from retained possible
// dispatch or callback handoffs.
type EdgeAuthority string

const (
	EdgeExact    EdgeAuthority = "exact"
	EdgePossible EdgeAuthority = "possible"
)

func (authority EdgeAuthority) Valid() bool {
	return authority == EdgeExact || authority == EdgePossible
}

// FrontierReason records why absence of a retained route cannot establish a
// closed unconnected result.
type FrontierReason string

const (
	FrontierDecoratorBoundary FrontierReason = "decorator_boundary"
	FrontierObjectsOmitted    FrontierReason = "program_objects_omitted"
	FrontierRelationsOmitted  FrontierReason = "program_relations_omitted"
)

func (reason FrontierReason) Valid() bool {
	switch reason {
	case FrontierDecoratorBoundary, FrontierObjectsOmitted, FrontierRelationsOmitted:
		return true
	default:
		return false
	}
}

// Step is the exact bounded projection of one retained ProgramIndex relation
// target used by a route. Witness bodies and unrelated alternative targets are
// intentionally not copied; ValidateAgainst restores this projection from the
// exact relation and selected ToID.
type Step struct {
	RelationID        string                    `json:"relation_id"`
	FromID            string                    `json:"from_id"`
	ToID              string                    `json:"to_id"`
	Kind              programindex.RelationKind `json:"kind"`
	Resolution        programindex.Resolution   `json:"resolution"`
	Authority         EdgeAuthority             `json:"authority"`
	Invocation        string                    `json:"invocation,omitempty"`
	Location          *programindex.Location    `json:"location,omitempty"`
	TargetsObserved   int                       `json:"targets_observed"`
	TargetsOmitted    int                       `json:"targets_omitted"`
	WitnessesObserved int                       `json:"witnesses_observed"`
	WitnessesOmitted  int                       `json:"witnesses_omitted"`
}

// Route is one normalized path result for one unique integration caller.
// Nodes are ordered from Activity through Caller. Frontier and Unconnected
// routes retain the exact Caller but have no invented nodes or steps.
type Route struct {
	ID               string                `json:"id"`
	Caller           programindex.Object   `json:"caller"`
	Status           Status                `json:"status"`
	Activity         *programindex.Object  `json:"activity,omitempty"`
	Nodes            []programindex.Object `json:"nodes"`
	Steps            []Step                `json:"steps"`
	Distance         int                   `json:"distance"`
	PossibleSteps    int                   `json:"possible_steps"`
	CallbackHandoffs int                   `json:"callback_handoffs"`
	Frontier         []FrontierReason      `json:"frontier"`
}

// Outcome preserves only the exact operation identity needed to join one
// selected integration use to its caller-shared route. Labels, dependency
// metadata, callsites, and importer inventories remain in their bound upstream
// artifacts and are never repeated per use here.
type Outcome struct {
	ID               string `json:"id"`
	DependencyID     string `json:"dependency_id"`
	RelationID       string `json:"relation_id"`
	WitnessIndex     int    `json:"witness_index"`
	ExternalSymbolID string `json:"external_symbol_id"`
	RouteID          string `json:"route_id"`
}

// Coverage is a lossless execution and outcome ledger. GraphBuilt is false
// only for the contractual no-integration-use bypass.
type Coverage struct {
	ActivitiesSelected           int  `json:"activities_selected"`
	DependenciesSelected         int  `json:"dependencies_selected"`
	UsesObserved                 int  `json:"uses_observed"`
	UniqueCallers                int  `json:"unique_callers"`
	Routes                       int  `json:"routes"`
	Outcomes                     int  `json:"outcomes"`
	GraphBuilt                   bool `json:"graph_built"`
	RelationsExamined            int  `json:"relations_examined"`
	TraversableRelations         int  `json:"traversable_relations"`
	ProjectedTraversalEdges      int  `json:"projected_traversal_edges"`
	ExactEdges                   int  `json:"exact_edges"`
	PossibleEdges                int  `json:"possible_edges"`
	UnresolvedTraversalRelations int  `json:"unresolved_traversal_relations"`
	TraversalTargetsOmitted      int  `json:"traversal_targets_omitted"`
	DecoratorRelations           int  `json:"decorator_relations"`
	CallablesWithoutLocation     int  `json:"callables_without_location"`
	SeededModulesWithoutLocation int  `json:"seeded_modules_without_location"`
	ProgramObjectsOmitted        int  `json:"program_objects_omitted"`
	ProgramRelationsOmitted      int  `json:"program_relations_omitted"`
	ExactOutcomes                int  `json:"exact_outcomes"`
	PossibleOutcomes             int  `json:"possible_outcomes"`
	FrontierOutcomes             int  `json:"frontier_outcomes"`
	UnconnectedOutcomes          int  `json:"unconnected_outcomes"`
	TotalPathSteps               int  `json:"total_path_steps"`
}

// Result binds every deterministic route and outcome to all four exact input
// artifacts and seals its own canonical representation.
type Result struct {
	Version                       int       `json:"version"`
	ProgramIndexSHA256            string    `json:"program_index_sha256"`
	ActivityEntrypointsSHA256     string    `json:"activity_entrypoints_sha256"`
	IntegrationDependenciesSHA256 string    `json:"integration_dependencies_sha256"`
	IntegrationUsageSHA256        string    `json:"integration_usage_sha256"`
	Routes                        []Route   `json:"routes"`
	Outcomes                      []Outcome `json:"outcomes"`
	Coverage                      Coverage  `json:"coverage"`
	SHA256                        string    `json:"sha256"`
}

type inputs struct {
	index           programindex.Index
	activities      activityentrypoint.Result
	integrations    integrationdependency.Result
	uses            integrationusage.Result
	activitiesSHA   string
	integrationsSHA string
	usesSHA         string
	objects         map[string]programindex.Object
	dependencyIDs   map[string]struct{}
}

// Build validates and snapshots every input, projects the complete bounded
// traversal graph, and emits exactly one Outcome per selected integration Use.
func Build(
	index programindex.Index,
	activities activityentrypoint.Result,
	integrations integrationdependency.Result,
	uses integrationusage.Result,
) (Result, error) {
	prepared, err := prepareInputs(index, activities, integrations, uses)
	if err != nil {
		return Result{}, err
	}
	result, err := build(prepared)
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

func prepareInputs(
	index programindex.Index,
	activities activityentrypoint.Result,
	integrations integrationdependency.Result,
	uses integrationusage.Result,
) (inputs, error) {
	ownedIndex := index.Snapshot()
	if err := ownedIndex.Validate(); err != nil {
		return inputs{}, fmt.Errorf("activity path: ProgramIndex: %w", err)
	}
	ownedActivities := activities.Snapshot()
	if err := ownedActivities.ValidateAgainst(ownedIndex); err != nil {
		return inputs{}, fmt.Errorf("activity path: activity entrypoints: %w", err)
	}
	integrationBytes, err := integrationdependency.Encode(integrations)
	if err != nil {
		return inputs{}, fmt.Errorf("activity path: integration dependencies: %w", err)
	}
	ownedIntegrations, err := integrationdependency.Decode(integrationBytes)
	if err != nil {
		return inputs{}, fmt.Errorf("activity path: integration dependencies: %w", err)
	}
	usageBytes, err := integrationusage.Encode(uses)
	if err != nil {
		return inputs{}, fmt.Errorf("activity path: integration usage: %w", err)
	}
	ownedUses, err := integrationusage.Decode(usageBytes)
	if err != nil {
		return inputs{}, fmt.Errorf("activity path: integration usage: %w", err)
	}
	if err := ownedUses.ValidateAgainst(ownedIndex, ownedIntegrations); err != nil {
		return inputs{}, fmt.Errorf("activity path: integration usage authority: %w", err)
	}
	activitiesSHA, err := ownedActivities.ArtifactSHA256()
	if err != nil {
		return inputs{}, fmt.Errorf("activity path: activity entrypoint identity: %w", err)
	}
	integrationsSHA, err := ownedIntegrations.ArtifactSHA256()
	if err != nil {
		return inputs{}, fmt.Errorf("activity path: integration dependency identity: %w", err)
	}
	usesSHA, err := ownedUses.ArtifactSHA256()
	if err != nil {
		return inputs{}, fmt.Errorf("activity path: integration usage identity: %w", err)
	}
	if ownedUses.IntegrationDependenciesSHA256 != integrationsSHA {
		return inputs{}, fmt.Errorf("activity path: integration usage does not bind the supplied dependencies")
	}
	objects := make(map[string]programindex.Object, len(ownedIndex.Objects))
	for _, object := range ownedIndex.Objects {
		objects[object.ID] = cloneObject(object)
	}
	dependencyIDs := make(map[string]struct{}, len(ownedIntegrations.Dependencies))
	for _, selected := range ownedIntegrations.Dependencies {
		dependencyIDs[selected.Dependency.ID] = struct{}{}
	}
	if len(ownedUses.Uses) > MaxOutcomes {
		return inputs{}, fmt.Errorf("activity path: %d uses exceed outcome bound %d", len(ownedUses.Uses), MaxOutcomes)
	}
	return inputs{
		index: ownedIndex, activities: ownedActivities, integrations: ownedIntegrations, uses: ownedUses,
		activitiesSHA: activitiesSHA, integrationsSHA: integrationsSHA, usesSHA: usesSHA,
		objects: objects, dependencyIDs: dependencyIDs,
	}, nil
}

func build(prepared inputs) (Result, error) {
	result := Result{
		Version: Version, ProgramIndexSHA256: prepared.index.SHA256,
		ActivityEntrypointsSHA256:     prepared.activitiesSHA,
		IntegrationDependenciesSHA256: prepared.integrationsSHA,
		IntegrationUsageSHA256:        prepared.usesSHA,
		Routes:                        []Route{}, Outcomes: []Outcome{},
		Coverage: Coverage{
			ActivitiesSelected:           len(prepared.activities.Objects),
			DependenciesSelected:         len(prepared.integrations.Dependencies),
			UsesObserved:                 len(prepared.uses.Uses),
			CallablesWithoutLocation:     prepared.activities.Coverage.CallablesWithoutLocation,
			SeededModulesWithoutLocation: prepared.activities.Coverage.SeededModulesWithoutLocation,
			ProgramObjectsOmitted:        prepared.index.Coverage.ObjectsOmitted,
			ProgramRelationsOmitted:      prepared.index.Coverage.RelationsOmitted,
		},
	}
	if len(prepared.uses.Uses) == 0 {
		return sealResult(result)
	}

	graph, err := compileGraph(prepared)
	if err != nil {
		return Result{}, err
	}
	result.Coverage.GraphBuilt = true
	result.Coverage.RelationsExamined = graph.relationsExamined
	result.Coverage.TraversableRelations = graph.traversableRelations
	result.Coverage.ProjectedTraversalEdges = graph.projectedEdges
	result.Coverage.ExactEdges = graph.exactEdges
	result.Coverage.PossibleEdges = graph.possibleEdges
	result.Coverage.UnresolvedTraversalRelations = graph.unresolvedRelations
	result.Coverage.TraversalTargetsOmitted = graph.targetsOmitted
	result.Coverage.DecoratorRelations = graph.decoratorRelations

	callerUseCounts := make(map[string]int)
	for _, use := range prepared.uses.Uses {
		callerUseCounts[use.Operation.CallerID]++
	}
	if len(callerUseCounts) > MaxRoutes {
		return Result{}, fmt.Errorf("activity path: %d unique callers exceed route bound %d", len(callerUseCounts), MaxRoutes)
	}
	orderedCallers := make([]programindex.Object, 0, len(callerUseCounts))
	for callerID := range callerUseCounts {
		caller, ok := prepared.objects[callerID]
		if !ok {
			return Result{}, fmt.Errorf("activity path: integration caller %q is outside ProgramIndex", callerID)
		}
		orderedCallers = append(orderedCallers, caller)
	}
	sort.Slice(orderedCallers, func(left, right int) bool {
		return objectKey(orderedCallers[left]) < objectKey(orderedCallers[right])
	})

	paths := search(prepared.activities.Objects, graph.allAdjacency)
	frontiers := graph.compileFrontierIndex(paths)
	routesByCaller := make(map[string]Route, len(orderedCallers))
	for _, caller := range orderedCallers {
		route, err := buildRoute(prepared, paths, frontiers, caller)
		if err != nil {
			return Result{}, err
		}
		if result.Coverage.TotalPathSteps > MaxTotalPathSteps-len(route.Steps) {
			return Result{}, fmt.Errorf("activity path: total retained path steps exceed %d", MaxTotalPathSteps)
		}
		result.Coverage.TotalPathSteps += len(route.Steps)
		incrementStatusCoverage(&result.Coverage, route.Status, callerUseCounts[caller.ID])
		result.Routes = append(result.Routes, route)
		routesByCaller[caller.ID] = route
	}

	for _, use := range prepared.uses.Uses {
		route, ok := routesByCaller[use.Operation.CallerID]
		if !ok {
			return Result{}, fmt.Errorf("activity path: use has no exact caller route")
		}
		if _, selected := prepared.dependencyIDs[use.Operation.DependencyID]; !selected {
			return Result{}, fmt.Errorf("activity path: use cites an unknown selected dependency")
		}
		result.Outcomes = append(result.Outcomes, Outcome{
			ID: outcomeIdentity(
				prepared.usesSHA, route.ID, use.Operation.DependencyID, use.Operation.RelationID,
				use.Operation.WitnessIndex, use.Operation.ExternalSymbolID,
			),
			DependencyID: use.Operation.DependencyID, RelationID: use.Operation.RelationID,
			WitnessIndex: use.Operation.WitnessIndex, ExternalSymbolID: use.Operation.ExternalSymbolID,
			RouteID: route.ID,
		})
	}
	result.Coverage.UniqueCallers = len(result.Routes)
	result.Coverage.Routes = len(result.Routes)
	result.Coverage.Outcomes = len(result.Outcomes)
	return sealResult(result)
}

func incrementStatusCoverage(coverage *Coverage, status Status, uses int) {
	switch status {
	case StatusExact:
		coverage.ExactOutcomes += uses
	case StatusPossible:
		coverage.PossibleOutcomes += uses
	case StatusFrontier:
		coverage.FrontierOutcomes += uses
	case StatusUnconnected:
		coverage.UnconnectedOutcomes += uses
	}
}

func routeIdentity(indexSHA256, activitiesSHA256, callerID string) string {
	return stableID("activity-route", indexSHA256, activitiesSHA256, callerID)
}

func outcomeIdentity(
	usageSHA256 string,
	routeID string,
	dependencyID string,
	relationID string,
	witnessIndex int,
	externalSymbolID string,
) string {
	return stableID(
		"activity-outcome", usageSHA256, routeID, dependencyID, relationID,
		fmt.Sprintf("%d", witnessIndex), externalSymbolID,
	)
}

func stableID(prefix string, fields ...string) string {
	digest := sha256.New()
	for _, field := range append([]string{prefix}, fields...) {
		fmt.Fprintf(digest, "%d\x00%s", len(field), field)
	}
	return prefix + "-" + hex.EncodeToString(digest.Sum(nil))
}

func cloneObject(value programindex.Object) programindex.Object {
	result := value
	if value.Location != nil {
		location := *value.Location
		result.Location = &location
	}
	return result
}

func cloneRoute(value Route) Route {
	result := value
	result.Caller = cloneObject(value.Caller)
	if value.Activity != nil {
		activity := cloneObject(*value.Activity)
		result.Activity = &activity
	}
	result.Nodes = make([]programindex.Object, len(value.Nodes))
	for position := range value.Nodes {
		result.Nodes[position] = cloneObject(value.Nodes[position])
	}
	result.Steps = make([]Step, len(value.Steps))
	for position := range value.Steps {
		result.Steps[position] = cloneStep(value.Steps[position])
	}
	result.Frontier = make([]FrontierReason, len(value.Frontier))
	copy(result.Frontier, value.Frontier)
	return result
}

func cloneStep(value Step) Step {
	result := value
	if value.Location != nil {
		location := *value.Location
		result.Location = &location
	}
	return result
}

func cloneOutcome(value Outcome) Outcome {
	return value
}

func objectKey(value programindex.Object) string {
	location := ""
	if value.Location != nil {
		location = fmt.Sprintf("%s\x00%09d\x00%09d", value.Location.Path, value.Location.Line, value.Location.Column)
	}
	return location + "\x00" + string(value.Kind) + "\x00" + value.Name + "\x00" + value.ID
}

func outcomeKey(value Outcome) string {
	return value.DependencyID + "\x00" + value.RelationID + "\x00" +
		fmt.Sprintf("%08d", value.WitnessIndex) + "\x00" + value.ExternalSymbolID
}
