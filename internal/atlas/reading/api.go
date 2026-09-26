package reading

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/programindex"
)

// apiRole is the model's reading of one external symbol; see atlas.APIRole.
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
	// usage is the first site: the line a reader would look at. A symbol
	// only registrations name uses its first registration's line.
	usagePath        string
	usageLine        int
	registrationPath string
	registrationLine int
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
			// A registration hands a callable over when it brings work in;
			// its ObjectID is otherwise the declaration making the call, and
			// fopen("/dev/null") hands nothing.
			s.handsCallable = s.handsCallable || b.Direction == atlas.DirectionIn
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
				s.registrationPath, s.registrationLine = place.Path, place.LineNo
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
			if !inTest {
				s.sites++
			}
			s.literals = appendUnique(s.literals, call.Values...)
			if s.signature == "" {
				s.signature = call.API.Signature
			}
			if s.usagePath == "" && call.Line > 0 {
				s.usagePath, s.usageLine = place.Path, call.Line
			}
		}
	}
	result := make([]*apiSymbol, 0, len(byName))
	for _, s := range byName {
		if s.usagePath == "" {
			s.usagePath, s.usageLine = s.registrationPath, s.registrationLine
		}
		if s.sites > 0 {
			result = append(result, s)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result
}

// readAPI asks the model what the external symbols do with what the
// repository gives them, one row per symbol.
func (r *reader) readAPI(ctx context.Context) error {
	r.api = make(map[string]apiRole)
	symbols := r.apiSymbols()
	var handed, other []*apiSymbol
	for _, s := range symbols {
		if s.handsCallable {
			handed = append(handed, s)
		} else {
			other = append(other, s)
		}
	}
	r.opts.Stage(lines.StageAPI, fmt.Sprintf("reading %d outside symbols: %d handed a callable, %d given values", len(symbols), len(handed), len(other)))
	groups := [][]*apiSymbol{handed, other}
	rows := make([][]table.Row, len(groups))
	for round, group := range groups {
		rows[round] = make([]table.Row, 0, len(group))
		for i, s := range group {
			fields := []table.Field{{Name: "symbol", Value: s.name}}
			if s.signature != "" {
				fields = append(fields, table.Field{Name: "declared", Value: s.signature})
			}
			if usage := strings.TrimSpace(r.source(s.usagePath, s.usageLine, s.usageLine)); usage != "" {
				fields = append(fields, table.Field{Name: "usage", Value: usage})
			}
			if len(s.literals) > 0 {
				literals := s.literals
				if len(literals) > 6 {
					literals = literals[:6]
				}
				fields = append(fields, table.Field{Name: "literals", Value: literals})
			}
			if s.handsCallable {
				fields = append(fields, table.Field{Name: "hands_callable", Value: true})
			}
			rows[round] = append(rows[round], table.Row{ID: fmt.Sprintf("sym%d", i+1), Fields: fields})
		}
	}
	// The handed and the other symbols are asked at once: neither round
	// reads the other. The second runs on its own view, joined after the
	// first; a failure of the first cancels the second and is reported.
	asking, cancel := context.WithCancel(ctx)
	defer cancel()
	second := r.view(nil)
	var secondAnswers []rowAnswer
	var secondErr error
	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		secondAnswers, secondErr = second.runTable(asking, lines.API(false), 2, rows[1])
	}()
	firstAnswers, err := r.runTable(asking, lines.API(true), 1, rows[0])
	if err != nil {
		cancel()
	}
	<-secondDone
	if err != nil {
		return err
	}
	if secondErr != nil {
		return secondErr
	}
	r.joinView(second)
	for round, answers := range [][]rowAnswer{firstAnswers, secondAnswers} {
		for i, s := range groups[round] {
			answer := answers[i].answer
			if answer == nil {
				continue
			}
			if role := apiRoleOf(answer); role != (apiRole{}) {
				r.api[s.name] = role
			}
		}
	}
	r.reportStage(lines.StageAPI)
	return nil
}

// apiRoleOf reads one accepted api row into the role the boundaries use.
func apiRoleOf(answer table.Answer) apiRole {
	role := apiRole{
		binds: answer["binds"], talks: answer["talks"], publishes: answer["publishes"] == "yes",
		middleware: answer["middleware"] == "yes",
	}
	// A middleware binds no entry and starts nothing the program serves.
	if role.middleware {
		role.binds, role.publishes = "", false
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
		switch {
		case b.Direction == atlas.DirectionIn:
			if role.binds == "" {
				delete(r.boundaries, id)
				continue
			}
			facts.GivenKind = role.binds
		case b.Handed && role.binds != "" && !role.publishes && role.talks == "":
			// A value handed to a binding symbol (Register("k6/x/dns",
			// new(DNS))) is an entry without a named callable.
			facts.Direction, facts.GivenKind = atlas.DirectionIn, role.binds
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
		if b.Source != "fact" || b.Direction != atlas.DirectionIn || b.ObjectID == "" || b.Holder == "" {
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
