package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/dvordrova/repomap/internal/secretscan"
)

var (
	ErrSensitivePreparedRequest = errors.New("llm: prepared request contains explicit credential material")
	ErrSensitiveResponse        = errors.New("llm: response contains explicit credential material")
)

type sensitiveAssessment struct {
	found      bool
	structured bool
}

func assessSensitiveMaterial(raw []byte) sensitiveAssessment {
	assessment := sensitiveAssessment{}
	if mayCarrySensitiveKey(raw) {
		if normalized, err := NormalizeJSON(raw); err == nil {
			decoder := json.NewDecoder(bytes.NewReader(normalized))
			decoder.UseNumber()
			var value any
			if decoder.Decode(&value) == nil {
				if _, found := sensitiveStructuredValue(value); found {
					assessment.found = true
					assessment.structured = true
				}
			}
		}
	}
	if _, found := secretscan.DetectPersistenceSensitiveBytes(raw); found {
		assessment.found = true
	}
	return assessment
}

func sensitiveStructuredValue(value any) (string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
			switch normalized {
			case "api_key", "apikey", "x_api_key", "authorization", "authorization_header",
				"proxy_authorization",
				"access_token", "refresh_token", "bearer_token", "client_secret",
				"password", "secret", "credential", "credentials":
				return key, true
			}
			if key, found := sensitiveStructuredValue(child); found {
				return key, true
			}
		}
	case []any:
		for _, child := range typed {
			if key, found := sensitiveStructuredValue(child); found {
				return key, true
			}
		}
	}
	return "", false
}

// Every key sensitiveStructuredValue closes on contains one of a few short
// stems, and normalizing a key only lowercases it and turns "-" and " " into
// "_", so a stem written literally in the payload survives that normalization.
// Decoding a payload carrying none of them can therefore never find a key,
// and decoding it into a tree is the single most expensive thing this guard
// does on a cached run.
//
// A JSON escape could spell a stem without those bytes appearing literally,
// so any backslash sends the payload down the full decode. This gate has no
// false negatives; it only skips work that could not have found anything.
var sensitiveKeyStems = []string{"key", "auth", "token", "secret", "password", "credential"}

func mayCarrySensitiveKey(raw []byte) bool {
	if bytes.IndexByte(raw, '\\') >= 0 {
		return true
	}
	for _, stem := range sensitiveKeyStems {
		if secretscan.ContainsFold(raw, stem) {
			return true
		}
	}
	return false
}
