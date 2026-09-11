// Package destinations names the runtime systems a boundary may exchange
// with. The vocabulary lives in code: the boundaries table offers it as a
// closed list the model chooses from, the report groups records by it, and
// an older atlas's free text is folded onto it by the words it contains.
// A package a target imports never becomes a destination by itself; it
// only says which listed system the target evidently talks to.
package destinations

import (
	"fmt"
	"sort"
	"strings"
)

// system pairs a word found in a destination text, a dependency path or a
// package name with the name the reader sees.
type system struct{ word, name string }

// known lists the systems in priority order: a specific service before its
// vendor ("sqs" before "aws"), so the first matching word decides. One run
// named one broker "RabbitMQ broker", "AMQP broker (RabbitMQ)", "RabbitMQ
// broker (queue topology)" and four more ways; the reader wants one row per
// system.
var known = []system{
	{"rabbitmq", "RabbitMQ"}, {"amqp", "RabbitMQ"}, {"amqp091", "RabbitMQ"}, {"amqplib", "RabbitMQ"}, {"pika", "RabbitMQ"},
	{"kafka", "Kafka"}, {"aiokafka", "Kafka"}, {"kafkajs", "Kafka"}, {"sarama", "Kafka"},
	{"nats", "NATS"},
	{"sqs", "Amazon SQS"}, {"sns", "Amazon SNS"}, {"pubsub", "Google Pub/Sub"},
	{"redis", "Redis"}, {"ioredis", "Redis"}, {"memcache", "Memcached"},
	{"postgres", "PostgreSQL"}, {"pgx", "PostgreSQL"}, {"psycopg", "PostgreSQL"}, {"asyncpg", "PostgreSQL"}, {"pg", "PostgreSQL"},
	{"mysql", "MySQL"}, {"pymysql", "MySQL"}, {"mariadb", "MariaDB"},
	{"sqlite", "SQLite"}, {"mongo", "MongoDB"}, {"pymongo", "MongoDB"}, {"mongoose", "MongoDB"},
	{"clickhouse", "ClickHouse"}, {"elasticsearch", "Elasticsearch"}, {"opensearch", "OpenSearch"},
	{"cassandra", "Cassandra"}, {"gocql", "Cassandra"}, {"neo4j", "Neo4j"}, {"influxdb", "InfluxDB"},
	{"dynamodb", "DynamoDB"}, {"bigquery", "BigQuery"}, {"firestore", "Firestore"},
	{"minio", "S3 storage"}, {"s3", "S3 storage"},
	{"etcd", "etcd"}, {"consul", "Consul"}, {"vault", "HashiCorp Vault"},
	{"github", "GitHub"}, {"gitlab", "GitLab"}, {"google", "Google"}, {"aws", "AWS"}, {"boto3", "AWS"}, {"botocore", "AWS"},
	{"slack", "Slack"}, {"telegram", "Telegram"}, {"discord", "Discord"},
	{"stripe", "Stripe"}, {"twilio", "Twilio"}, {"sendgrid", "SendGrid"},
	{"smtp", "SMTP server"}, {"smtplib", "SMTP server"}, {"nodemailer", "SMTP server"},
	{"sentry", "Sentry"}, {"otlp", "OpenTelemetry collector"}, {"opentelemetry", "OpenTelemetry collector"}, {"prometheus", "Prometheus"},
	{"openai", "OpenAI"}, {"anthropic", "Anthropic"},
	{"kubernetes", "Kubernetes API server"}, {"docker", "Docker daemon"},
}

// Known returns every system name once, in priority order.
func Known() []string {
	var names []string
	seen := make(map[string]bool)
	for _, system := range known {
		if !seen[system.name] {
			seen[system.name] = true
			names = append(names, system.name)
		}
	}
	return names
}

// Canonical is the group name for a destination text: the known system it
// names, else the text without its parenthetical qualifier. "remote
// PostgreSQL database" and "PostgreSQL database" are one group; "Cache store
// (concrete implementation unresolved)" stays "Cache store", not Redis,
// because nothing in it names Redis. A name from Known is its own canonical.
func Canonical(text string) string {
	base := strings.TrimSpace(text)
	if i := strings.Index(base, "("); i > 0 {
		base = strings.TrimSpace(base[:i])
	}
	for _, name := range Known() {
		if strings.EqualFold(base, name) {
			return name
		}
	}
	if name, ok := match(words(text)); ok {
		return name
	}
	return base
}

// Implied is the known system a dependency evidently reaches: the package
// path's words after its host and hosting organisation. "github.com/google/
// uuid" implies nothing; "github.com/google/go-github" implies GitHub;
// "github.com/rabbitmq/amqp091-go" implies RabbitMQ.
func Implied(dependency string) (string, bool) {
	segments := strings.Split(strings.TrimSpace(dependency), "/")
	if len(segments) > 1 && strings.Contains(segments[0], ".") {
		segments = segments[1:]
		if len(segments) > 1 && hostingOrganisations[strings.ToLower(segments[0])] {
			segments = segments[1:]
		}
	}
	return match(words(strings.Join(segments, "/")))
}

// hostingOrganisations are path organisations that publish many unrelated
// packages; their name says nothing about a runtime system.
var hostingOrganisations = map[string]bool{"google": true, "github": true, "gitlab": true, "golang": true, "microsoft": true, "go": true}

func match(words []string) (string, bool) {
	for _, system := range known {
		for _, word := range words {
			if word == system.word || len(system.word) >= 5 && strings.HasPrefix(word, system.word) {
				return system.name, true
			}
		}
	}
	return "", false
}

func words(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
}

// Entry is one offered destination: its window-local ref, the system's
// name and the target dependencies that evidently reach it.
type Entry struct {
	Ref          string   `json:"ref"`
	Value        string   `json:"value"`
	Dependencies []string `json:"dependencies,omitempty"`
}

// Catalog is the closed list a window chooses from: every known system in
// priority order, with the supplied dependency paths that imply it. The
// refs do not depend on the dependencies, so a new import changes one
// entry's annotation, not every row's choice.
func Catalog(dependencies []string) []Entry {
	implied := make(map[string][]string)
	for _, dependency := range dependencies {
		if name, ok := Implied(dependency); ok {
			implied[name] = append(implied[name], dependency)
		}
	}
	names := Known()
	entries := make([]Entry, 0, len(names))
	for i, name := range names {
		paths := implied[name]
		sort.Strings(paths)
		entries = append(entries, Entry{Ref: fmt.Sprintf("d%d", i+1), Value: name, Dependencies: dedupe(paths)})
	}
	return entries
}

// Refs lists the refs of a catalogue, the closed options of its column.
func Refs(entries []Entry) []string {
	refs := make([]string, 0, len(entries))
	for _, entry := range entries {
		refs = append(refs, entry.Ref)
	}
	return refs
}

// Value is the system a chosen ref names; empty when the ref is unknown.
func Value(entries []Entry, ref string) string {
	for _, entry := range entries {
		if entry.Ref == ref {
			return entry.Value
		}
	}
	return ""
}

func dedupe(values []string) []string {
	var result []string
	for i, value := range values {
		if i == 0 || value != values[i-1] {
			result = append(result, value)
		}
	}
	return result
}
