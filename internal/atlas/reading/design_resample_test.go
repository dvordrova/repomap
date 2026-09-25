package reading

import (
	"fmt"
	"sync"
	"testing"
)

// partsDraws answers the parts request with no groups for the first refused
// draws, then with one part per package, and counts the parts requests.
type partsDraws struct {
	mu      sync.Mutex
	refused int
	asked   int
}

func (draws *partsDraws) design(mode string, input []map[string]any) designProposals {
	result := designProposals{Groups: []designProposal{}}
	if mode != "parts" {
		return result
	}
	draws.mu.Lock()
	defer draws.mu.Unlock()
	draws.asked++
	if draws.asked <= draws.refused {
		return result
	}
	for _, pkg := range input {
		result.Groups = append(result.Groups, designProposal{Title: fmt.Sprint(pkg["package"]), Purpose: "Reads the supplied declarations."})
	}
	return result
}

// A parts answer with no groups is asked once more with the same request:
// a good second draw draws the map, and a second refusal leaves every
// declaration in its source file after exactly two parts requests.
func TestDesignPartsAreAskedOnceMoreAfterAWholeRefusal(t *testing.T) {
	for _, refused := range []int{1, 2} {
		draws := &partsDraws{refused: refused}
		provider := &tableProvider{designFor: draws.design}
		result, err := Read(t.Context(), readOptions(t, knowledgeGraph(t), provider, t.TempDir()))
		if err != nil {
			t.Fatal(err)
		}
		boxes := boxesByTitle(result)
		_, drawn := boxes["pkg/a"]
		if draws.asked != 2 || drawn != (refused == 1) {
			t.Fatalf("refused %d: parts asked %d times, map drawn %v: %+v", refused, draws.asked, drawn, boxes)
		}
	}
}

// A target without declarations has nothing to split and sends no parts
// request.
func TestDesignSkipsTheRequestForATargetWithoutUnits(t *testing.T) {
	draws := &partsDraws{}
	opts := readOptions(t, knowledgeGraph(t), &tableProvider{designFor: draws.design}, "")
	opts.Targets = append(opts.Targets, TargetMeta{ID: "t2", Language: "go", Kind: "package", Name: "example.com/empty", Root: "empty"})
	if _, err := Read(t.Context(), opts); err != nil {
		t.Fatal(err)
	}
	if draws.asked != 1 {
		t.Fatalf("parts asked %d times for one target with units", draws.asked)
	}
}
