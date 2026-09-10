package targetportfolio

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/dvordrova/repomap/internal/corpus"
)

// NativeLaunchGroup is one decision over forms with the same complete native
// callable evidence. Membership itself is not a product placement.
type NativeLaunchGroup struct {
	Ref         string   `json:"ref"`
	Members     []string `json:"members"`
	OwnerRefs   []string `json:"owner_refs"`
	CallableRef string   `json:"callable_ref"`
}

type NativeLaunchDecision struct {
	Ref       string `json:"ref"`
	Owner     string `json:"owner"`
	malformed bool
}

// A malformed group choice loses only that group's decision. The enclosing
// provider envelope must still be valid JSON.
func (d *NativeLaunchDecision) UnmarshalJSON(raw []byte) error {
	*d = NativeLaunchDecision{}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		d.malformed = true
		return nil
	}
	if json.Unmarshal(fields["ref"], &d.Ref) != nil || json.Unmarshal(fields["owner"], &d.Owner) != nil {
		d.malformed = true
	}
	return nil
}

func nativeLaunchGroups(native []NativeCandidate) []NativeLaunchGroup {
	rows, observations := nativeRequest(native)
	entries := make(map[string]bool)
	for _, observation := range observations {
		if observation.Kind == "launch_callable" && observation.Path != "" && observation.Line > 0 && len(observation.Values) == 3 && observation.Values[2] == "arguments=none" {
			entries[observation.Ref] = true
		}
	}
	type identity struct{ language, root, callable string }
	byEntry := make(map[identity][]NativeCandidate)
	var order []identity
	for _, row := range rows {
		if row.Kind != "executable" {
			continue
		}
		var entry string
		count := 0
		for _, ref := range row.EvidenceRefs {
			if entries[ref] {
				entry, count = ref, count+1
			}
		}
		if count != 1 {
			continue
		}
		key := identity{row.Language, row.Root, entry}
		if len(byEntry[key]) == 0 {
			order = append(order, key)
		}
		byEntry[key] = append(byEntry[key], row)
	}
	var groups []NativeLaunchGroup
	for _, key := range order {
		members := byEntry[key]
		if len(members) < 2 {
			continue
		}
		group := NativeLaunchGroup{Ref: fmt.Sprintf("g%d", len(groups)+1), CallableRef: key.callable}
		for _, owner := range members {
			group.Members = append(group.Members, owner.Ref)
			eligible := true
			for _, member := range members {
				if member.Ref == owner.Ref {
					continue
				}
				eligible = eligible && slices.ContainsFunc(member.SeedOwners, func(candidate NativeOwner) bool {
					return candidate.Ref == owner.Ref && candidate.SameLaunch && candidate.Kind == "executable"
				})
			}
			if eligible {
				group.OwnerRefs = append(group.OwnerRefs, owner.Ref)
			}
		}
		if len(group.OwnerRefs) > 0 {
			groups = append(groups, group)
		}
	}
	return groups
}

func resolveNativeLaunchDecisions(rows []NativeCandidate, answers []NativeDecision, choices []NativeLaunchDecision) []Placement {
	groups := nativeLaunchGroups(rows)
	byMember := make(map[string]NativeLaunchGroup)
	byGroup := make(map[string][]NativeLaunchDecision)
	for _, group := range groups {
		for _, ref := range group.Members {
			byMember[ref] = group
		}
		for _, choice := range choices {
			if choice.Ref == group.Ref && !slices.Contains(byGroup[group.Ref], choice) {
				byGroup[group.Ref] = append(byGroup[group.Ref], choice)
			}
		}
	}
	var expanded []NativeDecision
	for _, answer := range answers {
		if _, grouped := byMember[answer.Ref]; !grouped {
			expanded = append(expanded, answer)
		}
	}
	refused := make(map[string]string)
	accepted := make(map[string]NativeLaunchDecision)
	for _, group := range groups {
		decisions := byGroup[group.Ref]
		if len(decisions) != 1 || decisions[0].malformed || (decisions[0].Owner != "separate" && !slices.Contains(group.OwnerRefs, decisions[0].Owner)) {
			refused[group.Ref] = "decision not received: missing, conflicting or invalid launch-group owner"
			continue
		}
		choice := decisions[0]
		accepted[group.Ref] = choice
		if choice.Owner == "separate" {
			for _, answer := range answers {
				if slices.Contains(group.Members, answer.Ref) {
					expanded = append(expanded, answer)
				}
			}
			continue
		}
		for _, ref := range group.Members {
			decision := "standalone"
			if ref != choice.Owner {
				decision = "seed_of:" + choice.Owner
			}
			expanded = append(expanded, NativeDecision{Ref: ref, Decision: decision})
		}
	}
	placements := nativeDecisions(rows, expanded, false)
	for i := range placements {
		group, grouped := byMember[placements[i].Candidate.Ref]
		if !grouped {
			continue
		}
		if reason := refused[group.Ref]; reason != "" {
			placements[i].Reason = reason
			wire, _ := json.Marshal(byGroup[group.Ref])
			placements[i].Rejected = string(wire)
		} else {
			choice := accepted[group.Ref]
			placements[i].LaunchDecision = &choice
		}
	}
	return placements
}

// A launch group's original representatives are one indivisible provider unit.
// Shared representatives join units so no native decision is repeated.
func classificationUnits(compilation Compilation) [][]Candidate {
	parent := make(map[corpus.FileID]corpus.FileID)
	for _, candidate := range compilation.candidates {
		parent[candidate.FileRef] = candidate.FileRef
	}
	var find func(corpus.FileID) corpus.FileID
	find = func(ref corpus.FileID) corpus.FileID {
		if parent[ref] != ref {
			parent[ref] = find(parent[ref])
		}
		return parent[ref]
	}
	files := make(map[string]corpus.FileID)
	for _, row := range compilation.native {
		files[row.Ref] = row.FileRef
	}
	for _, group := range compilation.Request.LaunchGroups {
		first := files[group.Members[0]]
		for _, member := range group.Members[1:] {
			parent[find(files[member])] = find(first)
		}
	}
	var units [][]Candidate
	byRoot := make(map[corpus.FileID]int)
	for _, candidate := range compilation.candidates {
		root := find(candidate.FileRef)
		index, found := byRoot[root]
		if !found {
			index = len(units)
			byRoot[root] = index
			units = append(units, nil)
		}
		units[index] = append(units[index], candidate)
	}
	return units
}

func validateLaunchSubset(compilation Compilation, candidates []Candidate) error {
	selected := make(map[corpus.FileID]bool)
	for _, candidate := range candidates {
		selected[candidate.FileRef] = true
	}
	files := make(map[string]corpus.FileID)
	for _, row := range compilation.native {
		files[row.Ref] = row.FileRef
	}
	for _, group := range compilation.Request.LaunchGroups {
		count := 0
		for _, member := range group.Members {
			if selected[files[member]] {
				count++
			}
		}
		if count != 0 && count != len(group.Members) {
			return fmt.Errorf("target portfolio: launch group %s must retain all original members in one request", group.Ref)
		}
	}
	return nil
}
