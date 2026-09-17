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
	binds, talks string
	publishes    bool
}

// apiSymbol is what the code observed about one external symbol across the
// repository: the call word, whether a repository callable was handed to it,
// the literals it was given, its sites and the holders it acts on.
type apiSymbol struct {
	name          string
	word          string
	signature     string
	handsCallable bool
	literals      []string
	sites         int
	holders       map[string]bool
	// usage is the first site: the line a reader would look at.
	usagePath string
	usageLine int
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
			byName[name] = &apiSymbol{name: name, word: name[strings.LastIndex(name, ".")+1:], holders: make(map[string]bool)}
		}
		return byName[name]
	}
	for _, place := range r.opts.Graph.Places {
		if b := place.Boundary; b != nil && b.Source == "fact" && b.External != "" && b.GivenKind == "" {
			s := symbol(b.External)
			s.handsCallable = s.handsCallable || b.ObjectID != ""
			s.literals = appendUnique(s.literals, b.Values...)
			if b.Holder != "" {
				s.holders[b.Holder] = true
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
			s.sites++
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
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result
}

// readAPI asks the model what the external symbols do with what the
// repository gives them, one row per symbol.
func (r *reader) readAPI(ctx context.Context) error {
	r.api = make(map[string]apiRole)
	symbols := r.apiSymbols()
	holdersOf := make(map[string][]string)
	for _, s := range symbols {
		for holder := range s.holders {
			holdersOf[holder] = append(holdersOf[holder], s.name)
		}
	}
	var rows []table.Row
	for i, s := range symbols {
		fields := []table.Field{{Name: "symbol", Value: s.name}, {Name: "word", Value: s.word}}
		if s.signature != "" {
			fields = append(fields, table.Field{Name: "declared", Value: s.signature})
		}
		if usage := strings.TrimSpace(r.source(s.usagePath, s.usageLine, s.usageLine)); usage != "" {
			fields = append(fields, table.Field{Name: "usage", Value: usage})
		}
		if s.handsCallable {
			fields = append(fields, table.Field{Name: "hands_callable", Value: true})
		}
		if len(s.literals) > 0 {
			literals := s.literals
			if len(literals) > 6 {
				literals = literals[:6]
			}
			fields = append(fields, table.Field{Name: "literals", Value: literals})
		}
		if s.sites > 0 {
			fields = append(fields, table.Field{Name: "sites", Value: s.sites})
		}
		var beside []string
		for holder := range s.holders {
			for _, other := range holdersOf[holder] {
				if other != s.name {
					beside = appendUnique(beside, other)
				}
			}
		}
		if len(beside) > 0 {
			sort.Strings(beside)
			fields = append(fields, table.Field{Name: "beside", Value: beside})
		}
		rows = append(rows, table.Row{ID: fmt.Sprintf("sym%d", i+1), Fields: fields})
	}
	r.opts.Stage(lines.StageAPI, fmt.Sprintf("reading %d external symbols: what they bind, publish or talk to", len(rows)))
	answers, err := r.runTable(ctx, lines.API(), 1, rows)
	if err != nil {
		return err
	}
	for i, s := range symbols {
		answer := answers[i].answer
		if answer == nil {
			continue
		}
		role := apiRole{binds: answer["binds"], talks: answer["talks"], publishes: answer["publishes"] == "yes"}
		if role != (apiRole{}) {
			r.api[s.name] = role
		}
	}
	r.reportStage(lines.StageAPI)
	return nil
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
		result = append(result, atlas.APIRole{Symbol: name, Binds: role.binds, Publishes: role.publishes, Talks: role.talks})
	}
	return result
}

// applyAPIRoles turns the registrations into boundaries by their symbol's
// role: a handed callable becomes the entry the symbol binds, a call that
// publishes becomes the listener of its holder, a call to a symbol that
// talks to another system becomes that outgoing boundary. A registration
// whose symbol has no role is nothing. It returns the publishing states.
func (r *reader) applyAPIRoles() []*boundaryState {
	var publishes []*boundaryState
	for id, state := range r.boundaries {
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
		case role.binds != "" && !role.publishes && role.talks == "":
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
