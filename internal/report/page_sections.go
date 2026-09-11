package report

import (
	"fmt"
	"path"
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

	FactsAvailable bool
	Map            *pageMap
	RouteGroups    []pageRouteGroup
	Requests       []pageGroupOperation
	Activities     []pageGroupOperation
	InputsCount    int
	Outbound       []pageOutbound
	Coverage       []string
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
	Dynamic     []pageDynamic
	Config      []pageConfig
	Dead        []pageAnchor
	Todos       []pageTodo
	DeadFolders []pageFileFolder
	TodoFiles   []pageTodoFile
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
}

// pageGroup is one responsibility card. Members are grouped by file so the
// path is printed once and each chip carries only its line.
type pageGroup struct {
	SummaryRef  string
	ID          string
	Members     int
	Share       int
	Title       string
	Summary     string
	Highlights  []pageChipRow
	Inventory   []pageChipRow
	Operations  []pageGroupOperation
	Externals   []pageExternal
	Connections []pageConnection
	// Zone is the part this group is in, when it is in one, and ZoneHref
	// the frame on the map that draws it.
	Zone     string
	ZoneHref string
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
		section.Map = overview.buildMap(section)
		overview.fillSectionGroups(section)
		overview.fillSectionOperations(section)
		overview.fillSectionOutbound(section)
		overview.fillSectionData(section)
		section.InboundCount = section.NativeRouteCount() + len(section.Requests)
		section.InputsCount = section.InboundCount + len(section.Activities)
		section.Coverage = sectionCoverage(section)
		section.Flow = builder.flow(section)
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
// directory — so a repeated name gains the detail that separates them.
func labelSections(sections []*pageSection) {
	count := make(map[string]int, len(sections))
	roots := make(map[string]int, len(sections))
	rootKinds := make(map[string]int, len(sections))
	for _, section := range sections {
		count[section.Name]++
		roots[section.Root]++
		rootKinds[section.Root+"\x00"+section.Kind]++
	}
	for _, section := range sections {
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
	section.Calls = builder.httpRows(facts.KindHTTPCall, section.factsTargetID)
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
	card.Inventory = rows
	var selected []string
	for _, id := range group.MemberSubjectIDs {
		if ref, ok := builder.subjects[id]; ok && ref.subject.Interpretation != nil && ref.subject.Interpretation.Key {
			selected = append(selected, id)
		}
	}
	card.Highlights, _ = builder.memberChips(selected)
	for _, operation := range index.Operations {
		if operation.GroupID != group.ID {
			continue
		}
		card.Operations = append(card.Operations, pageGroupOperation{
			Name: builder.operationDisplayName(operation), Kind: operation.Kind, Summary: operation.Summary, Source: operation.Source,
			Href:   "#" + operationNodeID(sectionID, operation.ID),
			Anchor: builder.links.anchor(operation.Location.Path, operation.Location.Line, operation.Location.Column),
		})
	}
	card.Connections = builder.groupConnections(index, group)
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
			Doc: builder.docstringFor(anchor.Path, anchor.Line),
		}
		if ref.subject.Interpretation != nil {
			chip.Summary = ref.subject.Interpretation.Line
			chip.Alias = ref.subject.Interpretation.Alias
		}
		byPath[anchor.Path] = append(byPath[anchor.Path], chip)
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
					for _, operation := range otherIndex.Operations {
						if operationLocationKey(operation.Location) == operationLocationKey(*location) {
							row.Href = "#" + operationNodeID(section.ID, operation.ID)
							row.Title = builder.operationDisplayName(operation)
							break
						}
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
			seen := map[string]bool{}
			for _, next := range index.Connections {
				if next.From != other || next.To.TargetID == index.Target.ID || next.SourceKind != "integration" || seen[next.To.TargetID] {
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
	return collapseConnections(rows)
}

// collapseConnections says each distinct line once, with how often it was
// said, and does not repeat the label as its own explanation. A card read
// "← Client IP middleware: provides client IP context — provides client IP
// context" three times over; the model's summary of a connection is usually
// its label again, and when it is, it is noise on the line.
func collapseConnections(rows []pageConnection) []pageConnection {
	type key struct {
		arrow, title, otherTarget, label, from, to string
		possible                                   bool
	}
	at := make(map[key]int, len(rows))
	result := make([]pageConnection, 0, len(rows))
	for _, row := range rows {
		row.Summary = dropEcho(row.Summary, row.Label)
		k := key{arrow: row.Arrow, title: row.Title, otherTarget: row.OtherTarget, label: row.Label, possible: row.Possible}
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
