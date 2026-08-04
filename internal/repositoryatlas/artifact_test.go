package repositoryatlas

import (
	"bytes"
	"strings"
	"testing"
)

func TestDecodeCanonicalJSONRequiresExactCanonicalArtifact(t *testing.T) {
	atlas := validAtlasFixture()
	encoded, err := CanonicalJSON(atlas)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCanonicalJSON(encoded)
	if err != nil {
		t.Fatal(err)
	}
	reencoded, err := CanonicalJSON(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded, reencoded) {
		t.Fatalf("round trip changed canonical bytes:\n%s\n%s", encoded, reencoded)
	}

	noncanonical := bytes.Replace(encoded, []byte("\n  \"units\""), []byte("\n    \"units\""), 1)
	if _, err := DecodeCanonicalJSON(noncanonical); err == nil || !strings.Contains(err.Error(), "not canonical") {
		t.Fatalf("non-canonical error = %v", err)
	}
	unknown := bytes.Replace(encoded, []byte("{\n"), []byte("{\n  \"unknown\": true,\n"), 1)
	if _, err := DecodeCanonicalJSON(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown-field error = %v", err)
	}
	if _, err := DecodeCanonicalJSON(append(encoded, []byte("{}")...)); err == nil {
		t.Fatal("multiple JSON values were accepted")
	}
}

func TestDecodeCanonicalJSONRejectsEmptyAndOversizedArtifacts(t *testing.T) {
	if _, err := DecodeCanonicalJSON(nil); err == nil {
		t.Fatal("empty artifact was accepted")
	}
	if _, err := DecodeCanonicalJSON(make([]byte, MaxArtifactBytes+1)); err == nil {
		t.Fatal("oversized artifact was accepted")
	}
}
