package activitypath

import (
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/programindex"
)

type projectedEdge struct {
	step Step
	key  string
}

type graphCompilation struct {
	allAdjacency   map[string][]projectedEdge
	globalFrontier []FrontierReason
	boundaries     []frontierBoundary

	relationsExamined    int
	traversableRelations int
	projectedEdges       int
	exactEdges           int
	possibleEdges        int
	unresolvedRelations  int
	targetsOmitted       int
	decoratorRelations   int
}

// frontierBoundary is a retained, exact relation endpoint pair whose relation
// kind is deliberately not traversable. It can explain only the route for its
// exact opposite endpoint; it is never aggregate evidence for other callers.
type frontierBoundary struct {
	fromID string
	toID   string
	reason FrontierReason
}

// frontierIndex contains only globally applicable ProgramIndex omissions and
// exact per-caller relation boundaries. Adapter uncertainty with no retained
// target is coverage, not authority to classify arbitrary caller routes.
type frontierIndex struct {
	global   []FrontierReason
	byCaller map[string][]FrontierReason
}

type predecessor struct {
	fromID string
	edge   projectedEdge
}

type searchResult struct {
	rootByNode map[string]string
	previous   map[string]predecessor
}

type pathState struct {
	rootID           string
	rootRank         int
	possibleSteps    int
	callbackHandoffs int
	sequenceRank     int
}

type pathCandidate struct {
	toID             string
	fromID           string
	edge             projectedEdge
	rootID           string
	rootRank         int
	possibleSteps    int
	callbackHandoffs int
	previousRank     int
}

func compileGraph(prepared inputs) (graphCompilation, error) {
	graph := graphCompilation{
		allAdjacency:      make(map[string][]projectedEdge),
		relationsExamined: len(prepared.index.Relations),
	}
	frontier := make(map[FrontierReason]struct{})
	if prepared.index.Coverage.ObjectsOmitted > 0 {
		frontier[FrontierObjectsOmitted] = struct{}{}
	}
	if prepared.index.Coverage.RelationsOmitted > 0 {
		frontier[FrontierRelationsOmitted] = struct{}{}
	}
	for _, relation := range prepared.index.Relations {
		switch relation.Kind {
		case programindex.RelationDecorates:
			graph.decoratorRelations++
			for _, targetID := range relation.ToIDs {
				graph.boundaries = append(graph.boundaries, frontierBoundary{
					fromID: relation.FromID, toID: targetID, reason: FrontierDecoratorBoundary,
				})
			}
			continue
		case programindex.RelationCalls, programindex.RelationExecutes, programindex.RelationPassesCallback:
		default:
			continue
		}

		if relation.TargetsOmitted > 0 {
			graph.targetsOmitted += relation.TargetsOmitted
		}
		if relation.Resolution == programindex.ResolutionUnresolved {
			graph.unresolvedRelations++
			continue
		}
		if len(relation.ToIDs) == 0 {
			return graphCompilation{}, fmt.Errorf("activity path: retained traversal relation %q has no targets", relation.ID)
		}
		graph.traversableRelations++
		for _, targetID := range relation.ToIDs {
			authority := edgeAuthority(relation)
			edge := projectedEdge{step: Step{
				RelationID: relation.ID, FromID: relation.FromID, ToID: targetID,
				Kind: relation.Kind, Resolution: relation.Resolution, Authority: authority,
				Invocation: relation.Invocation, Location: cloneLocation(relation.Location),
				TargetsObserved: relation.TargetsObserved, TargetsOmitted: relation.TargetsOmitted,
				WitnessesObserved: relation.WitnessesObserved, WitnessesOmitted: relation.WitnessesOmitted,
			}}
			edge.key = projectedEdgeKey(edge, prepared.objects)
			graph.allAdjacency[relation.FromID] = append(graph.allAdjacency[relation.FromID], edge)
			graph.projectedEdges++
			if graph.projectedEdges > MaxProjectedTraversalEdges {
				return graphCompilation{}, fmt.Errorf(
					"activity path: projected traversal edges exceed %d", MaxProjectedTraversalEdges,
				)
			}
			if authority == EdgeExact {
				graph.exactEdges++
			} else {
				graph.possibleEdges++
			}
		}
	}
	if graph.exactEdges+graph.possibleEdges != graph.projectedEdges {
		return graphCompilation{}, fmt.Errorf("activity path: projected edge coverage mismatch")
	}
	for fromID := range graph.allAdjacency {
		sort.Slice(graph.allAdjacency[fromID], func(left, right int) bool {
			return graph.allAdjacency[fromID][left].key < graph.allAdjacency[fromID][right].key
		})
	}
	graph.globalFrontier = make([]FrontierReason, 0, len(frontier))
	for reason := range frontier {
		graph.globalFrontier = append(graph.globalFrontier, reason)
	}
	sort.Slice(graph.globalFrontier, func(left, right int) bool {
		return graph.globalFrontier[left] < graph.globalFrontier[right]
	})
	return graph, nil
}

func (graph graphCompilation) compileFrontierIndex(search searchResult) frontierIndex {
	byCaller := make(map[string]map[FrontierReason]struct{})
	for _, boundary := range graph.boundaries {
		_, fromReached := search.rootByNode[boundary.fromID]
		_, toReached := search.rootByNode[boundary.toID]
		if fromReached == toReached {
			continue
		}
		callerID := boundary.toID
		if toReached {
			callerID = boundary.fromID
		}
		if byCaller[callerID] == nil {
			byCaller[callerID] = make(map[FrontierReason]struct{})
		}
		byCaller[callerID][boundary.reason] = struct{}{}
	}
	result := frontierIndex{
		global:   append([]FrontierReason(nil), graph.globalFrontier...),
		byCaller: make(map[string][]FrontierReason, len(byCaller)),
	}
	for callerID, reasons := range byCaller {
		result.byCaller[callerID] = canonicalFrontierSet(reasons)
	}
	return result
}

func (index frontierIndex) snapshot(callerID string) []FrontierReason {
	values := make(map[FrontierReason]struct{}, len(index.global)+len(index.byCaller[callerID]))
	for _, reason := range index.global {
		values[reason] = struct{}{}
	}
	for _, reason := range index.byCaller[callerID] {
		values[reason] = struct{}{}
	}
	return canonicalFrontierSet(values)
}

func canonicalFrontierSet(values map[FrontierReason]struct{}) []FrontierReason {
	result := make([]FrontierReason, 0, len(values))
	for reason := range values {
		result = append(result, reason)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func edgeAuthority(relation programindex.Relation) EdgeAuthority {
	if relation.Kind == programindex.RelationPassesCallback {
		return EdgePossible
	}
	if relation.Resolution == programindex.ResolutionExact {
		return EdgeExact
	}
	return EdgePossible
}

func projectedEdgeKey(edge projectedEdge, objects map[string]programindex.Object) string {
	targetKey := edge.step.ToID
	if target, ok := objects[edge.step.ToID]; ok {
		targetKey = objectKey(target)
	}
	return targetKey + "\x00" + string(edge.step.Kind) + "\x00" + string(edge.step.Resolution) +
		"\x00" + locationKey(edge.step.Location) + "\x00" + edge.step.Invocation +
		"\x00" + edge.step.RelationID + "\x00" + edge.step.ToID
}

func search(
	activities []programindex.Object,
	adjacency map[string][]projectedEdge,
) searchResult {
	orderedRoots := make([]programindex.Object, len(activities))
	for position := range activities {
		orderedRoots[position] = cloneObject(activities[position])
	}
	sort.Slice(orderedRoots, func(left, right int) bool {
		return objectKey(orderedRoots[left]) < objectKey(orderedRoots[right])
	})
	result := searchResult{
		rootByNode: make(map[string]string),
		previous:   make(map[string]predecessor),
	}
	states := make(map[string]pathState)
	current := make([]string, 0, len(orderedRoots))
	for rootRank, root := range orderedRoots {
		if _, duplicate := result.rootByNode[root.ID]; duplicate {
			continue
		}
		result.rootByNode[root.ID] = root.ID
		states[root.ID] = pathState{rootID: root.ID, rootRank: rootRank}
		current = append(current, root.ID)
	}
	for len(current) > 0 {
		bestByTarget := make(map[string]pathCandidate)
		for _, fromID := range current {
			state := states[fromID]
			for _, edge := range adjacency[fromID] {
				toID := edge.step.ToID
				if _, seen := result.rootByNode[toID]; seen {
					continue
				}
				candidate := pathCandidate{
					toID: toID, fromID: fromID, edge: edge,
					rootID: state.rootID, rootRank: state.rootRank,
					possibleSteps:    state.possibleSteps,
					callbackHandoffs: state.callbackHandoffs,
					previousRank:     state.sequenceRank,
				}
				if edge.step.Authority == EdgePossible {
					candidate.possibleSteps++
				}
				if edge.step.Kind == programindex.RelationPassesCallback {
					candidate.callbackHandoffs++
				}
				previous, exists := bestByTarget[toID]
				if !exists || pathCandidateLess(candidate, previous) {
					bestByTarget[toID] = candidate
				}
			}
		}
		if len(bestByTarget) == 0 {
			break
		}
		winners := make([]pathCandidate, 0, len(bestByTarget))
		for _, candidate := range bestByTarget {
			winners = append(winners, candidate)
		}
		sort.Slice(winners, func(left, right int) bool {
			if winners[left].previousRank != winners[right].previousRank {
				return winners[left].previousRank < winners[right].previousRank
			}
			if winners[left].edge.key != winners[right].edge.key {
				return winners[left].edge.key < winners[right].edge.key
			}
			return winners[left].toID < winners[right].toID
		})
		next := make([]string, 0, len(winners))
		sequenceRank := -1
		previousRank := -1
		previousEdgeKey := ""
		for _, winner := range winners {
			if sequenceRank < 0 || winner.previousRank != previousRank || winner.edge.key != previousEdgeKey {
				sequenceRank++
				previousRank = winner.previousRank
				previousEdgeKey = winner.edge.key
			}
			result.rootByNode[winner.toID] = winner.rootID
			result.previous[winner.toID] = predecessor{fromID: winner.fromID, edge: winner.edge}
			states[winner.toID] = pathState{
				rootID: winner.rootID, rootRank: winner.rootRank,
				possibleSteps: winner.possibleSteps, callbackHandoffs: winner.callbackHandoffs,
				sequenceRank: sequenceRank,
			}
			next = append(next, winner.toID)
		}
		current = next
	}
	return result
}

func pathCandidateLess(left, right pathCandidate) bool {
	if left.possibleSteps != right.possibleSteps {
		return left.possibleSteps < right.possibleSteps
	}
	if left.callbackHandoffs != right.callbackHandoffs {
		return left.callbackHandoffs < right.callbackHandoffs
	}
	if left.rootRank != right.rootRank {
		return left.rootRank < right.rootRank
	}
	if left.previousRank != right.previousRank {
		return left.previousRank < right.previousRank
	}
	if left.edge.key != right.edge.key {
		return left.edge.key < right.edge.key
	}
	return left.fromID < right.fromID
}

func buildRoute(
	prepared inputs,
	paths searchResult,
	frontiers frontierIndex,
	caller programindex.Object,
) (Route, error) {
	route := Route{
		ID: routeIdentity(prepared.index.SHA256, prepared.activitiesSHA, caller.ID), Caller: cloneObject(caller),
		Nodes: []programindex.Object{}, Steps: []Step{}, Frontier: []FrontierReason{},
	}
	if _, found := paths.rootByNode[caller.ID]; found {
		nodes, steps, activity, err := restoreRoute(prepared.objects, paths, caller.ID)
		if err != nil {
			return Route{}, err
		}
		route.Activity = &activity
		route.Nodes = nodes
		route.Steps = steps
		setRouteMeasurements(&route)
		if route.PossibleSteps == 0 {
			route.Status = StatusExact
		} else {
			route.Status = StatusPossible
		}
		return route, nil
	}
	frontier := frontiers.snapshot(caller.ID)
	if len(frontier) > 0 {
		route.Status = StatusFrontier
		route.Frontier = frontier
		return route, nil
	}
	route.Status = StatusUnconnected
	return route, nil
}

func restoreRoute(
	objects map[string]programindex.Object,
	search searchResult,
	callerID string,
) ([]programindex.Object, []Step, programindex.Object, error) {
	rootID, ok := search.rootByNode[callerID]
	if !ok {
		return nil, nil, programindex.Object{}, fmt.Errorf("activity path: caller is absent from retained search")
	}
	reverseIDs := []string{callerID}
	reverseSteps := make([]Step, 0)
	current := callerID
	for current != rootID {
		if len(reverseSteps) >= MaxRouteSteps {
			return nil, nil, programindex.Object{}, fmt.Errorf(
				"activity path: route to %q exceeds %d steps", callerID, MaxRouteSteps,
			)
		}
		previous, exists := search.previous[current]
		if !exists || previous.edge.step.ToID != current || previous.edge.step.FromID != previous.fromID {
			return nil, nil, programindex.Object{}, fmt.Errorf("activity path: broken retained predecessor chain")
		}
		reverseSteps = append(reverseSteps, cloneStep(previous.edge.step))
		current = previous.fromID
		reverseIDs = append(reverseIDs, current)
	}
	nodes := make([]programindex.Object, len(reverseIDs))
	for position := range reverseIDs {
		id := reverseIDs[len(reverseIDs)-1-position]
		object, exists := objects[id]
		if !exists {
			return nil, nil, programindex.Object{}, fmt.Errorf("activity path: retained route cites unknown object %q", id)
		}
		nodes[position] = cloneObject(object)
	}
	steps := make([]Step, len(reverseSteps))
	for position := range reverseSteps {
		steps[position] = cloneStep(reverseSteps[len(reverseSteps)-1-position])
	}
	activity, exists := objects[rootID]
	if !exists {
		return nil, nil, programindex.Object{}, fmt.Errorf("activity path: retained route has unknown activity root")
	}
	return nodes, steps, cloneObject(activity), nil
}

func setRouteMeasurements(route *Route) {
	route.Distance = len(route.Steps)
	for _, step := range route.Steps {
		if step.Authority == EdgePossible {
			route.PossibleSteps++
		}
		if step.Kind == programindex.RelationPassesCallback {
			route.CallbackHandoffs++
		}
	}
}

func locationKey(location *programindex.Location) string {
	if location == nil {
		return ""
	}
	return fmt.Sprintf("%s:%09d:%09d", location.Path, location.Line, location.Column)
}

func cloneLocation(value *programindex.Location) *programindex.Location {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
