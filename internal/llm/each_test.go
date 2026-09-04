package llm

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// eachProvider answers by the request text: a request containing "bad"
// gets a response its decoder refuses, a request containing "fail" is a
// transport failure, everything else succeeds. It counts calls.
type eachProvider struct {
	mu    sync.Mutex
	calls map[string]int
}

func (*eachProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test","model":"each"}`)
}

func (*eachProvider) Prepare(prompt Prompt, _ Limits) (Prepared, error) {
	return NewPrepared([]byte(prompt.User))
}

func (provider *eachProvider) Complete(_ context.Context, prepared Prepared) (Completion, error) {
	user := string(prepared.Bytes())
	provider.mu.Lock()
	if provider.calls == nil {
		provider.calls = make(map[string]int)
	}
	provider.calls[user]++
	provider.mu.Unlock()
	if strings.Contains(user, "fail") {
		return Completion{}, errors.New("transport failed")
	}
	response := `{"ok":true}`
	if strings.Contains(user, "bad") {
		response = `{"ok":false}`
	}
	return Completion{
		Response: []byte(response), FinishReason: FinishStop, ChoiceCount: 1,
		Metrics: Metrics{Attempts: 1, UsageReported: true, InputTokens: 1, OutputTokens: 1},
	}, nil
}

type eachValue struct {
	OK bool `json:"ok"`
}

func eachCall(user string) Call[eachValue] {
	return Call[eachValue]{
		State:  []byte(`{"contract":"each-test"}`),
		Prompt: Prompt{System: "s", User: user},
		Limits: Limits{MaxRequestBytes: 1 << 20, MaxResponseBytes: 1 << 20, MaxOutputTokens: 100},
		Validate: func(value eachValue) error {
			if !value.OK {
				return errors.New("refused")
			}
			return nil
		},
	}
}

func TestExecuteJSONEachKeepsSiblingsWhenOneFails(t *testing.T) {
	provider := &eachProvider{}
	executor := Executor{BatchConcurrency: 3, BatchController: &BatchController{}}
	calls := []Call[eachValue]{eachCall("one"), eachCall("bad two"), eachCall("fail three"), eachCall("four")}
	results := ExecuteJSONEach(context.Background(), executor, provider, calls)
	if len(results) != 4 {
		t.Fatalf("results: %d", len(results))
	}
	if results[0].Err != nil || !results[0].Outcome.Value.OK || results[3].Err != nil || !results[3].Outcome.Value.OK {
		t.Fatalf("good items failed: %v %v", results[0].Err, results[3].Err)
	}
	if results[1].Err == nil || string(results[1].Outcome.Response) != `{"ok":false}` {
		t.Fatalf("the refused item kept no error or response: %v %q", results[1].Err, results[1].Outcome.Response)
	}
	if results[2].Err == nil {
		t.Fatal("the transport failure was swallowed")
	}
	for _, user := range []string{"one", "bad two", "fail three", "four"} {
		if provider.calls[user] != 1 {
			t.Fatalf("%q was called %d times", user, provider.calls[user])
		}
	}
}

func TestExecuteJSONEachDoesNotCacheARefusedAnswer(t *testing.T) {
	provider := &eachProvider{}
	executor := Executor{RootDir: t.TempDir(), Enabled: true, BatchConcurrency: 2, BatchController: &BatchController{}}
	first := ExecuteJSONEach(context.Background(), executor, provider, []Call[eachValue]{eachCall("one"), eachCall("bad two")})
	if first[0].Err != nil || first[1].Err == nil {
		t.Fatalf("first run: %v %v", first[0].Err, first[1].Err)
	}
	second := ExecuteJSONEach(context.Background(), executor, provider, []Call[eachValue]{eachCall("one"), eachCall("bad two")})
	if !second[0].Outcome.Cached {
		t.Fatal("the accepted answer was not cached")
	}
	if second[1].Outcome.Cached || provider.calls["bad two"] != 2 {
		t.Fatalf("the refused answer was cached: cached=%v calls=%d", second[1].Outcome.Cached, provider.calls["bad two"])
	}
}
