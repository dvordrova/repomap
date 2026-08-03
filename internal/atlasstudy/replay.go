package atlasstudy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// ReplayResponseRecord resolves one saved provider response against the exact
// canonical v5 request record that authorized it. It performs no I/O and has
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
	// Round-trip through the one canonical v5 request encoder/decoder. The CLI
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

	material := catalogMaterial{
		Version:            Version,
		AtlasSHA256:        request.AtlasSHA256,
		ArchitectureSHA256: request.ArchitectureSHA256,
		Language:           request.Language,
		Limits:             DefaultLimits(),
		ProjectionSHA256:   digest(wireJSON),
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
		request.Catalog,
	)
	product.wire = append([]byte(nil), wireJSON...)
	if err := product.ValidateRequestRecord(request); err != nil {
		return Product{}, fmt.Errorf("atlas study response replay: reconstruct exact request: %w", err)
	}
	return product, nil
}
