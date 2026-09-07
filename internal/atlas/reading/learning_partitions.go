package reading

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/llm"
)

// A partition memo remembers accepted request boundaries, never reviews or
// evidence. It cannot turn an unavailable response into a semantic result.
type learningPartitionMemo struct {
	Version int                      `json:"version"`
	Windows []learningPartitionRange `json:"windows"`
}

type learningPartitionRange struct {
	Start      int    `json:"start"`
	End        int    `json:"end"`
	RequestKey string `json:"request_key"`
}

type learningWindow struct {
	Pool       learningRequest
	Root       int
	Start, End int
}

type learningPartitions struct {
	roots    []learningRequest
	keys     []string
	split    []bool
	accepted [][]learningPartitionRange
}

func (r *reader) planLearningPartitions(pools []learningRequest, prompt string) (learningPartitions, []learningWindow) {
	plan := learningPartitions{roots: pools, keys: make([]string, len(pools)), split: make([]bool, len(pools)), accepted: make([][]learningPartitionRange, len(pools))}
	var windows []learningWindow
	for root, pool := range pools {
		original := learningWindow{Pool: pool, Root: root, End: len(pool.Evidence)}
		if r.dry || !r.opts.Executor.Enabled || len(pool.Evidence) < 2 {
			windows = append(windows, original)
			continue
		}
		call, err := learningCall(pool, prompt)
		if err == nil {
			state := append([]byte("repomap.atlas.learn.partitions.v1\n"), call.State...)
			plan.keys[root], err = llm.MemoIdentity(r.opts.Provider, state, call.Prompt, call.Limits)
		}
		var memo learningPartitionMemo
		found := false
		if err == nil {
			memo, found, err = llm.LoadMemo(r.opts.Executor, plan.keys[root], llm.DecodeJSON(func(memo learningPartitionMemo) error {
				return validateLearningPartition(memo, len(pool.Evidence))
			}))
		}
		var recalled []learningWindow
		if err == nil && found {
			recalled, err = r.recallLearningPartition(original, prompt, call, memo)
		}
		if err != nil {
			r.learningPartitionIssue(err)
		}
		if len(recalled) > 0 {
			plan.split[root] = true
			windows = append(windows, recalled...)
		} else {
			windows = append(windows, original)
		}
	}
	return plan, windows
}

func validateLearningPartition(memo learningPartitionMemo, count int) error {
	if memo.Version != 1 || len(memo.Windows) < 2 {
		return fmt.Errorf("learn: invalid partition memo")
	}
	// Execution may finish an accepted right sibling before splitting its
	// refused left neighbour. Preserve that review/proposal order in the memo;
	// sort only this copy to verify exhaustive, disjoint source coverage.
	ordered := append([]learningPartitionRange(nil), memo.Windows...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Start < ordered[j].Start })
	end := 0
	for _, window := range ordered {
		key, err := hex.DecodeString(window.RequestKey)
		if err != nil || len(key) != 32 || window.Start != end || window.End <= end || window.End > count {
			return fmt.Errorf("learn: incomplete or overlapping partition memo")
		}
		end = window.End
	}
	if end != count {
		return fmt.Errorf("learn: partition memo omits original evidence")
	}
	return nil
}

func (r *reader) recallLearningPartition(original learningWindow, prompt string, call llm.Call[learningResponse], memo learningPartitionMemo) ([]learningWindow, error) {
	// A later successful replay of the original request supersedes the older
	// split. The normal executor will validate that current response too.
	key, err := llm.MemoIdentity(r.opts.Provider, nil, call.Prompt, call.Limits)
	if err != nil {
		return nil, err
	}
	if _, found, err := llm.CachedExchange(r.opts.Executor.RootDir, key); err != nil || found {
		return nil, err
	}
	var windows []learningWindow
	for _, part := range memo.Windows {
		cached, found, err := llm.CachedExchange(r.opts.Executor.RootDir, part.RequestKey)
		if err != nil || !found {
			return nil, err
		}
		pool := newLearningPool(original.Pool.Evidence[part.Start:part.End], true)
		child, err := learningCall(pool, prompt)
		if err != nil {
			return nil, err
		}
		prepared, err := llm.Prepare(r.opts.Provider, child.Prompt, child.Limits)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(prepared.Bytes(), cached.Request) {
			return nil, fmt.Errorf("learn: partition request differs from current evidence or contract")
		}
		// Reviews are deliberately not read here. ExecuteJSONEach revalidates
		// the current cached response, including any replay, at its usual gate.
		windows = append(windows, learningWindow{Pool: pool, Root: original.Root, Start: part.Start, End: part.End})
	}
	return windows, nil
}

func (plan *learningPartitions) accept(window learningWindow, requestKey string) {
	plan.accepted[window.Root] = append(plan.accepted[window.Root], learningPartitionRange{Start: window.Start, End: window.End, RequestKey: requestKey})
}

func (r *reader) saveLearningPartitions(plan learningPartitions) {
	for root, parts := range plan.accepted {
		if !plan.split[root] || plan.keys[root] == "" {
			continue
		}
		memo := learningPartitionMemo{Version: 1, Windows: parts}
		if validateLearningPartition(memo, len(plan.roots[root].Evidence)) != nil {
			continue // A refused or incomplete subtree has no positive memo.
		}
		raw, err := json.Marshal(memo)
		if err == nil {
			err = llm.SaveMemo(r.opts.Executor, plan.keys[root], raw)
		}
		if err != nil {
			r.learningPartitionIssue(err)
		}
	}
}

func (r *reader) learningPartitionIssue(err error) {
	if r.opts.State != nil {
		r.opts.State(stageLearn, "cache miss", err.Error())
	}
}
