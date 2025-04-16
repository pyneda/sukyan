package scan

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/url"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
)

type InsertionPointType string

const (
	InsertionPointTypeParameter InsertionPointType = "parameter"
	InsertionPointTypeHeader    InsertionPointType = "header"
	InsertionPointTypeBody      InsertionPointType = "body"
	InsertionPointTypeCookie    InsertionPointType = "cookie"
	InsertionPointTypeURLPath   InsertionPointType = "urlpath"
	InsertionPointTypeFullBody  InsertionPointType = "fullbody"
)

type InsertionPoint struct {
	Type         InsertionPointType
	Name         string       // the name of the parameter/header/cookie
	Value        string       // the current value
	ValueType    lib.DataType // the type of the value (string, int, float, etc.)
	OriginalData string       // the original data (URL, header string, body, cookie string) in which this insertion point was found
	Behaviour    InsertionPointBehaviour
}

type InsertionPointBehaviour struct {
	// AcceptedDataTypes []lib.DataType
	IsReflected        bool
	ReflectionContexts []string
	IsDynamic          bool
	// Transformations   []Transformation
}

type Transformation struct {
	From         string
	FromDatatype lib.DataType
	To           string
	ToDatatype   lib.DataType
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
				Type:         "parameter",
				Name:         name,
				Value:        value,
				ValueType:    lib.GuessDataType(value),
				OriginalData: urlData.String(),
			})
		}
	}

	return points, nil
}

// Handle URL paths
func handleURLPaths(urlData *url.URL) ([]InsertionPoint, error) {
	var points []InsertionPoint

	// URL parameters
	for _, pathPart := range strings.Split(urlData.Path, "/") {
		if pathPart == "" {
			continue
		}
		points = append(points, InsertionPoint{
			Type:         InsertionPointTypeURLPath,
			Name:         pathPart,
			Value:        pathPart,
			ValueType:    lib.GuessDataType(pathPart),
			OriginalData: urlData.String(),
		})
	}

	return points, nil
}

// Handle Headers
func handleHeaders(header map[string][]string) ([]InsertionPoint, error) {
	var points []InsertionPoint
	for name, values := range header {
		if name == "cookie" {
			continue
		}
		for _, value := range values {
			points = append(points, InsertionPoint{
				Type:      InsertionPointTypeHeader,
				Name:      name,
				Value:     value,
				ValueType: lib.GuessDataType(value),

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
						Type:      InsertionPointTypeCookie,
						Name:      cookieParts[0],
						Value:     cookieParts[1],
						ValueType: lib.GuessDataType(cookieParts[1]),

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
					Type:      InsertionPointTypeBody,
					Name:      name,
					Value:     value,
					ValueType: lib.GuessDataType(value),

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
			valueStr := fmt.Sprintf("%v", value)
			points = append(points, InsertionPoint{
				Type:      InsertionPointTypeBody,
				Name:      name,
				Value:     valueStr,
				ValueType: lib.GuessDataType(valueStr),

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
			valueStr := fmt.Sprintf("%v", value)

			points = append(points, InsertionPoint{
				Type:      InsertionPointTypeBody,
				Name:      name,
				Value:     valueStr,
				ValueType: lib.GuessDataType(valueStr),

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
					Type:      InsertionPointTypeBody,
					Name:      name,
					Value:     value,
					ValueType: lib.GuessDataType(value),

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
	if lib.SliceContains(scoped, "parameters") {
		urlPoints, err := handleURLParameters(urlData)
		if err != nil {
			return nil, err
		}
		points = append(points, urlPoints...)
	}

	if lib.SliceContains(scoped, "urlpath") {
		urlPathPoints, err := handleURLPaths(urlData)
		if err != nil {
			return nil, err
		}
		points = append(points, urlPathPoints...)
	}

	headers, err := history.RequestHeaders()
	if err != nil {
		log.Error().Err(err).Str("headers", "failed to parse").Msg("Error getting request headers as map")
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
	body, _ := history.RequestBody()
	bodyStr := string(body)

	bodyPoints, err := handleBodyParameters(history.RequestContentType, body)
	if err != nil {
		return nil, err
	}
	points = append(points, bodyPoints...)
	if len(bodyPoints) > 0 {
		points = append(points, InsertionPoint{
			Type:         InsertionPointTypeFullBody,
			Name:         "fullbody",
			Value:        bodyStr,
			ValueType:    lib.GuessDataType(bodyStr),
			OriginalData: bodyStr,
		})
	}

	return points, nil
}
