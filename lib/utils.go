package lib

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode"
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

func LocalFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || os.IsExist(err)
}

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

// GenerateRandInt generates a random integer between min and max
func GenerateRandInt(min, max int) int {
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(max-min+1) + min
}

// GetUniqueItems takes a slice of strings and returns a new slice with unique items.
func GetUniqueItems(items []string) []string {
	uniqueItemsMap := make(map[string]bool)
	for _, item := range items {
		uniqueItemsMap[item] = true
	}

	uniqueItems := make([]string, 0, len(uniqueItemsMap))
	for item := range uniqueItemsMap {
		uniqueItems = append(uniqueItems, item)
	}

	return uniqueItems
}

// CapitalizeFirstLetter capitalizes the first letter of a string
func CapitalizeFirstLetter(input string) string {
	for _, v := range input {
		u := string(unicode.ToUpper(v))
		return u + input[len(u):]
	}
	return ""
}
