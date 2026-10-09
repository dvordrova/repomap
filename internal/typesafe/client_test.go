package typesafe

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/dvordrova/repomap/internal/llm"
)

func TestClientSendsTheBodyWithItsModelAndRetriesRateLimits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		var starts []time.Time
		client := handlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			starts = append(starts, time.Now())
			if r.Header.Get("Authorization") != "Bearer secret" {
				t.Errorf("authorization: %q", r.Header.Get("Authorization"))
			}
			if calls.Add(1) == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			var body map[string]json.RawMessage
			raw, _ := io.ReadAll(r.Body)
			if json.Unmarshal(raw, &body) != nil || string(body["model"]) != `"jev-test"` || body["questions"] == nil {
				t.Errorf("body: %s", raw)
			}
			w.Write([]byte(`{"model":"jev-test","answers":{"q":{"type":"noul","noul":0.9}},"usage":{"input_tokens":12,"output_tokens":3}}`))
		}))
		prepared, err := client.Prepare(llm.Prompt{User: `{"state":"s","questions":{"q":{"type":"noul","instructions":"yes?"}}}`}, llm.Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if string(prepared.Bytes()) == "" || json.Valid(prepared.Bytes()) == false {
			t.Fatal("prepared request is not JSON")
		}
		if bytes.Contains(prepared.Bytes(), []byte("secret")) {
			t.Fatal("the key entered the prepared request")
		}
		completion, err := client.Complete(t.Context(), prepared)
		if err != nil {
			t.Fatal(err)
		}
		if string(completion.Response) != `{"answers":{"q":{"type":"noul","noul":0.9}}}` || completion.Metrics.InputTokens != 12 || completion.Metrics.Attempts != 2 {
			t.Fatalf("completion: %s %+v", completion.Response, completion.Metrics)
		}
		// Retry-After 0 keeps the shared one-minute floor.
		if len(starts) != 2 || starts[1].Sub(starts[0]) != time.Minute {
			t.Fatalf("rate-limit retry waited %v", starts[1].Sub(starts[0]))
		}
	})
}

// The closed decisions have no other model: a missing key is an error that
// names it, never a client that quietly is not there.
func TestJevKeyIsRequired(t *testing.T) {
	t.Setenv(envAPIKey, " ")
	client, err := NewFromEnv()
	if client != nil || err == nil || !strings.Contains(err.Error(), "JEV_KEY is required") {
		t.Fatalf("client %+v err %v", client, err)
	}
	t.Setenv(envAPIKey, "secret")
	t.Setenv(envModel, "")
	if client, err := NewFromEnv(); err != nil || client.APIKey != "secret" || client.Model != defaultModel {
		t.Fatalf("client %+v err %v", client, err)
	}
}

// Each answer is read on its own: a malformed or unknown one leaves only its
// question without a verdict. A response without answers decided nothing.
func TestVerdictsReadEachAnswerAlone(t *testing.T) {
	verdicts, err := (&Client{}).Verdicts([]byte(`{"answers":{
		"a":{"type":"choice","choice":"x","probabilities":{"x":0.9,"y":0.1}},
		"b":"x",
		"c":{"type":"noul","noul":0.7},
		"d":{"type":"score","score":3}}}`))
	if err != nil || len(verdicts) != 2 || verdicts["a"].Choice != "x" || verdicts["a"].Probabilities["y"] != 0.1 || verdicts["c"].Yes == nil || *verdicts["c"].Yes != 0.7 {
		t.Fatalf("verdicts %+v err %v", verdicts, err)
	}
	for _, raw := range []string{`{}`, `{"answers":null}`, `{"answers":[]}`, `not json`} {
		if _, err := (&Client{}).Verdicts([]byte(raw)); err == nil {
			t.Fatalf("a response without answers was read: %s", raw)
		}
	}
}

func TestNoulOccurrencesKeepInvalidRequiredScores(t *testing.T) {
	good := `{"type":"noul","noul":0}`
	for _, raw := range []string{
		`{"type":"noul","noul":null}`,
		`{"type":"noul","noul":-0.1}`,
		`{"type":"noul","noul":1.1}`,
		`{"type":"noul","noul":1e309}`,
		`{"type":"noul","noul":"NaN"}`,
		`{"type":"noul","noul":0,"noul":0.9}`,
	} {
		for _, written := range []string{raw, raw + `,"bad":` + good, good + `,"bad":` + raw} {
			verdicts, err := (&Client{}).Verdicts([]byte(`{"answers":{"bad":` + written + `,"good":` + good + `}}`))
			if err != nil || !(verdicts["bad"].InvalidYes || verdicts["bad"].Conflict) || verdicts["good"].Yes == nil || *verdicts["good"].Yes != 0 {
				t.Fatalf("invalid noul became a score or poisoned its neighbour: %s / %+v / %v", written, verdicts, err)
			}
		}
	}
}

// A question names its item as its owner asks about it, and each option
// carries its structured criteria, else its meaning, else null: the owner's
// request shape (state = what we want, question = the item, criteria per
// option). The criteria's keys are written in sorted order.
func TestPromptNamesTheItemAndWritesEachOptionsCriteria(t *testing.T) {
	client := &Client{Model: "jev-test"}
	prompt, err := client.Prompt("task", map[string]any{}, map[string]llm.Question{
		"f1|boxes": {Name: "file", Item: map[string]any{"path": "a.c"}, Ask: "One box or several?", Options: []llm.Option{
			{Name: "one box", Criteria: &llm.Criteria{What: "one job", Includes: "its helpers", NotFor: "two jobs", Examples: []string{"a queue"}}},
			{Name: "several boxes", Meaning: "two or more jobs"},
			{Name: "none of these"},
		}},
		"s1|role": {Item: map[string]any{"part": "p1"}, Ask: "Which role?", Options: []llm.Option{{Name: "domain"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"questions":{"f1|boxes":{"criteria":{"none of these":null,"one box":{"examples":["a queue"],"includes":"its helpers","not_for":"two jobs","what":"one job"},"several boxes":"two or more jobs"},"instructions":{"file":{"path":"a.c"},"question":"One box or several?"},"type":"choice"},"s1|role":{"criteria":{"domain":null},"instructions":{"question":"Which role?","row":{"part":"p1"}},"type":"choice"}},"state":{"context":{},"task":"task"}}`
	if prompt.User != want {
		t.Fatalf("prompt:\n%s\nwant:\n%s", prompt.User, want)
	}
	if _, err := client.Prompt("task", nil, map[string]llm.Question{"q": {Name: "question", Ask: "?"}}); err == nil {
		t.Fatal("an item named like the question was accepted")
	}
}

// Every written copy of a question's answer counts until they are compared:
// the same answer twice is one answer, two different ones answer nothing and
// are marked a conflict, and the neighbours keep theirs (review B2: n1's
// open answered yes, then no, was read as no).
func TestVerdictsKeepEveryCopyOfAnAnswerUntilTheyAgree(t *testing.T) {
	yes := `{"type":"choice","choice":"yes","probabilities":{"yes":0.99,"no":0.01}}`
	no := `{"type":"choice","choice":"no","probabilities":{"no":0.99,"yes":0.01}}`
	sameYes := `{"probabilities":{"no":0.01,"yes":0.99},"choice":"yes","type":"choice"}`
	verdicts, err := (&Client{}).Verdicts([]byte(`{"answers":{
		"n1|open":` + yes + `,"n1|open":` + no + `,
		"n2|open":` + yes + `,"n2|open":` + sameYes + `,
		"n3|open":` + yes + `,"n3|open":"unreadable",
		"n4|open":` + yes + `}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !verdicts["n1|open"].Conflict || !verdicts["n3|open"].Conflict {
		t.Fatalf("different copies were read as one answer: %+v", verdicts)
	}
	if got := verdicts["n2|open"]; got.Conflict || got.Choice != "yes" || verdicts["n4|open"].Choice != "yes" {
		t.Fatalf("an identical repeat or a neighbour lost its answer: %+v", verdicts)
	}
	// An answers object written twice is read as all of its copies.
	verdicts, err = (&Client{}).Verdicts([]byte(`{"answers":{"n1|open":` + yes + `},"answers":{"n1|open":` + no + `,"n2|open":` + yes + `}}`))
	if err != nil || !verdicts["n1|open"].Conflict || verdicts["n2|open"].Choice != "yes" {
		t.Fatalf("a repeated answers object: %+v / %v", verdicts, err)
	}
}
