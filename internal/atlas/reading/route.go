package reading

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

type routePool struct {
	row        table.Row
	candidates []atlas.QuestionStep
}

// Every candidate is considered in the first reduction round. Later rounds
// compare selected original evidence, never summaries of previous summaries.
func (r *reader) readRoute(ctx context.Context) error {
	if r.question == nil {
		return fmt.Errorf("route: question candidates are required")
	}
	route := r.question
	candidates := uniqueRouteAnchors(route.Stops)
	guide := &atlas.QuestionGuide{State: "empty", Candidates: len(candidates), Source: atlas.SourceGiven, Rounds: []atlas.QuestionRound{}, Steps: []atlas.QuestionStep{}}
	route.Guide = guide
	partial := route.Coverage.UnresolvedChunks > 0
	if len(candidates) == 0 {
		if partial {
			guide.State = "unavailable"
		}
		return r.persistQuestion()
	}
	r.opts.Stage(lines.StageRoute, route.Question, fmt.Sprintf("ordering %d distinct source locations", len(candidates)))
	def := lines.Route()
	if r.opts.Through == "" || r.opts.Through == lines.StageRoute {
		if r.opts.Prompt != "" {
			def.System = r.opts.Prompt
		}
		if r.opts.InputBytes > 0 {
			def.MaxInputBytes = r.opts.InputBytes
		}
	}
	shared := []table.Field{{Name: "question", Value: route.Question}, {Name: "scope", Value: route.Scope}}
	for round := 0; len(candidates) > 0; round++ {
		pools, err := planRoutePools(def, shared, route, candidates)
		if err != nil {
			return err
		}
		rows := make([]table.Row, len(pools))
		for i := range pools {
			rows[i] = pools[i].row
		}
		answers, err := r.runTableWith(ctx, def, round, shared, rows, nil)
		if err != nil {
			return err
		}
		stats := atlas.QuestionRound{Candidates: len(candidates), Pools: len(pools)}
		var selected []atlas.QuestionStep
		var parts []atlas.QuestionGuidePart
		for i, answer := range answers {
			if answer.source == atlas.SourceGiven {
				stats.UnresolvedPools++
				partial = true
				continue
			}
			if guide.Source == atlas.SourceGiven || answer.source == atlas.SourceModel {
				guide.Source = answer.source
			}
			part := atlas.QuestionGuidePart{OpenQuestion: answer.answer["open_question"], Source: answer.source}
			for _, ref := range strings.Fields(answer.answer["order"]) {
				var index int
				if _, err := fmt.Sscanf(ref, "c%d", &index); err != nil || index < 1 || index > len(pools[i].candidates) {
					return fmt.Errorf("route: accepted unknown candidate %s", ref)
				}
				part.Steps = append(part.Steps, pools[i].candidates[index-1])
			}
			selected = append(selected, part.Steps...)
			if len(part.Steps) > 0 {
				parts = append(parts, part)
			}
			if len(pools) == 1 {
				guide.OpenQuestion, guide.Source = answer.answer["open_question"], answer.source
			}
		}
		stats.Selected = len(selected)
		guide.Rounds = append(guide.Rounds, stats)
		if len(pools) == 1 {
			guide.Steps = append(guide.Steps, selected...)
			guide.State = "ready"
			if len(selected) == 0 {
				guide.State = "empty"
			}
			if partial {
				guide.State = "partial"
				if len(selected) == 0 {
					guide.State = "unavailable"
				}
			}
			break
		}
		if len(selected) >= len(candidates) {
			// Every pool returned a valid selection, but no anchor was removed.
			// Stop at this fixed point: retrying the same evidence cannot ensure
			// progress. Keep each accepted local order without inventing a global
			// ranking or throwing away sources to force another reduction round.
			guide.Steps, guide.Parts = selected, parts
			guide.State = "partitioned"
			if partial {
				guide.State = "partial"
			}
			break
		}
		if len(selected) == 0 {
			guide.State = "empty"
			if partial {
				guide.State = "unavailable"
			}
			break
		}
		candidates = selected
	}
	if err := r.persistQuestion(); err != nil {
		return err
	}
	r.reportStage(lines.StageRoute)
	var details []string
	for _, step := range guide.Steps {
		details = append(details, fmt.Sprintf("%s:%d — %s", step.Path, step.Line, route.Stops[step.StopIndexes[0]].Why))
	}
	if guide.OpenQuestion != "" && guide.OpenQuestion != "none" {
		details = append(details, "Still to investigate: "+guide.OpenQuestion)
	}
	r.opts.State("Reading route", guide.State, details...)
	return nil
}

func uniqueRouteAnchors(stops []atlas.QuestionStop) []atlas.QuestionStep {
	var result []atlas.QuestionStep
	seen := make(map[string]int)
	for i, stop := range stops {
		key := fmt.Sprintf("%s:%d:%d", stop.Path, stop.Line, stop.Column)
		if j, ok := seen[key]; ok {
			result[j].StopIndexes = append(result[j].StopIndexes, i)
			continue
		}
		seen[key] = len(result)
		result = append(result, atlas.QuestionStep{Path: stop.Path, Line: stop.Line, Column: stop.Column, StopIndexes: []int{i}})
	}
	return result
}

func planRoutePools(def table.Definition, shared []table.Field, route *atlas.QuestionRoute, candidates []atlas.QuestionStep) ([]routePool, error) {
	limit := def.MaxInputBytes
	if limit == 0 {
		limit = table.DefaultInputBytes
	}
	var pools []routePool
	var split func([]atlas.QuestionStep) error
	split = func(part []atlas.QuestionStep) error {
		row := routePoolRow(route, part, false)
		request, err := table.Request(def, table.Window{Stage: def.Stage, Context: shared, Rows: []table.Row{row}})
		if err != nil {
			return err
		}
		if len(def.System)+len(request) > limit {
			if len(part) == 1 {
				return fmt.Errorf("route: source location %s:%d exceeds %d input bytes", part[0].Path, part[0].Line, limit)
			}
			middle := len(part) / 2
			if err := split(part[:middle]); err != nil {
				return err
			}
			return split(part[middle:])
		}
		row.ID = fmt.Sprintf("pool:%d", len(pools)+1)
		pools = append(pools, routePool{row, part})
		return nil
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	if err := split(candidates); err != nil {
		return nil, err
	}
	if len(pools) == 1 {
		pools[0].row = routePoolRow(route, pools[0].candidates, true)
		pools[0].row.ID = "pool:1"
	}
	return pools, nil
}

func routePoolRow(route *atlas.QuestionRoute, candidates []atlas.QuestionStep, final bool) table.Row {
	options := make([]string, len(candidates))
	data := make([]map[string]any, len(candidates))
	places := make(map[string][]string)
	for i, candidate := range candidates {
		ref := fmt.Sprintf("c%d", i+1)
		options[i] = ref
		var observations, suggestions []map[string]any
		seen := make(map[string]bool)
		for _, index := range candidate.StopIndexes {
			stop := route.Stops[index]
			if !seen[stop.PlaceID] {
				places[stop.PlaceID] = append(places[stop.PlaceID], ref)
				seen[stop.PlaceID] = true
			}
			observations = append(observations, map[string]any{"name": stop.Name, "kind": stop.Kind, "evidence": stop.Evidence})
			suggestions = append(suggestions, map[string]any{"relevance": stop.Relevance, "why": stop.Why})
		}
		data[i] = map[string]any{"ref": ref, "path": candidate.Path, "line": candidate.Line, "column": candidate.Column, "observations": observations, "prior_model_suggestions": suggestions}
	}
	connections := []map[string]any{}
	seen := make(map[string]bool)
	for _, edge := range route.Connections {
		for _, from := range places[edge.FromID] {
			for _, to := range places[edge.ToID] {
				if from == to {
					continue
				}
				// Reading order needs the file/entity relationship, not every
				// call inside those files. Repeating all callsite witnesses for
				// each pair of candidate anchors made a small pool enormous and
				// implied more precision than a file-level connection provides.
				// The full original witnesses remain in QuestionRoute.Connections.
				item := map[string]any{"from": from, "to": to, "kind": edge.Kind, "count": edge.Count, "level": "files_or_observed_entities", "declaration": edge.Evidence}
				raw, _ := json.Marshal(item)
				if !seen[string(raw)] {
					connections = append(connections, item)
					seen[string(raw)] = true
				}
			}
		}
	}
	preferredSteps := min(lines.RouteSteps, max(1, (len(candidates)+1)/2))
	mode := "partial_pool"
	if final {
		mode = "final_pool"
		preferredSteps = min(lines.RouteSteps, len(candidates))
	}
	return table.Row{Fields: []table.Field{{Name: "mode", Value: mode}, {Name: "candidate_options", Value: options}, {Name: "preferred_steps", Value: preferredSteps}, {Name: "candidates", Value: data}, {Name: "connections", Value: connections}}}
}
