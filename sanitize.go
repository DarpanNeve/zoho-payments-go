package zoho

import (
	"regexp"
	"strings"
)

var sanitizePattern = regexp.MustCompile(`[^a-zA-Z0-9\s.,\-()+]+`)

// SanitizeText strips characters Zoho rejects in description/name fields.
func SanitizeText(s string) string {
	return strings.TrimSpace(sanitizePattern.ReplaceAllString(s, ""))
}
