package report

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/activityentrypoint"
	"github.com/dvordrova/repomap/internal/activitypath"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/integrationusage"
	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	ActivityPathViewVersion  = 1
	MaxActivityPathViewBytes = 16 << 20
)

// ActivityPathView enriches the already published IntegrationUsage uses with
// one route per exact caller. It never republishes dependency or use rows:
// Outcomes carry only the exact operation tuple needed to join an existing
// IntegrationUsageView use to a shared route.
type ActivityPathView struct {
	Version                       int                       `json:"version"`
	ProgramTargetID               string                    `json:"program_target_id"`
	ProgramIndexSHA256            string                    `json:"program_index_sha256"`
	ActivityEntrypointsSHA256     string                    `json:"activity_entrypoints_sha256,omitempty"`
	IntegrationDependenciesSHA256 string                    `json:"integration_dependencies_sha256,omitempty"`
	IntegrationUsageSHA256        string                    `json:"integration_usage_sha256,omitempty"`
	ActivityPathsSHA256           string                    `json:"activity_paths_sha256,omitempty"`
	Objects                       []ActivityPathViewObject  `json:"objects"`
	Routes                        []ActivityPathViewRoute   `json:"routes"`
	Outcomes                      []ActivityPathViewOutcome `json:"outcomes"`
	Coverage                      activitypath.Coverage     `json:"coverage"`
}

// ActivityPathViewObject is one exact ProgramIndex object used by at least one
// route. Objects are stored once even when many selected uses share a caller.
type ActivityPathViewObject struct {
	ObjectID   string                  `json:"object_id"`
	Kind       programindex.ObjectKind `json:"kind"`
	Name       string                  `json:"name"`
	Signature  string                  `json:"signature,omitempty"`
	Visibility programindex.Visibility `json:"visibility"`
	Location   *programindex.Location  `json:"location,omitempty"`
}

// ActivityPathViewRoute is one caller-shared path. Connected routes use
// ActivityID as their first object and the step chain ends at CallerID.
type ActivityPathViewRoute struct {
	RouteID          string                        `json:"route_id"`
	CallerID         string                        `json:"caller_id"`
	Status           activitypath.Status           `json:"status"`
	ActivityID       string                        `json:"activity_id,omitempty"`
	Steps            []ActivityPathViewStep        `json:"steps"`
	Distance         int                           `json:"distance"`
	PossibleSteps    int                           `json:"possible_steps"`
	CallbackHandoffs int                           `json:"callback_handoffs"`
	Frontier         []activitypath.FrontierReason `json:"frontier"`
}

// ActivityPathViewStep keeps only exact trace facts used by presentation.
// Producer witness inventories and alternative candidates remain in the
// sealed ProgramIndex/activity-path artifacts and are not duplicated here.
type ActivityPathViewStep struct {
	RelationID string                     `json:"relation_id"`
	FromID     string                     `json:"from_id"`
	ToID       string                     `json:"to_id"`
	Kind       programindex.RelationKind  `json:"kind"`
	Resolution programindex.Resolution    `json:"resolution"`
	Authority  activitypath.EdgeAuthority `json:"authority"`
	Invocation string                     `json:"invocation,omitempty"`
	Location   *programindex.Location     `json:"location,omitempty"`
}

// ActivityPathViewOutcome is the minimal exact join to one existing
// IntegrationUsageView use. Label, mechanism, caller and dependency facts are
// deliberately not copied.
type ActivityPathViewOutcome struct {
	DependencyID     string `json:"dependency_id"`
	RelationID       string `json:"relation_id"`
	WitnessIndex     int    `json:"witness_index"`
	ExternalSymbolID string `json:"external_symbol_id"`
	RouteID          string `json:"route_id"`
}

// NewActivityPathView revalidates the deterministic producer result against
// all four exact inputs, then builds the compact caller-shared report handoff.
func NewActivityPathView(
	result activitypath.Result,
	index programindex.Index,
	activities activityentrypoint.Result,
	integrations integrationdependency.Result,
	usage integrationusage.Result,
) (*ActivityPathView, error) {
	if err := result.ValidateAgainst(index, activities, integrations, usage); err != nil {
		return nil, fmt.Errorf("activity path view: producer authority: %w", err)
	}
	artifactSHA256, err := result.ArtifactSHA256()
	if err != nil {
		return nil, fmt.Errorf("activity path view: artifact identity: %w", err)
	}
	view := &ActivityPathView{
		Version: ActivityPathViewVersion, ProgramTargetID: index.Target.ID,
		ProgramIndexSHA256:            index.SHA256,
		ActivityEntrypointsSHA256:     result.ActivityEntrypointsSHA256,
		IntegrationDependenciesSHA256: result.IntegrationDependenciesSHA256,
		IntegrationUsageSHA256:        result.IntegrationUsageSHA256,
		ActivityPathsSHA256:           artifactSHA256,
		Objects:                       []ActivityPathViewObject{}, Routes: []ActivityPathViewRoute{},
		Outcomes: []ActivityPathViewOutcome{}, Coverage: result.Coverage,
	}

	objectsByID := make(map[string]programindex.Object)
	for _, route := range result.Routes {
		objectsByID[route.Caller.ID] = route.Caller
		for _, object := range route.Nodes {
			objectsByID[object.ID] = object
		}
		projected := ActivityPathViewRoute{
			RouteID: route.ID, CallerID: route.Caller.ID, Status: route.Status,
			Steps:    make([]ActivityPathViewStep, 0, len(route.Steps)),
			Distance: route.Distance, PossibleSteps: route.PossibleSteps,
			CallbackHandoffs: route.CallbackHandoffs,
			Frontier:         append([]activitypath.FrontierReason{}, route.Frontier...),
		}
		if route.Activity != nil {
			projected.ActivityID = route.Activity.ID
		}
		for _, step := range route.Steps {
			projected.Steps = append(projected.Steps, ActivityPathViewStep{
				RelationID: step.RelationID, FromID: step.FromID, ToID: step.ToID,
				Kind: step.Kind, Resolution: step.Resolution, Authority: step.Authority,
				Invocation: step.Invocation, Location: cloneActivityPathLocation(step.Location),
			})
		}
		view.Routes = append(view.Routes, projected)
	}
	for _, object := range objectsByID {
		view.Objects = append(view.Objects, projectActivityPathObject(object))
	}
	sort.Slice(view.Objects, func(left, right int) bool {
		return activityPathViewObjectKey(view.Objects[left]) < activityPathViewObjectKey(view.Objects[right])
	})
	for _, outcome := range result.Outcomes {
		view.Outcomes = append(view.Outcomes, ActivityPathViewOutcome{
			DependencyID: outcome.DependencyID, RelationID: outcome.RelationID,
			WitnessIndex: outcome.WitnessIndex, ExternalSymbolID: outcome.ExternalSymbolID,
			RouteID: outcome.RouteID,
		})
	}
	if err := view.Validate(); err != nil {
		return nil, fmt.Errorf("activity path view: invalid projection: %w", err)
	}
	return view, nil
}

// ValidateAgainst re-derives the entire compact projection from the exact
// producer chain and rejects changed routes, joins or source facts.
func (view ActivityPathView) ValidateAgainst(
	result activitypath.Result,
	index programindex.Index,
	activities activityentrypoint.Result,
	integrations integrationdependency.Result,
	usage integrationusage.Result,
) error {
	if err := view.Validate(); err != nil {
		return err
	}
	expected, err := NewActivityPathView(result, index, activities, integrations, usage)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(view, *expected) {
		return fmt.Errorf("activity path view: projection does not match exact producer authority")
	}
	return nil
}

// Validate checks the standalone persisted/browser handoff. Artifact equality
// is separately enforced by ValidateAgainst and the run-manifest verifier.
func (view ActivityPathView) Validate() error {
	if view.Version != ActivityPathViewVersion ||
		!validCubeMapViewText(view.ProgramTargetID, false) ||
		!validCubeMapViewSHA256(view.ProgramIndexSHA256) ||
		!validCubeMapViewSHA256(view.ActivityEntrypointsSHA256) ||
		!validCubeMapViewSHA256(view.IntegrationDependenciesSHA256) ||
		!validCubeMapViewSHA256(view.IntegrationUsageSHA256) ||
		!validCubeMapViewSHA256(view.ActivityPathsSHA256) ||
		view.Objects == nil || view.Routes == nil || view.Outcomes == nil {
		return fmt.Errorf("activity path view: invalid identity or collection shape")
	}
	if err := validateActivityPathViewCoverage(view.Coverage, view.Routes, view.Outcomes); err != nil {
		return err
	}

	objectsByID := make(map[string]ActivityPathViewObject, len(view.Objects))
	previousObjectKey := ""
	for position, object := range view.Objects {
		if err := validateActivityPathViewObject(object); err != nil {
			return fmt.Errorf("activity path view: object %d: %w", position, err)
		}
		key := activityPathViewObjectKey(object)
		if previousObjectKey != "" && previousObjectKey >= key {
			return fmt.Errorf("activity path view: objects are not canonical")
		}
		previousObjectKey = key
		if _, duplicate := objectsByID[object.ObjectID]; duplicate {
			return fmt.Errorf("activity path view: duplicate object identity")
		}
		objectsByID[object.ObjectID] = object
	}

	routesByID := make(map[string]ActivityPathViewRoute, len(view.Routes))
	referencedObjects := make(map[string]struct{}, len(view.Objects))
	previousRouteKey := ""
	for position, route := range view.Routes {
		if err := validateActivityPathViewRoute(route, objectsByID); err != nil {
			return fmt.Errorf("activity path view: route %d: %w", position, err)
		}
		key := activityPathViewObjectKey(objectsByID[route.CallerID])
		if previousRouteKey != "" && previousRouteKey >= key {
			return fmt.Errorf("activity path view: routes are not canonical")
		}
		previousRouteKey = key
		if _, duplicate := routesByID[route.RouteID]; duplicate {
			return fmt.Errorf("activity path view: duplicate route identity")
		}
		routesByID[route.RouteID] = route
		referencedObjects[route.CallerID] = struct{}{}
		if route.ActivityID != "" {
			referencedObjects[route.ActivityID] = struct{}{}
		}
		for _, step := range route.Steps {
			referencedObjects[step.FromID] = struct{}{}
			referencedObjects[step.ToID] = struct{}{}
		}
	}
	if len(referencedObjects) != len(view.Objects) {
		return fmt.Errorf("activity path view: object dictionary contains dead or missing route objects")
	}

	referencedRoutes := make(map[string]struct{}, len(view.Routes))
	previousOutcomeKey := ""
	seenOutcomes := make(map[string]struct{}, len(view.Outcomes))
	for position, outcome := range view.Outcomes {
		if err := validateActivityPathViewOutcome(outcome); err != nil {
			return fmt.Errorf("activity path view: outcome %d: %w", position, err)
		}
		key := activityPathViewOutcomeKey(outcome)
		if previousOutcomeKey != "" && previousOutcomeKey >= key {
			return fmt.Errorf("activity path view: outcomes are not canonical")
		}
		previousOutcomeKey = key
		if _, duplicate := seenOutcomes[key]; duplicate {
			return fmt.Errorf("activity path view: duplicate integration use binding")
		}
		seenOutcomes[key] = struct{}{}
		if _, ok := routesByID[outcome.RouteID]; !ok {
			return fmt.Errorf("activity path view: outcome cites an unknown route")
		}
		referencedRoutes[outcome.RouteID] = struct{}{}
	}
	if len(referencedRoutes) != len(view.Routes) {
		return fmt.Errorf("activity path view: route has no exact integration use binding")
	}

	encoded, err := json.Marshal(view)
	if err != nil {
		return fmt.Errorf("activity path view: encode bound check: %w", err)
	}
	if len(encoded) > MaxActivityPathViewBytes {
		return fmt.Errorf(
			"activity path view: exact projection requires %d bytes; report projection limit is %d bytes",
			len(encoded), MaxActivityPathViewBytes,
		)
	}
	return nil
}

// ValidateReportJoins proves that the compact outcome ledger covers the exact
// IntegrationUsageView once and that every connected route starts at an exact
// selected ActivityEntrypointView object.
func (view ActivityPathView) ValidateReportJoins(
	activities *ActivityEntrypointView,
	usage *IntegrationUsageView,
) error {
	if activities == nil || usage == nil {
		return fmt.Errorf("activity path view: activity and integration report authorities are required")
	}
	if err := view.Validate(); err != nil {
		return err
	}
	if err := activities.Validate(); err != nil {
		return err
	}
	if err := usage.Validate(); err != nil {
		return err
	}
	if view.ProgramTargetID != activities.ProgramTargetID || view.ProgramTargetID != usage.ProgramTargetID ||
		view.ProgramIndexSHA256 != activities.ProgramIndexSHA256 ||
		view.ProgramIndexSHA256 != usage.ProgramIndexSHA256 ||
		view.IntegrationDependenciesSHA256 != usage.IntegrationDependenciesSHA256 ||
		view.IntegrationUsageSHA256 != usage.IntegrationUsageSHA256 {
		return fmt.Errorf("activity path view: report authorities do not share exact material identities")
	}

	objectsByID := make(map[string]ActivityPathViewObject, len(view.Objects))
	for _, object := range view.Objects {
		objectsByID[object.ObjectID] = object
	}
	activitiesByID := make(map[string]ActivityEntrypointViewObject, len(activities.Entrypoints))
	for _, activity := range activities.Entrypoints {
		activitiesByID[activity.ObjectID] = activity
	}
	routesByID := make(map[string]ActivityPathViewRoute, len(view.Routes))
	for _, route := range view.Routes {
		routesByID[route.RouteID] = route
		if route.Status == activitypath.StatusExact || route.Status == activitypath.StatusPossible {
			activity, ok := activitiesByID[route.ActivityID]
			if !ok || !sameActivityPathActivityObject(objectsByID[route.ActivityID], activity) {
				return fmt.Errorf("activity path view: connected route does not start at an exact selected activity")
			}
		}
	}

	usesByKey := make(map[string]IntegrationUsageUse)
	for _, dependency := range usage.Dependencies {
		for _, selectedUse := range dependency.Uses {
			key := integrationUsageViewUseKey(dependency.DependencyID, selectedUse)
			usesByKey[key] = selectedUse
		}
	}
	if len(usesByKey) != len(view.Outcomes) {
		return fmt.Errorf("activity path view: outcome count does not cover exact integration uses")
	}
	for _, outcome := range view.Outcomes {
		key := activityPathViewOutcomeKey(outcome)
		selectedUse, ok := usesByKey[key]
		if !ok {
			return fmt.Errorf("activity path view: outcome does not join an exact integration use")
		}
		route := routesByID[outcome.RouteID]
		caller := objectsByID[route.CallerID]
		if route.CallerID != selectedUse.CallerID || caller.Kind != selectedUse.CallerKind ||
			caller.Name != selectedUse.CallerName ||
			caller.Location == nil || *caller.Location != selectedUse.CallerLocation {
			return fmt.Errorf("activity path view: route caller differs from exact integration use caller")
		}
	}
	return nil
}

func validateActivityPathViewCoverage(
	coverage activitypath.Coverage,
	routes []ActivityPathViewRoute,
	outcomes []ActivityPathViewOutcome,
) error {
	counts := []int{
		coverage.ActivitiesSelected, coverage.DependenciesSelected, coverage.UsesObserved,
		coverage.UniqueCallers, coverage.Routes, coverage.Outcomes, coverage.RelationsExamined,
		coverage.TraversableRelations, coverage.ProjectedTraversalEdges, coverage.ExactEdges,
		coverage.PossibleEdges, coverage.UnresolvedTraversalRelations, coverage.TraversalTargetsOmitted,
		coverage.DecoratorRelations, coverage.CallablesWithoutLocation, coverage.CallablesIneligible,
		coverage.SeededModulesWithoutLocation, coverage.ProgramObjectsOmitted,
		coverage.ProgramRelationsOmitted, coverage.ExactOutcomes, coverage.PossibleOutcomes,
		coverage.FrontierOutcomes, coverage.UnconnectedOutcomes, coverage.TotalPathSteps,
	}
	for _, count := range counts {
		if count < 0 || count > programindex.MaxObservedCount {
			return fmt.Errorf("activity path view: invalid producer coverage count")
		}
	}
	if coverage.ActivitiesSelected > activityentrypoint.MaxSelectedEntrypoints ||
		coverage.DependenciesSelected > integrationdependency.MaxSelectedDependencies ||
		coverage.UsesObserved > activitypath.MaxOutcomes || coverage.UniqueCallers > activitypath.MaxRoutes ||
		coverage.Routes != len(routes) || coverage.UniqueCallers != len(routes) ||
		coverage.Outcomes != len(outcomes) || coverage.UsesObserved != len(outcomes) ||
		coverage.ProjectedTraversalEdges > activitypath.MaxProjectedTraversalEdges ||
		coverage.ExactEdges+coverage.PossibleEdges != coverage.ProjectedTraversalEdges ||
		coverage.TotalPathSteps > activitypath.MaxTotalPathSteps ||
		coverage.ExactOutcomes+coverage.PossibleOutcomes+coverage.FrontierOutcomes+
			coverage.UnconnectedOutcomes != len(outcomes) {
		return fmt.Errorf("activity path view: invalid producer coverage ledger")
	}
	totalSteps := 0
	statusCounts := map[activitypath.Status]int{}
	routeStatusByID := make(map[string]activitypath.Status, len(routes))
	for _, route := range routes {
		totalSteps += len(route.Steps)
		routeStatusByID[route.RouteID] = route.Status
	}
	for _, outcome := range outcomes {
		statusCounts[routeStatusByID[outcome.RouteID]]++
	}
	if totalSteps != coverage.TotalPathSteps ||
		statusCounts[activitypath.StatusExact] != coverage.ExactOutcomes ||
		statusCounts[activitypath.StatusPossible] != coverage.PossibleOutcomes ||
		statusCounts[activitypath.StatusFrontier] != coverage.FrontierOutcomes ||
		statusCounts[activitypath.StatusUnconnected] != coverage.UnconnectedOutcomes {
		return fmt.Errorf("activity path view: producer outcome coverage mismatch")
	}
	if coverage.UsesObserved == 0 {
		if coverage.GraphBuilt || coverage.RelationsExamined != 0 ||
			coverage.ProjectedTraversalEdges != 0 || len(routes) != 0 || len(outcomes) != 0 {
			return fmt.Errorf("activity path view: empty-use bypass has graph output")
		}
	} else if !coverage.GraphBuilt {
		return fmt.Errorf("activity path view: non-empty use set has no graph authority")
	}
	return nil
}

func validateActivityPathViewObject(value ActivityPathViewObject) error {
	if !validActivityPathProgramObjectID(value.ObjectID) || !value.Kind.Valid() ||
		!validCubeMapViewText(value.Name, false) || !validCubeMapViewText(value.Signature, true) ||
		!value.Visibility.Valid() {
		return fmt.Errorf("invalid exact object")
	}
	if value.Location != nil && !validCubeMapViewLocation(CubeMapViewLocation{
		Path: value.Location.Path, Line: value.Location.Line, Column: value.Location.Column,
	}, true) {
		return fmt.Errorf("invalid exact object location")
	}
	return nil
}

func validateActivityPathViewRoute(
	route ActivityPathViewRoute,
	objectsByID map[string]ActivityPathViewObject,
) error {
	if !validActivityPathRouteID(route.RouteID) || !validActivityPathProgramObjectID(route.CallerID) ||
		!route.Status.Valid() || route.Steps == nil || route.Frontier == nil ||
		route.Distance != len(route.Steps) || route.Distance < 0 ||
		route.PossibleSteps < 0 || route.CallbackHandoffs < 0 ||
		len(route.Steps) > activitypath.MaxRouteSteps {
		return fmt.Errorf("invalid route identity or measurements")
	}
	if _, ok := objectsByID[route.CallerID]; !ok {
		return fmt.Errorf("caller is absent from the exact object dictionary")
	}
	possibleSteps, callbacks := 0, 0
	seenObjects := make(map[string]struct{}, len(route.Steps)+1)
	if route.ActivityID != "" {
		if _, ok := objectsByID[route.ActivityID]; !ok {
			return fmt.Errorf("activity is absent from the exact object dictionary")
		}
		seenObjects[route.ActivityID] = struct{}{}
	}
	previousID := route.ActivityID
	for position, step := range route.Steps {
		if err := validateActivityPathViewStep(step); err != nil {
			return fmt.Errorf("step %d: %w", position, err)
		}
		if _, ok := objectsByID[step.FromID]; !ok {
			return fmt.Errorf("step source is absent from the exact object dictionary")
		}
		if _, ok := objectsByID[step.ToID]; !ok {
			return fmt.Errorf("step target is absent from the exact object dictionary")
		}
		if previousID == "" || step.FromID != previousID {
			return fmt.Errorf("step chain is discontinuous")
		}
		if _, duplicate := seenObjects[step.ToID]; duplicate {
			return fmt.Errorf("route contains an object cycle")
		}
		seenObjects[step.ToID] = struct{}{}
		previousID = step.ToID
		if step.Authority == activitypath.EdgePossible {
			possibleSteps++
		}
		if step.Kind == programindex.RelationPassesCallback {
			callbacks++
		}
	}
	if possibleSteps != route.PossibleSteps || callbacks != route.CallbackHandoffs {
		return fmt.Errorf("route uncertainty measurements differ from steps")
	}
	switch route.Status {
	case activitypath.StatusExact, activitypath.StatusPossible:
		if route.ActivityID == "" || len(route.Frontier) != 0 ||
			(len(route.Steps) == 0 && route.ActivityID != route.CallerID) ||
			(len(route.Steps) > 0 && previousID != route.CallerID) ||
			route.Status == activitypath.StatusExact && possibleSteps != 0 ||
			route.Status == activitypath.StatusPossible && possibleSteps == 0 {
			return fmt.Errorf("connected route status contradicts its exact step chain")
		}
	case activitypath.StatusFrontier:
		if route.ActivityID != "" || len(route.Steps) != 0 || len(route.Frontier) == 0 ||
			route.Distance != 0 || route.PossibleSteps != 0 || route.CallbackHandoffs != 0 {
			return fmt.Errorf("frontier route invents a connected path")
		}
	case activitypath.StatusUnconnected:
		if route.ActivityID != "" || len(route.Steps) != 0 || len(route.Frontier) != 0 ||
			route.Distance != 0 || route.PossibleSteps != 0 || route.CallbackHandoffs != 0 {
			return fmt.Errorf("unconnected route invents path or frontier authority")
		}
	}
	previousFrontier := activitypath.FrontierReason("")
	for _, reason := range route.Frontier {
		if !reason.Valid() || previousFrontier != "" && previousFrontier >= reason {
			return fmt.Errorf("frontier reasons are invalid or non-canonical")
		}
		previousFrontier = reason
	}
	return nil
}

func validateActivityPathViewStep(step ActivityPathViewStep) error {
	if !validActivityPathProgramRelationID(step.RelationID) ||
		!validActivityPathProgramObjectID(step.FromID) ||
		!validActivityPathProgramObjectID(step.ToID) || !step.Kind.Valid() ||
		!step.Resolution.Valid() || !step.Authority.Valid() ||
		!validCubeMapViewText(step.Invocation, true) {
		return fmt.Errorf("invalid exact step")
	}
	if step.Location != nil && !validCubeMapViewLocation(CubeMapViewLocation{
		Path: step.Location.Path, Line: step.Location.Line, Column: step.Location.Column,
	}, true) {
		return fmt.Errorf("invalid exact step location")
	}
	switch step.Kind {
	case programindex.RelationCalls, programindex.RelationExecutes:
		if step.Resolution == programindex.ResolutionUnresolved ||
			step.Authority == activitypath.EdgeExact && step.Resolution != programindex.ResolutionExact ||
			step.Authority == activitypath.EdgePossible && step.Resolution != programindex.ResolutionAlternatives {
			return fmt.Errorf("call step authority mismatch")
		}
	case programindex.RelationPassesCallback:
		if step.Resolution == programindex.ResolutionUnresolved || step.Authority != activitypath.EdgePossible {
			return fmt.Errorf("callback step authority mismatch")
		}
	default:
		return fmt.Errorf("non-traversable relation kind")
	}
	return nil
}

func validateActivityPathViewOutcome(value ActivityPathViewOutcome) error {
	if !validCubeMapViewText(value.DependencyID, false) ||
		!validActivityPathProgramRelationID(value.RelationID) || value.WitnessIndex < 0 ||
		!validActivityPathProgramObjectID(value.ExternalSymbolID) ||
		!validActivityPathRouteID(value.RouteID) {
		return fmt.Errorf("invalid exact integration use binding")
	}
	return nil
}

func projectActivityPathObject(value programindex.Object) ActivityPathViewObject {
	return ActivityPathViewObject{
		ObjectID: value.ID, Kind: value.Kind, Name: value.Name, Signature: value.Signature,
		Visibility: value.Visibility, Location: cloneActivityPathLocation(value.Location),
	}
}

func cloneActivityPathLocation(value *programindex.Location) *programindex.Location {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func activityPathViewObjectKey(value ActivityPathViewObject) string {
	location := ""
	if value.Location != nil {
		location = fmt.Sprintf("%s\x00%09d\x00%09d", value.Location.Path, value.Location.Line, value.Location.Column)
	}
	return strings.Join([]string{
		location, string(value.Kind), value.Name, value.ObjectID,
	}, "\x00")
}

func activityPathViewOutcomeKey(value ActivityPathViewOutcome) string {
	return fmt.Sprintf("%s\x00%s\x00%08d\x00%s",
		value.DependencyID, value.RelationID, value.WitnessIndex, value.ExternalSymbolID)
}

func validActivityPathProgramObjectID(value string) bool {
	return strings.HasPrefix(value, "program-object-") &&
		validCubeMapViewSHA256(strings.TrimPrefix(value, "program-object-"))
}

func validActivityPathProgramRelationID(value string) bool {
	return strings.HasPrefix(value, "program-relation-") &&
		validCubeMapViewSHA256(strings.TrimPrefix(value, "program-relation-"))
}

func validActivityPathRouteID(value string) bool {
	return strings.HasPrefix(value, "activity-route-") &&
		validCubeMapViewSHA256(strings.TrimPrefix(value, "activity-route-"))
}

func sameActivityPathActivityObject(
	pathObject ActivityPathViewObject,
	activity ActivityEntrypointViewObject,
) bool {
	return pathObject.ObjectID == activity.ObjectID && pathObject.Kind == activity.Kind &&
		pathObject.Name == activity.Name && pathObject.Signature == activity.Signature &&
		pathObject.Visibility == activity.Visibility && pathObject.Location != nil &&
		*pathObject.Location == activity.Location
}
