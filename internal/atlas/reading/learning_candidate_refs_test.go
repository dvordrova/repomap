package reading

import (
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

// A selection reason that names candidates by their request-local keys
// reaches the reader with the questions spelled out; keys outside the pool
// stay as written.
func TestSelectionReasonSpellsCandidateKeys(t *testing.T) {
	questions := []atlas.LearningQuestion{{Question: "How is the server started?"}, {Question: "Where is PG_URL read?"}, {Question: "What does the outbox relay do?"}}
	pool := []int{2, 0}
	got := spellCandidateRefs("q1 and q2 overlap; q7 is unknown, and eq1 is not a key", pool, questions)
	want := "«What does the outbox relay do?» and «How is the server started?» overlap; q7 is unknown, and eq1 is not a key"
	if got != want {
		t.Fatalf("spellCandidateRefs = %q, want %q", got, want)
	}
}
