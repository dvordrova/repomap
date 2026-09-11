package llm

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

type localContextProvider struct {
	*testProvider
	context []byte
	seen    [][]byte
}

func (p *localContextProvider) ResponseContext(Prompt) ([]byte, error) { return p.context, nil }

func (p *localContextProvider) AdaptResponse(local, _, response []byte) (AdaptedResponse, error) {
	p.seen = append(p.seen, cloneBytes(local))
	return AdaptedResponse{Domain: response}, nil
}

func TestLocalResponseContextKeepsExactCacheIdentityAndReplayOwnership(t *testing.T) {
	base := baseTestProvider()
	base.responses = [][]byte{[]byte(`{"value":"first"}`), []byte(`{"value":"replayed"}`)}
	p := &localContextProvider{testProvider: base, context: []byte(`{"rows":["r1","r2"]}`)}
	call := baseTestCall("local-context", "unchanged provider input")
	executor := Executor{Enabled: true, RootDir: t.TempDir()}
	prepared, err := Prepare(p, call.Prompt, call.Limits)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := Prepare(base, call.Prompt, call.Limits)
	if err != nil || !bytes.Equal(prepared.Bytes(), plain.Bytes()) {
		t.Fatal("local context changed provider input")
	}
	p.context[0] = '!'
	if !json.Valid(prepared.ResponseContext()) {
		t.Fatal("context retains provider-owned mutable bytes")
	}
	p.context = []byte(`{"rows":["r1","r2"]}`)
	first, err := ExecuteJSON(t.Context(), executor, p, call)
	if err != nil {
		t.Fatal(err)
	}
	originalContext := cloneBytes(p.context)
	// The same exact response can serve a changed local corpus/prose scope.
	p.context = []byte(`{"rows":["r1"]}`)
	warm, err := ExecuteJSON(t.Context(), executor, p, call)
	if err != nil || !warm.Cached || first.CacheKey != warm.CacheKey || !bytes.Equal(p.seen[1], p.context) {
		t.Fatalf("warm execution used saved rather than current owning context: %+v %v", warm, err)
	}
	saved, found, err := CachedExchange(executor.RootDir, first.CacheKey)
	if err != nil || !found || !sameLocalJSON(saved.ResponseContext, originalContext) {
		t.Fatal("original memo context was replaced by an isolated current scope")
	}
	exact, _ := NewPrepared(first.Request)
	if _, err := ReplayJSON(t.Context(), executor, base, exact); err != nil {
		t.Fatal(err)
	}
	saved, found, err = CachedExchange(executor.RootDir, first.CacheKey)
	if err != nil || !found || !sameLocalJSON(saved.ResponseContext, originalContext) || !bytes.Equal(saved.Response, []byte(`{"value":"replayed"}`)) {
		t.Fatalf("replay lost original context or did not replace response: %+v %v", saved, err)
	}
	if !reflect.DeepEqual(p.seen, [][]byte{originalContext, p.context}) || base.completeCalls != 2 {
		t.Fatal("response context or provider call count changed")
	}
}

func sameLocalJSON(a, b []byte) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}
