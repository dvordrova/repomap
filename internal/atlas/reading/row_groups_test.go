package reading

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

func TestRowGroupsPackTheirOwnWindowsInOneBatch(t *testing.T) {
	provider := &tableProvider{}
	r := answerTestReader(t, nil, provider)
	r.opts.Through = ""
	def := lines.ZoneLines()
	groups := rowGroups{
		{shared: []table.Field{{Name: "question", Value: "lines"}, {Name: "owner", Value: "first"}},
			rows: []table.Row{{ID: "a", Fields: []table.Field{{Name: "part", Value: "A"}}}, {ID: "b", Fields: []table.Field{{Name: "part", Value: "B"}}}}},
		{shared: []table.Field{{Name: "question", Value: "lines"}, {Name: "owner", Value: "second"}},
			rows: []table.Row{{ID: "c", Fields: []table.Field{{Name: "part", Value: "C"}}}}},
	}
	answers, err := r.runTableGroups(t.Context(), def, 1, groups, nil)
	if err != nil || len(answers) != 3 {
		t.Fatalf("answers: %+v / %v", answers, err)
	}
	// Each group is its own window with its own keys; the answers come back
	// in the groups' row order.
	if answers[0].answer["line"] != "Text for r1" || answers[1].answer["line"] != "Text for r2" || answers[2].answer["line"] != "Text for r1" {
		t.Fatalf("answers lost their group order: %+v", answers)
	}
	if use := r.use(def.Stage); use.Windows != 2 || use.Rows != 3 || provider.calls != 2 || answers[2].source != atlas.SourceModel {
		t.Fatalf("groups did not form two windows of one batch: %+v / calls=%d", use, provider.calls)
	}
	for _, name := range []string{"atlas_zones-r1-w0.input.ref.json", "atlas_zones-r1-w1.input.ref.json"} {
		if _, err := os.Stat(filepath.Join(r.opts.OwnerRunDir, atlas.TablesDir, name)); err != nil {
			t.Fatalf("window files are not numbered across groups: %v", err)
		}
	}
	if printed := r.tables.String(); strings.Count(printed, `context owner: "first"`) != 1 || strings.Count(printed, `context owner: "second"`) != 1 {
		t.Fatalf("tables.md lost a group's context:\n%s", printed)
	}
}
