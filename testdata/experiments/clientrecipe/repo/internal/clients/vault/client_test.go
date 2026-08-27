package vault

import (
	"context"
	"testing"
	"time"

	"example.com/launchservice/internal/observability"
)

func TestClientReadsSecret(t *testing.T) {
	client, err := NewClient(Config{
		Address: "http://vault.test", Token: "test-placeholder", Timeout: time.Second,
	}, observability.NewMetrics())
	if err != nil {
		t.Fatal(err)
	}
	value, err := client.ReadSecret(context.Background(), "launch/key")
	if err != nil {
		t.Fatal(err)
	}
	if value != "value:launch/key" {
		t.Fatalf("value = %q", value)
	}
}

// fakeVault is intentionally test-only and must not become a production client instance.
type fakeVault struct{}

func (fakeVault) ReadSecret(context.Context, string) (string, error) { return "fake", nil }
