package lib

import (
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"

	"github.com/rs/zerolog/log"
)

// DefaultRandomStringsCharset Default charset used for random string generation
const DefaultRandomStringsCharset = "abcdedfghijklmnopqrstABCDEFGHIJKLMNOP"

// Need to refactor existing contains to SliceContains
func Contains(slice []string, item string) bool {
	set := make(map[string]struct{}, len(slice))
	for _, s := range slice {
		set[s] = struct{}{}
	}

	_, ok := set[item]
	return ok
}

// SliceContains utility function to check if a slice of strings contains the specified string
func SliceContains(slice []string, item string) bool {
	set := make(map[string]struct{}, len(slice))
	for _, s := range slice {
		set[s] = struct{}{}
	}

	_, ok := set[item]
	return ok
}

// GetParametersToTest ...
func GetParametersToTest(path string, params []string, testAllParams bool) (parametersToTest []string) {
	parametersToTest = append(parametersToTest, params...)

	if testAllParams == false && len(params) > 0 {
		return parametersToTest
	}
	parsedURL, err := url.ParseRequestURI(path)
	if err != nil {
		log.Error().Err(err).Str("url", path).Msg("Invalid url")
	}
	query, err := url.ParseQuery(parsedURL.RawQuery)
	if err != nil {
		log.Warn().Str("url", path).Msg("Could not parse url query")
	}
	for key := range query {

		if Contains(params, key) == true {
			continue
		} else if testAllParams || len(params) == 0 {
			// If provided by params[], we ignore as already added
			parametersToTest = append(parametersToTest, key)
		}
	}
	return parametersToTest
}

// BuildURLWithParam ...
func BuildURLWithParam(original string, param string, payload string, urlEncode bool) (string, error) {
	parsedURL, err := url.ParseRequestURI(original)
	if err != nil {
		return "", err
	}
	values, _ := url.ParseQuery(parsedURL.RawQuery)
	if urlEncode {
		values.Set(param, payload)
	} else {
		values.Del(param)
	}

	parsedURL.RawQuery = values.Encode()
	testurl := parsedURL.String()
	// log.Printf("web url: %s", original)
	if urlEncode == false {
		if len(values) == 0 {
			if strings.HasSuffix(parsedURL.String(), "/") {
				testurl = parsedURL.String() + "?" + param + "=" + payload
			} else {
				testurl = parsedURL.String() + "/?" + param + "=" + payload
			}
		} else {
			testurl = parsedURL.String() + "&" + param + "=" + payload
		}
	}
	return testurl, nil
}

// GenerateRandomString returns a random string of the defined length
func GenerateRandomString(length int) string {
	var output strings.Builder
	charSet := DefaultRandomStringsCharset
	for i := 0; i < length; i++ {
		random := rand.Intn(len(charSet))
		randomChar := charSet[random]
		output.WriteString(string(randomChar))
	}
	return output.String()
}

func GenerateRandomLowercaseString(length int) string {
	result := GenerateRandomString(length)
	return strings.ToLower(result)
}

// Build404URL Adds a randomly generated path to the URL to fingerprint 404 errors
func Build404URL(original string) (string, error) {
	u, err := url.Parse(original)
	if err != nil {
		return "", err
	}
	length := rand.Intn(50)
	length = length + 10
	u.Path = path.Join(u.Path, GenerateRandomString(length))
	u.Path = path.Join(u.Path, GenerateRandomString(length))
	result := u.String()
	return result, err
}

func LocalFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || os.IsExist(err)
}

// func GetUrlParams(weburl string) []string {
// 	parsedURL, err := url.ParseRequestURI(weburl)
// 	if err != nil {
// 		fmt.Printf("Error Invalid URL: %s\n", weburl)
// 		return nil
// 	}
// 	return parsedURL
// }

// StringsSliceToText iterates a slice of strings to generate a text list, mainly for reporting
func StringsSliceToText(items []string) string {
	var sb strings.Builder
	for _, item := range items {
		sb.WriteString(" - " + item + "\n")
	}
	return sb.String()
}

// SetupCloseHandler creates a 'listener' on a new goroutine which will notify the
// program if it receives an interrupt from the OS. We then handle this by calling
// our clean up procedure and exiting the program.
func SetupCloseHandler() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\r- Ctrl+C pressed in Terminal")
		os.Exit(0)
	}()
}

// GetURLWithoutQueryString returns the base URL from the given URL by removing the query string
func GetURLWithoutQueryString(urlStr string) (string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	parsedURL.RawQuery = ""
	return parsedURL.String(), nil
}

func IsRootURL(urlStr string) (bool, error) {
	parsedURL, err := url.Parse(urlStr)
	parsedURL.RawQuery = ""
	if err != nil {
		return false, err
	}

	isRoot := strings.Trim(parsedURL.Path, "/") == "" && parsedURL.RawQuery == ""

	return isRoot, nil
}

// GetParentURL returns the parent URL for the given URL. If the given URL
// is already a parent URL, the function returns true as the second return value.
func GetParentURL(urlStr string) (string, bool, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", false, err
	}

	parentURL := parsedURL
	parentURL.Path = path.Dir(parsedURL.Path)

	isParentURL := parentURL.Path == "." || parentURL.Path == "/"

	return parentURL.String(), isParentURL, nil
}
