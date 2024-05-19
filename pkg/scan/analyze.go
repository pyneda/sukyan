package scan

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

type responseFingerprint struct {
	statusCode     int
	bodyLength     int
	bodyWordsCount int
}

type InsertionPointAnalysisOptions struct {
	HistoryCreateOptions http_utils.HistoryCreationOptions
}

func GetAndAnalyzeInsertionPoints(item *db.History, scoped []string, options InsertionPointAnalysisOptions) ([]InsertionPoint, error) {
	insertionPoints, err := GetInsertionPoints(item, scoped)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get insertion points")
		return insertionPoints, err
	}
	return AnalyzeInsertionPoints(item, insertionPoints, options), nil
}

// AnalyzeInsertionPoints by now just checks for reflection (which was already done by templates) and checks in a really simple way if an insertion point is dynamic. In a future it should be improved to also analyze different kinds of accepted inputs, transformations and other interesting behaviors
func AnalyzeInsertionPoints(item *db.History, insertionPoints []InsertionPoint, options InsertionPointAnalysisOptions) []InsertionPoint {
	client := http_utils.CreateHttpClient()
	seenDataTypes := make(map[lib.DataType]bool)
	seenResponseFingerprints := make(map[responseFingerprint]int)
	originalFingerprint := responseFingerprint{
		statusCode:     item.StatusCode,
		bodyLength:     len(item.ResponseBody),
		bodyWordsCount: len(strings.Fields(string(item.ResponseBody))),
	}

	for i := range insertionPoints {
		insertionPoint := &insertionPoints[i]
		originalDataType := lib.GuessDataType(insertionPoint.OriginalData)
		seenDataTypes[originalDataType] = true
		payload := lib.GenerateRandomLowercaseString(6)
		h, fg, err := insertionPointCheck(item, insertionPoint, payload, client, options)
		seenResponseFingerprints[fg]++
		if fg != originalFingerprint && fg.statusCode > 0 {
			insertionPoint.Behaviour.IsDynamic = true
		}
		if err != nil {
			log.Error().Err(err).Msg("Failed to check insertion point")
		} else if h != nil {
			// log.Info().Msg("Reflection detected")
			body := string(h.ResponseBody)
			if strings.Contains(body, payload) {
				insertionPoint.Behaviour.IsReflected = true
			}
		}

		if !insertionPoint.Behaviour.IsDynamic {
			basicPayloads := []string{
				fmt.Sprint(lib.GenerateRandInt(10, 10000)),
				"//",
				"null",
				"true",
				"undefined",
				`${{<%[%'"}}%\.\`,
				`:/*!--></>"+`,
				`'-- `,
			}
			for _, p := range basicPayloads {
				_, fg, err := insertionPointCheck(item, insertionPoint, p, client, options)
				seenResponseFingerprints[fg]++
				if fg != originalFingerprint && fg.statusCode > 0 {
					insertionPoint.Behaviour.IsDynamic = true
					// NOTE: Since only checking if it's dynamic and not checking other behaviors, by now here we can just assume it's dynamic and return
					break
				}
				if err != nil {
					continue
				}
			}
		}
	}
	return insertionPoints
}

func insertionPointCheck(item *db.History, insertionPoint *InsertionPoint, payload string, httpClient *http.Client, options InsertionPointAnalysisOptions) (*db.History, responseFingerprint, error) {
	builders := []InsertionPointBuilder{
		{
			Point:   *insertionPoint,
			Payload: payload,
		},
	}
	req, err := CreateRequestFromInsertionPoints(item, builders)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create request from insertion points")
		return nil, responseFingerprint{}, err
	}
	response, err := httpClient.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send request")
		return nil, responseFingerprint{}, err
	}

	history, err := http_utils.ReadHttpResponseAndCreateHistory(response, options.HistoryCreateOptions)
	if err != nil {
		return nil, responseFingerprint{}, err
	}
	fg := responseFingerprint{
		statusCode:     history.StatusCode,
		bodyLength:     len(history.ResponseBody),
		bodyWordsCount: len(strings.Fields(string(history.ResponseBody))),
	}
	return history, fg, nil
}
