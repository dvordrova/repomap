package reading

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

// Ordinary planning uses the fully prepared provider envelope. Explicit read
// budgets remain available, but neither a row quota nor 64 KiB limits this stage.
func (r *reader) planAnswers(ctx context.Context, def table.Definition, parts []answerQuestion) ([]answerWindow, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		return nil, nil
	}
	window, err := makeAnswerWindow(def, r.questions, parts)
	if err != nil {
		return nil, err
	}
	tooLarge := def.Window > 0 && len(parts) > def.Window || def.MaxInputBytes > 0 && len(def.System)+len(window.table.Request) > def.MaxInputBytes
	if !tooLarge && !r.dry {
		call, err := answerCall(def, window)
		if err != nil {
			return nil, err
		}
		prepared, err := llm.Prepare(r.opts.Provider, call.Prompt, call.Limits)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if !answerResourceFailure(err) {
				return nil, fmt.Errorf("answer: prepare: %w", err)
			}
			tooLarge = true
		} else {
			tooLarge = prepared.Len() > call.Limits.MaxRequestBytes
		}
	}
	if !tooLarge {
		return []answerWindow{window}, nil
	}
	children, err := r.splitAnswerWindow(def, parts)
	if err != nil {
		return nil, err
	}
	if len(children) == 0 {
		return nil, fmt.Errorf("answer: complete source and question singleton cannot fit the request envelope")
	}
	var result []answerWindow
	for _, child := range children {
		planned, err := r.planAnswers(ctx, def, child)
		if err != nil {
			return nil, err
		}
		result = append(result, planned...)
	}
	return result, nil
}

// Questions have different source sets. Split questions first and rebuild each
// child's union, preserving complete evidence per question whenever possible.
func (r *reader) splitAnswerWindow(def table.Definition, parts []answerQuestion) ([][]answerQuestion, error) {
	if len(parts) > 1 {
		weights := make([]int, len(parts))
		for i, part := range parts {
			window, err := makeAnswerWindow(def, r.questions, []answerQuestion{part})
			if err != nil {
				return nil, err
			}
			weights[i] = len(window.table.Request)
		}
		at := answerSplitPoint(weights)
		return [][]answerQuestion{parts[:at], parts[at:]}, nil
	}
	if len(parts) == 0 || len(parts[0].candidates) < 2 {
		return nil, nil
	}
	part := parts[0]
	weights := make([]int, len(part.candidates))
	for i, candidate := range part.candidates {
		source, err := sourceObservation(r.questions[part.index], candidate)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(source)
		if err != nil {
			return nil, err
		}
		weights[i] = len(raw)
	}
	at := answerSplitPoint(weights)
	return [][]answerQuestion{{{index: part.index, candidates: part.candidates[:at], complete: false}}, {{index: part.index, candidates: part.candidates[at:], complete: false}}}, nil
}

func answerSplitPoint(weights []int) int {
	total := 0
	for _, weight := range weights {
		total += weight
	}
	best, distance, prefix := 1, total, 0
	for i, weight := range weights[:len(weights)-1] {
		prefix += weight
		delta := prefix - (total - prefix)
		if delta < 0 {
			delta = -delta
		}
		if delta < distance {
			best, distance = i+1, delta
		}
	}
	return best
}

func answerResourceFailure(err error) bool {
	var resource *llm.ResourceLimitError
	if !errors.As(err, &resource) {
		return false
	}
	switch resource.Kind {
	case llm.ResourceLimitRequestBytes, llm.ResourceLimitResponseBytes, llm.ResourceLimitOutputTokens, llm.ResourceLimitContextTokens:
		return true
	}
	return false
}

func (r *reader) writeAnswerWindow(def table.Definition, window answerWindow, outcome llm.Outcome[table.Result], failure error, superseded bool) error {
	source, reason := atlas.SourceModel, ""
	if outcome.Cached {
		source = atlas.SourceCache
	}
	if r.dry {
		source = atlas.SourceGiven
		reason = "no provider"
	}
	if failure != nil {
		source = atlas.SourceGiven
		reason = failure.Error()
	}
	result, err := json.MarshalIndent(struct {
		Source     string               `json:"source"`
		Reason     string               `json:"reason,omitempty"`
		Superseded bool                 `json:"superseded,omitempty"`
		Rows       table.Answers        `json:"rows"`
		Rejections []table.RowRejection `json:"rejections,omitempty"`
	}{source, reason, superseded, outcome.Value.Answers, outcome.Value.Rejections}, "", "  ")
	if err != nil {
		return err
	}
	files := []struct {
		name string
		data []byte
	}{{"prompt.md", []byte(def.System)}, {"input.json", window.table.Request}, {"request.json", outcome.Request}, {"response.json", outcome.Response}, {"result.json", result}}
	r.questionKey = ""
	defer func() { r.questionKey = "" }()
	// Question-keyed references all point to the same immutable complete bytes.
	// The accepted answer's OriginRow distinguishes its result within that call.
	keys := []string{""}
	for _, part := range window.parts {
		keys = append(keys, fmt.Sprintf("%x", sha256.Sum256([]byte(r.questions[part.index].Question))))
	}
	for _, key := range keys {
		r.questionKey = key
		for _, file := range files {
			// The complete normalized result lives once beside the shared input;
			// each question already retains its own restored answer and row origin.
			if key != "" && file.name == "result.json" {
				continue
			}
			if len(file.data) > 0 {
				if err := r.writeWindowFile(window.table, file.name, file.data); err != nil {
					return err
				}
			}
		}
	}
	r.questionKey = ""
	if failure != nil && !superseded {
		r.use(def.Stage).Rejected++
		responseRef := ""
		if len(outcome.Response) > 0 {
			responseRef = filepath.ToSlash(filepath.Join(atlas.TablesDir, r.windowFileName(window.table, "response.ref.json")))
		}
		r.rejected = append(r.rejected, modeldiag.Row{Stage: def.Stage, Kind: "window_rejected", Count: len(window.parts), Reason: reason, ResponseRef: responseRef, Samples: []string{fmt.Sprintf("shared window %d", window.table.Index)}})
	}
	if failure == nil && len(outcome.Value.Rejections) > 0 {
		rejected := len(window.parts) - len(outcome.Value.AcceptedRowKeys())
		if rejected > 0 {
			r.use(def.Stage).Rejected++
			reason = fmt.Sprintf("%d answers rejected, %d accepted in this response", rejected, len(window.parts)-rejected)
			r.opts.State(def.Stage, "ready", reason)
		}
		responseRef := filepath.ToSlash(filepath.Join(atlas.TablesDir, r.windowFileName(window.table, "response.ref.json")))
		for _, rejection := range outcome.Value.Rejections {
			r.rejected = append(r.rejected, modeldiag.Row{Stage: def.Stage, Kind: "row_rejected", Count: 1, Reason: rejection.Reason, ResponseRef: responseRef, Samples: []string{rejection.Key}})
		}
	}
	if superseded {
		reason = "Provider resource refusal; complete partitions follow. This attempt supplies no answers."
		if !answerResourceFailure(failure) {
			reason = fmt.Sprintf("Provider refusal (%s); complete partitions follow. This attempt supplies no answers.", failure)
		}
	}
	if reason != "" {
		source += "; " + reason
	}
	r.printWindow(def, window.table, outcome.Value.Answers, source, outcome.Metrics.Latency)
	return nil
}
