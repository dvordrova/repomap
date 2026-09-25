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
// memberships, a group without a name is not drawn and its files are asked
// again, two parts sharing a name both stay. Only an answer that draws no
// part is refused whole: not JSON, no groups, or no group holding a listed
// file of its own.
func TestPartitionRefusesOnlyWhatIsWrong(t *testing.T) {
	listed := []string{"f1", "f2", "f3", "f4", "f5", "f6"}
	answer, err := decodeParts([]byte(`{"groups":[
		{"name":"Entry","files":["f1","f1","f9"]},
		{"name":"Store","files":["f2","f3"]},
		{"name":"store","files":["f3","f4"]},
		{"name":"","files":["f5"]},
		{"name":"Tools","files":"f6"},
		{"name":"Ghost","files":["f99"]}]}`), listed)
	if err != nil {
		t.Fatal(err)
	}
	result := validatePartition(answer, listed)
	if !slices.Equal(result.files[0], []string{"f1"}) || !slices.Equal(result.files[1], []string{"f2"}) || !slices.Equal(result.files[2], []string{"f4"}) {
		t.Fatalf("placements: %v %v", result.names, result.files)
	}
	if !slices.Equal(result.conflicts["f3"], []int{1, 2}) || !slices.Equal(result.leftOut, []string{"f5", "f6"}) {
		t.Fatalf("conflicts %v left out %v", result.conflicts, result.leftOut)
	}
	if !slices.Equal(result.unknown, []string{"f9", "f99"}) || len(result.refused) != 3 || len(result.repeated) != 2 {
		t.Fatalf("unknown %v refused %v repeated %v", result.unknown, result.refused, result.repeated)
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

// A parts answer refused on both draws leaves the target with an explicit
// map failure: every file and its boundaries stay in the atlas off the map,
// with their lines and declarations, and no part or area is invented.
func TestRefusedPartsAnswerIsAnExplicitMapFailure(t *testing.T) {
	graph := twoTargetGraph(t)
	provider := &tableProvider{partsResponse: func([]map[string]any) string { return `{"groups":[]}` }}
	result, err := Read(t.Context(), twoTargetOptions(t, graph, provider))
	if err != nil {
		t.Fatal(err)
	}
	svc := targetOf(t, result, "svc")
	if !strings.Contains(svc.MapFailure, "no groups") || len(svc.Boxes) != 0 || len(svc.Zones) != 0 {
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
	if provider.designRequests[designPartsTask] != 4 {
		t.Fatalf("parts requests of two targets asked twice: %v", provider.designRequests)
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
