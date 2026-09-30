package groupindex

import (
	"path"
	"strings"
)

// TellApart names each entry of a list so that entries sharing a name are
// told apart: an entry whose name no other shares keeps it, and those that
// share one take, together, the first of their other spellings that differs
// for each of them (spellings[i], in the caller's order of preference, "" for
// one it lacks). A group no spelling tells apart takes the first that
// differs at all, else keeps its name. litestream's Sync reaches eight
// ReplicaClient types, read as s3.ReplicaClient, gs.ReplicaClient, … (the
// column tells same-named inputs apart the same way, by where they stand).
func TellApart(names []string, spellings [][]string) []string {
	result := append([]string(nil), names...)
	groups := map[string][]int{}
	var order []string
	for position, name := range names {
		if _, known := groups[name]; !known {
			order = append(order, name)
		}
		groups[name] = append(groups[name], position)
	}
	for _, name := range order {
		members := groups[name]
		if len(members) < 2 {
			continue
		}
		levels := 0
		for _, position := range members {
			if position < len(spellings) {
				levels = max(levels, len(spellings[position]))
			}
		}
		spelled := func(position, level int) string {
			if position < len(spellings) && level < len(spellings[position]) {
				return spellings[position][level]
			}
			return ""
		}
		chosen := -1
		for _, distinct := range []bool{true, false} {
			for level := 0; level < levels && chosen < 0; level++ {
				seen := map[string]bool{}
				complete := true
				for _, position := range members {
					said := spelled(position, level)
					if said == "" || distinct && seen[said] {
						complete = false
						break
					}
					seen[said] = true
				}
				if complete && len(seen) > 1 {
					chosen = level
				}
			}
		}
		if chosen < 0 {
			continue
		}
		for _, position := range members {
			result[position] = spelled(position, chosen)
		}
	}
	return result
}

// Where are the spellings a declaration's name is told apart by where it
// stands: qualified by its folder (a Go package: s3.ReplicaClient), then by
// its file (a Python module: stored_callbacks.run), then followed by its
// part ("ReplicaClient in Replica storage backends").
func Where(name, file, part string) []string {
	var spellings []string
	if file != "" {
		base := strings.TrimSuffix(path.Base(file), path.Ext(file))
		folder := path.Base(path.Dir(file))
		if folder == "." || folder == "/" || folder == "" {
			folder = base
		}
		spellings = append(spellings, folder+"."+name, base+"."+name)
	} else {
		spellings = append(spellings, "", "")
	}
	if part != "" {
		spellings = append(spellings, name+" in "+part)
	}
	return spellings
}
