package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type testValue struct {
	Value string `json:"value"`
}

type classifiedTestProviderError struct {
	failure ProviderFailure
	cause   error
}

func (err *classifiedTestProviderError) Error() string {
	if err == nil || err.cause == nil {
		return "classified provider failure"
	}
	return err.cause.Error()
}

func (err *classifiedTestProviderError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func (err *classifiedTestProviderError) ProviderFailure() ProviderFailure {
	if err == nil {
		return ProviderFailure{Kind: ProviderFailureUnknown}
	}
	return err.failure
}

type testProvider struct {
	state         []byte
	secret        string
	prepared      []byte
	prepareErr    error
	responses     [][]byte
	errors        []error
	finishReasons []FinishReason
	stateCalls    int
	prepareCalls  int
	completeCalls int
	prepareOrder  []string
	completeOrder []string
}

type controlledBatchProvider struct {
	started   chan string
	releases  map[string]chan struct{}
	responses map[string][]byte
	errors    map[string]error
	attempts  map[string]int
	ignoreCtx map[string]bool

	mu            sync.Mutex
	active        int
	maxActive     int
	completeCalls map[string]int
}

func newControlledBatchProvider(users ...string) *controlledBatchProvider {
	releases := make(map[string]chan struct{}, len(users))
	for _, user := range users {
		releases[user] = make(chan struct{})
	}
	return &controlledBatchProvider{
		started: make(chan string, len(users)), releases: releases,
		responses: make(map[string][]byte), errors: make(map[string]error),
		attempts: make(map[string]int), ignoreCtx: make(map[string]bool),
		completeCalls: make(map[string]int),
	}
}

func (*controlledBatchProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test","model":"controlled"}`)
}

func (*controlledBatchProvider) Prepare(prompt Prompt, _ Limits) (Prepared, error) {
	return NewPrepared([]byte(prompt.User))
}

func (provider *controlledBatchProvider) Complete(
	ctx context.Context,
	prepared Prepared,
) (Completion, error) {
	user := string(prepared.Bytes())
	releaseAttempt, err := AcquireProviderAttempt(ctx)
	if err != nil {
		return Completion{}, err
	}
	defer releaseAttempt()
	provider.mu.Lock()
	provider.active++
	provider.completeCalls[user]++
	if provider.active > provider.maxActive {
		provider.maxActive = provider.active
	}
	provider.mu.Unlock()
	provider.started <- user

	if provider.ignoreCtx[user] {
		<-provider.releases[user]
	} else {
		select {
		case <-provider.releases[user]:
		case <-ctx.Done():
			provider.mu.Lock()
			provider.active--
			provider.mu.Unlock()
			return Completion{}, ctx.Err()
		}
	}
	provider.mu.Lock()
	provider.active--
	provider.mu.Unlock()

	response := provider.responses[user]
	if response == nil {
		response = []byte(`{"value":"` + user + `"}`)
	}
	attempts := provider.attempts[user]
	if attempts == 0 {
		attempts = 1
	}
	completion := Completion{
		Response: response, FinishReason: FinishStop, ChoiceCount: 1,
		Metrics: Metrics{
			InputTokens: 1, OutputTokens: 1, ProviderResponseBytes: len(response),
			Latency: time.Millisecond, Attempts: attempts,
		},
	}
	if isProviderOverload(provider.errors[user]) {
		CollapseProviderAttempts(ctx)
	}
	return completion, provider.errors[user]
}

func (provider *controlledBatchProvider) release(user string) {
	close(provider.releases[user])
}

func (provider *controlledBatchProvider) snapshot() (int, map[string]int) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	calls := make(map[string]int, len(provider.completeCalls))
	for user, count := range provider.completeCalls {
		calls[user] = count
	}
	return provider.maxActive, calls
}

func waitForBatchStarts(t *testing.T, started <-chan string, count int) []string {
	t.Helper()
	users := make([]string, 0, count)
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for len(users) < count {
		select {
		case user := <-started:
			users = append(users, user)
		case <-timer.C:
			t.Fatalf("timed out waiting for %d batch starts; got %v", count, users)
		}
	}
	return users
}

func requireNoBatchStart(t *testing.T, started <-chan string) {
	t.Helper()
	select {
	case user := <-started:
		t.Fatalf("unexpected batch start %q", user)
	case <-time.After(50 * time.Millisecond):
	}
}

func (provider *testProvider) State() []byte {
	provider.stateCalls++
	return cloneBytes(provider.state)
}

func (provider *testProvider) Prepare(prompt Prompt, limits Limits) (Prepared, error) {
	provider.prepareCalls++
	provider.prepareOrder = append(provider.prepareOrder, prompt.User)
	if provider.prepareErr != nil {
		return Prepared{}, provider.prepareErr
	}
	if len(provider.prepared) != 0 {
		return NewPrepared(provider.prepared)
	}
	exact, err := json.Marshal(struct {
		System    string `json:"system"`
		User      string `json:"user"`
		JSON      bool   `json:"json"`
		MaxTokens int    `json:"max_tokens"`
	}{
		System: prompt.System, User: prompt.User,
		JSON: prompt.ResponseFormatJSON, MaxTokens: limits.MaxOutputTokens,
	})
	if err != nil {
		return Prepared{}, err
	}
	return NewPrepared(exact)
}

func (provider *testProvider) Complete(_ context.Context, prepared Prepared) (Completion, error) {
	index := provider.completeCalls
	provider.completeCalls++
	var exact struct {
		User string `json:"user"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &exact); err == nil {
		provider.completeOrder = append(provider.completeOrder, exact.User)
	}
	response := sequenceValue(provider.responses, index, []byte(`{"value":"ok"}`))
	finish := sequenceValue(provider.finishReasons, index, FinishStop)
	err := sequenceValue(provider.errors, index, error(nil))
	return Completion{
		Response: cloneBytes(response), FinishReason: finish, ChoiceCount: 1,
		Metrics: Metrics{
			InputTokens: 11, OutputTokens: 7, ReasoningTokens: 2,
			PromptCacheHitTokens: 3, PromptCacheMissTokens: 8,
			ProviderResponseBytes: len(response) + 64, UsageReported: true,
			Latency: 5 * time.Millisecond, Attempts: 1,
		},
	}, err
}

func sequenceValue[T any](values []T, index int, fallback T) T {
	if len(values) == 0 {
		return fallback
	}
	if index >= len(values) {
		return values[len(values)-1]
	}
	return values[index]
}

func baseTestProvider() *testProvider {
	return &testProvider{
		state:     []byte(`{"endpoint":"https://provider.test/v1/chat","model":"fixture"}`),
		responses: [][]byte{[]byte(`{"value":"ok"}`)},
	}
}

func baseTestCall(cubeState, user string) Call[testValue] {
	return Call[testValue]{
		State: []byte(cubeState),
		Prompt: Prompt{
			System: "Return one bounded object.", User: user, ResponseFormatJSON: true,
		},
		Limits: Limits{
			MaxRequestBytes: 4096, MaxResponseBytes: 4096, MaxOutputTokens: 1024,
		},
		Validate: func(value testValue) error {
			if value.Value == "" {
				return errors.New("value is empty")
			}
			return nil
		},
	}
}

func TestExecuteJSONInvalidatesProviderCubeAndInputState(t *testing.T) {
	root := t.TempDir()
	provider := baseTestProvider()
	call := baseTestCall("cube-v1", "input-a")
	executor := Executor{RootDir: root, Enabled: true}

	first, err := ExecuteJSON(t.Context(), executor, provider, call)
	if err != nil || first.Cached || provider.completeCalls != 1 {
		t.Fatalf("first = %#v, calls = %d, err = %v", first, provider.completeCalls, err)
	}
	// Provider state is canonical JSON: field order and whitespace do not
	// change identity.
	provider.state = []byte(" { \n  \"model\": \"fixture\", \"endpoint\": \"https://provider.test/v1/chat\" }")
	warm, err := ExecuteJSON(t.Context(), executor, provider, call)
	if err != nil || !warm.Cached || warm.CacheKey != first.CacheKey || provider.completeCalls != 1 {
		t.Fatalf("canonical warm = %#v, calls = %d, err = %v", warm, provider.completeCalls, err)
	}

	cubeChanged := call
	cubeChanged.State = []byte("cube-v2")
	if outcome, err := ExecuteJSON(t.Context(), executor, provider, cubeChanged); err != nil || outcome.Cached {
		t.Fatalf("cube state change = %#v, err = %v", outcome, err)
	}
	inputChanged := call
	inputChanged.Prompt.User = "input-b"
	if outcome, err := ExecuteJSON(t.Context(), executor, provider, inputChanged); err != nil || outcome.Cached {
		t.Fatalf("input change = %#v, err = %v", outcome, err)
	}
	provider.state = []byte(`{"endpoint":"https://provider.test/v1/chat","model":"fixture-v2"}`)
	if outcome, err := ExecuteJSON(t.Context(), executor, provider, call); err != nil || outcome.Cached {
		t.Fatalf("provider state change = %#v, err = %v", outcome, err)
	}
	if provider.completeCalls != 4 {
		t.Fatalf("live calls = %d, want four distinct identities", provider.completeCalls)
	}
}

func TestExecuteJSONRevalidatesHitAndRefetchesOnce(t *testing.T) {
	root := t.TempDir()
	provider := baseTestProvider()
	provider.responses = [][]byte{
		[]byte(`{"value":"old"}`),
		[]byte("```json\n{\"value\":\"new\"}\n```"),
	}
	var events []Event
	executor := Executor{
		RootDir: root, Enabled: true,
		Observer: ObserverFunc(func(event Event) error {
			events = append(events, event)
			return nil
		}),
	}
	call := baseTestCall("cube-v1", "same-input")
	if _, err := ExecuteJSON(t.Context(), executor, provider, call); err != nil {
		t.Fatal(err)
	}
	strict := call
	strict.Validate = func(value testValue) error {
		if value.Value != "new" {
			return errors.New("stale semantic value")
		}
		return nil
	}
	replaced, err := ExecuteJSON(t.Context(), executor, provider, strict)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Cached || replaced.Value.Value != "new" || provider.completeCalls != 2 {
		t.Fatalf("replacement = %#v, calls = %d", replaced, provider.completeCalls)
	}
	if !hasIssue(replaced.Issues, IssueCacheValidate) {
		t.Fatalf("replacement issues = %#v", replaced.Issues)
	}
	warm, err := ExecuteJSON(t.Context(), executor, provider, strict)
	if err != nil || !warm.Cached || warm.Value.Value != "new" || provider.completeCalls != 2 {
		t.Fatalf("replacement warm = %#v, calls = %d, err = %v", warm, provider.completeCalls, err)
	}
	if got := eventKinds(events); !reflect.DeepEqual(got, []EventKind{
		EventLive, EventFailure, EventLive, EventCacheHit,
	}) {
		t.Fatalf("event kinds = %v", got)
	}
	if !reflect.DeepEqual(replaced.Response, provider.responses[1]) ||
		!reflect.DeepEqual(events[2].Request, replaced.Request) ||
		!reflect.DeepEqual(events[2].Response, replaced.Response) ||
		events[2].Metrics.InputTokens != 11 {
		t.Fatalf("live event lost exact exchange: event=%#v outcome=%#v", events[2], replaced)
	}
}

func TestExecuteJSONEvictsUnsafeCacheAndRefetchesOnce(t *testing.T) {
	tests := map[string]func(*testing.T, string){
		"corrupt": func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte(`{"broken"`), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"oversize": func(t *testing.T, path string) {
			file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			if err := file.Truncate(int64(maxCacheRecordBytes + 1)); err != nil {
				_ = file.Close()
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
		},
		"symlink": func(t *testing.T, path string) {
			target := filepath.Join(t.TempDir(), "outside.json")
			if err := os.WriteFile(target, []byte(`{"outside":true}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := os.Stat(target); err != nil {
					t.Errorf("cache eviction followed symlink: %v", err)
				}
			})
		},
		"directory": func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, "child"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, damage := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			provider := baseTestProvider()
			provider.responses = [][]byte{
				[]byte(`{"value":"cold"}`), []byte(`{"value":"replacement"}`),
			}
			executor := Executor{RootDir: root, Enabled: true}
			call := baseTestCall("cube-v1", "same-input")
			cold, err := ExecuteJSON(t.Context(), executor, provider, call)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, cacheDirectoryName, cold.CacheKey+".json")
			damage(t, path)

			replaced, err := ExecuteJSON(t.Context(), executor, provider, call)
			if err != nil {
				t.Fatal(err)
			}
			if replaced.Cached || replaced.Value.Value != "replacement" || provider.completeCalls != 2 {
				t.Fatalf("replacement = %#v, calls = %d", replaced, provider.completeCalls)
			}
			if !hasIssue(replaced.Issues, IssueCacheRead) {
				t.Fatalf("issues = %#v", replaced.Issues)
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				t.Fatalf("replacement cache = %#v, err = %v", info, err)
			}
			warm, err := ExecuteJSON(t.Context(), executor, provider, call)
			if err != nil || !warm.Cached || provider.completeCalls != 2 {
				t.Fatalf("warm = %#v, calls = %d, err = %v", warm, provider.completeCalls, err)
			}
		})
	}
}

func TestExecuteJSONEvictsTamperedCacheAccounting(t *testing.T) {
	mutations := map[string]func(*acceptedCacheRecord){
		"request bytes":  func(record *acceptedCacheRecord) { record.RequestBytes++ },
		"response bytes": func(record *acceptedCacheRecord) { record.ResponseBytes++ },
		"response hash":  func(record *acceptedCacheRecord) { record.ResponseSHA256 = strings.Repeat("0", 64) },
		"negative usage": func(record *acceptedCacheRecord) { record.Metrics.InputTokens = -1 },
		"zero attempts":  func(record *acceptedCacheRecord) { record.Metrics.Attempts = 0 },
		"not accepted":   func(record *acceptedCacheRecord) { record.Accepted = false },
		"not stopped":    func(record *acceptedCacheRecord) { record.FinishReason = FinishLength },
		"many choices":   func(record *acceptedCacheRecord) { record.ChoiceCount = 2 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			provider := baseTestProvider()
			provider.responses = [][]byte{
				[]byte(`{"value":"cold"}`), []byte(`{"value":"replacement"}`),
			}
			executor := Executor{RootDir: root, Enabled: true}
			call := baseTestCall("cube-v1", "same-input")
			cold, err := ExecuteJSON(t.Context(), executor, provider, call)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, cacheDirectoryName, cold.CacheKey+".json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var record acceptedCacheRecord
			if err := json.Unmarshal(data, &record); err != nil {
				t.Fatal(err)
			}
			mutate(&record)
			data, err = json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}

			replaced, err := ExecuteJSON(t.Context(), executor, provider, call)
			if err != nil || replaced.Cached || replaced.Value.Value != "replacement" ||
				provider.completeCalls != 2 || !hasIssue(replaced.Issues, IssueCacheRead) {
				t.Fatalf("replacement = %#v, calls = %d, err = %v", replaced, provider.completeCalls, err)
			}
		})
	}
}

func TestExecuteJSONDisabledCacheBypassesStateReadAndWrite(t *testing.T) {
	root := t.TempDir()
	provider := baseTestProvider()
	provider.state = []byte(`{"api_key":"must-not-be-read"}`)
	call := baseTestCall("", "live-only")
	executor := Executor{RootDir: root, Enabled: false}
	for range 2 {
		outcome, err := ExecuteJSON(t.Context(), executor, provider, call)
		if err != nil || outcome.Cached || outcome.CacheKey != "" {
			t.Fatalf("no-cache outcome = %#v, err = %v", outcome, err)
		}
	}
	if provider.stateCalls != 0 || provider.completeCalls != 2 {
		t.Fatalf("state/live calls = %d/%d", provider.stateCalls, provider.completeCalls)
	}
	if _, err := os.Lstat(filepath.Join(root, cacheDirectoryName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("disabled cache created persistence: %v", err)
	}
}

func TestExecuteJSONDoesNotCacheFailures(t *testing.T) {
	for _, failure := range []string{"provider", "syntax", "semantic"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			provider := baseTestProvider()
			provider.responses = [][]byte{[]byte(`{"value":"bad"}`), []byte(`{"value":"ok"}`)}
			call := baseTestCall("cube-v1", "same-input")
			switch failure {
			case "provider":
				provider.errors = []error{errors.New("temporary provider failure"), nil}
			case "syntax":
				provider.responses[0] = []byte(`{"value":`)
			case "semantic":
				call.Validate = func(value testValue) error {
					if value.Value == "bad" {
						return errors.New("rejected value")
					}
					return nil
				}
			}
			executor := Executor{RootDir: root, Enabled: true}
			failed, err := ExecuteJSON(t.Context(), executor, provider, call)
			if err == nil || failed.Cached {
				t.Fatalf("failed outcome = %#v, err = %v", failed, err)
			}
			if _, err := os.Lstat(filepath.Join(root, cacheDirectoryName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failure populated cache: %v", err)
			}
			accepted, err := ExecuteJSON(t.Context(), executor, provider, call)
			if err != nil || accepted.Cached || accepted.Value.Value != "ok" {
				t.Fatalf("accepted = %#v, err = %v", accepted, err)
			}
			warm, err := ExecuteJSON(t.Context(), executor, provider, call)
			if err != nil || !warm.Cached || provider.completeCalls != 2 {
				t.Fatalf("warm = %#v, calls = %d, err = %v", warm, provider.completeCalls, err)
			}
		})
	}
}

func TestExecuteJSONBatchPreservesOrderAndStops(t *testing.T) {
	provider := baseTestProvider()
	provider.responses = [][]byte{
		[]byte(`{"value":"one"}`), []byte(`{"value":"two"}`), []byte(`{"value":"three"}`),
	}
	provider.errors = []error{nil, errors.New("stop here"), nil}
	calls := []Call[testValue]{
		baseTestCall("", "one"), baseTestCall("", "two"), baseTestCall("", "three"),
	}
	outcomes, err := ExecuteJSONBatch(
		t.Context(), Executor{Enabled: false}, provider, calls,
	)
	if err == nil || len(outcomes) != 2 {
		t.Fatalf("batch outcomes/error = %#v / %v", outcomes, err)
	}
	if !reflect.DeepEqual(provider.prepareOrder, []string{"one", "two"}) ||
		!reflect.DeepEqual(provider.completeOrder, []string{"one", "two"}) {
		t.Fatalf("prepare/complete order = %v / %v", provider.prepareOrder, provider.completeOrder)
	}
}

func TestAttemptGateRejectsCanceledAdmission(t *testing.T) {
	gate := newAttemptGate(1)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if release, err := gate.acquire(canceled); !errors.Is(err, context.Canceled) || release != nil {
		t.Fatalf("canceled free admission = release %v, error %v", release != nil, err)
	}

	releaseFirst, err := gate.acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	waiting, cancelWaiting := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		release, acquireErr := gate.acquire(waiting)
		if release != nil {
			release()
		}
		done <- acquireErr
	}()
	select {
	case err := <-done:
		t.Fatalf("gate admitted a waiter while its only lease was held: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	cancelWaiting()
	releaseFirst()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiting admission error = %v", err)
	}
	gate.mu.Lock()
	active := gate.active
	gate.mu.Unlock()
	if active != 0 {
		t.Fatalf("active leases after cancellation = %d, want zero", active)
	}
}

func TestExecuteJSONBatchBoundsParallelismAndReplaysObserversInCallerOrder(t *testing.T) {
	users := []string{"one", "two", "three", "four"}
	provider := newControlledBatchProvider(users...)
	var observed []string
	executor := Executor{
		Enabled: false, BatchConcurrency: 2,
		Observer: ObserverFunc(func(event Event) error {
			observed = append(observed, string(event.Request))
			if string(event.Request) == "three" {
				return errors.New("observer unavailable")
			}
			return nil
		}),
	}
	type result struct {
		outcomes []Outcome[testValue]
		err      error
	}
	done := make(chan result, 1)
	go func() {
		calls := make([]Call[testValue], 0, len(users))
		for _, user := range users {
			calls = append(calls, baseTestCall("", user))
		}
		outcomes, err := ExecuteJSONBatch(t.Context(), executor, provider, calls)
		done <- result{outcomes: outcomes, err: err}
	}()

	first := waitForBatchStarts(t, provider.started, 2)
	if got := map[string]bool{first[0]: true, first[1]: true}; !got["one"] || !got["two"] || len(got) != 2 {
		t.Fatalf("initial batch starts = %v", first)
	}
	provider.release("two")
	if got := waitForBatchStarts(t, provider.started, 1); got[0] != "three" {
		t.Fatalf("replacement start = %v, want three", got)
	}
	provider.release("three")
	if got := waitForBatchStarts(t, provider.started, 1); got[0] != "four" {
		t.Fatalf("replacement start = %v, want four", got)
	}
	provider.release("four")
	provider.release("one")
	got := <-done
	if got.err != nil || len(got.outcomes) != len(users) {
		t.Fatalf("parallel batch = %#v / %v", got.outcomes, got.err)
	}
	for index, user := range users {
		if got.outcomes[index].Value.Value != user {
			t.Fatalf("outcome %d = %#v, want %q", index, got.outcomes[index], user)
		}
	}
	if !reflect.DeepEqual(observed, users) {
		t.Fatalf("observer order = %v, want %v", observed, users)
	}
	if !hasIssue(got.outcomes[2].Issues, IssueObserver) {
		t.Fatalf("third outcome issues = %#v, missing observer issue", got.outcomes[2].Issues)
	}
	maxActive, completeCalls := provider.snapshot()
	if maxActive != 2 || len(completeCalls) != len(users) {
		t.Fatalf("parallelism/calls = %d / %v", maxActive, completeCalls)
	}
	for _, user := range users {
		if completeCalls[user] != 1 {
			t.Fatalf("call count for %q = %d, want one", user, completeCalls[user])
		}
	}
}

func TestExecuteJSONBatchFailsFastAndPersistsRateLimitGate(t *testing.T) {
	users := []string{"one", "two", "three", "four", "five", "six"}
	provider := newControlledBatchProvider(users...)
	controller := &BatchController{}
	provider.attempts["two"] = 4
	provider.errors["two"] = &classifiedTestProviderError{
		failure: ProviderFailure{
			Kind: ProviderFailureHTTPStatus, HTTPStatus: 429,
			Attempts: 4, RetryExhausted: true,
		},
		cause: errors.New("rate limited"),
	}
	type result struct {
		outcomes []Outcome[testValue]
		err      error
	}
	done := make(chan result, 1)
	executor := Executor{
		Enabled: false, BatchConcurrency: 3, BatchController: controller,
	}
	go func() {
		calls := make([]Call[testValue], 0, len(users))
		for _, user := range users {
			calls = append(calls, baseTestCall("", user))
		}
		outcomes, err := ExecuteJSONBatch(t.Context(), executor, provider, calls)
		done <- result{outcomes: outcomes, err: err}
	}()

	first := waitForBatchStarts(t, provider.started, 3)
	initial := map[string]bool{first[0]: true, first[1]: true, first[2]: true}
	if !initial["one"] || !initial["two"] || !initial["three"] || len(initial) != 3 {
		t.Fatalf("initial batch starts = %v", first)
	}
	provider.release("two")
	got := <-done
	if got.err == nil || !strings.Contains(got.err.Error(), "batch item 1") ||
		len(got.outcomes) != len(users) {
		t.Fatalf("rate-limited batch = %#v / %v", got.outcomes, got.err)
	}
	var providerErr *ProviderError
	if !errors.As(got.err, &providerErr) {
		t.Fatalf("rate-limited batch lost provider error: %v", got.err)
	}
	failure := providerErr.ProviderFailure()
	if failure.Kind != ProviderFailureHTTPStatus || failure.HTTPStatus != 429 ||
		failure.Attempts != 4 || !failure.RetryExhausted {
		t.Fatalf("rate-limit failure = %#v", failure)
	}
	requireNoBatchStart(t, provider.started)
	maxActive, completeCalls := provider.snapshot()
	if maxActive != 3 || len(completeCalls) != 3 ||
		completeCalls["four"] != 0 || completeCalls["five"] != 0 || completeCalls["six"] != 0 {
		t.Fatalf("parallelism/calls = %d / %v", maxActive, completeCalls)
	}

	secondDone := make(chan result, 1)
	go func() {
		calls := []Call[testValue]{
			baseTestCall("", "four"), baseTestCall("", "five"), baseTestCall("", "six"),
		}
		outcomes, err := ExecuteJSONBatch(t.Context(), executor, provider, calls)
		secondDone <- result{outcomes: outcomes, err: err}
	}()
	for _, user := range []string{"four", "five", "six"} {
		if started := waitForBatchStarts(t, provider.started, 1); started[0] != user {
			t.Fatalf("serialized start = %v, want %q", started, user)
		}
		requireNoBatchStart(t, provider.started)
		provider.release(user)
	}
	second := <-secondDone
	if second.err != nil || len(second.outcomes) != 3 {
		t.Fatalf("post-rate-limit batch = %#v / %v", second.outcomes, second.err)
	}
}

func TestExecuteJSONDirectCallsShareCollapsedAttemptGate(t *testing.T) {
	users := []string{"one", "two", "three"}
	provider := newControlledBatchProvider(users...)
	provider.errors["one"] = &classifiedTestProviderError{
		failure: ProviderFailure{
			Kind: ProviderFailureHTTPStatus, HTTPStatus: 429,
			Attempts: 4, RetryExhausted: true,
		},
		cause: errors.New("rate limited"),
	}
	executor := Executor{
		Enabled: false, BatchConcurrency: 2, BatchController: &BatchController{},
	}
	type directResult struct {
		user    string
		outcome Outcome[testValue]
		err     error
	}
	firstDone := make(chan directResult, 1)
	go func() {
		outcome, err := ExecuteJSON(t.Context(), executor, provider, baseTestCall("", "one"))
		firstDone <- directResult{user: "one", outcome: outcome, err: err}
	}()
	if started := waitForBatchStarts(t, provider.started, 1); started[0] != "one" {
		t.Fatalf("first direct start = %v", started)
	}
	provider.release("one")
	if first := <-firstDone; first.err == nil {
		t.Fatalf("rate-limited direct call = %#v / %v", first.outcome, first.err)
	}

	done := make(chan directResult, 2)
	for _, user := range []string{"two", "three"} {
		user := user
		go func() {
			outcome, err := ExecuteJSON(t.Context(), executor, provider, baseTestCall("", user))
			done <- directResult{user: user, outcome: outcome, err: err}
		}()
	}
	started := waitForBatchStarts(t, provider.started, 1)[0]
	if started != "two" && started != "three" {
		t.Fatalf("serialized direct start = %q", started)
	}
	requireNoBatchStart(t, provider.started)
	provider.release(started)
	other := "two"
	if started == other {
		other = "three"
	}
	if next := waitForBatchStarts(t, provider.started, 1); next[0] != other {
		t.Fatalf("second serialized direct start = %v, want %q", next, other)
	}
	provider.release(other)
	for range 2 {
		result := <-done
		if result.err != nil || result.outcome.Value.Value != result.user {
			t.Fatalf("direct result = %#v / %v", result.outcome, result.err)
		}
	}
}

func TestExecuteJSONBatchDoesNotCollapseAfterSemanticFailure(t *testing.T) {
	users := []string{"one", "two", "three", "four"}
	provider := newControlledBatchProvider(users...)
	provider.responses["two"] = []byte(`not-json`)
	controller := &BatchController{}
	executor := Executor{
		Enabled: false, BatchConcurrency: 2, BatchController: controller,
	}
	type result struct {
		outcomes []Outcome[testValue]
		err      error
	}
	done := make(chan result, 1)
	go func() {
		calls := make([]Call[testValue], 0, len(users))
		for _, user := range users {
			calls = append(calls, baseTestCall("", user))
		}
		outcomes, err := ExecuteJSONBatch(t.Context(), executor, provider, calls)
		done <- result{outcomes: outcomes, err: err}
	}()

	_ = waitForBatchStarts(t, provider.started, 2)
	provider.release("two")
	got := <-done
	if got.err == nil || !strings.Contains(got.err.Error(), "batch item 1") ||
		len(got.outcomes) != len(users) {
		t.Fatalf("semantic-failure batch = %#v / %v", got.outcomes, got.err)
	}
	requireNoBatchStart(t, provider.started)

	secondDone := make(chan result, 1)
	go func() {
		outcomes, err := ExecuteJSONBatch(t.Context(), executor, provider, []Call[testValue]{
			baseTestCall("", "three"), baseTestCall("", "four"),
		})
		secondDone <- result{outcomes: outcomes, err: err}
	}()
	started := waitForBatchStarts(t, provider.started, 2)
	seen := map[string]bool{started[0]: true, started[1]: true}
	if !seen["three"] || !seen["four"] || len(seen) != 2 {
		t.Fatalf("starts after semantic failure = %v", started)
	}
	provider.release("three")
	provider.release("four")
	second := <-secondDone
	if second.err != nil || len(second.outcomes) != 2 {
		t.Fatalf("second batch = %#v / %v", second.outcomes, second.err)
	}
	maxActive, _ := provider.snapshot()
	if maxActive != 2 {
		t.Fatalf("semantic failure collapsed parallelism: max active = %d", maxActive)
	}
}

func TestExecuteJSONBatchSurfacesLowestConcurrentTerminalFailure(t *testing.T) {
	users := []string{"one", "two", "three"}
	provider := newControlledBatchProvider(users...)
	provider.responses["one"] = []byte(`invalid-one`)
	provider.responses["two"] = []byte(`invalid-two`)
	provider.ignoreCtx["one"] = true
	provider.ignoreCtx["two"] = true
	type result struct {
		outcomes []Outcome[testValue]
		err      error
	}
	done := make(chan result, 1)
	go func() {
		outcomes, err := ExecuteJSONBatch(t.Context(), Executor{
			Enabled: false, BatchConcurrency: 2,
		}, provider, []Call[testValue]{
			baseTestCall("", "one"), baseTestCall("", "two"), baseTestCall("", "three"),
		})
		done <- result{outcomes: outcomes, err: err}
	}()
	_ = waitForBatchStarts(t, provider.started, 2)
	provider.release("two")
	requireNoBatchStart(t, provider.started)
	provider.release("one")
	got := <-done
	if got.err == nil || !strings.Contains(got.err.Error(), "batch item 0") ||
		len(got.outcomes) != len(users) {
		t.Fatalf("concurrent failure batch = %#v / %v", got.outcomes, got.err)
	}
	requireNoBatchStart(t, provider.started)
}

func TestExecuteJSONBatchCancellationDoesNotStartQueuedCalls(t *testing.T) {
	users := []string{"one", "two", "three", "four"}
	provider := newControlledBatchProvider(users...)
	ctx, cancel := context.WithCancel(t.Context())
	type result struct {
		outcomes []Outcome[testValue]
		err      error
	}
	done := make(chan result, 1)
	go func() {
		calls := make([]Call[testValue], 0, len(users))
		for _, user := range users {
			calls = append(calls, baseTestCall("", user))
		}
		outcomes, err := ExecuteJSONBatch(ctx, Executor{
			Enabled: false, BatchConcurrency: 2,
		}, provider, calls)
		done <- result{outcomes: outcomes, err: err}
	}()
	_ = waitForBatchStarts(t, provider.started, 2)
	cancel()
	got := <-done
	if !errors.Is(got.err, context.Canceled) || len(got.outcomes) != len(users) {
		t.Fatalf("canceled batch = %#v / %v", got.outcomes, got.err)
	}
	requireNoBatchStart(t, provider.started)
	_, completeCalls := provider.snapshot()
	if len(completeCalls) != 2 || completeCalls["three"] != 0 || completeCalls["four"] != 0 {
		t.Fatalf("calls after cancellation = %v", completeCalls)
	}
}

func TestExecuteJSONKeepsAcceptedOutputWithOperationalIssues(t *testing.T) {
	rootFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(rootFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := baseTestProvider()
	executor := Executor{
		RootDir: rootFile, Enabled: true,
		Observer: ObserverFunc(func(Event) error { return errors.New("journal unavailable") }),
	}
	outcome, err := ExecuteJSON(t.Context(), executor, provider, baseTestCall("cube-v1", "input"))
	if err != nil || outcome.Value.Value != "ok" {
		t.Fatalf("accepted output = %#v, err = %v", outcome, err)
	}
	for _, kind := range []IssueKind{IssueCacheRead, IssueCacheEvict, IssueCacheWrite, IssueObserver} {
		if !hasIssue(outcome.Issues, kind) {
			t.Fatalf("issues = %#v, missing %s", outcome.Issues, kind)
		}
	}
}

func TestExecuteJSONDoesNotExposeProviderCredentials(t *testing.T) {
	const secret = "credential-that-must-never-escape"
	t.Run("success persistence and events", func(t *testing.T) {
		root := t.TempDir()
		provider := baseTestProvider()
		provider.secret = secret
		provider.state = []byte(`{"endpoint":"https://provider.test/v1/chat","model":"provider-state-marker"}`)
		var events []Event
		outcome, err := ExecuteJSON(t.Context(), Executor{
			RootDir: root, Enabled: true,
			Observer: ObserverFunc(func(event Event) error {
				events = append(events, event)
				return nil
			}),
		}, provider, baseTestCall("cube-state-marker", "input"))
		if err != nil {
			t.Fatal(err)
		}
		cacheBytes, err := os.ReadFile(filepath.Join(root, cacheDirectoryName, outcome.CacheKey+".json"))
		if err != nil {
			t.Fatal(err)
		}
		eventBytes, err := json.Marshal(events)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{secret, "provider-state-marker", "cube-state-marker"} {
			if strings.Contains(string(cacheBytes), forbidden) || strings.Contains(string(eventBytes), forbidden) {
				t.Fatalf("private state escaped into cache/event: %q", forbidden)
			}
		}
	})

	t.Run("sensitive response is redacted and not cached", func(t *testing.T) {
		root := t.TempDir()
		provider := baseTestProvider()
		response := []byte(`{"api_key":"sk-secret-shaped-provider-output"}`)
		provider.responses = [][]byte{response}
		var events []Event
		outcome, err := ExecuteJSON(t.Context(), Executor{
			RootDir: root, Enabled: true,
			Observer: ObserverFunc(func(event Event) error {
				events = append(events, event)
				return nil
			}),
		}, provider, baseTestCall("cube-v1", "input"))
		if !errors.Is(err, ErrSensitiveResponse) || !outcome.ResponseRedacted ||
			len(outcome.Response) != 0 || outcome.ResponseBytes != len(response) {
			t.Fatalf("sensitive response outcome = %#v, err = %v", outcome, err)
		}
		if len(events) != 1 || !events[0].ResponseRedacted || len(events[0].Response) != 0 {
			t.Fatalf("sensitive response event = %#v", events)
		}
		encoded, marshalErr := json.Marshal(events)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "sk-secret") {
			t.Fatal("sensitive response escaped into event")
		}
		if _, statErr := os.Lstat(filepath.Join(root, cacheDirectoryName)); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("sensitive response populated cache: %v", statErr)
		}
	})

	t.Run("sensitive request is omitted from observer and cache", func(t *testing.T) {
		root := t.TempDir()
		provider := baseTestProvider()
		call := baseTestCall("cube-v1", "Authorization: Bearer "+secret)
		var events []Event
		outcome, err := ExecuteJSON(t.Context(), Executor{
			RootDir: root, Enabled: true,
			Observer: ObserverFunc(func(event Event) error {
				events = append(events, event)
				return nil
			}),
		}, provider, call)
		if err != nil || !outcome.RequestRedacted || len(outcome.Request) != 0 || outcome.RequestBytes == 0 {
			t.Fatalf("sensitive request outcome = %#v, err = %v", outcome, err)
		}
		cacheBytes, readErr := os.ReadFile(filepath.Join(root, cacheDirectoryName, outcome.CacheKey+".json"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		encoded, marshalErr := json.Marshal(events)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if strings.Contains(string(cacheBytes), secret) || strings.Contains(string(encoded), secret) ||
			len(events) != 1 || !events[0].RequestRedacted || len(events[0].Request) != 0 {
			t.Fatalf("sensitive request escaped into cache/event: event=%#v", events)
		}
	})

	t.Run("provider cannot put configured key in prepared body", func(t *testing.T) {
		provider := baseTestProvider()
		provider.prepared = []byte(`{"model":"fixture","api_key":"configured-secret-value"}`)
		outcome, err := ExecuteJSON(
			t.Context(), Executor{Enabled: false}, provider, baseTestCall("", "input"),
		)
		if !errors.Is(err, ErrSensitivePreparedRequest) || provider.completeCalls != 0 ||
			!outcome.RequestRedacted || len(outcome.Request) != 0 {
			t.Fatalf("sensitive prepared request = %#v, calls = %d, err = %v", outcome, provider.completeCalls, err)
		}
	})

	t.Run("state is rejected before live call", func(t *testing.T) {
		provider := baseTestProvider()
		provider.state = []byte(fmt.Sprintf(`{"model":"fixture","api_key":%q}`, secret))
		_, err := ExecuteJSON(
			t.Context(), Executor{RootDir: t.TempDir(), Enabled: true},
			provider, baseTestCall("cube-v1", "input"),
		)
		if err == nil || strings.Contains(err.Error(), secret) || provider.completeCalls != 0 {
			t.Fatalf("sensitive state error/calls = %v / %d", err, provider.completeCalls)
		}
	})

	t.Run("provider failure text is closed", func(t *testing.T) {
		provider := baseTestProvider()
		provider.errors = []error{fmt.Errorf("Authorization: Bearer %s", secret)}
		var events []Event
		_, err := ExecuteJSON(t.Context(), Executor{
			Enabled: false,
			Observer: ObserverFunc(func(event Event) error {
				events = append(events, event)
				return nil
			}),
		}, provider, baseTestCall("", "input"))
		if err == nil || strings.Contains(err.Error(), secret) {
			t.Fatalf("provider error leaked credential: %v", err)
		}
		encoded, marshalErr := json.Marshal(events)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if strings.Contains(string(encoded), secret) {
			t.Fatal("failure event leaked provider error text")
		}
	})
}

func TestPreparedBytesAreImmutableCopies(t *testing.T) {
	original := []byte(`{"request":true}`)
	prepared, err := NewPrepared(original)
	if err != nil {
		t.Fatal(err)
	}
	original[0] = 'x'
	first := prepared.Bytes()
	first[0] = 'y'
	if got := string(prepared.Bytes()); got != `{"request":true}` {
		t.Fatalf("prepared bytes mutated: %q", got)
	}
}

func TestProviderErrorRendersOnlyClosedStructuredFailure(t *testing.T) {
	secret := "Authorization: Bearer provider-secret-value"
	cause := errors.New(secret)
	err := newProviderError("complete", &classifiedTestProviderError{
		failure: ProviderFailure{
			Kind: ProviderFailureHTTPStatus, HTTPStatus: 429,
			Attempts: 99, RetryExhausted: true,
		},
		cause: cause,
	}, 4)
	failure := err.ProviderFailure()
	if failure.Kind != ProviderFailureHTTPStatus || failure.HTTPStatus != 429 ||
		failure.Attempts != 4 || !failure.RetryExhausted || !errors.Is(err, cause) {
		t.Fatalf("provider failure = %#v / %v", failure, err)
	}
	rendered := err.Error()
	for _, want := range []string{
		"class=http_status", "status=429", "attempts=4",
		"retries_exhausted=true", "check provider rate limits or quota, then retry",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("provider error = %q, want %q", rendered, want)
		}
	}
	if strings.Contains(rendered, secret) {
		t.Fatalf("provider error leaked cause: %q", rendered)
	}

	retriedTimeout := newProviderError("complete", &classifiedTestProviderError{
		failure: ProviderFailure{
			Kind: ProviderFailureTimeout, Attempts: 4, RetryExhausted: true,
		},
		cause: cause,
	}, 4)
	if failure := retriedTimeout.ProviderFailure(); failure.Kind != ProviderFailureTimeout || failure.Attempts != 4 || !failure.RetryExhausted {
		t.Fatalf("retried timeout descriptor = %#v", failure)
	}

	invalid := newProviderError("complete", &classifiedTestProviderError{
		failure: ProviderFailure{
			Kind: ProviderFailureKind(secret), HTTPStatus: 999,
			Attempts: -1, RetryExhausted: true, ResourceKind: ResourceLimitKind(secret),
		},
		cause: cause,
	}, 0)
	invalidRendered := invalid.Error()
	if !strings.Contains(invalidRendered, "class=unknown") ||
		strings.Contains(invalidRendered, secret) || strings.Contains(invalidRendered, "status=") ||
		strings.Contains(invalidRendered, "attempts=") || strings.Contains(invalidRendered, "resource=") ||
		strings.Contains(invalidRendered, "retries_exhausted") {
		t.Fatalf("invalid provider descriptor was not closed: %q", invalidRendered)
	}
}

func hasIssue(issues []Issue, kind IssueKind) bool {
	for _, issue := range issues {
		if issue.Kind == kind {
			return true
		}
	}
	return false
}

func eventKinds(events []Event) []EventKind {
	kinds := make([]EventKind, 0, len(events))
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	return kinds
}
