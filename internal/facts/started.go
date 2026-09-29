package facts

import (
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// A started callable is the one exception, beside table rows, to a call of
// the repository's own function being delegation: a call written with the
// shared invocation word goroutine (Go's `go f()`) or async_task (a
// coroutine handed to another call, asyncio.create_task(f())) does not run
// its callee in place, it hands it over to run on its own. With one exact
// repository callee it is a registration handing that callee over. Its word
// is the statement that starts it, `go`, or the call the coroutine is
// handed to as written; its literals are the started call's own. No outside
// symbol receives it, so no symbol's role decides it: the reading asks each
// statement on its own what the started work is.

// startReader reads the started callables of one target.
type startReader struct {
	target *targetContext
	// startingCalls are, by the site of a call whose result another call is
	// given, that other call's word: refresh(stream)'s site names
	// create_task in asyncio.create_task(refresh(stream)).
	startingCalls map[sourcevalue.Anchor]string
	// from are each declaration's own calls, of repository and outside
	// callees alike.
	from map[string][]programindex.Relation
}

func (target *targetContext) newStartReader() *startReader {
	reader := &startReader{target: target, startingCalls: map[sourcevalue.Anchor]string{}, from: map[string][]programindex.Relation{}}
	for _, relation := range target.input.Index.Relations {
		if relation.Kind != programindex.RelationCalls && relation.Kind != programindex.RelationInvokesExternal {
			continue
		}
		reader.from[relation.FromID] = append(reader.from[relation.FromID], relation)
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				if at := producedAt(argument.Origin); at != nil && reader.startingCalls[*at] == "" {
					reader.startingCalls[*at] = pattern.Selector
				}
			}
		}
	}
	return reader
}

// shapes are the registrations a call makes by starting its one exact
// repository callee, one per site; none for any other call.
func (reader *startReader) shapes(relation programindex.Relation) []registrationShape {
	target := reader.target
	if relation.Kind != programindex.RelationCalls || relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 {
		return nil
	}
	if relation.Invocation != programindex.InvocationGoroutine && relation.Invocation != programindex.InvocationAsyncTask {
		return nil
	}
	callee, ok := target.object(relation.ToIDs[0])
	if !ok || !isCallable(callee) || !target.ownsObject(callee.ID) {
		return nil
	}
	var shapes []registrationShape
	for _, pattern := range relation.Patterns {
		word := "go"
		if relation.Invocation == programindex.InvocationAsyncTask {
			// A coroutine is started by the call it is handed to; one
			// handed to none is only created.
			if pattern.Location == nil {
				continue
			}
			if word = reader.startingCalls[sourcevalue.Anchor{Path: pattern.Location.Path, Line: pattern.Location.Line, Column: pattern.Location.Column}]; word == "" {
				continue
			}
		}
		handler := reader.handler(callee, pattern.Location)
		shape := registrationShape{relation: relation, pattern: pattern, word: word, handlerName: handler.Name, handlerID: handler.ID,
			handed: true, originKnown: true, invocation: relation.Invocation}
		for _, argument := range pattern.Arguments {
			if value, _, literal := literalValue(argument); literal && value != "" {
				shape.literals = append(shape.literals, value)
			}
		}
		shapes = append(shapes, shape)
	}
	return shapes
}

// handler is the callable a start hands over, as a reader looks it up. A
// function written in the starting statement itself (a closure: go func()
// { defer wg.Done(); s.monitor(ctx) }()) is named and handled by the one
// repository callable it calls itself, when it calls one: DB.Open's and
// Store.Open's closures are both Open$1, their monitors are not. A call
// whose result another of its calls is given is part of that call
// (monitor(ctx, s.Level()) is one call), and a deferred call runs only as
// it returns. A closure calling several, or a call the code could not
// follow, keeps its own name.
func (reader *startReader) handler(callee programindex.Object, statement *programindex.Location) programindex.Object {
	target := reader.target
	if !writtenIn(callee, statement) {
		return callee
	}
	inner := map[sourcevalue.Anchor]bool{}
	for _, relation := range reader.from[callee.ID] {
		for _, pattern := range relation.Patterns {
			if at := producedAt(pattern.ReceiverValue); at != nil {
				inner[*at] = true
			}
			for _, argument := range pattern.Arguments {
				if at := producedAt(argument.Origin); at != nil {
					inner[*at] = true
				}
			}
		}
	}
	only := ""
	for _, relation := range reader.from[callee.ID] {
		if relation.Kind != programindex.RelationCalls || relation.Invocation == programindex.InvocationDeferred {
			continue
		}
		for _, pattern := range relation.Patterns {
			if location := pattern.Location; location != nil && inner[sourcevalue.Anchor{Path: location.Path, Line: location.Line, Column: location.Column}] {
				continue
			}
			if relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) != 1 || only != "" && only != relation.ToIDs[0] {
				return callee
			}
			only = relation.ToIDs[0]
		}
	}
	if object, ok := target.object(only); ok && isCallable(object) && target.ownsObject(object.ID) {
		return object
	}
	return callee
}

// writtenIn reports a callable declared in the statement at a location:
// on its line, after the statement's first column.
func writtenIn(callee programindex.Object, statement *programindex.Location) bool {
	at := callee.Location
	return at != nil && statement != nil && at.Path == statement.Path && at.Line == statement.Line && at.Column > statement.Column
}
