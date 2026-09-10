package reading

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

// answerQuestion retains the current graph's source bindings locally. A source
// partition is one independent answer part, never a summary fed to another call.
type answerQuestion struct {
	index      int
	candidates []atlas.QuestionStep
	complete   bool
}

type answerWindow struct {
	parts    []answerQuestion
	table    table.Window
	bindings []map[string]atlas.QuestionStep
}

type answerSource struct {
	Ref          string           `json:"ref,omitempty"`
	ResultRows   []string         `json:"result_rows,omitempty"`
	Path         string           `json:"path"`
	Line         int              `json:"line"`
	Column       int              `json:"column"`
	Observations []map[string]any `json:"observations"`
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
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	return result
}

func sourceObservation(route atlas.QuestionRoute, candidate atlas.QuestionStep) (answerSource, error) {
	source := answerSource{Path: candidate.Path, Line: candidate.Line, Column: candidate.Column}
	// Canonicalize complete observations without merging different evidence at
	// the same location. Each question retains its original local stop indexes.
	byJSON := make(map[string]map[string]any)
	for _, index := range candidate.StopIndexes {
		stop := route.Stops[index]
		item := map[string]any{"name": stop.Name, "kind": stop.Kind, "evidence": stop.Evidence}
		raw, err := json.Marshal(item)
		if err != nil {
			return source, err
		}
		byJSON[string(raw)] = item
	}
	keys := make([]string, 0, len(byJSON))
	for key := range byJSON {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		source.Observations = append(source.Observations, byJSON[key])
	}
	return source, nil
}

func makeAnswerWindow(def table.Definition, routes []atlas.QuestionRoute, parts []answerQuestion) (answerWindow, error) {
	result := answerWindow{parts: parts, bindings: make([]map[string]atlas.QuestionStep, len(parts))}
	catalogue := make(map[string]answerSource)
	keys := make([][]string, len(parts))
	for i, part := range parts {
		for _, candidate := range part.candidates {
			source, err := sourceObservation(routes[part.index], candidate)
			if err != nil {
				return result, err
			}
			raw, err := json.Marshal(source)
			if err != nil {
				return result, err
			}
			key := string(raw)
			catalogue[key] = source
			keys[i] = append(keys[i], key)
		}
	}
	ordered := make([]string, 0, len(catalogue))
	for key := range catalogue {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(i, j int) bool {
		a, b := catalogue[ordered[i]], catalogue[ordered[j]]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		return ordered[i] < ordered[j]
	})
	sources := make([]answerSource, len(ordered))
	refs := make(map[string]string)
	for i, key := range ordered {
		source := catalogue[key]
		source.Ref = fmt.Sprintf("c%d", i+1)
		for q, questionKeys := range keys {
			for _, questionKey := range questionKeys {
				if questionKey == key {
					source.ResultRows = append(source.ResultRows, table.Key(q))
					break
				}
			}
		}
		sources[i] = source
		refs[key] = source.Ref
	}
	connections := make(map[string]map[string]any)
	scopes := make(map[string][]string)
	for _, part := range parts {
		scope := routes[part.index].Scope
		raw, err := json.Marshal(scope)
		if err != nil {
			return result, err
		}
		scopes[string(raw)] = scope
	}
	scopeKeys := make([]string, 0, len(scopes))
	for key := range scopes {
		scopeKeys = append(scopeKeys, key)
	}
	sort.Strings(scopeKeys)
	scopeRefs, scopeCatalog := make(map[string]string), make(map[string][]string)
	for i, key := range scopeKeys {
		ref := fmt.Sprintf("s%d", i+1)
		scopeRefs[key], scopeCatalog[ref] = ref, scopes[key]
	}
	for i, part := range parts {
		route := routes[part.index]
		scopeKey, _ := json.Marshal(route.Scope)
		options := make([]string, 0, len(part.candidates))
		hints := make([]map[string]any, 0, len(part.candidates))
		places := make(map[string][]string)
		result.bindings[i] = make(map[string]atlas.QuestionStep)
		for j, candidate := range part.candidates {
			ref := refs[keys[i][j]]
			options = append(options, ref)
			result.bindings[i][ref] = candidate
			var suggestions []map[string]any
			seen := make(map[string]bool)
			for _, index := range candidate.StopIndexes {
				stop := route.Stops[index]
				suggestions = append(suggestions, map[string]any{"relevance": stop.Relevance, "why": stop.Why})
				if !seen[stop.PlaceID] {
					places[stop.PlaceID] = append(places[stop.PlaceID], ref)
					seen[stop.PlaceID] = true
				}
			}
			sort.SliceStable(suggestions, func(i, j int) bool {
				a, _ := json.Marshal(suggestions[i])
				b, _ := json.Marshal(suggestions[j])
				return string(a) < string(b)
			})
			hints = append(hints, map[string]any{"ref": ref, "suggestions": suggestions})
		}
		for _, edge := range route.Connections {
			for _, from := range places[edge.FromID] {
				for _, to := range places[edge.ToID] {
					if from == to {
						continue
					}
					// Preserve the existing compact relationship, not thousands of unrelated
					// callsite witnesses. The original witnesses stay in QuestionRoute.
					item := map[string]any{"from": from, "to": to, "kind": edge.Kind, "count": edge.Count, "level": "files_or_observed_entities", "declaration": edge.Evidence}
					raw, err := json.Marshal(item)
					if err != nil {
						return result, err
					}
					key := string(raw)
					if previous, exists := connections[key]; exists {
						rows := previous["result_rows"].([]string)
						if rows[len(rows)-1] != table.Key(i) {
							previous["result_rows"] = append(rows, table.Key(i))
						}
					} else {
						item["result_rows"] = []string{table.Key(i)}
						connections[key] = item
					}
				}
			}
		}
		result.table.Rows = append(result.table.Rows, table.Row{ID: route.Question, Fields: []table.Field{
			{Name: "question", Value: route.Question}, {Name: "scope_ref", Value: scopeRefs[string(scopeKey)]},
			{Name: "retrieval_complete", Value: route.Coverage.UnresolvedChunks == 0}, {Name: "evidence_complete", Value: part.complete},
			{Name: "candidate_options", Value: options}, {Name: "prior_model_suggestions", Value: hints},
		}})
	}
	connectionKeys := make([]string, 0, len(connections))
	for key := range connections {
		connectionKeys = append(connectionKeys, key)
	}
	sort.Strings(connectionKeys)
	joined := make([]map[string]any, 0, len(connections))
	for _, key := range connectionKeys {
		joined = append(joined, connections[key])
	}
	result.table.Stage = def.Stage
	result.table.Context = []table.Field{{Name: "candidates", Value: sources}, {Name: "connections", Value: joined}, {Name: "scopes", Value: scopeCatalog}}
	raw, err := table.Request(def, result.table)
	if err != nil {
		return result, err
	}
	result.table.Request = raw
	return result, nil
}
