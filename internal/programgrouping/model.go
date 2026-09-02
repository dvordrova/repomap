// Package programgrouping owns the model-assisted grouping of one semantically
// enriched ProgramIndex. It restores request-local model refs and hands the
// accepted proposals directly to groupindex.Build; there is no intermediate
// public grouping artifact.
package programgrouping

import (
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
)

const (
	requestVersion        = 4
	executionContract     = "repomap.program-grouping.v5"
	preparationVersion    = 5
	responseSchemaVersion = 2
	outputTokenCount      = 128_000
)

type phase string

const (
	phaseGrouping phase = "grouping"
	phaseMerge    phase = "merge"
)

const (
	diagnosticMalformedGroup      = "malformed_group"
	diagnosticMalformedConnection = "malformed_connection"
	diagnosticInvalidGroup        = "invalid_group"
	diagnosticInvalidConnection   = "invalid_connection"
	diagnosticUnknownMemberRef    = "unknown_member_ref"
	diagnosticUnselectableMember  = "unselectable_member_ref"
	diagnosticUnknownEvidenceRef  = "unknown_evidence_ref"
	diagnosticUnsupportedEvidence = "unsupported_evidence_ref"
	diagnosticLaneMismatch        = "lane_mismatch"
	diagnosticUnknownGroupKey     = "unknown_group_key"
	diagnosticConflictingGroupKey = "conflicting_group_key"
)

type groupProposal struct {
	Key                string
	Title              string
	Summary            string
	Lane               groupindex.Lane
	MemberSubjectIDs   []string
	EvidenceSubjectIDs []string
}

type connectionProposal struct {
	FromGroupKey       string
	ToGroupKey         string
	SemanticKind       string
	Label              string
	Summary            string
	EvidenceSubjectIDs []string
}

type proposalSet struct {
	groups      []groupProposal
	connections []connectionProposal
	diagnostics []groupindex.Diagnostic
}

func (set proposalSet) groupIndexProposals() groupindex.Proposals {
	result := groupindex.Proposals{
		Groups:      make([]groupindex.GroupProposal, len(set.groups)),
		Connections: make([]groupindex.ConnectionProposal, len(set.connections)),
	}
	for position, group := range set.groups {
		result.Groups[position] = groupindex.GroupProposal{
			Key: group.Key, Title: group.Title, Summary: group.Summary, Lane: group.Lane,
			MemberSubjectIDs:   cloneStrings(group.MemberSubjectIDs),
			EvidenceSubjectIDs: cloneStrings(group.EvidenceSubjectIDs),
		}
	}
	for position, connection := range set.connections {
		result.Connections[position] = groupindex.ConnectionProposal{
			FromGroupKey: connection.FromGroupKey, ToGroupKey: connection.ToGroupKey,
			SemanticKind: connection.SemanticKind, Label: connection.Label, Summary: connection.Summary,
			EvidenceSubjectIDs: cloneStrings(connection.EvidenceSubjectIDs),
		}
	}
	return result
}

func namespaceProposalSet(value proposalSet, prefix string) proposalSet {
	keyMap := make(map[string]string, len(value.groups))
	result := proposalSet{
		groups:      make([]groupProposal, len(value.groups)),
		connections: make([]connectionProposal, 0, len(value.connections)),
		diagnostics: append([]groupindex.Diagnostic(nil), value.diagnostics...),
	}
	for position, group := range value.groups {
		oldKey := group.Key
		group.Key = prefix + oldKey
		keyMap[oldKey] = group.Key
		result.groups[position] = group
	}
	for _, connection := range value.connections {
		from, fromOK := keyMap[connection.FromGroupKey]
		to, toOK := keyMap[connection.ToGroupKey]
		if !fromOK || !toOK {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind:        diagnosticUnknownGroupKey,
				ProposalKey: connection.FromGroupKey + "->" + connection.ToGroupKey,
				Reason:      "connection cites a group key rejected from the same response",
			})
			continue
		}
		connection.FromGroupKey = from
		connection.ToGroupKey = to
		result.connections = append(result.connections, connection)
	}
	return result
}

func canonicalProposalSet(value proposalSet) proposalSet {
	groupsByKey := make(map[string]groupProposal, len(value.groups))
	conflictingKeys := make(map[string]struct{})
	for _, group := range value.groups {
		group.MemberSubjectIDs = canonicalStrings(group.MemberSubjectIDs)
		group.EvidenceSubjectIDs = canonicalStrings(group.EvidenceSubjectIDs)
		previous, exists := groupsByKey[group.Key]
		if !exists {
			groupsByKey[group.Key] = group
			continue
		}
		if previous.Title != group.Title || previous.Summary != group.Summary || previous.Lane != group.Lane {
			delete(groupsByKey, group.Key)
			conflictingKeys[group.Key] = struct{}{}
			continue
		}
		previous.MemberSubjectIDs = unionStrings(previous.MemberSubjectIDs, group.MemberSubjectIDs)
		previous.EvidenceSubjectIDs = unionStrings(previous.EvidenceSubjectIDs, group.EvidenceSubjectIDs)
		groupsByKey[group.Key] = previous
	}
	for key := range conflictingKeys {
		delete(groupsByKey, key)
		value.diagnostics = append(value.diagnostics, groupindex.Diagnostic{
			Kind: diagnosticConflictingGroupKey, ProposalKey: key,
			Reason: "group key has incompatible title, summary, or lane rows",
		})
	}
	keys := make([]string, 0, len(groupsByKey))
	for key := range groupsByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	value.groups = make([]groupProposal, 0, len(keys))
	for _, key := range keys {
		value.groups = append(value.groups, groupsByKey[key])
	}

	knownGroups := make(map[string]struct{}, len(value.groups))
	for _, group := range value.groups {
		knownGroups[group.Key] = struct{}{}
	}
	type connectionSlot struct {
		from    string
		to      string
		kind    string
		label   string
		summary string
	}
	connectionsBySlot := make(map[connectionSlot]connectionProposal)
	for _, connection := range value.connections {
		if _, ok := knownGroups[connection.FromGroupKey]; !ok {
			value.diagnostics = append(value.diagnostics, groupindex.Diagnostic{
				Kind:        diagnosticUnknownGroupKey,
				ProposalKey: connection.FromGroupKey + "->" + connection.ToGroupKey,
				Reason:      "connection cites an unknown or rejected source group key",
			})
			continue
		}
		if _, ok := knownGroups[connection.ToGroupKey]; !ok {
			value.diagnostics = append(value.diagnostics, groupindex.Diagnostic{
				Kind:        diagnosticUnknownGroupKey,
				ProposalKey: connection.FromGroupKey + "->" + connection.ToGroupKey,
				Reason:      "connection cites an unknown or rejected destination group key",
			})
			continue
		}
		connection.EvidenceSubjectIDs = canonicalStrings(connection.EvidenceSubjectIDs)
		slot := connectionSlot{
			from: connection.FromGroupKey, to: connection.ToGroupKey,
			kind: connection.SemanticKind, label: connection.Label, summary: connection.Summary,
		}
		previous, exists := connectionsBySlot[slot]
		if exists {
			previous.EvidenceSubjectIDs = unionStrings(previous.EvidenceSubjectIDs, connection.EvidenceSubjectIDs)
			connectionsBySlot[slot] = previous
			continue
		}
		connectionsBySlot[slot] = connection
	}
	value.connections = make([]connectionProposal, 0, len(connectionsBySlot))
	for _, connection := range connectionsBySlot {
		value.connections = append(value.connections, connection)
	}
	sort.Slice(value.connections, func(i, j int) bool {
		return connectionProposalKey(value.connections[i]) < connectionProposalKey(value.connections[j])
	})
	value.diagnostics = canonicalDiagnostics(value.diagnostics)
	return value
}

func connectionProposalKey(value connectionProposal) string {
	return strings.Join([]string{
		value.FromGroupKey, value.ToGroupKey, value.SemanticKind,
		value.Label, value.Summary, strings.Join(value.EvidenceSubjectIDs, "\x01"),
	}, "\x00")
}

func canonicalDiagnostics(values []groupindex.Diagnostic) []groupindex.Diagnostic {
	sort.Slice(values, func(i, j int) bool {
		return diagnosticKey(values[i]) < diagnosticKey(values[j])
	})
	result := values[:0]
	for _, value := range values {
		if len(result) > 0 && diagnosticKey(result[len(result)-1]) == diagnosticKey(value) {
			continue
		}
		result = append(result, value)
	}
	if result == nil {
		result = []groupindex.Diagnostic{}
	}
	return result
}

func diagnosticKey(value groupindex.Diagnostic) string {
	return value.Kind + "\x00" + value.ProposalKey + "\x00" + value.Reason
}

func canonicalStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	write := 0
	for _, value := range result {
		if write > 0 && result[write-1] == value {
			continue
		}
		result[write] = value
		write++
	}
	result = result[:write]
	if result == nil {
		result = []string{}
	}
	return result
}

func unionStrings(left, right []string) []string {
	return canonicalStrings(append(append([]string(nil), left...), right...))
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}
