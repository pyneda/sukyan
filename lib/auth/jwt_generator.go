package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/viper"
)

// refreshTokenType is the value of the refresh token's "typ" claim. Refresh and
// access tokens are otherwise shaped alike, so without this a refresh token
// would authenticate against any protected route.
const refreshTokenType = "refresh"

// Tokens struct to describe tokens object.
type Tokens struct {
	Access  string
	Refresh string
}

// RefreshClaims describes a verified refresh token.
type RefreshClaims struct {
	UserID   string
	AuthTime time.Time
	Expires  time.Time
}

// GenerateNewTokens issues the token pair for a fresh sign-in, starting a new
// absolute session window.
func GenerateNewTokens(id string) (*Tokens, error) {
	return generateTokens(id, time.Now())
}

// RenewTokens issues a token pair that continues an existing session, so the
// absolute expiry stays anchored to the original sign-in.
func RenewTokens(id string, authTime time.Time) (*Tokens, error) {
	return generateTokens(id, authTime)
}

func generateTokens(id string, authTime time.Time) (*Tokens, error) {
	accessToken, err := GenerateAccessToken(id)
	if err != nil {
		return nil, err
	}

	refreshToken, err := GenerateRefreshToken(id, authTime)
	if err != nil {
		return nil, err
	}

	return &Tokens{
		Access:  accessToken,
		Refresh: refreshToken,
	}, nil
}

// GenerateAccessToken issues a short-lived access token for the given user.
func GenerateAccessToken(id string) (string, error) {
	secret := viper.GetString("api.auth.jwt_secret_key")
	minutesCount := viper.GetInt("api.auth.jwt_secret_expire_minutes")

	now := time.Now()
	expires := now.Add(time.Minute * time.Duration(minutesCount))

	// "expires" mirrors "exp" for clients that read the non-registered claim.
	claims := jwt.MapClaims{
		"id":      id,
		"exp":     expires.Unix(),
		"iat":     now.Unix(),
		"expires": expires.Unix(),
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// GenerateRefreshToken issues a refresh token for the given user. Its expiry
// slides forward on every renewal but never past the absolute window measured
// from authTime, so an idle session ends and a stolen token cannot be extended
// indefinitely.
func GenerateRefreshToken(id string, authTime time.Time) (string, error) {
	secret := viper.GetString("api.auth.jwt_refresh_key")

	now := time.Now()
	expires := now.Add(refreshIdleWindow())

	if deadline := authTime.Add(refreshAbsoluteWindow()); expires.After(deadline) {
		expires = deadline
	}

	claims := jwt.MapClaims{
		"typ":       refreshTokenType,
		"id":        id,
		"exp":       expires.Unix(),
		"iat":       now.Unix(),
		"auth_time": authTime.Unix(),
	}

	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// ParseRefreshToken verifies a refresh token and returns its claims.
func ParseRefreshToken(refreshToken string) (*RefreshClaims, error) {
	secret := viper.GetString("api.auth.jwt_refresh_key")

	token, err := jwt.Parse(refreshToken, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	}, jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("malformed refresh token")
	}

	if typ, _ := claims["typ"].(string); typ != refreshTokenType {
		return nil, fmt.Errorf("not a refresh token")
	}

	id, _ := claims["id"].(string)
	if id == "" {
		return nil, fmt.Errorf("refresh token has no subject")
	}

	expires, err := claims.GetExpirationTime()
	if err != nil || expires == nil {
		return nil, fmt.Errorf("refresh token has no expiry")
	}

	authTime, ok := claims["auth_time"].(float64)
	if !ok {
		return nil, fmt.Errorf("refresh token has no auth time")
	}

	return &RefreshClaims{
		UserID:   id,
		AuthTime: time.Unix(int64(authTime), 0),
		Expires:  expires.Time,
	}, nil
}

func refreshIdleWindow() time.Duration {
	return time.Duration(viper.GetInt("api.auth.jwt_refresh_idle_hours")) * time.Hour
}

func refreshAbsoluteWindow() time.Duration {
	return time.Duration(viper.GetInt("api.auth.jwt_refresh_expire_hours")) * time.Hour
}

// RefreshCookieMaxAge is how long the refresh cookie should live: the longest a
// session can last before a new sign-in is required.
func RefreshCookieMaxAge() time.Duration {
	return refreshAbsoluteWindow()
}
