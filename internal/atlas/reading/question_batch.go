package reading

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/questionbatch"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

// readQuestionBatch finishes the shared retrieval before any question's route
// or answer is read. Source restoration still uses the original chunks and
// each question's separate result; batching changes neither graph nor identity.
func (r *reader) readQuestionBatch(ctx context.Context, chunks []lines.QuestionChunk, questions []string) ([][]questionbatch.ChunkResult, error) {
	answers := make([][]questionbatch.ChunkResult, len(questions))
	for q := range questions {
		answers[q] = make([]questionbatch.ChunkResult, len(chunks))
	}
	if len(questions) == 0 {
		return answers, nil
	}
	r.started[lines.StageQuestion] = time.Now()
	r.opts.Stage(lines.StageQuestion, fmt.Sprintf("inspecting %d complete evidence chunks once for %d independent questions", len(chunks), len(questions)))
	use := r.use(lines.StageQuestion)
	use.Rows += len(chunks) * len(questions)
	if r.dry {
		use.Given += len(chunks) * len(questions)
		fmt.Fprintf(&r.tables, "## %s\n\nNo provider: %d questions, %d complete evidence chunks remain unavailable.\n\n", lines.StageQuestion, len(questions), len(chunks))
		r.reportStage(lines.StageQuestion)
		return answers, nil
	}
	options := questionbatch.Options{}
	if r.opts.Through == "" || r.opts.Through == lines.StageQuestion {
		options.System = r.opts.Prompt
		options.MaxInputBytes = r.opts.InputBytes
		options.MaxRows = r.opts.WindowRows
	}
	result, err := questionbatch.Run(ctx, debugdump.BindStage(r.opts.Executor, lines.StageQuestion), r.opts.Provider,
		questionbatch.Input{Repository: r.opts.Repository, Chunks: chunks, Questions: questions}, options)
	if err != nil {
		return nil, err
	}
	if len(result.Questions) != len(questions) {
		return nil, fmt.Errorf("question: retrieval did not preserve question slots")
	}
	for q, result := range result.Questions {
		if result.Question != questions[q] || len(result.Chunks) != len(chunks) {
			return nil, fmt.Errorf("question: retrieval did not preserve evidence slots")
		}
		for i, chunk := range result.Chunks {
			if !chunk.Inspected {
				use.Given++
				continue
			}
			answers[q][i] = chunk
		}
	}
	for i, exchange := range result.Exchanges {
		use.Windows++
		if exchange.Reused {
			use.Reused += len(exchange.ChunkIndexes) * len(exchange.QuestionIndexes)
		}
		if exchange.Outcome.Cached {
			use.Cached++
		} else if exchange.Outcome.RequestBytes > 0 {
			use.Live++
		}
		if exchange.Superseded && r.opts.State != nil {
			r.opts.State("Question source selection", "partitioned", fmt.Sprintf("the provider refused %d questions with %d evidence groups in one request by resources; complete partitions follow", len(exchange.QuestionIndexes), len(exchange.ChunkIndexes)))
		}
		if exchange.Err != nil && !exchange.Superseded {
			use.Rejected++
			responseRef := ""
			if len(exchange.Outcome.Response) > 0 {
				responseRef = filepath.ToSlash(filepath.Join(atlas.TablesDir, r.windowFileName(table.Window{Stage: lines.StageQuestion, Index: i}, "response.ref.json")))
			}
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageQuestion, Kind: "window_rejected", Count: len(exchange.ChunkIndexes) * len(exchange.QuestionIndexes), Reason: exchange.Err.Error(), ResponseRef: responseRef,
				Samples: []string{fmt.Sprintf("shared window %d", i)}})
			details := []string{fmt.Sprintf("affected questions: %d; unavailable evidence groups per question: %d", len(exchange.QuestionIndexes), len(exchange.ChunkIndexes)), "reason: " + exchange.Err.Error()}
			for _, q := range exchange.QuestionIndexes {
				details = append(details, "question: "+questions[q])
			}
			r.opts.State("Question source selection", "response rejected", details...)
		}
		if exchange.Err == nil && len(exchange.Outcome.Value.Rejections) > 0 {
			rejected := 0
			journaled := false
			if observer, ok := r.opts.Executor.Observer.(interface{ JournalsRejections() bool }); ok {
				journaled = observer.JournalsRejections()
			}
			var details []string
			for _, rejection := range exchange.Outcome.Value.Rejections {
				for q, ref := range exchange.QuestionRefs {
					if rejection.Question != ref {
						continue
					}
					rejected++
					details = append(details, "question: "+questions[exchange.QuestionIndexes[q]], "reason: "+rejection.Reason)
					r.rejected = append(r.rejected, modeldiag.Row{AlreadyJournaled: journaled, Stage: lines.StageQuestion, Kind: "question_rejected", Count: rejection.Chunks, Reason: rejection.Reason,
						ResponseRef: filepath.ToSlash(filepath.Join(atlas.TablesDir, r.windowFileName(table.Window{Stage: lines.StageQuestion, Index: i}, "response.ref.json"))), Samples: []string{rejection.Question}})
				}
			}
			if rejected > 0 {
				use.Rejected++
				state := "partly accepted"
				if rejected == len(exchange.QuestionRefs) {
					state = "response rejected"
				}
				details = append([]string{fmt.Sprintf("questions in this response: %d accepted, %d rejected", len(exchange.QuestionRefs)-rejected, rejected)}, details...)
				r.opts.State("Question source selection", state, details...)
			}
		}
		if err := r.writeQuestionExchange(i, exchange, questions); err != nil {
			return nil, err
		}
	}
	for _, issue := range result.Issues {
		r.opts.State(lines.StageQuestion, "cache miss", issue.Error())
	}
	r.reportStage(lines.StageQuestion)
	return answers, nil
}

func (r *reader) writeQuestionExchange(index int, exchange questionbatch.Exchange, questions []string) error {
	window := table.Window{Stage: lines.StageQuestion, Index: index}
	r.questionKey = ""
	files := []struct {
		name string
		data []byte
	}{
		{"prompt.md", []byte(exchange.System)}, {"input.json", exchange.Input},
		{"request.json", exchange.Outcome.Request}, {"response.json", exchange.Outcome.Response},
	}
	for _, file := range files {
		if len(file.data) > 0 {
			if err := r.writeWindowFile(window, file.name, file.data); err != nil {
				return err
			}
		}
	}
	source := atlas.SourceModel
	if exchange.Outcome.Cached {
		source = atlas.SourceCache
	}
	reason := ""
	if exchange.Err != nil {
		source = atlas.SourceGiven
		reason = exchange.Err.Error()
	}
	raw, err := json.MarshalIndent(struct {
		Source     string                 `json:"source"`
		Reason     string                 `json:"reason,omitempty"`
		Superseded bool                   `json:"superseded,omitempty"`
		Result     questionbatch.Response `json:"result"`
	}{source, reason, exchange.Superseded, exchange.Outcome.Value}, "", "  ")
	if err != nil {
		return err
	}
	if err := r.writeWindowFile(window, "result.json", raw); err != nil {
		return err
	}
	// A question still has its own request references. Several references may
	// lead to one shared immutable payload, so source bytes are never duplicated.
	for _, q := range exchange.QuestionIndexes {
		if q < 0 || q >= len(questions) {
			return fmt.Errorf("question: exchange has an invalid question position")
		}
		r.questionKey = fmt.Sprintf("%x", sha256.Sum256([]byte(questions[q])))
		for _, file := range files {
			if len(file.data) > 0 {
				if err := r.writeWindowFile(window, file.name, file.data); err != nil {
					return err
				}
			}
		}
	}
	r.questionKey = ""
	fmt.Fprintf(&r.tables, "## %s · shared window %d · %s\n\nsource: %s\n", lines.StageQuestion, index, filepath.ToSlash(filepath.Join(atlas.TablesDir, r.windowFileName(window, "request.ref.json"))), source)
	if exchange.Superseded {
		r.tables.WriteString("Provider resource refusal; complete partitions follow. This attempt supplies no source decisions.\n")
	}
	if reason != "" {
		fmt.Fprintf(&r.tables, "reason: %s\n", reason)
	}
	fmt.Fprintf(&r.tables, "\nInput (shared evidence once):\n\n```json\n%s\n```\n\nResult:\n\n```json\n%s\n```\n\n", exchange.Input, raw)
	return nil
}
