package kubernetes

import (
	"context"
	"testing"

	"example.com/launchservice/internal/observability"
)

func TestClientListsNamespaces(t *testing.T) {
	client, err := NewClient(Config{Server: "https://kubernetes.test"}, observability.NewMetrics())
	if err != nil {
		t.Fatal(err)
	}
	namespaces, err := client.ListNamespaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(namespaces) != 2 || namespaces[1] != "launch" {
		t.Fatalf("namespaces = %v", namespaces)
	}
}
