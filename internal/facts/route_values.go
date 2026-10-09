package facts

import (
	"encoding/json"
	"sort"
	"strings"

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
	// returns holds, per owner, the calls whose result the walk reads from
	// that owner's return value: reading a call's result binds the call to
	// the returning owner, so the owner's parameters may read that call.
	returns map[sourcevalue.Anchor][]routeSourceCall
	// keys is each expression's encoding, made once: many registrations
	// reach the same expressions, and a key keeps its expression alive.
	keys map[*sourcevalue.Value]string
	// reach is what an expression can lead to under any branch, for each
	// expression whose strongly connected component is complete.
	reach map[string]routeReach
}

// routeReach says whether an expression leads to a literal, and to a text
// that may be an address, through any of the steps the walk can take from it
// under any fields, bindings, evidence or expressions already being read.
type routeReach struct {
	literal bool
	address bool
}

type routeLiteral struct {
	text     string
	evidence []Anchor
	possible bool
	bindings map[sourcevalue.Anchor]routeSourceCall
	// owner is the function whose returned value the walk is reading, by
	// its anchor, so a join inside it can be read with its comparisons.
	owner *sourcevalue.Anchor
}

func newRouteValueReader(target *targetContext) *routeValueReader {
	r := &routeValueReader{target: target, sites: make(map[sourcevalue.Anchor][]routeSourceCall), callers: make(map[string][]routeSourceCall), owners: make(map[sourcevalue.Anchor][]string), returns: make(map[sourcevalue.Anchor][]routeSourceCall), keys: make(map[*sourcevalue.Value]string), reach: make(map[string]routeReach)}
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
				if returned := pattern.ResultValue; returned != nil && returned.Owner != nil {
					r.returns[*returned.Owner] = append(r.returns[*returned.Owner], call)
				}
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
	return r.value(argument.Origin, nil, routeLiteral{}, make(map[string]bool), false)
}

// addresses reads an argument as argument does, leaving out branches that
// lead to no address: every literal whose text may be an address remains,
// and some others may.
func (r *routeValueReader) addresses(argument programindex.PatternArgument) []routeLiteral {
	if text, templated, ok := literalValue(argument); ok {
		return []routeLiteral{{text: text, possible: templated}}
	}
	return r.value(argument.Origin, nil, routeLiteral{}, make(map[string]bool), true)
}

func (r *routeValueReader) value(value *sourcevalue.Value, fields []string, branch routeLiteral, active map[string]bool, addresses bool) []routeLiteral {
	if value == nil {
		return nil
	}
	// The key is the expression alone: re-entering an expression that is
	// still being read is recursion, and a recursive field access (a node
	// reading its own `next`) would otherwise grow `fields` without end.
	key := r.key(value)
	if active[key] {
		return nil
	}
	// An expression that leads to no literal under any branch reads as
	// nothing, as walking every path through its callers would find; with
	// addresses, one that leads to no address reads as no address.
	if reach := r.reachOf(value, key); !reach.literal || addresses && !reach.address {
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
			return r.value(&value.Parts[0], append([]string{value.Text}, fields...), branch, active, addresses)
		}
	case "record":
		if len(fields) > 0 {
			for _, part := range value.Parts {
				if part.Kind == "field_value" && part.Text == fields[0] && len(part.Parts) == 1 {
					return r.value(&part.Parts[0], fields[1:], branch, active, addresses)
				}
			}
		}
	case "alternatives":
		var result []routeLiteral
		for i := range value.Parts {
			if r.outsideItsBranch(&value.Parts[i], branch) {
				continue
			}
			next := cloneRouteLiteral(branch)
			next.possible = true
			result = append(result, r.value(&value.Parts[i], fields, next, active, addresses)...)
		}
		return result
	case "concat":
		if len(fields) == 0 {
			// A part is a piece of the text: its literals all count,
			// addresses or not.
			branches := []routeLiteral{branch}
			for i := range value.Parts {
				var next []routeLiteral
				for _, previous := range branches {
					for _, part := range r.value(&value.Parts[i], nil, cloneRouteLiteral(previous), active, false) {
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
				owner := returned.Owner
				if owner == nil {
					// Python writes no owner on what a function returns: the
					// call's one repository callee is the function.
					owner = r.calleeAnchor(call)
				}
				next := r.bind(branch, call, owner)
				next.owner = owner
				result = append(result, r.value(returned, fields, next, active, addresses)...)
			}
			return result
		}
	case "parameter", "receiver":
		if value.Owner == nil {
			return nil
		}
		var calls []routeSourceCall
		if call, bound := r.boundCall(branch, *value.Owner); bound {
			calls = []routeSourceCall{call}
		} else if owner := r.ownerID(*value.Owner); owner != "" {
			// An unbound parameter reads what every caller supplies,
			// whatever path reached it: one read of one argument walks its
			// callers once. Walking them again from each path through the
			// callers' own callers never ended on nats-server's server
			// package; a second path adds no text, only its own evidence.
			seen := "callers\x00" + key + "\x00" + strings.Join(fields, "\x01")
			if active[seen] {
				return nil
			}
			active[seen] = true
			calls = r.callers[owner]
		}
		var result []routeLiteral
		for _, call := range calls {
			next := r.bind(branch, call, value.Owner)
			// The supplied value is read in the caller's body.
			next.owner = nil
			result = append(result, r.value(suppliedValue(value, call), fields, next, active, addresses)...)
		}
		return result
	}
	return nil
}

// suppliedValue is what a call hands to a formal parameter or receiver: the
// argument at the parameter's position or keyword, or the literal written
// there.
func suppliedValue(value *sourcevalue.Value, call routeSourceCall) *sourcevalue.Value {
	if value.Kind != "parameter" {
		return call.pattern.ReceiverValue
	}
	for _, supplied := range call.pattern.Arguments {
		if supplied.Position == value.Position && supplied.Keyword == "" || supplied.Keyword != "" && supplied.Keyword == value.Text {
			if supplied.Origin != nil {
				return supplied.Origin
			}
			if text, _, ok := literalValue(supplied); ok {
				return &sourcevalue.Value{Kind: "literal", Text: text}
			}
			return nil
		}
	}
	return nil
}

func (r *routeValueReader) key(value *sourcevalue.Value) string {
	key, known := r.keys[value]
	if !known {
		encoded, _ := json.Marshal(value)
		key = string(encoded)
		r.keys[value] = key
	}
	return key
}

// reachOf says what an expression can lead to. The walk's results depend on
// its fields, bindings, evidence and the expressions already being read; the
// reach depends on none of them. It follows every step the walk can take
// under any branch: each field of a record, every caller of a parameter's
// owner and every call that reading a result binds to that owner. So an
// expression without a literal in reach reads as nothing in every branch, and
// one without an address in reach reads no address. The walk is unchanged:
// reach only lets it skip what it would have found empty.
//
// The steps form a graph with cycles (recursion, mutual calls). Reach is
// recorded for a whole strongly connected component once it is complete,
// never for an expression whose component is still open, so a cycle cut
// short is never remembered as leading nowhere.
func (r *routeValueReader) reachOf(value *sourcevalue.Value, key string) routeReach {
	if reach, done := r.reach[key]; done {
		return reach
	}
	r.connect(value, key, &routeReachWalk{order: make(map[string]int), low: make(map[string]int), own: make(map[string]routeReach)})
	return r.reach[key]
}

type routeReachWalk struct {
	order map[string]int
	low   map[string]int
	own   map[string]routeReach
	stack []string
}

// connect is Tarjan's strongly connected components over the walk's steps.
func (r *routeValueReader) connect(value *sourcevalue.Value, key string, walk *routeReachWalk) {
	walk.order[key] = len(walk.order)
	walk.low[key] = walk.order[key]
	walk.stack = append(walk.stack, key)
	var own routeReach
	r.steps(value, func(next *sourcevalue.Value) {
		if next.Kind == "literal" {
			own = own.with(routeReach{literal: true, address: isAddressLiteral(next.Text)})
			return
		}
		nextKey := r.key(next)
		if reach, done := r.reach[nextKey]; done {
			own = own.with(reach)
			return
		}
		if _, seen := walk.order[nextKey]; seen {
			// Seen and not done: open on the stack, in this component.
			walk.low[key] = min(walk.low[key], walk.order[nextKey])
			return
		}
		r.connect(next, nextKey, walk)
		if reach, done := r.reach[nextKey]; done {
			own = own.with(reach)
			return
		}
		walk.low[key] = min(walk.low[key], walk.low[nextKey])
	})
	walk.own[key] = own
	if walk.low[key] != walk.order[key] {
		return
	}
	at := len(walk.stack) - 1
	for walk.stack[at] != key {
		at--
	}
	component := walk.stack[at:]
	walk.stack = walk.stack[:at]
	var reach routeReach
	for _, member := range component {
		reach = reach.with(walk.own[member])
	}
	for _, member := range component {
		r.reach[member] = reach
	}
}

// steps hands over every expression the walk may read next from value under
// any branch. A literal hands over itself; a concatenation hands over a
// stand-in literal that may be an address, since its text is made of its
// parts and the walk reads every part.
func (r *routeValueReader) steps(value *sourcevalue.Value, next func(*sourcevalue.Value)) {
	switch value.Kind {
	case "literal":
		next(value)
	case "concat":
		next(&routeConcatReach)
	case "field":
		if len(value.Parts) == 1 {
			next(&value.Parts[0])
		}
	case "record":
		for i := range value.Parts {
			if part := &value.Parts[i]; part.Kind == "field_value" && len(part.Parts) == 1 {
				next(&part.Parts[0])
			}
		}
	case "alternatives":
		for i := range value.Parts {
			next(&value.Parts[i])
		}
	case "call_result":
		if value.Anchor != nil {
			for _, call := range r.sites[*value.Anchor] {
				if call.pattern.ResultValue != nil {
					next(call.pattern.ResultValue)
				}
			}
		}
	case "parameter", "receiver":
		if value.Owner == nil {
			return
		}
		// Unbound, the walk reads the owner's callers; bound, the call the
		// branch bound, which is one of them or a call whose result it read.
		var calls []routeSourceCall
		if owner := r.ownerID(*value.Owner); owner != "" {
			calls = r.callers[owner]
		}
		for _, group := range [][]routeSourceCall{calls, r.returns[*value.Owner]} {
			for _, call := range group {
				if supplied := suppliedValue(value, call); supplied != nil {
					next(supplied)
				}
			}
		}
	}
}

// routeConcatReach stands for a concatenation's text in reach: a literal and
// possibly an address.
var routeConcatReach = sourcevalue.Value{Kind: "literal", Text: "/"}

func (reach routeReach) with(other routeReach) routeReach {
	return routeReach{literal: reach.literal || other.literal, address: reach.address || other.address}
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

// outsideItsBranch says a part of a join inside the function the walk is
// reading is no value of the call the walk is bound to: it lies in the
// branch of a case comparing a parameter with words, and the call supplies
// that parameter another word (sourcevalue.OutsideItsBranch).
func (r *routeValueReader) outsideItsBranch(part *sourcevalue.Value, branch routeLiteral) bool {
	if branch.owner == nil || part.Anchor == nil {
		return false
	}
	call, bound := r.boundCall(branch, *branch.owner)
	if !bound {
		return false
	}
	id := r.ownerID(*branch.owner)
	object, ok := r.target.object(id)
	if !ok || object.Location == nil || len(object.Comparisons) == 0 {
		return false
	}
	return sourcevalue.OutsideItsBranch(part.Anchor, object.Location.Path, comparedOf(object.Comparisons), func(position int, name string) (string, bool) {
		supplied := suppliedValue(&sourcevalue.Value{Kind: "parameter", Position: position, Text: name}, call)
		if supplied == nil || supplied.Kind != "literal" {
			return "", false
		}
		return supplied.Text, true
	})
}

// comparedOf are a function's comparisons as the walks read them.
func comparedOf(comparisons []programindex.Comparison) []sourcevalue.Compared {
	var result []sourcevalue.Compared
	for _, comparison := range comparisons {
		compared := sourcevalue.Compared{}
		if origin := comparison.Origin; origin != nil && origin.Kind == "parameter" {
			compared.Position, compared.Name = origin.Position, origin.Text
		}
		for _, item := range comparison.Cases {
			written := sourcevalue.ComparedCase{Words: item.Words, Exclusive: item.Exclusive}
			if item.Branch != nil {
				written.Line, written.EndLine = item.Branch.Line, item.Branch.EndLine
				written.Column, written.EndColumn = item.Branch.Column, item.Branch.EndColumn
			}
			compared.Cases = append(compared.Cases, written)
		}
		result = append(result, compared)
	}
	return result
}

// calleeAnchor is where the one repository function a call reaches is
// declared, nil for any other call.
func (r *routeValueReader) calleeAnchor(call routeSourceCall) *sourcevalue.Anchor {
	if len(call.relation.ToIDs) != 1 || !r.target.ownsObject(call.relation.ToIDs[0]) {
		return nil
	}
	object, ok := r.target.object(call.relation.ToIDs[0])
	if !ok || object.Location == nil || !isCallable(object) {
		return nil
	}
	return &sourcevalue.Anchor{Path: object.Location.Path, Line: object.Location.Line, Column: object.Location.Column}
}

// boundCall is the call the branch bound to the function declared at owner:
// at its anchor, or on its line when one side writes no column the other
// does (an owner's declaration and its parameters' owner may differ in
// column, as ownerID allows).
func (r *routeValueReader) boundCall(branch routeLiteral, owner sourcevalue.Anchor) (routeSourceCall, bool) {
	if call, bound := branch.bindings[owner]; bound {
		return call, true
	}
	if r.ownerID(owner) == "" {
		return routeSourceCall{}, false
	}
	var found []routeSourceCall
	for anchor, call := range branch.bindings {
		if anchor.Path == owner.Path && anchor.Line == owner.Line && r.ownerID(anchor) == r.ownerID(owner) {
			found = append(found, call)
		}
	}
	// Two bindings of one function that disagree bind none.
	if len(found) == 0 {
		return routeSourceCall{}, false
	}
	for _, call := range found[1:] {
		if call.pattern.ID != found[0].pattern.ID {
			return routeSourceCall{}, false
		}
	}
	return found[0], true
}
