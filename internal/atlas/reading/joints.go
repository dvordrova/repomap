package reading

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

// targetState is what the portfolio table said about a target.
type targetState struct {
	line string
	role string
}

// readTargets asks one line and a role per target when the run has more
// than one; a single target keeps its root directory's line.
func (r *reader) readTargets(ctx context.Context) error {
	r.targets = make(map[string]*targetState)
	for _, target := range r.opts.Targets {
		state := &targetState{role: lines.FallbackRole(target.Root, target.Kind)}
		if line, ok := r.lines[atlas.DirectoryID(target.Root)]; ok {
			state.line = line.value
		}
		r.targets[target.ID] = state
	}
	if len(r.opts.Targets) < 2 {
		return nil
	}
	def := lines.Targets()
	rows := make([]table.Row, 0, len(r.opts.Targets))
	ordered := append([]TargetMeta(nil), r.opts.Targets...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	for _, target := range ordered {
		rows = append(rows, lines.TargetRow(r.targetSummary(target)))
	}
	r.opts.Stage(def.Stage, fmt.Sprintf("%d targets in the portfolio", len(rows)))
	answers, err := r.runTable(ctx, def, 1, rows)
	if err != nil {
		return err
	}
	for i, target := range ordered {
		if answer := answers[i]; answer.answer != nil {
			r.targets[target.ID].line = answer.answer["line"]
			r.targets[target.ID].role = answer.answer["role"]
		}
	}
	r.reportStage(def.Stage)
	return nil
}

func (r *reader) targetSummary(target TargetMeta) lines.TargetSummary {
	summary := lines.TargetSummary{
		ID: target.ID, Name: target.Name, Root: target.Root, Language: target.Language, Kind: target.Kind,
		Boundaries: make(map[string]int),
	}
	if root, ok := r.places[atlas.DirectoryID(target.Root)]; ok {
		summary.Readme = root.Directory.Readme
		if summary.Readme == "" {
			summary.Readme = root.Directory.Doc
		}
	}
	dirs := make(map[string]struct{})
	for _, place := range r.opts.Graph.Places {
		if place.Kind != atlas.PlaceFile || !contains(place.TargetIDs, target.ID) {
			continue
		}
		summary.Files++
		dirs[parentDir(place.Path)] = struct{}{}
	}
	summary.Dirs = len(dirs)
	for _, seed := range r.opts.Graph.Seeds {
		if contains(r.places[seed].TargetIDs, target.ID) {
			summary.Entrypoint = r.places[seed].Path
			break
		}
	}
	for _, state := range r.boundaries {
		if contains(state.place.TargetIDs, target.ID) {
			summary.Boundaries[state.kind+" "+state.place.Boundary.Direction]++
		}
	}
	return summary
}

// jointState is one candidate joint and what the model said about it.
type jointState struct {
	joint atlas.Joint
	from  *boundaryState
	to    *boundaryState
}

// compatibleRoles says which targets may be joined: products with products
// and libraries, fixtures with fixtures under the same fixture root, tools
// and examples with nothing.
func (r *reader) compatible(a, b TargetMeta) bool {
	sa, sb := r.targets[a.ID], r.targets[b.ID]
	if sa == nil || sb == nil {
		return false
	}
	ra, rb := sa.role, sb.role
	switch {
	case ra == atlas.RoleTool || rb == atlas.RoleTool || ra == atlas.RoleExample || rb == atlas.RoleExample:
		return false
	case ra == atlas.RoleFixture || rb == atlas.RoleFixture:
		return ra == rb && fixtureRoot(a.Root) == fixtureRoot(b.Root)
	default:
		return true
	}
}

// fixtureRoot is the directory a fixture target lives under: one level
// beneath the first fixture marker, so backend and front of one sample
// repository share it.
func fixtureRoot(root string) string {
	parts := strings.Split(root, "/")
	for i, part := range parts {
		switch strings.ToLower(part) {
		case "testdata", "fixtures", "fixture", "test", "tests":
			if i+1 < len(parts) {
				return strings.Join(parts[:i+2], "/")
			}
			return strings.Join(parts[:i+1], "/")
		}
	}
	if len(parts) > 0 {
		return parts[0]
	}
	return root
}

// readJoints finds the joints between targets: candidates by equal value
// are confirmed one by one; outgoing boundaries without a candidate choose
// a peer from the closed list of the other targets' incoming boundaries.
func (r *reader) readJoints(ctx context.Context) error {
	r.joints = nil
	if len(r.opts.Targets) < 2 {
		return nil
	}
	byTarget := make(map[string]TargetMeta, len(r.opts.Targets))
	for _, target := range r.opts.Targets {
		byTarget[target.ID] = target
	}
	var outs, ins []*boundaryState
	for _, state := range r.boundaries {
		if state.kind == atlas.BoundaryConfig || state.kind == atlas.BoundaryDB {
			continue
		}
		if state.place.Boundary.Direction == atlas.DirectionOut {
			outs = append(outs, state)
		} else {
			ins = append(ins, state)
		}
	}
	sort.Slice(outs, func(i, j int) bool { return outs[i].place.ID < outs[j].place.ID })
	sort.Slice(ins, func(i, j int) bool { return ins[i].place.ID < ins[j].place.ID })
	var candidates []*jointState
	matched := make(map[string]bool)
	for _, out := range outs {
		for _, in := range ins {
			for _, fromTarget := range out.place.TargetIDs {
				for _, toTarget := range in.place.TargetIDs {
					if fromTarget == toTarget || !r.compatible(byTarget[fromTarget], byTarget[toTarget]) {
						continue
					}
					value, possible, ok := valuesJoin(out.place.Boundary, in.place.Boundary)
					if !ok {
						continue
					}
					matched[out.place.ID] = true
					candidates = append(candidates, &jointState{
						from: out, to: in,
						joint: atlas.Joint{
							ID:    fmt.Sprintf("joint:%s->%s", out.place.ID, in.place.ID),
							From:  atlas.Endpoint{TargetID: fromTarget, BoundaryID: out.place.ID},
							To:    atlas.Endpoint{TargetID: toTarget, BoundaryID: in.place.ID},
							Value: value, Possible: possible,
						},
					})
				}
			}
		}
	}
	def := lines.Joints()
	r.opts.Stage(def.Stage, fmt.Sprintf("%d candidate joints by shared value", len(candidates)))
	if len(candidates) > 0 {
		rows := make([]table.Row, 0, len(candidates))
		for _, candidate := range candidates {
			rows = append(rows, lines.JointRow(candidate.joint.ID, candidate.joint.Value, r.sideOf(candidate.from, byTarget), r.sideOf(candidate.to, byTarget)))
		}
		answers, err := r.runTableWith(ctx, def, 1, []table.Field{{Name: "question", Value: "joints"}}, rows, nil)
		if err != nil {
			return err
		}
		for i, candidate := range candidates {
			answer := answers[i]
			if answer.answer == nil {
				continue
			}
			if answer.answer["same"] == "yes" {
				candidate.joint.Same = true
				candidate.joint.Label = strings.Trim(answer.answer["label"], "- ")
				r.joints = append(r.joints, candidate.joint)
			}
		}
	}
	// Blind peers: outgoing boundaries of the kinds that reach a service,
	// with no candidate, against every compatible incoming boundary.
	var blind []*boundaryState
	for _, out := range outs {
		if matched[out.place.ID] {
			continue
		}
		switch out.kind {
		case atlas.BoundaryHTTPClient, atlas.BoundaryQueueProducer, atlas.BoundarySDK:
			blind = append(blind, out)
		}
	}
	if len(blind) > 0 && len(ins) > 0 {
		peersDef := lines.Peers()
		type peerRef struct {
			ref  string
			in   *boundaryState
			side lines.BoundarySide
		}
		var refs []string
		var sides []lines.BoundarySide
		var peers []peerRef
		for _, in := range ins {
			if len(refs) == 40 {
				break
			}
			ref := fmt.Sprintf("p%d", len(refs)+1)
			side := r.sideOf(in, byTarget)
			refs = append(refs, ref)
			sides = append(sides, side)
			peers = append(peers, peerRef{ref: ref, in: in, side: side})
		}
		rows := make([]table.Row, 0, len(blind))
		for _, out := range blind {
			rows = append(rows, lines.PeerRow(out.place.ID, r.sideOf(out, byTarget), refs))
		}
		r.opts.Stage(def.Stage, fmt.Sprintf("%d outgoing boundaries without a value match, %d incoming peers to choose from", len(blind), len(refs)))
		answers, err := r.runTableWith(ctx, peersDef, 2, []table.Field{{Name: "question", Value: "peers"}, lines.PeerContext(refs, sides)}, rows, nil)
		if err != nil {
			return err
		}
		for i, out := range blind {
			answer := answers[i]
			if answer.answer == nil || answer.answer["peer"] == lines.PeerNone {
				continue
			}
			for _, peer := range peers {
				if peer.ref != answer.answer["peer"] {
					continue
				}
				for _, fromTarget := range out.place.TargetIDs {
					for _, toTarget := range peer.in.place.TargetIDs {
						if fromTarget == toTarget || !r.compatible(byTarget[fromTarget], byTarget[toTarget]) {
							continue
						}
						r.joints = append(r.joints, atlas.Joint{
							ID:    fmt.Sprintf("joint:%s->%s", out.place.ID, peer.in.place.ID),
							From:  atlas.Endpoint{TargetID: fromTarget, BoundaryID: out.place.ID},
							To:    atlas.Endpoint{TargetID: toTarget, BoundaryID: peer.in.place.ID},
							Value: strings.Join(out.place.Boundary.Values, ", "),
							Same:  true, Label: strings.Trim(answer.answer["label"], "- "),
							Possible: true, Blind: true,
						})
					}
				}
			}
		}
	}
	r.joints = append(r.joints, r.linkJoints()...)
	sort.Slice(r.joints, func(i, j int) bool { return r.joints[i].ID < r.joints[j].ID })
	r.reportStage(def.Stage)
	return nil
}

// linkJoints are the joints the code sees without a value: a call or an
// import from a box of one target into a box of another. A workspace
// package of the same target is no boundary at all; one of another target
// is the seam between them, and it is exact.
func (r *reader) linkJoints() []atlas.Joint {
	type key struct{ from, fromBox, to, toBox string }
	seen := make(map[key]bool)
	weight := make(map[key]int)
	var joints []atlas.Joint
	for _, edge := range r.opts.Graph.Edges {
		fromBox, toBox := r.boxOfPlace(edge.From), r.boxOfPlace(edge.To)
		if fromBox == "" || toBox == "" || fromBox == toBox {
			continue
		}
		for _, fromTarget := range r.places[edge.From].TargetIDs {
			if r.targetFiles(r.boxes[fromBox], fromTarget) == 0 {
				continue
			}
			for _, toTarget := range r.places[edge.To].TargetIDs {
				if fromTarget == toTarget || r.targetFiles(r.boxes[toBox], toTarget) == 0 {
					continue
				}
				if contains(r.places[edge.To].TargetIDs, fromTarget) {
					// The callee is this target's own file too: an internal call.
					continue
				}
				if !r.underRoot(r.boxes[toBox].dir, toTarget) {
					// The callee is a shared package the other target merely
					// indexed: etcd's client module loads only its own
					// packages, while etcdctl loads api/* transitively, so
					// api/* looked like etcdctl's and the client seemed to
					// depend on the command.
					continue
				}
				k := key{fromTarget, fromBox, toTarget, toBox}
				weight[k] += edge.Count
				if seen[k] {
					continue
				}
				seen[k] = true
				joints = append(joints, atlas.Joint{
					ID:    fmt.Sprintf("joint:%s:%s->%s:%s", fromTarget, fromBox, toTarget, toBox),
					From:  atlas.Endpoint{TargetID: fromTarget, BoxID: fromBox},
					To:    atlas.Endpoint{TargetID: toTarget, BoxID: toBox},
					Value: edge.Kind + " " + r.boxes[toBox].dir,
					Same:  true, Label: "uses " + r.boxes[toBox].title,
				})
			}
		}
	}
	// A pair of targets keeps its five busiest seams: etcd's twenty-seven
	// targets produced 2,456 box-to-box links, which is a listing, not a map.
	sort.SliceStable(joints, func(i, j int) bool {
		a := key{joints[i].From.TargetID, joints[i].From.BoxID, joints[i].To.TargetID, joints[i].To.BoxID}
		b := key{joints[j].From.TargetID, joints[j].From.BoxID, joints[j].To.TargetID, joints[j].To.BoxID}
		return weight[a] > weight[b]
	})
	perPair := make(map[[2]string]int)
	kept := joints[:0]
	for _, joint := range joints {
		pair := [2]string{joint.From.TargetID, joint.To.TargetID}
		if perPair[pair] == maxLinkJointsPerPair {
			continue
		}
		perPair[pair]++
		kept = append(kept, joint)
	}
	return kept
}

// maxLinkJointsPerPair bounds the seams drawn between two targets.
const maxLinkJointsPerPair = 5

// underRoot says whether a directory lies under a target's root. A target
// rooted at the repository owns every directory.
func (r *reader) underRoot(dir, targetID string) bool {
	for _, target := range r.opts.Targets {
		if target.ID != targetID {
			continue
		}
		root := strings.TrimPrefix(strings.Trim(target.Root, "/"), "./")
		return root == "" || root == "." || dir == root || strings.HasPrefix(dir, root+"/")
	}
	return false
}

func (r *reader) sideOf(state *boundaryState, byTarget map[string]TargetMeta) lines.BoundarySide {
	name := ""
	if len(state.place.TargetIDs) > 0 {
		name = byTarget[state.place.TargetIDs[0]].Name
	}
	return lines.BoundarySide{
		Target: name, Line: state.line, External: state.place.Boundary.External,
		Method: state.place.Boundary.Method, Values: state.place.Boundary.Values,
	}
}

// valuesJoin says whether an outgoing and an incoming boundary share a
// value: an HTTP method and path matched segment by segment with holes, or
// an equal literal for everything else. Possible marks a match through a
// parameter.
func valuesJoin(out, in *atlas.BoundaryFacts) (string, bool, bool) {
	if out.Method != "" || in.Method != "" {
		if !methodsMatch(in.Method, out.Method) {
			return "", false, false
		}
		for _, callPath := range out.Values {
			for _, routePath := range in.Values {
				if match, possible := pathsMatch(routePath, callPath); match {
					method := out.Method
					if method == "" {
						method = in.Method
					}
					return strings.TrimSpace(method + " " + routePath), possible, true
				}
			}
		}
		return "", false, false
	}
	for _, a := range out.Values {
		for _, b := range in.Values {
			if a != "" && strings.EqualFold(a, b) {
				return b, strings.Contains(a, "{param}"), true
			}
		}
	}
	return "", false, false
}

func methodsMatch(routeMethod, callMethod string) bool {
	return routeMethod == "" || callMethod == "" || routeMethod == "ANY" || strings.EqualFold(routeMethod, callMethod)
}

// pathsMatch is the facts rule: segment by segment, a route parameter or a
// template hole matches one segment.
func pathsMatch(routePath, callPath string) (bool, bool) {
	routeSegments := pathSegments(routePath)
	callSegments := pathSegments(stripOrigin(callPath))
	if len(routeSegments) != len(callSegments) || len(routeSegments) == 0 {
		return false, false
	}
	possible := false
	for i := range routeSegments {
		switch {
		case routeSegments[i] == callSegments[i]:
		case isRouteParameter(routeSegments[i]) || callSegments[i] == "{param}":
			possible = true
		default:
			return false, false
		}
	}
	return true, possible
}

func pathSegments(value string) []string {
	if question := strings.Index(value, "?"); question >= 0 {
		value = value[:question]
	}
	value = strings.Trim(value, "/")
	if value == "" {
		return nil
	}
	return strings.Split(value, "/")
}

func stripOrigin(value string) string {
	for _, scheme := range []string{"http://", "https://"} {
		if !strings.HasPrefix(value, scheme) {
			continue
		}
		rest := value[len(scheme):]
		slash := strings.Index(rest, "/")
		if slash < 0 {
			return "/"
		}
		return rest[slash:]
	}
	return value
}

func isRouteParameter(segment string) bool {
	switch {
	case strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}"):
		return true
	case strings.HasPrefix(segment, "<") && strings.HasSuffix(segment, ">"):
		return true
	case strings.HasPrefix(segment, ":") && len(segment) > 1:
		return true
	case segment == "*":
		return true
	default:
		return false
	}
}
