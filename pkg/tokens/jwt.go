package tokens

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

//go:embed wordlists/jwt-secrets.txt
var embeddedWordlist []byte

type CrackResult struct {
	Attempts int
	Duration time.Duration
	Found    bool
	Secret   string
	mu       sync.Mutex
}

func CrackJWT(token, wordlist string, concurrency int, useEmbedded bool) *CrackResult {
	var scanner *bufio.Scanner
	var wordlistFile *os.File
	var err error

	if useEmbedded {
		scanner = bufio.NewScanner(bytes.NewReader(embeddedWordlist))
	} else {
		wordlistFile, err = os.Open(wordlist)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to open wordlist")
		}
		defer wordlistFile.Close()
		scanner = bufio.NewScanner(wordlistFile)
	}

	totalWords := countLines(scanner)
	if totalWords == 0 {
		log.Info().Msg("Wordlist is empty. Aborting crack attempt.")
		return &CrackResult{Duration: time.Since(time.Now())}
	}

	if useEmbedded {
		scanner = bufio.NewScanner(bytes.NewReader(embeddedWordlist))
	} else {
		_, err := wordlistFile.Seek(0, 0)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to reset wordlist file pointer")
		}
		scanner = bufio.NewScanner(wordlistFile)
	}

	progressInterval := totalWords / 10
	if progressInterval < 1 {
		progressInterval = 1
	}

	result := &CrackResult{
		Attempts: 0,
		Duration: 0,
		Found:    false,
		Secret:   "",
	}

	startTime := time.Now()
	log.Info().Str("jwt", token).Int("total", totalWords).Msg("Starting JWT cracking...")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := pool.New().WithMaxGoroutines(concurrency).WithContext(ctx)

	for scanner.Scan() {
		word := scanner.Text()
		p.Go(func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return nil
			default:
				result.mu.Lock()
				if result.Found {
					result.mu.Unlock()
					return nil
				}
				result.mu.Unlock()

				success, _ := decodeAndVerifyJWT(token, word)

				result.mu.Lock()
				result.Attempts++
				// log.Info().Int("attempts", result.Attempts).Str("word", word).Msg("Cracking JWT")

				if success {
					log.Info().Str("word", word).Msg("Found JWT secret!")
					result.Found = true
					result.Secret = word
					cancel() // Stop all other goroutines
				}

				if result.Attempts%progressInterval == 0 {
					progress := (result.Attempts * 100) / totalWords
					log.Info().Int("progress", progress).Int("attempts", result.Attempts).Int("total", totalWords).Msg("Cracking progress")
				}
				result.mu.Unlock()

				return nil
			}
		})
	}

	p.Wait()

	result.Duration = time.Since(startTime)

	if result.Found {
		log.Info().Str("secret", result.Secret).Msg("Cracking finished with success!")
	} else {
		log.Info().Msg("Cracking finished, no secret found.")
	}

	return result
}

func countLines(scanner *bufio.Scanner) int {
	lineCount := 0
	for scanner.Scan() {
		lineCount++
	}
	return lineCount
}

func decodeAndVerifyJWT(tokenString, secret string) (bool, *jwt.Token) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return false, nil
	}
	return true, token
}
