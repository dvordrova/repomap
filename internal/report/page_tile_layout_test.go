package report

import "testing"

// A part's tiles as a layered graph (page_tile_layout.go): a callee never
// stands left of its caller, calls round a cycle share a column, a type's
// methods stand in its block, and the order in a column follows the blocks
// it links to, so the links do not cross.
func TestTilesAreOrderedAsALayeredGraph(t *testing.T) {
	symbols := []pageNodeSymbol{
		{Name: "a", Kind: "function", Path: "x.go"}, // 0 calls d
		{Name: "b", Kind: "function", Path: "x.go"}, // 1 calls c
		{Name: "c", Kind: "function", Path: "x.go"}, // 2
		{Name: "d", Kind: "function", Path: "x.go"}, // 3
		{Name: "T", Kind: "type", Path: "x.go"},     // 4
		{Name: "m", Kind: "method", Owner: 5},       // 5, in T's block; calls b
		{Name: "p", Kind: "function", Path: "x.go"}, // 6 calls q
		{Name: "q", Kind: "function", Path: "x.go"}, // 7 calls p: a cycle
	}
	links := [][2]int{{0, 3}, {1, 2}, {5, 1}, {6, 7}, {7, 6}}
	layTiles(symbols, links)
	col := func(i int) int { return symbols[i].Col }
	if col(3) <= col(0) || col(2) <= col(1) || col(1) <= col(4) {
		t.Fatalf("a callee left of its caller: %+v", symbols)
	}
	if col(6) != col(7) {
		t.Fatalf("a cycle split across columns: p %d, q %d", col(6), col(7))
	}
	if symbols[5].Col != 0 || symbols[5].Ord != 0 {
		t.Fatalf("a member carries a place of its own: %+v", symbols[5])
	}
	// In column 1, b (called from T) and d (called from a): a is listed
	// first in column 0, so d stands before b and the links do not cross.
	if col(1) == col(3) && (symbols[3].Ord < symbols[1].Ord) != (symbols[0].Ord < symbols[4].Ord) {
		t.Fatalf("links cross: a %d T %d / d %d b %d", symbols[0].Ord, symbols[4].Ord, symbols[3].Ord, symbols[1].Ord)
	}
}
