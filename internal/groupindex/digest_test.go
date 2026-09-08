package groupindex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestDigestPreservesCanonicalBytesWithoutMutatingOwnedIndex(t *testing.T) {
	program := testProgramIndex(t, "first")
	grouped, _, err := Build(program, testProposals(testSubjectIDs(t, program)))
	if err != nil {
		t.Fatal(err)
	}
	empty, err := Empty(program)
	if err != nil {
		t.Fatal(err)
	}
	for name, index := range map[string]Index{"groups and connections": grouped, "empty": empty} {
		t.Run(name, func(t *testing.T) {
			before, err := json.Marshal(index)
			if err != nil {
				t.Fatal(err)
			}
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
			text := &index.Target.Name
			if len(index.Groups) > 0 {
				text = &index.Groups[0].Summary
			}
			original := *text
			*text = original + " changed meaning"
			changed, err := indexDigest(index)
			if err != nil || changed == digest || index.Validate() == nil {
				t.Fatalf("later mutation reused the previous seal: %s / %v", changed, err)
			}
			*text = original
			if err := index.Validate(); err != nil {
				t.Fatalf("restored exact bytes no longer validate: %v", err)
			}
		})
	}
}
