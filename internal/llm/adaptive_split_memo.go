package llm

import (
	"encoding/json"
	"fmt"
)

const adaptiveResponseRejected = "response_validation"
const adaptiveHTTP500 = "http_500"

// This is a refusal observation, not an accepted partition or model answer.
// The current owner must still split the complete item and execute/validate
// every child. No child boundaries, payloads or semantic decisions live here.
type adaptiveSplitMemo struct {
	Version         int               `json:"version"`
	Kind            ResourceLimitKind `json:"resource_kind,omitempty"`
	RejectionReason string            `json:"rejection_reason,omitempty"`
}

func adaptiveSplitKind(kind ResourceLimitKind) bool {
	return kind == ResourceLimitResponseBytes || kind == ResourceLimitOutputTokens ||
		kind == ResourceLimitContextTokens || kind == ResourceLimitAttemptTime
}

func adaptiveSplitKey(providerState, request []byte, limits Limits) string {
	// Response/request limits need not appear in provider bytes. A changed
	// envelope must retry the parent rather than inherit its old resource limit.
	state, _ := json.Marshal(struct {
		Contract string `json:"contract"`
		Limits   Limits `json:"limits"`
	}{Contract: "repomap.llm.adaptive-split.v1", Limits: limits})
	return executionCacheKey(providerState, state, request)
}

func loadAdaptiveSplit[T any](executor Executor, provider Provider, call Call[T]) (bool, error) {
	if !executor.Enabled || provider == nil || len(call.State) == 0 || validateLimits(call.Limits) != nil {
		return false, nil
	}
	if _, err := decoderForCall(call); err != nil {
		return false, nil
	}
	providerState, err := canonicalProviderState(provider.State())
	if err != nil {
		return false, nil // ExecuteJSON owns preparation diagnostics.
	}
	prepared, err := Prepare(provider, call.Prompt, call.Limits)
	if err != nil || prepared.Len() > call.Limits.MaxRequestBytes {
		return false, nil
	}
	request := prepared.Bytes()
	key := adaptiveSplitKey(providerState, request, call.Limits)
	memo, found, err := LoadMemo(executor, key, DecodeJSON(func(memo adaptiveSplitMemo) error {
		resource := adaptiveSplitKind(memo.Kind) && memo.RejectionReason == ""
		rejected := memo.Kind == "" && (memo.RejectionReason == adaptiveResponseRejected || memo.RejectionReason == adaptiveHTTP500)
		if memo.Version != 1 || (!resource && !rejected) {
			return fmt.Errorf("llm: invalid adaptive split memo")
		}
		return nil
	}))
	if !found && err == nil {
		return false, nil
	}
	if found && ((memo.RejectionReason == adaptiveResponseRejected && !call.SplitRejectedResponse) ||
		(memo.RejectionReason == adaptiveHTTP500 && !call.SplitHTTP500)) {
		return false, nil
	}
	// A successful whole-parent replay supersedes the old split. Even a
	// damaged cache record goes through ExecuteJSON's ordinary diagnosis,
	// eviction and domain validation rather than being hidden by this hint.
	// Check only hinted requests: ordinary hits need no second payload read.
	parentKey := executionCacheKey(providerState, nil, request)
	if _, parentFound, parentErr := loadAcceptedCache(executor.RootDir, parentKey, request, call.Limits); parentFound || parentErr != nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("llm: read adaptive split memo: %w", err)
	}
	return found, nil
}

func saveAdaptiveSplit(executor Executor, provider Provider, request []byte, limits Limits, memo adaptiveSplitMemo) error {
	if !executor.Enabled {
		return nil
	}
	providerState, err := canonicalProviderState(provider.State())
	if err != nil {
		return err
	}
	value, err := json.Marshal(memo)
	if err != nil {
		return err
	}
	// Use the exact failed request, never a second preparation of it.
	if err := SaveMemo(executor, adaptiveSplitKey(providerState, request, limits), value); err != nil {
		return fmt.Errorf("llm: save adaptive split memo: %w", err)
	}
	return nil
}

// RecallAdaptiveSplit lets a stage-owned independent loop reuse the same
// exact-request refusal hint and whole-parent replay precedence as the shared
// adaptive executors. The owner still prepares every complete child.
func RecallAdaptiveSplit[T any](executor Executor, provider Provider, call Call[T]) (bool, error) {
	return loadAdaptiveSplit(executor, provider, call)
}

// RememberAdaptiveSplit records only an eligible refusal. The owner calls it
// after establishing that the failed item has complete, lossless children.
func RememberAdaptiveSplit[T any](executor Executor, provider Provider, call Call[T], outcome Outcome[T], err error) (bool, error) {
	memo, eligible := adaptiveFailureMemo(call, outcome, err)
	if !eligible {
		return false, nil
	}
	return true, saveAdaptiveSplit(executor, provider, outcome.Request, call.Limits, memo)
}
