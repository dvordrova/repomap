package reading

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

// A walk ending at a value its adapter could not read establishes no
// address from code: Redis's connect is handed `(struct sockaddr*)&sa`, a
// cast of a local structure, which the page had printed as where the
// address passes through. The expression stays the frontier (the question
// still sees it) and the use is marked unread; a template with an unread
// part is still read.
func TestAWalkEndingAtAnUnreadValueEstablishesNoAddress(t *testing.T) {
	at := func(column int) *sourcevalue.Anchor { return &sourcevalue.Anchor{Path: "anet.c", Line: 158, Column: column} }
	call := func(column int, origin *sourcevalue.Value) atlas.SymbolCall {
		return atlas.SymbolCall{Name: "connect", Line: 158, Column: column, API: &atlas.CallAPI{Package: "socket.h", Name: "connect"},
			SourceArguments: []atlas.SourceArgument{{Position: 2, Origin: origin}}}
	}
	cast := call(9, &sourcevalue.Value{Kind: "unknown", Text: "(struct sockaddr*)&sa", Anchor: at(20)})
	template := call(40, &sourcevalue.Value{Kind: "concat", Parts: []sourcevalue.Value{{Kind: "literal", Text: "unix:"}, {Kind: "unknown", Text: "path", Anchor: at(50)}}})
	place := atlas.Place{ID: "anetTcpGenericConnect", Path: "anet.c", LineNo: 128, TargetIDs: []string{"cli"},
		Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: "anetTcpGenericConnect"}, Calls: []atlas.SymbolCall{cast, template}}}
	reader := NewDestinationReader([]atlas.Place{place}, argumentsAt("socket.h.connect", ArgumentChoice{Position: 2}))
	uses := reader.Read(place, cast)
	if len(uses) != 1 || !uses[0].Unread || uses[0].Address != "" || uses[0].Frontier != "(struct sockaddr*)&sa" {
		t.Fatalf("a cast the adapter could not read is not an unread end: %+v", uses)
	}
	if uses := reader.Read(place, template); len(uses) != 1 || uses[0].Unread || uses[0].Frontier != "unix:{path}" {
		t.Fatalf("a template with an unread part is not read as a template: %+v", uses)
	}
}
