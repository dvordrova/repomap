package terminology

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

//go:embed prompts/explain.md
var explainPrompt string

// explanations is one accepted explanation window: by name, the definition
// written for it.
type explanations struct {
	Explanations map[string]string
	Rejections   []llm.ResponseRejection
}

func (result explanations) ResponseRejections() []llm.ResponseRejection { return result.Rejections }

// explainCall asks for one definition per accepted name. Each name has a
// closed t ref and the p refs of every prose row that writes it; the prose
// catalogue holds exactly those rows, as written.
func explainCall(items []proseSource, names []glossaryName) (llm.Call[explanations], error) {
	refs := make(map[int]string)
	var rows []int
	for _, name := range names {
		for _, row := range name.Rows {
			if _, found := refs[row]; !found {
				refs[row] = ""
				rows = append(rows, row)
			}
		}
	}
	// Rows keep the collected order, whatever name first wrote them.
	ordered := make([]int, 0, len(rows))
	for i := range items {
		if _, found := refs[i]; found {
			ordered = append(ordered, i)
		}
	}
	prose := make([]map[string]any, 0, len(ordered))
	for _, row := range ordered {
		refs[row] = fmt.Sprintf("p%d", len(prose)+1)
		prose = append(prose, map[string]any{"ref": refs[row], "text": items[row].Texts})
	}
	byRef := make(map[string]string, len(names))
	terms := make([]map[string]any, 0, len(names))
	for i, name := range names {
		ref := fmt.Sprintf("t%d", i+1)
		byRef[ref] = name.Name
		written := make([]string, 0, len(name.Rows))
		for _, row := range name.Rows {
			written = append(written, refs[row])
		}
		terms = append(terms, map[string]any{"ref": ref, "name": name.Name, "rows": written})
	}
	input, err := json.Marshal(map[string]any{"prose": prose, "terms": terms})
	if err != nil {
		return llm.Call[explanations]{}, err
	}
	return llm.Call[explanations]{
		State: []byte(`{"contract":"repomap.glossary.explain.v1"}`),
		Prompt: llm.Prompt{System: explainPrompt, User: string(input), ResponseFormatJSON: true, NoResponseAdjunct: true,
			ResponseExample: `{"terms":[{"ref":"<t ref>","explanation":"<short plain-English definition>"}]}`},
		Limits: llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: glossaryOutputTokens},
		DecodeValidate: func(raw []byte) (explanations, error) {
			normalized, err := llm.NormalizeJSON(raw)
			if err != nil {
				return explanations{}, err
			}
			return decodeExplanations(normalized, byRef)
		},
	}, nil
}

// decodeExplanations reads one explanation per closed t ref, in any case and
// padding. An unknown ref is discarded. A ref without an explanation, or
// answered twice with different ones, leaves only that name unexplained; an
// identical repeat is one answer and extra members are ignored. The terms
// list may come bare or be missing (no rows). A window in which no name was
// explained is refused.
func decodeExplanations(raw []byte, byRef map[string]string) (explanations, error) {
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil || list == nil {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(raw, &envelope); err != nil || envelope == nil {
			return explanations{}, fmt.Errorf("glossary: a terms array is required")
		}
		if terms, present := envelope["terms"]; present && string(terms) != "null" {
			if err := json.Unmarshal(terms, &list); err != nil {
				return explanations{}, fmt.Errorf("glossary: a terms array is required")
			}
		}
	}
	result := explanations{Explanations: make(map[string]string)}
	byReason := make(map[string]int)
	reject := func(reason, sample string) {
		index, found := byReason[reason]
		if !found {
			index = len(result.Rejections)
			byReason[reason] = index
			result.Rejections = append(result.Rejections, llm.ResponseRejection{Kind: "glossary_term_rejected", Reason: reason})
		}
		result.Rejections[index].Count++
		if len(result.Rejections[index].Samples) < 5 {
			result.Rejections[index].Samples = append(result.Rejections[index].Samples, sample)
		}
	}
	answered := make(map[string]string)
	conflicting := make(map[string]bool)
	for index, item := range list {
		position := fmt.Sprintf("terms[%d]", index)
		var row struct {
			Ref         *string         `json:"ref"`
			Explanation json.RawMessage `json:"explanation"`
		}
		if json.Unmarshal(item, &row) != nil || row.Ref == nil {
			reject("an explanation without a term ref", position)
			continue
		}
		ref := strings.ToLower(strings.TrimSpace(*row.Ref))
		name, known := byRef[ref]
		if !known {
			reject("unknown term ref", position)
			continue
		}
		// An empty or "none" explanation explains nothing; the name is
		// journaled below as not explained.
		var explanation string
		if json.Unmarshal(row.Explanation, &explanation) != nil {
			continue
		}
		if explanation = strings.TrimSpace(explanation); explanation == "" || strings.EqualFold(explanation, "none") {
			continue
		}
		if previous, found := answered[name]; found && previous != explanation {
			conflicting[name] = true
			continue
		}
		answered[name] = explanation
	}
	refs := make([]string, 0, len(byRef))
	for ref := range byRef {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		return len(refs[i]) < len(refs[j]) || len(refs[i]) == len(refs[j]) && refs[i] < refs[j]
	})
	for _, ref := range refs {
		name := byRef[ref]
		explanation, found := answered[name]
		switch {
		case conflicting[name]:
			reject("a term answered twice with different explanations", name)
		case !found:
			reject("a term was not explained", name)
		default:
			result.Explanations[name] = explanation
		}
	}
	if len(result.Explanations) == 0 && len(byRef) > 0 {
		return result, fmt.Errorf("glossary: no term was explained")
	}
	return result, nil
}

// splitNames halves a window of names by the weight of their rows.
func splitNames(names []glossaryName) ([]glossaryName, []glossaryName, bool) {
	if len(names) < 2 {
		return nil, nil, false
	}
	total := 0
	for _, name := range names {
		total += len(name.Rows) + 1
	}
	middle, weight := 1, len(names[0].Rows)+1
	for middle < len(names)-1 && weight < total/2 {
		weight += len(names[middle].Rows) + 1
		middle++
	}
	return names[:middle], names[middle:], true
}
