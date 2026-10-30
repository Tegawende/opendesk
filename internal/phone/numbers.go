package phone

import "strings"

// NormalizeNumber returns a number in E.164 ("+32470123456") when it says its country (a
// leading + or 00), else its national digits; "" when it is not a phone number. Spaces,
// dots, dashes and brackets are dropped, as providers and people write them, and so is the
// national trunk zero written after a country code ("+33 (0)6").
func NormalizeNumber(raw string) string {
	s := strings.ReplaceAll(strings.TrimSpace(raw), "(0)", "")
	if s == "" {
		return ""
	}
	plus := strings.HasPrefix(s, "+")
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '.' || r == '-' || r == '(' || r == ')' || r == '/' || r == ' ' || (r == '+' && b.Len() == 0):
		default:
			return "" // letters, extensions and the like: not a number to match on
		}
	}
	digits := b.String()
	if !plus && strings.HasPrefix(digits, "00") {
		digits, plus = digits[2:], true
	}
	if len(digits) < 3 || len(digits) > 15 {
		return ""
	}
	if plus {
		return "+" + digits
	}
	return digits
}
