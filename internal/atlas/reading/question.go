package reading

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

func (r *reader) readQuestions(ctx context.Context) error {
	chunks := r.questionRows()
	questions := append([]string{}, r.opts.Questions...)
	origins := make(map[string][]atlas.LearningOrigin)
	if r.learning != nil {
		for _, proposal := range r.learning.Questions {
			if !contains(questions, proposal.Question) {
				questions = append(questions, proposal.Question)
			}
			origins[proposal.Question] = append(origins[proposal.Question], proposal.Origins...)
		}
	}
	for _, question := range questions {
		r.questionText = question
		r.questionKey = fmt.Sprintf("%x", sha256.Sum256([]byte(question)))
		if err := r.readQuestion(ctx, chunks); err != nil {
			return err
		}
		r.question.UserQuestion = contains(r.opts.Questions, question)
		r.question.Origins = origins[question]
		if r.opts.Through != lines.StageQuestion {
			if err := r.readRoute(ctx); err != nil {
				return err
			}
			if r.opts.Through != lines.StageRoute {
				if err := r.readAnswer(ctx); err != nil {
					return err
				}
			}
		}
		if err := r.persistQuestion(); err != nil {
			return err
		}
		r.questions = append(r.questions, *r.question)
		r.question = nil
	}
	r.questionText, r.questionKey = "", ""
	return nil
}

func (r *reader) questionRows() []lines.QuestionChunk {
	chunks := lines.QuestionRows(r.opts.Graph)
	for i, chunk := range chunks {
		// Enrich the same fact rows with explicitly labelled prior model
		// interpretations. Canonical entity and knowledge IDs stay local.
		if known := r.knowledge[chunk.Place.ID]; known != nil {
			chunk.Row.Fields = append(chunk.Row.Fields, table.Field{Name: "file_model_hypothesis", Value: known.Cells["line"]})
		}
		for _, field := range chunk.Row.Fields {
			if field.Name != "evidence" {
				continue
			}
			for _, unit := range field.Value.([]map[string]any) {
				ref, _ := unit["ref"].(string)
				if known := r.knowledgeSubjects[chunk.Anchors[ref].SubjectID]; known != nil {
					unit["prior_model_hypothesis"] = known.Cells["line"]
				}
			}
		}
		chunks[i] = chunk
	}
	return chunks
}

func (r *reader) readQuestion(ctx context.Context, chunks []lines.QuestionChunk) error {
	rows := make([]table.Row, len(chunks))
	files, entities, documents := make(map[string]bool), make(map[string]bool), make(map[string]bool)
	sourceFacts := 0
	for i, chunk := range chunks {
		rows[i] = chunk.Row
		if chunk.Place.Kind == atlas.PlaceFile {
			files[chunk.Place.ID] = true
		} else if chunk.Place.Kind == atlas.PlaceDocument {
			documents[chunk.Place.Path] = true
		} else if chunk.Place.Kind == atlas.PlaceSourceFact {
			sourceFacts++
		} else {
			entities[chunk.Place.ID] = true
		}
	}
	r.opts.Stage(lines.StageQuestion, r.questionText, fmt.Sprintf("inspecting %d code files, %d documents, %d launch/manifest facts and %d observed entities in %d complete evidence chunks", len(files), len(documents), sourceFacts, len(entities), len(rows)))
	answers, err := r.runTableWith(ctx, lines.Question(), 0, []table.Field{
		{Name: "question", Value: r.questionText}, {Name: "repository", Value: r.opts.Repository},
	}, rows, func(answers table.Answers) error {
		for _, answer := range answers {
			if (answer["relevance"] == "none") != (answer["anchors"] == "") {
				return fmt.Errorf("none relevance and an empty anchor selection must agree")
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	route := atlas.QuestionRoute{
		Version: 7, Question: r.questionText, Repository: r.opts.Repository, Revision: r.opts.Revision, GraphSHA256: r.opts.Graph.SHA256,
		Scope: []string{
			"All file declarations and extracted boundaries in the saved places graph, including generated files.",
			"Existing entrypoint and manifest observations retain their exact source locations and component context; they do not prove a successful launch.",
			"Names, signatures and author documentation guide reading; function bodies and field mutations were not inspected by this pass.",
			"Producer observations and their corpus members are included; configured inputs and outputs do not prove execution or generated provenance.",
			"Connections distinguish compiler call witnesses, producer declarations and corpus membership; these are not a complete execution trace.",
			"No runtime observations, deployment verification, test coverage or external paper was supplied.",
			"Markdown sections retain the author's instructions, commands and links. Linked external contents were not read; documented commands were not executed.",
			"Previously accepted entity descriptions may be reused as labelled model hints, with their original evidence and dependencies in knowledge.json.",
		},
		Coverage: atlas.QuestionCoverage{Files: len(files), Documents: len(documents), SourceFacts: sourceFacts, Entities: len(entities), Chunks: len(chunks)},
		Stops:    []atlas.QuestionStop{}, Connections: []atlas.QuestionConnection{},
	}
	selected := make(map[string]bool)
	seen := make(map[string]int)
	for i, answer := range answers {
		if answer.source == atlas.SourceGiven {
			route.Coverage.UnresolvedChunks++
			continue
		}
		route.Coverage.InspectedChunks++
		if answer.answer["relevance"] == "none" {
			continue
		}
		chunk := chunks[i]
		for _, ref := range strings.Fields(answer.answer["anchors"]) {
			anchor, known := chunk.Anchors[ref]
			if !known {
				return fmt.Errorf("question: validated anchor is absent from its row")
			}
			stop := atlas.QuestionStop{
				SubjectID: anchor.SubjectID,
				Evidence:  lines.AnchorEvidence(chunk, ref),
				PlaceID:   chunk.Place.ID, Path: anchor.Path, Line: anchor.Line, Column: anchor.Column, Name: anchor.Name, Kind: anchor.Kind,
				TargetIDs: append([]string{}, chunk.Place.TargetIDs...), Relevance: answer.answer["relevance"], Why: answer.answer["why"], Source: answer.source,
			}
			key := fmt.Sprintf("%s:%s:%d:%d:%s:%s", stop.PlaceID, stop.Path, stop.Line, stop.Column, stop.Kind, stop.Name)
			for _, known := range []*Knowledge{r.knowledge[chunk.Place.ID], r.knowledgeSubjects[anchor.SubjectID]} {
				if known != nil && !contains(stop.KnowledgeIDs, known.ID) {
					stop.KnowledgeIDs = append(stop.KnowledgeIDs, known.ID)
				}
			}
			if index, exists := seen[key]; exists {
				if stop.Relevance == "direct" && route.Stops[index].Relevance != "direct" {
					route.Stops[index] = stop
				}
				continue
			}
			seen[key] = len(route.Stops)
			route.Stops = append(route.Stops, stop)
			selected[chunk.Place.ID] = true
		}
	}
	sort.Slice(route.Stops, func(i, j int) bool {
		a, b := route.Stops[i], route.Stops[j]
		if a.Relevance != b.Relevance {
			return a.Relevance == "direct"
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.PlaceID < b.PlaceID
	})
	for _, edge := range r.opts.Graph.Edges {
		if !selected[edge.From] || !selected[edge.To] || len(edge.Witnesses) == 0 && edge.Evidence == nil {
			continue
		}
		connection := atlas.QuestionConnection{
			FromID: edge.From, ToID: edge.To, Evidence: edge.Evidence,
			FromPath: r.places[edge.From].Path, ToPath: r.places[edge.To].Path, Kind: edge.Kind, Count: edge.Count, Witnesses: append([]atlas.Witness{}, edge.Witnesses...),
		}
		if from := r.places[edge.From].Entity; from != nil {
			connection.FromName = from.Name
		}
		if to := r.places[edge.To].Entity; to != nil {
			connection.ToName = to.Name
		}
		route.Connections = append(route.Connections, connection)
	}
	sort.Slice(route.Connections, func(i, j int) bool {
		a, b := route.Connections[i], route.Connections[j]
		if a.FromID != b.FromID {
			return a.FromID < b.FromID
		}
		if a.ToID != b.ToID {
			return a.ToID < b.ToID
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Evidence != nil && b.Evidence != nil {
			if a.Evidence.Label != b.Evidence.Label {
				return a.Evidence.Label < b.Evidence.Label
			}
			if a.Evidence.Path != b.Evidence.Path {
				return a.Evidence.Path < b.Evidence.Path
			}
			return a.Evidence.LineNo < b.Evidence.LineNo
		}
		return false
	})
	r.question = &route
	if err := r.persistQuestion(); err != nil {
		return err
	}
	r.reportStage(lines.StageQuestion)
	r.opts.State("Question candidates", "ready", fmt.Sprintf("reading stops: %d; connections with source evidence: %d", len(route.Stops), len(route.Connections)), "result: "+filepath.Join(r.opts.OwnerRunDir, atlas.QuestionFilename))
	return nil
}

func (r *reader) persistQuestion() error {
	routes := append([]atlas.QuestionRoute{}, r.questions...)
	if r.question != nil {
		routes = append(routes, *r.question)
	}
	data, err := json.MarshalIndent(atlas.QuestionRoutes{Version: 1, Routes: routes}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(r.opts.OwnerRunDir, atlas.QuestionFilename), data, 0o600); err != nil {
		return err
	}
	return nil
}
