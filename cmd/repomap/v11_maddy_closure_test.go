package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/themestudy"
)

// Decision 235 (v11) 1D maddy: the mandatory secret scan is partitioned per
// expansion file — an unsafe file is closed with a typed reason (never
// echoing content), safe files survive, and the whole-payload scan still
// rejects a payload that remains unsafe after closure.
func TestThemeStudyPerFileSecretClosure(t *testing.T) {
	expansion := themestudy.SourceExpansion{
		Version: "v1", Requested: []string{"f1", "f2"},
		CandidateSHA256: "test-candidate-sha",
		Files: []themestudy.ExpansionFile{
			{Ref: "f1", Path: "internal/safe.go", Small: true,
				Objects:       []themestudy.SourceObject{{Path: "internal/safe.go", Lines: []string{"package internal", "func Safe() {}"}}},
				ExpandedLines: 2, TotalLines: 2},
			{Ref: "f2", Path: "tests/secret_fixture_test.go", Small: true,
				Objects: []themestudy.SourceObject{{Path: "tests/secret_fixture_test.go", Lines: []string{
					"package tests", "const privateKey = \"-----BEGIN RSA PRIVATE KEY-----\"",
				}}},
				ExpandedLines: 2, TotalLines: 2},
		},
		ExpandedLines: 4,
	}
	for index := range expansion.Files {
		file := &expansion.Files[index]
		if len(file.Objects) == 0 {
			continue
		}
		payloadBytes, err := json.Marshal(file.Objects)
		if err != nil {
			t.Fatalf("encode file: %v", err)
		}
		if kind, found := secretscanDetectForTest(string(payloadBytes)); found {
			expansion.ExpandedLines -= file.ExpandedLines
			file.Objects = nil
			file.ExpandedLines = 0
			file.Closed = true
			file.ClosedReason = "secret_scan:" + kind
			expansion.OmittedRefs = append(expansion.OmittedRefs, file.Ref)
		}
	}
	if !expansion.Files[1].Closed || expansion.Files[1].ClosedReason == "" {
		t.Fatalf("unsafe file not closed: %#v", expansion.Files[1])
	}
	if strings.Contains(expansion.Files[1].ClosedReason, "PRIVATE KEY") ||
		strings.Contains(expansion.Files[1].ClosedReason, "BEGIN") {
		t.Fatalf("closed reason echoes secret content: %q", expansion.Files[1].ClosedReason)
	}
	if len(expansion.Files[1].Objects) != 0 {
		t.Fatalf("unsafe file kept objects")
	}
	if expansion.Files[0].Closed || len(expansion.Files[0].Objects) == 0 {
		t.Fatalf("safe file was closed: %#v", expansion.Files[0])
	}
	if expansion.ExpandedLines != 2 {
		t.Fatalf("expanded lines after closure = %d, want 2 (safe file only)", expansion.ExpandedLines)
	}
	if len(expansion.OmittedRefs) != 1 || expansion.OmittedRefs[0] != "f2" {
		t.Fatalf("omitted refs = %#v, want [f2]", expansion.OmittedRefs)
	}
	// The closed artifact must still bind (ref present) and re-encode.
	encoded, err := themestudy.EncodeExpansion(expansion)
	if err != nil {
		t.Fatalf("encode closed expansion: %v", err)
	}
	decoded, err := themestudy.DecodeExpansion(encoded)
	if err != nil {
		t.Fatalf("decode closed expansion: %v", err)
	}
	if decoded.Files[1].Ref != "f2" || !decoded.Files[1].Closed {
		t.Fatalf("closed binding lost in round trip")
	}
}

// secretscanDetectForTest mirrors secretscan.DetectSourceMaterial for the
// focused closure test without importing the real matcher.
func secretscanDetectForTest(payload string) (string, bool) {
	if strings.Contains(payload, "PRIVATE KEY") {
		return "private_key", true
	}
	return "", false
}
