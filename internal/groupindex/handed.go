package groupindex

import (
	"slices"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// Handed is what the declarations that make calls on their own parameters
// are handed (a helper adding a flag to the parser it is given): the calls
// each declaration makes on a parameter, every call into it with the
// arguments a call made and the keys of a table each list it hands is
// looked up with, and the rows of those tables by key. Compiled from the
// bound ProgramIndex, never persisted: launch.go nests what such a
// declaration declares under the input whose own call made the object.
type Handed struct {
	// OnParameter are the calls a retained declaration makes on one of its
	// own parameters (a receiver that is the parameter), by site.
	OnParameter []ParameterCall
	// Calls are the calls into those declarations, and into the
	// declarations looking a table up with keys they are handed, in
	// relation order.
	Calls []HandedCall
	// Unfollowed are those declarations the code also reaches otherwise
	// than by an exact call of one declaration: a call resolved to
	// alternatives, a callable handed over, a read of it as a value.
	Unfollowed map[string]bool
	// Rows are the rows of the tables a handed list's keys look up: each
	// row's key, the first word it writes, at that word.
	Rows []TableRowKey
}

// ParameterCall is a call a declaration makes on its own parameter,
// numbered from one without the receiver.
type ParameterCall struct {
	SubjectID string
	Location  programindex.Location
	Parameter int
}

// HandedCall is one call into a declaration of Handed: Exact when it is
// resolved exactly to that one declaration. Made are, by the callee's
// parameter, where the call that made the argument's value is written (a
// call_result origin), and Keys, by table, the keys the list the call hands
// is looked up with in that table (a `keys` read at the call): the list's
// one-word rows, nil when the list holds no rows the code wrote.
type HandedCall struct {
	FromSubjectID string
	ToSubjectID   string
	Location      programindex.Location
	Exact         bool
	Made          map[int]programindex.Location
	Keys          map[string][]string
}

// TableRowKey is one row of a table: its key and where it is written.
type TableRowKey struct {
	TableID  string
	Key      string
	Location programindex.Location
}

// compileHanded lists what the retained declarations making calls on their
// own parameters, or looking a table up with keys they are handed, are
// handed.
func compileHanded(program programindex.Index, retained map[string]struct{}) Handed {
	result := Handed{Unfollowed: map[string]bool{}}
	objects := make(map[string]*programindex.Object, len(program.Objects))
	for position := range program.Objects {
		objects[program.Objects[position].ID] = &program.Objects[position]
	}
	// The calls made on a declaration's own parameter.
	callees := map[string]bool{}
	for _, relation := range program.Relations {
		owner := objects[relation.FromID]
		if _, ok := retained[relation.FromID]; !ok || owner == nil || owner.Location == nil {
			continue
		}
		for _, pattern := range relation.Patterns {
			receiver := pattern.ReceiverValue
			if pattern.Location == nil || receiver == nil || receiver.Kind != "parameter" || !sameAnchor(receiver.Owner, owner.Location) {
				continue
			}
			result.OnParameter = append(result.OnParameter, ParameterCall{SubjectID: owner.ID, Location: columnLocation(*pattern.Location), Parameter: receiver.Position})
			callees[owner.ID] = true
		}
	}
	// The keys reads: a read of a list with a `keys` witness naming the
	// table it is looked up in, by reader.
	type keysRead struct {
		list, table string
		at          programindex.Location
	}
	reads := map[string][]keysRead{}
	for _, relation := range program.Relations {
		if relation.Kind != programindex.RelationReads || relation.Resolution != programindex.ResolutionExact || relation.Location == nil || len(relation.ToIDs) != 1 {
			continue
		}
		for _, witness := range relation.Witnesses {
			if witness.Kind == programindex.WitnessKeys && witness.ObjectID != "" {
				reads[relation.FromID] = append(reads[relation.FromID], keysRead{list: relation.ToIDs[0], table: witness.ObjectID, at: *relation.Location})
			}
		}
	}
	// A declaration a call hands a keys read list to looks the table up
	// with keys it is handed.
	tables := map[string]bool{}
	for _, relation := range program.Relations {
		if relation.Kind != programindex.RelationCalls || relation.Location == nil || len(relation.Patterns) != 1 {
			continue
		}
		for _, read := range reads[relation.FromID] {
			if handsObject(relation.Patterns[0], read.list) && !locationBefore(&read.at, relation.Location) {
				for _, to := range relation.ToIDs {
					if _, ok := retained[to]; ok {
						callees[to] = true
					}
				}
			}
		}
	}
	calls := map[string][]int{}
	var patterns []programindex.RelationPattern
	for _, relation := range program.Relations {
		for _, to := range relation.ToIDs {
			if !callees[to] {
				continue
			}
			switch relation.Kind {
			case programindex.RelationImports, programindex.RelationImplements, programindex.RelationSources:
				continue
			}
			if relation.Kind != programindex.RelationCalls || relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 || relation.Location == nil || len(relation.Patterns) != 1 {
				result.Unfollowed[to] = true
				continue
			}
			call := HandedCall{FromSubjectID: relation.FromID, ToSubjectID: to, Location: *relation.Location, Exact: true, Made: map[int]programindex.Location{}}
			pattern := relation.Patterns[0]
			callee := objects[to]
			shift := 0
			if receiver := objects[pattern.ReceiverID]; callee.Kind == programindex.ObjectMethod && receiver != nil && receiver.Kind == programindex.ObjectType {
				// A method called through its class is handed its receiver
				// first.
				shift = 1
			}
			for _, argument := range pattern.Arguments {
				parameter := argument.Position - shift
				if argument.Keyword != "" {
					parameter = slices.IndexFunc(callee.Parameters, func(p programindex.TypedName) bool { return p.Name == argument.Keyword }) + 1
				}
				if parameter < 1 || argument.Origin == nil || argument.Origin.Kind != "call_result" || argument.Origin.Anchor == nil {
					continue
				}
				anchor := argument.Origin.Anchor
				call.Made[parameter] = columnLocation(programindex.Location{Path: anchor.Path, Line: anchor.Line, Column: anchor.Column})
			}
			calls[relation.FromID] = append(calls[relation.FromID], len(result.Calls))
			result.Calls = append(result.Calls, call)
			patterns = append(patterns, pattern)
		}
	}
	// Each keys read belongs to the latest call from its reader at or
	// before it that hands the list.
	for from, list := range reads {
		for _, read := range list {
			found := -1
			for _, position := range calls[from] {
				at := result.Calls[position].Location
				if locationBefore(&read.at, &at) || !handsObject(patterns[position], read.list) {
					continue
				}
				if found < 0 || locationBefore(&result.Calls[found].Location, &at) {
					found = position
				}
			}
			if found < 0 {
				continue
			}
			call := &result.Calls[found]
			if call.Keys == nil {
				call.Keys = map[string][]string{}
			}
			var keys []string
			if list := objects[read.list]; list != nil {
				for _, row := range list.Rows {
					if len(row.Literals) != 1 || row.Literals[0].Field != "" {
						keys = nil
						break
					}
					keys = append(keys, row.Literals[0].Value)
				}
			}
			call.Keys[read.table] = keys
			tables[read.table] = true
		}
	}
	for _, object := range program.Objects {
		if !tables[object.ID] {
			continue
		}
		for _, row := range object.Rows {
			if len(row.Literals) == 0 || row.Literals[0].Location == nil {
				continue
			}
			first := row.Literals[0]
			result.Rows = append(result.Rows, TableRowKey{TableID: object.ID, Key: first.Value, Location: columnLocation(*first.Location)})
		}
	}
	return result
}

// handsObject says a call's pattern hands an object as an argument.
func handsObject(pattern programindex.RelationPattern, object string) bool {
	for _, argument := range pattern.Arguments {
		if slices.Contains(argument.ObjectIDs, object) {
			return true
		}
	}
	return false
}

// sameAnchor says an anchor names the declaration at a location: its line
// (an adapter anchors a declaration at its keyword or at its name).
func sameAnchor(anchor *sourcevalue.Anchor, location *programindex.Location) bool {
	return anchor != nil && location != nil && anchor.Path == location.Path && anchor.Line == location.Line
}

func columnLocation(location programindex.Location) programindex.Location {
	location.Column = max(1, location.Column)
	return location
}
