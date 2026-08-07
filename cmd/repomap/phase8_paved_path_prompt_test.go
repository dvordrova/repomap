package main

import (
	"strings"
	"testing"
)

// Phase 8 reviewer finding (backend-authority leakage): the Paved Paths
// prompt schema no longer contains a version echo — version is backend-owned.
func TestPavedPathPromptOmitsVersionEcho(t *testing.T) {
	if strings.Contains(pavedPathUserPrompt, `"version"`) {
		t.Fatalf("paved path prompt still asks for a version echo")
	}
}
