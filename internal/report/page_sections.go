package report

import (
	"fmt"
	"sort"
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

	FactsAvailable   bool
	RouteGroups      []pageRouteGroup
	Triggers         []pageGroup
	Entrypoints      []pageEntrypoint
	Core             []pageGroup
	Calls            []pageHTTPRow
	DependencyGroups []pageGroup
	Dependencies     []pageDependency
	Flow             *pageFlow
	FlowMissing      string
	Risks            []pageRisk
	Config           []pageConfig
	Dead             []pageAnchor
	Todos            []pageTodo
}

type pageRouteGroup struct {
	Method string
	Rows   []pageHTTPRow
}

type pageEntrypoint struct {
	Symbol string
	Kind   string
	Anchor *pageAnchor
}

type pageDependency struct {
	Name    string
	Version string
	Anchor  *pageAnchor
}

type pageRisk struct {
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
	Title       string
	Summary     string
	Visible     []pageChipRow
	More        []pageChipRow
	MoreCount   int
	Externals   []string
	Connections []pageConnection
}

type pageChipRow struct {
	Path    string
	Members []pageChip
}

type pageChip struct {
	Name   string
	Line   int
	Anchor pageAnchor
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
}

// buildSections creates one section per analyzed target and fills it from the
// fact layer and that target's group graph. Sections are created first so
// every later builder can link a fact or a connection to its owning section.
func (builder *pageBuilder) buildSections() {
	builder.createSections()
	for _, section := range builder.sections {
		builder.fillSectionFacts(section)
		builder.fillSectionGroups(section)
		section.Flow = builder.flow(section)
		if section.Flow == nil {
			section.FlowMissing = notAvailableOrientation
		}
	}
}

// createSections derives the section list from the group graph, which is the
// one authority that exists for every analyzed target, then binds the fact
// target that describes the same root.
func (builder *pageBuilder) createSections() {
	used := make(map[string]struct{}, len(builder.data.GroupGraph.Indexes))
	for position := range builder.data.GroupGraph.Indexes {
		index := &builder.data.GroupGraph.Indexes[position]
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
	for _, fact := range builder.targetFacts(section.factsTargetID, facts.KindRisk) {
		section.Risks = append(section.Risks, pageRisk{
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
		groups = append(groups, pageRouteGroup{Method: method, Rows: byMethod[method]})
	}
	return groups
}

func (builder *pageBuilder) fillSectionGroups(section *pageSection) {
	index := builder.graphIndex(section.programTargetID)
	if index == nil {
		return
	}
	for _, group := range index.Groups {
		card := builder.groupCard(*index, group)
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

func (builder *pageBuilder) graphIndex(programTargetID string) *groupindex.Index {
	for position := range builder.data.GroupGraph.Indexes {
		index := &builder.data.GroupGraph.Indexes[position]
		if index.Target.ID == programTargetID {
			return index
		}
	}
	return nil
}

func (builder *pageBuilder) groupCard(index groupindex.Index, group groupindex.Group) pageGroup {
	card := pageGroup{Title: group.Title, Summary: group.Summary}
	rows, externals := builder.memberChips(group.MemberSubjectIDs)
	card.Externals = externals
	card.Visible, card.More, card.MoreCount = splitChipRows(rows, maxVisibleGroupMembers)
	card.Connections = builder.groupConnections(index, group)
	return card
}

// memberChips resolves member subjects to anchored chips grouped by file and
// deduplicated by path and line, so one file prints its path once. Members
// without a location (external packages) become plain labels.
func (builder *pageBuilder) memberChips(memberIDs []string) ([]pageChipRow, []string) {
	byPath := make(map[string][]pageChip)
	seen := make(map[string]struct{}, len(memberIDs))
	var externals []string
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
			externals = append(externals, name)
			continue
		}
		key := anchor.Path + ":" + name
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		byPath[anchor.Path] = append(byPath[anchor.Path], pageChip{
			Name: name, Line: anchor.Line, Anchor: *anchor,
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
	sort.Strings(externals)
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
			more = append(more, pageChipRow{Path: row.Path, Members: rest})
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
	return rows
}

// allConnections is the complete matched set. A cross-target connection is
// stored once, in the index that owns its source group, so both endpoints
// must look at every index to find it.
func (builder *pageBuilder) allConnections() []groupindex.Connection {
	var rows []groupindex.Connection
	for _, index := range builder.data.GroupGraph.Indexes {
		rows = append(rows, index.Connections...)
	}
	return rows
}
