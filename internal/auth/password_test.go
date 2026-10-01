package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	t.Parallel()

	password := "correct horse battery staple"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if hash == password {
		t.Fatal("HashPassword() returned plaintext")
	}
	if err := VerifyPassword(hash, password); err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
	if err := VerifyPassword(hash, "wrong password"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("VerifyPassword() wrong password error = %v", err)
	}
	if err := VerifyPassword("not-a-bcrypt-hash", password); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("VerifyPassword() malformed hash error = %v", err)
	}
}

func TestHashPasswordRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		want     error
	}{
		{name: "empty", password: "", want: ErrPasswordRequired},
		{name: "too long", password: strings.Repeat("a", MaxPasswordBytes+1), want: ErrPasswordTooLong},
		{name: "invalid UTF-8", password: string([]byte{0xff}), want: ErrInvalidPassword},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := HashPassword(tt.password); !errors.Is(err, tt.want) {
				t.Fatalf("HashPassword() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestHashPasswordUsesByteLimit(t *testing.T) {
	t.Parallel()

	// Eighteen four-byte runes occupy the full 72 bytes accepted by bcrypt.
	password := strings.Repeat("🐾", 18)
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if err := VerifyPassword(hash, password); err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
}
