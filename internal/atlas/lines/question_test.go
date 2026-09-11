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
	if strings.Contains(string(questionFieldJSON(t, input)), "private-") {
		t.Fatal("internal source identities reached question fields")
	}
}

func TestQuestionDocumentationIsLosslessAndAnchoredAcrossLongLines(t *testing.T) {
	text := "## Tests\nRead [the guide](docs/testing.md).\n\n```sh\nmake test-unit\nmake test-integration\nmake test-e2e\n```\n" + strings.Repeat("пример", DocumentChunkBytes) + "\nThe final instruction.\n"
	headings := []atlas.DocumentHeading{{Title: "Installed provider example", Line: 1}, {Title: "Tests", Line: 133}}
	place := atlas.Place{ID: "doc:private-document-id", Kind: atlas.PlaceDocument, Path: "dependency/CONTRIBUTING.md", LineNo: 133, Document: &atlas.DocumentFacts{Title: "Tests", Text: text, Headings: headings}}
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
		preserved := AnchorEvidence(row, "a1")
		evidence := preserved["evidence"].([]map[string]any)[0]
		// Question-batch v4: the section's own heading is section_title and
		// section_line, so heading_path holds only the enclosing titles, and
		// an anchor names its file only when it is not the row's path.
		if !reflect.DeepEqual(evidence["heading_path"], []string{"Installed provider example"}) || evidence["section_title"] != "Tests" || evidence["section_line"] != 133 {
			t.Fatalf("source partition lost the author's document scope: %+v", evidence)
		}
		if _, named := evidence["anchor_path"]; named || preserved["context"].(map[string]any)["path"] != place.Path {
			t.Fatalf("anchor repeated or lost the row's path: %+v", preserved)
		}
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
	if strings.Contains(string(questionFieldJSON(t, input)), place.ID) {
		t.Fatal("document identity reached question fields")
	}
}

func TestQuestionRowsKeepEveryDeclarationIncludingGeneratedTail(t *testing.T) {
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
	if strings.Contains(string(questionFieldJSON(t, inputRows)), "internal-id-never-sent") {
		t.Fatal("internal ID entered question fields")
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
	encoded := string(questionFieldJSON(t, input))
	for _, forbidden := range []string{"private-output-id", "private-missing-id"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatal("internal identity reached question fields")
		}
	}
}

// Question-batch v4 stops repeating what a row already says: a section's own
// heading closes no heading_path, an anchor names its file only when it is
// not the row's, and a type member that is an anchor of the same chunk is
// given by that ref. The preserved evidence of a selection reads on its own.
func TestQuestionEvidenceOmitsWhatItsRowAlreadySays(t *testing.T) {
	guide, install := atlas.DocumentHeading{Title: "Guide", Line: 1}, atlas.DocumentHeading{Title: "Install", Line: 4}
	sections := []atlas.Place{
		{ID: "doc:README.md:1", Kind: atlas.PlaceDocument, Path: "README.md", LineNo: 1, Document: &atlas.DocumentFacts{Title: "Guide", Text: "# Guide\n\n", Headings: []atlas.DocumentHeading{guide}}},
		{ID: "doc:README.md:4", Kind: atlas.PlaceDocument, Path: "README.md", LineNo: 4, Document: &atlas.DocumentFacts{Title: "Install", Text: "## Install\n\n", Headings: []atlas.DocumentHeading{guide, install}}},
		{ID: "doc:README.md:7", Kind: atlas.PlaceDocument, Path: "README.md", LineNo: 7, Document: &atlas.DocumentFacts{Title: "Linux", Text: "### Linux\nRun make.\n", Headings: []atlas.DocumentHeading{guide, install, {Title: "Linux", Line: 7}}}},
	}
	path := "queue/ticket.go"
	decls := []atlas.Decl{
		{Name: "Ticket", Kind: "type", LineNo: 3, ObjectID: "private-ticket", Signature: "type Ticket struct{}", Doc: "Tracks a pending job."},
		{Name: "Ticket.Renew", Kind: "method", LineNo: 9, ObjectID: "private-renew", Signature: "func() error", Doc: "Renew extends the ticket."},
		{Name: "Ticket.Done", Kind: "method", LineNo: 24, ObjectID: "private-done", Signature: "func()", Doc: "Done releases the ticket."},
		{Name: "Ticket.Close", Kind: "method", LineNo: 30, ObjectID: "private-close", Signature: "func()"},
	}
	members := []atlas.TypeMember{
		// A class attribute is never a declaration of the file: its record stays.
		{Path: path, Decl: atlas.Decl{Name: "items", Kind: "variable", LineNo: 4, Signature: "items []Point"}},
		// The same record as anchor a2.
		{Path: path, Decl: atlas.Decl{Name: "Ticket.Renew", Kind: "method", LineNo: 9, Signature: "func() error", Doc: "Renew extends the ticket."}},
		// Type context keeps the fuller author quote; the file row keeps its first sentence.
		{Path: path, Decl: atlas.Decl{Name: "Ticket.Done", Kind: "method", LineNo: 24, Signature: "func()", Doc: "Done releases the ticket. The job is then forgotten."}},
		// The same record as anchor a4, with no documentation either.
		{Path: path, Decl: atlas.Decl{Name: "Ticket.Close", Kind: "method", LineNo: 30, Signature: "func()"}},
		// Declared in another file: not an anchor of this row.
		{Path: "queue/renew.go", Decl: atlas.Decl{Name: "Ticket.Reset", Kind: "method", LineNo: 5, Signature: "func()"}},
	}
	file := atlas.Place{ID: atlas.FileID(path), Kind: atlas.PlaceFile, Path: path, File: &atlas.FileFacts{Decls: decls}}
	ticket := atlas.Place{ID: "sym:private-ticket", Kind: atlas.PlaceSymbol, Path: path, LineNo: 3, Parent: file.ID, Symbol: &atlas.SymbolFacts{Decl: decls[0], Members: members}}
	output := atlas.Place{ID: "entity:private-output", Kind: atlas.PlaceEntity, Path: "generated", Entity: &atlas.EntityFacts{Name: "output", Extractor: "company", Status: "present", Files: []string{path}}}
	graph := atlas.Graph{Places: append([]atlas.Place{file, ticket, output}, sections...),
		Edges: []atlas.Edge{{From: output.ID, To: output.ID, Kind: "observation", Evidence: &atlas.EdgeEvidence{Extractor: "company", Label: "configured output", Path: "codegen.json", LineNo: 7}}}}
	rows := QuestionRows(graph)
	var fileRow QuestionChunk
	wantHeadings := map[string]any{sections[0].ID: nil, sections[1].ID: []string{"Guide"}, sections[2].ID: []string{"Guide", "Install"}}
	for _, row := range rows {
		if row.Place.ID == file.ID {
			fileRow = row
		}
		want, isSection := wantHeadings[row.Place.ID]
		if !isSection {
			continue
		}
		unit := rowEvidence(t, row)[0]
		if !reflect.DeepEqual(unit["heading_path"], want) || unit["section_title"] != row.Place.Document.Title || unit["section_line"] != row.Place.LineNo {
			t.Fatalf("section %s: heading path repeats its own heading or lost its parents: %+v", row.Place.ID, unit)
		}
		if _, named := unit["anchor_path"]; named {
			t.Fatalf("section %s repeats the row's path: %+v", row.Place.ID, unit)
		}
	}
	if fileRow.Place.ID == "" {
		t.Fatal("file row is missing")
	}
	byRef := make(map[string]map[string]any)
	for _, unit := range rowEvidence(t, fileRow) {
		byRef[unit["ref"].(string)] = unit
	}
	wantOwned := []map[string]any{
		{"path": path, "line": 4, "name": "items", "kind": "variable", "signature": "items []Point", "author_doc": ""},
		{"ref": "a2"},
		{"path": path, "line": 24, "name": "Ticket.Done", "kind": "method", "signature": "func()", "author_doc": "Done releases the ticket. The job is then forgotten."},
		{"ref": "a4"},
		{"path": "queue/renew.go", "line": 5, "name": "Ticket.Reset", "kind": "method", "signature": "func()", "author_doc": ""},
	}
	if byRef["a1"]["name"] != "Ticket" || !reflect.DeepEqual(byRef["a1"]["owned_declarations"], wantOwned) {
		t.Fatalf("owned declarations of the type: %+v", byRef["a1"]["owned_declarations"])
	}
	for ref, unit := range byRef {
		named := unit["anchor_path"] != nil
		if observation := unit["kind"] == "observation"; named != observation || observation && unit["anchor_path"] != "codegen.json" {
			t.Fatalf("anchor %s repeats the row's path or lost its own: %+v", ref, unit)
		}
	}
	encoded := string(questionFieldJSON(t, []table.Row{fileRow.Row}))
	if strings.Count(encoded, "func() error") != 1 || strings.Count(encoded, `"anchor_path"`) != 1 || strings.Contains(encoded, "private-") {
		t.Fatalf("the row repeats a member declaration or a path, or leaks an identity: %s", encoded)
	}
	for _, ref := range []string{"a1", "file"} {
		preserved := AnchorEvidence(fileRow, ref)
		original := preserved["evidence"].([]map[string]any)[0]
		if !reflect.DeepEqual(original["owned_declarations"], ownedDeclarations(members)) {
			t.Fatalf("preserved evidence of %s did not restore the type's members: %+v", ref, original["owned_declarations"])
		}
		raw, err := json.Marshal(preserved)
		if err != nil || strings.Contains(string(raw), `"ref"`) || strings.Contains(string(raw), "private-") || preserved["context"].(map[string]any)["path"] != path {
			t.Fatalf("preserved evidence of %s carries a row-local ref or an identity, or lost the row's path: %s (%v)", ref, raw, err)
		}
	}
	observed := AnchorEvidence(fileRow, "a5")["evidence"].([]map[string]any)[0]
	if observed["kind"] != "observation" || observed["anchor_path"] != "codegen.json" {
		t.Fatalf("preserved observation lost its own file: %+v", observed)
	}
}

// A ref is local to its chunk: a member declared in another chunk of the
// same file keeps its complete record.
func TestQuestionOwnedDeclarationRefsStayWithinTheirChunk(t *testing.T) {
	path := "queue/many.go"
	decls := []atlas.Decl{{Name: "Ticket", Kind: "type", LineNo: 1, ObjectID: "private-ticket", Signature: "type Ticket struct{}"}}
	for i := 1; i < QuestionChunkAnchors; i++ {
		decls = append(decls, atlas.Decl{Name: fmt.Sprintf("helper%d", i), Kind: "function", LineNo: i + 1, ObjectID: fmt.Sprintf("private-helper-%d", i)})
	}
	last := atlas.Decl{Name: "Ticket.Done", Kind: "method", LineNo: QuestionChunkAnchors + 1, ObjectID: "private-done", Signature: "func()"}
	decls = append(decls, last)
	members := []atlas.TypeMember{{Path: path, Decl: atlas.Decl{Name: last.Name, Kind: last.Kind, LineNo: last.LineNo, Signature: last.Signature}}}
	file := atlas.Place{ID: atlas.FileID(path), Kind: atlas.PlaceFile, Path: path, File: &atlas.FileFacts{Decls: decls}}
	ticket := atlas.Place{ID: "sym:private-ticket", Kind: atlas.PlaceSymbol, Path: path, LineNo: 1, Parent: file.ID, Symbol: &atlas.SymbolFacts{Decl: decls[0], Members: members}}
	rows := QuestionRows(atlas.Graph{Places: []atlas.Place{file, ticket}})
	if len(rows) != 2 || rows[1].Anchors["a1"].Name != last.Name {
		t.Fatalf("chunks = %d, second chunk = %+v", len(rows), rows[1].Anchors)
	}
	if owned := rowEvidence(t, rows[0])[0]["owned_declarations"]; !reflect.DeepEqual(owned, ownedDeclarations(members)) {
		t.Fatalf("a member of another chunk was named by a ref this row cannot resolve: %+v", owned)
	}
}

func rowEvidence(t *testing.T, chunk QuestionChunk) []map[string]any {
	t.Helper()
	for _, field := range chunk.Row.Fields {
		if field.Name == "evidence" {
			return field.Value.([]map[string]any)
		}
	}
	t.Fatalf("row %s has no evidence", chunk.Row.ID)
	return nil
}

// The owning question cube serializes these fields; local Row.ID and the
// restoration map are deliberately outside this evidence contract.
func questionFieldJSON(t *testing.T, rows []table.Row) []byte {
	t.Helper()
	fields := make([][]table.Field, len(rows))
	for i, row := range rows {
		fields[i] = row.Fields
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
