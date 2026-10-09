package contracttest

import (
	"reflect"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythonprogramindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// Two declarations one scope binds to one name (casdoor's two
// LoginPage.login.loginHandler arrows, in two callbacks of login; a Python
// def in each branch of an if) stand in one file and one part, so nothing
// where they stand tells them apart: each is named by the first thing only
// it of them calls (groupindex.OwnUses, the rung callables written alike
// are named by). Go cannot declare one name twice in a scope (its closures
// take this rung through their inline names), C declares a function once
// per file, and a Clojure namespace's second defn of a name is the same
// var.
func TestSameNamedDeclarationsOfOneScopeReadApartByWhatOnlyEachCalls(t *testing.T) {
	root, repository := materializeFixtureRepository(t, "jsts")
	_, jsts, _, err := sharedJSTSFixture(t, repository, root)
	if err != nil {
		t.Fatal(err)
	}
	_, pythonRepository := materializeFixtureRepository(t, "python")
	catalog, err := pythontarget.Discover(t.Context(), pythonRepository)
	if err != nil {
		t.Fatal(err)
	}
	var library pythontarget.Target
	for _, candidate := range catalog.Entries {
		if candidate.Kind == pythontarget.KindLibrary && candidate.ProjectDir == "." {
			library = candidate
			break
		}
	}
	input, err := pythonprogramindex.BuildInput(t.Context(), pythonRepository, library)
	if err != nil {
		t.Fatal(err)
	}
	python, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct {
		index      programindex.Index
		name, path string
		want       []string
	}{
		{jsts, "loginEither.loginHandler", "src/stored-callbacks.ts", []string{"acceptClient", "flushReplies"}},
		{python, "login_handler", "src/fixture_app/stored_callbacks.py", []string{"accept_client", "flush_replies"}},
	} {
		var ids []string
		for _, object := range check.index.Objects {
			if object.Name == check.name && object.Location != nil && object.Location.Path == check.path {
				ids = append(ids, object.ID)
			}
		}
		if len(ids) != 2 {
			t.Fatalf("%s: %d declarations named %s, want two", check.path, len(ids), check.name)
		}
		var words []string
		uses := groupindex.OwnUses(check.index, ids)
		if indexed := groupindex.NewOwnUseReader(check.index).OwnUses(ids); !reflect.DeepEqual(indexed, uses) {
			t.Fatalf("%s: indexed reader changed native names or source uses: %+v / %+v", check.path, indexed, uses)
		}
		for _, use := range uses {
			if use.Reads {
				t.Fatalf("%s: %s reads by what it reads, want what it calls: %+v", check.path, check.name, use)
			}
			words = append(words, use.Word)
		}
		slices.Sort(words)
		if !slices.Equal(words, check.want) {
			t.Fatalf("%s: %s told apart by %q, want %q", check.path, check.name, words, check.want)
		}
	}
}
