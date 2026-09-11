package contracttest

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

// assertEvidenceVocabulary renders every declaration of a graph as the
// symbols and operations tables do and checks that each enumerated value the
// rows carry (invocation, resolution, kind, extractor, a control_context
// label, the words of a local_calls line) is named, in backticks, by the
// prompt the row goes with. No internal enumeration reaches the model
// undefined; a value that is not rendered needs no definition. The expected
// values must have been rendered, so the check cannot pass on an empty walk.
func assertEvidenceVocabulary(t *testing.T, graph atlas.Graph, expected ...string) {
	t.Helper()
	declarations := make(map[string]atlas.Place)
	for _, place := range graph.Places {
		if place.Symbol == nil {
			continue
		}
		if id := place.Symbol.Decl.ObjectID; id != "" {
			declarations[id] = place
		}
		declarations[place.ID] = place
	}
	prompts := map[string]string{"symbols": lines.SymbolSelection(false).System, "operations": lines.Operations().System}
	for name, prompt := range prompts {
		if !strings.Contains(prompt, "`synchronous`") || !strings.Contains(prompt, "`control_context`") {
			t.Fatalf("%s prompt carries no evidence vocabulary", name)
		}
	}
	undefined := make(map[string]string)
	seen := make(map[string]bool)
	rendered := 0
	for _, place := range graph.Places {
		if place.Symbol == nil || place.Symbol.Decl.Kind == "type" {
			continue
		}
		rows := map[string]table.Row{"symbols": lines.SymbolRow(place, "")}
		rows["operations"], _ = reading.OperationRow(place, declarations, nil)
		for name, row := range rows {
			fields := make(map[string]any, len(row.Fields))
			for _, field := range row.Fields {
				fields[field.Name] = field.Value
			}
			raw, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			var decoded any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			renderedEnumerations(decoded, func(field, value string) {
				rendered++
				seen[value] = true
				if !definedInPrompt(prompts[name], value) {
					undefined[name+" "+field+" "+value] = place.Path + ":" + place.Symbol.Decl.Name
				}
			})
		}
	}
	if rendered == 0 {
		t.Fatal("no enumerated value was rendered from the fixture graph")
	}
	for _, value := range expected {
		if !seen[value] {
			t.Fatalf("the fixture rows no longer render %q; the vocabulary check lost that case", value)
		}
	}
	if len(undefined) > 0 {
		keys := make([]string, 0, len(undefined))
		for key := range undefined {
			keys = append(keys, key+" ("+undefined[key]+")")
		}
		sort.Strings(keys)
		t.Fatalf("values rendered without a definition in their prompt:\n%s", strings.Join(keys, "\n"))
	}
}

// renderedEnumerations walks a rendered row and reports every enumerated
// value: invocation, resolution, kind and extractor fields wherever they
// occur, the label of a control_context observation, binding table cells
// under an invocation or resolution column, and the invocation and control
// statements of a local_calls line.
func renderedEnumerations(value any, report func(field, value string)) {
	switch node := value.(type) {
	case map[string]any:
		extractor, _ := node["extractor"].(string)
		columns, _ := node["columns"].([]any)
		for key, item := range node {
			switch key {
			case "invocation", "resolution", "kind", "extractor":
				if text, ok := item.(string); ok && text != "" {
					report(key, text)
				}
			case "label":
				if text, ok := item.(string); ok && extractor == "control_context" {
					report(key, text)
				}
			case "rows":
				for _, row := range asList(item) {
					for i, cell := range asList(row) {
						if i < len(columns) && (columns[i] == "invocation" || columns[i] == "resolution") {
							if text, ok := cell.(string); ok && text != "" {
								report(columns[i].(string), text)
							}
						}
					}
				}
			case "local_calls":
				for _, line := range asList(item) {
					if text, ok := line.(string); ok {
						localCallEnumerations(text, report)
					}
				}
			}
			renderedEnumerations(item, report)
		}
	case []any:
		for _, item := range node {
			renderedEnumerations(item, report)
		}
	}
}

// A local_calls line is name@line, an optional invocation word, and control
// statements in parentheses separated by semicolons.
func localCallEnumerations(line string, report func(field, value string)) {
	rest := line
	if at := strings.Index(rest, "@"); at >= 0 {
		rest = rest[at+1:]
		if space := strings.IndexByte(rest, ' '); space >= 0 {
			rest = rest[space+1:]
		} else {
			rest = ""
		}
	}
	if open := strings.Index(rest, "("); open >= 0 {
		for _, statement := range strings.Split(strings.TrimSuffix(rest[open+1:], ")"), "; ") {
			report("label", statement)
		}
		rest = strings.TrimSpace(rest[:open])
	}
	if rest != "" {
		report("invocation", rest)
	}
}

func asList(value any) []any {
	list, _ := value.([]any)
	return list
}

// definedInPrompt accepts a value named in backticks. A prefixed value such
// as interface_invoke:synchronous is defined by its prefix with the colon and
// by the word after it, each named on its own.
func definedInPrompt(prompt, value string) bool {
	if strings.Contains(prompt, "`"+value+"`") {
		return true
	}
	if colon := strings.IndexByte(value, ':'); colon > 0 {
		return strings.Contains(prompt, "`"+value[:colon+1]) && strings.Contains(prompt, "`"+value[colon+1:]+"`")
	}
	return false
}
