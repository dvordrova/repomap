package report

import "testing"

// Decision 236 (owner corrective): the touchpoint family classification is
// a CLOSED generic taxonomy computed deterministically in the backend —
// never a per-repository keyword table and never guessed from component
// properties in JavaScript. The same rules apply to every repository.
func TestTouchpointFamilyClosedClassification(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"database/sql", "database"},
		{"github.com/jackc/pgx/v5", "database"},
		{"gorm.io/gorm", "database"},
		{"net/http", "HTTP-gRPC-SDK"},
		{"github.com/gin-gonic/gin", "HTTP-gRPC-SDK"},
		{"google.golang.org/grpc", "HTTP-gRPC-SDK"},
		{"github.com/segmentio/kafka-go", "broker/pub-sub"},
		{"github.com/rabbitmq/amqp091-go", "broker/pub-sub"},
		{"github.com/nats-io/nats.go", "broker/pub-sub"},
		{"os", "filesystem"},
		{"os/exec", "process-OS"},
		{"io", "filesystem"},
		{"path/filepath", "filesystem"},
		{"github.com/aws/aws-sdk-go/service/s3", "object-storage"},
		{"github.com/minio/minio-go/v7", "object-storage"},
		{"github.com/redis/go-redis/v9", "cache-lock"},
		{"github.com/bradfitz/gomemcache/memcache", "cache-lock"},
		{"github.com/spf13/viper", "config-secrets"},
		{"github.com/joho/godotenv", "config-secrets"},
		{"syscall", "process-OS"},
		{"github.com/custom/thing", "other"},
		{"", ""},
	}
	for _, tc := range cases {
		got := touchpointFamilyFromImportPath(tc.path)
		if got != tc.want {
			t.Errorf("touchpointFamilyFromImportPath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
