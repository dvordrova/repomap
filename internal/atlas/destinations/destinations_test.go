package destinations

import (
	"slices"
	"testing"
)

func TestCanonicalFoldsFreeTextOntoOneSystemAndKeepsUnknownText(t *testing.T) {
	for text, want := range map[string]string{
		"RabbitMQ broker (morfeu.events exchange)": "RabbitMQ", "AMQP broker (RabbitMQ)": "RabbitMQ", "message broker": "message broker",
		"remote PostgreSQL database": "PostgreSQL", "Postgres": "PostgreSQL", "Redis cache server": "Redis",
		"Cache store (concrete implementation unresolved)": "Cache store", "S3-compatible object storage (MinIO)": "S3 storage",
		"OTLP trace collector": "OpenTelemetry collector", "AWS S3 bucket": "S3 storage", "proxy service": "proxy service", "": "",
	} {
		if got := Canonical(text); got != want {
			t.Fatalf("Canonical(%q) = %q, want %q", text, got, want)
		}
	}
	for _, name := range Known() {
		if got := Canonical(name); got != name {
			t.Fatalf("a listed system is not its own canonical: %q -> %q", name, got)
		}
	}
}

func TestImpliedReadsDependencyPathsPastHostAndOrganisation(t *testing.T) {
	for dependency, want := range map[string]string{
		"github.com/rabbitmq/amqp091-go": "RabbitMQ", "github.com/jackc/pgx/v5/pgxpool": "PostgreSQL", "github.com/redis/go-redis/v9": "Redis",
		"github.com/aws/aws-sdk-go-v2/service/sqs": "Amazon SQS", "github.com/aws/aws-sdk-go": "AWS", "cloud.google.com/go/pubsub": "Google Pub/Sub",
		"github.com/google/go-github/v50/github": "GitHub", "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp": "OpenTelemetry collector",
		"psycopg2": "PostgreSQL", "kafka-python": "Kafka", "@aws-sdk/client-s3": "S3 storage", "amqplib": "RabbitMQ",
	} {
		got, ok := Implied(dependency)
		if !ok || got != want {
			t.Fatalf("Implied(%q) = %q/%t, want %q", dependency, got, ok, want)
		}
	}
	for _, dependency := range []string{"github.com/google/uuid", "go.uber.org/zap", "github.com/spf13/cobra", "requests", "axios", "google.golang.org/grpc", ""} {
		if got, ok := Implied(dependency); ok {
			t.Fatalf("a library without a system implied %q: %q", got, dependency)
		}
	}
}

func TestCatalogKeepsFixedRefsAndAnnotatesImpliedSystems(t *testing.T) {
	bare := Catalog(nil)
	annotated := Catalog([]string{"github.com/rabbitmq/amqp091-go", "github.com/spf13/cobra", "github.com/jackc/pgx/v5", "github.com/lib/pq/../pgx", "github.com/jackc/pgx/v5"})
	if len(bare) != len(Known()) || len(annotated) != len(bare) {
		t.Fatalf("catalogue size changed with dependencies: %d / %d", len(bare), len(annotated))
	}
	for i := range bare {
		if bare[i].Ref != annotated[i].Ref || bare[i].Value != annotated[i].Value || len(bare[i].Dependencies) != 0 {
			t.Fatalf("dependencies moved a ref: %+v / %+v", bare[i], annotated[i])
		}
	}
	if Value(annotated, "d1") != "RabbitMQ" || !slices.Equal(annotated[0].Dependencies, []string{"github.com/rabbitmq/amqp091-go"}) {
		t.Fatalf("implied system lost its dependency: %+v", annotated[0])
	}
	var postgres Entry
	for _, entry := range annotated {
		if entry.Value == "PostgreSQL" {
			postgres = entry
		}
	}
	if !slices.Equal(postgres.Dependencies, []string{"github.com/jackc/pgx/v5", "github.com/lib/pq/../pgx"}) {
		t.Fatalf("dependencies are not sorted and deduplicated: %+v", postgres)
	}
	if refs := Refs(annotated); refs[0] != "d1" || refs[len(refs)-1] != "d"+itoa(len(refs)) || Value(annotated, "d999") != "" {
		t.Fatalf("refs or unknown lookup wrong: %v", refs)
	}
}

func itoa(n int) string {
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
