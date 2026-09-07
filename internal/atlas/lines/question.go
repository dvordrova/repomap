package lines

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

const StageQuestion = "atlas_question"

// QuestionChunkAnchors is a partition size, not a selection limit: every
// declaration and boundary goes into exactly one chunk of its file.
const QuestionChunkAnchors = 24

// DocumentChunkBytes partitions prose losslessly; every part is reviewed.
// Leave room for JSON escaping, section context and the shared question.
const DocumentChunkBytes = 4096

//go:embed prompts/question.md
var questionPrompt string

func Question() table.Definition {
	return table.Definition{
		Stage: StageQuestion, Contract: "repomap.atlas.question.v6", Window: 24,
		System: questionPrompt,
		Columns: []table.Column{
			{Name: "relevance", Kind: table.Choice, Options: []string{"none", "direct", "context"}},
			{Name: "anchors", Kind: table.Sequence, OptionsFrom: "anchor_options"},
			{Name: "why", Kind: table.Text, MaxRunes: LineRunes},
		},
	}
}

// QuestionAnchor is restored locally; only its short row-local ref and
// explanatory facts enter the request. Paths and positions never come back
// from the model.
type QuestionAnchor struct {
	SubjectID string
	Path      string
	Name      string
	Kind      string
	Line      int
	Column    int
}

type questionUnit struct {
	anchor QuestionAnchor
	facts  map[string]any
}

type QuestionChunk struct {
	Row     table.Row
	Place   atlas.Place
	Anchors map[string]QuestionAnchor
}

func QuestionRows(graph atlas.Graph) []QuestionChunk {
	places := make(map[string]atlas.Place, len(graph.Places))
	boundaries := make(map[string][]atlas.Place)
	typeDeclarations := make(map[string][]atlas.TypeMember)
	var files []atlas.Place
	for _, place := range graph.Places {
		places[place.ID] = place
		if place.Symbol != nil && place.Symbol.Decl.Kind == "type" {
			typeDeclarations[place.Symbol.Decl.ObjectID] = place.Symbol.Members
		}
		if place.Kind == atlas.PlaceFile {
			files = append(files, place)
		}
		if place.Kind == atlas.PlaceBoundary {
			boundaries[place.Parent] = append(boundaries[place.Parent], place)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	observations := questionObservations(graph, places)
	var result []QuestionChunk
	for _, file := range files {
		var units []questionUnit
		for _, decl := range file.File.Decls {
			facts := map[string]any{"name": decl.Name, "kind": decl.Kind, "signature": decl.Signature, "author_doc": decl.Doc}
			if members := typeDeclarations[decl.ObjectID]; len(members) > 0 {
				facts["owned_declarations"] = ownedDeclarations(members)
			}
			units = append(units, questionUnit{
				anchor: QuestionAnchor{SubjectID: decl.ObjectID, Path: file.Path, Name: decl.Name, Kind: decl.Kind, Line: decl.LineNo, Column: decl.Column},
				facts:  facts,
			})
		}
		for _, boundary := range boundaries[file.ID] {
			facts := boundary.Boundary
			units = append(units, questionUnit{
				anchor: QuestionAnchor{SubjectID: boundary.ID, Path: file.Path, Name: facts.Caller, Kind: "boundary", Line: boundary.LineNo},
				facts:  map[string]any{"kind": "boundary", "caller": facts.Caller, "external": facts.External, "method": facts.Method, "values": facts.Values, "direction": facts.Direction, "known_kind": facts.GivenKind},
			})
		}
		sort.SliceStable(units, func(i, j int) bool {
			if units[i].anchor.Line != units[j].anchor.Line {
				return units[i].anchor.Line < units[j].anchor.Line
			}
			if units[i].anchor.Name != units[j].anchor.Name {
				return units[i].anchor.Name < units[j].anchor.Name
			}
			return units[i].anchor.Kind < units[j].anchor.Kind
		})
		units = append(units, observations[file.ID]...)
		fields := []table.Field{{Name: "generated", Value: file.File.Generated}, {Name: "file_author_doc", Value: file.File.Doc}}
		if directory := places[file.Parent]; directory.Directory != nil {
			fields = append(fields, table.Field{Name: "directory_author_doc", Value: directory.Directory.Doc}, table.Field{Name: "directory_readme_claim", Value: directory.Directory.Readme})
		}
		result = append(result, questionChunks(file, units, fields)...)
	}
	for _, place := range graph.Places {
		if place.Document != nil {
			result = append(result, documentQuestionChunks(place)...)
		}
		if place.Entity == nil {
			continue
		}
		units := append([]questionUnit{}, observations[place.ID]...)
		for _, path := range place.Entity.Files {
			line := 1
			subjectID := places[atlas.FileID(path)].ID
			if path == place.Path {
				line = max(1, place.LineNo)
				if subjectID == "" {
					subjectID = place.ID
				}
			}
			units = append(units, questionUnit{anchor: QuestionAnchor{SubjectID: subjectID, Path: path, Name: path, Kind: "corpus_file", Line: line},
				facts: map[string]any{"kind": "corpus_membership", "path": path, "referenced_path": place.Path}})
		}
		result = append(result, questionChunks(place, units, []table.Field{
			{Name: "entity", Value: entityDescription(place)},
		})...)
	}
	// Append launch/manifest observations after the existing evidence reservoir.
	// Unchanged earlier windows retain their exact request/cache identity.
	for _, place := range graph.Places {
		fact := place.SourceFact
		if fact == nil {
			continue
		}
		name, subject := fact.Name, fact.ObjectID
		if name == "" {
			name = fact.Key
		}
		if subject == "" {
			subject = place.ID
		}
		evidence := map[string]any{"kind": fact.Kind, "name": fact.Name, "value": fact.Value,
			"language": fact.Language, "component": fact.Component, "component_kind": fact.ComponentKind, "root": fact.Root}
		if fact.Kind == "entrypoint" {
			evidence["entrypoint_kind"] = fact.Key
		} else {
			evidence["manifest_key"] = fact.Key
		}
		unit := questionUnit{anchor: QuestionAnchor{SubjectID: subject, Path: place.Path, Name: name, Kind: fact.Kind, Line: place.LineNo, Column: place.Column}, facts: evidence}
		result = append(result, questionChunks(place, []questionUnit{unit}, nil)...)
	}
	return result
}

func documentQuestionChunks(place atlas.Place) []QuestionChunk {
	var result []QuestionChunk
	text := place.Document.Text
	line, column := place.LineNo, 1
	for len(text) > 0 {
		end := min(DocumentChunkBytes, len(text))
		if end < len(text) {
			if newline := strings.LastIndexByte(text[:end], '\n'); newline >= 0 {
				end = newline + 1
			} else {
				for !utf8.RuneStart(text[end]) {
					end--
				}
			}
		}
		part := text[:end]
		anchor := QuestionAnchor{SubjectID: place.ID, Path: place.Path, Name: place.Document.Title, Kind: "documentation", Line: line, Column: column}
		facts := map[string]any{"kind": "documentation", "section_title": place.Document.Title, "section_line": place.LineNo, "author_text": part}
		chunks := questionChunks(place, []questionUnit{{anchor: anchor, facts: facts}}, nil)
		row := chunks[0]
		row.Row.ID = fmt.Sprintf("%s#%d", place.ID, len(result)+1)
		result = append(result, row)
		line += strings.Count(part, "\n")
		if newline := strings.LastIndexByte(part, '\n'); newline >= 0 {
			column = len(part) - newline
		} else {
			column += len(part)
		}
		text = text[end:]
	}
	for i := range result {
		for j := range result[i].Row.Fields {
			switch result[i].Row.Fields[j].Name {
			case "chunk":
				result[i].Row.Fields[j].Value = i + 1
			case "chunks":
				result[i].Row.Fields[j].Value = len(result)
			}
		}
	}
	return result
}

func questionChunks(place atlas.Place, units []questionUnit, context []table.Field) []QuestionChunk {
	var result []QuestionChunk
	count := max(1, (len(units)+QuestionChunkAnchors-1)/QuestionChunkAnchors)
	for chunk := 0; chunk < count; chunk++ {
		anchors := map[string]QuestionAnchor{}
		options := []string{}
		if place.Kind == atlas.PlaceFile {
			anchors["file"] = QuestionAnchor{SubjectID: place.ID, Path: place.Path, Kind: "file", Line: 1}
			options = append(options, "file")
		}
		facts := make([]map[string]any, 0)
		start := chunk * QuestionChunkAnchors
		for i, item := range units[start:min(start+QuestionChunkAnchors, len(units))] {
			ref := fmt.Sprintf("a%d", i+1)
			anchors[ref] = item.anchor
			options = append(options, ref)
			item.facts["ref"] = ref
			item.facts["anchor_path"] = item.anchor.Path
			item.facts["anchor_line"] = item.anchor.Line
			facts = append(facts, item.facts)
		}
		fields := []table.Field{
			{Name: "path", Value: place.Path}, {Name: "place_kind", Value: string(place.Kind)},
			{Name: "chunk", Value: chunk + 1}, {Name: "chunks", Value: count},
			{Name: "evidence", Value: facts}, {Name: "anchor_options", Value: options},
		}
		fields = append(fields, context...)
		result = append(result, QuestionChunk{Row: table.Row{ID: fmt.Sprintf("%s#%d", place.ID, chunk+1), Fields: fields}, Place: place, Anchors: anchors})
	}
	return result
}

func entityDescription(place atlas.Place) map[string]any {
	return map[string]any{"name": place.Entity.Name, "path": place.Path, "extractor": place.Entity.Extractor, "corpus_status": place.Entity.Status}
}

// Every observation remains independent. Related file rows receive the same
// source declaration, never another model's interpretation or a guessed call.
func questionObservations(graph atlas.Graph, places map[string]atlas.Place) map[string][]questionUnit {
	result := make(map[string][]questionUnit)
	for _, edge := range graph.Edges {
		if edge.Kind != "observation" {
			continue
		}
		from, to := places[edge.From], places[edge.To]
		e := edge.Evidence
		add := func(id string, member string) {
			facts := map[string]any{"kind": "observation", "from": entityDescription(from), "to": entityDescription(to), "label": e.Label, "extractor": e.Extractor, "source_path": e.Path}
			if member != "" {
				facts["member_path"] = member
			}
			result[id] = append(result[id], questionUnit{anchor: QuestionAnchor{SubjectID: edge.From, Path: e.Path, Line: e.LineNo, Name: e.Label, Kind: "observation"}, facts: facts})
		}
		add(edge.From, "")
		if edge.To != edge.From {
			add(edge.To, "")
		}
		seen := make(map[string]bool)
		for _, node := range []atlas.Place{from, to} {
			for _, path := range node.Entity.Files {
				id := atlas.FileID(path)
				if places[id].File != nil && !seen[id] {
					add(id, path)
					seen[id] = true
				}
			}
		}
	}
	return result
}
