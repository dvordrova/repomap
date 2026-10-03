package report

import (
	"encoding/json"
	"fmt"
	"maps"
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
		// The declaration as the page names it, its identity with its link.
		_, anchors[id] = b.subjectDisplay(subject)
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
// whatever its case, those "Called from" reaches first and counted (owner,
// 2026-09-29), the keys marked; a type keeps every field and its type,
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
		kinds = append(kinds, fmt.Sprintf("%s %d: %s", members.Kind, members.Outside, strings.Join(names(members.Decls), " ")))
	}
	if want := []string{"function 4: beforeSleep daemonize initServer serverCron appendServerSaveParams tryResizeHashTables Zfree", "type 0: redisClient", "variable 0: server"}; !slices.Equal(kinds, want) {
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
// the part at the other end, its own part first; the variables it reads are
// not among what it calls.
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
}

// Who changes a field and who reads it (owner, 2026-09-29): a record
// type's reading lists each field it declares with the functions writing
// and reading it by part, own part first, by every path the code reaches
// it by; a global variable's lists each field as the code reaches it
// through the variable, written by a function that reads the variable
// itself; a function's names the fields it writes, in the order it first
// writes them, each with the type declaring it. No site is kept.
func TestFieldsListTheirWritersAndReadersByPart(t *testing.T) {
	b, index, part, anchors := readingFixture(t)
	add := func(id, name string, kind programindex.ObjectKind, line int, owner string) {
		b.subjects[subjectKey("t1", id)] = subjectRef{subject: groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: name, Kind: kind, OwnerID: owner,
			Location: &programindex.Location{Path: "server.go", Line: line, Column: 1}}}}
		anchors[id] = b.links.anchorPointer("server.go", line, 1)
	}
	add("srv", "redisServer", programindex.ObjectType, 500, "module")
	add("f-dirty", "dirty", programindex.ObjectVariable, 501, "srv")
	add("f-hz", "hz", programindex.ObjectVariable, 502, "srv")
	edge := func(from, to, kind, path string, line int) groupindex.StructuralEdge {
		return groupindex.StructuralEdge{FromSubjectID: from, ToSubjectID: to, Role: groupindex.EdgeRelationTarget, RelationKind: programindex.RelationKind(kind),
			Resolution: programindex.ResolutionExact, FieldPath: path, Location: &programindex.Location{Path: "server.go", Line: line, Column: 5}}
	}
	index.StructuralEdges = []groupindex.StructuralEdge{
		edge("cron", "state", "reads", "", 21),
		edge("cron", "f-hz", "writes", "server.hz", 22),
		edge("cron", "f-dirty", "writes", "server.dirty", 23),
		edge("cron", "f-dirty", "writes", "server.dirty", 24),
		edge("cron", "f-fd", "writes", "redisClient.fd", 25),
		edge("init", "f-fd", "reads", "redisClient.fd", 31),
		edge("events", "f-fd", "reads", "redisClient.fd", 301),
		edge("events", "state", "reads", "", 302),
		edge("events", "f-dirty", "reads", "server.dirty", 303),
		// daemonize reaches a dirty of another server: it never reads ours.
		edge("daemonize", "f-dirty", "reads", "server.dirty", 211),
	}
	b.indexes[0] = index
	reading := decodeReading(t, b.groupReading(index, part, pageGroup{ID: "t1-g14", Title: part.Title, InternalConnections: []pageConnection{
		readingRow(anchors, "", "", "", "writes", "cron", "serverCron", "f-fd", "fd"),
		readingRow(anchors, "", "", "", "reads", "cron", "serverCron", "state", "server"),
	}}))
	at := func(name string) int {
		return slices.IndexFunc(reading.Decls, func(decl pageReadingDecl) bool { return decl.Name == name })
	}
	own := func(name string) pageReadingOwner {
		found := slices.IndexFunc(reading.Own, func(owner pageReadingOwner) bool { return owner.Decl == at(name) })
		if found < 0 {
			t.Fatalf("%s has no reading of its own", name)
		}
		return reading.Own[found]
	}
	said := func(rows []pageReadingFieldUse) []string {
		var result []string
		for _, row := range rows {
			for _, side := range []struct {
				word   string
				groups []pageReadingNames
			}{{"written", row.Written}, {"read", row.Read}} {
				for _, group := range side.groups {
					var names []string
					for _, decl := range group.Decls {
						names = append(names, reading.Decls[decl].Name)
					}
					result = append(result, row.Name+" "+side.word+" "+group.Part+" "+strings.Join(names, ","))
				}
			}
		}
		return result
	}
	if got, want := said(own("redisClient").Fields), []string{"fd written #t1-g14 serverCron", "fd read #t1-g14 initServer", "fd read #t1-g6 processTimeEvents"}; !slices.Equal(got, want) {
		t.Fatalf("redisClient's fields: %q, want %q", got, want)
	}
	if got, want := said(own("server").Fields), []string{"server.dirty written #t1-g14 serverCron", "server.dirty read #t1-g6 processTimeEvents", "server.hz written #t1-g14 serverCron"}; !slices.Equal(got, want) {
		t.Fatalf("server's fields: %q, want %q", got, want)
	}
	var writes []string
	for _, write := range own("serverCron").Writes {
		if write.Decl == nil {
			t.Fatalf("%s names no type", write.Path)
		}
		writes = append(writes, write.Path+" "+reading.Decls[*write.Decl].Name)
	}
	if want := []string{"server.hz redisServer", "server.dirty redisServer", "redisClient.fd redisClient"}; !slices.Equal(writes, want) {
		t.Fatalf("serverCron writes %q, want %q", writes, want)
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
// in the canvas's order of kinds, each list by name; a component is named without the kind
// its label adds when no other component shares the name.
func TestInputCollectionListsItsInputsByKindAndName(t *testing.T) {
	nodes := map[string]pageMapNode{
		"o1": {FullTitle: "set", Activation: "request", Catalogue: "cmd"},
		"o2": {FullTitle: "Append", Activation: "request", Catalogue: "cmd"},
		"o3": {FullTitle: "-p", Activation: "command", Catalogue: "flags"},
		"o4": {FullTitle: "port", Activation: "setting"},
		"o5": {FullTitle: "bind", Activation: "setting"},
		"o6": {FullTitle: "click", Activation: "interaction"},
		"o7": {FullTitle: "loop", Activation: "continuous"},
		"o8": {FullTitle: "cron", Activation: "scheduled"},
	}
	var collection pageInputCollection
	if err := json.Unmarshal([]byte(inputCollection([]string{"o6", "o3", "o7", "o1", "o4", "o8", "o2", "o5"}, func(id string) pageMapNode { return nodes[id] })), &collection); err != nil {
		t.Fatal(err)
	}
	var groups, kinds []string
	for _, group := range collection.Groups {
		groups = append(groups, group.Kind+" "+group.Catalogue+" "+strings.Join(group.Inputs, ","))
	}
	for _, kind := range collection.Kinds {
		kinds = append(kinds, kind.Kind+" "+strings.Join(kind.Inputs, ","))
	}
	// The kinds in the canvas's order (owner, 2026-10-01): scheduled
	// before continuous before interaction.
	if want := []string{"request o1 o2,o1", "command o3 o3", "setting  o5,o4", "scheduled  o8", "continuous  o7", "interaction  o6"}; !slices.Equal(groups, want) {
		t.Fatalf("groups %q, want %q", groups, want)
	}
	if want := []string{"request o2,o1", "command o3", "setting o5,o4", "scheduled o8", "continuous o7", "interaction o6"}; !slices.Equal(kinds, want) {
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

// The files a program is built from are a build fact (owner, 2026-09-28): a
// model's summary had said all four Redis programs share ae, sds, adlist,
// dict and anet, and redis-check-dump links none of them. For C they are
// the units its link line names; for another language the files its
// declarations are written in, tests left out.
func TestAProgramIsBuiltFromItsLinkedUnits(t *testing.T) {
	builder := &pageBuilder{data: &ReportData{ProgramPortfolio: &ProgramPortfolio{Entries: []programindex.Index{
		{Target: programindex.Target{ID: "t3", Language: "c", Sources: []programindex.TargetSource{{Path: "redis-check-dump.c"}, {Path: "Makefile"}, {Path: "lzf_d.c"}, {Path: "lzf_c.c"}}}},
		{Target: programindex.Target{ID: "t5", Language: "go", TestSources: []string{"app/app_test.go"}}, Objects: []programindex.Object{
			{ID: "n1", Location: &programindex.Location{Path: "app/main.go", Line: 1}}, {ID: "n2", Location: &programindex.Location{Path: "app/app_test.go", Line: 1}}, {ID: "n3", Location: &programindex.Location{Path: "app/main.go", Line: 9}}}},
	}}}}
	if got := strings.Join(builder.builtFrom("t3"), " "); got != "lzf_c.c lzf_d.c redis-check-dump.c" {
		t.Fatalf("C: %s", got)
	}
	if got := strings.Join(builder.builtFrom("t5"), " "); got != "app/main.go" {
		t.Fatalf("Go: %s", got)
	}
}

// A part of files in subdirectories names each member's file by the path
// its file list uses, so the column finds them file by file (litestream's
// cmd/litestream/main.go read as an empty part while the file was said
// as main.go); fields a struct declares on one line keep their own names.
func TestAPartsMembersKeepTheirFilesPathAndTheirOwnNames(t *testing.T) {
	b, index, part, anchors := readingFixture(t)
	move := func(id, path string, line int) {
		ref := b.subjects[subjectKey("t1", id)]
		object := *ref.subject.Object
		object.Location = &programindex.Location{Path: path, Line: line, Column: 1}
		ref.subject.Object = &object
		b.subjects[subjectKey("t1", id)] = ref
		_, anchors[id] = b.subjectDisplay(ref.subject)
	}
	move("sleep", "cmd/app/main.go", 40)
	move("alpha", "cmd/tool/main.go", 50)
	move("f-argc", "server.go", 97)
	move("f-mbargc", "server.go", 97)
	reading := decodeReading(t, b.groupReading(index, part, pageGroup{ID: "t1-g14", Title: part.Title, Connections: []pageConnection{
		readingRow(anchors, "→", "#t1-g4", "Core data structures", "reads", "cron", "serverCron", "f-argc", "argc"),
		readingRow(anchors, "→", "#t1-g4", "Core data structures", "reads", "cron", "serverCron", "f-mbargc", "mbargc"),
	}}))
	for _, want := range []string{"cmd/app/main.go", "cmd/tool/main.go"} {
		if !slices.Contains(reading.Files, want) {
			t.Fatalf("files %q lack %s", reading.Files, want)
		}
	}
	members := map[string]bool{}
	for _, kind := range reading.Members {
		for _, position := range kind.Decls {
			decl := reading.Decls[position]
			if !slices.Contains(reading.Files, decl.File) {
				t.Fatalf("%s is in %q, which the part's files %q do not list", decl.Name, decl.File, reading.Files)
			}
			members[decl.Name] = true
		}
	}
	if !members["beforeSleep"] || !members["appendServerSaveParams"] {
		t.Fatalf("members %v lack the subdirectory files' declarations", members)
	}
	var fields []string
	for _, decl := range reading.Decls {
		if decl.Kind == "field" {
			fields = append(fields, decl.Name)
		}
	}
	slices.Sort(fields)
	// Each named with its type, as a reading lists a field.
	if !slices.Contains(fields, "redisClient.argc") || !slices.Contains(fields, "redisClient.mbargc") {
		t.Fatalf("fields on one line read as %q, want argc and mbargc each", fields)
	}
}

// Inputs of one kind a program names alike read apart by the words saved
// beside each name (page_apart.go), the name kept: each takes the first
// kind of word that tells them apart and that it has (the subcommands it is
// an option of, its catalogue's declaration, its key, a word its handler
// declares, the function registering it); those still alike add the next
// word that differs. An input no other shares its name with carries none,
// and the collection carries what each list item shows.
func TestSameNamedInputsReadApartByTheirSavedWords(t *testing.T) {
	handler := func(word, at string) []pageApartWord { return []pageApartWord{{Word: word, Of: apartHandler, At: at}} }
	registered := func(name string) pageApartWord { return pageApartWord{Word: name, Of: apartRegistered, At: "gw.go:1"} }
	nodes := []*pageMapNode{
		// etcd's Election and lock APIs: a route registered by the server's
		// and the client's functions, one registration whose handler declares
		// no word.
		{ID: "campaign", FullTitle: "POST", Activation: "request", apart: pageApartFacts{handler: handler("/v3electionpb.Election/Campaign", "gw.go:182"), registered: registered("RegisterElectionHandlerServer")}},
		{ID: "campaign-client", FullTitle: "POST", Activation: "request", apart: pageApartFacts{handler: handler("/v3electionpb.Election/Campaign", "gw.go:307"), registered: registered("RegisterElectionHandlerClient")}},
		{ID: "observe", FullTitle: "POST", Activation: "request", apart: pageApartFacts{registered: registered("RegisterElectionHandlerServer")}},
		{ID: "lock", FullTitle: "POST", Activation: "request", apart: pageApartFacts{handler: handler("/v3lockpb.Lock/Lock", "lock.go:90"), registered: registered("RegisterLockHandlerServer")}},
		// freqtrade: two options of one name, of two subcommands; two table
		// rows of one name, by their keys; a name no other shares.
		{ID: "download-data", FullTitle: "download-data", Activation: "command", apart: pageApartFacts{options: []string{"erase-1"}}},
		{ID: "install-ui", FullTitle: "install-ui", Activation: "command", apart: pageApartFacts{options: []string{"erase-2"}}},
		{ID: "erase-1", FullTitle: "--erase", Activation: "command", HandlerUnknown: true},
		{ID: "erase-2", FullTitle: "--erase", Activation: "command", HandlerUnknown: true},
		{ID: "version", FullTitle: "-V --version", Activation: "command", Key: "version"},
		{ID: "version-main", FullTitle: "-V --version", Activation: "command", Key: "version_main"},
		{ID: "trade", FullTitle: "trade", Activation: "command", Key: "start_trading"},
	}
	tellInputsApart(nodes)
	said := map[string]string{}
	byID := map[string]pageMapNode{}
	var ids []string
	for _, node := range nodes {
		var words []string
		for _, word := range node.apartWords {
			words = append(words, word.Word)
		}
		said[node.ID] = strings.Join(words, " · ")
		byID[node.ID], ids = *node, append(ids, node.ID)
	}
	want := map[string]string{
		"campaign":        "/v3electionpb.Election/Campaign · RegisterElectionHandlerServer",
		"campaign-client": "/v3electionpb.Election/Campaign · RegisterElectionHandlerClient",
		"observe":         "RegisterElectionHandlerServer",
		"lock":            "/v3lockpb.Lock/Lock",
		"download-data":   "", "install-ui": "",
		"erase-1": "download-data", "erase-2": "install-ui",
		"version": "version", "version-main": "version_main",
		"trade": "",
	}
	if !maps.Equal(said, want) {
		t.Fatalf("words beside the names %v, want %v", said, want)
	}
	if words := byID["campaign"].apartWords; words[0].Of != apartHandler || words[0].At != "gw.go:182" || words[1].Of != apartRegistered {
		t.Fatalf("the words lose what they are and where they are written: %+v", words)
	}
	var collection pageInputCollection
	if err := json.Unmarshal([]byte(inputCollection(ids, func(id string) pageMapNode { return byID[id] })), &collection); err != nil {
		t.Fatal(err)
	}
	if len(collection.Apart) != 8 || collection.Apart["lock"][0].Word != "/v3lockpb.Lock/Lock" || collection.Apart["trade"] != nil {
		t.Fatalf("the collection carries %v", collection.Apart)
	}
}

// A reading names two declarations it lists alike by where they stand, so
// no list of it says one name for two things (reviewer, 2026-10-02):
// headscale's Policy engine's two PolicyManager types, beets's Item.path of
// the library and of a test. A name no other shares, and two alike in one
// file, which nothing where they stand tells apart, keep theirs.
func TestAReadingTellsItsSameNamedDeclarationsApart(t *testing.T) {
	decls := []pageReadingDecl{
		{Name: "PolicyManager", File: "hscontrol/policy/pm.go"},
		{Name: "PolicyManager", File: "hscontrol/policy/v2/policy.go"},
		{Name: "Item.path", File: "beets/library/models.py"},
		{Name: "Item.path", File: "test/plugins/test_fromfilename.py"},
		{Name: "BeatportClient.search", File: "beetsplug/beatport.py"},
		{Name: "BeatportClient.search", File: "beetsplug/beatport.py"},
		{Name: "NewState", File: "hscontrol/state/state.go"},
	}
	tellDeclsApart(decls)
	var got []string
	for _, decl := range decls {
		got = append(got, decl.Name)
	}
	want := []string{"policy.PolicyManager", "v2.PolicyManager", "library.Item.path", "plugins.Item.path", "BeatportClient.search", "BeatportClient.search", "NewState"}
	if !slices.Equal(got, want) {
		t.Fatalf("names = %q, want %q", got, want)
	}
}

// In an Inputs collection a kind's inputs running a handler come before
// those only declaring a value, catalogued or not (reviewer, 2026-10-02:
// freqtrade's trade, backtesting and webserver had stood after every option
// of AVAILABLE_CLI_OPTIONS); a catalogue of handled inputs keeps its place
// before the inputs no catalogue holds, and the full catalogue stays.
func TestAnInputCollectionListsWhatRunsBeforeWhatOnlyDeclaresAValue(t *testing.T) {
	nodes := map[string]pageMapNode{
		"o1": {FullTitle: "--allow-limit-orders", Activation: "command", Catalogue: "options", HandlerUnknown: true},
		"o2": {FullTitle: "-V --version", Activation: "command", Catalogue: "options", HandlerUnknown: true},
		"o3": {FullTitle: "trade", Activation: "command"},
		"o4": {FullTitle: "backtesting", Activation: "command"},
		"o5": {FullTitle: "--config", Activation: "command", HandlerUnknown: true},
		"o6": {FullTitle: "get", Activation: "request", Catalogue: "cmdTable"},
		"o7": {FullTitle: "ping", Activation: "request"},
	}
	var collection pageInputCollection
	if err := json.Unmarshal([]byte(inputCollection([]string{"o1", "o2", "o3", "o4", "o5", "o6", "o7"}, func(id string) pageMapNode { return nodes[id] })), &collection); err != nil {
		t.Fatal(err)
	}
	var groups []string
	for _, group := range collection.Groups {
		groups = append(groups, group.Kind+" "+group.Catalogue+" "+strings.Join(group.Inputs, ","))
	}
	if want := []string{"request o6 o6", "request  o7", "command  o4,o3", "command o1 o1,o2", "command  o5"}; !slices.Equal(groups, want) {
		t.Fatalf("groups %q, want %q", groups, want)
	}
}

// A page with no remote link, a source unavailable at the captured
// revision, or a served path with no openable ID keeps every declaration's
// reading: its place, plain text with "No source", keys it, so two
// functions of one name in two files stay two, each with its caller
// (control review, 2026-10-02: a render of a run with no remote had lost
// every function's reading and Code search).
func TestDeclarationsWithoutASourceLinkKeepTheirReadings(t *testing.T) {
	for name, data := range map[string]*ReportData{
		"no remote": {},
		"unavailable at the captured revision": {GitHubSourceLinks: &GitHubSourceLinks{RepositoryURL: "https://github.com/etcd-io/etcd", Revision: "abc"},
			UnavailableSourcePaths: []string{"election.go", "lock.go", "main.go"}},
		"served with no openable ID": {SourceIDs: map[string]string{"server.go": "s1"}},
	} {
		t.Run(name, func(t *testing.T) {
			b := &pageBuilder{subjects: map[string]subjectRef{}, links: newPageLinks(data), byProgram: map[string]*pageSection{"t1": {ID: "t1"}},
				groupTitles: map[groupindex.Endpoint]string{}}
			anchors := map[string]*pageAnchor{}
			object := func(id, name, path string) {
				b.subjects[subjectKey("t1", id)] = subjectRef{subject: groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: name,
					Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: path, Line: 10, Column: 1}}}}
				anchors[id] = b.links.anchorPointer(path, 10, 1)
			}
			object("election", "Campaign", "election.go")
			object("lock", "Campaign", "lock.go")
			object("main", "main", "main.go")
			part := groupindex.Group{ID: "g1", Title: "Election and lock APIs", MemberSubjectIDs: []string{"election", "lock"}}
			index := groupindex.Index{Target: programindex.Target{ID: "t1"}, Groups: []groupindex.Group{part, {ID: "g2", Title: "main", MemberSubjectIDs: []string{"main"}}}}
			b.indexes = []groupindex.Index{index}
			for _, group := range index.Groups {
				b.groupTitles[groupindex.Endpoint{TargetID: "t1", GroupID: group.ID}] = group.Title
			}
			card := pageGroup{ID: "t1-g1", Title: part.Title, Connections: []pageConnection{
				readingRow(anchors, "←", "#t1-g2", "main", "calls", "main", "main", "election", "Campaign"),
				readingRow(anchors, "←", "#t1-g2", "main", "calls", "main", "main", "lock", "Campaign"),
			}}
			reading := decodeReading(t, b.groupReading(index, part, card))
			keys := map[string]string{}
			for _, decl := range reading.Decls {
				if decl.Key == "" || !decl.NoSource || decl.Href != "" || decl.Open != "" || decl.At != decl.File+":10" {
					t.Fatalf("%s is not read by its place with No source: %+v", decl.Name, decl)
				}
				keys[decl.Key] = decl.File
			}
			if len(reading.Members) != 1 || len(reading.Members[0].Decls) != 2 || len(keys) != len(reading.Decls) {
				t.Fatalf("members %+v, decls %+v: both functions named Campaign, each by its own place", reading.Members, reading.Decls)
			}
			if len(reading.In) != 1 || len(reading.In[0].Lines) != 1 || len(reading.In[0].Lines[0].Ends) != 2 {
				t.Fatalf("callers %+v: main calls both", reading.In)
			}
		})
	}
}

// A declaration's name decides nothing about where it is read: JavaScript's
// public `price$`, `active$` and `_token` stand on tiles and among the
// part's members, while a callable written inline, which GroupsIndex says
// is one (ObjectFacts.Anonymous), stands on neither, whatever its name
// (external review, 2026-10-03: the report had hidden every name with "$").
func TestNamesWithDollarsOrUnderscoresAreTilesAndMembers(t *testing.T) {
	b := &pageBuilder{subjects: map[string]subjectRef{}, links: pageLinks{repositoryURL: "https://github.com/o/r", blobPrefix: "/blob/", revision: "abc"},
		byProgram: map[string]*pageSection{"t1": {ID: "t1"}}, groupTitles: map[groupindex.Endpoint]string{}}
	object := func(id, name string, kind programindex.ObjectKind, line int, anonymous bool) {
		b.subjects[subjectKey("t1", id)] = subjectRef{subject: groupindex.Subject{ID: id, Object: &groupindex.ObjectFacts{Name: name, Kind: kind, OwnerID: "module",
			Visibility: programindex.VisibilityPublic, Anonymous: anonymous, Location: &programindex.Location{Path: "src/price.js", Line: line, Column: 17}}}}
	}
	object("module", "src/price.js", programindex.ObjectModule, 1, false)
	object("dollar", "price$", programindex.ObjectFunction, 1, false)
	object("plain", "price", programindex.ObjectFunction, 2, false)
	object("underscore", "_token", programindex.ObjectVariable, 3, false)
	object("active", "active$", programindex.ObjectFunction, 5, false)
	object("closure", "active$$1", programindex.ObjectFunction, 6, true)
	part := groupindex.Group{ID: "g1", Title: "Prices", MemberSubjectIDs: []string{"dollar", "plain", "underscore", "active", "closure"}}
	index := groupindex.Index{Target: programindex.Target{ID: "t1"}, Groups: []groupindex.Group{part}}
	b.indexes = []groupindex.Index{index}
	raw, _ := b.groupSymbols("t1", part)
	var symbols []pageNodeSymbol
	if err := json.Unmarshal([]byte(raw), &symbols); err != nil {
		t.Fatal(err)
	}
	var tiles []string
	for _, symbol := range symbols {
		tiles = append(tiles, symbol.Name)
	}
	slices.Sort(tiles)
	if want := []string{"_token", "active$", "price", "price$"}; !slices.Equal(tiles, want) {
		t.Fatalf("tiles %q, want %q", tiles, want)
	}
	reading := decodeReading(t, b.groupReading(index, part, pageGroup{ID: "t1-g1", Title: part.Title}))
	var members []string
	for _, kind := range reading.Members {
		for _, position := range kind.Decls {
			members = append(members, kind.Kind+" "+reading.Decls[position].Name)
		}
	}
	slices.Sort(members)
	if want := []string{"function active$", "function price", "function price$", "variable _token"}; !slices.Equal(members, want) {
		t.Fatalf("members %q, want %q", members, want)
	}
}
