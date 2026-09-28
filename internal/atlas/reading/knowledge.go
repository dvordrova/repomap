package reading

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

const (
	KnowledgeFilename = "knowledge.json"
	KnowledgeVersion  = 3
)

// Knowledge is one interpretation attached to an internal entity. Input is
// the exact evidence shown for that entity; DependsOn names earlier model
// interpretations, rather than silently presenting them as source facts.
// Descriptions currently inspect declarations and extracted facts, not bodies.
type Knowledge struct {
	ID             string          `json:"id"`
	SubjectID      string          `json:"subject_id"`
	PlaceID        string          `json:"place_id"`
	ContextID      string          `json:"context_id,omitempty"`
	TargetIDs      []string        `json:"target_ids"`
	Path           string          `json:"path"`
	Line           int             `json:"line,omitempty"`
	Stage          string          `json:"stage"`
	Contract       string          `json:"contract"`
	PromptSHA256   string          `json:"prompt_sha256"`
	BasisID        string          `json:"basis_id"`
	Input          json.RawMessage `json:"input"`
	DependsOn      []string        `json:"depends_on,omitempty"`
	Cells          table.Answer    `json:"cells"`
	Source         string          `json:"source"`
	OriginRequest  string          `json:"origin_request_sha256"`
	OriginResponse string          `json:"origin_response_sha256"`
}

// knowledgeRecordKey keeps independent interpretations of one place apart
// without manufacturing another entity identity from a string prefix.
type knowledgeRecordKey struct {
	PlaceID  string
	Stage    string
	Contract string
}

// A memo is an index into the current shared response, never a second copy
// of the answer. Replaying that request updates every entity bound to it.
type rememberedRow struct {
	RequestKey string `json:"request_key"`
	RowKey     string `json:"row_key"`
}

type rememberedTable struct {
	adapted                 llm.AdaptedResponse
	observed                bool
	rows                    map[string][]map[string]json.RawMessage
	request, response       []byte
	requestSHA, responseSHA string
	err                     error
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (r *reader) knowledgeInput(def table.Definition, shared []table.Field, row table.Row) (Knowledge, table.Window, error) {
	place, known := r.places[row.ID]
	if subject, named := r.rowSubjects[row.ID]; named {
		// A row about no place, such as an outside symbol, is its own
		// subject: no target, no context, where it is first used.
		place, known = atlas.Place{ID: subject.id, Path: subject.path, LineNo: subject.line}, true
	}
	if !known {
		return Knowledge{}, table.Window{}, fmt.Errorf("knowledge: row %s has no internal entity", row.ID)
	}
	k := Knowledge{
		SubjectID: place.ID, PlaceID: place.ID, Path: place.Path, Line: place.LineNo,
		TargetIDs: append([]string(nil), place.TargetIDs...),
		Stage:     def.Stage, Contract: def.Contract, PromptSHA256: digest([]byte(def.System)),
	}
	sort.Strings(k.TargetIDs)
	if place.Symbol != nil && place.Symbol.Decl.ObjectID != "" {
		k.SubjectID = place.Symbol.Decl.ObjectID
	}
	// A file's placement uses its directory; a declaration/boundary uses
	// its file. These interpretations retain that context even when it is
	// deterministic and contributes no model dependency.
	k.ContextID = place.Parent
	unlock := r.lock()
	parent := r.knowledge[place.Parent]
	unlock()
	if parent != nil {
		for _, field := range row.Fields {
			if (field.Name == "directory_hypothesis" || field.Name == "file_hypothesis" || field.Name == "description_hypothesis") && field.Value == parent.Cells["line"] {
				k.DependsOn = []string{parent.ID}
				break
			}
		}
	}
	window := table.Window{Stage: def.Stage, Context: shared, Rows: []table.Row{row}}
	input, err := table.Request(def, window)
	if err != nil {
		return k, window, err
	}
	window.Request, k.Input = input, input
	// Reuse depends on the evidence the model receives and the table contract.
	// The artifact ID is an owner binding, not a second semantic input: equal
	// evidence can share one accepted interpretation without renaming either row.
	// The basis names the provider that answers: the categorizer's answer
	// is never recalled for a text model's table, or the reverse.
	k.BasisID, err = table.MemoIdentity(r.providerFor(def), def, window)
	return k, window, err
}

// identity binds an answer to its current subject and provenance. Reusing the
// same answer after a scope change must not retain stale dependency IDs.
func (k Knowledge) identity(repository string) string {
	raw, _ := json.Marshal(struct {
		Repository string       `json:"repository"`
		Subject    string       `json:"subject"`
		Place      string       `json:"place"`
		Context    string       `json:"context"`
		Targets    []string     `json:"targets"`
		DependsOn  []string     `json:"depends_on"`
		Basis      string       `json:"basis"`
		Cells      table.Answer `json:"cells"`
	}{repository, k.SubjectID, k.PlaceID, k.ContextID, k.TargetIDs, k.DependsOn, k.BasisID, k.Cells})
	return digest(raw)
}

// recallRow takes a remembered answer for this row and accepts it at once.
func (r *reader) recallRow(def table.Definition, window table.Window, ref rememberedRow) (rowAnswer, bool, error) {
	answer, found, err := r.recallAnswer(def, window, ref)
	if found && err == nil {
		r.acceptRecall(def, ref, answer.partial)
	}
	return answer, found, err
}

// recallAnswer reads a remembered answer for this row from its shared
// response, read and parsed once per request, and validates it against the
// current table. It changes nothing else, so rows may be recalled at once;
// acceptRecall then accepts each recalled row in row order.
func (r *reader) recallAnswer(def table.Definition, window table.Window, ref rememberedRow) (rowAnswer, bool, error) {
	if r.classifies(def) {
		return r.recallClassifierRow(def, window, ref)
	}
	cached := loadOnce(r, &r.responseTables, "table", ref.RequestKey, func() rememberedTable {
		exchange, found, err := llm.CachedExchange(r.opts.Executor.RootDir, ref.RequestKey)
		cached := rememberedTable{requestSHA: exchange.RequestSHA256, responseSHA: exchange.ResponseSHA256, request: exchange.Request, response: exchange.Response, err: err}
		if err == nil && found {
			adapted, unwrapErr := llm.AdaptResponse(r.opts.Provider, exchange.ResponseContext, exchange.Request, exchange.Response)
			cached.adapted = adapted
			cached.err = unwrapErr
			if unwrapErr == nil {
				cached.rows, cached.err = rememberedResponseRows(adapted.Domain)
			}
		}
		return cached
	})
	if cached.err != nil {
		return rowAnswer{}, false, cached.err
	}
	copies := cached.rows[ref.RowKey]
	if len(copies) == 0 {
		return rowAnswer{}, false, nil
	}
	// Every copy the response holds for the row is decoded as it was live:
	// identical copies are one answer, differing ones none.
	rows := make([]map[string]json.RawMessage, 0, len(copies))
	for _, original := range copies {
		cells := make(map[string]json.RawMessage, len(original))
		for key, value := range original {
			cells[key] = value
		}
		cells["key"], _ = json.Marshal(window.Rows[0].ID)
		rows = append(rows, cells)
	}
	raw, err := json.Marshal(map[string]any{"rows": rows})
	if err != nil {
		return rowAnswer{}, false, err
	}
	result, err := table.DecodeResult(def, window, raw)
	if err != nil {
		return rowAnswer{}, false, err
	}
	return rowAnswer{answer: result.Answers[0], source: atlas.SourceCache, requestSHA: cached.requestSHA,
		responseSHA: cached.responseSHA, requestKey: ref.RequestKey, rowKey: ref.RowKey,
		partial: len(result.AcceptedRowKeys()) == 0}, true, nil
}

// acceptRecall accepts one recalled row of a text-model response: its prose
// reaches the glossary unless the row lost a cell, and the first row
// recalled from a response with rejected rows journals that response once.
func (r *reader) acceptRecall(def table.Definition, ref rememberedRow, partial bool) {
	if r.classifies(def) {
		return
	}
	unlock := r.lock()
	cached := r.responseTables[ref.RequestKey]
	observe := !cached.observed && len(cached.adapted.Rejections) > 0
	if observe {
		cached.observed = true
		r.responseTables[ref.RequestKey] = cached
	}
	unlock()
	if !partial {
		cached.adapted.Accepted([]string{ref.RowKey})
	}
	if !observe {
		return
	}
	executor := debugdump.BindStage(r.opts.Executor, def.Stage)
	if executor.Observer != nil {
		if err := executor.Observer.Observe(llm.Event{
			Kind: llm.EventCacheHit, Source: llm.SourceCache, Cached: true,
			CacheRoot: executor.RootDir, CacheKey: ref.RequestKey,
			Request: cached.request, Response: cached.response,
			RequestSHA256: cached.requestSHA, ResponseSHA256: cached.responseSHA,
			RequestBytes: len(cached.request), ResponseBytes: len(cached.response),
			ResponseRejections: cached.adapted.Rejections,
		}); err != nil {
			r.rejected = append(r.rejected, modeldiag.Row{Stage: def.Stage, Kind: "metadata_observer_failed", Count: 1, Reason: err.Error()})
		}
	}
}

// recallClassifierRow takes a remembered decision-model answer for this row
// from its cached response, validated against the current table.
func (r *reader) recallClassifierRow(def table.Definition, window table.Window, ref rememberedRow) (rowAnswer, bool, error) {
	// One remembered response answers many rows: read and parse it once.
	cached := loadOnce(r, &r.classifierResponses, "classifier", ref.RequestKey, func() rememberedClassifier {
		exchange, found, err := llm.CachedExchange(r.opts.Executor.RootDir, ref.RequestKey)
		cached := rememberedClassifier{found: found, err: err, requestSHA: exchange.RequestSHA256, responseSHA: exchange.ResponseSHA256}
		if err == nil && found {
			cached.answers, cached.err = r.opts.Categorizer.Verdicts(exchange.Response)
		}
		return cached
	})
	if cached.err != nil || !cached.found {
		return rowAnswer{}, false, cached.err
	}
	original := window
	original.Rows = []table.Row{{ID: ref.RowKey, Fields: window.Rows[0].Fields}}
	result, err := table.DecodeClassifierAnswers(def, original, cached.answers)
	if err != nil {
		return rowAnswer{}, false, err
	}
	// A row the remembered response answered uncertainly is found with no
	// answer: the same response would leave it so again.
	return rowAnswer{answer: result.Answers[0], source: atlas.SourceCache, requestSHA: cached.requestSHA,
		responseSHA: cached.responseSHA, requestKey: ref.RequestKey, rowKey: ref.RowKey}, true, nil
}

// rememberedClassifier is one cached categorizer response, parsed once.
type rememberedClassifier struct {
	answers                 map[string]llm.Verdict
	found                   bool
	requestSHA, responseSHA string
	err                     error
}

// Index response rows without letting an invalid neighbour invalidate an
// independently memoized answer. Required cells are checked when that row is
// recalled against its current definition and exact input. Every copy of a
// key is kept, so a recall applies the live rule to repeats. The envelope
// forms are the live decoder's.
func rememberedResponseRows(raw []byte) (map[string][]map[string]json.RawMessage, error) {
	normalized, err := llm.NormalizeJSON(raw)
	if err != nil {
		return nil, err
	}
	responseRows, err := table.ResponseRows(normalized)
	if err != nil {
		return nil, fmt.Errorf("knowledge: %w", err)
	}
	rows := make(map[string][]map[string]json.RawMessage, len(responseRows))
	for _, rawRow := range responseRows {
		var cells map[string]json.RawMessage
		var key string
		if json.Unmarshal(rawRow, &cells) != nil || json.Unmarshal(cells["key"], &key) != nil || strings.TrimSpace(key) == "" {
			continue
		}
		key = strings.TrimSpace(key)
		rows[key] = append(rows[key], cells)
	}
	return rows, nil
}

// runIndependent removes known entities and coalesces identical missing inputs
// before planning provider windows. Every original entity keeps its own binding
// to the one accepted answer, independent of its provider batch key.
func (r *reader) runIndependent(ctx context.Context, def table.Definition, round int, groups rowGroups) ([]rowAnswer, error) {
	answers := make([]rowAnswer, groups.count())
	rows := make([]table.Row, 0, len(answers))
	inputs := make([]Knowledge, len(answers))
	reused := make([]bool, len(answers))
	// A row the cache does not answer is asked once per exact basis, in its
	// own group's window; rows sharing that basis take the same answer.
	type missingRef struct{ group, position int }
	missing := make(rowGroups, len(groups))
	missingByBasis := make(map[string]missingRef)
	var order []missingRef
	sharing := make(map[missingRef][]int)
	prepared := r.prepareRows(def, groups)
	for g, group := range groups {
		missing[g].shared = group.shared
		for _, row := range group.rows {
			i := len(rows)
			rows = append(rows, row)
			if prepared[i].fatal != nil {
				return nil, prepared[i].fatal
			}
			k, answer, found, err := prepared[i].input, prepared[i].answer, prepared[i].found, prepared[i].err
			inputs[i] = k
			if found {
				r.acceptRecall(def, prepared[i].ref, prepared[i].answer.partial)
			}
			if err != nil {
				r.rejected = append(r.rejected, modeldiag.Row{Stage: def.Stage, Kind: "knowledge_rejected", Count: 1, Reason: err.Error(), Samples: []string{row.ID}})
			}
			if found {
				reused[i] = true
				answers[i] = answer
				r.use(def.Stage).Reused++
				r.use(def.Stage).Rows++
				if answer.answer == nil {
					r.use(def.Stage).Given++
				}
				fmt.Fprintf(&r.tables, "- Reused %s · %s\n", row.ID, answer.answer["line"])
			} else if r.recallOnly {
				answers[i] = rowAnswer{source: atlas.SourceGiven}
				r.use(def.Stage).Rows++
				r.use(def.Stage).Given++
			} else {
				at, found := missingByBasis[k.BasisID]
				if !found {
					at = missingRef{g, len(missing[g].rows)}
					missing[g].rows = append(missing[g].rows, row)
					missingByBasis[k.BasisID] = at
					order = append(order, at)
				}
				sharing[at] = append(sharing[at], i)
			}
		}
	}
	if len(order) > 0 {
		fresh, err := r.runPreparedGroups(ctx, def, round, missing, nil)
		if err != nil {
			return nil, err
		}
		offsets := make([]int, len(missing))
		for g := 1; g < len(missing); g++ {
			offsets[g] = offsets[g-1] + len(missing[g-1].rows)
		}
		for _, at := range order {
			flat := offsets[at.group] + at.position
			for alias, position := range sharing[at] {
				answers[position] = fresh[flat]
				if alias == 0 {
					continue
				}
				r.use(def.Stage).Rows++
				if fresh[flat].answer == nil {
					r.use(def.Stage).Given++
				}
				fmt.Fprintf(&r.tables, "- Shared exact input %s · representative %s · request %s · row %s\n",
					rows[position].ID, missing[at.group].rows[at.position].ID, fresh[flat].requestKey, fresh[flat].rowKey)
			}
		}
	}
	// Each exact basis remembers the request and row that answered it. The
	// small memo files are independent, so they are written at once.
	type memo struct {
		key string
		raw []byte
	}
	var memos []memo
	saved := make(map[string]bool)
	for i, answer := range answers {
		if answer.answer == nil {
			// A row a decision model answered uncertainly keeps no knowledge,
			// but its memo: the same response leaves it undecided again, and
			// asking once more would be drawing again for a clearer answer.
			if answer.uncertain && answer.requestKey != "" && !r.recallOnly && !reused[i] && !saved[inputs[i].BasisID] {
				raw, err := json.Marshal(rememberedRow{RequestKey: answer.requestKey, RowKey: answer.rowKey})
				if err != nil {
					return nil, err
				}
				memos = append(memos, memo{key: inputs[i].BasisID, raw: raw})
				saved[inputs[i].BasisID] = true
			}
			continue
		}
		k := inputs[i]
		k.Cells, k.Source, k.OriginRequest = answer.answer, answer.source, answer.requestSHA
		k.OriginResponse = answer.responseSHA
		k.ID = k.identity(r.opts.Repository)
		unlock := r.lock()
		if r.knowledgeRecords == nil {
			r.knowledgeRecords = make(map[knowledgeRecordKey]*Knowledge)
		}
		r.knowledgeRecords[knowledgeRecordKey{PlaceID: k.PlaceID, Stage: k.Stage, Contract: k.Contract}] = &k
		r.shared.knowledgeVersion++
		r.knowledge[k.PlaceID] = &k
		r.knowledgeSubjects[k.SubjectID] = &k
		unlock()
		if !r.recallOnly && !reused[i] && !saved[k.BasisID] {
			raw, err := json.Marshal(rememberedRow{RequestKey: answer.requestKey, RowKey: answer.rowKey})
			if err != nil {
				return nil, err
			}
			memos = append(memos, memo{key: k.BasisID, raw: raw})
			saved[k.BasisID] = true
		}
	}
	failed := make([]error, len(memos))
	eachIndex(len(memos), func(i int) {
		failed[i] = llm.SaveMemo(r.opts.Executor, memos[i].key, memos[i].raw)
	})
	for _, err := range failed {
		if err != nil && r.opts.State != nil {
			r.opts.State(def.Stage, "cache write failed", err.Error())
		}
	}
	return answers, nil
}

// preparedRow is one independent row's exact single-row input and what its
// memo recalls. Rows are prepared at once and committed in row order.
type preparedRow struct {
	input  Knowledge
	answer rowAnswer
	ref    rememberedRow
	found  bool
	// err is a failed recall: the row is asked again. fatal is a row
	// without an input: the reading stops there.
	err, fatal error
}

// prepareRows builds every row's input, loads its memo and recalls its
// remembered answer, on all processors. It only reads the reader's state
// and fills the shared request caches; nothing is counted, printed or
// accepted here.
func (r *reader) prepareRows(def table.Definition, groups rowGroups) []preparedRow {
	type job struct {
		shared []table.Field
		row    table.Row
	}
	var jobs []job
	for _, group := range groups {
		for _, row := range group.rows {
			jobs = append(jobs, job{shared: group.shared, row: row})
		}
	}
	prepared := make([]preparedRow, len(jobs))
	r.lock()()
	eachIndex(len(jobs), func(i int) {
		p := &prepared[i]
		var window table.Window
		if p.input, window, p.fatal = r.knowledgeInput(def, jobs[i].shared, jobs[i].row); p.fatal != nil {
			return
		}
		ref, found, err := llm.LoadMemo(r.opts.Executor, p.input.BasisID, llm.DecodeJSON(func(value rememberedRow) error {
			if len(value.RequestKey) != 64 || value.RowKey == "" {
				return fmt.Errorf("knowledge: invalid response row reference")
			}
			return nil
		}))
		if found {
			p.ref = ref
			p.answer, found, err = r.recallAnswer(def, window, ref)
		}
		p.found, p.err = found, err
	})
	return prepared
}

// persistKnowledge writes knowledge.json when its records changed since the
// last write, and always the first time.
func (r *reader) persistKnowledge() error {
	unlock := r.lock()
	version := r.shared.knowledgeVersion
	if r.knowledgeWritten && r.knowledgeSaved == version {
		unlock()
		return nil
	}
	rows := make([]Knowledge, 0, len(r.knowledgeRecords))
	for _, record := range r.knowledgeRecords {
		rows = append(rows, *record)
	}
	unlock()
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].PlaceID != rows[j].PlaceID {
			return rows[i].PlaceID < rows[j].PlaceID
		}
		if rows[i].Stage != rows[j].Stage {
			return rows[i].Stage < rows[j].Stage
		}
		return rows[i].Contract < rows[j].Contract
	})
	raw, err := json.MarshalIndent(struct {
		Version    int         `json:"version"`
		Repository string      `json:"repository"`
		Revision   string      `json:"revision"`
		Records    []Knowledge `json:"records"`
	}{KnowledgeVersion, r.opts.Repository, r.opts.Revision, rows}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(r.opts.OwnerRunDir, KnowledgeFilename), raw, 0o600); err != nil {
		return err
	}
	r.knowledgeWritten, r.knowledgeSaved = true, version
	return nil
}
