package modeldiag

import "testing"

func TestAlreadyJournaledRowsRemainInSummaryButAreNotAppendedTwice(t *testing.T) {
	dir := t.TempDir()
	row := Row{Stage: "atlas_question", Kind: "question_rejected", Count: 28, Samples: []string{"q1"}, Reason: "conflicting relevance"}
	if err := Append(dir, []Row{row}); err != nil {
		t.Fatal(err)
	}
	row.AlreadyJournaled = true
	if len(Summary([]Row{row})) != 1 {
		t.Fatal("summary lost rejection")
	}
	if err := Append(dir, []Row{row}); err != nil {
		t.Fatal(err)
	}
	got, err := Read(dir)
	if err != nil || len(got) != 1 || got[0].Count != 28 {
		t.Fatalf("duplicate rejection or wrong coverage: %+v %v", got, err)
	}
}
