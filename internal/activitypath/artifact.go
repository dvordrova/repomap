package activitypath

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/activityentrypoint"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/integrationusage"
	"github.com/dvordrova/repomap/internal/programindex"
)

const MaxArtifactBytes = 16 << 20

func sealResult(result Result) (Result, error) {
	result.Routes = cloneRoutes(result.Routes)
	result.Outcomes = cloneOutcomes(result.Outcomes)
	digest, err := resultDigest(result)
	if err != nil {
		return Result{}, err
	}
	result.SHA256 = digest
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}

// Snapshot returns a consumer-owned copy.
func (result Result) Snapshot() Result {
	copy := result
	copy.Routes = cloneRoutes(result.Routes)
	copy.Outcomes = cloneOutcomes(result.Outcomes)
	return copy
}

// Validate checks the standalone canonical shape, complete outcome ledger,
// route/status invariants, bounded projections, and result seal.
func (result Result) Validate() error {
	if result.Version != Version || !validSHA256(result.ProgramIndexSHA256) ||
		!validSHA256(result.ActivityEntrypointsSHA256) ||
		!validSHA256(result.IntegrationDependenciesSHA256) ||
		!validSHA256(result.IntegrationUsageSHA256) || !validSHA256(result.SHA256) ||
		result.Routes == nil || result.Outcomes == nil {
		return fmt.Errorf("activity path: invalid result identity")
	}
	if err := validateCoverage(result.Coverage, result.Routes, result.Outcomes); err != nil {
		return err
	}
	routes := make(map[string]Route, len(result.Routes))
	callers := make(map[string]struct{}, len(result.Routes))
	for position, route := range result.Routes {
		if err := validateRoute(result.ProgramIndexSHA256, result.ActivityEntrypointsSHA256, route); err != nil {
			return fmt.Errorf("activity path: route %d: %w", position, err)
		}
		if _, duplicate := routes[route.ID]; duplicate {
			return fmt.Errorf("activity path: duplicate route identity")
		}
		if _, duplicate := callers[route.Caller.ID]; duplicate {
			return fmt.Errorf("activity path: duplicate caller route")
		}
		routes[route.ID] = route
		callers[route.Caller.ID] = struct{}{}
		if position > 0 && objectKey(result.Routes[position-1].Caller) >= objectKey(route.Caller) {
			return fmt.Errorf("activity path: routes are not canonical")
		}
	}
	seenOutcomes := make(map[string]struct{}, len(result.Outcomes))
	referencedRoutes := make(map[string]struct{}, len(result.Routes))
	for position, outcome := range result.Outcomes {
		if err := validateOutcome(result.IntegrationUsageSHA256, outcome, routes); err != nil {
			return fmt.Errorf("activity path: outcome %d: %w", position, err)
		}
		if _, duplicate := seenOutcomes[outcome.ID]; duplicate {
			return fmt.Errorf("activity path: duplicate outcome identity")
		}
		seenOutcomes[outcome.ID] = struct{}{}
		referencedRoutes[outcome.RouteID] = struct{}{}
		if position > 0 && outcomeKey(result.Outcomes[position-1]) >= outcomeKey(outcome) {
			return fmt.Errorf("activity path: outcomes are not canonical")
		}
	}
	if len(referencedRoutes) != len(result.Routes) {
		return fmt.Errorf("activity path: caller route has no exact use outcome")
	}
	want, err := resultDigest(result)
	if err != nil {
		return err
	}
	if result.SHA256 != want {
		return fmt.Errorf("activity path: artifact sha256 mismatch")
	}
	return nil
}

// ValidateAgainst rebuilds the deterministic artifact from all four exact
// inputs and requires complete byte-level structural equality.
func (result Result) ValidateAgainst(
	index programindex.Index,
	activities activityentrypoint.Result,
	integrations integrationdependency.Result,
	uses integrationusage.Result,
) error {
	if err := result.Validate(); err != nil {
		return err
	}
	prepared, err := prepareInputs(index, activities, integrations, uses)
	if err != nil {
		return err
	}
	want, err := build(prepared)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(result, want) {
		return fmt.Errorf("activity path: result does not match exact input authority")
	}
	return nil
}

// Encode returns exact canonical standalone artifact bytes.
func Encode(result Result) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("activity path: encode artifact: %w", err)
	}
	if len(encoded) == 0 || len(encoded) > MaxArtifactBytes {
		return nil, fmt.Errorf("activity path: artifact is %d bytes, limit is %d", len(encoded), MaxArtifactBytes)
	}
	return encoded, nil
}

// Decode rejects unknown fields, trailing data, noncanonical bytes, invalid
// seals, and any result not reproducible from the supplied exact inputs.
func Decode(
	encoded []byte,
	index programindex.Index,
	activities activityentrypoint.Result,
	integrations integrationdependency.Result,
	uses integrationusage.Result,
) (Result, error) {
	if len(encoded) == 0 || len(encoded) > MaxArtifactBytes {
		return Result{}, fmt.Errorf("activity path: invalid artifact size")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var result Result
	if err := decoder.Decode(&result); err != nil {
		return Result{}, fmt.Errorf("activity path: decode artifact: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Result{}, fmt.Errorf("activity path: trailing JSON value")
		}
		return Result{}, fmt.Errorf("activity path: trailing artifact data: %w", err)
	}
	if err := result.ValidateAgainst(index, activities, integrations, uses); err != nil {
		return Result{}, err
	}
	canonical, err := Encode(result)
	if err != nil {
		return Result{}, err
	}
	if !bytes.Equal(encoded, canonical) {
		return Result{}, fmt.Errorf("activity path: artifact is not canonical")
	}
	return result, nil
}

// ArtifactSHA256 returns the digest of exact canonical artifact bytes.
func (result Result) ArtifactSHA256() (string, error) {
	encoded, err := Encode(result)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validateCoverage(coverage Coverage, routes []Route, outcomes []Outcome) error {
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
			return fmt.Errorf("activity path: invalid coverage count")
		}
	}
	if coverage.ActivitiesSelected > activityentrypoint.MaxSelectedEntrypoints ||
		coverage.DependenciesSelected > integrationdependency.MaxSelectedDependencies ||
		coverage.UsesObserved > MaxOutcomes || coverage.UniqueCallers > MaxRoutes ||
		coverage.UniqueCallers > coverage.UsesObserved ||
		coverage.Routes != len(routes) || coverage.UniqueCallers != len(routes) ||
		coverage.Outcomes != len(outcomes) || coverage.UsesObserved != len(outcomes) ||
		coverage.RelationsExamined > programindex.MaxRelations ||
		coverage.TraversableRelations > coverage.RelationsExamined ||
		coverage.UnresolvedTraversalRelations > coverage.RelationsExamined ||
		coverage.DecoratorRelations > coverage.RelationsExamined ||
		coverage.TraversableRelations+coverage.UnresolvedTraversalRelations+
			coverage.DecoratorRelations > coverage.RelationsExamined ||
		coverage.TraversableRelations > coverage.ProjectedTraversalEdges ||
		coverage.ProjectedTraversalEdges > MaxProjectedTraversalEdges ||
		coverage.ExactEdges+coverage.PossibleEdges != coverage.ProjectedTraversalEdges ||
		coverage.TotalPathSteps > MaxTotalPathSteps ||
		coverage.ExactOutcomes+coverage.PossibleOutcomes+coverage.FrontierOutcomes+
			coverage.UnconnectedOutcomes != len(outcomes) {
		return fmt.Errorf("activity path: invalid coverage ledger")
	}
	if coverage.UsesObserved > 0 && coverage.DependenciesSelected == 0 {
		return fmt.Errorf("activity path: use coverage has no selected dependency authority")
	}
	if coverage.ActivitiesSelected == 0 && (coverage.ExactOutcomes > 0 || coverage.PossibleOutcomes > 0) {
		return fmt.Errorf("activity path: connected outcomes have no selected activity authority")
	}
	totalSteps := 0
	statusCounts := map[Status]int{}
	routesByID := make(map[string]Route, len(routes))
	for _, route := range routes {
		totalSteps += len(route.Steps)
		routesByID[route.ID] = route
	}
	for _, outcome := range outcomes {
		statusCounts[routesByID[outcome.RouteID].Status]++
	}
	if totalSteps != coverage.TotalPathSteps || statusCounts[StatusExact] != coverage.ExactOutcomes ||
		statusCounts[StatusPossible] != coverage.PossibleOutcomes ||
		statusCounts[StatusFrontier] != coverage.FrontierOutcomes ||
		statusCounts[StatusUnconnected] != coverage.UnconnectedOutcomes {
		return fmt.Errorf("activity path: outcome coverage mismatch")
	}
	if coverage.UsesObserved == 0 {
		if coverage.GraphBuilt || coverage.RelationsExamined != 0 || coverage.ProjectedTraversalEdges != 0 ||
			len(routes) != 0 || len(outcomes) != 0 {
			return fmt.Errorf("activity path: empty-use bypass has graph output")
		}
	} else if !coverage.GraphBuilt {
		return fmt.Errorf("activity path: non-empty use set has no graph authority")
	}
	return nil
}

func validateRoute(indexSHA256, activitiesSHA256 string, route Route) error {
	if route.ID != routeIdentity(indexSHA256, activitiesSHA256, route.Caller.ID) {
		return fmt.Errorf("route identity mismatch")
	}
	if !route.Status.Valid() || route.Nodes == nil || route.Steps == nil || route.Frontier == nil ||
		route.Distance < 0 || route.Distance != len(route.Steps) ||
		route.PossibleSteps < 0 || route.CallbackHandoffs < 0 {
		return fmt.Errorf("invalid route measurements")
	}
	if err := validateObject(route.Caller, true); err != nil {
		return fmt.Errorf("caller: %w", err)
	}
	if !canonicalFrontier(route.Frontier) {
		return fmt.Errorf("frontier reasons are not canonical")
	}
	switch route.Status {
	case StatusExact, StatusPossible:
		if route.Activity == nil || len(route.Nodes) == 0 || len(route.Steps)+1 != len(route.Nodes) || len(route.Frontier) != 0 {
			return fmt.Errorf("connected route has invalid path shape")
		}
		if err := validateObject(*route.Activity, true); err != nil {
			return fmt.Errorf("activity: %w", err)
		}
		if !activityAnchorKind(route.Activity.Kind) {
			return fmt.Errorf("activity has a non-selectable object kind")
		}
		if !reflect.DeepEqual(*route.Activity, route.Nodes[0]) ||
			!reflect.DeepEqual(route.Caller, route.Nodes[len(route.Nodes)-1]) {
			return fmt.Errorf("connected route endpoints do not match path nodes")
		}
		seenNodes := make(map[string]struct{}, len(route.Nodes))
		for position, node := range route.Nodes {
			if err := validateObject(node, false); err != nil {
				return fmt.Errorf("node %d: %w", position, err)
			}
			if _, duplicate := seenNodes[node.ID]; duplicate {
				return fmt.Errorf("route contains an object cycle")
			}
			seenNodes[node.ID] = struct{}{}
		}
		possibleSteps, callbacks := 0, 0
		for position, step := range route.Steps {
			if err := validateStep(step); err != nil {
				return fmt.Errorf("step %d: %w", position, err)
			}
			if step.FromID != route.Nodes[position].ID || step.ToID != route.Nodes[position+1].ID {
				return fmt.Errorf("step does not bind adjacent nodes")
			}
			if step.Authority == EdgePossible {
				possibleSteps++
			}
			if step.Kind == programindex.RelationPassesCallback {
				callbacks++
			}
		}
		if possibleSteps != route.PossibleSteps || callbacks != route.CallbackHandoffs {
			return fmt.Errorf("route uncertainty measurements mismatch")
		}
		if route.Status == StatusExact && possibleSteps != 0 || route.Status == StatusPossible && possibleSteps == 0 {
			return fmt.Errorf("route status does not match step authority")
		}
	case StatusFrontier:
		if route.Activity != nil || len(route.Nodes) != 0 || len(route.Steps) != 0 || len(route.Frontier) == 0 ||
			route.Distance != 0 || route.PossibleSteps != 0 || route.CallbackHandoffs != 0 {
			return fmt.Errorf("frontier route has invented path authority")
		}
	case StatusUnconnected:
		if route.Activity != nil || len(route.Nodes) != 0 || len(route.Steps) != 0 || len(route.Frontier) != 0 ||
			route.Distance != 0 || route.PossibleSteps != 0 || route.CallbackHandoffs != 0 {
			return fmt.Errorf("unconnected route has invented path authority")
		}
	}
	if len(route.Steps) > MaxRouteSteps {
		return fmt.Errorf("route step bound exceeded")
	}
	return nil
}

func validateStep(step Step) error {
	if !validProgramRelationID(step.RelationID) || !validProgramObjectID(step.FromID) ||
		!validProgramObjectID(step.ToID) || !step.Resolution.Valid() || !step.Authority.Valid() ||
		!validOptionalText(step.Invocation) || !validOptionalLocation(step.Location) ||
		step.TargetsObserved <= 0 || step.TargetsOmitted < 0 || step.TargetsOmitted >= step.TargetsObserved ||
		step.WitnessesObserved <= 0 || step.WitnessesOmitted < 0 || step.WitnessesOmitted >= step.WitnessesObserved {
		return fmt.Errorf("invalid step projection")
	}
	switch step.Kind {
	case programindex.RelationCalls, programindex.RelationExecutes:
		if step.Resolution == programindex.ResolutionUnresolved ||
			step.Authority == EdgeExact && step.Resolution != programindex.ResolutionExact ||
			step.Authority == EdgePossible && step.Resolution != programindex.ResolutionAlternatives {
			return fmt.Errorf("call step authority mismatch")
		}
	case programindex.RelationPassesCallback:
		if step.Resolution == programindex.ResolutionUnresolved || step.Authority != EdgePossible {
			return fmt.Errorf("callback step authority mismatch")
		}
	default:
		return fmt.Errorf("non-traversable relation kind")
	}
	return nil
}

func validateOutcome(usageSHA256 string, outcome Outcome, routes map[string]Route) error {
	if outcome.ID != outcomeIdentity(
		usageSHA256, outcome.RouteID, outcome.DependencyID, outcome.RelationID,
		outcome.WitnessIndex, outcome.ExternalSymbolID,
	) || !validText(outcome.DependencyID) ||
		!validProgramRelationID(outcome.RelationID) || outcome.WitnessIndex < 0 ||
		!validProgramObjectID(outcome.ExternalSymbolID) {
		return fmt.Errorf("invalid outcome authority")
	}
	if _, ok := routes[outcome.RouteID]; !ok {
		return fmt.Errorf("outcome route binding mismatch")
	}
	return nil
}

func activityAnchorKind(kind programindex.ObjectKind) bool {
	switch kind {
	case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectLambda,
		programindex.ObjectModule, programindex.ObjectPackage:
		return true
	default:
		return false
	}
}

func validateObject(object programindex.Object, requireLocation bool) error {
	if !validProgramObjectID(object.ID) || !validText(object.SourceRef) || !object.Kind.Valid() ||
		!validText(object.Name) || !object.Visibility.Valid() || !validOptionalText(object.Signature) ||
		!validOptionalProgramObjectID(object.OwnerID) || !validOptionalProgramObjectID(object.ContainerID) ||
		!validOptionalLocation(object.Location) || requireLocation && object.Location == nil {
		return fmt.Errorf("invalid ProgramIndex object")
	}
	return nil
}

func canonicalFrontier(values []FrontierReason) bool {
	for position, value := range values {
		if !value.Valid() || position > 0 && values[position-1] >= value {
			return false
		}
	}
	return true
}

func resultDigest(result Result) (string, error) {
	payload := result.Snapshot()
	payload.SHA256 = ""
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("activity path: encode digest material: %w", err)
	}
	if len(encoded) > MaxArtifactBytes {
		return "", fmt.Errorf("activity path: digest material exceeds artifact bound")
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func cloneRoutes(values []Route) []Route {
	result := make([]Route, len(values))
	for position := range values {
		result[position] = cloneRoute(values[position])
	}
	return result
}

func cloneOutcomes(values []Outcome) []Outcome {
	result := make([]Outcome, len(values))
	for position := range values {
		result[position] = cloneOutcome(values[position])
	}
	return result
}

func validProgramObjectID(value string) bool {
	return strings.HasPrefix(value, "program-object-") && validSHA256(strings.TrimPrefix(value, "program-object-"))
}

func validOptionalProgramObjectID(value string) bool {
	return value == "" || validProgramObjectID(value)
}

func validProgramRelationID(value string) bool {
	return strings.HasPrefix(value, "program-relation-") && validSHA256(strings.TrimPrefix(value, "program-relation-"))
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validText(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > programindex.MaxTextBytes || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validOptionalText(value string) bool { return value == "" || validText(value) }

func validOptionalLocation(value *programindex.Location) bool {
	return value == nil || validLocation(*value)
}

func validLocation(value programindex.Location) bool {
	return validPath(value.Path) && value.Line > 0 && value.Column > 0
}

func validPath(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || strings.Contains(value, "\\") ||
		strings.HasPrefix(value, "/") || !fs.ValidPath(value) || value == "." || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
