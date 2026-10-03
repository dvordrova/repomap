package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/report"
	"github.com/dvordrova/repomap/internal/reportserver"
)

// kvd's strbuf.c writes sbRewind and sbTruncate on one line (strbuf.c:90,
// columns 6 and 49), and freeClient calls both. clang keeps them two
// declarations; the page had keyed a declaration by its link, which names
// no column, so the part's reading listed sbRewind alone, sbTruncate's tile
// read sbRewind and freeClient's calls named sbRewind twice over (review,
// 2026-10-03, reproduced through this path). The cumulative fixture is read
// by the ordinary run without the model; its map of parts, one part per
// file as the C preset draws it, is projected by the ordinary GroupsIndex
// projection and the ordinary HTML is rendered with GitHub links.
func TestCDeclarationsWrittenOnOneLineAreReadApart(t *testing.T) {
	root, _ := cumulativeEvidenceRepository(t, "c")
	debugDir := t.TempDir()
	var console strings.Builder
	runErr := runDefaultWithDeps(root, []string{"--no-model", "--target", "c:kvd", "--no-open", "--debug-dir", debugDir}, defaultRunDeps{
		ctx: t.Context(), stdout: &console, stderr: &console,
		serveReport: func(context.Context, reportserver.Options) error { return nil },
		openReport:  func(string) error { return nil },
	})
	if runErr != nil {
		t.Fatalf("run: %v\n%s", runErr, console.String())
	}
	latest := filepath.Join(debugDir, "latest")
	restored, err := report.ReadRunReceipt(latest)
	if err != nil {
		t.Fatal(err)
	}
	data := restored.Data()
	value, err := atlas.Read(latest)
	if err != nil {
		t.Fatal(err)
	}
	for i := range value.Targets {
		target := &value.Targets[i]
		for _, off := range target.OffMap {
			box := atlas.Box{ID: fmt.Sprintf("p%d", len(target.Boxes)+1), Dir: path.Dir(off.File.Path), Title: off.File.Path, Side: "mid", Open: true, Files: []atlas.File{off.File}}
			for _, symbol := range off.File.Symbols {
				box.MemberIDs = append(box.MemberIDs, symbol.ObjectID)
			}
			if len(box.MemberIDs) > 0 {
				target.Boxes = append(target.Boxes, box)
			}
		}
		target.OffMap, target.MapFailure = nil, ""
	}
	programs := map[string]programindex.Index{}
	for _, entry := range data.ProgramPortfolio.Entries {
		programs[entry.Target.ID] = entry
	}
	indexes, err := groupindex.ProjectAtlas(programs, value)
	if err != nil {
		t.Fatal(err)
	}
	if err := report.BindGroupGraphView(data, indexes); err != nil {
		t.Fatal(err)
	}
	for _, links := range []string{"GitHub", "GitLab", "no remote"} {
		t.Run(links, func(t *testing.T) {
			data.GitHubSourceLinks, data.GitLabSourceLinks = nil, nil
			switch links {
			case "GitHub":
				data.GitHubSourceLinks = &report.GitHubSourceLinks{RepositoryURL: "https://github.com/example/kvd", Revision: data.CapturedRevision}
			case "GitLab":
				data.GitLabSourceLinks = &report.GitLabSourceLinks{RepositoryURL: "https://gitlab.com/example/kvd", Revision: data.CapturedRevision}
			}
			readsApart(t, data, restored.RenderOptions())
		})
	}
}

// readsApart renders the page and reads strbuf.c's two functions on one
// line apart, as the page's script reads them.
func readsApart(t *testing.T, data *report.ReportData, options report.RenderOptions) {
	t.Helper()
	html, err := report.RenderHTMLWithOptions(data, options)
	if err != nil {
		t.Fatal(err)
	}
	page := readPageData(t, html)

	// strbuf.c's part lists both functions among its members, and each tile
	// reads its own declaration.
	part, reading := page.part(t, "strbuf.c")
	var functions []string
	for _, kind := range reading.list("members") {
		if kind.str("kind") == "function" {
			for _, position := range kind.list("decls") {
				functions = append(functions, reading.decl(position.int()).str("name"))
			}
		}
	}
	for _, name := range []string{"sbRewind", "sbTruncate"} {
		if !slices.Contains(functions, name) {
			t.Errorf("strbuf.c's part lists %v, without %s", functions, name)
		}
	}
	read := map[string]string{}
	for _, tile := range page.symbols(t, part) {
		name := tile.str("name")
		if name != "sbRewind" && name != "sbTruncate" {
			continue
		}
		for _, decl := range reading.decls() {
			if page.declKey(decl) == page.tileKey(tile) {
				read[name] = decl.str("name")
			}
		}
	}
	if read["sbRewind"] != "sbRewind" || read["sbTruncate"] != "sbTruncate" {
		t.Errorf("a tile reads %v, not its own declaration", read)
	}
	// kvd.c's part says freeClient calls each of them.
	_, kvd := page.part(t, "kvd.c")
	var callees []string
	for _, own := range kvd.list("own") {
		if kvd.decl(own.get("decl").int()).str("name") != "freeClient" {
			continue
		}
		for _, side := range own.list("callees") {
			for _, end := range side.list("decls") {
				callees = append(callees, kvd.decl(end.get("decl").int()).str("name"))
			}
		}
	}
	for _, name := range []string{"sbRewind", "sbTruncate"} {
		if !slices.Contains(callees, name) {
			t.Errorf("freeClient's calls name %v, without %s", callees, name)
		}
	}
}

// pageValue is one value of the page's data, its shared parts restored.
type pageValue struct {
	raw  any
	page *pageDataView
}

func (value pageValue) get(name string) pageValue {
	object, _ := value.raw.(map[string]any)
	return pageValue{value.page.expand(object[name]), value.page}
}
func (value pageValue) str(name string) string { text, _ := value.get(name).raw.(string); return text }
func (value pageValue) int() int {
	number, _ := value.raw.(json.Number)
	result, _ := number.Int64()
	return int(result)
}
func (value pageValue) list(name string) []pageValue {
	items, _ := value.get(name).raw.([]any)
	result := make([]pageValue, len(items))
	for i, item := range items {
		result[i] = pageValue{value.page.expand(item), value.page}
	}
	return result
}

// decls are a reading's declarations, each restored from the page's table.
func (value pageValue) decls() []pageValue {
	var result []pageValue
	for _, item := range value.list("decls") {
		if index, numbered := item.raw.(json.Number); numbered {
			position, _ := index.Int64()
			result = append(result, value.page.decl(int(position)))
			continue
		}
		result = append(result, item)
	}
	return result
}
func (value pageValue) decl(position int) pageValue { return value.decls()[position] }

// pageDataView is the page's rm-page-data and the elements referring to it.
type pageDataView struct {
	html   string
	data   map[string]any
	shared []any
}

func readPageData(t *testing.T, html []byte) *pageDataView {
	t.Helper()
	match := regexp.MustCompile(`(?s)<script type="application/json" id="rm-page-data">(.*?)</script>`).FindSubmatch(html)
	if match == nil {
		t.Fatal("the page has no data")
	}
	decoder := json.NewDecoder(bytes.NewReader(match[1]))
	decoder.UseNumber()
	view := &pageDataView{html: string(html)}
	if err := decoder.Decode(&view.data); err != nil {
		t.Fatal(err)
	}
	view.shared, _ = view.data["shared"].([]any)
	return view
}

func (view *pageDataView) expand(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		if ref, single := typed["$"]; single && len(typed) == 1 {
			number, _ := ref.(json.Number)
			index, _ := number.Int64()
			return view.expand(view.shared[index])
		}
		result := map[string]any{}
		for key, item := range typed {
			result[key] = view.expand(item)
		}
		// An identity written as the rest of its place (10-ui.js rmPage).
		for _, pair := range [][2]string{{"key", "at"}, {"key", "source"}, {"Key", "Text"}} {
			key, _ := result[pair[0]].(string)
			place, _ := result[pair[1]].(string)
			if strings.HasPrefix(key, ":") && place != "" {
				result[pair[0]] = place + key
			}
		}
		if key, _ := result["decl_key"].(string); strings.HasPrefix(key, ":") {
			path, _ := result["path"].(string)
			line, _ := result["line"].(json.Number)
			result["decl_key"] = path + ":" + line.String() + key
		}
		// A link its place says is written as 1 (10-ui.js rmPage).
		if number, said := result["href"].(json.Number); said && number.String() == "1" {
			if at, _ := result["at"].(string); at != "" {
				base, _ := view.data["base"].(string)
				if colon := strings.LastIndexByte(at, ':'); colon > 0 {
					result["href"] = base + at[:colon] + "#L" + at[colon+1:]
				}
			}
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = view.expand(item)
		}
		return result
	case string:
		if base, _ := view.data["base"].(string); strings.HasPrefix(typed, "\u0001") {
			return base + typed[1:]
		}
	}
	return value
}

func (view *pageDataView) value(index int) pageValue {
	values, _ := view.data["values"].([]any)
	return pageValue{view.expand(values[index]), view}
}

func (view *pageDataView) decl(index int) pageValue {
	decls, _ := view.data["decls"].([]any)
	return pageValue{view.expand(decls[index]), view}
}

// part is the node of the part titled title and its reading.
func (view *pageDataView) part(t *testing.T, title string) (string, pageValue) {
	t.Helper()
	node := regexp.MustCompile(`<a[^>]* id="(n-t\d+-g\d+)"[^>]* data-title="` + regexp.QuoteMeta(title) + `"`).FindStringSubmatch(view.html)
	if node == nil {
		t.Fatalf("no part is titled %s", title)
	}
	group := regexp.MustCompile(`data-map-alias="` + node[1] + `"[^>]* data-reading="(\d+)"`).FindStringSubmatch(view.html)
	if group == nil {
		t.Fatalf("%s has no reading", title)
	}
	var index int
	fmt.Sscan(group[1], &index)
	return node[1], view.value(index)
}

// symbols are a part's tiles, each restored from the declaration it names.
func (view *pageDataView) symbols(t *testing.T, node string) []pageValue {
	t.Helper()
	match := regexp.MustCompile(`<a[^>]* id="` + node + `"[^>]* data-symbols="(\d+)"`).FindStringSubmatch(view.html)
	if match == nil {
		t.Fatalf("%s has no tiles", node)
	}
	var index int
	fmt.Sscan(match[1], &index)
	items, _ := view.value(index).raw.([]any)
	var result []pageValue
	for _, item := range items {
		tile, _ := item.(map[string]any)
		if number, named := tile["d"].(json.Number); named {
			at, _ := number.Int64()
			decl, _ := view.decl(int(at)).raw.(map[string]any)
			for _, field := range []string{"name", "href"} {
				if decl[field] != nil {
					tile[field] = decl[field]
				}
			}
			if decl["key"] != nil {
				tile["decl_key"] = decl["key"]
			}
		}
		result = append(result, pageValue{tile, view})
	}
	return result
}

// declKey and tileKey are how the page's script keys a reading's
// declaration and a tile (26-map-members.js).
func (view *pageDataView) declKey(decl pageValue) string {
	return firstText(decl.str("key"), decl.str("href"), decl.str("open"))
}

func (view *pageDataView) tileKey(tile pageValue) string {
	return firstText(tile.str("decl_key"), tile.str("href"), tile.str("open"))
}

func firstText(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
