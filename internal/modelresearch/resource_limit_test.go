package modelresearch

import "testing"

func TestResourceLimitErrorKeepsProviderContentPrivate(t *testing.T) {
	t.Parallel()

	content := []byte("private provider content")
	err := NewResourceLimitError(ResourceLimitError{
		Stage: "test", Kind: ResourceLimitOutputTokens, Limit: 64_000,
	}, content)
	content[0] = 'X'
	first := err.ProviderContent()
	if string(first) != "private provider content" {
		t.Fatalf("ProviderContent() = %q", first)
	}
	first[0] = 'Y'
	if got := string(err.ProviderContent()); got != "private provider content" {
		t.Fatalf("ProviderContent() returned mutable storage: %q", got)
	}
	clone := err.Clone()
	if clone == err || string(clone.ProviderContent()) != "private provider content" {
		t.Fatalf("Clone() = %#v", clone)
	}
}
