package jstsproject

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex/adaptertest"
)

// A chain starts every call at its leftmost receiver. Anchored there, split
// and join of `path.split("/").join("/")` became one boundary holding two
// facts of one target, and the second `on` of a chained registration was
// dropped as a repeat of the first.
func TestCumulativeJSTSChainedCallsKeepTheirOwnPositions(t *testing.T) {
	root := preparedCompilerProject(t)
	tracked := []string{"package.json", "tsconfig.json", "src/platform.ts", "src/server.ts"}
	for _, path := range tracked {
		contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, path, string(contents))
	}
	materializeCumulativeJSTSDependencyTypes(t, root)
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	result, _, _, err := Build(t.Context(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildInputFromResult(result)
	if err != nil {
		t.Fatal(err)
	}
	// const normalized = path.split("/").join("/")
	// const repeated = name.split("/").join("").split("/")
	// const head = path.split("/")[0].split("/")
	// join("") names no address, so it is no registration.
	const split, join = "platform:javascript.String.split", "platform:javascript.Array.join"
	adaptertest.AssertCallSiteBoundaries(t, repository, input, "src/platform.ts", []adaptertest.CallSite{
		{Line: 76, Column: 27, Key: "split", Text: split, Path: "/"},
		{Line: 76, Column: 38, Key: "join", Text: join, Path: "/"},
		{Line: 77, Column: 25, Key: "split", Text: split, Path: "/"},
		{Line: 77, Column: 45, Key: "split", Text: split, Path: "/"},
		{Line: 78, Column: 21, Key: "split", Text: split, Path: "/"},
		{Line: 78, Column: 35, Key: "split", Text: split, Path: "/"},
	})
	// createConsumer().on("orders.chained", handleOrder).on("orders.chained", recordOrder)
	adaptertest.AssertCallSiteBoundaries(t, repository, input, "src/server.ts", []adaptertest.CallSite{
		{Line: 168, Column: 20, Key: "on", Text: "@fixture/kafka-client.Consumer.on", Path: "orders.chained", Symbol: "handleOrder"},
		{Line: 168, Column: 54, Key: "on", Text: "@fixture/kafka-client.Consumer.on", Path: "orders.chained", Symbol: "recordOrder"},
	})
}
