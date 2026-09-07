package reading

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

const stageLearn = "atlas_learn"

var learningInternalRef = regexp.MustCompile(`\b[eq][0-9]+\b`)

//go:embed prompts/learning.md
var learningPrompt string

//go:embed prompts/learning-merge.md
var learningMergePrompt string

//go:embed prompts/learning-select.md
var learningSelectPrompt string

type learningIntent struct{ ID, Title, Goal string }

func learningIntents() []learningIntent {
	var result []learningIntent
	for _, line := range strings.Split(learningPrompt, "\n") {
		if strings.HasPrefix(line, "## ") {
			id, title, ok := strings.Cut(strings.TrimPrefix(line, "## "), " | ")
			if ok {
				result = append(result, learningIntent{ID: id, Title: title})
			}
		} else if len(result) > 0 && strings.TrimSpace(line) != "" {
			intent := &result[len(result)-1]
			intent.Goal = strings.TrimSpace(intent.Goal + " " + strings.TrimSpace(line))
		}
	}
	return result
}

type learningEvidence struct {
	Ref     string             `json:"ref"`
	Context map[string]any     `json:"context"`
	Source  atlas.QuestionStop `json:"-"`
}
type learningRequest struct {
	PartialContext bool               `json:"partial_context"`
	Evidence       []learningEvidence `json:"evidence"`
}
type learningProposal struct {
	Question string   `json:"question"`
	Why      string   `json:"why"`
	Sources  []string `json:"sources"`
}
type learningReview struct {
	Intent    string             `json:"intent"`
	State     string             `json:"state"`
	Reason    string             `json:"reason"`
	Sources   []string           `json:"sources"`
	Questions []learningProposal `json:"questions"`
}
type learningResponse struct {
	Reviews []learningReview `json:"reviews"`
}

// Reuse the same original source units as question retrieval. All documents,
// file purposes, interpreted key declarations and boundaries participate;
// incidental local declarations need not become learning topics.
func (r *reader) learningEvidence() []learningEvidence {
	var result []learningEvidence
	seenFiles := map[string]bool{}
	for _, chunk := range r.questionRows() {
		area := ""
		if box := r.boxes[r.boxOf[chunk.Place.ID]]; box != nil {
			area = box.title + ": " + box.line
		}
		var components []string
		for _, meta := range r.opts.Targets {
			if slices.Contains(chunk.Place.TargetIDs, meta.ID) {
				components = append(components, meta.Name)
			}
		}
		appendSource := func(stop atlas.QuestionStop) {
			result = append(result, learningEvidence{Source: stop, Context: map[string]any{
				"components": components, "area_model_hypothesis": area, "original": stop.Evidence,
			}})
		}
		if line, ok := r.Line(chunk.Place.ID); ok && chunk.Place.File != nil && !seenFiles[chunk.Place.ID] {
			seenFiles[chunk.Place.ID] = true
			appendSource(atlas.QuestionStop{PlaceID: chunk.Place.ID, SubjectID: chunk.Place.ID,
				Path: chunk.Place.Path, Line: 1, Kind: "file", Name: chunk.Place.Path, TargetIDs: chunk.Place.TargetIDs,
				Evidence: map[string]any{"evidence": []map[string]any{{"anchor_path": chunk.Place.Path, "anchor_line": 1,
					"author_doc": chunk.Place.File.Doc, "prior_model_hypothesis": line}}}})
		}
		refs := make([]string, 0, len(chunk.Anchors))
		for ref := range chunk.Anchors {
			refs = append(refs, ref)
		}
		sort.Strings(refs)
		for _, ref := range refs {
			anchor := chunk.Anchors[ref]
			known := r.knowledgeSubjects[anchor.SubjectID]
			if anchor.Kind != "documentation" && anchor.Kind != "boundary" && chunk.Place.Entity == nil && chunk.Place.SourceFact == nil && (known == nil || known.Cells["key_symbol"] != "yes") {
				continue
			}
			stop := atlas.QuestionStop{PlaceID: chunk.Place.ID, SubjectID: anchor.SubjectID, Path: anchor.Path,
				Line: anchor.Line, Column: anchor.Column, Name: anchor.Name, Kind: anchor.Kind,
				TargetIDs: chunk.Place.TargetIDs, Evidence: lines.AnchorEvidence(chunk, ref)}
			if known != nil {
				stop.KnowledgeIDs = []string{known.ID}
			}
			appendSource(stop)
		}
	}
	return result
}

func learningPools(evidence []learningEvidence, budget int, prompt string) ([]learningRequest, error) {
	if budget == 0 {
		budget = 64 * 1024
	}
	var pools []learningRequest
	var split func([]learningEvidence) error
	split = func(items []learningEvidence) error {
		pool := learningRequest{PartialContext: true, Evidence: append([]learningEvidence{}, items...)}
		for i := range pool.Evidence {
			pool.Evidence[i].Ref = fmt.Sprintf("e%d", i+1)
		}
		raw, err := json.Marshal(pool)
		if err != nil {
			return err
		}
		if len(raw)+len(prompt) > budget {
			if len(items) < 2 {
				return fmt.Errorf("learn: one original context item exceeds the input budget")
			}
			mid := len(items) / 2
			if err := split(items[:mid]); err != nil {
				return err
			}
			return split(items[mid:])
		}
		pools = append(pools, pool)
		return nil
	}
	if err := split(evidence); err != nil {
		return nil, err
	}
	if len(pools) == 1 {
		pools[0].PartialContext = false
	}
	return pools, nil
}

func decodeLearning(raw []byte, pool learningRequest) (learningResponse, error) {
	var result learningResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, err
	}
	intents := map[string]bool{}
	for _, intent := range learningIntents() {
		intents[intent.ID] = true
	}
	refs := map[string]bool{}
	for _, item := range pool.Evidence {
		refs[item.Ref] = true
	}
	filter := func(values []string) []string {
		var kept []string
		for _, ref := range values {
			if refs[ref] && !slices.Contains(kept, ref) {
				kept = append(kept, ref)
			}
		}
		return kept
	}
	seen := map[string]bool{}
	var reviews []learningReview
	for _, review := range result.Reviews {
		if !intents[review.Intent] {
			continue
		}
		if seen[review.Intent] {
			return result, fmt.Errorf("learn: duplicate intent review")
		}
		seen[review.Intent] = true
		review.Sources = filter(review.Sources)
		if strings.TrimSpace(review.Reason) == "" || learningInternalRef.MatchString(review.Reason) {
			return result, fmt.Errorf("learn: a review needs a reason")
		}
		switch review.State {
		case "questions":
			if len(review.Questions) == 0 {
				return result, fmt.Errorf("learn: questions review is empty")
			}
		case "not_applicable":
			if pool.PartialContext || len(review.Sources) == 0 {
				return result, fmt.Errorf("learn: inapplicability needs complete context and positive evidence")
			}
			fallthrough
		case "unknown":
			if len(review.Questions) != 0 {
				return result, fmt.Errorf("learn: non-question review contains questions")
			}
		default:
			return result, fmt.Errorf("learn: unknown review state")
		}
		for i := range review.Questions {
			q := &review.Questions[i]
			q.Question, q.Why = strings.TrimSpace(q.Question), strings.TrimSpace(q.Why)
			q.Sources = filter(q.Sources)
			if q.Question == "" || q.Why == "" || len(q.Sources) == 0 || learningInternalRef.MatchString(q.Question) || learningInternalRef.MatchString(q.Why) {
				return result, fmt.Errorf("learn: a proposed question needs wording, reason and original sources")
			}
		}
		reviews = append(reviews, review)
	}
	if len(seen) != len(intents) {
		return result, fmt.Errorf("learn: incomplete intent reviews")
	}
	result.Reviews = reviews
	return result, nil
}

func (r *reader) readLearning(ctx context.Context) error {
	r.started[stageLearn] = time.Now()
	prompt := learningPrompt
	if r.opts.Through == stageLearn && r.opts.Prompt != "" {
		prompt = r.opts.Prompt
	}
	budget := 0
	if r.opts.Through == "" || r.opts.Through == stageLearn {
		budget = r.opts.InputBytes
	}
	pools, err := learningPools(r.learningEvidence(), budget, prompt)
	if err != nil {
		return err
	}
	r.learning = &atlas.LearningPlan{Version: 1, GraphSHA256: r.opts.Graph.SHA256, State: "ready"}
	r.opts.Stage(stageLearn, fmt.Sprintf("adapting %d learning intents in %d context windows", len(learningIntents()), len(pools)))
	calls := make([]llm.Call[learningResponse], len(pools))
	for i, pool := range pools {
		raw, err := json.Marshal(pool)
		if err != nil {
			return err
		}
		calls[i] = llm.Call[learningResponse]{State: []byte("repomap.atlas.learn.v1"),
			Prompt:         llm.Prompt{System: prompt, User: string(raw), ResponseFormatJSON: true},
			Limits:         llm.Limits{MaxRequestBytes: llm.SemanticRecordByteLimit, MaxResponseBytes: llm.ProviderResponseByteLimit, MaxOutputTokens: llm.DefaultMaxOutputTokens},
			DecodeValidate: func(raw []byte) (learningResponse, error) { return decodeLearning(raw, pool) }}
	}
	// The ordinary shared executor owns cache, retries, concurrency and journal.
	return r.executeLearning(ctx, pools, calls, prompt)
}

func (r *reader) executeLearning(ctx context.Context, pools []learningRequest, calls []llm.Call[learningResponse], prompt string) error {
	results := make([]llm.EachResult[learningResponse], len(calls))
	if !r.dry {
		results = llm.ExecuteJSONEach(ctx, debugdump.BindStage(r.opts.Executor, stageLearn), r.opts.Provider, calls)
	}
	use := r.use(stageLearn)
	use.Windows += len(calls)
	use.Rows += len(calls) * len(learningIntents())
	titles := map[string]string{}
	for _, intent := range learningIntents() {
		titles[intent.ID] = intent.Title
	}
	for i, result := range results {
		window := table.Window{Stage: stageLearn, Index: i + 1}
		for _, item := range []struct {
			name string
			data []byte
		}{
			{"prompt.md", []byte(prompt)}, {"input.json", []byte(calls[i].Prompt.User)},
			{"request.json", result.Outcome.Request}, {"response.json", result.Outcome.Response},
		} {
			if len(item.data) > 0 {
				if err := r.writeWindowFile(window, item.name, item.data); err != nil {
					return err
				}
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		source := atlas.SourceModel
		if result.Outcome.Cached {
			source = atlas.SourceCache
			use.Cached++
		} else if !r.dry {
			use.Live++
		}
		if result.Err != nil || r.dry {
			r.learning.State = "partial"
			use.Given += len(learningIntents())
			reason := "No model provider was available."
			if result.Err != nil {
				use.Rejected++
				reason = result.Err.Error()
				r.rejected = append(r.rejected, modeldiag.Row{Stage: stageLearn, Kind: "window_rejected", Count: len(learningIntents()), Reason: reason,
					ResponseRef: filepath.ToSlash(filepath.Join(atlas.TablesDir, r.windowFileName(window, "response.ref.json")))})
			}
			for _, intent := range learningIntents() {
				r.learning.Reviews = append(r.learning.Reviews, atlas.LearningReview{
					Intent: intent.ID, Title: intent.Title, State: "unavailable", Reason: reason, Source: atlas.SourceGiven, Window: i + 1, PartialContext: pools[i].PartialContext})
			}
			continue
		}
		restore := func(refs []string) []atlas.QuestionStop {
			var sources []atlas.QuestionStop
			for _, ref := range refs {
				for _, item := range pools[i].Evidence {
					if item.Ref == ref {
						stop := item.Source
						stop.Source = source
						sources = append(sources, stop)
						break
					}
				}
			}
			return sources
		}
		for _, review := range result.Outcome.Value.Reviews {
			r.learning.Reviews = append(r.learning.Reviews, atlas.LearningReview{Intent: review.Intent, Title: titles[review.Intent],
				Window: i + 1, PartialContext: pools[i].PartialContext, State: review.State, Reason: review.Reason, Source: source, Sources: restore(review.Sources)})
			for _, q := range review.Questions {
				origin := atlas.LearningOrigin{Intent: review.Intent, Title: titles[review.Intent], Question: q.Question, Why: q.Why, Source: source, Sources: restore(q.Sources)}
				index := slices.IndexFunc(r.learning.Questions, func(old atlas.LearningQuestion) bool { return old.Question == q.Question })
				if index < 0 {
					r.learning.Questions = append(r.learning.Questions, atlas.LearningQuestion{Question: q.Question, Origins: []atlas.LearningOrigin{origin}})
				} else {
					r.learning.Questions[index].Origins = append(r.learning.Questions[index].Origins, origin)
				}
			}
		}
		raw, err := json.MarshalIndent(result.Outcome.Value, "", "  ")
		if err != nil {
			return err
		}
		if err := r.writeWindowFile(window, "result.json", raw); err != nil {
			return err
		}
		fmt.Fprintf(&r.tables, "## %s · window %d · %s\n\n%s\n\n", stageLearn, i+1, source, raw)
	}
	if err := r.selectLearning(ctx); err != nil {
		return err
	}
	if err := r.mergeLearning(ctx); err != nil {
		return err
	}
	if len(r.learning.Reviews) > 0 && !slices.ContainsFunc(r.learning.Reviews, func(review atlas.LearningReview) bool { return review.State != "unavailable" }) {
		r.learning.State = "unavailable"
	}
	r.reportStage(stageLearn)
	raw, err := json.MarshalIndent(r.learning, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.opts.OwnerRunDir, "learning-plan.json"), raw, 0600)
}

// Proposals from evidence fragments are not yet a repository curriculum.
// Compose the topic menus together against existing component roles before answering.
// There is no local score, quota or removal based on answer availability.
func (r *reader) selectLearning(ctx context.Context) error {
	var rows []table.Row
	for i, question := range r.learning.Questions {
		var reasons, intents []string
		targetIDs := map[string]bool{}
		for _, origin := range question.Origins {
			if !slices.Contains(intents, origin.Title) {
				intents = append(intents, origin.Title)
			}
			if !slices.Contains(reasons, origin.Why) {
				reasons = append(reasons, origin.Why)
			}
			for _, source := range origin.Sources {
				for _, id := range source.TargetIDs {
					targetIDs[id] = true
				}
			}
		}
		var components []map[string]string
		for _, target := range r.opts.Targets {
			if !targetIDs[target.ID] {
				continue
			}
			component := map[string]string{"name": target.Name, "kind": target.Kind}
			if state := r.targets[target.ID]; state != nil {
				component["model_role"], component["model_purpose"] = state.role, state.line
			}
			components = append(components, component)
		}
		rows = append(rows, table.Row{ID: fmt.Sprint(i), Fields: []table.Field{
			{Name: "question", Value: question.Question}, {Name: "learning_intents", Value: intents},
			{Name: "proposal_reasons", Value: reasons}, {Name: "components", Value: components},
		}})
	}
	intents := learningIntents()
	def := table.Definition{Stage: stageLearn, Contract: "repomap.atlas.learn.select.v3", System: learningSelectPrompt,
		Window: len(intents), Columns: []table.Column{
			{Name: "questions", Kind: table.Sequence, OptionsFrom: "candidate_options"},
			{Name: "reason", Kind: table.Text, MaxRunes: 600},
		}}
	if r.opts.Through == "" || r.opts.Through == stageLearn {
		def.MaxInputBytes = r.opts.InputBytes
	}
	candidates := make([]int, len(rows))
	for i := range candidates {
		candidates[i] = i
	}
	decisions := map[string]int{}
	prepare := func(ids []int) (table.Definition, []table.Field, []table.Row) {
		var catalogue []map[string]any
		var refs []string
		for i, id := range ids {
			ref := fmt.Sprintf("q%d", i+1)
			refs = append(refs, ref)
			candidate := map[string]any{"ref": ref}
			for _, field := range rows[id].Fields {
				candidate[field.Name] = field.Value
			}
			catalogue = append(catalogue, candidate)
		}
		var batch []table.Row
		for _, intent := range intents {
			var options []string
			for i, id := range ids {
				if slices.ContainsFunc(r.learning.Questions[id].Origins, func(o atlas.LearningOrigin) bool { return o.Intent == intent.ID }) {
					options = append(options, refs[i])
				}
			}
			if len(options) > 0 {
				batch = append(batch, table.Row{ID: intent.ID, Fields: []table.Field{
					{Name: "learning_intent", Value: intent.Title}, {Name: "learning_goal", Value: intent.Goal}, {Name: "candidate_options", Value: options},
				}})
			}
		}
		shared := []table.Field{{Name: "repository", Value: r.opts.Repository}, {Name: "candidate_questions", Value: catalogue}}
		return def, shared, batch
	}
	for len(candidates) > 0 {
		var pools [][]int
		var split func([]int) error
		split = func(ids []int) error {
			prepared, shared, batch := prepare(ids)
			windows, err := table.WindowsWithContext(prepared, 1, shared, batch)
			if err == nil && len(windows) == 1 {
				pools = append(pools, ids)
				return nil
			}
			if len(ids) < 2 {
				return fmt.Errorf("learn: one question and its learning goals exceed the input budget")
			}
			mid := len(ids) / 2
			if err := split(ids[:mid]); err != nil {
				return err
			}
			return split(ids[mid:])
		}
		if err := split(candidates); err != nil {
			return err
		}
		chosenIDs := map[int]bool{}
		for _, pool := range pools {
			prepared, shared, batch := prepare(pool)
			r.learningRound++
			answers, err := r.runLearningTable(ctx, prepared, r.learningRound, shared, batch)
			if err != nil {
				return err
			}
			for rowIndex, answer := range answers {
				intentIndex := slices.IndexFunc(intents, func(intent learningIntent) bool { return intent.ID == batch[rowIndex].ID })
				intent := intents[intentIndex]
				chosen := strings.Fields(answer.answer["questions"])
				for i, id := range pool {
					if !slices.ContainsFunc(r.learning.Questions[id].Origins, func(o atlas.LearningOrigin) bool { return o.Intent == intent.ID }) {
						continue
					}
					selection := atlas.LearningSelection{LearningQuestion: r.learning.Questions[id], Intent: intent.ID, Title: intent.Title, Audience: "unavailable", Source: answer.source, PartialContext: len(pools) > 1}
					if answer.source == atlas.SourceGiven {
						r.learning.State = "partial"
					} else {
						selection.Audience, selection.Reason = "not_selected", answer.answer["reason"]
						if slices.Contains(chosen, fmt.Sprintf("q%d", i+1)) {
							selection.Audience = "first_day"
							chosenIDs[id] = true
						}
					}
					key := fmt.Sprintf("%s/%d", intent.ID, id)
					if at, ok := decisions[key]; ok {
						r.learning.Selections[at] = selection
					} else {
						decisions[key] = len(r.learning.Selections)
						r.learning.Selections = append(r.learning.Selections, selection)
					}
				}
			}
		}
		var next []int
		for _, id := range candidates {
			if chosenIDs[id] {
				next = append(next, id)
			}
		}
		complete := len(pools) == 1 || len(next) == len(candidates)
		candidates = next
		if complete {
			break
		}
	}
	var kept []atlas.LearningQuestion
	for _, id := range candidates {
		kept = append(kept, r.learning.Questions[id])
	}
	r.learning.Questions = kept
	return nil
}

// --prompt customizes proposal generation. Audience and consolidation each
// retain their own schemas and prompts within the same Learn stage.
func (r *reader) runLearningTable(ctx context.Context, def table.Definition, round int, shared []table.Field, rows []table.Row) ([]rowAnswer, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	if r.opts.Through == "" || r.opts.Through == stageLearn {
		if r.opts.InputBytes > 0 {
			def.MaxInputBytes = r.opts.InputBytes
		}
		if r.opts.WindowRows > 0 {
			def.Window = r.opts.WindowRows
		}
	}
	return r.runPreparedTable(ctx, def, round, shared, rows, nil)
}

// Compare every pair of proposals when the catalogue needs partitioning.
// Each response groups, never drops, questions. All parent intents and their
// original reasons/anchors survive consolidation into a shared answer.
func (r *reader) mergeLearning(ctx context.Context) error {
	questions := r.learning.Questions
	if len(questions) < 2 {
		return nil
	}
	def := table.Definition{Stage: stageLearn, Contract: "repomap.atlas.learn.merge.v1", System: learningMergePrompt, Window: len(questions),
		Columns: []table.Column{{Name: "representative", Kind: table.Choice}}}
	if r.opts.Through == "" || r.opts.Through == stageLearn {
		def.MaxInputBytes = r.opts.InputBytes
	}
	prepare := func(ids []int) (table.Definition, []table.Field, []table.Row) {
		var catalog []map[string]string
		var refs []string
		for i, id := range ids {
			ref := fmt.Sprintf("q%d", i+1)
			refs = append(refs, ref)
			catalog = append(catalog, map[string]string{"ref": ref, "question": questions[id].Question})
		}
		var rows []table.Row
		for i, id := range ids {
			rows = append(rows, table.Row{ID: fmt.Sprint(id), Fields: []table.Field{{Name: "question", Value: questions[id].Question}, {Name: "own_ref", Value: refs[i]}}})
		}
		prepared := def
		prepared.Columns = []table.Column{{Name: "representative", Kind: table.Choice, Options: refs}}
		return prepared, []table.Field{{Name: "questions", Value: catalog}}, rows
	}
	var pools [][]int
	fits := func(ids []int) bool {
		prepared, shared, rows := prepare(ids)
		windows, err := table.WindowsWithContext(prepared, 1, shared, rows)
		// The catalogue and all its assignments must fit together. Accepting
		// a catalogue that leaves room for just one row multiplies requests.
		return err == nil && len(windows) == 1
	}
	var cross func([]int, []int) error
	cross = func(a, b []int) error {
		ids := append(append([]int{}, a...), b...)
		if fits(ids) {
			pools = append(pools, ids)
			return nil
		}
		if len(a) < len(b) {
			a, b = b, a
		}
		if len(a) < 2 {
			return fmt.Errorf("learn: one pair of questions exceeds the input budget")
		}
		mid := len(a) / 2
		if err := cross(a[:mid], b); err != nil {
			return err
		}
		return cross(a[mid:], b)
	}
	var divide func([]int) error
	divide = func(ids []int) error {
		if len(ids) < 2 {
			return nil
		}
		if fits(ids) {
			pools = append(pools, ids)
			return nil
		}
		mid := len(ids) / 2
		if err := divide(ids[:mid]); err != nil {
			return err
		}
		if err := divide(ids[mid:]); err != nil {
			return err
		}
		return cross(ids[:mid], ids[mid:])
	}
	ids := make([]int, len(questions))
	parent := make([]int, len(questions))
	for i := range ids {
		ids[i] = i
		parent[i] = i
	}
	if err := divide(ids); err != nil {
		return err
	}
	var root func(int) int
	root = func(i int) int {
		if parent[i] != i {
			parent[i] = root(parent[i])
		}
		return parent[i]
	}
	for _, pool := range pools {
		prepared, shared, rows := prepare(pool)
		r.learningRound++
		answers, err := r.runLearningTable(ctx, prepared, r.learningRound, shared, rows)
		if err != nil {
			return err
		}
		for i, answer := range answers {
			if answer.source == atlas.SourceGiven {
				r.learning.State = "unavailable"
				r.learning.Questions = nil
				return nil
			}
			var ref int
			if _, err := fmt.Sscanf(answer.answer["representative"], "q%d", &ref); err != nil || ref < 1 || ref > len(pool) {
				return fmt.Errorf("learn: unknown accepted representative")
			}
			parent[root(pool[i])] = root(pool[ref-1])
		}
	}
	var merged []atlas.LearningQuestion
	positions := map[int]int{}
	for i, q := range questions {
		rep := root(i)
		position, ok := positions[rep]
		if !ok {
			position = len(merged)
			positions[rep] = position
			merged = append(merged, atlas.LearningQuestion{Question: questions[rep].Question})
		}
		merged[position].Origins = append(merged[position].Origins, q.Origins...)
	}
	r.learning.Questions = merged
	return nil
}
