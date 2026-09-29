package reading

import (
	"slices"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// Words an established entry's handler compares with what it was handed
// are that entry's sub-arguments (owner's rule K3), a code fact: Redis's
// sortCommand, the function a command table's row hands sort to, compares
// an element of its client's argument vector with "limit". Such a call is
// not asked what its words become (api_call.go asks it no enters
// question); it is an entry of the handler's kind, declared by the
// handler, whose handler is not established, and GroupsIndex lists it
// under the entry its handler handles (Launch.Nested, Reach.SubArguments).
// readCalls applies it only to a call it would otherwise ask, after its
// talks, handed and other-fact filters, so a call to another program is
// never taken for one.

// handlerKinds are, by the symbol place of each declaration that handles
// established entries, their one kind: a callable handed to an outside
// symbol whose binds answer is an entry, a callable the repository's own
// function keeps whose question decided one, and a function a starting
// statement starts. A declaration handling entries of two kinds has none.
func (r *reader) handlerKinds() map[string]string {
	kinds := map[string][]string{}
	for _, place := range r.opts.Graph.Places {
		b := place.Boundary
		if b == nil || b.Source != "fact" || b.Direction != atlas.DirectionIn || b.SubjectID == "" || b.GivenKind != "" || b.Handed || r.testFile(place.Parent) {
			continue
		}
		var kind string
		switch {
		case b.Invocation != "":
			kind = r.startKinds[startSite(place)]
		case b.Registrar != nil:
			kind = r.storedKinds[storedKey(b)]
		case b.External != "":
			kind = r.api[b.External].binds
		}
		if slices.Contains(atlas.EntryKinds(), kind) && !slices.Contains(kinds[b.SubjectID], kind) {
			kinds[b.SubjectID] = append(kinds[b.SubjectID], kind)
		}
	}
	result := make(map[string]string, len(kinds))
	for subject, found := range kinds {
		if len(found) == 1 {
			result[subject] = found[0]
		}
	}
	return result
}

// subArgumentKind is the kind of the entry whose sub-argument a call's
// words are, or "". The call is written in an established entry's handler
// and compares its word with part of what that handler was handed, a field
// or an element of one of its own parameters: an argument written before
// the call's first word is one (strcasecmp(c->argv[j]->ptr, "limit"),
// strings.EqualFold(r.Method, "HEAD")), or the call is made on one and the
// word is its first argument (r.Header.Get("X-Verbose")). A parameter used
// whole beside a word is as often where the handler writes its answer
// (fmt.Fprintf(w, "…"), res.send("…"), c.String(200, "…")) as what it looks
// up (req.get("X-Token")), so such a call is asked; a word written before
// the value formats it (log.Printf("from %s", r.RemoteAddr)). A method's
// receiver (Go's, Python's self) is the handler's own state, not what it
// was handed.
func subArgumentKind(place atlas.Place, call atlas.SymbolCall, kinds map[string]string) string {
	kind := kinds[place.ID]
	if kind == "" || place.Symbol == nil {
		return ""
	}
	decl := place.Symbol.Decl
	own := func(value *sourcevalue.Value) bool {
		return value != nil && value.Kind == "parameter" && value.Owner != nil && value.Owner.Path == place.Path && value.Owner.Line == decl.LineNo
	}
	var part func(value *sourcevalue.Value) bool
	part = func(value *sourcevalue.Value) bool {
		if value == nil || value.Kind != "field" && value.Kind != "index" || len(value.Parts) == 0 {
			return false
		}
		return own(&value.Parts[0]) || part(&value.Parts[0])
	}
	firstWord := 0
	for _, argument := range call.SourceArguments {
		if argument.Origin != nil && argument.Origin.Kind == "literal" && argument.Position > 0 && (firstWord == 0 || argument.Position < firstWord) {
			firstWord = argument.Position
		}
	}
	if firstWord == 0 {
		return ""
	}
	if firstWord == 1 && part(call.ReceiverValue) {
		return kind
	}
	for _, argument := range call.SourceArguments {
		if argument.Position > 0 && argument.Position < firstWord && part(argument.Origin) {
			return kind
		}
	}
	return ""
}
