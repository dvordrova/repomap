package report

import (
	"cmp"
	"slices"
)

// layTiles orders a part's tiles as a layered graph, once, where the page is
// built (owner, 2026-10-15: the tiles' links had crossed tiles and run right
// to left; "a precomputation on the back end, like graph theory"). A block is
// a declaration with the members drawn inside it (symbolBlocks in
// symbols.mjs builds the same blocks). Calls running round a cycle make one
// node (strongly connected components), so a callee never stands left of its
// caller; a block's column is the longest chain of links leading to it; the
// blocks of a column are ordered as the page lists them, then by the mean
// place of the blocks they link to (barycentres, sweeping down and up) so
// fewer links cross. The head of each block carries its column and its place
// in the column (Col, Ord); the browser places the tiles in that order and
// fits them to the card's pixels.
func layTiles(symbols []pageNodeSymbol, links [][2]int) {
	n := len(symbols)
	blockOf := make([]int, n)
	for i := range blockOf {
		blockOf[i] = -1
	}
	var heads []int
	for i, symbol := range symbols {
		if symbol.Owner == 0 && symbol.Kind != "skip" {
			blockOf[i] = len(heads)
			heads = append(heads, i)
		}
	}
	for i, symbol := range symbols {
		if symbol.Owner == 0 || symbol.Kind == "skip" {
			continue
		}
		if owner := symbol.Owner - 1; owner >= 0 && owner < n && blockOf[owner] >= 0 && symbols[owner].Owner == 0 {
			blockOf[i] = blockOf[owner]
			continue
		}
		blockOf[i] = len(heads)
		heads = append(heads, i)
	}
	blocks := len(heads)
	if blocks < 2 {
		return
	}
	succ, pred := make([][]int, blocks), make([][]int, blocks)
	seen := map[[2]int]bool{}
	for _, link := range links {
		if link[0] < 0 || link[0] >= n || link[1] < 0 || link[1] >= n {
			continue
		}
		a, b := blockOf[link[0]], blockOf[link[1]]
		if a < 0 || b < 0 || a == b || seen[[2]int{a, b}] {
			continue
		}
		seen[[2]int{a, b}] = true
		succ[a] = append(succ[a], b)
		pred[b] = append(pred[b], a)
	}

	// Strongly connected components (Tarjan), each a node of the DAG.
	component := make([]int, blocks)
	index, low, onStack := make([]int, blocks), make([]int, blocks), make([]bool, blocks)
	for i := range index {
		index[i] = -1
	}
	var stack []int
	next, components := 0, 0
	var connect func(v int)
	connect = func(v int) {
		index[v], low[v] = next, next
		next++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range succ[v] {
			if index[w] < 0 {
				connect(w)
				low[v] = min(low[v], low[w])
			} else if onStack[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] == index[v] {
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				component[w] = components
				if w == v {
					break
				}
			}
			components++
		}
	}
	for v := 0; v < blocks; v++ {
		if index[v] < 0 {
			connect(v)
		}
	}
	// A component's layer: the longest chain of components leading to it.
	// Tarjan numbers a component after every component it reaches, so the
	// numbers taken in decreasing order are a topological order.
	layer := make([]int, components)
	members := make([][]int, components)
	for v, c := range component {
		members[c] = append(members[c], v)
	}
	for c := components - 1; c >= 0; c-- {
		for _, v := range members[c] {
			for _, w := range succ[v] {
				if d := component[w]; d != c && layer[d] < layer[c]+1 {
					layer[d] = layer[c] + 1
				}
			}
		}
	}

	// The page's order: files in the order their first declaration is listed,
	// a file's declarations as listed (symbolBlocks' rank).
	files := map[string]int{}
	for _, symbol := range symbols {
		if symbol.Kind == "skip" {
			continue
		}
		if _, known := files[symbol.Path]; !known {
			files[symbol.Path] = len(files)
		}
	}
	rank := make([]int, blocks)
	for i := range rank {
		rank[i] = int(^uint(0) >> 1)
	}
	for i, symbol := range symbols {
		if b := blockOf[i]; b >= 0 {
			rank[b] = min(rank[b], files[symbol.Path]*n+i)
		}
	}
	columns := 0
	for _, c := range component {
		columns = max(columns, layer[c]+1)
	}
	byColumn := make([][]int, columns)
	for b := 0; b < blocks; b++ {
		column := layer[component[b]]
		byColumn[column] = append(byColumn[column], b)
	}
	place := make([]float64, blocks)
	order := func() {
		for _, column := range byColumn {
			for at, b := range column {
				place[b] = float64(at)
			}
		}
	}
	for _, column := range byColumn {
		slices.SortStableFunc(column, func(a, b int) int { return cmp.Compare(rank[a], rank[b]) })
	}
	order()
	// Barycentres: each block moves to the mean place of the blocks it links
	// to in the columns already ordered; one with none keeps its place.
	sweep := func(columns []int, neighbours [][]int, before func(int, int) bool) {
		for _, c := range columns {
			column := byColumn[c]
			centre := make(map[int]float64, len(column))
			for _, b := range column {
				sum, count := 0.0, 0
				for _, other := range neighbours[b] {
					if before(layer[component[other]], c) {
						sum += place[other]
						count++
					}
				}
				centre[b] = place[b]
				if count > 0 {
					centre[b] = sum / float64(count)
				}
			}
			slices.SortStableFunc(column, func(a, b int) int { return cmp.Compare(centre[a], centre[b]) })
			for at, b := range column {
				place[b] = float64(at)
			}
		}
	}
	down, up := make([]int, 0, columns), make([]int, 0, columns)
	for c := 1; c < columns; c++ {
		down = append(down, c)
	}
	for c := columns - 2; c >= 0; c-- {
		up = append(up, c)
	}
	for range 4 {
		sweep(down, pred, func(other, c int) bool { return other < c })
		sweep(up, succ, func(other, c int) bool { return other > c })
	}
	for c, column := range byColumn {
		for at, b := range column {
			symbols[heads[b]].Col, symbols[heads[b]].Ord = c, at
		}
	}
}
