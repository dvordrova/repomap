package report

func normalizedReportLanguage(value string) string {
	if value == "ru" {
		return "ru"
	}
	return "en"
}

func storedReportLanguage(value string) string {
	if normalizedReportLanguage(value) == "ru" {
		return "ru"
	}
	return ""
}
