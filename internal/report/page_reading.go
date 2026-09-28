package report

import (
	"cmp"
	"encoding/json"
	"path"
	"slices"
	"strings"

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
//     calls, grouped by the part at the other end (its own part first), and
//     the variables it uses. A type also names the functions of its part
//     that return or take it.
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
	// the file alone, the one the reading writes.
	At   string `json:"at,omitempty"`
	File string `json:"file,omitempty"`
	// Kind is function, type or variable (a method is a function, a
	// type's field a field); empty when the declaration is none of them.
	Kind string `json:"kind,omitempty"`
	// Part is the link to the part it stands in, "#…": the reading reads
	// it there and names that part on hover.
	Part string `json:"part,omitempty"`
	Bold bool   `json:"bold,omitempty"`
	// Doc is the comment its author wrote above it, quoted as written: an
	// author's claim, never the model's.
	Doc    string             `json:"doc,omitempty"`
	Fields []pageReadingField `json:"fields,omitempty"`
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
}

// pageReadingKind is the part's declarations of one kind, by name.
type pageReadingKind struct {
	Kind  string `json:"kind"`
	Decls []int  `json:"decls"`
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
}

// pageReadingEnd is a declaration at a relation's other end: its kind of
// relation, whether it is only possible, and every place it is written.
type pageReadingEnd struct {
	Decl     int               `json:"decl"`
	Kind     string            `json:"kind"`
	Possible bool              `json:"possible,omitempty"`
	Sites    []pageReadingSite `json:"sites,omitempty"`
}

// pageReadingSite is one place a relation is written: its words
// ("redis.c:2011") and the link to that line (owner, 2026-09-28: an edge
// links to the call's own line).
type pageReadingSite struct {
	At   string `json:"at"`
	Href string `json:"href,omitempty"`
	Open string `json:"open,omitempty"`
}

// pageReadingOwner is one declaration of the part and its relations.
type pageReadingOwner struct {
	Decl    int                    `json:"decl"`
	Callers []pageReadingPeerDecls `json:"callers,omitempty"`
	Callees []pageReadingPeerDecls `json:"callees,omitempty"`
	Uses    []pageReadingEnd       `json:"uses,omitempty"`
	Returns []int                  `json:"returns,omitempty"`
	Takes   []int                  `json:"takes,omitempty"`
}

// pageReadingPeerDecls is one part's declarations at the other end of a
// declaration's relations, by name.
type pageReadingPeerDecls struct {
	Part  string           `json:"part"`
	Title string           `json:"title"`
	Own   bool             `json:"own,omitempty"`
	Decls []pageReadingEnd `json:"decls"`
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
	// declare names a declaration once, by the key the page's script keys
	// it by; a later mention fills what an earlier one did not know.
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
		return declare(pageReadingDecl{Name: label, Key: declarationKey(anchor), Href: anchor.Href, Open: anchor.Open, NoSource: anchor.NoSource, Code: anchor.Code,
			At: anchor.Text, File: path.Base(anchor.Path), Kind: kind, Part: part})
	}

	// The members: the declarations a reader looks for, as the part's tiles
	// draw them, each type with every one of its fields.
	lists := map[string][]int{}
	types := map[string]int{}
	var fields []string
	for _, id := range group.MemberSubjectIDs {
		ref, known := builder.subject(targetID, id)
		if !known || ref.subject.Object == nil {
			continue
		}
		label, anchor := builder.subjectDisplay(ref.subject)
		if label == "" || anchor == nil || strings.Contains(label, "$") {
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
		position := fromAnchor(label, anchor, kind, own)
		if position < 0 {
			continue
		}
		decl := &reading.Decls[position]
		decl.Bold = ref.subject.Interpretation != nil && ref.subject.Interpretation.Key
		decl.Doc = builder.docstringFor(anchor.Path, anchor.Line)
		if !slices.Contains(lists[kind], position) {
			lists[kind] = append(lists[kind], position)
		}
		if anchor.Path != "" && !slices.Contains(reading.Files, anchor.Path) {
			reading.Files = append(reading.Files, anchor.Path)
		}
		if kind == "type" {
			types[id] = position
		}
	}
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
	rows := make([]pageConnection, 0, len(card.Connections)+len(card.InternalConnections))
	rows = append(append(rows, card.Connections...), card.InternalConnections...)
	for _, row := range rows {
		if row.Phrase() == "" || row.FromDecl == nil || row.ToDecl == nil {
			continue
		}
		fromPart, fromTitle := partOf(row.fromTarget, row.fromSubject)
		toPart, toTitle := partOf(row.toTarget, row.toSubject)
		from := fromAnchor(row.FromName, row.FromDecl, kindOf(row.fromTarget, row.fromSubject), fromPart)
		to := fromAnchor(row.ToName, row.ToDecl, kindOf(row.toTarget, row.toSubject), toPart)
		if from < 0 || to < 0 {
			continue
		}
		var sites []pageReadingSite
		if row.FromSource != nil {
			sites = []pageReadingSite{{At: row.FromSource.Text, Href: row.FromSource.Href, Open: row.FromSource.Open}}
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
			}
			peers[peer].ends[line] = mergeEnd(peers[peer].ends[line], pageReadingEnd{Decl: to, Kind: kind, Possible: row.Possible})
		}
		// The declaration's own reading: the caller's callees, the callee's
		// callers, each by the part the other end stands in.
		if fromPart == own {
			addEnd(callees, from, cmp.Or(toPart, row.Href), cmp.Or(toTitle, row.Title), pageReadingEnd{Decl: to, Kind: kind, Possible: row.Possible, Sites: sites})
		}
		if toPart == own {
			addEnd(callers, to, cmp.Or(fromPart, row.Href), cmp.Or(fromTitle, row.Title), pageReadingEnd{Decl: from, Kind: kind, Possible: row.Possible, Sites: sites})
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
				item.Lines = append(item.Lines, pageReadingLine{Caller: line, Ends: sortEnds(peer.ends[line])})
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

	// Each member's own reading.
	declGroups := func(sides map[string]*side, decl int, variables bool) ([]pageReadingPeerDecls, []pageReadingEnd) {
		var groups []pageReadingPeerDecls
		var uses []pageReadingEnd
		for _, peer := range sides {
			item := pageReadingPeerDecls{Part: peer.part, Title: peer.title, Own: peer.part == own}
			for _, end := range peer.ends[decl] {
				if variables && usesVariable(end.Kind) {
					uses = append(uses, end)
					continue
				}
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
		return groups, sortEnds(uses)
	}
	for _, members := range reading.Members {
		for _, decl := range members.Decls {
			owner := pageReadingOwner{Decl: decl}
			owner.Callers, _ = declGroups(callers[decl], decl, false)
			owner.Callees, owner.Uses = declGroups(callees[decl], decl, true)
			if len(owner.Callers)+len(owner.Callees)+len(owner.Uses) > 0 {
				reading.Own = append(reading.Own, owner)
			}
		}
	}
	// A type of the part is returned or taken by the part's functions.
	for id, typePosition := range types {
		var returns, takes []int
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
				if result.TypeID == id && !slices.Contains(returns, position) {
					returns = append(returns, position)
				}
			}
			for _, parameter := range ref.subject.Object.Parameters {
				if parameter.TypeID == id && !slices.Contains(takes, position) {
					takes = append(takes, position)
				}
			}
		}
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
	slices.SortFunc(reading.Own, func(a, b pageReadingOwner) int { return cmp.Compare(a.Decl, b.Decl) })
	if len(reading.Decls) == 0 {
		return ""
	}
	raw, err := json.Marshal(reading)
	if err != nil {
		return ""
	}
	return string(raw)
}

// mergeEnd adds an end to a declaration's list, one entry per declaration
// and relation kind with every place it is written.
func mergeEnd(ends []pageReadingEnd, end pageReadingEnd) []pageReadingEnd {
	for i := range ends {
		if ends[i].Decl == end.Decl && ends[i].Kind == end.Kind {
			ends[i].Possible = ends[i].Possible && end.Possible
			for _, site := range end.Sites {
				if !slices.ContainsFunc(ends[i].Sites, func(listed pageReadingSite) bool { return listed.At == site.At }) {
					ends[i].Sites = append(ends[i].Sites, site)
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
}

// pageCollectionGroup is inputs of one kind, by name; Catalogue names one
// of them whose catalogue (data-catalogue) the group is.
type pageCollectionGroup struct {
	Kind      string   `json:"kind"`
	Catalogue string   `json:"catalogue,omitempty"`
	Inputs    []string `json:"inputs"`
}

var inputKindOrder = []string{"request", "command", "setting", "interaction", "continuous", "scheduled"}

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
	collection.Groups = append(collection.Groups, byKind(loose)...)
	// Requests first, as the kinds stand everywhere; a kind's catalogues
	// before its inputs no catalogue holds.
	slices.SortStableFunc(collection.Groups, func(a, b pageCollectionGroup) int { return cmp.Compare(kindRank(a.Kind), kindRank(b.Kind)) })
	raw, err := json.Marshal(collection)
	if err != nil {
		return ""
	}
	return string(raw)
}

// pageEntry is one entrypoint as a component's reading names it: its name,
// a call when it is callable, the link into its code and, for the
// program's seed, the part it is read in ("#…") by its key.
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
		item := pageEntry{Name: entry.Symbol, Callable: entry.Kind == "callable"}
		if entry.Anchor != nil {
			item.Href, item.Open, item.NoSource, item.Key = entry.Anchor.Href, entry.Anchor.Open, entry.Anchor.NoSource, declarationKey(entry.Anchor)
		}
		if section.EntryPart != "" && item.Key != "" && item.Key == section.EntrySource {
			item.Part = "#" + section.EntryPart
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
