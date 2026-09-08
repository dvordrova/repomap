package questionbatch

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"github.com/dvordrova/repomap/internal/llm"
)

// A memo contains only an index into current shared responses. Original
// question text/ref metadata lets a provider-neutral caller reprepare and
// verify the complete original request using today's exact catalogue rows.
// It contains neither a second evidence catalogue nor copied response cells.
type rememberedWindow struct {
	RequestKey  string          `json:"request_key"`
	QuestionRef string          `json:"question_ref"`
	Rows        []string        `json:"rows"`
	Questions   []modelQuestion `json:"questions"`
}

type questionMemo struct {
	Version int                `json:"version"`
	Windows []rememberedWindow `json:"windows"`
}

// One original window may back several question memos. Its accepted result
// belongs to these exact ordered refs, not to an unchecked request key alone.
// This value lives only inside a recall against one current catalogue/provider.
type validatedMemoWindow struct {
	originalRows      []string
	originalQuestions []modelQuestion
	rows              []int
	call              llm.Call[Response]
	response          Response
}

func (data catalogue) memoIdentity(provider llm.Provider, question modelQuestion) (string, error) {
	state, err := json.Marshal(struct {
		Contract       string `json:"contract"`
		EvidenceSHA256 string `json:"evidence_sha256"`
		MaxInputBytes  int    `json:"max_input_bytes"`
		MaxRows        int    `json:"max_rows"`
	}{Contract, data.evidenceSHA, data.opts.MaxInputBytes, data.opts.MaxRows})
	if err != nil {
		return "", err
	}
	// Evidence contributes its exact digest to local semantic state. This
	// isolated identity preparation must itself work for catalogues larger
	// than one provider request. The identity-only input is never completed.
	question.Key = "q1"
	encoded, err := json.Marshal(struct {
		Repository string        `json:"repository"`
		Question   modelQuestion `json:"question"`
	}{data.input.Repository, question})
	if err != nil {
		return "", err
	}
	return llm.MemoIdentity(provider, state, llm.Prompt{
		System: data.opts.System, User: string(encoded), ResponseFormatJSON: true, ResponseExample: responseExample, Reasoning: true,
	}, limits())
}

func (data catalogue) reference(part window, q int, requestKey string) rememberedWindow {
	ref := rememberedWindow{RequestKey: requestKey, QuestionRef: data.questions[q].Key, Questions: data.windowQuestions(part)}
	for _, row := range part.rows {
		ref.Rows = append(ref.Rows, rowRef(row))
	}
	return ref
}

func (data catalogue) recall(ctx context.Context, executor llm.Executor, provider llm.Provider, result *Result) ([]string, [][]rememberedWindow, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	keys := make([]string, len(data.questions))
	remembered := make([][]rememberedWindow, len(data.questions))
	if !executor.Enabled {
		return keys, remembered, nil
	}
	type cachedValue struct {
		adapted   llm.AdaptedResponse
		accepted  bool
		exchange  llm.Outcome[json.RawMessage]
		found     bool
		err       error
		validated *validatedMemoWindow
	}
	exchanges := make(map[string]cachedValue)
	reusedExchanges := make(map[string]int)
	for q, question := range data.questions {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		key, err := data.memoIdentity(provider, question)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, nil, ctxErr
		}
		if err != nil {
			return nil, nil, fmt.Errorf("question batch: prepare memo identity: %w", err)
		}
		keys[q] = key
		memo, found, err := llm.LoadMemo(executor, key, llm.DecodeJSON(func(memo questionMemo) error { return data.validateMemo(question, memo) }))
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, nil, ctxErr
		}
		if err != nil {
			result.Issues = append(result.Issues, fmt.Errorf("question batch: rejected memo for %s: %w", question.Key, err))
		}
		if !found {
			continue
		}
		for _, ref := range memo.Windows {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			cached, exists := exchanges[ref.RequestKey]
			if !exists {
				cached.exchange, cached.found, cached.err = llm.CachedExchange(executor.RootDir, ref.RequestKey)
				if cached.err == nil && cached.found {
					cached.adapted, cached.err = llm.AdaptResponse(provider, cached.exchange.Request, cached.exchange.Response)
				}
				exchanges[ref.RequestKey] = cached
			}
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			if cached.err != nil {
				result.Issues = append(result.Issues, fmt.Errorf("question batch: rejected cached window: %w", cached.err))
				continue
			}
			if !cached.found {
				continue
			}
			checked := cached.validated
			if checked == nil || !slices.Equal(checked.originalRows, ref.Rows) || !slices.Equal(checked.originalQuestions, ref.Questions) {
				rows := data.referenceRows(ref)
				call, err := data.requestCall(rows, ref.Questions)
				if err == nil {
					var prepared llm.Prepared
					prepared, err = llm.Prepare(provider, call.Prompt, call.Limits)
					if ctxErr := ctx.Err(); ctxErr != nil {
						return nil, nil, ctxErr
					}
					if err == nil && !bytes.Equal(prepared.Bytes(), cached.exchange.Request) {
						err = fmt.Errorf("original request differs from current evidence, question metadata or contract")
					}
				}
				var response Response
				if err == nil {
					response, err = call.DecodeValidate(cached.adapted.Domain)
				}
				if ctxErr := ctx.Err(); ctxErr != nil {
					return nil, nil, ctxErr
				}
				if err != nil {
					result.Issues = append(result.Issues, fmt.Errorf("question batch: rejected cached window for %s: %w", question.Key, err))
					continue
				}
				checked = &validatedMemoWindow{originalRows: slices.Clone(ref.Rows), originalQuestions: slices.Clone(ref.Questions), rows: rows, call: call, response: response}
				cached.validated = checked
				exchanges[ref.RequestKey] = cached
			}
			rows, call, response := checked.rows, checked.call, checked.response
			if !cached.accepted {
				cached.adapted.Accepted(nil)
				cached.accepted = true
				exchanges[ref.RequestKey] = cached
				if len(cached.adapted.Rejections) > 0 && executor.Observer != nil {
					if err := executor.Observer.Observe(llm.Event{
						Kind: llm.EventCacheHit, Source: llm.SourceCache, Cached: true,
						CacheRoot: executor.RootDir, CacheKey: ref.RequestKey,
						Request: cached.exchange.Request, Response: cached.exchange.Response,
						RequestSHA256: cached.exchange.RequestSHA256, ResponseSHA256: cached.exchange.ResponseSHA256,
						RequestBytes: len(cached.exchange.Request), ResponseBytes: len(cached.exchange.Response),
						ResponseRejections: cached.adapted.Rejections,
					}); err != nil {
						result.Issues = append(result.Issues, err)
					}
				}
			}
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			outcome := llm.Outcome[Response]{
				Value: response, CacheKey: ref.RequestKey, Cached: true,
				Request: cached.exchange.Request, Response: cached.exchange.Response,
				RequestSHA256: cached.exchange.RequestSHA256, ResponseSHA256: cached.exchange.ResponseSHA256,
				RequestBytes: len(cached.exchange.Request), ResponseBytes: len(cached.exchange.Response),
			}
			data.apply(&result.Questions[q], rows, ref.QuestionRef, outcome)
			remembered[q] = append(remembered[q], ref)
			position, reused := reusedExchanges[ref.RequestKey]
			if !reused {
				position = len(result.Exchanges)
				reusedExchanges[ref.RequestKey] = position
				result.Exchanges = append(result.Exchanges, Exchange{
					ChunkIndexes: append([]int(nil), rows...), Input: json.RawMessage(call.Prompt.User), System: call.Prompt.System,
					Outcome: outcome, Reused: true,
				})
			}
			result.Exchanges[position].QuestionIndexes = append(result.Exchanges[position].QuestionIndexes, data.questionInputs[q])
			result.Exchanges[position].QuestionRefs = append(result.Exchanges[position].QuestionRefs, ref.QuestionRef)
		}
	}
	return keys, remembered, ctx.Err()
}

func (data catalogue) validateMemo(question modelQuestion, memo questionMemo) error {
	if memo.Version != 1 || len(memo.Windows) == 0 {
		return fmt.Errorf("invalid question memo")
	}
	covered := make(map[string]bool)
	for _, ref := range memo.Windows {
		if raw, err := hex.DecodeString(ref.RequestKey); err != nil || len(raw) != 32 {
			return fmt.Errorf("invalid request reference")
		}
		if len(ref.Rows) == 0 || len(ref.Questions) == 0 {
			return fmt.Errorf("empty original window")
		}
		for _, row := range ref.Rows {
			if _, known := data.rowByRef[row]; !known || covered[row] {
				return fmt.Errorf("unknown or overlapping original row reference")
			}
			covered[row] = true
		}
		seen := make(map[string]bool)
		matched := false
		for _, original := range ref.Questions {
			if !questionRef(original.Key) || original.Question == "" || seen[original.Key] {
				return fmt.Errorf("invalid original question reference")
			}
			seen[original.Key] = true
			if original.Key == ref.QuestionRef {
				matched = original.Question == question.Question
			}
		}
		if !matched {
			return fmt.Errorf("memo question does not match its original request reference")
		}
	}
	return nil
}

func questionRef(ref string) bool {
	if len(ref) < 2 || ref[0] != 'q' {
		return false
	}
	number, err := strconv.Atoi(ref[1:])
	return err == nil && number > 0 && strconv.Itoa(number) == ref[1:]
}

func (data catalogue) referenceRows(ref rememberedWindow) []int {
	rows := make([]int, len(ref.Rows))
	for i, key := range ref.Rows {
		rows[i] = data.rowByRef[key]
	}
	return rows
}

func (data catalogue) remember(executor llm.Executor, keys []string, remembered [][]rememberedWindow, result *Result) {
	if !executor.Enabled {
		return
	}
	for q, windows := range remembered {
		if len(windows) == 0 {
			continue
		}
		raw, err := json.Marshal(questionMemo{Version: 1, Windows: windows})
		if err == nil {
			err = llm.SaveMemo(executor, keys[q], raw)
		}
		if err != nil {
			result.Issues = append(result.Issues, fmt.Errorf("question batch: save memo for %s: %w", data.questions[q].Key, err))
		}
	}
}
