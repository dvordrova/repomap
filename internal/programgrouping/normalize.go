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

// normalizeResponse turns one grouping answer into proposals. The answer is a
// flat assignment — one label per selectable ref — plus the links between
// those labels, and nothing else. The model makes one decision and the code
// makes every other: which lane a group answers on, what it holds, what the
// evidence is. Asking the model for titles, summaries, lanes, members and
// evidence in one answer is asking a model with reasoning disabled to think,
// and that freedom is where the run-to-run spread came from.
func normalizeResponse(raw []byte, compilation Compilation, request Request) (proposalSet, error) {
	normalized, err := llm.NormalizeJSON(raw)
	if err != nil {
		return proposalSet{}, err
	}
	// The envelope must carry the one thing that is asked for and nothing
	// unknown. `links` is optional: a shard whose groups say nothing to each
	// other has none, and refusing that answer lost the whole target.
	if !hasExactObjectKeys(normalized, []string{"assign", "links"}) &&
		!hasExactObjectKeys(normalized, []string{"assign"}) {
		return proposalSet{}, fmt.Errorf("program grouping: response envelope does not match the closed schema")
	}
	var envelope struct {
		Assign []json.RawMessage `json:"assign"`
		Links  []json.RawMessage `json:"links"`
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
	if envelope.Assign == nil {
		return proposalSet{}, fmt.Errorf("program grouping: response assign must be an array")
	}

	selectableRefs := make(map[string]struct{}, len(request.GroupRefs))
	for _, ref := range request.GroupRefs {
		selectableRefs[ref] = struct{}{}
	}
	result := proposalSet{}

	// Gather the assignments by label, and inside a label by lane. A lane
	// follows from a member's own categories, so a label covering two lanes is
	// two groups and nobody moves; the model is never asked for a lane.
	type bucket struct {
		title   string
		lane    groupindex.Lane
		members []string
	}
	buckets := make(map[string]*bucket)
	var order []string
	placed := make(map[string]struct{}, len(envelope.Assign))
	for _, rawRow := range envelope.Assign {
		var row responseAssign
		if !decodeStrictRow(rawRow, []string{"ref", "group"}, &row) {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind: diagnosticMalformedGroup, Reason: "assignment row does not match the closed response schema",
			})
			continue
		}
		if !validText(row.Group) {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind: diagnosticInvalidGroup, ProposalKey: row.Group, Reason: "assignment has an invalid group label",
			})
			continue
		}
		if _, selectable := selectableRefs[row.Ref]; !selectable {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind: diagnosticUnselectableMember, ProposalKey: row.Group,
				Reason: "assignment names a ref this request does not offer",
			})
			continue
		}
		if _, repeated := placed[row.Ref]; repeated {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind: diagnosticUnknownMemberRef, ProposalKey: row.Group,
				Reason: "assignment repeats a ref already placed",
			})
			continue
		}
		subject := compilation.subjectByRef[row.Ref]
		lane := subjectLane(subject)
		if lane == "" {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind: diagnosticLaneMismatch, ProposalKey: row.Group,
				Reason: "subject categories support no lane",
			})
			continue
		}
		placed[row.Ref] = struct{}{}
		key := row.Group + "\x00" + string(lane)
		item, known := buckets[key]
		if !known {
			item = &bucket{title: row.Group, lane: lane}
			buckets[key] = item
			order = append(order, key)
		}
		item.members = append(item.members, subject.id)
	}
	labelKey := make(map[string]string, len(order))
	for position, key := range order {
		item := buckets[key]
		proposalKey := fmt.Sprintf("g%d", position+1)
		labelKey[key] = proposalKey
		result.groups = append(result.groups, groupProposal{
			Key: proposalKey, Title: item.title, Summary: item.title,
			Lane: item.lane, MemberSubjectIDs: canonicalStrings(item.members),
			EvidenceSubjectIDs: []string{},
		})
	}

	// A link joins two labels. It is kept for every lane pair those labels
	// resolved to, because a label that became two groups is still the thing
	// the link was about.
	for _, rawRow := range envelope.Links {
		var row responseLink
		if !decodeStrictRow(rawRow, []string{"from", "to", "label"}, &row) {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind: diagnosticMalformedConnection, Reason: "link row does not match the closed response schema",
			})
			continue
		}
		if !validText(row.From) || !validText(row.To) || !validText(row.Label) || row.From == row.To {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind: diagnosticInvalidConnection, ProposalKey: row.From + "->" + row.To,
				Reason: "link has an invalid label or joins one group to itself",
			})
			continue
		}
		fromKeys, toKeys := labelKeys(labelKey, row.From), labelKeys(labelKey, row.To)
		if len(fromKeys) == 0 || len(toKeys) == 0 {
			result.diagnostics = append(result.diagnostics, groupindex.Diagnostic{
				Kind: diagnosticUnknownGroupKey, ProposalKey: row.From + "->" + row.To,
				Reason: "link names a group label nothing was assigned to",
			})
			continue
		}
		for _, from := range fromKeys {
			for _, to := range toKeys {
				if from == to {
					continue
				}
				result.connections = append(result.connections, connectionProposal{
					FromGroupKey: from, ToGroupKey: to,
					SemanticKind: "relates_to", Label: row.Label, Summary: row.Label,
					EvidenceSubjectIDs: []string{},
				})
			}
		}
	}
	return canonicalProposalSet(result), nil
}

// subjectLane is the one lane a subject answers on, decided by its own
// categories. Inbound and background activity are both ways work begins, so
// they share the triggers lane.
func subjectLane(subject subjectAuthority) groupindex.Lane {
	for _, lane := range []groupindex.Lane{
		groupindex.LaneTriggers, groupindex.LaneCore, groupindex.LaneDependencies,
	} {
		if subjectSupportsLane(subject, lane) {
			return lane
		}
	}
	return ""
}

func labelKeys(labelKey map[string]string, label string) []string {
	var result []string
	for _, lane := range []groupindex.Lane{
		groupindex.LaneTriggers, groupindex.LaneCore, groupindex.LaneDependencies,
	} {
		if key, known := labelKey[label+"\x00"+string(lane)]; known {
			result = append(result, key)
		}
	}
	return result
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

// hasExactObjectKeys closes the ambiguous-key and case-insensitive field
// matching gaps left by encoding/json's struct decoder. Typed decoding still
// validates the values after this exact key-set check.
//
// A key repeated with the same value is accepted. Two identical spellings of
// one answer are one answer, and refusing them costs whole rows: a model that
// repeated "summary" verbatim in every group of one target had all twelve of
// its groups discarded, and with them the twenty connections that named them.
// A key repeated with a different value is still refused, because choosing
// between two answers would be repair.
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
	seen := make(map[string]json.RawMessage, len(expected))
	for decoder.More() {
		token, tokenErr := decoder.Token()
		key, keyIsString := token.(string)
		if tokenErr != nil || !keyIsString {
			return false
		}
		if _, known := wanted[key]; !known {
			return false
		}
		var value json.RawMessage
		if decodeErr := decoder.Decode(&value); decodeErr != nil {
			return false
		}
		if earlier, duplicate := seen[key]; duplicate && !sameJSONValue(earlier, value) {
			return false
		}
		seen[key] = value
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') || len(seen) != len(wanted) {
		return false
	}
	var trailing any
	return errors.Is(decoder.Decode(&trailing), io.EOF)
}

// sameJSONValue compares two encodings by value rather than by bytes, so
// whitespace or key order inside a repeated object cannot turn one answer
// into two.
func sameJSONValue(first, second json.RawMessage) bool {
	var left, right any
	if json.Unmarshal(first, &left) != nil || json.Unmarshal(second, &right) != nil {
		return false
	}
	leftEncoded, leftErr := json.Marshal(left)
	rightEncoded, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftEncoded, rightEncoded)
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
