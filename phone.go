package zoho

import "strings"

// NormalizePhone strips the country code prefix from a WhatsApp-style number.
// WhatsApp delivers "919XXXXXXXXX" (12 chars); Zoho expects bare 10-digit + PhoneCountryCode.
func NormalizePhone(phone, countryCode string) string {
	switch countryCode {
	case "IN":
		if len(phone) == 12 && strings.HasPrefix(phone, "91") {
			return phone[2:]
		}
	}
	return phone
}
