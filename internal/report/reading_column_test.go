package report

import (
	"regexp"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

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
	// A callable written inline is named as GroupsIndex names it, spaces
	// and all: casdoor's "StartLdapServer (inline, 3)".
	builder.subjects[subjectKey("server", "closure")] = subjectRef{subject: groupindex.Subject{ID: "closure", Object: &groupindex.ObjectFacts{Name: "StartLdapServer$3",
		Inline: "StartLdapServer (inline, 3)", Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "ldap/server.go", Line: 50, Column: 13}}}}
	server.Connections = append(server.Connections, groupindex.Connection{ID: "x4", From: here, To: groupindex.Endpoint{TargetID: "server", GroupID: "lists"},
		Label: "StartLdapServer (inline, 3) calls initServer", SourceKind: "native_calls", FromSubjectID: "closure", ToSubjectID: "init",
		FromLocation: &programindex.Location{Path: "ldap/server.go", Line: 57, Column: 3}})
	builder.indexes = []groupindex.Index{server}
	// The arrow's card reads a call as caller, relation and callee, each a
	// field of its own, the relation's underscores read as spaces, and links
	// the two names; a longer sentence showed neither name, and cmdTable's
	// callbacks read "Generic key commands redis.c:709". The joint names
	// both functions there too, where "integrates with" named none. A name
	// with spaces is no sentence: the card had split the call's one label
	// at its spaces and lost casdoor's LDAP call (review, 2026-10-03).
	for connection, want := range map[int][3]string{0: {"initServer", "passes callback", "acceptHandler"}, 1: {"anetTcpGenericConnect", "connects to", "anetAccept"},
		2: {"redis.c", "includes", "adlist.h"}, 3: {"StartLdapServer (inline, 3)", "calls", "initServer"}} {
		call := builder.connectionCall(server.Connections[connection])
		if call == nil {
			t.Fatalf("connection %d has no call on its arrow", connection)
		}
		if call.Label != "" || call.CallerName != want[0] || strings.ReplaceAll(call.Kind, "_", " ") != want[1] || call.CalleeName != want[2] {
			t.Fatalf("the arrow's card cannot read %+v as %v", call, want)
		}
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
	// A declaration without a link is keyed by its place, as the script keys it.
	if got := declarationKey(&pageAnchor{Path: `a "b".c`, Line: 7, NoSource: true}); got != `["a \"b\".c",7]` {
		t.Fatalf("a declaration without a link is keyed %s", got)
	}
}

// A name in a connection of the reading column opened GitHub: the arrow's
// call carried only where it is written and where it lands, and redis-cli's
// joint lands at anet.c:256, inside anetAccept, not at its declaration. The
// call names the declarations at both of its ends, keyed as the reading
// keys a declaration, so the name reads that declaration in the report.
func TestAnArrowsCallNamesTheDeclarationsAtItsEnds(t *testing.T) {
	links := pageLinks{repositoryURL: "https://github.com/redis/redis", blobPrefix: "/blob/", revision: "abc"}
	builder := pageBuilder{subjects: map[string]subjectRef{}, links: links, groupTitles: map[groupindex.Endpoint]string{},
		byProgram: map[string]*pageSection{"server": {ID: "server", Language: "c"}, "cli": {ID: "cli", Language: "c"}}}
	for _, item := range []struct {
		target, id, name string
		line             int
	}{{"cli", "connect", "anetTcpGenericConnect", 128}, {"server", "accept", "anetAccept", 248}} {
		builder.subjects[subjectKey(item.target, item.id)] = subjectRef{subject: groupindex.Subject{ID: item.id, Object: &groupindex.ObjectFacts{Name: item.name, Kind: programindex.ObjectFunction,
			Location: &programindex.Location{Path: "anet.c", Line: item.line, Column: 1}}}}
	}
	joint := groupindex.Connection{ID: "x", From: groupindex.Endpoint{TargetID: "cli", GroupID: "sockets"}, To: groupindex.Endpoint{TargetID: "server", GroupID: "networking"},
		Label: "integrates with", SourceKind: "integration", FromSubjectID: "connect", ToSubjectID: "accept",
		FromLocation: &programindex.Location{Path: "anet.c", Line: 158, Column: 1}, ToLocation: &programindex.Location{Path: "anet.c", Line: 256, Column: 1}}
	call := builder.connectionCall(joint)
	if call == nil {
		t.Fatal("the joint has no call on its arrow")
	}
	want := func(line int) string { return links.anchor("anet.c", line, 1).Href }
	if call.Caller != want(128) || call.Callee != want(248) || call.From != want(158) || call.To != want(256) {
		t.Fatalf("the call does not name its declarations apart from where it is written and lands: %+v", call)
	}
	raw := pageMapEdge{Calls: []pageEdgeCall{*call}}.CallsJSON()
	if !strings.Contains(raw, `"caller":"`+want(128)+`"`) || !strings.Contains(raw, `"callee":"`+want(248)+`"`) {
		t.Fatalf("the page's script cannot read the declarations: %s", raw)
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

// An input's reading is marked as one and drawn in the Inputs colour, never
// in core's purple, which reads as core.
func TestAnInputsReadingIsInTheInputsBlue(t *testing.T) {
	raw, err := reportTemplateFS.ReadFile("templates/css/43-map-reading.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	rules := regexp.MustCompile(`[^}]*\.map-reading-input[^{]*\{[^}]*\}`).FindAllString(css, -1)
	if len(rules) == 0 {
		t.Fatal("an input's reading has no colour of its own")
	}
	for _, rule := range rules {
		for _, purple := range []string{"#63429d", "#4f3aa3", "#4f2f86", "#755299", "#7252b3"} {
			if strings.Contains(rule, purple) {
				t.Fatalf("an input's reading is drawn in core's purple: %s", rule)
			}
		}
	}
	script, err := reportTemplateFS.ReadFile("templates/js/29-operation-view.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), `classList.toggle('map-reading-input',!!n.dataset.activation)`) {
		t.Fatal("the reading of an input is not marked as one")
	}
}

// Every fold of the reading column shows its disclosure marker (reviewer,
// 2026-09-30): "Called from" and "Calls into", their summaries laid out as
// flex, had lost the ▶ and read as empty headings between two rules, while
// "Inputs reaching this part" beside them showed it, and a caller's
// dispatch line had its marker pushed outside, out of sight. A summary the
// column's styles lay out as flex or grid, strip of its list marker or push
// it outside draws a marker of its own (::before or ::after), unless it is
// a row that opens by its own twist or link: a flow's call and a
// connection's line.
func TestEveryColumnFoldShowsItsMarker(t *testing.T) {
	ownTwist := map[string]bool{".map-frame-connections>details>summary": true, ".map-flow-row>summary": true}
	rule := regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)
	stripped := regexp.MustCompile(`display:\s*(flex|grid|inline-flex|block)|list-style:\s*none|list-style-position:\s*outside`)
	for _, name := range []string{"templates/css/38-reading.css", "templates/css/43-map-reading.css"} {
		raw, err := reportTemplateFS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		css := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(string(raw), "")
		marked := map[string]bool{}
		for _, match := range rule.FindAllStringSubmatch(css, -1) {
			for _, selector := range strings.Split(match[1], ",") {
				selector = strings.TrimSpace(selector)
				for _, pseudo := range []string{"::before", "::after"} {
					if base, found := strings.CutSuffix(selector, pseudo); found && strings.Contains(match[2], "content") {
						marked[strings.ReplaceAll(base, "[open]", "")] = true
					}
				}
			}
		}
		for _, match := range rule.FindAllStringSubmatch(css, -1) {
			if !stripped.MatchString(match[2]) {
				continue
			}
			for _, selector := range strings.Split(match[1], ",") {
				selector = strings.TrimSpace(selector)
				if !strings.HasSuffix(selector, "summary") || strings.Contains(selector, "target-picker") || ownTwist[selector] || marked[selector] {
					continue
				}
				t.Errorf("%s: %q is laid out without its marker (%s)", name, selector, strings.TrimSpace(match[2]))
			}
		}
	}
}
