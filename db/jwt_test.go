package db

import (
	"testing"

	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"gorm.io/datatypes"
	"time"
)

func TestFillJwtFromToken(t *testing.T) {
	// Set up a range of test cases
	testCases := []struct {
		name          string
		token         string
		expectedToken *JsonWebToken
		expectedError error
	}{
		{
			"Valid JWT",
			"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
			&JsonWebToken{
				Signature: "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
				Token:     "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
				Header:    datatypes.JSON(`{ "alg": "HS256", "typ": "JWT" }`),
				Payload:   datatypes.JSON(`{ "sub": "1234567890", "name": "John Doe", "iat": 1516239022 }`),
				Algorithm: "HS256",
			},
			nil,
		},
		{
			"Valid JWT with Additional Fields",
			"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
			&JsonWebToken{
				Signature:  "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
				Token:      "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
				Header:     datatypes.JSON(`{ "alg": "HS256", "typ": "JWT" }`),
				Payload:    datatypes.JSON(`{ "sub": "1234567890", "name": "John Doe", "iat": 1516239022 }`),
				Algorithm:  "HS256",
				Issuer:     "issuer",
				Subject:    "subject",
				Audience:   "audience",
				Expiration: time.Now().Add(time.Hour),
				IssuedAt:   time.Now(),
			},
			nil,
		},
		{
			"Valid JWT with Empty Claims",
			"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.Ne2Q3v8i0wo2xt27byyoj6JLr5tJvJ2CLWd8NDbnBk4",
			&JsonWebToken{
				Signature: "Ne2Q3v8i0wo2xt27byyoj6JLr5tJvJ2CLWd8NDbnBk4",
				Token:     "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30.Ne2Q3v8i0wo2xt27byyoj6JLr5tJvJ2CLWd8NDbnBk4",
				Header:    datatypes.JSON(`{ "alg": "HS256", "typ": "JWT" }`),
				Payload:   datatypes.JSON(`{}`),
				Algorithm: "HS256",
			},
			nil,
		},
		{
			"Invalid JWT Format",
			"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.e30",
			nil,
			errors.New("invalid JWT format"),
		},
		{
			"Invalid JWT Token",
			"invalid_token",
			nil,
			errors.New("invalid JWT format"),
		},
	}

	// Run each test case
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function
			token, err := FillJwtFromToken(tc.token)
			fmt.Println(token)
			if tc.expectedError != nil {
				assert.Equal(t, tc.expectedError.Error(), err.Error())
				return
			}
			// Check the results
			assert.Equal(t, tc.expectedToken.Token, token.Token)
			assert.Equal(t, tc.expectedToken.Signature, token.Signature)
			assert.Equal(t, tc.expectedToken.Algorithm, token.Algorithm)
			assert.Equal(t, tc.expectedError, err)
		})
	}
}
