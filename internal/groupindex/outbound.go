package groupindex

import (
	"fmt"
	"slices"
	"sort"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/programindex"
)

// OutboundCall keeps an atlas communication boundary independently of the
// containing group's lane. A request handler can both receive and send work.
// Values are source observations, not a claim that every literal is an address.
type OutboundCall struct {
	Uses []atlas.DestinationUse `json:"uses,omitempty"`
	// Graph is every distinct step and transition of Uses' routes (atlas
	// Boundary.Graph), its steps' subjects local.
	Graph       *atlas.DestinationGraph `json:"graph,omitempty"`
	ID          string                  `json:"id"`
	SubjectID   string                  `json:"subject_id,omitempty"`
	GroupID     string                  `json:"group_id,omitempty"`
	FactID      string                  `json:"fact_id,omitempty"`
	Kind        string                  `json:"kind"`
	External    string                  `json:"external,omitempty"`
	Destination string                  `json:"destination,omitempty"`
	// Alternatives are the systems the destination reaches one of depending
	// on configuration (atlas Boundary.Alternatives), a model choice;
	// Destination is them joined by " / ".
	Alternatives []string `json:"alternatives,omitempty"`
	// DestinationTarget is the target of this repository's program the
	// destination is (atlas Boundary.DestinationTarget), a model choice.
	DestinationTarget string   `json:"destination_target,omitempty"`
	Address           string   `json:"address,omitempty"`
	Basis             string   `json:"basis,omitempty"`
	Method            string   `json:"method,omitempty"`
	Values            []string `json:"values"`
	// DataIDs are the data records this call names: the tables of its
	// statement, joined by name to the extracted schema.
	DataIDs  []string              `json:"data_ids,omitempty"`
	Summary  string                `json:"summary,omitempty"`
	Source   string                `json:"source"`
	Location programindex.Location `json:"location"`
	// ProgramNotNamed marks a started program none of its call's words
	// names (atlas.BoundaryRunsProgram); a named one is its Destination.
	ProgramNotNamed bool `json:"program_not_named,omitempty"`
	// ReachedFrom are the callers outside the call's part the program
	// reaches it from (outbound_reached.go).
	ReachedFrom []OutboundCaller `json:"reached_from,omitempty"`
}

func projectOutbound(program programindex.Index, target atlas.Target, groups map[string]string, sourceRefs map[string]string) []OutboundCall {
	local := make(map[string]string, len(program.Objects))
	for _, object := range program.Objects {
		if key := DeclarationKey(object); key != "" {
			local[key] = object.ID
		}
	}
	var calls []OutboundCall
	// A use step may pass through a declaration off the map, such as every
	// declaration of a target whose map failed.
	localSymbols := make(map[string]string)
	addSymbols := func(file atlas.File) {
		for _, symbol := range file.Symbols {
			localSymbols[symbol.ID] = local[sourceRefs[symbol.ObjectID]]
		}
	}
	for _, box := range target.Boxes {
		for _, file := range box.Files {
			addSymbols(file)
		}
	}
	for _, off := range target.OffMap {
		addSymbols(off.File)
	}
	for _, boundary := range target.Boundaries {
		if boundary.Direction != atlas.DirectionOut || !communicationKind(boundary.Kind) {
			continue
		}
		source := "model"
		if boundary.Source == "fact" || boundary.FactID != "" {
			source = "fact"
		}
		uses := cloneDestinationUses(boundary.Uses)
		for i := range uses {
			for j := range uses[i].Steps {
				uses[i].Steps[j].SubjectID = localSymbols[uses[i].Steps[j].SubjectID]
			}
		}
		graph := cloneDestinationGraph(boundary.Graph)
		if graph != nil {
			for j := range graph.Steps {
				graph.Steps[j].SubjectID = localSymbols[graph.Steps[j].SubjectID]
			}
		}
		calls = append(calls, OutboundCall{
			Uses: uses, Graph: graph,
			ID: boundary.ID, SubjectID: local[sourceRefs[boundary.ObjectID]], GroupID: groups[boundary.BoxID],
			FactID: boundary.FactID, Kind: boundary.Kind, External: boundary.External,
			Destination: boundary.Destination, Alternatives: cloneStrings(boundary.Alternatives), DestinationTarget: boundary.DestinationTarget, Address: boundary.Address, Basis: boundary.Basis,
			Method: boundary.Method, Values: cloneStrings(boundary.Values), Summary: boundary.Line, Source: source,
			Location:        programindex.Location{Path: boundary.Path, Line: boundary.LineNo, Column: max(1, boundary.Column)},
			ProgramNotNamed: boundary.ProgramNotNamed,
		})
	}
	sort.Slice(calls, func(i, j int) bool { return calls[i].ID < calls[j].ID })
	return calls
}

// joinOutboundData names, on every outbound call, the extracted tables its
// values name. A statement's tables come first among its values.
func joinOutboundData(calls []OutboundCall, data []DataRecord) {
	tables := make(map[string][]string)
	for _, record := range data {
		if record.Data != nil && record.Data.Kind == "table" {
			tables[record.Data.Name] = append(tables[record.Data.Name], record.ID)
		}
	}
	for position := range calls {
		for _, value := range calls[position].Values {
			for _, id := range tables[value] {
				calls[position].DataIDs = appendUniqueString(calls[position].DataIDs, id)
			}
		}
	}
}

func cloneDestinationUses(uses []atlas.DestinationUse) []atlas.DestinationUse {
	if uses == nil {
		return nil
	}
	result := append([]atlas.DestinationUse(nil), uses...)
	for i := range result {
		result[i].Steps = append([]atlas.DestinationStep(nil), uses[i].Steps...)
		result[i].TargetIDs = append([]string(nil), uses[i].TargetIDs...)
		result[i].Through = append([]int(nil), uses[i].Through...)
	}
	return result
}

func cloneDestinationGraph(graph *atlas.DestinationGraph) *atlas.DestinationGraph {
	if graph == nil {
		return nil
	}
	return &atlas.DestinationGraph{Steps: append([]atlas.DestinationStep(nil), graph.Steps...), Edges: append([][2]int(nil), graph.Edges...)}
}

// communicationKind is the same closed list the boundaries table offers an
// outgoing candidate, so no accepted kind is silently dropped here.
func communicationKind(kind string) bool {
	return slices.Contains(atlas.OutgoingBoundaryKinds(), kind)
}

func (index Index) validateOutbound(subjects map[string]Subject, groups map[string]struct{}) error {
	for i, call := range index.Outbound {
		_, subjectExists := subjects[call.SubjectID]
		_, groupExists := groups[call.GroupID]
		if !validText(call.ID) || !communicationKind(call.Kind) ||
			call.Basis != "" && call.Basis != "dispatch" && call.Basis != "configuration" ||
			call.ProgramNotNamed && (call.Kind != atlas.BoundaryRunsProgram || call.Destination != "") ||
			call.SubjectID != "" && !subjectExists || call.GroupID != "" && !groupExists ||
			(call.Source != "fact" && call.Source != "model") ||
			call.Location.Path == "" || call.Location.Line < 1 || call.Location.Column < 1 {
			return fmt.Errorf("group index: invalid outbound call %q", call.ID)
		}
		if err := validateOutboundCallers(call, subjects, groups); err != nil {
			return err
		}
		if i > 0 && index.Outbound[i-1].ID >= call.ID {
			return fmt.Errorf("group index: outbound calls are not canonical")
		}
	}
	return nil
}

func appendUniqueString(values []string, value string) []string {
	for _, known := range values {
		if known == value {
			return values
		}
	}
	return append(values, value)
}
