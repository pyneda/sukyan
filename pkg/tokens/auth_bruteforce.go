package tokens

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/viper"
)

// Embedded wordlists
//
//go:embed wordlists/usernames.txt
var embeddedUsernamesDefault []byte

//go:embed wordlists/passwords.txt
var embeddedPasswordsDefault []byte

//go:embed wordlists/usernames-xs.txt
var embeddedUsernamesXs []byte

//go:embed wordlists/passwords-xs.txt
var embeddedPasswordsXs []byte

//go:embed wordlists/usernames-lg.txt
var embeddedUsernamesLg []byte

//go:embed wordlists/passwords-lg.txt
var embeddedPasswordsLg []byte

//go:embed wordlists/default-user-password-pair.txt
var embeddedUserPasswordPairsDefault []byte

//go:embed wordlists/default-user-password-pair-xs.txt
var embeddedUserPasswordPairsXs []byte

//go:embed wordlists/default-user-password-pair-lg.txt
var embeddedUserPasswordPairsLg []byte

type WordlistMode string
type CredentialMode string

const (
	WordlistModeEmbedded   WordlistMode = "embedded"   // Use only embedded wordlists
	WordlistModeFilesystem WordlistMode = "filesystem" // Use only custom files
	WordlistModeMixed      WordlistMode = "mixed"      // Use both embedded and custom files
)

const (
	CredentialModeSeparate CredentialMode = "separate" // Use separate username and password lists
	CredentialModePairs    CredentialMode = "pairs"    // Use username:password pairs format
)

type AuthType string

const (
	AuthTypeBasic  AuthType = "basic"
	AuthTypeDigest AuthType = "digest"
)

type AuthBruteforceResult struct {
	AuthType   AuthType
	Found      bool
	Username   string
	Password   string
	StatusCode int
	Duration   time.Duration
	Attempts   int
	History    *db.History
	Error      error
	mu         sync.Mutex
}

type AuthBruteforceConfig struct {
	AuthType         AuthType
	Mode             string // "embedded", "filesystem", "mixed"
	Format           string // "separate", "pairs"
	Size             string // "xs", "default", "lg"
	CustomUsernames  string // Custom usernames file path
	CustomPasswords  string // Custom passwords file path
	CustomPairs      string // Custom pairs file path
	Concurrency      int
	MaxAttempts      int
	StopOnSuccess    bool
	RequestTimeout   time.Duration
	DelayBetweenReqs time.Duration
}

func BruteforceAuth(historyItem *db.History, authHeader string, config AuthBruteforceConfig) *AuthBruteforceResult {
	result := &AuthBruteforceResult{
		AuthType: config.AuthType,
		Duration: 0,
		Found:    false,
		Attempts: 0,
	}

	startTime := time.Now()
	defer func() {
		result.Duration = time.Since(startTime)
	}()

	// Load wordlists based on configuration
	var usernames, passwords []string
	var err error

	if config.Format == "pairs" {
		// Load username:password pairs and split them
		usernames, passwords, err = loadUserPasswordPairs(config)
		if err != nil {
			result.Error = fmt.Errorf("failed to load user-password pairs: %w", err)
			return result
		}
	} else {
		// Load separate username and password wordlists
		usernames, err = loadUsernameWordlist(config)
		if err != nil {
			result.Error = fmt.Errorf("failed to load usernames: %w", err)
			return result
		}

		passwords, err = loadPasswordWordlist(config)
		if err != nil {
			result.Error = fmt.Errorf("failed to load passwords: %w", err)
			return result
		}
	}

	if len(usernames) == 0 || len(passwords) == 0 {
		result.Error = fmt.Errorf("empty wordlists")
		return result
	}

	log.Info().
		Str("auth_type", string(config.AuthType)).
		Int("usernames", len(usernames)).
		Int("passwords", len(passwords)).
		Int("total_combinations", len(usernames)*len(passwords)).
		Str("url", historyItem.URL).
		Msg("Starting authentication brute force")

	totalCombinations := len(usernames) * len(passwords)
	if config.MaxAttempts > 0 && config.MaxAttempts < totalCombinations {
		totalCombinations = config.MaxAttempts
	}

	progressInterval := totalCombinations / 10
	if progressInterval < 1 {
		progressInterval = 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := pool.New().WithMaxGoroutines(config.Concurrency).WithContext(ctx)

	historyOptions := http_utils.HistoryCreationOptions{
		WorkspaceID: *historyItem.WorkspaceID,
		TaskID:      *historyItem.TaskID,
	}

	attemptCount := 0
	for _, username := range usernames {
		for _, password := range passwords {
			if config.MaxAttempts > 0 && attemptCount >= config.MaxAttempts {
				break
			}

			username := username
			password := password
			attemptCount++

			p.Go(func(ctx context.Context) error {
				select {
				case <-ctx.Done():
					return nil
				default:
					result.mu.Lock()
					if result.Found && config.StopOnSuccess {
						result.mu.Unlock()
						return nil
					}
					result.mu.Unlock()

					success, statusCode, history, err := attemptAuth(historyItem, authHeader, username, password, config.AuthType, historyOptions)

					result.mu.Lock()
					result.Attempts++

					if success {
						log.Info().
							Str("username", username).
							Str("password", password).
							Int("status_code", statusCode).
							Msg("Authentication succeeded!")
						result.Found = true
						result.Username = username
						result.Password = password
						result.StatusCode = statusCode
						result.History = history
						if config.StopOnSuccess {
							cancel()
						}
					}

					if err != nil {
						log.Debug().Err(err).
							Str("username", username).
							Str("password", password).
							Msg("Auth attempt error")
					}

					if result.Attempts%progressInterval == 0 {
						progress := (result.Attempts * 100) / totalCombinations
						log.Info().
							Int("progress", progress).
							Int("attempts", result.Attempts).
							Int("total", totalCombinations).
							Msg("Brute force progress")
					}
					result.mu.Unlock()

					if config.DelayBetweenReqs > 0 {
						time.Sleep(config.DelayBetweenReqs)
					}

					return nil
				}
			})
		}
		if config.MaxAttempts > 0 && attemptCount >= config.MaxAttempts {
			break
		}
	}

	p.Wait()

	if result.Found {
		log.Info().
			Str("username", result.Username).
			Str("password", result.Password).
			Str("auth_type", string(config.AuthType)).
			Float64("duration_seconds", result.Duration.Seconds()).
			Msg("Brute force completed successfully!")
	} else {
		log.Info().
			Int("attempts", result.Attempts).
			Float64("duration_seconds", result.Duration.Seconds()).
			Msg("Brute force completed, no valid credentials found")
	}

	return result
}

func attemptAuth(historyItem *db.History, authHeader, username, password string, authType AuthType, historyOptions http_utils.HistoryCreationOptions) (bool, int, *db.History, error) {
	req, err := http_utils.BuildRequestFromHistoryItem(historyItem)
	if err != nil {
		return false, 0, nil, fmt.Errorf("failed to build request from history: %w", err)
	}

	switch authType {
	case AuthTypeBasic:
		auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		req.Header.Set("Authorization", "Basic "+auth)
	case AuthTypeDigest:
		digestAuth, err := createDigestAuth(username, password, authHeader, req.Method, req.URL.Path)
		if err != nil {
			return false, 0, nil, fmt.Errorf("failed to create digest auth: %w", err)
		}
		req.Header.Set("Authorization", digestAuth)
	default:
		return false, 0, nil, fmt.Errorf("unsupported auth type: %s", authType)
	}

	executionResult := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		CreateHistory:          true,
		HistoryCreationOptions: historyOptions,
	})

	if executionResult.Err != nil {
		return false, 0, nil, executionResult.Err
	}

	statusCode := executionResult.Response.StatusCode
	success := statusCode >= 200 && statusCode < 400

	return success, statusCode, executionResult.History, nil
}

func createDigestAuth(username, password, authHeader, method, uri string) (string, error) {
	params := ParseDigestParams(authHeader)

	realm, ok := params["realm"]
	if !ok {
		return "", fmt.Errorf("missing realm in digest auth")
	}

	nonce, ok := params["nonce"]
	if !ok {
		return "", fmt.Errorf("missing nonce in digest auth")
	}

	cnonce := generateCnonce()
	nc := "00000001"

	ha1 := md5Hash(username + ":" + realm + ":" + password)
	ha2 := md5Hash(method + ":" + uri)

	qop := params["qop"]
	var response string
	if qop == "auth" || qop == "auth-int" {
		response = md5Hash(ha1 + ":" + nonce + ":" + nc + ":" + cnonce + ":" + qop + ":" + ha2)
	} else {
		response = md5Hash(ha1 + ":" + nonce + ":" + ha2)
	}

	var authParts []string
	authParts = append(authParts, fmt.Sprintf(`username="%s"`, username))
	authParts = append(authParts, fmt.Sprintf(`realm="%s"`, realm))
	authParts = append(authParts, fmt.Sprintf(`nonce="%s"`, nonce))
	authParts = append(authParts, fmt.Sprintf(`uri="%s"`, uri))
	authParts = append(authParts, fmt.Sprintf(`response="%s"`, response))

	if algorithm, ok := params["algorithm"]; ok {
		authParts = append(authParts, fmt.Sprintf(`algorithm="%s"`, algorithm))
	}

	if qop != "" {
		authParts = append(authParts, fmt.Sprintf(`qop=%s`, qop))
		authParts = append(authParts, fmt.Sprintf(`nc=%s`, nc))
		authParts = append(authParts, fmt.Sprintf(`cnonce="%s"`, cnonce))
	}

	return "Digest " + strings.Join(authParts, ", "), nil
}

func ParseDigestParams(authHeader string) map[string]string {
	params := make(map[string]string)

	authHeader = strings.TrimPrefix(authHeader, "Digest ")

	re := regexp.MustCompile(`(\w+)=(?:"([^"]*)"|([^,\s]*))`)
	matches := re.FindAllStringSubmatch(authHeader, -1)

	for _, match := range matches {
		key := match[1]
		value := match[2]
		if value == "" {
			value = match[3]
		}
		params[key] = value
	}

	return params
}

func ExtractDigestParam(authHeader string, param string) string {
	params := ParseDigestParams(authHeader)
	return params[param]
}

func generateCnonce() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func md5Hash(text string) string {
	hash := md5.Sum([]byte(text))
	return fmt.Sprintf("%x", hash)
}

// loadUsernameWordlist loads username wordlist based on configuration
func loadUsernameWordlist(config AuthBruteforceConfig) ([]string, error) {
	if config.Mode == "filesystem" || config.Mode == "mixed" {
		if config.CustomUsernames != "" {
			// Use custom wordlist file
			if config.Mode == "filesystem" {
				return loadCredentialsFromFile(config.CustomUsernames)
			} else {
				// Mixed mode: combine custom and embedded
				customList, err := loadCredentialsFromFile(config.CustomUsernames)
				if err != nil {
					log.Warn().Err(err).Msg("Failed to load custom usernames, using embedded only")
				} else {
					embeddedList, _ := loadEmbeddedUsernameWordlist(config.Size)
					return append(customList, embeddedList...), nil
				}
			}
		}
	}

	// Use embedded wordlist
	return loadEmbeddedUsernameWordlist(config.Size)
}

// loadEmbeddedUsernameWordlist loads embedded username wordlist by size
func loadEmbeddedUsernameWordlist(size string) ([]string, error) {
	var embeddedData []byte

	switch size {
	case "xs":
		embeddedData = embeddedUsernamesXs
	case "lg":
		embeddedData = embeddedUsernamesLg
	default: // "default"
		embeddedData = embeddedUsernamesDefault
	}

	return loadCredentialsFromBytes(embeddedData)
}

// loadPasswordWordlist loads password wordlist based on configuration
func loadPasswordWordlist(config AuthBruteforceConfig) ([]string, error) {
	if config.Mode == "filesystem" || config.Mode == "mixed" {
		if config.CustomPasswords != "" {
			// Use custom wordlist file
			if config.Mode == "filesystem" {
				return loadCredentialsFromFile(config.CustomPasswords)
			} else {
				// Mixed mode: combine custom and embedded
				customList, err := loadCredentialsFromFile(config.CustomPasswords)
				if err != nil {
					log.Warn().Err(err).Msg("Failed to load custom passwords, using embedded only")
				} else {
					embeddedList, _ := loadEmbeddedPasswordWordlist(config.Size)
					return append(customList, embeddedList...), nil
				}
			}
		}
	}

	// Use embedded wordlist
	return loadEmbeddedPasswordWordlist(config.Size)
}

// loadEmbeddedPasswordWordlist loads embedded password wordlist by size
func loadEmbeddedPasswordWordlist(size string) ([]string, error) {
	var embeddedData []byte

	switch size {
	case "xs":
		embeddedData = embeddedPasswordsXs
	case "lg":
		embeddedData = embeddedPasswordsLg
	default: // "default"
		embeddedData = embeddedPasswordsDefault
	}

	return loadCredentialsFromBytes(embeddedData)
}

// loadUserPasswordPairs loads username:password pairs and splits them
func loadUserPasswordPairs(config AuthBruteforceConfig) ([]string, []string, error) {
	var lines []string
	var err error

	if config.Mode == "filesystem" || config.Mode == "mixed" {
		if config.CustomPairs != "" {
			// Use custom pairs file
			if config.Mode == "filesystem" {
				lines, err = loadCredentialsFromFile(config.CustomPairs)
			} else {
				// Mixed mode: combine custom and embedded
				customLines, customErr := loadCredentialsFromFile(config.CustomPairs)
				if customErr != nil {
					log.Warn().Err(customErr).Msg("Failed to load custom pairs, using embedded only")
				} else {
					embeddedLines, _ := loadEmbeddedUserPasswordPairs(config.Size)
					lines = append(customLines, embeddedLines...)
				}
			}
		}
	}

	// If no custom pairs loaded, use embedded pairs
	if len(lines) == 0 {
		lines, err = loadEmbeddedUserPasswordPairs(config.Size)
	}

	if err != nil {
		return nil, nil, err
	}

	var usernames, passwords []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			usernames = append(usernames, strings.TrimSpace(parts[0]))
			passwords = append(passwords, strings.TrimSpace(parts[1]))
		}
	}

	return usernames, passwords, nil
}

// loadEmbeddedUserPasswordPairs loads embedded user:password pairs by size
func loadEmbeddedUserPasswordPairs(size string) ([]string, error) {
	var embeddedData []byte

	switch size {
	case "xs":
		embeddedData = embeddedUserPasswordPairsXs
	case "lg":
		embeddedData = embeddedUserPasswordPairsLg
	default: // "default"
		embeddedData = embeddedUserPasswordPairsDefault
	}

	return loadCredentialsFromBytes(embeddedData)
}

// getConfiguredWordlistSize gets wordlist size from viper config with fallback
func getConfiguredWordlistSize(wordlistType, configuredSize string) string {
	if configuredSize != "" {
		// Validate the size value
		if configuredSize == "xs" || configuredSize == "default" || configuredSize == "lg" {
			return configuredSize
		}
		log.Warn().Str("configured_size", configuredSize).Str("wordlist_type", wordlistType).
			Msg("Invalid wordlist size provided, falling back to default")
		return "default"
	}

	// Try to get from viper configuration
	configKey := fmt.Sprintf("auth.bruteforce.wordlists.%s.size", wordlistType)
	if viperSize := viper.GetString(configKey); viperSize != "" {
		// Validate the size value
		if viperSize == "xs" || viperSize == "default" || viperSize == "lg" {
			return viperSize
		}
		log.Warn().Str("configured_size", viperSize).Str("wordlist_type", wordlistType).
			Msg("Invalid wordlist size in configuration, falling back to default")
	}

	return "default"
}

// loadCredentialsFromFile loads credentials from a file
func loadCredentialsFromFile(filepath string) ([]string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filepath, err)
	}
	defer file.Close()

	var credentials []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			credentials = append(credentials, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file %s: %w", filepath, err)
	}

	return credentials, nil
}

// loadCredentialsFromBytes loads credentials from embedded bytes
func loadCredentialsFromBytes(data []byte) ([]string, error) {
	var credentials []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			credentials = append(credentials, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading embedded data: %w", err)
	}

	return credentials, nil
}

// CreateDefaultAuthBruteforceConfig creates a default configuration for auth bruteforce with validation
func CreateDefaultAuthBruteforceConfig(authType AuthType) AuthBruteforceConfig {
	// Get values from viper with validation and fallbacks
	mode := viper.GetString("auth.bruteforce.mode")
	if mode != "embedded" && mode != "filesystem" && mode != "mixed" {
		log.Warn().Str("invalid_mode", mode).Msg("Invalid auth bruteforce mode, falling back to 'embedded'")
		mode = "embedded"
	}

	format := viper.GetString("auth.bruteforce.format")
	if format != "pairs" && format != "separate" {
		log.Warn().Str("invalid_format", format).Msg("Invalid auth bruteforce format, falling back to 'pairs'")
		format = "pairs"
	}

	size := viper.GetString("auth.bruteforce.size")
	if size != "xs" && size != "default" && size != "lg" {
		log.Warn().Str("invalid_size", size).Msg("Invalid auth bruteforce size, falling back to 'default'")
		size = "default"
	}

	concurrency := viper.GetInt("auth.bruteforce.concurrency")
	if concurrency <= 0 || concurrency > 50 {
		log.Warn().Int("invalid_concurrency", concurrency).Msg("Invalid concurrency value, falling back to 5")
		concurrency = 5
	}

	maxAttempts := viper.GetInt("auth.bruteforce.max_attempts")
	if maxAttempts < 0 || maxAttempts > 10000 {
		log.Warn().Int("invalid_max_attempts", maxAttempts).Msg("Invalid max attempts value, falling back to 500")
		maxAttempts = 500
	}

	requestTimeout := viper.GetDuration("auth.bruteforce.request_timeout")
	if requestTimeout <= 0 || requestTimeout > 5*time.Minute {
		log.Warn().Dur("invalid_timeout", requestTimeout).Msg("Invalid request timeout, falling back to 30s")
		requestTimeout = 30 * time.Second
	}

	delayBetweenReqs := viper.GetDuration("auth.bruteforce.delay_between_requests")
	if delayBetweenReqs < 0 || delayBetweenReqs > 10*time.Second {
		log.Warn().Dur("invalid_delay", delayBetweenReqs).Msg("Invalid delay between requests, falling back to 100ms")
		delayBetweenReqs = 100 * time.Millisecond
	}

	// Validate custom file paths if using filesystem or mixed mode
	customUsernames := viper.GetString("auth.bruteforce.custom.usernames")
	customPasswords := viper.GetString("auth.bruteforce.custom.passwords")
	customPairs := viper.GetString("auth.bruteforce.custom.pairs")

	if mode == "filesystem" {
		if format == "pairs" && customPairs == "" {
			log.Warn().Msg("Filesystem mode with pairs format requires custom pairs file, falling back to embedded mode")
			mode = "embedded"
		} else if format == "separate" && (customUsernames == "" || customPasswords == "") {
			log.Warn().Msg("Filesystem mode with separate format requires both usernames and passwords files, falling back to embedded mode")
			mode = "embedded"
		}
	}

	return AuthBruteforceConfig{
		AuthType:         authType,
		Mode:             mode,
		Format:           format,
		Size:             size,
		CustomUsernames:  customUsernames,
		CustomPasswords:  customPasswords,
		CustomPairs:      customPairs,
		Concurrency:      concurrency,
		MaxAttempts:      maxAttempts,
		StopOnSuccess:    viper.GetBool("auth.bruteforce.stop_on_success"),
		RequestTimeout:   requestTimeout,
		DelayBetweenReqs: delayBetweenReqs,
	}
}

// CreateAuthBruteforceConfigWithUserPasswordPairs creates a configuration for using username:password pairs
func CreateAuthBruteforceConfigWithUserPasswordPairs(authType AuthType) AuthBruteforceConfig {
	config := CreateDefaultAuthBruteforceConfig(authType)
	config.Format = "pairs"
	return config
}
