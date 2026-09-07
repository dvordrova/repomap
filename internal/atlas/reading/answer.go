package reading

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

var answerCandidateRef = regexp.MustCompile(`\bc[0-9]+\b`)

func (r *reader) readAnswer(ctx context.Context) error {
	route := r.question
	if route == nil || route.Guide == nil {
		return fmt.Errorf("answer: a reading route is required")
	}
	answer := &atlas.QuestionAnswer{State: "unavailable", Parts: []atlas.QuestionAnswerPart{}}
	route.Answer = answer
	if len(route.Guide.Steps) == 0 {
		if route.Coverage.UnresolvedChunks == 0 && route.Guide.State == "empty" {
			answer.State = "unanswered"
		}
		return r.persistQuestion()
	}
	def := lines.Answer()
	if r.opts.Through == lines.StageAnswer && r.opts.Prompt != "" {
		def.System = r.opts.Prompt
	}
	if (r.opts.Through == "" || r.opts.Through == lines.StageAnswer) && r.opts.InputBytes > 0 {
		def.MaxInputBytes = r.opts.InputBytes
	}
	shared := []table.Field{{Name: "question", Value: route.Question}, {Name: "scope", Value: route.Scope},
		{Name: "retrieval_complete", Value: route.Coverage.UnresolvedChunks == 0}, {Name: "route_state", Value: route.Guide.State}}
	// Preserve every selected original anchor. More than one pool produces
	// separately anchored partial answers rather than summaries of summaries.
	pools, err := planRoutePools(def, shared, route, route.Guide.Steps)
	if err != nil {
		return err
	}
	rows := make([]table.Row, len(pools))
	for i := range pools {
		rows[i] = pools[i].row
	}
	r.opts.Stage(lines.StageAnswer, route.Question, fmt.Sprintf("answering from %d selected source locations", len(route.Guide.Steps)))
	values, err := r.runTableWith(ctx, def, 0, shared, rows, func(values table.Answers) error {
		for _, value := range values {
			for _, field := range []string{"answer", "basis", "remaining"} {
				if answerCandidateRef.MatchString(value[field]) {
					return fmt.Errorf("answer prose contains an internal source ref in %s", field)
				}
			}
			state, text, refs, gap := value["state"], value["answer"], value["sources"], value["remaining"]
			if state == "unanswered" {
				if text != "none" || refs != "" || gap == "none" {
					return fmt.Errorf("unanswered needs no answer or sources and must name the missing evidence")
				}
			} else if text == "none" || refs == "" || value["basis"] == "none" {
				return fmt.Errorf("a substantive answer needs text, its basis and original source refs")
			}
			if (state == "answered" || state == "not_applicable") && gap != "none" {
				return fmt.Errorf("a settled answer cannot have an unresolved part")
			}
			if state == "partial" && gap == "none" {
				return fmt.Errorf("a partial answer must identify the unanswered part")
			}
			if state == "not_applicable" && (len(pools) != 1 || route.Coverage.UnresolvedChunks != 0 || route.Guide.State != "ready") {
				return fmt.Errorf("incomplete evidence cannot establish inapplicability")
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for i, value := range values {
		part := atlas.QuestionAnswerPart{State: "unavailable", Source: value.source, Steps: []atlas.QuestionStep{}}
		if value.source != atlas.SourceGiven {
			part.State, part.Text, part.Remaining = value.answer["state"], value.answer["answer"], value.answer["remaining"]
			part.Basis = value.answer["basis"]
			if part.Text == "none" {
				part.Text = ""
			}
			if part.Remaining == "none" {
				part.Remaining = ""
			}
			if part.Basis == "none" {
				part.Basis = ""
			}
			for _, ref := range strings.Fields(value.answer["sources"]) {
				var index int
				if _, err := fmt.Sscanf(ref, "c%d", &index); err != nil || index < 1 || index > len(pools[i].candidates) {
					return fmt.Errorf("answer: accepted unknown candidate %s", ref)
				}
				part.Steps = append(part.Steps, pools[i].candidates[index-1])
			}
		}
		answer.Parts = append(answer.Parts, part)
	}
	answer.State = answerState(answer.Parts)
	r.reportStage(lines.StageAnswer)
	return r.persistQuestion()
}

func answerState(parts []atlas.QuestionAnswerPart) string {
	if len(parts) == 1 {
		return parts[0].State
	}
	state := "unavailable"
	for _, part := range parts {
		if part.Text != "" {
			return "partial"
		}
		if part.State == "unanswered" {
			state = "unanswered"
		}
	}
	return state
}
