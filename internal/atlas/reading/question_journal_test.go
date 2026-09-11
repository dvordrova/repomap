package reading

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/questionbatch"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

func TestRefusedQuestionIsJournaledOnceAndRetainedInSummary(t *testing.T) {
	opts, provider := questionFixture(t)
	opts.WindowRows = 0
	opts.Questions = []string{"Where is state stored?", "Where is it encrypted?"}
	// A question the model answered badly is refused in place. (A question the
	// model left out is re-asked instead; see TestOmittedQuestionIsRecovered….)
	provider.questionBatchFor = func(request questionBatchRequest, response questionbatch.Response) questionbatch.Response {
		for i, question := range request.Questions {
			if question.Question == opts.Questions[1] {
				response.Questions[i].Selections[0].Relevance = "invalid"
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

// A question the model left out of a shared response is asked again over the
// same rows. Once answered there, neither the owner's summary nor the shared
// journal counts the first-round omission as a loss, and the question is
// complete for the answer stage.
func TestOmittedQuestionIsRecoveredByReaskAndNotJournaledAsLoss(t *testing.T) {
	opts, provider := questionFixture(t)
	opts.WindowRows = 0
	opts.Questions = []string{"Where is state stored?", "Where is it encrypted?"}
	omitted := false
	provider.questionBatchFor = func(request questionBatchRequest, response questionbatch.Response) questionbatch.Response {
		for i, question := range request.Questions {
			if question.Question == opts.Questions[1] && !omitted {
				omitted = true
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
	var messages []string
	opts.State = func(stage, state string, details ...string) {
		messages = append(messages, stage+": "+state+"\n"+strings.Join(details, "\n"))
	}
	result, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 2 || len(result.Rejected) != 0 {
		t.Fatalf("omission was not re-asked once or was counted as a loss: calls=%d rejected=%+v", provider.calls, result.Rejected)
	}
	for _, route := range result.Questions {
		if len(route.Stops) != 4 || route.Coverage.InspectedChunks != 4 || route.Coverage.UnresolvedChunks != 0 {
			t.Fatalf("re-asked question is not complete: %+v", route)
		}
	}
	log := strings.Join(messages, "\n")
	for _, want := range []string{
		"questions in this response: 1 accepted, 0 rejected, 1 omitted by the model and re-asked (1 recovered)",
		"question: Where is it encrypted?\nomitted by the model: question batch: missing question q2",
		"re-asked 1 questions omitted by the model in 1 windows; 1 recovered",
		"questions: 2 with sources, 0 with no sources selected, 0 with incomplete selection, 0 unavailable",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("question log omitted %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "state: response rejected") || strings.Contains(log, "\nreason:") {
		t.Fatalf("recovered omission reported as a rejection:\n%s", log)
	}
	rows, err := modeldiag.Read(opts.OwnerRunDir)
	if err != nil || len(rows) != 1 || rows[0].Kind != "question_omitted" || rows[0].Count != 4 || rows[0].Samples[0] != "q2" {
		t.Fatalf("shared journal counted the recovered omission as a loss: %+v / %v", rows, err)
	}
}
