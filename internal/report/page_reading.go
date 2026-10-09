package report

import (
	"cmp"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// pageGroupReading is what the reading column shows of a part and of each
// declaration in it, prepared and sorted here: the page's script renders it
// in this order and sorts nothing (owner, 2026-09-28). Every declaration it
// names stands once in Decls; the rest refer to it by position there.
//
//   - Members are the part's declarations by kind, functions, types, then
//     variables, each list by name whatever its case; a key is bold.
//   - In are the parts calling into this one, each with its callers by name
//     and what each calls here: "main() → daemonize() initServer() …
//     beforeSleep() passed as a callback".
//   - Out are the parts this one calls into, each with its callees by
//     relation (calls, callbacks, other relations, then the variables it
//     uses) and by name.
//   - Own is, for each declaration of the part, who calls it and what it
//     calls, grouped by the part at the other end (its own part first),
//     each by its name alone: no place a relation is written (owner,
//     2026-09-29: "человек будет видеть код"; the name reads its code). A
//     type also names the functions of its part that return or take it. A
//     record type and a global variable list their fields with who writes
//     and reads each, and a declaration the fields it writes and the fields
//     and global variables it reads (page_field_uses.go).
//
// It is read from the part's own relation rows (Connections and
// InternalConnections) and its members: a relation the part does not list
// is not here. A model's sentence with no two declarations named is not a
// relation between names and stays on the part's card and the arrow's.
type pageGroupReading struct {
	Decls []pageReadingDecl `json:"decls"`
	// Files are the files its members are written in, by path.
	Files   []string           `json:"files,omitempty"`
	Members []pageReadingKind  `json:"members,omitempty"`
	In      []pageReadingPeer  `json:"in,omitempty"`
	Out     []pageReadingPeer  `json:"out,omitempty"`
	Own     []pageReadingOwner `json:"own,omitempty"`
}

// pageReadingDecl is one declaration as the reading names it: its code
// name, the link into its code, the file it is written in and, for a
// member, whether the model chose it as a key, what its author wrote above
// it and a type's fields.
type pageReadingDecl struct {
	Name     string `json:"name"`
	Key      string `json:"key"`
	Href     string `json:"href,omitempty"`
	Open     string `json:"open,omitempty"`
	NoSource bool   `json:"no_source,omitempty"`
	// Code is the link to all of its lines, where Href names the first.
	Code string `json:"code,omitempty"`
	// At is where it is declared, "redis.c:1155", said on hover; File is
	// its file by path, as Files lists it, so a part of files in
	// subdirectories (cmd/litestream/main.go) reads its members file by file.
	At   string `json:"at,omitempty"`
	File string `json:"file,omitempty"`
	// Kind is function, type or variable (a method is a function, a
	// type's field a field); empty when the declaration is none of them.
	Kind string `json:"kind,omitempty"`
	// Anonymous marks a callable written inline, named in words ("anonymous
	// function in Start"): a tree writes no "()" after it.
	Anonymous bool `json:"anonymous,omitempty"`
	// Part is the link to the part it stands in, "#…": the reading reads
	// it there and names that part on hover.
	Part string `json:"part,omitempty"`
	Bold bool   `json:"bold,omitempty"`
	// Doc is the comment its author wrote above it, quoted as written: an
	// author's claim, never the model's.
	Doc    string             `json:"doc,omitempty"`
	Fields []pageReadingField `json:"fields,omitempty"`
	// Apart is, for a declaration the reading names as another, nothing
	// where it stands telling them apart, the first thing only it of them
	// calls or reads (tellDeclsApartByUse): casdoor's two
	// LoginPage.login.loginHandler, one calling URL, the other goToLink.
	Apart []pageApartWord `json:"apart,omitempty"`
}

// MarshalJSON leaves the key out when it is the declaration's link, as it
// is on a static page; the script takes the link for it.
func (decl pageReadingDecl) MarshalJSON() ([]byte, error) {
	type plain pageReadingDecl
	written := struct {
		plain
		Key *string `json:"key,omitempty"`
	}{plain: plain(decl)}
	if decl.Key != decl.Href {
		written.Key = &decl.Key
	}
	return json.Marshal(written)
}

// pageReadingField is one field of a type with the type it holds: every
// field, where a tile shows the first few.
type pageReadingField struct {
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Href     string `json:"href,omitempty"`
	Open     string `json:"open,omitempty"`
	NoSource bool   `json:"no_source,omitempty"`
	At       string `json:"at,omitempty"`
	// TypeDecl is the repository type its declared type names, in the
	// reading's declarations (`listNode *head` names listNode).
	TypeDecl *int `json:"type_decl,omitempty"`
}

// pageReadingKind is the part's declarations of one kind, by name, the
// Outside first of them those reached from outside the part: a caller in
// another part, an input registered at it, a callable handed over (its
// "Called from").
type pageReadingKind struct {
	Kind    string `json:"kind"`
	Decls   []int  `json:"decls"`
	Outside int    `json:"outside,omitempty"`
}

// pageReadingPeer is one part at the other end of this part's relations:
// Count is its caller → callee pairs, the number the arrow's card counts.
type pageReadingPeer struct {
	Part  string `json:"part"`
	Title string `json:"title"`
	// Inputs marks the one neighbour standing for every input registered
	// at the part; Count is then its inputs.
	Inputs bool              `json:"inputs,omitempty"`
	Count  int               `json:"count"`
	Lines  []pageReadingLine `json:"lines"`
}

// readingInputs keys the inputs' neighbour among a part's neighbours.
const readingInputs = "\x00inputs"

// pageReadingLine is one caller and what it reaches over a relation's
// kind: an incoming peer's line is a caller of that part with the
// declarations of this one it reaches; an outgoing peer's line has no
// caller and lists the callees of one relation kind.
type pageReadingLine struct {
	Caller int              `json:"caller"`
	Ends   []pageReadingEnd `json:"ends"`
	// Fan marks a caller reaching many declarations of this part through
	// one dispatch site: the line reads as one ("loadAppendOnlyFile() → 17
	// request handlers, possible, via cmdTable"), its ends folded under it.
	Fan *pageReadingFan `json:"fan,omitempty"`
}

// pageReadingFan is a caller's dispatch into a part: Of is how many
// declarations its site can call in all, Noun the kind of input each end
// handles when they all handle one kind ("request"), and Via the
// declarations that hand the whole set over (cmdTable), by position.
type pageReadingFan struct {
	Of   int    `json:"of"`
	Noun string `json:"noun,omitempty"`
	Via  []int  `json:"via,omitempty"`
}

// readingFanOut is how many ends of one caller's dispatch make its line one.
const readingFanOut = 5

// pageReadingEnd is a declaration at a relation's other end: its kind of
// relation and whether it is only possible. Where the relation is written
// is not kept: a caller calling from several places is one name.
type pageReadingEnd struct {
	Decl     int    `json:"decl"`
	Kind     string `json:"kind"`
	Possible bool   `json:"possible,omitempty"`
	// Site is, on a "Called by" end of a declaration's own reading, where
	// the caller makes the call: the first place in source order, its
	// link, and every place in its words ("redis.c:2221 · 2240"), which the
	// name's code link will open (owner, 2026-09-29: queueMultiCommand's
	// caller had opened processCommand at its top, 70 lines above the
	// call). The page prints no code mark; no other end keeps a place.
	Site  *pageReadingSite `json:"site,omitempty"`
	sites []pageAnchor
}

// pageReadingSite is one place a call is written: its words, said on a
// name's hover ("called at redis.c:1273 · 1288"), and on a caller the link
// to that line.
type pageReadingSite struct {
	At   string `json:"at"`
	Href string `json:"href,omitempty"`
	Open string `json:"open,omitempty"`
}

// callSite is a caller's call site: the first of its places in source
// order, and every place in its words, the file once while it stays the
// same.
func callSite(sites []pageAnchor) *pageReadingSite {
	if len(sites) == 0 {
		return nil
	}
	sites = slices.Clone(sites)
	slices.SortFunc(sites, func(a, b pageAnchor) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), strings.Compare(a.Text, b.Text))
	})
	var words []string
	for i, site := range sites {
		if i > 0 && site.Path != "" && site.Path == sites[i-1].Path && site.Line > 0 {
			words = append(words, strconv.Itoa(site.Line))
			continue
		}
		words = append(words, site.Text)
	}
	return &pageReadingSite{At: strings.Join(words, " · "), Href: sites[0].Href, Open: sites[0].Open}
}

// pageReadingOwner is one declaration of the part and its relations.
type pageReadingOwner struct {
	Decl    int                    `json:"decl"`
	Callers []pageReadingPeerDecls `json:"callers,omitempty"`
	// NotCalledIn names this program when its adapter proved it never runs
	// the declaration (owner, 2026-09-28: "not called in redis-benchmark"),
	// while other programs of the report call it.
	NotCalledIn string `json:"not_called_in,omitempty"`
	// Callees are what it calls and relates to other than the variables
	// it reads and writes, which Reads and Writes say.
	Callees []pageReadingPeerDecls `json:"callees,omitempty"`
	// Flow is what a function calls, in the order its calls are written
	// (page_flow.go); Cases, for a function handling inputs a case of its
	// comparison declares, what each case's lines call, by the case's first
	// line (an input's "Its calls:").
	Flow    []pageFlowCall    `json:"flow,omitempty"`
	Cases   []pageReadingCase `json:"cases,omitempty"`
	Returns []int             `json:"returns,omitempty"`
	Takes   []int             `json:"takes,omitempty"`
	// Fields are a record type's fields, or a global variable's fields as
	// the code reaches them through it, each with its writers and readers;
	// Writes the fields a function writes, and Reads the fields and global
	// variables it reads, each once.
	Fields []pageReadingFieldUse `json:"fields,omitempty"`
	Writes []pageReadingPath     `json:"writes,omitempty"`
	Reads  []pageReadingPath     `json:"reads,omitempty"`
}

// pageReadingCase is what one case's lines of a function call.
type pageReadingCase struct {
	Line int            `json:"line"`
	Flow []pageFlowCall `json:"flow"`
}

// pageReadingPeerDecls is one part's declarations at the other end of a
// declaration's relations, by name.
type pageReadingPeerDecls struct {
	Part  string           `json:"part"`
	Title string           `json:"title"`
	Own   bool             `json:"own,omitempty"`
	Decls []pageReadingEnd `json:"decls"`
	// Program names another program of the report whose code makes these
	// calls into a declaration both hold (page_shared_code.go).
	Program string `json:"program,omitempty"`
}

// relationOrder is where a relation's ends stand in a list: calls first,
// then callbacks, then the other relations, and the variables used last.
func relationOrder(kind string) int {
	switch kind {
	case string(programindex.RelationCalls), string(programindex.RelationInvokesExternal):
		return 0
	case string(programindex.RelationPassesCallback):
		return 1
	case string(programindex.RelationReads), string(programindex.RelationWrites):
		return 3
	}
	return 2
}

// usesVariable says whether a relation's other end is a variable the
// declaration uses rather than something it calls.
func usesVariable(kind string) bool { return relationOrder(kind) == 3 }

// groupReading prepares the part's reading from its card's members and
// relation rows; "" when the part lists nothing.
func (builder *pageBuilder) groupReading(index groupindex.Index, group groupindex.Group, card pageGroup) string {
	targetID, own := index.Target.ID, "#"+card.ID
	reading := pageGroupReading{}
	at := map[string]int{}
	name := func(position int) string { return reading.Decls[position].Name }
	byName := func(a, b int) int {
		return cmp.Or(strings.Compare(strings.ToLower(name(a)), strings.ToLower(name(b))), strings.Compare(name(a), name(b)),
			strings.Compare(reading.Decls[a].Key, reading.Decls[b].Key))
	}
	kindOf := func(target, id string) string {
		ref, known := builder.subject(target, id)
		if !known || ref.subject.Object == nil {
			return ""
		}
		object := ref.subject.Object
		switch object.Kind {
		case programindex.ObjectFunction, programindex.ObjectMethod:
			return "function"
		case programindex.ObjectType:
			return "type"
		case programindex.ObjectVariable:
			if owner, known := builder.subject(target, object.OwnerID); known && owner.subject.Object != nil && owner.subject.Object.Kind == programindex.ObjectType {
				return "field"
			}
			return "variable"
		}
		return ""
	}
	// A method or a field is named with its type wherever a reading lists
	// it (reviewer, 2026-09-30: litestream's lists had read "OpenLTXFile()"
	// twice and Main.Run's calls "Run()" twelve times, freqtrade's
	// "ANALYZED_DF" twice, one per enum): "ReplicateCommand.Run",
	// "RPCMessageType.ANALYZED_DF".
	qualified := func(target, id, label string) string {
		ref, known := builder.subject(target, id)
		if !known || ref.subject.Object == nil || ref.subject.Object.OwnerID == "" {
			return label
		}
		object := ref.subject.Object
		if object.Kind != programindex.ObjectMethod && object.Kind != programindex.ObjectVariable {
			return label
		}
		owner, known := builder.subject(target, object.OwnerID)
		if !known || owner.subject.Object == nil || owner.subject.Object.Kind != programindex.ObjectType || strings.HasPrefix(label, owner.subject.Object.Name+".") {
			return label
		}
		return owner.subject.Object.Name + "." + label
	}
	// declare names a declaration once, by the key the page's script keys
	// it by, its identity (declarationKey): fields a struct declares on one
	// line (shared.cone, shared.czero) and two functions written on one line
	// stay two. A later mention fills what an earlier one did not know.
	declare := func(decl pageReadingDecl) int {
		if decl.Key == "" {
			return -1
		}
		if position, known := at[decl.Key]; known {
			listed := &reading.Decls[position]
			listed.Kind = cmp.Or(listed.Kind, decl.Kind)
			listed.Part = cmp.Or(listed.Part, decl.Part)
			return position
		}
		at[decl.Key] = len(reading.Decls)
		reading.Decls = append(reading.Decls, decl)
		return len(reading.Decls) - 1
	}
	fromAnchor := func(label string, anchor *pageAnchor, kind, part string) int {
		if anchor == nil {
			return -1
		}
		return declare(pageReadingDecl{Name: label, Key: declarationKey(anchor), Anonymous: anchor.words, Href: anchor.Href, Open: anchor.Open, NoSource: anchor.NoSource, Code: anchor.Code,
			At: anchor.Text, File: anchor.Path, Kind: kind, Part: part})
	}
	// subjectOf is, by declaration, the program and subject it is, where
	// the reading knows them (tellDeclsApartByUse).
	subjectOf := map[int][2]string{}
	knownAs := func(position int, programTargetID, subjectID string) int {
		if position >= 0 && subjectID != "" {
			subjectOf[position] = [2]string{programTargetID, subjectID}
		}
		return position
	}

	// The members: the declarations a reader looks for, as the part's tiles
	// draw them, each type with every one of its fields.
	lists := map[string][]int{}
	types := map[string]int{}
	functions := map[string]int{}
	variables := map[string]int{}
	fieldsOf := map[string][]string{}
	var fields []string
	for _, id := range group.MemberSubjectIDs {
		ref, known := builder.subject(targetID, id)
		if !known || ref.subject.Object == nil {
			continue
		}
		label, anchor := builder.subjectDisplay(ref.subject)
		if label == "" || anchor == nil || builder.inline(ref.subject) {
			continue
		}
		kind := kindOf(targetID, id)
		if kind == "field" {
			fields = append(fields, id)
			continue
		}
		if kind == "variable" && !builder.moduleVariable(targetID, ref.subject.Object) {
			continue
		}
		object := ref.subject.Object
		if object.Kind == programindex.ObjectMethod {
			if owner, known := builder.subject(targetID, object.OwnerID); known && owner.subject.Object != nil &&
				owner.subject.Object.Kind == programindex.ObjectType && !strings.HasPrefix(label, owner.subject.Object.Name+".") {
				label = owner.subject.Object.Name + "." + label
			}
		}
		if kind == "" {
			continue
		}
		// A declaration with no link and no place the page can key it by
		// is not one the script can read.
		position := knownAs(fromAnchor(label, anchor, kind, own), targetID, id)
		if position < 0 {
			continue
		}
		decl := &reading.Decls[position]
		decl.Bold = ref.subject.Interpretation != nil && ref.subject.Interpretation.Key
		decl.Doc = builder.docstringFor(anchor.Path, anchor.Line, anchor.Column)
		if !slices.Contains(lists[kind], position) {
			lists[kind] = append(lists[kind], position)
		}
		if anchor.Path != "" && !slices.Contains(reading.Files, anchor.Path) {
			reading.Files = append(reading.Files, anchor.Path)
		}
		if kind == "type" {
			types[id] = position
		}
		if kind == "function" {
			functions[id] = position
		}
		if kind == "variable" {
			variables[id] = position
		}
	}
	type typedField struct {
		owner, field int
		id           string
	}
	var typedFields []typedField
	for _, id := range fields {
		ref, _ := builder.subject(targetID, id)
		owner, inPart := types[ref.subject.Object.OwnerID]
		if !inPart {
			continue
		}
		label, anchor := builder.subjectDisplay(ref.subject)
		field := pageReadingField{Name: label, Type: strings.TrimSpace(strings.TrimPrefix(symbolText(ref.subject.Object, label), ":")),
			Href: anchor.Href, Open: anchor.Open, NoSource: anchor.NoSource, At: anchor.Text}
		reading.Decls[owner].Fields = append(reading.Decls[owner].Fields, field)
		fieldsOf[ref.subject.Object.OwnerID] = append(fieldsOf[ref.subject.Object.OwnerID], id)
		typedFields = append(typedFields, typedField{owner: owner, field: len(reading.Decls[owner].Fields) - 1, id: id})
	}
	slices.Sort(reading.Files)
	for _, kind := range []string{"function", "type", "variable"} {
		if members := lists[kind]; len(members) > 0 {
			slices.SortFunc(members, byName)
			reading.Members = append(reading.Members, pageReadingKind{Kind: kind, Decls: members})
		}
	}

	// The relations, each row once per end it has in this part.
	type side struct {
		part, title string
		ends        map[int][]pageReadingEnd // keyed by this part's declaration
	}
	type pairKey struct{ from, to int }
	incoming, outgoing := map[string]*side{}, map[string]*side{}
	var incomingOrder, outgoingOrder []string
	callers, callees := map[int]map[string]*side{}, map[int]map[string]*side{}
	pairs := map[string]map[pairKey]bool{}
	addEnd := func(sides map[int]map[string]*side, decl int, part, title string, end pageReadingEnd) {
		if sides[decl] == nil {
			sides[decl] = map[string]*side{}
		}
		peer := sides[decl][part]
		if peer == nil {
			peer = &side{part: part, title: title, ends: map[int][]pageReadingEnd{}}
			sides[decl][part] = peer
		}
		peer.ends[decl] = mergeEnd(peer.ends[decl], end)
	}
	// Each end stands in the part its declaration is a member of (the
	// last, as the page's relation lists count it), whatever the row's
	// other end: a table row of an input names the table, not the input.
	partOf := func(target, subject string) (string, string) {
		other, section := builder.graphIndex(target), builder.byProgram[target]
		if other == nil || section == nil {
			return "", ""
		}
		group := builder.edgesBetweenGroups(*other).groupOf[subject]
		if group == "" {
			return "", ""
		}
		return "#" + groupAnchorID(section.ID, group), builder.groupTitles[groupindex.Endpoint{TargetID: target, GroupID: group}]
	}
	inputs := map[string]bool{}
	// The dispatch set each incoming caller reaches this part through.
	folds := map[int]*dispatchFold{}
	mixed := map[int]bool{}
	rows := make([]pageConnection, 0, len(card.Connections)+len(card.InternalConnections))
	rows = append(append(rows, card.Connections...), card.InternalConnections...)
	for _, row := range rows {
		if row.Phrase() == "" || row.FromDecl == nil || row.ToDecl == nil {
			continue
		}
		fromPart, fromTitle := partOf(row.fromTarget, row.fromSubject)
		toPart, toTitle := partOf(row.toTarget, row.toSubject)
		from := knownAs(fromAnchor(qualified(row.fromTarget, row.fromSubject, row.FromName), row.FromDecl, kindOf(row.fromTarget, row.fromSubject), fromPart), row.fromTarget, row.fromSubject)
		to := knownAs(fromAnchor(qualified(row.toTarget, row.toSubject, row.ToName), row.ToDecl, kindOf(row.toTarget, row.toSubject), toPart), row.toTarget, row.toSubject)
		if from < 0 || to < 0 {
			continue
		}
		kind := row.Kind
		if row.Arrow != "" {
			// Every input registered at this part is one neighbour, Inputs,
			// counted by its inputs: Server core state had 95 neighbours
			// named get, set, … each saying cmdTable calls redisCommand.
			peers, order, peer, title := outgoing, &outgoingOrder, row.Href, row.Title
			if row.Arrow == "←" {
				peers, order = incoming, &incomingOrder
			}
			if row.input {
				inputs[row.Href] = true
				peer, title = readingInputs, ""
			}
			if peers[peer] == nil {
				peers[peer] = &side{part: peer, title: title, ends: map[int][]pageReadingEnd{}}
				*order = append(*order, peer)
			}
			if pairs[row.Arrow+peer] == nil {
				pairs[row.Arrow+peer] = map[pairKey]bool{}
			}
			pairs[row.Arrow+peer][pairKey{from, to}] = true
			line := from
			if row.Arrow == "→" {
				line = -1
			} else if !row.input {
				relation, _, _ := strings.Cut(row.EvidenceID, "\x00")
				fold := builder.dispatch(row.fromTarget).site[relation]
				if known, seen := folds[line]; seen && known != fold || fold == nil {
					mixed[line] = true
				}
				folds[line] = fold
			}
			peers[peer].ends[line] = mergeEnd(peers[peer].ends[line], pageReadingEnd{Decl: to, Kind: kind, Possible: row.Possible})
		}
		// The declaration's own reading: the caller's callees, the callee's
		// callers, each by the part the other end stands in.
		if fromPart == own {
			addEnd(callees, from, cmp.Or(toPart, row.Href), cmp.Or(toTitle, row.Title), pageReadingEnd{Decl: to, Kind: kind, Possible: row.Possible})
		}
		if toPart == own {
			end := pageReadingEnd{Decl: from, Kind: kind, Possible: row.Possible}
			if row.FromSource != nil && !usesVariable(kind) {
				end.sites = []pageAnchor{*row.FromSource}
			}
			addEnd(callers, to, cmp.Or(fromPart, row.Href), cmp.Or(fromTitle, row.Title), end)
		}
	}
	sortEnds := func(ends []pageReadingEnd) []pageReadingEnd {
		slices.SortFunc(ends, func(a, b pageReadingEnd) int {
			return cmp.Or(cmp.Compare(relationOrder(a.Kind), relationOrder(b.Kind)), byName(a.Decl, b.Decl), strings.Compare(a.Kind, b.Kind))
		})
		return ends
	}
	peerLines := func(peers map[string]*side, order []string, arrow string) []pageReadingPeer {
		var result []pageReadingPeer
		for _, part := range order {
			peer := peers[part]
			item := pageReadingPeer{Part: part, Title: peer.title, Count: len(pairs[arrow+part])}
			if part == readingInputs {
				item.Part, item.Inputs, item.Count = "", true, len(inputs)
			}
			var lines []int
			for line := range peer.ends {
				lines = append(lines, line)
			}
			slices.SortFunc(lines, func(a, b int) int {
				if a < 0 || b < 0 {
					return cmp.Compare(a, b)
				}
				return byName(a, b)
			})
			for _, line := range lines {
				entry := pageReadingLine{Caller: line, Ends: sortEnds(peer.ends[line])}
				if fold := folds[line]; arrow == "←" && fold != nil && !mixed[line] && len(entry.Ends) >= readingFanOut {
					entry.Fan = builder.readingFan(targetID, fold, entry.Ends, reading.Decls, declare)
				}
				item.Lines = append(item.Lines, entry)
			}
			result = append(result, item)
		}
		// The heaviest neighbour first, as the arrows' cards count them.
		slices.SortStableFunc(result, func(a, b pageReadingPeer) int {
			return cmp.Or(cmp.Compare(b.Count, a.Count), strings.Compare(a.Title, b.Title))
		})
		return result
	}
	reading.In = peerLines(incoming, incomingOrder, "←")
	reading.Out = peerLines(outgoing, outgoingOrder, "→")
	// The declarations "Called from" reaches stand first in their kind's
	// list, each list still by name (owner, 2026-09-29: Persistence's 40
	// functions had not said which of them are its ways in).
	outside := map[int]bool{}
	for _, peer := range reading.In {
		for _, line := range peer.Lines {
			for _, end := range line.Ends {
				outside[end.Decl] = true
			}
		}
	}
	for i := range reading.Members {
		slices.SortStableFunc(reading.Members[i].Decls, func(a, b int) int { return boolFirst(outside[a], outside[b]) })
		for _, decl := range reading.Members[i].Decls {
			if outside[decl] {
				reading.Members[i].Outside++
			}
		}
	}

	// Each member's own reading. The variables a declaration reads and
	// writes are said by its Reads and Writes lines (page_field_uses.go),
	// not among what it calls.
	declGroups := func(sides map[string]*side, decl int, variables bool) []pageReadingPeerDecls {
		var groups []pageReadingPeerDecls
		for _, peer := range sides {
			item := pageReadingPeerDecls{Part: peer.part, Title: peer.title, Own: peer.part == own}
			for _, end := range peer.ends[decl] {
				if variables && usesVariable(end.Kind) {
					continue
				}
				end.Site = callSite(end.sites)
				item.Decls = append(item.Decls, end)
			}
			if len(item.Decls) > 0 {
				item.Decls = sortEnds(item.Decls)
				groups = append(groups, item)
			}
		}
		// The declaration's own part first, then the part with most names.
		slices.SortFunc(groups, func(a, b pageReadingPeerDecls) int {
			return cmp.Or(boolFirst(a.Own, b.Own), cmp.Compare(len(b.Decls), len(a.Decls)), strings.Compare(a.Title, b.Title), strings.Compare(a.Part, b.Part))
		})
		return groups
	}
	for _, members := range reading.Members {
		for _, decl := range members.Decls {
			owner := pageReadingOwner{Decl: decl}
			owner.Callers = declGroups(callers[decl], decl, false)
			owner.Callees = declGroups(callees[decl], decl, true)
			if len(owner.Callers)+len(owner.Callees) > 0 {
				reading.Own = append(reading.Own, owner)
			}
		}
	}
	// A type of the part is returned or taken by the part's functions.
	// Read each original member once; constructing its source link for every
	// type made this catalogue grow as types multiplied by all declarations.
	returnsByType, takesByType := map[string][]int{}, map[string][]int{}
	if len(types) > 0 {
		for _, member := range group.MemberSubjectIDs {
			ref, known := builder.subject(targetID, member)
			if !known || ref.subject.Object == nil {
				continue
			}
			label, anchor := builder.subjectDisplay(ref.subject)
			position, listed := at[declarationKey(anchor)]
			if !listed || label == "" || reading.Decls[position].Kind != "function" {
				continue
			}
			for _, result := range ref.subject.Object.Results {
				if _, known := types[result.TypeID]; known && !slices.Contains(returnsByType[result.TypeID], position) {
					returnsByType[result.TypeID] = append(returnsByType[result.TypeID], position)
				}
			}
			for _, parameter := range ref.subject.Object.Parameters {
				if _, known := types[parameter.TypeID]; known && !slices.Contains(takesByType[parameter.TypeID], position) {
					takesByType[parameter.TypeID] = append(takesByType[parameter.TypeID], position)
				}
			}
		}
	}
	for id, typePosition := range types {
		returns, takes := returnsByType[id], takesByType[id]
		if len(returns)+len(takes) == 0 {
			continue
		}
		slices.SortFunc(returns, byName)
		slices.SortFunc(takes, byName)
		found := slices.IndexFunc(reading.Own, func(owner pageReadingOwner) bool { return owner.Decl == typePosition })
		if found < 0 {
			reading.Own = append(reading.Own, pageReadingOwner{Decl: typePosition})
			found = len(reading.Own) - 1
		}
		reading.Own[found].Returns, reading.Own[found].Takes = returns, takes
	}
	// Calls into the part's functions from the other programs of the report
	// that hold them, each program's callers it runs, after its own; and a
	// function its own program never runs says so.
	ownerOf := func(position int) *pageReadingOwner {
		found := slices.IndexFunc(reading.Own, func(owner pageReadingOwner) bool { return owner.Decl == position })
		if found < 0 {
			reading.Own = append(reading.Own, pageReadingOwner{Decl: position})
			found = len(reading.Own) - 1
		}
		return &reading.Own[found]
	}
	ids := make([]string, 0, len(functions))
	for id := range functions {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b string) int { return cmp.Compare(functions[a], functions[b]) })
	for _, id := range ids {
		position := functions[id]
		elsewhere := builder.callersElsewhere(targetID, id)
		var groups []pageReadingPeerDecls
		for _, call := range elsewhere {
			ref, known := builder.subject(call.targetID, call.caller)
			if !known {
				continue
			}
			label, anchor := builder.subjectDisplay(ref.subject)
			part, title := partOf(call.targetID, call.caller)
			caller := knownAs(fromAnchor(qualified(call.targetID, call.caller, label), anchor, kindOf(call.targetID, call.caller), part), call.targetID, call.caller)
			if caller < 0 {
				continue
			}
			end := pageReadingEnd{Decl: caller, Kind: string(call.edge.RelationKind), Possible: call.edge.Resolution != programindex.ResolutionExact}
			program := componentTitle(builder.byProgram[call.targetID], builder.sections)
			at := slices.IndexFunc(groups, func(group pageReadingPeerDecls) bool { return group.Program == program && group.Part == part })
			if at < 0 {
				groups = append(groups, pageReadingPeerDecls{Part: part, Title: title, Program: program})
				at = len(groups) - 1
			}
			groups[at].Decls = mergeEnd(groups[at].Decls, end)
		}
		for i := range groups {
			groups[i].Decls = sortEnds(groups[i].Decls)
		}
		notCalled := len(groups) > 0 && builder.neverRun(targetID, id)
		if len(groups) == 0 && !notCalled {
			continue
		}
		owner := ownerOf(position)
		owner.Callers = append(owner.Callers, groups...)
		if notCalled {
			owner.NotCalledIn = componentTitle(builder.byProgram[targetID], builder.sections)
		}
	}
	// Each function's flow, and the flow of each declaration no part holds
	// that one of them calls (lookupKeyRead), so a call opens in place
	// wherever it goes and none is dropped.
	subjectAt := map[int]string{}
	declareSubject := func(id string) int {
		ref, known := builder.subject(targetID, id)
		if !known || ref.subject.Object == nil {
			return -1
		}
		label, anchor := builder.subjectDisplay(ref.subject)
		part, _ := partOf(targetID, id)
		position := knownAs(fromAnchor(qualified(targetID, id, label), anchor, kindOf(targetID, id), part), targetID, id)
		if position >= 0 {
			subjectAt[position] = id
		}
		return position
	}
	// A field's type links to that type when it is the repository's.
	for _, typed := range typedFields {
		for _, at := range builder.fieldTypes(targetID, typed.id) {
			if typeID := builder.subjectAt[subjectLocationKey(targetID, at.Path, at.Line)]; typeID != "" && kindOf(targetID, typeID) == "type" {
				if position := declareSubject(typeID); position >= 0 {
					reading.Decls[typed.owner].Fields[typed.field].TypeDecl = &position
					break
				}
			}
		}
	}
	queue := append([]string(nil), ids...)
	flowed := map[string]bool{}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if flowed[id] {
			continue
		}
		flowed[id] = true
		position := declareSubject(id)
		if position < 0 {
			continue
		}
		flow := builder.flowOf(&index, id, declareSubject, nil)
		if len(flow) == 0 {
			continue
		}
		ownerOf(position).Flow = flow
		for _, operation := range index.Operations {
			owner := ownerOf(position)
			if operation.SubjectID != id || operation.Branch == nil || slices.ContainsFunc(owner.Cases, func(other pageReadingCase) bool { return other.Line == operation.Branch.Line }) {
				continue
			}
			owner.Cases = append(owner.Cases, pageReadingCase{Line: operation.Branch.Line, Flow: builder.flowOf(&index, id, declareSubject, operation.Branch)})
		}
		for _, call := range flow {
			if call.Decl == nil {
				continue
			}
			if callee := subjectAt[*call.Decl]; callee != "" && reading.Decls[*call.Decl].Part == "" && kindOf(targetID, callee) == "function" {
				queue = append(queue, callee)
			}
		}
	}
	builder.fieldReadings(&index, own, &reading, types, variables, functions, fieldsOf, declareSubject, partOf, ownerOf, byName)
	slices.SortFunc(reading.Own, func(a, b pageReadingOwner) int { return cmp.Compare(a.Decl, b.Decl) })
	for i := range reading.Own {
		partOfDecl := func(position int) string { return reading.Decls[position].Part }
		reading.Own[i].Flow = groupFlowByPart(reading.Own[i].Flow, partOfDecl)
		for c := range reading.Own[i].Cases {
			reading.Own[i].Cases[c].Flow = groupFlowByPart(reading.Own[i].Cases[c].Flow, partOfDecl)
		}
	}
	if len(reading.Decls) == 0 {
		return ""
	}
	tellDeclsApart(reading.Decls)
	builder.tellDeclsApartByUse(reading.Decls, subjectOf)
	raw, err := json.Marshal(reading)
	if err != nil {
		return ""
	}
	return string(raw)
}

// tellDeclsApart names the declarations a reading names alike by where
// they stand (groupindex.TellApart), so no list of the reading says one
// name for two things: headscale's Policy engine holds policy's and v2's
// PolicyManager, policy.PolicyManager and v2.PolicyManager; beets's Item.path
// of the library and of a test's Item are library.Item.path and
// plugins.Item.path.
func tellDeclsApart(decls []pageReadingDecl) {
	names := make([]string, len(decls))
	spellings := make([][]string, len(decls))
	for position, decl := range decls {
		names[position], spellings[position] = decl.Name, groupindex.Where(decl.Name, decl.File, "")
	}
	for position, told := range groupindex.TellApart(names, spellings) {
		decls[position].Name = told
	}
}

// tellDeclsApartByUse gives the declarations a reading still names alike
// after where they stand (tellDeclsApart), each of one program, the first
// thing only it of them calls or reads, in its source order, a repository
// declaration before an outside one (groupindex.OwnUses, the rung callables
// written alike are named by): casdoor's two LoginPage.login.loginHandler,
// arrows in two `.then` callbacks of login, read "loginHandler URL" and
// "loginHandler goToLink", each word saying on its hover what it is and
// where it is written. Nothing tells apart one using only what the others
// use, and no line or number is said.
func (builder *pageBuilder) tellDeclsApartByUse(decls []pageReadingDecl, subjectOf map[int][2]string) {
	alike := map[string][]int{}
	var order []string
	for position, decl := range decls {
		if _, seen := alike[decl.Name]; !seen {
			order = append(order, decl.Name)
		}
		alike[decl.Name] = append(alike[decl.Name], position)
	}
	for _, name := range order {
		positions := alike[name]
		if len(positions) < 2 {
			continue
		}
		program, ids := "", make([]string, 0, len(positions))
		for _, position := range positions {
			subject, known := subjectOf[position]
			if !known || program != "" && subject[0] != program {
				ids = nil
				break
			}
			program = subject[0]
			ids = append(ids, subject[1])
		}
		if len(ids) < 2 {
			continue
		}
		builder.activateNativeScope(program)
		if builder.ownUseReader == nil {
			index, found := builder.programIndex(program)
			if !found {
				continue
			}
			builder.ownUseReader = groupindex.NewOwnUseReader(index)
		}
		for at, use := range builder.ownUseReader.OwnUses(ids) {
			if use.Word == "" {
				continue
			}
			of := apartCalls
			if use.Reads {
				of = apartReads
			}
			decls[positions[at]].Apart = []pageApartWord{{Word: use.Word, Of: of, At: placeText(use.At.Path, use.At.Line)}}
		}
	}
}

// programIndex is a program's saved ProgramIndex.
func (builder *pageBuilder) programIndex(programTargetID string) (programindex.Index, bool) {
	if builder.data == nil || builder.data.ProgramPortfolio == nil || programTargetID == "" {
		return programindex.Index{}, false
	}
	for _, entry := range builder.nativeTargets(programTargetID) {
		if entry.Target.ID == programTargetID {
			return entry, true
		}
	}
	return programindex.Index{}, false
}

// readingFan says a caller's dispatch into a part: how many its site can
// call, the kind of input they all handle, and what hands the set over.
func (builder *pageBuilder) readingFan(targetID string, fold *dispatchFold, ends []pageReadingEnd, decls []pageReadingDecl, declare func(pageReadingDecl) int) *pageReadingFan {
	fan := &pageReadingFan{Of: len(fold.members)}
	if index := builder.graphIndex(targetID); index != nil {
		kinds := map[string]string{}
		for _, operation := range index.Operations {
			if operation.SubjectID != "" && !operation.HandlerUnknown {
				kinds[declarationKeyOf(builder, targetID, operation.SubjectID)] = operation.Kind
			}
		}
		for i, end := range ends {
			kind := kinds[decls[end.Decl].Key]
			if kind == "" || i > 0 && kind != fan.Noun {
				fan.Noun = ""
				break
			}
			fan.Noun = kind
		}
	}
	for _, hander := range builder.dispatch(targetID).handers(fold) {
		ref, known := builder.subject(targetID, hander)
		if !known {
			continue
		}
		name, anchor := builder.subjectDisplay(ref.subject)
		if anchor == nil {
			continue
		}
		if position := declare(pageReadingDecl{Name: name, Key: declarationKey(anchor), Anonymous: anchor.words, Href: anchor.Href, Open: anchor.Open, NoSource: anchor.NoSource, Code: anchor.Code, At: anchor.Text, File: anchor.Path}); position >= 0 {
			fan.Via = append(fan.Via, position)
		}
	}
	return fan
}

func declarationKeyOf(builder *pageBuilder, targetID, subjectID string) string {
	ref, known := builder.subject(targetID, subjectID)
	if !known {
		return ""
	}
	_, anchor := builder.subjectDisplay(ref.subject)
	return declarationKey(anchor)
}

// mergeEnd adds an end to a declaration's list, one entry per declaration
// and relation kind, possible only when every relation it stands for is.
func mergeEnd(ends []pageReadingEnd, end pageReadingEnd) []pageReadingEnd {
	for i := range ends {
		if ends[i].Decl == end.Decl && ends[i].Kind == end.Kind {
			ends[i].Possible = ends[i].Possible && end.Possible
			for _, site := range end.sites {
				if !slices.ContainsFunc(ends[i].sites, func(other pageAnchor) bool { return other.Text == site.Text }) {
					ends[i].sites = append(ends[i].sites, site)
				}
			}
			return ends
		}
	}
	return append(ends, end)
}

func boolFirst(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	}
	return 1
}

// moduleVariable says whether a variable is a module's or a package's own,
// the only variables a part lists: a local, a parameter or a produced value
// has no such holder.
func (builder *pageBuilder) moduleVariable(targetID string, object *groupindex.ObjectFacts) bool {
	level := func(id string) bool {
		ref, known := builder.subject(targetID, id)
		if !known || ref.subject.Object == nil {
			return false
		}
		kind := ref.subject.Object.Kind
		return kind == programindex.ObjectModule || kind == programindex.ObjectPackage
	}
	return level(object.OwnerID) || level(object.ContainerID)
}

// componentTitle is a component's name on the system map: its short label
// without the kind the label adds to tell programs sharing a root apart
// ("redis-server", not "redis-server (executable)"), since the reading
// states the kind beside it ("C executable"). The kind stays when another
// component would otherwise read the same.
func componentTitle(section *pageSection, sections []*pageSection) string {
	suffix := " (" + section.Kind + ")"
	title, cut := strings.CutSuffix(section.ShortLabel, suffix)
	if section.Kind == "" || !cut {
		return section.ShortLabel
	}
	for _, other := range sections {
		if other != section && (other.ShortLabel == title || strings.TrimSuffix(other.ShortLabel, " ("+other.Kind+")") == title) {
			return section.ShortLabel
		}
	}
	return title
}

// pageInputCollection is an Inputs collection's reading: its catalogues in
// their order, each with its members by name, then the inputs no catalogue
// holds by kind; Kinds counts every input by kind for its component's
// reading. Kinds stand in one fixed order.
type pageInputCollection struct {
	Groups []pageCollectionGroup `json:"groups"`
	Kinds  []pageCollectionGroup `json:"kinds"`
	// Apart are, by input sharing its name and kind with another, the
	// words beside its name telling it apart (page_apart.go), as the list
	// shows them.
	Apart map[string][]pageApartWord `json:"apart,omitempty"`
}

// pageCollectionGroup is inputs of one kind, by name; Catalogue names one
// of them whose catalogue (data-catalogue) the group is.
type pageCollectionGroup struct {
	Kind      string   `json:"kind"`
	Catalogue string   `json:"catalogue,omitempty"`
	Inputs    []string `json:"inputs"`
}

// inputKindOrder is the order the canvas stands an Inputs collection's
// kinds in (scene.go sceneInputKinds, web/model.mjs inputKinds), which the
// column reads them in too (owner, 2026-10-01).
var inputKindOrder = sceneInputKinds

func inputCollection(children []string, node func(string) pageMapNode) string {
	byName := func(ids []string) []string {
		ids = slices.Clone(ids)
		slices.SortStableFunc(ids, func(a, b string) int {
			left, right := node(a).FullTitle, node(b).FullTitle
			return cmp.Or(strings.Compare(strings.ToLower(left), strings.ToLower(right)), strings.Compare(left, right), strings.Compare(a, b))
		})
		return ids
	}
	kindRank := func(kind string) int {
		if at := slices.Index(inputKindOrder, kind); at >= 0 {
			return at
		}
		return len(inputKindOrder)
	}
	byKind := func(ids []string) []pageCollectionGroup {
		kinds := map[string][]string{}
		var order []string
		for _, id := range ids {
			kind := node(id).Activation
			if kinds[kind] == nil {
				order = append(order, kind)
			}
			kinds[kind] = append(kinds[kind], id)
		}
		slices.SortStableFunc(order, func(a, b string) int { return cmp.Or(cmp.Compare(kindRank(a), kindRank(b)), strings.Compare(a, b)) })
		groups := make([]pageCollectionGroup, 0, len(order))
		for _, kind := range order {
			groups = append(groups, pageCollectionGroup{Kind: kind, Inputs: byName(kinds[kind])})
		}
		return groups
	}
	collection := pageInputCollection{Kinds: byKind(children)}
	for _, id := range children {
		if words := node(id).apartWords; len(words) > 0 {
			if collection.Apart == nil {
				collection.Apart = map[string][]pageApartWord{}
			}
			collection.Apart[id] = words
		}
	}
	catalogues := map[string]int{}
	var loose []string
	for _, id := range children {
		input := node(id)
		if input.Catalogue == "" {
			loose = append(loose, id)
			continue
		}
		at, known := catalogues[input.Catalogue]
		if !known {
			at = len(collection.Groups)
			catalogues[input.Catalogue] = at
			collection.Groups = append(collection.Groups, pageCollectionGroup{Kind: input.Activation, Catalogue: id})
		}
		collection.Groups[at].Inputs = append(collection.Groups[at].Inputs, id)
	}
	for i := range collection.Groups {
		collection.Groups[i].Inputs = byName(collection.Groups[i].Inputs)
	}
	// The inputs no catalogue holds, those running a handler apart from
	// those whose handler is not established.
	var handled, values []string
	for _, id := range loose {
		if node(id).HandlerUnknown {
			values = append(values, id)
		} else {
			handled = append(handled, id)
		}
	}
	collection.Groups = append(append(collection.Groups, byKind(handled)...), byKind(values)...)
	// Requests first, as the kinds stand everywhere. In a kind, what runs a
	// handler before what only declares a value (reviewer, 2026-10-02:
	// freqtrade's trade, backtesting and webserver had stood after the
	// options of AVAILABLE_CLI_OPTIONS, a catalogue none of whose entries
	// runs code); then a kind's catalogues before its inputs no catalogue
	// holds.
	declaresOnly := func(group pageCollectionGroup) int {
		if slices.ContainsFunc(group.Inputs, func(id string) bool { return !node(id).HandlerUnknown }) {
			return 0
		}
		return 1
	}
	slices.SortStableFunc(collection.Groups, func(a, b pageCollectionGroup) int {
		return cmp.Or(cmp.Compare(kindRank(a.Kind), kindRank(b.Kind)), cmp.Compare(declaresOnly(a), declaresOnly(b)))
	})
	raw, err := json.Marshal(collection)
	if err != nil {
		return ""
	}
	return string(raw)
}

// pageEntry is one entrypoint as a component's reading names it: its name,
// a call when it is callable, the link into its code and the part it is
// read in ("#…") by its key, a seed's or a library's export's
// (GroupsIndex's Entries).
type pageEntry struct {
	Name     string `json:"name"`
	Callable bool   `json:"callable,omitempty"`
	Href     string `json:"href,omitempty"`
	Open     string `json:"open,omitempty"`
	NoSource bool   `json:"no_source,omitempty"`
	Part     string `json:"part,omitempty"`
	Key      string `json:"key,omitempty"`
}

// componentEntries are the program's entrypoints for its component's
// reading, in their order; the seed its "Entrypoints" link lands on is read
// in its part.
func componentEntries(section *pageSection) string {
	var entries []pageEntry
	for _, entry := range section.Entrypoints {
		if entry.Symbol == "" {
			continue
		}
		item := pageEntry{Name: entry.Symbol, Callable: entry.Kind == "callable" || entry.Kind == facts.EntrypointExport}
		if entry.Anchor != nil {
			item.Href, item.Open, item.NoSource, item.Key = entry.Anchor.Href, entry.Anchor.Open, entry.Anchor.NoSource, declarationKey(entry.Anchor)
		}
		if part := section.EntryParts[entry.ObjectID]; entry.ObjectID != "" && part != "" && entry.Anchor != nil &&
			section.EntryAt[entry.ObjectID] == entry.Anchor.Path+":"+strconv.Itoa(entry.Anchor.Line) {
			// Joined by its subject where the fact stands at that declaration,
			// and read by the key its part lists it under. A module run as a
			// script stands at its __main__ block, not at the module's first
			// line, and keeps that place (freqtrade.main at main.py:82).
			item.Part = "#" + part
			if key := section.EntryKeys[entry.ObjectID]; key != "" {
				item.Key = key
			}
		}
		entries = append(entries, item)
	}
	if len(entries) == 0 {
		return ""
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return ""
	}
	return string(raw)
}
