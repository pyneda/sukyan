package reflection

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

// CharacterEfficiency represents how well a character passes through encoding/filtering
type CharacterEfficiency struct {
	Char       string // The character tested
	Efficiency int    // 0 = blocked, 90 = escaped, 100 = passed through unchanged
	EncodedAs  string // How the character appeared in the response (if found)
}

// TestCharacters are the special characters to test for encoding behavior
var TestCharacters = []string{
	"<", ">", // Tag injection
	"\"", "'", "`", // Quote breaking
	"(", ")", // Function calls
	"/",  // Comments, closing tags
	"=",  // Attributes
	";",  // Statement termination
	"\\", // Escape character
}

// EncodingTestOptions configures how character testing is performed
type EncodingTestOptions struct {
	TestAllEncodings bool // Test URL encoded, double encoded, etc.
}

// CanaryPrefix and CanarySuffix bracket every reflection probe. The prefix is a
// fixed family marker so scanner-injected content stays recognisable anywhere
// without per-request state; the nonce between them is what makes a match
// attributable to the request that sent it. Endpoints that store what we submit
// echo earlier probes back on later responses, so matching on the fixed part
// alone reports our own residue as live reflection.
const CanaryPrefix = "st4r7s"
const CanarySuffix = "3nd"

// NewCanaryNonce returns the per-request component of a canary.
func NewCanaryNonce() string {
	return lib.GenerateRandomLowercaseString(8)
}

func canaryPayload(nonce, value string) string {
	return CanaryPrefix + nonce + value + CanarySuffix
}

// AnalyzeCharacterEfficiencies tests each special character to determine how it's handled
// This is inspired by XSStrike's filterChecker approach
func AnalyzeCharacterEfficiencies(
	originalItem *db.History,
	insertionPoint InsertionPointInfo,
	client *http.Client,
	historyOptions http_utils.HistoryCreationOptions,
) []CharacterEfficiency {
	var efficiencies []CharacterEfficiency

	for _, char := range TestCharacters {
		efficiency := testCharacterEfficiency(originalItem, insertionPoint, char, client, historyOptions)
		efficiencies = append(efficiencies, efficiency)
	}

	return efficiencies
}

// InsertionPointInfo contains minimal info needed for character testing
type InsertionPointInfo struct {
	Name         string
	Type         string
	OriginalData string
}

// testCharacterEfficiency sends a request with the character and analyzes the response
func testCharacterEfficiency(
	originalItem *db.History,
	insertionPoint InsertionPointInfo,
	char string,
	client *http.Client,
	historyOptions http_utils.HistoryCreationOptions,
) CharacterEfficiency {
	nonce := NewCanaryNonce()
	testPayload := canaryPayload(nonce, char)

	result := CharacterEfficiency{
		Char:       char,
		Efficiency: 0,
	}

	// Build and send the request
	req, err := buildTestRequest(originalItem, insertionPoint, testPayload)
	if err != nil {
		log.Debug().Err(err).Str("char", char).Msg("Failed to build test request for character")
		return result
	}

	execResult := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		Client:                 client,
		CreateHistory:          false, // Don't pollute history with encoding tests
		HistoryCreationOptions: historyOptions,
	})

	if execResult.Err != nil {
		log.Debug().Err(execResult.Err).Str("char", char).Msg("Failed to execute test request for character")
		return result
	}

	// Analyze the response
	body := string(execResult.ResponseData.Body)

	// Check if the character passed through unchanged
	if strings.Contains(body, testPayload) {
		result.Efficiency = 100
		result.EncodedAs = char
		return result
	}

	// Check if the character was escaped (backslash added)
	escapedPayload := canaryPayload(nonce, "\\"+char)
	if strings.Contains(body, escapedPayload) {
		result.Efficiency = 90
		result.EncodedAs = "\\" + char
		return result
	}

	// Check for HTML entity encoding
	htmlEncoded := html.EscapeString(char)
	if htmlEncoded != char {
		htmlEncodedPayload := canaryPayload(nonce, htmlEncoded)
		if strings.Contains(body, htmlEncodedPayload) {
			result.Efficiency = getHTMLEncodedEfficiency(char)
			result.EncodedAs = htmlEncoded
			return result
		}
	}

	// Check for URL encoding
	urlEncoded := url.QueryEscape(char)
	if urlEncoded != char {
		urlEncodedPayload := canaryPayload(nonce, urlEncoded)
		if strings.Contains(body, urlEncodedPayload) {
			result.Efficiency = 50
			result.EncodedAs = urlEncoded
			return result
		}
	}

	// Check for numeric HTML entities
	numericEntity := getNumericHTMLEntity(char)
	if numericEntity != "" {
		numericPayload := canaryPayload(nonce, numericEntity)
		if strings.Contains(body, numericPayload) {
			result.Efficiency = getHTMLEncodedEfficiency(char)
			result.EncodedAs = numericEntity
			return result
		}
	}

	// Check if this probe's canary markers are present but the char is stripped
	if strings.Contains(body, CanaryPrefix+nonce) && strings.Contains(body, CanarySuffix) {
		// Character was stripped/filtered
		result.Efficiency = 0
		result.EncodedAs = "[stripped]"
		return result
	}

	// Canary not found at all - parameter might not be reflected
	result.Efficiency = 0
	result.EncodedAs = "[not reflected]"
	return result
}

// canInjectCanary reports whether buildTestRequest can actually place a payload
// in this insertion point. Every other type falls through to "send the original
// request unchanged", which produces a not-reflected result that says nothing
// about the target. Callers use this to tell "not reflected" apart from
// "never asked".
//
// It must stay in sync with buildTestRequest below;
// TestCanInjectCanaryMatchesBuildTestRequest enforces that by checking whether
// the canary actually appears in the request buildTestRequest produces.
func canInjectCanary(insertionPoint InsertionPointInfo, contentType string) bool {
	switch insertionPoint.Type {
	case "parameter", "urlpath", "header", "cookie":
		return true
	case "body":
		return strings.Contains(contentType, "application/x-www-form-urlencoded") ||
			strings.Contains(contentType, "application/json")
	default:
		return false
	}
}

// buildTestRequest creates an HTTP request with the test payload in the insertion point
func buildTestRequest(originalItem *db.History, insertionPoint InsertionPointInfo, payload string) (*http.Request, error) {
	newURL := originalItem.URL

	// Handle URL parameters
	if insertionPoint.Type == "parameter" {
		parsedURL, err := url.Parse(newURL)
		if err != nil {
			return nil, err
		}
		q := parsedURL.Query()
		q.Set(insertionPoint.Name, payload)
		parsedURL.RawQuery = q.Encode()
		newURL = parsedURL.String()
	}

	// Handle URL path segments. Mirrors createRequestFromURLPath in
	// pkg/scan/fill_insertion_points.go, which is what actually delivers payloads
	// here — if the canary went somewhere else, reflection would be measured for a
	// location the payload never reaches.
	if insertionPoint.Type == "urlpath" {
		parsedURL, err := url.Parse(newURL)
		if err != nil {
			return nil, err
		}
		segments := strings.Split(parsedURL.Path, "/")
		for i, segment := range segments {
			if segment == insertionPoint.Name {
				segments[i] = payload
			}
		}
		parsedURL.Path = strings.Join(segments, "/")
		newURL = parsedURL.String()
	}

	// Handle body parameters
	var requestBody io.Reader
	contentType := originalItem.RequestContentType

	if insertionPoint.Type == "body" {
		body, err := originalItem.RequestBody()
		if err != nil {
			return nil, err
		}

		switch {
		case strings.Contains(contentType, "application/x-www-form-urlencoded"):
			values, err := url.ParseQuery(string(body))
			if err != nil {
				return nil, err
			}
			values.Set(insertionPoint.Name, payload)
			requestBody = strings.NewReader(values.Encode())

		case strings.Contains(contentType, "application/json"):
			var jsonData map[string]interface{}
			if err := json.Unmarshal(body, &jsonData); err != nil {
				return nil, err
			}
			jsonData[insertionPoint.Name] = payload
			modifiedBody, err := json.Marshal(jsonData)
			if err != nil {
				return nil, err
			}
			requestBody = bytes.NewReader(modifiedBody)

		default:
			// Fall back to original body
			requestBody = bytes.NewReader(body)
		}
	} else {
		// For non-body insertion points, use original body
		body, _ := originalItem.RequestBody()
		if len(body) > 0 {
			requestBody = bytes.NewReader(body)
		}
	}

	req, err := http.NewRequest(originalItem.Method, newURL, requestBody)
	if err != nil {
		return nil, err
	}

	// Set headers from original request
	http_utils.SetRequestHeadersFromHistoryItem(req, originalItem)

	// Header and cookie points are injected after the original headers are
	// applied, so the canary overrides the recorded value rather than being
	// overwritten by it.
	switch insertionPoint.Type {
	case "header":
		req.Header.Set(insertionPoint.Name, payload)
	case "cookie":
		if existing := req.Header.Get("Cookie"); existing != "" {
			cookies := http_utils.ParseCookies(existing)
			for _, cookie := range cookies {
				if cookie.Name == insertionPoint.Name {
					cookie.Value = payload
				}
			}
			req.Header.Set("Cookie", http_utils.JoinCookies(cookies))
		}
	}

	return req, nil
}

// getHTMLEncodedEfficiency returns the efficiency for HTML encoded characters
// Some HTML entities can still be useful depending on context
func getHTMLEncodedEfficiency(char string) int {
	// In some contexts (like srcdoc), HTML entities are decoded
	switch char {
	case "<", ">":
		return 30 // Might work in srcdoc or innerHTML contexts
	case "\"", "'":
		return 40 // Might work in certain attribute contexts
	default:
		return 20
	}
}

// getNumericHTMLEntity returns the numeric HTML entity for a character
func getNumericHTMLEntity(char string) string {
	if len(char) != 1 {
		return ""
	}
	return fmt.Sprintf("&#%d;", char[0])
}

// EfficiencyFlags provides quick boolean access to encoding results
type EfficiencyFlags struct {
	CanInjectTags       bool // < and > pass through (efficiency >= 100)
	CanBreakDoubleQuote bool // " passes through
	CanBreakSingleQuote bool // ' passes through
	CanUseBackticks     bool // ` passes through
	CanCallFunctions    bool // ( and ) pass through
	CanUseSlash         bool // / passes through
	CanUseEquals        bool // = passes through
	CanUseSemicolon     bool // ; passes through
	CanEscape           bool // \ passes through
}

// ComputeEfficiencyFlags converts efficiencies to boolean flags for quick access
func ComputeEfficiencyFlags(efficiencies []CharacterEfficiency) EfficiencyFlags {
	flags := EfficiencyFlags{}
	effMap := make(map[string]int)

	for _, eff := range efficiencies {
		effMap[eff.Char] = eff.Efficiency
	}

	flags.CanInjectTags = effMap["<"] >= 100 && effMap[">"] >= 100
	flags.CanBreakDoubleQuote = effMap["\""] >= 100
	flags.CanBreakSingleQuote = effMap["'"] >= 100
	flags.CanUseBackticks = effMap["`"] >= 100
	flags.CanCallFunctions = effMap["("] >= 100 && effMap[")"] >= 100
	flags.CanUseSlash = effMap["/"] >= 100
	flags.CanUseEquals = effMap["="] >= 100
	flags.CanUseSemicolon = effMap[";"] >= 100
	flags.CanEscape = effMap["\\"] >= 100

	return flags
}

// GetEfficiencyForChar returns the efficiency for a specific character
func GetEfficiencyForChar(efficiencies []CharacterEfficiency, char string) int {
	for _, eff := range efficiencies {
		if eff.Char == char {
			return eff.Efficiency
		}
	}
	return 0
}
