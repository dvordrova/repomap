package run

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

// replayAnswerProvider answers every replayed request with its next answer.
type replayAnswerProvider struct{ answers [][]byte }

func (*replayAnswerProvider) State() []byte { return []byte(`{"model":"replay-test"}`) }
func (*replayAnswerProvider) Prepare(llm.Prompt, llm.Limits) (llm.Prepared, error) {
	return llm.Prepared{}, nil
}
func (p *replayAnswerProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	answer := p.answers[0]
	p.answers = p.answers[1:]
	return llm.Completion{Response: answer, ChoiceCount: 1, FinishReason: llm.FinishStop, Metrics: llm.Metrics{Attempts: 1}}, nil
}

// An accepted replay refreshes the cache and names its stored payloads; a
// refused one stores nothing there: its answer is on stdout and its request
// is the file the developer gave.
func TestRefusedReplayLeavesNothingInTheCache(t *testing.T) {
	debugDir := t.TempDir()
	request := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(request, []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}],"max_tokens":16,"temperature":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := &replayAnswerProvider{answers: [][]byte{[]byte(`{"ok":true}`), []byte("not json")}}
	factory := func() (llm.Provider, error) { return provider, nil }
	replay := func() (string, string, error) {
		var stdout, stderr bytes.Buffer
		err := runReplayWithProvider(t.Context(), []string{"--file", request, "--debug-dir", debugDir}, &stdout, &stderr, factory)
		return stdout.String(), stderr.String(), err
	}
	payloads := func() []string {
		names, _ := filepath.Glob(filepath.Join(debugDir, llm.CacheDirectoryName, "payloads", "*"))
		return names
	}

	if _, console, err := replay(); err != nil || len(payloads()) != 2 || !strings.Contains(console, filepath.Join(debugDir, llm.CacheDirectoryName, "payloads")) {
		t.Fatalf("accepted replay: payloads %v, console:\n%s\nerr: %v", payloads(), console, err)
	}
	answer, console, err := replay()
	if err == nil || strings.TrimSpace(answer) != "not json" {
		t.Fatalf("refused replay accepted or its answer hidden: %q / %v", answer, err)
	}
	if got := payloads(); len(got) != 2 {
		t.Errorf("refused replay stored a payload in the cache: %v", got)
	}
	if !strings.Contains(console, request) || strings.Contains(console, filepath.Join(llm.CacheDirectoryName, "payloads")) {
		t.Errorf("refused replay must name the given request and no cache payload:\n%s", console)
	}
}
