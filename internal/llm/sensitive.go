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
	if _, found := secretscan.DetectPersistenceSensitive(string(raw)); found {
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
