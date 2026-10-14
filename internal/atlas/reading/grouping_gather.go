package reading

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

// The gathering of a divided box's smaller boxes (owner, 2026-10-14). The
// proposal names a box's responsibilities and nothing makes them fewer:
// Airflow's core had 25 boxes at its top level, nats-server's 22, a web of
// arrows a newcomer could not take in. A box divided into more than
// groupLeafSize smaller boxes, the owner's bound of what reads as it is, has
// DeepSeek say which of them sit together in a larger part, from their names,
// holds and declaration counts alone. Code makes each group of two or more a
// box between the box and its smaller boxes, holding their declarations; it
// is divided by construction, so nothing asks about it again, and the
// smaller boxes go on to be asked about as before. One pass, no recursion.

//go:embed prompts/group_gather.md
var groupGatherPrompt string

const groupGatherTask = "repomap.atlas.group_gather.v1"

type groupGatherInput struct {
	Task  string            `json:"task"`
	Box   groupProposeBox   `json:"box"`
	Boxes []groupGatherItem `json:"boxes"`
}

type groupGatherItem struct {
	Ref          string `json:"ref"`
	Name         string `json:"name"`
	Holds        string `json:"holds,omitempty"`
	Declarations int    `json:"declarations"`
}

// gatherGroup is one larger part of an answer: its name, holds and the
// indexes of its smaller boxes.
type gatherGroup struct {
	name, holds string
	boxes       []int
}

// gatherAnswer is a decoded gathering. Refused names the groups given
// without a name or holds and the refs put in two groups: each such smaller
// box stays where it is.
type gatherAnswer struct {
	Groups  []gatherGroup
	Refused []string
}

func gatherInput(node *groupNode) groupGatherInput {
	input := groupGatherInput{Task: groupGatherTask, Box: groupProposeBox{Name: node.name, Holds: node.holds, Inside: node.inside()}}
	for i, child := range node.children {
		input.Boxes = append(input.Boxes, groupGatherItem{Ref: fmt.Sprintf("b%d", i+1), Name: child.name, Holds: child.holds, Declarations: len(child.elements)})
	}
	return input
}

func groupGatherCall(input groupGatherInput) (llm.Call[gatherAnswer], error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return llm.Call[gatherAnswer]{}, err
	}
	count := len(input.Boxes)
	return llm.Call[gatherAnswer]{
		State: []byte(groupGatherTask),
		Prompt: llm.Prompt{System: groupGatherPrompt, User: string(raw), ResponseFormatJSON: true, NoResponseAdjunct: true,
			ResponseExample: `{"groups":[{"name":"Order handling","holds":"Taking an order from checkout through payment to shipping.","boxes":["b2","b5","b7"]}]}`},
		Limits:         llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: designOutputFloor},
		DecodeValidate: func(raw []byte) (gatherAnswer, error) { return decodeGather(raw, count) },
	}, nil
}

// decodeGather reads {"groups":[{"name","holds","boxes":["b1",...]}]} over
// `count` smaller boxes b1..bN. A wrapper object, keys in another case, a
// ref in another case or with spaces, a string for a list and a ref repeated
// in one group are forms; an unknown ref is discarded. A group without a
// name or holds is refused, and a ref in two groups is refused in both: the
// smaller boxes stay where they are. A group left with fewer than two boxes,
// or holding every box, gains nothing and is dropped. An answer that is not
// JSON or holds no list of groups is refused.
func decodeGather(raw []byte, count int) (gatherAnswer, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return gatherAnswer{}, fmt.Errorf("groups: the answer is not JSON")
	}
	list, ok := namedList(value, "groups", 2)
	if !ok {
		return gatherAnswer{}, fmt.Errorf("groups: the answer has no list of groups")
	}
	var answer gatherAnswer
	owner := map[int]int{} // box -> the group it was first put in, -1 once in two
	var groups []gatherGroup
	for position, element := range list {
		fields, _ := element.(map[string]any)
		name, holds := cleanText(textField(fields, "name")), cleanText(textField(fields, "holds"))
		var refs []any
		for key, field := range fields {
			if strings.EqualFold(strings.TrimSpace(key), "boxes") {
				switch typed := field.(type) {
				case []any:
					refs = typed
				case string:
					refs = []any{typed}
				}
			}
		}
		if name == "" || holds == "" {
			answer.Refused = append(answer.Refused, fmt.Sprintf("group %d", position+1))
			continue
		}
		group := gatherGroup{name: name, holds: holds}
		seen := map[int]bool{}
		for _, ref := range refs {
			text, _ := ref.(string)
			text = strings.ToLower(strings.TrimSpace(text))
			var box int
			if _, err := fmt.Sscanf(text, "b%d", &box); err != nil || fmt.Sprintf("b%d", box) != text || box < 1 || box > count || seen[box-1] {
				continue
			}
			seen[box-1] = true
			group.boxes = append(group.boxes, box-1)
			if first, placed := owner[box-1]; placed && first != len(groups) {
				owner[box-1] = -1
			} else if !placed {
				owner[box-1] = len(groups)
			}
		}
		groups = append(groups, group)
	}
	for _, group := range groups {
		var kept []int
		for _, box := range group.boxes {
			if owner[box] < 0 {
				continue
			}
			kept = append(kept, box)
		}
		if len(kept) < 2 || len(kept) == count {
			continue
		}
		group.boxes = kept
		answer.Groups = append(answer.Groups, group)
	}
	for box := 0; box < count; box++ {
		if owner[box] < 0 {
			answer.Refused = append(answer.Refused, fmt.Sprintf("b%d", box+1))
		}
	}
	return answer, nil
}

// namedList finds a list: the value itself, its `name` field in any case,
// or the same inside one wrapper object.
func namedList(value any, name string, depth int) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case map[string]any:
		for key, field := range typed {
			if strings.EqualFold(strings.TrimSpace(key), name) {
				if field == nil {
					return nil, true
				}
				list, ok := field.([]any)
				return list, ok
			}
		}
		if len(typed) == 1 && depth > 0 {
			for _, field := range typed {
				if wrapped, ok := field.(map[string]any); ok {
					return namedList(wrapped, name, depth-1)
				}
			}
		}
	}
	return nil, false
}

// gatherGroups gathers the smaller boxes of each node divided into more than
// groupLeafSize of them, all nodes of a level at once. A refused answer
// leaves the node as it is.
func (r *reader) gatherGroups(ctx context.Context, round, level int, nodes []*groupNode) error {
	var many []*groupNode
	for _, node := range nodes {
		if len(node.children) > groupLeafSize {
			many = append(many, node)
		}
	}
	if len(many) == 0 {
		return nil
	}
	target := r.opts.Targets[(round-1)%len(r.opts.Targets)]
	r.opts.Stage(lines.StageGroupGather, fmt.Sprintf("%s: gathering the smaller boxes of %d boxes of level %d into larger parts", target.Name, len(many), level))
	calls := make([]llm.Call[gatherAnswer], len(many))
	for i, node := range many {
		call, err := groupGatherCall(gatherInput(node))
		if err != nil {
			return err
		}
		calls[i] = call
	}
	results := llm.ExecuteJSONEach(ctx, debugdump.BindStage(r.opts.Executor, lines.StageGroupGather), r.opts.Provider, calls)
	if err := ctx.Err(); err != nil {
		return err
	}
	use := r.use(lines.StageGroupGather)
	for i, node := range many {
		result := results[i]
		window := table.Window{Stage: lines.StageGroupGather, Round: round, Index: i}
		use.Windows++
		use.Rows += len(node.children)
		if result.Outcome.Cached {
			use.Cached++
		} else {
			use.Live++
		}
		ref := ""
		if len(result.Outcome.Response) > 0 {
			ref = path.Join(atlas.TablesDir, r.windowFileName(window, "response.ref.json"))
		}
		fmt.Fprintf(&r.tables, "## %s · round %d · window %d · %s (%d smaller boxes)\n\n", lines.StageGroupGather, round, i, node.name, len(node.children))
		if err := r.writeWindowExchange(window, []byte(calls[i].Prompt.System), []byte(calls[i].Prompt.User), result.Outcome.Request, result.Outcome.Response, result.Err != nil); err != nil {
			return err
		}
		if result.Err != nil {
			use.Rejected++
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageGroupGather, Target: target.ID, Kind: "window_rejected", Count: len(node.children), Samples: []string{node.name}, Reason: result.Err.Error(), ResponseRef: ref})
			fmt.Fprintf(&r.tables, "gathering refused; its smaller boxes stay as they are: %s\n\n", result.Err)
			continue
		}
		answer := result.Outcome.Value
		if len(answer.Refused) > 0 {
			r.rejected = append(r.rejected, modeldiag.Row{Stage: lines.StageGroupGather, Target: target.ID, Kind: "group_refused", Count: len(answer.Refused), Samples: answer.Refused,
				Reason: node.name + ": a group without a name or holds, or a smaller box put in two groups; each stays where it is", ResponseRef: ref})
		}
		gatherInto(node, answer.Groups)
		for _, child := range node.children {
			if len(child.children) > 0 && child.gathered {
				names := make([]string, len(child.children))
				for j, box := range child.children {
					names[j] = box.name
				}
				fmt.Fprintf(&r.tables, "- %s: %s (%s)\n", child.name, child.holds, strings.Join(names, ", "))
			}
		}
		if len(answer.Groups) == 0 {
			r.tables.WriteString("no larger parts; its smaller boxes stay as they are\n")
		}
		r.tables.WriteString("\n")
	}
	return nil
}

// gatherInto makes each group a box between node and the smaller boxes it
// gathers, standing where its first smaller box stood; it holds their
// declarations in their order.
func gatherInto(node *groupNode, groups []gatherGroup) {
	if len(groups) == 0 {
		return
	}
	groupOf := map[int]int{}
	for g, group := range groups {
		for _, box := range group.boxes {
			groupOf[box] = g
		}
	}
	made := make([]*groupNode, len(groups))
	var children []*groupNode
	for i, child := range node.children {
		g, ok := groupOf[i]
		if !ok {
			children = append(children, child)
			continue
		}
		if made[g] == nil {
			made[g] = &groupNode{name: groups[g].name, holds: groups[g].holds, parent: node, gathered: true}
			children = append(children, made[g])
		}
		child.parent = made[g]
		made[g].children = append(made[g].children, child)
		made[g].elements = append(made[g].elements, child.elements...)
	}
	node.children = children
}
