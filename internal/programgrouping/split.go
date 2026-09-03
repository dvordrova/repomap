package programgrouping

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
)

// Splitting is the same operation as grouping, pointed the other way.
//
// Consolidation can only join, so a shard that put a third of a target into
// one group leaves it there: chi's router package came out with a group called
// "Middleware" holding 385 symbols, sitting beside six containers whose names
// all ended in "Middleware". No amount of joining fixes that. Asking what
// parts such a group is made of is the same question grouping already answers,
// asked of one group's members instead of a target's subjects — and the
// group's own name is then exactly the container over the answer, so the level
// above comes for free.
//
// A split is a partition or it does not happen. Parts that cover only some of
// the members would drop the rest off the map, which is the failure
// consolidation was built to make impossible, so a response that does not
// place every member exactly once leaves the group whole and says so.
const (
	// splitShare is the share of a target one group may hold before it is
	// asked what it is made of. The map already calls a group this size a
	// bucket in its own caption.
	splitShare = 20
	// splitFloor keeps a small target's largest group whole. Three groups of
	// four are not a hierarchy.
	splitFloor = 24
	// splitMinParts is how many parts make a split worth keeping. One part is
	// the group again under another name.
	splitMinParts = 2
)

// runSplits asks each oversized group what parts it is made of and turns the
// answer into those parts plus a container named after the group.
func runSplits(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	set proposalSet,
	subjects int,
) (proposalSet, []groupindex.Diagnostic) {
	var diagnostics []groupindex.Diagnostic
	result := proposalSet{
		connections: set.connections,
		containers:  append([]groupindex.ContainerProposal(nil), set.containers...),
		diagnostics: append([]groupindex.Diagnostic(nil), set.diagnostics...),
	}
	held := make(map[string]struct{})
	for _, container := range set.containers {
		for _, key := range container.GroupKeys {
			held[key] = struct{}{}
		}
	}
	for position, group := range set.groups {
		if !worthSplitting(group, subjects) {
			result.groups = append(result.groups, group)
			continue
		}
		if _, inside := held[group.Key]; inside {
			// A group already inside a part keeps its place; splitting it
			// would put a level under a level for no reader's benefit.
			result.groups = append(result.groups, group)
			continue
		}
		parts, splitDiagnostics, err := splitGroup(
			ctx, executor, provider, compilation, group, fmt.Sprintf("s%d", position+1),
		)
		diagnostics = append(diagnostics, splitDiagnostics...)
		if err != nil || len(parts) < splitMinParts {
			if err != nil && ctx.Err() == nil {
				diagnostics = append(diagnostics, groupindex.Diagnostic{
					Kind: diagnosticSplitSkipped, ProposalKey: group.Key, Reason: err.Error(),
				})
			}
			result.groups = append(result.groups, group)
			continue
		}
		container := groupindex.ContainerProposal{
			Key: group.Key + ":part", Title: group.Title, Summary: group.Summary, Lane: group.Lane,
		}
		for _, part := range parts {
			container.GroupKeys = append(container.GroupKeys, part.Key)
			result.groups = append(result.groups, part)
		}
		result.containers = append(result.containers, container)
		result.connections = moveConnections(result.connections, group.Key, parts)
	}
	return result, diagnostics
}

func worthSplitting(group groupProposal, subjects int) bool {
	if len(group.MemberSubjectIDs) < splitFloor || subjects <= 0 {
		return false
	}
	return len(group.MemberSubjectIDs)*100/subjects >= splitShare
}

// splitGroup asks the grouping phase about one group's members and keeps the
// answer only when it places every one of them exactly once.
func splitGroup(
	ctx context.Context,
	executor llm.Executor,
	provider llm.Provider,
	compilation Compilation,
	group groupProposal,
	prefix string,
) ([]groupProposal, []groupindex.Diagnostic, error) {
	refs := make([]string, 0, len(group.MemberSubjectIDs))
	for _, id := range group.MemberSubjectIDs {
		if ref := compilation.refBySubjectID[id]; ref != "" {
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return nil, nil, fmt.Errorf("program grouping: group %q has no restorable members", group.Title)
	}
	// The grouping phase is sparse on purpose — a subject it says nothing
	// about simply has no group — so asking it to split never partitions
	// anything. The split phase asks for a partition and says so.
	request, err := compilation.request(phaseSplit, refs, proposalSet{})
	if err != nil {
		return nil, nil, err
	}
	// A split that has to be sharded cannot be a partition: no shard sees the
	// whole group, so none of them can promise to place every member.
	fits, err := requestFits(provider, request)
	if err != nil {
		return nil, nil, err
	}
	if !fits {
		return nil, []groupindex.Diagnostic{{
			Kind: diagnosticSplitTooLarge, ProposalKey: group.Key, Reason: group.Title,
		}}, nil
	}
	wire, err := json.Marshal(request)
	if err != nil {
		return nil, nil, fmt.Errorf("program grouping: encode split request: %w", err)
	}
	state, err := cubeState(compilation.index.SHA256, phaseSplit, wire)
	if err != nil {
		return nil, nil, err
	}
	outcome, err := llm.ExecuteJSON(ctx, executor, provider, llm.Call[proposalSet]{
		State: state,
		Prompt: llm.Prompt{
			System: strings.TrimSpace(promptText), User: string(wire), ResponseFormatJSON: true,
		},
		Limits: limits(),
		DecodeValidate: func(raw []byte) (proposalSet, error) {
			return normalizeResponse(raw, compilation, request)
		},
	})
	if err != nil {
		return nil, nil, err
	}
	parts := namespaceProposalSet(canonicalProposalSet(outcome.Value), prefix+":")
	if reason := partitionFailure(group, parts.groups); reason != "" {
		return nil, []groupindex.Diagnostic{{
			Kind: diagnosticSplitNotAPartition, ProposalKey: group.Key, Reason: reason,
		}}, nil
	}
	for position := range parts.groups {
		// A part of a group answers on the same lane as the group: its
		// members are the same members, and their categories have not moved.
		parts.groups[position].Lane = group.Lane
	}
	return parts.groups, parts.diagnostics, nil
}

// partitionFailure says why an answer is not a partition of the group, or
// nothing when it is one.
func partitionFailure(group groupProposal, parts []groupProposal) string {
	placed := make(map[string]int, len(group.MemberSubjectIDs))
	for _, part := range parts {
		for _, member := range part.MemberSubjectIDs {
			placed[member]++
		}
	}
	missing, repeated, foreign := 0, 0, 0
	inGroup := make(map[string]struct{}, len(group.MemberSubjectIDs))
	for _, member := range group.MemberSubjectIDs {
		inGroup[member] = struct{}{}
		switch placed[member] {
		case 1:
		case 0:
			missing++
		default:
			repeated++
		}
	}
	for member := range placed {
		if _, known := inGroup[member]; !known {
			foreign++
		}
	}
	if missing == 0 && repeated == 0 && foreign == 0 {
		return ""
	}
	return fmt.Sprintf("%d of %d members unplaced, %d in more than one part, %d not in the group",
		missing, len(group.MemberSubjectIDs), repeated, foreign)
}

// moveConnections re-points what the split group was connected to. A part
// inherits a connection when it holds a member the connection cites; when
// nothing says which part, every part inherits it, because dropping the
// connection would lose a fact the shards established.
func moveConnections(
	connections []connectionProposal,
	splitKey string,
	parts []groupProposal,
) []connectionProposal {
	result := make([]connectionProposal, 0, len(connections))
	partOf := make(map[string][]string, len(parts))
	for _, part := range parts {
		for _, member := range part.MemberSubjectIDs {
			partOf[member] = append(partOf[member], part.Key)
		}
	}
	inherit := func(evidence []string) []string {
		keys := make(map[string]struct{})
		for _, member := range evidence {
			for _, key := range partOf[member] {
				keys[key] = struct{}{}
			}
		}
		if len(keys) == 0 {
			for _, part := range parts {
				keys[part.Key] = struct{}{}
			}
		}
		list := make([]string, 0, len(keys))
		for key := range keys {
			list = append(list, key)
		}
		sort.Strings(list)
		return list
	}
	for _, connection := range connections {
		switch {
		case connection.FromGroupKey == splitKey && connection.ToGroupKey == splitKey:
			continue
		case connection.FromGroupKey == splitKey:
			for _, key := range inherit(connection.EvidenceSubjectIDs) {
				moved := connection
				moved.FromGroupKey = key
				result = append(result, moved)
			}
		case connection.ToGroupKey == splitKey:
			for _, key := range inherit(connection.EvidenceSubjectIDs) {
				moved := connection
				moved.ToGroupKey = key
				result = append(result, moved)
			}
		default:
			result = append(result, connection)
		}
	}
	return result
}
