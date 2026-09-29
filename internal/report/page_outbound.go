package report

import (
	"cmp"
	"encoding/json"
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
	Connections      []string
	MapGroup         string
	Operations       string
	KindLabel        string
	DestinationCount int
	Uses             []pageOutboundUse
	// reached are the callers outside its part the program reaches the
	// call from (GroupsIndex ReachedFrom), read with its tile; program is
	// the component making it.
	reached []pageReachedName
	program string
	// Caller is the declaration the outside call is written in; Side the
	// program's own code reaching it (page_shared_code.go), which says the
	// program connects out.
	Caller                      string
	Side                        *pageCallSide
	CallerAnchor                pageAnchor
	ID                          string
	Destination, DestinationRef string
	Summary, SummaryRef         string
	Address, NativeLabel        string
	External, Basis, Source     string
	Method                      string
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
	// Runs are this repository's programs whose executable is named what
	// the started program is (programsNamed): equal names are the code
	// fact, "runs litestream" is cmd/litestream.
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
// whose name the code computes, or one the model did not decide.
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

type pageOutboundUse struct {
	Address, Frontier, Method string
	Steps                     []pageOutboundStep
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

func (row pageOutbound) AddressText() pageOutboundAddress    { return outboundAddressText(row.Address) }
func (use pageOutboundUse) AddressText() pageOutboundAddress { return outboundAddressText(use.Address) }

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
			Destination: call.Destination, Summary: call.Summary,
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
		destinations := make(map[string]bool)
		for _, use := range call.Uses {
			destinations[use.Address+"\x00"+use.Frontier] = true
			value := pageOutboundUse{Address: use.Address, Frontier: use.Frontier, Method: use.Method}
			for _, step := range use.Steps {
				name := step.Name
				// Existing operation subjects already own their interpreted
				// names. Matching the original declaration binds this source
				// chain to that operation without inventing another label.
				for _, operation := range index.Operations {
					if step.SubjectID != "" && operation.SubjectID == step.SubjectID {
						name = operation.Name
						break
					}
				}
				// Two steps in one declaration on one line read as one:
				// freqtrade's start_install_ui and its dl_url both read
				// "install-ui".
				anchor := builder.links.anchor(step.Path, step.Line, step.Column)
				if last := len(value.Steps) - 1; last >= 0 && value.Steps[last].Name == name && value.Steps[last].Anchor.Href == anchor.Href && value.Steps[last].Anchor.Text == anchor.Text {
					continue
				}
				value.Steps = append(value.Steps, pageOutboundStep{Name: name, Anchor: anchor})
			}
			row.Uses = append(row.Uses, value)
		}
		row.DestinationCount = len(destinations)
		if len(call.Uses) > 0 {
			row.Address = ""
			if len(destinations) == 1 {
				row.Address = call.Uses[0].Address
			}
		}
		// Native HTTP facts may have no semantic label. Compose only their
		// supplied method and address, never a guessed destination.
		if call.Method != "" && call.Address != "" {
			row.NativeLabel = call.Method + " " + call.Address
		} else {
			row.NativeLabel = displayCallable(call.External)
		}
		section.Outbound = append(section.Outbound, row)
	}
}

// pageReachedName is one caller an outside call is reached from, as its
// tile's reading names it: the declaration, read in the part it stands in,
// and the place it makes the call into the call's part, said on hover.
type pageReachedName struct {
	decl                 pageReadingDecl
	part, title, program string
	site                 pageAnchor
}

// outboundReached names the callers a call is reached from, in the saved
// order: each part's, then each path's start inside the call's own part.
func (builder *pageBuilder) outboundReached(index *groupindex.Index, section *pageSection, call groupindex.OutboundCall) []pageReachedName {
	program := componentTitle(section, builder.sections)
	var names []pageReachedName
	for _, caller := range call.ReachedFrom {
		ref, known := builder.subject(index.Target.ID, caller.SubjectID)
		if !known {
			continue
		}
		label, anchor := builder.subjectDisplay(ref.subject)
		key := declarationKey(anchor)
		if label == "" || key == "" {
			continue
		}
		name := pageReachedName{program: program}
		if caller.GroupID != "" {
			name.part = "#" + groupAnchorID(section.ID, caller.GroupID)
			name.title = builder.groupTitles[groupindex.Endpoint{TargetID: index.Target.ID, GroupID: caller.GroupID}]
		}
		kind := ""
		if object := ref.subject.Object; object != nil && (object.Kind == programindex.ObjectFunction || object.Kind == programindex.ObjectMethod) {
			kind = "function"
		}
		name.decl = pageReadingDecl{Name: builder.withType(index.Target.ID, ref.subject, label), Key: key, Href: anchor.Href, Open: anchor.Open, NoSource: anchor.NoSource,
			Code: anchor.Code, At: anchor.Text, File: anchor.Path, Kind: kind, Part: name.part}
		if caller.Location != nil {
			name.site = builder.links.anchor(caller.Location.Path, caller.Location.Line, caller.Location.Column)
		}
		names = append(names, name)
	}
	return names
}

// pageReached is the reading of the callers an outside call's tile is
// reached from (31-reading-column.js rmReachedFrom): the declarations once,
// and each part's callers by name, the tile's own program first and every
// other program's part named with its program. No line number is written:
// a name reads its function, its call's place said on hover.
type pageReached struct {
	Decls  []pageReadingDecl      `json:"decls"`
	Groups []pageReadingPeerDecls `json:"groups"`
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
	for _, row := range rows {
		for _, name := range row.reached {
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
			group := slices.IndexFunc(reading.Groups, func(group pageReadingPeerDecls) bool {
				return group.Program == program && group.Part == name.part && group.Title == name.title
			})
			if group < 0 {
				group = len(reading.Groups)
				reading.Groups = append(reading.Groups, pageReadingPeerDecls{Part: name.part, Title: name.title, Program: program})
			}
			end := pageReadingEnd{Decl: position, Kind: string(programindex.RelationCalls)}
			if name.site.Text != "" {
				end.sites = []pageAnchor{name.site}
			}
			reading.Groups[group].Decls = mergeEnd(reading.Groups[group].Decls, end)
		}
	}
	if len(reading.Groups) == 0 {
		return ""
	}
	// The tile's own program first, each part's callers by name.
	slices.SortStableFunc(reading.Groups, func(a, b pageReadingPeerDecls) int { return boolFirst(a.Program == "", b.Program == "") })
	for i := range reading.Groups {
		ends := reading.Groups[i].Decls
		for j := range ends {
			ends[j].Site = callSite(ends[j].sites)
		}
		slices.SortStableFunc(ends, func(a, b pageReadingEnd) int {
			left, right := reading.Decls[a.Decl].Name, reading.Decls[b.Decl].Name
			return cmp.Or(strings.Compare(strings.ToLower(left), strings.ToLower(right)), strings.Compare(left, right))
		})
	}
	raw, err := json.Marshal(reading)
	if err != nil {
		return ""
	}
	return string(raw)
}

// pageOutboundGroup presents every record naming one destination as one
// row: the destination, how many records name it, their shared kind, basis
// and address. The records are compact lines nested beneath it, three in
// view and the rest under one disclosure; each opens its own purpose,
// address and source. Nothing is merged in the data.
type pageOutboundGroup struct {
	Destination, NativeLabel, KindLabel string
	Basis, Source, Address              string
	// Program marks the programs a component starts: Destination is the
	// word naming one as written, ProgramLabel stands for one no word names.
	Program      bool
	ProgramLabel string
	Addresses    int
	Rows         []pageOutbound
}

func (group pageOutboundGroup) BasisLabel() string {
	return pageOutbound{Basis: group.Basis}.BasisLabel()
}

func (group pageOutboundGroup) AddressText() pageOutboundAddress {
	return outboundAddressText(group.Address)
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
		frontier := use.Frontier != "" && (row.External == "" || displayCallable(use.Frontier) != row.External)
		if use.Address != "" || frontier || len(use.Steps) > 1 {
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
// words are one literal, and a program no word names is its call's own.
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
		case row.Program:
			key = "o\x00" + row.ID
		case destination != "":
			key = "d\x00" + strings.ToLower(destination)
		case row.NativeLabel != "":
			key = "n\x00" + row.NativeLabel
		}
		at, known := position[key]
		if !known {
			at = len(groups)
			position[key] = at
			groups = append(groups, pageOutboundGroup{Destination: destination, NativeLabel: row.NativeLabel,
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
	for i := range groups {
		addresses := make(map[string]bool)
		for _, row := range groups[i].Rows {
			if row.Address != "" {
				addresses[row.Address] = true
			}
			if row.DestinationCount > 1 {
				for _, use := range row.Uses {
					if use.Address != "" {
						addresses[use.Address] = true
					}
				}
			}
		}
		groups[i].Addresses = len(addresses)
		if len(addresses) == 1 {
			for address := range addresses {
				groups[i].Address = address
			}
		}
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
