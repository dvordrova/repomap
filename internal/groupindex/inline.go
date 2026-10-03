package groupindex

import (
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// InlineName is how a reader names a callable written inline in another,
// as fields: the repository callable it only wraps (Wraps), else the
// function whose lines hold it (In, a method with its type), with the word
// its hand-over gives it (For) or, when its holder writes several alike and
// no word tells them apart, how many they are (Of). The report says it in
// words in its own language ("one of three anonymous functions in
// StartLdapServer"); nothing reads these fields back out of a name
// (review, 2026-10-03: the page had parsed "StartLdapServer (inline, 3)").
type InlineName struct {
	Wraps string
	In    string
	For   string
	Of    int
}

// IsZero says the callable is named by its own name: no reader's name was
// found for it.
func (name InlineName) IsZero() bool { return name == InlineName{} }

// String is the name as analysis writes it into saved text (an entry
// named by its handler, a connection's label): its wrapped callable's name,
// or "In (inline)", "In (inline for For)", "In (inline, Of)".
func (name InlineName) String() string {
	switch {
	case name.Wraps != "":
		return name.Wraps
	case name.In == "":
		return ""
	case name.For != "":
		return name.In + " (inline for " + name.For + ")"
	case name.Of > 1:
		return name.In + " (inline, " + strconv.Itoa(name.Of) + ")"
	}
	return name.In + " (inline)"
}

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
func inlineNames(program programindex.Index) map[string]InlineName {
	return inlineNamesBy(program, true)
}

// inlineHolders names every callable written inline by the function whose
// lines hold it, followed by " (inline)", never by a function it wraps: a
// relation between it and that function would read "IsSQLiteDatabase calls
// IsSQLiteDatabase".
func inlineHolders(program programindex.Index) map[string]InlineName {
	return inlineNamesBy(program, false)
}

// writtenInline says an object is a callable written inline in another,
// with no name of its own in the code, as its adapter says: a lambda by its
// kind, a Go function literal by ProgramIndex's Anonymous (go/ssa names it
// Run$1). A name decides nothing: a JavaScript function may well be called
// price$1 (external review, 2026-10-03).
func writtenInline(object programindex.Object) bool {
	return object.Kind.Callable() && (object.Kind == programindex.ObjectLambda || object.Anonymous)
}

func inlineNamesBy(program programindex.Index, wraps bool) map[string]InlineName {
	inline := writtenInline
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
	result := make(map[string]InlineName, len(closures))
	// holderOf is, by closure named after the function whose lines hold it,
	// that function's ID: closures are counted alike by the function holding
	// them, never by its name, which two functions (two packages' main, two
	// types' Start) may share.
	holderOf := make(map[string]string, len(closures))
	for _, closure := range closures {
		if called := callees[closure.ID]; wraps && len(called) == 1 {
			if callee, known := byID[called[0]]; known && callee.Location != nil && callee.External == nil && callee.Kind.Callable() && !inline(callee) {
				result[closure.ID] = InlineName{Wraps: named(callee)}
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
			result[closure.ID] = InlineName{In: named(*holder)}
			holderOf[closure.ID] = holder.ID
		}
	}
	// Callables one function writes alike are told apart by the word each
	// one's hand-over gives it: a field of the value it is handed in
	// (headscale's cmd/hi commands, Name = "doctor"), read "anonymous
	// function in main for doctor". When nothing tells them apart they are
	// each said as one of how many they are, never by a number or an
	// ordinal of one's own (owner's reviewer, 2026-10-02): casdoor's two
	// goroutines of Start are "Start (inline, 2)", read "one of two
	// anonymous functions in Start".
	alike := map[string][]string{}
	var order []string
	for _, closure := range closures {
		holderID, held := holderOf[closure.ID]
		if !held {
			continue
		}
		if _, seen := alike[holderID]; !seen {
			order = append(order, holderID)
		}
		alike[holderID] = append(alike[holderID], closure.ID)
	}
	for _, holderID := range order {
		ids := alike[holderID]
		if len(ids) < 2 {
			continue
		}
		holder := named(byID[holderID])
		if words := handedWords(program, ids); words != nil {
			for position, id := range ids {
				result[id] = InlineName{In: holder, For: words[position]}
			}
			continue
		}
		for _, id := range ids {
			result[id] = InlineName{In: holder, Of: len(ids)}
		}
	}
	return result
}

// handedWords are, for callables written alike, the words telling them
// apart: the string one field of the value each is handed in holds
// (callable_receiver_field witnesses, Name = "doctor"), the first field
// every one of them has, with a word of no space that differs for each.
// Nil when no field tells them all apart.
func handedWords(program programindex.Index, ids []string) []string {
	fields := make([]map[string]string, len(ids))
	var names []string
	position := make(map[string]int, len(ids))
	for i, id := range ids {
		fields[i] = map[string]string{}
		position[id] = i
	}
	for _, relation := range program.Relations {
		if relation.Kind != programindex.RelationPassesCallback {
			continue
		}
		for _, to := range relation.ToIDs {
			at, ours := position[to]
			if !ours {
				continue
			}
			for _, witness := range relation.Witnesses {
				field, literal, found := strings.Cut(witness.Detail, " = ")
				if witness.Kind != "callable_receiver_field" || !found {
					continue
				}
				word, err := strconv.Unquote(literal)
				if err != nil || fields[at][field] != "" {
					continue
				}
				fields[at][field] = word
				if !containsString(names, field) {
					names = append(names, field)
				}
			}
		}
	}
	for _, field := range names {
		words := make([]string, len(ids))
		seen := map[string]bool{}
		for i := range ids {
			word := fields[i][field]
			if word == "" || seen[word] || strings.ContainsAny(word, " \t\r\n()") || !validText(word) {
				words = nil
				break
			}
			seen[word] = true
			words[i] = word
		}
		if words != nil {
			return words
		}
	}
	return nil
}
