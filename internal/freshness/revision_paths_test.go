package freshness

import (
	"context"
	"errors"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
)

func TestCorpusPathsUseCapturedRevisionNotCurrentHeadOrIndex(t *testing.T) {
	root := testRepository(t)
	writeTestFile(t, root, "main.py", "def main(): pass\n")
	gitTest(t, root, "add", "main.py")
	gitTest(t, root, "commit", "-m", "initial")
	state, err := captureTestRepository(t, root)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "new.py", "def new(): pass\n")
	repository, err := corpus.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	for _, stage := range []string{"untracked", "staged", "committed after capture"} {
		switch stage {
		case "staged":
			gitTest(t, root, "add", "new.py")
		case "committed after capture":
			gitTest(t, root, "commit", "-m", "later")
		}
		missing, err := CorpusPathOutsideRevision(t.Context(), root, repository, state)
		if err != nil || missing != "new.py" {
			t.Fatalf("%s: absent path = %q, error = %v", stage, missing, err)
		}
	}
	current, err := CaptureRepository(t.Context(), root, repository)
	if err != nil {
		t.Fatal(err)
	}
	if missing, err := CorpusPathOutsideRevision(t.Context(), root, repository, current); err != nil || missing != "" {
		t.Fatalf("committed path missing from current revision: %q, %v", missing, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := CorpusPathOutsideRevision(ctx, root, repository, current); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled revision lookup = %v", err)
	}
}
