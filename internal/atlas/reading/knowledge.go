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

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

const (
	KnowledgeFilename = "knowledge.json"
	KnowledgeVersion  = 2
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

// A memo is an index into the current shared response, never a second copy
// of the answer. Replaying that request updates every entity bound to it.
type rememberedRow struct {
	RequestKey string `json:"request_key"`
	RowKey     string `json:"row_key"`
}

type rememberedTable struct {
	adapted                 llm.AdaptedResponse
	observed                bool
	rows                    map[string]map[string]json.RawMessage
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
	if parent := r.knowledge[place.Parent]; parent != nil {
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
	call, err := table.Call(def, window)
	if err != nil {
		return k, window, err
	}
	// Reuse depends on what the model actually receives and the table contract.
	// Ownership and prior knowledge IDs are local bindings, not model input.
	// A changed parent line still changes this row's exact request.
	k.BasisID, err = llm.MemoIdentity(r.opts.Provider, call.State, call.Prompt, call.Limits)
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

func (r *reader) recallRow(def table.Definition, window table.Window, ref rememberedRow) (rowAnswer, bool, error) {
	cached, known := r.responseTables[ref.RequestKey]
	if !known {
		exchange, found, err := llm.CachedExchange(r.opts.Executor.RootDir, ref.RequestKey)
		cached = rememberedTable{requestSHA: exchange.RequestSHA256, responseSHA: exchange.ResponseSHA256, request: exchange.Request, response: exchange.Response, err: err}
		if err == nil && found {
			adapted, unwrapErr := llm.AdaptResponse(r.opts.Provider, exchange.Request, exchange.Response)
			cached.adapted = adapted
			cached.err = unwrapErr
			if unwrapErr == nil {
				cached.rows, cached.err = rememberedResponseRows(adapted.Domain)
			}
		}
		r.responseTables[ref.RequestKey] = cached
	}
	if cached.err != nil {
		return rowAnswer{}, false, cached.err
	}
	original, found := cached.rows[ref.RowKey]
	if !found {
		return rowAnswer{}, false, nil
	}
	if original == nil {
		return rowAnswer{}, false, fmt.Errorf("knowledge: duplicate response row key %q", ref.RowKey)
	}
	cells := make(map[string]json.RawMessage, len(original))
	for key, value := range original {
		cells[key] = value
	}
	cells["key"], _ = json.Marshal(table.Key(0))
	raw, err := json.Marshal(map[string]any{"rows": []map[string]json.RawMessage{cells}})
	if err != nil {
		return rowAnswer{}, false, err
	}
	answers, err := table.Decode(def, window, raw)
	if err != nil {
		return rowAnswer{}, false, err
	}
	cached.adapted.Accepted([]string{ref.RowKey})
	if !cached.observed && len(cached.adapted.Rejections) > 0 {
		cached.observed = true
		r.responseTables[ref.RequestKey] = cached
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
	return rowAnswer{answer: answers[0], source: atlas.SourceCache, requestSHA: cached.requestSHA,
		responseSHA: cached.responseSHA, requestKey: ref.RequestKey, rowKey: ref.RowKey}, true, nil
}

// Index response rows without letting an invalid neighbour invalidate an
// independently memoized answer. Required cells are checked when that row is
// recalled against its current definition and exact input.
func rememberedResponseRows(raw []byte) (map[string]map[string]json.RawMessage, error) {
	normalized, err := llm.NormalizeJSON(raw)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Rows []json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(normalized, &envelope); err != nil || envelope.Rows == nil {
		return nil, fmt.Errorf("knowledge: response is not {\"rows\": [...]}")
	}
	rows := make(map[string]map[string]json.RawMessage, len(envelope.Rows))
	for _, rawRow := range envelope.Rows {
		var cells map[string]json.RawMessage
		var key string
		if json.Unmarshal(rawRow, &cells) != nil || json.Unmarshal(cells["key"], &key) != nil || key == "" {
			continue
		}
		if _, duplicate := rows[key]; duplicate {
			rows[key] = nil
		} else {
			rows[key] = cells
		}
	}
	return rows, nil
}

// runIndependent removes known entities and coalesces identical missing inputs
// before planning provider windows. Every original entity keeps its own binding
// to the one accepted answer, independent of its provider batch key.
func (r *reader) runIndependent(ctx context.Context, def table.Definition, round int, shared []table.Field, rows []table.Row) ([]rowAnswer, error) {
	answers := make([]rowAnswer, len(rows))
	inputs := make([]Knowledge, len(rows))
	reused := make([]bool, len(rows))
	var missing []table.Row
	var positions [][]int
	missingByBasis := make(map[string]int)
	for i, row := range rows {
		k, window, err := r.knowledgeInput(def, shared, row)
		if err != nil {
			return nil, err
		}
		inputs[i] = k
		ref, found, err := llm.LoadMemo(r.opts.Executor, k.BasisID, llm.DecodeJSON(func(value rememberedRow) error {
			if len(value.RequestKey) != 64 || value.RowKey == "" {
				return fmt.Errorf("knowledge: invalid response row reference")
			}
			return nil
		}))
		var answer rowAnswer
		if found {
			answer, found, err = r.recallRow(def, window, ref)
		}
		if err != nil {
			r.rejected = append(r.rejected, modeldiag.Row{Stage: def.Stage, Kind: "knowledge_rejected", Count: 1, Reason: err.Error(), Samples: []string{row.ID}})
		}
		if found {
			reused[i] = true
			answers[i] = answer
			r.use(def.Stage).Reused++
			r.use(def.Stage).Rows++
			fmt.Fprintf(&r.tables, "- Reused %s · %s\n", row.ID, answer.answer["line"])
		} else if r.recallOnly {
			answers[i] = rowAnswer{source: atlas.SourceGiven}
			r.use(def.Stage).Rows++
			r.use(def.Stage).Given++
		} else {
			if group, found := missingByBasis[k.BasisID]; found {
				positions[group] = append(positions[group], i)
			} else {
				missingByBasis[k.BasisID] = len(missing)
				missing = append(missing, row)
				positions = append(positions, []int{i})
			}
		}
	}
	if len(missing) > 0 {
		fresh, err := r.runPreparedTable(ctx, def, round, shared, missing, nil)
		if err != nil {
			return nil, err
		}
		for j, group := range positions {
			for alias, position := range group {
				answers[position] = fresh[j]
				if alias == 0 {
					continue
				}
				r.use(def.Stage).Rows++
				if fresh[j].answer == nil {
					r.use(def.Stage).Given++
				}
				fmt.Fprintf(&r.tables, "- Shared exact input %s · representative %s · request %s · row %s\n",
					rows[position].ID, missing[j].ID, fresh[j].requestKey, fresh[j].rowKey)
			}
		}
	}
	saved := make(map[string]bool)
	for i, answer := range answers {
		if answer.answer == nil {
			continue
		}
		k := inputs[i]
		k.Cells, k.Source, k.OriginRequest = answer.answer, answer.source, answer.requestSHA
		k.OriginResponse = answer.responseSHA
		k.ID = k.identity(r.opts.Repository)
		r.knowledge[k.PlaceID] = &k
		r.knowledgeSubjects[k.SubjectID] = &k
		if !r.recallOnly && !reused[i] && !saved[k.BasisID] {
			raw, err := json.Marshal(rememberedRow{RequestKey: answer.requestKey, RowKey: answer.rowKey})
			if err != nil {
				return nil, err
			}
			if err := llm.SaveMemo(r.opts.Executor, k.BasisID, raw); err != nil && r.opts.State != nil {
				r.opts.State(def.Stage, "cache write failed", err.Error())
			}
			saved[k.BasisID] = true
		}
	}
	return answers, nil
}

func (r *reader) persistKnowledge() error {
	rows := make([]Knowledge, 0, len(r.knowledge))
	for _, record := range r.knowledge {
		rows = append(rows, *record)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].PlaceID < rows[j].PlaceID })
	raw, err := json.MarshalIndent(struct {
		Version    int         `json:"version"`
		Repository string      `json:"repository"`
		Revision   string      `json:"revision"`
		Records    []Knowledge `json:"records"`
	}{KnowledgeVersion, r.opts.Repository, r.opts.Revision, rows}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.opts.OwnerRunDir, KnowledgeFilename), raw, 0o600)
}
