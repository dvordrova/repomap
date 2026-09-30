package reading

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/modeldiag"
	"github.com/dvordrova/repomap/internal/programindex"
)

// apiRole is the model's reading of one external symbol; see atlas.APIRole.
// What the words of its calls become is each call's own (api_call.go).
type apiRole struct {
	binds, talks          string
	publishes, middleware bool
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
	// called is whether code outside tests calls the symbol: a handed
	// symbol only registrations name (a table row's field) has no call
	// for its talks answer to decide.
	called bool
	// usage is the first site: the call a reader would look at, for a
	// symbol handed nothing the first that gives it words (a literal). A
	// symbol only registrations name uses its first registration.
	usagePath                            string
	usageLine, usageColumn               int
	registrationPath                     string
	registrationLine, registrationColumn int
	wordUsagePath                        string
	wordUsageLine, wordUsageCol          int
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
				s.called = true
			}
			s.literals = appendUnique(s.literals, call.Values...)
			if s.signature == "" {
				s.signature = call.API.Signature
			}
			if s.usagePath == "" && call.Line > 0 {
				s.usagePath, s.usageLine, s.usageColumn = place.Path, call.Line, call.Column
			}
			if !inTest && len(call.Values) > 0 && call.Line > 0 && s.wordUsagePath == "" {
				s.wordUsagePath, s.wordUsageLine, s.wordUsageCol = place.Path, call.Line, call.Column
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
		switch {
		case s.wordUsagePath != "" && !s.handsCallable:
			s.usagePath, s.usageLine, s.usageColumn = s.wordUsagePath, s.wordUsageLine, s.wordUsageCol
		case s.usagePath == "":
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
// (lines.CallText), reading and lexing each file once.
func (r *reader) sourceText(files map[string]*lines.CallFile, path string, line, column int) string {
	if file := r.callFile(files, path); file != nil && line >= 1 {
		return file.Text(line, column)
	}
	return ""
}

// rowText is a table's row as the code wrote it, bounded by the other
// rows' words (lines.CallFile.RowText).
func (r *reader) rowText(files map[string]*lines.CallFile, path string, line, column int, others [][2]int) string {
	if file := r.callFile(files, path); file != nil && line >= 1 {
		return file.RowText(line, column, others)
	}
	return ""
}

// callFile is a file lexed once, or nil when it cannot be read.
func (r *reader) callFile(files map[string]*lines.CallFile, path string) *lines.CallFile {
	if r.opts.ReadSource == nil {
		return nil
	}
	file, read := files[path]
	if !read {
		if content, err := r.opts.ReadSource(path); err == nil {
			file = lines.NewCallFile(content, path)
		}
		files[path] = file
	}
	return file
}

// apiSubject is the knowledge subject of an outside symbol's row: no
// compact place ID holds a colon, so it names no place.
func apiSubject(symbol string) string { return "api:" + symbol }

// readAPI asks the model what the external symbols do with what the
// repository gives them, one row per symbol, then what the words each call
// gives becomes (api_call.go).
func (r *reader) readAPI(ctx context.Context) error {
	r.api = make(map[string]apiRole)
	symbols := r.apiSymbols()
	// Three tables, asked at once, each symbol in exactly one: every symbol
	// the code calls is asked what its call does with other running
	// programs or with files, and a symbol handed a callable what the
	// callable becomes; a handed symbol no call names (a table row's field)
	// has no call to decide, and is asked only that.
	var handed, other, uncalled []*apiSymbol
	for _, s := range symbols {
		switch {
		case s.handsCallable && s.called:
			handed = append(handed, s)
		case s.handsCallable:
			uncalled = append(uncalled, s)
		default:
			other = append(other, s)
		}
	}
	r.opts.Stage(lines.StageAPI, fmt.Sprintf("reading %d outside symbols: %d what their calls do with other programs or files, %d what the callable they are handed becomes", len(symbols), len(handed)+len(other), len(handed)+len(uncalled)))
	groups := [][]*apiSymbol{handed, other, uncalled}
	definitions := []table.Definition{lines.API(true, true), lines.API(false, true), lines.API(true, false)}
	rounds := []int{1, 2, 5}
	rows := make([][]table.Row, len(groups))
	// Each round remembers every symbol's answer apart: a row is its own
	// subject, keyed by its symbol, so a new call asks only its symbol.
	subjects := make([]map[string]rowSubject, len(groups))
	files := make(map[string]*lines.CallFile)
	for round, group := range groups {
		rows[round] = make([]table.Row, 0, len(group))
		subjects[round] = make(map[string]rowSubject, len(group))
		for i, s := range group {
			id := fmt.Sprintf("sym%d", i+1)
			subjects[round][id] = rowSubject{id: apiSubject(s.name), path: s.usagePath, line: s.usageLine}
			fields := []table.Field{{Name: "symbol", Value: s.name}}
			if s.signature != "" {
				fields = append(fields, table.Field{Name: "declared", Value: s.signature})
			}
			if usage := r.sourceText(files, s.usagePath, s.usageLine, s.usageColumn); usage != "" {
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
	// The rounds read nothing of each other. The later ones run on views of
	// their own, joined after the first in order; a failure of any cancels
	// the others and the first failure is reported.
	asking, cancel := context.WithCancel(ctx)
	defer cancel()
	views := make([]*reader, len(groups))
	views[0] = r
	for round := 1; round < len(groups); round++ {
		views[round] = r.view(nil)
	}
	defer func() { r.rowSubjects = nil }()
	answers := make([][]rowAnswer, len(groups))
	failures := make([]error, len(groups))
	var asked sync.WaitGroup
	for round := range groups {
		views[round].rowSubjects = subjects[round]
		asked.Add(1)
		go func(round int) {
			defer asked.Done()
			answers[round], failures[round] = views[round].runTable(asking, definitions[round], rounds[round], rows[round])
			if failures[round] != nil {
				cancel()
			}
		}(round)
	}
	asked.Wait()
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
	for _, view := range views[1:] {
		r.joinView(view)
	}
	if err := r.readStarts(ctx); err != nil {
		return err
	}
	// The talks answer each symbol was given, none among them: a call's
	// words are asked only beside none or no decided answer.
	talks := map[string]string{}
	for round, group := range groups {
		for i, s := range group {
			answer := answers[round][i].answer
			if answer == nil {
				continue
			}
			talks[s.name] = answer["talks"]
			if role := apiRoleOf(answer); role != (apiRole{}) {
				r.api[s.name] = role
			}
		}
	}
	// The kept callables are decided before the calls: a call their
	// handlers make may compare words they were handed (sub_arguments.go).
	if err := r.readInputs(ctx); err != nil {
		return err
	}
	if err := r.readCalls(ctx, symbols, talks); err != nil {
		return err
	}
	// Which argument names what a call reaches reads the talks answers and
	// the options the calls' words declare.
	if err := r.readArguments(ctx, symbols, talks); err != nil {
		return err
	}
	r.reportStage(lines.StageAPI)
	return nil
}

// apiRoleOf restores one accepted api row's closed choices into the role the
// boundaries use: none is no role, a handed callable that becomes
// middleware binds no entry, and a call that serves, handed or not, is the
// program's listening side.
func apiRoleOf(answer table.Answer) apiRole {
	var role apiRole
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
		result = append(result, atlas.APIRole{Symbol: name, Binds: role.binds, Publishes: role.publishes, Talks: role.talks, Middleware: role.middleware})
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
		// A registration handing nothing over, outside tests, whose call's
		// answer says the words it is given are an entry is that entry
		// (api_call.go: a call whose symbol talks to other programs is never
		// asked). Its handler is not established.
		words := callWords[sourceSite{state.place.Path, state.place.LineNo, state.place.Column}]
		if len(words) == 0 {
			words = b.Values
		}
		enters := ""
		if b.Direction != atlas.DirectionIn && !b.Handed && len(words) > 0 && !r.testFile(state.place.Parent) {
			enters = r.readWordCall(state.place, id, b.ObjectID, state.place.LineNo, state.place.Column, b.External, words)
		}
		entry := enters != ""
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
		case role.talks != "" && role.talks != lines.APIFile:
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

// testFile reports a file place of test code: a file the target's adapter
// lists among its testing sources. It is the one test rule of the reading;
// testPath is the same rule by the file's path.
func (r *reader) testFile(fileID string) bool {
	file := r.places[fileID]
	return file.File != nil && file.File.Test
}

func (r *reader) testPath(filePath string) bool {
	return r.testPaths[filePath]
}

// dropTestBoundaries removes every boundary a test file makes once all are
// known. Test code is testing, not the program: its calls reach no input,
// outbound call or question, and its listeners no address.
func (r *reader) dropTestBoundaries(publishes []*boundaryState) []*boundaryState {
	for id, state := range r.boundaries {
		if r.testPath(state.place.Path) {
			delete(r.boundaries, id)
		}
	}
	kept := publishes[:0]
	for _, state := range publishes {
		if !r.testPath(state.place.Path) {
			kept = append(kept, state)
		}
	}
	return kept
}

// readWordCall records what the reading made of one call outside tests
// that gives words, by its own answer (api_call.go), and returns the entry
// kind it is, or "": an entry of the kind answered, none, undecided (unsure),
// or no entry when none of its words can name one (unsure). A call never
// asked is nothing and not recorded.
func (r *reader) readWordCall(running atlas.Place, sampleID, objectID string, line, column int, symbol string, words []string) string {
	answer, asked := r.callEnters[sourceSite{running.Path, line, column}]
	switch {
	case !asked:
	case answer == "":
		r.recordWordCall(running, objectID, line, column, symbol, wordUndecided, "")
	case answer == lines.APINone:
		r.recordWordCall(running, objectID, line, column, symbol, wordNone, "")
	case len(lines.NameableWords(words)) == 0:
		r.noEntryWithoutWords(atlas.Place{ID: sampleID, Path: running.Path, LineNo: line}, answer)
		r.recordWordCall(running, objectID, line, column, symbol, wordNoWords, "")
	default:
		r.recordWordCall(running, objectID, line, column, symbol, wordEntry, answer)
		return answer
	}
	return ""
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
