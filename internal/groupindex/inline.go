package groupindex

import (
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// InlineName is how a reader names a callable written inline in another,
// as fields: the repository callable it only wraps (Wraps), else the
// function whose lines hold it (In, a method with its type), with the word
// its hand-over gives it (For), else the first thing only it of those its
// holder writes alike calls (Calls) or reads (Reads), or, when nothing
// tells it apart, how many they are (Of). The report says it in
// words in its own language ("one of three anonymous functions in
// StartLdapServer"); nothing reads these fields back out of a name
// (review, 2026-10-03: the page had parsed "StartLdapServer (inline, 3)").
type InlineName struct {
	Wraps string
	In    string
	For   string
	Calls string
	Reads string
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
	case name.Calls != "":
		return name.In + " (inline calling " + name.Calls + ")"
	case name.Reads != "":
		return name.In + " (inline reading " + name.Reads + ")"
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
		uses := ownUses(program, ids, byID, named, inline)
		for position, id := range ids {
			switch use := uses[position]; {
			case use.word == "":
				result[id] = InlineName{In: holder, Of: len(ids)}
			case use.reads:
				result[id] = InlineName{In: holder, Reads: use.word}
			default:
				result[id] = InlineName{In: holder, Calls: use.word}
			}
		}
	}
	return result
}

// ownUse is the first thing one callable of several written alike uses
// that none of the others does: a callee's name, or a read declaration's
// with reads set.
type ownUse struct {
	word  string
	reads bool
}

// ownUses tells callables one function writes alike apart by what only
// each of them uses, in its own source order, a repository declaration
// before an outside one: etcd's startPeer goroutines read recvc and propc,
// casdoor's Start goroutines call http.ListenAndServe and
// Config.GetCertificate, the gateway's handlers in
// RegisterElectionHandlerServer each call their own local_request_. A
// callable using nothing the others do keeps no word (they stay one of how
// many).
func ownUses(program programindex.Index, ids []string, byID map[string]programindex.Object, named func(programindex.Object) string, inline func(programindex.Object) bool) []ownUse {
	type use struct {
		to    string
		reads bool
		at    programindex.Location
	}
	position := make(map[string]int, len(ids))
	for i, id := range ids {
		position[id] = i
	}
	uses := make([][]use, len(ids))
	for _, relation := range program.Relations {
		at, ours := position[relation.FromID]
		if !ours || relation.Location == nil {
			continue
		}
		reads := relation.Kind == programindex.RelationReads
		if !reads && relation.Kind != programindex.RelationCalls && relation.Kind != programindex.RelationInvokesExternal {
			continue
		}
		for _, to := range relation.ToIDs {
			if to != relation.FromID {
				uses[at] = append(uses[at], use{to: to, reads: reads, at: *relation.Location})
			}
		}
	}
	users := map[string]map[int]bool{}
	for at, list := range uses {
		for _, used := range list {
			if users[used.to] == nil {
				users[used.to] = map[int]bool{}
			}
			users[used.to][at] = true
		}
	}
	// An outside callee reads as its code writes it: a method with its
	// type (Config.GetCertificate), a function with its package's last
	// element (http.ListenAndServe, status.Error).
	word := func(object programindex.Object) string {
		if external := object.External; external != nil {
			if receiver := strings.TrimPrefix(external.Receiver, "*"); receiver != "" {
				return receiver + "." + external.Name
			}
			if external.PackagePath != "" {
				return path.Base(external.PackagePath) + "." + external.Name
			}
			return external.Name
		}
		return named(object)
	}
	result := make([]ownUse, len(ids))
	for at, list := range uses {
		sort.SliceStable(list, func(i, j int) bool {
			a, b := list[i].at, list[j].at
			return a.Path < b.Path || a.Path == b.Path && (a.Line < b.Line || a.Line == b.Line && a.Column < b.Column)
		})
		for _, outside := range []bool{false, true} {
			for _, used := range list {
				object, known := byID[used.to]
				if !known || len(users[used.to]) != 1 || (object.External != nil) != outside || !outside && (object.Location == nil || inline(object)) {
					continue
				}
				if said := word(object); said != "" && !strings.ContainsAny(said, " \t\r\n()") && validText(said) {
					result[at] = ownUse{word: said, reads: used.reads}
					break
				}
			}
			if result[at].word != "" {
				break
			}
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
