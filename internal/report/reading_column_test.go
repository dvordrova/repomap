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
	builder.indexes = []groupindex.Index{server}
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

// An input's reading carries the Inputs blue its tile and collection are
// drawn in: the owner saw an input opened in purple, which reads as core.
// Its heading bar, its kind and its links take the blue; no rule of it
// takes a purple.
func TestAnInputsReadingIsInTheInputsBlue(t *testing.T) {
	raw, err := reportTemplateFS.ReadFile("templates/css/43-map-reading.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	for _, want := range []string{`.map-reading-input .map-object-heading{box-shadow:inset 3px 0 #356faa`, `.map-reading-input .map-object-heading .map-card-kind{color:#356faa}`,
		`.flow-enabled .map-reading-input .map-card a,.flow-enabled .map-reading-input .system-path-part{color:#204a7b}`} {
		if !strings.Contains(css, want) {
			t.Fatalf("the input's reading lost its blue: %s", want)
		}
	}
	for _, rule := range regexp.MustCompile(`[^}]*\.map-reading-input[^{]*\{[^}]*\}`).FindAllString(css, -1) {
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
