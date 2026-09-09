package reading

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
)

func (r *reader) readAnswers(ctx context.Context) error {
	if len(r.questions) == 0 {
		return nil
	}
	def := lines.Answer()
	if r.opts.Through == "" || r.opts.Through == lines.StageAnswer {
		if r.opts.Prompt != "" {
			def.System = r.opts.Prompt
		}
		def.MaxInputBytes, def.Window = r.opts.InputBytes, r.opts.WindowRows
	}
	var parts []answerQuestion
	for i := range r.questions {
		route := &r.questions[i]
		route.Answer = &atlas.QuestionAnswer{State: "unavailable", Parts: []atlas.QuestionAnswerPart{}}
		candidates := uniqueRouteAnchors(route.Stops)
		if len(candidates) == 0 {
			if route.Coverage.UnresolvedChunks == 0 {
				route.Answer.State = "unanswered"
			}
		} else {
			parts = append(parts, answerQuestion{index: i, candidates: candidates, complete: true})
		}
	}
	sort.SliceStable(parts, func(i, j int) bool {
		return r.questions[parts[i].index].Question < r.questions[parts[j].index].Question
	})
	r.started[lines.StageAnswer] = time.Now()
	r.opts.Stage(lines.StageAnswer, fmt.Sprintf("answering %d questions from their shared original sources", len(parts)))
	use := r.use(lines.StageAnswer)
	planned, err := r.planAnswers(ctx, def, parts)
	if err != nil {
		return err
	}
	windowIndex := 0
	for len(planned) > 0 {
		calls := make([]llm.Call[table.Answers], len(planned))
		for i := range planned {
			planned[i].table.Index = windowIndex
			windowIndex++
			calls[i], err = answerCall(def, planned[i])
			if err != nil {
				return err
			}
		}
		outcomes := make([]llm.Outcome[table.Answers], len(planned))
		failures := make([]error, len(planned))
		failureKinds := make(map[string]llm.FailureKind)
		if !r.dry {
			executor := debugdump.BindStage(r.opts.Executor, lines.StageAnswer)
			observer := executor.Observer
			executor.Observer = llm.ObserverFunc(func(event llm.Event) error {
				if event.Kind == llm.EventFailure && event.Source == llm.SourceLive {
					failureKinds[event.RequestSHA256] = event.Failure
				}
				if observer != nil {
					return observer.Observe(event)
				}
				return nil
			})
			if executor.PlanNotice != nil {
				executor.PlanNotice(len(calls))
			}
			results := llm.ExecuteJSONEach(ctx, executor, r.opts.Provider, calls)
			if err := ctx.Err(); err != nil {
				return err
			}
			for i, result := range results {
				outcomes[i], failures[i] = result.Outcome, result.Err
			}
		}

		var next []answerWindow
		for i, window := range planned {
			outcome, failure := outcomes[i], failures[i]
			// Optional cache failures do not invalidate an accepted answer.
			// Mandatory artifact and observer failures still stop the reading.
			for _, issue := range outcome.Issues {
				switch issue.Kind {
				case llm.IssueCacheValidate:
				case llm.IssueCacheRead:
					r.opts.State(lines.StageAnswer, "cache read failed", issue.Error())
				case llm.IssueCacheWrite:
					r.opts.State(lines.StageAnswer, "cache write failed", issue.Error())
				case llm.IssueCacheEvict:
					r.opts.State(lines.StageAnswer, "cache eviction failed", issue.Error())
				default:
					return fmt.Errorf("answer: %w", issue)
				}
			}
			if failure != nil {
				if errors.Is(failure, context.Canceled) || errors.Is(failure, context.DeadlineExceeded) {
					return failure
				}
				if !answerResourceFailure(failure) {
					switch failureKinds[outcome.RequestSHA256] {
					case llm.FailureProvider, llm.FailureResponse, llm.FailureValidation:
					default:
						return failure
					}
				}
			}
			use.Windows++
			use.Rows += len(window.parts)
			if outcome.Cached {
				use.Cached++
			} else if outcome.RequestBytes > 0 {
				use.Live++
			}
			superseded := false
			if answerResourceFailure(failure) {
				children, splitErr := r.splitAnswerWindow(def, window.parts)
				if splitErr != nil {
					return splitErr
				}
				if len(children) == 0 {
					if err := r.writeAnswerWindow(def, window, outcome, failure, false); err != nil {
						return err
					}
					return fmt.Errorf("answer: indivisible source/question resource refusal: %w", failure)
				}
				for _, child := range children {
					prepared, err := r.planAnswers(ctx, def, child)
					if err != nil {
						return err
					}
					next = append(next, prepared...)
				}
				superseded = true
			}
			if err := r.writeAnswerWindow(def, window, outcome, failure, superseded); err != nil {
				return err
			}
			if superseded {
				continue
			}
			for j, partInput := range window.parts {
				part := atlas.QuestionAnswerPart{State: "unavailable", Source: atlas.SourceGiven, Steps: []atlas.QuestionStep{}}
				if failure == nil && !r.dry {
					part.Source = atlas.SourceModel
					if outcome.Cached {
						part.Source = atlas.SourceCache
					}
					value := outcome.Value[j]
					part.OriginRequest, part.OriginRow = outcome.RequestSHA256, table.Key(j)
					part.State = value["state"]
					part.Text = answerProse(value["answer"])
					part.Basis = answerProse(value["basis"])
					part.Remaining = answerProse(value["remaining"])
					for _, ref := range strings.Fields(value["sources"]) {
						step, ok := window.bindings[j][ref]
						if !ok {
							return fmt.Errorf("answer: accepted unknown source %s", ref)
						}
						part.Steps = append(part.Steps, step)
					}
				} else {
					use.Given++
				}
				r.questions[partInput.index].Answer.Parts = append(r.questions[partInput.index].Answer.Parts, part)
			}
		}
		planned = next
	}
	for i := range r.questions {
		route := &r.questions[i]
		if len(route.Answer.Parts) > 0 {
			route.Answer.State = answerState(route.Answer.Parts)
		}
		deriveAnswerGuide(route)
	}
	r.reportStage(lines.StageAnswer)
	return nil
}

func answerCall(def table.Definition, window answerWindow) (llm.Call[table.Answers], error) {
	call, err := table.Call(def, window.table)
	if err != nil {
		return call, err
	}
	decode := call.DecodeValidate
	call.DecodeValidate = func(raw []byte) (table.Answers, error) {
		values, err := decode(raw)
		if err != nil {
			return nil, err
		}
		for i, value := range values {
			state, text, refs, gap := value["state"], value["answer"], value["sources"], value["remaining"]
			if state == "unanswered" {
				if text != "none" || refs != "" || gap == "none" || value["basis"] != "none" {
					return nil, fmt.Errorf("unanswered needs no answer, basis or sources and must name the missing evidence")
				}
			} else if text == "none" || refs == "" || value["basis"] == "none" {
				return nil, fmt.Errorf("a substantive answer needs text, its basis and original source refs")
			}
			if (state == "answered" || state == "not_applicable") && gap != "none" {
				return nil, fmt.Errorf("a settled answer cannot have an unresolved part")
			}
			if state == "partial" && gap == "none" {
				return nil, fmt.Errorf("a partial answer must identify the unanswered part")
			}
			if state == "not_applicable" {
				for _, field := range window.table.Rows[i].Fields {
					if (field.Name == "retrieval_complete" || field.Name == "evidence_complete") && field.Value == false {
						return nil, fmt.Errorf("incomplete evidence cannot establish inapplicability")
					}
				}
			}
		}
		return values, nil
	}
	return call, nil
}

func answerProse(value string) string {
	if value == "none" {
		return ""
	}
	return value
}

func deriveAnswerGuide(route *atlas.QuestionRoute) {
	guide := &atlas.QuestionGuide{State: "empty", Candidates: len(uniqueRouteAnchors(route.Stops)), Source: atlas.SourceGiven, Steps: []atlas.QuestionStep{}}
	partial := route.Coverage.UnresolvedChunks > 0
	for _, part := range route.Answer.Parts {
		if part.Source == atlas.SourceGiven {
			partial = true
			continue
		}
		if guide.Source == atlas.SourceGiven || part.Source == atlas.SourceModel {
			guide.Source = part.Source
		}
		if len(part.Steps) > 0 {
			guide.Steps = append(guide.Steps, part.Steps...)
			guide.Parts = append(guide.Parts, atlas.QuestionGuidePart{Source: part.Source, Steps: part.Steps})
		}
	}
	if len(guide.Steps) > 0 {
		guide.State = "ready"
	}
	if len(guide.Parts) > 1 {
		guide.State = "partitioned"
	} else {
		guide.Parts = nil
	}
	if partial {
		guide.State = "partial"
		if len(guide.Steps) == 0 {
			guide.State = "unavailable"
		}
	}
	route.Guide = guide
}

func answerState(parts []atlas.QuestionAnswerPart) string {
	if len(parts) == 1 {
		return parts[0].State
	}
	allUnanswered := len(parts) > 0
	for _, part := range parts {
		if part.Text != "" {
			return "partial"
		}
		allUnanswered = allUnanswered && part.State == "unanswered"
	}
	if allUnanswered {
		return "unanswered"
	}
	return "unavailable"
}
