package claims

import (
	"github.com/dvordrova/repomap/internal/corpus"
	"path/filepath"
	"testing"
)

func TestClojureCumulativeAuthorDocstring(t *testing.T) {
	root, err := filepath.Abs("../../testdata/repositories/clojure")
	if err != nil {
		t.Fatal(err)
	}
	repository, err := corpus.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	ref, ok := repository.ID("src/example/service.cljc")
	if !ok {
		t.Fatal("source absent")
	}
	info, _ := repository.Info(ref)
	quotes, err := fileQuotes(repository, info.Entry)
	if err != nil {
		t.Fatal(err)
	}
	for _, quote := range quotes {
		if quote.Source == SourceDocstring && quote.Line == 5 && quote.Text == "Build the greeting before delivery." {
			return
		}
	}
	t.Fatalf("native source author quote missing: %+v", quotes)
}
