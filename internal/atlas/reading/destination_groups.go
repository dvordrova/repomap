package reading

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

// destinationMember is one outgoing row as one of its programs makes it: a
// row written in code several programs share (Redis's anet.c connect) is
// each program's own call, reached from that program's callers and walked
// to that program's values, so each program's destination is named by its
// own (the server's connect reaches its master, the clients' the server).
type destinationMember struct {
	state  *boundaryState
	target string
}

// destinationKey is what an outgoing row reaches in one program as the
// code knows it: the program, the row's kind and the places the walks of
// its reaching call's value end there (destinationEnd), each once and
// sorted. Rows with one key reach one destination and are named by one
// answer. Rows of two kinds are two exchanges even on one value: a lookup
// of a host's addresses (sdk, redis's gethostbyname(server.masterhost))
// is answered by the resolver, the connect made after it (client_request)
// by the host. A row with no
// reaching call at its site, or whose walk read nothing in the program, is
// its own destination: the walk of another call there (the fmt.Sprintf
// formatting a query's text) says nothing about what the row reaches.
func destinationKey(member destinationMember) string {
	state := member.state
	var ends []string
	if state.reaching {
		for _, use := range state.exchangeEnds() {
			if slices.Contains(use.TargetIDs, member.target) {
				ends = append(ends, destinationEnd(use))
			}
		}
	}
	if len(ends) == 0 {
		return "row\x00" + state.place.ID + "\x00" + member.target
	}
	slices.Sort(ends)
	return member.target + "\x02" + state.kind + "\x02" + strings.Join(slices.Compact(ends), "\x01")
}

// exchangeEnds are the walks a row's destination is keyed and named by:
// where its exchange ends (boundaryState.through), else its own walk.
func (state *boundaryState) exchangeEnds() []atlas.DestinationUse {
	if len(state.through) > 0 {
		return state.through
	}
	return state.uses
}

// destinationEnd is where one walk ends. An absolute URL is its scheme and
// host as written (two paths of one server are one destination), and a
// setting's value (`{--socket}`, `{env:API}/users`) is that setting; both
// are the same wherever the program writes them. Any other address, such
// as a bare path, and an expression the walk could not follow are that
// value at the site where the walk stopped: two clients with different
// base URLs both asking "/health" stay apart.
func destinationEnd(use atlas.DestinationUse) string {
	if use.Address != "" {
		if key := addressKey(use.Address); key != "" {
			return "address\x00" + key
		}
	}
	site := ""
	if n := len(use.Steps); n > 0 {
		step := use.Steps[n-1]
		site = fmt.Sprintf("%s:%d:%d", step.Path, step.Line, step.Column)
	}
	if use.Address != "" {
		return "value\x00" + use.Address + "\x00" + site
	}
	return "unresolved\x00" + use.Frontier + "\x00" + site
}

// addressKey is an address that is the same wherever it is written: an
// absolute URL's scheme and host as written (cut at the first /, ? or #
// after them), or the setting that begins it, a command-line option or an
// environment variable. Empty for any other address.
func addressKey(address string) string {
	if i := strings.Index(address, "://"); i > 0 && urlScheme(address[:i]) {
		host := address[i+len("://"):]
		if j := strings.IndexAny(host, "/?#"); j >= 0 {
			host = host[:j]
		}
		if host != "" {
			return address[:i] + "://" + host
		}
	}
	if strings.HasPrefix(address, "{--") || strings.HasPrefix(address, "{env:") {
		if j := strings.IndexByte(address, '}'); j > 0 {
			return address[:j+1]
		}
	}
	return ""
}

// urlScheme reports a URL scheme: a letter, then letters, digits, +, - or .
func urlScheme(text string) bool {
	for i, r := range text {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && (r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.'):
		default:
			return false
		}
	}
	return text != ""
}

// packageSystem is the system the row's own outside package reaches, as
// its catalogue spells it: atlas_systems decided it once for the package,
// so no row asks it again. Empty when the row names no package, its
// package reaches no system, or it reaches several (listed under each):
// which one this row's call reaches is the destination question's, never
// the package's first name (control review B7).
func packageSystem(state *boundaryState, catalog []lines.Destination) string {
	if state.outside == "" {
		return ""
	}
	system := ""
	for _, entry := range catalog {
		if !slices.Contains(entry.Packages, state.outside) {
			continue
		}
		if system != "" {
			return ""
		}
		system = entry.Value
	}
	return system
}

// nameDestinations gives every outgoing row its destination in each of its
// programs. A row whose package atlas_systems named takes that name; rows
// the code knows reach one destination in one program (destinationKey)
// share it, so a destination naming exactly one system gives it to its rows
// without a package too. One naming none is asked once
// (lines.DestinationNames). One whose packages name several systems is
// shown by the facts not to be one destination: each row keeps its
// package's, and each row without one is asked alone. A refused answer
// names none.
func (r *reader) nameDestinations(ctx context.Context, states []*boundaryState, packages targetPackages, names map[string][]string, owners map[string]atlas.Place) error {
	r.programNamesOf = programNames(r.opts.Targets, names)
	byKey := make(map[string][]destinationMember)
	var keys []string
	for _, state := range states {
		for _, target := range rowTargets(state) {
			member := destinationMember{state: state, target: target}
			key := destinationKey(member)
			if _, known := byKey[key]; !known {
				keys = append(keys, key)
			}
			byKey[key] = append(byKey[key], member)
		}
	}
	sort.Strings(keys)
	type asked struct {
		key     string
		members []destinationMember
		catalog []lines.Destination
	}
	name := func(member destinationMember, system string) {
		if member.state.destinations == nil {
			member.state.destinations = make(map[string]string)
		}
		member.state.destinations[member.target] = system
	}
	var questions []asked
	for _, key := range keys {
		group := byKey[key]
		catalog := packages.catalog(group[0].target, names)
		var unnamed []destinationMember
		systems := make(map[string]string)
		for _, member := range group {
			if system := packageSystem(member.state, catalog); system != "" {
				name(member, system)
				systems[strings.ToLower(system)] = system
			} else {
				unnamed = append(unnamed, member)
			}
		}
		switch {
		case len(unnamed) == 0:
		case len(systems) == 1:
			for _, system := range systems {
				for _, member := range unnamed {
					name(member, system)
				}
			}
		case len(systems) == 0:
			questions = append(questions, asked{key: key, members: unnamed, catalog: lines.WithPrograms(catalog, r.programDestinations(group[0].target, unnamed))})
		default:
			for _, member := range unnamed {
				questions = append(questions, asked{key: "row\x00" + member.state.place.ID + "\x00" + member.target, members: []destinationMember{member}, catalog: lines.WithPrograms(catalog, r.programDestinations(member.target, []destinationMember{member}))})
			}
		}
	}
	// An sdk row reaches the service behind its outside package, whatever
	// value it hands that package (the resolver answers gethostbyname for
	// any host), unless an address decides it (sdkService): rows of one
	// package no system named are one destination in every program, asked
	// once, where they are asked first, and named the same everywhere by
	// that answer. redis-server's resolver had read
	// "Resolver", redis-cli's and redis-benchmark's "System Resolver".
	followers := make(map[int][]asked)
	first := make(map[string]int)
	kept := questions[:0:0]
	for _, question := range questions {
		service := sdkService(question.members)
		if at, seen := first[service]; seen && service != "" {
			followers[at] = append(followers[at], question)
			continue
		}
		if service != "" {
			first[service] = len(kept)
		}
		kept = append(kept, question)
	}
	questions = kept
	if len(questions) == 0 {
		return nil
	}
	def := lines.DestinationNames()
	files := make(map[string]*lines.CallFile)
	handlers := r.entryHandlers()
	subjects := make(map[string]rowSubject, len(questions))
	readable := strings.NewReplacer("\x00", " ", "\x01", " | ", "\x02", " · ")
	// The program asking keeps its target name: renaming it re-asks every
	// window of that program, offered a program or not, and casdoor's
	// server, re-asked so, had named a GetUserInfo of its Casdoor identity
	// provider "Adyen" (one draw).
	programs := make(map[string]string, len(r.opts.Targets))
	for _, target := range r.opts.Targets {
		programs[target.ID] = target.Name
	}
	byCatalog := make(map[string]int)
	var groups rowGroups
	var members [][]asked
	for i, question := range questions {
		id := fmt.Sprintf("g%d", i+1)
		first := question.members[0].state.place
		subjects[id] = rowSubject{id: "destination:" + readable.Replace(question.key), path: first.Path, line: first.LineNo}
		// One program's destinations offered one catalogue share windows,
		// which name the program; another program's, or another
		// catalogue's, never see them.
		program := question.members[0].target
		raw, _ := json.Marshal(question.catalog)
		at, known := byCatalog[program+"\x00"+string(raw)]
		if !known {
			at = len(groups)
			byCatalog[program+"\x00"+string(raw)] = at
			groups = append(groups, rowGroup{shared: append(lines.DestinationFields(question.catalog), lines.DestinationProgram(programs[program]))})
			members = append(members, nil)
		}
		groups[at].rows = append(groups[at].rows, r.destinationRow(id, question.members, owners, files, handlers))
		members[at] = append(members[at], question)
	}
	// The answers come back in the groups' row order.
	var ordered []asked
	var orderedFollowers [][]asked
	position := make(map[string]int, len(questions))
	for i, question := range questions {
		position[question.key] = i
	}
	for _, group := range members {
		ordered = append(ordered, group...)
		for _, question := range group {
			orderedFollowers = append(orderedFollowers, followers[position[question.key]])
		}
	}
	outgoing := 0
	for _, state := range states {
		outgoing += len(rowTargets(state))
	}
	r.opts.Stage(lines.StageBoundaries, fmt.Sprintf("naming %d destinations of %d outgoing calls", len(questions), outgoing))
	previous := r.rowSubjects
	r.rowSubjects = subjects
	defer func() { r.rowSubjects = previous }()
	answers, err := r.runTableGroups(ctx, def, 3, groups, nil)
	if err != nil {
		return err
	}
	for i, question := range ordered {
		chosen := "not decided"
		if answer := answers[i].answer; answer != nil {
			chosen = destinationChoice(def, question.catalog, answer["destination"])
			program, named := lines.DestinationTarget(question.catalog, answer["destination"], chosen)
			if program != "" {
				chosen = named
			}
			// One of several systems, depending on configuration: the
			// destination is them all, never a program (B7, 2026-10-04).
			systems := destinationSystems(def, question.catalog, answer["destination"])
			switch len(systems) {
			case 0:
			case 1:
				chosen, program = systems[0], ""
			default:
				chosen, program = strings.Join(systems, " / "), ""
			}
			all := slices.Clone(question.members)
			for _, follower := range orderedFollowers[i] {
				all = append(all, follower.members...)
			}
			for _, member := range all {
				name(member, chosen)
				if len(systems) > 1 {
					if member.state.alternatives == nil {
						member.state.alternatives = make(map[string][]string)
					}
					member.state.alternatives[member.target] = slices.Clone(systems)
				}
				if program != "" {
					if member.state.destinationTargets == nil {
						member.state.destinationTargets = make(map[string]string)
					}
					member.state.destinationTargets[member.target] = program
				}
			}
		}
		fmt.Fprintf(&r.tables, "%s: destination %s · %d calls · %s\n", lines.StageBoundaries, readable.Replace(question.key), len(question.members), chosen)
		for _, follower := range orderedFollowers[i] {
			fmt.Fprintf(&r.tables, "%s: destination %s · %d calls · %s, as named first\n", lines.StageBoundaries, readable.Replace(follower.key), len(follower.members), chosen)
		}
	}
	fmt.Fprintln(&r.tables)
	foldDestinationSpellings(states)
	return nil
}

// foldDestinationSpellings writes one system's name one way in every
// program: names equal but for letter case are one name, spelled as the
// first row by ID and program names it (etcd's windows had answered "DNS
// Resolver" and "DNS resolver", "Etcd Server" and "Etcd server", and its
// home stood a shared Outside frame for each spelling). The systems
// question asks for one name per system; only its case is the code's.
func foldDestinationSpellings(states []*boundaryState) {
	ordered := slices.Clone(states)
	sort.Slice(ordered, func(i, j int) bool { return compactIDLess(ordered[i].place.ID, ordered[j].place.ID) })
	first := map[string]string{}
	for _, state := range ordered {
		targets := make([]string, 0, len(state.destinations))
		for target := range state.destinations {
			targets = append(targets, target)
		}
		sort.Strings(targets)
		for _, target := range targets {
			name := state.destinations[target]
			if _, seen := first[strings.ToLower(name)]; !seen && name != "" {
				first[strings.ToLower(name)] = name
			}
		}
	}
	for _, state := range ordered {
		for _, systems := range state.alternatives {
			for _, name := range systems {
				if _, seen := first[strings.ToLower(name)]; !seen && name != "" {
					first[strings.ToLower(name)] = name
				}
			}
		}
	}
	for _, state := range ordered {
		for target, name := range state.destinations {
			// A destination of several systems folds each and is them
			// joined again; its joined name is never folded whole.
			if systems := state.alternatives[target]; len(systems) > 1 {
				for i, system := range systems {
					if spelled := first[strings.ToLower(system)]; spelled != "" {
						systems[i] = spelled
					}
				}
				state.destinations[target] = strings.Join(systems, " / ")
				continue
			}
			if spelled := first[strings.ToLower(name)]; spelled != "" {
				state.destinations[target] = spelled
			}
		}
	}
}

// sdkService is the outside package a destination's rows all reach a
// service through, when every row is an sdk call (their external, for a
// fact naming no package) and no walk of theirs ends at an address that is
// the same wherever it is written (a URL's host, a setting): such an
// address decides what the call reaches, and its own destination key
// stays. "" otherwise.
func sdkService(members []destinationMember) string {
	service := ""
	for _, member := range members {
		state := member.state
		through := state.outside
		if through == "" && state.place.Boundary != nil {
			through = state.place.Boundary.External
		}
		if state.kind != atlas.BoundarySDK || through == "" || service != "" && through != service {
			return ""
		}
		for _, use := range state.exchangeEnds() {
			if addressKey(use.Address) != "" {
				return ""
			}
		}
		service = through
	}
	return service
}

// destinationRow is one destination's item: where its value ends in its
// program, its calls as written, the functions making them with the calls
// beside them, and the functions its program reaches them from, by name,
// each once.
func (r *reader) destinationRow(id string, members []destinationMember, owners map[string]atlas.Place, files map[string]*lines.CallFile, handlers map[string]bool) table.Row {
	var ends []lines.DestinationEnd
	var calls []lines.DestinationCall
	var reached []string
	own := make(map[sourceSite]bool)
	rowLines := make(map[string][]int)
	var declarations []atlas.Place
	callers := make(map[string]*lines.DestinationCaller)
	var order []string
	for _, member := range members {
		state := member.state
		site := sourceSite{state.place.Path, state.place.LineNo, state.place.Column}
		own[site] = true
		// The walk of a call that reaches nothing says nothing of where the
		// row goes, and a walk another program makes says nothing of where
		// it goes in this one.
		for _, use := range state.exchangeEnds() {
			if !state.reaching {
				break
			}
			if !slices.Contains(use.TargetIDs, member.target) {
				continue
			}
			end := lines.DestinationEnd{Address: use.Address}
			if use.Address == "" {
				end.Unresolved = use.Frontier
			}
			if n := len(use.Steps); n > 0 {
				step := use.Steps[n-1]
				if at := (sourceSite{step.Path, step.Line, step.Column}); at != site {
					end.Written = r.sourceText(files, step.Path, step.Line, step.Column)
				}
			}
			if !slices.Contains(ends, end) {
				ends = append(ends, end)
			}
		}
		facts := state.place.Boundary
		call := lines.DestinationCall{Call: r.sourceText(files, site.path, site.line, site.column), Kind: state.kind, Package: state.outside, Values: facts.Values}
		if call.Call == "" {
			call.Call = facts.External
		}
		if !slices.ContainsFunc(calls, func(known lines.DestinationCall) bool {
			return known.Call == call.Call && known.Kind == call.Kind && known.Package == call.Package && slices.Equal(known.Values, call.Values)
		}) {
			calls = append(calls, call)
		}
		owner := boundaryOwner(facts, owners)
		key := facts.Caller + "\x00" + state.place.Path
		if owner.Symbol != nil {
			key = owner.ID
		}
		if callers[key] == nil {
			callers[key] = &lines.DestinationCaller{Name: facts.Caller, Path: state.place.Path}
			if owner.Symbol != nil {
				callers[key].Name, callers[key].Path, callers[key].Signature = owner.Symbol.Decl.Name, owner.Path, owner.Symbol.Decl.Signature
				declarations = append(declarations, owner)
			}
			order = append(order, key)
		}
		rowLines[key] = append(rowLines[key], state.place.LineNo)
		reached = append(reached, r.reachedFrom([]string{member.target}, owner, handlers)...)
	}
	for _, owner := range declarations {
		caller := callers[owner.ID]
		for _, call := range lines.OwnerCalls(owner, rowLines[owner.ID]) {
			if own[sourceSite{owner.Path, call.Line, call.Column}] {
				continue
			}
			text := r.sourceText(files, owner.Path, call.Line, call.Column)
			if text == "" {
				text = strings.TrimSpace(call.Name + " " + strings.Join(call.Values, " "))
			}
			if !slices.Contains(caller.Calls, text) {
				caller.Calls = append(caller.Calls, text)
			}
		}
		for _, binding := range owner.Symbol.Bindings {
			if binding.Kind != "passes_callback" {
				continue
			}
			receiving, _, _ := strings.Cut(binding.Detail, " <- ")
			switch name := owner.Symbol.Decl.Name; {
			case binding.To == name && binding.From != name:
				caller.HandedOver = appendOnce(caller.HandedOver, "by "+binding.From+" to "+receiving)
			case binding.From == name && binding.To != name:
				caller.HandsOver = appendOnce(caller.HandsOver, binding.To+" to "+receiving)
			}
		}
	}
	listed := make([]lines.DestinationCaller, 0, len(order))
	for _, key := range order {
		listed = append(listed, *callers[key])
	}
	slices.Sort(reached)
	return lines.DestinationRow(id, ends, calls, listed, slices.Compact(reached))
}

func appendOnce(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}
