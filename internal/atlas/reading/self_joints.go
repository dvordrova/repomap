package reading

import (
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
)

// selfJoints are a program's requests to what it serves itself: an
// outgoing request whose written address is on this machine (a path with
// no host, a loopback host, a unix socket) and whose method and path, or
// value, are those of one of the same program's own requests or listening
// addresses (valuesJoin, the equal-value rule of joints between programs).
// It is an exchange of the program with itself, joined to that input as a
// call of another program is (joint, source kind integration), never an
// outside system: litestream's subcommands post to http://localhost/start
// and /info over the unix socket its own server listens on, and had stood
// as an Outside "Litestream" beside the routes /start and /info its own
// Server serves (as a program starting itself runs its own entry, 28fcda00).
// A request to another host with the same path is another server's. A
// unix socket dialled where the program listens on one is joined as
// possible: the socket's path is known only at run time (litestream's
// clients dial --socket, its Server listens on its configured path).
func (r *reader) selfJoints() []atlas.Joint {
	var outs, ins []*boundaryState
	for _, state := range r.boundaries {
		b := state.place.Boundary
		switch {
		case b == nil || r.testFile(state.place.Parent):
		case b.Direction == atlas.DirectionOut && state.kind == atlas.BoundaryClientRequest && slices.ContainsFunc(b.Values, localAddress):
			outs = append(outs, state)
		case b.Direction == atlas.DirectionIn && !state.handlerUnknown && (state.kind == atlas.BoundaryRequest || state.kind == atlas.BoundaryListenAddress):
			ins = append(ins, state)
		}
	}
	sort.Slice(outs, func(i, j int) bool { return compactIDLess(outs[i].place.ID, outs[j].place.ID) })
	sort.Slice(ins, func(i, j int) bool { return compactIDLess(ins[i].place.ID, ins[j].place.ID) })
	var joints []atlas.Joint
	for _, out := range outs {
		for _, in := range ins {
			if out.place.Boundary.ObjectID != "" && out.place.Boundary.ObjectID == in.place.Boundary.ObjectID {
				continue
			}
			value, possible, ok := valuesJoin(out.place.Boundary, in.place.Boundary)
			if !ok {
				continue
			}
			possible = possible || value == "unix" || out.place.Boundary.Source == "model" || in.place.Boundary.Source == "model"
			for _, target := range rowTargets(out) {
				if !slices.Contains(in.place.TargetIDs, target) {
					continue
				}
				joints = append(joints, atlas.Joint{
					ID:    r.compactID("j", &r.nextJoint),
					From:  atlas.Endpoint{TargetID: target, BoundaryID: out.place.ID},
					To:    atlas.Endpoint{TargetID: target, BoundaryID: in.place.ID},
					Value: value, Same: true, Possible: possible, SourceKind: "integration",
				})
			}
		}
	}
	return joints
}

// localAddress reports an address written on this machine: a path with no
// host, a URL whose host is a loopback name or address, or the unix
// network a socket is dialled on.
func localAddress(value string) bool {
	switch {
	case value == "unix" || strings.HasPrefix(value, "/"):
		return true
	case strings.Contains(value, "://"):
		host := value[strings.Index(value, "://")+len("://"):]
		if end := strings.IndexAny(host, "/?#"); end >= 0 {
			host = host[:end]
		}
		if at := strings.LastIndex(host, "@"); at >= 0 {
			host = host[at+1:]
		}
		if strings.HasPrefix(host, "[") {
			if end := strings.Index(host, "]"); end >= 0 {
				host = host[1:end]
			}
		} else if colon := strings.LastIndex(host, ":"); colon >= 0 {
			host = host[:colon]
		}
		switch strings.ToLower(host) {
		case "localhost", "127.0.0.1", "::1", "0.0.0.0":
			return true
		}
	}
	return false
}
