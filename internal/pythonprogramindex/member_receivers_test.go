package pythonprogramindex

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

// duck_receivers.py calls methods on values nothing types, as huey's Huey
// calls self.storage.enqueue on the storage create_storage builds: each
// call goes to the methods of the classes having every member the code
// uses on the value (basis members), and stays unresolved where the code
// names only the method, a builtin could be the value, the code tests the
// value's shape, a store is an outside value, or a candidate's member is
// no method.
func TestCumulativePythonCallsOnUntypedValuesGoToTheClassesHavingTheirMembers(t *testing.T) {
	const path = "src/fixture_app/duck_receivers.py"
	files := cumulativePythonSources(t, path)
	repository := pythonCorpus(t, files)
	index, err := buildOneForTest(t.Context(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	objects := map[string]programindex.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	lines := strings.Split(files[path], "\n")
	got := map[string]string{}
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.Location == nil || relation.Location.Path != path {
			continue
		}
		line := strings.TrimSpace(lines[relation.Location.Line-1])
		if !strings.Contains(line, "self.storage.") && !strings.Contains(line, "self.queue.") && !strings.Contains(line, "self.sink.") &&
			!strings.HasPrefix(line, "counts.") && !strings.HasPrefix(line, "return counts.") && !strings.Contains(line, "storage.dequeue()") &&
			!strings.Contains(line, "storage.size()") && !strings.Contains(line, "self.conn.") && !strings.Contains(line, "self.timer.") {
			continue
		}
		var names []string
		for _, id := range relation.ToIDs {
			names = append(names, objects[objects[id].OwnerID].Name+"."+objects[id].Name)
		}
		slices.Sort(names)
		row := strings.TrimSpace(string(relation.Resolution) + " " + strings.Join(names, " "))
		if relation.Basis != "" {
			row += " (" + relation.Dispatch + ", " + relation.Basis + ")"
		}
		got[line] = row
	}
	want := map[string]string{
		"return self.storage.enqueue(item)":              "alternatives FileStorage.enqueue MemoryStorage.enqueue (interface, members)",
		"return self.storage.dequeue()":                  "alternatives FileStorage.dequeue MemoryStorage.dequeue (interface, members)",
		"return self.storage.size()":                     "exact Storage.size (interface, members)",
		"return self.storage.enqueue(self.storage.path)": "exact FileStorage.enqueue (interface, members)",
		"item = self.queue.take()":                       "exact Queue.take (interface, members)",
		"return self.queue.put(item)":                    "exact Queue.put (interface, members)",
		"return self.sink.write(text)":                   "unresolved",
		"counts.get(\"total\")":                          "unresolved",
		"return counts.items()":                          "unresolved",
		"storage.size()":                                 "unresolved",
		"return storage.dequeue()":                       "unresolved",
		"self.conn.execute(statement)":                   "unresolved",
		"return self.conn.commit()":                      "unresolved",
		"self.timer.reset()":                             "alternatives Counter.reset Metronome.reset (interface, members)",
		"return self.timer.tick()":                       "unresolved",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls on untyped values in duck_receivers.py:\n have %q\n want %q", got, want)
	}
	for _, relation := range index.Relations {
		if relation.Basis != programindex.BasisMembers || relation.Location == nil || relation.Location.Path != path {
			continue
		}
		used := ""
		for _, witness := range relation.Witnesses {
			if witness.Kind == "members_used" {
				used = witness.Detail
			}
		}
		if strings.TrimSpace(lines[relation.Location.Line-1]) == "return self.storage.enqueue(item)" && used != "dequeue, enqueue, size used on self.storage" {
			t.Fatalf("members witness of Queue.put = %q", used)
		}
		if used == "" {
			t.Fatalf("members-basis call at line %d names no members used", relation.Location.Line)
		}
	}
}

// subclass_calls.py calls methods a subclass runs, as freqtrade's bot calls
// its strategy's abstract populate_indicators: an abstract method's call,
// and a concrete one a subclass overrides, go to the method each concrete
// class the receiver's class is or derives into runs (basis overrides); a
// protocol's to each class having every member it declares (basis
// implements). A call naming the class and a method nothing implements
// keep their exact call (super() is resolved by no rule yet); a test's
// class serves only the test.
func TestCumulativePythonCallsASubclassRunsGoToItsMethods(t *testing.T) {
	const path = "src/fixture_app/subclass_calls.py"
	const testPath = "tests/test_exporters.py"
	files := cumulativePythonSources(t, path, testPath)
	repository := pythonCorpus(t, files)
	index, err := buildOneForTest(t.Context(), repository, targetOfKind(t, repository, pythontarget.KindLibrary))
	if err != nil {
		t.Fatal(err)
	}
	objects := map[string]programindex.Object{}
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	lines := map[string][]string{path: strings.Split(files[path], "\n"), testPath: strings.Split(files[testPath], "\n")}
	got := map[string]string{}
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationCalls || relation.Location == nil || lines[relation.Location.Path] == nil || relation.Invocation != "" {
			continue
		}
		line := strings.TrimSpace(lines[relation.Location.Path][relation.Location.Line-1])
		var names []string
		for _, id := range relation.ToIDs {
			if owner, ok := objects[objects[id].OwnerID]; ok {
				names = append(names, owner.Name+"."+objects[id].Name)
			}
		}
		if len(names) == 0 || !strings.Contains(line, ".") {
			continue
		}
		slices.Sort(names)
		row := strings.TrimSpace(string(relation.Resolution) + " " + strings.Join(names, " "))
		if relation.Basis != "" {
			row += " (" + relation.Dispatch + ", " + relation.Basis + ")"
		}
		got[line] = row
	}
	exporters := "alternatives CsvExporter.export FileExporter.export JsonExporter.export QuietCsvExporter.export (interface, overrides)"
	want := map[string]string{
		"return self.export(rows)":                  exporters,
		"return exporter.export(rows)":              exporters,
		"return self.export([])":                    "exact FileExporter.export (interface, overrides)",
		"return exporter.export([])":                "exact FileExporter.export (interface, overrides)",
		"loader.describe()":                         "exact Loader.describe",
		"return loader.load(name)":                  "exact DiskLoader.load (interface, overrides)",
		`self.log("filtering")`:                     "exact Logged.log",
		"pairs = handler.filter(pairs)":             "alternatives AgeFilter.filter Filter.filter (interface, overrides)",
		"return CsvExporter.export(exporter, rows)": "exact CsvExporter.export",
		"return sink.write(text)":                   "alternatives FileSink.write NamedSink.write (interface, implements)",
		"return target.store(item)":                 "exact Archive.store",
		`return exporter.export(["b"])`:             "alternatives CsvExporter.export FileExporter.export JsonExporter.export MockExporter.export QuietCsvExporter.export (interface, overrides)",
	}
	for line := range got {
		if _, listed := want[line]; !listed {
			delete(got, line)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls a subclass runs in subclass_calls.py:\n have %q\n want %q", got, want)
	}
}
