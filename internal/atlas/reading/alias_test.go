package reading

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

// aliasProvider answers like tableProvider and writes the English alias of a
// row whose request asks one. It records the cells every description request
// asked of each declaration name. It holds each description request until
// together of them are in flight, or two seconds have passed, and records how
// many were in flight at once: requests asked side by side are all answered
// at once, requests asked one after another each wait alone.
type aliasProvider struct {
	tableProvider
	aliases  map[string]string
	together int
	askedMu  sync.Mutex
	asked    map[string][][]string
	arrived  int
	inFlight int
	peak     int
	ready    chan struct{}
}

func (provider *aliasProvider) Complete(ctx context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var request struct {
		Table string
		Fill  []struct{ Name string }
		Rows  []struct{ Key, Name string }
	}
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil || request.Table != lines.StageSymbols {
		completion, completeErr := provider.tableProvider.Complete(ctx, prepared)
		if completeErr != nil {
			return completion, completeErr
		}
		return completion, err
	}
	var cells []string
	for _, column := range request.Fill {
		cells = append(cells, column.Name)
	}
	if slices.Contains(cells, "key_symbol") {
		return provider.tableProvider.Complete(ctx, prepared)
	}
	provider.askedMu.Lock()
	if provider.asked == nil {
		provider.asked = make(map[string][][]string)
		provider.ready = make(chan struct{})
	}
	for _, row := range request.Rows {
		provider.asked[row.Name] = append(provider.asked[row.Name], cells)
	}
	provider.arrived++
	provider.inFlight++
	provider.peak = max(provider.peak, provider.inFlight)
	if provider.arrived == provider.together {
		close(provider.ready)
	}
	ready := provider.ready
	provider.askedMu.Unlock()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
	}
	completion, err := provider.tableProvider.Complete(ctx, prepared)
	provider.askedMu.Lock()
	provider.inFlight--
	provider.askedMu.Unlock()
	if err != nil || !slices.Contains(cells, lines.AliasColumn) {
		return completion, err
	}
	byKey := make(map[string]string)
	for _, row := range request.Rows {
		byKey[row.Key] = "none"
		if alias := provider.aliases[row.Name]; alias != "" {
			byKey[row.Key] = alias
		}
	}
	var response struct{ Rows []map[string]string }
	if err := json.Unmarshal(completion.Response, &response); err != nil {
		return completion, err
	}
	for _, row := range response.Rows {
		row[lines.AliasColumn] = byKey[row["key"]]
	}
	completion.Response, err = json.Marshal(map[string]any{"rows": response.Rows})
	return completion, err
}

func columnNames(def table.Definition) []string {
	var columns []string
	for _, column := range def.Columns {
		columns = append(columns, column.Name)
	}
	return columns
}

func columnNotes(def table.Definition) []string {
	var notes []string
	for _, column := range def.Columns {
		notes = append(notes, column.Note)
	}
	return notes
}

// A table narrowed for its request, by --captions or by the names it is
// asked of, keeps one prompt: each prompt describes every cell its table
// defines and leaves which ones to write to fill. "fill exactly two cells,
// with line and alias in each result row" had the model write an alias on
// 82 of 82 caption-less types rows, every one discarded (prof6
// atlas_symbols-r4); "Fill two cells" stood in the symbols, targets and
// joints prompts while their text cells were not asked.
func TestPromptsDemandOnlyTheCellsFillAdvertises(t *testing.T) {
	for _, value := range []struct {
		name string
		def  table.Definition
		// asked is each request shape's cells: with captions and without,
		// of a name that needs no alias and, for declarations, one that does.
		asked map[string][]string
	}{
		{"symbols", lines.Symbols(), map[string][]string{
			"captions": {"line"}, "captions alias": {"line", "alias"}, "decisions": nil, "decisions alias": {"alias"}}},
		{"types", lines.Types(), map[string][]string{
			"captions": {"line"}, "captions alias": {"line", "alias"}, "decisions": {"line"}, "decisions alias": {"line", "alias"}}},
		{"targets", lines.Targets(), map[string][]string{"captions": {"line", "role"}, "decisions": {"role"}}},
		{"joints", lines.Joints(), map[string][]string{"captions": {"same", "label"}, "decisions": {"same"}}},
		{"peers", lines.Peers(), map[string][]string{"captions": {"peer", "label"}, "decisions": {"peer"}}},
	} {
		shapes := map[string]table.Definition{"captions": value.def, "decisions": withoutCaptions(value.def)}
		if _, declarations := value.asked["captions alias"]; declarations {
			shapes = map[string]table.Definition{
				"captions": withoutAlias(value.def), "captions alias": value.def,
				"decisions": withoutCaptions(withoutAlias(value.def)), "decisions alias": withoutCaptions(value.def),
			}
		}
		for shape, def := range shapes {
			if got := columnNames(def); !slices.Equal(got, value.asked[shape]) {
				t.Errorf("%s asks %v in %s, want %v", value.name, got, shape, value.asked[shape])
			}
			if def.System != value.def.System {
				t.Errorf("%s changed its prompt in %s", value.name, shape)
			}
		}
		prompt := value.def.System
		if !strings.Contains(prompt, "advertised by fill") {
			t.Errorf("the %s prompt does not leave its cells to fill:\n%s", value.name, prompt)
		}
		for _, column := range value.def.Columns {
			if !regexp.MustCompile("(?m)^- `?" + column.Name + "`?:").MatchString(prompt) {
				t.Errorf("the %s prompt lost the text of its %s cell", value.name, column.Name)
			}
		}
		if demand := regexp.MustCompile(`(?i)\b(one|two|three|four|\d+) cells|every row and nothing else|\b(line|alias)\b[^.]*\b(each|every) result row`).FindString(prompt); demand != "" {
			t.Errorf("the %s prompt demands cells fill may not advertise: %q", value.name, demand)
		}
		// Code decides which names are asked an alias (lines.NeedsAlias):
		// neither the prompt nor the cell's note leaves the model to judge
		// whether a name is English or clear enough already.
		judged := regexp.MustCompile(`(?i)recogni[sz]able English|\bnot English\b|already (clear|English)|unclear to a newcomer`)
		for _, text := range append([]string{prompt}, columnNotes(value.def)...) {
			if judgement := judged.FindString(text); judgement != "" {
				t.Errorf("the %s table asks the model to judge a name: %q", value.name, judgement)
			}
		}
	}
	// Narrowed by captions and by name, a table is marked once: an English
	// type asks its line alone, with or without captions, in one request
	// shape and memo.
	if english := withoutCaptions(withoutAlias(lines.Types())); english.Contract != lines.Types().Contract+".decisions" ||
		!reflect.DeepEqual(english, withoutAlias(lines.Types())) {
		t.Fatalf("an English type is asked another table with and without captions: %s %v", english.Contract, columnNames(english))
	}
}

// Only a declaration whose name is not written in Latin letters is asked its
// English alias, with or without --captions (owner decision 2026-09-26). The
// alias follows that declaration's knowledge into the atlas, and an English
// name is never offered the cell.
func TestSymbolAndTypeAliasesFollowExistingKnowledgeWithoutRenamingDeclarations(t *testing.T) {
	for _, captions := range []bool{false, true} {
		name := "without captions"
		if captions {
			name = "with captions"
		}
		t.Run(name, func(t *testing.T) { testAliasesAskedByName(t, captions) })
	}
}

func testAliasesAskedByName(t *testing.T, captions bool) {
	graph := knowledgeGraph(t)
	rename := func(decl *atlas.Decl) {
		switch decl.Name {
		case "Main":
			decl.Name, decl.Doc = "시작", "Starts the program."
		case "Z":
			decl.Name, decl.Doc = "주가정보", "Represents a daily stock quote."
		}
	}
	// An English type beside the Korean one; help is the English function.
	quote := atlas.Decl{Name: "Quote", Kind: "type", LineNo: 9, Exported: true, Doc: "Quote is one traded price.", ObjectID: "quote"}
	for i := range graph.Places {
		place := &graph.Places[i]
		if place.File != nil {
			for j := range place.File.Decls {
				rename(&place.File.Decls[j])
			}
			if place.Path == "pkg/b/z.go" {
				place.File.Decls = append(place.File.Decls, quote)
			}
		}
		if place.Symbol != nil {
			rename(&place.Symbol.Decl)
			place.ID = atlas.SymbolID(place.Path, place.LineNo, place.Symbol.Decl.Name)
		}
	}
	graph.Places = append(graph.Places, atlas.Place{ID: atlas.SymbolID("pkg/b/z.go", quote.LineNo, quote.Name), Kind: atlas.PlaceSymbol,
		Path: "pkg/b/z.go", LineNo: quote.LineNo, Parent: atlas.FileID("pkg/b/z.go"), TargetIDs: []string{"t1"}, Given: quote.Name,
		Symbol: &atlas.SymbolFacts{Decl: quote, Candidate: true, Rank: 2}})
	// The program these declarations come from, for the report: the graph
	// names its objects by the index's own IDs.
	program := aliasProgram(t, graph)
	want := make(map[string]atlas.Decl)
	for i := range graph.Places {
		place := &graph.Places[i]
		if place.File != nil {
			for j := range place.File.Decls {
				place.File.Decls[j].ObjectID = program.ids[place.File.Decls[j].ObjectID]
			}
		}
		if place.Symbol != nil {
			place.Symbol.Decl.ObjectID = program.ids[place.Symbol.Decl.ObjectID]
			if place.Symbol.Decl.Name != "Gen" {
				want[place.Symbol.Decl.ObjectID] = place.Symbol.Decl
			}
		}
	}
	atlas.SortPlaces(graph.Places)
	encoded, err := atlas.EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	if graph, err = atlas.DecodeGraph(encoded); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(graph)
	aliases := map[string]string{"시작": "program start", "주가정보": "stock quote"}
	// Each description request asks the alias of a Korean name and never of
	// an English one. Without captions a symbol is asked its alias alone,
	// and an English symbol nothing at all.
	asked := map[string][]string{"시작": {"alias"}, "주가정보": {"line", "alias"}, "Quote": {"line"}}
	if captions {
		asked = map[string][]string{"시작": {"line", "alias"}, "help": {"line"}, "주가정보": {"line", "alias"}, "Quote": {"line"}}
	}
	// Here every name is its own request: functions and types, with and
	// without an alias. None reads another, so they are asked side by side.
	provider := &aliasProvider{aliases: aliases, together: len(asked)}
	cache := t.TempDir()
	opts := readOptions(t, graph, provider, cache)
	opts.NoCaptions = !captions
	first, err := Read(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.asked) != len(asked) {
		t.Fatalf("described declarations %v, want %v", provider.asked, asked)
	}
	if provider.peak != len(asked) {
		t.Fatalf("%d of %d description requests were in flight at once", provider.peak, len(asked))
	}
	for name, cells := range asked {
		if requests := provider.asked[name]; len(requests) != 1 || !slices.Equal(requests[0], cells) {
			t.Fatalf("%s was asked %v, want one request for %v", name, requests, cells)
		}
	}
	assertAliases := func(result Result) {
		t.Helper()
		seen := make(map[string]bool)
		for _, target := range result.Atlas.Targets {
			for _, box := range target.Boxes {
				for _, file := range box.Files {
					for _, symbol := range file.Symbols {
						decl, known := want[symbol.ObjectID]
						if !known {
							if symbol.Alias != "" {
								t.Fatal("unreviewed declaration acquired an alias")
							}
							continue
						}
						seen[symbol.ObjectID] = true
						if symbol.Alias != aliases[decl.Name] || symbol.Name != decl.Name || symbol.ObjectID != decl.ObjectID || symbol.LineNo != decl.LineNo || symbol.Column != decl.Column || symbol.Signature != decl.Signature || symbol.Doc != decl.Doc {
							t.Fatalf("English label replaced a native declaration or lost its model value: %+v", symbol)
						}
						// Without captions a function has no line; every
						// declaration here stays a key.
						if (symbol.Line == "") != (!captions && symbol.Kind != "type") || !symbol.Key {
							t.Fatalf("an alias changed a declaration's line or key: %+v", symbol)
						}
					}
				}
			}
		}
		if len(seen) != len(want) {
			t.Fatalf("only %d of %d reviewed declarations were published", len(seen), len(want))
		}
	}
	assertAliases(first)
	// Selection asks every declaration once; each description request asks
	// one table of names that need an alias or of names that need none.
	windows := 5
	if captions {
		windows = 6
	}
	for _, use := range first.Uses {
		if use.Stage == lines.StageSymbols && (use.Rows != 8 || use.Windows != windows) {
			t.Fatalf("selection and caption accounting: %+v", use)
		}
	}
	raw, err := os.ReadFile(filepath.Join(opts.OwnerRunDir, KnowledgeFilename))
	if err != nil {
		t.Fatal(err)
	}
	var knowledge struct{ Records []Knowledge }
	if err := json.Unmarshal(raw, &knowledge); err != nil {
		t.Fatal(err)
	}
	described := make(map[string]bool)
	for _, record := range knowledge.Records {
		decl, known := want[record.SubjectID]
		if _, selection := record.Cells["key_symbol"]; !known || selection {
			continue
		}
		alias, asked := record.Cells[lines.AliasColumn]
		if described[decl.Name] || asked != lines.NeedsAlias(decl.Name) || alias != aliases[decl.Name] || record.OriginRequest == "" {
			t.Fatalf("alias lost its accepted knowledge provenance: %+v", record)
		}
		described[decl.Name] = true
	}
	if len(described) != len(asked) {
		t.Fatalf("described knowledge %v, want %v", described, asked)
	}
	// The report shows each alias beside its native name: the type's in the
	// glossary, the function's on its key code. An English name keeps its own.
	assertReport := func(result Result) {
		t.Helper()
		page := aliasReport(t, program.index, result.Atlas)
		// Read the actual embedded payload: the ordinary renderer may gzip
		// its physical representation without changing any saved reading.
		block := regexp.MustCompile(`<script type="application/json" id="rm-page-data"( data-rm-encoding="gzip-base64")?>([^<]*)</script>`).FindStringSubmatch(page)
		if len(block) != 3 {
			t.Fatal("missing report reading payload")
		}
		data := []byte(block[2])
		if block[1] != "" {
			packed, err := base64.StdEncoding.DecodeString(block[2])
			if err != nil {
				t.Fatal(err)
			}
			reader, err := gzip.NewReader(bytes.NewReader(packed))
			if err != nil {
				t.Fatal(err)
			}
			data, err = io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			if err := reader.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if !json.Valid(data) || !bytes.Contains(data, []byte(`"alias":"program start"`)) {
			t.Fatal("the complete report reading lost the function's accepted alias")
		}
		for _, shown := range []string{
			`data-term-name="stock quote" data-term-original="주가정보"`, `data-term-name="Quote" data-term-original="Quote"`,
		} {
			if !strings.Contains(page, shown) {
				t.Fatalf("the report does not show %s", shown)
			}
		}
		shown := func(attribute string) []string {
			var values []string
			for _, match := range regexp.MustCompile(attribute+`="([^"]*)"`).FindAllStringSubmatch(page, -1) {
				if match[1] != "" && !slices.Contains(values, match[1]) {
					values = append(values, match[1])
				}
			}
			slices.Sort(values)
			return values
		}
		if terms := shown("data-term-name"); !slices.Equal(terms, []string{"Quote", "stock quote"}) {
			t.Fatalf("the report shows glossary names %v", terms)
		}
	}
	assertReport(first)
	// The empty provider would answer none. An exact warm replay must restore
	// the original accepted aliases along with their descriptions, without calls.
	warm := &aliasProvider{}
	opts.Provider, opts.OwnerRunDir = warm, t.TempDir()
	second, err := Read(t.Context(), opts)
	if err != nil || warm.calls != 0 {
		t.Fatalf("alias did not reuse existing knowledge: calls %d, %v", warm.calls, err)
	}
	assertAliases(second)
	assertReport(second)
	after, _ := json.Marshal(graph)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("alias interpretation changed the source graph")
	}
}

// aliasIndex is the program index of a reading graph's declarations, with the
// index ID of each declaration's graph object ID.
type aliasIndex struct {
	index programindex.Index
	ids   map[string]string
}

func aliasProgram(t *testing.T, graph atlas.Graph) aliasIndex {
	t.Helper()
	var objects []programindex.ObjectInput
	var sources []programindex.TargetSource
	for _, place := range graph.Places {
		if place.File == nil {
			continue
		}
		sources = append(sources, programindex.TargetSource{FileRef: fmt.Sprintf("f%d", len(sources)+1), Path: place.Path})
		for _, decl := range place.File.Decls {
			kind := programindex.ObjectFunction
			if decl.Kind == "type" {
				kind = programindex.ObjectType
			}
			objects = append(objects, programindex.ObjectInput{SourceRef: decl.ObjectID, Kind: kind, Name: decl.Name, Visibility: programindex.VisibilityPublic,
				Location: &programindex.Location{Path: place.Path, Line: decl.LineNo, Column: max(1, decl.Column)}})
		}
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target:   programindex.TargetInput{Language: "go", Kind: "executable", Name: "example.com/x", Selector: "go:example.com/x", Sources: sources, AnchorFileRef: "f1"},
		Objects:  objects,
		Coverage: programindex.CoverageInput{Measured: true, ObjectsObserved: len(objects)},
	})
	if err != nil {
		t.Fatal(err)
	}
	program := aliasIndex{index: index, ids: make(map[string]string)}
	for _, object := range index.Objects {
		for _, input := range objects {
			if input.Location.Path == object.Location.Path && input.Location.Line == object.Location.Line && input.Name == object.Name {
				program.ids[input.SourceRef] = object.ID
			}
		}
	}
	if len(program.ids) != len(objects) {
		t.Fatalf("program index lost a declaration: %+v", index.Objects)
	}
	return program
}

// aliasReport renders the ordinary report page of one reading.
func aliasReport(t *testing.T, index programindex.Index, value atlas.Atlas) string {
	t.Helper()
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{index.Target.ID: index}, value)
	if err != nil {
		t.Fatal(err)
	}
	portfolio, err := report.NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	groups, err := report.NewGroupGraphView(indexes, index.Target.ID)
	if err != nil {
		t.Fatal(err)
	}
	page := report.TargetNavigationPage{RunID: "20260926-120000-alias-a1b2c3", ProgramTarget: index.Target.Snapshot(), ArtifactFilename: programindex.ArtifactFilename}
	selected, err := targetoutcome.NewSelectedTarget(index.Target.ID, targetoutcome.LanguageGroupGo, targetoutcome.ScopeExecutable, index.Target.Name, index.Target.Selector)
	if err != nil {
		t.Fatal(err)
	}
	analyzed, err := targetoutcome.NewAnalyzed(selected, index.Target, page.RunID)
	if err != nil {
		t.Fatal(err)
	}
	outcomes, err := targetoutcome.Build(selected.ID, []targetoutcome.Outcome{analyzed})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := programpage.Build(index.Target.ID, []programpage.Page{{Target: index.Target.Snapshot(), RunID: page.RunID}})
	if err != nil {
		t.Fatal(err)
	}
	outcomeView, err := report.NewTargetOutcomePortfolioView(outcomes, pages)
	if err != nil {
		t.Fatal(err)
	}
	navigation, err := report.BuildTargetNavigation([]report.TargetNavigationPage{page}, index.Target.ID, index.Target.ID)
	if err != nil {
		t.Fatal(err)
	}
	data := report.ReportData{FormatVersion: report.CurrentFormatVersion, RepoName: "x", CapturedRevision: strings.Repeat("a", 40),
		ProgramPortfolio: portfolio, GroupGraph: groups, TargetOutcomePortfolio: outcomeView}
	html, err := report.RenderHTMLWithOptions(&data, report.RenderOptions{TargetNavigation: navigation})
	if err != nil {
		t.Fatal(err)
	}
	return string(html)
}
