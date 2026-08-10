package atlasstudy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

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

// SelectAnalysisTargetRootFrontier returns the exact D277/D280 provider seed
// frontier selected by the same bounded policy as Compile. Pre-D277 inputs
// are returned unchanged so executable/surface seed behavior is byte-stable.
// The complete considered set remains in Compile's CandidateCoverage.
func SelectAnalysisTargetRootFrontier(input Input) (Input, error) {
	if input.AnalysisTargetRoot == nil {
		return input, nil
	}
	canonical, _, _, err := canonicalInput(input)
	if err != nil {
		return Input{}, err
	}
	selected, _, err := selectStudyCandidates(canonical)
	if err != nil {
		return Input{}, err
	}
	return selected, nil
}

// OrderAnalysisTargetRootReadingTargets exposes the D277/D280 provider order for an
// already selected input. It rejects any incomplete, duplicate or non-root
// member so Theme seed and anchor refs cannot silently diverge from the exact
// constructor/function/receiver-family frontier after canonical ID sorting.
func OrderAnalysisTargetRootReadingTargets(input Input) ([]ReadingTarget, error) {
	if input.AnalysisTargetRoot == nil {
		return nil, fmt.Errorf("atlas study: selected AnalysisTarget root scope is required")
	}
	targets := make(map[string]ReadingTarget, len(input.ReadingTargets))
	for _, target := range input.ReadingTargets {
		if _, duplicate := targets[target.ID]; duplicate {
			return nil, fmt.Errorf("atlas study: duplicate selected public API reading target")
		}
		targets[target.ID] = target
	}
	supports := make(map[string]ReadingSupport, len(input.ReadingSupports))
	for _, support := range input.ReadingSupports {
		if _, duplicate := supports[support.ID]; duplicate {
			return nil, fmt.Errorf("atlas study: duplicate selected public API support")
		}
		supports[support.ID] = support
	}
	ordered := orderedStudySpans(input.RouteSpans, supports, targets)
	result := make([]ReadingTarget, 0, len(ordered))
	seen := make(map[string]struct{}, len(ordered))
	for _, span := range ordered {
		target, ok := publicAPIRootSpanTarget(span, supports, targets)
		if !ok {
			return nil, fmt.Errorf("atlas study: selected AnalysisTarget frontier contains a non-root span")
		}
		if _, duplicate := seen[target.ID]; duplicate {
			return nil, fmt.Errorf("atlas study: selected public API reading target appears more than once")
		}
		seen[target.ID] = struct{}{}
		result = append(result, target)
	}
	if len(result) != len(input.ReadingTargets) {
		return nil, fmt.Errorf("atlas study: selected public API reading frontier is incomplete")
	}
	return result, nil
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

	roles := make([]SupportRole, 0, len(roleBuckets))
	for role := range roleBuckets {
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i] < roles[j] })

	// The complete considered span set is the locally supported D210 span set
	// exactly as produced: every focused span compiled from an exact support
	// and every system-path span compiled from one exact producer join. It is
	// never trimmed to the advertised budget; it is bounded only by the
	// existing hard producer/resource limits.
	consideredSpans := cloneRouteSpans(input.RouteSpans)
	for _, span := range consideredSpans {
		for _, supportID := range span.RequiredSupportIDs {
			if _, ok := supportByID[supportID]; !ok {
				return Input{}, CandidateCoverage{}, fmt.Errorf("atlas study: route span references unknown support candidate")
			}
		}
		for _, targetID := range span.AllowedTargetIDs {
			if _, ok := targets[targetID]; !ok {
				return Input{}, CandidateCoverage{}, fmt.Errorf("atlas study: route span references unknown reading target")
			}
		}
	}
	sort.Slice(consideredSpans, func(i, j int) bool { return consideredSpans[i].ID < consideredSpans[j].ID })

	// The advertised frontier is deterministic breadth over the complete
	// considered set: every observed support role is represented by at least
	// one advertised span or the request is provider-free unavailable; within
	// each role exact target-package buckets round-robin so one repeated
	// package family cannot monopolize the frontier; system-path spans stay
	// eligible only through their exact producer join (they already carry
	// Joins); the frontier is capped at MaxAdvertisedSpans. Selection order is
	// a request-budget mechanism, never semantic importance.
	selectedSpans := selectSpansByRoleWithBucketOrder(
		consideredSpans, supportByID, targets, roles,
		analysisTargetRootPackageBucketOrder(input.AnalysisTargetRoot),
		input.Limits.MaxAdvertisedSpans,
	)
	if selectedSpans == nil {
		return Input{}, CandidateCoverage{}, &CandidateUnavailableError{Reason: "advertised span budget cannot represent every observed support role"}
	}

	selectedTargets := make(map[string]struct{}, len(selectedSpans))
	selectedSupports := make(map[string]struct{}, len(selectedSpans))
	selectedRelationIDs := make(map[string]struct{})
	for _, span := range selectedSpans {
		for _, targetID := range span.AllowedTargetIDs {
			selectedTargets[targetID] = struct{}{}
		}
		for _, supportID := range span.RequiredSupportIDs {
			selectedSupports[supportID] = struct{}{}
		}
		for _, join := range span.Joins {
			selectedRelationIDs[join.RelationID] = struct{}{}
		}
	}

	if len(selectedTargets) > input.Limits.MaxReadingTargets {
		return Input{}, CandidateCoverage{}, &CandidateUnavailableError{Reason: "reading target budget cannot represent the typed route spans"}
	}

	coverage := candidateCoverage(input, selectedTargets, selectedSpans)
	input.ReadingTargets = filterTargets(input.ReadingTargets, selectedTargets)
	input.ReadingSupports = filterSupports(input.ReadingSupports, selectedSupports)
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

// selectSpansByRole builds the advertised request frontier from the complete
// considered span set. It is deterministic breadth: every observed support
// role is represented by one seed span or nil is returned (provider-free
// unavailable), then exact target-package buckets round-robin so one repeated
// package family cannot monopolize the frontier, then remaining spans fill by
// ID order up to the limit. Selection order is a request-budget mechanism,
// never semantic importance.
func selectSpansByRole(
	spans []RouteSpan,
	supports map[string]ReadingSupport,
	targets map[string]ReadingTarget,
	roles []SupportRole,
	limit int,
) []RouteSpan {
	return selectSpansByRoleWithBucketOrder(spans, supports, targets, roles, nil, limit)
}

func selectSpansByRoleWithBucketOrder(
	spans []RouteSpan,
	supports map[string]ReadingSupport,
	targets map[string]ReadingTarget,
	roles []SupportRole,
	preferredBuckets []string,
	limit int,
) []RouteSpan {
	ordered := orderedStudySpans(spans, supports, targets)
	result := minimumSpansByRole(ordered, supports, roles, preferredBuckets, limit)
	if result == nil {
		return nil
	}
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
	for _, bucket := range preferredBuckets {
		if _, exists := allBucketSet[bucket]; !exists {
			continue
		}
		allBuckets = append(allBuckets, bucket)
		delete(allBucketSet, bucket)
	}
	remainingBuckets := make([]string, 0, len(allBucketSet))
	for bucket := range allBucketSet {
		remainingBuckets = append(remainingBuckets, bucket)
	}
	sort.Strings(remainingBuckets)
	allBuckets = append(allBuckets, remainingBuckets...)
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

func minimumSpansByRole(
	spans []RouteSpan,
	supports map[string]ReadingSupport,
	roles []SupportRole,
	preferredBuckets []string,
	limit int,
) []RouteSpan {
	if limit <= 0 {
		return nil
	}
	bucketRank := make(map[string]int, len(preferredBuckets))
	for index, bucket := range preferredBuckets {
		bucketRank[bucket] = index
	}
	ordered := cloneRouteSpans(spans)
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
			if !spanHasRole(span, supports, role) {
				continue
			}
			if chosen < 0 || spanPreferredBucketRank(span, supports, bucketRank) <
				spanPreferredBucketRank(ordered[chosen], supports, bucketRank) ||
				spanPreferredBucketRank(span, supports, bucketRank) ==
					spanPreferredBucketRank(ordered[chosen], supports, bucketRank) &&
					len(span.AllowedTargetIDs) < len(ordered[chosen].AllowedTargetIDs) {
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

func spanPreferredBucketRank(
	span RouteSpan,
	supports map[string]ReadingSupport,
	ranks map[string]int,
) int {
	best := len(ranks)
	for _, bucket := range spanPackageBuckets(span, supports) {
		if rank, ok := ranks[bucket]; ok && rank < best {
			best = rank
		}
	}
	return best
}

func analysisTargetRootPackageBucketOrder(scope *AnalysisTargetRootScope) []string {
	packages := analysisTargetRootPackages(scope)
	result := make([]string, 0, len(packages))
	for _, pkg := range packages {
		result = append(result, string(SupportAnalysisTargetRoot)+"\x00"+pkg.UnitID)
	}
	return result
}

// orderedStudySpans preserves the existing ID order for every pre-D277 span.
// Focused public-API roots use a Go-only request-budget order: constructors,
// other top-level functions, then receiver-family round-robin methods. Source
// position, not opaque/hash identity, owns ordering inside each group.
func orderedStudySpans(
	spans []RouteSpan,
	supports map[string]ReadingSupport,
	targets map[string]ReadingTarget,
) []RouteSpan {
	result := cloneRouteSpans(spans)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })

	var constructors, functions []RouteSpan
	methodFamilies := make(map[string][]RouteSpan)
	rootIDs := make(map[string]struct{})
	for _, span := range result {
		target, ok := publicAPIRootSpanTarget(span, supports, targets)
		if !ok {
			continue
		}
		rootIDs[span.ID] = struct{}{}
		switch target.Kind {
		case ReadingTargetFunction:
			if goConstructorName(target.Label) {
				constructors = append(constructors, span)
			} else {
				functions = append(functions, span)
			}
		case ReadingTargetMethod:
			receiver, _, ok := strings.Cut(target.Label, ".")
			if !ok || receiver == "" {
				receiver = target.Label
			}
			methodFamilies[receiver] = append(methodFamilies[receiver], span)
		}
	}
	spanSourceLess := func(left, right RouteSpan) bool {
		leftTarget, _ := publicAPIRootSpanTarget(left, supports, targets)
		rightTarget, _ := publicAPIRootSpanTarget(right, supports, targets)
		return readingTargetSourceLess(leftTarget, rightTarget)
	}
	sort.SliceStable(constructors, func(i, j int) bool { return spanSourceLess(constructors[i], constructors[j]) })
	sort.SliceStable(functions, func(i, j int) bool { return spanSourceLess(functions[i], functions[j]) })
	type methodFamily struct {
		name  string
		spans []RouteSpan
	}
	families := make([]methodFamily, 0, len(methodFamilies))
	for name, values := range methodFamilies {
		sort.SliceStable(values, func(i, j int) bool { return spanSourceLess(values[i], values[j]) })
		families = append(families, methodFamily{name: name, spans: values})
	}
	sort.Slice(families, func(i, j int) bool {
		left, _ := publicAPIRootSpanTarget(families[i].spans[0], supports, targets)
		right, _ := publicAPIRootSpanTarget(families[j].spans[0], supports, targets)
		if readingTargetSourceLess(left, right) {
			return true
		}
		if readingTargetSourceLess(right, left) {
			return false
		}
		return families[i].name < families[j].name
	})
	orderedRoots := append(constructors, functions...)
	for remaining := true; remaining; {
		remaining = false
		for index := range families {
			if len(families[index].spans) == 0 {
				continue
			}
			remaining = true
			orderedRoots = append(orderedRoots, families[index].spans[0])
			families[index].spans = families[index].spans[1:]
		}
	}

	return replaceOrderedRootSlots(result, orderedRoots, rootIDs)
}

// replaceOrderedRootSlots leaves every non-root span in place while replacing
// root slots in their request-budget order. The classifier currently makes the
// number of slots and ordered roots identical. Keep that invariant fail-closed:
// if a future root kind is added without an ordering bucket, retain the stable
// ID order instead of publishing a partial reorder or panicking.
func replaceOrderedRootSlots(
	spans []RouteSpan,
	orderedRoots []RouteSpan,
	rootIDs map[string]struct{},
) []RouteSpan {
	result := make([]RouteSpan, 0, len(spans))
	remainingRoots := orderedRoots
	for _, span := range spans {
		if _, root := rootIDs[span.ID]; !root {
			result = append(result, span)
			continue
		}
		if len(remainingRoots) == 0 {
			return spans
		}
		result = append(result, remainingRoots[0])
		remainingRoots = remainingRoots[1:]
	}
	if len(remainingRoots) != 0 {
		return spans
	}
	return result
}

func goConstructorName(name string) bool {
	if name == "New" {
		return true
	}
	if !strings.HasPrefix(name, "New") || len(name) == len("New") {
		return false
	}
	r, _ := utf8.DecodeRuneInString(name[len("New"):])
	return unicode.IsUpper(r) || unicode.IsDigit(r)
}

func publicAPIRootSpanTarget(
	span RouteSpan,
	supports map[string]ReadingSupport,
	targets map[string]ReadingTarget,
) (ReadingTarget, bool) {
	if span.Kind != RouteSpanFocused || len(span.RequiredSupportIDs) != 1 ||
		len(span.AllowedTargetIDs) != 1 {
		return ReadingTarget{}, false
	}
	support, ok := supports[span.RequiredSupportIDs[0]]
	if !ok || support.Role != SupportAnalysisTargetRoot || support.TargetID != span.AllowedTargetIDs[0] {
		return ReadingTarget{}, false
	}
	target, ok := targets[support.TargetID]
	return target, ok && (target.Kind == ReadingTargetFunction || target.Kind == ReadingTargetMethod)
}

func readingTargetSourceLess(left, right ReadingTarget) bool {
	if left.Location.Path != right.Location.Path {
		return left.Location.Path < right.Location.Path
	}
	if left.Location.Line != right.Location.Line {
		return left.Location.Line < right.Location.Line
	}
	if left.Location.Column != right.Location.Column {
		return left.Location.Column < right.Location.Column
	}
	if left.Label != right.Label {
		return left.Label < right.Label
	}
	return left.Symbol < right.Symbol
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
	coverage.Omissions = candidateOmissions(input.RouteSpans, selectedSpans)
	return coverage
}

// candidateOmissions records bounded aggregate omission diagnostics keyed by
// closed reason. Every considered span omitted from the advertised frontier is
// counted once with a bounded list of representative typed refs; the complete
// candidate digest already binds the full set, so an unbounded list is never
// persisted.
func candidateOmissions(considered []RouteSpan, advertised []RouteSpan) []CoverageOmission {
	advertisedSet := make(map[string]struct{}, len(advertised))
	for _, span := range advertised {
		advertisedSet[span.ID] = struct{}{}
	}
	var omitted []RouteSpan
	for _, span := range considered {
		if _, ok := advertisedSet[span.ID]; !ok {
			omitted = append(omitted, span)
		}
	}
	if len(omitted) == 0 {
		return nil
	}
	sort.Slice(omitted, func(i, j int) bool { return omitted[i].ID < omitted[j].ID })
	omission := CoverageOmission{Reason: OmissionAdvertisedBudget, Count: len(omitted)}
	for _, span := range omitted {
		if len(omission.Representatives) >= MaxOmissionRepresentatives {
			break
		}
		omission.Representatives = append(
			omission.Representatives,
			CanonicalRef{Kind: RefRouteSpan, ID: span.ID},
		)
	}
	return []CoverageOmission{omission}
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
	value.Omissions = append([]CoverageOmission(nil), value.Omissions...)
	for index := range value.Omissions {
		value.Omissions[index].Representatives = append(
			[]CanonicalRef(nil), value.Omissions[index].Representatives...,
		)
	}
	return value
}
