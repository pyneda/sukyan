package scan

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/scan/reflection"
	"github.com/rs/zerolog/log"
)

type responseFingerprint struct {
	statusCode     int
	bodyLength     int
	bodyWordsCount int
}

type InsertionPointAnalysisOptions struct {
	HistoryCreateOptions http_utils.HistoryCreationOptions

	// ReflectionAnalysis enables reflection analysis including
	// context detection and character efficiency testing
	ReflectionAnalysis bool

	// TestCharacterEfficiencies enables per-character encoding analysis
	// Only used when ReflectionAnalysis is true
	TestCharacterEfficiencies bool
}

func GetAndAnalyzeInsertionPoints(item *db.History, scoped []string, options InsertionPointAnalysisOptions) ([]InsertionPoint, error) {
	insertionPoints, err := GetInsertionPoints(item, scoped)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get insertion points")
		return insertionPoints, err
	}
	return AnalyzeInsertionPoints(item, insertionPoints, options), nil
}

// AnalyzeInsertionPoints analyzes insertion points for reflection and dynamic behavior.
// When ReflectionAnalysis option is enabled, it performs context detection
// and character efficiency testing to support context-aware payload selection.
func AnalyzeInsertionPoints(item *db.History, insertionPoints []InsertionPoint, options InsertionPointAnalysisOptions) []InsertionPoint {
	client := http_utils.CreateHttpClient()
	seenDataTypes := make(map[lib.DataType]bool)
	seenResponseFingerprints := make(map[responseFingerprint]int)

	originalBody, _ := item.ResponseBody()

	originalFingerprint := responseFingerprint{
		statusCode:     item.StatusCode,
		bodyLength:     len(originalBody),
		bodyWordsCount: len(strings.Fields(string(originalBody))),
	}

	for i := range insertionPoints {
		insertionPoint := &insertionPoints[i]
		originalDataType := lib.GuessDataType(insertionPoint.OriginalData)
		seenDataTypes[originalDataType] = true

		if options.ReflectionAnalysis {
			analysis, err := reflection.AnalyzeReflection(
				item,
				reflection.InsertionPointInfo{
					Name:         insertionPoint.Name,
					Type:         string(insertionPoint.Type),
					OriginalData: insertionPoint.OriginalData,
				},
				reflection.AnalysisOptions{
					TestCharacterEfficiencies: options.TestCharacterEfficiencies,
					DetectBadContexts:         true,
					Client:                    client,
					HistoryCreationOptions:    options.HistoryCreateOptions,
				},
			)
			if err != nil {
				log.Debug().Err(err).Str("insertionPoint", insertionPoint.Name).Msg("Failed to analyze reflection")
			} else {
				insertionPoint.Behaviour.ReflectionAnalysis = analysis
				insertionPoint.Behaviour.IsReflected = analysis.IsReflected

				// Populate ReflectionContexts for backwards compatibility
				if analysis.IsReflected {
					for _, ctx := range analysis.Contexts {
						insertionPoint.Behaviour.ReflectionContexts = append(
							insertionPoint.Behaviour.ReflectionContexts,
							ctx.String(),
						)
					}
				}
			}
		} else {
			// Legacy reflection check (simple canary-based)
			payload := lib.GenerateRandomLowercaseString(6)
			h, fg, err := insertionPointCheck(item, insertionPoint, payload, client, options)
			seenResponseFingerprints[fg]++
			if fg != originalFingerprint && fg.statusCode > 0 {
				insertionPoint.Behaviour.IsDynamic = true
				log.Info().Str("insertionPoint", insertionPoint.Name).Msg("Dynamic insertion point detected")
			}
			if err != nil {
				log.Error().Err(err).Msg("Failed to check insertion point")
			} else if h != nil {
				responseBody, _ := h.ResponseBody()
				if strings.Contains(string(responseBody), payload) {
					insertionPoint.Behaviour.IsReflected = true
				}
			}
		}

		// Dynamic behavior detection (runs regardless of ReflectionAnalysis mode)
		if !insertionPoint.Behaviour.IsDynamic {
			basicPayloads := []string{
				fmt.Sprint(lib.GenerateRandInt(4, 10)),
				fmt.Sprint(lib.GenerateRandInt(1000, 10000)),
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

	executionResult := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		Client:                 httpClient,
		CreateHistory:          true,
		HistoryCreationOptions: options.HistoryCreateOptions,
	})
	if executionResult.Err != nil {
		log.Error().Err(executionResult.Err).Msg("Failed to send request")
		return nil, responseFingerprint{}, executionResult.Err
	}

	history := executionResult.History
	historyBody, _ := history.ResponseBody()
	fg := responseFingerprint{
		statusCode:     history.StatusCode,
		bodyLength:     len(historyBody),
		bodyWordsCount: len(strings.Fields(string(historyBody))),
	}
	return history, fg, nil
}
