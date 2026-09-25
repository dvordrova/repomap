package targetportfolio

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/llm"
)

// batchSelection is one classification answer restored to its batch beside
// the members the decoder discarded. The discards are journaled with the
// exchange; none of them reaches the Selection.
type batchSelection struct {
	Selection
	rejected []llm.ResponseRejection
}

func (batch batchSelection) ResponseRejections() []llm.ResponseRejection { return batch.rejected }

// ResolveResponse restores one classification answer. The answer decides only
// what it validly names: an extra field, a non-text or unadvertised member, a
// malformed decision row and an unusable default are discarded and recorded,
// never repaired. Required native representatives come from the compilation,
// not from the answer, so the model cannot suppress them. Omitted guidance
// stays unclassified; a native target without one valid decision keeps its
// recorded standalone fallback. A batch without a usable default leaves the
// repository default to Run: the single eligible target or a separate
// closed-ref default comparison. Only an answer that is not one JSON object
// is refused.
func ResolveResponse(compilation Compilation, raw []byte) (Selection, error) {
	result, err := resolveResponse(compilation, raw)
	return result.Selection, err
}

func resolveResponse(compilation Compilation, raw []byte) (batchSelection, error) {
	if err := validateCompilation(compilation); err != nil {
		return batchSelection{}, err
	}
	if len(raw) == 0 || len(raw) > MaxResponseBytes {
		return batchSelection{}, fmt.Errorf("target portfolio: response exceeds bounded envelope")
	}
	var wire struct {
		DefaultFileRef  json.RawMessage `json:"default_file_ref"`
		TargetFileRefs  json.RawMessage `json:"target_file_refs"`
		NativeDecisions json.RawMessage `json:"native_decisions"`
		LaunchDecisions json.RawMessage `json:"launch_decisions"`
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' ||
		json.Unmarshal(raw, &wire) != nil {
		return batchSelection{}, fmt.Errorf("target portfolio: response must be one JSON object")
	}
	var rejected []llm.ResponseRejection
	reject := func(position, reason string) {
		rejected = append(rejected, llm.ResponseRejection{
			Kind: "portfolio_rejected", Count: 1, Samples: []string{position}, Reason: reason,
		})
	}

	authority := make(map[corpus.FileID]VisibleCandidate, len(compilation.Request.Candidates))
	for _, candidate := range compilation.Request.Candidates {
		authority[candidate.FileRef] = candidate
	}
	var targetValues []json.RawMessage
	if !absentJSON(wire.TargetFileRefs) && json.Unmarshal(wire.TargetFileRefs, &targetValues) != nil {
		reject("target_file_refs", "target_file_refs is not an array")
		targetValues = nil
	}
	targetSet := make(map[corpus.FileID]struct{}, len(targetValues)+len(compilation.requiredTargetFileRefs))
	for index, value := range targetValues {
		position := fmt.Sprintf("target_file_refs[%d]", index)
		var fileRef corpus.FileID
		if json.Unmarshal(value, &fileRef) != nil {
			reject(position, "target file ref is not text")
			continue
		}
		if _, known := authority[fileRef]; !known {
			reject(position, "target file ref was not advertised")
			continue
		}
		targetSet[fileRef] = struct{}{}
	}
	for _, fileRef := range compilation.requiredTargetFileRefs {
		targetSet[fileRef] = struct{}{}
	}

	var chosen *VisibleCandidate
	if !absentJSON(wire.DefaultFileRef) {
		// An unusable default is discarded, never added to the targets.
		var defaultRef corpus.FileID
		if json.Unmarshal(wire.DefaultFileRef, &defaultRef) != nil {
			reject("default_file_ref", "default file ref is not text")
		} else if candidate, known := authority[defaultRef]; !known {
			reject("default_file_ref", "default file ref was not advertised")
		} else if _, selected := targetSet[defaultRef]; !selected {
			reject("default_file_ref", "default file ref is not a selected target")
		} else {
			defaultCopy := cloneVisibleCandidate(candidate)
			chosen = &defaultCopy
		}
	}

	nativeRefs := make(map[string]struct{}, len(compilation.native))
	for _, row := range compilation.native {
		nativeRefs[row.Ref] = struct{}{}
	}
	var decisionValues []json.RawMessage
	if !absentJSON(wire.NativeDecisions) && json.Unmarshal(wire.NativeDecisions, &decisionValues) != nil {
		reject("native_decisions", "native_decisions is not an array")
		decisionValues = nil
	}
	decisions := make([]NativeDecision, 0, len(decisionValues))
	for index, value := range decisionValues {
		position := fmt.Sprintf("native_decisions[%d]", index)
		var row struct {
			Ref      json.RawMessage `json:"ref"`
			Decision json.RawMessage `json:"decision"`
		}
		var ref string
		if json.Unmarshal(value, &row) != nil || json.Unmarshal(row.Ref, &ref) != nil || ref == "" {
			reject(position, "native decision row has no text ref")
			continue
		}
		if _, known := nativeRefs[ref]; !known {
			reject(position, "native decision ref was not advertised")
			continue
		}
		var decision string
		if json.Unmarshal(row.Decision, &decision) != nil {
			// The row names its target but no readable decision. The target
			// keeps its standalone fallback, journaled with the original value;
			// the unreadable value can never equal a closed decision word.
			decision = string(bytes.TrimSpace(row.Decision))
		}
		decisions = append(decisions, NativeDecision{Ref: ref, Decision: decision})
	}
	var launchDecisions []NativeLaunchDecision
	if !absentJSON(wire.LaunchDecisions) && json.Unmarshal(wire.LaunchDecisions, &launchDecisions) != nil {
		reject("launch_decisions", "launch_decisions is not an array")
		launchDecisions = nil
	}

	result := batchSelection{
		Selection: Selection{
			Placements:   resolveNativeLaunchDecisions(compilation.native, decisions, launchDecisions),
			Default:      chosen,
			Targets:      make([]VisibleCandidate, 0, len(targetSet)),
			Unclassified: make([]VisibleCandidate, 0, len(compilation.Request.Candidates)-len(targetSet)),
		},
		rejected: rejected,
	}
	for _, candidate := range compilation.Request.Candidates {
		if _, selected := targetSet[candidate.FileRef]; selected {
			result.Targets = append(result.Targets, cloneVisibleCandidate(candidate))
			continue
		}
		result.Unclassified = append(result.Unclassified, cloneVisibleCandidate(candidate))
	}
	return result, nil
}

// absentJSON reports an omitted field or an explicit null: the same "nothing
// said" in either form.
func absentJSON(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}
