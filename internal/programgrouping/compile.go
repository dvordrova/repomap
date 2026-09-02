package programgrouping

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

type subjectKind string

const (
	subjectObject  subjectKind = "object"
	subjectPattern subjectKind = "pattern"
)

type subjectAuthority struct {
	ref        string
	id         string
	kind       subjectKind
	categories []programindex.Category
	object     *programindex.Object
	relation   *programindex.Relation
	pattern    *programindex.RelationPattern
}

type sourceArgumentAuthority struct {
	ref       string
	id        string
	patternID string
	position  int
	keyword   string
}

type graphEdge struct {
	ref               string
	role              string
	fromID            string
	toID              string
	relationID        string
	relationKind      programindex.RelationKind
	resolution        programindex.Resolution
	invocation        string
	location          *programindex.Location
	targetsObserved   int
	targetsOmitted    int
	witnesses         []programindex.Witness
	witnessesObserved int
	witnessesOmitted  int
	sourceArgument    *sourceArgumentAuthority
}

// Compilation owns the exact enriched ProgramIndex and its request-local
// subject/edge dictionary. Canonical ProgramIndex identities never cross the
// provider boundary.
type Compilation struct {
	index             programindex.Index
	subjects          []subjectAuthority
	subjectByRef      map[string]subjectAuthority
	refBySubjectID    map[string]string
	arguments         []sourceArgumentAuthority
	argumentByID      map[string]sourceArgumentAuthority
	refByArgumentID   map[string]string
	categorizedRefs   []string
	categorizedRefSet map[string]struct{}
	edges             []graphEdge
}

// Compile validates one enriched ProgramIndex and projects all objects,
// relation patterns, and exact structural joints into stable request-local
// refs. Unclassified subjects stay available only as context/evidence.
func Compile(index programindex.Index) (Compilation, error) {
	if err := index.Validate(); err != nil {
		return Compilation{}, fmt.Errorf("program grouping: ProgramIndex: %w", err)
	}
	if index.Categorization == nil {
		return Compilation{}, fmt.Errorf("program grouping: ProgramIndex is not categorized")
	}
	compilation := Compilation{
		index:             index.Snapshot(),
		subjectByRef:      make(map[string]subjectAuthority),
		refBySubjectID:    make(map[string]string),
		argumentByID:      make(map[string]sourceArgumentAuthority),
		refByArgumentID:   make(map[string]string),
		categorizedRefSet: make(map[string]struct{}),
	}
	categoriesByID := make(map[string][]programindex.Category, len(index.Categorization.Assignments))
	for _, assignment := range index.Categorization.Assignments {
		categoriesByID[assignment.SubjectID] = append([]programindex.Category(nil), assignment.Categories...)
	}

	for position := range compilation.index.Objects {
		object := compilation.index.Objects[position]
		objectCopy := object
		compilation.subjects = append(compilation.subjects, subjectAuthority{
			id: object.ID, kind: subjectObject,
			categories: append([]programindex.Category(nil), categoriesByID[object.ID]...),
			object:     &objectCopy,
		})
	}
	for relationPosition := range compilation.index.Relations {
		relation := compilation.index.Relations[relationPosition]
		for patternPosition := range relation.Patterns {
			pattern := relation.Patterns[patternPosition]
			relationCopy := relation
			patternCopy := pattern
			compilation.subjects = append(compilation.subjects, subjectAuthority{
				id: pattern.ID, kind: subjectPattern,
				categories: append([]programindex.Category(nil), categoriesByID[pattern.ID]...),
				relation:   &relationCopy, pattern: &patternCopy,
			})
		}
	}
	sort.Slice(compilation.subjects, func(i, j int) bool {
		return compilation.subjects[i].id < compilation.subjects[j].id
	})
	for position := range compilation.subjects {
		compilation.subjects[position].ref = "s" + strconv.Itoa(position+1)
		subject := compilation.subjects[position]
		compilation.subjectByRef[subject.ref] = subject
		compilation.refBySubjectID[subject.id] = subject.ref
		if len(subject.categories) > 0 {
			compilation.categorizedRefs = append(compilation.categorizedRefs, subject.ref)
			compilation.categorizedRefSet[subject.ref] = struct{}{}
		}
	}
	for _, relation := range compilation.index.Relations {
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				compilation.arguments = append(compilation.arguments, sourceArgumentAuthority{
					id: argument.ID, patternID: pattern.ID, position: argument.Position, keyword: argument.Keyword,
				})
			}
		}
	}
	sort.Slice(compilation.arguments, func(i, j int) bool {
		return compilation.arguments[i].id < compilation.arguments[j].id
	})
	for position := range compilation.arguments {
		compilation.arguments[position].ref = "a" + strconv.Itoa(position+1)
		authority := compilation.arguments[position]
		compilation.argumentByID[authority.id] = authority
		compilation.refByArgumentID[authority.id] = authority.ref
	}
	compilation.compileEdges()
	return compilation, nil
}

func (compilation *Compilation) compileEdges() {
	for _, object := range compilation.index.Objects {
		if object.OwnerID != "" {
			compilation.addEdge(graphEdge{
				role: "object_owner", fromID: object.OwnerID, toID: object.ID,
				resolution: programindex.ResolutionExact, location: cloneLocation(object.Location),
			})
		}
		if object.ContainerID != "" {
			compilation.addEdge(graphEdge{
				role: "object_container", fromID: object.ContainerID, toID: object.ID,
				resolution: programindex.ResolutionExact, location: cloneLocation(object.Location),
			})
		}
	}
	for _, relation := range compilation.index.Relations {
		sourceArgument := sourceArgumentForRelation(relation, compilation.argumentByID)
		base := graphEdge{
			relationID: relation.ID, relationKind: relation.Kind, resolution: relation.Resolution,
			invocation: relation.Invocation, location: cloneLocation(relation.Location),
			targetsObserved: relation.TargetsObserved, targetsOmitted: relation.TargetsOmitted,
			witnesses:         cloneWitnesses(relation.Witnesses),
			witnessesObserved: relation.WitnessesObserved, witnessesOmitted: relation.WitnessesOmitted,
			sourceArgument: sourceArgument,
		}
		for _, targetID := range relation.ToIDs {
			edge := base
			edge.role = "relation_target"
			edge.fromID = relation.FromID
			edge.toID = targetID
			compilation.addEdge(edge)
		}
		for _, pattern := range relation.Patterns {
			location := pattern.Location
			if location == nil {
				location = relation.Location
			}
			edge := base
			edge.role = "relation_pattern"
			edge.fromID = relation.FromID
			edge.toID = pattern.ID
			edge.location = cloneLocation(location)
			compilation.addEdge(edge)
			for _, targetID := range relation.ToIDs {
				edge := base
				edge.role = "pattern_target"
				edge.fromID = pattern.ID
				edge.toID = targetID
				edge.location = cloneLocation(location)
				compilation.addEdge(edge)
			}
			if pattern.ResultID != "" {
				edge := base
				edge.role = "pattern_result"
				edge.fromID = pattern.ID
				edge.toID = pattern.ResultID
				edge.resolution = programindex.ResolutionExact
				edge.location = cloneLocation(location)
				compilation.addEdge(edge)
			}
			if pattern.ReceiverID != "" {
				edge := base
				edge.role = "pattern_receiver"
				edge.fromID = pattern.ID
				edge.toID = pattern.ReceiverID
				edge.resolution = programindex.ResolutionExact
				edge.location = cloneLocation(location)
				compilation.addEdge(edge)
			}
			for _, originID := range pattern.ReceiverOriginIDs {
				edge := base
				edge.role = "pattern_receiver_origin"
				edge.fromID = pattern.ID
				edge.toID = originID
				edge.resolution = pattern.ReceiverOriginResolution
				edge.location = cloneLocation(location)
				compilation.addEdge(edge)
			}
			for _, argument := range pattern.Arguments {
				for _, objectID := range argument.ObjectIDs {
					edge := base
					edge.role = "pattern_argument_object"
					edge.fromID = pattern.ID
					edge.toID = objectID
					edge.resolution = argument.Resolution
					edge.location = cloneLocation(location)
					compilation.addEdge(edge)
				}
			}
		}
	}
	sort.Slice(compilation.edges, func(i, j int) bool {
		return graphEdgeKey(compilation.edges[i]) < graphEdgeKey(compilation.edges[j])
	})
	write := 0
	for _, edge := range compilation.edges {
		if write > 0 && graphEdgeKey(compilation.edges[write-1]) == graphEdgeKey(edge) {
			continue
		}
		compilation.edges[write] = edge
		write++
	}
	compilation.edges = compilation.edges[:write]
	for position := range compilation.edges {
		compilation.edges[position].ref = "e" + strconv.Itoa(position+1)
	}
}

func (compilation *Compilation) addEdge(edge graphEdge) {
	if compilation.refBySubjectID[edge.fromID] == "" || compilation.refBySubjectID[edge.toID] == "" {
		return
	}
	compilation.edges = append(compilation.edges, edge)
}

func sourceArgumentForRelation(
	relation programindex.Relation,
	argumentsByID map[string]sourceArgumentAuthority,
) *sourceArgumentAuthority {
	if relation.SourceArgumentID == "" {
		return nil
	}
	argument, ok := argumentsByID[relation.SourceArgumentID]
	if !ok {
		return nil
	}
	copyValue := argument
	return &copyValue
}

func graphEdgeKey(edge graphEdge) string {
	location := ""
	if edge.location != nil {
		location = edge.location.Path + ":" + strconv.Itoa(edge.location.Line) + ":" + strconv.Itoa(edge.location.Column)
	}
	return strings.Join([]string{
		edge.role, edge.fromID, edge.toID, edge.relationID, string(edge.relationKind),
		string(edge.resolution), edge.invocation, location,
	}, "\x00")
}

func cloneLocation(value *programindex.Location) *programindex.Location {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneWitnesses(values []programindex.Witness) []programindex.Witness {
	if values == nil {
		return nil
	}
	result := make([]programindex.Witness, len(values))
	for position, value := range values {
		result[position] = value
		result[position].Location = cloneLocation(value.Location)
	}
	return result
}
