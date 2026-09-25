package reading

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/questionbatch"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

func TestRefusedQuestionIsJournaledOnceAndRetainedInSummary(t *testing.T) {
	opts, provider := questionFixture(t)
	opts.WindowRows = 0
	opts.Questions = []string{"Where is state stored?", "Where is it encrypted?"}
	// A cell the model answered badly is refused in place; the question keeps
	// its other rows. (A question the model left out is re-asked instead; see
	// TestOmittedQuestionIsRecovered….)
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
	if len(result.Questions) != 2 || len(result.Questions[0].Stops) != 4 || len(result.Questions[1].Stops) != 3 || result.Questions[1].Coverage.UnresolvedChunks != 1 {
		t.Fatalf("refused cell affected its accepted neighbours: %+v", result.Questions)
	}
	if err := modeldiag.Append(opts.OwnerRunDir, result.Rejected); err != nil {
		t.Fatal(err)
	}
	rows, err := modeldiag.Read(opts.OwnerRunDir)
	if err != nil || len(rows) != 1 || rows[0].Kind != "question_rejected" || rows[0].Count != 1 {
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

// A shared question window whose accepted answer had a cell refused is what
// the journal's row points at (owner, 2026-09-26). The run that asked and the
// warm run that reuses the remembered answer each keep that exchange's
// payloads, so after cache clear every row and every ref of the question
// windows still lead to bytes. (The warm run asks the refused cell's row
// again; that answer is refused whole.)
func TestPartlyRefusedQuestionWindowStaysInItsRun(t *testing.T) {
	opts, provider := questionFixture(t)
	opts.WindowRows = 0
	opts.Questions = []string{"Where is state stored?", "Where is it encrypted?"}
	provider.questionBatchFor = func(request questionBatchRequest, response questionbatch.Response) questionbatch.Response {
		for i, question := range request.Questions {
			if question.Question == opts.Questions[1] {
				response.Questions[i].Selections[0].Relevance = "invalid"
			}
		}
		return response
	}
	base := t.TempDir()
	read := func(name string) (string, int) {
		t.Helper()
		writer, err := debugdump.NewWriter(base, name)
		if err != nil {
			t.Fatal(err)
		}
		defer writer.Close()
		run := opts
		run.OwnerRunDir = filepath.Join(base, name)
		run.Executor.Observer = debugdump.NewSemanticObserver(writer)
		result, err := Read(t.Context(), run)
		if err != nil {
			t.Fatal(err)
		}
		if err := modeldiag.Append(run.OwnerRunDir, result.Rejected); err != nil {
			t.Fatal(err)
		}
		reused := 0
		for _, use := range result.Uses {
			if use.Stage == lines.StageQuestion {
				reused = use.Reused
			}
		}
		return run.OwnerRunDir, reused
	}
	cold, _ := read("cold")
	warm, reused := read("warm")
	if reused == 0 {
		t.Fatal("warm reading did not reuse the remembered answer")
	}
	if err := os.RemoveAll(filepath.Join(opts.Executor.RootDir, llm.CacheDirectoryName)); err != nil {
		t.Fatal(err)
	}
	for _, run := range []string{cold, warm} {
		rows, err := modeldiag.Read(run)
		if err != nil || !slices.ContainsFunc(rows, func(row modeldiag.Row) bool { return row.Kind == "question_rejected" }) {
			t.Fatalf("%s: the refused cell was not journaled: %+v / %v", filepath.Base(run), rows, err)
		}
		for _, row := range rows {
			if row.ResponseRef == "" {
				continue
			}
			ref := filepath.Join(run, filepath.FromSlash(row.ResponseRef))
			if strings.HasSuffix(ref, ".ref.json") {
				if body, err := readWindowPayload(ref); err != nil || len(body) == 0 {
					t.Errorf("%s: after cache clear %s leads nowhere: %v", filepath.Base(run), row.ResponseRef, err)
				}
				continue
			}
			raw, err := os.ReadFile(ref)
			if err != nil {
				t.Fatal(err)
			}
			var record debugdump.SemanticExchangeRecord
			if err := json.Unmarshal(raw, &record); err != nil {
				t.Fatal(err)
			}
			for _, payload := range []debugdump.SemanticPayloadRecord{record.Request, record.Response} {
				if body, err := os.ReadFile(filepath.Join(filepath.Dir(ref), payload.File)); err != nil || len(body) == 0 {
					t.Errorf("%s: after cache clear the %s row's %s leads nowhere: %v", filepath.Base(run), row.Kind, row.ResponseRef, err)
				}
			}
		}
		refs, err := filepath.Glob(filepath.Join(run, atlas.TablesDir, lines.StageQuestion+"*.ref.json"))
		if err != nil || len(refs) == 0 {
			t.Fatalf("%s: no question window refs: %v", filepath.Base(run), err)
		}
		for _, ref := range refs {
			if body, err := readWindowPayload(ref); err != nil || len(body) == 0 {
				t.Errorf("%s: after cache clear %s leads nowhere: %v", filepath.Base(run), filepath.Base(ref), err)
			}
		}
	}
}
