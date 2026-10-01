// Package auth provides password hashing, access-token handling, and HTTP
// authorization middleware.
package auth

import (
	"errors"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const MaxPasswordBytes = 72

var (
	ErrPasswordRequired = errors.New("password is required")
	ErrPasswordTooLong  = errors.New("password must be at most 72 bytes")
	ErrInvalidPassword  = errors.New("invalid password")
)

// HashPassword returns a bcrypt hash suitable for persistent storage. Password
// policy, other than bcrypt's hard byte limit, belongs at the request boundary.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", ErrPasswordRequired
	}
	if len(password) > MaxPasswordBytes {
		return "", ErrPasswordTooLong
	}
	if !utf8.ValidString(password) {
		return "", ErrInvalidPassword
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword reports whether password matches a bcrypt hash. It returns the
// same error for a mismatch and a malformed hash so callers do not reveal which
// credential was invalid.
func VerifyPassword(hash, password string) error {
	if hash == "" || password == "" || len(password) > MaxPasswordBytes || !utf8.ValidString(password) {
		return ErrInvalidPassword
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return ErrInvalidPassword
	}
	return nil
}
