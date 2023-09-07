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
	Occurrences map[string]*HeaderData
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

func getOccurrencesReport(data map[string]*HeaderData) string {
	var commonHeaders, uncommonHeaders []string

	// Sort keys by Count
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return data[keys[i]].Count > data[keys[j]].Count
	})

	for _, key := range keys {
		headerData := data[key]
		headerInfo := fmt.Sprintf("  * %s (Count: %d)\n", key, headerData.Count)
		for _, value := range headerData.Values {
			headerInfo += fmt.Sprintf("      - %s\n", value)
		}

		if headerData.UncommonHeader {
			uncommonHeaders = append(uncommonHeaders, headerInfo)
		} else {
			commonHeaders = append(commonHeaders, headerInfo)
		}
	}

	var reportBuilder strings.Builder

	reportBuilder.WriteString("Find below a list of headers found in the responses of the target application during the crawl phase.\n\n")

	reportBuilder.WriteString("Uncommon Headers:\n\n")
	reportBuilder.WriteString("--------------------------------\n\n")
	reportBuilder.WriteString(strings.Join(uncommonHeaders, "\n"))

	reportBuilder.WriteString("Common Headers:\n")
	reportBuilder.WriteString("--------------------------------\n\n")
	reportBuilder.WriteString(strings.Join(commonHeaders, "\n"))

	return reportBuilder.String()
}

func getHeadersOccurrences(histories []*db.History) map[string]*HeaderData {
	foundHeaders := make(map[string]*HeaderData)

	for _, item := range histories {
		headers, err := item.GetResponseHeadersAsMap()
		if err != nil {
			log.Error().Err(err).Msg("Failed to get response headers as map")
			continue
		}

		for key, values := range headers {
			headerData, exists := foundHeaders[key]
			if !exists {
				isCommon := http_utils.IsCommonHTTPHeader(key)
				headerData = &HeaderData{
					Count:          0,
					Values:         make([]string, 0),
					UncommonHeader: !isCommon,
				}
				foundHeaders[key] = headerData
			}

			for _, value := range values {
				addValueToHeaderData(headerData, value)
			}
		}
	}

	return foundHeaders
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
