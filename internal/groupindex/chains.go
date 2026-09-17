package groupindex

import (
	"fmt"

	"github.com/dvordrova/repomap/internal/programindex"
)

// Chain follows one operation into the repository until a call leaves it:
// the subjects walked in order, from the operation's own declaration to the
// one that makes the outbound call. The layers between an entry and the
// system it reaches are the chain's middle; two chains to one system with no
// shared middle are two ways in.
type Chain struct {
	ID          string   `json:"id"`
	OperationID string   `json:"operation_id"`
	SubjectIDs  []string `json:"subject_ids"`
	OutboundID  string   `json:"outbound_id"`
	// TypeIDs are the repository types the chain's signatures carry, in
	// walking order: what crosses from the entry to the system.
	TypeIDs []string `json:"type_ids,omitempty"`
}

const (
	chainDepth    = 8
	chainsPerFlow = 32
)

// projectChains walks exact and alternative calls from each operation's
// subject in reading order and records every path that reaches a subject
// with an outbound call.
func projectChains(program programindex.Index, operations []Operation, outbound []OutboundCall) []Chain {
	objects := make(map[string]programindex.Object, len(program.Objects))
	for _, object := range program.Objects {
		objects[object.ID] = object
	}
	callees := make(map[string][]string)
	for _, relation := range program.Relations {
		if relation.Kind != programindex.RelationCalls || relation.Resolution == programindex.ResolutionUnresolved {
			continue
		}
		for _, to := range relation.ToIDs {
			callees[relation.FromID] = appendUniqueString(callees[relation.FromID], to)
		}
	}
	leaving := make(map[string][]string)
	for _, call := range outbound {
		if call.SubjectID != "" {
			leaving[call.SubjectID] = append(leaving[call.SubjectID], call.ID)
		}
	}
	var chains []Chain
	for _, operation := range operations {
		if operation.SubjectID == "" {
			continue
		}
		found := 0
		onPath := map[string]bool{operation.SubjectID: true}
		var walk func(path []string)
		walk = func(path []string) {
			current := path[len(path)-1]
			for _, id := range leaving[current] {
				if found >= chainsPerFlow {
					return
				}
				found++
				var types []string
				for _, subject := range path {
					types = carriedTypes(types, objects[subject])
				}
				chains = append(chains, Chain{OperationID: operation.ID, SubjectIDs: append([]string(nil), path...), OutboundID: id, TypeIDs: types})
			}
			if len(path) >= chainDepth {
				return
			}
			for _, next := range callees[current] {
				if onPath[next] {
					continue
				}
				onPath[next] = true
				walk(append(path, next))
				delete(onPath, next)
			}
		}
		walk([]string{operation.SubjectID})
	}
	for position := range chains {
		chains[position].ID = compactOrdinal("c", position)
	}
	return chains
}

// carriedTypes adds the repository types an object's signature carries.
func carriedTypes(types []string, object programindex.Object) []string {
	for _, value := range append(append([]programindex.TypedName(nil), object.Parameters...), object.Results...) {
		if value.TypeID != "" {
			types = appendUniqueString(types, value.TypeID)
		}
	}
	return types
}

// operationTypes names what an operation takes in and gives back: the
// repository types of its subject's parameters, and the repository types
// produced by the repository callees whose results the subject hands to
// calls outside the repository (the value written into a response).
func operationTypes(program programindex.Index, operations []Operation) {
	objects := make(map[string]programindex.Object, len(program.Objects))
	for _, object := range program.Objects {
		objects[object.ID] = object
	}
	type site struct {
		path         string
		line, column int
	}
	producers := make(map[site][]string)
	for _, relation := range program.Relations {
		if relation.Kind != programindex.RelationCalls {
			continue
		}
		// A call resolved through an interface has no pattern of its own; the
		// relation's location is its site.
		locations := []*programindex.Location{relation.Location}
		for _, pattern := range relation.Patterns {
			locations = append(locations, pattern.Location)
		}
		for _, location := range locations {
			if location != nil {
				key := site{location.Path, location.Line, location.Column}
				for _, id := range relation.ToIDs {
					producers[key] = appendUniqueString(producers[key], id)
				}
			}
		}
	}
	for position := range operations {
		subject := operations[position].SubjectID
		if subject == "" {
			continue
		}
		for _, value := range objects[subject].Parameters {
			if value.TypeID != "" {
				operations[position].RequestTypeIDs = appendUniqueString(operations[position].RequestTypeIDs, value.TypeID)
			}
		}
		for _, relation := range program.Relations {
			if relation.FromID != subject || relation.Kind != programindex.RelationInvokesExternal {
				continue
			}
			for _, pattern := range relation.Patterns {
				for _, argument := range pattern.Arguments {
					origin := argument.Origin
					if origin == nil || origin.Kind != "call_result" || origin.Anchor == nil {
						continue
					}
					for _, callee := range producers[site{origin.Anchor.Path, origin.Anchor.Line, origin.Anchor.Column}] {
						for _, value := range objects[callee].Results {
							if value.TypeID != "" {
								operations[position].ResponseTypeIDs = appendUniqueString(operations[position].ResponseTypeIDs, value.TypeID)
							}
						}
					}
				}
			}
		}
	}
}

func appendUniqueString(values []string, value string) []string {
	for _, known := range values {
		if known == value {
			return values
		}
	}
	return append(values, value)
}

func (index Index) validateChains(subjects map[string]Subject) error {
	operations := make(map[string]bool, len(index.Operations))
	for _, operation := range index.Operations {
		operations[operation.ID] = true
	}
	outbound := make(map[string]string, len(index.Outbound))
	for _, call := range index.Outbound {
		outbound[call.ID] = call.SubjectID
	}
	for position, chain := range index.Chains {
		if chain.ID != compactOrdinal("c", position) || !operations[chain.OperationID] || len(chain.SubjectIDs) == 0 || len(chain.SubjectIDs) > chainDepth {
			return fmt.Errorf("group index: invalid chain %q", chain.ID)
		}
		subject, known := outbound[chain.OutboundID]
		if !known || subject != chain.SubjectIDs[len(chain.SubjectIDs)-1] {
			return fmt.Errorf("group index: chain %q does not end at its outbound call", chain.ID)
		}
		seen := make(map[string]bool, len(chain.SubjectIDs))
		for _, id := range chain.SubjectIDs {
			if _, ok := subjects[id]; !ok || seen[id] {
				return fmt.Errorf("group index: chain %q walks an unknown or repeated subject", chain.ID)
			}
			seen[id] = true
		}
	}
	return nil
}
