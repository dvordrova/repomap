package programindex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestDigestPreservesCanonicalBytesWithoutMutatingOwnedIndex(t *testing.T) {
	base := categorizationTestIndex(t)
	enriched, err := Enrich(base, categorizationTestDocumentationSHA256, []CategoryAssignment{{
		SubjectID:  objectWithSourceRef(t, base, "target-a").ID,
		Categories: []Category{CategoryCore},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for name, index := range map[string]Index{"base with patterns": base, "enriched": enriched} {
		t.Run(name, func(t *testing.T) {
			before, err := json.Marshal(index)
			if err != nil {
				t.Fatal(err)
			}
			// Preserve the previous canonical digest material as the oracle,
			// including nested collections and their empty/null distinctions.
			previous := index.Snapshot()
			previous.SHA256 = ""
			previousBytes, err := json.Marshal(previous)
			if err != nil {
				t.Fatal(err)
			}
			previousHash := sha256.Sum256(previousBytes)
			digest, err := indexDigest(index)
			if err != nil || digest != hex.EncodeToString(previousHash[:]) || digest != index.SHA256 {
				t.Fatalf("digest format changed: %s / %s / %v", digest, index.SHA256, err)
			}
			if err := index.Validate(); err != nil {
				t.Fatal(err)
			}
			after, err := json.Marshal(index)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("digest/validation mutated index or nested storage: %v", err)
			}
			// Public nested storage remains mutable: an earlier successful
			// validation must never authorize a later change under its old seal.
			original := index.Relations[0].Witnesses[0].Detail
			index.Relations[0].Witnesses[0].Detail = original + "changed evidence"
			changed, err := indexDigest(index)
			if err != nil || changed == digest || index.Validate() == nil {
				t.Fatalf("later nested mutation reused the previous seal: %s / %v", changed, err)
			}
			index.Relations[0].Witnesses[0].Detail = original
			if err := index.Validate(); err != nil {
				t.Fatalf("restored exact bytes no longer validate: %v", err)
			}
		})
	}
}
