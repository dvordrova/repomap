package reading

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/modeldiag"
	"github.com/dvordrova/repomap/internal/programindex"
)

// apiRole is the model's reading of one external symbol; see atlas.APIRole.
type apiRole struct {
	binds, talks, enters  string
	publishes, middleware bool
	// perCall says the symbol's words mean different things at different
	// calls: each call's own answer (api_call.go) is its entry.
	perCall bool
}

// apiSymbol is what the code observed about one external symbol across the
// repository: the call word, whether a repository callable was handed to it,
// the literals it was given, its sites and the holders it acts on.
type apiSymbol struct {
	name          string
	signature     string
	handsCallable bool
	literals      []string
	// sites counts the calls outside test files; a symbol only tests call
	// belongs to testing and is not asked about.
	sites   int
	holders map[string]bool
	// usage is the first site: the call a reader would look at. A symbol
	// only registrations name uses its first registration.
	usagePath                            string
	usageLine, usageColumn               int
	registrationPath                     string
	registrationLine, registrationColumn int
	// wordCalls counts the calls outside test files that give the symbol
	// words (a literal); wordUsage is the first of them, the usage of a
	// symbol asked what its words become.
	wordCalls                   int
	wordUsagePath               string
	wordUsageLine, wordUsageCol int
	// resultReceives counts, by name, the calls made on what a call to
	// the symbol returns: parser.add_argument on ArgumentParser(...)'s
	// result. The code states the fact; what it makes of the symbol is the
	// model's.
	resultReceives map[string]int
}

// receivedCalls is resultReceives as a row shows it: "add_argument ×2",
// by name.
func (s *apiSymbol) receivedCalls() []string {
	names := make([]string, 0, len(s.resultReceives))
	for name := range s.resultReceives {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, fmt.Sprintf("%s ×%d", name, s.resultReceives[name]))
	}
	return result
}

func apiName(api atlas.CallAPI) string {
	name := api.Name
	if api.Receiver != "" {
		name = strings.TrimPrefix(api.Receiver, "*") + "." + name
	}
	return api.Package + "." + name
}

// apiSymbols collects the external symbols of the graph: from registration
// boundaries, which know what was handed over and on which holder, and from
// the calls of every declaration.
func (r *reader) apiSymbols() []*apiSymbol {
	byName := make(map[string]*apiSymbol)
	callAt := make(map[sourceSite]*apiSymbol)
	symbol := func(name string) *apiSymbol {
		if byName[name] == nil {
			byName[name] = &apiSymbol{name: name, holders: make(map[string]bool)}
		}
		return byName[name]
	}
	for _, place := range r.opts.Graph.Places {
		inTest := false
		if file := r.places[place.Parent]; file.File != nil && file.File.Test {
			inTest = true
		}
		if b := place.Boundary; b != nil && b.Source == "fact" && b.External != "" && b.GivenKind == "" {
			s := symbol(b.External)
			// A registration hands a callable over when it brings work in,
			// and a value the repository built when it is Handed
			// (Register("k6/x/dns", new(DNS)), whose entry applyAPIRoles
			// makes from binds); its ObjectID is otherwise the declaration
			// making the call, and fopen("/dev/null") hands nothing.
			s.handsCallable = s.handsCallable || b.Direction == atlas.DirectionIn || b.Handed
			s.literals = appendUnique(s.literals, b.Values...)
			if b.Holder != "" {
				s.holders[b.Holder] = true
			}
			if !inTest {
				s.sites++
			}
			// A registrar only rows of a table name (a record's field) is
			// called nowhere: its usage is the first registration.
			if s.registrationPath == "" {
				s.registrationPath, s.registrationLine, s.registrationColumn = place.Path, place.LineNo, place.Column
			}
		}
		if place.Symbol == nil {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.Kind != string(programindex.RelationInvokesExternal) || call.API == nil {
				continue
			}
			s := symbol(apiName(*call.API))
			if call.Line > 0 {
				callAt[sourceSite{place.Path, call.Line, call.Column}] = s
			}
			if !inTest {
				s.sites++
			}
			s.literals = appendUnique(s.literals, call.Values...)
			if s.signature == "" {
				s.signature = call.API.Signature
			}
			if s.usagePath == "" && call.Line > 0 {
				s.usagePath, s.usageLine, s.usageColumn = place.Path, call.Line, call.Column
			}
			if !inTest && len(call.Values) > 0 && call.Line > 0 {
				s.wordCalls++
				if s.wordUsagePath == "" {
					s.wordUsagePath, s.wordUsageLine, s.wordUsageCol = place.Path, call.Line, call.Column
				}
			}
		}
	}
	// What a call's result receives: the calls outside test files whose
	// receiver is the result of another outside symbol's call.
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil {
			continue
		}
		if file := r.places[place.Parent]; file.File != nil && file.File.Test {
			continue
		}
		for _, call := range place.Symbol.Calls {
			receiver := call.ReceiverValue
			if call.Kind != string(programindex.RelationInvokesExternal) || call.API == nil || receiver == nil || receiver.Kind != "call_result" || receiver.Anchor == nil {
				continue
			}
			producer := callAt[sourceSite{receiver.Anchor.Path, receiver.Anchor.Line, receiver.Anchor.Column}]
			if producer == nil {
				continue
			}
			if producer.resultReceives == nil {
				producer.resultReceives = make(map[string]int)
			}
			producer.resultReceives[call.API.Name]++
		}
	}
	result := make([]*apiSymbol, 0, len(byName))
	for _, s := range byName {
		if s.usagePath == "" {
			s.usagePath, s.usageLine, s.usageColumn = s.registrationPath, s.registrationLine, s.registrationColumn
		}
		if s.sites > 0 {
			result = append(result, s)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result
}

// sourceText is the call at a source position as the code wrote it
// (lines.CallText), reading each file once.
func (r *reader) sourceText(files map[string][]byte, path string, line, column int) string {
	if r.opts.ReadSource == nil || line < 1 {
		return ""
	}
	content, read := files[path]
	if !read {
		var err error
		if content, err = r.opts.ReadSource(path); err != nil {
			content = nil
		}
		files[path] = content
	}
	if content == nil {
		return ""
	}
	return lines.CallText(content, path, line, column)
}

// apiSubject is the knowledge subject of an outside symbol's row: no
// compact place ID holds a colon, so it names no place.
func apiSubject(symbol string) string { return "api:" + symbol }

// readAPI asks the model what the external symbols do with what the
// repository gives them, one row per symbol.
func (r *reader) readAPI(ctx context.Context) error {
	r.api = make(map[string]apiRole)
	symbols := r.apiSymbols()
	// Three questions, asked at once, each symbol in exactly one: a symbol
	// handed a callable is asked what the callable becomes; one whose calls
	// give it words also what the words become; any other only what its
	// call does with other running programs.
	var handed, other, given []*apiSymbol
	for _, s := range symbols {
		switch {
		case s.handsCallable:
			handed = append(handed, s)
		case s.wordCalls > 0:
			given = append(given, s)
		default:
			other = append(other, s)
		}
	}
	r.opts.Stage(lines.StageAPI, fmt.Sprintf("reading %d outside symbols: %d handed a callable, %d given words, %d given other values", len(symbols), len(handed), len(given), len(other)))
	groups := [][]*apiSymbol{handed, other, given}
	definitions := []table.Definition{lines.API(true), lines.API(false), lines.APIGiven()}
	rows := make([][]table.Row, len(groups))
	// Each round remembers every symbol's answer apart: a row is its own
	// subject, keyed by its symbol, so a new call asks only its symbol.
	subjects := make([]map[string]rowSubject, len(groups))
	files := make(map[string][]byte)
	for round, group := range groups {
		rows[round] = make([]table.Row, 0, len(group))
		subjects[round] = make(map[string]rowSubject, len(group))
		for i, s := range group {
			id := fmt.Sprintf("sym%d", i+1)
			path, line, column := s.usagePath, s.usageLine, s.usageColumn
			if round == 2 {
				// Asked what its words become, a symbol shows a call that
				// gives it words.
				path, line, column = s.wordUsagePath, s.wordUsageLine, s.wordUsageCol
			}
			subjects[round][id] = rowSubject{id: apiSubject(s.name), path: path, line: line}
			fields := []table.Field{{Name: "symbol", Value: s.name}}
			if s.signature != "" {
				fields = append(fields, table.Field{Name: "declared", Value: s.signature})
			}
			if usage := r.sourceText(files, path, line, column); usage != "" {
				fields = append(fields, table.Field{Name: "usage", Value: usage})
			}
			// Every literal: an oversized row goes alone into its own window
			// (FitClassifierWindows), never trimmed.
			if len(s.literals) > 0 {
				fields = append(fields, table.Field{Name: "literals", Value: s.literals})
			}
			if received := s.receivedCalls(); len(received) > 0 {
				fields = append(fields, table.Field{Name: "result_receives", Value: received})
			}
			if s.handsCallable {
				fields = append(fields, table.Field{Name: "hands_callable", Value: true})
			}
			rows[round] = append(rows[round], table.Row{ID: id, Fields: fields})
		}
	}
	// No round reads another. Each after the first runs on its own view,
	// joined after the first; a failure of any cancels the others and the
	// first failure is reported.
	asking, cancel := context.WithCancel(ctx)
	defer cancel()
	views := make([]*reader, len(groups))
	for round := range groups {
		if round == 0 {
			continue
		}
		views[round] = r.view(nil)
		views[round].rowSubjects = subjects[round]
	}
	r.rowSubjects = subjects[0]
	defer func() { r.rowSubjects = nil }()
	answers := make([][]rowAnswer, len(groups))
	failures := make([]error, len(groups))
	done := make(chan struct{})
	for round := 1; round < len(groups); round++ {
		go func(round int) {
			defer func() { done <- struct{}{} }()
			answers[round], failures[round] = views[round].runTable(asking, definitions[round], round+1, rows[round])
			if failures[round] != nil {
				cancel()
			}
		}(round)
	}
	answers[0], failures[0] = r.runTable(asking, definitions[0], 1, rows[0])
	if failures[0] != nil {
		cancel()
	}
	for round := 1; round < len(groups); round++ {
		<-done
	}
	for _, err := range failures {
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}
	for _, err := range failures {
		if err != nil {
			return err
		}
	}
	for round := 1; round < len(groups); round++ {
		r.joinView(views[round])
	}
	r.undecidedEnters = make(map[string]bool)
	for round, group := range groups {
		for i, s := range group {
			answer := answers[round][i].answer
			// A symbol asked what its words become with no decided answer
			// leaves each of its word calls unsure.
			if round == 2 && (answer == nil || answer["enters"] == "") {
				r.undecidedEnters[s.name] = true
			}
			if answer == nil {
				continue
			}
			answer = r.talksStands(rows[round][i].ID, s.name, answer)
			if role := apiRoleOf(answer); role != (apiRole{}) {
				r.api[s.name] = role
			}
		}
	}
	perCall := map[string]bool{}
	for name, role := range r.api {
		if role.perCall {
			perCall[name] = true
		}
	}
	if err := r.readCalls(ctx, perCall); err != nil {
		return err
	}
	r.reportStage(lines.StageAPI)
	return nil
}

// talksStands refuses an entry kind answered beside any answer but none of
// what the call does with other programs: a call that is the program's
// listening side stays that, and the words a call passes to another
// program, one it starts or one it sends to, are that program's, not an
// entry of this one. Only the enters cell is refused, and journaled.
func (r *reader) talksStands(rowID, symbol string, answer table.Answer) table.Answer {
	talks := answer["talks"]
	if talks == "" || talks == lines.APINone || answer["enters"] == "" || answer["enters"] == lines.APINone {
		return answer
	}
	reason := fmt.Sprintf("cell %q: %s beside %s: the talks answer stands", "enters", answer["enters"], talks)
	r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageAPI, Kind: "cell_rejected", Count: 1, Reason: reason, Samples: []string{rowID, symbol}})
	fmt.Fprintf(&r.tables, "- Rejected %s (%s): %s\n", rowID, symbol, reason)
	kept := maps.Clone(answer)
	delete(kept, "enters")
	return kept
}

// apiRoleOf restores one accepted api row's closed choices into the role the
// boundaries use: none is no role, a handed callable that becomes
// middleware binds no entry, and a call that serves, handed or not, is the
// program's listening side.
func apiRoleOf(answer table.Answer) apiRole {
	role := apiRole{publishes: answer["publishes"] == lines.APIServes}
	switch binds := answer["binds"]; binds {
	case "", lines.APINone:
	case lines.APIMiddleware:
		// A middleware binds no entry and starts nothing the program serves.
		return apiRole{middleware: true}
	default:
		role.binds = binds
	}
	switch talks := answer["talks"]; talks {
	case "", lines.APINone:
	case lines.APIServes:
		role.publishes = true
	default:
		role.talks = talks
	}
	switch enters := answer["enters"]; enters {
	case "", lines.APINone:
	case lines.APIPerCall:
		role.perCall = true
	default:
		role.enters = enters
	}
	return role
}

// apiRoles is the atlas record of the roles the model gave.
func (r *reader) apiRoles() []atlas.APIRole {
	names := make([]string, 0, len(r.api))
	for name := range r.api {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]atlas.APIRole, 0, len(names))
	for _, name := range names {
		role := r.api[name]
		result = append(result, atlas.APIRole{Symbol: name, Binds: role.binds, Publishes: role.publishes, Talks: role.talks, Enters: role.enters, Middleware: role.middleware})
	}
	return result
}

// applyAPIRoles turns the registrations into boundaries by their symbol's
// role: a handed callable becomes the entry the symbol binds, a call that
// publishes becomes the listener of its holder, a call to a symbol that
// talks to another system becomes that outgoing boundary. A registration
// whose symbol has no role is nothing. It returns the publishing states in
// boundary order, so the first of two publishes on one holder is always the
// same one.
func (r *reader) applyAPIRoles() []*boundaryState {
	var publishes []*boundaryState
	// The words a registration's call was given, as the call wrote them: a
	// registration's values keep only its address when it has one
	// (fs.String("socket", "/var/run/x.sock", …) keeps the path).
	callWords := make(map[sourceSite][]string)
	for _, place := range r.opts.Graph.Places {
		if place.Symbol == nil {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if len(call.Values) > 0 {
				callWords[sourceSite{place.Path, call.Line, call.Column}] = call.Values
			}
		}
	}
	for _, id := range sortedKeys(r.boundaries) {
		state := r.boundaries[id]
		b := state.place.Boundary
		if b.Source != "fact" || b.GivenKind != "" {
			continue
		}
		// A registration whose external symbol the code could not name has
		// no row and no role.
		role := r.api[b.External]
		facts := *b
		// A call given words, outside tests, to a symbol whose words are an
		// entry is that entry (a symbol that talks to other programs never
		// gives words an entry: talksStands refuses enters beside it). Its
		// handler is not established.
		words := callWords[sourceSite{state.place.Path, state.place.LineNo, state.place.Column}]
		if len(words) == 0 {
			words = b.Values
		}
		enters, undecided := r.entersAt(role, state.place.Path, state.place.LineNo, state.place.Column)
		if undecided && b.Direction != atlas.DirectionIn && !b.Handed && len(words) > 0 && !r.testFile(state.place.Parent) {
			r.recordWordCall(state.place, b.ObjectID, state.place.LineNo, state.place.Column, b.External, wordPerCallUndecided, "")
		}
		entry := b.Direction != atlas.DirectionIn && !b.Handed && enters != "" && len(words) > 0 && !r.testFile(state.place.Parent)
		if entry && len(lines.NameableWords(words)) == 0 {
			r.noEntryWithoutWords(state.place, enters)
			r.recordWordCall(state.place, b.ObjectID, state.place.LineNo, state.place.Column, b.External, wordNoWords, enters)
			entry = false
		}
		if entry {
			r.recordWordCall(state.place, b.ObjectID, state.place.LineNo, state.place.Column, b.External, wordEntry, enters)
		}
		switch {
		case b.Direction == atlas.DirectionIn:
			if role.binds == "" {
				delete(r.boundaries, id)
				continue
			}
			facts.GivenKind = role.binds
		case b.Handed && role.binds != "" && !role.publishes && role.talks == "":
			// A value handed to a binding symbol (Register("k6/x/dns",
			// new(DNS))) is an entry without a named callable: its handler
			// is not established, and the caller registering it is not taken
			// for it.
			facts.Direction, facts.GivenKind = atlas.DirectionIn, role.binds
			state.handlerUnknown = true
		case entry:
			facts.Direction, facts.GivenKind = atlas.DirectionIn, enters
			facts.Words = append([]string(nil), words...)
			state.handlerUnknown = true
		case role.publishes:
			facts.Direction, facts.GivenKind = atlas.DirectionIn, atlas.BoundaryListenAddress
		case role.talks != "":
			facts.GivenKind = role.talks
		default:
			delete(r.boundaries, id)
			continue
		}
		if role.publishes {
			state.address = publishAddress(facts.Values)
			publishes = append(publishes, state)
		}
		state.kind = facts.GivenKind
		state.place.Boundary = &facts
		r.places[id] = state.place
	}
	return publishes
}

// testFile reports a file place of test code.
func (r *reader) testFile(fileID string) bool {
	file := r.places[fileID]
	return file.File != nil && file.File.Test
}

// noEntryWithoutWords records a call whose words would be an entry but
// none of which can name one (a format with a line break): no entry is
// made and no other name is invented for it.
func (r *reader) noEntryWithoutWords(place atlas.Place, kind string) {
	reason := fmt.Sprintf("no %s entry at %s:%d: none of the words it is given can name it", kind, place.Path, place.LineNo)
	r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageBoundaries, Kind: "entry_unnamed", Count: 1, Reason: reason, Samples: []string{place.ID}})
	fmt.Fprintf(&r.tables, "- %s\n", reason)
}

// publishAddress is the literal among a publishing call's values that reads
// as an address: a host:port, a :port, a URL or a socket path. A module path
// or a name is not where the program listens.
func publishAddress(values []string) string {
	for _, value := range values {
		if strings.Contains(value, "://") || strings.HasPrefix(value, "/") {
			return value
		}
		if colon := strings.LastIndex(value, ":"); colon >= 0 && colon < len(value)-1 && strings.Trim(value[colon+1:], "0123456789") == "" {
			return value
		}
	}
	return ""
}

// joinPublishes gives the entries of a holder the address its publishing
// call states. A publishing call whose holder the code could not follow to
// any entry goes to the model with the holders that hold entries.
func (r *reader) joinPublishes(ctx context.Context, publishes []*boundaryState) error {
	type holder struct {
		ref     string
		entries []*boundaryState
	}
	byHolder := make(map[string]*holder)
	var keys []string
	for _, id := range sortedKeys(r.boundaries) {
		state := r.boundaries[id]
		b := state.place.Boundary
		if b.Source != "fact" || b.Direction != atlas.DirectionIn || b.ObjectID == "" || b.Holder == "" || state.handlerUnknown {
			continue
		}
		if byHolder[b.Holder] == nil {
			byHolder[b.Holder] = &holder{}
			keys = append(keys, b.Holder)
		}
		byHolder[b.Holder].entries = append(byHolder[b.Holder].entries, state)
	}
	give := func(entries []*boundaryState, address string) {
		for _, entry := range entries {
			if entry.address == "" {
				entry.address = address
			}
		}
	}
	var open []*boundaryState
	for _, publish := range publishes {
		if held := byHolder[publish.place.Boundary.Holder]; held != nil {
			give(held.entries, publish.address)
			continue
		}
		open = append(open, publish)
	}
	if len(open) == 0 || len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	var holders []map[string]any
	var options []string
	for i, key := range keys {
		byHolder[key].ref = fmt.Sprintf("h%d", i+1)
		options = append(options, byHolder[key].ref)
		var holds []string
		for _, entry := range byHolder[key].entries {
			b := entry.place.Boundary
			holds = append(holds, strings.TrimSpace(b.Method+" "+strings.Join(b.Values, " ")+" → "+b.Caller))
		}
		holders = append(holders, map[string]any{"ref": byHolder[key].ref, "holder": key, "holds": holds})
	}
	sort.Slice(open, func(i, j int) bool { return compactIDLess(open[i].place.ID, open[j].place.ID) })
	var rows []table.Row
	for i, publish := range open {
		b := publish.place.Boundary
		rows = append(rows, table.Row{ID: fmt.Sprintf("pub%d", i+1), Fields: []table.Field{
			{Name: "path", Value: publish.place.Path}, {Name: "line", Value: publish.place.LineNo},
			{Name: "caller", Value: b.Caller}, {Name: "symbol", Value: b.External}, {Name: "values", Value: b.Values},
		}})
	}
	shared := []table.Field{{Name: "holders", Value: holders}, {Name: "holder_options", Value: options}}
	r.opts.Stage(lines.StagePublish, fmt.Sprintf("%d publishing calls without a followed holder, %d holders", len(rows), len(holders)))
	answers, err := r.runTableWith(ctx, lines.Publish(), 1, shared, rows, nil)
	if err != nil {
		return err
	}
	for i, publish := range open {
		answer := answers[i].answer
		if answer == nil {
			continue
		}
		for _, key := range keys {
			if byHolder[key].ref == answer["holder"] {
				give(byHolder[key].entries, publish.address)
			}
		}
	}
	r.reportStage(lines.StagePublish)
	return nil
}

func appendUnique(values []string, more ...string) []string {
	for _, value := range more {
		if !contains(values, value) {
			values = append(values, value)
		}
	}
	return values
}

func sortedKeys(states map[string]*boundaryState) []string {
	keys := make([]string, 0, len(states))
	for key := range states {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return compactIDLess(keys[i], keys[j]) })
	return keys
}
