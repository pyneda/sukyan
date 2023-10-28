package manual

import (
	"bytes"
	"github.com/projectdiscovery/rawhttp"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
	"io/ioutil"
	"net/http"
	"net/url"
)

type RequestFuzzOptions struct {
	URL             string                 `json:"url" validate:"required"`
	Raw             string                 `json:"raw" validate:"required"`
	InsertionPoints []FuzzerInsertionPoint `json:"insertion_points" validate:"required"`
	Session         db.PlaygroundSession   `json:"session" validate:"required"`
	Options         RequestOptions         `json:"options"`
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
		payloads = append(payloads, group.Payloads...)
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

func Fuzz(input RequestFuzzOptions, taskID uint) error {
	parsedUrl, err := url.Parse(input.URL)
	if err != nil {
		return err
	}
	// https://github.com/projectdiscovery/rawhttp/blob/acd587a6157ef709f2fb6ba25866bfffc28b7594/pipelineoptions.go#L20C5-L20C27
	pipeOptions := rawhttp.DefaultPipelineOptions
	pipeOptions.Host = parsedUrl.Host
	pipeOptions.AutomaticHostHeader = false
	pipeClient := rawhttp.NewPipelineClient(pipeOptions)

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

		fuzzedRawRequest := replacePayloadsInRaw(input.Raw, input.InsertionPoints, payloadsForThisRequest)
		log.Info().Msgf("Fuzzed request: %s", fuzzedRawRequest)
		parsedRequest, err := ParseRawRequest(fuzzedRawRequest, input.URL)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing fuzzed request")
			continue
		}
		log.Info().Interface("parsedRequest", parsedRequest).Msg("Parsed fuzzed request")
		bodyReader := bytes.NewReader([]byte(parsedRequest.Body))
		response, err := pipeClient.DoRaw(parsedRequest.Method, parsedRequest.URL, parsedRequest.URI, parsedRequest.Headers, bodyReader)
		if err != nil {
			log.Error().Err(err).Msg("Error sending fuzzed request")
			continue
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

		_, err = http_utils.ReadHttpResponseAndCreateHistory(response, historyOptions)
		if err != nil {
			log.Error().Err(err).Msg("Error creating history from fuzzed response")
		}
	}

	return nil
}
