package groupindex

import (
	"sort"
	"strconv"

	"github.com/dvordrova/repomap/internal/programindex"
)

// Catalogue is the inputs whose handler is not established that one
// declaration declares, of one kind: redis-cli's options, which parseOptions
// compares its arguments with, are one catalogue, never N identical
// readings. It is keyed by what declares the inputs: DeclaredOn, the object
// a declaring call is made on (a parser, a flag set), when the facts name
// one; otherwise DeclaredBy, the function whose code makes the declaring
// calls (kept beside On when one function declares them all). Code only groups and lists: where these inputs take effect is not
// established by it.
//
// Calls are the calls into DeclaredBy (calls and executes, resolved exactly
// or as alternatives), in source order: where the declaring code runs from.
// Uses are the variables DeclaredBy reads exactly, each with every other
// running declaration that reads it exactly: code that uses the same data,
// never claimed to be where an input takes effect.
//
// Derived by Derive, never persisted.
type Catalogue struct {
	// On is the object, when the facts name one, and OnOperationID the
	// input declared at the call that made it, when there is one: its
	// handler, if established, is where the members are handled from.
	On            *DeclaredOn
	OnOperationID string
	DeclaredBy    string
	Kind          string
	OperationIDs  []string
	Calls         []int
	Uses          []CatalogueUse
	// Readers are, for inputs a table declares (DeclaredBy a variable),
	// the functions that read the table exactly (look up in it), each with
	// its reads and the calls into it, in subject order.
	Readers []CatalogueReader
}

// CatalogueReader is one function reading a table of inputs: Edges are its
// reads of the table, Calls the calls into it (calls and executes, exact or
// alternatives), in source order.
type CatalogueReader struct {
	SubjectID string
	Edges     []int
	Calls     []int
}

// CatalogueUse is one variable the declaring code reads: Edges are the
// positions of its reads from DeclaredBy in Index.StructuralEdges, and Users
// every other function, method, lambda or module body that reads it
// exactly, in subject order. No cap: every reader is listed.
type CatalogueUse struct {
	SubjectID string
	Edges     []int
	Users     []string
}

// catalogues groups the handler-less inputs by what declares them.
func catalogues(index *Index) []Catalogue {
	type key struct{ on, by, kind string }
	positionOf := make(map[string]int, len(index.Subjects))
	for position, subject := range index.Subjects {
		positionOf[subject.ID] = position
	}
	kindOf := func(id string) programindex.ObjectKind {
		if position, ok := positionOf[id]; ok && index.Subjects[position].Object != nil {
			return index.Subjects[position].Object.Kind
		}
		return ""
	}
	var result []Catalogue
	at := map[key]int{}
	for _, operation := range index.Operations {
		// A value of another input's words is that input's sub-argument, no
		// member of a catalogue.
		if !operation.HandlerUnknown || operation.ValueOf != "" || operation.DeclaredBy == "" && operation.DeclaredOn == nil {
			continue
		}
		k := key{by: operation.DeclaredBy, kind: operation.Kind}
		if operation.DeclaredOn != nil {
			// One object is one catalogue, whichever function declares on it.
			k = key{on: operationLocationKey(operation.DeclaredOn.Location), kind: operation.Kind}
		}
		position, seen := at[k]
		if !seen {
			position = len(result)
			at[k] = position
			catalogue := Catalogue{DeclaredBy: operation.DeclaredBy, Kind: operation.Kind}
			if on := operation.DeclaredOn; on != nil {
				copied := *on
				catalogue.On = &copied
				for _, other := range index.Operations {
					if operationLocationKey(other.Location) == k.on && other.ID != operation.ID {
						catalogue.OnOperationID = other.ID
						break
					}
				}
			}
			result = append(result, catalogue)
		} else if result[position].DeclaredBy != operation.DeclaredBy {
			// Declared on one object from several functions: no one
			// function declares them.
			result[position].DeclaredBy = ""
		}
		result[position].OperationIDs = append(result[position].OperationIDs, operation.ID)
	}
	if len(result) == 0 {
		return nil
	}
	operationAt := make(map[string]int, len(index.Operations))
	for position, operation := range index.Operations {
		operationAt[operation.ID] = position
	}
	for position := range result {
		catalogue := &result[position]
		sort.SliceStable(catalogue.OperationIDs, func(i, j int) bool {
			left, right := index.Operations[operationAt[catalogue.OperationIDs[i]]].Location, index.Operations[operationAt[catalogue.OperationIDs[j]]].Location
			return locationBefore(&left, &right)
		})
		if catalogue.DeclaredBy == "" {
			continue
		}
		if kindOf(catalogue.DeclaredBy) == programindex.ObjectVariable {
			catalogue.Readers = tableReaders(index, catalogue.DeclaredBy, kindOf, positionOf)
			continue
		}
		uses := map[string]int{}
		for edgePosition, edge := range index.StructuralEdges {
			if edge.Role != EdgeRelationTarget {
				continue
			}
			switch {
			case edge.ToSubjectID == catalogue.DeclaredBy && (edge.RelationKind == programindex.RelationCalls || edge.RelationKind == programindex.RelationExecutes) &&
				(edge.Resolution == programindex.ResolutionExact || edge.Resolution == programindex.ResolutionAlternatives):
				catalogue.Calls = append(catalogue.Calls, edgePosition)
			case edge.FromSubjectID == catalogue.DeclaredBy && edge.RelationKind == programindex.RelationReads && edge.Resolution == programindex.ResolutionExact &&
				kindOf(edge.ToSubjectID) == programindex.ObjectVariable:
				use, seen := uses[edge.ToSubjectID]
				if !seen {
					use = len(catalogue.Uses)
					uses[edge.ToSubjectID] = use
					catalogue.Uses = append(catalogue.Uses, CatalogueUse{SubjectID: edge.ToSubjectID})
				}
				catalogue.Uses[use].Edges = append(catalogue.Uses[use].Edges, edgePosition)
			}
		}
		sort.SliceStable(catalogue.Calls, func(i, j int) bool {
			return locationBefore(index.StructuralEdges[catalogue.Calls[i]].Location, index.StructuralEdges[catalogue.Calls[j]].Location)
		})
		if len(catalogue.Uses) == 0 {
			continue
		}
		users := make([]map[string]bool, len(catalogue.Uses))
		for i := range users {
			users[i] = map[string]bool{}
		}
		for _, edge := range index.StructuralEdges {
			if edge.Role != EdgeRelationTarget || edge.RelationKind != programindex.RelationReads || edge.Resolution != programindex.ResolutionExact || edge.FromSubjectID == catalogue.DeclaredBy {
				continue
			}
			use, ok := uses[edge.ToSubjectID]
			if !ok {
				continue
			}
			switch kindOf(edge.FromSubjectID) {
			case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectLambda, programindex.ObjectModule:
				users[use][edge.FromSubjectID] = true
			}
		}
		for i := range catalogue.Uses {
			for id := range users[i] {
				catalogue.Uses[i].Users = append(catalogue.Uses[i].Users, id)
			}
			sort.Slice(catalogue.Uses[i].Users, func(a, b int) bool {
				return positionOf[catalogue.Uses[i].Users[a]] < positionOf[catalogue.Uses[i].Users[b]]
			})
		}
	}
	return result
}

// operationLocationKey is a source site as one key, column 1 standing for
// none.
func operationLocationKey(location programindex.Location) string {
	return location.Path + "\x00" + strconv.Itoa(location.Line) + "\x00" + strconv.Itoa(max(1, location.Column))
}

// tableReaders are the functions reading a table exactly, with the calls
// into each.
func tableReaders(index *Index, table string, kindOf func(string) programindex.ObjectKind, positionOf map[string]int) []CatalogueReader {
	at := map[string]int{}
	var readers []CatalogueReader
	for position, edge := range index.StructuralEdges {
		if edge.Role != EdgeRelationTarget || edge.RelationKind != programindex.RelationReads || edge.Resolution != programindex.ResolutionExact || edge.ToSubjectID != table {
			continue
		}
		switch kindOf(edge.FromSubjectID) {
		case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectLambda, programindex.ObjectModule:
		default:
			continue
		}
		reader, seen := at[edge.FromSubjectID]
		if !seen {
			reader = len(readers)
			at[edge.FromSubjectID] = reader
			readers = append(readers, CatalogueReader{SubjectID: edge.FromSubjectID})
		}
		readers[reader].Edges = append(readers[reader].Edges, position)
	}
	for position, edge := range index.StructuralEdges {
		reader, ok := at[edge.ToSubjectID]
		if !ok || edge.Role != EdgeRelationTarget || edge.RelationKind != programindex.RelationCalls && edge.RelationKind != programindex.RelationExecutes ||
			edge.Resolution != programindex.ResolutionExact && edge.Resolution != programindex.ResolutionAlternatives {
			continue
		}
		readers[reader].Calls = append(readers[reader].Calls, position)
	}
	for i := range readers {
		calls := readers[i].Calls
		sort.SliceStable(calls, func(a, b int) bool {
			return locationBefore(index.StructuralEdges[calls[a]].Location, index.StructuralEdges[calls[b]].Location)
		})
	}
	sort.SliceStable(readers, func(i, j int) bool { return positionOf[readers[i].SubjectID] < positionOf[readers[j].SubjectID] })
	return readers
}
