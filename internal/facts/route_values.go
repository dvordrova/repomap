package facts

import (
	"encoding/json"
	"sort"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

type routeSourceCall struct {
	relation programindex.Relation
	pattern  programindex.RelationPattern
}

// This reader follows only the index's original value expressions and native
// calls. A branch binds its constructor/wrapper invocation before reading a
// formal parameter, so different instances cannot exchange field values.
type routeValueReader struct {
	target  *targetContext
	sites   map[sourcevalue.Anchor][]routeSourceCall
	callers map[string][]routeSourceCall
	owners  map[sourcevalue.Anchor][]string
}

type routeLiteral struct {
	text     string
	evidence []Anchor
	possible bool
	bindings map[sourcevalue.Anchor]routeSourceCall
}

func newRouteValueReader(target *targetContext) *routeValueReader {
	r := &routeValueReader{target: target, sites: make(map[sourcevalue.Anchor][]routeSourceCall), callers: make(map[string][]routeSourceCall), owners: make(map[sourcevalue.Anchor][]string)}
	for _, object := range target.input.Index.Objects {
		if object.Location == nil || (object.Kind != programindex.ObjectFunction && object.Kind != programindex.ObjectMethod && object.Kind != programindex.ObjectLambda) {
			continue
		}
		at := sourcevalue.Anchor{Path: object.Location.Path, Line: object.Location.Line, Column: object.Location.Column}
		r.owners[at] = append(r.owners[at], object.ID)
		if at.Column != 0 {
			at.Column = 0
			r.owners[at] = append(r.owners[at], object.ID)
		}
	}
	for _, relation := range target.input.Index.Relations {
		if relation.Kind != programindex.RelationCalls && relation.Kind != programindex.RelationInvokesExternal {
			continue
		}
		for _, pattern := range relation.Patterns {
			call := routeSourceCall{relation, pattern}
			if at := target.patternAnchor(relation, pattern); at != nil {
				key := sourcevalue.Anchor{Path: at.Path, Line: at.Line, Column: at.Column}
				r.sites[key] = append(r.sites[key], call)
			}
			for _, id := range relation.ToIDs {
				r.callers[id] = append(r.callers[id], call)
			}
		}
	}
	return r
}

func (r *routeValueReader) argument(argument programindex.PatternArgument) []routeLiteral {
	if text, templated, ok := literalValue(argument); ok {
		return []routeLiteral{{text: text, possible: templated}}
	}
	return r.value(argument.Origin, nil, routeLiteral{}, make(map[string]bool))
}

func (r *routeValueReader) value(value *sourcevalue.Value, fields []string, branch routeLiteral, active map[string]bool) []routeLiteral {
	if value == nil {
		return nil
	}
	encoded, _ := json.Marshal(struct {
		Value  *sourcevalue.Value
		Fields []string
	}{value, fields})
	key := string(encoded)
	if active[key] {
		return nil
	}
	active[key] = true
	defer delete(active, key)
	if value.Anchor != nil {
		branch.evidence = appendRouteValueAnchor(branch.evidence, *value.Anchor)
	}
	switch value.Kind {
	case "literal":
		if len(fields) == 0 {
			branch.text = value.Text
			return []routeLiteral{branch}
		}
	case "field":
		if len(value.Parts) == 1 {
			return r.value(&value.Parts[0], append([]string{value.Text}, fields...), branch, active)
		}
	case "record":
		if len(fields) > 0 {
			for _, part := range value.Parts {
				if part.Kind == "field_value" && part.Text == fields[0] && len(part.Parts) == 1 {
					return r.value(&part.Parts[0], fields[1:], branch, active)
				}
			}
		}
	case "alternatives":
		var result []routeLiteral
		for i := range value.Parts {
			next := cloneRouteLiteral(branch)
			next.possible = true
			result = append(result, r.value(&value.Parts[i], fields, next, active)...)
		}
		return result
	case "concat":
		if len(fields) == 0 {
			branches := []routeLiteral{branch}
			for i := range value.Parts {
				var next []routeLiteral
				for _, previous := range branches {
					for _, part := range r.value(&value.Parts[i], nil, cloneRouteLiteral(previous), active) {
						part.text = previous.text + part.text
						next = append(next, part)
					}
				}
				branches = next
			}
			return branches
		}
	case "call_result":
		if value.Anchor != nil {
			var result []routeLiteral
			for _, call := range r.sites[*value.Anchor] {
				returned := call.pattern.ResultValue
				if returned == nil {
					continue
				}
				next := r.bind(branch, call, returned.Owner)
				result = append(result, r.value(returned, fields, next, active)...)
			}
			return result
		}
	case "parameter", "receiver":
		if value.Owner == nil {
			return nil
		}
		var calls []routeSourceCall
		if call, bound := branch.bindings[*value.Owner]; bound {
			calls = []routeSourceCall{call}
		} else if owner := r.ownerID(*value.Owner); owner != "" {
			calls = r.callers[owner]
		}
		var result []routeLiteral
		for _, call := range calls {
			next := r.bind(branch, call, value.Owner)
			argument := call.pattern.ReceiverValue
			if value.Kind == "parameter" {
				argument = nil
				for _, supplied := range call.pattern.Arguments {
					if supplied.Position == value.Position && supplied.Keyword == "" || supplied.Keyword != "" && supplied.Keyword == value.Text {
						argument = supplied.Origin
						if argument == nil {
							if text, _, ok := literalValue(supplied); ok {
								argument = &sourcevalue.Value{Kind: "literal", Text: text}
							}
						}
						break
					}
				}
			}
			result = append(result, r.value(argument, fields, next, active)...)
		}
		return result
	}
	return nil
}

func (r *routeValueReader) bind(branch routeLiteral, call routeSourceCall, owner *sourcevalue.Anchor) routeLiteral {
	next := cloneRouteLiteral(branch)
	if owner != nil {
		next.bindings[*owner] = call
	}
	if at := r.target.patternAnchor(call.relation, call.pattern); at != nil {
		next.evidence = appendRouteValueAnchor(next.evidence, sourcevalue.Anchor{Path: at.Path, Line: at.Line, Column: at.Column})
	}
	if call.relation.Resolution != programindex.ResolutionExact || call.relation.TargetsOmitted != 0 {
		next.possible = true
	}
	return next
}

// SSA writes the enclosing FuncDecl start while objects use the declared
// identifier. Both are native anchors: a unique declaration at that source
// line is required when their columns differ. Ambiguous same-line owners stop.
func (r *routeValueReader) ownerID(owner sourcevalue.Anchor) string {
	if exact := r.owners[owner]; len(exact) == 1 {
		return exact[0]
	}
	owner.Column = 0
	candidates := r.owners[owner]
	if len(candidates) == 1 {
		return candidates[0]
	}
	return ""
}

func cloneRouteLiteral(value routeLiteral) routeLiteral {
	result := value
	result.evidence = append([]Anchor(nil), value.evidence...)
	result.bindings = make(map[sourcevalue.Anchor]routeSourceCall, len(value.bindings)+1)
	for key, call := range value.bindings {
		result.bindings[key] = call
	}
	return result
}

func appendRouteValueAnchor(values []Anchor, at sourcevalue.Anchor) []Anchor {
	anchor := Anchor{Path: at.Path, Line: at.Line, Column: at.Column}
	for _, value := range values {
		if value == anchor {
			return values
		}
	}
	return append(values, anchor)
}

func mergeRouteEvidence(a, b []Anchor) []Anchor {
	result := append([]Anchor(nil), a...)
	for _, at := range b {
		result = appendRouteValueAnchor(result, sourcevalue.Anchor{Path: at.Path, Line: at.Line, Column: at.Column})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		if result[i].Line != result[j].Line {
			return result[i].Line < result[j].Line
		}
		return result[i].Column < result[j].Column
	})
	return result
}
