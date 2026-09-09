package reporttranslation

import (
	"testing"

	"github.com/dvordrova/repomap/internal/report"
)

func TestTranslationHTTP500OnlySplitsDivisibleWindows(t *testing.T) {
	catalog := testCatalog(t, plainEntries(2))
	for _, size := range []int{1, 2} {
		call, err := translationCall(catalog.Entries[:size], report.Russian)
		if err != nil || call.SplitHTTP500 != (size > 1) {
			t.Fatalf("HTTP 500 split policy for %d complete texts: %v / %v", size, call.SplitHTTP500, err)
		}
	}
}
