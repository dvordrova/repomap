package reading

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/questionbatch"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

func TestQuestionsShareGraphAndKeepIndependentCacheEntries(t *testing.T) {
	opts, provider := questionFixture(t)
	first := opts.Questions[0]
	second := "How does state change?"
	opts.Questions = []string{first, second}
	read := func(wantCalls int) Result {
		t.Helper()
		result, err := Read(context.Background(), opts)
		if err != nil {
			t.Fatal(err)
		}
		if provider.calls != wantCalls || len(result.Questions) != len(opts.Questions) {
			t.Fatalf("calls=%d, questions=%d", provider.calls, len(result.Questions))
		}
		for i, route := range result.Questions {
			if route.Question != opts.Questions[i] || route.GraphSHA256 != result.Questions[0].GraphSHA256 || len(route.Stops) != 4 {
				t.Fatalf("question lost its identity or shared graph: %+v", route)
			}
			pattern := fmt.Sprintf("%s-%x-r0-w*.request.ref.json", lines.StageQuestion, sha256.Sum256([]byte(route.Question)))
			files, err := filepath.Glob(filepath.Join(opts.OwnerRunDir, atlas.TablesDir, pattern))
			if err != nil || len(files) != 4 {
				t.Fatalf("question request references overwritten: %v %v", files, err)
			}
			for _, file := range files {
				raw, err := readWindowPayload(file)
				if err != nil || !strings.Contains(string(raw), route.Question) {
					t.Fatalf("question reference lost its original shared request: %s / %v", file, err)
				}
			}
		}
		return result
	}
	initial := read(4)
	// Reordering questions changes presentation, not provider requests.
	opts.OwnerRunDir, opts.Questions = t.TempDir(), []string{second, first}
	warm := read(4)
	for _, route := range warm.Questions {
		if route.GraphSHA256 != initial.Questions[0].GraphSHA256 || route.Stops[0].Source != atlas.SourceCache {
			t.Fatalf("reordered question did not retain cached source provenance: %+v", route)
		}
	}
	// Adding one question must not invalidate either existing question.
	opts.OwnerRunDir = t.TempDir()
	opts.Questions = append(opts.Questions, "Where are the tests?")
	expanded := read(8)
	for i, route := range expanded.Questions {
		wantSource := atlas.SourceCache
		if i == 2 {
			wantSource = atlas.SourceModel
		}
		if route.Stops[0].Source != wantSource {
			t.Fatalf("added question changed old provenance: question=%s source=%s", route.Question, route.Stops[0].Source)
		}
	}
	raw, err := os.ReadFile(filepath.Join(opts.OwnerRunDir, atlas.QuestionFilename))
	if err != nil {
		t.Fatal(err)
	}
	var saved atlas.QuestionRoutes
	if err := json.Unmarshal(raw, &saved); err != nil || len(saved.Routes) != 3 {
		t.Fatalf("saved collection lost a question: %+v %v", saved, err)
	}
}

func questionFixture(t *testing.T) (Options, *tableProvider) {
	t.Helper()
	provider := &tableProvider{questionFor: make(map[string]table.Answer)}
	for _, path := range []string{"pkg/a/x.go", "pkg/a/y.go", "pkg/b/z.go", "pkg/b/gen.go"} {
		provider.questionFor[path] = table.Answer{"relevance": "direct", "anchors": "a1", "why": "Inspect this declaration."}
	}
	opts := readOptions(t, testGraph(t), provider, t.TempDir())
	opts.Questions, opts.Through, opts.WindowRows = []string{"Where is state stored?"}, lines.StageQuestion, 1
	return opts, provider
}

func readQuestionResult(t *testing.T, opts Options) (Result, atlas.QuestionRoute) {
	t.Helper()
	result, err := Read(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(opts.OwnerRunDir, atlas.QuestionFilename))
	if err != nil {
		t.Fatal(err)
	}
	var routes atlas.QuestionRoutes
	if err := json.Unmarshal(raw, &routes); err != nil {
		t.Fatal(err)
	}
	if routes.Version != 1 || len(routes.Routes) != 1 {
		t.Fatalf("question collection: %+v", routes)
	}
	return result, routes.Routes[0]
}

func TestQuestionRestoresAnchorsAndOnlyWitnessedFileConnections(t *testing.T) {
	opts, provider := questionFixture(t)
	// places.Build returns an unsealed in-memory value on the ordinary path.
	opts.Graph.SHA256 = ""
	result, route := readQuestionResult(t, opts)
	input, err := LoadInput(filepath.Join(opts.OwnerRunDir, InputFilename))
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete || len(result.Uses) != 1 || result.Uses[0].Stage != lines.StageQuestion {
		t.Fatalf("question ran map stages: %+v", result.Uses)
	}
	if route.Question != opts.Questions[0] || route.GraphSHA256 == "" || route.GraphSHA256 != input.Graph.SHA256 || len(route.Stops) != 4 {
		t.Fatalf("route binding: %+v", route)
	}
	if route.Coverage.Files != 4 || route.Coverage.InspectedChunks != 4 || route.Coverage.UnresolvedChunks != 0 {
		t.Fatalf("coverage: %+v", route.Coverage)
	}
	positions := map[string]int{"pkg/a/x.go": 3, "pkg/a/y.go": 3, "pkg/b/z.go": 5, "pkg/b/gen.go": 9}
	for _, stop := range route.Stops {
		if stop.Line != positions[stop.Path] || stop.Source != atlas.SourceModel {
			t.Fatalf("anchor not restored: %+v", stop)
		}
	}
	if len(route.Connections) != 1 || route.Connections[0].FromPath != "pkg/a/x.go" || route.Connections[0].ToPath != "pkg/a/y.go" || route.Connections[0].Witnesses[0].LineNo != 4 {
		t.Fatalf("unwitnessed or invented connections: %+v", route.Connections)
	}
	if provider.calls != 4 {
		t.Fatalf("calls = %d", provider.calls)
	}

	// An identical question reuses its windows. Changing only the question
	// must invalidate all its requests and never run unrelated map stages.
	// Starting with the saved graph must keep the ordinary run's binding.
	opts.Graph = input.Graph
	opts.OwnerRunDir = t.TempDir()
	_, warm := readQuestionResult(t, opts)
	if provider.calls != 4 || warm.Stops[0].Source != atlas.SourceCache || warm.GraphSHA256 != route.GraphSHA256 {
		t.Fatal("question cache not reused")
	}
	opts.OwnerRunDir, opts.Questions = t.TempDir(), []string{"How does state change?"}
	readQuestionResult(t, opts)
	if provider.calls != 8 {
		t.Fatal("changed question reused an incompatible answer")
	}
}

func TestQuestionKeepsComplementaryDeclarationsFromOneChunk(t *testing.T) {
	opts, provider := questionFixture(t)
	for i := range opts.Graph.Places {
		place := &opts.Graph.Places[i]
		if place.ID != atlas.FileID("pkg/a/x.go") {
			continue
		}
		place.File.Decls[0].ObjectID = "start-declaration"
		place.File.Decls = append(place.File.Decls, atlas.Decl{ObjectID: "stop-declaration", Name: "Stop", Kind: "function", Signature: "func()", Doc: "Stop cancels the worker.", LineNo: 8})
	}
	provider.questionFor["pkg/a/x.go"] = table.Answer{"relevance": "direct", "anchors": "a2 a1 a2 a999", "why": "Inspect the worker lifecycle declarations."}
	result, route := readQuestionResult(t, opts)
	if len(result.Rejected) != 0 || route.Coverage.InspectedChunks != 4 || len(route.Stops) != 5 {
		t.Fatalf("context partition discarded a useful declaration: %+v", route)
	}
	var selected []atlas.QuestionStop
	for _, stop := range route.Stops {
		if stop.Path == "pkg/a/x.go" {
			selected = append(selected, stop)
		}
	}
	if len(selected) != 2 || selected[0].SubjectID != "start-declaration" || selected[0].Line != 3 || selected[1].SubjectID != "stop-declaration" || selected[1].Line != 8 {
		t.Fatalf("selected declarations lost their own identities: %+v", selected)
	}
	for _, stop := range selected {
		evidence, ok := stop.Evidence["evidence"].([]any)
		if !ok || len(evidence) != 1 {
			t.Fatalf("selected declaration lost its own evidence: %+v", stop.Evidence)
		}
		declaration, ok := evidence[0].(map[string]any)
		if !ok || declaration["name"] != stop.Name {
			t.Fatalf("another declaration's evidence attached to %s: %+v", stop.Name, stop.Evidence)
		}
	}
}

func TestQuestionRejectedAnchorDoesNotBecomeANegativeFinding(t *testing.T) {
	opts, provider := questionFixture(t)
	provider.questionFor["pkg/a/y.go"]["anchors"] = "a999"
	result, route := readQuestionResult(t, opts)
	if len(result.Rejected) != 1 || route.Coverage.UnresolvedChunks != 1 || route.Coverage.InspectedChunks != 3 || len(route.Stops) != 3 {
		t.Fatalf("refused anchor became an answer: %+v", route)
	}
	if len(route.Connections) != 0 {
		t.Fatal("refused row entered a connection")
	}

	opts.OwnerRunDir, opts.Provider = t.TempDir(), nil
	_, dry := readQuestionResult(t, opts)
	if len(dry.Stops) != 0 || dry.Coverage.UnresolvedChunks != 4 || dry.Coverage.InspectedChunks != 0 {
		t.Fatalf("dry run invented model findings: %+v", dry)
	}
}

func TestQuestionCanRunOnTheOrdinaryReadingPath(t *testing.T) {
	opts, provider := questionFixture(t)
	opts.Through, opts.WindowRows = "", 0
	opts.Questions = append(opts.Questions, "Where is the entry point?")
	for i := range opts.Graph.Places {
		if place := &opts.Graph.Places[i]; place.Path == "pkg/a/x.go" && place.File != nil {
			place.File.Decls[0].ObjectID = "original-main-declaration"
		}
	}
	provider.questionBatchFor = func(request questionBatchRequest, response questionbatch.Response) questionbatch.Response {
		for i, question := range request.Questions {
			if question.Question != opts.Questions[1] {
				continue
			}
			response.Questions[i].Selections = nil
			for _, row := range request.Evidence {
				if row["path"] == "pkg/a/x.go" {
					response.Questions[i].Selections = append(response.Questions[i].Selections, questionbatch.Selection{
						Row: row["key"].(string), Anchors: []string{"a1"}, Relevance: "direct", Why: "Inspect the original entry declaration.",
					})
				}
			}
		}
		return response
	}
	result, err := Read(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || len(result.Atlas.Targets) == 0 || result.Through != lines.StageAnswer || len(result.Questions) != 2 || len(provider.questionRequests) != 1 {
		t.Fatalf("ordinary questions did not share one provider call: complete=%t questions=%d retrieval calls=%d", result.Complete, len(result.Questions), len(provider.questionRequests))
	}
	var sent questionBatchRequest
	if err := json.Unmarshal(provider.questionRequests[0], &sent); err != nil {
		t.Fatal(err)
	}
	if len(sent.Evidence) != 4 || len(sent.Questions) != 2 || strings.Contains(string(provider.questionRequests[0]), "original-main-declaration") {
		t.Fatal("shared request duplicated its corpus, omitted a question or exposed local source identity")
	}
	input, err := LoadInput(filepath.Join(opts.OwnerRunDir, InputFilename))
	if err != nil {
		t.Fatal(err)
	}
	for i, route := range result.Questions {
		if route.Question != opts.Questions[i] || route.GraphSHA256 != input.Graph.SHA256 || route.Coverage.InspectedChunks != 4 || route.Coverage.UnresolvedChunks != 0 || route.Guide == nil || route.Answer == nil {
			t.Fatalf("shared retrieval changed a question's graph, coverage or downstream reading: %+v", route)
		}
		pattern := fmt.Sprintf("%s-%x-r0-w*.request.ref.json", lines.StageQuestion, sha256.Sum256([]byte(route.Question)))
		refs, err := filepath.Glob(filepath.Join(opts.OwnerRunDir, atlas.TablesDir, pattern))
		if err != nil || len(refs) != 1 {
			t.Fatalf("question lost its shared request reference: %v %v", refs, err)
		}
		raw, err := readWindowPayload(refs[0])
		if err != nil || string(raw) != string(provider.questionRequests[0]) {
			t.Fatalf("question reference no longer resolves its exact shared provider bytes: %v", err)
		}
	}
	first, second := result.Questions[0], result.Questions[1]
	if len(first.Stops) != 4 || len(first.Connections) != 1 || first.Connections[0].Witnesses[0].LineNo != 4 || len(second.Stops) != 1 || len(second.Connections) != 0 {
		t.Fatalf("questions competed for sources or shared decisions: first=%+v second=%+v", first.Stops, second.Stops)
	}
	stop := second.Stops[0]
	if stop.SubjectID != "original-main-declaration" || stop.Name != "Main" || stop.Path != "pkg/a/x.go" || stop.Line != 3 || stop.Why != "Inspect the original entry declaration." {
		t.Fatalf("entry question lost its exact source: %+v", stop)
	}
	for _, use := range result.Uses {
		if use.Stage == lines.StageQuestion && (use.Windows != 1 || use.Live != 1 || use.Rows != 8) {
			t.Fatalf("retrieval accounting counted question/corpus pairs as provider calls: %+v", use)
		}
	}

	t.Run("missing question rejects the whole shared window", func(t *testing.T) {
		opts, provider := questionFixture(t)
		opts.Through, opts.WindowRows = "", 0
		opts.Questions = append(opts.Questions, "Where is the entry point?")
		provider.questionBatchFor = func(_ questionBatchRequest, response questionbatch.Response) questionbatch.Response {
			return questionbatch.Response{Questions: response.Questions[:1]}
		}
		result, err := Read(context.Background(), opts)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Questions) != 2 || len(provider.questionRequests) != 1 || len(result.Rejected) != 1 || result.Rejected[0].Stage != lines.StageQuestion || result.Rejected[0].Count != 8 {
			t.Fatalf("missing question was not one refused shared window: %+v", result.Rejected)
		}
		for _, route := range result.Questions {
			if len(route.Stops) != 0 || route.Coverage.InspectedChunks != 0 || route.Coverage.UnresolvedChunks != 4 || route.Answer == nil || route.Answer.State != "unavailable" {
				t.Fatalf("missing decision became a negative finding or partial success: %+v", route)
			}
		}
		provider.questionBatchFor = nil
		opts.OwnerRunDir = t.TempDir()
		retried, err := Read(context.Background(), opts)
		if err != nil {
			t.Fatal(err)
		}
		if len(provider.questionRequests) != 2 || len(retried.Rejected) != 0 {
			t.Fatal("refused shared response was reused from cache")
		}
		for _, route := range retried.Questions {
			if len(route.Stops) != 4 || route.Coverage.UnresolvedChunks != 0 || route.Stops[0].Source != atlas.SourceModel {
				t.Fatalf("fresh complete response did not restore the question: %+v", route)
			}
		}
	})
}

func TestAnswerKeepsOriginalSourcesAndReusesTheReadingRoute(t *testing.T) {
	opts, provider := questionFixture(t)
	opts.Through = lines.StageAnswer
	readerGuess := "Which unsupported framework import command is required?"
	provider.routeFor = func(map[string]any) table.Answer {
		return table.Answer{"order": "c4 c2", "open_question": readerGuess}
	}
	provider.answerFor = func(map[string]any) table.Answer {
		return table.Answer{"state": "partial", "answer": "The source declares the storage interface.", "sources": "c2 c999 c1 c2", "remaining": "The persistence implementation is not present in this evidence."}
	}
	result, route := readQuestionResult(t, opts)
	if len(result.Rejected) != 0 || result.Through != lines.StageAnswer || route.Version != 7 || route.Answer.State != "partial" {
		t.Fatalf("answer failed: %+v", route.Answer)
	}
	part := route.Answer.Parts[0]
	if len(part.Steps) != 2 || part.Steps[0].Path != route.Stops[1].Path || part.Steps[1].Path != route.Stops[3].Path {
		t.Fatalf("source refs not restored through selected route: %+v", part.Steps)
	}
	if part.Text != "The source declares the storage interface." || part.Basis != "The selected declarations and their signatures suggest this role." || part.Source != atlas.SourceModel {
		t.Fatalf("answer changed: %+v", part)
	}
	if route.Guide.OpenQuestion != readerGuess {
		t.Fatal("the reading route lost its original interpretation")
	}
	inputs, err := filepath.Glob(filepath.Join(opts.OwnerRunDir, atlas.TablesDir, "atlas_answer-*.input.ref.json"))
	if err != nil || len(inputs) != 1 {
		t.Fatalf("answer inputs: %v %v", inputs, err)
	}
	input, err := readWindowPayload(inputs[0])
	if err != nil || !strings.Contains(string(input), route.Question) || strings.Contains(string(input), readerGuess) {
		t.Fatalf("the answer must judge the original question without adopting the reader's extra task: %s / %v", input, err)
	}
	calls := provider.calls
	opts.OwnerRunDir = t.TempDir()
	_, warm := readQuestionResult(t, opts)
	if provider.calls != calls || warm.Answer.Parts[0].Source != atlas.SourceCache {
		t.Fatal("answer did not reuse its exact cached input")
	}
	opts.OwnerRunDir, opts.Prompt = t.TempDir(), "Revise only the answer wording."
	result, _ = readQuestionResult(t, opts)
	if provider.calls != calls+1 {
		t.Fatal("answer-only edit reran retrieval or route selection")
	}
	for _, use := range result.Uses {
		if use.Stage != lines.StageAnswer && use.Live != 0 {
			t.Fatalf("unrelated live stage: %+v", use)
		}
	}
}

func TestAnswerKeepsOriginalEvidenceAndLabelledModelHypotheses(t *testing.T) {
	evidence := map[string]any{"context": map[string]any{"file_model_hypothesis": "Earlier speculation", "file_author_doc": "Author contract"},
		"evidence": []map[string]any{{"signature": "run(code)", "prior_model_hypothesis": "Suggested effect"}}}
	route := &atlas.QuestionRoute{Stops: []atlas.QuestionStop{{Path: "run.go", Line: 10, Evidence: evidence, Why: "Earlier route guess"}}}
	pools, err := planRoutePools(lines.Answer(), nil, route, uniqueRouteAnchors(route.Stops))
	if err != nil {
		t.Fatal(err)
	}
	input, err := table.Request(lines.Answer(), table.Window{Stage: lines.StageAnswer, Rows: []table.Row{pools[0].row}})
	if err != nil {
		t.Fatal(err)
	}
	for _, retained := range []string{"file_model_hypothesis", "Earlier speculation", "prior_model_hypothesis", "Suggested effect", "file_author_doc", "Author contract", "run(code)", "prior_model_suggestions", "Earlier route guess"} {
		if !strings.Contains(string(input), retained) {
			t.Fatalf("answer lost evidence or its attribution: %s", retained)
		}
	}
}

func TestAnswerDoesNotTurnMissingEvidenceIntoInapplicability(t *testing.T) {
	opts, provider := questionFixture(t)
	opts.Through = lines.StageAnswer
	provider.answerFor = func(map[string]any) table.Answer {
		return table.Answer{"state": "answered", "answer": "Unsupported assertion.", "sources": "c999", "remaining": "none"}
	}
	result, route := readQuestionResult(t, opts)
	if len(result.Rejected) != 1 || route.Answer.State != "unavailable" || route.Answer.Parts[0].Text != "" {
		t.Fatal("unsupported source was accepted")
	}
	// Positive citations alone cannot establish non-applicability when a
	// retrieval window was refused. Preserve that failure as unknown.
	opts.OwnerRunDir = t.TempDir()
	opts.Executor.Enabled = false
	// A provider with room for the original complete request must partition
	// the expanded evidence by its actual prepared byte envelope. One refused
	// partition must leave accepted neighbours, not a wholly empty route.
	provider.maxQuestionBytes = len(provider.questionRequests[0])
	provider.questionRequests = nil
	for i := 0; i < 24; i++ {
		path := fmt.Sprintf("pkg/b/extra-%02d.go", i)
		opts.Graph.Places = append(opts.Graph.Places, atlas.Place{ID: atlas.FileID(path), Kind: atlas.PlaceFile, Path: path,
			Parent: atlas.DirectoryID("pkg/b"), TargetIDs: []string{"t1"},
			File: &atlas.FileFacts{Decls: []atlas.Decl{{Name: "Extra", Kind: "function", LineNo: 1}}}})
		provider.questionFor[path] = table.Answer{"relevance": "direct", "anchors": "a1", "why": "Inspect the declaration."}
	}
	atlas.SortPlaces(opts.Graph.Places)
	provider.questionFor["pkg/a/y.go"]["anchors"] = "a999"
	provider.answerFor = func(map[string]any) table.Answer {
		return table.Answer{"state": "not_applicable", "answer": "The premise does not apply.", "sources": "c1", "remaining": "none"}
	}
	result, route = readQuestionResult(t, opts)
	unresolved := 0
	for _, raw := range provider.questionRequests {
		if len(raw) > provider.maxQuestionBytes {
			t.Fatal("retrieval exceeded the provider's prepared byte envelope")
		}
		var request questionBatchRequest
		if err := json.Unmarshal(raw, &request); err != nil {
			t.Fatal(err)
		}
		for _, row := range request.Evidence {
			if row["path"] == "pkg/a/y.go" {
				unresolved += len(request.Evidence)
			}
		}
	}
	if len(provider.questionRequests) < 2 || unresolved == 0 || route.Coverage.InspectedChunks == 0 || route.Coverage.InspectedChunks+unresolved != 28 || route.Answer.State != "unavailable" || route.Coverage.UnresolvedChunks != unresolved || len(result.Rejected) != 2 {
		t.Fatalf("partial scan: state=%s unresolved=%d rejected=%+v", route.Answer.State, route.Coverage.UnresolvedChunks, result.Rejected)
	}
	// A legitimate empty retrieval is still unanswered, never inapplicable.
	opts.OwnerRunDir = t.TempDir()
	for path := range provider.questionFor {
		provider.questionFor[path] = table.Answer{"relevance": "none", "anchors": "none", "why": "No useful source here."}
	}
	_, route = readQuestionResult(t, opts)
	if route.Answer.State != "unanswered" || len(route.Answer.Parts) != 0 {
		t.Fatalf("empty evidence state: %+v", route.Answer)
	}
}

func TestAnswerKeepsInternalReferencesOutOfExplanations(t *testing.T) {
	for _, field := range []string{"answer", "basis", "remaining"} {
		t.Run(field, func(t *testing.T) {
			opts, provider := questionFixture(t)
			opts.Through = lines.StageAnswer
			provider.answerFor = func(map[string]any) table.Answer {
				return table.Answer{field: "See c1 to c2 for this."}
			}
			result, route := readQuestionResult(t, opts)
			if len(result.Rejected) != 1 || route.Answer.State != "unavailable" {
				t.Fatal("request-local source numbers escaped into user-facing prose")
			}
		})
	}
}

func TestQuestionUsesObservationSourceWithoutInventingOutputFile(t *testing.T) {
	opts, provider := questionFixture(t)
	entity := func(id, path, status string, members []string) atlas.Place {
		return atlas.Place{ID: id, Kind: atlas.PlaceEntity, Path: path, Entity: &atlas.EntityFacts{Name: path, Extractor: "company", Status: status, Files: members}}
	}
	opts.Graph.Places = append(opts.Graph.Places,
		entity("entity:generator", "sqlc.yaml", "present", []string{"sqlc.yaml"}),
		entity("entity:absent", "missing-output", "not_in_corpus", []string{}),
	)
	opts.Graph.Edges = append(opts.Graph.Edges, atlas.Edge{From: "entity:generator", To: "entity:absent", Kind: "observation", Count: 1,
		Evidence: &atlas.EdgeEvidence{Extractor: "company", Label: "configured output", Path: "sqlc.yaml", LineNo: 8}})
	atlas.SortPlaces(opts.Graph.Places)
	for _, path := range []string{"sqlc.yaml", "missing-output"} {
		provider.questionFor[path] = table.Answer{"relevance": "direct", "anchors": "a1", "why": "Inspect the configured output."}
	}
	_, route := readQuestionResult(t, opts)
	if route.Coverage.Files != 4 || route.Coverage.Entities != 2 {
		t.Fatalf("coverage: %+v", route.Coverage)
	}
	for _, stop := range route.Stops {
		if stop.PlaceID == "entity:absent" && (stop.Path != "sqlc.yaml" || stop.Line != 8 || stop.Kind != "observation") {
			t.Fatalf("invented output anchor: %+v", stop)
		}
	}
	var observed int
	for _, connection := range route.Connections {
		if connection.Kind == "observation" {
			observed++
			if connection.Evidence == nil || connection.Evidence.LineNo != 8 || len(connection.Witnesses) != 0 {
				t.Fatal("observation lost its origin")
			}
		}
	}
	if observed != 1 {
		t.Fatal("declared relationship disappeared")
	}
	input, err := LoadInput(filepath.Join(opts.OwnerRunDir, InputFilename))
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Graph.Edges) != len(opts.Graph.Edges) {
		t.Fatal("saved reading lost observations")
	}
	// It also survives the full reader without becoming a call/group arrow.
	opts.OwnerRunDir, opts.Through = t.TempDir(), ""
	result, _ := readQuestionResult(t, opts)
	if !result.Complete {
		t.Fatal("ordinary reader did not finish")
	}
}
