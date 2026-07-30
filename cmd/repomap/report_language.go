package main

import (
	"fmt"
	"strings"

	"github.com/dvordrova/repomap/internal/deepseek"
)

func normalizeReportLanguage(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "en":
		return "en", nil
	case "ru":
		return "ru", nil
	default:
		return "", fmt.Errorf("--lang must be \"en\" or \"ru\"")
	}
}

func configureClientOutputLanguage(client *deepseek.Client, values []string) {
	if client == nil || len(values) == 0 {
		return
	}
	client.OutputLanguage = values[0]
}

func storedReportLanguage(value string) string {
	if value == "ru" {
		return "ru"
	}
	return ""
}
