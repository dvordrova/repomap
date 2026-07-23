package semanticdiscovery

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBundleValidateFactSource(t *testing.T) {
	if err := sourceFactTestBundle().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*FactSource)
	}{
		{
			name: "empty path",
			mutate: func(source *FactSource) {
				source.Path = ""
			},
		},
		{
			name: "absolute path",
			mutate: func(source *FactSource) {
				source.Path = "/workspace/internal/router.go"
			},
		},
		{
			name: "traversing path",
			mutate: func(source *FactSource) {
				source.Path = "../internal/router.go"
			},
		},
		{
			name: "unclean path",
			mutate: func(source *FactSource) {
				source.Path = "internal/../router.go"
			},
		},
		{
			name: "backslash path",
			mutate: func(source *FactSource) {
				source.Path = `internal\router.go`
			},
		},
		{
			name: "zero start line",
			mutate: func(source *FactSource) {
				source.StartLine = 0
			},
		},
		{
			name: "reversed line range",
			mutate: func(source *FactSource) {
				source.EndLine = source.StartLine - 1
			},
		},
		{
			name: "empty enclosing symbol",
			mutate: func(source *FactSource) {
				source.EnclosingSymbol = ""
			},
		},
		{
			name: "malformed enclosing symbol",
			mutate: func(source *FactSource) {
				source.EnclosingSymbol = " (*Router).dispatch"
			},
		},
		{
			name: "empty content digest",
			mutate: func(source *FactSource) {
				source.ContentSHA256 = ""
			},
		},
		{
			name: "short content digest",
			mutate: func(source *FactSource) {
				source.ContentSHA256 = strings.Repeat("a", 63)
			},
		},
		{
			name: "non hexadecimal content digest",
			mutate: func(source *FactSource) {
				source.ContentSHA256 = strings.Repeat("g", 64)
			},
		},
		{
			name: "uppercase content digest",
			mutate: func(source *FactSource) {
				source.ContentSHA256 = strings.Repeat("A", 64)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := sourceFactTestBundle()
			test.mutate(bundle.Facts[0].Source)
			if err := bundle.Validate(); err == nil {
				t.Fatal("Validate() accepted invalid fact source")
			}
		})
	}
}

func TestBundleHashBindsCanonicalFactSource(t *testing.T) {
	bundle := sourceFactTestBundle()
	canonical := canonicalFact(bundle.Facts[0])
	if canonical.Source == bundle.Facts[0].Source {
		t.Fatal("canonicalFact() retained the source pointer")
	}
	bundle.Facts[0].Source.Path = "internal/changed.go"
	if canonical.Source.Path != "internal/router.go" {
		t.Fatalf("canonical source path = %q after input mutation", canonical.Source.Path)
	}

	baseHash, _, err := BundleHash(sourceFactTestBundle())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*FactSource)
	}{
		{
			name: "path",
			mutate: func(source *FactSource) {
				source.Path = "internal/other.go"
			},
		},
		{
			name: "start line",
			mutate: func(source *FactSource) {
				source.StartLine++
			},
		},
		{
			name: "end line",
			mutate: func(source *FactSource) {
				source.EndLine++
			},
		},
		{
			name: "enclosing symbol",
			mutate: func(source *FactSource) {
				source.EnclosingSymbol = "(*Router).other"
			},
		},
		{
			name: "content digest",
			mutate: func(source *FactSource) {
				source.ContentSHA256 = strings.Repeat("b", 64)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := sourceFactTestBundle()
			test.mutate(changed.Facts[0].Source)
			changedHash, _, err := BundleHash(changed)
			if err != nil {
				t.Fatal(err)
			}
			if changedHash == baseHash {
				t.Fatal("BundleHash() did not bind fact source metadata")
			}
		})
	}
}

func TestOpportunityPromptStripsFactSource(t *testing.T) {
	bundle := sourceFactTestBundle()
	prompt, err := BuildOpportunityPrompt(bundle)
	if err != nil {
		t.Fatal(err)
	}

	_, raw, found := strings.Cut(prompt.User, opportunityBundleMarker)
	if !found {
		t.Fatal("opportunity prompt is missing its bundle marker")
	}
	var payload struct {
		Facts []map[string]json.RawMessage `json:"facts"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode opportunity payload: %v", err)
	}
	if len(payload.Facts) != 1 {
		t.Fatalf("prompt facts = %d, want 1", len(payload.Facts))
	}
	if _, exists := payload.Facts[0]["source"]; exists {
		t.Fatal("provider fact contains local source metadata")
	}
	if bundle.Facts[0].Source == nil || bundle.Facts[0].Source.Path != "internal/router.go" {
		t.Fatal("BuildOpportunityPrompt() mutated local fact source metadata")
	}
	for _, secret := range []string{
		bundle.Facts[0].Source.Path,
		bundle.Facts[0].Source.EnclosingSymbol,
		bundle.Facts[0].Source.ContentSHA256,
	} {
		if strings.Contains(raw, secret) {
			t.Fatalf("provider payload leaked fact source value %q", secret)
		}
	}
}

func sourceFactTestBundle() Bundle {
	return Bundle{
		Version:  BundleVersion,
		RepoName: "sample",
		Facts: []Fact{{
			ID:           "fact-source-window",
			Kind:         FactSourceSignal,
			Statement:    "A bounded local window identifies a direct dispatch call",
			SourceGroup:  "group-source-window",
			Capabilities: []Capability{CapabilityStatic, CapabilityDirectCall},
			Scope:        FactScopeLocal,
			Source: &FactSource{
				Path:            "internal/router.go",
				StartLine:       20,
				EndLine:         24,
				EnclosingSymbol: "(*Router).dispatch",
				ContentSHA256:   strings.Repeat("a", 64),
			},
		}},
	}
}
