package report

import (
	"bytes"
	"html/template"
	"regexp"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

func renderGroupFragment(t *testing.T, language DisplayLanguage, group pageGroup) string {
	t.Helper()
	parsed, err := template.New("report").Funcs(pageTemplateFuncs(language)).ParseFS(reportTemplateFS, "templates/html/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := parsed.ExecuteTemplate(&out, "group", group); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// The reading printed a relation's kind as it was stored: "initServer
// passes_callback acceptHandler" beside "initServer passes callback
// acceptHandler", and a joint between two programs as "integrates with"
// with neither function named. Every kind is said through one closed
// vocabulary of UI messages, an include as an include, and a joint names
// the declarations at both of its ends.
func TestRelationRowsAreSaidInOneVocabulary(t *testing.T) {
	for _, kind := range []programindex.RelationKind{programindex.RelationCalls, programindex.RelationImports, programindex.RelationImplements,
		programindex.RelationDecorates, programindex.RelationPassesCallback, programindex.RelationBindsImplementation, programindex.RelationSources,
		programindex.RelationExecutes, programindex.RelationReads, programindex.RelationWrites, programindex.RelationInvokesExternal} {
		if !kind.Valid() {
			t.Fatalf("%s is not a relation kind", kind)
		}
		word := relationWord(string(kind), "go")
		if word == "" {
			t.Fatalf("%s has no phrase", kind)
		}
		if _, translated := russianUI[relationPhrases[word]]; !translated {
			t.Fatalf("%s is said outside the UI vocabulary: %q", kind, relationPhrases[word])
		}
	}
	if relationWord("imports", "c") != relationIncludes || relationWord("imports", "go") != "imports" {
		t.Fatal("a C include is not said as an include")
	}
	if _, translated := russianUI[relationPhrases[relationIntegration]]; !translated {
		t.Fatal("a joint between programs is said outside the UI vocabulary")
	}

	subject := func(target, id, name, path string, line int) (string, subjectRef) {
		return subjectKey(target, id), subjectRef{subject: groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: name, Kind: programindex.ObjectFunction,
			Location: &programindex.Location{Path: path, Line: line, Column: 1}}}}
	}
	builder := pageBuilder{subjects: map[string]subjectRef{}, groupTitles: map[groupindex.Endpoint]string{},
		byProgram: map[string]*pageSection{"server": {ID: "server", Language: "c", ShortLabel: "redis-server"}, "cli": {ID: "cli", Language: "c", ShortLabel: "redis-cli"}}}
	for _, item := range [][5]any{{"server", "init", "initServer", "redis.c", 1560}, {"server", "accept", "acceptHandler", "redis.c", 2551},
		{"server", "anetAccept", "anetAccept", "anet.c", 250}, {"cli", "connect", "anetTcpGenericConnect", "anet.c", 140}, {"server", "main", "redis.c", "redis.c", 1}, {"server", "header", "adlist.h", "adlist.h", 1}} {
		key, ref := subject(item[0].(string), item[1].(string), item[2].(string), item[3].(string), item[4].(int))
		builder.subjects[key] = ref
	}
	here := groupindex.Endpoint{TargetID: "server", GroupID: "runtime"}
	builder.groupTitles[groupindex.Endpoint{TargetID: "server", GroupID: "clients"}] = "Client connections"
	builder.groupTitles[groupindex.Endpoint{TargetID: "cli", GroupID: "sockets"}] = "Network sockets"
	builder.groupTitles[groupindex.Endpoint{TargetID: "server", GroupID: "lists"}] = "Linked list"
	server := groupindex.Index{Target: programindex.Target{ID: "server"}, Connections: []groupindex.Connection{
		{ID: "x1", From: here, To: groupindex.Endpoint{TargetID: "server", GroupID: "clients"}, Label: "initServer passes_callback acceptHandler", Summary: "initServer passes_callback acceptHandler",
			SourceKind: "native_passes_callback", FromSubjectID: "init", ToSubjectID: "accept", FromLocation: &programindex.Location{Path: "redis.c", Line: 1577, Column: 5}},
		{ID: "x2", From: groupindex.Endpoint{TargetID: "cli", GroupID: "sockets"}, To: here, Label: "integrates with", SourceKind: "integration",
			FromSubjectID: "connect", ToSubjectID: "anetAccept", SupportResolution: programindex.PatternValuePossible,
			FromLocation: &programindex.Location{Path: "anet.c", Line: 158, Column: 1}, ToLocation: &programindex.Location{Path: "anet.c", Line: 256, Column: 1}},
		{ID: "x3", From: here, To: groupindex.Endpoint{TargetID: "server", GroupID: "lists"}, Label: "redis.c imports adlist.h", SourceKind: "native_imports", FromSubjectID: "main", ToSubjectID: "header"},
	}}
	builder.indexes = []groupindex.Index{server}
	card := pageGroup{ID: "runtime", Title: "Server runtime", Connections: builder.groupConnections(server, groupindex.Group{ID: "runtime"})}
	english := renderGroupFragment(t, English, card)
	for _, want := range []string{"initServer passes acceptHandler as a callback", "anetTcpGenericConnect connects to anetAccept", "redis.c includes adlist.h"} {
		if !strings.Contains(english, want) {
			t.Fatalf("a relation is not said in the vocabulary %q:\n%s", want, english)
		}
	}
	shown := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(english, " ")
	for _, raw := range []string{"passes_callback", "integrates with", "imports adlist.h"} {
		if strings.Contains(shown, raw) {
			t.Fatalf("a stored word %q is shown raw:\n%s", raw, english)
		}
	}
	if russian := renderGroupFragment(t, Russian, card); !strings.Contains(russian, "initServer передаёт acceptHandler как обратный вызов") {
		t.Fatalf("the relation's words are not the report's translated words:\n%s", russian)
	}
	// The arrow's card reads a call as caller, relation and callee, three
	// words with the relation's underscores read as spaces, and links the
	// two names; a longer sentence showed neither name, and cmdTable's
	// callbacks read "Generic key commands redis.c:709". The joint names
	// both functions there too, where "integrates with" named none.
	cardCall := regexp.MustCompile(`^(\S+) (\S+) (\S+)$`)
	for connection, want := range map[int][3]string{0: {"initServer", "passes callback", "acceptHandler"}, 1: {"anetTcpGenericConnect", "connects to", "anetAccept"}, 2: {"redis.c", "includes", "adlist.h"}} {
		call := builder.connectionCall(server.Connections[connection])
		if call == nil {
			t.Fatalf("connection %d has no call on its arrow", connection)
		}
		parts := cardCall.FindStringSubmatch(call.Label)
		if parts == nil || parts[1] != want[0] || strings.ReplaceAll(parts[2], "_", " ") != want[1] || parts[3] != want[2] {
			t.Fatalf("the arrow's card cannot read %q as %v", call.Label, want)
		}
	}
}

// Client connections and replies listed "cmdTable calls …" ninety-seven
// times, one row each, and the answer a reader wanted sat below them. Rows
// of one caller and one relation kind fold into one line with its count;
// every row stays inside it, and a list with folds can open them all.
func TestEvidenceListsFoldOneCallerAndKindIntoOneLine(t *testing.T) {
	anchor := func(path string, line int) *pageAnchor {
		return &pageAnchor{Path: path, Line: line, Href: "https://example.test/" + path + "#L" + strings.Repeat("1", line%3+1), Text: path}
	}
	row := func(from, kind, to string, line int) pageConnection {
		return pageConnection{Native: true, Kind: kind, FromName: from, ToName: to, FromDecl: anchor("redis.c", 700), ToDecl: anchor("redis.c", line), FromSource: anchor("redis.c", line), Label: from + " " + kind + " " + to}
	}
	rows := []pageConnection{row("cmdTable", "calls", "getCommand", 3700), row("processCommand", "calls", "call", 2100), row("cmdTable", "calls", "setCommand", 3750),
		row("cmdTable", "passes_callback", "getCommand", 3700), row("cmdTable", "calls", "getCommand", 3701)}
	folds := foldRelationRows(rows)
	if len(folds) != 3 || len(folds[0].Rows) != 3 || folds[0].Callees() != "getCommand, setCommand" || folds[1].Folded() || folds[2].Folded() {
		t.Fatalf("rows are not folded by caller and kind: %+v", folds)
	}
	html := renderGroupFragment(t, English, pageGroup{ID: "clients", Title: "Clients", InternalConnections: rows})
	if !strings.Contains(html, `<details class="conn-fold"><summary>cmdTable calls getCommand, setCommand · 3</summary>`) {
		t.Fatalf("the fold is not one line with its count:\n%s", html)
	}
	if strings.Count(html, `class="conn"`) != len(rows) || strings.Count(html, "data-open-all") != 1 {
		t.Fatalf("a folded row was lost, or the list cannot open its folds at once:\n%s", html)
	}
	single := renderGroupFragment(t, English, pageGroup{ID: "clients", Title: "Clients", InternalConnections: rows[1:2]})
	if strings.Contains(single, "conn-fold") || strings.Contains(single, "data-open-all") {
		t.Fatalf("a list with nothing folded offers to open folds:\n%s", single)
	}
}

// A declaration's reading finds its callers and callees in the rows its
// part already lists. Each row carries the declarations at its ends the way
// the page's script keys a declaration, not only their names.
func TestRelationRowsNameTheDeclarationsAtTheirEnds(t *testing.T) {
	part := groupindex.Group{ID: "clients", Title: "Client connections", MemberSubjectIDs: []string{"input", "command"}}
	builder := pageBuilder{subjects: map[string]subjectRef{}, links: pageLinks{repositoryURL: "https://github.com/redis/redis", blobPrefix: "/blob/", revision: "abc"}}
	for _, item := range []struct {
		id, name string
		line     int
	}{{"input", "processInputBuffer", 2304}, {"command", "processCommand", 2072}} {
		builder.subjects[item.id] = subjectRef{subject: groupindex.Subject{ID: item.id, Object: &groupindex.ObjectFacts{Name: item.name, Location: &programindex.Location{Path: "redis.c", Line: item.line, Column: 1}}}}
	}
	index := groupindex.Index{Groups: []groupindex.Group{part}, StructuralEdges: []groupindex.StructuralEdge{{Role: groupindex.EdgeRelationTarget, RelationID: "r1", RelationKind: programindex.RelationCalls,
		FromSubjectID: "input", ToSubjectID: "command", Resolution: programindex.ResolutionExact, Location: &programindex.Location{Path: "redis.c", Line: 2353, Column: 9}}}}
	rows := builder.internalGroupConnections(index, part)
	_, input := builder.subjectDisplay(builder.subjects["input"].subject)
	_, command := builder.subjectDisplay(builder.subjects["command"].subject)
	if len(rows) != 1 || rows[0].FromKey() != input.Href || rows[0].ToKey() != command.Href || rows[0].FromSource.Line != 2353 {
		t.Fatalf("the row does not name its declarations apart from its call site: %+v", rows)
	}
	html := renderGroupFragment(t, English, pageGroup{ID: "clients", Title: "Clients", InternalConnections: rows})
	for _, want := range []string{`data-kind="calls"`, `data-from-decl="` + input.Href + `"`, `data-to-decl="` + command.Href + `"`, `data-from-name="processInputBuffer"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("the row's ends are not in the page: %s\n%s", want, html)
		}
	}
	// A declaration without a link is keyed by its place, as the script keys it.
	if got := declarationKey(&pageAnchor{Path: `a "b".c`, Line: 7, NoSource: true}); got != `["a \"b\".c",7]` {
		t.Fatalf("a declaration without a link is keyed %s", got)
	}
}

// "To explanation" and "To code" scrolled to what was already on screen,
// "More details ↓" moved a small box by a step, and a click on any tile but
// the keys said "No explanation saved". Their words are gone from the
// report's vocabulary, and the reading's script throws on a word it does
// not have, so none of them can come back unnoticed.
func TestReadingOffersNoButtonsThatPointAtWhatIsVisible(t *testing.T) {
	for _, gone := range []string{"To code", "More details ↓", "↑ Back to top", "No explanation saved. Open the source to inspect this element.", "legend"} {
		if _, present := russianUI[gone]; present {
			t.Errorf("the reading still has the words %q", gone)
		}
	}
}

// Code in this part listed only the model's keys under a heading that said
// all of it: Replication's showed replicationFeedSlaves and not
// syncWithMaster. It lists every declaration, the keys first and bold, as
// the part's tiles draw them.
func TestCodeInThisPartListsEveryDeclarationKeysFirst(t *testing.T) {
	chip := func(name string, line int, key bool) pageChip {
		return pageChip{Name: name, Line: line, Key: key, Anchor: pageAnchor{Path: "redis.c", Line: line, Text: "redis.c:" + string(rune('0'+line%10))}}
	}
	rows := []pageChipRow{{Path: "anet.c", Members: []pageChip{chip("anetAccept", 250, false)}}, {Path: "redis.c", Members: []pageChip{chip("syncWithMaster", 7216, false), chip("replicationFeedSlaves", 2234, true)}}}
	ordered := keysFirst(rows)
	if ordered[0].Path != "redis.c" || ordered[0].Members[0].Name != "replicationFeedSlaves" || ordered[0].Members[1].Name != "syncWithMaster" || ordered[1].Members[0].Name != "anetAccept" {
		t.Fatalf("the keys do not come first: %+v", ordered)
	}
	if rows[1].Members[0].Name != "syncWithMaster" {
		t.Fatal("ordering the code list reordered the source index")
	}
	html := renderGroupFragment(t, English, pageGroup{ID: "replication", Title: "Replication", Highlights: ordered})
	if !regexp.MustCompile(`data-key="true"><strong><span class="chip"[^>]*>replicationFeedSlaves`).MatchString(html) ||
		!regexp.MustCompile(`class="key-symbol" data-alias=""><span class="chip"[^>]*>syncWithMaster`).MatchString(html) {
		t.Fatalf("every declaration is not listed, or a key is not marked as the tiles mark it:\n%s", html)
	}
}

// "possible" was painted in the warning red and read as an error.
func TestPossibleIsMutedNotAWarning(t *testing.T) {
	raw, err := reportTemplateFS.ReadFile("templates/css/30-components.css")
	if err != nil {
		t.Fatal(err)
	}
	rule := regexp.MustCompile(`(?m)^\.possible\{([^}]*)\}`).FindStringSubmatch(string(raw))
	if rule == nil || !strings.Contains(rule[1], "color:var(--muted)") || strings.Contains(rule[1], "--warn") {
		t.Fatalf("possible is not in the muted text colour: %v", rule)
	}
}

// Code in this part lists every declaration. A bordered box a declaration
// put Client connections' 31 declarations some 1,700 pixels long above the
// part's connections; each is one line, as the part's tiles draw it.
func TestCodeInThisPartIsALineADeclaration(t *testing.T) {
	raw, err := reportTemplateFS.ReadFile("templates/css/43-map-reading.css")
	if err != nil {
		t.Fatal(err)
	}
	rule := regexp.MustCompile(`(?m)^\.map-all-members \.map-member\{([^}]*)\}`).FindStringSubmatch(string(raw))
	if rule == nil || !strings.Contains(rule[1], "border:0") || !strings.Contains(rule[1], "min-height:0") {
		t.Fatalf("a declaration in the part's code list is drawn as a box: %v", rule)
	}
}

// The reading column's height comes from the canvas's workspace. A rule
// that let it grow outside the canvas made the static map a canvas failure
// leaves behind carry a reading 6,500 px tall with no scrolling box.
func TestReadingColumnHeightComesOnlyFromTheCanvas(t *testing.T) {
	files, err := reportTemplateFS.ReadDir("templates/css")
	if err != nil {
		t.Fatal(err)
	}
	rule := regexp.MustCompile(`([^{}]+)\{([^}]*)\}`)
	for _, file := range files {
		raw, err := reportTemplateFS.ReadFile("templates/css/" + file.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range rule.FindAllStringSubmatch(string(raw), -1) {
			selector, body := match[1], match[2]
			if strings.Contains(selector, "map-reading-column") && strings.Contains(selector, "map-inspector") &&
				strings.Contains(body, "height") && !strings.Contains(selector, ".flow-enabled") {
				t.Errorf("%s sizes the reading column outside the canvas: %s{%s}", file.Name(), strings.TrimSpace(selector), body)
			}
		}
	}
}
