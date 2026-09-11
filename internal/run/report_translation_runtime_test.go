package run

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/modeldiag"
	"github.com/dvordrova/repomap/internal/reporttranslation"
)

func TestUntranslatedTextsAreJournaledAndNamedOnConsoleOnce(t *testing.T) {
	runDir := t.TempDir()
	var console bytes.Buffer
	untranslated := []reporttranslation.Untranslated{
		{Ref: "t38", Reason: "llm: reject response: report translation: missing translation for t38"},
		{Ref: "t52", Reason: "llm: provider complete failed (class=response attempts=1)"},
	}
	line := recordUntranslatedTexts(newRunOutput(&console), runDir, untranslated)
	if line != "2 texts kept in the source language: t38, t52" {
		t.Fatalf("console line: %q", line)
	}
	if console.Len() != 0 {
		t.Fatalf("journaling the kept texts warned:\n%s", console.String())
	}
	rows, err := modeldiag.Read(runDir)
	if err != nil || len(rows) != len(untranslated) {
		t.Fatalf("journal rows: %+v / %v", rows, err)
	}
	for i, row := range rows {
		if row.Stage != reporttranslation.StageName || row.Kind != "entry_untranslated" || row.Count != 1 ||
			!reflect.DeepEqual(row.Samples, []string{untranslated[i].Ref}) || row.Reason != untranslated[i].Reason {
			t.Fatalf("journal row %d lost its ref or reason: %+v", i, row)
		}
	}
	if line := recordUntranslatedTexts(newRunOutput(&console), runDir, untranslated[:1]); line != "1 text kept in the source language: t38" {
		t.Fatalf("singular console line: %q", line)
	}
	var many []reporttranslation.Untranslated
	for i := 1; i <= 12; i++ {
		many = append(many, reporttranslation.Untranslated{Ref: "t" + strings.Repeat("1", i), Reason: "refused"})
	}
	line = recordUntranslatedTexts(newRunOutput(&console), runDir, many)
	if !strings.HasPrefix(line, "12 texts kept in the source language: t1, t11, ") || !strings.HasSuffix(line, " … and 2 more in "+modeldiag.Filename) {
		t.Fatalf("bounded console line: %q", line)
	}
	if rows, err := modeldiag.Read(runDir); err != nil || len(rows) != 2+1+12 {
		t.Fatalf("journal did not keep every kept text: %d rows, %v", len(rows), err)
	}
}
