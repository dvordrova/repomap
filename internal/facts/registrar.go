package facts

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// Registrar is the repository function a registration hands its callable
// to: its declaration, as a method with its type (`eventLoop.register`), its
// file, its declared signature, and the fields or module-level variables it
// stores the parameter in, as the code names them. That it stores the
// callable is structure; what for, the reading asks. During says what runs
// when the registering call is made (registeredDuring).
type Registrar struct {
	ObjectID  string             `json:"object_id"`
	Name      string             `json:"name"`
	Path      string             `json:"path"`
	Signature string             `json:"signature,omitempty"`
	Slots     []string           `json:"slots"`
	During    []RegisteredDuring `json:"during,omitempty"`
}

// RegisteredDuring is one way the registering call is reached, walking back
// over its callers: from the program's start (Seed, a seed declaration),
// or while callables another registration hands over run (Handlers, handed
// to HandedTo). Through are the declarations between that start and the
// registering call, the registering function last, in walk order. Every
// one is listed: no count stands in for names.
type RegisteredDuring struct {
	Seed     string   `json:"seed,omitempty"`
	HandedTo string   `json:"handed_to,omitempty"`
	Handlers []string `json:"handlers,omitempty"`
	Through  []string `json:"through,omitempty"`
}

// addRepositoryRegistrations emits the registrations the repository owns
// both sides of (owner decision 2026-09-27, "a"): an exact call hands a
// repository callable to a repository function, and that function stores the
// very parameter it received there in a field or a module-level variable (a
// parameter store of the program index). The function is the call's
// registrar. That the callable is kept is structure; whether it is an entry,
// and which, the reading asks once per registrar and callable. Each call is
// its own registration, at its own source position. A callee that stores
// some other value, or runs the callable and keeps nothing, registers
// nothing. Enabled for the C adapter, the one that records parameter stores.
func (b *builder) addRepositoryRegistrations(target *targetContext) {
	type argumentAt struct {
		relation programindex.Relation
		pattern  programindex.RelationPattern
		argument programindex.PatternArgument
	}
	arguments := make(map[string]argumentAt)
	for _, relation := range target.input.Index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 {
			continue
		}
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				arguments[argument.ID] = argumentAt{relation: relation, pattern: pattern, argument: argument}
			}
		}
	}
	type pending struct {
		fact Fact
		from string
		name []string
	}
	var found []pending
	for _, callback := range target.input.Index.Relations {
		if callback.Kind != programindex.RelationPassesCallback || callback.Resolution != programindex.ResolutionExact ||
			len(callback.ToIDs) != 1 || callback.SourceArgumentID == "" {
			continue
		}
		at, ok := arguments[callback.SourceArgumentID]
		if !ok || target.unreachable(at.relation) || !target.ownsObject(at.relation.ToIDs[0]) {
			continue
		}
		handed, ok := target.object(callback.ToIDs[0])
		if !ok || !isCallable(handed) {
			continue
		}
		registrar, ok := target.object(at.relation.ToIDs[0])
		if !ok || registrar.Location == nil {
			continue
		}
		slots := storedSlots(registrar, at.argument)
		if len(slots) == 0 {
			continue
		}
		anchor := target.patternAnchor(at.relation, at.pattern)
		if anchor == nil || !b.once(strings.Join([]string{string(KindRegistration), "registrar", target.target.ID, anchor.String(), strconv.Itoa(anchor.Column), handed.ID}, "\x00")) {
			continue
		}
		var literals []string
		for _, argument := range at.pattern.Arguments {
			if value, _, literal := literalValue(argument); literal && value != "" {
				literals = append(literals, value)
			}
		}
		_, ownerID := target.enclosingSymbol(at.relation.FromID)
		found = append(found, pending{from: ownerID, name: []string{at.pattern.Selector, handed.Name, registrar.Name}, fact: Fact{
			Kind:     KindRegistration,
			TargetID: target.target.ID,
			Anchor:   anchor,
			Key:      at.pattern.Selector,
			Values:   literals,
			Symbol:   handed.Name,
			ObjectID: handed.ID,
			OwnerID:  ownerID,
			Registrar: &Registrar{
				ObjectID: registrar.ID, Name: target.declarationName(registrar), Path: registrar.Location.Path,
				Signature: registrar.Signature, Slots: slots,
			},
			Resolution: ResolutionExact,
		}})
	}
	if len(found) == 0 {
		return
	}
	// What every registration of the target hands over: the walk back from
	// a registering call stops at each of them.
	handedTo := map[string][]string{}
	note := func(handler, to string) {
		if handler != "" && to != "" && !slices.Contains(handedTo[handler], to) {
			handedTo[handler] = append(handedTo[handler], to)
		}
	}
	for _, fact := range b.facts {
		if fact.Kind == KindRegistration && fact.TargetID == target.target.ID && fact.ObjectID != "" {
			to := fact.Text
			if to == "" {
				to = fact.Key
			}
			note(fact.ObjectID, to)
		}
	}
	for _, item := range found {
		note(item.fact.ObjectID, item.fact.Registrar.Name)
	}
	for _, item := range found {
		item.fact.Registrar.During = target.registeredDuring(item.from, handedTo)
		b.add(target.root, item.fact, item.name...)
	}
}

// registeredDuring walks back from the function making a registering call
// over its exact and alternative callers, and stops at the program's seeds
// and at every callable a registration hands over: what runs when the call
// is made. Each start is listed with the declarations between it and the
// call; a seed stands alone, and handlers handed to one registrar or symbol
// are one entry.
func (target *targetContext) registeredDuring(from string, handedTo map[string][]string) []RegisteredDuring {
	if from == "" {
		return nil
	}
	seeds := map[string]bool{}
	for _, seed := range target.input.Index.Target.Seeds {
		seeds[seed.ObjectID] = true
	}
	// A library's export is where a program using it starts the call.
	for _, export := range target.input.Index.Target.Exports {
		seeds[export.ObjectID] = true
	}
	callers := map[string][]string{}
	for _, relation := range target.input.Index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.Resolution != programindex.ResolutionExact && relation.Resolution != programindex.ResolutionAlternatives {
			continue
		}
		caller := relation.FromID
		if _, id := target.enclosingSymbol(relation.FromID); id != "" {
			caller = id
		}
		for _, to := range relation.ToIDs {
			if !slices.Contains(callers[to], caller) {
				callers[to] = append(callers[to], caller)
			}
		}
	}
	name := func(id string) string {
		if object, ok := target.object(id); ok {
			return target.declarationName(object)
		}
		return id
	}
	parent := map[string]string{from: ""}
	queue := []string{from}
	var stops []string
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if seeds[current] || len(handedTo[current]) > 0 {
			stops = append(stops, current)
			continue
		}
		next := slices.Clone(callers[current])
		sort.Strings(next)
		for _, caller := range next {
			if _, seen := parent[caller]; seen {
				continue
			}
			parent[caller] = current
			queue = append(queue, caller)
		}
	}
	through := func(stop string) []string {
		var chain []string
		for at := parent[stop]; at != ""; at = parent[at] {
			chain = append(chain, name(at))
		}
		if stop == from {
			return nil
		}
		return chain
	}
	var result []RegisteredDuring
	grouped := map[string]int{}
	for _, stop := range stops {
		if seeds[stop] {
			result = append(result, RegisteredDuring{Seed: name(stop), Through: through(stop)})
			continue
		}
		for _, to := range handedTo[stop] {
			at, ok := grouped[to]
			if !ok {
				at = len(result)
				grouped[to] = at
				result = append(result, RegisteredDuring{HandedTo: to})
			}
			result[at].Handlers = append(result[at].Handlers, name(stop))
			for _, step := range through(stop) {
				if !slices.Contains(result[at].Through, step) {
					result[at].Through = append(result[at].Through, step)
				}
			}
		}
	}
	return result
}

// storedSlots are the fields and module-level variables a registrar stores
// the parameter an argument fills, as it received it: matched by position,
// or by the parameter's name for a keyword argument.
func storedSlots(registrar programindex.Object, argument programindex.PatternArgument) []string {
	var slots []string
	for _, store := range registrar.ParameterStores {
		switch {
		case argument.Keyword != "" && store.Name == argument.Keyword:
		case argument.Keyword == "" && argument.Position > 0 && store.Parameter == argument.Position:
		default:
			continue
		}
		if !slices.Contains(slots, store.Slot) {
			slots = append(slots, store.Slot)
		}
	}
	return slots
}

// declarationName is a declaration as a reader looks it up: a method with
// its type (eventLoop.register), unless its own name already says it.
func (target *targetContext) declarationName(object programindex.Object) string {
	if object.Kind != programindex.ObjectMethod || strings.Contains(object.Name, ".") {
		return object.Name
	}
	if owner, ok := target.object(object.OwnerID); ok && owner.Kind == programindex.ObjectType && owner.Name != "" {
		return owner.Name + "." + object.Name
	}
	return object.Name
}
