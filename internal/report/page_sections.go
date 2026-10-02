package report

import (
	"cmp"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// pageSection is one analyzed target as the reader walks it: what calls in,
// where execution starts, what the core does, what it calls out to, the main
// flow, and then the warnings.
type pageSection struct {
	Data pageDataCatalog
	ID   string
	// Name is what the reader calls this target; Label additionally
	// distinguishes two targets that share a name.
	Name       string
	Label      string
	ShortLabel string
	Language   string
	Kind       string
	Root       string
	// Role and Purpose are the orientation's one-line answer to "what is
	// this component"; the overview card shows the same sentence, and the
	// component page repeats it under its heading so a reader who lands
	// here from a question or the toolbar is not left with a name only.
	Role, Purpose       string
	RoleRef, PurposeRef string

	// programTargetID and factsTargetID join this section to the group graph
	// and the fact layer; neither ever reaches the page.
	programTargetID string
	factsTargetID   string
	// title is what the program is titled by when its directory does not
	// say it (programTitle).
	title string

	FactsAvailable bool
	// BuiltFrom are the files its program is built from, by path: for C
	// the units its link line names, else the files its declarations are
	// written in, tests left out. The home's table of programs lists them:
	// a model's summary had claimed all four Redis programs share ae, sds,
	// adlist, dict and anet, which the Makefile does not.
	BuiltFrom []string
	// Libraries are the import names under which its build also installs
	// the program's code as a library: a library of the same manifest the
	// portfolio folded into it (ProgramTarget Libraries), named on the
	// program instead of drawn as a second component.
	Libraries   []string
	Map         *pageMap
	RouteGroups []pageRouteGroup
	Requests    []pageGroupOperation
	Activities  []pageGroupOperation
	InputsCount int
	Outbound    []pageOutbound
	// UnnamedLaunches are the calls starting a program the code does not
	// name (unnamedLaunch), read under "What is missing" and with the
	// function making them, never as an outside system.
	UnnamedLaunches []pageOutbound
	Coverage        []string
	// InboundCount counts native route records plus unmatched request
	// interpretations. They can describe the same endpoint.
	InboundCount     int
	Triggers         []pageGroup
	Entrypoints      []pageEntrypoint
	Core             []pageGroup
	Calls            []pageHTTPRow
	DependencyGroups []pageGroup
	Dependencies     []pageDependency
	SharedCode       []pageExternal
	Flow             *pageFlow
	// Start is where to start reading when no model flow passes through
	// this target: each entrypoint, the group it lands in, and what that
	// group reaches first. Three of chi's four targets said only that the
	// repository's main flow does not pass through them.
	Start       []pageStart
	FlowMissing string
	// OwnWork is what the program runs on its own, read after its Main
	// flow (ownWork).
	OwnWork []pageOwnWork
	Dynamic []pageDynamic
	Config  []pageConfig
	Dead    []pageAnchor
	// Unreached are the declarations the adapter proved this program never
	// runs, by file: what they call out to or register is not its own.
	// UnreachedParts are the parts it never runs, which leave its map: their
	// declarations by file, with the part's name, not repeated in Unreached.
	Unreached      []pageChipRow
	UnreachedCount int
	UnreachedParts []pageOffMapRow
	Todos          []pageTodo
	DeadFolders    []pageFileFolder
	TodoFiles      []pageTodoFile
	// TestFiles and OffMap are the component's files the map of parts does
	// not draw: those of parts made only of test code, and every other file
	// no part holds, with why. MapFailure says why there is no map at all.
	TestFiles  []pageOffMapRow
	OffMap     []pageOffMapRow
	MapFailure string
	// OffMapEntries are the program's launch points no part holds, with why
	// (GroupsIndex's Entries): the map then draws no entry part, and the
	// component's reading names them.
	OffMapEntries []pageOffMapEntry
	// EntryPart and EntrySource are where the component's "Entrypoints"
	// link lands (GroupsIndex's Entries): the part holding every seed of
	// the program, and the seed's source key, read there, when it is one.
	// A seed no part holds leaves them empty: the link then reads the
	// component at its entry line.
	EntryPart   string
	EntrySource string
	// EntryGroup is that part's group (entryGroup): where a launch of the
	// program by its own code goes on the map.
	EntryGroup string
	// EntryParts are the parts holding the program's entries by their
	// declaration keys (GroupsIndex's Entries): each seed's and each
	// export of a library, which the component's Entry list reads in its
	// part and folds by it.
	EntryParts map[string]string
}

// pageOffMapEntry is a launch point the map of parts does not draw.
type pageOffMapEntry struct {
	Chip   pageChip
	Reason string
}

// pageOffMapRow is one file outside the map of parts: its source link, the
// test part it belongs to or why no part holds it.
type pageOffMapRow struct {
	Anchor pageAnchor
	Part   string
	Reason string
	// Members are a split file's declarations no box of it took, with their
	// source links; Find lists them as code.
	Members []pageChip
}

// offMapReasons are the reader's words for why a file is off the map.
var offMapReasons = map[string]string{
	"left_out":    "Left out of the parts",
	"conflict":    "Listed in two parts",
	"no_units":    "No declarations of its own",
	"map_failure": "No map of parts",
	"undecided":   "In no part of its file",
	"blocked":     "Used by code in no part",
}

// mapFailureReasons are the reader's words for why there is no map at all.
var mapFailureReasons = map[string]string{
	"refused":  "the model's answer was refused",
	"no_model": "no model was asked",
}

// fillSectionOffMap lists the files the map does not draw from the complete
// group graph, so test code the overview hides stays reachable here.
func (builder *pageBuilder) fillSectionOffMap(section *pageSection) {
	index := builder.graphIndex(section.programTargetID)
	if index == nil {
		return
	}
	section.MapFailure = mapFailureReasons[index.MapFailure]
	section.EntryPart, section.EntrySource = builder.entryLanding(section.ID, index)
	section.EntryGroup = entryGroup(index)
	offEntries := map[string]bool{}
	for _, entry := range index.Entries {
		if entry.GroupID != "" {
			if key := declarationKeyOf(builder, index.Target.ID, entry.SubjectID); key != "" {
				if section.EntryParts == nil {
					section.EntryParts = map[string]string{}
				}
				section.EntryParts[key] = groupAnchorID(section.ID, entry.GroupID)
			}
			continue
		}
		offEntries[entry.SubjectID] = true
		chips, _ := builder.memberChips(index.Target.ID, []string{entry.SubjectID})
		for _, row := range chips {
			for _, chip := range row.Members {
				section.OffMapEntries = append(section.OffMapEntries, pageOffMapEntry{Chip: chip, Reason: offMapReasons[entry.OffMap]})
			}
		}
	}
	for _, file := range index.OffMap {
		row := pageOffMapRow{Anchor: builder.links.anchor(file.Path, 0, 0), Part: file.Part}
		chips, _ := builder.memberChips(index.Target.ID, file.SubjectIDs)
		for _, chip := range chips {
			row.Members = append(row.Members, chip.Members...)
		}
		for position := range row.Members {
			row.Members[position].Entry = offEntries[row.Members[position].objectID]
		}
		if file.Reason == groupindex.OffMapTests {
			section.TestFiles = append(section.TestFiles, row)
			continue
		}
		// A part the program never runs is listed where its unreachable
		// code is, under "Not reachable from the entrypoints".
		if file.Reason == groupindex.OffMapUnreachable {
			builder.markRunBy(index.Target.ID, row.Members)
			section.UnreachedParts = append(section.UnreachedParts, row)
			continue
		}
		row.Reason = offMapReasons[file.Reason]
		section.OffMap = append(section.OffMap, row)
	}
}

// entryLanding is where a component's "Entrypoints" link lands: the part
// holding every seed of the program, with the seed read there when there is
// one. A seed off the map, or seeds in two parts, give no part, and the link
// reads the component at its entry line. It had landed on the inputs, where
// a reader looking for main found none.
func (builder *pageBuilder) entryLanding(sectionID string, index *groupindex.Index) (string, string) {
	group := entryGroup(index)
	if group == "" {
		return "", ""
	}
	source := ""
	if len(index.Entries) == 1 {
		if ref, known := builder.subject(index.Target.ID, index.Entries[0].SubjectID); known {
			if _, anchor := builder.subjectDisplay(ref.subject); anchor != nil {
				source = anchor.Href
				if source == "" {
					source = anchor.Open
				}
			}
		}
	}
	return groupAnchorID(sectionID, group), source
}

// entryGroup is the part holding every seed of the program; none when a
// seed is off the map or the seeds stand in two parts.
func entryGroup(index *groupindex.Index) string {
	group := ""
	for _, entry := range index.Entries {
		if entry.GroupID == "" || group != "" && entry.GroupID != group {
			return ""
		}
		group = entry.GroupID
	}
	return group
}

type pageFileFolder struct {
	Path  string
	Files []pageAnchor
}
type pageTodoFile struct {
	Path string
	Rows []pageTodo
}

type pageRouteGroup struct {
	Method string
	Rows   []pageRouteRow
	// Paths is how many paths the method answers on, which is what the jump
	// bar counts; a row can hold several.
	Paths int
}

// pageOperationFile is one source file's rows of a long operation list. A
// list of seventeen user actions reads faster as six files than as one
// column; the owner's audit asked for exactly this grouping.
type pageOperationFile struct {
	Path string
	Rows []pageGroupOperation
}

// operationsByFile groups rows by the file of their anchor, in first-seen
// order. Rows without an anchor keep a group of their own at the end.
func operationsByFile(rows []pageGroupOperation) []pageOperationFile {
	var files []pageOperationFile
	position := make(map[string]int)
	for _, row := range rows {
		path := row.Anchor.Path
		at, known := position[path]
		if !known {
			at = len(files)
			position[path] = at
			files = append(files, pageOperationFile{Path: path})
		}
		files[at].Rows = append(files[at].Rows, row)
	}
	return files
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
	// Part and Key read the entry in its part, as a model step's do; Code
	// is the link to all of its lines, its name's.
	Part string
	Key  string
	Code string
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
	TitleRef string
	Title    string
	Steps    []pageFlowStep
}

type pageFlowStep struct {
	ExplanationRef string
	Label          string
	Target         string
	Explanation    string
	Anchor         *pageAnchor
	// Part and Key are, for a step that is a declaration of this program,
	// the part it is read in ("#…") and its key there: the step's name reads
	// it and shows it on the canvas (owner, 2026-09-28).
	Part string
	Key  string
	// Registers and RunBy are, for a step citing a registration, where the
	// callable it names is registered and what runs it: a Main flow step's
	// as orientation saved them (flowRegistrations), a callable run on its
	// own as its saved facts say (ownWorkReading).
	Registers []pageStepRegistration
	RunBy     [][]pageStepName
	// Via is how the step before reaches it, as code says it ("called",
	// "handed to quil.core.sketch.setup"); ViaFrom, for one of a dispatch
	// site's alternatives, the function holding the site, a name read as a
	// step's is ("one of the calls call may make"), never a file and line.
	// Fork, on the last step of a flow ending at an undecided split, holds
	// the candidates it could go on through, read folded under one line.
	Via     string
	ViaFrom *pageStepName
	// OneOf is, for a step reached as one of the callables a call through a
	// value may run, that reach in words ("one of the calls serverCron may
	// make"), never a count; Via is then empty.
	OneOf bool
	// ViaKey and ViaArg are Via as a message of the page's language:
	// called, handed to {0}, handed over or its member; any other Via
	// reads as saved.
	ViaKey string
	ViaArg string
	Fork   *pageFlowFork
	// Passed, on a step where the walk decided a split, are the candidates
	// the path did not follow, read folded under "also calls:".
	Passed *pageFlowFork
	// PartHead is, on the first of a run of steps read in one part, that
	// part ("#…"): the column stands the run under the part's box, its
	// title alone, its description on hover (review 2026-10-02, item 2).
	PartHead string
	// TypeName and TypeLine are, on the first of a run of steps that are
	// methods of one type, that type and its own atlas line, read "Type —
	// line" (freqtrade's FreqtradeBot.process: what a FreqtradeBot is). The
	// step's Explanation stays its own line only.
	TypeName    string
	TypeLine    string
	TypeLineRef string
	typeID      string
	// Handles are the inputs the step's declaration handles, its saved
	// operations, by kind: start_trading handles the command trade.
	Handles []pageFlowHandles
	// Ways are, on the step where the flow parts, each way it goes on, read
	// after the step (owner, 2026-09-30: several main paths are allowed
	// where the model is torn between them).
	Ways []pageFlowWay
}

// pageFlowWay is one way a Main flow goes on where it parts: its first step,
// always shown, then the rest, folded when the way is long.
type pageFlowWay struct {
	Head   pageFlowStep
	Rest   []pageFlowStep
	Folded bool
}

// pageOneOf is what the "one-of-calls" template reads: the function whose
// call through a value may run the callable, or the words said when no
// function is known.
type pageOneOf struct {
	From *pageStepName
	Else string
}

// pageFlowFork is a Main flow's named fork, read in words ("one of the
// calls call may make"), or a step's passed calls ("also calls:"), with each
// candidate's name.
type pageFlowFork struct {
	Label string
	From  *pageStepName
	Names []pageStepName
	// OneOf is an undecided fork, read in words: "one of the calls {From}
	// may make" when its candidates share one dispatch site, else "one of
	// the calls it may make", never a count.
	OneOf bool
}

// pageFlowHandles are the inputs of one kind a Main flow step handles: the
// kind's words ("handles the command"), each input's name as its
// registration wrote it and its node on the map. Several fold under the
// kind's plural words, a step never running on into a wall of names.
type pageFlowHandles struct {
	Words  string
	Names  []pageFlowInput
	Folded bool
}

type pageFlowInput struct {
	Name  string
	Input string
}

// pageGroup is one responsibility card. Members are grouped by file so the
// path is printed once and each chip carries only its line.
type pageGroup struct {
	SummaryRef string
	ID         string
	// NodeID is the part's node on the map: its anchor on the page stands
	// for it there.
	NodeID              string
	Members             int
	Share               int
	Title               string
	Summary             string
	Operations          []pageGroupOperation
	Connections         []pageConnection
	InternalConnections []pageConnection
	// Zone is the part this group is in, when it is in one, and ZoneHref
	// the frame on the map that draws it.
	Zone     string
	ZoneHref string
	// Reading is the reading column's data for the part and each of its
	// declarations (page_reading.go), sorted here.
	Reading string
}

type pageChipRow struct {
	Path    string
	Members []pageChip
}

type pageChip struct {
	SummaryRef string
	Name       string
	Alias      string
	Line       int
	Anchor     pageAnchor
	// Doc is what the author wrote above this symbol, when they wrote
	// anything. A hundred and ninety docstrings of chi were quoted into the
	// claims layer and none reached the page; a card that shows a symbol
	// can show the sentence that explains it.
	Doc     string
	Summary string
	// Fields are a type's own fields, listed inside the type's row as the
	// deep view of a part draws them, never as peers of its functions.
	Fields []pageChip
	// Key marks a declaration the model chose as one of its part's keys,
	// the ones the part's tiles draw first and in bold.
	Key bool
	// RunBy are the other programs of this report that run a declaration
	// this program never runs (runByOthers), set only where the page lists
	// what the program never runs.
	RunBy []pageExternal
	// Entry marks the program's launch point where a list names it off the
	// map.
	Entry bool
	// objectID is the listed subject, the target's own object.
	objectID string
}

// SymbolCount counts a file's declarations, a type's fields with them.
func (row pageChipRow) SymbolCount() int {
	count := len(row.Members)
	for _, member := range row.Members {
		count += len(member.Fields)
	}
	return count
}

type pageGroupOperation struct {
	Data                      []pageDataReference
	SummaryRef                string
	Name, Kind, Summary, Href string
	Source                    string
	Anchor                    pageAnchor
}

// pageDoc is one author's sentence shown on a card, under the model's.
type pageDoc struct {
	Symbol string
	Text   string
}

// pageConnection is one model sentence between two groups. A connection to
// another target renders as a stub that links to that target's section.
type pageConnection struct {
	EvidenceID           string
	Native               bool
	LabelRef, SummaryRef string
	Arrow                string
	Title                string
	OtherTarget          string
	Href                 string
	Label                string
	Summary              string
	Possible             bool
	FromSource, ToSource *pageAnchor
	Continues            []pageExternal
	// Count is how many times this same line was said. Three exact calls
	// from one group to another were three identical rows on the card.
	Count int
	// Kind, FromName and ToName are the row as one relation between two
	// named declarations, said through the relation vocabulary (Phrase);
	// FromDecl and ToDecl are those declarations' own anchors, not the call
	// site. A row without them is a sentence of its own and keeps its Label.
	Kind             string
	FromName, ToName string
	FromDecl, ToDecl *pageAnchor
	// fromSubject and at are the declaration a call is written in and where,
	// when the connection is a native call: the order an entrypoint is read
	// forward in. They are never shown.
	fromSubject string
	at          *programindex.Location
	// fromTarget, toTarget and toSubject name the row's two declarations in
	// their programs, so the reading column's data can say what kind of
	// declaration each end is.
	fromTarget, toTarget, toSubject string
	// input says the other end (Href) is an input, not a part.
	input bool
}

// buildSections creates one section per analyzed target and fills it from the
// fact layer and that target's group graph. Sections are created first so
// every later builder can link a fact or a connection to its owning section.
func (builder *pageBuilder) buildSections() {
	builder.createSections()
	overview := builder.overviewBuilder()
	builder.overviewIndexes = overview.indexes
	builder.testPaths = overview.testPaths
	for _, section := range builder.sections {
		builder.fillSectionFacts(section)
		builder.fillSectionOffMap(section)
		section.Map = overview.buildMap(section)
		overview.fillSectionGroups(section)
		overview.fillSectionOperations(section)
		overview.fillSectionOutbound(section)
		overview.fillSectionData(section)
		section.BuiltFrom = builder.builtFrom(section.programTargetID)
		section.Libraries = builder.libraryFacet(section.programTargetID)
		section.InboundCount = section.NativeRouteCount() + len(section.Requests)
		section.InputsCount = section.InboundCount + len(section.Activities)
		section.Coverage = sectionCoverage(section)
		var shown map[string]bool
		section.Flow, shown = builder.flow(section)
		if shown == nil {
			shown = map[string]bool{}
		}
		section.OwnWork = builder.ownWork(section, section.Flow, shown)
		if section.Flow == nil {
			if index := builder.graphIndex(section.programTargetID); index != nil {
				section.Start = builder.startSteps(section, *index)
			}
		}
		if section.Flow == nil {
			section.FlowMissing = "This run produced no main flow."
			if builder.data.Orientation != nil && len(builder.data.Orientation.MainFlow.Steps) > 0 {
				section.FlowMissing = "The model did not include this target in its selected main flow."
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
			ID:              index.Target.ID,
			Name:            index.Target.Name,
			Language:        index.Target.Language,
			Kind:            index.Target.Kind,
			programTargetID: index.Target.ID,
		}
		if target, ok := builder.factsTargetFor(index.Target.ID, used); ok {
			section.factsTargetID = target.ID
			section.FactsAvailable = true
			section.Root = target.Root
			if orient := builder.data.Orientation; orient != nil {
				for _, role := range orient.Roles {
					if role.TargetID == target.ID {
						section.Role, section.Purpose = role.Role, role.Purpose
						break
					}
				}
			}
			used[target.ID] = struct{}{}
			builder.byFacts[target.ID] = section
		}
		section.title = programTitle(index.Target, section.Root)
		builder.byProgram[index.Target.ID] = section
		builder.sections = append(builder.sections, section)
	}
	labelSections(builder.sections)
	for _, section := range builder.sections {
		index := builder.graphIndex(section.programTargetID)
		if index == nil {
			continue
		}
		for _, id := range index.SharedCode {
			if peer := builder.byProgram[id]; peer != nil {
				section.SharedCode = append(section.SharedCode, pageExternal{Name: peer.Label, Href: "#" + peer.ID})
			}
		}
	}
}

// labelSections keeps every target distinguishable in the navigation. Two
// targets can legitimately share a name — a library and a command in one
// directory — so a repeated name gains the detail that separates them. A
// program titled otherwise than by its directory (programTitle) takes that
// title when no other program does.
func labelSections(sections []*pageSection) {
	count := make(map[string]int, len(sections))
	roots := make(map[string]int, len(sections))
	rootKinds := make(map[string]int, len(sections))
	for _, section := range sections {
		count[section.Name]++
		roots[section.Root]++
		rootKinds[section.Root+"\x00"+section.Kind]++
	}
	titled := titledLabels(sections)
	for _, section := range sections {
		if titled[section] {
			continue
		}
		section.ShortLabel = section.Root
		if section.Root == "." {
			section.ShortLabel = section.Name
		}
		if section.Root == "" {
			section.ShortLabel = section.Name
		}
		if roots[section.Root] > 1 {
			if rootKinds[section.Root+"\x00"+section.Kind] > 1 {
				section.ShortLabel = section.Name
			}
			section.ShortLabel += " (" + section.Kind + ")"
		}
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

// factsTargetFor matches the same compact target identity across graph and facts.
func (builder *pageBuilder) factsTargetFor(
	programTargetID string,
	used map[string]struct{},
) (facts.Target, bool) {
	if builder.data.Facts == nil {
		return facts.Target{}, false
	}
	for _, target := range builder.data.Facts.Targets {
		if target.ID != programTargetID {
			continue
		}
		if _, taken := used[target.ID]; taken {
			continue
		}
		return target, true
	}
	return facts.Target{}, false
}

func (builder *pageBuilder) fillSectionFacts(section *pageSection) {
	if !section.FactsAvailable {
		return
	}
	section.Calls = builder.outboundRequests(section.programTargetID, "")
	for _, fact := range builder.targetFacts(section.factsTargetID, facts.KindEntrypoint) {
		section.Entrypoints = append(section.Entrypoints, pageEntrypoint{
			Symbol: shortEntrypointName(fact.Symbol),
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
		if vendoredPath(factSourcePath(fact)) {
			continue
		}
		section.Config = append(section.Config, pageConfig{
			Key: fact.Key, Default: fact.Value, Anchor: builder.links.factAnchor(fact),
		})
	}
	// Manifest settings are configuration too: the port a proxy points at or
	// the command a script runs answers the same reader question.
	for _, fact := range builder.targetFacts(section.factsTargetID, facts.KindManifest) {
		if vendoredPath(factSourcePath(fact)) {
			continue
		}
		section.Config = append(section.Config, pageConfig{
			Key: fact.Key, Default: fact.Value, Anchor: builder.links.factAnchor(fact),
		})
	}
	// Settings read in code and settings declared in manifests read as one
	// table; sorted by their source file they group themselves.
	sort.SliceStable(section.Config, func(i, j int) bool {
		a, b := section.Config[i], section.Config[j]
		pa, pb := "", ""
		if a.Anchor != nil {
			pa = a.Anchor.Path
		}
		if b.Anchor != nil {
			pb = b.Anchor.Path
		}
		if pa != pb {
			return pa < pb
		}
		return a.Key < b.Key
	})
	for _, fact := range builder.targetFacts(section.factsTargetID, facts.KindDeadModule) {
		if anchor := builder.links.factAnchor(fact); anchor != nil {
			section.Dead = append(section.Dead, builder.links.anchor(anchor.Path, 0, 0))
		}
	}
	for _, fact := range builder.targetFacts(section.factsTargetID, facts.KindTODO) {
		if vendoredPath(factSourcePath(fact)) {
			continue
		}
		section.Todos = append(section.Todos, pageTodo{
			Text: fact.Text, Anchor: builder.links.factAnchor(fact),
		})
	}
	section.DeadFolders = groupFiles(section.Dead)
	section.TodoFiles = groupTodos(section.Todos)
	section.Unreached = builder.unreachedRows(section.programTargetID)
	for _, row := range section.Unreached {
		section.UnreachedCount += len(row.Members)
	}
}

// unreachedRows are the declarations of a program its adapter proved nothing
// it runs reaches (ProgramIndex `unreachable`), by file in line order.
func (builder *pageBuilder) unreachedRows(programTargetID string) []pageChipRow {
	if builder.data.ProgramPortfolio == nil {
		return nil
	}
	for _, index := range builder.data.ProgramPortfolio.Entries {
		if index.Target.ID != programTargetID {
			continue
		}
		// A part the program never runs lists its own declarations.
		listed := map[string]bool{}
		if graph := builder.graphIndex(programTargetID); graph != nil {
			for _, file := range graph.OffMap {
				if file.Reason == groupindex.OffMapUnreachable {
					for _, id := range file.SubjectIDs {
						listed[id] = true
					}
				}
			}
		}
		var ids []string
		for _, object := range index.Objects {
			if object.Unreachable && !listed[object.ID] {
				ids = append(ids, object.ID)
			}
		}
		rows, _ := builder.memberChips(programTargetID, ids)
		for _, row := range rows {
			builder.markRunBy(programTargetID, row.Members)
		}
		return rows
	}
	return nil
}

// runByOthers joins the programs' saved `unreachable` facts (ProgramIndex;
// only the C adapter proves them) across the targets of this report, by the
// declaration identity GroupsIndex already uses across programs
// (groupindex.DeclarationKey: path, line, column, kind and name). It walks no
// graph: each program's adapter already decided what that program runs.
// A program runs a declaration when its index holds it, does not mark it
// unreachable, and marks something else unreachable: an adapter marks a
// program's callables all or not at all, and an index that marks nothing
// (a library, a program other code can enter by any name, every other
// adapter) proves nothing here. redis-server never runs aeStop, and
// redis-benchmark does.
type runByOthers struct {
	// programs are, by declaration key, the targets that run it.
	programs map[string][]string
	// keys are the unreachable callables' declaration keys, by
	// target-qualified object ID.
	keys map[string]string
}

func (builder *pageBuilder) runByJoin() *runByOthers {
	if builder.runBy != nil {
		return builder.runBy
	}
	join := &runByOthers{programs: make(map[string][]string), keys: make(map[string]string)}
	builder.runBy = join
	if builder.data.ProgramPortfolio == nil {
		return join
	}
	for _, index := range builder.data.ProgramPortfolio.Entries {
		if !slices.ContainsFunc(index.Objects, func(object programindex.Object) bool { return object.Unreachable }) {
			continue
		}
		for _, object := range index.Objects {
			key := groupindex.DeclarationKey(object)
			if key == "" || !object.Kind.Callable() {
				continue
			}
			if object.Unreachable {
				join.keys[subjectKey(index.Target.ID, object.ID)] = key
				continue
			}
			if !slices.Contains(join.programs[key], index.Target.ID) {
				join.programs[key] = append(join.programs[key], index.Target.ID)
			}
		}
	}
	return join
}

// markRunBy names, beside each declaration its program never runs, the
// other programs of this report that run it, in the page's order, each
// linking to its component.
func (builder *pageBuilder) markRunBy(targetID string, chips []pageChip) {
	join := builder.runByJoin()
	for i := range chips {
		programs := join.programs[join.keys[subjectKey(targetID, chips[i].objectID)]]
		if len(programs) == 0 {
			continue
		}
		for _, section := range builder.sections {
			if section.programTargetID != targetID && slices.Contains(programs, section.programTargetID) {
				chips[i].RunBy = append(chips[i].RunBy, pageExternal{Name: section.Label, Href: "#" + section.ID})
			}
		}
	}
}

// vendoredPath reports a path inside a vendored or generated dependency
// tree. Its TODO markers, manifests and SQL-looking strings describe the
// dependency's authors' work, not this repository's; the owner's audit found
// TODO lists that were vendor-only (gop: 43 of 43 files, meetup: 138 of 139)
// and a Go error string from vendor/ presented as the frontend's one SQL text.
// factSourcePath is where a fact was observed: its anchor when it has one,
// else the path it names.
func factSourcePath(fact facts.Fact) string {
	if fact.Anchor != nil && fact.Anchor.Path != "" {
		return fact.Anchor.Path
	}
	return fact.Path
}

func vendoredPath(path string) bool {
	for _, segment := range strings.Split(path, "/") {
		switch segment {
		case "vendor", "node_modules", "third_party", ".terraform", ".venv", "venv", "site-packages":
			return true
		}
	}
	return false
}

func groupFiles(files []pageAnchor) []pageFileFolder {
	byPath := make(map[string][]pageAnchor)
	for _, file := range files {
		folder := path.Dir(file.Path)
		file.Text = path.Base(file.Path)
		byPath[folder] = append(byPath[folder], file)
	}
	var groups []pageFileFolder
	for folder, files := range byPath {
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		groups = append(groups, pageFileFolder{folder, files})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Path < groups[j].Path })
	return groups
}

func groupTodos(rows []pageTodo) []pageTodoFile {
	byPath := make(map[string][]pageTodo)
	for _, row := range rows {
		file := "Source unavailable"
		if row.Anchor != nil {
			anchor := *row.Anchor
			file = anchor.Path
			anchor.Text = "line " + strconv.Itoa(anchor.Line)
			row.Anchor = &anchor
		}
		byPath[file] = append(byPath[file], row)
	}
	var groups []pageTodoFile
	for file, rows := range byPath {
		groups = append(groups, pageTodoFile{file, rows})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Path < groups[j].Path })
	return groups
}

// routeGroups buckets the target's routes by method so a reader scans one
// verb at a time instead of a flat list.
func groupRouteRows(rows []pageHTTPRow) []pageRouteGroup {
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
		path := pageRoutePath{Path: row.Path, Anchor: row.Anchor, Possible: row.Possible, OperationHrefs: row.OperationHrefs}
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
	zoneOfGroup := make(map[string]groupindex.Container)
	for _, container := range index.Containers {
		for _, id := range container.GroupIDs {
			zoneOfGroup[id] = container
		}
	}
	for _, group := range index.Groups {
		card := builder.groupCard(section.ID, *index, group)
		if zone, inZone := zoneOfGroup[group.ID]; inZone {
			card.Zone = zone.Title
			// Operations have a different map layout without zone frames.
			// Keep the grouping label, but link only to a frame actually drawn.
			if section.Map != nil {
				for _, frame := range section.Map.Frames {
					if frame.ID == zone.ID {
						card.ZoneHref = "#" + frame.ID
						break
					}
				}
			}
		}
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

func (builder *pageBuilder) siblingForExternal(external *programindex.ExternalSymbol, targetID string) *pageSection {
	if external.RepositoryPath != "" {
		for _, section := range builder.sections {
			if section.Root == external.RepositoryPath && (section.Language == "javascript" || section.Language == "typescript") {
				return section
			}
		}
		return nil
	}
	if owner := builder.byProgram[targetID]; owner != nil && (owner.Language == "javascript" || owner.Language == "typescript") {
		// A matching package name does not establish a repository origin.
		return nil
	}
	return builder.siblingByPackage(external.PackagePath)
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
			for _, object := range entry.Objects {
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
		ID: groupAnchorID(sectionID, group.ID), NodeID: targetMapNodeID(index.Target.ID, mapNodeID(group.ID)), Title: group.Title,
		// A summary that is the title again is the title said twice.
		Summary: dropEcho(group.Summary, group.Title),
		Members: len(group.MemberSubjectIDs), Share: laneShare(index, group),
	}
	for _, operation := range index.Operations {
		// A part lists what it implements; an input whose handler is not
		// established is only declared there.
		if operation.GroupID != group.ID || operation.HandlerUnknown {
			continue
		}
		card.Operations = append(card.Operations, pageGroupOperation{
			Name: builder.operationDisplayName(index.Target.ID, operation), Kind: operation.Kind, Summary: operation.Summary, Source: operation.Source,
			Href:   "#" + operationNodeID(sectionID, operation.ID),
			Anchor: builder.links.anchor(operation.Location.Path, operation.Location.Line, operation.Location.Column),
		})
	}
	card.Connections = builder.groupConnections(index, group)
	card.InternalConnections = builder.internalGroupConnections(index, group)
	card.Reading = builder.groupReading(index, group, card)
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

func (builder *pageBuilder) memberChips(targetID string, memberIDs []string) ([]pageChipRow, []pageExternal) {
	type listed struct {
		id, owner string
		chip      pageChip
	}
	byPath := make(map[string][]listed)
	seen := make(map[string]struct{}, len(memberIDs))
	var externals []pageExternal
	for _, id := range memberIDs {
		ref, known := builder.subject(targetID, id)
		if !known {
			continue
		}
		name, anchor := builder.subjectDisplay(ref.subject)
		if name == "" {
			continue
		}
		if anchor == nil {
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			row := pageExternal{Name: name}
			if object := ref.subject.Object; object != nil && object.External != nil {
				if sibling := builder.siblingForExternal(object.External, ref.programTargetID); sibling != nil {
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
		chip := pageChip{
			Name: name, Line: anchor.Line, Anchor: *anchor,
			Doc: builder.docstringFor(anchor.Path, anchor.Line), objectID: id,
		}
		if ref.subject.Interpretation != nil {
			chip.Summary = ref.subject.Interpretation.Line
			chip.Alias = ref.subject.Interpretation.Alias
			chip.Key = ref.subject.Interpretation.Key
		}
		// A variable its type owns is a field of that type.
		owner := ""
		if object := ref.subject.Object; object != nil && object.Kind == programindex.ObjectVariable && object.OwnerID != "" {
			if typed, known := builder.subject(targetID, object.OwnerID); known && typed.subject.Object != nil && typed.subject.Object.Kind == programindex.ObjectType {
				owner = object.OwnerID
			}
		}
		byPath[anchor.Path] = append(byPath[anchor.Path], listed{id: id, owner: owner, chip: chip})
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	rows := make([]pageChipRow, 0, len(paths))
	for _, path := range paths {
		items := byPath[path]
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].chip.Line != items[j].chip.Line {
				return items[i].chip.Line < items[j].chip.Line
			}
			return items[i].chip.Name < items[j].chip.Name
		})
		types := make(map[string]int)
		for i, item := range items {
			types[item.id] = i
		}
		var fields = make(map[int][]pageChip)
		var top []int
		for i, item := range items {
			if at, inside := types[item.owner]; inside && item.owner != "" {
				fields[at] = append(fields[at], item.chip)
				continue
			}
			top = append(top, i)
		}
		chips := make([]pageChip, 0, len(top))
		for _, i := range top {
			chip := items[i].chip
			chip.Fields = fields[i]
			chips = append(chips, chip)
		}
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

// groupConnections renders the model's one-line sentences for every
// connection incident to this group. A connection whose other endpoint lives
// in another target becomes a stub linking to that target's section.
func (builder *pageBuilder) groupConnections(
	index groupindex.Index,
	group groupindex.Group,
) []pageConnection {
	here := groupindex.Endpoint{TargetID: index.Target.ID, GroupID: group.ID}
	incident := builder.incidentConnections()
	// Each other target's inputs by where they are written, the first of a
	// place in the index's order, and this target's integrations by the
	// group they leave, each read once a call: read again for every
	// connection, they took headscale's render 300 s once its library's
	// 1,788 exports were entrypoints, each asking its group's connections.
	inputsAt := make(map[string]map[string]groupindex.Operation)
	inputAt := func(other *groupindex.Index, location programindex.Location) (groupindex.Operation, bool) {
		at, built := inputsAt[other.Target.ID]
		if !built {
			at = make(map[string]groupindex.Operation, len(other.Operations))
			for _, operation := range other.Operations {
				key := operationLocationKey(operation.Location)
				if _, first := at[key]; !first {
					at[key] = operation
				}
			}
			inputsAt[other.Target.ID] = at
		}
		operation, found := at[operationLocationKey(location)]
		return operation, found
	}
	var leaving map[groupindex.Endpoint][]groupindex.Connection
	var rows []pageConnection
	for _, position := range incident.byEndpoint[here] {
		connection := incident.all[position]
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
			EvidenceID:  connection.SourceID + "\x00" + connection.ToSubjectID,
			Native:      strings.HasPrefix(connection.SourceKind, "native_"),
			Arrow:       arrow,
			Title:       builder.groupTitles[other],
			Label:       connection.Label,
			Summary:     connection.Summary,
			Possible:    connection.SupportResolution == programindex.PatternValuePossible,
			fromSubject: connection.FromSubjectID,
			at:          connection.FromLocation,
			fromTarget:  connection.From.TargetID,
			toTarget:    connection.To.TargetID,
			toSubject:   connection.ToSubjectID,
		}
		if row.Title == "" {
			row.Title = strings.ReplaceAll(connection.SemanticKind, "_", " ")
		}
		builder.nameConnectionEnds(&row, connection)
		if section := builder.byProgram[other.TargetID]; section != nil {
			row.Href = "#" + groupAnchorID(section.ID, other.GroupID)
			if other.TargetID != index.Target.ID {
				row.OtherTarget = section.ShortLabel
			}
			location := connection.ToLocation
			if arrow == "←" {
				location = connection.FromLocation
			}
			if location != nil {
				if otherIndex := builder.graphIndex(other.TargetID); otherIndex != nil {
					if operation, found := inputAt(otherIndex, *location); found {
						row.Href = "#" + operationNodeID(section.ID, operation.ID)
						row.input = true
						row.Title = builder.operationDisplayName(otherIndex.Target.ID, operation)
					}
				}
			}
		}
		if connection.FromLocation != nil {
			location := connection.FromLocation
			row.FromSource = builder.links.anchorPointer(location.Path, location.Line, location.Column)
		}
		if connection.ToLocation != nil {
			location := connection.ToLocation
			row.ToSource = builder.links.anchorPointer(location.Path, location.Line, location.Column)
		}
		// This names the neighbouring group's own integrations; it does not
		// turn a group-level connection into a trace of the selected function.
		if arrow == "→" && other.TargetID == index.Target.ID {
			if leaving == nil {
				leaving = make(map[groupindex.Endpoint][]groupindex.Connection)
				for _, next := range index.Connections {
					if next.To.TargetID != index.Target.ID && next.SourceKind == "integration" {
						leaving[next.From] = append(leaving[next.From], next)
					}
				}
			}
			seen := map[string]bool{}
			for _, next := range leaving[other] {
				if seen[next.To.TargetID] {
					continue
				}
				if peer := builder.byProgram[next.To.TargetID]; peer != nil {
					row.Continues = append(row.Continues, pageExternal{Name: peer.ShortLabel, Href: row.Href})
					seen[next.To.TargetID] = true
				}
			}
		}
		rows = append(rows, row)
	}
	rows = append(rows, builder.nativeGroupConnections(index, group)...)
	return collapseConnections(rows)
}

// collapseConnections says each distinct line once, with how often it was
// said, and does not repeat the label as its own explanation. A card read
// "← Client IP middleware: provides client IP context — provides client IP
// context" three times over; the model's summary of a connection is usually
// its label again, and when it is, it is noise on the line.
func collapseConnections(rows []pageConnection) []pageConnection {
	type key struct {
		arrow, title, otherTarget, href, label, summary, from, to, evidence string
		possible, native                                                    bool
	}
	at := make(map[key]int, len(rows))
	result := make([]pageConnection, 0, len(rows))
	for _, row := range rows {
		row.Summary = dropEcho(row.Summary, row.Label)
		k := key{arrow: row.Arrow, title: row.Title, otherTarget: row.OtherTarget, href: row.Href, label: row.Label, summary: row.Summary, possible: row.Possible, native: row.Native}
		k.evidence = row.EvidenceID
		if row.FromSource != nil {
			k.from = row.FromSource.Text
		}
		if row.ToSource != nil {
			k.to = row.ToSource.Text
		}
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

// connectionEnds finds every connection of the page by the groups at its
// two ends, in allConnections order.
type connectionEnds struct {
	indexes    *groupindex.Index
	count      int
	all        []groupindex.Connection
	byEndpoint map[groupindex.Endpoint][]int
}

func (builder *pageBuilder) incidentConnections() *connectionEnds {
	indexes := firstOf(builder.indexes)
	// Only for the very indexes it was built from: a map copy of the builder
	// reads other ones.
	if builder.connectionEnds != nil && builder.connectionEnds.indexes == indexes && builder.connectionEnds.count == len(builder.indexes) {
		return builder.connectionEnds
	}
	ends := &connectionEnds{indexes: indexes, count: len(builder.indexes), all: builder.allConnections(), byEndpoint: map[groupindex.Endpoint][]int{}}
	for position, connection := range ends.all {
		ends.byEndpoint[connection.From] = append(ends.byEndpoint[connection.From], position)
		if connection.To != connection.From {
			ends.byEndpoint[connection.To] = append(ends.byEndpoint[connection.To], position)
		}
	}
	builder.connectionEnds = ends
	return ends
}

// laneShare is how much of the target one group holds. The same number sizes
// its node on the map, so the card and the picture cannot disagree.
func laneShare(index groupindex.Index, group groupindex.Group) int {
	if len(index.Subjects) == 0 {
		return 0
	}
	return len(group.MemberSubjectIDs) * 100 / len(index.Subjects)
}

// docstringReach is how far above a declaration its docstring may start.
// A Go doc comment sits directly above; a long one starts a dozen lines up.
const docstringReach = 12

// docstringFor is the docstring written above the symbol declared at this
// line of this file, if one was quoted into the claims. A C docstring names
// the declaration it sits on, the same rule places reads it with, so a C
// file's description or a comment above a prototype is no symbol's.
func (builder *pageBuilder) docstringFor(path string, line int) string {
	if claims.CPath(path) {
		return claims.CDocstring(builder.docstrings[path], line, builder.declarations[path])
	}
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
	// A group's connections are read once however many entrypoints it
	// holds: a library's every export is one (headscale's 1,788).
	connectionsOf := make(map[string][]pageConnection)
	var steps []pageStart
	for _, entry := range section.Entrypoints {
		step := pageStart{Symbol: entry.Symbol, Anchor: entry.Anchor}
		if entry.Anchor != nil {
			if subjectID, known := builder.subjectAt[subjectLocationKey(index.Target.ID, entry.Anchor.Path, entry.Anchor.Line)]; known {
				if group, inGroup := groupOf[subjectID]; inGroup {
					step.Group = group.Title
					step.Href = "#" + groupAnchorID(section.ID, group.ID)
					// Its name reads the entry in its part, and its calls open
					// under it in the order they are written, as a model
					// step's do: redis-cli's start had read only as "main in
					// Command line client".
					step.Part, step.Key = step.Href, declarationKeyOf(builder, index.Target.ID, subjectID)
					if ref, known := builder.subject(index.Target.ID, subjectID); known {
						if _, anchor := builder.subjectDisplay(ref.subject); anchor != nil {
							step.Code = cmp.Or(anchor.Code, anchor.Href)
						}
					}
					rows, read := connectionsOf[group.ID]
					if !read {
						rows = builder.groupConnections(index, group)
						connectionsOf[group.ID] = rows
					}
					step.Reaches = startReaches(rows, subjectID, maxStartReaches)
				}
			}
		}
		steps = append(steps, step)
	}
	return steps
}

// startReaches reads an entrypoint forward: the outgoing connections of its
// group, the entrypoint's own calls first and then the group's others, each
// in the order they are written, the first few, each line once. Three call
// sites of main calling aeMain are one step, and the next distinct
// connection takes the freed place. In the connections' stored order,
// grouped by the part they reach, redis-benchmark's start read "main calls
// aeMain" before the aeCreateEventLoop main calls thirty lines earlier.
func startReaches(rows []pageConnection, entry string, most int) []pageConnection {
	own := func(row pageConnection) bool { return entry != "" && row.fromSubject == entry }
	rows = slices.Clone(rows)
	slices.SortStableFunc(rows, func(a, b pageConnection) int {
		switch {
		case own(a) != own(b) && own(a):
			return -1
		case own(a) != own(b):
			return 1
		}
		switch {
		case a.at == nil && b.at == nil:
			return 0
		case a.at == nil:
			return 1
		case b.at == nil:
			return -1
		}
		return cmp.Or(strings.Compare(a.at.Path, b.at.Path), cmp.Compare(a.at.Line, b.at.Line), cmp.Compare(a.at.Column, b.at.Column))
	})
	var out []pageConnection
	shown := map[[3]string]bool{}
	for _, row := range rows {
		line := [3]string{row.Label, row.Title, row.Href}
		if row.Arrow != "→" || len(out) == most || shown[line] {
			continue
		}
		shown[line] = true
		out = append(out, row)
	}
	return out
}
