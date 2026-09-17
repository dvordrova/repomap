package lines

import (
	"slices"
	"testing"
)

// The talks cell has no "other" to fall into: a symbol that talks to
// nothing named leaves the cell out.
func TestTalksOffersNoOther(t *testing.T) {
	for _, def := range []struct{ handed bool }{{true}, {false}} {
		for _, column := range API(def.handed).Columns {
			if column.Name == "talks" && slices.Contains(column.Options, "other") {
				t.Fatalf("talks offers other: %v", column.Options)
			}
		}
	}
}
