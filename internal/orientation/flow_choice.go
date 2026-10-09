package orientation

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"slices"

	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

//go:embed flow-selection.md
var flowSelectionPrompt string

//go:embed flow-comparison.md
var flowComparisonPrompt string

type flowChoiceCall struct {
	call     llm.Call[table.Result]
	verdicts map[string]llm.Verdict
	window   table.Window
	entries  []map[string]any
}

func newFlowChoiceCall(c llm.Categorizer, def table.Definition, window table.Window, entries []map[string]any) (*flowChoiceCall, error) {
	row := window.Rows[0]
	row.Fields = slices.Clone(row.Fields)
	for i, field := range row.Fields {
		if field.Name == "candidates" {
			row.Fields[i].Value = entries
		}
	}
	window.Rows = []table.Row{row}
	call, err := table.ClassifierCall(c, def, window)
	if err != nil {
		return nil, err
	}
	read := &flowChoiceCall{call: call, window: window, entries: entries}
	decode := call.DecodeValidate
	read.call.DecodeValidate = func(raw []byte) (table.Result, error) {
		result, err := decode(raw)
		// A failed known decision supplies no shortlist or global alternatives.
		// Only a fully validated answer (including explicit uncertainty) can
		// contribute its probabilities to the owning Main-flow decision.
		read.verdicts = nil
		if err == nil {
			written, _ := c.Verdicts(raw)
			key := window.Rows[0].ID + "|next"
			labels := make([]string, len(entries))
			for i, entry := range entries {
				labels[i], _ = entry["title"].(string)
				if labels[i] == "" {
					labels[i], _ = entry["ref"].(string)
				}
			}
			verdict := table.LabelForm(written[key], labels)
			probabilities := make(map[string]float64, len(labels))
			for _, label := range labels {
				if value, present := verdict.Probabilities[label]; present {
					probabilities[label] = value
				}
			}
			verdict.Probabilities = probabilities
			read.verdicts = map[string]llm.Verdict{key: verdict}
		}
		return result, err
	}
	return read, nil
}

func prepareFlowChoice(c llm.Categorizer, call llm.Call[table.Result]) (int, error) {
	prepared, err := llm.Prepare(c, call.Prompt, call.Limits)
	if err == nil {
		if prepared.Len() > call.Limits.MaxRequestBytes {
			return prepared.Len(), llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitRequestBytes, Limit: call.Limits.MaxRequestBytes, Observed: prepared.Len(), ObservedKnown: true})
		}
		return prepared.Len(), nil
	}
	var resource *llm.ResourceLimitError
	if errors.As(err, &resource) && resource.ObservedKnown {
		size := resource.Observed
		if resource.Kind == llm.ResourceLimitContextTokens {
			size -= resource.ConfiguredMaxTokens
		}
		return size, err
	}
	return 0, err
}

func flowInputRefusal(err error) bool {
	var resource *llm.ResourceLimitError
	return errors.As(err, &resource) && (resource.Kind == llm.ResourceLimitContextTokens || resource.Kind == llm.ResourceLimitRequestBytes)
}

// fitFlowChoices partitions full original candidate records, never their
// criteria or the shared step. All leaves are prepared before any selection
// calls, so an indivisible input cannot consume model work on its neighbours.
func fitFlowChoices(c llm.Categorizer, def table.Definition, window table.Window, entries []map[string]any) ([]*flowChoiceCall, error) {
	var plan []*flowChoiceCall
	var fit func([]map[string]any) error
	fit = func(entries []map[string]any) error {
		read, err := newFlowChoiceCall(c, def, window, entries)
		if err != nil {
			return err
		}
		_, err = prepareFlowChoice(c, read.call)
		if err == nil {
			plan = append(plan, read)
			return nil
		}
		if !flowInputRefusal(err) || len(entries) < 2 {
			return err
		}
		half := len(entries) / 2
		if err := fit(entries[:half]); err != nil {
			return err
		}
		return fit(entries[half:])
	}
	err := fit(entries)
	return plan, err
}

func splitFlowChoices(c llm.Categorizer, def table.Definition, window table.Window, entries []map[string]any) ([]*flowChoiceCall, error) {
	if len(entries) < 2 {
		return nil, fmt.Errorf("orientation flow: indivisible candidate input")
	}
	half := len(entries) / 2
	left, err := fitFlowChoices(c, def, window, entries[:half])
	if err != nil {
		return nil, err
	}
	right, err := fitFlowChoices(c, def, window, entries[half:])
	return append(left, right...), err
}

// Unexpected actual input refusals may make a prepared-fitting leaf smaller.
// Accepted neighbours are retained in memory. Semantic/output failures do not
// change a choice's input or stand in for a new model decision.
func executeFlowSelections(ctx context.Context, executor llm.Executor, c llm.Categorizer, def table.Definition, window table.Window, plan []*flowChoiceCall) ([]*flowChoiceCall, []llm.EachResult[table.Result], error) {
	type node struct {
		read   *flowChoiceCall
		result llm.EachResult[table.Result]
		done   bool
	}
	nodes := make([]node, len(plan))
	for i, read := range plan {
		nodes[i].read = read
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		var calls []llm.Call[table.Result]
		for _, item := range nodes {
			if !item.done {
				calls = append(calls, item.read.call)
			}
		}
		if len(calls) == 0 {
			reads := make([]*flowChoiceCall, len(nodes))
			results := make([]llm.EachResult[table.Result], len(nodes))
			for i, item := range nodes {
				reads[i], results[i] = item.read, item.result
			}
			return reads, results, nil
		}
		results := llm.ExecuteJSONEach(ctx, executor, c, calls)
		var next []node
		at := 0
		for _, item := range nodes {
			if item.done {
				next = append(next, item)
				continue
			}
			item.result, item.done = results[at], true
			at++
			if flowInputRefusal(item.result.Err) && len(item.read.entries) > 1 {
				children, err := splitFlowChoices(c, def, window, item.read.entries)
				if err != nil {
					return nil, nil, err
				}
				for _, child := range children {
					next = append(next, node{read: child})
				}
				continue
			}
			next = append(next, item)
		}
		nodes = next
	}
}

func flowShortlist(read *flowChoiceCall, result table.Result) ([]map[string]any, error) {
	if len(result.Answers) != 1 {
		return nil, fmt.Errorf("orientation flow: missing required selection row")
	}
	if chosen := result.Answers[0]["next"]; chosen != "" {
		for _, entry := range read.entries {
			if entry["ref"] == chosen {
				return []map[string]any{entry}, nil
			}
		}
		return nil, fmt.Errorf("orientation flow: selection outside original closed choices")
	}
	if !result.Uncertain(0) {
		return nil, fmt.Errorf("orientation flow: missing required selection decision")
	}
	key := read.window.Rows[0].ID + "|next"
	verdict, present := read.verdicts[key]
	if !present || verdict.Conflict {
		return nil, fmt.Errorf("orientation flow: missing or conflicting required selection decision")
	}
	labels := make([]string, len(read.entries))
	for i, entry := range read.entries {
		labels[i], _ = entry["title"].(string)
	}
	verdict = table.LabelForm(verdict, labels)
	positions := withinMargin(verdict, labels)
	if len(positions) < 2 {
		return nil, fmt.Errorf("orientation flow: uncertain selection has no supported alternatives")
	}
	kept := make([]map[string]any, 0, len(positions))
	// Original order, identities and full native/model fields survive. Local
	// normalized probabilities are not carried into the independent comparison.
	for i, entry := range read.entries {
		if slices.Contains(positions, i) {
			kept = append(kept, entry)
		}
	}
	return kept, nil
}

// executeFlowChoice preserves the ordinary complete single-question path when
// it fits. Only an actual input envelope refusal activates model selection of
// original records. Local outcomes are provisional eliminations, not a global
// probability ranking; a final independent model decision compares the full
// surviving records. Failed leaves invalidate that dependent choice alone.
func executeFlowChoice(ctx context.Context, executor llm.Executor, c llm.Categorizer, def table.Definition, window table.Window) (llm.Outcome[table.Result], map[string]llm.Verdict, []string, error) {
	var entries []map[string]any
	for _, field := range window.Rows[0].Fields {
		if field.Name == "candidates" {
			entries, _ = field.Value.([]map[string]any)
		}
	}
	if len(entries) == 0 {
		return llm.Outcome[table.Result]{}, nil, nil, fmt.Errorf("orientation flow: missing original candidate catalogue")
	}
	current := entries
	finalDef := def
	var selected []string
	previous := 0
	refused := map[string]error{}
	for {
		if err := ctx.Err(); err != nil {
			return llm.Outcome[table.Result]{}, nil, selected, err
		}
		read, err := newFlowChoiceCall(c, finalDef, window, current)
		if err != nil {
			return llm.Outcome[table.Result]{}, nil, selected, err
		}
		size, err := prepareFlowChoice(c, read.call)
		forceSplit := false
		if err == nil {
			if prior, identical := refused[read.call.Prompt.User]; identical {
				return llm.Outcome[table.Result]{}, nil, selected, fmt.Errorf("orientation flow: unchanged refused candidate request: %w", prior)
			}
			outcome, err := llm.ExecuteJSON(ctx, executor, c, read.call)
			if !flowInputRefusal(err) || len(current) < 2 {
				return outcome, read.verdicts, selected, err
			}
			refused[read.call.Prompt.User] = err
			forceSplit = true
			// Exact prepared bytes already measured the request. An actual
			// provider refusal need not repeat that scalar in its response.
			if previous > 0 && size >= previous {
				return outcome, nil, selected, fmt.Errorf("orientation flow: model selection cannot reduce the complete prepared candidate request: %w", err)
			}
		}
		if !forceSplit && !flowInputRefusal(err) {
			return llm.Outcome[table.Result]{}, nil, selected, err
		}
		if size <= 0 {
			return llm.Outcome[table.Result]{}, nil, selected, fmt.Errorf("orientation flow: provider refusal has no complete prepared byte measurement: %w", err)
		}
		if previous > 0 && (size == 0 || size >= previous) {
			return llm.Outcome[table.Result]{}, nil, selected, fmt.Errorf("orientation flow: model selection cannot reduce the complete prepared candidate request: %w", err)
		}
		selectionDef := def
		selectionDef.Contract += ".selection.v1"
		selectionDef.System += "\n" + flowSelectionPrompt
		var plan []*flowChoiceCall
		var fitErr error
		if forceSplit {
			plan, fitErr = splitFlowChoices(c, selectionDef, window, current)
		} else {
			plan, fitErr = fitFlowChoices(c, selectionDef, window, current)
		}
		if fitErr != nil {
			if flowInputRefusal(fitErr) {
				if forceSplit {
					return llm.Outcome[table.Result]{}, nil, selected, fitErr
				}
				// Preserve the ordinary exact typed unsent refusal diagnostics.
				outcome, failure := llm.ExecuteJSON(ctx, executor, c, read.call)
				return outcome, nil, selected, failure
			}
			return llm.Outcome[table.Result]{}, nil, selected, fitErr
		}
		plan, results, err := executeFlowSelections(ctx, executor, c, selectionDef, window, plan)
		if err != nil {
			return llm.Outcome[table.Result]{}, nil, selected, err
		}
		var next []map[string]any
		for i, result := range results {
			if result.Err != nil {
				return result.Outcome, nil, selected, result.Err
			}
			kept, err := flowShortlist(plan[i], result.Outcome.Value)
			if err != nil {
				return result.Outcome, nil, selected, err
			}
			next = append(next, kept...)
		}
		previous, current = size, next
		selected = selected[:0]
		for _, entry := range current {
			ref, _ := entry["ref"].(string)
			selected = append(selected, ref)
		}
		finalDef = def
		finalDef.Contract += ".comparison.v1"
		finalDef.System += "\n" + flowComparisonPrompt
	}
}
