package main

import (
	"fmt"
	"strings"
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

func storedReportLanguage(value string) string {
	if value == "ru" {
		return "ru"
	}
	return ""
}
