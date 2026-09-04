package report

import (
	"strings"
	"testing"
)

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
	if !repoRowHasBoxBetween(left, right, nodes) {
		t.Fatal("the middle box was not seen as in the way")
	}
	if detour.LabelY <= straight.LabelY || reach <= straightReach {
		t.Errorf("the arrow over a box did not swing under the row: label %.0f, reach %.0f", detour.LabelY, reach)
	}
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
