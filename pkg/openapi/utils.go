package openapi

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

type SwaggerRequest struct {
	ApiKey      string
	ApiKeyName  string
	ApiInQuery  bool
	BasePath    string
	Body        map[string]any
	BodyData    []byte
	Def         *openapi3.T `json:"-"`
	Path        string
	Paths       []string
	Query       url.Values
	RawQuery    string
	ResultsJSON []string
	URL         url.URL
}

type VerboseResult struct {
	Method  string `json:"method"`
	Preview string `json:"preview"`
	Status  int    `json:"status"`
	Target  string `json:"target"`
}

var jsonResultsStringArray []string
var jsonVerboseResultArray []VerboseResult

// To migrate

var basePath string
var apiTarget string
var contentType string

type OpenapiParseInput struct {
	BodyBytes  []byte
	SwaggerURL string
	Format     string
}

func GenerateRequests(input OpenapiParseInput) ([]string, error) {

	var s SwaggerRequest
	def, err := unmarshalSpec(input)
	if err != nil {
		log.Error().Err(err).Msg("Error unmarshalling spec.")
		return nil, err
	}
	s.Def = def

	security := CheckSecDefs(CheckSecDefsInput{
		Doc3: *s.Def,
	})
	s.ApiInQuery = security.ApiInQuery
	s.ApiKey = security.ApiKey
	s.ApiKeyName = security.ApiKeyName

	log.Info().Str("summary", security.HumanReadableSummary).Interface("headers", security.Headers).Msg("Security definitions processed.")
	u, err := url.Parse(input.SwaggerURL)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing swagger URL.")
	}
	s.URL = *u

	if s.Def.Paths == nil {
		log.Warn().Msg("Could not find any defined operations.")
	}

	if len(s.Def.Servers) > 1 {
		// if !quiet && (os.Args[1] != "endpoints") {
		// 	if apiTarget == "" {
		// 		log.Warn("Multiple servers detected in documentation. You can manually set a server to test with the -T flag.\nThe detected servers are as follows:")
		// 		for i, server := range s.Def.Servers {
		// 			fmt.Printf("Server %d: %s\n", i+1, server.URL)
		// 		}
		// 		fmt.Println()
		// 	}
		// }
		if apiTarget == "" {
			for _, server := range s.Def.Servers {
				log.Info().Str("server", server.URL).Msg("Checking server.")

				if server.URL == "/" {
					s.Path = ""
				}

				if strings.Contains(server.URL, "localhost") || strings.Contains(server.URL, "127.0.0.1") || strings.Contains(server.URL, "::1") {
					log.Warn().Msg("The server(s) documented in the definition file contain(s) a local host value and may result in errors. Supply a target manually using the '-T' flag.")
				}

				u, _ := url.Parse(server.URL)
				s.URL = *u
				s = s.IterateOverPaths()
				fmt.Println()
			}
		} else {
			if apiTarget != "" {
				u, _ = url.Parse(apiTarget)
			}
			s.URL = *u
			s = s.IterateOverPaths()
		}
	} else {
		if apiTarget != "" {
			u, _ = url.Parse(apiTarget)
		} else {
			if input.SwaggerURL == "" && s.Def.Servers != nil {
				u, _ = url.Parse(s.Def.Servers[0].URL)
				if strings.Contains(s.Def.Servers[0].URL, "localhost") || strings.Contains(s.Def.Servers[0].URL, "127.0.0.1") || strings.Contains(s.Def.Servers[0].URL, "::1") {
					log.Warn().Msg("The server documented in the definition file contains a local host value and may result in errors. Supply a target manually using the '-T' flag.")
				}
			}
		}
		s.URL = *u
		s = s.IterateOverPaths()
	}

	slices.Sort(jsonResultsStringArray)
	for r := range jsonResultsStringArray {

		var verboseResult VerboseResult
		err := json.Unmarshal([]byte(strings.TrimPrefix(jsonResultsStringArray[r], ",")), &verboseResult)
		if err != nil {
			log.Error().Err(err).Msg("Error marshalling JSON.")
		}
		jsonVerboseResultArray = append(jsonVerboseResultArray, verboseResult)

	}
	log.Info().Interface("results", jsonVerboseResultArray).Interface("request", s).Msg("Results")

	return s.Paths, nil
}

func (s SwaggerRequest) IterateOverPaths() SwaggerRequest {
	for path, pathItem := range s.Def.Paths.Map() {
		operations := map[string]*openapi3.Operation{
			"CONNECT": pathItem.Connect,
			"GET":     pathItem.Get,
			"HEAD":    pathItem.Head,
			"OPTIONS": pathItem.Options,
			"PATCH":   pathItem.Patch,
			"POST":    pathItem.Post,
			"PUT":     pathItem.Put,
			"TRACE":   pathItem.Trace,
		}

		for method, op := range operations {
			// Do all the things here :D
			s.BodyData = nil
			if op != nil {
				s.URL.Path = s.Path + path
				s = s.BuildDefinedRequests(method, pathItem, op)
			}
		}
	}
	return s
}

func (s SwaggerRequest) BuildDefinedRequests(method string, pathItem *openapi3.PathItem, op *openapi3.Operation) SwaggerRequest {
	s.ApiInQuery = false
	b := make(map[string]any)
	s.Body = b
	s.Query = url.Values{}

	var pathMap = make(map[string]bool)

	basePathResult := s.GetBasePath()
	s.URL.Path = basePathResult + s.URL.Path

	s = s.AddParametersToRequest(op)

	var errorDescriptions = make(map[any]string)
	for status := range op.Responses.Map() {
		if op.Responses.Map()[status].Ref == "" {
			if op.Responses.Map()[status].Value == nil {
				continue
			} else {
				errorDescriptions[status] = *op.Responses.Map()[status].Value.Description
			}
		} else {
			continue
		}
	}

	s.URL.RawQuery = s.Query.Encode()

	for k := range s.Def.Paths.Map() {
		if !pathMap[k] {
			s.Paths = append(s.Paths, basePathResult+k)
			pathMap[k] = true
		}
	}
	return s
}

// This whole function needs to be refactored/cleaned up a bit
func (s SwaggerRequest) AddParametersToRequest(op *openapi3.Operation) SwaggerRequest {
	for _, param := range op.Parameters {
		if param.Value == nil || param.Value.Schema.Ref == "" {
			continue
		} else if param.Value.In == "path" {
			if param.Value.Schema != nil {
				if param.Value.Schema.Ref != "" {
					s = s.SetParametersFromSchema(param, "path", param.Value.Schema.Ref, nil, 0)
				} else if len(param.Value.Schema.Value.Type.Slice()) > 0 && param.Value.Schema.Value.Type.Includes("string") {
					if strings.Contains(s.URL.Path, param.Value.Name) {
						s.URL.Path = strings.ReplaceAll(s.URL.Path, "{"+param.Value.Name+"}", "test")
					} else {
						s.URL.Path = strings.ReplaceAll(s.URL.Path, "{"+strings.ToLower(param.Value.Name)+"}", "test")
					}
				} else {
					if strings.Contains(s.URL.Path, param.Value.Name) {
						s.URL.Path = strings.ReplaceAll(s.URL.Path, "{"+param.Value.Name+"}", "1")
					} else {
						s.URL.Path = strings.ReplaceAll(s.URL.Path, "{"+strings.ToLower(param.Value.Name)+"}", "1")
					}
				}
			} else {
				if strings.Contains(s.URL.Path, param.Value.Name) {
					s.URL.Path = strings.ReplaceAll(s.URL.Path, "{"+param.Value.Name+"}", "test")
				} else {
					s.URL.Path = strings.ReplaceAll(s.URL.Path, "{"+strings.ToLower(param.Value.Name)+"}", "test")
				}
			}
		} else if param.Value.In == "query" {
			if param.Value.Schema != nil {
				if param.Value.Schema.Ref != "" {
					s = s.SetParametersFromSchema(param, "query", param.Value.Schema.Ref, nil, 0)
				} else {
					if len(param.Value.Schema.Value.Type.Slice()) > 0 && param.Value.Schema.Value.Type.Includes("string") {

						s.Query.Add(param.Value.Name, "test")
					} else {
						s.Query.Add(param.Value.Name, "1")
					}
				}
			} else {
				s.Query.Add(param.Value.Name, "test")
			}

		} else if param.Value.In == "header" && param.Value.Required && strings.ToLower(param.Value.Name) != "content-type" {
			Headers = append(Headers, fmt.Sprintf("%s: %s", param.Value.Name, "1"))
		} else if param.Value.In == "body" || param.Value.In == "formData" {
			if param.Value.Schema.Ref != "" {
				s = s.SetParametersFromSchema(param, "body", param.Value.Schema.Ref, nil, 0)
			}
			if param.Value.Schema.Value.Type.Includes("string") {
				s.Body[param.Value.Name] = "test"
			} else {
				s.Body[param.Value.Name] = 1
			}
			var data []string
			for k, v := range s.Body {
				data = append(data, fmt.Sprintf("%s=%s", k, v))
			}
			s.BodyData = []byte(strings.Join(data, "&"))
		} else {
			continue
		}
	}

	if op.RequestBody != nil {
		if op.RequestBody.Value != nil {
			if op.RequestBody.Value.Content != nil {
				for i := range op.RequestBody.Value.Content {
					if contentType == "" {
						EnforceSingleContentType(i)
					} else {
						EnforceSingleContentType(contentType)
					}
					if op.RequestBody.Value.Content.Get(i).Schema != nil {
						if op.RequestBody.Value.Content.Get(i).Schema.Value == nil {
							s = s.SetParametersFromSchema(nil, "body", op.RequestBody.Value.Content.Get(i).Schema.Ref, op.RequestBody, 0)
							if strings.Contains(i, "json") {
								s.BodyData, _ = json.Marshal(s.Body)
							} else if strings.Contains(i, "x-www-form-urlencoded") {
								var formData []string
								for j := range s.Body {
									formData = append(formData, fmt.Sprintf("%s=%s", j, fmt.Sprint(s.Body[j])))
								}
								s.BodyData = []byte(strings.Join(formData, "&"))
							} else if strings.Contains(i, "xml") {
								type Element struct {
									XMLName xml.Name
									Content any `xml:",chardata"`
								}

								type Root struct {
									XMLName  xml.Name  `xml:"root"`
									Elements []Element `xml:",any"`
								}

								var elements []Element
								for key, value := range s.Body {
									elements = append(elements, Element{
										XMLName: xml.Name{Local: key},
										Content: value,
									})
								}

								root := Root{
									Elements: elements,
								}

								xmlData, err := xml.Marshal(root)
								if err != nil {
									log.Error().Err(err).Msg("Error marshalling XML data.")
								}
								s.BodyData = xmlData
							} else {
								log.Warn().Str("path", s.URL.Path).Str("content_type", i).Msg("Content type not supported.")
							}
						} else {
							var formData []string

							for j := range op.RequestBody.Value.Content.Get(i).Schema.Value.Properties {
								if op.RequestBody.Value.Content.Get(i).Schema.Value.Properties[j].Ref != "" {
									s = s.SetParametersFromSchema(nil, "body", op.RequestBody.Value.Content.Get(i).Schema.Value.Properties[j].Ref, op.RequestBody, 0)
								} else {
									valueTypes := op.RequestBody.Value.Content.Get(i).Schema.Value.Properties[j].Value.Type
									if op.RequestBody.Value.Content.Get(i).Schema.Value.Properties[j].Value != nil {
										if valueTypes.Includes("string") {
											s.Body[j] = "test"
										} else if valueTypes.Includes("boolean") {
											s.Body[j] = false
										} else if valueTypes.Includes("integer") || valueTypes.Includes("number") {
											s.Body[j] = 1
										} else {
											s.Body[j] = "unknown_type_populate_manually"
										}
										if i == "application/x-www-form-urlencoded" {
											formData = append(formData, fmt.Sprintf("%s=%s", j, fmt.Sprint(s.Body[j])))
										}
									}

									if i == "application/x-www-form-urlencoded" {
										s.BodyData = []byte(strings.Join(formData, "&"))
									} else if strings.Contains(i, "json") || i == "*/*" {
										s.BodyData, _ = json.Marshal(s.Body)
									} else if strings.Contains(i, "xml") {
										//
										type Element struct {
											XMLName xml.Name
											Content any `xml:",chardata"`
										}

										type Root struct {
											XMLName  xml.Name  `xml:"root"`
											Elements []Element `xml:",any"`
										}

										var elements []Element
										for key, value := range s.Body {
											elements = append(elements, Element{
												XMLName: xml.Name{Local: key},
												Content: value,
											})
										}

										root := Root{
											Elements: elements,
										}

										xmlData, err := xml.Marshal(root)
										if err != nil {
											log.Error().Err(err).Msg("Error marshalling XML data.")
										}
										s.BodyData = xmlData
									} else {
										s.Body["test"] = "test"
										s.BodyData = []byte("test=test")
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if s.ApiInQuery && s.ApiKey != "" {
		s.Query.Add(s.ApiKeyName, s.ApiKey)
	}
	return s
}

func (s SwaggerRequest) GetBasePath() string {
	if basePath == "" {
		if s.Def.Servers != nil {
			if strings.Contains(s.Def.Servers[0].URL, ":") {
				var schemeIndex int
				if strings.Contains(s.Def.Servers[0].URL, "://") {
					schemeIndex = strings.Index(s.Def.Servers[0].URL, "://") + 3
				} else {
					schemeIndex = 0
				}
				s.URL.Host = s.Def.Servers[0].URL[schemeIndex:]
			}

			if s.Def.Servers[0].URL == "/" {
				basePath = "/"
			} else if strings.Contains(s.Def.Servers[0].URL, "http") && !strings.Contains(s.Def.Servers[0].URL, s.URL.Host) { // Check to see if the server object being used for the base path contains a different host than the target
				basePath = s.Def.Servers[0].URL
				basePath = strings.ReplaceAll(basePath, "http://", "")
				basePath = strings.ReplaceAll(basePath, "https://", "")
				indexSubdomain := strings.Index(basePath, "/")
				basePath = basePath[indexSubdomain:]
				if !strings.HasSuffix(basePath, "/") {
					basePath = basePath + "/"
				}
			} else {
				basePath = s.Def.Servers[0].URL
				if strings.Contains(basePath, s.URL.Host) || strings.Contains(basePath, "http") {
					basePath = strings.ReplaceAll(basePath, s.URL.Host, "")
					basePath = strings.ReplaceAll(basePath, "http://", "")
					basePath = strings.ReplaceAll(basePath, "https://", "")
				}
			}

		}

	}
	basePath = strings.TrimSuffix(basePath, "/")
	return basePath
}

func PrintSpecInfo(i openapi3.Info) {
	if i.Title != "" {
		fmt.Println("Title:", i.Title)
	}

	if i.Description != "" {
		fmt.Printf("Description: %s\n\n", i.Description)
	}

	if i.Title == "" && i.Description == "" {
		log.Warn().Msg("Detected possible error in parsing the definition file. Title and description values are empty.\n\n")
	}
}

func SetScheme(swaggerURL string) (scheme string) {
	if strings.HasPrefix(swaggerURL, "http://") {
		scheme = "http"
	} else if strings.HasPrefix(swaggerURL, "https://") {
		scheme = "https"
	} else {
		scheme = "https"
	}
	return scheme
}

/*
TrimHostScheme trims the scheme from the provided URL if the '-T' flag is supplied to sj.
*/
func TrimHostScheme(apiTarget, fullUrlHost string) (host string) {
	if apiTarget != "" {
		if strings.HasPrefix(apiTarget, "http://") {
			host = strings.TrimPrefix(apiTarget, "http://")
		} else if strings.HasPrefix(apiTarget, "https://") {
			host = strings.TrimPrefix(apiTarget, "https://")
		} else {
			host = apiTarget
		}
	} else {
		host = fullUrlHost
	}
	return host
}

func unmarshalSpec(input OpenapiParseInput) (newDoc *openapi3.T, err error) {
	var doc openapi2.T
	var doc3 openapi3.T

	bodyBytes := input.BodyBytes
	switch input.Format {
	case "js":
		bodyBytes = ExtractSpecFromJS(bodyBytes)
	case "yaml", "yml":
		err = yaml.Unmarshal(bodyBytes, &doc)
		if err != nil {
			log.Error().Err(err).Msg("Error unmarshalling YAML.")
			return nil, err
		}

		err = yaml.Unmarshal(bodyBytes, &doc3)
		if err != nil {
			log.Error().Err(err).Msg("Error unmarshalling YAML.")
			return nil, err
		}
	}

	err = json.Unmarshal(bodyBytes, &doc)
	if err != nil {
		log.Error().Err(err).Msg("Error unmarshalling JSON.")
		return nil, err
	}

	err = json.Unmarshal(bodyBytes, &doc3)
	if err != nil {
		log.Error().Err(err).Msg("Error unmarshalling JSON.")
		return nil, err
	}

	if strings.HasPrefix(doc3.OpenAPI, "3") {
		newDoc := &doc3
		return newDoc, nil
	} else if strings.HasPrefix(doc.Swagger, "2") {
		newDoc, err := openapi2conv.ToV3(&doc)
		if err != nil {
			log.Error().Err(err).Msg("Error converting v2 document to v3.")
			return nil, err
		}
		return newDoc, nil

	} else {
		log.Error().Msg("Error parsing definition file.")
		return nil, err
	}
}

func ExtractSpecFromJS(bodyBytes []byte) []byte {
	var openApiIndex int
	var specClose int
	var bodyString, spec string

	bodyString = string(bodyBytes)
	spec = strings.ReplaceAll(bodyString, "\n", "")
	spec = strings.ReplaceAll(spec, "\t", "")
	spec = strings.ReplaceAll(spec, " ", "")

	if strings.Contains(strings.ReplaceAll(bodyString, " ", ""), `"swagger":"2.0"`) {
		openApiIndex = strings.Index(spec, `"swagger":`) - 1
		specClose = strings.LastIndex(spec, "]}") + 2

		var doc2 openapi2.T
		bodyBytes = []byte(spec[openApiIndex:specClose])
		_ = json.Unmarshal(bodyBytes, &doc2)
		if !strings.Contains(doc2.Swagger, "2") {
			specClose = strings.LastIndex(spec, "}") + 1
			bodyBytes = []byte(spec[openApiIndex:specClose])
			_ = json.Unmarshal(bodyBytes, &doc2)
			if !strings.Contains(doc2.Swagger, "2") {
				log.Error().Msg("Error parsing JavaScript file for spec. Try saving the object as a JSON file and reference it locally.")
			}
		}
	} else if strings.Contains(strings.ReplaceAll(bodyString, " ", ""), `"openapi":"3`) {
		openApiIndex = strings.Index(spec, `"openapi":`) - 1

		specClose = strings.LastIndex(spec, "]}") + 2

		var doc3 openapi3.T
		bodyBytes = []byte(spec[openApiIndex:specClose])
		_ = json.Unmarshal(bodyBytes, &doc3)
		if !strings.Contains(doc3.OpenAPI, "3") {
			specClose = strings.LastIndex(spec, "}") + 1
			bodyBytes = []byte(spec[openApiIndex:specClose])
			_ = json.Unmarshal(bodyBytes, &doc3)
			if !strings.Contains(doc3.OpenAPI, "3") {
				log.Error().Msg("Error parsing JavaScript file for spec. Try saving the object as a JSON file and reference it locally.")
			}
		}
	} else {
		log.Error().Msg("Error parsing JavaScript file for spec. Try saving the object as a JSON file and reference it locally.")
	}

	return bodyBytes
}

func EnforceSingleContentType(newContentType string) {
	newContentType = strings.TrimSpace(newContentType)
	if Headers != nil {
		headerString := strings.Join(Headers, ",")
		Headers = nil
		ctIndex := strings.Index(strings.ToLower(headerString), "content-type:") + 14
		headerString = headerString[ctIndex:]
		if strings.Contains(headerString, ",") {
			headerString = strings.TrimPrefix(headerString, ",")
			ctEndIndex := strings.Index(headerString[ctIndex:], ",") + 1
			headerString = headerString[:ctEndIndex]
		} else if !strings.Contains(headerString, ":") {
			headerString = ""
		}
		if headerString != "" {
			Headers = append(Headers, strings.Split(headerString, ",")...)
		}
	}

	Headers = append(Headers, "Content-Type: "+newContentType)
}
