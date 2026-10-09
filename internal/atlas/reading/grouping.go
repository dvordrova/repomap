package reading

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

// The grouping of a target's map (owner, 2026-10-08) is one loop over boxes,
// starting with the whole target:
//
//  1. the box's declarations are its elements: every unit of the target that
//     Jev did not decide is a helper (the helper question), test code aside;
//  2. a box of at most groupLeafSize elements is read as it is; Jev asks
//     whether any larger box is grouped enough;
//  3. DeepSeek proposes the smaller boxes of a box that is not;
//  4. Jev puts each of its elements in one of them;
//  5. each smaller box goes back to step 2.
//
// A box whose elements all went in one smaller box gains nothing from it and
// is read as it is. The boxes read as they are become the parts of the map,
// the divided ones its areas. Helpers go with their users afterwards, by
// code. Test code is one part of its own, kept off the canvas.

//go:embed prompts/group_propose.md
var groupProposePrompt string

const groupProposeTask = "repomap.atlas.group_propose.v1"

// groupLeafSize is the owner's bound (2026-10-08): a box of this many
// elements or fewer is not divided and not asked about.
const groupLeafSize = 4

// groupElement is one declaration the grouping places: a unit of the
// target (a type with its methods, a function, a variable or a module body).
type groupElement struct {
	id, file, path, name, kind, signature string
	// unit carries the facts of a unit of a file that is neither test nor
	// generated code; nil for a generated file's unit.
	unit *roleUnit
}

// groupNode is one box of the grouping tree.
type groupNode struct {
	name, holds string
	parent      *groupNode
	elements    []*groupElement
	children    []*groupNode
	// helpers are the helper units code placed in this part.
	helpers []string
}

// inside names the boxes a node sits in, outermost first, the target's own
// box left out.
func (node *groupNode) inside() []string {
	var names []string
	for at := node.parent; at != nil && at.parent != nil; at = at.parent {
		names = append([]string{at.name}, names...)
	}
	return names
}

// declarations are the node's elements as "path: name", in element order.
func (node *groupNode) declarations() []string {
	names := make([]string, len(node.elements))
	for i, element := range node.elements {
		names[i] = element.path + ": " + element.name
	}
	return names
}

// groupTarget builds the grouping tree of one target. It never fails the
// target for a refused answer: a box whose question, proposal or assignment
// was refused is read as it is, and tables.md and the rejected rows say why.
func (r *reader) groupTarget(ctx context.Context, view *designView, round int, facts *roleFacts, helpers map[string]bool) (*groupNode, []string, error) {
	target := r.opts.Targets[(round-1)%len(r.opts.Targets)]
	root := &groupNode{name: target.Name}
	for _, file := range view.files {
		if file.test {
			continue
		}
		for _, id := range file.units {
			if helpers[id] {
				continue
			}
			decl := r.places[id].Symbol.Decl
			element := &groupElement{id: id, file: file.id, path: file.path, name: decl.Name, kind: decl.Kind, signature: decl.Signature, unit: facts.units[id]}
			root.elements = append(root.elements, element)
		}
	}
	pending := []*groupNode{root}
	for level := 1; len(pending) > 0; level++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		var ask []*groupNode
		for _, node := range pending {
			if len(node.elements) > groupLeafSize {
				ask = append(ask, node)
			}
		}
		// Each level is its own round of windows after every target's
		// earlier levels, so no window overwrites another's journal.
		levelRound := round + len(r.opts.Targets)*(level-1)
		divide, err := r.askEnough(ctx, levelRound, level, ask)
		if err != nil {
			return nil, nil, err
		}
		proposals, err := r.proposeGroups(ctx, levelRound, level, divide)
		if err != nil {
			return nil, nil, err
		}
		var assigning []*groupNode
		var catalogues [][]roleBox
		for i, node := range divide {
			if len(proposals[i]) >= 2 {
				assigning = append(assigning, node)
				catalogues = append(catalogues, proposals[i])
			}
		}
		children, err := r.assignGroups(ctx, levelRound, level, facts, assigning, catalogues)
		if err != nil {
			return nil, nil, err
		}
		pending = children
	}
	blocked := r.placeHelpers(view, root, facts, helpers)
	return root, blocked, nil
}

// askEnough asks whether each node is grouped enough and returns the nodes
// that need smaller boxes. A node whose question does not fit one question
// of the classifier is too large to read as one box and needs them; a node
// the answer leaves undecided is read as it is.
func (r *reader) askEnough(ctx context.Context, round, level int, nodes []*groupNode) ([]*groupNode, error) {
	if len(nodes) == 0 {
		return nil, nil
	}
	def := lines.GroupEnoughTable()
	target := r.opts.Targets[(round-1)%len(r.opts.Targets)]
	r.opts.Stage(lines.StageGroupEnough, fmt.Sprintf("%s: asking whether %d boxes of level %d need smaller boxes", target.Name, len(nodes), level))
	var divide, asked []*groupNode
	var groups rowGroups
	for i, node := range nodes {
		fields := []table.Field{{Name: "name", Value: node.name}}
		if node.holds != "" {
			fields = append(fields, table.Field{Name: "holds", Value: node.holds})
		}
		if inside := node.inside(); len(inside) > 0 {
			fields = append(fields, table.Field{Name: "inside", Value: inside})
		}
		fields = append(fields, table.Field{Name: "declarations", Value: node.declarations()})
		row := table.Row{ID: fmt.Sprintf("b%d", i+1), Fields: fields}
		windows, err := table.WindowsWithContext(def, round, nil, []table.Row{row})
		if err != nil {
			return nil, err
		}
		large, err := table.ClassifierNeedsPartition(r.opts.Categorizer, def, windows[0])
		if err != nil {
			return nil, err
		}
		if large {
			fmt.Fprintf(&r.tables, "%s: %d declarations do not fit one grouping question; divided\n", node.name, len(node.elements))
			divide = append(divide, node)
			continue
		}
		groups = append(groups, rowGroup{rows: []table.Row{row}})
		asked = append(asked, node)
	}
	answers, err := r.runTableGroups(ctx, def, round, groups, nil)
	if err != nil {
		return nil, err
	}
	for i, node := range asked {
		switch answers[i].answer["grouping"] {
		case lines.GroupNeedsSmaller:
			divide = append(divide, node)
		case lines.GroupEnough:
			fmt.Fprintf(&r.tables, "%s: grouped enough (%d declarations)\n", node.name, len(node.elements))
		default:
			fmt.Fprintf(&r.tables, "%s: grouping undecided; read as it is (%d declarations)\n", node.name, len(node.elements))
		}
	}
	r.tables.WriteString("\n")
	return divide, nil
}

// groupProposeInput is the proposal request of one box.
type groupProposeInput struct {
	Task   string           `json:"task"`
	Box    groupProposeBox  `json:"box"`
	Files  []groupFileNames `json:"files,omitempty"`
	Counts []groupDirCounts `json:"directories,omitempty"`
}

type groupProposeBox struct {
	Name   string   `json:"name"`
	Holds  string   `json:"holds,omitempty"`
	Inside []string `json:"inside,omitempty"`
}

type groupFileNames struct {
	Path  string   `json:"path"`
	Names []string `json:"names"`
}

type groupDirCounts struct {
	Dir   string           `json:"dir"`
	Files []groupFileCount `json:"files"`
}

type groupFileCount struct {
	Name         string `json:"name"`
	Declarations int    `json:"declarations"`
}

// proposeInput is a node's proposal request with every declaration name by
// file, or, when names is false, every file with its declaration count by
// directory: the representation of a box too large to list its names.
func proposeInput(node *groupNode, names bool) groupProposeInput {
	input := groupProposeInput{Task: groupProposeTask, Box: groupProposeBox{Name: node.name, Holds: node.holds, Inside: node.inside()}}
	byPath := map[string][]string{}
	var paths []string
	for _, element := range node.elements {
		if _, seen := byPath[element.path]; !seen {
			paths = append(paths, element.path)
		}
		byPath[element.path] = append(byPath[element.path], element.name)
	}
	sort.Strings(paths)
	if names {
		for _, file := range paths {
			input.Files = append(input.Files, groupFileNames{Path: file, Names: byPath[file]})
		}
		return input
	}
	dirs := map[string]*groupDirCounts{}
	var order []string
	for _, file := range paths {
		dir := path.Dir(file)
		if dirs[dir] == nil {
			dirs[dir] = &groupDirCounts{Dir: dir}
			order = append(order, dir)
		}
		dirs[dir].Files = append(dirs[dir].Files, groupFileCount{Name: path.Base(file), Declarations: len(byPath[file])})
	}
	for _, dir := range order {
		input.Counts = append(input.Counts, *dirs[dir])
	}
	return input
}

func groupProposeCall(input groupProposeInput) (llm.Call[boxesAnswer], error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return llm.Call[boxesAnswer]{}, err
	}
	return llm.Call[boxesAnswer]{
		State: []byte(groupProposeTask),
		Prompt: llm.Prompt{System: lines.RoleMap + groupProposePrompt, User: string(raw), ResponseFormatJSON: true, NoResponseAdjunct: true,
			ResponseExample: `{"boxes":[{"name":"Move search","holds":"The search over candidate moves and its scoring."},{"name":"Board state","holds":"The board, its pieces and applying a move to it."}]}`},
		Limits:         llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: designOutputFloor},
		DecodeValidate: decodeBoxes,
	}, nil
}

// fits says whether the provider takes a call's request whole.
func (r *reader) fits(call llm.Call[boxesAnswer]) (bool, error) {
	prepared, err := llm.Prepare(r.opts.Provider, call.Prompt, call.Limits)
	if err == nil && prepared.Len() > call.Limits.MaxRequestBytes {
		return false, nil
	}
	if inputRefused(err) {
		return false, nil
	}
	return err == nil, err
}

// proposeGroups asks DeepSeek for the smaller boxes of each node, all nodes
// of a level at once. A node's proposals are nil when the answer was refused
// or incomplete; the node is then read as it is.
func (r *reader) proposeGroups(ctx context.Context, round, level int, nodes []*groupNode) ([][]roleBox, error) {
	proposals := make([][]roleBox, len(nodes))
	if len(nodes) == 0 {
		return proposals, nil
	}
	target := r.opts.Targets[(round-1)%len(r.opts.Targets)]
	r.opts.Stage(lines.StageZones, fmt.Sprintf("%s: proposing the smaller boxes of %d boxes of level %d", target.Name, len(nodes), level))
	var calls []llm.Call[boxesAnswer]
	var asked []int
	for i, node := range nodes {
		call, err := groupProposeCall(proposeInput(node, true))
		if err != nil {
			return nil, err
		}
		ok, err := r.fits(call)
		if err != nil {
			return nil, err
		}
		if !ok {
			fmt.Fprintf(&r.tables, "%s: %d declaration names do not fit one proposal; files with their counts by directory sent instead\n", node.name, len(node.elements))
			if call, err = groupProposeCall(proposeInput(node, false)); err != nil {
				return nil, err
			}
			if ok, err = r.fits(call); err != nil {
				return nil, err
			}
			if !ok {
				r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageZones, Target: target.ID, Kind: "over_envelope", Count: len(node.elements), Samples: []string{node.name}, Reason: "the box's files do not fit one proposal; read as it is"})
				fmt.Fprintf(&r.tables, "%s: its files do not fit one proposal; read as it is\n", node.name)
				continue
			}
		}
		calls = append(calls, call)
		asked = append(asked, i)
	}
	results := llm.ExecuteJSONEach(ctx, debugdump.BindStage(r.opts.Executor, lines.StageZones), r.opts.Provider, calls)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	use := r.use(lines.StageZones)
	for j, i := range asked {
		node, result := nodes[i], results[j]
		window := table.Window{Stage: lines.StageZones, Round: round, Index: j}
		use.Windows++
		use.Rows += len(node.elements)
		if result.Outcome.Cached {
			use.Cached++
		} else {
			use.Live++
		}
		answer := result.Outcome.Value
		refused := result.Err != nil || len(answer.Incomplete) > 0
		ref := ""
		if len(result.Outcome.Response) > 0 {
			ref = path.Join(atlas.TablesDir, r.windowFileName(window, "response.ref.json"))
		}
		fmt.Fprintf(&r.tables, "## %s · round %d · window %d · %s (%d declarations)\n\n", lines.StageZones, round, window.Index, node.name, len(node.elements))
		if err := r.writeWindowExchange(window, []byte(calls[j].Prompt.System), []byte(calls[j].Prompt.User), result.Outcome.Request, result.Outcome.Response, refused); err != nil {
			return nil, err
		}
		if refused {
			use.Rejected++
			reason := "boxes without a name or holds"
			if result.Err != nil {
				reason = result.Err.Error()
			}
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageZones, Target: target.ID, Kind: "window_rejected", Count: len(node.elements), Samples: []string{node.name}, Reason: reason, ResponseRef: ref})
			fmt.Fprintf(&r.tables, "proposal refused; read as it is: %s\n\n", reason)
			continue
		}
		if len(answer.Repeated) > 0 {
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageZones, Target: target.ID, Kind: "group_repeated_name", Count: len(answer.Repeated), Samples: answer.Repeated, Reason: "names given to two boxes, each kept", ResponseRef: ref})
		}
		for _, box := range answer.Boxes {
			fmt.Fprintf(&r.tables, "- %s: %s\n", box.Name, box.Holds)
		}
		if len(answer.Boxes) < 2 {
			r.tables.WriteString("fewer than two smaller boxes; read as it is\n")
		}
		r.tables.WriteString("\n")
		proposals[i] = answer.Boxes
	}
	return proposals, nil
}

// assignItem is an element as the assignment asks about it: the helper
// question's item for a unit with facts, its name, kind, file and signature
// otherwise.
func (element *groupElement) assignItem(facts *roleFacts) []table.Field {
	if element.unit != nil {
		return element.unit.helperItem(facts)
	}
	item := []table.Field{{Name: "name", Value: element.name}, {Name: "kind", Value: element.kind}, {Name: "file", Value: element.path}}
	if element.signature != "" {
		item = append(item, table.Field{Name: "signature", Value: element.signature})
	}
	return item
}

// assignGroups puts each element of each node in one of its proposed boxes
// and returns the smaller boxes of every node that divides. An element the
// answer leaves undecided goes, by code, where most of the elements it
// calls or that call it went; one with no such neighbour is a box of its own,
// named by its declaration. A node whose elements fill fewer than two boxes
// is read as it is.
func (r *reader) assignGroups(ctx context.Context, round, level int, facts *roleFacts, nodes []*groupNode, catalogues [][]roleBox) ([]*groupNode, error) {
	if len(nodes) == 0 {
		return nil, nil
	}
	def := lines.GroupAssignTable()
	target := r.opts.Targets[(round-1)%len(r.opts.Targets)]
	total := 0
	var groups rowGroups
	for i, node := range nodes {
		catalogue := make([]map[string]any, len(catalogues[i]))
		for j, box := range catalogues[i] {
			catalogue[j] = map[string]any{"ref": fmt.Sprintf("b%d", j+1), "title": box.Name, "holds": box.Holds}
		}
		inside := append(node.inside(), node.name)
		if node.parent == nil {
			inside = []string{node.name}
		}
		group := rowGroup{shared: []table.Field{{Name: "inside", Value: inside}, {Name: "boxes", Value: catalogue}}}
		for j, element := range node.elements {
			group.rows = append(group.rows, table.Row{ID: fmt.Sprintf("d%d", j+1), Fields: element.assignItem(facts)})
		}
		total += len(node.elements)
		groups = append(groups, group)
	}
	r.opts.Stage(lines.StageGroupAssign, fmt.Sprintf("%s: putting %d declarations of %d boxes of level %d in their smaller boxes", target.Name, total, len(nodes), level))
	answers, err := r.runTableGroups(ctx, def, round, groups, nil)
	if err != nil {
		return nil, err
	}
	var children []*groupNode
	at := 0
	for i, node := range nodes {
		boxes := catalogues[i]
		choice := make(map[string]int, len(node.elements))
		var open []*groupElement
		for _, element := range node.elements {
			box := boxChoice(answers[at].answer, boxes)
			at++
			if box < 0 {
				open = append(open, element)
				continue
			}
			choice[element.id] = box
		}
		// An open element goes where most of its decided neighbours went.
		var alone []*groupElement
		for _, element := range open {
			count := make([]int, len(boxes))
			if unit := element.unit; unit != nil {
				for _, ids := range []map[string]bool{unit.callees, unit.calledByAll, unit.readers, unit.handers} {
					for id := range ids {
						if box, ok := choice[id]; ok {
							count[box]++
						}
					}
				}
			}
			best := -1
			for box, n := range count {
				if n > 0 && (best < 0 || n > count[best]) {
					best = box
				}
			}
			if best < 0 {
				alone = append(alone, element)
				continue
			}
			choice[element.id] = best
		}
		parts := make([]*groupNode, len(boxes))
		for _, element := range node.elements {
			box, ok := choice[element.id]
			if !ok {
				continue
			}
			if parts[box] == nil {
				parts[box] = &groupNode{name: boxes[box].Name, holds: boxes[box].Holds, parent: node}
			}
			parts[box].elements = append(parts[box].elements, element)
		}
		var kept []*groupNode
		var empty []string
		for box, part := range parts {
			if part == nil {
				empty = append(empty, boxes[box].Name)
				continue
			}
			kept = append(kept, part)
		}
		for _, element := range alone {
			kept = append(kept, &groupNode{name: element.name, parent: node, elements: []*groupElement{element}})
		}
		fmt.Fprintf(&r.tables, "%s: %d declarations in %d smaller boxes", node.name, len(node.elements), len(kept))
		if len(open) > 0 {
			fmt.Fprintf(&r.tables, "; %d undecided, %d of them placed with their neighbours", len(open), len(open)-len(alone))
		}
		if len(empty) > 0 {
			fmt.Fprintf(&r.tables, "; empty: %s", strings.Join(empty, ", "))
		}
		r.tables.WriteString("\n")
		if len(alone) > 0 {
			names := make([]string, len(alone))
			for j, element := range alone {
				names[j] = element.path + ":" + element.name
			}
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageGroupAssign, Target: target.ID, Kind: "group_undecided", Count: len(names), Samples: names,
				Reason: node.name + ": undecided with no decided neighbour; a box of its own"})
		}
		if len(kept) < 2 {
			fmt.Fprintf(&r.tables, "%s: every declaration went in one smaller box; read as it is\n", node.name)
			continue
		}
		node.children = kept
		children = append(children, kept...)
	}
	r.tables.WriteString("\n")
	return children, nil
}

// leaves are the tree's parts in depth-first order.
func (node *groupNode) leaves() []*groupNode {
	if len(node.children) == 0 {
		return []*groupNode{node}
	}
	var leaves []*groupNode
	for _, child := range node.children {
		leaves = append(leaves, child.leaves()...)
	}
	return leaves
}

// placeHelpers puts every helper in the part that holds most of the units
// that use it, by code: exact calls, decorations, reads of what does not run
// and a type taken as a parameter; a hand-over is no use.
// A helper only other helpers use follows them once they are placed; one
// whose users are never placed goes in the part holding most of its file's
// units, and with none it stays off the map as blocked.
func (r *reader) placeHelpers(view *designView, root *groupNode, facts *roleFacts, helpers map[string]bool) []string {
	leafOf := map[string]*groupNode{}
	leaves := root.leaves()
	for _, leaf := range leaves {
		for _, element := range leaf.elements {
			leafOf[element.id] = leaf
		}
	}
	index := map[*groupNode]int{}
	for i, leaf := range leaves {
		index[leaf] = i
	}
	var waiting, blocked []string
	for _, file := range view.files {
		for _, id := range file.units {
			if helpers[id] {
				waiting = append(waiting, id)
			}
		}
	}
	best := func(ids []string) *groupNode {
		count := map[*groupNode]int{}
		for _, id := range ids {
			if leaf := leafOf[id]; leaf != nil {
				count[leaf]++
			}
		}
		var top *groupNode
		for leaf, n := range count {
			if top == nil || n > count[top] || n == count[top] && index[leaf] < index[top] {
				top = leaf
			}
		}
		return top
	}
	for len(waiting) > 0 {
		var still []string
		for _, id := range waiting {
			unit := facts.units[id]
			// Users only: a hand-over, or a call through the function value
			// one stored, is no use (roleUnit.users).
			var users []string
			if unit != nil {
				for user := range unit.users {
					users = append(users, user)
				}
			}
			if leaf := best(users); leaf != nil {
				leafOf[id] = leaf
				leaf.helpers = append(leaf.helpers, id)
				continue
			}
			still = append(still, id)
		}
		if len(still) == len(waiting) {
			// No helper found a placed user this round: each goes with its
			// file's units.
			for _, id := range still {
				var mates []string
				if file := view.byID[view.unitFile[id]]; file != nil {
					mates = file.units
				}
				if leaf := best(mates); leaf != nil {
					leafOf[id] = leaf
					leaf.helpers = append(leaf.helpers, id)
					continue
				}
				blocked = append(blocked, id)
			}
			break
		}
		waiting = still
	}
	for _, leaf := range leaves {
		slices.SortFunc(leaf.helpers, compactIDLess3)
	}
	return blocked
}

func compactIDLess3(a, b string) int {
	switch {
	case compactIDLess(a, b):
		return -1
	case compactIDLess(b, a):
		return 1
	}
	return 0
}

// targetTree is a target's grouping: its tree, its parts in the order of
// the drafts they became, and the units Jev decided are helpers.
type targetTree struct {
	root    *groupNode
	leaves  []*groupNode
	helpers map[string]bool
}

// zoneDraft is one area of a target before it takes its compact ID: a box
// of the tree that was divided, the target's own box aside. Parent is the
// index of the area it sits in, or -1; Parts are the IDs of the parts that
// sit in it directly.
type zoneDraft struct {
	title, line string
	parent      int
	parts       []string
}

// zones are the areas of the tree once its parts have their IDs: drafts[i]
// is leaves[i]'s part.
func (tree *targetTree) zones(drafts []designPart) []zoneDraft {
	if tree == nil || tree.root == nil {
		return nil
	}
	partOf := map[*groupNode]string{}
	for i, leaf := range tree.leaves {
		partOf[leaf] = drafts[i].id
	}
	var zones []zoneDraft
	var walk func(node *groupNode, parent int)
	walk = func(node *groupNode, parent int) {
		at := parent
		if node.parent != nil {
			zones = append(zones, zoneDraft{title: node.name, line: node.holds, parent: parent})
			at = len(zones) - 1
		}
		for _, child := range node.children {
			if len(child.children) == 0 {
				if at >= 0 {
					zones[at].parts = append(zones[at].parts, partOf[child])
				}
				continue
			}
			walk(child, at)
		}
	}
	walk(tree.root, -1)
	return zones
}

// groupedUnits reads a target's units into parts: the helper question, the
// grouping tree over the units that are no helpers, then one row per part of
// the tree, and one more for the units of the target's test files. drafts[i]
// is units[i]'s part.
func (r *reader) groupedUnits(ctx context.Context, view *designView, round int, target TargetMeta, outcome *designOutcome) ([]*designUnit, []designPart, *targetTree, error) {
	tree := &targetTree{helpers: map[string]bool{}}
	count := 0
	for _, file := range view.files {
		count += len(file.units)
	}
	if count == 0 {
		// A target without code has a legitimate empty map.
		return nil, nil, tree, nil
	}
	if r.dry {
		outcome.failure = atlas.MapFailureNoModel
		return nil, nil, tree, nil
	}
	facts := r.unitFacts(view)
	if len(facts.files) > 0 {
		helpers, err := r.askHelpers(ctx, round, facts)
		if err != nil {
			return nil, nil, nil, err
		}
		tree.helpers = helpers
	}
	root, blocked, err := r.groupTarget(ctx, view, round, facts, tree.helpers)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, id := range blocked {
		outcome.unitReason[id] = atlas.OffMapBlocked
	}
	var units []*designUnit
	var drafts []designPart
	if len(root.elements) > 0 {
		tree.root = root
		tree.leaves = root.leaves()
		for i, leaf := range tree.leaves {
			ids := make([]string, 0, len(leaf.elements)+len(leaf.helpers))
			for _, element := range leaf.elements {
				ids = append(ids, element.id)
			}
			ids = append(ids, leaf.helpers...)
			first := view.byID[leaf.elements[0].file]
			unit := view.designUnitOf(fmt.Sprintf("g%d", i+1), first, "", ids)
			units = append(units, unit)
			drafts = append(drafts, designPart{name: leaf.name, holds: leaf.holds, refs: []string{unit.ref}})
		}
	}
	// The units of the target's test files are one part, kept off the
	// canvas as test code.
	var tests []string
	var testFile *designFile
	for _, file := range view.files {
		if file.test {
			if testFile == nil {
				testFile = file
			}
			tests = append(tests, file.units...)
		}
	}
	if len(tests) > 0 {
		unit := view.designUnitOf("tests", testFile, "", tests)
		units = append(units, unit)
		drafts = append(drafts, designPart{name: "Tests", refs: []string{unit.ref}})
	}
	return units, drafts, tree, nil
}
