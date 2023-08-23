package auth

import (
	"testing"
)

func TestCheckPasswordPolicy(t *testing.T) {
	tests := []struct {
		password string
		err      error
	}{
		{"", ErrEmptyPassword},
		{"a1", ErrPasswordTooShort},
		{"short", ErrPasswordTooShort},
		{"password", ErrMissingLetterOrNumber},
		{"password1", nil},
		{"123456789", ErrMissingLetterOrNumber},
		{"abcd1234", nil},
	}

	for _, tt := range tests {
		t.Run(tt.password, func(t *testing.T) {
			err := CheckPasswordPolicy(tt.password)
			if err != tt.err {
				t.Errorf("for password: %s, expected error: %v, got: %v", tt.password, tt.err, err)
			}
		})
	}
}
