package zoho

import "strings"

// NormalizePhone strips the India country code prefix from a WhatsApp phone number.
// WhatsApp delivers "919XXXXXXXXX" (12 chars); Zoho checkout expects bare 10-digit + phone_country_code field.
func NormalizePhone(phone, countryCode string) string {
	switch countryCode {
	case "IN":
		if len(phone) == 12 && strings.HasPrefix(phone, "91") {
			return phone[2:]
		}
	}
	return phone
}
