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
	Uses        []atlas.DestinationUse `json:"uses,omitempty"`
	ID          string                 `json:"id"`
	SubjectID   string                 `json:"subject_id,omitempty"`
	GroupID     string                 `json:"group_id,omitempty"`
	FactID      string                 `json:"fact_id,omitempty"`
	Kind        string                 `json:"kind"`
	External    string                 `json:"external,omitempty"`
	Destination string                 `json:"destination,omitempty"`
	Address     string                 `json:"address,omitempty"`
	Basis       string                 `json:"basis,omitempty"`
	Method      string                 `json:"method,omitempty"`
	Values      []string               `json:"values"`
	Summary     string                 `json:"summary,omitempty"`
	Source      string                 `json:"source"`
	Location    programindex.Location  `json:"location"`
}

func projectOutbound(program programindex.Index, target atlas.Target, groups map[string]string, sourceRefs map[string]string) []OutboundCall {
	local := make(map[string]string, len(program.Objects))
	for _, object := range program.Objects {
		if key := declarationKey(object); key != "" {
			local[key] = object.ID
		}
	}
	var calls []OutboundCall
	localSymbols := make(map[string]string)
	for _, box := range target.Boxes {
		for _, file := range box.Files {
			for _, symbol := range file.Symbols {
				localSymbols[symbol.ID] = local[sourceRefs[symbol.ObjectID]]
			}
		}
	}
	for _, boundary := range target.Boundaries {
		if boundary.Direction != atlas.DirectionOut || !communicationKind(boundary.Kind) ||
			boundary.Kind == atlas.BoundaryOther && boundary.Basis == "" {
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
		calls = append(calls, OutboundCall{
			Uses: uses,
			ID:   boundary.ID, SubjectID: local[sourceRefs[boundary.ObjectID]], GroupID: groups[boundary.BoxID],
			FactID: boundary.FactID, Kind: boundary.Kind, External: boundary.External,
			Destination: boundary.Destination, Address: boundary.Address, Basis: boundary.Basis,
			Method: boundary.Method, Values: cloneStrings(boundary.Values), Summary: boundary.Line, Source: source,
			Location: programindex.Location{Path: boundary.Path, Line: boundary.LineNo, Column: max(1, boundary.Column)},
		})
	}
	sort.Slice(calls, func(i, j int) bool { return calls[i].ID < calls[j].ID })
	return calls
}

func cloneDestinationUses(uses []atlas.DestinationUse) []atlas.DestinationUse {
	if uses == nil {
		return nil
	}
	result := append([]atlas.DestinationUse(nil), uses...)
	for i := range result {
		result[i].Steps = append([]atlas.DestinationStep(nil), uses[i].Steps...)
		result[i].TargetIDs = append([]string(nil), uses[i].TargetIDs...)
	}
	return result
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
			call.Kind == atlas.BoundaryOther && (call.Basis == "" || call.Destination == "") ||
			call.Basis != "" && call.Basis != "dispatch" && call.Basis != "configuration" ||
			call.SubjectID != "" && !subjectExists || call.GroupID != "" && !groupExists ||
			(call.Source != "fact" && call.Source != "model") ||
			call.Location.Path == "" || call.Location.Line < 1 || call.Location.Column < 1 {
			return fmt.Errorf("group index: invalid outbound call %q", call.ID)
		}
		if i > 0 && index.Outbound[i-1].ID >= call.ID {
			return fmt.Errorf("group index: outbound calls are not canonical")
		}
	}
	return nil
}
