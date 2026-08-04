package navigator

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/repositoryatlas"
)

func TestProductArtifactsRoundTripAndBindExactCompiledProduct(t *testing.T) {
	atlas := twoUnitAtlas()
	product, err := CompileProduct(ProductInput{Atlas: atlas, Limits: generousLimits()})
	if err != nil {
		t.Fatal(err)
	}
	request, err := product.RequestRecord()
	if err != nil {
		t.Fatal(err)
	}
	requestJSON, err := EncodeRequestRecord(request)
	if err != nil {
		t.Fatal(err)
	}
	decodedRequest, err := DecodeRequestRecord(requestJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decodedRequest, request) {
		t.Fatalf("decoded request = %#v, want %#v", decodedRequest, request)
	}
	if err := product.ValidateRequestRecord(decodedRequest); err != nil {
		t.Fatal(err)
	}

	compiled, _ := product.CompiledRequest()
	selected, err := product.ResolveRecommendation(mustJSON(t, validProductResponse(t, compiled, 0)))
	if err != nil {
		t.Fatal(err)
	}
	resultJSON, err := EncodeRecommendationRecord(selected)
	if err != nil {
		t.Fatal(err)
	}
	decodedResult, err := DecodeRecommendationRecord(resultJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decodedResult, selected) {
		t.Fatalf("decoded result = %#v, want %#v", decodedResult, selected)
	}
	if err := product.ValidateRecommendationRecord(decodedResult); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRecommendationRecordAgainstAtlas(decodedResult, atlas); err != nil {
		t.Fatal(err)
	}

	statuses := []Status{product.PreparedStatus()}
	selectedStatus, err := product.SelectedStatus(selected)
	if err != nil {
		t.Fatal(err)
	}
	statuses = append(statuses, selectedStatus)
	failedStatus, err := product.FailureStatus(FailureProvider)
	if err != nil {
		t.Fatal(err)
	}
	statuses = append(statuses, failedStatus)
	for _, status := range statuses {
		encoded, err := EncodeStatus(status)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeStatus(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decoded, status) {
			t.Fatalf("decoded status = %#v, want %#v", decoded, status)
		}
		for _, forbidden := range []string{"provider response", "api_key", "authorization", "https://"} {
			if bytes.Contains(encoded, []byte(forbidden)) {
				t.Fatalf("status leaked %q: %s", forbidden, encoded)
			}
		}
	}
}

func TestEmptyProductResultIsCanonicalAndAtlasBound(t *testing.T) {
	atlas, _ := manyProcessAtlas(0)
	product, err := CompileProduct(ProductInput{Atlas: atlas, Limits: generousLimits()})
	if err != nil {
		t.Fatal(err)
	}
	record, err := product.EmptyRecord()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := EncodeRecommendationRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRecommendationRecord(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if err := product.ValidateRecommendationRecord(decoded); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRecommendationRecordAgainstAtlas(decoded, atlas); err != nil {
		t.Fatal(err)
	}
	statusJSON, err := EncodeStatus(product.PreparedStatus())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeStatus(statusJSON); err != nil {
		t.Fatal(err)
	}
}

func TestProductArtifactsRejectTamperingUnknownFieldsAndNoncanonicalBytes(t *testing.T) {
	atlas := twoUnitAtlas()
	product, err := CompileProduct(ProductInput{Atlas: atlas, Limits: generousLimits()})
	if err != nil {
		t.Fatal(err)
	}
	request, err := product.RequestRecord()
	if err != nil {
		t.Fatal(err)
	}
	tamperedRequest := request
	tamperedRequest.WireJSON += " "
	tamperedRequest.WireSHA256 = productDigest([]byte(tamperedRequest.WireJSON))
	if err := product.ValidateRequestRecord(tamperedRequest); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("tampered request error = %v", err)
	}

	compiled, _ := product.CompiledRequest()
	record, err := product.ResolveRecommendation(mustJSON(t, validProductResponse(t, compiled, 0)))
	if err != nil {
		t.Fatal(err)
	}
	tamperedResult := record
	tamperedResult.Actions = cloneRecommendationActions(record.Actions)
	tamperedResult.Actions[0].RelationID += "-other"
	tamperedResult.Selected = &tamperedResult.Actions[0]
	if err := product.ValidateRecommendationRecord(tamperedResult); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("tampered result error = %v", err)
	}
	if err := ValidateRecommendationRecordAgainstAtlas(tamperedResult, atlas); err == nil || !strings.Contains(err.Error(), "startup vertical") {
		t.Fatalf("Atlas-bound tamper error = %v", err)
	}

	encoded, err := EncodeRecommendationRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	unknown := append(append([]byte(nil), encoded[:len(encoded)-2]...), []byte(",\n  \"extra\": true\n}\n")...)
	if _, err := DecodeRecommendationRecord(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
	noncanonical := append(append([]byte(nil), encoded...), ' ')
	if _, err := DecodeRecommendationRecord(noncanonical); err == nil || !strings.Contains(err.Error(), "not canonical") {
		t.Fatalf("noncanonical bytes error = %v", err)
	}

	wrongAtlas := record
	wrongAtlas.AtlasSHA256 = strings.Repeat("0", 64)
	if err := ValidateRecommendationRecordAgainstAtlas(wrongAtlas, atlas); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("Atlas digest error = %v", err)
	}
}

func TestProductArtifactUsesSharedAtlasCeiling(t *testing.T) {
	data := bytes.Repeat([]byte{'x'}, repositoryatlas.MaxArtifactBytes+1)
	if _, err := DecodeRequestRecord(data); err == nil || !strings.Contains(err.Error(), "33554432") {
		t.Fatalf("oversized request error = %v", err)
	}
}
