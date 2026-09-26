package reading

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
)

func boxesByTitle(result Result) map[string]atlas.Box {
	boxes := map[string]atlas.Box{}
	for _, box := range result.Atlas.Targets[0].Boxes {
		boxes[box.Title] = box
	}
	return boxes
}

func boxFiles(box atlas.Box) string {
	var files []string
	for _, file := range box.Files {
		files = append(files, file.Path)
	}
	return strings.Join(files, " ")
}

func targetOf(t *testing.T, result Result, id string) atlas.Target {
	t.Helper()
	for _, target := range result.Atlas.Targets {
		if target.ID == id {
			return target
		}
	}
	t.Fatalf("no target %s", id)
	return atlas.Target{}
}

func offMapReasons(target atlas.Target) map[string]string {
	reasons := map[string]string{}
	for _, entry := range target.OffMap {
		reasons[entry.File.Path] = entry.Reason
	}
	return reasons
}

// Saved real parts answers of cmd/repomap replay through the decoder with
// their request-local refs mapped to sealed ones. Each keeps every good file:
// go-1 listed one file in two parts and go-2 left one out; only that file is
// asked again, and no answer is refused. The saved "about" is an extra field.
func TestSavedPartsAnswersKeepEveryGoodFile(t *testing.T) {
	var files struct {
		Files []struct {
			Ref, Path string
		}
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "parts-replay", "files.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &files); err != nil {
		t.Fatal(err)
	}
	listed := make([]string, len(files.Files))
	pathOf := map[string]string{}
	for i, file := range files.Files {
		listed[i] = file.Ref
		pathOf[file.Ref] = file.Path
	}
	for draw, want := range map[string]struct{ conflict, leftOut string }{
		"go-1": {conflict: "internal/repoconfig/config.go"},
		"go-2": {leftOut: "internal/atlas/destinations/destinations.go"},
		"go-4": {},
	} {
		raw, err := os.ReadFile(filepath.Join("testdata", "parts-replay", draw+".json"))
		if err != nil {
			t.Fatal(err)
		}
		answer, err := decodeParts(raw, listed)
		if err != nil {
			t.Fatalf("%s refused: %v", draw, err)
		}
		result := validatePartition(answer, listed)
		var asked []string
		for ref := range result.conflicts {
			asked = append(asked, "conflict "+pathOf[ref])
		}
		for _, ref := range result.leftOut {
			asked = append(asked, "left out "+pathOf[ref])
		}
		var expected []string
		if want.conflict != "" {
			expected = append(expected, "conflict "+want.conflict)
		}
		if want.leftOut != "" {
			expected = append(expected, "left out "+want.leftOut)
		}
		if !slices.Equal(asked, expected) || len(result.unknown) != 0 || len(result.refused) != 0 {
			t.Fatalf("%s: asked again %v, want %v; unknown %v refused %v", draw, asked, expected, result.unknown, result.refused)
		}
		placed := 0
		for _, part := range result.files {
			placed += len(part)
		}
		if placed != len(listed)-len(expected) {
			t.Fatalf("%s placed %d of %d files", draw, placed, len(listed))
		}
	}
}

// A partition is validated file by file: an unknown ref is discarded, a file
// named twice in one part is kept once, a file in two parts loses both
// memberships, a group without a name or without a list of files is not
// drawn and its files are asked again, two parts sharing a name over
// different files both stay. Only an answer that draws no part is refused
// whole: not JSON, no groups, or no group holding a listed file of its own.
func TestPartitionRefusesOnlyWhatIsWrong(t *testing.T) {
	listed := []string{"f1", "f2", "f3", "f4", "f5", "f6", "f7"}
	answer, err := decodeParts([]byte(`{"groups":[
		{"name":"Entry","files":["f1","f1","f9"]},
		{"name":"Store","files":["f2","f3"]},
		{"name":"store","files":["f3","f4"]},
		{"name":"","files":["f5"]},
		{"name":"Tools","files":"f6"},
		{"name":"Broken","files":{"f7":true}},
		{"name":"Ghost","files":["f99"]}]}`), listed)
	if err != nil {
		t.Fatal(err)
	}
	result := validatePartition(answer, listed)
	if len(result.files) != 4 || !slices.Equal(result.files[0], []string{"f1"}) || !slices.Equal(result.files[1], []string{"f2"}) ||
		!slices.Equal(result.files[2], []string{"f4"}) || !slices.Equal(result.files[3], []string{"f6"}) {
		t.Fatalf("placements: %v %v", result.names, result.files)
	}
	if !slices.Equal(result.conflicts["f3"], []int{1, 2}) || !slices.Equal(result.leftOut, []string{"f5", "f7"}) {
		t.Fatalf("conflicts %v left out %v", result.conflicts, result.leftOut)
	}
	if !slices.Equal(result.unknown, []string{"f9", "f99"}) || len(result.refused) != 3 || len(result.repeated) != 2 || len(result.repeatedGroups) != 0 {
		t.Fatalf("unknown %v refused %v repeated %v %v", result.unknown, result.refused, result.repeated, result.repeatedGroups)
	}
	for _, raw := range []string{
		`not json`, `{"groups":[]}`, `{"parts":[{"name":"A","files":["f1"]}]}`,
		`{"groups":[{"name":"Entry","files":["cmd/main.go","internal/store.go"]}]}`,
		`{"groups":[{"name":"","files":["f1","f2"]}]}`,
		`{"groups":[{"name":"A","files":["f1","f2"]},{"name":"B","files":["f1","f2"]}]}`,
	} {
		if _, err := decodeParts([]byte(raw), []string{"f1", "f2"}); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

// A group stated twice, with the same name ignoring case and the same set of
// listed files, is one answer: it is drawn once and the repeat is recorded.
// Equal names alone never make two groups one, and different names over one
// file never settle it: those files stay conflicts for the follow-up.
func TestIdenticalRepeatedGroupIsDrawnOnce(t *testing.T) {
	listed := []string{"f1", "f2", "f3"}
	partition := func(raw string) partition {
		t.Helper()
		answer, err := decodeParts([]byte(raw), listed)
		if err != nil {
			t.Fatalf("%s refused: %v", raw, err)
		}
		return validatePartition(answer, listed)
	}
	repeated := partition(`{"groups":[{"name":"Board","files":["f1","f2"]},{"name":" board ","files":["f2","f1","f9"]},{"name":"Search","files":["f3"]}]}`)
	if !slices.Equal(repeated.names, []string{"Board", "Search"}) || !slices.Equal(repeated.files[0], []string{"f1", "f2"}) ||
		!slices.Equal(repeated.files[1], []string{"f3"}) || len(repeated.conflicts) != 0 || len(repeated.leftOut) != 0 ||
		len(repeated.repeatedGroups) != 1 || len(repeated.repeated) != 0 {
		t.Fatalf("repeated group: %+v", repeated)
	}
	otherName := partition(`{"groups":[{"name":"Store","files":["f1"]},{"name":"Cache","files":["f1"]},{"name":"Rest","files":["f2","f3"]}]}`)
	if !slices.Equal(otherName.conflicts["f1"], []int{0, 1}) || len(otherName.repeatedGroups) != 0 {
		t.Fatalf("different names over one file: %+v", otherName)
	}
	otherFiles := partition(`{"groups":[{"name":"Board","files":["f1","f2"]},{"name":"Board","files":["f2","f3"]}]}`)
	if !slices.Equal(otherFiles.conflicts["f2"], []int{0, 1}) || !slices.Equal(otherFiles.files[0], []string{"f1"}) ||
		!slices.Equal(otherFiles.files[1], []string{"f3"}) || len(otherFiles.repeatedGroups) != 0 {
		t.Fatalf("one name over different files: %+v", otherFiles)
	}

	// An answer made only of one group stated twice draws it; two different
	// groups over the same files hold no file of their own and stay refused.
	answer, err := decodeParts([]byte(`{"groups":[{"name":"Board","files":["f1","f2"]},{"name":"Board","files":["f1","f2"]}]}`), []string{"f1", "f2"})
	if err != nil {
		t.Fatalf("a group stated twice was refused: %v", err)
	}
	if drawn := validatePartition(answer, []string{"f1", "f2"}); len(drawn.names) != 1 || !slices.Equal(drawn.files[0], []string{"f1", "f2"}) {
		t.Fatalf("a group stated twice: %+v", drawn)
	}
	if _, err := decodeParts([]byte(`{"groups":[{"name":"Board","files":["f1","f2"]},{"name":"Game","files":["f1","f2"]}]}`), []string{"f1", "f2"}); err == nil ||
		!strings.Contains(err.Error(), "no group holds a listed file of its own") {
		t.Fatalf("two groups over the same files: %v", err)
	}
}

// The target's map draws a group stated twice once and records the repeat
// as part_repeated_group; nothing is left for the follow-up.
func TestRepeatedGroupRecordedOnTheMap(t *testing.T) {
	var mu sync.Mutex
	placed := 0
	provider := &tableProvider{placeFor: func(row map[string]any) string {
		mu.Lock()
		placed++
		mu.Unlock()
		options, _ := row["part_options"].([]any)
		return fmt.Sprint(options[0])
	}}
	provider.partsResponse = func(files []map[string]any) string {
		var refs []string
		for _, file := range files {
			refs = append(refs, fmt.Sprintf("%q", file["ref"]))
		}
		if !strings.HasPrefix(fmt.Sprint(files[0]["path"]), "svc/") {
			return `{"groups":[{"name":"Client","files":[` + strings.Join(refs, ",") + `]}]}`
		}
		rest := refs[:len(refs)-1]
		reversed := slices.Clone(rest)
		slices.Reverse(reversed)
		return `{"groups":[{"name":"Board","files":[` + strings.Join(rest, ",") + `]},` +
			`{"name":"board","files":[` + strings.Join(reversed, ",") + `]},` +
			`{"name":"Search","files":[` + refs[len(refs)-1] + `]}]}`
	}
	result, err := Read(t.Context(), twoTargetOptions(t, twoTargetGraph(t), provider))
	if err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	var titles []string
	for _, box := range svc.Boxes {
		titles = append(titles, box.Title)
	}
	slices.Sort(titles)
	if !slices.Equal(titles, []string{"Board", "Search"}) || svc.MapFailure != "" || len(svc.OffMap) != 0 {
		t.Fatalf("parts %v, failure %q, off the map %+v", titles, svc.MapFailure, svc.OffMap)
	}
	recorded := 0
	for _, row := range result.Rejected {
		if row.Stage == lines.StageZones && row.Target == "svc" {
			if row.Kind != "part_repeated_group" {
				t.Fatalf("unexpected annotation: %+v", row)
			}
			recorded++
		}
	}
	if recorded != 1 || placed != 0 {
		t.Fatalf("recorded %d repeats; %d files placed by the follow-up", recorded, placed)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
}

// A map's annotations are rejected rows the reading records after the answer
// was accepted. Each points at its window, which keeps its prompt, input,
// request and response in the run (owner, 2026-09-26): after cache clear
// every such row still leads to the answer it annotates. The warm reading
// takes the accepted answers from the cache and records the same rows.
func TestAnnotatedMapAnswersStayInTheRun(t *testing.T) {
	provider := &tableProvider{
		partsResponse: func(files []map[string]any) string {
			type group struct {
				Name  string   `json:"name"`
				Files []string `json:"files"`
			}
			var groups []group
			at := map[string]int{}
			for _, file := range files {
				name := filepath.Dir(fmt.Sprint(file["path"]))
				if _, seen := at[name]; !seen {
					at[name] = len(groups)
					groups = append(groups, group{Name: name})
				}
				groups[at[name]].Files = append(groups[at[name]].Files, fmt.Sprint(file["ref"]))
			}
			// A ref the request did not list is discarded and noted.
			groups[0].Files = append(groups[0].Files, "f999")
			raw, _ := json.Marshal(map[string]any{"groups": groups})
			return string(raw)
		},
		// An area of one part is drawn as that part and noted.
		areaFor: func(part map[string]any) string {
			switch part["name"] {
			case "svc/api", "svc/core":
				return "Serving"
			case "svc/db":
				return "Storage"
			}
			return ""
		},
	}
	cache := t.TempDir()
	read := func() (string, Result) {
		t.Helper()
		opts := twoTargetOptions(t, twoTargetGraph(t), provider)
		opts.Executor = llm.Executor{RootDir: cache, Enabled: true, BatchConcurrency: 2, BatchController: &llm.BatchController{}}
		result, err := Read(t.Context(), opts)
		if err != nil {
			t.Fatal(err)
		}
		return opts.OwnerRunDir, result
	}
	coldRun, cold := read()
	calls := provider.calls
	warmRun, warm := read()
	if provider.calls != calls {
		t.Fatalf("warm reading asked the provider %d more times; want every accepted answer from the cache", provider.calls-calls)
	}
	if err := os.RemoveAll(filepath.Join(cache, llm.CacheDirectoryName)); err != nil {
		t.Fatal(err)
	}
	for run, result := range map[string]Result{coldRun: cold, warmRun: warm} {
		noted := map[string]bool{}
		for _, row := range result.Rejected {
			if row.ResponseRef == "" {
				continue
			}
			noted[row.Stage+" "+row.Kind] = true
			window := strings.TrimSuffix(filepath.Join(run, filepath.FromSlash(row.ResponseRef)), "response.ref.json")
			for _, label := range []string{"prompt", "input", "request", "response"} {
				if body, err := readWindowPayload(window + label + ".ref.json"); err != nil || len(body) == 0 {
					t.Errorf("after cache clear the %s %s row's window lost its %s: %v", row.Stage, row.Kind, label, err)
				}
			}
		}
		if !noted[lines.StageZones+" part_unknown_ref"] || !noted[lines.StageAreas+" area_annotation"] {
			t.Fatalf("the map's annotations were not recorded: %v", noted)
		}
	}
}

// failingTasks is a provider that fails every request of the named design
// tasks without any response, as a timeout or a refused connection does.
type failingTasks struct {
	*tableProvider
	tasks map[string]bool
}

func (p failingTasks) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var request struct {
		Task string `json:"task"`
	}
	if json.Unmarshal(prepared.Bytes(), &request) == nil && p.tasks[request.Task] {
		return llm.Completion{Metrics: llm.Metrics{Attempts: 1}}, fmt.Errorf("provider unavailable")
	}
	return p.tableProvider.Complete(ctx, prepared)
}

// A map request the provider failed without a response is recorded with no
// response to point at, like a failed table window: every response_ref the
// reading records leads to bytes.
func TestResponselessMapRefusalsNameNoResponse(t *testing.T) {
	for name, stages := range map[string]map[string]string{
		"parts":              {designPartsTask: lines.StageZones},
		"areas and describe": {designAreasTask: lines.StageAreas, designDescribeTask: lines.StageDescribe},
	} {
		t.Run(name, func(t *testing.T) {
			provider := &tableProvider{}
			opts := twoTargetOptions(t, twoTargetGraph(t), provider)
			failing := failingTasks{tableProvider: provider, tasks: map[string]bool{}}
			for task := range stages {
				failing.tasks[task] = true
			}
			opts.Provider = failing
			result, err := Read(t.Context(), opts)
			if err != nil {
				t.Fatal(err)
			}
			refused := map[string]bool{}
			for _, row := range result.Rejected {
				if row.Kind == "window_rejected" || row.Kind == "description_refused" {
					refused[row.Stage] = true
				}
				if row.ResponseRef == "" {
					continue
				}
				if _, err := readWindowPayload(filepath.Join(opts.OwnerRunDir, filepath.FromSlash(row.ResponseRef))); err != nil {
					t.Errorf("the %s %s row's %s leads nowhere: %v", row.Stage, row.Kind, row.ResponseRef, err)
				}
			}
			for _, stage := range stages {
				if !refused[stage] {
					t.Fatalf("no %s request was refused: %+v", stage, result.Rejected)
				}
			}
		})
	}
}

// A files string of refs separated by spaces or commas is that list, and
// each ref is still checked; an object is not a list of files.
func TestPartsFilesStringIsAListOfRefs(t *testing.T) {
	listed := []string{"f1", "f2", "f3"}
	for _, files := range []string{`"f1 f2"`, `"f1, f2"`, `" f1,f2 "`} {
		answer, err := decodeParts([]byte(`{"groups":[{"name":"Board","files":`+files+`},{"name":"Search","files":["f3"]}]}`), listed)
		if err != nil {
			t.Fatalf("%s: %v", files, err)
		}
		if result := validatePartition(answer, listed); len(result.files) != 2 || !slices.Equal(result.files[0], []string{"f1", "f2"}) || len(result.refused) != 0 {
			t.Fatalf("%s: %+v", files, result)
		}
	}
	answer, err := decodeParts([]byte(`{"groups":[{"name":"Board","files":{"x":1}},{"name":"Ghost","files":"f99"},{"name":"Search","files":["f3"]}]}`), listed)
	if err != nil {
		t.Fatal(err)
	}
	result := validatePartition(answer, listed)
	if !slices.Equal(result.names, []string{"Search"}) || !slices.Equal(result.unknown, []string{"f99"}) ||
		!slices.Equal(result.leftOut, []string{"f1", "f2"}) || len(result.refused) != 2 ||
		!strings.Contains(result.refused[0], "not a name with a list of files") {
		t.Fatalf("an object or an unknown ref in a string gained a part: %+v", result)
	}
	for _, raw := range []string{`{"groups":[{"name":"Board","files":"f99 f98"}]}`, `{"groups":[{"name":"Board","files":7}]}`} {
		if _, err := decodeParts([]byte(raw), listed); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

// The follow-up places a left-out file among every drawn part, offers a file
// listed in two parts only those two, and leaves a file whose choice is
// refused off the map with its reason.
func TestFollowUpPlacesLeftOutAndConflictingFiles(t *testing.T) {
	graph := withSymbols(t, twoTargetGraph(t))
	var offered map[string][]any
	provider := &tableProvider{
		partsResponse: func([]map[string]any) string {
			ref := func(path string) string { return graphPlaceID(t, graph, atlas.PlaceFile, path, 0, "") }
			return fmt.Sprintf(`{"groups":[{"name":"Serving","files":[%q,%q]},{"name":"Storage","files":[%q,%q]}]}`,
				ref("svc/api/h.go"), ref("svc/core/c.go"), ref("svc/db/d.go"), ref("svc/core/c.go"))
		},
		placeFor: func(row map[string]any) string {
			options, _ := row["part_options"].([]any)
			if offered == nil {
				offered = map[string][]any{}
			}
			offered[fmt.Sprint(row["path"])] = options
			switch row["path"] {
			case "svc/jobs/j.go":
				return "p99" // not an offered part: refused
			case "svc/core/c.go":
				return fmt.Sprint(options[1])
			}
			return fmt.Sprint(options[0])
		},
	}
	result, err := Read(t.Context(), twoTargetOptions(t, graph, provider))
	if err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	parts := map[string]string{}
	for _, box := range svc.Boxes {
		for _, file := range box.Files {
			parts[file.Path] = box.Title
		}
	}
	if len(offered["svc/core/c.go"]) != 2 || len(offered["svc/util/u.go"]) != 2 {
		t.Fatalf("offered options: %v", offered)
	}
	if parts["svc/core/c.go"] != "Storage" || parts["svc/util/u.go"] != "Serving" || parts["svc/api/h.go"] != "Serving" {
		t.Fatalf("placement: %v", parts)
	}
	if reasons := offMapReasons(svc); reasons["svc/jobs/j.go"] != atlas.OffMapLeftOut || len(reasons) != 1 {
		t.Fatalf("off the map: %v", reasons)
	}
	if svc.MapFailure != "" {
		t.Fatalf("map failure: %q", svc.MapFailure)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
}

// A refused parts answer leaves the target with an explicit map failure:
// every file and its boundaries stay in the atlas off the map, with their
// lines and declarations, and no part or area is invented. The refused
// answer is not asked again.
func TestRefusedPartsAnswerIsAnExplicitMapFailure(t *testing.T) {
	graph := twoTargetGraph(t)
	provider := &tableProvider{partsResponse: func([]map[string]any) string { return `{"groups":[]}` }}
	result, err := Read(t.Context(), twoTargetOptions(t, graph, provider))
	if err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	if svc.MapFailure != atlas.MapFailureRefused || len(svc.Boxes) != 0 || len(svc.Zones) != 0 {
		t.Fatalf("map failure %q boxes %d zones %d", svc.MapFailure, len(svc.Boxes), len(svc.Zones))
	}
	files := 0
	for _, place := range graph.Places {
		if place.File != nil && contains(place.TargetIDs, "svc") {
			files++
		}
	}
	reasons := offMapReasons(svc)
	if len(reasons) != files || svc.Files != files {
		t.Fatalf("off the map %v of %d files", reasons, files)
	}
	for path, reason := range reasons {
		if reason != atlas.OffMapFailure {
			t.Fatalf("%s: %s", path, reason)
		}
	}
	for _, entry := range svc.OffMap {
		if entry.File.Path == "svc/api/h.go" && len(entry.File.Symbols) != 1 {
			t.Fatalf("an off-map file lost its declaration: %+v", entry)
		}
	}
	boundaries := 0
	for _, boundary := range svc.Boundaries {
		if boundary.BoxID == "" {
			boundaries++
		}
	}
	if boundaries != 2 {
		t.Fatalf("boundaries off the map: %+v", svc.Boundaries)
	}
	if provider.designRequests[designPartsTask] != 2 {
		t.Fatalf("parts requests of two targets, want one each: %v", provider.designRequests)
	}
	refused := 0
	for _, row := range result.Rejected {
		if row.Stage == lines.StageZones && row.Kind == "window_rejected" {
			refused++
		}
	}
	if refused != 2 {
		t.Fatalf("refusals: %+v", result.Rejected)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
}

// One target's refused parts answer is that target's map failure after its
// one request; the other target, answered well, draws its map.
func TestRefusedPartsAnswerLeavesItsSiblingDrawn(t *testing.T) {
	var mu sync.Mutex
	asked := map[string]int{}
	provider := &tableProvider{}
	provider.partsResponse = func(files []map[string]any) string {
		target := strings.SplitN(fmt.Sprint(files[0]["path"]), "/", 2)[0]
		mu.Lock()
		asked[target]++
		mu.Unlock()
		if target == "svc" {
			return `{"groups":[]}`
		}
		var groups []string
		for _, file := range files {
			groups = append(groups, fmt.Sprintf(`{"name":%q,"files":[%q]}`, file["path"], file["ref"]))
		}
		return `{"groups":[` + strings.Join(groups, ",") + `]}`
	}
	result, err := Read(t.Context(), twoTargetOptions(t, twoTargetGraph(t), provider))
	if err != nil {
		t.Fatal(err)
	}
	if asked["svc"] != 1 || asked["web"] != 1 {
		t.Fatalf("parts requests %v, want one per target", asked)
	}
	svc := targetOf(t, result, "svc")
	if svc.MapFailure != atlas.MapFailureRefused || len(svc.Boxes) != 0 || len(svc.OffMap) == 0 {
		t.Fatalf("svc: failure %q boxes %d off-map %d", svc.MapFailure, len(svc.Boxes), len(svc.OffMap))
	}
	for _, entry := range svc.OffMap {
		if entry.Reason != atlas.OffMapFailure {
			t.Fatalf("%s: %s", entry.File.Path, entry.Reason)
		}
	}
	rejected := 0
	for _, row := range result.Rejected {
		if row.Stage == lines.StageZones && row.Kind == "window_rejected" {
			if row.Target != "svc" {
				t.Fatalf("a refusal of the answered target: %+v", row)
			}
			rejected++
		}
	}
	if rejected != 1 {
		t.Fatalf("refusals: %+v", result.Rejected)
	}
	if web := targetOf(t, result, "web"); web.MapFailure != "" || len(web.Boxes) == 0 {
		t.Fatalf("web: failure %q boxes %d", web.MapFailure, len(web.Boxes))
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
}

// Without a model a target with several files has no map of parts: an
// explicit map failure, never an invented grouping. A one-file target draws
// its one part, named after the target, without asking.
func TestWithoutAModelTheMapIsAnExplicitFailure(t *testing.T) {
	opts := twoTargetOptions(t, twoTargetGraph(t), nil)
	opts.Provider = nil
	result, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	if svc.MapFailure == "" || len(svc.Boxes) != 0 || len(svc.OffMap) != 5 {
		t.Fatalf("svc: failure %q boxes %d off-map %d", svc.MapFailure, len(svc.Boxes), len(svc.OffMap))
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
}

// A target without code sends no request and gets an empty map, not a
// failure; a one-file target sends no parts request and its part takes the
// target's name, with a description.
func TestSmallTargetsNeedNoPartsRequest(t *testing.T) {
	withoutDecls := func(paths ...string) atlas.Graph {
		graph := twoTargetGraph(t)
		for i := range graph.Places {
			if graph.Places[i].File != nil && slices.Contains(paths, graph.Places[i].Path) {
				graph.Places[i].File.Decls = nil
			}
		}
		return graph
	}
	// web holds one file with code: its one part takes the target's name.
	provider := &tableProvider{}
	result, err := Read(t.Context(), twoTargetOptions(t, withoutDecls("web/src/client.ts"), provider))
	if err != nil {
		t.Fatal(err)
	}
	web := targetOf(t, result, "web")
	if len(web.Boxes) != 1 || web.Boxes[0].Title != "web" || web.Boxes[0].Line != "About web." || web.MapFailure != "" {
		t.Fatalf("one-file part: %+v", web.Boxes)
	}
	if reasons := offMapReasons(web); len(reasons) != 1 || reasons["web/src/client.ts"] != atlas.OffMapNoUnits {
		t.Fatalf("file without units: %v", reasons)
	}
	if provider.designRequests[designPartsTask] != 1 {
		t.Fatalf("a one-file target sent a parts request: %v", provider.designRequests)
	}
	// web holds no code at all: a legitimate empty map, not a failure.
	provider = &tableProvider{}
	result, err = Read(t.Context(), twoTargetOptions(t, withoutDecls("web/src/client.ts", "web/src/app.ts"), provider))
	if err != nil {
		t.Fatal(err)
	}
	web = targetOf(t, result, "web")
	if web.MapFailure != "" || len(web.Boxes) != 0 {
		t.Fatalf("empty map: failure %q boxes %+v", web.MapFailure, web.Boxes)
	}
	if reasons := offMapReasons(web); reasons["web/src/app.ts"] != atlas.OffMapNoUnits || reasons["web/src/client.ts"] != atlas.OffMapNoUnits {
		t.Fatalf("files without units: %v", reasons)
	}
	if provider.designRequests[designPartsTask] != 1 {
		t.Fatalf("a target without code sent a parts request: %v", provider.designRequests)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
}

// Every drawn part gets its description from its members' names and
// signatures, never their documentation; a refused description leaves the
// explicit no-description state, and nothing fills it in.
func TestPartDescriptionsComeFromMembersOrStayAbsent(t *testing.T) {
	graph := twoTargetGraph(t)
	provider := &tableProvider{describe: func(name string) string {
		if name == "svc/db" {
			return ""
		}
		return "Does " + name + "."
	}}
	result, err := Read(t.Context(), twoTargetOptions(t, graph, provider))
	if err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	lines := map[string]string{}
	for _, box := range svc.Boxes {
		lines[box.Title] = box.Line
	}
	if lines["svc/api"] != "Does svc/api." || lines["svc/db"] != "" {
		t.Fatalf("lines: %v", lines)
	}
	requests, _ := filepath.Glob(filepath.Join(filepath.Dir(result.TablesPath), atlas.TablesDir, "atlas_describe-*.input.ref.json"))
	if len(requests) == 0 {
		t.Fatal("no description requests saved")
	}
	for _, name := range requests {
		raw, err := readWindowPayload(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "F does.") || strings.Contains(string(raw), "doc") {
			t.Fatalf("a description request carried documentation: %s", raw)
		}
	}
}

// Areas are a closed split of the described parts: a part in two areas or in
// none stands alone, and an area of one part is that part.
func TestAreasAreAClosedSplitOfTheParts(t *testing.T) {
	graph := twoTargetGraph(t)
	provider := &tableProvider{areaFor: func(part map[string]any) string {
		switch part["name"] {
		case "svc/api", "svc/core":
			return "Serving"
		case "svc/db":
			return "Storage"
		}
		return ""
	}}
	result, err := Read(t.Context(), twoTargetOptions(t, graph, provider))
	if err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	boxes := map[string]atlas.Box{}
	for _, box := range svc.Boxes {
		boxes[box.Title] = box
	}
	if len(svc.Zones) != 1 || svc.Zones[0].Title != "Serving" || svc.Zones[0].Line != "About Serving." ||
		!slices.Equal(svc.Zones[0].BoxIDs, []string{boxes["svc/api"].ID, boxes["svc/core"].ID}) {
		t.Fatalf("zones: %+v", svc.Zones)
	}
	if boxes["svc/db"].ZoneID != "" || boxes["svc/jobs"].ZoneID != "" || boxes["svc/api"].ZoneID != svc.Zones[0].ID {
		t.Fatalf("boxes: %+v", boxes)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}

	// A part named in two areas stands alone; the other area keeps its two.
	listed := []string{"p1", "p2", "p3", "p4", "p5"}
	answer, err := decodeAreas([]byte(`{"areas":[{"name":"A","parts":["p1","p2","p3"]},{"name":"B","parts":["p3","p4"]},{"name":"C","parts":["p5","p9"]}]}`), listed)
	if err != nil {
		t.Fatal(err)
	}
	areas := validateAreas(answer, listed)
	if !slices.Equal(areas.names, []string{"A"}) || !slices.Equal(areas.parts[0], []string{"p1", "p2"}) || !slices.Equal(areas.unknown, []string{"p9"}) {
		t.Fatalf("areas: %+v", areas)
	}
	// No areas, or areas of one part each, leave every part alone; areas that
	// hold no listed part, such as parts named instead of referenced, refuse
	// the answer whole.
	for raw, whole := range map[string]bool{
		`{"areas":[]}`: false,
		`{"areas":[{"name":"A","parts":["p1"]},{"name":"B","parts":["p2"]}]}`: false,
		`{"areas":[{"name":"Serving","parts":["HTTP API","Core"]}]}`:          true,
		`{"areas":[{"name":"","parts":["p1","p2"]}]}`:                         true,
	} {
		if _, err := decodeAreas([]byte(raw), listed); (err != nil) != whole {
			t.Fatalf("%s: %v", raw, err)
		}
	}
}

// Areas keep the order the answer lists them, whatever the order of the parts
// it was shown: z1 is the first area named. The request asks for no order and
// nothing checks one; GroupsIndex and the page keep this one.
func TestAreasKeepTheAnswersOrder(t *testing.T) {
	provider := &tableProvider{areasResponse: func(parts []map[string]any) string {
		ref := map[string]string{}
		for _, part := range parts {
			ref[fmt.Sprint(part["name"])] = fmt.Sprint(part["ref"])
		}
		raw, _ := json.Marshal(map[string]any{"areas": []map[string]any{
			{"name": "Storage", "parts": []string{ref["svc/db"], ref["svc/jobs"]}},
			{"name": "Serving", "parts": []string{ref["svc/api"], ref["svc/core"]}},
		}})
		return string(raw)
	}}
	result, err := Read(t.Context(), twoTargetOptions(t, twoTargetGraph(t), provider))
	if err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	var titles []string
	for _, zone := range svc.Zones {
		titles = append(titles, zone.Title)
	}
	if !slices.Equal(titles, []string{"Storage", "Serving"}) || !compactIDLess(svc.Zones[0].ID, svc.Zones[1].ID) {
		t.Fatalf("zones %+v, want the answer's order", svc.Zones)
	}
}

// An area stated twice, with the same name ignoring case and the same set of
// listed parts, is drawn once and noted; two different areas that list one
// part still leave that part alone. A parts string of refs is that list.
func TestIdenticalRepeatedAreaIsDrawnOnce(t *testing.T) {
	listed := []string{"p1", "p2", "p3", "p4"}
	areas := func(raw string) validAreas {
		t.Helper()
		answer, err := decodeAreas([]byte(raw), listed)
		if err != nil {
			t.Fatalf("%s refused: %v", raw, err)
		}
		return validateAreas(answer, listed)
	}
	repeated := areas(`{"areas":[{"name":"Core","parts":["p1","p2"]},{"name":"core","parts":["p2","p1","p9"]},{"name":"Infra","parts":["p3","p4"]}]}`)
	if !slices.Equal(repeated.names, []string{"Core", "Infra"}) || !slices.Equal(repeated.parts[0], []string{"p1", "p2"}) ||
		!slices.Equal(repeated.parts[1], []string{"p3", "p4"}) || len(repeated.notes) != 1 || !strings.Contains(repeated.notes[0], "repeats") {
		t.Fatalf("repeated area: %+v", repeated)
	}
	shared := areas(`{"areas":[{"name":"Core","parts":["p1","p2"]},{"name":"Infra","parts":["p2","p3"]}]}`)
	if len(shared.names) != 0 || !slices.ContainsFunc(shared.notes, func(note string) bool { return strings.HasPrefix(note, "p2 is in two areas") }) {
		t.Fatalf("a part in two different areas: %+v", shared)
	}
	// Two names over the same parts are two answers, never a first-wins pick.
	sameParts := areas(`{"areas":[{"name":"Core","parts":["p1","p2"]},{"name":"Infra","parts":["p2","p1"]},{"name":"Edge","parts":["p3","p4"]}]}`)
	if !slices.Equal(sameParts.names, []string{"Edge"}) || slices.ContainsFunc(sameParts.notes, func(note string) bool { return strings.Contains(note, "repeats") }) {
		t.Fatalf("two names over the same parts: %+v", sameParts)
	}
	sameName := areas(`{"areas":[{"name":"Core","parts":["p1","p2","p3"]},{"name":"Core","parts":["p3","p4"]}]}`)
	if !slices.Equal(sameName.names, []string{"Core"}) || !slices.Equal(sameName.parts[0], []string{"p1", "p2"}) {
		t.Fatalf("one name over different parts: %+v", sameName)
	}
	for _, parts := range []string{`"p1 p2"`, `"p1, p2"`} {
		if read := areas(`{"areas":[{"name":"Core","parts":` + parts + `}]}`); !slices.Equal(read.names, []string{"Core"}) || !slices.Equal(read.parts[0], []string{"p1", "p2"}) {
			t.Fatalf("%s: %+v", parts, read)
		}
	}
	for _, raw := range []string{`{"areas":[{"name":"Core","parts":"Board Search"}]}`, `{"areas":[{"name":"Core","parts":{"p1":true}}]}`} {
		if _, err := decodeAreas([]byte(raw), listed); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

// Every target keeps its own parts windows: a target's request holds only its
// own files, and each window is printed once.
func TestEachTargetKeepsItsOwnPartsWindows(t *testing.T) {
	opts := twoTargetOptions(t, twoTargetGraph(t), &tableProvider{})
	result, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	tables, err := os.ReadFile(result.TablesPath)
	if err != nil {
		t.Fatal(err)
	}
	for round, own := range []string{"svc/", "web/"} {
		other := []string{"web/", "svc/"}[round]
		name := fmt.Sprintf("%s-r%d-w0.input.ref.json", lines.StageZones, round+1)
		raw, err := readWindowPayload(filepath.Join(opts.OwnerRunDir, atlas.TablesDir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(string(raw), own) || strings.Contains(string(raw), other) {
			t.Fatalf("%s holds another target's files: %s", name, raw)
		}
		if heading := fmt.Sprintf("## %s · round %d · window 0", lines.StageZones, round+1); strings.Count(string(tables), heading) != 1 {
			t.Fatalf("tables.md has %d %q headings", strings.Count(string(tables), heading), heading)
		}
	}
}

// A part made only of test code stays in the atlas with its files and keys,
// off the canvas by fact; one ordinary file keeps a part drawn. Test-only
// parts get no description request.
func TestTestOnlyPartsLeaveTheCanvasByFact(t *testing.T) {
	graph := twoTargetGraph(t)
	for i := range graph.Places {
		if file := graph.Places[i].File; file != nil && (graph.Places[i].Path == "svc/jobs/j.go" || graph.Places[i].Path == "svc/util/u.go") {
			file.Test = true
		}
	}
	var mu sync.Mutex
	var described []string
	provider := &tableProvider{
		partFor: func(file map[string]any) string {
			switch file["path"] {
			case "svc/jobs/j.go":
				return "Job tests"
			case "svc/util/u.go", "svc/db/d.go":
				return "Mixed"
			}
			return "Program"
		},
		describe: func(name string) string {
			mu.Lock()
			defer mu.Unlock()
			described = append(described, name)
			return "About " + name + "."
		},
	}
	result, err := Read(t.Context(), twoTargetOptions(t, graph, provider))
	if err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	for _, box := range svc.Boxes {
		if box.ForTests != (box.Title == "Job tests") {
			t.Fatalf("%s: for tests %v", box.Title, box.ForTests)
		}
		if box.Title == "Job tests" && (box.Line != "" || len(box.MemberIDs) == 0) {
			t.Fatalf("test part: %+v", box)
		}
	}
	if slices.Contains(described, "Job tests") || !slices.Contains(described, "Mixed") {
		t.Fatalf("described: %v", described)
	}
}

// Split windows are whole directory subtrees, halved by file count; a single
// flat directory halves into contiguous runs in path order.
func TestWindowsSplitAlongDirectorySubtrees(t *testing.T) {
	file := func(path string) *designFile {
		return &designFile{id: path, path: path, dir: filepath.Dir(path)}
	}
	paths := func(files []*designFile) string {
		var result []string
		for _, f := range files {
			result = append(result, f.path)
		}
		return strings.Join(result, " ")
	}
	tree := []*designFile{file("a/x/1.go"), file("a/x/2.go"), file("a/y/3.go"), file("a/z.go"), file("a/y/4.go")}
	left, right, ok := splitWindow(tree)
	if !ok || paths(left) != "a/x/1.go a/x/2.go" || paths(right) != "a/y/3.go a/y/4.go a/z.go" {
		t.Fatalf("subtrees: %q | %q", paths(left), paths(right))
	}
	flat := []*designFile{file("d/1.go"), file("d/2.go"), file("d/3.go"), file("d/4.go"), file("d/5.go")}
	left, right, ok = splitWindow(flat)
	if !ok || paths(left) != "d/1.go d/2.go" || paths(right) != "d/3.go d/4.go d/5.go" {
		t.Fatalf("flat: %q | %q", paths(left), paths(right))
	}
	if _, _, ok := splitWindow(flat[:1]); ok {
		t.Fatal("split one file")
	}
}

// A parts request the provider refuses for its input size is asked again as
// directory-subtree windows of complete files; parts never cross windows.
func TestTooLargePartsRequestIsAskedInWindows(t *testing.T) {
	graph := twoTargetGraph(t)
	provider := &sizeLimitedProvider{tableProvider: &tableProvider{}, maxFiles: 2}
	opts := twoTargetOptions(t, graph, provider.tableProvider)
	opts.Provider = provider
	result, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	if svc.MapFailure != "" || len(svc.OffMap) != 0 || len(svc.Boxes) != 5 {
		t.Fatalf("windows: failure %q off %v boxes %d", svc.MapFailure, svc.OffMap, len(svc.Boxes))
	}
	if provider.refused == 0 {
		t.Fatal("the provider never refused a whole request")
	}
}

// cutProvider cuts the answers of one design task at the output-token cap,
// the way the provider reports a looping answer. The cut answer still
// carries a complete, well-formed body, so accepting what was written before
// the cut would draw it.
type cutProvider struct {
	*tableProvider
	task string
}

func (provider *cutProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var request struct {
		Task string `json:"task"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return llm.Completion{}, err
	}
	completion, err := provider.tableProvider.Complete(ctx, prepared)
	if err != nil || request.Task != provider.task {
		return completion, err
	}
	completion.FinishReason = llm.FinishLength
	return completion, llm.NewResourceLimitError(llm.ResourceLimitError{Stage: lines.StageZones, Kind: llm.ResourceLimitOutputTokens, FinishReason: "length"})
}

// A parts or areas answer cut at the output-token cap is an ordinary refusal
// of its window: asked once, neither split nor accepted in part. A cut parts
// answer of a one-window target is its map failure; a cut areas answer draws
// no areas and the parts stay drawn.
func TestAnswerCutAtTheOutputCapIsAnOrdinaryRefusal(t *testing.T) {
	graph := twoTargetGraph(t)
	parts := &cutProvider{tableProvider: &tableProvider{}, task: designPartsTask}
	opts := twoTargetOptions(t, graph, parts.tableProvider)
	opts.Provider = parts
	result, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	if svc.MapFailure != atlas.MapFailureRefused || len(svc.Boxes) != 0 || parts.designRequests[designPartsTask] != 2 {
		t.Fatalf("cut parts: failure %q boxes %d requests %v", svc.MapFailure, len(svc.Boxes), parts.designRequests)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}

	areas := &cutProvider{tableProvider: &tableProvider{areaFor: func(map[string]any) string { return "Everything" }}, task: designAreasTask}
	opts = twoTargetOptions(t, graph, areas.tableProvider)
	opts.Provider = areas
	result, err = Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	svc = targetOf(t, result, "svc")
	refused := 0
	for _, row := range result.Rejected {
		if row.Stage == lines.StageAreas && row.Kind == "window_rejected" {
			refused++
		}
	}
	if len(svc.Zones) != 0 || len(svc.Boxes) == 0 || svc.MapFailure != "" || refused != 1 || areas.designRequests[designAreasTask] != 1 {
		t.Fatalf("cut areas: zones %d boxes %d failure %q refusals %d requests %v", len(svc.Zones), len(svc.Boxes), svc.MapFailure, refused, areas.designRequests)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
}

// sizeLimitedProvider refuses a parts request listing more than maxFiles
// files the way a provider refuses too large an input.
type sizeLimitedProvider struct {
	*tableProvider
	maxFiles int
	refused  int
}

func (provider *sizeLimitedProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var request struct {
		Task  string           `json:"task"`
		Files []map[string]any `json:"files"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return llm.Completion{}, err
	}
	if request.Task == designPartsTask && len(request.Files) > provider.maxFiles {
		provider.mu.Lock()
		provider.refused++
		provider.mu.Unlock()
		return llm.Completion{}, llm.NewResourceLimitError(llm.ResourceLimitError{Stage: lines.StageZones, Kind: llm.ResourceLimitContextTokens, Limit: 100, Observed: 200, ObservedKnown: true})
	}
	return provider.tableProvider.Complete(ctx, prepared)
}

// withCrossFileMethods gives svc two types whose methods are declared in
// other files: Store in svc/db/d.go with Save in svc/db/save.go, a file
// that declares nothing else and holds a config boundary, and Job in
// svc/jobs/j.go with Run in svc/api/h.go.
func withCrossFileMethods(t *testing.T, graph atlas.Graph) atlas.Graph {
	t.Helper()
	method := func(path string, line int, name string) atlas.Decl {
		return atlas.Decl{ObjectID: "n" + name, Name: name, Kind: "method", Signature: "func()", LineNo: line, Exported: true}
	}
	save, run := method("svc/db/save.go", 3, "Store.Save"), method("svc/api/h.go", 20, "Job.Run")
	types := map[string]atlas.Decl{
		"svc/db/d.go":   {ObjectID: "nStore", Name: "Store", Kind: "type", Signature: "struct", LineNo: 10, Exported: true},
		"svc/jobs/j.go": {ObjectID: "nJob", Name: "Job", Kind: "type", Signature: "struct", LineNo: 10, Exported: true},
	}
	members := map[string]atlas.TypeMember{"Store": {Path: "svc/db/save.go", Decl: save}, "Job": {Path: "svc/api/h.go", Decl: run}}
	var added []atlas.Place
	symbol := func(path, parent string, decl atlas.Decl) atlas.Place {
		facts := &atlas.SymbolFacts{Decl: decl}
		if member, ok := members[decl.Name]; ok {
			facts.Members = []atlas.TypeMember{member}
		}
		return atlas.Place{ID: atlas.SymbolID(path, decl.LineNo, decl.Name), Kind: atlas.PlaceSymbol, Path: path, LineNo: decl.LineNo,
			Parent: parent, TargetIDs: []string{"svc"}, Given: decl.Name, Symbol: facts}
	}
	dbDir := ""
	for i := range graph.Places {
		place := &graph.Places[i]
		if place.File == nil {
			continue
		}
		if decl, ok := types[place.Path]; ok {
			place.File.Decls = append(place.File.Decls, decl)
			added = append(added, symbol(place.Path, place.ID, decl))
		}
		if place.Path == "svc/api/h.go" {
			place.File.Decls = append(place.File.Decls, run)
			added = append(added, symbol(place.Path, place.ID, run))
		}
		if place.Path == "svc/db/d.go" {
			dbDir = place.Parent
		}
	}
	file := atlas.FileID("svc/db/save.go")
	added = append(added,
		atlas.Place{ID: file, Kind: atlas.PlaceFile, Path: "svc/db/save.go", Depth: 2, Parent: dbDir, TargetIDs: []string{"svc"},
			Given: "given svc/db/save.go", File: &atlas.FileFacts{Decls: []atlas.Decl{save}}},
		symbol("svc/db/save.go", file, save),
		atlas.Place{ID: "bnd:svc/db/save.go:4:config", Kind: atlas.PlaceBoundary, Path: "svc/db/save.go", LineNo: 4, Parent: file,
			TargetIDs: []string{"svc"}, Given: "config STORE_PATH",
			Boundary: &atlas.BoundaryFacts{Source: "fact", Caller: "Store.Save", Direction: atlas.DirectionOut, GivenKind: atlas.BoundaryConfig, Values: []string{"STORE_PATH"}}},
	)
	graph.Places = append(graph.Places, added...)
	atlas.SortPlaces(graph.Places)
	encoded, err := atlas.EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := atlas.DecodeGraph(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

// A method declared outside its type's file goes with its type. A file that
// declares only such methods is on the map through them: it has no entry off
// the map, and a boundary in it stands in its methods' part. A method whose
// type is off the map is listed off the map in its own file under its
// type's reason, naming the part that still holds its file.
func TestFilesOfMethodsDeclaredElsewhereStayOnTheMap(t *testing.T) {
	graph := withCrossFileMethods(t, twoTargetGraph(t))
	provider := &tableProvider{
		partFor: func(file map[string]any) string {
			if file["path"] == "svc/jobs/j.go" {
				return "" // a group without a name: j.go is left out
			}
			return filepath.Dir(fmt.Sprint(file["path"]))
		},
		placeFor: func(row map[string]any) string { return "p99" }, // the follow-up is refused
	}
	result, err := Read(t.Context(), twoTargetOptions(t, graph, provider))
	if err != nil {
		t.Fatal(err)
	}
	if err := atlas.Validate(result.Atlas); err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	partOf, boxOf := map[string]string{}, map[string]string{}
	for _, box := range svc.Boxes {
		boxOf[box.Title] = box.ID
		for _, file := range box.Files {
			for _, symbol := range file.Symbols {
				partOf[symbol.Name] = box.Title
			}
		}
	}
	if partOf["Store"] != "svc/db" || partOf["Store.Save"] != "svc/db" || partOf["Job.Run"] != "" {
		t.Fatalf("parts: %v", partOf)
	}
	for _, boundary := range svc.Boundaries {
		if boundary.Path == "svc/db/save.go" && boundary.BoxID != boxOf["svc/db"] {
			t.Fatalf("the boundary of a methods-only file stands in %q", boundary.BoxID)
		}
	}
	entries := map[string]atlas.OffMapFile{}
	for _, entry := range svc.OffMap {
		entries[entry.File.Path] = entry
	}
	if _, listed := entries["svc/db/save.go"]; listed || len(entries) != 2 {
		t.Fatalf("off the map: %+v", entries)
	}
	if jobs := entries["svc/jobs/j.go"]; jobs.Reason != atlas.OffMapLeftOut || jobs.BoxID != "" {
		t.Fatalf("the left-out file: %+v", jobs)
	}
	stray := entries["svc/api/h.go"]
	if stray.Reason != atlas.OffMapLeftOut || stray.BoxID != boxOf["svc/api"] || len(stray.File.Symbols) != 1 || stray.File.Symbols[0].Name != "Job.Run" {
		t.Fatalf("the method of a type off the map: %+v", stray)
	}
}
