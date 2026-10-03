package api

import (
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type fieldErrors map[string]string

func (e fieldErrors) add(field, message string) { e[field] = message }

func (e fieldErrors) empty() bool { return len(e) == 0 }

func required(e fieldErrors, field, value string, max int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		e.add(field, "is required")
	} else if utf8.RuneCountInString(value) > max {
		e.add(field, "must contain at most "+itoa(max)+" characters")
	}
	return value
}

func optional(e fieldErrors, field, value string, max int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > max {
		e.add(field, "must contain at most "+itoa(max)+" characters")
	}
	return value
}

func validEmail(e fieldErrors, value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value || len(value) > 254 {
		e.add("email", "must be a valid email address")
	}
	return value
}

func validPassword(e fieldErrors, value string) {
	if len(value) < 12 || len(value) > 72 {
		e.add("password", "must contain between 12 and 72 bytes")
		return
	}
	var letters, numbers bool
	for _, r := range value {
		letters = letters || unicode.IsLetter(r)
		numbers = numbers || unicode.IsNumber(r)
	}
	if !letters || !numbers {
		e.add("password", "must contain a letter and a number")
	}
}

func validImageURL(e fieldErrors, field, value string) string {
	value = strings.TrimSpace(value)
	u, err := url.ParseRequestURI(value)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		e.add(field, "must be an absolute HTTP or HTTPS URL")
	}
	return value
}

func futureTime(e fieldErrors, field string, value time.Time) {
	if value.IsZero() || !value.After(time.Now().UTC()) {
		e.add(field, "must be in the future")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
