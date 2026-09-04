package report

import (
	"fmt"
	"github.com/dvordrova/repomap/internal/claims"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// maxVisibleGroupMembers is how many member chips a group shows before the
// rest move behind one disclosure. Three keeps a target under two screens.
const maxVisibleGroupMembers = 3

// pageSection is one analyzed target as the reader walks it: what calls in,
// where execution starts, what the core does, what it calls out to, the main
// flow, and then the warnings.
type pageSection struct {
	ID string
	// Name is what the reader calls this target; Label additionally
	// distinguishes two targets that share a name.
	Name     string
	Label    string
	Language string
	Kind     string
	Root     string

	// programTargetID and factsTargetID join this section to the group graph
	// and the fact layer; neither ever reaches the page.
	programTargetID string
	factsTargetID   string

	FactsAvailable bool
	Map            *pageMap
	RouteGroups    []pageRouteGroup
	// InboundCount is how many route rows this target shows, so the jump bar
	// can say what is behind a link before it is followed.
	InboundCount     int
	Triggers         []pageGroup
	Entrypoints      []pageEntrypoint
	Core             []pageGroup
	Calls            []pageHTTPRow
	DependencyGroups []pageGroup
	Dependencies     []pageDependency
	Flow             *pageFlow
	// Start is where to start reading when no model flow passes through
	// this target: each entrypoint, the group it lands in, and what that
	// group reaches first. Three of chi's four targets said only that the
	// repository's main flow does not pass through them.
	Start       []pageStart
	FlowMissing string
	Dynamic     []pageDynamic
	Config      []pageConfig
	Dead        []pageAnchor
	Todos       []pageTodo
}

type pageRouteGroup struct {
	Method string
	Rows   []pageRouteRow
	// Paths is how many paths the method answers on, which is what the jump
	// bar counts; a row can hold several.
	Paths int
}

type pageEntrypoint struct {
	Symbol string
	Kind   string
	Anchor *pageAnchor
}

// pageStart is one entrypoint read forward: the symbol, the group it is in,
// and the first groups that group reaches.
type pageStart struct {
	Symbol  string
	Anchor  *pageAnchor
	Group   string
	Href    string
	Reaches []pageConnection
}

type pageDependency struct {
	Name    string
	Version string
	Anchor  *pageAnchor
}

// pageDynamic is one place where the program runs code it was handed rather
// than code the reader can follow.
type pageDynamic struct {
	Pattern string
	Symbol  string
	Witness string
	Anchor  *pageAnchor
}

type pageConfig struct {
	Key     string
	Default string
	Anchor  *pageAnchor
}

type pageTodo struct {
	Text   string
	Anchor *pageAnchor
}

type pageFlow struct {
	Title string
	Steps []pageFlowStep
}

type pageFlowStep struct {
	Label       string
	Target      string
	Explanation string
	Anchor      *pageAnchor
}

// pageGroup is one responsibility card. Members are grouped by file so the
// path is printed once and each chip carries only its line.
type pageGroup struct {
	ID          string
	Members     int
	Share       int
	Title       string
	Summary     string
	Visible     []pageChipRow
	More        []pageChipRow
	MoreCount   int
	Externals   []pageExternal
	Connections []pageConnection
	Docs        []pageDoc
}

type pageChipRow struct {
	Path    string
	Members []pageChip
}

type pageChip struct {
	Name   string
	Line   int
	Anchor pageAnchor
	// Doc is what the author wrote above this symbol, when they wrote
	// anything. A hundred and ninety docstrings of chi were quoted into the
	// claims layer and none reached the page; a card that shows a symbol
	// can show the sentence that explains it.
	Doc string
}

// pageDoc is one author's sentence shown on a card, under the model's.
type pageDoc struct {
	Symbol string
	Text   string
}

// pageConnection is one model sentence between two groups. A connection to
// another target renders as a stub that links to that target's section.
type pageConnection struct {
	Arrow       string
	Title       string
	OtherTarget string
	Href        string
	Label       string
	Summary     string
	Possible    bool
	// Count is how many times this same line was said. Three exact calls
	// from one group to another were three identical rows on the card.
	Count int
}

// buildSections creates one section per analyzed target and fills it from the
// fact layer and that target's group graph. Sections are created first so
// every later builder can link a fact or a connection to its owning section.
func (builder *pageBuilder) buildSections() {
	builder.createSections()
	for _, section := range builder.sections {
		builder.fillSectionFacts(section)
		builder.fillSectionGroups(section)
		section.Map = builder.buildMap(section)
		for _, group := range section.RouteGroups {
			section.InboundCount += group.Paths
		}
		section.Flow = builder.flow(section)
		if section.Flow == nil {
			if index := builder.graphIndex(section.programTargetID); index != nil {
				section.Start = builder.startSteps(section, *index)
			}
		}
		if section.Flow == nil {
			section.FlowMissing = "This run produced no main flow."
			if builder.data.Orientation != nil && len(builder.data.Orientation.MainFlow.Steps) > 0 {
				section.FlowMissing = "The repository's main flow does not pass through this target."
			}
		}
	}
}

// createSections derives the section list from the group graph, which is the
// one authority that exists for every analyzed target, then binds the fact
// target that describes the same root.
func (builder *pageBuilder) createSections() {
	used := make(map[string]struct{}, len(builder.indexes))
	for position := range builder.indexes {
		index := &builder.indexes[position]
		section := &pageSection{
			ID:              sectionID(index.Target.Name, position),
			Name:            index.Target.Name,
			Language:        index.Target.Language,
			Kind:            index.Target.Kind,
			programTargetID: index.Target.ID,
		}
		if target, ok := builder.factsTargetFor(index.Target.ID, used); ok {
			section.factsTargetID = target.ID
			section.FactsAvailable = true
			section.Root = target.Root
			used[target.ID] = struct{}{}
			builder.byFacts[target.ID] = section
		}
		builder.byProgram[index.Target.ID] = section
		builder.sections = append(builder.sections, section)
	}
	labelSections(builder.sections)
}

// labelSections keeps every target distinguishable in the navigation. Two
// targets can legitimately share a name — a library and a command in one
// directory — so a repeated name gains the detail that separates them.
func labelSections(sections []*pageSection) {
	count := make(map[string]int, len(sections))
	for _, section := range sections {
		count[section.Name]++
	}
	for _, section := range sections {
		section.Label = section.Name
		if count[section.Name] < 2 {
			continue
		}
		switch {
		case section.Kind != "":
			section.Label = section.Name + " (" + section.Kind + ")"
		case section.Root != "":
			section.Label = section.Name + " (" + section.Root + ")"
		}
	}
}

// factsTargetFor matches a graph target to its fact target by the program
// target id the fact layer recorded. Matching is exact: a fact target is
// never guessed from a name.
func (builder *pageBuilder) factsTargetFor(
	programTargetID string,
	used map[string]struct{},
) (facts.Target, bool) {
	if builder.data.Facts == nil {
		return facts.Target{}, false
	}
	for _, target := range builder.data.Facts.Targets {
		if target.ProgramTargetID != programTargetID {
			continue
		}
		if _, taken := used[target.ID]; taken {
			continue
		}
		return target, true
	}
	return facts.Target{}, false
}

func sectionID(name string, position int) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '-'
		}
	}, name)
	cleaned = strings.Trim(cleaned, "-")
	if cleaned == "" {
		cleaned = "target"
	}
	return fmt.Sprintf("%s-%d", cleaned, position+1)
}

func (builder *pageBuilder) fillSectionFacts(section *pageSection) {
	if !section.FactsAvailable {
		return
	}
	section.RouteGroups = builder.routeGroups(section.factsTargetID)
	section.Calls = builder.httpRows(facts.KindHTTPCall, section.factsTargetID)
	for _, fact := range builder.targetFacts(section.factsTargetID, facts.KindEntrypoint) {
		section.Entrypoints = append(section.Entrypoints, pageEntrypoint{
			Symbol: fact.Symbol,
			Kind:   strings.ReplaceAll(fact.Key, "_", " "),
			Anchor: builder.links.factAnchor(fact),
		})
	}
	for _, fact := range builder.targetFacts(section.factsTargetID, facts.KindDependency) {
		section.Dependencies = append(section.Dependencies, pageDependency{
			Name: fact.Key, Version: fact.Value, Anchor: builder.links.factAnchor(fact),
		})
	}
	for _, fact := range builder.targetFacts(section.factsTargetID, facts.KindDynamicExecution) {
		section.Dynamic = append(section.Dynamic, pageDynamic{
			Pattern: fact.Key, Symbol: fact.Symbol, Witness: fact.Text,
			Anchor: builder.links.factAnchor(fact),
		})
	}
	for _, fact := range builder.targetFacts(section.factsTargetID, facts.KindConfigRead) {
		section.Config = append(section.Config, pageConfig{
			Key: fact.Key, Default: fact.Value, Anchor: builder.links.factAnchor(fact),
		})
	}
	// Manifest settings are configuration too: the port a proxy points at or
	// the command a script runs answers the same reader question.
	for _, fact := range builder.targetFacts(section.factsTargetID, facts.KindManifest) {
		section.Config = append(section.Config, pageConfig{
			Key: fact.Key, Default: fact.Value, Anchor: builder.links.factAnchor(fact),
		})
	}
	for _, fact := range builder.targetFacts(section.factsTargetID, facts.KindDeadModule) {
		if anchor := builder.links.factAnchor(fact); anchor != nil {
			section.Dead = append(section.Dead, *anchor)
		}
	}
	for _, fact := range builder.targetFacts(section.factsTargetID, facts.KindTODO) {
		section.Todos = append(section.Todos, pageTodo{
			Text: fact.Text, Anchor: builder.links.factAnchor(fact),
		})
	}
}

// routeGroups buckets the target's routes by method so a reader scans one
// verb at a time instead of a flat list.
func (builder *pageBuilder) routeGroups(targetID string) []pageRouteGroup {
	rows := builder.httpRows(facts.KindHTTPRoute, targetID)
	if len(rows) == 0 {
		return nil
	}
	byMethod := make(map[string][]pageHTTPRow, len(rows))
	for _, row := range rows {
		byMethod[row.Method] = append(byMethod[row.Method], row)
	}
	groups := make([]pageRouteGroup, 0, len(byMethod))
	for _, method := range sortedMethods(byMethod) {
		merged := mergeRoutesByHandler(byMethod[method])
		paths := 0
		for _, row := range merged {
			paths += len(row.Paths)
		}
		groups = append(groups, pageRouteGroup{Method: method, Rows: merged, Paths: paths})
	}
	return groups
}

// mergeRoutesByHandler puts every path one handler answers on into one row.
// The order routes were found in is kept, so the first path of a handler is
// where its row appears.
func mergeRoutesByHandler(rows []pageHTTPRow) []pageRouteRow {
	type key struct {
		symbol string
		anchor string
	}
	position := make(map[key]int, len(rows))
	merged := make([]pageRouteRow, 0, len(rows))
	var unnamed []pageRoutePath
	for _, row := range rows {
		identity := key{symbol: row.Symbol}
		if row.SymbolAnchor != nil {
			identity.anchor = row.SymbolAnchor.Text
		}
		path := pageRoutePath{Path: row.Path, Anchor: row.Anchor, Possible: row.Possible}
		if row.Symbol == "" {
			// Paths whose handler the code does not name go together at the
			// end. One a line, each under a row that does name a handler,
			// they read as if that handler answered them too.
			unnamed = append(unnamed, path)
			continue
		}
		if index, seen := position[identity]; seen {
			merged[index].Paths = append(merged[index].Paths, path)
			continue
		}
		position[identity] = len(merged)
		merged = append(merged, pageRouteRow{
			Paths: []pageRoutePath{path}, Symbol: row.Symbol, SymbolAnchor: row.SymbolAnchor,
		})
	}
	if len(unnamed) > 0 {
		merged = append(merged, pageRouteRow{Paths: unnamed})
	}
	return merged
}

func (builder *pageBuilder) fillSectionGroups(section *pageSection) {
	index := builder.graphIndex(section.programTargetID)
	if index == nil {
		return
	}
	for _, group := range index.Groups {
		card := builder.groupCard(section.ID, *index, group)
		switch group.Lane {
		case groupindex.LaneTriggers:
			section.Triggers = append(section.Triggers, card)
		case groupindex.LaneCore:
			section.Core = append(section.Core, card)
		case groupindex.LaneDependencies:
			section.DependencyGroups = append(section.DependencyGroups, card)
		}
	}
}

// siblingByPackage finds the analyzed target a package path names. chi's
// rest-example imports github.com/go-chi/chi/v5, and that is a target on this
// same page: calling it external is true of the compiler and false of the
// reader.
func (builder *pageBuilder) siblingByPackage(packagePath string) *pageSection {
	if packagePath == "" {
		return nil
	}
	return ownerOfPackage(packagePath, builder.packageOwners())
}

// packageOwner is one target and the package paths it indexed. A target owns
// the packages it was read from, so an import is credited by what it names
// and not by whose module root it happens to sit under.
type packageOwner struct {
	section  *pageSection
	packages []string
}

func (builder *pageBuilder) packageOwners() []packageOwner {
	if builder.owners != nil {
		return builder.owners
	}
	byTarget := make(map[string][]string)
	if builder.data.ProgramPortfolio != nil {
		for _, entry := range builder.data.ProgramPortfolio.Entries {
			for _, object := range entry.View.Objects {
				if object.Kind == programindex.ObjectPackage || object.Kind == programindex.ObjectModule {
					byTarget[entry.Target.ID] = append(byTarget[entry.Target.ID], object.Name)
				}
			}
		}
	}
	builder.owners = make([]packageOwner, 0, len(builder.sections))
	for _, section := range builder.sections {
		packages := append([]string(nil), byTarget[section.programTargetID]...)
		packages = append(packages, section.Name, section.Label)
		builder.owners = append(builder.owners, packageOwner{section: section, packages: packages})
	}
	return builder.owners
}

// ownerOfPackage is the target an import path belongs to. Exactly the package
// a target indexed is the surest match. Next comes the target's own directory
// in the repository, found inside the path — chi's example executable imports
// github.com/go-chi/chi/v5/_examples/versions/presenter/v2, and the library
// beside it lives at _examples/versions; so does the package's module-relative
// tail, versions/presenter/v2, when the report knows the target's packages.
// Only failing all of those does a path under a target's module root count for
// that target, which is how those presenter imports were credited to chi/v5
// and the example library was reached by nothing. Among equals the longer
// name wins, and a library beats a main, since nothing imports a main.
func ownerOfPackage(packagePath string, owners []packageOwner) *pageSection {
	var found *pageSection
	bestRank, bestLength := 0, 0
	consider := func(section *pageSection, rank, length int) {
		better := rank > bestRank ||
			(rank == bestRank && length > bestLength) ||
			(rank == bestRank && length == bestLength && found != nil &&
				found.Kind != "library" && section.Kind == "library")
		if better {
			found, bestRank, bestLength = section, rank, length
		}
	}
	for _, owner := range owners {
		if root := strings.Trim(owner.section.Root, "/"); root != "" && root != "." &&
			strings.Contains("/"+packagePath+"/", "/"+root+"/") {
			consider(owner.section, 2, len(root))
		}
		for _, name := range owner.packages {
			if name == "" {
				continue
			}
			switch {
			case packagePath == name:
				consider(owner.section, 3, len(name))
			case strings.HasSuffix(packagePath, "/"+name):
				consider(owner.section, 2, len(name))
			case strings.HasPrefix(packagePath, name+"/"):
				consider(owner.section, 1, len(name))
			}
		}
	}
	return found
}

func (builder *pageBuilder) graphIndex(programTargetID string) *groupindex.Index {
	for position := range builder.indexes {
		index := &builder.indexes[position]
		if index.Target.ID == programTargetID {
			return index
		}
	}
	return nil
}

func (builder *pageBuilder) groupCard(sectionID string, index groupindex.Index, group groupindex.Group) pageGroup {
	card := pageGroup{
		ID: groupAnchorID(sectionID, group.ID), Title: group.Title,
		// A summary that is the title again is the title said twice.
		Summary: dropEcho(group.Summary, group.Title),
		Members: len(group.MemberSubjectIDs), Share: laneShare(index, group),
	}
	rows, externals := builder.memberChips(group.MemberSubjectIDs)
	card.Externals = externals
	card.Visible, card.More, card.MoreCount = splitChipRows(rows, maxVisibleGroupMembers)
	card.Connections = builder.groupConnections(index, group)
	card.Docs = cardDocs(card.Visible, maxCardDocs)
	return card
}

// memberChips resolves member subjects to anchored chips grouped by file and
// deduplicated by path and line, so one file prints its path once. Members
// without a location (external packages) become plain labels.
// pageExternal is a symbol this group uses that has no line in this target. It
// is usually genuinely outside the repository — but when its package is
// another target of this same repository, saying "outside" is wrong and the
// chip leads there instead.
type pageExternal struct {
	Name string
	Href string
}

func (builder *pageBuilder) memberChips(memberIDs []string) ([]pageChipRow, []pageExternal) {
	byPath := make(map[string][]pageChip)
	seen := make(map[string]struct{}, len(memberIDs))
	var externals []pageExternal
	for _, id := range memberIDs {
		ref, known := builder.subjects[id]
		if !known {
			continue
		}
		name, anchor := builder.subjectDisplay(ref.subject)
		if name == "" {
			continue
		}
		if anchor == nil {
			if _, duplicate := seen[name]; duplicate {
				continue
			}
			seen[name] = struct{}{}
			row := pageExternal{Name: name}
			if object := ref.subject.Object; object != nil && object.External != nil {
				if sibling := builder.siblingByPackage(object.External.PackagePath); sibling != nil {
					row.Href = "#" + sibling.ID
				}
			}
			externals = append(externals, row)
			continue
		}
		key := anchor.Path + ":" + name
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		byPath[anchor.Path] = append(byPath[anchor.Path], pageChip{
			Name: name, Line: anchor.Line, Anchor: *anchor,
			Doc: builder.docstringFor(anchor.Path, anchor.Line),
		})
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	rows := make([]pageChipRow, 0, len(paths))
	for _, path := range paths {
		chips := byPath[path]
		sort.SliceStable(chips, func(i, j int) bool {
			if chips[i].Line != chips[j].Line {
				return chips[i].Line < chips[j].Line
			}
			return chips[i].Name < chips[j].Name
		})
		rows = append(rows, pageChipRow{Path: path, Members: chips})
	}
	// A sibling target first: a name a reader can follow is worth more than
	// one they cannot.
	sort.SliceStable(externals, func(left, right int) bool {
		if (externals[left].Href != "") != (externals[right].Href != "") {
			return externals[left].Href != ""
		}
		return externals[left].Name < externals[right].Name
	})
	return rows, externals
}

// splitChipRows keeps the first limit chips visible and moves the rest behind
// a disclosure, splitting a file's chips across the boundary when needed.
func splitChipRows(rows []pageChipRow, limit int) (visible, more []pageChipRow, moreCount int) {
	budget := limit
	for _, row := range rows {
		switch {
		case budget <= 0:
			more = append(more, row)
			moreCount += len(row.Members)
		case len(row.Members) <= budget:
			visible = append(visible, row)
			budget -= len(row.Members)
		default:
			visible = append(visible, pageChipRow{Path: row.Path, Members: row.Members[:budget]})
			rest := row.Members[budget:]
			// The path is printed once. This row's continuation opens directly
			// under the chips it continues, so repeating its path there reads
			// as a second file with the same name.
			more = append(more, pageChipRow{Members: rest})
			moreCount += len(rest)
			budget = 0
		}
	}
	return visible, more, moreCount
}

// groupConnections renders the model's one-line sentences for every
// connection incident to this group. A connection whose other endpoint lives
// in another target becomes a stub linking to that target's section.
func (builder *pageBuilder) groupConnections(
	index groupindex.Index,
	group groupindex.Group,
) []pageConnection {
	here := groupindex.Endpoint{TargetID: index.Target.ID, GroupID: group.ID}
	var rows []pageConnection
	for _, connection := range builder.allConnections() {
		var other groupindex.Endpoint
		var arrow string
		switch {
		case connection.From == here:
			other, arrow = connection.To, "→"
		case connection.To == here:
			other, arrow = connection.From, "←"
		default:
			continue
		}
		row := pageConnection{
			Arrow:    arrow,
			Title:    builder.groupTitles[other],
			Label:    connection.Label,
			Summary:  connection.Summary,
			Possible: connection.SupportResolution == programindex.PatternValuePossible,
		}
		if row.Title == "" {
			row.Title = strings.ReplaceAll(connection.SemanticKind, "_", " ")
		}
		if other.TargetID != index.Target.ID {
			if section := builder.byProgram[other.TargetID]; section != nil {
				row.OtherTarget = section.Name
				row.Href = "#" + section.ID
			}
		}
		rows = append(rows, row)
	}
	return collapseConnections(rows)
}

// collapseConnections says each distinct line once, with how often it was
// said, and does not repeat the label as its own explanation. A card read
// "← Client IP middleware: provides client IP context — provides client IP
// context" three times over; the model's summary of a connection is usually
// its label again, and when it is, it is noise on the line.
func collapseConnections(rows []pageConnection) []pageConnection {
	type key struct {
		arrow, title, otherTarget, label string
		possible                         bool
	}
	at := make(map[key]int, len(rows))
	result := make([]pageConnection, 0, len(rows))
	for _, row := range rows {
		row.Summary = dropEcho(row.Summary, row.Label)
		k := key{row.Arrow, row.Title, row.OtherTarget, row.Label, row.Possible}
		if position, seen := at[k]; seen {
			result[position].Count++
			if result[position].Summary == "" {
				result[position].Summary = row.Summary
			}
			continue
		}
		row.Count = 1
		at[k] = len(result)
		result = append(result, row)
	}
	return result
}

// dropEcho is text unless it only repeats what stands beside it.
func dropEcho(text, beside string) string {
	fold := func(value string) string {
		return strings.ToLower(strings.TrimRight(strings.TrimSpace(value), "."))
	}
	if fold(text) == fold(beside) {
		return ""
	}
	return text
}

// allConnections is the complete matched set. A cross-target connection is
// stored once, in the index that owns its source group, so both endpoints
// must look at every index to find it.
func (builder *pageBuilder) allConnections() []groupindex.Connection {
	var rows []groupindex.Connection
	for _, index := range builder.indexes {
		rows = append(rows, index.Connections...)
	}
	return rows
}

// laneShare is how much of the target one group holds. The same number sizes
// its node on the map, so the card and the picture cannot disagree.
func laneShare(index groupindex.Index, group groupindex.Group) int {
	if len(index.Subjects) == 0 {
		return 0
	}
	return len(group.MemberSubjectIDs) * 100 / len(index.Subjects)
}

// maxCardDocs is how many authors' sentences a card quotes. Every member's
// docstring is still on its chip; the card leads with the first few.
const maxCardDocs = 2

// docstringReach is how far above a declaration its docstring may start.
// A Go doc comment sits directly above; a long one starts a dozen lines up.
const docstringReach = 12

func cardDocs(rows []pageChipRow, most int) []pageDoc {
	var docs []pageDoc
	for _, row := range rows {
		for _, chip := range row.Members {
			if chip.Doc == "" || len(docs) == most {
				continue
			}
			docs = append(docs, pageDoc{Symbol: chip.Name, Text: chip.Doc})
		}
	}
	return docs
}

// docstringFor is the docstring written above the symbol declared at this
// line of this file, if one was quoted into the claims.
func (builder *pageBuilder) docstringFor(path string, line int) string {
	return nearestDocstring(builder.docstrings[path], builder.declarations[path], line)
}

// nearestDocstring picks, from a file's docstrings in line order, the last
// one that starts above the line and within reach of it — and belongs to
// this symbol and not to one declared between them. Matched by reach alone,
// the sentence above adminRouter was also NewRouter's, declared nine lines
// below it.
func nearestDocstring(docs []claims.Claim, declarations []int, line int) string {
	found := ""
	for _, doc := range docs {
		if doc.Line > line {
			break
		}
		if line-doc.Line > docstringReach {
			continue
		}
		claimed := false
		for _, declared := range declarations {
			if declared > doc.Line && declared < line {
				claimed = true
				break
			}
		}
		if !claimed {
			found = doc.Text
		}
	}
	return found
}

// maxStartReaches is how many first hops one entrypoint shows.
const maxStartReaches = 3

// startSteps reads each entrypoint forward through the group graph: the
// group the entrypoint's symbol is in, then what that group reaches. It is
// assembled from facts and the graph, so it exists for every target, model
// flow or not.
func (builder *pageBuilder) startSteps(section *pageSection, index groupindex.Index) []pageStart {
	groupOf := make(map[string]groupindex.Group)
	for _, group := range index.Groups {
		for _, member := range group.MemberSubjectIDs {
			groupOf[member] = group
		}
	}
	var steps []pageStart
	for _, entry := range section.Entrypoints {
		step := pageStart{Symbol: entry.Symbol, Anchor: entry.Anchor}
		if entry.Anchor != nil {
			if subjectID, known := builder.subjectAt[entry.Anchor.Path+":"+strconv.Itoa(entry.Anchor.Line)]; known {
				if group, inGroup := groupOf[subjectID]; inGroup {
					step.Group = group.Title
					step.Href = "#" + groupAnchorID(section.ID, group.ID)
					step.Reaches = startReaches(builder.groupConnections(index, group), maxStartReaches)
				}
			}
		}
		steps = append(steps, step)
	}
	return steps
}

// startReaches keeps the outgoing connections of a group, the first few.
func startReaches(rows []pageConnection, most int) []pageConnection {
	var out []pageConnection
	for _, row := range rows {
		if row.Arrow != "→" || len(out) == most {
			continue
		}
		out = append(out, row)
	}
	return out
}
