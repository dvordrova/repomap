package atlasstudy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// ReplayResponseRecord resolves one saved provider response against the exact
// canonical v6 request record that authorized it. It performs no I/O and has
// no provider, cache, or semantic-journal dependency.
func ReplayResponseRecord(
	request RequestRecord,
	raw []byte,
) (ResultRecord, Status, Diagnostics, error) {
	product, err := productFromReplayRequest(request)
	if err != nil {
		return ResultRecord{}, Status{}, Diagnostics{}, err
	}
	if len(raw) > DefaultLimits().MaxResponseBytes {
		return ResultRecord{}, Status{}, Diagnostics{}, &ResourceLimitError{
			Section: "response_bytes",
			Limit:   DefaultLimits().MaxResponseBytes,
			Actual:  len(raw),
		}
	}
	result, diagnostics, err := product.ResolveResponseJSON(raw)
	if err != nil {
		return ResultRecord{}, Status{}, diagnostics, err
	}
	if err := product.ValidateResultRecord(result); err != nil {
		return ResultRecord{}, Status{}, diagnostics, err
	}
	status, err := product.AcceptedStatus(result)
	if err != nil {
		return ResultRecord{}, Status{}, diagnostics, err
	}
	if err := product.ValidateStatus(status); err != nil {
		return ResultRecord{}, Status{}, diagnostics, err
	}
	return result, status, diagnostics, nil
}

func productFromReplayRequest(request RequestRecord) (Product, error) {
	// Round-trip through the one canonical v6 request encoder/decoder. The CLI
	// additionally pins the original bytes, while this pure seam rejects any
	// structurally invalid in-memory record.
	encoded, err := EncodeRequestRecord(request)
	if err != nil {
		return Product{}, fmt.Errorf("atlas study response replay: validate request: %w", err)
	}
	canonical, err := DecodeRequestRecord(encoded)
	if err != nil {
		return Product{}, fmt.Errorf("atlas study response replay: canonical request: %w", err)
	}
	if !reflect.DeepEqual(canonical, request) {
		return Product{}, fmt.Errorf("atlas study response replay: request is not canonical")
	}

	wireJSON := []byte(request.WireJSON)
	var wire wireProjection
	if err := decodeStrict(wireJSON, &wire); err != nil {
		return Product{}, fmt.Errorf("atlas study response replay: decode exact request wire: %w", err)
	}
	canonicalWire, err := json.Marshal(wire)
	if err != nil {
		return Product{}, fmt.Errorf("atlas study response replay: encode exact request wire: %w", err)
	}
	if !bytes.Equal(canonicalWire, wireJSON) ||
		wire.Version != Version || wire.Language != request.Language {
		return Product{}, fmt.Errorf("atlas study response replay: request wire is not canonical v%d", Version)
	}
	if err := validateReplayRouteProjection(wire, request.Catalog, request.CandidateCoverage); err != nil {
		return Product{}, fmt.Errorf("atlas study response replay: %w", err)
	}

	material := catalogMaterial{
		Version:            Version,
		AtlasSHA256:        request.AtlasSHA256,
		ArchitectureSHA256: request.ArchitectureSHA256,
		Language:           request.Language,
		Limits:             DefaultLimits(),
		ProjectionSHA256:   digest(wireJSON),
		Coverage:           cloneCandidateCoverage(request.CandidateCoverage),
		Objects:            request.Catalog,
	}
	materialJSON, err := json.Marshal(material)
	if err != nil {
		return Product{}, fmt.Errorf("atlas study response replay: encode exact request catalog: %w", err)
	}
	if digest(materialJSON) != request.CatalogSHA256 {
		return Product{}, fmt.Errorf("atlas study response replay: request catalog hash mismatch")
	}

	product := productFromArtifact(
		request.AtlasSHA256,
		request.ArchitectureSHA256,
		request.WireSHA256,
		request.CatalogSHA256,
		request.CatalogRef,
		request.Language,
		request.CandidateCoverage,
		request.Catalog,
	)
	product.wire = append([]byte(nil), wireJSON...)
	if err := product.ValidateRequestRecord(request); err != nil {
		return Product{}, fmt.Errorf("atlas study response replay: reconstruct exact request: %w", err)
	}
	return product, nil
}

func validateReplayRouteProjection(
	wire wireProjection,
	catalog []CatalogObject,
	coverage CandidateCoverage,
) error {
	canonicalRef := make(map[CanonicalRef]string, len(catalog))
	var supports []CatalogObject
	var spans []CatalogObject
	targetCount := 0
	for _, object := range catalog {
		key := CanonicalRef{Kind: object.Kind, ID: object.CanonicalID}
		canonicalRef[key] = object.Ref
		switch object.Kind {
		case RefReadingTarget:
			targetCount++
		case RefRouteSupport:
			supports = append(supports, object)
		case RefRouteSpan:
			spans = append(spans, object)
		}
	}
	if len(wire.Targets) != targetCount || coverage.TargetsSelected != targetCount ||
		len(wire.RouteSupports) != len(supports) || len(wire.RouteSpans) != len(spans) ||
		coverage.SpansSelected != len(spans) {
		return fmt.Errorf("request shelf counts do not match exact catalog")
	}
	for index, support := range supports {
		if support.SupportTarget == nil {
			return fmt.Errorf("route support lacks exact target")
		}
		want := wireRouteSupport{
			Ref: support.Ref, Role: support.SupportRole,
			TargetRef: canonicalRef[*support.SupportTarget], Authority: support.Authority,
		}
		if !reflect.DeepEqual(wire.RouteSupports[index], want) {
			return fmt.Errorf("route support wire does not match exact catalog")
		}
	}
	for index, span := range spans {
		want := wireRouteSpan{
			Ref: span.Ref, Kind: span.SpanKind, Question: span.Question,
			TargetJob: span.TargetJob, LearningStage: span.LearningStage,
		}
		for _, support := range span.RequiredSupportRefs {
			want.RequiredSupportRefs = append(want.RequiredSupportRefs, canonicalRef[support])
		}
		for _, target := range span.AllowedTargetRefs {
			want.AllowedTargetRefs = append(want.AllowedTargetRefs, canonicalRef[target])
		}
		if !reflect.DeepEqual(wire.RouteSpans[index], want) {
			return fmt.Errorf("route span wire does not match exact catalog")
		}
	}

	selectedRoles := make(map[string]map[CanonicalRef]struct{})
	selectedPackages := make(map[string]map[CanonicalRef]struct{})
	for _, support := range supports {
		target := *support.SupportTarget
		role := string(support.SupportRole)
		if selectedRoles[role] == nil {
			selectedRoles[role] = make(map[CanonicalRef]struct{})
		}
		selectedRoles[role][target] = struct{}{}
		if selectedPackages[support.PackageBucket] == nil {
			selectedPackages[support.PackageBucket] = make(map[CanonicalRef]struct{})
		}
		selectedPackages[support.PackageBucket][target] = struct{}{}
	}
	if err := validateSelectedCoverageCounts(coverage.PerRole, selectedRoles); err != nil {
		return err
	}
	return validateSelectedCoverageCounts(coverage.PerPackage, selectedPackages)
}

func validateSelectedCoverageCounts(
	counts []CandidateCoverageCount,
	selected map[string]map[CanonicalRef]struct{},
) error {
	seen := make(map[string]struct{}, len(counts))
	for _, count := range counts {
		if count.Selected != len(selected[count.Key]) {
			return fmt.Errorf("candidate coverage selected count does not match exact catalog")
		}
		seen[count.Key] = struct{}{}
	}
	for key := range selected {
		if _, ok := seen[key]; !ok {
			return fmt.Errorf("candidate coverage omits selected catalog bucket")
		}
	}
	return nil
}
