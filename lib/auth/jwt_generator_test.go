package auth

import (
	"testing"
)

func TestParseRefreshTokenRejectsMalformedInput(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		want    int64
		wantErr bool
	}{
		{"no separator", "nodothere", 0, true},
		{"empty", "", 0, true},
		{"empty expiry", "hash.", 0, true},
		{"non numeric expiry", "hash.notanumber", 0, true},
		{"valid", "hash.1700000000", 1700000000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRefreshToken(tt.token)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseRefreshToken(%q) error = %v, wantErr %v", tt.token, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseRefreshToken(%q) = %d, want %d", tt.token, got, tt.want)
			}
		})
	}
}
