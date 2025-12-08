package auth

import (
	"errors"
	"fmt"

	"unicode"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

// NormalizePassword func for a returning the users input as a byte slice.
func NormalizePassword(p string) []byte {
	return []byte(p)
}

// GeneratePassword func for a making hash & salt with user password.
func GeneratePassword(p string) string {
	return GeneratePasswordWithCost(p, bcrypt.DefaultCost)
}

// GeneratePasswordWithCost func for making hash & salt with user password and custom bcrypt cost.
func GeneratePasswordWithCost(p string, cost int) string {
	// Normalize password from string to []byte.
	bytePwd := NormalizePassword(p)

	// The cost can be any value provided it's between bcrypt.MinCost (4) and bcrypt.MaxCost (31).
	hash, err := bcrypt.GenerateFromPassword(bytePwd, cost)
	if err != nil {
		log.Error().Err(err).Msg("Error generating password hash")
		return ""
	}

	// GenerateFromPassword returns a byte slice so we need to
	// convert the bytes to a string and return it.
	return string(hash)
}

// ComparePasswords func for a comparing password.
func ComparePasswords(hashedPwd, inputPwd string) bool {
	// Since we'll be getting the hashed password from the DB it will be a string,
	// so we'll need to convert it to a byte slice.
	byteHash := NormalizePassword(hashedPwd)
	byteInput := NormalizePassword(inputPwd)

	// Return result.
	if err := bcrypt.CompareHashAndPassword(byteHash, byteInput); err != nil {
		log.Error().Err(err).Msg("Error comparing passwords")
		return false
	}

	return true
}

const MinPasswordLength = 7

var ErrEmptyPassword = errors.New("No password provided")
var ErrPasswordTooShort = fmt.Errorf("Password must be at least %d characters", MinPasswordLength)
var ErrMissingLetterOrNumber = errors.New("Password must contain both letters and numbers")

// CheckPasswordPolicy checks if a password meets the minimum requirements.
func CheckPasswordPolicy(password string) error {
	hasLetter := false
	hasNumber := false

	for _, char := range password {
		switch {
		case unicode.IsLetter(char):
			hasLetter = true
		case unicode.IsNumber(char):
			hasNumber = true
		}
	}

	switch {
	case password == "":
		return ErrEmptyPassword
	case len(password) < MinPasswordLength:
		return ErrPasswordTooShort
	case !hasLetter || !hasNumber:
		return ErrMissingLetterOrNumber
	}
	return nil
}
