package reading

import (
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/questionbatch"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

func TestRefusedQuestionIsJournaledOnceAndRetainedInSummary(t *testing.T) {
	opts, provider := questionFixture(t)
	opts.WindowRows = 0
	opts.Questions = []string{"Where is state stored?", "Where is it encrypted?"}
	provider.questionBatchFor = func(request questionBatchRequest, response questionbatch.Response) questionbatch.Response {
		for i, question := range request.Questions {
			if question.Question == opts.Questions[1] {
				response.Questions = append(response.Questions[:i], response.Questions[i+1:]...)
				break
			}
		}
		return response
	}
	base := t.TempDir()
	writer, err := debugdump.NewWriter(base, "reading")
	if err != nil {
		t.Fatal(err)
	}
	opts.OwnerRunDir = filepath.Join(base, "reading")
	opts.Executor.Observer = debugdump.NewSemanticObserver(writer)
	result, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rejected) != 1 || !result.Rejected[0].AlreadyJournaled || len(modeldiag.Summary(result.Rejected)) != 1 {
		t.Fatalf("owner lost rejection summary or emitted duplicate: %+v", result.Rejected)
	}
	if len(result.Questions) != 2 || len(result.Questions[0].Stops) == 0 || len(result.Questions[1].Stops) != 0 {
		t.Fatalf("refused question affected its accepted neighbour: %+v", result.Questions)
	}
	if err := modeldiag.Append(opts.OwnerRunDir, result.Rejected); err != nil {
		t.Fatal(err)
	}
	rows, err := modeldiag.Read(opts.OwnerRunDir)
	if err != nil || len(rows) != 1 || rows[0].Kind != "question_rejected" || rows[0].Count != 4 {
		t.Fatalf("shared journal has duplicate or lost coverage: %+v / %v", rows, err)
	}
}
