package themestudy

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ReplayScoutResponse resolves one saved provider response against the exact
// canonical Scout request record that authorized it. It performs no I/O and
// has no provider, cache, or semantic-journal dependency. The request is
// re-encoded and decoded so only canonical in-memory records are accepted.
func ReplayScoutResponse(request ScoutRequest, raw []byte) (ScoutResult, ScoutStatusRecord, error) {
	canonical, err := EncodeScoutRequest(request)
	if err != nil {
		return ScoutResult{}, ScoutStatusRecord{}, fmt.Errorf("theme scout response replay: validate request: %w", err)
	}
	decoded, err := DecodeScoutRequest(canonical)
	if err != nil {
		return ScoutResult{}, ScoutStatusRecord{}, fmt.Errorf("theme scout response replay: canonical request: %w", err)
	}
	reencoded, err := EncodeScoutRequest(decoded)
	if err != nil {
		return ScoutResult{}, ScoutStatusRecord{}, err
	}
	if !bytes.Equal(canonical, reencoded) {
		return ScoutResult{}, ScoutStatusRecord{}, fmt.Errorf("theme scout response replay: request is not canonical")
	}
	if request.WireSHA256 != decoded.WireSHA256 || request.CatalogSHA256 != decoded.CatalogSHA256 {
		return ScoutResult{}, ScoutStatusRecord{}, fmt.Errorf("theme scout response replay: request identity mismatch")
	}
	// Re-validate the embedded wire is canonical JSON of the request shape.
	var wire wireScout
	if err := json.Unmarshal([]byte(decoded.WireJSON), &wire); err != nil {
		return ScoutResult{}, ScoutStatusRecord{}, fmt.Errorf("theme scout response replay: decode exact request wire: %w", err)
	}
	canonicalWire, err := json.Marshal(wire)
	if err != nil {
		return ScoutResult{}, ScoutStatusRecord{}, err
	}
	if !bytes.Equal(canonicalWire, []byte(decoded.WireJSON)) {
		return ScoutResult{}, ScoutStatusRecord{}, fmt.Errorf("theme scout response replay: request wire is not canonical")
	}
	return ResolveScoutResponse(decoded, raw)
}

// ReplayAdjudicationResponse resolves one saved provider response against the
// exact canonical Adjudication request record that authorized it. It performs
// no I/O and has no provider, cache, or semantic-journal dependency.
func ReplayAdjudicationResponse(request AdjudicationRequest, raw []byte) (AdjudicationResult, AdjudicationStatusRecord, error) {
	canonical, err := EncodeAdjudicationRequest(request)
	if err != nil {
		return AdjudicationResult{}, AdjudicationStatusRecord{}, fmt.Errorf("theme adjudication response replay: validate request: %w", err)
	}
	decoded, err := DecodeAdjudicationRequest(canonical)
	if err != nil {
		return AdjudicationResult{}, AdjudicationStatusRecord{}, fmt.Errorf("theme adjudication response replay: canonical request: %w", err)
	}
	reencoded, err := EncodeAdjudicationRequest(decoded)
	if err != nil {
		return AdjudicationResult{}, AdjudicationStatusRecord{}, err
	}
	if !bytes.Equal(canonical, reencoded) {
		return AdjudicationResult{}, AdjudicationStatusRecord{}, fmt.Errorf("theme adjudication response replay: request is not canonical")
	}
	if request.WireSHA256 != decoded.WireSHA256 || request.CatalogSHA256 != decoded.CatalogSHA256 {
		return AdjudicationResult{}, AdjudicationStatusRecord{}, fmt.Errorf("theme adjudication response replay: request identity mismatch")
	}
	var wire wireAdjudication
	if err := json.Unmarshal([]byte(decoded.WireJSON), &wire); err != nil {
		return AdjudicationResult{}, AdjudicationStatusRecord{}, fmt.Errorf("theme adjudication response replay: decode exact request wire: %w", err)
	}
	canonicalWire, err := json.Marshal(wire)
	if err != nil {
		return AdjudicationResult{}, AdjudicationStatusRecord{}, err
	}
	if !bytes.Equal(canonicalWire, []byte(decoded.WireJSON)) {
		return AdjudicationResult{}, AdjudicationStatusRecord{}, fmt.Errorf("theme adjudication response replay: request wire is not canonical")
	}
	return ResolveAdjudicationResponse(decoded, raw)
}
