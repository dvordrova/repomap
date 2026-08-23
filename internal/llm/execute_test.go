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
	"testing"
	"time"
)

type testValue struct {
	Value string `json:"value"`
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
