package scan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"
)

type InsertionPointBuilder struct {
	Point   InsertionPoint
	Payload string
}

func createRequestFromURLParameter(history *db.History, builder InsertionPointBuilder) (string, error) {
	return lib.BuildURLWithParam(history.URL, builder.Point.Name, builder.Payload, false)
}

func createRequestFromURLPath(history *db.History, builder InsertionPointBuilder) (string, error) {
	initialUrl, err := url.Parse(history.URL)
	if err != nil {
		return "", err
	}
	pathParts := strings.Split(initialUrl.Path, "/")
	for i, part := range pathParts {
		if part == builder.Point.Name {
			pathParts[i] = builder.Payload
		}
	}
	initialUrl.Path = strings.Join(pathParts, "/")
	return initialUrl.String(), nil
}

func createRequestFromHeader(history *db.History, builder InsertionPointBuilder) (http.Header, error) {
	headers, err := history.GetRequestHeadersAsMap()
	if err != nil {
		return nil, err
	}
	headers[builder.Point.Name] = []string{builder.Payload}
	return headers, nil
}

func createRequestFromCookie(history *db.History, builder InsertionPointBuilder) (http.Header, error) {
	headers, err := history.GetRequestHeadersAsMap()
	if err != nil {
		return nil, err
	}

	existingCookies := headers["Cookie"]
	updatedCookies := make([]string, len(existingCookies))

	for i, cookieStr := range existingCookies {
		cookies := http_utils.ParseCookies(cookieStr)
		for _, cookie := range cookies {
			if cookie.Name == builder.Point.Name {
				cookie.Value = builder.Payload
			}
		}
		updatedCookies[i] = http_utils.JoinCookies(cookies)
	}

	headers["Cookie"] = updatedCookies
	return headers, nil
}

func createRequestFromBody(history *db.History, builders []InsertionPointBuilder) (io.Reader, string, error) {
	body, err := history.RequestBody()
	if err != nil {
		return nil, "", err
	}
	switch {
	case strings.Contains(history.RequestContentType, "application/x-www-form-urlencoded"):

		values, err := url.ParseQuery(string(body))
		if err != nil {
			log.Error().Err(err).Uint("history_id", history.ID).Msg("Error parsing form data creating scan request from body")
			return nil, "", err
		}
		for _, builder := range builders {
			values.Set(builder.Point.Name, builder.Payload)
		}
		return strings.NewReader(values.Encode()), "application/x-www-form-urlencoded", nil
	case strings.Contains(history.RequestContentType, "application/json"):
		var graphQLVarBuilders, graphQLInlineBuilders, flatBodyBuilders []InsertionPointBuilder
		for _, b := range builders {
			switch b.Point.Type {
			case InsertionPointTypeGraphQLVariable:
				graphQLVarBuilders = append(graphQLVarBuilders, b)
			case InsertionPointTypeGraphQLInlineArg:
				graphQLInlineBuilders = append(graphQLInlineBuilders, b)
			default:
				flatBodyBuilders = append(flatBodyBuilders, b)
			}
		}

		modified := body
		if len(graphQLVarBuilders) > 0 {
			var err error
			modified, err = modifyGraphQLVariables(modified, graphQLVarBuilders)
			if err != nil {
				return nil, "", err
			}
		}
		if len(graphQLInlineBuilders) > 0 {
			var err error
			modified, err = modifyGraphQLInlineArg(modified, graphQLInlineBuilders)
			if err != nil {
				return nil, "", err
			}
		}
		if len(flatBodyBuilders) > 0 {
			var requestBody map[string]interface{}
			if err := json.Unmarshal(modified, &requestBody); err != nil {
				return nil, "", err
			}
			for _, builder := range flatBodyBuilders {
				requestBody[builder.Point.Name] = builder.Payload
			}
			var err error
			modified, err = json.Marshal(requestBody)
			if err != nil {
				return nil, "", err
			}
		}
		return strings.NewReader(string(modified)), "application/json", nil
	case strings.Contains(history.RequestContentType, "multipart/form-data"):
		var b bytes.Buffer
		writer := multipart.NewWriter(&b)
		for _, builder := range builders {
			if _, _, err := createMultipartForm(history, builder, &b, writer); err != nil {
				return nil, "", err
			}
		}
		writer.Close()
		return &b, writer.FormDataContentType(), nil
	default:
		// TODO: Support other content types
		return nil, "", errors.New("unsupported Content-Type for body")
	}
}

func createMultipartForm(history *db.History, builder InsertionPointBuilder, b *bytes.Buffer, writer *multipart.Writer) (io.Reader, string, error) {
	_, params, err := mime.ParseMediaType(history.RequestContentType)
	if err != nil {
		return nil, "", err
	}
	boundary, ok := params["boundary"]
	if !ok {
		return nil, "", errors.New("invalid Content-Type, boundary not found")
	}

	body, err := history.RequestBody()
	if err != nil {
		return nil, "", err
	}

	reader := multipart.NewReader(strings.NewReader(string(body)), boundary)
	form, err := reader.ReadForm(10 << 20) // Max memory 10 MB
	if err != nil {
		return nil, "", err
	}

	// Iterate over form.Value and form.File
	for name, values := range form.Value {
		if name == builder.Point.Name {
			values[0] = builder.Payload // Replace the value at the insertion point with the payload
		}
		for _, value := range values {
			writer.WriteField(name, value)
		}
	}
	for _, files := range form.File {
		for _, file := range files {
			part, err := writer.CreatePart(textproto.MIMEHeader(file.Header))
			if err != nil {
				return nil, "", err
			}
			f, err := file.Open()
			if err != nil {
				return nil, "", err
			}
			io.Copy(part, f)
			f.Close()
		}
	}

	return b, writer.FormDataContentType(), nil
}

func CreateRequestFromInsertionPoints(history *db.History, builders []InsertionPointBuilder) (*http.Request, error) {
	return createRequestFromInsertionPointsInternal(history, builders, false)
}

func createRequestFromInsertionPointsInternal(history *db.History, builders []InsertionPointBuilder, alreadyRetried bool) (*http.Request, error) {
	var urlStr string
	headers := make(http.Header)
	var requestBody io.Reader
	var contentType string
	var err error
	var bodyBuilders []InsertionPointBuilder

	for _, builder := range builders {
		switch builder.Point.Type {
		case InsertionPointTypeParameter:
			urlStr, err = createRequestFromURLParameter(history, builder)
			if err != nil {
				return nil, err
			}

		case InsertionPointTypeURLPath:
			urlStr, err = createRequestFromURLPath(history, builder)
			if err != nil {
				return nil, err
			}

		case InsertionPointTypeHeader:
			h, err := createRequestFromHeader(history, builder)
			if err != nil {
				return nil, err
			}
			for name, values := range h {
				headers[name] = values
			}
		case InsertionPointTypeCookie:
			h, err := createRequestFromCookie(history, builder)
			if err != nil {
				return nil, err
			}
			for name, values := range h {
				headers[name] = values
			}
		case InsertionPointTypeBody, InsertionPointTypeGraphQLVariable, InsertionPointTypeGraphQLInlineArg:
			bodyBuilders = append(bodyBuilders, builder)
		// case InsertionPointTypeFullBody:
		// 	requestBody = strings.NewReader(builder.Payload)
		case InsertionPointTypeFullBody:
			requestBody = strings.NewReader(builder.Payload)

		default:
			return nil, fmt.Errorf("unsupported insertion point type: %s", builder.Point.Type)
		}
	}

	if requestBody == nil {
		requestBody, contentType, _ = createRequestFromBody(history, bodyBuilders)
	}

	// if err != nil {
	// 	return nil, err
	// }
	if urlStr == "" {
		urlStr = history.URL
	}
	// if len(headers) == 0 {
	// 	h, err := history.GetRequestHeadersAsMap()
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	for name, values := range h {
	// 		headers[name] = values
	// 	}
	// }

	req, err := http.NewRequest(history.Method, urlStr, requestBody)
	if err != nil {
		if !alreadyRetried && strings.Contains(err.Error(), "invalid control character in URL") {
			// Retry with URL encoding for parameter insertion points only
			encodedBuilders := make([]InsertionPointBuilder, len(builders))
			copy(encodedBuilders, builders)

			hasParameterPayloads := false
			for i, builder := range encodedBuilders {
				if builder.Point.Type == InsertionPointTypeParameter {
					encodedBuilders[i].Payload = url.QueryEscape(builder.Payload)
					hasParameterPayloads = true
				}
			}

			if hasParameterPayloads {
				return createRequestFromInsertionPointsInternal(history, encodedBuilders, true)
			}
		}
		return nil, err
	}

	// Set the same requests as the history item had, before possibly overriding by insertion points
	http_utils.SetRequestHeadersFromHistoryItem(req, history)

	for name, values := range headers {
		if name == "Content-Length" || name == "content-length" {
			continue
		}
		req.Header[name] = values
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return req, nil
}
