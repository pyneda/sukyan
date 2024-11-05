package manual

import (
	"bytes"
	"time"

	"github.com/projectdiscovery/rawhttp"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/sourcegraph/conc/pool"

	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/rs/zerolog/log"
)

type RequestFuzzOptions struct {
	URL             string                 `json:"url" validate:"required"`
	Raw             string                 `json:"raw" validate:"required"`
	InsertionPoints []FuzzerInsertionPoint `json:"insertion_points" validate:"required"`
	Session         db.PlaygroundSession   `json:"session" validate:"required"`
	Options         RequestOptions         `json:"options"`
	// MaxConnections     int                    `json:"max_connections"`
	// MaxPendingRequests int                    `json:"max_pending_requests"`
}

type FuzzerPayloadsGroup struct {
	Payloads   []string `json:"payloads"`
	Type       string   `json:"type"`
	Processors []string `json:"processors,omitempty" validate:"omitempty,dive,oneof=base64encode base64decode urlencode urldecode sha1hash sha256hash md5hash" example:"base64encode"`
	Wordlist   string   `json:"wordlist,omitempty"`
}

type FuzzerInsertionPoint struct {
	Start         int                   `json:"start"`
	End           int                   `json:"end"`
	OriginalValue string                `json:"originalValue"`
	PayloadGroups []FuzzerPayloadsGroup `json:"payloadGroups"`
}

func (p *FuzzerInsertionPoint) generatePayloads() []string {
	payloads := make([]string, 0)
	for _, group := range p.PayloadGroups {
		if len(group.Payloads) > 0 {
			if group.Processors != nil {
				processors := make([]lib.StringProcessor, 0)
				for _, processor := range group.Processors {
					processors = append(processors, lib.StringProcessor{Type: lib.StringOperation(processor)})
				}
				for _, payload := range group.Payloads {
					processedPayload, err := lib.ProcessString(payload, processors)
					if err != nil {
						log.Error().Err(err).Str("payload", payload).Interface("processors", processors).Msg("Error processing payload")
					} else {
						payloads = append(payloads, processedPayload)
					}
				}
			} else {
				payloads = append(payloads, group.Payloads...)
			}
		}
		if group.Wordlist != "" {
			storage := NewFilesystemWordlistStorage()
			wordlist, err := storage.GetWordlistByID(group.Wordlist)
			if err != nil {
				log.Error().Err(err).Str("wordlist", group.Wordlist).Msg("Error getting wordlist")
			} else {
				lines, err := storage.ReadWordlist(wordlist.Name, 0)
				if err != nil {
					log.Error().Err(err).Interface("wordlist", wordlist).Msg("Error reading wordlist")
				} else {
					if group.Processors != nil {
						processors := make([]lib.StringProcessor, 0)
						for _, processor := range group.Processors {
							processors = append(processors, lib.StringProcessor{Type: lib.StringOperation(processor)})
						}
						for _, line := range lines {
							processedLine, err := lib.ProcessString(line, processors)
							if err != nil {
								log.Error().Err(err).Str("wordlist", group.Wordlist).Str("payload", line).Interface("processors", processors).Msg("Error processing payload")
							} else {
								payloads = append(payloads, processedLine)
							}
						}
					} else {
						payloads = append(payloads, lines...)
					}
				}
			}
		}
	}
	if len(payloads) == 0 {
		log.Warn().Interface("insertion_point", p).Msg("No payloads generated for insertion point")
	}

	return payloads
}

func replacePayloadsInRaw(raw string, points []FuzzerInsertionPoint, payloads []string) string {
	offset := 0
	for i, point := range points {
		raw = raw[:point.Start+offset] + payloads[i] + raw[point.End+offset:]
		offset += len(payloads[i]) - len(point.OriginalValue)
	}
	return raw
}

func Fuzz(input RequestFuzzOptions, taskID uint) (int, error) {
	parsedUrl, err := url.Parse(input.URL)
	if err != nil {
		return 0, err
	}
	// https://github.com/projectdiscovery/rawhttp/blob/acd587a6157ef709f2fb6ba25866bfffc28b7594/pipelineoptions.go#L20C5-L20C27
	pipeOptions := rawhttp.PipelineOptions{
		Host:                parsedUrl.Host,
		Timeout:             30 * time.Second,
		MaxConnections:      5,
		MaxPendingRequests:  100,
		AutomaticHostHeader: input.Options.UpdateHostHeader,
	}
	if input.Options.Timeout > 0 {
		pipeOptions.Timeout = time.Duration(input.Options.Timeout) * time.Second
	}

	pipeClient := rawhttp.NewPipelineClient(pipeOptions)
	// NOTE: Concurrency should be provided as option. Same as other pipeline options.
	p := pool.New().WithMaxGoroutines(30)
	scheduledRequests := 0

	// Determine the smallest payload set
	smallestPayloadSetSize := len(input.InsertionPoints[0].generatePayloads())
	for _, point := range input.InsertionPoints {
		if len(point.generatePayloads()) < smallestPayloadSetSize {
			smallestPayloadSetSize = len(point.generatePayloads())
		}
	}
	historyOptions := http_utils.HistoryCreationOptions{
		Source:              db.SourceFuzzer,
		WorkspaceID:         input.Session.WorkspaceID,
		TaskID:              taskID,
		CreateNewBodyStream: true,
		PlaygroundSessionID: input.Session.ID,
	}
	// Generate and send fuzzed requests
	for i := 0; i < smallestPayloadSetSize; i++ {
		payloadsForThisRequest := make([]string, len(input.InsertionPoints))
		for j, point := range input.InsertionPoints {
			allPayloads := point.generatePayloads()
			payloadsForThisRequest[j] = allPayloads[i]
		}
		p.Go(func() {
			fuzzedRawRequest := replacePayloadsInRaw(input.Raw, input.InsertionPoints, payloadsForThisRequest)
			log.Info().Msgf("Fuzzed request: %s", fuzzedRawRequest)
			parsedRequest, err := ParseRawRequest(fuzzedRawRequest, input.URL)
			if err != nil {
				log.Error().Err(err).Msg("Error parsing fuzzed request")
				return
			}
			log.Info().Interface("parsedRequest", parsedRequest).Msg("Parsed fuzzed request")
			bodyReader := bytes.NewReader([]byte(parsedRequest.Body))
			response, err := pipeClient.DoRaw(parsedRequest.Method, parsedRequest.URL, parsedRequest.URI, parsedRequest.Headers, bodyReader)
			if err != nil {
				log.Error().Err(err).Msg("Error sending fuzzed request")
				return
			}
			// NOTE: rawhttp doesn't set the http.Response.Request field, so we need to do it manually

			reqUrl, err := url.Parse(parsedRequest.URL + parsedRequest.URI)
			if err != nil {
				reqUrl = parsedUrl
			}

			response.Request = &http.Request{
				Method: parsedRequest.Method,
				URL:    reqUrl,
				Header: parsedRequest.Headers,
				Body:   ioutil.NopCloser(bytes.NewReader([]byte(parsedRequest.Body))),
			}

			history, err := http_utils.ReadHttpResponseAndCreateHistory(response, historyOptions)
			if err != nil {
				log.Error().Err(err).Msg("Error creating history from fuzzed response")
			}
			log.Info().Uint("historyID", history.ID).Msg("Created history from fuzzed response")
		})
		scheduledRequests++
	}

	// p.Wait()
	return scheduledRequests, nil
}
