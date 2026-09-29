package lines

import (
	_ "embed"

	"github.com/dvordrova/repomap/internal/atlas/table"
)

//go:embed prompts/destinations.md
var destinationsPrompt string

// DestinationsContract names the question asking what the outgoing calls
// of one destination reach.
const DestinationsContract = "repomap.atlas.destinations.v1"

// DestinationNames asks, of each destination of the outgoing calls, which
// outside system it is (repomap.atlas.destinations.v1, in the boundaries
// stage, the text model). A destination is the calls whose walked values
// end at the same place, a code fact; its name is asked once for all of
// them, never per call. The one cell chooses a catalogue entry or writes a
// name after the free prefix; refused, it names none.
func DestinationNames() table.Definition {
	return table.Definition{Stage: StageBoundaries, Contract: DestinationsContract, System: destinationsPrompt, Memoize: true,
		Columns: []table.Column{destinationColumn()}}
}

// DestinationEnd is where the value naming what the calls reach ends: an
// address the walk wrote, or an expression it could not follow, with the
// code where it ends as written.
type DestinationEnd struct {
	Address    string `json:"address,omitempty"`
	Unresolved string `json:"unresolved,omitempty"`
	Written    string `json:"written,omitempty"`
}

// DestinationCall is one call of a destination as the repository wrote it:
// its kind, the outside package it goes through when the code names one,
// and the literals it was given.
type DestinationCall struct {
	Call    string   `json:"call"`
	Kind    string   `json:"kind"`
	Package string   `json:"package,omitempty"`
	Values  []string `json:"values,omitempty"`
}

// DestinationCaller is a function making a destination's calls, with its
// file, its declared type and the calls it makes beside them as written
// (OwnerCalls). A
// function handed over to be called later, such as a closure, says by
// whom and to which call (handed_over); a function handing one over says
// which and to which call (hands_over).
type DestinationCaller struct {
	Name       string   `json:"name"`
	Path       string   `json:"path"`
	Signature  string   `json:"signature,omitempty"`
	HandedOver []string `json:"handed_over,omitempty"`
	HandsOver  []string `json:"hands_over,omitempty"`
	Calls      []string `json:"calls,omitempty"`
}

// DestinationRow is one destination: where its value ends, its calls, the
// functions making them and those the program reaches the calls from, each
// once.
func DestinationRow(id string, ends []DestinationEnd, calls []DestinationCall, callers []DestinationCaller, reachedFrom []string) table.Row {
	fields := []table.Field{{Name: "ends", Value: ends}, {Name: "calls", Value: calls}, {Name: "callers", Value: callers}}
	if len(reachedFrom) > 0 {
		fields = append(fields, table.Field{Name: "reached_from", Value: reachedFrom})
	}
	return table.Row{ID: id, Fields: fields}
}
