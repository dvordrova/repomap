package jstsproject

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex"
)

// src/duck-receivers.js is Python's duck_receivers.py in JavaScript, the
// way bee-queue's _finishJob calls job.computeDelay() on a parameter the
// compiler types `any`: a call on such a field or parameter goes to the
// methods of the repository classes having every member the code uses on
// it (basis members), and stays unresolved where the code names only the
// method, tests the value's shape, a caller hands an object literal, or a
// candidate's member of that name is no method.
func TestCumulativeJSTSCallsOnUntypedValuesGoToTheClassesHavingTheirMembers(t *testing.T) {
	root := preparedCompilerProject(t)
	tracked := []string{"package.json", "tsconfig.json", "src/duck-receivers.js"}
	for _, path := range tracked {
		contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, path, string(contents))
	}
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, index, _, err := Build(t.Context(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	objects := map[string]programindex.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	source, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", "src", "duck-receivers.js"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(source), "\n")
	got := map[string]string{}
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.Location == nil || relation.Invocation != "" {
			continue
		}
		line := strings.TrimSpace(lines[relation.Location.Line-1])
		called := relation.Witnesses[len(relation.Witnesses)-1].SourceExpression
		if !slices.ContainsFunc([]string{"this.storage.", "this.queue.", "job.", "view.", "this.sink.", "storage.", "this.timer."}, func(receiver string) bool {
			return strings.HasPrefix(called, receiver)
		}) {
			continue
		}
		var names []string
		for _, id := range relation.ToIDs {
			names = append(names, objects[id].Name) // a method's name is Owner.method
		}
		slices.Sort(names)
		row := strings.TrimSpace(string(relation.Resolution) + " " + strings.Join(names, " "))
		if relation.Basis != "" {
			used := ""
			for _, witness := range relation.Witnesses {
				if witness.Kind == "members_used" {
					used = witness.Detail
				}
			}
			row += " (" + relation.Dispatch + ", " + relation.Basis + ": " + used + ")"
		}
		got[line+" "+called] = row
	}
	storage := "dequeue, enqueue, size used on this.storage"
	want := map[string]string{
		"put(item) { return this.storage.enqueue(item) } this.storage.enqueue":            "alternatives FileStorage.enqueue MemoryStorage.enqueue Storage.enqueue (interface, members: " + storage + ")",
		"take() { return this.storage.dequeue() } this.storage.dequeue":                   "alternatives FileStorage.dequeue MemoryStorage.dequeue Storage.dequeue (interface, members: " + storage + ")",
		"pending() { return this.storage.size() } this.storage.size":                      "exact Storage.size (interface, members: " + storage + ")",
		"flush() { return this.storage.enqueue(this.storage.path) } this.storage.enqueue": "exact FileStorage.enqueue (interface, members: dequeue, enqueue, path, size used on this.storage)",
		"const item = this.queue.take() this.queue.take":                                  "exact Queue.take (interface, members: put, take used on this.queue)",
		"return this.queue.put(item) this.queue.put":                                      "exact Queue.put (interface, members: put, take used on this.queue)",
		"const delay = failed ? job.computeDelay() : -1 job.computeDelay":                 "exact Job.computeDelay (interface, members: computeDelay, toData used on job)",
		"return delay >= 0 ? job.toData() : \"\" job.toData":                              "exact Job.toData (interface, members: computeDelay, toData used on job)",
		"view.draw() view.draw":                                         "unresolved",
		"return view.size() view.size":                                  "unresolved",
		"notify(text) { return this.sink.write(text) } this.sink.write": "unresolved",
		"if (\"size\" in storage) storage.size() storage.size":          "unresolved",
		"return storage.dequeue() storage.dequeue":                      "unresolved",
		"this.timer.reset() this.timer.reset":                           "alternatives Counter.reset Metronome.reset (interface, members: reset, tick used on this.timer)",
		"return this.timer.tick() this.timer.tick":                      "unresolved",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls on untyped values in duck-receivers.js:\n have %q\n want %q", got, want)
	}
}

// src/interface-calls.ts calls through values of interfaces the repository
// declares, as Go's RegisterTicketHandlerServer does: the targets are the
// methods of the repository's classes assignable to the interface (basis
// implements), a structural match included, an abstract class and a
// generic class not enumerated; an interface no class implements leaves
// the call unresolved.
func TestCumulativeJSTSInterfaceCallsFollowAssignableClasses(t *testing.T) {
	root := preparedCompilerProject(t)
	tracked := []string{"package.json", "tsconfig.json", "src/interface-calls.ts"}
	for _, path := range tracked {
		contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, path, string(contents))
	}
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: tracked, RegularPaths: tracked})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	_, index, _, err := Build(t.Context(), repository, root)
	if err != nil {
		t.Fatal(err)
	}
	objects := map[string]programindex.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	source, err := os.ReadFile(filepath.Join("..", "..", "testdata", "repositories", "jsts", "src", "interface-calls.ts"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(source), "\n")
	got := map[string]string{}
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.Location == nil || relation.Invocation != "" {
			continue
		}
		called := relation.Witnesses[len(relation.Witnesses)-1].SourceExpression
		if !slices.ContainsFunc([]string{"store.", "this.store.", "receipt.", "ledger."}, func(receiver string) bool {
			return strings.HasPrefix(called, receiver)
		}) {
			continue
		}
		var names []string
		for _, id := range relation.ToIDs {
			names = append(names, objects[id].Name)
		}
		slices.Sort(names)
		row := strings.TrimSpace(string(relation.Resolution) + " " + strings.Join(names, " "))
		if relation.Basis != "" {
			row += " (" + relation.Dispatch + ", " + relation.Basis + ")"
		}
		got[strings.TrimSpace(lines[relation.Location.Line-1])] = row
	}
	want := map[string]string{
		`store.put("k")`:                       "alternatives CachedStore.put DiskStore.put MemoryStore.put (interface, implements)",
		`return store.get("k")`:                "alternatives BaseStore.get DiskStore.get MemoryStore.get (interface, implements)",
		`run() { return this.store.get("a") }`: "alternatives BaseStore.get DiskStore.get MemoryStore.get (interface, implements)",
		`return receipt.print()`:               "exact PaperReceipt.print (interface, implements)",
		`ledger.record("entry")`:               "unresolved",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls through repository interfaces in interface-calls.ts:\n have %q\n want %q", got, want)
	}
}
