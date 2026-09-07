package lines

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

func TestQuestionSourceFactsKeepLaunchContextAndExistingRequestPrefix(t *testing.T) {
	file := atlas.Place{ID: "file:main.go", Kind: atlas.PlaceFile, Path: "main.go", File: &atlas.FileFacts{Decls: []atlas.Decl{{Name: "main", Kind: "function", LineNo: 5, ObjectID: "private-main"}}}}
	before := QuestionRows(atlas.Graph{Places: []atlas.Place{file}})
	launch := atlas.Place{ID: "fact:private-launch", Kind: atlas.PlaceSourceFact, Path: "main.go", LineNo: 5, Column: 1,
		SourceFact: &atlas.SourceFact{Kind: "entrypoint", Name: "main", Key: "main", ObjectID: "private-main", Language: "go", Component: "server", ComponentKind: "executable", Root: "."}}
	manifest := atlas.Place{ID: "fact:private-manifest", Kind: atlas.PlaceSourceFact, Path: "go.mod", LineNo: 3,
		SourceFact: &atlas.SourceFact{Kind: "manifest", Key: "go", Value: "1.26", Language: "go", Component: "server", ComponentKind: "executable", Root: "."}}
	after := QuestionRows(atlas.Graph{Places: []atlas.Place{launch, manifest, file}})
	if len(after) != len(before)+2 || !reflect.DeepEqual(before, after[:len(before)]) {
		t.Fatal("new observations changed unrelated question rows")
	}
	for i, place := range []atlas.Place{launch, manifest} {
		row := after[len(before)+i]
		anchor := row.Anchors["a1"]
		if anchor.Path != place.Path || anchor.Line != place.LineNo || anchor.Column != place.Column || anchor.Kind != place.SourceFact.Kind {
			t.Fatalf("original source was replaced by the enclosing declaration: %+v", anchor)
		}
		if i == 0 && anchor.SubjectID != "private-main" || i == 1 && anchor.SubjectID != manifest.ID {
			t.Fatal("source lost its native or fact identity")
		}
		original := AnchorEvidence(row, "a1")["evidence"].([]map[string]any)[0]
		if original["language"] != "go" || original["component"] != "server" || original["root"] != "." || original["value"] != place.SourceFact.Value {
			t.Fatalf("question source lost launch/manifest context: %+v", original)
		}
	}
	var input []table.Row
	for _, chunk := range after {
		input = append(input, chunk.Row)
	}
	windows, err := table.WindowsWithContext(Question(), 0, nil, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, window := range windows {
		if strings.Contains(string(window.Request), "private-") {
			t.Fatal("internal source identities reached the provider")
		}
	}
}

func TestQuestionDocumentationIsLosslessAndAnchoredAcrossLongLines(t *testing.T) {
	text := "## Tests\nRead [the guide](docs/testing.md).\n\n```sh\nmake test-unit\nmake test-integration\nmake test-e2e\n```\n" + strings.Repeat("пример", DocumentChunkBytes) + "\nThe final instruction.\n"
	place := atlas.Place{ID: "doc:private-document-id", Kind: atlas.PlaceDocument, Path: "CONTRIBUTING.md", LineNo: 133, Document: &atlas.DocumentFacts{Title: "Tests", Text: text}}
	rows := QuestionRows(atlas.Graph{Places: []atlas.Place{place}})
	var joined strings.Builder
	var input []table.Row
	line, column := place.LineNo, 1
	for _, row := range rows {
		input = append(input, row.Row)
		anchor := row.Anchors["a1"]
		if anchor.Line != line || anchor.Column != column || anchor.SubjectID != place.ID || anchor.Kind != "documentation" {
			t.Fatalf("source anchor lost across a partition: %+v", anchor)
		}
		evidence := AnchorEvidence(row, "a1")["evidence"].([]map[string]any)[0]
		part := evidence["author_text"].(string)
		if !utf8.ValidString(part) || len(part) > DocumentChunkBytes {
			t.Fatal("partition split UTF-8 or exceeded its input allocation")
		}
		joined.WriteString(part)
		line += strings.Count(part, "\n")
		if newline := strings.LastIndexByte(part, '\n'); newline >= 0 {
			column = len(part) - newline
		} else {
			column += len(part)
		}
	}
	if len(rows) < 3 || joined.String() != text {
		t.Fatal("commands, links, a long line or the document tail were discarded")
	}
	windows, err := table.WindowsWithContext(Question(), 0, nil, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, window := range windows {
		if strings.Contains(string(window.Request), place.ID) {
			t.Fatal("document identity reached the provider")
		}
	}
}

func TestQuestionRowsKeepEveryDeclarationIncludingGeneratedTail(t *testing.T) {
	if !strings.Contains(strings.ToLower(Question().System), "json") {
		t.Fatal("provider JSON mode requires an explicit JSON instruction")
	}
	file := atlas.Place{ID: "file:generated.go", Kind: atlas.PlaceFile, Path: "generated.go", File: &atlas.FileFacts{Generated: true}}
	for i := 0; i < QuestionChunkAnchors*2+1; i++ {
		file.File.Decls = append(file.File.Decls, atlas.Decl{Name: fmt.Sprintf("D%d", i), Kind: "function", LineNo: i + 1, ObjectID: "internal-id-never-sent"})
	}
	rows := QuestionRows(atlas.Graph{Places: []atlas.Place{file}})
	if len(rows) != 3 {
		t.Fatalf("chunks = %d", len(rows))
	}
	seen := make(map[string]int)
	var inputRows []table.Row
	for _, row := range rows {
		inputRows = append(inputRows, row.Row)
		for ref, anchor := range row.Anchors {
			if ref != "file" {
				seen[anchor.Name]++
			}
		}
	}
	for _, decl := range file.File.Decls {
		if seen[decl.Name] != 1 {
			t.Fatalf("declaration %s occurs %d times", decl.Name, seen[decl.Name])
		}
	}
	windows, err := table.WindowsWithContext(Question(), 0, []table.Field{{Name: "question", Value: "Where is state?"}}, inputRows)
	if err != nil {
		t.Fatal(err)
	}
	for _, window := range windows {
		if strings.Contains(string(window.Request), "internal-id-never-sent") {
			t.Fatal("internal ID entered provider input")
		}
		if !json.Valid(window.Request) {
			t.Fatal("invalid request")
		}
	}
}

func TestObservationInputKeepsAllMembersAndNeverOffersAMissingPath(t *testing.T) {
	output := atlas.Place{ID: "entity:private-output-id", Kind: atlas.PlaceEntity, Path: "generated", Entity: &atlas.EntityFacts{Name: "output", Extractor: "company", Status: "present", Files: []string{}}}
	for i := 0; i < QuestionChunkAnchors*2+1; i++ {
		output.Entity.Files = append(output.Entity.Files, fmt.Sprintf("generated/f%03d.go", i))
	}
	missing := atlas.Place{ID: "entity:private-missing-id", Kind: atlas.PlaceEntity, Path: "missing", Entity: &atlas.EntityFacts{Name: "unbuilt", Extractor: "company", Status: "not_in_corpus", Files: []string{}}}
	graph := atlas.Graph{Places: []atlas.Place{output, missing}, Edges: []atlas.Edge{{From: output.ID, To: missing.ID, Kind: "observation", Evidence: &atlas.EdgeEvidence{Extractor: "company", Label: "configured output", Path: "codegen.json", LineNo: 7}}}}
	rows := QuestionRows(graph)
	members := map[string]int{}
	var input []table.Row
	for _, row := range rows {
		input = append(input, row.Row)
		for _, anchor := range row.Anchors {
			if anchor.Kind == "corpus_file" {
				members[anchor.Path]++
			}
			if anchor.Path == "missing" || anchor.Path == "generated" {
				t.Fatal("offered a directory or missing file as a source anchor")
			}
			if anchor.Kind == "observation" && (anchor.Path != "codegen.json" || anchor.Line != 7) {
				t.Fatal("source declaration moved")
			}
		}
	}
	if len(members) != len(output.Entity.Files) {
		t.Fatal("tail inventory was truncated")
	}
	for _, count := range members {
		if count != 1 {
			t.Fatal("member repeated across chunks")
		}
	}
	windows, err := table.WindowsWithContext(Question(), 0, nil, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, window := range windows {
		for _, forbidden := range []string{"private-output-id", "private-missing-id"} {
			if strings.Contains(string(window.Request), forbidden) {
				t.Fatal("internal identity reached the model")
			}
		}
	}
}
