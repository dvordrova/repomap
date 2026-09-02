package llm

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"strings"
	"testing"
)

// structuredFindWithoutGate is the pre-gate behaviour of the structured half
// of assessSensitiveMaterial, kept as the reference the gate must agree with.
func structuredFindWithoutGate(raw []byte) bool {
	normalized, err := NormalizeJSON(raw)
	if err != nil {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(normalized))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return false
	}
	_, found := sensitiveStructuredValue(value)
	return found
}

func TestSensitiveKeyGateNeverHidesAStructuredFind(t *testing.T) {
	cases := []string{
		`{"subjects":[{"ref":"o1","name":"handleOrder"}]}`,
		`{"api_key":"abcdefgh"}`,
		`{"API-KEY":"abcdefgh"}`,
		`{"Authorization":"Bearer xyz"}`,
		`{"nested":{"deep":[{"client_secret":"s"}]}}`,
		`{"credentials":{}}`,
		`{"credential":1}`,
		`{"x_api_key":"v"}`,
		`{"proxy_authorization":"v"}`,
		`{"bearer_token":"v"}`,
		`{"refresh_token":"v"}`,
		`{"password":""}`,
		// A key spelled with escapes has none of the stems in its raw bytes.
		`{"api_key":"v"}`,
		`{"api_key":"v"}`,
		`{"password":"v"}`,
		// Folding cases the gate has to accept.
		`{"API_KEY":"v"}`,
		"{\"api_Key\":\"v\"}",
		`not json at all`,
		`{"truncated":`,
	}
	random := rand.New(rand.NewSource(20260903))
	fragments := []string{"key", "auth", "token", "secret", "password", "credential",
		"name", "ref", "path", "line", `_`, "_", "-", " ", "API", "x"}
	for count := 0; count < 300; count++ {
		var builder strings.Builder
		builder.WriteString(`{"`)
		for parts := 1 + random.Intn(3); parts > 0; parts-- {
			builder.WriteString(fragments[random.Intn(len(fragments))])
		}
		builder.WriteString(`":"value"}`)
		cases = append(cases, builder.String())
	}
	for _, raw := range cases {
		want := structuredFindWithoutGate([]byte(raw))
		got := mayCarrySensitiveKey([]byte(raw)) && structuredFindWithoutGate([]byte(raw))
		if got != want {
			t.Fatalf("gate hid a structured find for %q: gate=%v want=%v",
				raw, mayCarrySensitiveKey([]byte(raw)), want)
		}
		if want && !mayCarrySensitiveKey([]byte(raw)) {
			t.Fatalf("gate rejected a payload with a sensitive key: %q", raw)
		}
	}
}

func BenchmarkAssessSensitiveMaterial(b *testing.B) {
	var builder strings.Builder
	builder.WriteString(`{"subjects":[`)
	for builder.Len() < 120_000 {
		builder.WriteString(`{"ref":"o41","name":"handleOrder","signature":"func(ctx) error","path":"internal/orders/handler.go","line":118},`)
	}
	builder.WriteString(`{"ref":"o0"}]}`)
	payload := []byte(builder.String())
	b.SetBytes(int64(len(payload)))
	for b.Loop() {
		if assessSensitiveMaterial(payload).found {
			b.Fatal("benchmark payload must be clean")
		}
	}
}
