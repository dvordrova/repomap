package contracttest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// inputsPreset reads a cumulative fixture end to end, without captions as
// an ordinary run reads it, with each closed question answered as a reader
// of the question's own item would: decide gets the column, the item and
// the offered options and says the choice, or false for the preset's
// neutral answer (none, not named, one box, responsibility). Every item
// asked is kept by column. The text tables get one part per unit, no
// areas, a destination written as other:, no system, no joint and no peer.
type inputsPreset struct {
	decide func(column string, item map[string]any, options []string) (string, bool)
	// read, when set, is asked first with each option's criteria: a reader
	// deciding by what an option is not for.
	read  func(column string, item map[string]any, options []llm.Option) (string, bool)
	mu    sync.Mutex
	asked map[string][]map[string]any
}

// notFor is an option's "not for" criteria, or "".
func notFor(options []llm.Option, name string) string {
	for _, option := range options {
		if option.Name == name && option.Criteria != nil {
			return option.Criteria.NotFor
		}
	}
	return ""
}

func (*inputsPreset) State() []byte { return []byte(`{"provider":"inputs-preset"}`) }

func (*inputsPreset) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}

func (p *inputsPreset) record(column string, item map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.asked == nil {
		p.asked = map[string][]map[string]any{}
	}
	p.asked[column] = append(p.asked[column], item)
}

// items are the items asked under a column, as JSON text.
func (p *inputsPreset) items(column string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var result []string
	for _, item := range p.asked[column] {
		raw, _ := json.Marshal(item)
		result = append(result, string(raw))
	}
	return result
}

func (p *inputsPreset) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var request struct {
		Task    string           `json:"task"`
		Table   string           `json:"table"`
		Fill    []map[string]any `json:"fill"`
		Context map[string]any   `json:"context"`
		Rows    []map[string]any `json:"rows"`
		Units   []struct {
			Ref  string `json:"ref"`
			Path string `json:"path"`
			Box  string `json:"box"`
		} `json:"units"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return llm.Completion{}, err
	}
	var answer any
	switch request.Task {
	case "repomap.atlas.parts.v2":
		var groups []map[string]any
		for _, unit := range request.Units {
			name := unit.Path
			if unit.Box != "" {
				name = unit.Path + ": " + unit.Box
			}
			groups = append(groups, map[string]any{"name": name, "units": []string{unit.Ref}})
		}
		answer = map[string]any{"groups": groups}
	case "repomap.atlas.describe.v1":
		answer = map[string]any{"description": "Preset description."}
	case "repomap.atlas.areas.v1":
		answer = map[string]any{"areas": []any{}}
	default:
		if request.Table == "" {
			return llm.Completion{}, fmt.Errorf("inputs preset: no answer for task %q", request.Task)
		}
		rows := make([]map[string]any, 0, len(request.Rows))
		for _, row := range request.Rows {
			cells := map[string]any{"key": row["key"]}
			for _, column := range request.Fill {
				name, _ := column["name"].(string)
				if !conditionHolds(column, cells, row) {
					continue
				}
				p.record(request.Table+"."+name, row)
				switch {
				case name == "name":
					// An entry is named by the first word its call wrote,
					// past a description, help or usage text when the
					// column says never one and the word says what it is
					// given as.
					words, _ := row["words"].([]any)
					note, _ := column["note"].(string)
					for _, word := range words {
						offered, _ := word.(map[string]any)
						if given, _ := offered["given"].(string); strings.Contains(note, "never a description, help or usage text") && (given == "description" || given == "help" || given == "usage") {
							continue
						}
						cells[name] = []any{offered["ref"]}
						break
					}
				case name == "destination":
					cells[name] = "other: preset system"
				case name == "address":
					cells[name] = "unknown"
				case name == "system":
					cells[name] = "none"
				case name == "same":
					cells[name] = "no"
				case name == "peer":
					cells[name] = "none"
				case column["kind"] == "text" || column["kind"] == "prose":
					cells[name] = "Preset text."
				case column["kind"] == "sequence":
					cells[name] = "none"
				case column["kind"] == "choice":
					options, _ := column["options"].([]any)
					if len(options) > 0 {
						cells[name] = options[0]
					}
				default:
					return llm.Completion{}, fmt.Errorf("inputs preset: no answer for %s.%s", request.Table, name)
				}
			}
			rows = append(rows, cells)
		}
		answer = map[string]any{"rows": rows}
	}
	response, err := json.Marshal(answer)
	if err != nil {
		return llm.Completion{}, err
	}
	return llm.Completion{Response: response, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, nil
}

func (p *inputsPreset) categorizer() *typesafetest.Categorizer {
	neutral := map[string]string{
		"role": "domain", "key_symbol": "yes", "boxes": "one box", "helper": "responsibility",
		"binds": "none", "talks": "none", "argument": "none", "enters": "none", "becomes": "none", "starts": "none",
	}
	return &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		column := key[strings.LastIndex(key, "|")+1:]
		p.record(column, question.Item)
		var options []string
		for _, option := range question.Options {
			options = append(options, option.Name)
		}
		if p.read != nil {
			if choice, ok := p.read(column, question.Item, question.Options); ok {
				return typesafetest.Choose(choice), true
			}
		}
		if choice, ok := p.decide(column, question.Item, options); ok {
			return typesafetest.Choose(choice), true
		}
		switch column {
		case "explains":
			return typesafetest.Yes(0.9), true
		case "program":
			return typesafetest.Choose(lines.ProgramNotNamed), true
		}
		if choice, ok := neutral[column]; ok {
			return typesafetest.Choose(choice), true
		}
		return llm.Verdict{}, false
	}}
}

// readInputs reads one fixture target with the preset and projects its
// GroupsIndex; a question the preset leaves unanswered fails the test.
func readInputs(t *testing.T, graph atlas.Graph, program programindex.Index, meta reading.TargetMeta, root string, preset *inputsPreset) groupindex.Index {
	t.Helper()
	result, err := reading.Read(t.Context(), reading.Options{
		Graph: graph, Repository: "fixture", Revision: "test", NoCaptions: true, Targets: []reading.TargetMeta{meta},
		Executor: llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}},
		Provider: preset, Categorizer: preset.categorizer(), OwnerRunDir: t.TempDir(),
		ReadSource: func(path string) ([]byte, error) { return os.ReadFile(filepath.Join(root, filepath.FromSlash(path))) },
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range result.Rejected {
		if row.Kind == "window_rejected" {
			t.Fatalf("a question was left unanswered: %+v", row)
		}
	}
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{program.Target.ID: program}, result.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	return indexes[0]
}

// inputRow is one input as a test compares it: its kind and name, what
// declares it and the object it is declared on, what its handler is, and
// where its call is.
type inputRow struct {
	kind, name, declaredBy, on, handler, at string
}

func inputRows(index groupindex.Index, paths ...string) []inputRow {
	names := map[string]string{}
	for _, subject := range index.Subjects {
		if subject.Object != nil {
			names[subject.ID] = subject.Object.Name
		}
	}
	var rows []inputRow
	for _, operation := range index.Operations {
		if len(paths) > 0 && !slices.Contains(paths, operation.Location.Path) {
			continue
		}
		row := inputRow{kind: operation.Kind, name: operation.Name, declaredBy: names[operation.DeclaredBy], handler: names[operation.SubjectID],
			at: fmt.Sprintf("%s:%d", operation.Location.Path, operation.Location.Line)}
		if operation.DeclaredOn != nil {
			row.on = operation.DeclaredOn.Text
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, func(a, b inputRow) int { return strings.Compare(a.at+" "+a.name, b.at+" "+b.name) })
	return rows
}

// catalogueRows are the catalogues of an index as "kind on|by: members".
func catalogueRows(index groupindex.Index) []string {
	names := map[string]string{}
	for _, subject := range index.Subjects {
		if subject.Object != nil {
			names[subject.ID] = subject.Object.Name
		}
	}
	operations := map[string]string{}
	for _, operation := range index.Operations {
		operations[operation.ID] = operation.Name
	}
	var rows []string
	for _, catalogue := range index.Catalogues {
		key := "by " + names[catalogue.DeclaredBy]
		if catalogue.On != nil {
			key = "on " + catalogue.On.Text
			if catalogue.OnOperationID != "" {
				key += " (" + operations[catalogue.OnOperationID] + ")"
			}
		}
		var members []string
		for _, id := range catalogue.OperationIDs {
			members = append(members, operations[id])
		}
		rows = append(rows, catalogue.Kind+" "+key+": "+strings.Join(members, " "))
	}
	slices.Sort(rows)
	return rows
}

// writtenRows are the registrations of the inputs declared in a file as the
// code wrote them, by name: the reading shows them (a table's row with its
// arity and flags).
func writtenRows(index groupindex.Index, file string) map[string]string {
	written := map[string]string{}
	for _, operation := range index.Operations {
		if operation.Location.Path == file {
			written[operation.Name] = operation.Written
		}
	}
	return written
}
