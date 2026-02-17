package logging

import "strings"

const RedactedValue = "[REDACTED]"

var sensitiveMarkers = []string{
	"token",
	"password",
	"secret",
	"apikey",
	"api_key",
	"session",
	"otp",
}

func RedactString(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return trimmed
	}

	lower := strings.ToLower(trimmed)
	for _, marker := range sensitiveMarkers {
		if strings.Contains(lower, marker) {
			return RedactedValue
		}
	}

	return trimmed
}
