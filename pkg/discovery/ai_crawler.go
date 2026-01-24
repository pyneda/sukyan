package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

// AICrawlerPaths contains paths for AI crawler instruction files
var AICrawlerPaths = []string{
	"llms.txt",
	"llms-full.txt",
	".well-known/llms.txt",
}

// IsLlmsTxtValidationFunc validates llms.txt files
func IsLlmsTxtValidationFunc(history *db.History, ctx *ValidationContext) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	body, err := history.ResponseBody()
	if err != nil {
		return false, "", 0
	}

	bodyStr := string(body)
	confidence := 0
	var details strings.Builder

	details.WriteString(fmt.Sprintf("LLMs.txt file detected: %s\n\n", history.URL))

	// Check content type
	if strings.Contains(history.ResponseContentType, "text/plain") {
		confidence += 20
		details.WriteString("- Valid content type (text/plain)\n")
	}

	// Check for common llms.txt patterns
	llmsIndicators := []struct {
		pattern     string
		description string
		weight      int
	}{
		{"# ", "Comment/section headers", 10},
		{"title:", "Title field", 20},
		{"description:", "Description field", 20},
		{"url:", "URL field", 15},
		{"preferred-languages:", "Preferred languages", 15},
		{"allow:", "Allow directive", 15},
		{"disallow:", "Disallow directive", 15},
		{"instructions:", "Instructions field", 20},
		{"context:", "Context field", 15},
		{"llm", "LLM-related content", 10},
		{"ai ", "AI-related content", 10},
		{"model", "Model-related content", 10},
	}

	for _, indicator := range llmsIndicators {
		if strings.Contains(strings.ToLower(bodyStr), strings.ToLower(indicator.pattern)) {
			confidence += indicator.weight
			details.WriteString(fmt.Sprintf("- Contains: %s\n", indicator.description))
		}
	}

	// Check for structured format (markdown-like)
	if strings.Contains(bodyStr, "##") || strings.Contains(bodyStr, "---") {
		confidence += 10
		details.WriteString("- Uses structured markdown format\n")
	}

	// Minimum content length check
	if len(bodyStr) > 50 {
		confidence += 10
	}

	details.WriteString("\nThis file provides instructions for AI language models interacting with this website.\n")

	return confidence >= minConfidence(), details.String(), min(confidence, 100)
}

// DiscoverAICrawler discovers AI crawler instruction files
func DiscoverAICrawler(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:                    options.BaseURL,
			Method:                 "GET",
			Paths:                  AICrawlerPaths,
			Concurrency:            5,
			Timeout:                DefaultTimeout,
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsLlmsTxtValidationFunc,
		IssueCode:      db.LlmsTxtDetectedCode,
	})
}
