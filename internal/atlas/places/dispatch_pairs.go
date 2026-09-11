package places

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

type nativeSymbolCall struct {
	owner  string
	path   string
	call   atlas.SymbolCall
	source *nativeDispatchSource
}

type dispatchSite struct {
	path         string
	line, column int
}

// One external relation family can contain many calls. Index its native
// witnesses once, rather than rescan the whole family for every call pattern.
type nativeDispatchSource struct {
	kind, invocation, resolution string
	bySite                       map[dispatchSite][]atlas.DispatchWitness
	support                      []atlas.DispatchWitness
}

func dispatchSource(relation programindex.Relation) *nativeDispatchSource {
	if !strings.HasPrefix(relation.Invocation, "interface_invoke:") && !strings.HasPrefix(relation.Invocation, "declared_interface_dispatch:") {
		return nil
	}
	result := &nativeDispatchSource{kind: string(relation.Kind), invocation: relation.Invocation, resolution: string(relation.Resolution), bySite: make(map[dispatchSite][]atlas.DispatchWitness)}
	for _, witness := range relation.Witnesses {
		at := witness.Location
		row := atlas.DispatchWitness{Kind: witness.Kind, Detail: witness.Detail, SourceExpression: witness.SourceExpression}
		if at != nil {
			row.Path, row.Line, row.Column = at.Path, at.Line, at.Column
		}
		if at == nil || witness.Kind == "interface_field_assignment" {
			result.support = append(result.support, row)
		} else {
			key := dispatchSite{row.Path, row.Line, row.Column}
			result.bySite[key] = append(result.bySite[key], row)
		}
	}
	return result
}

func (source *nativeDispatchSource) observation(call atlas.SymbolCall, path string) atlas.DispatchObservation {
	result := atlas.DispatchObservation{Kind: source.kind, Invocation: source.invocation, Resolution: source.resolution, Detail: call.Detail}
	result.Witnesses = append(result.Witnesses, source.support...)
	result.Witnesses = append(result.Witnesses, source.bySite[dispatchSite{path, call.Line, call.Column}]...)
	sort.Slice(result.Witnesses, func(i, j int) bool {
		a, _ := json.Marshal(result.Witnesses[i])
		b, _ := json.Marshal(result.Witnesses[j])
		return string(a) < string(b)
	})
	return result
}

// The external symbol is the declared interface method, not a resolved runtime
// implementation. One native source invocation has both views; merging their
// presentation must preserve its unresolved dispatch and the original witnesses.
func mergeDeclaredDispatchPairs(rows []nativeSymbolCall) []nativeSymbolCall {
	type site struct {
		owner, path  string
		line, column int
	}
	bySite := make(map[site][]int)
	for i, row := range rows {
		if row.call.Line > 0 && row.call.Column > 0 {
			key := site{row.owner, row.path, row.call.Line, row.call.Column}
			bySite[key] = append(bySite[key], i)
		}
	}
	omit := make(map[int]bool)
	for _, positions := range bySite {
		// A third view might carry a possible implementation or different API.
		// Do not choose a counterpart among such independent alternatives.
		if len(positions) != 2 {
			continue
		}
		plain, rich := positions[0], positions[1]
		if !unresolvedDispatch(rows[plain].call) {
			plain, rich = rich, plain
		}
		a, b := rows[plain].call, rows[rich].call
		if !unresolvedDispatch(a) || !declaredDispatch(b) || rows[plain].source == nil || rows[rich].source == nil ||
			strings.TrimPrefix(a.Invocation, "interface_invoke:") != strings.TrimPrefix(b.Invocation, "declared_interface_dispatch:") {
			continue
		}
		rows[rich].call.Resolution = a.Resolution
		rows[rich].call.DispatchObservations = []atlas.DispatchObservation{rows[plain].source.observation(a, rows[plain].path), rows[rich].source.observation(b, rows[rich].path)}
		omit[plain] = true
	}
	result := make([]nativeSymbolCall, 0, len(rows))
	for i, row := range rows {
		if !omit[i] {
			result = append(result, row)
		}
	}
	return result
}

func unresolvedDispatch(call atlas.SymbolCall) bool {
	return call.Kind == string(programindex.RelationCalls) && call.Name == "" && call.Resolution == string(programindex.ResolutionUnresolved) &&
		strings.HasPrefix(call.Invocation, "interface_invoke:") && call.API == nil && len(call.CalleeIDs) == 0 &&
		len(call.SourceArguments) == 0 && len(call.Values) == 0 && len(call.Arguments) == 0 && call.ReceiverValue == nil && call.ResultValue == nil
}

func declaredDispatch(call atlas.SymbolCall) bool {
	return call.Kind == string(programindex.RelationInvokesExternal) && call.API != nil && len(call.CalleeIDs) == 0 &&
		strings.HasPrefix(call.Invocation, "declared_interface_dispatch:") && len(call.DispatchObservations) == 0
}
