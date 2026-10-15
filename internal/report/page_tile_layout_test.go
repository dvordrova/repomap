package report

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"
)

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

// The layered order's promises on any graph (the consilium's check for the
// tiles, 2026-10-15): on random parts — types with members, skipped rows,
// cycles, links between members — no link runs right to left except between
// blocks round one cycle, which share a column; each column's places are 0..k-1
// once each; the same input gives the same order.
func TestTilesLayeredOrderHoldsOnAnyGraph(t *testing.T) {
	rng := rand.New(rand.NewSource(20261015))
	for round := 0; round < 400; round++ {
		n := 2 + rng.Intn(24)
		symbols := make([]pageNodeSymbol, n)
		for i := range symbols {
			symbols[i] = pageNodeSymbol{Name: fmt.Sprint("s", i), Kind: "function", Path: fmt.Sprint("f", rng.Intn(3), ".go")}
			switch r := rng.Intn(10); {
			case r == 0:
				symbols[i].Kind = "skip"
			case r < 3 && i > 0:
				symbols[i].Kind, symbols[i].Owner = "method", 1+rng.Intn(i)
			}
		}
		var links [][2]int
		for k := rng.Intn(3 * n); k > 0; k-- {
			links = append(links, [2]int{rng.Intn(n), rng.Intn(n)})
		}
		again := slices.Clone(symbols)
		layTiles(symbols, links)
		layTiles(again, links)
		if !slices.Equal(symbols, again) {
			t.Fatalf("round %d: the same part laid out twice differently", round)
		}

		// Blocks as page_tile_layout.go's comment states them: a head is a
		// declaration of no owner; a member whose owner is a head is in its
		// block; any other row not skipped heads a block of its own.
		head := make([]int, n)
		for i, symbol := range symbols {
			head[i] = -1
			if symbol.Kind == "skip" {
				continue
			}
			head[i] = i
			if o := symbol.Owner - 1; symbol.Owner > 0 && symbols[o].Owner == 0 && symbols[o].Kind != "skip" {
				head[i] = o
			}
		}
		heads := 0
		for i := range symbols {
			if head[i] == i {
				heads++
			}
		}
		if heads < 2 {
			continue
		}
		// Reachability between blocks, to know the cycles.
		reach := make([][]bool, n)
		for i := range reach {
			reach[i] = make([]bool, n)
			reach[i][i] = true
		}
		for _, link := range links {
			if a, b := head[link[0]], head[link[1]]; a >= 0 && b >= 0 {
				reach[a][b] = true
			}
		}
		for k := 0; k < n; k++ {
			for i := 0; i < n; i++ {
				for j := 0; j < n; j++ {
					reach[i][j] = reach[i][j] || reach[i][k] && reach[k][j]
				}
			}
		}
		for _, link := range links {
			a, b := head[link[0]], head[link[1]]
			if a < 0 || b < 0 || a == b {
				continue
			}
			ca, cb := symbols[a].Col, symbols[b].Col
			if reach[b][a] {
				if ca != cb {
					t.Fatalf("round %d: %d and %d are one cycle in columns %d and %d", round, a, b, ca, cb)
				}
			} else if cb <= ca {
				t.Fatalf("round %d: link %d→%d runs right to left (columns %d → %d)", round, a, b, ca, cb)
			}
		}
		places := map[int][]int{}
		for i, symbol := range symbols {
			if head[i] == i {
				places[symbol.Col] = append(places[symbol.Col], symbol.Ord)
			}
		}
		for column, ords := range places {
			slices.Sort(ords)
			for at, ord := range ords {
				if ord != at {
					t.Fatalf("round %d: column %d places %v, not 0..%d once each", round, column, ords, len(ords)-1)
				}
			}
		}
	}
}
