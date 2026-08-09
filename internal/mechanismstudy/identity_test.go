package mechanismstudy

import (
	"testing"

	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

func TestPersistedStudyMechanismIdentityIsPinned(t *testing.T) {
	if ArtifactVersion != 1 {
		t.Fatalf("ArtifactVersion = %d, want 1", ArtifactVersion)
	}
	if CompilationVersion != 2 {
		t.Fatalf("CompilationVersion = %d, want 2", CompilationVersion)
	}
	if RequestVersion != 2 {
		t.Fatalf("RequestVersion = %d, want 2", RequestVersion)
	}
	if ResultVersion != 2 {
		t.Fatalf("ResultVersion = %d, want 2", ResultVersion)
	}
	if PromptVersion != "mechanism-study-prompt-218f4d98678d" {
		t.Fatalf("PromptVersion = %q, want %q", PromptVersion, "mechanism-study-prompt-218f4d98678d")
	}
	if surfacediscovery.DirectCallIndexVersion != 1 {
		t.Fatalf("DirectCallIndexVersion = %d, want 1", surfacediscovery.DirectCallIndexVersion)
	}
}

func TestRequestValidationRejectsStaleV1PromptIdentity(t *testing.T) {
	_, batch := compileChainBatch(t)
	request := batch.Request
	request.PromptVersion = "mechanism-study-prompt-v1"

	if err := request.Validate(); err == nil {
		t.Fatal("Request.Validate accepted stale v1 prompt identity")
	}
}
