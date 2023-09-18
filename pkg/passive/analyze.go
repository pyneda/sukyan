package passive

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
	"sort"
	"strings"
)

type HeaderData struct {
	Count          int
	Values         []string
	UncommonHeader bool
}

type HeaderAnalysisResult struct {
	Occurrences map[string]map[string]*HeaderData
	Details     string
	Issue       db.Issue
}

func AnalyzeHeaders(baseURL string, histories []*db.History) HeaderAnalysisResult {
	occurrences := getHeadersOccurrences(histories)
	details := getOccurrencesReport(occurrences)
	issue := db.GetIssueTemplateByCode(db.HeaderInsightsReportCode)
	issue.Details = details
	issue.Confidence = 100
	issue.WorkspaceID = histories[0].WorkspaceID
	issue.URL = baseURL
	created, err := db.Connection.CreateIssue(*issue)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create HeaderInsightsReportCode issue")
	}

	return HeaderAnalysisResult{
		Occurrences: occurrences,
		Details:     details,
		Issue:       created,
	}
}

func getHeadersOccurrences(histories []*db.History) map[string]map[string]*HeaderData {
	foundHeaders := make(map[string]map[string]*HeaderData)

	for _, item := range histories {
		headers, err := item.GetResponseHeadersAsMap()
		if err != nil {
			log.Error().Err(err).Msg("Failed to get response headers as map")
			continue
		}

		for key, values := range headers {
			category := http_utils.ClassifyHTTPResponseHeader(key)

			if foundHeaders[category] == nil {
				foundHeaders[category] = make(map[string]*HeaderData)
			}

			headerData, exists := foundHeaders[category][key]
			if !exists {
				headerData = &HeaderData{
					Count:  0,
					Values: make([]string, 0),
				}
				foundHeaders[category][key] = headerData
			}

			for _, value := range values {
				addValueToHeaderData(headerData, value)
			}
		}
	}

	return foundHeaders
}

func getOccurrencesReport(data map[string]map[string]*HeaderData) string {
	var reportBuilder strings.Builder

	reportBuilder.WriteString("Find below a list of headers found in the responses of the target application during the crawl phase, categorized by their purpose.\n\n")

	for category, headers := range data {
		reportBuilder.WriteString(fmt.Sprintf("%s Headers:\n", category))
		reportBuilder.WriteString("--------------------------------\n\n")

		keys := make([]string, 0, len(headers))
		for k := range headers {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			return headers[keys[i]].Count > headers[keys[j]].Count
		})

		for _, key := range keys {
			headerData := headers[key]
			headerInfo := fmt.Sprintf("  * %s (Count: %d)\n", key, headerData.Count)
			for _, value := range headerData.Values {
				headerInfo += fmt.Sprintf("      - %s\n", value)
			}
			reportBuilder.WriteString(headerInfo)
		}

		reportBuilder.WriteString("\n\n")
	}

	return reportBuilder.String()
}

func addValueToHeaderData(d *HeaderData, value string) {
	d.Count++
	for _, existingValue := range d.Values {
		if existingValue == value {
			return
		}
	}
	d.Values = append(d.Values, value)
}
