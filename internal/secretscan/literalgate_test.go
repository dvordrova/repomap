package secretscan

import (
	"math/rand"
	"strings"
	"testing"
)

// detectPersistenceSensitiveUngated is the pre-gate behaviour, kept here as
// the reference the gate must agree with on every input.
func detectPersistenceSensitiveUngated(text string) (string, bool) {
	for _, candidate := range persistencePatterns {
		for _, location := range candidate.pattern.FindAllStringIndex(text, -1) {
			match := text[location[0]:location[1]]
			if candidate.bearer && !looksLikeBearerCredential(text, location, match) {
				continue
			}
			if candidate.assignment && (looksLikePlaceholder(match) || !looksLikeCredentialAssignment(match)) {
				continue
			}
			return candidate.kind, true
		}
	}
	return "", false
}

func TestLiteralGateAgreesWithTheUngatedScan(t *testing.T) {
	cases := []string{
		"",
		"nothing interesting here",
		"-----BEGIN RSA PRIVATE KEY-----",
		"-----begin openssh private key-----",
		"Authorization: Bearer abcdefghijklmnop",
		"authorization:\ttoken-value-here",
		`{"Authorization": "Basic Zm9vOmJhcg=="}`,
		"BEARER ABCDEFGHIJKL",
		"sk-abcdefghijklmnop",
		"ghp_ABCDEFGHIJKLMNOPQRSTU",
		"github_pat_ABCDEFGHIJKLMNOPQRSTU",
		"AKIAIOSFODNN7EXAMPLE",
		`api_key = "abcdefghijkl"`,
		`API-KEY: abcdefghijklmnopq`,
		`"apiKey":"abcdefghijklmnopq"`,
		"api_key = \"${PLACEHOLDER}\"",
		// The two runes that fold to an ASCII letter. A pattern folding
		// case-insensitively can match these, so the gate must pass them.
		"api_Key: abcdefghijklmnopq",
		"ſk-abcdefghijklmnop",
		"routes/authorization:authorize",
		strings.Repeat("ordinary program identity text ", 200),
	}
	random := rand.New(rand.NewSource(20260903))
	alphabet := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 _-:=\"'/.\nKſ")
	for count := 0; count < 400; count++ {
		var builder strings.Builder
		for length := random.Intn(80); length > 0; length-- {
			builder.WriteRune(alphabet[random.Intn(len(alphabet))])
		}
		cases = append(cases, builder.String())
	}
	for _, text := range cases {
		wantKind, wantFound := detectPersistenceSensitiveUngated(text)
		gotKind, gotFound := DetectPersistenceSensitive(text)
		if gotKind != wantKind || gotFound != wantFound {
			t.Fatalf("DetectPersistenceSensitive(%q) = %q/%v, ungated = %q/%v",
				text, gotKind, gotFound, wantKind, wantFound)
		}
	}
}

func benchmarkPayload() string {
	var builder strings.Builder
	for builder.Len() < 120_000 {
		builder.WriteString(`{"ref":"o41","name":"handleOrder","kind":"function","signature":"func(ctx context.Context) error","path":"internal/orders/handler.go","line":118},`)
	}
	return builder.String()
}

func BenchmarkDetectPersistenceSensitive(b *testing.B) {
	payload := benchmarkPayload()
	b.SetBytes(int64(len(payload)))
	for b.Loop() {
		if _, found := DetectPersistenceSensitive(payload); found {
			b.Fatal("benchmark payload must be clean")
		}
	}
}

func BenchmarkDetectPersistenceSensitiveUngated(b *testing.B) {
	payload := benchmarkPayload()
	b.SetBytes(int64(len(payload)))
	for b.Loop() {
		if _, found := detectPersistenceSensitiveUngated(payload); found {
			b.Fatal("benchmark payload must be clean")
		}
	}
}
