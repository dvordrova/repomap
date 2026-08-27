package integration

import (
	"context"
	"testing"

	"example.com/launchservice/internal/clients/clickhouse"
	"example.com/launchservice/internal/observability"
)

func TestClickHouseClientContract(t *testing.T) {
	client, err := clickhouse.NewClient(
		clickhouse.Config{DSN: "clickhouse://local.test/launches"},
		observability.NewMetrics(),
	)
	if err != nil {
		t.Fatal(err)
	}
	launches, err := client.RecentLaunches(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(launches) != 2 || launches[0] != "launch-001" {
		t.Fatalf("launches = %v", launches)
	}
}
