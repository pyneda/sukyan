package tokens

import (
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func generateTestJWT(secret string) string {
	claims := jwt.MapClaims{
		"foo": "bar",
		"exp": time.Now().Add(time.Hour * 1).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(secret))
	return tokenString
}

func TestCrackJWTWithEmbeddedWordlist(t *testing.T) {
	secret := "TEST-ACCESS-TOKEN-AUTO"
	token := generateTestJWT(secret)

	result := CrackJWT(token, "", 5, true)

	assert.True(t, result.Found, "The secret should be found in the wordlist.")
	assert.Equal(t, secret, result.Secret, "The found secret should match the expected secret.")
	assert.Greater(t, result.Attempts, 0, "There should be attempts made to find the secret.")
}

func TestCrackJWTWithFilesystemWordlist(t *testing.T) {
	secret := "TEST-ACCESS-TOKEN-AUTO"
	token := generateTestJWT(secret)

	wordlist := []byte("not-the-secret\nanother-wrong-secret\nTEST-ACCESS-TOKEN-AUTO\n")
	tmpFile, err := os.CreateTemp("", "jwt-wordlist-*.txt")
	assert.NoError(t, err, "Temporary wordlist file should be created successfully.")
	defer os.Remove(tmpFile.Name()) // Clean up the temporary file after the test.

	_, err = tmpFile.Write(wordlist)
	assert.NoError(t, err, "Temporary wordlist should be written successfully.")
	tmpFile.Close()

	result := CrackJWT(token, tmpFile.Name(), 5, false)
	assert.True(t, result.Found, "The secret should be found in the wordlist.")
	assert.Equal(t, secret, result.Secret, "The found secret should match the expected secret.")
	assert.Greater(t, result.Attempts, 0, "There should be attempts made to find the secret.")

	wordlistWithoutSecret := []byte("not-the-secret\nanother-wrong-secret\nasfasfdasf\nqwerty\n")
	tmpFile, err = os.CreateTemp("", "jwt-wordlist-no-secret-*.txt")
	assert.NoError(t, err, "Temporary wordlist file should be created successfully.")
	defer os.Remove(tmpFile.Name()) // Clean up the temporary file after the test.

	_, err = tmpFile.Write(wordlistWithoutSecret)
	assert.NoError(t, err, "Temporary wordlist should be written successfully.")
	tmpFile.Close()

	result = CrackJWT(token, tmpFile.Name(), 5, false)
	assert.False(t, result.Found, "The secret should not be found in the modified wordlist.")
	assert.Equal(t, "", result.Secret, "The secret should be empty when not found.")
	assert.Greater(t, result.Attempts, 0, "There should still be attempts made even if the secret is not found.")
}

func TestDecodeAndVerifyJWT_Success(t *testing.T) {
	secret := "test-jwtsecret"
	token := generateTestJWT(secret)

	success, parsedToken := decodeAndVerifyJWT(token, secret)

	assert.True(t, success, "The verification should be successful with the correct secret.")
	assert.NotNil(t, parsedToken, "The parsed token should not be nil on success.")
}

func TestDecodeAndVerifyJWT_Failure_WrongSecret(t *testing.T) {
	correctSecret := "test-jwtsecret"
	wrongSecret := "wrongsecret"
	token := generateTestJWT(correctSecret)

	success, parsedToken := decodeAndVerifyJWT(token, wrongSecret)

	assert.False(t, success, "The verification should fail with the wrong secret.")
	assert.Nil(t, parsedToken, "The parsed token should be nil when verification fails.")
}

func TestDecodeAndVerifyJWT_Failure_InvalidToken(t *testing.T) {
	secret := "test-jwtsecret"
	invalidToken := "this.is.not.a.valid.token"

	success, parsedToken := decodeAndVerifyJWT(invalidToken, secret)

	assert.False(t, success, "The verification should fail with an invalid token.")
	assert.Nil(t, parsedToken, "The parsed token should be nil when the token is invalid.")
}

func TestDecodeAndVerifyJWT_Failure_ExpiredToken(t *testing.T) {
	secret := "test-jwtsecret"
	claims := jwt.MapClaims{
		"foo": "bar",
		"exp": time.Now().Add(-time.Hour).Unix(), // Token expired 1 hour ago
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(secret))

	success, parsedToken := decodeAndVerifyJWT(tokenString, secret)

	assert.False(t, success, "The verification should fail with an expired token.")
	assert.Nil(t, parsedToken, "The parsed token should be nil when the token is expired.")
}
