package reading

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/llm"
)

func learningReply() learningResponse {
	var result learningResponse
	for _, intent := range learningIntents() {
		result.Reviews = append(result.Reviews, learningReview{Intent: intent.ID, State: "unknown", Reason: "No relevant evidence in this context."})
	}
	result.Reviews[0] = learningReview{Intent: "purpose", State: "questions", Reason: "The project has meaningful state.", Questions: []learningProposal{
		{Question: "What does a lease control?", Why: "It determines the lifetime of stored data.", Sources: []string{"e1"}},
	}}
	result.Reviews[3] = learningReview{Intent: "data", State: "questions", Reason: "Two different concepts matter.", Questions: []learningProposal{
		{Question: "Which data does a lease control?", Why: "The concept controls data lifetime.", Sources: []string{"e1"}},
		{Question: "What does a revision identify?", Why: "The concept explains versioned state.", Sources: []string{"e1"}},
	}}
	return result
}

func TestOrdinaryLearnAnswersProposalsAndPreservesExplicitQuestions(t *testing.T) {
	opts, provider := questionFixture(t)
	opts.Through, opts.Learn = "", true
	explicit := opts.Questions[0]
	opts.Questions = append(opts.Questions, "An explicit question without a proposed topic?")
	provider.learningFor = func(pool learningRequest) learningResponse {
		reply := learningReply()
		for i := range reply.Reviews {
			reply.Reviews[i].State, reply.Reviews[i].Questions = "unknown", nil
		}
		reply.Reviews[0].State = "questions"
		reply.Reviews[0].Questions = []learningProposal{
			{Question: explicit, Why: "State is central.", Sources: []string{pool.Evidence[0].Ref}},
			{Question: "What should I change first?", Why: "A concrete way to begin.", Sources: []string{pool.Evidence[0].Ref}},
		}
		return reply
	}
	result, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.Learning == nil || len(result.Questions) != 3 {
		t.Fatalf("ordinary learning did not reach publication: %+v", result)
	}
	for i, route := range result.Questions {
		if route.Answer == nil || route.UserQuestion != (i < 2) || route.GraphSHA256 != result.Learning.GraphSHA256 {
			t.Fatalf("lost answer, user intent or common graph: %+v", route)
		}
	}
	if len(result.Questions[0].Origins) != 1 || len(result.Questions[1].Origins) != 0 || len(result.Questions[2].Origins) != 1 {
		t.Fatal("shared explicit question lost its origin or an explicit question was filtered")
	}
	raw, err := os.ReadFile(filepath.Join(opts.OwnerRunDir, atlas.QuestionFilename))
	var saved atlas.QuestionRoutes
	if err != nil || json.Unmarshal(raw, &saved) != nil || len(saved.Routes) != 3 || len(saved.Routes[0].Origins) != 1 || !saved.Routes[0].UserQuestion {
		t.Fatal("persistence lost question provenance")
	}
	opts.OwnerRunDir, opts.Questions, opts.Through = t.TempDir(), nil, stageLearn
	isolated, err := Read(t.Context(), opts)
	if err != nil || isolated.Complete || len(isolated.Questions) != 0 || isolated.Learning == nil || len(isolated.Learning.Questions) != 2 {
		t.Fatalf("learn stop also answered or lost its plan: %+v %v", isolated, err)
	}
}

func TestLearningUsesLaunchAndManifestObservationsWithoutModelDescriptions(t *testing.T) {
	entry := atlas.Place{ID: "fact:private-entry", Kind: atlas.PlaceSourceFact, Path: "backend/main.py", LineNo: 14, TargetIDs: []string{"backend"},
		SourceFact: &atlas.SourceFact{Kind: "entrypoint", Name: "main", Key: "main_guard", ObjectID: "private-module", Language: "python", Component: "backend", ComponentKind: "executable", Root: "backend"}}
	manifest := atlas.Place{ID: "fact:private-manifest", Kind: atlas.PlaceSourceFact, Path: "backend/Pipfile", LineNo: 15, TargetIDs: []string{"backend"},
		SourceFact: &atlas.SourceFact{Kind: "manifest", Key: "requires.python_version", Value: "3.12", Language: "python", Component: "backend", ComponentKind: "executable", Root: "backend"}}
	r := reader{opts: Options{Graph: atlas.Graph{Places: []atlas.Place{entry, manifest}}}}
	evidence := r.learningEvidence()
	if len(evidence) != 2 || evidence[0].Source.Kind != "entrypoint" || evidence[0].Source.SubjectID != entry.SourceFact.ObjectID || evidence[1].Source.Kind != "manifest" || evidence[1].Source.Line != 15 {
		t.Fatalf("launch sources required a key-symbol interpretation: %+v", evidence)
	}
	pools, err := learningPools(evidence, 0, learningPrompt)
	if err != nil {
		t.Fatal(err)
	}
	for _, pool := range pools {
		raw, err := json.Marshal(pool)
		if err != nil || strings.Contains(string(raw), "private-") {
			t.Fatalf("internal launch identities entered Learn: %v", err)
		}
	}
}

func TestLearningAllowsZeroOneManyWithOriginalReasons(t *testing.T) {
	pool := learningRequest{Evidence: []learningEvidence{{Ref: "e1"}}}
	result := learningReply()
	result.Reviews[0].Questions[0].Sources = []string{"invented", "e1", "e1"}
	result.Reviews = append(result.Reviews, learningReview{Intent: "unadvertised", State: "questions"})
	raw, _ := json.Marshal(result)
	got, err := decodeLearning(raw, pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Reviews) != len(learningIntents()) || len(got.Reviews[0].Questions) != 1 || len(got.Reviews[1].Questions) != 0 || len(got.Reviews[3].Questions) != 2 {
		t.Fatalf("zero/one/many lost: %+v", got)
	}
	if refs := got.Reviews[0].Questions[0].Sources; len(refs) != 1 || refs[0] != "e1" {
		t.Fatalf("refs: %v", refs)
	}
	for _, test := range []string{"missing-intent", "duplicate-intent", "no-source", "partial-inapplicable", "unsupported-inapplicable", "internal-ref"} {
		t.Run(test, func(t *testing.T) {
			bad := learningReply()
			context := pool
			switch test {
			case "missing-intent":
				bad.Reviews = bad.Reviews[1:]
			case "duplicate-intent":
				bad.Reviews = append(bad.Reviews, bad.Reviews[0])
			case "no-source":
				bad.Reviews[0].Questions[0].Sources = []string{"unknown"}
			case "partial-inapplicable":
				context.PartialContext = true
				bad.Reviews[1] = learningReview{Intent: "run", State: "not_applicable", Reason: "Not a program.", Sources: []string{"e1"}}
			case "unsupported-inapplicable":
				bad.Reviews[1] = learningReview{Intent: "run", State: "not_applicable", Reason: "Not found."}
			case "internal-ref":
				bad.Reviews[0].Questions[0].Why = "e1 proves this."
			}
			raw, _ := json.Marshal(bad)
			if _, err := decodeLearning(raw, context); err == nil {
				t.Fatal("unsupported decision accepted")
			}
		})
	}
}

func TestExplicitQuestionBypassesLearningAudienceSelection(t *testing.T) {
	opts, provider := questionFixture(t)
	opts.Through, opts.Learn = "", true
	provider.learningSelectNone = true
	provider.learningFor = func(pool learningRequest) learningResponse {
		reply := learningReply()
		for i := range reply.Reviews {
			for j := range reply.Reviews[i].Questions {
				reply.Reviews[i].Questions[j].Question = opts.Questions[0]
				reply.Reviews[i].Questions[j].Sources = []string{pool.Evidence[0].Ref}
			}
		}
		return reply
	}
	result, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Learning.Questions) != 0 || len(result.Learning.Selections) != 2 || len(result.Questions) != 1 || !result.Questions[0].UserQuestion || result.Questions[0].Answer == nil {
		t.Fatalf("audience filtering suppressed an explicit question: %+v", result)
	}
}

func TestLearningPartitionsEveryContextWithoutLeakingIdentities(t *testing.T) {
	var evidence []learningEvidence
	for i := 0; i < 137; i++ {
		evidence = append(evidence, learningEvidence{Context: map[string]any{"signature": strings.Repeat("x", 300), "ordinal": i}, Source: atlas.QuestionStop{SubjectID: "private-identity", Path: fmt.Sprintf("file%d.go", i), Line: 1}})
	}
	pools, err := learningPools(evidence, len(learningPrompt)+4000, learningPrompt)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	if len(pools) < 2 {
		t.Fatal("expected multiple windows")
	}
	for _, pool := range pools {
		if !pool.PartialContext {
			t.Fatal("partial context called complete")
		}
		raw, _ := json.Marshal(pool)
		if strings.Contains(string(raw), "private-identity") {
			t.Fatal("internal subject sent")
		}
		for _, e := range pool.Evidence {
			i := e.Context["ordinal"].(int)
			if seen[i] {
				t.Fatal("duplicate item")
			}
			seen[i] = true
			if e.Source.Path != fmt.Sprintf("file%d.go", i) {
				t.Fatal("source identity lost")
			}
		}
	}
	if len(seen) != len(evidence) {
		t.Fatal("context tail omitted")
	}
}

type learningProvider struct {
	calls        int
	requests     [][]byte
	refuseMerge  bool
	specialist   bool
	menuFirst    bool
	dropMenuLast bool
}

type learningMenuRequest struct {
	Context struct {
		Candidates []struct {
			Ref      string `json:"ref"`
			Question string `json:"question"`
		} `json:"candidate_questions"`
	} `json:"context"`
	Rows []struct {
		Key     string   `json:"key"`
		Intent  string   `json:"learning_intent"`
		Goal    string   `json:"learning_goal"`
		Options []string `json:"candidate_options"`
	} `json:"rows"`
}

func (*learningProvider) State() []byte { return []byte(`{"provider":"local-learning-test"}`) }
func (*learningProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	raw, err := json.Marshal(prompt)
	if err != nil {
		return llm.Prepared{}, err
	}
	return llm.NewPrepared(raw)
}
func (p *learningProvider) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	p.calls++
	p.requests = append(p.requests, prepared.Bytes())
	var prompt llm.Prompt
	_ = json.Unmarshal(prepared.Bytes(), &prompt)
	var raw []byte
	if strings.Contains(prompt.System, "Choose a complementary introduction") {
		var request learningMenuRequest
		if err := json.Unmarshal([]byte(prompt.User), &request); err != nil {
			return llm.Completion{}, err
		}
		var rows []map[string]string
		for _, row := range request.Rows {
			var selected []string
			for _, candidate := range request.Context.Candidates {
				if !slices.Contains(row.Options, candidate.Ref) || p.specialist && strings.Contains(candidate.Question, "revision") || p.menuFirst && len(selected) > 0 {
					continue
				}
				selected = append(selected, candidate.Ref)
			}
			choice := strings.Join(selected, " ")
			if choice == "" {
				choice = "none"
			}
			rows = append(rows, map[string]string{"key": row.Key, "questions": choice, "reason": "These questions provide a complementary introduction to the topic."})
		}
		if p.dropMenuLast && len(rows) > 0 {
			rows = rows[:len(rows)-1]
		}
		raw, _ = json.Marshal(map[string]any{"rows": rows})
	} else if strings.Contains(prompt.System, "Consolidate learning") {
		var request struct {
			Rows []map[string]any `json:"rows"`
		}
		if err := json.Unmarshal([]byte(prompt.User), &request); err != nil {
			return llm.Completion{}, err
		}
		var rows []map[string]string
		leaseRef := ""
		for _, row := range request.Rows {
			if strings.Contains(row["question"].(string), "lease") {
				leaseRef = row["own_ref"].(string)
				break
			}
		}
		for _, row := range request.Rows {
			ref := row["own_ref"].(string)
			if p.refuseMerge {
				ref = "unadvertised"
			}
			if strings.Contains(row["question"].(string), "lease") {
				ref = leaseRef
			}
			rows = append(rows, map[string]string{"key": row["key"].(string), "representative": ref})
		}
		raw, _ = json.Marshal(map[string]any{"rows": rows})
	} else {
		raw, _ = json.Marshal(learningReply())
	}
	return llm.Completion{Response: raw, FinishReason: "stop", ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, nil
}

func TestLearningSelectionPreservesUnselectedQuestionsAndSources(t *testing.T) {
	r := isolatedLearningReader(t, t.TempDir(), &learningProvider{specialist: true})
	if err := r.readLearning(t.Context()); err != nil {
		t.Fatal(err)
	}
	if r.learning.State != "ready" || len(r.learning.Questions) != 1 || len(r.learning.Selections) != 3 {
		t.Fatalf("selection lost its decisions or did not precede consolidation: %+v", r.learning)
	}
	for _, selection := range r.learning.Selections {
		if selection.Reason == "" || selection.Source != atlas.SourceModel || len(selection.Origins) == 0 || len(selection.Origins[0].Sources) == 0 {
			t.Fatalf("uncheckable selection: %+v", selection)
		}
	}
	if r.learning.Selections[2].Audience != "not_selected" {
		t.Fatal("unselected question was declared inapplicable or lost")
	}
}

func TestLearningMenuReducesCompletePoolsAndReusesTheirCache(t *testing.T) {
	for _, reduce := range []bool{true, false} {
		t.Run(fmt.Sprintf("reduce=%v", reduce), func(t *testing.T) {
			cache := t.TempDir()
			provider := &learningProvider{menuFirst: reduce}
			var questions []atlas.LearningQuestion
			for i := 0; i < 9; i++ {
				q := fmt.Sprintf("Topic %d: %s?", i, strings.Repeat("context ", 70))
				questions = append(questions, atlas.LearningQuestion{Question: q, Origins: []atlas.LearningOrigin{{
					Intent: "purpose", Title: "Purpose", Question: q, Why: fmt.Sprintf("Original reason %d", i),
					Sources: []atlas.QuestionStop{{SubjectID: fmt.Sprintf("private-subject-%d", i), Path: fmt.Sprintf("file%d.go", i), Line: i + 1}},
				}}})
			}
			run := func() *reader {
				r := isolatedLearningReader(t, cache, provider)
				r.opts.InputBytes = 8000
				r.learning = &atlas.LearningPlan{State: "ready", Questions: append([]atlas.LearningQuestion{}, questions...)}
				if err := r.selectLearning(t.Context()); err != nil {
					t.Fatal(err)
				}
				return r
			}
			r := run()
			want := len(questions)
			if reduce {
				want = 1
			}
			if r.learning.State != "ready" || len(r.learning.Questions) != want || len(r.learning.Selections) != len(questions) || provider.calls < 2 {
				t.Fatalf("lost a pool, a decision or fixed-point termination: %+v; calls=%d", r.learning, provider.calls)
			}
			if !reflect.DeepEqual(r.learning.Questions[0], questions[0]) {
				t.Fatal("selection rewrote the original question or lost its sources")
			}
			for i, selection := range r.learning.Selections {
				if !reflect.DeepEqual(selection.LearningQuestion, questions[i]) || selection.Reason == "" || selection.Source != atlas.SourceModel {
					t.Fatal("pool reduction lost an original decision or its evidence")
				}
				if !reduce && !selection.PartialContext {
					t.Fatal("independent fixed-point menus claimed a whole-menu comparison")
				}
			}
			if reduce && r.learning.Selections[0].PartialContext {
				t.Fatal("final comparison did not replace the earlier partial decision")
			}
			for _, raw := range provider.requests {
				var prompt llm.Prompt
				_ = json.Unmarshal(raw, &prompt)
				var request learningMenuRequest
				if err := json.Unmarshal([]byte(prompt.User), &request); err != nil {
					t.Fatal(err)
				}
				if len(request.Rows) != 1 || len(request.Context.Candidates) == 0 || strings.Contains(string(raw), "private-subject-") {
					t.Fatal("menu was not one decision over its catalogue or leaked local identity")
				}
				for i, candidate := range request.Context.Candidates {
					if candidate.Ref != fmt.Sprintf("q%d", i+1) || !slices.ContainsFunc(questions, func(q atlas.LearningQuestion) bool { return q.Question == candidate.Question }) {
						t.Fatal("menu catalogue has no matching original candidate")
					}
				}
			}
			calls := provider.calls
			warm := run()
			for i := range warm.learning.Selections {
				if warm.learning.Selections[i].Source != atlas.SourceCache {
					t.Fatal("warm selection did not report its cached origin")
				}
				warm.learning.Selections[i].Source = r.learning.Selections[i].Source
			}
			if provider.calls != calls || !reflect.DeepEqual(warm.learning, r.learning) {
				t.Fatal("same menu comparison failed to reuse its cache or changed the result")
			}
		})
	}
}

func TestLearningMenuComparesGoalsTogetherAndRequiresEveryGoalDecision(t *testing.T) {
	for _, incomplete := range []bool{false, true} {
		t.Run(fmt.Sprintf("incomplete=%v", incomplete), func(t *testing.T) {
			provider := &learningProvider{dropMenuLast: incomplete}
			r := isolatedLearningReader(t, t.TempDir(), provider)
			questions := []atlas.LearningQuestion{
				{Question: "Where do I begin?", Origins: []atlas.LearningOrigin{{Intent: "purpose", Title: "Purpose"}}},
				{Question: "What is a version?", Origins: []atlas.LearningOrigin{{Intent: "data", Title: "Data"}}},
				{Question: "How does state serve the project?", Origins: []atlas.LearningOrigin{{Intent: "purpose", Title: "Purpose"}, {Intent: "data", Title: "Data"}}},
			}
			r.learning = &atlas.LearningPlan{State: "ready", Questions: questions}
			if err := r.selectLearning(t.Context()); err != nil {
				t.Fatal(err)
			}
			if provider.calls != 1 || len(r.learning.Selections) != 4 {
				t.Fatal("topic menus were split or a goal's decision was lost")
			}
			var prompt llm.Prompt
			_ = json.Unmarshal(provider.requests[0], &prompt)
			var request learningMenuRequest
			if err := json.Unmarshal([]byte(prompt.User), &request); err != nil {
				t.Fatal(err)
			}
			if len(request.Context.Candidates) != 3 || len(request.Rows) != 2 || !slices.Equal(request.Rows[0].Options, []string{"q1", "q3"}) || !slices.Equal(request.Rows[1].Options, []string{"q2", "q3"}) {
				t.Fatal("goals did not share one catalogue with their own closed candidate sets")
			}
			for _, row := range request.Rows {
				if !slices.ContainsFunc(learningIntents(), func(intent learningIntent) bool {
					return row.Intent == intent.Title && row.Goal != "" && row.Goal == intent.Goal
				}) {
					t.Fatal("selection lost the original curated learning goal")
				}
			}
			if incomplete {
				if r.learning.State != "partial" || len(r.learning.Questions) != 0 || slices.ContainsFunc(r.learning.Selections, func(s atlas.LearningSelection) bool { return s.Audience != "unavailable" }) {
					t.Fatal("missing goal decision was silently repaired or accepted")
				}
			} else if r.learning.State != "ready" || !reflect.DeepEqual(r.learning.Questions, questions) {
				t.Fatal("complete joint menu changed question identity or duplicated a shared question")
			}
		})
	}
}

func TestLearnPromptKeepsAudienceAndMergeContracts(t *testing.T) {
	provider := &learningProvider{}
	r := isolatedLearningReader(t, t.TempDir(), provider)
	r.opts.Through, r.opts.Prompt = stageLearn, learningPrompt+"\nAdditional guidance for proposals.\n"
	if err := r.readLearning(t.Context()); err != nil {
		t.Fatal(err)
	}
	if r.learning.State != "ready" || len(r.learning.Questions) != 2 {
		t.Fatal("proposal customization replaced a later subtable's contract")
	}
	seenAudience, seenMerge := false, false
	for _, raw := range provider.requests {
		var prompt llm.Prompt
		if err := json.Unmarshal(raw, &prompt); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(prompt.System, "prose in English.") {
			t.Fatal("learning request lost the shared English response policy")
		}
		seenAudience = seenAudience || strings.HasSuffix(prompt.System, "\n\n"+learningSelectPrompt)
		seenMerge = seenMerge || strings.HasSuffix(prompt.System, "\n\n"+learningMergePrompt)
	}
	if !seenAudience || !seenMerge {
		t.Fatal("missing independent subtable prompts")
	}
}

func isolatedLearningReader(t *testing.T, cache string, p llm.Provider) *reader {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, atlas.TablesDir), 0700); err != nil {
		t.Fatal(err)
	}
	graph := testGraph(t)
	r := &reader{opts: Options{Graph: graph, OwnerRunDir: dir, Provider: p, Executor: llm.Executor{Enabled: true, RootDir: cache}, Stage: func(string, ...string) {}, State: func(string, string, ...string) {}},
		places: map[string]atlas.Place{}, lines: map[string]cell{}, uses: map[string]*atlas.StageUse{}, started: map[string]time.Time{}, knowledge: map[string]*Knowledge{}, knowledgeSubjects: map[string]*Knowledge{}}
	for _, place := range graph.Places {
		r.places[place.ID] = place
		if place.File != nil {
			r.lines[place.ID] = cell{value: "Manages persistent data.", source: atlas.SourceModel}
		}
	}
	return r
}

func TestLearningConsolidatesAnswersAndRetainsAllIntentSources(t *testing.T) {
	cache := t.TempDir()
	provider := &learningProvider{}
	r := isolatedLearningReader(t, cache, provider)
	if err := r.readLearning(t.Context()); err != nil {
		t.Fatal(err)
	}
	if r.learning.State != "ready" || len(r.learning.Questions) != 2 {
		t.Fatalf("plan: %+v", r.learning)
	}
	lease := r.learning.Questions[0]
	if len(lease.Origins) != 2 || lease.Origins[0].Intent != "purpose" || lease.Origins[1].Intent != "data" {
		t.Fatalf("origins lost: %+v", lease)
	}
	for _, q := range r.learning.Questions {
		for _, origin := range q.Origins {
			if origin.Why == "" || origin.Question == "" || len(origin.Sources) != 1 || origin.Sources[0].Path == "" {
				t.Fatal("uncheckable origin")
			}
		}
	}
	calls := provider.calls
	warm := isolatedLearningReader(t, cache, provider)
	if err := warm.readLearning(t.Context()); err != nil {
		t.Fatal(err)
	}
	if provider.calls != calls || len(warm.learning.Questions) != 2 {
		t.Fatal("same preparation did not reuse cache")
	}
	if _, err := os.Stat(filepath.Join(warm.opts.OwnerRunDir, "learning-plan.json")); err != nil {
		t.Fatal(err)
	}
	for _, raw := range provider.requests {
		if strings.Contains(string(raw), "file:") || strings.Contains(string(raw), "sym:") {
			t.Fatal("canonical identity in prompt")
		}
	}
}

func TestLearningMergeReviewsEveryPairAcrossContextWindows(t *testing.T) {
	provider := &learningProvider{}
	r := isolatedLearningReader(t, t.TempDir(), provider)
	r.opts.InputBytes = 8000
	r.learning = &atlas.LearningPlan{State: "ready"}
	for i := 0; i < 9; i++ {
		r.learning.Questions = append(r.learning.Questions, atlas.LearningQuestion{Question: fmt.Sprintf("Topic %d: %s?", i, strings.Repeat("context ", 70)), Origins: []atlas.LearningOrigin{{Intent: fmt.Sprint(i)}}})
	}
	if err := r.mergeLearning(t.Context()); err != nil {
		t.Fatal(err)
	}
	if r.learning.State != "ready" || len(r.learning.Questions) != 9 {
		t.Fatal("partitioning discarded questions")
	}
	pairs := map[string]bool{}
	for _, raw := range provider.requests {
		var prompt llm.Prompt
		_ = json.Unmarshal(raw, &prompt)
		var request struct {
			Context struct {
				Questions []struct {
					Question string `json:"question"`
				} `json:"questions"`
			} `json:"context"`
		}
		if err := json.Unmarshal([]byte(prompt.User), &request); err != nil {
			t.Fatal(err)
		}
		for _, a := range request.Context.Questions {
			for _, b := range request.Context.Questions {
				if a.Question < b.Question {
					pairs[a.Question+"|"+b.Question] = true
				}
			}
		}
	}
	if len(pairs) != 9*8/2 {
		t.Fatalf("only %d pairs compared", len(pairs))
	}
}

func TestLearningDoesNotPublishInventedMergeAssignments(t *testing.T) {
	r := isolatedLearningReader(t, t.TempDir(), &learningProvider{refuseMerge: true})
	if err := r.readLearning(t.Context()); err != nil {
		t.Fatal(err)
	}
	if r.learning.State != "unavailable" || len(r.learning.Questions) != 0 || len(r.rejected) == 0 {
		t.Fatal("failed consolidation promoted into a completed plan")
	}
	dry := isolatedLearningReader(t, t.TempDir(), nil)
	dry.dry = true
	if err := dry.readLearning(t.Context()); err != nil {
		t.Fatal(err)
	}
	if dry.learning.State != "unavailable" || len(dry.learning.Reviews) != len(learningIntents()) {
		t.Fatal("missing provider interpreted as inapplicability")
	}
}
