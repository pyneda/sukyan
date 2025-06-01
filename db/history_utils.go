package db

import (
	"net/url"
	"strings"

	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// Update History item based on viper configuration
func enhanceHistoryItem(record *History) {
	ignoredExtensions := viper.GetStringSlice("history.responses.ignored.extensions")
	ignoredContentTypes := viper.GetStringSlice("history.responses.ignored.content_types")
	maxSize := viper.GetInt("history.responses.ignored.max_size")

	if record.RequestBodySize == 0 {
		requestBody, err := record.RequestBody()
		if err == nil {
			record.RequestBodySize = len(requestBody)
		}
	}

	if record.ResponseBodySize == 0 {
		responseBody, err := record.ResponseBody()
		if err == nil {
			record.ResponseBodySize = len(responseBody)
		}
	}

	if record.ParametersCount == 0 || record.CleanURL == "" {
		parsedUrl, err := url.Parse(record.URL)
		if err != nil {
			log.Error().Str("url", record.URL).Err(err).Msg("Error parsing URL")
		} else {
			parameters := parsedUrl.Query()
			if record.ParametersCount == 0 {
				record.ParametersCount = len(parameters)
			}

			if record.CleanURL == "" {
				parsedUrl.RawQuery = ""
				parsedUrl.Fragment = ""
				cleanURL := parsedUrl.String()

				// Truncate CleanURL if it exceeds PostgreSQL btree index limit
				const maxCleanURLLength = 2000
				if len(cleanURL) > maxCleanURLLength {
					record.CleanURL = cleanURL[:maxCleanURLLength]
					if record.Note != "" {
						record.Note += "\n"
					}
					record.Note += "CleanURL has been truncated due to length exceeding database index limit"
					log.Debug().Str("url", record.URL).Int("original_length", len(cleanURL)).Msg("CleanURL truncated")
				} else {
					record.CleanURL = cleanURL
				}
			}
		}
	}

	shouldRemoveBody := false
	reason := ""

	for _, extension := range ignoredExtensions {
		if strings.HasSuffix(record.URL, extension) {
			shouldRemoveBody = true
			reason = "Response body was removed due to ignored file extension: " + extension
			break
		}
	}

	if !shouldRemoveBody {
		for _, contentType := range ignoredContentTypes {
			if strings.Contains(record.ResponseContentType, contentType) {
				shouldRemoveBody = true
				reason = "Response body was removed due to ignored content type: " + contentType
				break
			}
		}
	}

	if !shouldRemoveBody && maxSize > 0 && record.ResponseBodySize > maxSize {
		shouldRemoveBody = true
		reason = "Response body was removed due to exceeding max size limit."
	}

	if shouldRemoveBody {
		headers, _, err := lib.SplitHTTPMessage(record.RawResponse)
		if err == nil {
			record.RawResponse = append(headers, []byte("\r\n\r\n")...)
			record.Note = reason
			log.Debug().Uint("id", record.ID).Str("url", record.URL).Msg(reason)
		}
	}
}
