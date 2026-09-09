package reading

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
)

type answerRowsAdapter struct {
	*answerTestProvider
	accepted [][]string
}

func (p *answerRowsAdapter) AdaptResponse(_, response []byte) (llm.AdaptedResponse, error) {
	return llm.AdaptedResponse{Domain: response, Accept: func(rows []string) { p.accepted = append(p.accepted, rows) }}, nil
}

func TestIndependentAnswersPreserveNeighboursExactCacheAndReplay(t *testing.T) {
	for name, invalidate := range map[string]func(map[string]any){
		"wrong cell type":            func(row map[string]any) { row["answer"] = 42 },
		"unknown source":             func(row map[string]any) { row["sources"] = "c999" },
		"contradictory completeness": func(row map[string]any) { row["state"] = "answered" },
	} {
		t.Run(name, func(t *testing.T) {
			provider := &answerRowsAdapter{answerTestProvider: &answerTestProvider{tableProvider: &tableProvider{}}}
			prose := "The declaration supplies this answer."
			provider.completeAnswer = func(request answerTestRequest) ([]byte, error) {
				var rows []map[string]any
				for _, input := range request.Rows {
					row := map[string]any{"key": input["key"], "answer": prose, "basis": "Original source declaration.", "sources": input["candidate_options"].([]any)[0], "state": "partial", "remaining": "The implementation was not inspected.", "extra": []int{1}}
					if input["question"] == "Question 01?" {
						invalidate(row)
					}
					rows = append(rows, row)
				}
				return json.Marshal(map[string]any{"rows": rows, "extra": true})
			}
			r := answerTestReader(t, answerTestRoutes(3), provider)
			if err := r.readAnswers(t.Context()); err != nil {
				t.Fatal(err)
			}
			if len(provider.requests) != 1 || len(r.rejected) != 1 || r.rejected[0].Kind != "row_rejected" || r.rejected[0].Samples[0] != "r2" || r.rejected[0].ResponseRef == "" {
				t.Fatalf("one invalid answer lost row diagnostics or generated retries: %+v", r.rejected)
			}
			check := func(reader *reader, source string) {
				t.Helper()
				for i, question := range reader.questions {
					part := question.Answer.Parts[0]
					if i == 1 {
						if question.Answer.State != "unavailable" || part.Source != atlas.SourceGiven || part.Text != "" || len(question.Guide.Steps) != 0 {
							t.Fatal("invalid answer became prose, a negative, or a guide")
						}
					} else if question.Answer.State != "partial" || part.Source != source || part.Text != prose || len(part.Steps) != 1 || len(question.Guide.Steps) != 1 {
						t.Fatalf("valid answer or original sources lost: %+v", question)
					}
				}
			}
			check(r, atlas.SourceModel)
			var parts []answerQuestion
			for i, question := range r.questions {
				parts = append(parts, answerQuestion{index: i, complete: true, candidates: uniqueRouteAnchors(question.Stops)})
			}
			windows, err := r.planAnswers(t.Context(), lines.Answer(), parts)
			if err != nil || len(windows) != 1 {
				t.Fatalf("changed original evidence union: %v", err)
			}
			call, err := answerCall(lines.Answer(), windows[0])
			if err != nil {
				t.Fatal(err)
			}
			cached, err := llm.RecallJSON(t.Context(), r.opts.Executor, provider, call)
			if err != nil || !cached.Cached || cached.Value.Answers[1] != nil || len(cached.Value.Rejections) != 1 {
				t.Fatalf("accepted neighbours lost the shared exact response: %+v / %v", cached, err)
			}
			warm := answerTestReader(t, answerTestRoutes(3), provider)
			warm.opts.Executor = r.opts.Executor
			if err := warm.readAnswers(t.Context()); err != nil {
				t.Fatal(err)
			}
			check(warm, atlas.SourceCache)
			if len(provider.requests) != 1 {
				t.Fatal("same partial response was called again")
			}
			prose = "The replay supplies the updated answer."
			prepared, err := llm.NewPrepared(cached.Request)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := llm.ReplayJSON(t.Context(), r.opts.Executor, provider, prepared); err != nil {
				t.Fatal(err)
			}
			updated := answerTestReader(t, answerTestRoutes(3), provider)
			updated.opts.Executor = r.opts.Executor
			if err := updated.readAnswers(t.Context()); err != nil {
				t.Fatal(err)
			}
			check(updated, atlas.SourceCache)
			if len(provider.requests) != 2 {
				t.Fatal("replay did not refresh the existing shared cache entry")
			}
			for _, accepted := range provider.accepted {
				if !reflect.DeepEqual(accepted, []string{"r1", "r3"}) {
					t.Fatalf("refused answer authorized terminology: %v", accepted)
				}
			}
		})
	}
}
