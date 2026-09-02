package groupmatching

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

type targetAuthority struct {
	ref   string
	index groupindex.Index
}

type groupAuthority struct {
	ref       string
	targetRef string
	targetID  string
	group     groupindex.Group
}

type subjectAuthority struct {
	ref       string
	targetRef string
	targetID  string
	subject   groupindex.Subject
}

type argumentAuthority struct {
	ref             string
	targetID        string
	ownerSubjectRef string
	argument        groupindex.PatternArgument
}

type valueCandidateAuthority struct {
	ref             string
	targetID        string
	ownerSubjectRef string
	candidate       groupindex.PatternValueCandidate
}

type edgeAuthority struct {
	ref       string
	targetRef string
	targetID  string
	edge      groupindex.StructuralEdge
}

type localConnectionAuthority struct {
	ref        string
	connection groupindex.Connection
}

type boundaryEdgeBasis string

const (
	boundaryEdgeExactExternal   boundaryEdgeBasis = "exact_external"
	boundaryEdgeSemanticTrigger boundaryEdgeBasis = "semantic_trigger"
)

type boundaryEdgeAuthority struct {
	edgeRef     string
	patternRef  string
	externalRef string
	basis       boundaryEdgeBasis
	resolution  programindex.PatternValueResolution
}

type pairAuthority struct {
	ref                      string
	leftGroupRef             string
	rightGroupRef            string
	targetRefs               map[string]struct{}
	subjectRefs              map[string]struct{}
	edgeRefs                 map[string]struct{}
	connectionRefs           map[string]struct{}
	leftEvidenceRefs         map[string]struct{}
	rightEvidenceRefs        map[string]struct{}
	leftBoundaryEdges        map[string]boundaryEdgeAuthority
	rightBoundaryEdges       map[string]boundaryEdgeAuthority
	leftBoundaryPatternRefs  map[string]struct{}
	rightBoundaryPatternRefs map[string]struct{}
	witnessCandidates        []witnessCandidateAuthority
	witnessByRef             map[string]witnessCandidateAuthority
}

// Compilation is the complete request-local dictionary for one validated
// GroupsIndex set. Canonical IDs remain only in these restoration tables.
type Compilation struct {
	input                   []groupindex.Index
	targets                 []targetAuthority
	groups                  []groupAuthority
	groupByRef              map[string]groupAuthority
	groupRefByEndpoint      map[string]string
	groupsByTargetRef       map[string][]groupAuthority
	subjects                []subjectAuthority
	subjectByRef            map[string]subjectAuthority
	subjectRefByEndpoint    map[string]string
	arguments               []argumentAuthority
	argumentByRef           map[string]argumentAuthority
	argumentsByOwnerRef     map[string][]argumentAuthority
	argumentRefByEndpoint   map[string]string
	argumentOwnerByEndpoint map[string]string
	valueCandidates         []valueCandidateAuthority
	valueRefByEndpoint      map[string]string
	valueOwnerByEndpoint    map[string]string
	edges                   []edgeAuthority
	edgeByRef               map[string]edgeAuthority
	localConnections        []localConnectionAuthority
	boundaryEdgesByGroupRef map[string]map[string]boundaryEdgeAuthority
	pairs                   []pairAuthority
	pairByRef               map[string]pairAuthority
}

// Compile validates the complete set, snapshots it, assigns deterministic
// refs, and constructs every unordered cross-target group pair exactly once.
func Compile(indexes []groupindex.Index) (Compilation, error) {
	if err := groupindex.ValidateSet(indexes); err != nil {
		return Compilation{}, fmt.Errorf("group matching: GroupsIndex set: %w", err)
	}
	compilation := Compilation{
		input:                   make([]groupindex.Index, len(indexes)),
		groupByRef:              make(map[string]groupAuthority),
		groupRefByEndpoint:      make(map[string]string),
		groupsByTargetRef:       make(map[string][]groupAuthority),
		subjectByRef:            make(map[string]subjectAuthority),
		subjectRefByEndpoint:    make(map[string]string),
		argumentRefByEndpoint:   make(map[string]string),
		argumentByRef:           make(map[string]argumentAuthority),
		argumentsByOwnerRef:     make(map[string][]argumentAuthority),
		argumentOwnerByEndpoint: make(map[string]string),
		valueRefByEndpoint:      make(map[string]string),
		valueOwnerByEndpoint:    make(map[string]string),
		edgeByRef:               make(map[string]edgeAuthority),
		boundaryEdgesByGroupRef: make(map[string]map[string]boundaryEdgeAuthority),
		pairByRef:               make(map[string]pairAuthority),
	}
	for position, index := range indexes {
		compilation.input[position] = index.Snapshot()
		compilation.targets = append(compilation.targets, targetAuthority{index: index.Snapshot()})
	}
	sort.Slice(compilation.targets, func(i, j int) bool {
		return compilation.targets[i].index.Target.ID < compilation.targets[j].index.Target.ID
	})
	for position := range compilation.targets {
		compilation.targets[position].ref = "t" + strconv.Itoa(position+1)
	}

	for _, target := range compilation.targets {
		for _, group := range target.index.Groups {
			compilation.groups = append(compilation.groups, groupAuthority{
				targetRef: target.ref, targetID: target.index.Target.ID, group: group,
			})
		}
		for _, subject := range target.index.Subjects {
			compilation.subjects = append(compilation.subjects, subjectAuthority{
				targetRef: target.ref, targetID: target.index.Target.ID, subject: subject,
			})
		}
		for _, edge := range target.index.StructuralEdges {
			compilation.edges = append(compilation.edges, edgeAuthority{
				targetRef: target.ref, targetID: target.index.Target.ID, edge: edge,
			})
		}
		for _, connection := range target.index.Connections {
			if connection.From.TargetID != target.index.Target.ID ||
				connection.To.TargetID != target.index.Target.ID {
				continue
			}
			// "Local connection" includes its evidence authority: a same-target
			// edge backed by a foreign subject is already cross-target output and
			// must not be replayed to the matcher as local context.
			localEvidence := true
			for _, evidence := range connection.Evidence {
				if evidence.TargetID != target.index.Target.ID {
					localEvidence = false
					break
				}
			}
			if !localEvidence {
				continue
			}
			compilation.localConnections = append(compilation.localConnections, localConnectionAuthority{
				connection: connection,
			})
		}
	}
	sort.Slice(compilation.groups, func(i, j int) bool {
		return groupEndpointKey(compilation.groups[i].targetID, compilation.groups[i].group.ID) <
			groupEndpointKey(compilation.groups[j].targetID, compilation.groups[j].group.ID)
	})
	for position := range compilation.groups {
		compilation.groups[position].ref = "g" + strconv.Itoa(position+1)
		group := compilation.groups[position]
		compilation.groupByRef[group.ref] = group
		compilation.groupRefByEndpoint[groupEndpointKey(group.targetID, group.group.ID)] = group.ref
		compilation.groupsByTargetRef[group.targetRef] = append(compilation.groupsByTargetRef[group.targetRef], group)
	}
	sort.Slice(compilation.subjects, func(i, j int) bool {
		return subjectEndpointKey(compilation.subjects[i].targetID, compilation.subjects[i].subject.ID) <
			subjectEndpointKey(compilation.subjects[j].targetID, compilation.subjects[j].subject.ID)
	})
	for position := range compilation.subjects {
		compilation.subjects[position].ref = "s" + strconv.Itoa(position+1)
		subject := compilation.subjects[position]
		compilation.subjectByRef[subject.ref] = subject
		compilation.subjectRefByEndpoint[subjectEndpointKey(subject.targetID, subject.subject.ID)] = subject.ref
	}
	for _, subject := range compilation.subjects {
		if subject.subject.Pattern == nil {
			continue
		}
		for _, argument := range subject.subject.Pattern.Arguments {
			compilation.arguments = append(compilation.arguments, argumentAuthority{
				targetID: subject.targetID, ownerSubjectRef: subject.ref, argument: argument,
			})
			for _, candidate := range argument.ValueCandidates {
				compilation.valueCandidates = append(compilation.valueCandidates, valueCandidateAuthority{
					targetID: subject.targetID, ownerSubjectRef: subject.ref, candidate: candidate,
				})
			}
		}
	}
	sort.Slice(compilation.arguments, func(i, j int) bool {
		return nestedEndpointKey(compilation.arguments[i].targetID, compilation.arguments[i].argument.ID) <
			nestedEndpointKey(compilation.arguments[j].targetID, compilation.arguments[j].argument.ID)
	})
	for position := range compilation.arguments {
		compilation.arguments[position].ref = "a" + strconv.Itoa(position+1)
		authority := compilation.arguments[position]
		key := nestedEndpointKey(authority.targetID, authority.argument.ID)
		compilation.argumentRefByEndpoint[key] = authority.ref
		compilation.argumentOwnerByEndpoint[key] = authority.ownerSubjectRef
		compilation.argumentByRef[authority.ref] = authority
		compilation.argumentsByOwnerRef[authority.ownerSubjectRef] = append(
			compilation.argumentsByOwnerRef[authority.ownerSubjectRef],
			authority,
		)
	}
	sort.Slice(compilation.valueCandidates, func(i, j int) bool {
		return nestedEndpointKey(compilation.valueCandidates[i].targetID, compilation.valueCandidates[i].candidate.ID) <
			nestedEndpointKey(compilation.valueCandidates[j].targetID, compilation.valueCandidates[j].candidate.ID)
	})
	for position := range compilation.valueCandidates {
		compilation.valueCandidates[position].ref = "v" + strconv.Itoa(position+1)
		authority := compilation.valueCandidates[position]
		key := nestedEndpointKey(authority.targetID, authority.candidate.ID)
		compilation.valueRefByEndpoint[key] = authority.ref
		compilation.valueOwnerByEndpoint[key] = authority.ownerSubjectRef
	}
	sort.Slice(compilation.edges, func(i, j int) bool {
		return structuralEdgeKey(compilation.edges[i]) < structuralEdgeKey(compilation.edges[j])
	})
	for position := range compilation.edges {
		compilation.edges[position].ref = "e" + strconv.Itoa(position+1)
		compilation.edgeByRef[compilation.edges[position].ref] = compilation.edges[position]
	}
	sort.Slice(compilation.localConnections, func(i, j int) bool {
		return localConnectionKey(compilation.localConnections[i].connection) <
			localConnectionKey(compilation.localConnections[j].connection)
	})
	for position := range compilation.localConnections {
		compilation.localConnections[position].ref = "c" + strconv.Itoa(position+1)
	}
	for _, group := range compilation.groups {
		compilation.boundaryEdgesByGroupRef[group.ref] = compilation.compileGroupBoundaryEdges(group)
	}

	for leftTargetPosition := 0; leftTargetPosition < len(compilation.targets); leftTargetPosition++ {
		leftTarget := compilation.targets[leftTargetPosition]
		for rightTargetPosition := leftTargetPosition + 1; rightTargetPosition < len(compilation.targets); rightTargetPosition++ {
			rightTarget := compilation.targets[rightTargetPosition]
			for _, leftGroup := range compilation.groupsByTargetRef[leftTarget.ref] {
				for _, rightGroup := range compilation.groupsByTargetRef[rightTarget.ref] {
					pair := compilation.compilePair(leftGroup, rightGroup)
					pair.ref = "p" + strconv.Itoa(len(compilation.pairs)+1)
					compilation.pairs = append(compilation.pairs, pair)
					compilation.pairByRef[pair.ref] = pair
				}
			}
		}
	}
	return compilation, nil
}

func (compilation Compilation) compilePair(left, right groupAuthority) pairAuthority {
	leftBoundaryEdges := compilation.boundaryEdgesByGroupRef[left.ref]
	rightBoundaryEdges := compilation.boundaryEdgesByGroupRef[right.ref]
	pair := pairAuthority{
		leftGroupRef: left.ref, rightGroupRef: right.ref,
		targetRefs:  map[string]struct{}{left.targetRef: {}, right.targetRef: {}},
		subjectRefs: make(map[string]struct{}), edgeRefs: make(map[string]struct{}),
		connectionRefs:           make(map[string]struct{}),
		leftEvidenceRefs:         make(map[string]struct{}),
		rightEvidenceRefs:        make(map[string]struct{}),
		leftBoundaryEdges:        leftBoundaryEdges,
		rightBoundaryEdges:       rightBoundaryEdges,
		leftBoundaryPatternRefs:  boundaryPatternRefs(leftBoundaryEdges),
		rightBoundaryPatternRefs: boundaryPatternRefs(rightBoundaryEdges),
	}
	// The model decides one exact pair from a pair-local dossier. Its selectable
	// evidence is the complete member/evidence cover of the two endpoint groups;
	// unrelated subjects from either target never become evidence authority.
	compilation.addGroupEvidenceRefs(left, pair.leftEvidenceRefs)
	compilation.addGroupEvidenceRefs(right, pair.rightEvidenceRefs)
	unionRefSets(pair.subjectRefs, pair.leftEvidenceRefs)
	unionRefSets(pair.subjectRefs, pair.rightEvidenceRefs)
	unionRefSets(pair.subjectRefs, pair.leftBoundaryPatternRefs)
	unionRefSets(pair.subjectRefs, pair.rightBoundaryPatternRefs)
	compilation.addBoundaryEdges(pair.edgeRefs, pair.subjectRefs, pair.leftBoundaryEdges)
	compilation.addBoundaryEdges(pair.edgeRefs, pair.subjectRefs, pair.rightBoundaryEdges)

	// Local semantic context is useful only when it touches an endpoint group.
	// Keep every such connection and its exact evidence, while its neighboring
	// group is represented compactly by localConnectionWire rather than pulling
	// that group's complete membership into this dossier.
	for _, connection := range compilation.localConnections {
		fromRef := compilation.groupRefByEndpoint[groupEndpointKey(
			connection.connection.From.TargetID, connection.connection.From.GroupID,
		)]
		toRef := compilation.groupRefByEndpoint[groupEndpointKey(
			connection.connection.To.TargetID, connection.connection.To.GroupID,
		)]
		if fromRef != left.ref && fromRef != right.ref && toRef != left.ref && toRef != right.ref {
			continue
		}
		pair.connectionRefs[connection.ref] = struct{}{}
		for _, evidence := range connection.connection.Evidence {
			compilation.addSubjectEndpointRef(pair.subjectRefs, evidence.TargetID, evidence.SubjectID)
		}
	}

	// Retain the complete one-hop structural incidence of the endpoint groups'
	// member/evidence subjects. Counterpart subjects and nested provenance owners
	// are then added so every retained edge has closed request-local refs. This is
	// an identity projection, not a semantic candidate heuristic, and it never
	// recursively walks unrelated program edges.
	incidentSeeds := make(map[string]struct{}, len(pair.leftEvidenceRefs)+len(pair.rightEvidenceRefs)+
		len(pair.leftBoundaryPatternRefs)+len(pair.rightBoundaryPatternRefs))
	unionRefSets(incidentSeeds, pair.leftEvidenceRefs)
	unionRefSets(incidentSeeds, pair.rightEvidenceRefs)
	unionRefSets(incidentSeeds, pair.leftBoundaryPatternRefs)
	unionRefSets(incidentSeeds, pair.rightBoundaryPatternRefs)
	for _, edge := range compilation.edges {
		fromRef := compilation.subjectRefByEndpoint[subjectEndpointKey(edge.targetID, edge.edge.FromSubjectID)]
		toRef := compilation.subjectRefByEndpoint[subjectEndpointKey(edge.targetID, edge.edge.ToSubjectID)]
		_, fromIncident := incidentSeeds[fromRef]
		_, toIncident := incidentSeeds[toRef]
		if !fromIncident && !toIncident {
			continue
		}
		pair.edgeRefs[edge.ref] = struct{}{}
		addNonEmptyRef(pair.subjectRefs, fromRef)
		addNonEmptyRef(pair.subjectRefs, toRef)
		addNonEmptyRef(pair.subjectRefs, compilation.argumentOwnerByEndpoint[nestedEndpointKey(edge.targetID, edge.edge.ArgumentID)])
		addNonEmptyRef(pair.subjectRefs, compilation.valueOwnerByEndpoint[nestedEndpointKey(edge.targetID, edge.edge.ValueCandidateID)])
		addNonEmptyRef(pair.subjectRefs, compilation.argumentOwnerByEndpoint[nestedEndpointKey(edge.targetID, edge.edge.SourceArgumentID)])
	}
	compilation.closeSubjectRefs(pair.subjectRefs)
	pair.witnessCandidates, pair.witnessByRef = compilation.compileWitnessCandidates(pair)
	return pair
}

// compileWitnessCandidates enumerates every joint that could survive local
// normalization for this exact pair. The rows are request-local evidence
// candidates only: they create no semantic connection until selected by the
// model and restored through the same validator.
func (compilation Compilation) compileWitnessCandidates(
	pair pairAuthority,
) ([]witnessCandidateAuthority, map[string]witnessCandidateAuthority) {
	leftEdgeRefs := sortedBoundaryEdgeRefs(pair.leftBoundaryEdges)
	rightEdgeRefs := sortedBoundaryEdgeRefs(pair.rightBoundaryEdges)
	byKey := make(map[string]witnessJoint)
	for _, leftEdgeRef := range leftEdgeRefs {
		leftBoundary := pair.leftBoundaryEdges[leftEdgeRef]
		leftArguments := compilation.argumentsByOwnerRef[leftBoundary.patternRef]
		for _, rightEdgeRef := range rightEdgeRefs {
			rightBoundary := pair.rightBoundaryEdges[rightEdgeRef]
			rightArguments := compilation.argumentsByOwnerRef[rightBoundary.patternRef]
			for _, leftArgument := range leftArguments {
				for _, rightArgument := range rightArguments {
					row := responseWitnessJoint{
						Kind:                witnessJointArgumentValue,
						LeftBoundaryEdgeRef: leftEdgeRef, LeftArgumentRef: leftArgument.ref,
						RightBoundaryEdgeRef: rightEdgeRef, RightArgumentRef: rightArgument.ref,
					}
					joint, _, accepted := compilation.normalizeWitnessJoint(row, pair)
					if accepted {
						byKey[witnessJointKey(joint)] = joint
					}
				}
			}
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]witnessCandidateAuthority, 0, len(keys))
	byRef := make(map[string]witnessCandidateAuthority, len(keys))
	for position, key := range keys {
		joint := byKey[key]
		requiredFromGroupRef, requiredToGroupRef := compilation.requiredWitnessDirection(joint, pair)
		authority := witnessCandidateAuthority{
			ref: "j" + strconv.Itoa(position+1), joint: joint,
			requiredFromGroupRef: requiredFromGroupRef, requiredToGroupRef: requiredToGroupRef,
		}
		result = append(result, authority)
		byRef[authority.ref] = authority
	}
	if result == nil {
		result = []witnessCandidateAuthority{}
	}
	return result, byRef
}

// requiredWitnessDirection records only the narrow direction already proven
// by positive graph authority: an inbound delivery boundary receives work
// from an exact outbound dependency call. It does not generalize trigger-lane
// membership into direction; background activities and other shapes remain
// model-directed.
func (compilation Compilation) requiredWitnessDirection(
	joint witnessJoint,
	pair pairAuthority,
) (string, string) {
	leftBoundary, leftKnown := pair.leftBoundaryEdges[joint.leftBoundaryEdgeRef]
	rightBoundary, rightKnown := pair.rightBoundaryEdges[joint.rightBoundaryEdgeRef]
	if !leftKnown || !rightKnown {
		return "", ""
	}
	leftInbound := compilation.isInboundDeliveryBoundary(pair.leftGroupRef, leftBoundary)
	rightInbound := compilation.isInboundDeliveryBoundary(pair.rightGroupRef, rightBoundary)
	leftOutbound := compilation.isExactOutboundDependencyBoundary(leftBoundary)
	rightOutbound := compilation.isExactOutboundDependencyBoundary(rightBoundary)
	switch {
	case leftInbound && rightOutbound:
		return pair.rightGroupRef, pair.leftGroupRef
	case rightInbound && leftOutbound:
		return pair.leftGroupRef, pair.rightGroupRef
	default:
		return "", ""
	}
}

func (compilation Compilation) isInboundDeliveryBoundary(
	groupRef string,
	boundary boundaryEdgeAuthority,
) bool {
	group, groupKnown := compilation.groupByRef[groupRef]
	pattern, patternKnown := compilation.subjectByRef[boundary.patternRef]
	if !groupKnown || !patternKnown || pattern.subject.Pattern == nil ||
		group.group.Lane != groupindex.LaneTriggers ||
		boundary.basis != boundaryEdgeSemanticTrigger {
		return false
	}
	return hasCategory(pattern.subject.Categories, programindex.CategoryInbound) &&
		!hasCategory(pattern.subject.Categories, programindex.CategoryDependency)
}

func (compilation Compilation) isExactOutboundDependencyBoundary(
	boundary boundaryEdgeAuthority,
) bool {
	pattern, known := compilation.subjectByRef[boundary.patternRef]
	if !known || pattern.subject.Pattern == nil ||
		boundary.basis != boundaryEdgeExactExternal ||
		boundary.resolution != programindex.PatternValueExact ||
		pattern.subject.Pattern.RelationKind != programindex.RelationInvokesExternal {
		return false
	}
	return hasCategory(pattern.subject.Categories, programindex.CategoryDependency) &&
		!hasTriggerCategory(pattern.subject.Categories)
}

func hasCategory(categories []programindex.Category, expected programindex.Category) bool {
	for _, category := range categories {
		if category == expected {
			return true
		}
	}
	return false
}

func sortedBoundaryEdgeRefs(values map[string]boundaryEdgeAuthority) []string {
	result := make([]string, 0, len(values))
	for ref := range values {
		result = append(result, ref)
	}
	sort.Strings(result)
	return result
}

func boundaryPatternRefs(edges map[string]boundaryEdgeAuthority) map[string]struct{} {
	result := make(map[string]struct{})
	for _, authority := range edges {
		result[authority.patternRef] = struct{}{}
	}
	return result
}

func (compilation Compilation) addBoundaryEdges(
	edgeRefs map[string]struct{},
	subjectRefs map[string]struct{},
	edges map[string]boundaryEdgeAuthority,
) {
	for _, authority := range edges {
		edgeRefs[authority.edgeRef] = struct{}{}
		addNonEmptyRef(subjectRefs, authority.patternRef)
		addNonEmptyRef(subjectRefs, authority.externalRef)
	}
}

// compileGroupBoundaryEdges projects existing exact structural edges into a
// group-scoped matching dossier. It creates no new graph identity or semantic
// relation: every advertised ref remains one GroupsIndex StructuralEdge.
func (compilation Compilation) compileGroupBoundaryEdges(group groupAuthority) map[string]boundaryEdgeAuthority {
	result := make(map[string]boundaryEdgeAuthority)
	for _, edge := range compilation.edges {
		authority, accepted := compilation.boundaryEdgeForGroup(group, edge)
		if accepted {
			result[edge.ref] = authority
		}
	}
	return result
}

// boundaryEdgeForGroup is also the normalize-time defense-in-depth validator;
// callers never trust an advertised edge ref without rechecking its graph facts.
func (compilation Compilation) boundaryEdgeForGroup(
	group groupAuthority,
	edge edgeAuthority,
) (boundaryEdgeAuthority, bool) {
	if edge.targetID != group.targetID {
		return boundaryEdgeAuthority{}, false
	}
	switch edge.edge.Role {
	case groupindex.EdgePatternTarget, groupindex.EdgePatternReceiver, groupindex.EdgePatternReceiverOrigin:
	default:
		return boundaryEdgeAuthority{}, false
	}
	patternRef := compilation.subjectRefByEndpoint[subjectEndpointKey(edge.targetID, edge.edge.FromSubjectID)]
	externalRef := compilation.subjectRefByEndpoint[subjectEndpointKey(edge.targetID, edge.edge.ToSubjectID)]
	pattern, patternKnown := compilation.subjectByRef[patternRef]
	external, externalKnown := compilation.subjectByRef[externalRef]
	if !patternKnown || pattern.subject.Pattern == nil || !externalKnown || external.subject.Object == nil ||
		external.subject.Object.Kind != programindex.ObjectExternalSymbol ||
		!programindex.IsExternalPackageAuthority(external.subject.Object.External) ||
		!compilation.patternSourceOwnedByGroup(pattern, group) {
		return boundaryEdgeAuthority{}, false
	}
	facts := pattern.subject.Pattern
	roleMatches := false
	switch edge.edge.Role {
	case groupindex.EdgePatternTarget:
		roleMatches = containsString(facts.ToIDs, edge.edge.ToSubjectID) &&
			edge.edge.Resolution == facts.RelationResolution
	case groupindex.EdgePatternReceiver:
		roleMatches = facts.ReceiverID == edge.edge.ToSubjectID && edge.edge.Resolution == programindex.ResolutionExact
	case groupindex.EdgePatternReceiverOrigin:
		roleMatches = containsString(facts.ReceiverOriginIDs, edge.edge.ToSubjectID) &&
			edge.edge.Resolution == facts.ReceiverOriginResolution
	}
	if !roleMatches {
		return boundaryEdgeAuthority{}, false
	}
	authority := boundaryEdgeAuthority{
		edgeRef: edge.ref, patternRef: patternRef, externalRef: externalRef,
	}
	if edge.edge.Resolution == programindex.ResolutionExact {
		authority.basis = boundaryEdgeExactExternal
		authority.resolution = programindex.PatternValueExact
		return authority, true
	}
	if edge.edge.Role != groupindex.EdgePatternReceiverOrigin ||
		edge.edge.Resolution != programindex.ResolutionAlternatives ||
		group.group.Lane != groupindex.LaneTriggers ||
		len(facts.ReceiverOriginIDs) != 1 || facts.ReceiverOriginIDs[0] != edge.edge.ToSubjectID ||
		!hasTriggerCategory(pattern.subject.Categories) {
		return boundaryEdgeAuthority{}, false
	}
	authority.basis = boundaryEdgeSemanticTrigger
	authority.resolution = programindex.PatternValuePossible
	return authority, true
}

func hasTriggerCategory(categories []programindex.Category) bool {
	for _, category := range categories {
		if category == programindex.CategoryInbound || category == programindex.CategoryBackgroundActivity {
			return true
		}
	}
	return false
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

// patternSourceOwnedByGroup accepts only a finite exact OwnerID/ContainerID
// ancestry from the pattern's source object to a presentation member. It never
// walks calls, structural adjacency, group evidence, or semantic connections.
func (compilation Compilation) patternSourceOwnedByGroup(pattern subjectAuthority, group groupAuthority) bool {
	if pattern.targetID != group.targetID || pattern.subject.Pattern == nil {
		return false
	}
	members := make(map[string]struct{}, len(group.group.MemberSubjectIDs))
	for _, id := range group.group.MemberSubjectIDs {
		ref := compilation.subjectRefByEndpoint[subjectEndpointKey(group.targetID, id)]
		authority, known := compilation.subjectByRef[ref]
		if known && authority.subject.Object != nil && authority.subject.Object.External == nil &&
			authority.subject.Object.Kind != programindex.ObjectExternalSymbol {
			members[id] = struct{}{}
		}
	}
	pending := []string{pattern.subject.Pattern.FromID}
	visited := make(map[string]struct{})
	for len(pending) > 0 {
		id := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if id == "" {
			continue
		}
		if _, member := members[id]; member {
			return true
		}
		if _, seen := visited[id]; seen {
			continue
		}
		visited[id] = struct{}{}
		ref := compilation.subjectRefByEndpoint[subjectEndpointKey(group.targetID, id)]
		authority, known := compilation.subjectByRef[ref]
		if !known || authority.subject.Object == nil || authority.subject.Object.External != nil ||
			authority.subject.Object.Kind == programindex.ObjectExternalSymbol {
			continue
		}
		pending = append(pending, authority.subject.Object.OwnerID, authority.subject.Object.ContainerID)
	}
	return false
}

func (compilation Compilation) addGroupEvidenceRefs(group groupAuthority, destination map[string]struct{}) {
	for _, id := range group.group.MemberSubjectIDs {
		compilation.addSubjectEndpointRef(destination, group.targetID, id)
	}
	for _, id := range group.group.EvidenceSubjectIDs {
		compilation.addSubjectEndpointRef(destination, group.targetID, id)
	}
}

func (compilation Compilation) addSubjectEndpointRef(destination map[string]struct{}, targetID, subjectID string) {
	addNonEmptyRef(destination, compilation.subjectRefByEndpoint[subjectEndpointKey(targetID, subjectID)])
}

func addNonEmptyRef(destination map[string]struct{}, ref string) bool {
	if ref == "" {
		return false
	}
	if _, exists := destination[ref]; exists {
		return false
	}
	destination[ref] = struct{}{}
	return true
}

func (compilation Compilation) closeSubjectRefs(subjectRefs map[string]struct{}) {
	for {
		changed := false
		for _, authority := range compilation.subjects {
			if _, include := subjectRefs[authority.ref]; !include {
				continue
			}
			addID := func(id string) {
				if id == "" {
					return
				}
				changed = compilation.addSubjectReference(subjectRefs, authority.targetID, id) || changed
			}
			if authority.subject.Object != nil {
				addID(authority.subject.Object.OwnerID)
				addID(authority.subject.Object.ContainerID)
				continue
			}
			pattern := authority.subject.Pattern
			addID(pattern.FromID)
			for _, id := range pattern.ToIDs {
				addID(id)
			}
			addID(pattern.ResultID)
			addID(pattern.ReceiverID)
			for _, id := range pattern.ReceiverOriginIDs {
				addID(id)
			}
			for _, argument := range pattern.Arguments {
				for _, id := range argument.ObjectIDs {
					addID(id)
				}
				for _, candidate := range argument.ValueCandidates {
					for _, id := range candidate.SourceObjectIDs {
						addID(id)
					}
					for _, id := range candidate.SourceArgumentIDs {
						changed = addNonEmptyRef(subjectRefs, compilation.argumentOwnerByEndpoint[nestedEndpointKey(authority.targetID, id)]) || changed
					}
				}
			}
		}
		if !changed {
			return
		}
	}
}

func (compilation Compilation) addSubjectReference(destination map[string]struct{}, targetID, subjectID string) bool {
	return addNonEmptyRef(destination, compilation.subjectRefByEndpoint[subjectEndpointKey(targetID, subjectID)])
}

func groupEndpointKey(targetID, groupID string) string {
	return targetID + "\x00" + groupID
}

func subjectEndpointKey(targetID, subjectID string) string {
	return targetID + "\x00" + subjectID
}

func nestedEndpointKey(targetID, nestedID string) string {
	return targetID + "\x00" + nestedID
}

func structuralEdgeKey(value edgeAuthority) string {
	edge := value.edge
	return strings.Join([]string{
		value.targetID, edge.FromSubjectID, edge.ToSubjectID, string(edge.Role),
		edge.RelationID, string(edge.RelationKind), string(edge.Resolution),
		edge.ArgumentID, edge.ValueCandidateID, edge.SourceArgumentID,
		string(edge.ValueResolution), string(edge.ValueSourceKind),
	}, "\x00")
}

func localConnectionKey(value groupindex.Connection) string {
	fields := []string{
		value.From.TargetID, value.From.GroupID, value.To.TargetID, value.To.GroupID,
		value.SemanticKind, value.Label, value.Summary,
	}
	for _, evidence := range value.Evidence {
		fields = append(fields, evidence.TargetID, evidence.SubjectID)
	}
	return strings.Join(fields, "\x00")
}
