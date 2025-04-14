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

	if record.ParametersCount == 0 {
		parsedUrl, err := url.Parse(record.URL)
		if err != nil {
			log.Error().Str("url", record.URL).Err(err).Msg("Error parsing URL")
		} else {
			parameters := parsedUrl.Query()
			record.ParametersCount = len(parameters)
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
