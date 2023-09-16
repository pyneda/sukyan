package fuzz

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"mime"
	"mime/multipart"
	"net/url"
	"strings"
)

type InsertionPoint struct {
	Type         string // "Parameter", "Header", "Body", or "Cookie"
	Name         string // the name of the parameter/header/cookie
	Value        string // the current value
	OriginalData string // the original data (URL, header string, body, cookie string) in which this insertion point was found
}

func (i *InsertionPoint) String() string {
	return fmt.Sprintf("%s: %s", i.Type, i.Name)
}

// Handle URL parameters
func handleURLParameters(urlData *url.URL) ([]InsertionPoint, error) {
	var points []InsertionPoint

	// URL parameters
	for name, values := range urlData.Query() {
		for _, value := range values {
			points = append(points, InsertionPoint{
				Type:         "Parameter",
				Name:         name,
				Value:        value,
				OriginalData: urlData.String(),
			})
		}
	}

	return points, nil
}

// Handle Headers
func handleHeaders(header map[string][]string) ([]InsertionPoint, error) {
	var points []InsertionPoint
	for name, values := range header {
		if name == "Cookie" {
			continue
		}
		for _, value := range values {
			points = append(points, InsertionPoint{
				Type:         "Header",
				Name:         name,
				Value:        value,
				OriginalData: header[name][0],
			})
		}
	}

	return points, nil
}

// Handle Cookies
func handleCookies(header map[string][]string) ([]InsertionPoint, error) {
	var points []InsertionPoint

	// Cookies
	if cookies, ok := header["Cookie"]; ok {
		for _, cookieString := range cookies {
			cookieValues := strings.Split(cookieString, ";")
			for _, cookieValue := range cookieValues {
				cookieParts := strings.SplitN(strings.TrimSpace(cookieValue), "=", 2)
				if len(cookieParts) == 2 {
					points = append(points, InsertionPoint{
						Type:         "Cookie",
						Name:         cookieParts[0],
						Value:        cookieParts[1],
						OriginalData: cookieString,
					})
				}
			}
		}
	}

	return points, nil
}

// Handle Body parameters
func handleBodyParameters(contentType string, body []byte) ([]InsertionPoint, error) {
	var points []InsertionPoint

	// URL-encoded body
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		formData, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}

		for name, values := range formData {
			for _, value := range values {
				points = append(points, InsertionPoint{
					Type:         "Body",
					Name:         name,
					Value:        value,
					OriginalData: string(body),
				})
			}
		}
	}

	// JSON body
	if strings.Contains(contentType, "application/json") {
		var jsonData map[string]interface{}
		err := json.Unmarshal(body, &jsonData)
		if err != nil {
			return nil, err
		}

		for name, value := range jsonData {
			points = append(points, InsertionPoint{
				Type:         "Body",
				Name:         name,
				Value:        fmt.Sprintf("%v", value),
				OriginalData: string(body),
			})
		}
	}

	// XML body
	if strings.Contains(contentType, "application/xml") {
		var xmlData map[string]interface{}
		err := xml.Unmarshal(body, &xmlData)
		if err != nil {
			return nil, err
		}

		for name, value := range xmlData {
			points = append(points, InsertionPoint{
				Type:         "Body",
				Name:         name,
				Value:        fmt.Sprintf("%v", value),
				OriginalData: string(body),
			})
		}
	}

	// Multipart form body
	// Multipart form body
	if strings.Contains(contentType, "multipart/form-data") {
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return nil, err
		}
		boundary, ok := params["boundary"]
		if !ok {
			return nil, errors.New("Content-Type does not contain boundary parameter")
		}

		mr := multipart.NewReader(strings.NewReader(string(body)), boundary)
		form, err := mr.ReadForm(10 << 20) // Max memory 10 MB
		if err != nil {
			return nil, err
		}

		for name, values := range form.Value {
			for _, value := range values {
				points = append(points, InsertionPoint{
					Type:         "Body",
					Name:         name,
					Value:        value,
					OriginalData: string(body),
				})
			}
		}
	}

	return points, nil
}

func GetInsertionPoints(history *db.History, scoped []string) ([]InsertionPoint, error) {
	var points []InsertionPoint

	// Analyze URL
	urlData, err := url.Parse(history.URL)
	if err != nil {
		return nil, err
	}
	urlPoints, err := handleURLParameters(urlData)
	if err != nil {
		return nil, err
	}
	points = append(points, urlPoints...)

	// Convert datatypes.JSON to http.Header equivalent
	headers, err := history.GetRequestHeadersAsMap()
	if err != nil {
		log.Error().Err(err).Interface("headers", history.RequestHeaders).Msg("Error getting request headers as map")
	} else {
		if lib.SliceContains(scoped, "headers") {
			// Headers
			headerPoints, err := handleHeaders(headers)
			if err != nil {
				return nil, err
			}
			points = append(points, headerPoints...)
		}

		if lib.SliceContains(scoped, "cookies") {
			// Cookies
			cookiePoints, err := handleCookies(headers)
			if err != nil {
				return nil, err
			}
			points = append(points, cookiePoints...)
		}
	}

	// Body parameters
	bodyPoints, err := handleBodyParameters(history.RequestContentType, history.RequestBody)
	if err != nil {
		return nil, err
	}
	points = append(points, bodyPoints...)

	return points, nil
}
