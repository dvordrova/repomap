package programgrouping

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
)

func normalizeResponse(raw []byte, compilation Compilation, request Request) (proposalSet, error) {
	normalized, err := llm.NormalizeJSON(raw)
	if err != nil {
		return proposalSet{}, err
	}
	if !hasExactObjectKeys(normalized, []string{"groups", "connections"}) {
		return proposalSet{}, fmt.Errorf("program grouping: response envelope does not match the closed schema")
	}
	var envelope struct {
		Groups      []json.RawMessage `json:"groups"`
		Connections []json.RawMessage `json:"connections"`
	}
	decoder := json.NewDecoder(bytes.NewReader(normalized))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return proposalSet{}, fmt.Errorf("program grouping: decode response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return proposalSet{}, fmt.Errorf("program grouping: response has trailing data")
	}
	if envelope.Groups == nil || envelope.Connections == nil {
		return proposalSet{}, fmt.Errorf("program grouping: response groups and connections must be arrays")
	}

	knownRefs := make(map[string]struct{}, len(request.Subjects))
	for _, subject := range request.Subjects {
		knownRefs[subject.Ref] = struct{}{}
	}
	selectableRefs := make(map[string]struct{}, len(request.GroupRefs))
	for _, ref := range request.GroupRefs {
		selectableRefs[ref] = struct{}{}
	}
	result := proposalSet{}
	for _, rawRow := range envelope.Groups {
		var row responseGroup
		if !decodeStrictRow(rawRow, []string{
			"key", "title", "summary", "lane", "member_refs", "evidence_refs",
		}, &row) {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind: diagnosticMalformedGroup, Reason: "group row does not match the closed response schema",
			})
			continue
		}
		if !validText(row.Key) || !validText(row.Title) || !validText(row.Summary) ||
			!row.Lane.Valid() || len(row.MemberRefs) == 0 || row.EvidenceRefs == nil {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind: diagnosticInvalidGroup, ProposalKey: row.Key,
				Reason: "group has an invalid key, title, summary, lane, members, or evidence array",
			})
			continue
		}
		memberIDs := make([]string, 0, len(row.MemberRefs))
		for _, ref := range canonicalStrings(row.MemberRefs) {
			if _, known := knownRefs[ref]; !known {
				result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
					Kind: diagnosticUnknownMemberRef, ProposalKey: row.Key,
					Reason: "group member ref was not advertised in this request",
				})
				continue
			}
			if _, selectable := selectableRefs[ref]; !selectable {
				result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
					Kind: diagnosticUnselectableMember, ProposalKey: row.Key,
					Reason: "context subject is not selectable as a group member in this request",
				})
				continue
			}
			subject := compilation.subjectByRef[ref]
			if !subjectSupportsLane(subject, row.Lane) {
				result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
					Kind: diagnosticLaneMismatch, ProposalKey: row.Key,
					Reason: "group member category does not support the selected lane",
				})
				continue
			}
			memberIDs = append(memberIDs, subject.id)
		}
		if len(memberIDs) == 0 {
			continue
		}
		evidenceIDs := make([]string, 0, len(row.EvidenceRefs))
		for _, ref := range canonicalStrings(row.EvidenceRefs) {
			if _, known := knownRefs[ref]; !known {
				result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
					Kind: diagnosticUnknownEvidenceRef, ProposalKey: row.Key,
					Reason: "group evidence ref was not advertised in this request",
				})
				continue
			}
			subject := compilation.subjectByRef[ref]
			if row.Lane == groupindex.LaneDependencies &&
				!programindex.CategorySupported(compilation.index, subject.id, programindex.CategoryDependency) {
				result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
					Kind: diagnosticUnsupportedEvidence, ProposalKey: row.Key,
					Reason: "platform authority cannot evidence a dependencies-lane group",
				})
				continue
			}
			evidenceIDs = append(evidenceIDs, subject.id)
		}
		result.groups = append(result.groups, groupProposal{
			Key: row.Key, Title: row.Title, Summary: row.Summary, Lane: row.Lane,
			MemberSubjectIDs: memberIDs, EvidenceSubjectIDs: evidenceIDs,
		})
	}
	for _, rawRow := range envelope.Connections {
		var row responseConnection
		if !decodeStrictRow(rawRow, []string{
			"from_group_key", "to_group_key", "semantic_kind", "label", "summary", "evidence_refs",
		}, &row) {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind:   diagnosticMalformedConnection,
				Reason: "connection row does not match the closed response schema",
			})
			continue
		}
		proposalKey := row.FromGroupKey + "->" + row.ToGroupKey + ":" + row.SemanticKind
		if !validText(row.FromGroupKey) || !validText(row.ToGroupKey) ||
			!validSnakeCase(row.SemanticKind) || !validText(row.Label) || !validText(row.Summary) ||
			row.EvidenceRefs == nil {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind: diagnosticInvalidConnection, ProposalKey: proposalKey,
				Reason: "connection has an invalid endpoint, semantic kind, label, summary, or evidence array",
			})
			continue
		}
		evidenceIDs := make([]string, 0, len(row.EvidenceRefs))
		for _, ref := range canonicalStrings(row.EvidenceRefs) {
			if _, known := knownRefs[ref]; !known {
				result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
					Kind: diagnosticUnknownEvidenceRef, ProposalKey: proposalKey,
					Reason: "connection evidence ref was not advertised in this request",
				})
				continue
			}
			evidenceIDs = append(evidenceIDs, compilation.subjectByRef[ref].id)
		}
		result.connections = append(result.connections, connectionProposal{
			FromGroupKey: row.FromGroupKey, ToGroupKey: row.ToGroupKey,
			SemanticKind: row.SemanticKind, Label: row.Label, Summary: row.Summary,
			EvidenceSubjectIDs: evidenceIDs,
		})
	}
	result = canonicalProposalSet(result)
	if request.Phase == phaseMerge {
		if err := validateMergeCandidateCoverage(result, compilation, request); err != nil {
			return proposalSet{}, err
		}
	}
	return result, nil
}

func validateMergeCandidateCoverage(result proposalSet, compilation Compilation, request Request) error {
	for _, candidate := range request.CandidateGroups {
		expected := make([]string, 0, len(candidate.MemberRefs))
		for _, ref := range canonicalStrings(candidate.MemberRefs) {
			subject, known := compilation.subjectByRef[ref]
			if !known {
				return fmt.Errorf("program grouping: merge request retained an unknown candidate member ref %q", ref)
			}
			expected = append(expected, subject.id)
		}
		covered := false
		for _, group := range result.groups {
			if group.Lane != candidate.Lane || !containsAllStrings(group.MemberSubjectIDs, expected) {
				continue
			}
			covered = true
			break
		}
		if !covered {
			return fmt.Errorf(
				"program grouping: merge response omitted validated candidate memberships from %s",
				candidate.Ref,
			)
		}
	}
	return nil
}

func containsAllStrings(values, expected []string) bool {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	for _, value := range expected {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func subjectSupportsLane(subject subjectAuthority, lane groupindex.Lane) bool {
	for _, category := range subject.categories {
		switch lane {
		case groupindex.LaneTriggers:
			if category == programindex.CategoryInbound || category == programindex.CategoryBackgroundActivity {
				return true
			}
		case groupindex.LaneCore:
			if category == programindex.CategoryCore {
				return true
			}
		case groupindex.LaneDependencies:
			if category == programindex.CategoryDependency {
				return true
			}
		}
	}
	return false
}

func decodeStrictRow(raw json.RawMessage, expected []string, destination any) bool {
	if !hasExactObjectKeys(raw, expected) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return false
	}
	var trailing any
	return errors.Is(decoder.Decode(&trailing), io.EOF)
}

// hasExactObjectKeys closes duplicate-key and case-insensitive field matching
// gaps left by encoding/json's struct decoder. Typed decoding still validates
// the values after this exact key-set check.
func hasExactObjectKeys(raw []byte, expected []string) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return false
	}
	wanted := make(map[string]struct{}, len(expected))
	for _, key := range expected {
		wanted[key] = struct{}{}
	}
	seen := make(map[string]struct{}, len(expected))
	for decoder.More() {
		token, tokenErr := decoder.Token()
		key, keyIsString := token.(string)
		if tokenErr != nil || !keyIsString {
			return false
		}
		if _, known := wanted[key]; !known {
			return false
		}
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
		var value json.RawMessage
		if decodeErr := decoder.Decode(&value); decodeErr != nil {
			return false
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') || len(seen) != len(wanted) {
		return false
	}
	var trailing any
	return errors.Is(decoder.Decode(&trailing), io.EOF)
}

func validText(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validSnakeCase(value string) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	previousUnderscore := false
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			previousUnderscore = false
		case character == '_' && !previousUnderscore:
			previousUnderscore = true
		default:
			return false
		}
	}
	return !previousUnderscore
}
