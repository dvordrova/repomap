package llm

import (
	"encoding/json"
	"fmt"
)

// This is a resource observation, not an accepted partition or model answer.
// The current owner must still split the complete item and execute/validate
// every child. No child boundaries, payloads or semantic decisions live here.
type adaptiveSplitMemo struct {
	Version int               `json:"version"`
	Kind    ResourceLimitKind `json:"resource_kind"`
}

func adaptiveSplitKind(kind ResourceLimitKind) bool {
	return kind == ResourceLimitResponseBytes || kind == ResourceLimitOutputTokens ||
		kind == ResourceLimitContextTokens
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
	_, found, err := LoadMemo(executor, key, DecodeJSON(func(memo adaptiveSplitMemo) error {
		if memo.Version != 1 || !adaptiveSplitKind(memo.Kind) {
			return fmt.Errorf("llm: invalid adaptive resource split memo")
		}
		return nil
	}))
	if !found && err == nil {
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
		return false, fmt.Errorf("llm: read adaptive resource split memo: %w", err)
	}
	return found, nil
}

func saveAdaptiveSplit(executor Executor, provider Provider, request []byte, limits Limits, kind ResourceLimitKind) error {
	if !executor.Enabled {
		return nil
	}
	providerState, err := canonicalProviderState(provider.State())
	if err != nil {
		return err
	}
	value, err := json.Marshal(adaptiveSplitMemo{Version: 1, Kind: kind})
	if err != nil {
		return err
	}
	// Use the exact failed request, never a second preparation of it.
	if err := SaveMemo(executor, adaptiveSplitKey(providerState, request, limits), value); err != nil {
		return fmt.Errorf("llm: save adaptive resource split memo: %w", err)
	}
	return nil
}
