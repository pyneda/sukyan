package scan

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
)

// GetWebSocketMessageInsertionPoints analyzes a WebSocket message and identifies insertion points
// based on content type (JSON, XML, plain text)
func GetWebSocketMessageInsertionPoints(message *db.WebSocketMessage, scoped []string) ([]InsertionPoint, error) {
	var points []InsertionPoint

	// Always add the raw message as an insertion point
	if lib.SliceContains(scoped, "ws_raw") {
		points = append(points, InsertionPoint{
			Type:         InsertionPointTypeWSRawMessage,
			Name:         "message",
			Value:        message.PayloadData,
			ValueType:    lib.GuessDataType(message.PayloadData),
			OriginalData: message.PayloadData,
		})
	}

	// Try to determine if the message is JSON and extract JSON insertion points
	if lib.SliceContains(scoped, "ws_json") && isLikelyJSON(message.PayloadData) {
		jsonPoints, err := extractJSONInsertionPoints(message.PayloadData)
		if err != nil {
			log.Debug().Err(err).Str("payload", message.PayloadData).Msg("Failed to extract JSON insertion points")
		} else {
			points = append(points, jsonPoints...)
		}
	}

	// Try to determine if the message is XML and extract XML insertion points
	if lib.SliceContains(scoped, "ws_xml") && isLikelyXML(message.PayloadData) {
		xmlPoints, err := extractXMLInsertionPoints(message.PayloadData)
		if err != nil {
			log.Debug().Err(err).Str("payload", message.PayloadData).Msg("Failed to extract XML insertion points")
		} else {
			points = append(points, xmlPoints...)
		}
	}

	return points, nil
}

// isLikelyJSON checks if a string appears to be JSON
func isLikelyJSON(data string) bool {
	data = strings.TrimSpace(data)
	return (strings.HasPrefix(data, "{") && strings.HasSuffix(data, "}")) ||
		(strings.HasPrefix(data, "[") && strings.HasSuffix(data, "]"))
}

// isLikelyXML checks if a string appears to be XML
func isLikelyXML(data string) bool {
	data = strings.TrimSpace(data)
	return strings.HasPrefix(data, "<") && strings.HasSuffix(data, ">")
}

// extractJSONInsertionPoints extracts all possible insertion points from JSON data
func extractJSONInsertionPoints(data string) ([]InsertionPoint, error) {
	var points []InsertionPoint

	// Try to parse as object first
	var jsonObj map[string]interface{}
	err := json.Unmarshal([]byte(data), &jsonObj)

	if err == nil {
		// Successfully parsed as object
		// Add the entire object as an insertion point
		points = append(points, InsertionPoint{
			Type:         InsertionPointTypeWSJSONObject,
			Name:         "root",
			Value:        data,
			ValueType:    lib.TypeJSON,
			OriginalData: data,
		})

		// Extract all fields recursively
		objectPoints := extractJSONObjectPoints("", jsonObj, data)
		points = append(points, objectPoints...)

		return points, nil
	}

	// Try to parse as array
	var jsonArray []interface{}
	err = json.Unmarshal([]byte(data), &jsonArray)

	if err == nil {
		// Successfully parsed as array
		// Add the entire array as an insertion point
		points = append(points, InsertionPoint{
			Type:         InsertionPointTypeWSJSONArray,
			Name:         "root",
			Value:        data,
			ValueType:    lib.TypeJSON,
			OriginalData: data,
		})

		// Extract all array items recursively
		arrayPoints := extractJSONArrayPoints("root", jsonArray, data)
		points = append(points, arrayPoints...)

		return points, nil
	}

	return nil, fmt.Errorf("failed to parse JSON: %v", err)
}

// extractJSONObjectPoints recursively extracts insertion points from a JSON object
func extractJSONObjectPoints(path string, obj map[string]interface{}, originalData string) []InsertionPoint {
	var points []InsertionPoint

	for key, value := range obj {
		currentPath := key
		if path != "" {
			currentPath = path + "." + key
		}

		// Add the key itself as an insertion point
		points = append(points, InsertionPoint{
			Type:         InsertionPointTypeWSJSONKey,
			Name:         currentPath,
			Value:        key,
			ValueType:    lib.TypeString,
			OriginalData: originalData,
		})

		switch v := value.(type) {
		case map[string]interface{}:
			// For nested objects, recurse and collect points
			nestedPoints := extractJSONObjectPoints(currentPath, v, originalData)
			points = append(points, nestedPoints...)

			// Also add the entire object value as a field
			objBytes, _ := json.Marshal(v)
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeWSJSONField,
				Name:         currentPath,
				Value:        string(objBytes),
				ValueType:    lib.TypeJSON,
				OriginalData: originalData,
			})

		case []interface{}:
			// For arrays, extract array-specific points
			arrayPoints := extractJSONArrayPoints(currentPath, v, originalData)
			points = append(points, arrayPoints...)

			// Also add the entire array as a field
			arrayBytes, _ := json.Marshal(v)
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeWSJSONField,
				Name:         currentPath,
				Value:        string(arrayBytes),
				ValueType:    lib.TypeJSON,
				OriginalData: originalData,
			})

		default:
			// For primitive values
			valueStr := fmt.Sprintf("%v", v)

			// Add as a field (key+value pair)
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeWSJSONField,
				Name:         currentPath,
				Value:        valueStr,
				ValueType:    lib.GuessDataType(valueStr),
				OriginalData: originalData,
			})

			// Add just the value
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeWSJSONValue,
				Name:         currentPath,
				Value:        valueStr,
				ValueType:    lib.GuessDataType(valueStr),
				OriginalData: originalData,
			})
		}
	}

	return points
}

// extractJSONArrayPoints recursively extracts insertion points from a JSON array
func extractJSONArrayPoints(path string, array []interface{}, originalData string) []InsertionPoint {
	var points []InsertionPoint

	for i, item := range array {
		currentPath := fmt.Sprintf("%s[%d]", path, i)

		// Add the index as an insertion point
		points = append(points, InsertionPoint{
			Type:         InsertionPointTypeWSJSONArrayIndex,
			Name:         currentPath,
			Value:        strconv.Itoa(i),
			ValueType:    lib.TypeInt,
			OriginalData: originalData,
		})

		switch v := item.(type) {
		case map[string]interface{}:
			// For objects in arrays, extract object points
			objectPoints := extractJSONObjectPoints(currentPath, v, originalData)
			points = append(points, objectPoints...)

			// Also add the entire object as an array item
			objBytes, _ := json.Marshal(v)
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeWSJSONArrayItem,
				Name:         currentPath,
				Value:        string(objBytes),
				ValueType:    lib.TypeJSON,
				OriginalData: originalData,
			})

		case []interface{}:
			// For nested arrays, recurse
			nestedArrayPoints := extractJSONArrayPoints(currentPath, v, originalData)
			points = append(points, nestedArrayPoints...)

			// Also add the entire nested array as an array item
			arrayBytes, _ := json.Marshal(v)
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeWSJSONArrayItem,
				Name:         currentPath,
				Value:        string(arrayBytes),
				ValueType:    lib.TypeJSON,
				OriginalData: originalData,
			})

		default:
			// For primitive values in arrays
			valueStr := fmt.Sprintf("%v", v)
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeWSJSONArrayItem,
				Name:         currentPath,
				Value:        valueStr,
				ValueType:    lib.GuessDataType(valueStr),
				OriginalData: originalData,
			})
		}
	}

	return points
}

// extractXMLInsertionPoints extracts insertion points from XML data
func extractXMLInsertionPoints(data string) ([]InsertionPoint, error) {
	var points []InsertionPoint

	// Add the entire XML document as an insertion point
	points = append(points, InsertionPoint{
		Type:         InsertionPointTypeWSXMLElement,
		Name:         "document",
		Value:        data,
		ValueType:    lib.TypeXML,
		OriginalData: data,
	})

	// Extract tag names
	tagPattern := regexp.MustCompile(`<([a-zA-Z0-9_:-]+)(\s+[^>]*)?[>/]`)
	tagMatches := tagPattern.FindAllStringSubmatch(data, -1)
	tagNames := make(map[string]bool)

	for _, match := range tagMatches {
		if len(match) > 1 {
			tagName := match[1]
			if !tagNames[tagName] {
				tagNames[tagName] = true
				points = append(points, InsertionPoint{
					Type:         InsertionPointTypeWSXMLTag,
					Name:         tagName,
					Value:        tagName,
					ValueType:    lib.TypeString,
					OriginalData: data,
				})
			}
		}
	}

	// Extract attributes
	attrPattern := regexp.MustCompile(`([a-zA-Z0-9_:-]+)=["']([^"']*)["']`)
	attrMatches := attrPattern.FindAllStringSubmatch(data, -1)

	for _, match := range attrMatches {
		if len(match) > 2 {
			attrName := match[1]
			attrValue := match[2]
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeWSXMLAttribute,
				Name:         attrName,
				Value:        attrValue,
				ValueType:    lib.GuessDataType(attrValue),
				OriginalData: data,
			})
		}
	}

	// Extract namespaces
	nsPattern := regexp.MustCompile(`xmlns:([a-zA-Z0-9_-]+)=["']([^"']*)["']`)
	nsMatches := nsPattern.FindAllStringSubmatch(data, -1)

	for _, match := range nsMatches {
		if len(match) > 2 {
			nsPrefix := match[1]
			nsValue := match[2]

			// Add namespace prefix
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeWSXMLNSPrefix,
				Name:         nsPrefix,
				Value:        nsPrefix,
				ValueType:    lib.TypeString,
				OriginalData: data,
			})

			// Add namespace value
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeWSXMLNamespace,
				Name:         nsPrefix,
				Value:        nsValue,
				ValueType:    lib.TypeString,
				OriginalData: data,
			})
		}
	}

	// Extract processing instructions
	piPattern := regexp.MustCompile(`<\?([a-zA-Z0-9_-]+)([^?]*)\?>`)
	piMatches := piPattern.FindAllStringSubmatch(data, -1)

	for _, match := range piMatches {
		if len(match) > 2 {
			piTarget := match[1]
			piData := strings.TrimSpace(match[2])
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeWSXMLProcessing,
				Name:         piTarget,
				Value:        piData,
				ValueType:    lib.TypeString,
				OriginalData: data,
			})
		}
	}

	return points, nil
}

// CreateModifiedWebSocketMessage applies a payload to a specific insertion point in a WebSocket message
func CreateModifiedWebSocketMessage(original *db.WebSocketMessage, insertionPoint InsertionPoint, payload string) (*db.WebSocketMessage, error) {
	// Create a new message based on the original
	modified := *original

	switch insertionPoint.Type {
	case InsertionPointTypeWSRawMessage:
		// For raw messages, replace the entire payload
		modified.PayloadData = payload
		return &modified, nil

	case InsertionPointTypeWSJSONObject, InsertionPointTypeWSJSONArray:
		// Replace entire JSON structure
		if insertionPoint.Name == "root" {
			modified.PayloadData = payload
			return &modified, nil
		}
		return nil, fmt.Errorf("cannot replace non-root JSON structure")

	case InsertionPointTypeWSJSONField, InsertionPointTypeWSJSONValue,
		InsertionPointTypeWSJSONKey, InsertionPointTypeWSJSONArrayItem:
		// Modify JSON structure
		newPayload, err := modifyJSONWithPayload(original.PayloadData, insertionPoint, payload)
		if err != nil {
			return nil, err
		}
		modified.PayloadData = newPayload
		return &modified, nil

	case InsertionPointTypeWSXMLElement, InsertionPointTypeWSXMLTag,
		InsertionPointTypeWSXMLAttribute, InsertionPointTypeWSXMLNamespace,
		InsertionPointTypeWSXMLNSPrefix, InsertionPointTypeWSXMLProcessing:
		// Modify XML structure
		newPayload, err := modifyXMLWithPayload(original.PayloadData, insertionPoint, payload)
		if err != nil {
			return nil, err
		}
		modified.PayloadData = newPayload
		return &modified, nil

	default:
		return nil, fmt.Errorf("unsupported insertion point type: %s", insertionPoint.Type)
	}
}

// modifyJSONWithPayload applies a payload to a JSON structure at the specified insertion point
func modifyJSONWithPayload(jsonStr string, point InsertionPoint, payload string) (string, error) {
	// First determine if we're dealing with an object or array
	if strings.HasPrefix(strings.TrimSpace(jsonStr), "{") {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &obj); err != nil {
			return "", err
		}

		result, err := modifyJSONObject(obj, point, payload)
		if err != nil {
			return "", err
		}

		modified, err := json.Marshal(result)
		if err != nil {
			return "", err
		}

		return string(modified), nil
	} else if strings.HasPrefix(strings.TrimSpace(jsonStr), "[") {
		var arr []interface{}
		if err := json.Unmarshal([]byte(jsonStr), &arr); err != nil {
			return "", err
		}

		result, err := modifyJSONArray(arr, point, payload)
		if err != nil {
			return "", err
		}

		modified, err := json.Marshal(result)
		if err != nil {
			return "", err
		}

		return string(modified), nil
	}

	return "", fmt.Errorf("invalid JSON structure")
}

// modifyJSONObject applies a payload to a JSON object
func modifyJSONObject(obj map[string]interface{}, point InsertionPoint, payload string) (map[string]interface{}, error) {
	pathParts := strings.Split(point.Name, ".")

	// Handle root-level fields
	if len(pathParts) == 1 && !strings.Contains(pathParts[0], "[") {
		key := pathParts[0]

		switch point.Type {
		case InsertionPointTypeWSJSONKey:
			// Change a key name
			if _, exists := obj[key]; exists {
				value := obj[key]
				delete(obj, key)
				obj[payload] = value
			}

		case InsertionPointTypeWSJSONValue:
			// Change just the value
			if _, exists := obj[key]; exists {
				var newValue interface{} = payload

				// Try to maintain the type of the original value
				switch obj[key].(type) {
				case float64:
					if floatVal, err := strconv.ParseFloat(payload, 64); err == nil {
						newValue = floatVal
					}
				case int:
					if intVal, err := strconv.Atoi(payload); err == nil {
						newValue = intVal
					}
				case bool:
					if payload == "true" {
						newValue = true
					} else if payload == "false" {
						newValue = false
					}
				}

				obj[key] = newValue
			}

		case InsertionPointTypeWSJSONField:
			// Change both key and value (replace field)
			var newValue interface{} = payload

			// If payload is valid JSON, parse it
			if isLikelyJSON(payload) {
				if strings.HasPrefix(payload, "{") {
					var nestedObj map[string]interface{}
					if err := json.Unmarshal([]byte(payload), &nestedObj); err == nil {
						newValue = nestedObj
					}
				} else if strings.HasPrefix(payload, "[") {
					var nestedArr []interface{}
					if err := json.Unmarshal([]byte(payload), &nestedArr); err == nil {
						newValue = nestedArr
					}
				}
			}

			obj[key] = newValue
		}

		return obj, nil
	}

	// Handle nested paths
	if len(pathParts) > 1 {
		firstPart := pathParts[0]

		// Check if this is an array access
		if arrayIdx, rest, isArray := parseArrayAccess(firstPart); isArray {
			if arr, ok := obj[rest].([]interface{}); ok && arrayIdx < len(arr) {
				// Handle array elements
				if strings.Contains(point.Name, ".") {
					// Nested path within array element
					remainingPath := strings.Join(pathParts[1:], ".")
					newPoint := InsertionPoint{
						Type:         point.Type,
						Name:         remainingPath,
						Value:        point.Value,
						ValueType:    point.ValueType,
						OriginalData: point.OriginalData,
					}

					if nestedObj, ok := arr[arrayIdx].(map[string]interface{}); ok {
						modified, err := modifyJSONObject(nestedObj, newPoint, payload)
						if err == nil {
							arr[arrayIdx] = modified
							obj[rest] = arr
						}
					}
				}
			}
		} else {
			// Handle nested objects
			if nestedObj, ok := obj[firstPart].(map[string]interface{}); ok {
				remainingPath := strings.Join(pathParts[1:], ".")
				newPoint := InsertionPoint{
					Type:         point.Type,
					Name:         remainingPath,
					Value:        point.Value,
					ValueType:    point.ValueType,
					OriginalData: point.OriginalData,
				}

				modified, err := modifyJSONObject(nestedObj, newPoint, payload)
				if err == nil {
					obj[firstPart] = modified
				}
			}
		}

		return obj, nil
	}

	// Handle array access at root level
	if arrayIdx, name, isArray := parseArrayAccess(point.Name); isArray {
		if arr, ok := obj[name].([]interface{}); ok && arrayIdx < len(arr) {
			switch point.Type {
			case InsertionPointTypeWSJSONArrayItem:
				// Replace array item with payload
				var newValue interface{} = payload

				// If payload is valid JSON, parse it
				if isLikelyJSON(payload) {
					if strings.HasPrefix(payload, "{") {
						var nestedObj map[string]interface{}
						if err := json.Unmarshal([]byte(payload), &nestedObj); err == nil {
							newValue = nestedObj
						}
					} else if strings.HasPrefix(payload, "[") {
						var nestedArr []interface{}
						if err := json.Unmarshal([]byte(payload), &nestedArr); err == nil {
							newValue = nestedArr
						}
					}
				}

				arr[arrayIdx] = newValue
				obj[name] = arr
			}
		}
	}

	return obj, nil
}

// modifyJSONArray applies a payload to a JSON array
func modifyJSONArray(arr []interface{}, point InsertionPoint, payload string) ([]interface{}, error) {
	// Extract the array index from the path
	if arrayIdx, _, isArray := parseArrayAccess(point.Name); isArray && arrayIdx < len(arr) {
		switch point.Type {
		case InsertionPointTypeWSJSONArrayItem:
			// Replace array item with payload
			var newValue interface{} = payload

			// If payload is valid JSON, parse it
			if isLikelyJSON(payload) {
				if strings.HasPrefix(payload, "{") {
					var nestedObj map[string]interface{}
					if err := json.Unmarshal([]byte(payload), &nestedObj); err == nil {
						newValue = nestedObj
					}
				} else if strings.HasPrefix(payload, "[") {
					var nestedArr []interface{}
					if err := json.Unmarshal([]byte(payload), &nestedArr); err == nil {
						newValue = nestedArr
					}
				}
			}

			arr[arrayIdx] = newValue

		case InsertionPointTypeWSJSONValue:
			// If the array item is a primitive, replace it
			if _, ok := arr[arrayIdx].(map[string]interface{}); !ok {
				if _, ok := arr[arrayIdx].([]interface{}); !ok {
					// It's a primitive value
					var newValue interface{} = payload

					// Try to convert to the right type
					switch arr[arrayIdx].(type) {
					case float64:
						if floatVal, err := strconv.ParseFloat(payload, 64); err == nil {
							newValue = floatVal
						}
					case int:
						if intVal, err := strconv.Atoi(payload); err == nil {
							newValue = intVal
						}
					case bool:
						if payload == "true" {
							newValue = true
						} else if payload == "false" {
							newValue = false
						}
					}

					arr[arrayIdx] = newValue
				}
			}

		case InsertionPointTypeWSJSONArrayIndex:
			// This should be handled elsewhere as it doesn't modify the array itself
			return nil, fmt.Errorf("cannot directly modify array indices")
		}
	}

	return arr, nil
}

// parseArrayAccess extracts array index from a path like "users[0]"
func parseArrayAccess(path string) (int, string, bool) {
	re := regexp.MustCompile(`(.*)\[(\d+)\]$`)
	matches := re.FindStringSubmatch(path)

	if len(matches) == 3 {
		idx, err := strconv.Atoi(matches[2])
		if err != nil {
			return 0, "", false
		}
		return idx, matches[1], true
	}

	return 0, "", false
}

// modifyXMLWithPayload applies a payload to an XML structure at the specified insertion point
func modifyXMLWithPayload(xmlStr string, point InsertionPoint, payload string) (string, error) {
	switch point.Type {
	case InsertionPointTypeWSXMLElement:
		if point.Name == "document" {
			// Replace entire document
			return payload, nil
		}

		// For named elements, find and replace
		pattern := fmt.Sprintf(`(<%s[^>]*>)(.*?)(</%s>)`, regexp.QuoteMeta(point.Name), regexp.QuoteMeta(point.Name))
		re := regexp.MustCompile(pattern)
		return re.ReplaceAllString(xmlStr, "${1}"+payload+"${3}"), nil

	case InsertionPointTypeWSXMLTag:
		// Replace tag name
		oldTag := point.Value
		pattern := fmt.Sprintf(`<%s([^>]*)>`, regexp.QuoteMeta(oldTag))
		re := regexp.MustCompile(pattern)
		result := re.ReplaceAllString(xmlStr, "<"+payload+"${1}>")

		// Also replace closing tag
		closingPattern := fmt.Sprintf(`</%s>`, regexp.QuoteMeta(oldTag))
		closingRe := regexp.MustCompile(closingPattern)
		return closingRe.ReplaceAllString(result, "</"+payload+">"), nil

	case InsertionPointTypeWSXMLAttribute:
		// Replace attribute value
		attrName := point.Name
		pattern := fmt.Sprintf(`(%s=)["']([^"']*)["']`, regexp.QuoteMeta(attrName))
		re := regexp.MustCompile(pattern)
		return re.ReplaceAllString(xmlStr, "${1}\""+payload+"\""), nil

	case InsertionPointTypeWSXMLNamespace:
		// Replace namespace URI
		nsPrefix := point.Name
		pattern := fmt.Sprintf(`(xmlns:%s=)["']([^"']*)["']`, regexp.QuoteMeta(nsPrefix))
		re := regexp.MustCompile(pattern)
		return re.ReplaceAllString(xmlStr, "${1}\""+payload+"\""), nil

	case InsertionPointTypeWSXMLNSPrefix:
		// Replace namespace prefix - this is complex and might break XML
		oldPrefix := point.Value
		newXML := xmlStr

		// Replace in xmlns declaration
		nsPattern := fmt.Sprintf(`xmlns:%s=`, regexp.QuoteMeta(oldPrefix))
		nsReplacement := fmt.Sprintf(`xmlns:%s=`, payload)
		newXML = strings.Replace(newXML, nsPattern, nsReplacement, -1)

		// Replace in element usage
		tagPattern := fmt.Sprintf(`(</?)%s:`, regexp.QuoteMeta(oldPrefix))
		tagRe := regexp.MustCompile(tagPattern)
		newXML = tagRe.ReplaceAllString(newXML, "${1}"+payload+":")

		// Replace in attribute usage
		attrPattern := fmt.Sprintf(`%s:([a-zA-Z0-9_-]+)=`, regexp.QuoteMeta(oldPrefix))
		attrRe := regexp.MustCompile(attrPattern)
		newXML = attrRe.ReplaceAllString(newXML, payload+":${1}=")

		return newXML, nil

	case InsertionPointTypeWSXMLProcessing:
		// Replace processing instruction content
		piName := point.Name
		pattern := fmt.Sprintf(`(<\?%s)([^?]*)\?>`, regexp.QuoteMeta(piName))
		re := regexp.MustCompile(pattern)
		return re.ReplaceAllString(xmlStr, "${1} "+payload+"?>"), nil

	default:
		return "", fmt.Errorf("unsupported XML insertion point type: %s", point.Type)
	}
}
