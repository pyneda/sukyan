package payloads

import (
	"bufio"
	"embed"
	"regexp"

	"github.com/rs/zerolog/log"
)

//go:embed wordlists/*
var wordlistsFS embed.FS

type GenericPayload struct {
	BasePayload
	Value    string
	Platform string
}

// GetValue gets the payload value as string
func (p GenericPayload) GetValue() string {
	return p.Value
}

// MatchAgainstString Checks if the payload match against a string
func (p GenericPayload) MatchAgainstString(text string) (bool, error) {
	return regexp.MatchString(p.Value, text)
}

func GetXSSPayloads() []PayloadInterface {
	var payloads []PayloadInterface
	f, err := wordlistsFS.Open("wordlists/xss.txt")
	if err != nil {
		log.Error().Err(err).Msg("Failed to open XSS payloads file")
		return payloads
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		payloads = append(payloads, GenericPayload{
			Value:    scanner.Text(),
			Platform: "*",
		})
	}

	if err := scanner.Err(); err != nil {
		log.Error().Err(err).Msg("Error reading XSS payloads file")
	}

	return payloads
}
