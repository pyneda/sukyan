package fuzz

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
)

type InsertionPointBuilder struct {
	Point   InsertionPoint
	Payload string
}

func createRequestFromURL(history *db.History, builder InsertionPointBuilder) (string, error) {
	parsedURL, err := url.Parse(history.URL)
	if err != nil {
		return "", err
	}
	query := parsedURL.Query()
	query.Set(builder.Point.Name, builder.Payload)
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
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
	switch {
	case strings.Contains(history.RequestContentType, "application/x-www-form-urlencoded"):
		values, err := url.ParseQuery(string(history.RequestBody))
		if err != nil {
			return nil, "", err
		}
		for _, builder := range builders {
			values.Set(builder.Point.Name, builder.Payload)
		}
		return strings.NewReader(values.Encode()), "application/x-www-form-urlencoded", nil
	case strings.Contains(history.RequestContentType, "application/json"):
		var requestBody map[string]interface{}
		if err := json.Unmarshal(history.RequestBody, &requestBody); err != nil {
			return nil, "", err
		}
		for _, builder := range builders {
			requestBody[builder.Point.Name] = builder.Payload
		}
		jsonPayload, err := json.Marshal(requestBody)
		if err != nil {
			return nil, "", err
		}
		return strings.NewReader(string(jsonPayload)), "application/json", nil
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

	reader := multipart.NewReader(strings.NewReader(string(history.RequestBody)), boundary)
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
	var urlStr string
	headers := make(http.Header)
	var requestBody io.Reader
	var contentType string
	var err error
	var bodyBuilders []InsertionPointBuilder

	for _, builder := range builders {
		switch builder.Point.Type {
		case "URL":
			urlStr, err = createRequestFromURL(history, builder)
			if err != nil {
				return nil, err
			}
		case "Header":
			h, err := createRequestFromHeader(history, builder)
			if err != nil {
				return nil, err
			}
			for name, values := range h {
				headers[name] = values
			}
		case "Cookie":
			h, err := createRequestFromCookie(history, builder)
			if err != nil {
				return nil, err
			}
			for name, values := range h {
				headers[name] = values
			}
		case "Body":
			bodyBuilders = append(bodyBuilders, builder)

		default:
			return nil, errors.New("unsupported insertion point type")
		}
	}

	requestBody, contentType, err = createRequestFromBody(history, bodyBuilders)
	if err != nil {
		return nil, err
	}
	if urlStr == "" {
		urlStr = history.URL
	}
	if len(headers) == 0 {
		h, err := history.GetRequestHeadersAsMap()
		if err != nil {
			return nil, err
		}
		for name, values := range h {
			headers[name] = values
		}
	}

	req, err := http.NewRequest(history.Method, urlStr, requestBody)
	if err != nil {
		return nil, err
	}

	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	req.Header.Set("Content-Type", contentType)

	return req, nil
}
