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
// Decision 232 (Archive 9): Navigator v2 — the model selects exactly one
// action_ref; the trail, endpoint entities, evidence and operation are
// backend-owned and restored by the Product from its own catalog.
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

	// Decision 232: v2 drops the model echo of trail/endpoints/evidence/
	// gaps. The single action ref is the entire provider contract.
	if len(envelope.EntityRefs) != 0 || len(envelope.TrailRefs) != 0 ||
		len(envelope.IntersectionRefs) != 0 || len(envelope.EvidenceRefs) != 0 ||
		len(envelope.GapRefs) != 0 {
		return ResolvedResponse{}, fmt.Errorf("navigator response: v2 must not echo backend-owned refs")
	}
	actionEntries, err := compiled.resolveRefs("action_refs", envelope.ActionRefs, catalogAction)
	if err != nil {
		return ResolvedResponse{}, err
	}
	if len(actionEntries) != 1 {
		return ResolvedResponse{}, fmt.Errorf("navigator response: must select exactly one advertised action")
	}

	resolved := ResolvedResponse{}
	for _, entry := range actionEntries {
		resolved.ActionKeys = append(resolved.ActionKeys, entry.CanonicalID)
		action, ok := compiled.actions[entry.CanonicalID]
		if !ok {
			return ResolvedResponse{}, fmt.Errorf("navigator response: resolved action catalog is unavailable")
		}
		resolved.Actions = append(resolved.Actions, action)
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
