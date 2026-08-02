package navigator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

// ValidateResponseJSON resolves only exact refs from this Compiled request.
// It performs no prefix matching, substitution, or cross-request repair.
func (compiled Compiled) ValidateResponseJSON(data []byte) (ResolvedResponse, error) {
	if compiled.catalogRef == "" || len(compiled.catalog.entries) == 0 {
		return ResolvedResponse{}, fmt.Errorf("navigator response: compiled catalog is unavailable")
	}
	if len(data) == 0 {
		return ResolvedResponse{}, fmt.Errorf("navigator response: empty payload")
	}
	if len(data) > compiled.maxResponseBytes {
		return ResolvedResponse{}, &ResourceLimitError{
			Section: "response_bytes", Limit: compiled.maxResponseBytes, Actual: len(data),
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var envelope responseEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return ResolvedResponse{}, fmt.Errorf("navigator response: decode: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return ResolvedResponse{}, fmt.Errorf("navigator response: multiple JSON values")
		}
		return ResolvedResponse{}, fmt.Errorf("navigator response: trailing data: %w", err)
	}
	if envelope.Version != Version {
		return ResolvedResponse{}, fmt.Errorf("navigator response: unsupported version %d", envelope.Version)
	}
	if envelope.CatalogRef != compiled.catalogRef {
		return ResolvedResponse{}, &ReferenceError{Field: "catalog_ref", Position: 0, Code: "catalog_ref_mismatch"}
	}

	entityEntries, err := compiled.resolveRefs("entity_refs", envelope.EntityRefs, catalogEntity)
	if err != nil {
		return ResolvedResponse{}, err
	}
	trailEntries, err := compiled.resolveRefs("trail_refs", envelope.TrailRefs, catalogTrail)
	if err != nil {
		return ResolvedResponse{}, err
	}
	intersectionEntries, err := compiled.resolveRefs("intersection_refs", envelope.IntersectionRefs, catalogIntersection)
	if err != nil {
		return ResolvedResponse{}, err
	}
	evidenceEntries, err := compiled.resolveRefs("evidence_refs", envelope.EvidenceRefs, catalogEvidence)
	if err != nil {
		return ResolvedResponse{}, err
	}
	gapEntries, err := compiled.resolveRefs("gap_refs", envelope.GapRefs, catalogGap)
	if err != nil {
		return ResolvedResponse{}, err
	}
	actionEntries, err := compiled.resolveRefs("action_refs", envelope.ActionRefs, catalogAction)
	if err != nil {
		return ResolvedResponse{}, err
	}

	resolved := ResolvedResponse{}
	for _, entry := range entityEntries {
		resolved.Entities = append(resolved.Entities, repositoryEntityRef(entry))
	}
	for _, entry := range trailEntries {
		resolved.RelationIDs = append(resolved.RelationIDs, entry.CanonicalID)
	}
	for _, entry := range intersectionEntries {
		resolved.IntersectionEntityIDs = append(resolved.IntersectionEntityIDs, entry.CanonicalID)
	}
	for _, entry := range evidenceEntries {
		resolved.EvidenceIDs = append(resolved.EvidenceIDs, entry.CanonicalID)
	}
	for _, entry := range gapEntries {
		resolved.GapKeys = append(resolved.GapKeys, entry.CanonicalID)
	}
	for _, entry := range actionEntries {
		resolved.ActionKeys = append(resolved.ActionKeys, entry.CanonicalID)
	}
	return resolved, nil
}

func (compiled Compiled) resolveRefs(
	field string,
	refs []string,
	want catalogKind,
) ([]catalogEntry, error) {
	result := make([]catalogEntry, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for position, ref := range refs {
		if _, duplicate := seen[ref]; duplicate {
			return nil, &ReferenceError{Field: field, Position: position, Code: "duplicate_ref"}
		}
		seen[ref] = struct{}{}
		entry, ok := compiled.catalog.entries[ref]
		if !ok {
			if len(compiled.catalog.byCanonical[ref]) > 0 {
				return nil, &ReferenceError{Field: field, Position: position, Code: "raw_canonical_ref"}
			}
			if _, outside := compiled.catalog.outsideCanonical[ref]; outside {
				return nil, &ReferenceError{Field: field, Position: position, Code: "cross_scope_ref"}
			}
			return nil, &ReferenceError{Field: field, Position: position, Code: "unknown_ref"}
		}
		if entry.Kind != want {
			return nil, &ReferenceError{Field: field, Position: position, Code: "wrong_kind_ref"}
		}
		result = append(result, entry)
	}
	return result, nil
}

func repositoryEntityRef(entry catalogEntry) repositoryatlas.EntityRef {
	return repositoryatlas.EntityRef{Kind: entry.EntityKind, ID: entry.CanonicalID}
}
