package report

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// readingFixture is Redis's Server lifecycle part in miniature: its
// functions (one a key, one named in lower case), a type with more fields
// than a tile shows, a module's variable, the part main calling into it and
// the part it calls.
func readingFixture(t *testing.T) (*pageBuilder, groupindex.Index, groupindex.Group, map[string]*pageAnchor) {
	t.Helper()
	b := &pageBuilder{subjects: map[string]subjectRef{}, links: pageLinks{repositoryURL: "https://github.com/redis/redis", blobPrefix: "/blob/", revision: "abc"},
		byProgram: map[string]*pageSection{"t1": {ID: "t1"}}, groupTitles: map[groupindex.Endpoint]string{},
		docstrings: map[string][]claims.Claim{"server.go": {{Line: 19, Text: "serverCron runs every 100 ms."}}}}
	anchors := map[string]*pageAnchor{}
	object := func(id, name string, kind programindex.ObjectKind, line int, owner, signature string, key bool) {
		subject := groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: name, Kind: kind, OwnerID: owner, Signature: signature,
			Location: &programindex.Location{Path: "server.go", Line: line, Column: 1}}}
		if key {
			subject.Interpretation = &groupindex.Interpretation{Key: true}
		}
		b.subjects[subjectKey("t1", id)] = subjectRef{subject: subject}
		anchors[id] = b.links.anchorPointer("server.go", line, 1)
	}
	object("module", "server", programindex.ObjectModule, 1, "", "", false)
	object("cron", "serverCron", programindex.ObjectFunction, 20, "module", "", false)
	object("init", "initServer", programindex.ObjectFunction, 30, "module", "", true)
	object("sleep", "beforeSleep", programindex.ObjectFunction, 40, "module", "", false)
	object("alpha", "appendServerSaveParams", programindex.ObjectFunction, 50, "module", "", false)
	object("zeta", "Zfree", programindex.ObjectFunction, 60, "module", "", false)
	object("resize", "tryResizeHashTables", programindex.ObjectFunction, 70, "module", "", false)
	object("state", "server", programindex.ObjectVariable, 80, "module", "server struct redisServer", false)
	object("client", "redisClient", programindex.ObjectType, 90, "module", "", false)
	var members = []string{"cron", "init", "sleep", "alpha", "zeta", "resize", "state", "client"}
	for i, field := range []string{"fd", "db", "dictid", "querybuf", "argv", "mbargv", "argc", "mbargc", "bulklen", "multibulk"} {
		object("f-"+field, field, programindex.ObjectVariable, 91+i, "client", field+" int", false)
		members = append(members, "f-"+field)
	}
	object("main", "main", programindex.ObjectFunction, 200, "module", "", false)
	object("daemonize", "daemonize", programindex.ObjectFunction, 210, "module", "", false)
	object("events", "processTimeEvents", programindex.ObjectFunction, 300, "module", "", false)
	object("dict", "dictResize", programindex.ObjectFunction, 400, "module", "", false)
	members = append(members, "daemonize")
	part := groupindex.Group{ID: "g14", Title: "Server lifecycle and cron", MemberSubjectIDs: members}
	index := groupindex.Index{Target: programindex.Target{ID: "t1"}, Groups: []groupindex.Group{part,
		{ID: "g21", Title: "main", MemberSubjectIDs: []string{"main"}},
		{ID: "g6", Title: "Event loop and networking", MemberSubjectIDs: []string{"events"}},
		{ID: "g4", Title: "Core data structures", MemberSubjectIDs: []string{"dict"}}}}
	b.indexes = []groupindex.Index{index}
	for _, group := range index.Groups {
		b.groupTitles[groupindex.Endpoint{TargetID: "t1", GroupID: group.ID}] = group.Title
	}
	return b, index, part, anchors
}

func readingRow(anchors map[string]*pageAnchor, arrow, peer, title, kind, from, fromName, to, toName string) pageConnection {
	return pageConnection{Native: true, Arrow: arrow, Href: peer, Title: title, Kind: kind, FromName: fromName, ToName: toName,
		FromDecl: anchors[from], ToDecl: anchors[to], FromSource: anchors[from],
		fromTarget: "t1", toTarget: "t1", fromSubject: from, toSubject: to}
}

func decodeReading(t *testing.T, raw string) pageGroupReading {
	t.Helper()
	var reading pageGroupReading
	if err := json.Unmarshal([]byte(raw), &reading); err != nil {
		t.Fatalf("reading %q: %v", raw, err)
	}
	return reading
}

// A part's reading lists its declarations by kind, each list by name
// whatever its case, the keys marked; a type keeps every field and its type,
// bulklen past the tile's "… +2" included; the part's callers are one line
// per caller with what it calls here, calls before callbacks; every input
// registered at the part is one neighbour counted by its inputs.
func TestPartReadingListsMembersByNameAndCallersByCaller(t *testing.T) {
	b, index, part, anchors := readingFixture(t)
	card := pageGroup{ID: "t1-g14", Title: part.Title, Connections: []pageConnection{
		readingRow(anchors, "←", "#t1-g21", "main", "calls", "main", "main", "init", "initServer"),
		readingRow(anchors, "←", "#t1-g21", "main", "passes_callback", "main", "main", "sleep", "beforeSleep"),
		readingRow(anchors, "←", "#t1-g21", "main", "calls", "main", "main", "daemonize", "daemonize"),
		readingRow(anchors, "←", "#t1-o1", "get", "calls", "state", "server", "cron", "serverCron"),
		readingRow(anchors, "←", "#t1-o2", "set", "calls", "state", "server", "cron", "serverCron"),
		readingRow(anchors, "→", "#t1-g4", "Core data structures", "calls", "resize", "tryResizeHashTables", "dict", "dictResize"),
	}}
	card.Connections[3].input, card.Connections[4].input = true, true
	reading := decodeReading(t, b.groupReading(index, part, card))
	names := func(positions []int) []string {
		var result []string
		for _, position := range positions {
			result = append(result, reading.Decls[position].Name)
		}
		return result
	}
	var kinds []string
	for _, members := range reading.Members {
		kinds = append(kinds, members.Kind+": "+strings.Join(names(members.Decls), " "))
	}
	if want := []string{"function: appendServerSaveParams beforeSleep daemonize initServer serverCron tryResizeHashTables Zfree", "type: redisClient", "variable: server"}; !slices.Equal(kinds, want) {
		t.Fatalf("members %q, want %q", kinds, want)
	}
	for _, decl := range reading.Decls {
		if decl.Bold != (decl.Name == "initServer") {
			t.Fatalf("%s bold = %v: only the model's key is bold", decl.Name, decl.Bold)
		}
		if decl.Name == "serverCron" && decl.Doc != "serverCron runs every 100 ms." {
			t.Fatalf("serverCron's author comment = %q", decl.Doc)
		}
		if decl.Name == "redisClient" {
			var fields []string
			for _, field := range decl.Fields {
				fields = append(fields, field.Name+" "+field.Type)
			}
			if len(fields) != 10 || fields[8] != "bulklen int" {
				t.Fatalf("redisClient's fields %q: every field keeps its type", fields)
			}
		}
	}
	if len(reading.In) != 2 || reading.In[0].Title != "main" || reading.In[0].Count != 3 || !reading.In[1].Inputs || reading.In[1].Count != 2 {
		t.Fatalf("callers %+v: main by its three pairs, then the two inputs as one neighbour", reading.In)
	}
	var tree []string
	for _, line := range reading.In[0].Lines {
		for _, end := range line.Ends {
			tree = append(tree, reading.Decls[line.Caller].Name+" → "+reading.Decls[end.Decl].Name+" "+end.Kind)
		}
	}
	if want := []string{"main → daemonize calls", "main → initServer calls", "main → beforeSleep passes_callback"}; !slices.Equal(tree, want) {
		t.Fatalf("main's line %q, want %q", tree, want)
	}
	if len(reading.In[1].Lines) != 1 || reading.Decls[reading.In[1].Lines[0].Caller].Part != "#t1-g14" {
		t.Fatalf("the inputs' line %+v: the table row is in the part it is declared in, not in an input", reading.In[1].Lines)
	}
	if len(reading.Out) != 1 || reading.Out[0].Part != "#t1-g4" || reading.Decls[reading.Out[0].Lines[0].Ends[0].Decl].Name != "dictResize" {
		t.Fatalf("callees %+v", reading.Out)
	}
}

// A declaration's reading names who calls it and what it calls, grouped by
// the part at the other end, its own part first, and the variables it uses
// apart from what it calls.
func TestDeclarationReadingGroupsItsRelationsByPartOwnPartFirst(t *testing.T) {
	b, index, part, anchors := readingFixture(t)
	card := pageGroup{ID: "t1-g14", Title: part.Title, Connections: []pageConnection{
		readingRow(anchors, "←", "#t1-g6", "Event loop and networking", "calls", "events", "processTimeEvents", "cron", "serverCron"),
		readingRow(anchors, "→", "#t1-g4", "Core data structures", "calls", "cron", "serverCron", "dict", "dictResize"),
	}, InternalConnections: []pageConnection{
		readingRow(anchors, "", "", "", "passes_callback", "init", "initServer", "cron", "serverCron"),
		readingRow(anchors, "", "", "", "calls", "cron", "serverCron", "resize", "tryResizeHashTables"),
		readingRow(anchors, "", "", "", "reads", "cron", "serverCron", "state", "server"),
	}}
	reading := decodeReading(t, b.groupReading(index, part, card))
	cron := slices.IndexFunc(reading.Decls, func(decl pageReadingDecl) bool { return decl.Name == "serverCron" })
	own := reading.Own[slices.IndexFunc(reading.Own, func(owner pageReadingOwner) bool { return owner.Decl == cron })]
	said := func(groups []pageReadingPeerDecls) []string {
		var result []string
		for _, group := range groups {
			for _, end := range group.Decls {
				result = append(result, group.Title+": "+reading.Decls[end.Decl].Name+" "+end.Kind)
			}
		}
		return result
	}
	if got, want := said(own.Callers), []string{"Server lifecycle and cron: initServer passes_callback", "Event loop and networking: processTimeEvents calls"}; !slices.Equal(got, want) {
		t.Fatalf("callers %q, want %q", got, want)
	}
	if got, want := said(own.Callees), []string{"Server lifecycle and cron: tryResizeHashTables calls", "Core data structures: dictResize calls"}; !slices.Equal(got, want) {
		t.Fatalf("callees %q, want %q", got, want)
	}
	if len(own.Uses) != 1 || reading.Decls[own.Uses[0].Decl].Name != "server" || reading.Decls[own.Uses[0].Decl].Part != "#t1-g14" {
		t.Fatalf("uses %+v: the variable, with the part holding it", own.Uses)
	}
}

// A type with more fields than its tile shows keeps them all in the page
// data: the tile's "… +N" row is added after the list and takes no field's
// place. redisClient's ninth field, bulklen, had been that row.
func TestATilesMoreRowTakesNoFieldsPlace(t *testing.T) {
	b, _, part, _ := readingFixture(t)
	raw, _ := b.groupSymbols("t1", part)
	var symbols []pageNodeSymbol
	if err := json.Unmarshal([]byte(raw), &symbols); err != nil {
		t.Fatal(err)
	}
	fields, more := 0, 0
	for _, symbol := range symbols {
		switch {
		case symbol.Name == "bulklen" && symbol.Text != ": int":
			t.Fatalf("bulklen = %+v: it keeps its type", symbol)
		case symbol.Owner != 0 && (symbol.Kind == "field" || symbol.Kind == "skip"):
			fields++
		case symbol.Kind == "more":
			more++
			if symbol.Name != "… +2" || symbols[len(symbols)-1] != symbol {
				t.Fatalf("the more row %+v stands last and counts the two fields past the first eight", symbol)
			}
		}
	}
	if fields != 10 || more != 1 {
		t.Fatalf("%d fields and %d more rows, want 10 and 1", fields, more)
	}
}

// An Inputs collection reads its catalogues and its other inputs by kind,
// requests first, each list by name; a component is named without the kind
// its label adds when no other component shares the name.
func TestInputCollectionListsItsInputsByKindAndName(t *testing.T) {
	nodes := map[string]pageMapNode{
		"o1": {FullTitle: "set", Activation: "request", Catalogue: "cmd"},
		"o2": {FullTitle: "Append", Activation: "request", Catalogue: "cmd"},
		"o3": {FullTitle: "-p", Activation: "command", Catalogue: "flags"},
		"o4": {FullTitle: "port", Activation: "setting"},
		"o5": {FullTitle: "bind", Activation: "setting"},
	}
	var collection pageInputCollection
	if err := json.Unmarshal([]byte(inputCollection([]string{"o3", "o1", "o4", "o2", "o5"}, func(id string) pageMapNode { return nodes[id] })), &collection); err != nil {
		t.Fatal(err)
	}
	var groups, kinds []string
	for _, group := range collection.Groups {
		groups = append(groups, group.Kind+" "+group.Catalogue+" "+strings.Join(group.Inputs, ","))
	}
	for _, kind := range collection.Kinds {
		kinds = append(kinds, kind.Kind+" "+strings.Join(kind.Inputs, ","))
	}
	if want := []string{"request o1 o2,o1", "command o3 o3", "setting  o5,o4"}; !slices.Equal(groups, want) {
		t.Fatalf("groups %q, want %q", groups, want)
	}
	if want := []string{"request o2,o1", "command o3", "setting o5,o4"}; !slices.Equal(kinds, want) {
		t.Fatalf("kinds %q, want %q", kinds, want)
	}
	server := &pageSection{ShortLabel: "redis-server (executable)", Kind: "executable"}
	cli := &pageSection{ShortLabel: "redis-cli (executable)", Kind: "executable"}
	library := &pageSection{ShortLabel: "app (library)", Kind: "library"}
	program := &pageSection{ShortLabel: "app (executable)", Kind: "executable"}
	sections := []*pageSection{server, cli, library, program}
	for section, want := range map[*pageSection]string{server: "redis-server", cli: "redis-cli", library: "app (library)", program: "app (executable)"} {
		if got := componentTitle(section, sections); got != want {
			t.Fatalf("%q is named %q, want %q", section.ShortLabel, got, want)
		}
	}
}
