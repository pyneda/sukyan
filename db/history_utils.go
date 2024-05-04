package db

import (
	"net/url"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// Update History item based on viper configuration
func enhanceHistoryItem(record *History) {
	ignoredExtensions := viper.GetStringSlice("history.responses.ignored.extensions")
	ignoredContentTypes := viper.GetStringSlice("history.responses.ignored.content_types")
	maxSize := viper.GetInt("history.responses.ignored.max_size")

	// Body sizes
	if record.RequestBodySize == 0 {
		record.RequestBodySize = len(record.RequestBody)
	}

	if record.ResponseBodySize == 0 {
		record.ResponseBodySize = len(record.ResponseBody)
	}

	// Parameters count
	if record.ParametersCount == 0 {
		parsedUrl, err := url.Parse(record.URL)
		if err != nil {
			log.Error().Str("url", record.URL).Err(err).Msg("Error parsing URL")
		} else {
			parameters := parsedUrl.Query()
			record.ParametersCount = len(parameters)
		}
	}

	// Remove response body according to the viper configuration
	for _, extension := range ignoredExtensions {
		if strings.HasSuffix(record.URL, extension) {
			record.ResponseBody = []byte("")
			record.Note = "Response body was removed due to ignored file extension: " + extension
			log.Debug().Interface("history", record).Msg("Response body was removed due to ignored file extension")
			return
		}
	}

	for _, contentType := range ignoredContentTypes {
		if strings.Contains(record.ResponseContentType, contentType) {
			record.ResponseBody = []byte("")
			record.Note = "Response body was removed due to ignored content type: " + contentType
			log.Debug().Interface("history", record).Msg("Response body was removed due to ignored content type")
			return
		}
	}

	if maxSize > 0 && record.ResponseBodySize > maxSize {
		log.Debug().Interface("history", record).Int("size", record.ResponseBodySize).Msg("Response body was removed due to exceeding max size limit.")
		record.ResponseBody = []byte("")
		record.Note = "Response body was removed due to exceeding max size limit."
	}
}
