package reading

import (
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
)

// programDestinations are this repository's other programs a call the
// program target makes can reach while they run, in the targets' order,
// each with what it takes in: the requests it serves and the addresses it
// listens on, as their entries are named. A message it consumes comes from
// a queue, the system a producer's call reaches. A program taking none of
// these takes nothing a call reaches; a program is
// never its own destination (a replica's primary is another copy of it,
// named by what it is to the program). A fixture is offered only to the
// fixtures of its root, as joints join them. freqtrade-client's requests
// had reached "Freqtrade Server", an outside system beside freqtrade: its
// one call is a generic request whose path the code computes, and no one
// of freqtrade's inputs is its counterpart.
//
// A program is reached by a request sent or a connection opened to it
// (client_request, the talks answer an earlier step gave the call's
// symbol), so only a destination with such a call is offered one. A call
// through a library (sdk) reaches the system the library talks to:
// redis-cli's gethostbyname of the server's host is answered by the
// resolver, and offered redis-server it had been drawn into redis-server
// ("Redis server (host lookup)"), where it had been "DNS Resolver".
func (r *reader) programDestinations(target string, members []destinationMember) []lines.Destination {
	if !slices.ContainsFunc(members, func(member destinationMember) bool { return member.state.kind == atlas.BoundaryClientRequest }) {
		return nil
	}
	role := func(meta TargetMeta) string {
		if meta.SelectedRole != "" {
			return meta.SelectedRole
		}
		return lines.FallbackRole(meta.Kind)
	}
	var calling TargetMeta
	for _, meta := range r.opts.Targets {
		if meta.ID == target {
			calling = meta
		}
	}
	takes := make(map[string][]string)
	ids := make([]string, 0, len(r.boundaries))
	for id := range r.boundaries {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return compactIDLess(ids[i], ids[j]) })
	for _, id := range ids {
		state := r.boundaries[id]
		facts := state.place.Boundary
		if facts.Direction != atlas.DirectionIn || state.handlerUnknown {
			continue
		}
		switch state.kind {
		case atlas.BoundaryRequest, atlas.BoundaryListenAddress:
		default:
			continue
		}
		name := state.name
		if name == "" {
			name = strings.Join(facts.Values, " ")
		}
		// An entry named with its method's word says it once.
		method := facts.Method
		if slices.ContainsFunc(strings.Fields(name), func(word string) bool { return strings.EqualFold(word, method) }) {
			method = ""
		}
		text := strings.Join(strings.Fields(state.kind+" "+method+" "+name), " ")
		for _, program := range state.place.TargetIDs {
			if !slices.Contains(takes[program], text) {
				takes[program] = append(takes[program], text)
			}
		}
	}
	var programs []lines.Destination
	for _, meta := range r.opts.Targets {
		if meta.ID == target || len(takes[meta.ID]) == 0 {
			continue
		}
		if a, b := role(calling), role(meta); (a == atlas.RoleFixture || b == atlas.RoleFixture) && (a != b || fixtureRoot(calling.Root) != fixtureRoot(meta.Root)) {
			continue
		}
		programs = append(programs, lines.Destination{Value: meta.Name, Program: true, Takes: takes[meta.ID], Target: meta.ID})
	}
	return programs
}
