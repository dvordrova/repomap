package report

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// pageOutbound is one accepted communication record. Destination and Summary
// are display prose; Address, NativeLabel and External retain source spelling.
// No dependency-group membership is required and no package import creates one.
type pageOutbound struct {
	// Connections keep exact saved integration identities for the canvas.
	// Destination text groups presentation only and never establishes a peer.
	Connections []string
	MapGroup    string
	Operations  string
	KindLabel   string
	Uses        []pageOutboundUse
	// reached are the callers outside its part the program reaches the
	// call from (GroupsIndex ReachedFrom), read with its tile; program is
	// the component making it.
	reached []pageReachedName
	program string
	// made is the declaration the call is written in, in its part
	// (OutboundCall.SubjectID and GroupID): the part the canvas stands the
	// call's destination by, said beside "Called from" (casdoor's Custom
	// Logout Endpoint is made in Core data models' callProviderLogoutUrl,
	// called from API controllers' ApiController.Logout).
	made *pageReachedName
	// Caller is the declaration the outside call is written in; Side the
	// program's own code reaching it (page_shared_code.go), which says the
	// program connects out.
	Caller                      string
	Side                        *pageCallSide
	CallerAnchor                pageAnchor
	ID                          string
	Destination, DestinationRef string
	// Alternatives are the systems the destination is one of, depending on
	// configuration (GroupsIndex OutboundCall.Alternatives); Destination
	// names them all.
	Alternatives            []string
	Summary, SummaryRef     string
	Address, NativeLabel    string
	External, Basis, Source string
	Method                  string
	// LineWithType keeps the callable's type on the line because another
	// record of the same destination has the same member on another type:
	// "PodInterface.Patch" and "DeploymentInterface.Patch", not "Patch, Patch".
	LineWithType bool
	Anchor       pageAnchor
	// Program marks a call that starts another program; Words are every
	// word the call writes, as written, and Destination the one that names
	// the program. ProgramNotNamed says none of them names it.
	Program         bool
	ProgramNotNamed bool
	Words           []string
	// Runs are this repository's programs the record reaches: for a
	// started program, those whose executable is named what it is
	// (programsNamed: equal names are the code fact, "runs litestream" is
	// cmd/litestream); for another record, the program its destination is
	// (joinOwnPrograms). Its arrow goes into that program's component.
	Runs []pageRunsProgram
}

// pageRunsProgram is one program of the report a started program's name
// names: its component's section and title.
type pageRunsProgram struct {
	Section, Href, Title string
}

// programsNamed are the report's programs whose build gives their
// executable this name (ProgramIndex target executables: C's link output,
// a Go main package's directory, a Python console_script, a package.json
// bin command), in the report's order. Only equal names join; nothing is
// matched loosely.
func (builder *pageBuilder) programsNamed(name string) []pageRunsProgram {
	if name == "" {
		return nil
	}
	var programs []pageRunsProgram
	for _, section := range builder.sections {
		index := builder.graphIndex(section.programTargetID)
		if index == nil || !slices.Contains(index.Target.Executables, name) {
			continue
		}
		programs = append(programs, pageRunsProgram{Section: section.ID, Href: "#" + section.ID, Title: componentTitle(section, builder.sections)})
	}
	return programs
}

// ProgramLabel names a started program no word of its call names: one
// whose name the code computes, or one the model did not decide. Such a
// launch is read under "What is missing" (unnamedLaunch).
func (row pageOutbound) ProgramLabel() string {
	switch {
	case !row.Program || row.Destination != "":
		return ""
	case row.ProgramNotNamed:
		return "A program named at run time"
	default:
		return "Program not established"
	}
}

// pageOutboundUse is one chain the walk read for the call's argument: the
// value it ends in (DestinationUse.Address, as written) is argument
// evidence, never the call's address. The address is only the boundary's
// accepted one (pageOutbound.Address): casdoor's oss Put printed its object
// key "%s/%s" as "Address" eleven times after the decision said unknown.
type pageOutboundUse struct {
	Value, Frontier, Method string
	// Unread says the walk ended at a value its adapter could not read: the
	// value is not established from code (atlas DestinationUse Unread).
	Unread bool
	Steps  []pageOutboundStep
	// Routes is how many saved chains reach this end, Steps the shortest
	// of them: each end stands once (outboundEnds). Sources are the other
	// steps on its routes (GroupsIndex OutboundCall.Graph), read when the
	// reader expands the end.
	Routes  int
	Sources []pageOutboundStep
}

// FrontierName is the frontier the address passes through, or "" when it
// names nothing: freqtrade's getattr(ccxt, name)(config) is a frontier
// "()", which printed "Address passes through ()". The chain's steps stand.
// An unread frontier names nothing either: Redis's connect had read
// "Address passes through (struct sockaddr*)&sa", the expression its
// address is cast from.
func (use pageOutboundUse) FrontierName() string {
	if use.Unread || strings.Trim(use.Frontier, "() \t") == "" {
		return ""
	}
	return use.Frontier
}

type pageOutboundStep struct {
	Name   string
	Anchor pageAnchor
}

// AddressText expands the destination reader's leading configuration notation
// for display only. It never resolves a setting or changes the saved address.
type pageOutboundAddress struct {
	Text, Setting, SettingLabel, Suffix string
}

// displayCallable drops the program index's platform notation from a
// callable's name: "platform:javascript.WebSocket" reads WebSocket on the
// page while the saved atlas keeps the original spelling.
func displayCallable(name string) string {
	rest, ok := strings.CutPrefix(name, "platform:")
	if !ok {
		return name
	}
	if _, after, found := strings.Cut(rest, "."); found && after != "" {
		return after
	}
	return rest
}

func outboundAddressText(address string) pageOutboundAddress {
	result := pageOutboundAddress{Text: address}
	prefix, label := "{--", "Address from command-line option"
	if strings.HasPrefix(address, "{env:") {
		prefix, label = "{env:", "Address from environment variable"
	}
	if !strings.HasPrefix(address, prefix) {
		return result
	}
	name, suffix, found := strings.Cut(strings.TrimPrefix(address, prefix), "}")
	if !found || name == "" || strings.ContainsAny(name, "{} \t\r\n") || strings.ContainsAny(suffix, "{}") {
		return result
	}
	result.Setting, result.SettingLabel, result.Suffix = name, label, suffix
	if prefix == "{--" {
		result.Setting = "--" + name
	}
	return result
}

func (row pageOutbound) AddressText() pageOutboundAddress { return outboundAddressText(row.Address) }

// ValueText is a chain's value with its setting spelled out, labelled as
// the argument's value it is.
func (use pageOutboundUse) ValueText() pageOutboundAddress {
	text := outboundAddressText(use.Value)
	switch text.SettingLabel {
	case "Address from command-line option":
		text.SettingLabel = "Value from command-line option"
	case "Address from environment variable":
		text.SettingLabel = "Value from environment variable"
	}
	return text
}

func (builder *pageBuilder) fillSectionOutbound(section *pageSection) {
	index := builder.graphIndex(section.programTargetID)
	if index == nil {
		return
	}
	// The inputs reaching a call are those whose saved reach holds the
	// declaration that makes it.
	reachable := make(map[string]map[string]bool)
	for position, operation := range index.Operations {
		reached := map[string]bool{}
		if position < len(index.Reach) {
			for _, subject := range index.Reach[position].Subjects {
				reached[subject.SubjectID] = true
			}
		}
		reachable[operationNodeID(section.ID, operation.ID)] = reached
	}
	for _, call := range index.Outbound {
		row := pageOutbound{
			MapGroup:    call.GroupID,
			KindLabel:   outboundKindLabel(call.Kind),
			ID:          section.ID + "-out-" + call.ID,
			Destination: call.Destination, Alternatives: slices.Clone(call.Alternatives), Summary: call.Summary,
			Address: call.Address, External: displayCallable(call.External), Basis: call.Basis, Source: call.Source, Method: call.Method,
			Anchor: builder.links.anchor(call.Location.Path, call.Location.Line, call.Location.Column),
		}
		if call.Kind == atlas.BoundaryRunsProgram {
			row.Program, row.ProgramNotNamed = true, call.ProgramNotNamed
			row.Runs = builder.programsNamed(call.Destination)
			// A word that cannot stand on one line (a script handed to an
			// interpreter) stays in the call at its source link.
			for _, word := range call.Values {
				if !strings.ContainsAny(word, "\n\r") {
					row.Words = append(row.Words, word)
				}
			}
		}
		row.reached, row.program = builder.outboundReached(index, section, call), componentTitle(section, builder.sections)
		if made, ok := builder.reachedName(index, section, call.SubjectID, call.GroupID, &call.Location, false); ok {
			row.made = &made
		}
		if ref, known := builder.subject(index.Target.ID, call.SubjectID); known && call.SubjectID != "" {
			name, anchor := builder.subjectDisplay(ref.subject)
			row.Caller = name
			if anchor != nil {
				row.CallerAnchor = *anchor
			}
			row.Side = builder.callSide(index.Target.ID, call.SubjectID)
		}
		var inputs []string
		for _, connection := range index.Connections {
			if connection.SourceKind == "integration" && connection.From.TargetID == index.Target.ID &&
				connection.FromSubjectID == call.SubjectID && call.SubjectID != "" &&
				connection.FromLocation != nil && *connection.FromLocation == call.Location {
				row.Connections = append(row.Connections, connectionKey(index.Target.ID, connection.ID))
			}
		}
		for id, reached := range reachable {
			if call.SubjectID != "" && reached[call.SubjectID] {
				inputs = append(inputs, id)
			}
		}
		sort.Strings(inputs)
		row.Operations = strings.Join(inputs, " ")
		ends, routes := outboundEnds(call.Uses)
		for position, use := range ends {
			value := pageOutboundUse{Value: use.Address, Frontier: use.Frontier, Unread: use.Unread, Method: use.Method, Routes: routes[position]}
			named := func(step atlas.DestinationStep) (string, pageAnchor) {
				name := step.Name
				// A declaration reads by its report name: a method with its
				// type, a callable written inline as a reader names it, never
				// Run$1 (owner, 2026-09-29).
				if step.SubjectID != "" {
					if ref, known := builder.subject(index.Target.ID, step.SubjectID); known && ref.subject.Object != nil {
						if said, _ := builder.subjectDisplay(ref.subject); said != "" {
							name = builder.withType(index.Target.ID, ref.subject, said)
						}
					}
				}
				// Existing operation subjects already own their interpreted
				// names. Matching the original declaration binds this source
				// chain to that operation without inventing another label.
				for _, operation := range index.Operations {
					if step.SubjectID != "" && operation.SubjectID == step.SubjectID {
						name = builder.operationDisplayName(index.Target.ID, operation)
						break
					}
				}
				return name, builder.links.anchor(step.Path, step.Line, step.Column)
			}
			onChain := map[atlas.DestinationStep]bool{}
			for _, step := range use.Steps {
				onChain[step] = true
				name, anchor := named(step)
				// Two steps in one declaration on one line read as one:
				// freqtrade's start_install_ui and its dl_url both read
				// "install-ui".
				if last := len(value.Steps) - 1; last >= 0 && value.Steps[last].Name == name && value.Steps[last].Anchor.Href == anchor.Href && value.Steps[last].Anchor.Text == anchor.Text {
					continue
				}
				value.Steps = append(value.Steps, pageOutboundStep{Name: name, Anchor: anchor})
			}
			if call.Graph != nil {
				for _, at := range use.Through {
					if at < 0 || at >= len(call.Graph.Steps) || onChain[call.Graph.Steps[at]] {
						continue
					}
					name, anchor := named(call.Graph.Steps[at])
					value.Sources = append(value.Sources, pageOutboundStep{Name: name, Anchor: anchor})
				}
			}
			row.Uses = append(row.Uses, value)
		}
		// Native HTTP facts may have no semantic label. Compose only their
		// supplied method and address, never a guessed destination.
		if call.Method != "" && call.Address != "" {
			row.NativeLabel = call.Method + " " + call.Address
		} else {
			row.NativeLabel = displayCallable(call.External)
		}
		// A program no word of its call names is an unknown about that one
		// call, no outside system: it is read with the call's function and
		// under "What is missing", never as a destination, a frame or a
		// connection (unnamedLaunch).
		if unnamedLaunch(call) {
			section.UnnamedLaunches = append(section.UnnamedLaunches, row)
			continue
		}
		section.Outbound = append(section.Outbound, row)
	}
	builder.joinOwnPrograms(index, section)
}

// unnamedLaunch reports a call starting another program whose program the
// code does not name: none of its words does (ProgramNotNamed), or it was
// given none, or no answer chose one. It names no destination.
func unnamedLaunch(call groupindex.OutboundCall) bool {
	return call.Kind == atlas.BoundaryRunsProgram && call.Destination == ""
}

// joinOwnPrograms makes a destination that is one of this repository's own
// programs that program. A record whose integration connection (a joint the
// reading confirmed by protocol and input) ends in another program of the
// report says the destination it names is that program: redis-cli's
// connect reaches redis-server's listening socket, so its "Redis server",
// the connect and the gethostbyname resolving the server's host, is
// redis-server. Every record of the destination then reaches that program
// (Runs), whose component its arrow goes into: no outside system stands for
// it. A destination whose records reach several programs, or none, stays
// as it is; a started program keeps the program its word names.
func (builder *pageBuilder) joinOwnPrograms(index *groupindex.Index, section *pageSection) {
	peers := make(map[string]string)
	for _, connection := range index.Connections {
		if connection.SourceKind == "integration" && connection.To.TargetID != index.Target.ID {
			peers[connectionKey(index.Target.ID, connection.ID)] = connection.To.TargetID
		}
	}
	reached := make(map[string]map[string]bool)
	// A destination the reading chose as one of the report's programs
	// (GroupsIndex DestinationTarget) is that program: freqtrade-client's
	// one generic request, whose path the code computes, reaches no one of
	// freqtrade's inputs, and it had stood as "Freqtrade Server" outside.
	for _, call := range index.Outbound {
		if name := strings.ToLower(canonicalDestination(call.Destination)); call.DestinationTarget != "" && name != "" && call.Kind != atlas.BoundaryRunsProgram {
			if reached[name] == nil {
				reached[name] = make(map[string]bool)
			}
			reached[name][call.DestinationTarget] = true
		}
	}
	for _, row := range section.Outbound {
		name := strings.ToLower(canonicalDestination(row.Destination))
		if row.Program || name == "" {
			continue
		}
		for _, connection := range row.Connections {
			if peer := peers[connection]; peer != "" {
				if reached[name] == nil {
					reached[name] = make(map[string]bool)
				}
				reached[name][peer] = true
			}
		}
	}
	for i := range section.Outbound {
		row := &section.Outbound[i]
		name := strings.ToLower(canonicalDestination(row.Destination))
		if row.Program || len(reached[name]) != 1 {
			continue
		}
		for target := range reached[name] {
			if peer := builder.byProgram[target]; peer != nil && peer != section {
				row.Runs = []pageRunsProgram{{Section: peer.ID, Href: "#" + peer.ID, Title: componentTitle(peer, builder.sections)}}
			}
		}
	}
}

// pageReachedName is one caller an outside call is reached from, as its
// tile's reading names it: the declaration, read in the part it stands in,
// and the place it makes the call into the call's part, said on hover.
type pageReachedName struct {
	decl                 pageReadingDecl
	part, title, program string
	site                 pageAnchor
	// possible marks a caller reached through a call resolved to
	// alternatives (GroupsIndex OutboundCaller.Possible).
	possible bool
}

// outboundReached names the callers a call is reached from, in the saved
// order: each part's, then each path's start inside the call's own part.
func (builder *pageBuilder) outboundReached(index *groupindex.Index, section *pageSection, call groupindex.OutboundCall) []pageReachedName {
	var names []pageReachedName
	for _, caller := range call.ReachedFrom {
		if name, ok := builder.reachedName(index, section, caller.SubjectID, caller.GroupID, caller.Location, caller.Possible); ok {
			names = append(names, name)
		}
	}
	return names
}

// reachedName is a declaration an outside call is made in or reached from,
// read in its part, with the place it makes its call.
func (builder *pageBuilder) reachedName(index *groupindex.Index, section *pageSection, subjectID, groupID string, location *programindex.Location, possible bool) (pageReachedName, bool) {
	ref, known := builder.subject(index.Target.ID, subjectID)
	if subjectID == "" || !known {
		return pageReachedName{}, false
	}
	label, anchor := builder.subjectDisplay(ref.subject)
	key := declarationKey(anchor)
	if label == "" || key == "" {
		return pageReachedName{}, false
	}
	name := pageReachedName{program: componentTitle(section, builder.sections), possible: possible}
	if groupID != "" {
		name.part = "#" + groupAnchorID(section.ID, groupID)
		name.title = builder.groupTitles[groupindex.Endpoint{TargetID: index.Target.ID, GroupID: groupID}]
	}
	kind := ""
	if object := ref.subject.Object; object != nil && (object.Kind == programindex.ObjectFunction || object.Kind == programindex.ObjectMethod) {
		kind = "function"
	}
	name.decl = pageReadingDecl{Name: builder.withType(index.Target.ID, ref.subject, label), Key: key, Anonymous: anchor.words, Href: anchor.Href, Open: anchor.Open, NoSource: anchor.NoSource,
		Code: anchor.Code, At: anchor.Text, File: anchor.Path, Kind: kind, Part: name.part}
	if location != nil {
		name.site = builder.links.anchor(location.Path, location.Line, location.Column)
	}
	return name, true
}

// pageReached is the reading of the callers an outside call's tile is
// reached from (31-reading-column.js rmReachedFrom): the declarations once,
// and each part's callers by name, the tile's own program first and every
// other program's part named with its program. No line number is written:
// a name reads its function, its call's place said on hover.
type pageReached struct {
	Decls  []pageReadingDecl      `json:"decls"`
	Groups []pageReadingPeerDecls `json:"groups"`
	// Made are, the same way, the declarations the calls are written in,
	// by the part each stands in: "Made in" beside "Called from".
	Made []pageReadingPeerDecls `json:"made,omitempty"`
}

// reachedReading is the JSON of the callers every record a tile stands for
// is reached from; "" when none is.
func reachedReading(rows []pageOutbound) string {
	if len(rows) == 0 {
		return ""
	}
	own := rows[0].program
	reading := pageReached{Decls: []pageReadingDecl{}, Groups: []pageReadingPeerDecls{}}
	at := map[string]int{}
	add := func(groups []pageReadingPeerDecls, name pageReachedName) []pageReadingPeerDecls {
		position, known := at[name.decl.Key]
		if !known {
			position = len(reading.Decls)
			at[name.decl.Key] = position
			reading.Decls = append(reading.Decls, name.decl)
		}
		program := ""
		if name.program != own {
			program = name.program
		}
		group := slices.IndexFunc(groups, func(group pageReadingPeerDecls) bool {
			return group.Program == program && group.Part == name.part && group.Title == name.title
		})
		if group < 0 {
			group = len(groups)
			groups = append(groups, pageReadingPeerDecls{Part: name.part, Title: name.title, Program: program})
		}
		end := pageReadingEnd{Decl: position, Kind: string(programindex.RelationCalls), Possible: name.possible}
		if name.site.Text != "" {
			end.sites = []pageAnchor{name.site}
		}
		groups[group].Decls = mergeEnd(groups[group].Decls, end)
		return groups
	}
	for _, row := range rows {
		if row.made != nil {
			reading.Made = add(reading.Made, *row.made)
		}
		for _, name := range row.reached {
			reading.Groups = add(reading.Groups, name)
		}
	}
	if len(reading.Groups) == 0 && len(reading.Made) == 0 {
		return ""
	}
	// The tile's own program first, each part's declarations by name.
	for _, groups := range [][]pageReadingPeerDecls{reading.Made, reading.Groups} {
		slices.SortStableFunc(groups, func(a, b pageReadingPeerDecls) int { return boolFirst(a.Program == "", b.Program == "") })
		for i := range groups {
			ends := groups[i].Decls
			for j := range ends {
				ends[j].Site = callSite(ends[j].sites)
			}
			slices.SortStableFunc(ends, func(a, b pageReadingEnd) int {
				left, right := reading.Decls[a.Decl].Name, reading.Decls[b.Decl].Name
				return cmp.Or(strings.Compare(strings.ToLower(left), strings.ToLower(right)), strings.Compare(left, right))
			})
		}
	}
	raw, err := json.Marshal(reading)
	if err != nil {
		return ""
	}
	return string(raw)
}

// pageOutboundGroup presents every record naming one destination as one
// row: the destination, how many records name it, their shared kind and
// basis. The records are compact lines nested beneath it, three in view and
// the rest under one disclosure; each opens its own purpose, address and
// source. Nothing is merged in the data: the group carries no address of
// its own, since one would be completed here from its records' walked
// values (casdoor's Object Storage had read "%s/%s", an object key).
type pageOutboundGroup struct {
	Destination, NativeLabel, KindLabel string
	// Alternatives are the systems the destination is one of, depending on
	// configuration: it reads as one of them, never as several connections.
	Alternatives  []string
	Basis, Source string
	// Program marks the programs a component starts: Destination is the
	// word naming one as written, ProgramLabel stands for one no word names.
	Program      bool
	ProgramLabel string
	Rows         []pageOutbound
}

func (group pageOutboundGroup) BasisLabel() string {
	return pageOutbound{Basis: group.Basis}.BasisLabel()
}

// outboundGroupPreview is how many call records a destination shows before
// the rest wait under one "Expand" disclosure. Every record is rendered.
const outboundGroupPreview = 3

func (group pageOutboundGroup) First() []pageOutbound {
	if len(group.Rows) <= outboundGroupPreview {
		return group.Rows
	}
	return group.Rows[:outboundGroupPreview]
}

func (group pageOutboundGroup) Rest() []pageOutbound {
	if len(group.Rows) <= outboundGroupPreview {
		return nil
	}
	return group.Rows[outboundGroupPreview:]
}

// genericCallables are member names that say nothing without their type:
// "Ping" reads as "Pool.Ping", while "ExchangeDeclare" stands on its own.
var genericCallables = map[string]bool{"New": true, "Close": true, "Ping": true, "Get": true, "Set": true, "Do": true, "Run": true, "Start": true, "Stop": true,
	"Connect": true, "Open": true, "Exec": true, "Query": true, "Call": true, "Send": true, "Write": true, "Read": true, "Begin": true, "Commit": true,
	"Rollback": true, "Publish": true, "Consume": true, "Dial": true, "Delete": true, "Update": true, "Create": true, "List": true, "Put": true, "Post": true,
	"Up": true, "Down": true, "Version": true, "Save": true, "Load": true, "Fetch": true, "Store": true, "Add": true, "Remove": true, "Find": true,
	"Patch": true, "Watch": true, "Apply": true, "Scan": true, "Push": true, "Pull": true, "Insert": true, "Select": true, "Subscribe": true, "Unsubscribe": true, "Emit": true, "On": true}

// Line is one record's line beneath its destination: a native HTTP fact
// keeps its method and address; a callable drops the package the group
// already implies ("amqp091-go.Channel.Confirm" under RabbitMQ reads
// "Confirm") and keeps its type only when the member name alone is generic
// ("Pool.Ping", "Migrate.Up"). The full callable stays in the record's body.
func (row pageOutbound) Line() string {
	if row.Method != "" && row.NativeLabel != "" {
		return row.NativeLabel
	}
	if row.External != "" {
		return shortCallable(row.External, row.LineWithType)
	}
	// An unresolved interface call has no callable of its own; the function
	// that makes it is the next best name for the line.
	for _, use := range row.Uses {
		for _, step := range use.Steps {
			if step.Name != "" {
				return shortCallable(step.Name, false)
			}
		}
	}
	return ""
}

// InformativeUses are the destination chains worth a line: ones that reach
// an address, stop at a named frontier, or pass through more than one
// step. A chain of one step at the record's own location says nothing the
// record's anchor does not, nor does a frontier that is the record's own
// callable: "Address passes through netdb.h.gethostbyname" under
// gethostbyname's record named the call a second time.
func (row pageOutbound) InformativeUses() []pageOutboundUse {
	var uses []pageOutboundUse
	for _, use := range row.Uses {
		frontier := use.FrontierName() != "" && (row.External == "" || displayCallable(use.Frontier) != row.External)
		if use.Value != "" || frontier || len(use.Steps) > 1 {
			uses = append(uses, use)
		}
	}
	return uses
}

func shortCallable(external string, keepType bool) string {
	parts := strings.Split(external, ".")
	if len(parts) < 2 {
		return external
	}
	member := parts[len(parts)-1]
	if (keepType || genericCallables[member]) && len(parts) >= 3 {
		return parts[len(parts)-2] + "." + member
	}
	return member
}

// callableParts splits "pkg.Type.Member" into its type (empty for a
// package-level function) and member.
func callableParts(external string) (string, string) {
	parts := strings.Split(external, ".")
	if len(parts) < 3 {
		return "", parts[len(parts)-1]
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}

// Brief is the note printed right after the call on its line: the first
// sentence of the record's purpose, at most 120 runes. With the telegraphic
// boundaries note that is the whole purpose; a longer purpose keeps its
// full text under the disclosure (see MoreThanBrief).
func (row pageOutbound) Brief() string {
	if text := strings.TrimSpace(row.Summary); text != "" {
		return leadSentence(text, 120)
	}
	return ""
}

// MoreThanBrief reports a purpose longer than the note on the line, which
// the disclosure then repeats in full.
func (row pageOutbound) MoreThanBrief() bool {
	return strings.TrimSpace(row.Summary) != row.Brief()
}

func leadSentence(text string, limit int) string {
	if end := strings.Index(text, ". "); end > 0 {
		text = text[:end+1]
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	cut := limit - 1
	for cut > limit/2 && runes[cut] != ' ' {
		cut--
	}
	return strings.TrimRight(string(runes[:cut]), " ,;:") + "…"
}

// groupOutbound groups records by their destination text (case-insensitive),
// or by native label or kind when the model named no destination. A started
// program is one destination only with the very word its calls wrote: equal
// words are one literal; a program no word names is no destination.
// Groups with more records come first; equal counts keep record order.
// Grouping is a rendering step over translated rows, so it changes no saved
// data.
func groupOutbound(rows []pageOutbound) []pageOutboundGroup {
	var groups []pageOutboundGroup
	position := make(map[string]int)
	for _, row := range rows {
		key := "k\x00" + row.KindLabel
		destination := canonicalDestination(row.Destination)
		switch {
		case row.Program && row.Destination != "":
			key, destination = "p\x00"+row.Destination, row.Destination
		case destination != "":
			key = "d\x00" + strings.ToLower(destination)
		case row.NativeLabel != "":
			key = "n\x00" + row.NativeLabel
		}
		at, known := position[key]
		if !known {
			at = len(groups)
			position[key] = at
			groups = append(groups, pageOutboundGroup{Destination: destination, Alternatives: row.Alternatives, NativeLabel: row.NativeLabel,
				KindLabel: row.KindLabel, Basis: row.Basis, Source: row.Source, Program: row.Program, ProgramLabel: row.ProgramLabel()})
		}
		group := &groups[at]
		if group.KindLabel != row.KindLabel {
			group.KindLabel = "External communication"
		}
		if group.Basis != row.Basis {
			group.Basis = ""
		}
		if group.Source != row.Source {
			group.Source = "model"
		}
		group.Rows = append(group.Rows, row)
	}
	// Within one destination a member shared by several types keeps its
	// type on the line; a member used by one type reads alone.
	for i := range groups {
		types := make(map[string]map[string]bool)
		for _, row := range groups[i].Rows {
			if row.External == "" {
				continue
			}
			typ, member := callableParts(row.External)
			if types[member] == nil {
				types[member] = make(map[string]bool)
			}
			types[member][typ] = true
		}
		for j := range groups[i].Rows {
			row := &groups[i].Rows[j]
			if row.External == "" {
				continue
			}
			_, member := callableParts(row.External)
			row.LineWithType = len(types[member]) > 1
		}
	}
	sort.SliceStable(groups, func(i, j int) bool { return len(groups[i].Rows) > len(groups[j].Rows) })
	return groups
}

// canonicalDestination is the group name for a destination text: the name
// the reading stored, without a parenthetical qualifier ("RabbitMQ broker
// (queue topology)" is "RabbitMQ broker"). No text is folded onto another
// name. Records keep their own wording.
func canonicalDestination(text string) string {
	base := strings.TrimSpace(text)
	if i := strings.Index(base, "("); i > 0 {
		base = strings.TrimSpace(base[:i])
	}
	return base
}

func outboundKindLabel(kind string) string {
	switch kind {
	case "client_request":
		return "Request"
	case "db":
		return "Database"
	case "queue_producer", "queue_consumer":
		return "Queue"
	case "sdk":
		return "SDK"
	case atlas.BoundaryRunsProgram:
		return "Runs a program"
	default:
		return "External communication"
	}
}

func (row pageOutbound) BasisLabel() string {
	switch row.Basis {
	case "configuration":
		return "Configured communication"
	case "dispatch":
		return "Communication call"
	default:
		return ""
	}
}

// outboundEnds lists each end of a call's value once, in the order the
// chains first reach it, with the shortest chain reaching it and how many
// do: the routes through one value's code multiply (casdoor's avatar
// Client.Get had 77,299 chains to 54 ends over 326 steps, 255 MB of the
// page), and its reader asks where the value comes from and how one route
// gets there, as the reading's own request lists it (atlas reading
// destinations.go, c839c34b).
func outboundEnds(uses []atlas.DestinationUse) ([]atlas.DestinationUse, []int) {
	type end struct {
		value, frontier, method, targets string
		unread                           bool
		at                               string
	}
	at := map[end]int{}
	var ends []atlas.DestinationUse
	var routes []int
	for _, use := range uses {
		key := end{use.Address, use.Frontier, use.Method, strings.Join(use.TargetIDs, " "), use.Unread, ""}
		if n := len(use.Steps); n > 0 {
			last := use.Steps[n-1]
			key.at = fmt.Sprintf("%s:%d:%d", last.Path, last.Line, last.Column)
		}
		// A saved end carries how many routes reach it (atlas 21).
		count := max(1, use.Routes)
		position, seen := at[key]
		if !seen {
			at[key] = len(ends)
			ends, routes = append(ends, use), append(routes, count)
			continue
		}
		routes[position] += count
		if len(use.Steps) < len(ends[position].Steps) {
			ends[position] = use
		}
	}
	return ends, routes
}
