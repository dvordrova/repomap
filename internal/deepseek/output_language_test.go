package deepseek

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalEnglishContractWrapsDefaultSemanticRequestSeams(t *testing.T) {
	t.Parallel()

	client := &Client{Model: "test", MaxTokens: 1000}
	orientationJSON, err := client.OrientPromptJSON(
		[]byte(`{"allowed_paths":["cmd/server/main.go"]}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	flowJSON, err := client.FlowExplainPromptJSON(
		`{"opaque_id":"direction-1","path":"cmd/server/main.go"}`,
		"Return valid JSON only.",
	)
	if err != nil {
		t.Fatal(err)
	}
	textJSON, err := client.flowExplainPromptText(
		"Explain the bounded result.",
		"Return plain text.",
	)
	if err != nil {
		t.Fatal(err)
	}

	for name, raw := range map[string][]byte{
		"orientation": orientationJSON,
		"json stage":  flowJSON,
		"text stage":  textJSON,
	} {
		var request chatRequest
		if err := json.Unmarshal(raw, &request); err != nil {
			t.Fatalf("%s request: %v", name, err)
		}
		if len(request.Messages) != 2 {
			t.Fatalf("%s messages = %#v", name, request.Messages)
		}
		for _, want := range []string{
			"CANONICAL OUTPUT LANGUAGE CONTRACT",
			SemanticOutputLanguageContractVersion,
			"human-readable prose value in English",
			"repository paths",
			"code identifiers",
			"closed lists of allowed literals",
		} {
			joined := request.Messages[0].Content + "\n" + request.Messages[1].Content
			if !strings.Contains(joined, want) {
				t.Fatalf("%s request is missing %q: %s", name, want, joined)
			}
		}
		if strings.Contains(
			request.Messages[0].Content+"\n"+request.Messages[1].Content,
			"prose value in Russian",
		) {
			t.Fatalf("%s request retained the superseded Russian semantic contract", name)
		}
	}

	if !strings.Contains(string(orientationJSON), `cmd/server/main.go`) ||
		!strings.Contains(string(flowJSON), `cmd/server/main.go`) {
		t.Fatal("canonical-English wrapping changed an opaque repository path")
	}
}

func TestChatRequestConstructionHasOneSemanticChokePointAndOneLocalizationException(
	t *testing.T,
) {
	t.Parallel()

	want := map[string]int{
		"semanticRequest":          1,
		"BuildLocalizationRequest": 1,
	}
	got := make(map[string]int)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(".", name)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				composite, ok := node.(*ast.CompositeLit)
				if !ok || len(composite.Elts) == 0 {
					return true
				}
				identifier, ok := composite.Type.(*ast.Ident)
				if ok && identifier.Name == "chatRequest" {
					got[function.Name.Name]++
				}
				return true
			})
		}
	}
	if len(got) != len(want) {
		t.Fatalf("chatRequest constructors = %#v, want %#v", got, want)
	}
	for function, count := range want {
		if got[function] != count {
			t.Fatalf("chatRequest constructor %s count = %d, want %d", function, got[function], count)
		}
	}
}
