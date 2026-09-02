package programcategorization

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/documentationreduce"
	"github.com/dvordrova/repomap/internal/programindex"
)

type subjectKind string

const (
	subjectObject  subjectKind = "object"
	subjectPattern subjectKind = "pattern"
)

type subjectAuthority struct {
	ref      string
	id       string
	kind     subjectKind
	object   *programindex.Object
	relation *programindex.Relation
	pattern  *programindex.RelationPattern
}

type argumentAuthority struct {
	ref       string
	id        string
	patternID string
	argument  programindex.PatternArgument
}

type graphEdge struct {
	ref          string
	kind         string
	fromID       string
	toID         string
	relationKind programindex.RelationKind
	resolution   programindex.Resolution
	location     *programindex.Location
}

type documentationKind string

const (
	documentationOverview documentationKind = "overview"
	documentationClaim    documentationKind = "claim"
	documentationConcept  documentationKind = "concept"
)

type documentationAuthority struct {
	ref        string
	path       string
	sourceKind string
	kind       documentationKind
	text       string
}

// Compilation is the request-local authority. Canonical ProgramIndex IDs stay
// here and are never placed in a provider request.
type Compilation struct {
	index              programindex.Index
	documentation      documentationreduce.Result
	subjects           []subjectAuthority
	subjectByRef       map[string]subjectAuthority
	refBySubjectID     map[string]string
	arguments          []argumentAuthority
	argumentByID       map[string]argumentAuthority
	refByArgumentID    map[string]string
	edges              []graphEdge
	documentationRows  []documentationAuthority
	documentationByRef map[string]documentationAuthority
}

// Compile validates and owns the two exact lower-layer handoffs, then builds a
// language-neutral graph of objects, relation patterns, and local edges.
func Compile(index programindex.Index, documentation documentationreduce.Result) (Compilation, error) {
	if err := index.Validate(); err != nil {
		return Compilation{}, fmt.Errorf("program categorization: ProgramIndex: %w", err)
	}
	if index.Categorization != nil {
		return Compilation{}, fmt.Errorf("program categorization: ProgramIndex is already enriched")
	}
	documentationSnapshot, err := documentation.Snapshot()
	if err != nil {
		return Compilation{}, err
	}
	compilation := Compilation{
		index:              index.Snapshot(),
		documentation:      documentationSnapshot,
		subjectByRef:       make(map[string]subjectAuthority),
		refBySubjectID:     make(map[string]string),
		argumentByID:       make(map[string]argumentAuthority),
		refByArgumentID:    make(map[string]string),
		documentationByRef: make(map[string]documentationAuthority),
	}

	objects := make([]subjectAuthority, 0, len(compilation.index.Objects))
	for position := range compilation.index.Objects {
		object := compilation.index.Objects[position]
		objectCopy := object
		objects = append(objects, subjectAuthority{
			id: object.ID, kind: subjectObject, object: &objectCopy,
		})
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].id < objects[j].id })
	for position := range objects {
		objects[position].ref = "o" + strconv.Itoa(position+1)
		compilation.addSubject(objects[position])
	}

	patterns := make([]subjectAuthority, 0)
	for relationPosition := range compilation.index.Relations {
		relation := compilation.index.Relations[relationPosition]
		for patternPosition := range relation.Patterns {
			pattern := relation.Patterns[patternPosition]
			relationCopy := relation
			patternCopy := pattern
			patterns = append(patterns, subjectAuthority{
				id: pattern.ID, kind: subjectPattern,
				relation: &relationCopy, pattern: &patternCopy,
			})
		}
	}
	sort.Slice(patterns, func(i, j int) bool { return patterns[i].id < patterns[j].id })
	for position := range patterns {
		patterns[position].ref = "p" + strconv.Itoa(position+1)
		compilation.addSubject(patterns[position])
	}

	for _, relation := range compilation.index.Relations {
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				compilation.arguments = append(compilation.arguments, argumentAuthority{
					id: argument.ID, patternID: pattern.ID, argument: argument,
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

	if err := compilation.compileEdges(); err != nil {
		return Compilation{}, err
	}
	compilation.compileDocumentation()
	return compilation, nil
}

func (compilation *Compilation) addSubject(subject subjectAuthority) {
	compilation.subjects = append(compilation.subjects, subject)
	compilation.subjectByRef[subject.ref] = subject
	compilation.refBySubjectID[subject.id] = subject.ref
}

func (compilation *Compilation) compileEdges() error {
	for _, object := range compilation.index.Objects {
		if object.OwnerID != "" {
			compilation.addEdge(graphEdge{
				kind: "owns", fromID: object.OwnerID, toID: object.ID,
				resolution: programindex.ResolutionExact, location: cloneLocation(object.Location),
			})
		}
		if object.ContainerID != "" {
			compilation.addEdge(graphEdge{
				kind: "contains", fromID: object.ContainerID, toID: object.ID,
				resolution: programindex.ResolutionExact, location: cloneLocation(object.Location),
			})
		}
	}
	for relationPosition := range compilation.index.Relations {
		relation := compilation.index.Relations[relationPosition]
		for _, targetID := range relation.ToIDs {
			compilation.addEdge(graphEdge{
				kind: "relation", fromID: relation.FromID, toID: targetID,
				relationKind: relation.Kind, resolution: relation.Resolution,
				location: cloneLocation(relation.Location),
			})
		}
		for patternPosition := range relation.Patterns {
			pattern := relation.Patterns[patternPosition]
			location := pattern.Location
			if location == nil {
				location = relation.Location
			}
			compilation.addEdge(graphEdge{
				kind: "pattern_owner", fromID: relation.FromID, toID: pattern.ID,
				relationKind: relation.Kind, resolution: programindex.ResolutionExact,
				location: cloneLocation(location),
			})
			for _, targetID := range relation.ToIDs {
				compilation.addEdge(graphEdge{
					kind: "pattern_target", fromID: pattern.ID, toID: targetID,
					relationKind: relation.Kind, resolution: relation.Resolution,
					location: cloneLocation(location),
				})
			}
			if pattern.Form == programindex.PatternDecoratorCall {
				compilation.addEdge(graphEdge{
					kind: "decorates", fromID: pattern.ID, toID: relation.FromID,
					relationKind: relation.Kind, resolution: programindex.ResolutionExact,
					location: cloneLocation(location),
				})
			}
			if pattern.ResultID != "" {
				compilation.addEdge(graphEdge{
					kind: "produces", fromID: pattern.ID, toID: pattern.ResultID,
					relationKind: relation.Kind, resolution: programindex.ResolutionExact,
					location: cloneLocation(location),
				})
			}
			if pattern.ReceiverID != "" {
				compilation.addEdge(graphEdge{
					kind: "receiver_invokes", fromID: pattern.ReceiverID, toID: pattern.ID,
					relationKind: relation.Kind, resolution: programindex.ResolutionExact,
					location: cloneLocation(location),
				})
			}
			for _, originID := range pattern.ReceiverOriginIDs {
				compilation.addEdge(graphEdge{
					kind: "receiver_origin", fromID: originID, toID: pattern.ID,
					relationKind: relation.Kind, resolution: pattern.ReceiverOriginResolution,
					location: cloneLocation(location),
				})
			}
			for _, argument := range pattern.Arguments {
				for _, objectID := range argument.ObjectIDs {
					compilation.addEdge(graphEdge{
						kind: "argument_value", fromID: objectID, toID: pattern.ID,
						relationKind: relation.Kind, resolution: argument.Resolution,
						location: cloneLocation(location),
					})
				}
			}
		}
	}

	sort.Slice(compilation.edges, func(i, j int) bool {
		return edgeKey(compilation.edges[i]) < edgeKey(compilation.edges[j])
	})
	if len(compilation.edges) > 1 {
		write := 1
		for _, edge := range compilation.edges[1:] {
			if edgeKey(edge) == edgeKey(compilation.edges[write-1]) {
				continue
			}
			compilation.edges[write] = edge
			write++
		}
		compilation.edges = compilation.edges[:write]
	}
	for position := range compilation.edges {
		compilation.edges[position].ref = "e" + strconv.Itoa(position+1)
	}
	return nil
}

func (compilation *Compilation) addEdge(edge graphEdge) {
	if compilation.refBySubjectID[edge.fromID] == "" || compilation.refBySubjectID[edge.toID] == "" {
		return
	}
	compilation.edges = append(compilation.edges, edge)
}

func (compilation *Compilation) compileDocumentation() {
	if compilation.documentation.Overview != "" {
		compilation.documentationRows = append(compilation.documentationRows, documentationAuthority{
			kind: documentationOverview, text: compilation.documentation.Overview,
		})
	}
	for _, source := range compilation.documentation.Sources {
		for _, claim := range source.Claims {
			compilation.documentationRows = append(compilation.documentationRows, documentationAuthority{
				path: source.Path, sourceKind: string(source.Kind), kind: documentationClaim, text: claim,
			})
		}
		for _, concept := range source.Concepts {
			compilation.documentationRows = append(compilation.documentationRows, documentationAuthority{
				path: source.Path, sourceKind: string(source.Kind), kind: documentationConcept, text: concept,
			})
		}
	}
	sort.Slice(compilation.documentationRows, func(i, j int) bool {
		left, right := compilation.documentationRows[i], compilation.documentationRows[j]
		if left.path != right.path {
			return left.path < right.path
		}
		if left.sourceKind != right.sourceKind {
			return left.sourceKind < right.sourceKind
		}
		if left.kind != right.kind {
			return left.kind < right.kind
		}
		return left.text < right.text
	})
	for position := range compilation.documentationRows {
		compilation.documentationRows[position].ref = "d" + strconv.Itoa(position+1)
		compilation.documentationByRef[compilation.documentationRows[position].ref] = compilation.documentationRows[position]
	}
}

func edgeKey(edge graphEdge) string {
	location := ""
	if edge.location != nil {
		location = edge.location.Path + ":" + strconv.Itoa(edge.location.Line) + ":" + strconv.Itoa(edge.location.Column)
	}
	return strings.Join([]string{
		edge.kind, edge.fromID, edge.toID, string(edge.relationKind), string(edge.resolution), location,
	}, "\x00")
}

func cloneLocation(location *programindex.Location) *programindex.Location {
	if location == nil {
		return nil
	}
	copy := *location
	return &copy
}
