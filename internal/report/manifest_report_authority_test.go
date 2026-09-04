package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestDecodeRunManifestRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	manifestJSON, err := json.Marshal(validRunManifestFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	var withUnknown map[string]any
	if err := json.Unmarshal(manifestJSON, &withUnknown); err != nil {
		t.Fatal(err)
	}
	withUnknown["legacy_authority"] = true
	unknownJSON, err := json.Marshal(withUnknown)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeRunManifest(unknownJSON); err == nil ||
		!strings.Contains(err.Error(), `unknown field "legacy_authority"`) {
		t.Fatalf("unknown manifest field error = %v", err)
	}
	if _, err := DecodeRunManifest(append(manifestJSON, []byte(`{}`)...)); err == nil ||
		!strings.Contains(err.Error(), "multiple json values") {
		t.Fatalf("trailing manifest value error = %v", err)
	}
}

func TestDecodeRunManifestRejectsPreviousVersions(t *testing.T) {
	for _, version := range []int{24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37} {
		manifest := validRunManifestFixture(t)
		manifest.Version = version
		encoded, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeRunManifest(encoded); err == nil ||
			!strings.Contains(err.Error(), fmt.Sprintf("unsupported version %d", version)) {
			t.Fatalf("previous manifest version %d error = %v", version, err)
		}
	}
}
