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
}

const (
	chainDepth    = 8
	chainsPerFlow = 32
)

// projectChains walks exact and alternative calls from each operation's
// subject in reading order and records every path that reaches a subject
// with an outbound call.
func projectChains(program programindex.Index, operations []Operation, outbound []OutboundCall) []Chain {
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
				chains = append(chains, Chain{OperationID: operation.ID, SubjectIDs: append([]string(nil), path...), OutboundID: id})
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
