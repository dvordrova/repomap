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
		if target.SelectedRole != "" {
			state.role = target.SelectedRole
		}
		if line, ok := r.lines[atlas.DirectoryID(target.Root)]; ok {
			state.line = line.value
		}
		r.targets[target.ID] = state
	}
	if len(r.opts.Targets) < 2 {
		return nil
	}
	rolesBound := true
	for _, target := range r.opts.Targets {
		rolesBound = rolesBound && target.SelectedRole != ""
	}
	def := lines.Targets(rolesBound)
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
			if target.SelectedRole == "" {
				r.targets[target.ID].role = answer.answer["role"]
			}
		}
	}
	r.reportStage(def.Stage)
	return nil
}

func (r *reader) targetSummary(target TargetMeta) lines.TargetSummary {
	summary := lines.TargetSummary{
		ID: target.ID, Name: target.Name, Root: target.Root, Language: target.Language, Kind: target.Kind,
		SelectedRole: target.SelectedRole,
		Boundaries:   make(map[string]int),
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
	seenOperations := make(map[string]bool)
	for id, operation := range r.operations {
		if contains(r.places[id].TargetIDs, target.ID) {
			name := operation[0] + ": " + operation[1]
			if !seenOperations[name] {
				summary.Operations = append(summary.Operations, name)
				seenOperations[name] = true
			}
		}
	}
	sort.Strings(summary.Operations)
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

// A tool or example may call any real service. Fixture pairs remain isolated
// within their fixture root so identical sample endpoints do not cross-join.
func (r *reader) compatible(a, b TargetMeta) bool {
	sa, sb := r.targets[a.ID], r.targets[b.ID]
	if sa == nil || sb == nil {
		return false
	}
	ra, rb := sa.role, sb.role
	switch {
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
			if out.place.Boundary.ObjectID != "" && out.place.Boundary.ObjectID == in.place.Boundary.ObjectID {
				continue
			}
			for _, fromTarget := range out.place.TargetIDs {
				for _, toTarget := range in.place.TargetIDs {
					if fromTarget == toTarget || !r.compatible(byTarget[fromTarget], byTarget[toTarget]) {
						continue
					}
					value, possible, ok := valuesJoin(out.place.Boundary, in.place.Boundary)
					if !ok {
						continue
					}
					candidates = append(candidates, &jointState{
						from: out, to: in,
						joint: atlas.Joint{
							ID:    fmt.Sprintf("joint:%s:%s->%s:%s", fromTarget, out.place.ID, toTarget, in.place.ID),
							From:  atlas.Endpoint{TargetID: fromTarget, BoundaryID: out.place.ID},
							To:    atlas.Endpoint{TargetID: toTarget, BoundaryID: in.place.ID},
							Value: value, Possible: possible || out.place.Boundary.Source == "model" || in.place.Boundary.Source == "model", SourceKind: "integration",
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
				matched[candidate.from.place.ID] = true
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
	// Every eligible endpoint participates. Window choices are candidates for
	// one final counterpart, not separate published integrations.
	round := 2
	choices, err := r.chooseBlindPeers(ctx, []peerBatch{{outs: blind, ins: ins}}, byTarget, &round)
	if err != nil {
		return err
	}
	for _, out := range blind {
		peer, ok := choices[out.place.ID]
		if !ok {
			continue
		}
		for _, fromTarget := range out.place.TargetIDs {
			for _, toTarget := range peer.in.place.TargetIDs {
				if fromTarget == toTarget || !r.compatible(byTarget[fromTarget], byTarget[toTarget]) {
					continue
				}
				r.joints = append(r.joints, atlas.Joint{
					ID:    fmt.Sprintf("joint:%s:%s->%s:%s", fromTarget, out.place.ID, toTarget, peer.in.place.ID),
					From:  atlas.Endpoint{TargetID: fromTarget, BoundaryID: out.place.ID},
					To:    atlas.Endpoint{TargetID: toTarget, BoundaryID: peer.in.place.ID},
					Value: strings.Join(out.place.Boundary.Values, ", "),
					Same:  true, Label: peer.label, Possible: true, Blind: true, SourceKind: "integration",
				})
			}
		}
	}
	r.joints = append(r.joints, r.linkJoints()...)
	sort.Slice(r.joints, func(i, j int) bool { return r.joints[i].ID < r.joints[j].ID })
	r.reportStage(def.Stage)
	return nil
}

type peerChoice struct {
	in    *boundaryState
	label string
}

// Reduce disjoint candidate windows until each outgoing call has one choice
// or none. Each round reuses original endpoint evidence, not previous labels.
// Value-confirmed joints (including several subscribers) remain separate.
func (r *reader) chooseBlindPeers(ctx context.Context, batches []peerBatch, targets map[string]TargetMeta, round *int) (map[string]peerChoice, error) {
	const peerWindow = 48
	chosen := make(map[string]peerChoice)
	for len(batches) > 0 {
		var next []peerBatch
		byPeers := make(map[string]int)
		for _, batch := range batches {
			candidates := make(map[string][]peerChoice)
			for start := 0; start < len(batch.ins); start += peerWindow {
				window := batch.ins[start:min(start+peerWindow, len(batch.ins))]
				// Eligibility can differ by just one self-reference. Split on
				// those differences inside this window, not the whole reservoir:
				// unrelated windows still share their outgoing rows.
				for _, eligible := range r.peerBatches(batch.outs, window, targets) {
					peers := eligible.ins
					var refs []string
					var sides []lines.BoundarySide
					for i, in := range peers {
						refs = append(refs, fmt.Sprintf("p%d", i+1))
						sides = append(sides, r.sideOf(in, targets))
					}
					var rows []table.Row
					for _, out := range eligible.outs {
						rows = append(rows, lines.PeerRow(out.place.ID, r.sideOf(out, targets), refs))
					}
					r.opts.Stage(lines.StageJoints, fmt.Sprintf("%d outgoing boundaries, %d candidate counterparts", len(rows), len(peers)))
					answers, err := r.runTableWith(ctx, lines.Peers(), *round, []table.Field{{Name: "question", Value: "peers"}, lines.PeerContext(refs, sides)}, rows, nil)
					*round++
					if err != nil {
						return nil, err
					}
					for i, out := range eligible.outs {
						answer := answers[i].answer
						for j, ref := range refs {
							if answer != nil && answer["peer"] == ref {
								candidates[out.place.ID] = append(candidates[out.place.ID], peerChoice{in: peers[j], label: strings.Trim(answer["label"], "- ")})
							}
						}
					}
				}
			}
			for _, out := range batch.outs {
				peers := candidates[out.place.ID]
				if len(peers) == 1 {
					chosen[out.place.ID] = peers[0]
				} else if len(peers) > 1 {
					var ids []string
					var ins []*boundaryState
					for _, peer := range peers {
						ids = append(ids, peer.in.place.ID)
						ins = append(ins, peer.in)
					}
					key := strings.Join(ids, "\x00")
					index, ok := byPeers[key]
					if !ok {
						index = len(next)
						byPeers[key] = index
						next = append(next, peerBatch{ins: ins})
					}
					next[index].outs = append(next[index].outs, out)
				}
			}
		}
		batches = next
	}
	return chosen, nil
}

type peerBatch struct {
	outs, ins []*boundaryState
}

func (r *reader) peerBatches(outs, ins []*boundaryState, targets map[string]TargetMeta) []peerBatch {
	var batches []peerBatch
	byPeers := make(map[string]int)
	for _, out := range outs {
		var eligible []*boundaryState
		var ids []string
		for _, in := range ins {
			if out.place.Boundary.ObjectID != "" && out.place.Boundary.ObjectID == in.place.Boundary.ObjectID {
				continue
			}
			compatible := false
			for _, from := range out.place.TargetIDs {
				for _, to := range in.place.TargetIDs {
					if from != to && r.compatible(targets[from], targets[to]) {
						compatible = true
					}
				}
			}
			if compatible {
				eligible = append(eligible, in)
				ids = append(ids, in.place.ID)
			}
		}
		if len(eligible) == 0 {
			continue
		}
		key := strings.Join(ids, "\x00")
		index, exists := byPeers[key]
		if !exists {
			index = len(batches)
			byPeers[key] = index
			batches = append(batches, peerBatch{ins: eligible})
		}
		batches[index].outs = append(batches[index].outs, out)
	}
	return batches
}

// linkJoints are the joints the code sees without a value: a call or an
// import from a box of one target into a box of another. A workspace
// package of the same target is no boundary at all; one of another target
// is the seam between them, and it is exact.
func (r *reader) linkJoints() []atlas.Joint {
	type key struct{ from, fromBox, to, toBox, kind string }
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
				k := key{fromTarget, fromBox, toTarget, toBox, edge.Kind}
				weight[k] += edge.Count
				if seen[k] {
					continue
				}
				seen[k] = true
				joints = append(joints, atlas.Joint{
					ID:         fmt.Sprintf("joint:%s:%s:%s->%s:%s", edge.Kind, fromTarget, fromBox, toTarget, toBox),
					From:       atlas.Endpoint{TargetID: fromTarget, BoxID: fromBox},
					To:         atlas.Endpoint{TargetID: toTarget, BoxID: toBox},
					Value:      edge.Kind + " " + r.boxes[toBox].dir,
					SourceKind: edge.Kind, Witnesses: append([]atlas.Witness(nil), edge.Witnesses...),
					Same: true, Label: "uses " + r.boxes[toBox].title,
				})
			}
		}
	}
	// Presentation can choose a neighbourhood; the stored graph keeps every seam.
	sort.SliceStable(joints, func(i, j int) bool {
		a := key{joints[i].From.TargetID, joints[i].From.BoxID, joints[i].To.TargetID, joints[i].To.BoxID, joints[i].SourceKind}
		b := key{joints[j].From.TargetID, joints[j].From.BoxID, joints[j].To.TargetID, joints[j].To.BoxID, joints[j].SourceKind}
		if weight[a] != weight[b] {
			return weight[a] > weight[b]
		}
		return joints[i].ID < joints[j].ID
	})
	return joints
}

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
	signature := ""
	if objectID := state.place.Boundary.ObjectID; objectID != "" {
		if file := r.places[state.place.Parent]; file.File != nil {
			for _, decl := range file.File.Decls {
				if decl.ObjectID == objectID {
					signature = decl.Signature
					break
				}
			}
		}
	}
	return lines.BoundarySide{
		Target: name, Line: state.line, External: state.place.Boundary.External,
		Path: state.place.Path, Caller: state.place.Boundary.Caller, Source: state.place.Boundary.Source,
		Method: state.place.Boundary.Method, Values: state.place.Boundary.Values,
		Signature: signature, CallerDoc: state.place.Boundary.CallerDoc,
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
