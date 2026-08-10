package atlasstudy

import (
	"encoding/json"
	"fmt"
	"sort"
)

// MockResponse builds one bounded, provider-free response for an exact request
// record. It selects up to MaxDirections advertised spans in wire order and
// emits for each a valid direction whose principals and readings derive from
// the span's own allowed reading targets, so every returned direction passes
// item-local validation and the replay status is accepted. The response never
// references provider prose, canonical identities, repository paths or raw
// symbols: it is a deterministic fixture for offline report rendering and
// replay tests, and it never invokes a provider or touches the network.
//
// The returned directions carry no question text: direction titles are
// restored from the span catalog by the resolver, so the fixture always
// renders the request's own backend-owned questions.
func MockResponse(request RequestRecord) ([]byte, error) {
	product, err := productFromReplayRequest(request)
	if err != nil {
		return nil, fmt.Errorf("atlas study mock response: %w", err)
	}
	envelope := responseEnvelope{RepositoryType: RepositoryService}

	// Brief support refs use the first advertised reading targets, which are
	// valid brief-support kinds and never leak canonical identity.
	var briefRefs []string
	for _, ref := range product.selectedSpanIDs {
		span, ok := product.byCanonical[CanonicalRef{Kind: RefRouteSpan, ID: ref}]
		if !ok || len(span.AllowedTargetRefs) == 0 {
			continue
		}
		for _, targetRef := range span.AllowedTargetRefs {
			target, ok := product.byCanonical[targetRef]
			if !ok || target.Ref == "" {
				continue
			}
			briefRefs = append(briefRefs, target.Ref)
			if len(briefRefs) >= 3 {
				break
			}
		}
		if len(briefRefs) >= 3 {
			break
		}
	}
	if len(briefRefs) == 0 {
		return nil, fmt.Errorf("atlas study mock response: no advertised reading targets for the brief")
	}
	envelope.Brief = providerBrief{
		WhatItIs:              providerStatement{Text: "The repository exposes an exact application surface reached from the process entry.", SupportRefs: briefRefs},
		Problem:               providerStatement{Text: "The study route traces the exact static calls that connect the entry to repository-local work.", SupportRefs: briefRefs},
		MainInput:             providerStatement{Text: "Configured requests and environment state enter through the process entry.", SupportRefs: briefRefs},
		CentralResponsibility: providerStatement{Text: "Repository-local functions own the exact called work.", SupportRefs: briefRefs},
		ObservableResult:      providerStatement{Text: "The called boundary produces the observed repository behavior.", SupportRefs: briefRefs},
	}

	readingLabels := []ReadingLabel{ReadingStart, ReadingContinue, ReadingConnect, ReadingVerify, ReadingContrast}
	positions := 0
	for _, spanID := range product.selectedSpanIDs {
		if positions >= MaxDirections {
			break
		}
		span, ok := product.byCanonical[CanonicalRef{Kind: RefRouteSpan, ID: spanID}]
		if !ok || span.Ref == "" || len(span.AllowedTargetRefs) == 0 {
			continue
		}
		direction, ok := mockDirectionForSpan(product, span, readingLabels)
		if !ok {
			continue
		}
		rawDirection, err := json.Marshal(direction)
		if err != nil {
			return nil, fmt.Errorf("atlas study mock response: direction: %w", err)
		}
		envelope.Directions = append(envelope.Directions, rawDirection)
		positions++
	}
	if len(envelope.Directions) == 0 {
		return nil, fmt.Errorf("atlas study mock response: no buildable directions")
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("atlas study mock response: encode: %w", err)
	}
	// The resolver decodes strictly; confirm the fixture is canonical.
	var roundtrip responseEnvelope
	if err := decodeStrict(raw, &roundtrip); err != nil {
		return nil, fmt.Errorf("atlas study mock response: canonical: %w", err)
	}
	return raw, nil
}

// mockDirectionForSpan builds one valid provider direction for a span from the
// span's own allowed targets: readings start from the targets that cover the
// span's required supports, principals are the reading targets' advertised
// components, and system-path spans always receive at least two readings.
func mockDirectionForSpan(
	product Product,
	span CatalogObject,
	readingLabels []ReadingLabel,
) (providerDirection, bool) {
	allowed := make([]CatalogObject, 0, len(span.AllowedTargetRefs))
	byCanonical := make(map[CanonicalRef]CatalogObject, len(span.AllowedTargetRefs))
	for _, targetRef := range span.AllowedTargetRefs {
		target, ok := product.byCanonical[targetRef]
		if !ok || target.Ref == "" {
			continue
		}
		allowed = append(allowed, target)
		byCanonical[targetRef] = target
	}
	if len(allowed) == 0 {
		return providerDirection{}, false
	}

	// Required supports must be covered by the chosen readings.
	requiredTargets := make(map[CanonicalRef]struct{}, len(span.RequiredSupportRefs))
	for _, supportRef := range span.RequiredSupportRefs {
		support, ok := product.byCanonical[supportRef]
		if !ok || support.SupportTarget == nil {
			continue
		}
		if _, exists := byCanonical[*support.SupportTarget]; exists {
			requiredTargets[*support.SupportTarget] = struct{}{}
		}
	}

	chosen := make([]CatalogObject, 0, MaxDirectionReadingCount)
	seen := make(map[string]struct{}, MaxDirectionReadingCount)
	for _, target := range allowed {
		if _, needed := requiredTargets[CanonicalRef{Kind: RefReadingTarget, ID: target.CanonicalID}]; !needed {
			continue
		}
		chosen = append(chosen, target)
		seen[target.Ref] = struct{}{}
		if len(chosen) >= MaxDirectionReadingCount {
			break
		}
	}
	for _, target := range allowed {
		if len(chosen) >= MaxDirectionReadingCount {
			break
		}
		if _, duplicate := seen[target.Ref]; duplicate {
			continue
		}
		chosen = append(chosen, target)
		seen[target.Ref] = struct{}{}
	}
	// System-path spans require at least two readings.
	if span.SpanKind == RouteSpanSystemPath && len(chosen) < 2 {
		return providerDirection{}, false
	}

	// Principals are the chosen readings' advertised components. A focused
	// public-API root may instead use its one exact selected package Unit.
	principalSet := make(map[CanonicalRef]struct{}, MaxDirectionReadingCount)
	for _, target := range chosen {
		for _, principal := range target.PrincipalRefs {
			if principal.Kind == RefComponent {
				principalSet[principal] = struct{}{}
			}
		}
	}
	if len(principalSet) == 0 {
		for _, target := range chosen {
			for _, principal := range target.PrincipalRefs {
				if principal.Kind == RefUnit && product.isAnalysisTargetRootPrincipal(principal) {
					principalSet[principal] = struct{}{}
				}
			}
		}
	}
	if len(principalSet) == 0 {
		return providerDirection{}, false
	}
	principals := make([]string, 0, len(principalSet))
	for principal := range principalSet {
		object, ok := product.byCanonical[principal]
		if !ok || object.Ref == "" {
			continue
		}
		principals = append(principals, object.Ref)
	}
	if len(principals) == 0 || len(principals) > 5 {
		return providerDirection{}, false
	}
	sort.Strings(principals)

	reading := make([]providerReading, 0, len(chosen))
	for index, target := range chosen {
		label := readingLabels[index%len(readingLabels)]
		reading = append(reading, providerReading{
			TargetRef:     target.Ref,
			Label:         label,
			WhatToLookFor: "Inspect the exact saved source at this reading target.",
		})
	}
	return providerDirection{
		SpanRef:         span.Ref,
		WhyItMatters:    "This route connects an exact principal to exact reading targets.",
		LearningOutcome: "The reader can identify the exact repository boundary for this route.",
		TargetJob:       span.TargetJob,
		LearningStage:   span.LearningStage,
		PrincipalRefs:   principals,
		Reading:         reading,
	}, true
}
