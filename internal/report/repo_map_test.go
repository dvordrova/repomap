package report

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/targetoutcome"
)

const repoNodeWidth, repoNodeHeight = 184.0, 74.0 // Small arrow-obstacle fixtures.

func TestRootComponentKeepsItsNativeNameAcrossNavigationAndTranslation(t *testing.T) {
	sections := []*pageSection{
		{ID: "app", Name: "example.org/native", Root: ".", Kind: "executable"},
		{ID: "lib", Name: "example.org/native", Root: ".", Kind: "library", Map: &pageMap{Nodes: []pageMapNode{{ID: "group", FullTitle: "Root"}}}},
		{ID: "nested", Name: "Root", Root: "nested/path", Kind: "library"},
	}
	labelSections(sections)
	if sections[0].ShortLabel != "example.org/native (executable)" || sections[1].ShortLabel != "example.org/native (library)" || sections[2].ShortLabel != "nested/path" {
		t.Fatalf("native component labels lost: %+v", sections)
	}
	page := &PreparedPage{view: &pageView{Sections: sections}}
	page.rebuildDisplayLabels(Russian)
	if sections[0].ShortLabel != "example.org/native (исполняемый компонент)" || sections[1].ShortLabel != "example.org/native (библиотека)" {
		t.Fatalf("translated navigation renamed the native component: %q / %q", sections[0].ShortLabel, sections[1].ShortLabel)
	}
	if sections[2].Name != "Root" || sections[2].Root != "nested/path" || sections[1].Map.Nodes[0].FullTitle != "Root" {
		t.Fatal("component display changed a native name, source path or model group title")
	}
}

func TestTranslatedCrossComponentLabelsKeepTheirExactDestinations(t *testing.T) {
	for _, language := range []DisplayLanguage{English, Russian} {
		t.Run(string(language), func(t *testing.T) {
			peer := &pageSection{ID: "peer", programTargetID: "peer-id", Name: "test.command", Root: "test", Kind: "executable", Triggers: []pageGroup{{ID: "peer-group", Title: "Набор тестов"}}}
			sameRoot := &pageSection{ID: "sibling", programTargetID: "sibling-id", Name: "test.other", Root: "test", Kind: "executable"}
			here := &pageSection{ID: "here", programTargetID: "here-id", Name: "jieba", Root: "jieba", Kind: "library"}
			sections := []*pageSection{here, peer, sameRoot}
			labelSections(sections)
			const original = "test.command (executable) / cut"
			here.Map = &pageMap{Nodes: []pageMapNode{
				{ID: "remote", Component: "peer-id", Href: "#peer-group", FullTitle: original, CanonicalTitle: original},
				{ID: "native", Component: "here-id", Href: "#local", FullTitle: original, CanonicalTitle: original},
				{ID: "unbound", Component: "unknown", Href: "#missing", FullTitle: original, CanonicalTitle: original},
			}}
			here.Triggers = []pageGroup{{ID: "local", Connections: []pageConnection{
				{Href: "#peer-group", OtherTarget: peer.ShortLabel, Title: "Набор тестов", Arrow: "→", Label: "uses", Possible: true},
				{Href: "#missing", OtherTarget: "test (executable)", Title: "unchanged"},
			}}}
			page := &PreparedPage{view: &pageView{Sections: sections}}
			page.rebuildDisplayLabels(language)
			wantPrefix := "test.command (" + englishUI(language, "executable") + ")"
			if node := here.Map.Nodes[0]; node.FullTitle != wantPrefix+" / cut" || node.CanonicalTitle != original || node.ID != "remote" || node.Component != "peer-id" || node.Href != "#peer-group" {
				t.Fatalf("remote display lost its exact identity or native title: %+v", node)
			}
			for _, node := range here.Map.Nodes[1:] {
				if node.FullTitle != original {
					t.Fatalf("a name without matching remote ownership was rewritten: %+v", node)
				}
			}
			links := here.Triggers[0].Connections
			if link := links[0]; link.OtherTarget != wantPrefix || link.Href != "#peer-group" || link.Title != "Набор тестов" || link.Arrow != "→" || link.Label != "uses" || !link.Possible {
				t.Fatalf("cross-component link lost its destination or relation: %+v", link)
			}
			if links[1].OtherTarget != "test (executable)" || peer.Root != "test" || peer.Name != "test.command" {
				t.Fatal("unknown destination, path, or native name was guessed")
			}
		})
	}
}

func TestDisconnectedRepositoryMapKeepsNativeIdentityAndDefaultFirst(t *testing.T) {
	sections := []*pageSection{
		{ID: "library", programTargetID: "library-id", Name: "example.org/server", Kind: "library", Root: "."},
		{ID: "tool", programTargetID: "tool-id", Name: "scripts.patch", Kind: "executable", Root: "scripts"},
		{ID: "server", programTargetID: "server-id", Name: "example.org/server", Kind: "executable", Root: "."},
	}
	labelSections(sections)
	data := &ReportData{TargetOutcomePortfolio: &TargetOutcomePortfolioView{
		DefaultSelectedTargetID: "selected-server",
		Outcomes: []TargetOutcomeView{
			{SelectedTargetID: "selected-library", ProgramTargetID: "library-id", State: targetoutcome.StateAnalyzed},
			{SelectedTargetID: "selected-tool", ProgramTargetID: "tool-id", State: targetoutcome.StateAnalyzed},
			{SelectedTargetID: "selected-server", ProgramTargetID: "server-id", State: targetoutcome.StateAnalyzed},
		},
	}}
	builder := &pageBuilder{data: data, sections: sections}
	result := builder.buildRepoMap(&pageView{})
	if len(result.Edges) != 0 || len(result.Nodes) != 3 {
		t.Fatalf("component layout invented or lost graph content: %+v", result)
	}
	want := []string{"repo-server", "repo-library", "repo-tool"}
	for i, node := range result.Nodes {
		if node.ID != want[i] || node.Default != (i == 0) {
			t.Fatalf("default or stable order lost: %+v", result.Nodes)
		}
		if node.Y != result.Nodes[0].Y || (i > 0 && node.X <= result.Nodes[i-1].X+result.Nodes[i-1].Width) {
			t.Fatalf("unconnected cards are not a compact non-overlapping row: %+v", result.Nodes)
		}
	}
	if got := result.Nodes[0]; got.NativeName != "example.org/server" || got.Root != "." || got.Kind != "executable" {
		t.Fatalf("native component identity was replaced by Root or presentation text: %+v", got)
	}
	// A failed default remains first and disabled; it does not promote an
	// arbitrary successful component to be the repository's default.
	data.TargetOutcomePortfolio.Outcomes[2].State = targetoutcome.StateNotAnalyzed
	data.TargetOutcomePortfolio.Outcomes[2].ProgramTargetID = ""
	data.TargetOutcomePortfolio.Outcomes[2].DisplayName = "failed server"
	builder.sections = sections[:2]
	result = builder.buildRepoMap(&pageView{})
	if first := result.Nodes[0]; !first.Default || first.Analyzed || first.Href != "" || first.NativeName != "failed server" {
		t.Fatalf("failed default was hidden or made openable: %+v", first)
	}
}

func TestPortfolioArrowStaysOutsideItsColumn(t *testing.T) {
	top := &pageRepoNode{X: 20, Y: 20, Width: repoNodeWidth, Height: repoNodeHeight}
	middle := &pageRepoNode{X: 20, Y: 180, Width: repoNodeWidth, Height: repoNodeHeight}
	bottom := &pageRepoNode{X: 20, Y: 340, Width: repoNodeWidth, Height: repoNodeHeight}
	nodes := map[string]*pageRepoNode{"top": top, "middle": middle, "bottom": bottom}
	for _, endpoints := range [][2]*pageRepoNode{{top, bottom}, {bottom, top}} {
		edge, _ := repoEdgeGeometry(endpoints[0], endpoints[1], nodes)
		var x0, y0, x1, y1, x2, y2, x3, y3 float64
		if n, err := fmt.Sscanf(edge.Path, "M%f %f C%f %f %f %f %f %f", &x0, &y0, &x1, &y1, &x2, &y2, &x3, &y3); err != nil || n != 8 {
			t.Fatalf("invalid loop %q: %v", edge.Path, err)
		}
		if x3 != endpoints[1].X+endpoints[1].Width || x2 <= x3 {
			t.Fatalf("arrow does not arrive from outside its destination: %s", edge.Path)
		}
		for i := 1; i < 100; i++ {
			u := float64(i) / 100
			x := (1-u)*(1-u)*(1-u)*x0 + 3*(1-u)*(1-u)*u*x1 + 3*(1-u)*u*u*x2 + u*u*u*x3
			if x <= middle.X+middle.Width {
				t.Fatalf("loop crosses its column at t=%f: %s", u, edge.Path)
			}
		}
	}
}

// An arrow between two targets with a third between them on the row ran
// straight through the third box, where it merged with that box's own arrow.
// It swings under the row now, and the map grows to hold it.
func TestPortfolioArrowSwingsUnderABoxInItsWay(t *testing.T) {
	row := func(x float64) *pageRepoNode {
		return &pageRepoNode{X: x, Y: 20, Width: repoNodeWidth, Height: repoNodeHeight}
	}
	left, middle, right := row(20), row(320), row(620)
	nodes := map[string]*pageRepoNode{"left": left, "middle": middle, "right": right}

	straight, straightReach := repoEdgeGeometry(middle, right, nodes)
	// Straight: the label sits just above the row's centre line and the arrow
	// reaches no lower than the boxes do.
	if straight.LabelY >= 20+repoNodeHeight/2 || straightReach > 20+repoNodeHeight {
		t.Errorf("an arrow with nothing in its way was bent: %s reaching %.0f", straight.Path, straightReach)
	}
	if !strings.HasPrefix(straight.Path, "M504.0 57.0 C") {
		t.Errorf("straight arrow leaves from the wrong place: %s", straight.Path)
	}
	detour, reach := repoEdgeGeometry(left, right, nodes)
	if detour.LabelY <= straight.LabelY || reach <= straightReach {
		t.Errorf("the arrow over a box did not swing under the row: label %.0f, reach %.0f", detour.LabelY, reach)
	}
}

// Dense rows reproduce the overview bug: a diagonal could cross an unrelated
// box even though no box shared the endpoints' row. Check both directions,
// nearby and distant endpoints, and the compact grid's narrower gutters.
func TestPortfolioArrowsAvoidEveryCard(t *testing.T) {
	for _, gap := range []float64{repoCalledGapX, repoNodeGapX} {
		nodes := make(map[string]*pageRepoNode)
		for row := 0; row < 5; row++ {
			for column := 0; column < 4; column++ {
				id := fmt.Sprintf("%d/%d", row, column)
				nodes[id] = &pageRepoNode{ID: id,
					X:     mapPadding + float64(column)*(repoNodeWidth+gap),
					Y:     repoRowGap + float64(row)*(repoNodeHeight+repoNodeGapY),
					Width: repoNodeWidth, Height: repoNodeHeight,
				}
			}
		}
		for _, from := range nodes {
			for _, to := range nodes {
				if from == to {
					continue
				}
				edge, reach := repoEdgeGeometry(from, to, nodes)
				points := sampleRepoPath(t, edge.Path)
				end, before := points[len(points)-1], points[len(points)-2]
				if end[1] != to.Y+to.Height/2 || (end[0] != to.X && end[0] != to.X+to.Width) {
					t.Fatalf("arrow misses destination %s: %s", to.ID, edge.Path)
				}
				if (end[0] == to.X && before[0] >= end[0]) || (end[0] > to.X && before[0] <= end[0]) {
					t.Fatalf("arrowhead points away from destination %s: %s", to.ID, edge.Path)
				}
				for _, point := range points {
					if point[1] > reach+0.01 {
						t.Fatalf("arrow leaves canvas: %s", edge.Path)
					}
					for _, box := range nodes {
						if point[0] > box.X+0.01 && point[0] < box.X+box.Width-0.01 && point[1] > box.Y+0.01 && point[1] < box.Y+box.Height-0.01 {
							t.Fatalf("gap %.0f: %s -> %s crosses %s at %v: %s", gap, from.ID, to.ID, box.ID, point, edge.Path)
						}
					}
				}
			}
		}
	}
}

func sampleRepoPath(t *testing.T, path string) [][2]float64 {
	t.Helper()
	reader := strings.NewReader(strings.NewReplacer("M", "M ", "C", "C ", "L", "L ").Replace(path))
	var command string
	var current [2]float64
	if n, err := fmt.Fscan(reader, &command, &current[0], &current[1]); err != nil || n != 3 || command != "M" {
		t.Fatalf("invalid path: %s", path)
	}
	points := [][2]float64{current}
	for reader.Len() > 0 {
		if _, err := fmt.Fscan(reader, &command); err != nil {
			t.Fatal(err)
		}
		var a, b, end [2]float64
		switch command {
		case "C":
			if _, err := fmt.Fscan(reader, &a[0], &a[1], &b[0], &b[1], &end[0], &end[1]); err != nil {
				t.Fatal(err)
			}
		case "L":
			if _, err := fmt.Fscan(reader, &end[0], &end[1]); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("unsupported path command %s", command)
		}
		for i := 1; i <= 200; i++ {
			u := float64(i) / 200
			var point [2]float64
			for axis := range point {
				if command == "L" {
					point[axis] = (1-u)*current[axis] + u*end[axis]
				} else {
					point[axis] = (1-u)*(1-u)*(1-u)*current[axis] + 3*(1-u)*(1-u)*u*a[axis] + 3*(1-u)*u*u*b[axis] + u*u*u*end[axis]
				}
			}
			points = append(points, point)
		}
		current = end
	}
	return points
}

// A target owns the packages it indexed. chi's example executable imports
// github.com/go-chi/chi/v5/_examples/versions/presenter/v2, which the example
// library beside it indexed as versions/presenter/v2 and which also sits under
// chi/v5's module root; it belongs to the library, and the map showed the
// library reached by nothing while chi/v5 was credited with its symbols.
func TestOwnerOfPackageIsWhoIndexedIt(t *testing.T) {
	executable := &pageSection{Name: "versions", Label: "versions (executable)", Kind: "executable"}
	library := &pageSection{Name: "versions", Label: "versions (library)", Kind: "library"}
	chi := &pageSection{Name: "github.com/go-chi/chi/v5", Label: "github.com/go-chi/chi/v5", Kind: "library", Root: "."}
	rest := &pageSection{Name: "rest-example", Label: "rest-example", Kind: "executable", Root: "_examples/rest"}
	executable.Root, library.Root = "_examples/versions", "_examples/versions"
	owners := []packageOwner{
		{section: rest, packages: []string{"rest-example"}},
		{section: executable, packages: []string{"versions", "versions", "versions (executable)"}},
		{section: library, packages: []string{"versions/data", "versions/presenter/v1", "versions/presenter/v2", "versions", "versions (library)"}},
		{section: chi, packages: []string{"github.com/go-chi/chi/v5/middleware", "github.com/go-chi/chi/v5"}},
	}
	for path, want := range map[string]*pageSection{
		"github.com/go-chi/chi/v5/_examples/versions/presenter/v2": library,
		// The report knows only the default target's packages; the others
		// are owned through the directory they live in.
		"github.com/go-chi/chi/v5/_examples/rest/handlers": rest,
		"github.com/go-chi/chi/v5/_examples/versions/data": library,
		"github.com/go-chi/chi/v5/middleware":              chi,
		"github.com/go-chi/chi/v5":                         chi,
		"github.com/go-chi/chi/v5/somewhere/new":           chi,
		"versions":                                         library,
		"net/http":                                         nil,
	} {
		if got := ownerOfPackage(path, owners); got != want {
			t.Errorf("ownerOfPackage(%q) = %s, want %s", path, label(got), label(want))
		}
	}
}

func label(section *pageSection) string {
	if section == nil {
		return "nil"
	}
	return section.Label
}

func TestPartsOfOneMapDoNotShareAColour(t *testing.T) {
	frames := []pageMapFrame{{ID: "a", Zone: 4}, {ID: "b", Zone: 4}, {ID: "c", Zone: 3}, {ID: "d", Zone: 3}, {ID: "e", Zone: 8}}
	spreadZones(frames)
	seen := make(map[int]bool)
	for _, frame := range frames {
		if seen[frame.Zone] {
			t.Fatalf("zones: %+v", frames)
		}
		seen[frame.Zone] = true
	}
	if frames[0].Zone != 4 || frames[2].Zone != 3 || frames[4].Zone != 8 {
		t.Fatalf("hashed colours were not kept: %+v", frames)
	}
}
