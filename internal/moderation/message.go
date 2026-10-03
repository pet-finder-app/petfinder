package moderation

import (
	"regexp"
	"strings"
	"unicode"
)

type Result struct {
	Flagged bool
	Reasons []string
}

var (
	paymentPattern = regexp.MustCompile(`\b(pix|transferencia|deposito|pagamento|taxa|frete|money|payment)\b`)
	contactPattern = regexp.MustCompile(`\b(whatsapp|telegram|instagram|telefone|phone|chama no|me liga)\b`)
	linkPattern    = regexp.MustCompile(`https?://|www\.`)
)

// Inspect flags common attempts to request payment or move an adoption
// conversation outside the platform. It does not reject the message: moderators
// still need the original context and can hide it after review.
func Inspect(body string) Result {
	normalized := normalize(body)
	result := Result{}
	if paymentPattern.MatchString(normalized) {
		result.Flagged = true
		result.Reasons = append(result.Reasons, "payment_request")
	}
	if contactPattern.MatchString(normalized) || linkPattern.MatchString(strings.ToLower(body)) {
		result.Flagged = true
		result.Reasons = append(result.Reasons, "off_platform_contact")
	}
	return result
}

func normalize(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	for _, r := range value {
		switch r {
		case 'á', 'à', 'â', 'ã':
			r = 'a'
		case 'é', 'ê':
			r = 'e'
		case 'í':
			r = 'i'
		case 'ó', 'ô', 'õ':
			r = 'o'
		case 'ú':
			r = 'u'
		case 'ç':
			r = 'c'
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
