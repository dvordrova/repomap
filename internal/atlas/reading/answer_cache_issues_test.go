package reading

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

func answerCacheRecordCollision(t *testing.T, kind string) (*reader, *answerTestProvider, llm.Outcome[table.Result]) {
	t.Helper()
	p := &answerTestProvider{tableProvider: &tableProvider{}}
	r := answerTestReader(t, answerTestRoutes(1), p)
	def := lines.Answer()
	parts := []answerQuestion{{index: 0, candidates: uniqueRouteAnchors(r.questions[0].Stops), complete: true}}
	windows, err := r.planAnswers(context.Background(), def, parts)
	if err != nil || len(windows) != 1 {
		t.Fatalf("plan: %v, windows=%d", err, len(windows))
	}
	windows[0].table.Index = 0
	call, err := answerCall(def, windows[0])
	if err != nil {
		t.Fatal(err)
	}
	// Obtain the exact cache record key through the executor, using a mock.
	cold, err := llm.ExecuteJSON(context.Background(), r.opts.Executor, p, call)
	if err != nil || len(cold.Value.Answers) != 1 || cold.Value.Answers[0]["answer"] == "" {
		t.Fatalf("prime: %v / %#v", err, cold.Value)
	}
	cacheRecord := filepath.Join(r.opts.Executor.RootDir, llm.CacheDirectoryName, cold.CacheKey+".json")
	if err := os.Remove(cacheRecord); err != nil {
		t.Fatal(err)
	}
	makeCollision := func() error {
		if err := os.Mkdir(cacheRecord, 0o700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(cacheRecord, "collision"), []byte("only the cache record pathname is occupied"), 0o600)
	}
	if kind == "read" {
		if err := makeCollision(); err != nil {
			t.Fatal(err)
		}
	} else {
		// Lookup sees a miss; only the optional record write is obstructed
		// when the successfully generated answer returns from the provider.
		p.completeAnswer = func(answerTestRequest) ([]byte, error) { return nil, makeCollision() }
	}
	// Required payload and run-artifact paths are independent of the collision.
	for _, dir := range []string{filepath.Join(r.opts.Executor.RootDir, llm.CacheDirectoryName, "payloads"), filepath.Join(r.opts.OwnerRunDir, atlas.TablesDir)} {
		probe := filepath.Join(dir, "write-probe")
		if err := os.WriteFile(probe, []byte("writable"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(probe); err != nil {
			t.Fatal(err)
		}
	}
	return r, p, cold
}

func assertAcceptedAnswerArtifacts(t *testing.T, r *reader, expected llm.Outcome[table.Result], source string) {
	t.Helper()
	dir := filepath.Join(r.opts.OwnerRunDir, atlas.TablesDir)
	results, err := filepath.Glob(filepath.Join(dir, "atlas_answer*.result.json"))
	if err != nil || len(results) != 1 {
		t.Fatalf("saved results: %v / %v", results, err)
	}
	raw, err := os.ReadFile(results[0])
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Source string        `json:"source"`
		Reason string        `json:"reason"`
		Rows   table.Answers `json:"rows"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || result.Source != source || result.Reason != "" || !reflect.DeepEqual(result.Rows, expected.Value.Answers) {
		t.Fatalf("accepted result changed: %s / %v", raw, err)
	}
	for _, payload := range []struct {
		name string
		want []byte
	}{{"request", expected.Request}, {"response", expected.Response}} {
		refs, err := filepath.Glob(filepath.Join(dir, "atlas_answer*."+payload.name+".ref.json"))
		if err != nil || len(refs) != 2 {
			t.Fatalf("missing shared and question-keyed %s references: %v / %v", payload.name, refs, err)
		}
		for _, filename := range refs {
			raw, err := readWindowPayload(filename)
			if err != nil || !bytes.Equal(raw, payload.want) {
				t.Fatalf("exact accepted %s payload not preserved: %v", payload.name, err)
			}
		}
	}
}

func TestAnswerCacheRecordFailuresPreserveAcceptedAnswer(t *testing.T) {
	for _, kind := range []string{"read", "write"} {
		t.Run(kind, func(t *testing.T) {
			r, p, expected := answerCacheRecordCollision(t, kind)
			acceptedEvents := 0
			r.opts.Executor.Observer = llm.ObserverFunc(func(event llm.Event) error {
				if event.Kind == llm.EventLive {
					acceptedEvents++
				}
				return nil
			})
			var warnings []string
			r.opts.State = func(stage, state string, details ...string) {
				if stage == lines.StageAnswer && strings.HasPrefix(state, "cache ") {
					warnings = append(warnings, state)
					if len(details) == 0 || details[0] == "" {
						t.Error("cache warning lost its diagnostic")
					}
				}
			}
			if err := r.readAnswers(context.Background()); err != nil {
				t.Fatalf("accepted answer discarded after optional cache %s failure: %v", kind, err)
			}
			if acceptedEvents != 1 || len(p.requests) != 2 {
				t.Fatalf("accepted completions=%d calls=%d; want one after prime", acceptedEvents, len(p.requests))
			}
			if !reflect.DeepEqual(warnings, []string{"cache " + kind + " failed"}) {
				t.Fatalf("missing optional-cache diagnostic: %v", warnings)
			}
			answer := r.questions[0].Answer
			if answer.State != "partial" || len(answer.Parts) != 1 {
				t.Fatalf("accepted answer not restored: %#v", answer)
			}
			part, value := answer.Parts[0], expected.Value.Answers[0]
			if part.Source != atlas.SourceModel || part.OriginRequest != expected.RequestSHA256 || part.OriginRow != "r1" || part.Text != value["answer"] || part.Basis != value["basis"] || part.Remaining != value["remaining"] {
				t.Fatalf("accepted content or provenance changed: %+v", part)
			}
			if !reflect.DeepEqual(part.Steps, []atlas.QuestionStep{{Path: "source-00.go", Line: 1, StopIndexes: []int{0}}}) || !reflect.DeepEqual(r.questions[0].Stops, answerTestRoutes(1)[0].Stops) || !reflect.DeepEqual(r.questions[0].Guide.Steps, part.Steps) {
				t.Fatalf("original source restoration changed: %+v", r.questions[0])
			}
			if len(r.rejected) != 0 {
				t.Fatalf("accepted answer was marked rejected: %+v", r.rejected)
			}
			assertAcceptedAnswerArtifacts(t, r, expected, atlas.SourceModel)
			if kind == "read" {
				// Corruption was evicted, then the accepted response was cached.
				// A fresh reading restores it without paying for another answer.
				warm := answerTestReader(t, answerTestRoutes(1), p)
				warm.opts.Executor.RootDir = r.opts.Executor.RootDir
				warm.opts.State = r.opts.State
				warnings = nil
				if err := warm.readAnswers(context.Background()); err != nil {
					t.Fatal(err)
				}
				if len(p.requests) != 2 || len(warnings) != 0 || len(warm.questions[0].Answer.Parts) != 1 {
					t.Fatalf("warm recovery failed: calls=%d warnings=%v answer=%+v", len(p.requests), warnings, warm.questions[0].Answer)
				}
				want := part
				want.Source = atlas.SourceCache
				if !reflect.DeepEqual(warm.questions[0].Answer.Parts[0], want) {
					t.Fatalf("warm response changed: %+v", warm.questions[0].Answer.Parts[0])
				}
				assertAcceptedAnswerArtifacts(t, warm, expected, atlas.SourceCache)
			}
		})
	}
}

func TestAnswerObserverFailureRemainsTerminal(t *testing.T) {
	p := &answerTestProvider{tableProvider: &tableProvider{}}
	r := answerTestReader(t, answerTestRoutes(1), p)
	want := errors.New("required answer observer unavailable")
	r.opts.Executor.Observer = llm.ObserverFunc(func(event llm.Event) error {
		if event.Kind == llm.EventLive {
			return want
		}
		return nil
	})
	if err := r.readAnswers(context.Background()); !errors.Is(err, want) {
		t.Fatalf("mandatory observer failure was hidden: %v", err)
	}
}
