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
//
// A call into a package this repository builds (repositoryPackage) goes
// through the repository's own code, and is offered the programs too:
// etcd's clients and tools call go.etcd.io/etcd/client/v3's KV.Get (a db
// call) and the api module's gRPC stubs, which the systems question had
// named "etcd" and "Etcd Server" as outside systems beside the server
// program they reach.
func (r *reader) programDestinations(target string, members []destinationMember) []lines.Destination {
	if !slices.ContainsFunc(members, func(member destinationMember) bool {
		return member.state.kind == atlas.BoundaryClientRequest || r.repositoryPackage(member.state.outside)
	}) {
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
		programs = append(programs, lines.Destination{Value: r.programName(meta), Program: true, Takes: takes[meta.ID], Target: meta.ID})
	}
	return programs
}

// programNames are, by target, the names the destination question offers
// this repository's programs by: the one name the toolchain gives
// a program's executable (TargetMeta.Executables), the name the report
// titles it by, else its target name. casdoor's server was offered as
// github.com/casdoor/casdoor, and its web's calls to it were answered
// "other: Casdoor" in three windows of six; offered "casdoor", that answer
// writes the offered name. A program keeps its target name where its
// executable's name is another program's, or a system's the systems
// question gave any package of the run (names, case aside): two things the
// question offers, and the destinations the page joins by name, are never
// spelled alike. Facts only: no name is composed or shortened.
func programNames(targets []TargetMeta, names map[string]string) map[string]string {
	systems := make(map[string]bool, len(names))
	for _, name := range names {
		if name != "" {
			systems[strings.ToLower(name)] = true
		}
	}
	executable := func(meta TargetMeta) string {
		if len(meta.Executables) == 1 && meta.Executables[0] != "" && !systems[strings.ToLower(meta.Executables[0])] {
			return meta.Executables[0]
		}
		return ""
	}
	count := make(map[string]int, len(targets))
	for _, meta := range targets {
		if name := executable(meta); name != "" {
			count[strings.ToLower(name)]++
		}
	}
	result := make(map[string]string, len(targets))
	for _, meta := range targets {
		result[meta.ID] = meta.Name
		if name := executable(meta); name != "" && count[strings.ToLower(name)] == 1 && !slices.ContainsFunc(targets, func(other TargetMeta) bool {
			return other.ID != meta.ID && strings.EqualFold(other.Name, name) && executable(other) == ""
		}) {
			result[meta.ID] = name
		}
	}
	return result
}

// programName is the name a program is offered and called by
// (programNames), its target name before the systems are named.
func (r *reader) programName(meta TargetMeta) string {
	if name := r.programNamesOf[meta.ID]; name != "" {
		return name
	}
	return meta.Name
}

// repositoryPackage reports a Go package this repository builds: one of a
// Go program's module path or a package below it (go.etcd.io/etcd/client/v3
// and go.etcd.io/etcd/client/v3/concurrency, the client/v3 library's). Such
// a package is the repository's own code, never an outside system, though
// a program that does not load it calls it as an outside symbol. Python,
// JS/TS, C and Clojure name no program by its import path here: a recorded
// missing equivalent.
func (r *reader) repositoryPackage(pkg string) bool {
	if pkg == "" {
		return false
	}
	for _, target := range r.opts.Targets {
		if target.Language == "go" && target.Name != "" && (pkg == target.Name || strings.HasPrefix(pkg, target.Name+"/")) {
			return true
		}
	}
	return false
}
