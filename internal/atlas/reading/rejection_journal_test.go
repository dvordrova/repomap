package reading

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/modeldiag"
)

// A table response that refuses one row is one refusal and one journal row.
// The run's observer journals it with the exchange that holds the exact
// request and response; the reading keeps its own row only for the stage
// summary. Until 2026-09-29 rejected.jsonl also got the reading's row, under
// the window's response ref (the same response bytes) and with the row key
// twice in its samples.
func TestTableRowRefusalIsJournaledOnce(t *testing.T) {
	def := choiceTable()
	def.Stage = lines.StageRoleHelper
	rows := []table.Row{
		{ID: "first", Fields: []table.Field{{Name: "path", Value: "first.go"}}},
		{ID: "second", Fields: []table.Field{{Name: "path", Value: "second.go"}}},
	}
	read := func(journal *debugdump.Writer, runDir string) *reader {
		t.Helper()
		provider := &independentResponseProvider{response: []byte(`{"rows":[{"key":"first","role":"adapter"},{"key":"second","role":"u1"}]}`)}
		r := answerTestReader(t, nil, provider)
		if journal != nil {
			r.opts.OwnerRunDir = runDir
			if err := os.Mkdir(filepath.Join(runDir, atlas.TablesDir), 0o700); err != nil {
				t.Fatal(err)
			}
			r.opts.Executor.Observer = debugdump.NewSemanticObserver(journal)
		}
		r.opts.Through = ""
		r.places = make(map[string]atlas.Place)
		r.knowledge = make(map[string]*Knowledge)
		r.knowledgeSubjects = make(map[string]*Knowledge)
		r.responseTables = make(map[string]rememberedTable)
		for _, row := range rows {
			r.places[row.ID] = atlas.Place{ID: row.ID, Path: row.ID + ".go"}
		}
		answers, err := r.runIndependent(t.Context(), def, 1, rowGroups{{rows: rows}})
		if err != nil || answers[0].answer["role"] != "adapter" || answers[1].answer != nil {
			t.Fatalf("the refused row changed its neighbour: %+v / %v", answers, err)
		}
		if err := modeldiag.Append(r.opts.OwnerRunDir, r.rejected); err != nil {
			t.Fatal(err)
		}
		return r
	}

	base := t.TempDir()
	journal, err := debugdump.NewWriter(base, "run")
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	runDir := filepath.Join(base, "run")
	observed := read(journal, runDir)
	if len(observed.rejected) != 1 || !observed.rejected[0].AlreadyJournaled || modeldiag.Summary(observed.rejected)[0] != "atlas_role_helper row_rejected: 1 (second)" {
		t.Fatalf("the reading lost its summary of the refusal: %+v", observed.rejected)
	}
	journaled, err := modeldiag.Read(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(journaled) != 1 || journaled[0].Stage != def.Stage || journaled[0].Kind != "row_rejected" ||
		!slices.Equal(journaled[0].Samples, []string{"second"}) || !strings.HasPrefix(journaled[0].ResponseRef, debugdump.SemanticExchangesDir+"/") {
		t.Fatalf("one refused row is not one journal row pointing at its exchange: %+v", journaled)
	}

	// With no observer the reading's own row is the journal's one record.
	alone := read(nil, "")
	rowsAlone, err := modeldiag.Read(alone.opts.OwnerRunDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rowsAlone) != 1 || !slices.Equal(rowsAlone[0].Samples, []string{"second"}) || !strings.HasPrefix(rowsAlone[0].ResponseRef, atlas.TablesDir+"/") {
		t.Fatalf("without an observer the refusal is not journaled once by the reading: %+v", rowsAlone)
	}
}
