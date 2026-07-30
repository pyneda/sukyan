package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/viper"
)

const (
	testAccessSecret  = "access-secret"
	testRefreshSecret = "refresh-secret"
)

func configureTokens(t *testing.T, idleHours, absoluteHours int) {
	t.Helper()
	viper.Set("api.auth.jwt_secret_key", testAccessSecret)
	viper.Set("api.auth.jwt_refresh_key", testRefreshSecret)
	viper.Set("api.auth.jwt_secret_expire_minutes", 15)
	viper.Set("api.auth.jwt_refresh_idle_hours", idleHours)
	viper.Set("api.auth.jwt_refresh_expire_hours", absoluteHours)
	t.Cleanup(viper.Reset)
}

func TestParseRefreshTokenRejectsMalformedInput(t *testing.T) {
	configureTokens(t, 24, 168)

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"not a jwt", "nodothere"},
		{"legacy hash format", "hash.1700000000"},
		{"wrong segment count", "a.b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseRefreshToken(tt.token); err == nil {
				t.Fatalf("ParseRefreshToken(%q) succeeded, want error", tt.token)
			}
		})
	}
}

func TestParseRefreshTokenRejectsAccessToken(t *testing.T) {
	configureTokens(t, 24, 168)

	access, err := GenerateAccessToken("user-1")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	if _, err := ParseRefreshToken(access); err == nil {
		t.Fatal("an access token was accepted as a refresh token")
	}
}

// A refresh token signed with the access secret must not verify, otherwise the
// two keys would offer no separation.
func TestParseRefreshTokenRejectsWrongSigningKey(t *testing.T) {
	configureTokens(t, 24, 168)

	claims := jwt.MapClaims{
		"typ":       refreshTokenType,
		"id":        "user-1",
		"exp":       time.Now().Add(time.Hour).Unix(),
		"iat":       time.Now().Unix(),
		"auth_time": time.Now().Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testAccessSecret))
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	if _, err := ParseRefreshToken(token); err == nil {
		t.Fatal("a token signed with the access secret was accepted")
	}
}

func TestParseRefreshTokenRejectsExpiredToken(t *testing.T) {
	configureTokens(t, 24, 168)

	// An auth time far enough in the past that the absolute cap has already
	// elapsed, which is what pins the expiry into the past.
	token, err := GenerateRefreshToken("user-1", time.Now().Add(-200*time.Hour))
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	if _, err := ParseRefreshToken(token); err == nil {
		t.Fatal("an expired refresh token was accepted")
	}
}

func TestParseRefreshTokenRoundTrip(t *testing.T) {
	configureTokens(t, 24, 168)

	authTime := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	token, err := GenerateRefreshToken("user-1", authTime)
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	claims, err := ParseRefreshToken(token)
	if err != nil {
		t.Fatalf("ParseRefreshToken: %v", err)
	}

	if claims.UserID != "user-1" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user-1")
	}
	if !claims.AuthTime.Equal(authTime) {
		t.Errorf("AuthTime = %v, want %v", claims.AuthTime, authTime)
	}
}

func TestGenerateRefreshTokenSlidesWithinIdleWindow(t *testing.T) {
	configureTokens(t, 24, 168)

	authTime := time.Now().Add(-2 * time.Hour)
	token, err := GenerateRefreshToken("user-1", authTime)
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	claims, err := ParseRefreshToken(token)
	if err != nil {
		t.Fatalf("ParseRefreshToken: %v", err)
	}

	want := time.Now().Add(24 * time.Hour)
	if diff := claims.Expires.Sub(want); diff > time.Minute || diff < -time.Minute {
		t.Errorf("Expires = %v, want ~%v", claims.Expires, want)
	}
}

// The absolute window is what stops an attacker renewing a stolen token forever.
func TestGenerateRefreshTokenCapsAtAbsoluteWindow(t *testing.T) {
	configureTokens(t, 24, 168)

	authTime := time.Now().Add(-160 * time.Hour)
	token, err := GenerateRefreshToken("user-1", authTime)
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	claims, err := ParseRefreshToken(token)
	if err != nil {
		t.Fatalf("ParseRefreshToken: %v", err)
	}

	want := authTime.Add(168 * time.Hour)
	if diff := claims.Expires.Sub(want); diff > time.Minute || diff < -time.Minute {
		t.Errorf("Expires = %v, want ~%v (capped)", claims.Expires, want)
	}
}

func TestAccessTokenCarriesRegisteredExpiry(t *testing.T) {
	configureTokens(t, 24, 168)

	access, err := GenerateAccessToken("user-1")
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	token, err := jwt.Parse(access, func(*jwt.Token) (interface{}, error) {
		return []byte(testAccessSecret), nil
	}, jwt.WithExpirationRequired())
	if err != nil {
		t.Fatalf("parsing access token: %v", err)
	}

	claims := token.Claims.(jwt.MapClaims)
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		t.Fatal("access token has no exp claim")
	}
	if claims["expires"] != float64(exp.Unix()) {
		t.Errorf("expires = %v, want it to mirror exp %d", claims["expires"], exp.Unix())
	}
}
