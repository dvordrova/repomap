package groupindex

import (
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// inlineNames are, by object, how a reader names a callable written inline
// in another (a Go closure the adapter numbers Run$1, a lambda): the
// repository function or method it only wraps, when that call is the only
// one it makes, or else the function whose lines hold it followed by
// " (inline)" (owner, 2026-09-29: never print $N). litestream's metrics
// goroutine, Run$1, calls only net/http and is "ReplicateCommand.Run
// (inline)"; RestoreTool$1 calls isReplicaURL among its outside calls, and is
// "RestoreTool (inline)", not a helper it checks with. The function holding
// it is the innermost callable, not itself inline, whose lines hold it; a
// method reads with its type. A callable no other holds keeps its name.
func inlineNames(program programindex.Index) map[string]string {
	return inlineNamesBy(program, true)
}

// inlineHolders names every callable written inline by the function whose
// lines hold it, followed by " (inline)", never by a function it wraps: a
// relation between it and that function would read "IsSQLiteDatabase calls
// IsSQLiteDatabase".
func inlineHolders(program programindex.Index) map[string]string {
	return inlineNamesBy(program, false)
}

func inlineNamesBy(program programindex.Index, wraps bool) map[string]string {
	inline := func(object programindex.Object) bool {
		return object.Kind == programindex.ObjectLambda || closureNumbered(object.Name)
	}
	byID := make(map[string]programindex.Object, len(program.Objects))
	var callables, closures []programindex.Object
	for _, object := range program.Objects {
		byID[object.ID] = object
		if !object.Kind.Callable() || object.Location == nil {
			continue
		}
		if inline(object) {
			closures = append(closures, object)
		} else {
			callables = append(callables, object)
		}
	}
	if len(closures) == 0 {
		return nil
	}
	named := func(object programindex.Object) string {
		if owner, ok := byID[object.OwnerID]; ok && object.Kind == programindex.ObjectMethod && owner.Kind == programindex.ObjectType && owner.Name != "" && !strings.Contains(object.Name, ".") {
			return strings.TrimPrefix(owner.Name, "*") + "." + object.Name
		}
		return object.Name
	}
	// Every callee each closure calls, repository or outside, once.
	callees := map[string][]string{}
	for _, relation := range program.Relations {
		if relation.Kind != programindex.RelationCalls && relation.Kind != programindex.RelationInvokesExternal {
			continue
		}
		for _, to := range relation.ToIDs {
			if to != relation.FromID && !containsString(callees[relation.FromID], to) {
				callees[relation.FromID] = append(callees[relation.FromID], to)
			}
		}
	}
	result := make(map[string]string, len(closures))
	for _, closure := range closures {
		if called := callees[closure.ID]; wraps && len(called) == 1 {
			if callee, known := byID[called[0]]; known && callee.Location != nil && callee.External == nil && callee.Kind.Callable() && !inline(callee) {
				result[closure.ID] = named(callee)
				continue
			}
		}
		at := closure.Location
		var holder *programindex.Object
		for i := range callables {
			candidate := &callables[i]
			start := candidate.Location
			if start.Path != at.Path || start.Line > at.Line || candidate.EndLine < at.Line {
				continue
			}
			if holder == nil || start.Line > holder.Location.Line || start.Line == holder.Location.Line && candidate.EndLine < holder.EndLine {
				holder = candidate
			}
		}
		if holder != nil {
			result[closure.ID] = named(*holder) + " (inline)"
		}
	}
	return result
}

// closureNumbered says a name ends as the Go adapter numbers a function
// literal: its enclosing function's name, a dollar sign and a number.
func closureNumbered(name string) bool {
	at := strings.LastIndexByte(name, '$')
	if at <= 0 || at == len(name)-1 {
		return false
	}
	for _, r := range name[at+1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
