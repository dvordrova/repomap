package entrycall

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/secretscan"
)

const (
	SurfaceRequestVersion = 1

	MaxSurfaceCandidates            = 128
	MaxSurfaceFactsPerCandidate     = 8
	MaxSurfaceFacts                 = 512
	MaxSurfaceCandidateSectionBytes = 32 * 1024
	MaxSelectedSurfaceProposals     = MaxSurfaceCandidates
	MaxProviderRequestBytes         = 320 * 1024
	MaxSurfaceSketchRunes           = 192
	MaxSurfaceFactLabelRunes        = 96
	MaxSurfaceFactValueRunes        = 128
	SurfaceKindRefCLICommand        = "k1"
	SurfaceKindRefHTTPRoute         = "k2"
	SurfaceKindRefScheduledJob      = "k3"
	SurfaceSlotRefIdentity          = "s1"
	SurfaceSlotRefMethod            = "s2"
	SurfaceSlotRefPath              = "s3"
	SurfaceSlotRefHandler           = "s4"
	SurfaceKindCLICommand           = "cli_command"
	SurfaceKindHTTPRoute            = "http_route"
	SurfaceKindScheduledJob         = "scheduled_job"
	SurfaceRoleDescriptor           = "descriptor"
	SurfaceRoleEntrySurface         = "entry_surface"
)

// RequestSurfaceCatalog is the only provider-visible projection of the
// private generic syntax candidate reservoir. Choice refs make the response
// refs-only even for kind and semantic-slot classification.
type RequestSurfaceCatalog struct {
	Version           int                       `json:"version"`
	Kinds             []RequestSurfaceChoice    `json:"kinds"`
	Slots             []RequestSurfaceChoice    `json:"slots"`
	Candidates        []RequestSurfaceCandidate `json:"candidates"`
	OmittedCandidates int                       `json:"omitted_candidates"`
	OmittedFacts      int                       `json:"omitted_facts"`
}

type RequestSurfaceChoice struct {
	Ref   string `json:"ref"`
	Label string `json:"label"`
}

type RequestSurfaceCandidate struct {
	Ref     string               `json:"ref"`
	RootRef string               `json:"root_ref"`
	Form    SurfaceCandidateForm `json:"form"`
	Sketch  string               `json:"sketch"`
	Facts   []RequestSurfaceFact `json:"facts"`
}

type RequestSurfaceFact struct {
	Ref      string          `json:"ref"`
	Kind     SurfaceFactKind `json:"kind"`
	Position int             `json:"position"`
	Label    string          `json:"label"`
	Value    string          `json:"value"`
}

// SurfaceCandidateCoverage is backend-owned bounded accounting. It contains
// no candidate identity and is safe to retain in status/result artifacts.
type SurfaceCandidateCoverage struct {
	ConsideredCandidates          int `json:"considered_candidates"`
	AdvertisedCandidates          int `json:"advertised_candidates"`
	OmittedCandidates             int `json:"omitted_candidates"`
	ConsideredFacts               int `json:"considered_facts"`
	AdvertisedFacts               int `json:"advertised_facts"`
	OmittedFacts                  int `json:"omitted_facts"`
	UnsafeFactsExcluded           int `json:"unsafe_facts_excluded"`
	UnreachableCandidatesExcluded int `json:"unreachable_candidates_excluded"`
}

type surfaceCandidateAuthority struct {
	exact     ExactSurfaceCandidate
	request   RequestSurfaceCandidate
	factByRef map[string]ExactSurfaceFact
}

func defaultRequestSurfaceCatalog() RequestSurfaceCatalog {
	return RequestSurfaceCatalog{
		Version: SurfaceRequestVersion,
		Kinds: []RequestSurfaceChoice{
			{Ref: SurfaceKindRefCLICommand, Label: "CLI command"},
			{Ref: SurfaceKindRefHTTPRoute, Label: "HTTP route"},
			{Ref: SurfaceKindRefScheduledJob, Label: "Scheduled job"},
		},
		Slots: []RequestSurfaceChoice{
			{Ref: SurfaceSlotRefIdentity, Label: "command or scheduled-job identity"},
			{Ref: SurfaceSlotRefMethod, Label: "HTTP method"},
			{Ref: SurfaceSlotRefPath, Label: "route path"},
			{Ref: SurfaceSlotRefHandler, Label: "handler callback"},
		},
		Candidates: []RequestSurfaceCandidate{},
	}
}

func compileSurfaceCatalog(
	substrate Substrate,
	rootRefByNodeID map[string]string,
) (RequestSurfaceCatalog, map[string]surfaceCandidateAuthority, SurfaceCandidateCoverage, error) {
	catalog := defaultRequestSurfaceCatalog()
	authority := make(map[string]surfaceCandidateAuthority)
	coverage := SurfaceCandidateCoverage{
		ConsideredCandidates:          substrate.Coverage.SurfaceCandidatesConsidered,
		ConsideredFacts:               substrate.Coverage.SurfaceCandidateFactsConsidered,
		UnsafeFactsExcluded:           substrate.Coverage.UnsafeSurfaceCandidateFactsExcluded,
		UnreachableCandidatesExcluded: substrate.Coverage.UnreachableSurfaceCandidatesExcluded,
	}
	if coverage.ConsideredCandidates < len(substrate.SurfaceCandidates) {
		coverage.ConsideredCandidates = len(substrate.SurfaceCandidates)
	}
	indexedFacts := 0
	for _, candidate := range substrate.SurfaceCandidates {
		indexedFacts += len(candidate.Facts)
	}
	if coverage.ConsideredFacts < indexedFacts {
		coverage.ConsideredFacts = indexedFacts
	}

	candidates := append([]ExactSurfaceCandidate(nil), substrate.SurfaceCandidates...)
	for index := range candidates {
		candidates[index].Facts = append([]ExactSurfaceFact(nil), candidates[index].Facts...)
		sort.Slice(candidates[index].Facts, func(i, j int) bool {
			left, right := candidates[index].Facts[i], candidates[index].Facts[j]
			if left.Position != right.Position {
				return left.Position < right.Position
			}
			if left.Kind != right.Kind {
				return left.Kind < right.Kind
			}
			return left.ID < right.ID
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		leftRank, rightRank := exactSurfaceCandidateRank(left), exactSurfaceCandidateRank(right)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if left.RootNodeID != right.RootNodeID {
			return left.RootNodeID < right.RootNodeID
		}
		if left.Site.Path != right.Site.Path {
			return left.Site.Path < right.Site.Path
		}
		if left.Site.Line != right.Site.Line {
			return left.Site.Line < right.Site.Line
		}
		if left.Site.Column != right.Site.Column {
			return left.Site.Column < right.Site.Column
		}
		if left.Form != right.Form {
			return left.Form < right.Form
		}
		return left.ID < right.ID
	})

	nextFactRef := 1
	for _, exact := range candidates {
		rootRef, rootAdvertised := rootRefByNodeID[exact.RootNodeID]
		if !rootAdvertised || len(catalog.Candidates) >= MaxSurfaceCandidates {
			continue
		}
		remainingFacts := MaxSurfaceFacts - coverage.AdvertisedFacts
		if remainingFacts < 2 {
			continue
		}
		requestCandidate := RequestSurfaceCandidate{
			Ref:     fmt.Sprintf("c%d", len(catalog.Candidates)+1),
			RootRef: rootRef,
			Form:    exact.Form,
			Facts:   []RequestSurfaceFact{},
		}
		factByRef := make(map[string]ExactSurfaceFact)
		selectedFacts := selectSurfaceFacts(exact.Facts, minInt(MaxSurfaceFactsPerCandidate, remainingFacts))
		for _, exactFact := range selectedFacts {
			fact, safe := compileSurfaceFact(exactFact, nextFactRef)
			if !safe {
				coverage.UnsafeFactsExcluded++
				continue
			}
			nextFactRef++
			requestCandidate.Facts = append(requestCandidate.Facts, fact)
			factByRef[fact.Ref] = exactFact
			coverage.AdvertisedFacts++
		}
		if !requestSurfaceCandidateAdmissible(requestCandidate) {
			coverage.AdvertisedFacts -= len(requestCandidate.Facts)
			continue
		}
		requestCandidate.Sketch = buildSurfaceSketch(exact.Sketch, requestCandidate)
		catalog.Candidates = append(catalog.Candidates, requestCandidate)
		authority[requestCandidate.Ref] = surfaceCandidateAuthority{
			exact: exact, request: requestCandidate, factByRef: factByRef,
		}
	}

	coverage.AdvertisedCandidates = len(catalog.Candidates)
	coverage.OmittedCandidates = maxInt(0, coverage.ConsideredCandidates-coverage.AdvertisedCandidates)
	coverage.OmittedFacts = maxInt(0, coverage.ConsideredFacts-coverage.AdvertisedFacts)
	catalog.OmittedCandidates = coverage.OmittedCandidates
	catalog.OmittedFacts = coverage.OmittedFacts

	for surfaceCatalogSize(catalog) > MaxSurfaceCandidateSectionBytes && len(catalog.Candidates) > 0 {
		last := catalog.Candidates[len(catalog.Candidates)-1]
		catalog.Candidates = catalog.Candidates[:len(catalog.Candidates)-1]
		delete(authority, last.Ref)
		coverage.AdvertisedCandidates--
		coverage.AdvertisedFacts -= len(last.Facts)
		coverage.OmittedCandidates++
		coverage.OmittedFacts += len(last.Facts)
		catalog.OmittedCandidates = coverage.OmittedCandidates
		catalog.OmittedFacts = coverage.OmittedFacts
	}
	if surfaceCatalogSize(catalog) > MaxSurfaceCandidateSectionBytes {
		return RequestSurfaceCatalog{}, nil, SurfaceCandidateCoverage{},
			fmt.Errorf("entry call: surface candidate section exceeds bounded envelope")
	}
	return catalog, authority, coverage, nil
}

func selectSurfaceFacts(facts []ExactSurfaceFact, limit int) []ExactSurfaceFact {
	if limit <= 0 || len(facts) == 0 {
		return nil
	}
	selected := make(map[string]ExactSurfaceFact, minInt(limit, len(facts)))
	// A bounded suffix must not silently erase the structural pair needed for
	// classification. Preserve a route-like path first when present, then the
	// existing string+callable pair and a token method hint, and fill the
	// remaining budget in canonical source order.
	for _, fact := range facts {
		if exactSurfacePathLikeFact(fact) {
			selected[fact.ID] = fact
			break
		}
	}
	for _, required := range []SurfaceFactKind{SurfaceFactString, SurfaceFactCallable, SurfaceFactToken} {
		if len(selected) >= limit {
			break
		}
		for _, fact := range facts {
			if fact.Kind == required {
				selected[fact.ID] = fact
				break
			}
		}
	}
	for _, fact := range facts {
		if len(selected) >= limit {
			break
		}
		selected[fact.ID] = fact
	}
	result := make([]ExactSurfaceFact, 0, len(selected))
	for _, fact := range selected {
		result = append(result, fact)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Position != result[j].Position {
			return result[i].Position < result[j].Position
		}
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func compileSurfaceFact(exact ExactSurfaceFact, number int) (RequestSurfaceFact, bool) {
	if !exact.Kind.Valid() || exact.Position < 0 || !validLocation(exact.Location) {
		return RequestSurfaceFact{}, false
	}
	label := sanitizeSurfaceLabel(exact.Label, MaxSurfaceFactLabelRunes)
	value := sanitizeSurfaceValue(exact.Value)
	if label == "" || value == "" || value != exact.Value {
		return RequestSurfaceFact{}, false
	}
	if _, found := secretscan.DetectAlways(label + "\n" + value); found {
		return RequestSurfaceFact{}, false
	}
	return RequestSurfaceFact{
		Ref: fmt.Sprintf("v%d", number), Kind: exact.Kind,
		Position: exact.Position, Label: label, Value: value,
	}, true
}

func requestSurfaceCandidateAdmissible(candidate RequestSurfaceCandidate) bool {
	var stringFact, callable bool
	for _, fact := range candidate.Facts {
		switch fact.Kind {
		case SurfaceFactString:
			stringFact = true
		case SurfaceFactCallable:
			callable = true
		}
	}
	if stringFact && callable {
		return true
	}
	if candidate.Form != SurfaceCandidateDirectCall {
		return false
	}
	for _, pathFact := range candidate.Facts {
		if !requestSurfacePathLikeFact(pathFact) {
			continue
		}
		for _, companion := range candidate.Facts {
			if companion.Ref != pathFact.Ref &&
				(companion.Kind == SurfaceFactString || companion.Kind == SurfaceFactToken) {
				return true
			}
		}
	}
	return false
}

func exactSurfaceCandidateRank(candidate ExactSurfaceCandidate) int {
	var stringFact, callable bool
	for _, fact := range candidate.Facts {
		switch fact.Kind {
		case SurfaceFactString:
			stringFact = true
		case SurfaceFactCallable:
			callable = true
		}
	}
	if stringFact && callable {
		return 0
	}
	if candidate.Form == SurfaceCandidateDirectCall {
		for _, pathFact := range candidate.Facts {
			if !exactSurfacePathLikeFact(pathFact) {
				continue
			}
			for _, companion := range candidate.Facts {
				if companion.ID != pathFact.ID &&
					(companion.Kind == SurfaceFactString || companion.Kind == SurfaceFactToken) {
					return 1
				}
			}
		}
	}
	return 2
}

func exactSurfacePathLikeFact(fact ExactSurfaceFact) bool {
	return fact.Kind == SurfaceFactString && strings.HasPrefix(fact.Value, "/")
}

func requestSurfacePathLikeFact(fact RequestSurfaceFact) bool {
	return fact.Kind == SurfaceFactString && strings.HasPrefix(fact.Value, "/")
}

func buildSurfaceSketch(base string, candidate RequestSurfaceCandidate) string {
	base = sanitizeSurfaceLabel(base, MaxSurfaceSketchRunes)
	if base == "" {
		base = string(candidate.Form)
	}
	parts := make([]string, 0, len(candidate.Facts))
	for _, fact := range candidate.Facts {
		parts = append(parts, fact.Label+"="+fact.Ref)
	}
	sketch := base + "(" + strings.Join(parts, ",") + ")"
	return sanitizeSurfaceLabel(sketch, MaxSurfaceSketchRunes)
}

func sanitizeSurfaceLabel(value string, maxRunes int) string {
	value = strings.Map(func(character rune) rune {
		switch {
		case unicode.IsControl(character), character == '/', character == '\\':
			return ' '
		default:
			return character
		}
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	return truncateRunes(value, maxRunes)
}

func sanitizeSurfaceValue(value string) string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, value)
	value = strings.TrimSpace(value)
	return truncateRunes(value, MaxSurfaceFactValueRunes)
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 || value == "" {
		return ""
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit]))
}

func surfaceCatalogSize(catalog RequestSurfaceCatalog) int {
	encoded, err := json.Marshal(catalog)
	if err != nil {
		return MaxSurfaceCandidateSectionBytes + 1
	}
	return len(encoded)
}

func surfaceProposalID(candidateID, kind string) string {
	digest := sha256.Sum256([]byte(candidateID + "\x00" + kind))
	return "model-surface-" + hex.EncodeToString(digest[:12])
}

func validSurfaceProposalID(value string) bool {
	if !strings.HasPrefix(value, "model-surface-") || len(value) != len("model-surface-")+24 {
		return false
	}
	for _, character := range value[len("model-surface-"):] {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func validSurfaceCoverage(coverage SurfaceCandidateCoverage) bool {
	values := []int{
		coverage.ConsideredCandidates, coverage.AdvertisedCandidates, coverage.OmittedCandidates,
		coverage.ConsideredFacts, coverage.AdvertisedFacts, coverage.OmittedFacts,
		coverage.UnsafeFactsExcluded, coverage.UnreachableCandidatesExcluded,
	}
	for _, value := range values {
		if value < 0 {
			return false
		}
	}
	return coverage.AdvertisedCandidates <= MaxSurfaceCandidates &&
		coverage.AdvertisedCandidates <= coverage.ConsideredCandidates &&
		coverage.AdvertisedCandidates+coverage.OmittedCandidates == coverage.ConsideredCandidates &&
		coverage.AdvertisedFacts <= MaxSurfaceFacts &&
		coverage.AdvertisedFacts <= coverage.ConsideredFacts &&
		coverage.AdvertisedFacts+coverage.OmittedFacts == coverage.ConsideredFacts
}

func validateCompiledSurfaceCatalog(compilation Compilation) error {
	catalog := compilation.Request.SurfaceCatalog
	defaults := defaultRequestSurfaceCatalog()
	if catalog.Version != SurfaceRequestVersion || catalog.Candidates == nil ||
		!surfaceChoicesEqual(catalog.Kinds, defaults.Kinds) ||
		!surfaceChoicesEqual(catalog.Slots, defaults.Slots) ||
		catalog.OmittedCandidates != compilation.surfaceCoverage.OmittedCandidates ||
		catalog.OmittedFacts != compilation.surfaceCoverage.OmittedFacts ||
		len(catalog.Candidates) != compilation.surfaceCoverage.AdvertisedCandidates ||
		len(catalog.Candidates) != len(compilation.surfaceAuthority) ||
		surfaceCatalogSize(catalog) > MaxSurfaceCandidateSectionBytes {
		return fmt.Errorf("entry call: invalid compiled surface catalog")
	}
	rootRefs := make(map[string]struct{}, len(compilation.Request.Entries))
	for _, entry := range compilation.Request.Entries {
		rootRefs[entry.Ref] = struct{}{}
	}
	seenCandidateRefs := make(map[string]struct{}, len(catalog.Candidates))
	seenFactRefs := make(map[string]struct{})
	totalFacts := 0
	for index, candidate := range catalog.Candidates {
		if candidate.Ref != fmt.Sprintf("c%d", index+1) || !candidate.Form.Valid() ||
			candidate.Sketch == "" || sanitizeSurfaceLabel(candidate.Sketch, MaxSurfaceSketchRunes) != candidate.Sketch ||
			len(candidate.Facts) == 0 || len(candidate.Facts) > MaxSurfaceFactsPerCandidate {
			return fmt.Errorf("entry call: invalid compiled surface candidate")
		}
		if _, duplicate := seenCandidateRefs[candidate.Ref]; duplicate {
			return fmt.Errorf("entry call: duplicate compiled surface candidate")
		}
		seenCandidateRefs[candidate.Ref] = struct{}{}
		if _, known := rootRefs[candidate.RootRef]; !known {
			return fmt.Errorf("entry call: compiled surface candidate cites unknown root")
		}
		authority, known := compilation.surfaceAuthority[candidate.Ref]
		if !known || !requestSurfaceCandidatesEqual(candidate, authority.request) ||
			authority.exact.RootNodeID == "" || len(authority.factByRef) != len(candidate.Facts) {
			return fmt.Errorf("entry call: compiled surface authority mismatch")
		}
		for _, fact := range candidate.Facts {
			totalFacts++
			if !validRef(fact.Ref, "v") || !fact.Kind.Valid() || fact.Position < 0 ||
				fact.Label == "" || sanitizeSurfaceLabel(fact.Label, MaxSurfaceFactLabelRunes) != fact.Label ||
				fact.Value == "" || sanitizeSurfaceValue(fact.Value) != fact.Value {
				return fmt.Errorf("entry call: invalid compiled surface fact")
			}
			if _, duplicate := seenFactRefs[fact.Ref]; duplicate {
				return fmt.Errorf("entry call: duplicate compiled surface fact")
			}
			seenFactRefs[fact.Ref] = struct{}{}
			exact, known := authority.factByRef[fact.Ref]
			if !known || exact.Kind != fact.Kind || exact.Position != fact.Position {
				return fmt.Errorf("entry call: compiled surface fact authority mismatch")
			}
			if _, found := secretscan.DetectAlways(fact.Label + "\n" + fact.Value); found {
				return fmt.Errorf("entry call: compiled surface fact contains credential-shaped content")
			}
		}
	}
	if totalFacts != compilation.surfaceCoverage.AdvertisedFacts || totalFacts > MaxSurfaceFacts {
		return fmt.Errorf("entry call: invalid compiled surface fact accounting")
	}
	return nil
}

func surfaceChoicesEqual(left, right []RequestSurfaceChoice) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func requestSurfaceCandidatesEqual(left, right RequestSurfaceCandidate) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}
