package report

import (
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A part whose declarations share one namespace (Clojure's ns/name) names
// its tiles without it; one of another namespace, or with none, keeps every
// name whole.
func TestAPartsSharedNamespaceIsSaidOnce(t *testing.T) {
	symbols := func(names ...string) []pageNodeSymbol {
		var out []pageNodeSymbol
		for _, name := range names {
			out = append(out, pageNodeSymbol{Name: name, Kind: "function"})
		}
		return out
	}
	if got := sharedNamespace(symbols("othello.ui.host/on-press", "othello.ui.host/on-key")); got != "othello.ui.host/" {
		t.Fatalf("shared namespace = %q", got)
	}
	for _, names := range [][]string{{"othello.ui.host/on-press", "othello.ui.draw/draw-state"}, {"othello.ui.host/on-press", "main"}, {"processCommand"}} {
		if got := sharedNamespace(symbols(names...)); got != "" {
			t.Fatalf("%q share %q", names, got)
		}
	}
	// A field of a record keeps the part's namespace question to its
	// declarations.
	withField := append(symbols("othello.board/make-board"), pageNodeSymbol{Name: "cells", Kind: "field"})
	if got := sharedNamespace(withField); got != "othello.board/" {
		t.Fatalf("a field decided the namespace: %q", got)
	}
	// Most of a part's declarations in one namespace are named without it;
	// the one of another keeps its whole name.
	if got := sharedNamespace(symbols("othello.ai/move", "othello.ai.search/choose", "othello.ai.search/negamax")); got != "othello.ai.search/" {
		t.Fatalf("the namespace most share = %q", got)
	}
}

// A callable written inline is read by the name GroupsIndex gives it,
// never by its compiler's number, and still stands on no tile: GroupsIndex
// says it is one (ObjectFacts.Anonymous). A name with a dollar sign is no
// such word: JavaScript's `export function price$()` is a tile and a
// member like any declaration (external review, 2026-10-03).
func TestAnInlineCallableIsReadByItsInlineName(t *testing.T) {
	builder := &pageBuilder{links: pageLinks{repositoryURL: "https://github.com/o/r", blobPrefix: "/blob/", revision: "abc"}}
	closure := groupindex.Subject{ID: "n42", Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{
		Name: "Run$1", Kind: programindex.ObjectFunction, Inline: groupindex.InlineName{In: "ReplicateCommand.Run"}, Anonymous: true,
		Location: &programindex.Location{Path: "cmd/litestream/replicate.go", Line: 183, Column: 7}}}
	if name, _ := builder.subjectDisplay(closure); name != "anonymous function in ReplicateCommand.Run" {
		t.Fatalf("the closure reads %q", name)
	}
	if !builder.inline(closure) {
		t.Fatal("a compiler-numbered callable would stand on a tile")
	}
	named := groupindex.Subject{ID: "n12", Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: "Run", Kind: programindex.ObjectMethod,
		Location: &programindex.Location{Path: "cmd/litestream/replicate.go", Line: 90, Column: 1}}}
	if name, _ := builder.subjectDisplay(named); name != "Run" || builder.inline(named) {
		t.Fatalf("a named method reads %q, inline %v", name, builder.inline(named))
	}
	for _, name := range []string{"price$", "active$", "price$1"} {
		dollar := groupindex.Subject{ID: "n7", Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: name, Kind: programindex.ObjectFunction, Visibility: programindex.VisibilityPublic,
			Location: &programindex.Location{Path: "src/price.js", Line: 1, Column: 17}}}
		if builder.inline(dollar) {
			t.Fatalf("JavaScript's %s stands on no tile", name)
		}
	}
}

// A callable written inline is named in words, in the page's language,
// from GroupsIndex's fields, a holder with a space included (the browser's
// pattern had missed one): the page writes the words and its script reads
// no name back into parts (review, 2026-10-03). An entry saved under its
// inline handler's name is said in the same words; a call made by it
// carries them as its caller's name; and a row between two named
// declarations is said by code, so neither the words nor the row reach
// the model's translation catalogue, which is the same in both languages.
func TestAnInlineCallableIsNamedInWordsOfThePagesLanguage(t *testing.T) {
	for _, check := range []struct {
		name   groupindex.InlineName
		en, ru string
	}{
		{groupindex.InlineName{In: "Main loop"}, "anonymous function in Main loop", "анонимная функция в Main loop"},
		{groupindex.InlineName{In: "Main loop", For: "doctor"}, "anonymous function in Main loop for doctor", "анонимная функция в Main loop для doctor"},
		{groupindex.InlineName{In: "startPeer", Reads: "recvc"}, "anonymous function in startPeer reading recvc", "анонимная функция в startPeer, читающая recvc"},
		{groupindex.InlineName{In: "Start", Calls: "ListenAndServe"}, "anonymous function in Start calling ListenAndServe", "анонимная функция в Start, вызывающая ListenAndServe"},
		{groupindex.InlineName{In: "Main loop", Of: 3}, "one of three anonymous functions in Main loop", "одна из трёх анонимных функций в Main loop"},
		{groupindex.InlineName{In: "Headscale.Serve", Of: 12}, "one of many anonymous functions in Headscale.Serve", ""},
		{groupindex.InlineName{Wraps: "listDatabases"}, "listDatabases", "listDatabases"},
		{groupindex.InlineName{}, "", ""},
	} {
		for language, want := range map[DisplayLanguage]string{English: check.en, Russian: check.ru} {
			if want == "" && check.name.In != "" {
				continue
			}
			builder := &pageBuilder{language: language}
			if got := builder.inlineWords(check.name); got != want {
				t.Fatalf("%s: %+v reads %q, want %q", language, check.name, got, want)
			}
		}
	}

	// An entry named by its inline handler, and a call the handler makes.
	at := func(line int) *programindex.Location {
		return &programindex.Location{Path: "service/loop.go", Line: line, Column: 2}
	}
	closure := groupindex.Subject{ID: "n2", Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: "Main loop$1", Kind: programindex.ObjectFunction,
		Inline: groupindex.InlineName{In: "Main loop", Of: 3}, Anonymous: true, Location: at(12)}}
	callee := groupindex.Subject{ID: "n3", Kind: groupindex.SubjectObject, Object: &groupindex.ObjectFacts{Name: "GetConfigString", Kind: programindex.ObjectFunction, Location: at(40)}}
	for language, want := range map[DisplayLanguage]string{English: "one of three anonymous functions in Main loop", Russian: "одна из трёх анонимных функций в Main loop"} {
		builder := &pageBuilder{language: language, subjects: map[string]subjectRef{subjectKey("t1", "n2"): {subject: closure}, subjectKey("t1", "n3"): {subject: callee}},
			byProgram: map[string]*pageSection{"t1": {ID: "t1", Language: "go"}}, links: pageLinks{repositoryURL: "https://github.com/o/r", blobPrefix: "/blob/", revision: "abc"}}
		entry := groupindex.Operation{ID: "o1", SubjectID: "n2", Kind: "continuous", Name: closure.Object.Inline.String(), Source: "fact"}
		if got := builder.operationDisplayName("t1", entry); got != want {
			t.Fatalf("%s: the entry reads %q, want %q", language, got, want)
		}
		named := entry
		named.Name = "loop"
		if got := builder.operationDisplayName("t1", named); got != "loop" {
			t.Fatalf("%s: an entry its registration named reads %q", language, got)
		}
		connection := groupindex.Connection{ID: "x1", From: groupindex.Endpoint{TargetID: "t1", GroupID: "g1"}, To: groupindex.Endpoint{TargetID: "t1", GroupID: "g2"},
			SourceKind: "native_calls", FromSubjectID: "n2", ToSubjectID: "n3", Label: "Main loop (inline, 3) calls GetConfigString", FromLocation: at(13)}
		call := builder.connectionCall(connection)
		if call == nil || call.CallerName != want || call.CalleeName != "GetConfigString" {
			t.Fatalf("%s: the call reads %+v", language, call)
		}
		var row pageConnection
		row.Summary = connection.Label
		builder.nameConnectionEnds(&row, connection)
		builder.sayRelation(&row, connection.Label)
		if words, _ := uiText(language, "{0} calls {1}", want, "GetConfigString"); row.Label != words || row.Summary != "" {
			t.Fatalf("%s: the row reads %q beside %q", language, row.Label, row.Summary)
		}
	}
}

// A row between two named declarations is said by code (sayRelation) and
// is no text for the model to translate; a row the model wrote is. So a
// callable's words never reach the translation catalogue, which stays the
// same whatever the page's language.
func TestARowSaidByCodeIsNoTextForTheModel(t *testing.T) {
	catalogue := func(language DisplayLanguage) []string {
		builder := &pageBuilder{language: language}
		native := pageConnection{Kind: "calls", FromName: builder.inlineWords(groupindex.InlineName{In: "Main loop", Of: 2}), ToName: "GetConfigString",
			Label: "Main loop (inline, 2) calls GetConfigString"}
		builder.sayRelation(&native, native.Label)
		model := pageConnection{Label: "Feeds the configuration cache."}
		page := &PreparedPage{view: &pageView{Sections: []*pageSection{{ID: "t1", Core: []pageGroup{{Title: "Loop", Connections: []pageConnection{native, model}}}}}},
			catalog: DisplayTextCatalog{Version: DisplayTextVersion, Entries: []DisplayTextEntry{}}}
		if err := page.collectDisplayTexts(&ReportData{}, false); err != nil {
			t.Fatal(err)
		}
		var texts []string
		for _, entry := range page.catalog.Entries {
			texts = append(texts, entry.Text)
		}
		return texts
	}
	english, russian := catalogue(English), catalogue(Russian)
	if !slices.Equal(english, russian) || slices.ContainsFunc(english, func(text string) bool { return strings.Contains(text, "anonymous") || strings.Contains(text, "inline") }) ||
		!slices.Contains(english, "Feeds the configuration cache.") {
		t.Fatalf("catalogue in English %q, in Russian %q", english, russian)
	}
}
