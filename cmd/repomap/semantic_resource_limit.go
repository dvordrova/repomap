package main

import (
	"errors"

	"github.com/dvordrova/repomap/internal/deepseek"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

func isSemanticResourceLimit(err error) bool {
	var limitErr *deepseek.ResourceLimitError
	return errors.As(err, &limitErr)
}

// providerFailureContentForExchange is only for the existing redacting
// semantic-exchange recorder. Provider prose must not enter warnings, status
// errors, fallback artifacts, or any other diagnostic path.
func providerFailureContentForExchange(err error, fallback []byte) []byte {
	var limitErr *deepseek.ResourceLimitError
	if errors.As(err, &limitErr) {
		return limitErr.ProviderContent()
	}
	return fallback
}

func providerResultResponseBytes(result modelresearch.ProviderResult) int {
	return max(len(result.Content), result.ResponseBytes)
}
