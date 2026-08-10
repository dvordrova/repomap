package entrycall

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

type Response struct {
	Version    int             `json:"version"`
	RequestRef string          `json:"request_ref"`
	Entries    []ResponseEntry `json:"entries"`
}

type ResponseEntry struct {
	RootRef    string   `json:"root_ref"`
	FamilyRefs []string `json:"family_refs"`
}

type Result struct {
	Version               int           `json:"version"`
	PromptVersion         string        `json:"prompt_version"`
	RequestRef            string        `json:"request_ref"`
	RequestSHA256         string        `json:"request_sha256"`
	SubstrateSHA256       string        `json:"substrate_sha256"`
	RepositoryStateSHA256 string        `json:"repository_state_sha256,omitempty"`
	Entries               []ResultEntry `json:"entries"`
}

type ResultEntry struct {
	RootRef          string                 `json:"root_ref"`
	Label            string                 `json:"label"`
	Declaration      Location               `json:"declaration"`
	Families         []ResultFamily         `json:"families"`
	RejectedFamilies []RejectedResultFamily `json:"rejected_families"`
	Frontier         []RequestFrontier      `json:"frontier"`
	Omitted          Omitted                `json:"omitted"`
}

type RejectedFamilyReason string

const RejectedFamilyUnreachable RejectedFamilyReason = "unreachable_from_root"

type RejectedResultFamily struct {
	Ref    string               `json:"ref"`
	Reason RejectedFamilyReason `json:"reason"`
}

type ResultFamily struct {
	Ref          string     `json:"ref"`
	CallerLabel  string     `json:"caller_label"`
	CalleeLabel  string     `json:"callee_label"`
	Invocation   Invocation `json:"invocation"`
	WitnessCount int        `json:"witness_count"`
	Callsites    []Location `json:"callsites"`
}

func (result Result) SelectedFamilyCount() int {
	total := 0
	for _, entry := range result.Entries {
		total += len(entry.Families)
	}
	return total
}

func (result Result) RejectedFamilyCount() int {
	total := 0
	for _, entry := range result.Entries {
		total += len(entry.RejectedFamilies)
	}
	return total
}

// Reduce strictly validates refs and rooted connectivity, ignores response
// order, and restores exact local callsites from private compilation authority.
func Reduce(compilation Compilation, raw []byte) (Result, error) {
	if err := validateCompilation(compilation); err != nil {
		return Result{}, err
	}
	response, err := decodeResponse(raw)
	if err != nil {
		return Result{}, err
	}
	if response.Version != ResultVersion || response.RequestRef != compilation.Request.RequestRef ||
		len(response.Entries) != len(compilation.Request.Entries) {
		return Result{}, fmt.Errorf("entry call: response identity mismatch")
	}
	responseByRoot := make(map[string]ResponseEntry, len(response.Entries))
	for _, entry := range response.Entries {
		if entry.FamilyRefs == nil {
			return Result{}, fmt.Errorf("entry call: response family_refs must be an array")
		}
		if _, duplicate := responseByRoot[entry.RootRef]; duplicate {
			return Result{}, fmt.Errorf("entry call: duplicate response root ref")
		}
		if _, known := compilation.authority[entry.RootRef]; !known {
			return Result{}, fmt.Errorf("entry call: response cites unknown root ref")
		}
		responseByRoot[entry.RootRef] = entry
	}
	result := Result{
		Version: ResultVersion, PromptVersion: PromptVersion,
		RequestRef: compilation.Request.RequestRef, RequestSHA256: compilation.wireSHA,
		SubstrateSHA256: compilation.SubstrateSHA256, Entries: []ResultEntry{},
	}
	for _, requestEntry := range compilation.Request.Entries {
		responseEntry, present := responseByRoot[requestEntry.Ref]
		if !present {
			return Result{}, fmt.Errorf("entry call: response omitted root ref")
		}
		selected, rejected, err := validateSelectedFamilies(requestEntry, responseEntry)
		if err != nil {
			return Result{}, err
		}
		authority := compilation.authority[requestEntry.Ref]
		resultEntry := ResultEntry{
			RootRef: requestEntry.Ref, Label: requestEntry.Label,
			Declaration: authority.rootNode.Declaration,
			Families:    []ResultFamily{}, RejectedFamilies: append([]RejectedResultFamily{}, rejected...),
			Frontier: append([]RequestFrontier{}, requestEntry.Frontier...),
			Omitted:  requestEntry.Omitted,
		}
		nodeByRef := make(map[string]RequestNode, len(requestEntry.Nodes))
		for _, node := range requestEntry.Nodes {
			nodeByRef[node.Ref] = node
		}
		for _, familyRef := range selected {
			requestFamily := requestFamilyByRef(requestEntry, familyRef)
			exact := authority.familyByRef[familyRef]
			callsites := append([]Location(nil), exact.Callsites...)
			sortLocations(callsites)
			resultEntry.Families = append(resultEntry.Families, ResultFamily{
				Ref:         familyRef,
				CallerLabel: nodeByRef[requestFamily.CallerRef].Label,
				CalleeLabel: nodeByRef[requestFamily.CalleeRef].Label,
				Invocation:  requestFamily.Invocation, WitnessCount: requestFamily.WitnessCount,
				Callsites: callsites,
			})
		}
		result.Entries = append(result.Entries, resultEntry)
	}
	return result, nil
}

func decodeResponse(raw []byte) (Response, error) {
	if len(raw) == 0 || len(raw) > MaxResponseBytes {
		return Response{}, fmt.Errorf("entry call: response exceeds bounded envelope")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var response Response
	if err := decoder.Decode(&response); err != nil {
		return Response{}, fmt.Errorf("entry call: decode response: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Response{}, err
	}
	return response, nil
}

func validateSelectedFamilies(request RequestEntry, response ResponseEntry) ([]string, []RejectedResultFamily, error) {
	if len(response.FamilyRefs) > MaxSelectedFamiliesPerRoot {
		return nil, nil, fmt.Errorf("entry call: response selection exceeds per-root bound")
	}
	familyByRef := make(map[string]RequestFamily, len(request.Families))
	for _, family := range request.Families {
		familyByRef[family.Ref] = family
	}
	remaining := make(map[string]RequestFamily, len(response.FamilyRefs))
	for _, ref := range response.FamilyRefs {
		family, known := familyByRef[ref]
		if !known {
			return nil, nil, fmt.Errorf("entry call: response cites unknown family ref")
		}
		if _, duplicate := remaining[ref]; duplicate {
			return nil, nil, fmt.Errorf("entry call: response repeats family ref")
		}
		remaining[ref] = family
	}
	reachable := map[string]bool{request.RootNodeRef: true}
	ordered := make([]string, 0, len(remaining))
	rejected := make([]RejectedResultFamily, 0)
	for len(remaining) > 0 {
		progress := false
		for _, family := range request.Families {
			selected, pending := remaining[family.Ref]
			if !pending || !reachable[selected.CallerRef] {
				continue
			}
			ordered = append(ordered, family.Ref)
			reachable[selected.CalleeRef] = true
			delete(remaining, family.Ref)
			progress = true
		}
		if !progress {
			for _, family := range request.Families {
				if _, pending := remaining[family.Ref]; !pending {
					continue
				}
				rejected = append(rejected, RejectedResultFamily{
					Ref: family.Ref, Reason: RejectedFamilyUnreachable,
				})
			}
			break
		}
	}
	return ordered, rejected, nil
}

func requestFamilyByRef(entry RequestEntry, ref string) RequestFamily {
	for _, family := range entry.Families {
		if family.Ref == ref {
			return family
		}
	}
	return RequestFamily{}
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("entry call: response contains trailing JSON")
		}
		return fmt.Errorf("entry call: decode response tail: %w", err)
	}
	return nil
}

func validDigest(value string, optional bool) bool {
	if optional && value == "" {
		return true
	}
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func canonicalResultFamilyLess(left, right ResultFamily) bool {
	if left.Ref != right.Ref {
		return refLess(left.Ref, right.Ref)
	}
	return left.CallerLabel+"\x00"+left.CalleeLabel < right.CallerLabel+"\x00"+right.CalleeLabel
}

func sortResult(result *Result) {
	sort.Slice(result.Entries, func(i, j int) bool { return refLess(result.Entries[i].RootRef, result.Entries[j].RootRef) })
	for index := range result.Entries {
		sort.Slice(result.Entries[index].Families, func(i, j int) bool {
			return canonicalResultFamilyLess(result.Entries[index].Families[i], result.Entries[index].Families[j])
		})
		sort.Slice(result.Entries[index].RejectedFamilies, func(i, j int) bool {
			return refLess(result.Entries[index].RejectedFamilies[i].Ref, result.Entries[index].RejectedFamilies[j].Ref)
		})
	}
}

func refLess(left, right string) bool {
	if len(left) < 2 || len(right) < 2 {
		return left < right
	}
	leftNumber, leftErr := strconv.Atoi(left[1:])
	rightNumber, rightErr := strconv.Atoi(right[1:])
	if leftErr == nil && rightErr == nil && leftNumber != rightNumber {
		return leftNumber < rightNumber
	}
	return left < right
}
