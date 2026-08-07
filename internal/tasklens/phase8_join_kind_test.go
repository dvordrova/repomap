package tasklens

import (
	"context"
	"strings"
	"testing"
)

// The prompt no longer instructs the model to copy relation endpoints or
// relation_kind for locally_observed joins — the backend restores them.
func TestPromptLocallyObservedBackendRestoresKind(t *testing.T) {
	repo := newTaskLensTestRepo(t, "prompt-join-kind-contract")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "Inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildSynthesisPrompt(bundle)
	if err != nil {
		t.Fatal(err)
	}
	text := prompt.System + "\n" + prompt.User
	if strings.Contains(text, "copy one supplied local relation ID") {
		t.Fatalf("prompt still instructs copying relation identity: %s", text)
	}
	if !strings.Contains(text, "the backend restores its endpoints, relation_kind") {
		t.Fatalf("prompt does not state backend restores relation_kind: %s", text)
	}
	if strings.Contains(text, `"relation_kind": "short relation name"`) {
		t.Fatalf("prompt schema still requires relation_kind echo: %s", text)
	}
}
