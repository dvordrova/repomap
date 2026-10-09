package jstsproject

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex"
)

// TypeFest's Jsonify has a long attached JSDoc. Its quote must survive the
// native compiler and claims owners, without becoming the file's caption
// or the following alias's quote. A variable named a keeps its type any.
func TestCumulativeJSTSAttachedAuthorCommentKeepsItsExactDeclaration(t *testing.T) {
	for _, crlf := range []bool{false, true} {
		name := "LF"
		if crlf {
			name = "CRLF"
		}
		t.Run(name, func(t *testing.T) { assertCumulativeJSTSAttachedAuthorComment(t, crlf) })
	}
}
func assertCumulativeJSTSAttachedAuthorComment(t *testing.T, crlf bool) {
	root := preparedCompilerProject(t)
	tracked := []string{"package.json", "tsconfig.json", "src/type-members.ts"}
	for _, path := range tracked {
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		if crlf && path == "src/type-members.ts" {
			data = []byte(strings.ReplaceAll(string(data), "\n", "\r\n"))
		}
		writeTestFile(t, root, path, string(data))
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	git("init", "--quiet")
	git(append([]string{"add", "--"}, tracked...)...)
	git("-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--quiet", "--no-gpg-sign", "-m", "fixture")
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, index, catalog, err := Build(t.Context(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	quoted, err := claims.Extract(t.Context(), claims.Input{Repository: repository, RepoPath: root, Revision: "HEAD", ReadIndexes: []func() (programindex.Index, error){func() (programindex.Index, error) { return index, nil }}})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index, Dependencies: &catalog}}, Claims: quoted})
	if err != nil {
		t.Fatal(err)
	}
	const source = "src/type-members.ts"
	const purpose = "Serialize a pair while retaining its two declared types."
	var attached claims.Claim
	for _, claim := range quoted.Claims {
		if claim.Path == source && strings.HasPrefix(claim.Text, purpose) {
			attached = claim
		}
	}
	if attached.DeclarationLine-attached.Line <= 12 {
		t.Fatalf("long author quote lost its exact declaration: %+v", attached)
	}
	want := map[string]string{
		"SerializedPair": attached.Text, "UndocumentedPair": "", "a": "",
		"authoredCount": "Counts an authored level.", "readAuthoredCount": "Read the authored count.", "undocumentedCount": "",
		"AuthoredAfterBlank": "A blank line still leaves compiler-owned documentation.", "UndocumentedAfterBlank": "",
		"AuthoredShape":  "Names the authored response shape.",
		"AuthoredReader": "Holds two contrasting methods.", "AuthoredReader.read": "Reads the authored level.", "AuthoredReader.other": "",
		"sharedCommentFirst": "Describes two variables in one source declaration.", "sharedCommentSecond": "",
		"inlineOwned": "Counts inline levels.", "inlineUndocumented": "",
		"afterInlineOwned": "Reads the next independent level.", "afterInlineUndocumented": "",
		"sameLineListFirst": "Describes the first same-line variable.", "sameLineListSecond": "",
		"insideDeclarationOwned": "Counts a compiler-owned inner comment.",
		"inlinePairFirst":        "Describes the first inline declaration.", "inlinePairSecond": "Describes the second inline declaration.",
		"unicodeLead": "", "unicodeOwned": "Reports the Unicode-aware source position.",
	}
	seen := map[string]bool{}
	for _, place := range graph.Places {
		if place.Path != source {
			continue
		}
		if place.File != nil && place.File.Doc != "" {
			t.Fatalf("attached author quote became module documentation: %q", place.File.Doc)
		}
		if place.Symbol == nil {
			continue
		}
		decl := place.Symbol.Decl
		if doc, check := want[decl.Name]; check {
			seen[decl.Name] = true
			if decl.Doc != doc {
				t.Errorf("%s: quote %q, want %q", decl.Name, decl.Doc, doc)
			}
			if decl.Name == "SerializedPair" && decl.LineNo != attached.DeclarationLine {
				t.Errorf("author quote names declaration %d, native alias is at %d", attached.DeclarationLine, decl.LineNo)
			}
			if decl.Name == "a" && decl.Signature != "any" {
				t.Errorf("a's complete compiler type = %q", decl.Signature)
			}
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("native declarations missing: %v", seen)
	}
	objects := make(map[string]programindex.Object)
	for _, object := range index.Objects {
		if object.Location != nil && object.Location.Path == source {
			objects[object.Name] = object
		}
	}
	for _, name := range []string{"authoredCount", "readAuthoredCount", "AuthoredAfterBlank", "AuthoredShape", "AuthoredShape.count", "AuthoredReader.read", "inlineOwned", "afterInlineOwned", "sameLineListFirst", "insideDeclarationOwned", "inlinePairFirst", "inlinePairSecond", "unicodeOwned"} {
		object := objects[name]
		if object.Location == nil || len(object.DocstringRanges) != 1 {
			t.Fatalf("%s: compiler attachment missing: %+v", name, object)
		}
		found := false
		for _, claim := range quoted.Claims {
			if claim.Path == source && claim.Line == object.DocstringRanges[0].Line && claim.Column == object.DocstringRanges[0].Column {
				found = claim.DeclarationLine == object.Location.Line && claim.DeclarationColumn == object.Location.Column
			}
		}
		if !found {
			t.Fatalf("%s: original compiler name line does not own the quote", name)
		}
	}
	for _, name := range []string{"UndocumentedPair", "undocumentedCount", "UndocumentedAfterBlank", "AuthoredShape.next", "AuthoredReader.other", "inlineUndocumented", "afterInlineUndocumented", "sameLineListSecond", "unicodeLead"} {
		if object := objects[name]; object.Location == nil || len(object.DocstringRanges) != 0 {
			t.Fatalf("%s inherited another declaration's native comment: %+v", name, object)
		}
	}
	first, second := objects["sharedCommentFirst"], objects["sharedCommentSecond"]
	if first.Location == nil || second.Location == nil || first.Location.Line == second.Location.Line || len(first.DocstringRanges) != 1 || len(second.DocstringRanges) != 0 {
		t.Fatalf("variable-list documentation differs from actual compiler attachment: %+v %+v", first, second)
	}
	listSeen := false
	for _, claim := range quoted.Claims {
		if claim.Path == source && claim.Line == first.DocstringRanges[0].Line && claim.Column == first.DocstringRanges[0].Column {
			listSeen = true
			if claim.DeclarationLine != first.Location.Line || claim.DeclarationColumn != first.Location.Column || claim.Text != "Describes two variables in one source declaration." {
				t.Fatalf("native variable-list owner was guessed or its quote lost: %+v", claim)
			}
		}
	}
	if !listSeen {
		t.Fatal("variable-list author quote disappeared")
	}
	first, second = objects["sameLineListFirst"], objects["sameLineListSecond"]
	if first.Location == nil || second.Location == nil || first.Location.Line != second.Location.Line || first.Location.Column == second.Location.Column || len(first.DocstringRanges) != 1 || len(second.DocstringRanges) != 0 {
		t.Fatalf("same-line variable-list native attachment: %+v %+v", first, second)
	}
	first, second = objects["inlinePairFirst"], objects["inlinePairSecond"]
	if first.Location == nil || second.Location == nil || first.Location.Line != second.Location.Line || len(first.DocstringRanges) != 1 || len(second.DocstringRanges) != 1 || first.DocstringRanges[0] == second.DocstringRanges[0] {
		t.Fatalf("distinct inline native comments lost their exact ranges: %+v %+v", first, second)
	}
}
