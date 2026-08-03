package atlasstudy

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

// CandidateUnavailableError is a provider-free closed outcome. It means the
// configured request budget cannot represent the observed support contract;
// no provider request is valid.
type CandidateUnavailableError struct{ Reason string }

func (err *CandidateUnavailableError) Error() string {
	if err == nil || err.Reason == "" {
		return "atlas study: typed candidate shelf unavailable"
	}
	return "atlas study: typed candidate shelf unavailable: " + err.Reason
}

type selectionLane struct {
	role    SupportRole
	targets []string
}

func selectStudyCandidates(input Input) (Input, CandidateCoverage, error) {
	targets := make(map[string]ReadingTarget, len(input.ReadingTargets))
	for _, target := range input.ReadingTargets {
		if target.ID == "" {
			return Input{}, CandidateCoverage{}, fmt.Errorf("atlas study: empty reading target identity")
		}
		targets[target.ID] = target
	}
	supportByID := make(map[string]ReadingSupport, len(input.ReadingSupports))
	roleBuckets := make(map[SupportRole]map[string][]string)
	for _, support := range input.ReadingSupports {
		if support.ID == "" || support.TargetID == "" || support.PackageBucket == "" ||
			!support.Role.Valid() || !support.Authority.Valid() {
			return Input{}, CandidateCoverage{}, fmt.Errorf("atlas study: invalid route support candidate")
		}
		if !validSupportAuthority(support.Role, support.Authority) {
			return Input{}, CandidateCoverage{}, fmt.Errorf("atlas study: support role has invalid producer authority")
		}
		if _, ok := targets[support.TargetID]; !ok {
			return Input{}, CandidateCoverage{}, fmt.Errorf("atlas study: route support candidate references unknown target")
		}
		if _, duplicate := supportByID[support.ID]; duplicate {
			return Input{}, CandidateCoverage{}, fmt.Errorf("atlas study: duplicate route support candidate")
		}
		supportByID[support.ID] = support
		if roleBuckets[support.Role] == nil {
			roleBuckets[support.Role] = make(map[string][]string)
		}
		roleBuckets[support.Role][support.PackageBucket] = append(
			roleBuckets[support.Role][support.PackageBucket], support.TargetID,
		)
	}
	if len(roleBuckets) == 0 {
		return Input{}, CandidateCoverage{}, &CandidateUnavailableError{Reason: "no observed support roles"}
	}

	lanes := make([]selectionLane, 0, len(roleBuckets))
	roles := make([]SupportRole, 0, len(roleBuckets))
	for role := range roleBuckets {
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i] < roles[j] })
	for _, role := range roles {
		lanes = append(lanes, selectionLane{role: role, targets: roundRobinBuckets(roleBuckets[role])})
	}
	selectedTargets := make(map[string]struct{}, input.Limits.MaxReadingTargets)
	selectedRoles := make(map[SupportRole]struct{}, len(roles))
	selectedRolePackages := make(map[SupportRole]map[string]struct{}, len(roles))
	markTarget := func(id string) {
		selectedTargets[id] = struct{}{}
		for _, support := range input.ReadingSupports {
			if support.TargetID != id {
				continue
			}
			selectedRoles[support.Role] = struct{}{}
			if selectedRolePackages[support.Role] == nil {
				selectedRolePackages[support.Role] = make(map[string]struct{})
			}
			selectedRolePackages[support.Role][support.PackageBucket] = struct{}{}
		}
	}
	seedSpans := minimumSpansByRole(input.RouteSpans, supportByID, roles, input.Limits.MaxRouteSpans)
	if seedSpans == nil {
		return Input{}, CandidateCoverage{}, &CandidateUnavailableError{Reason: "route span budget cannot represent every observed support role"}
	}
	for _, span := range seedSpans {
		for _, supportID := range span.RequiredSupportIDs {
			support, ok := supportByID[supportID]
			if !ok {
				return Input{}, CandidateCoverage{}, fmt.Errorf("atlas study: route span references unknown support candidate")
			}
			selectedRoles[support.Role] = struct{}{}
		}
		for _, targetID := range span.AllowedTargetIDs {
			if _, ok := targets[targetID]; !ok {
				return Input{}, CandidateCoverage{}, fmt.Errorf("atlas study: route span references unknown reading target")
			}
			markTarget(targetID)
		}
	}
	if len(selectedTargets) > input.Limits.MaxReadingTargets {
		return Input{}, CandidateCoverage{}, &CandidateUnavailableError{Reason: "reading target budget cannot represent the typed route spans"}
	}
	addFromLane := func(lane *selectionLane) bool {
		chosen := ""
		for _, id := range lane.targets {
			if _, duplicate := selectedTargets[id]; duplicate {
				continue
			}
			if chosen == "" {
				chosen = id
			}
			for _, support := range input.ReadingSupports {
				if support.TargetID == id && support.Role == lane.role {
					if _, represented := selectedRolePackages[lane.role][support.PackageBucket]; !represented {
						chosen = id
						break
					}
				}
			}
			if chosen == id {
				for _, support := range input.ReadingSupports {
					if support.TargetID == id && support.Role == lane.role {
						if _, represented := selectedRolePackages[lane.role][support.PackageBucket]; !represented {
							markTarget(chosen)
							return true
						}
					}
				}
			}
		}
		if chosen == "" {
			return false
		}
		markTarget(chosen)
		return true
	}
	for index := range lanes {
		if _, alreadyCovered := selectedRoles[lanes[index].role]; alreadyCovered {
			continue
		}
		if len(selectedTargets) == input.Limits.MaxReadingTargets {
			break
		}
		addFromLane(&lanes[index])
	}
	for len(selectedTargets) < input.Limits.MaxReadingTargets {
		progress := false
		for index := range lanes {
			if len(selectedTargets) == input.Limits.MaxReadingTargets {
				break
			}
			progress = addFromLane(&lanes[index]) || progress
		}
		if !progress {
			break
		}
	}
	if len(selectedRoles) != len(roles) {
		return Input{}, CandidateCoverage{}, &CandidateUnavailableError{Reason: "not every observed support role has a selected target"}
	}

	selectedSupports := make(map[string]struct{})
	for id, support := range supportByID {
		if _, ok := selectedTargets[support.TargetID]; ok {
			selectedSupports[id] = struct{}{}
		}
	}
	eligibleSpans := make([]RouteSpan, 0, len(input.RouteSpans))
	for _, span := range input.RouteSpans {
		eligible := true
		for _, supportID := range span.RequiredSupportIDs {
			if _, ok := selectedSupports[supportID]; !ok {
				eligible = false
				break
			}
		}
		for _, targetID := range span.AllowedTargetIDs {
			if _, ok := selectedTargets[targetID]; !ok {
				eligible = false
				break
			}
		}
		if eligible {
			eligibleSpans = append(eligibleSpans, span)
		}
	}
	sort.Slice(eligibleSpans, func(i, j int) bool { return eligibleSpans[i].ID < eligibleSpans[j].ID })
	selectedSpans := selectSpansByRole(eligibleSpans, supportByID, roles, input.Limits.MaxRouteSpans)
	if selectedSpans == nil {
		return Input{}, CandidateCoverage{}, &CandidateUnavailableError{Reason: "route span budget cannot represent every observed support role"}
	}

	coverage := candidateCoverage(input, selectedTargets, selectedSpans)
	input.ReadingTargets = filterTargets(input.ReadingTargets, selectedTargets)
	input.ReadingSupports = filterSupports(input.ReadingSupports, selectedSupports)
	selectedRelationIDs := make(map[string]struct{})
	for _, span := range selectedSpans {
		for _, join := range span.Joins {
			selectedRelationIDs[join.RelationID] = struct{}{}
		}
	}
	input.ProducerRelations = filterRelations(input.ProducerRelations, selectedRelationIDs)
	input.RouteSpans = selectedSpans
	for index := range input.Architecture.Components {
		input.Architecture.Components[index].ReadingTargetIDs = filterStrings(
			input.Architecture.Components[index].ReadingTargetIDs, selectedTargets,
		)
	}
	for index := range input.Surfaces {
		input.Surfaces[index].ReadingTargetIDs = filterStrings(input.Surfaces[index].ReadingTargetIDs, selectedTargets)
	}
	return input, coverage, nil
}

func validSupportAuthority(role SupportRole, authority repositoryatlas.Authority) bool {
	if role == SupportSurfaceCandidate {
		return authority == repositoryatlas.AuthorityPartial
	}
	return role.Valid() && (authority == repositoryatlas.AuthorityObserved || authority == repositoryatlas.AuthorityResolved)
}

func roundRobinBuckets(buckets map[string][]string) []string {
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
		sort.Strings(buckets[key])
		buckets[key] = uniqueStrings(buckets[key])
	}
	sort.Strings(keys)
	var result []string
	for ordinal := 0; ; ordinal++ {
		progress := false
		for _, key := range keys {
			if ordinal < len(buckets[key]) {
				result = append(result, buckets[key][ordinal])
				progress = true
			}
		}
		if !progress {
			return result
		}
	}
}

func selectSpansByRole(spans []RouteSpan, supports map[string]ReadingSupport, roles []SupportRole, limit int) []RouteSpan {
	result := minimumSpansByRole(spans, supports, roles, limit)
	if result == nil {
		return nil
	}
	ordered := cloneRouteSpans(spans)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	selected := make(map[string]struct{}, len(result))
	coveredBuckets := make(map[string]struct{})
	for _, span := range result {
		selected[span.ID] = struct{}{}
		markSpanPackageBuckets(coveredBuckets, span, supports)
	}
	allBucketSet := make(map[string]struct{})
	for _, span := range ordered {
		markSpanPackageBuckets(allBucketSet, span, supports)
	}
	allBuckets := make([]string, 0, len(allBucketSet))
	for bucket := range allBucketSet {
		allBuckets = append(allBuckets, bucket)
	}
	sort.Strings(allBuckets)
	for _, bucket := range allBuckets {
		if len(result) == limit {
			break
		}
		if _, covered := coveredBuckets[bucket]; covered {
			continue
		}
		chosen, bestGain := -1, -1
		for index, span := range ordered {
			if _, exists := selected[span.ID]; exists || !spanHasPackageBucket(span, supports, bucket) {
				continue
			}
			gain := spanUncoveredPackageBucketCount(span, supports, coveredBuckets)
			if gain > bestGain {
				chosen, bestGain = index, gain
			}
		}
		if chosen >= 0 {
			span := ordered[chosen]
			selected[span.ID] = struct{}{}
			result = append(result, span)
			markSpanPackageBuckets(coveredBuckets, span, supports)
		}
	}
	for _, span := range ordered {
		if len(result) == limit {
			break
		}
		if _, exists := selected[span.ID]; !exists {
			selected[span.ID] = struct{}{}
			result = append(result, span)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func spanPackageBuckets(span RouteSpan, supports map[string]ReadingSupport) []string {
	set := make(map[string]struct{})
	for _, supportID := range span.RequiredSupportIDs {
		if support, ok := supports[supportID]; ok {
			set[string(support.Role)+"\x00"+support.PackageBucket] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for bucket := range set {
		result = append(result, bucket)
	}
	sort.Strings(result)
	return result
}

func markSpanPackageBuckets(dst map[string]struct{}, span RouteSpan, supports map[string]ReadingSupport) {
	for _, bucket := range spanPackageBuckets(span, supports) {
		dst[bucket] = struct{}{}
	}
}

func spanHasPackageBucket(span RouteSpan, supports map[string]ReadingSupport, want string) bool {
	for _, bucket := range spanPackageBuckets(span, supports) {
		if bucket == want {
			return true
		}
	}
	return false
}

func spanUncoveredPackageBucketCount(span RouteSpan, supports map[string]ReadingSupport, covered map[string]struct{}) int {
	count := 0
	for _, bucket := range spanPackageBuckets(span, supports) {
		if _, exists := covered[bucket]; !exists {
			count++
		}
	}
	return count
}

func minimumSpansByRole(spans []RouteSpan, supports map[string]ReadingSupport, roles []SupportRole, limit int) []RouteSpan {
	if limit <= 0 {
		return nil
	}
	ordered := cloneRouteSpans(spans)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	selected := make(map[string]struct{})
	covered := make(map[SupportRole]struct{})
	result := make([]RouteSpan, 0, min(limit, len(spans)))
	for _, role := range roles {
		if _, ok := covered[role]; ok {
			continue
		}
		chosen := -1
		for index, span := range ordered {
			if _, exists := selected[span.ID]; exists {
				continue
			}
			if spanHasRole(span, supports, role) && (chosen < 0 ||
				len(span.AllowedTargetIDs) < len(ordered[chosen].AllowedTargetIDs) ||
				(len(span.AllowedTargetIDs) == len(ordered[chosen].AllowedTargetIDs) && span.ID < ordered[chosen].ID)) {
				chosen = index
			}
		}
		if chosen < 0 || len(result) == limit {
			return nil
		}
		span := ordered[chosen]
		selected[span.ID] = struct{}{}
		result = append(result, span)
		for _, supportID := range span.RequiredSupportIDs {
			covered[supports[supportID].Role] = struct{}{}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func spanHasRole(span RouteSpan, supports map[string]ReadingSupport, role SupportRole) bool {
	for _, id := range span.RequiredSupportIDs {
		if supports[id].Role == role {
			return true
		}
	}
	return false
}

func candidateCoverage(input Input, selectedTargets map[string]struct{}, selectedSpans []RouteSpan) CandidateCoverage {
	identity := struct {
		Targets []struct {
			ID      string                 `json:"id"`
			Locator readingLocatorIdentity `json:"locator"`
		} `json:"targets"`
		Supports  []ReadingSupport        `json:"supports"`
		Relations []RouteProducerRelation `json:"relations"`
		Spans     []RouteSpan             `json:"spans"`
	}{}
	for _, target := range input.ReadingTargets {
		identity.Targets = append(identity.Targets, struct {
			ID      string                 `json:"id"`
			Locator readingLocatorIdentity `json:"locator"`
		}{ID: target.ID, Locator: readingLocatorKey(target)})
	}
	identity.Supports = append(identity.Supports, input.ReadingSupports...)
	identity.Relations = append(identity.Relations, input.ProducerRelations...)
	identity.Spans = cloneRouteSpans(input.RouteSpans)
	encoded, _ := json.Marshal(identity)
	coverage := CandidateCoverage{CandidateSHA256: digest(encoded), TargetsConsidered: len(input.ReadingTargets), TargetsSelected: len(selectedTargets), SpansConsidered: len(input.RouteSpans), SpansSelected: len(selectedSpans)}
	coverage.Complete = coverage.TargetsConsidered == coverage.TargetsSelected && coverage.SpansConsidered == coverage.SpansSelected
	roleCounts := make(map[string]*CandidateCoverageCount)
	packageCounts := make(map[string]*CandidateCoverageCount)
	seenRoleTarget := make(map[string]struct{})
	seenPackageTarget := make(map[string]struct{})
	for _, support := range input.ReadingSupports {
		rkey, pkey := string(support.Role)+"\x00"+support.TargetID, support.PackageBucket+"\x00"+support.TargetID
		if _, ok := seenRoleTarget[rkey]; !ok {
			count := roleCounts[string(support.Role)]
			if count == nil {
				count = &CandidateCoverageCount{Key: string(support.Role)}
				roleCounts[string(support.Role)] = count
			}
			count.Considered++
			if _, yes := selectedTargets[support.TargetID]; yes {
				count.Selected++
			}
			seenRoleTarget[rkey] = struct{}{}
		}
		if _, ok := seenPackageTarget[pkey]; !ok {
			count := packageCounts[support.PackageBucket]
			if count == nil {
				count = &CandidateCoverageCount{Key: support.PackageBucket}
				packageCounts[support.PackageBucket] = count
			}
			count.Considered++
			if _, yes := selectedTargets[support.TargetID]; yes {
				count.Selected++
			}
			seenPackageTarget[pkey] = struct{}{}
		}
	}
	for _, count := range roleCounts {
		coverage.PerRole = append(coverage.PerRole, *count)
	}
	for _, count := range packageCounts {
		coverage.PerPackage = append(coverage.PerPackage, *count)
	}
	sort.Slice(coverage.PerRole, func(i, j int) bool { return coverage.PerRole[i].Key < coverage.PerRole[j].Key })
	sort.Slice(coverage.PerPackage, func(i, j int) bool { return coverage.PerPackage[i].Key < coverage.PerPackage[j].Key })
	return coverage
}

func filterTargets(values []ReadingTarget, selected map[string]struct{}) []ReadingTarget {
	var out []ReadingTarget
	for _, value := range values {
		if _, ok := selected[value.ID]; ok {
			out = append(out, value)
		}
	}
	return out
}
func filterSupports(values []ReadingSupport, selected map[string]struct{}) []ReadingSupport {
	var out []ReadingSupport
	for _, value := range values {
		if _, ok := selected[value.ID]; ok {
			out = append(out, value)
		}
	}
	return out
}
func filterRelations(values []RouteProducerRelation, selected map[string]struct{}) []RouteProducerRelation {
	var out []RouteProducerRelation
	for _, value := range values {
		if _, ok := selected[value.ID]; ok {
			out = append(out, value)
		}
	}
	return out
}
func filterStrings(values []string, selected map[string]struct{}) []string {
	var out []string
	for _, value := range values {
		if _, ok := selected[value]; ok {
			out = append(out, value)
		}
	}
	return out
}
func uniqueStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}
func selectedSpanIDs(values []RouteSpan) []string {
	result := make([]string, len(values))
	for i, v := range values {
		result[i] = v.ID
	}
	return result
}
func cloneRouteSpans(values []RouteSpan) []RouteSpan {
	result := append([]RouteSpan(nil), values...)
	for i := range result {
		result[i].RequiredSupportIDs = append([]string(nil), result[i].RequiredSupportIDs...)
		result[i].AllowedTargetIDs = append([]string(nil), result[i].AllowedTargetIDs...)
		result[i].Joins = append([]RouteSpanJoin(nil), result[i].Joins...)
	}
	return result
}
func cloneCandidateCoverage(value CandidateCoverage) CandidateCoverage {
	value.PerRole = append([]CandidateCoverageCount(nil), value.PerRole...)
	value.PerPackage = append([]CandidateCoverageCount(nil), value.PerPackage...)
	return value
}
