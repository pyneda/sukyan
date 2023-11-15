package lib

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"strings"
)

type FormatType string

const (
	Pretty FormatType = "pretty"
	Text   FormatType = "text"
	JSON   FormatType = "json"
	YAML   FormatType = "yaml"
)

type Formattable interface {
	String() string
	Pretty() string
}

func FormatOutput[T Formattable](data []T, format FormatType) (string, error) {
	switch format {
	case Text:
		var textOutput []string
		for _, item := range data {
			textOutput = append(textOutput, item.String())
		}
		return strings.Join(textOutput, "\n"), nil
	case Pretty:
		var prettyOutput []string
		for _, item := range data {
			prettyOutput = append(prettyOutput, item.Pretty())
		}
		return strings.Join(prettyOutput, "\n"), nil
	case JSON:
		j, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return "", err
		}
		return string(j), nil
	case YAML:
		y, err := yaml.Marshal(data)
		if err != nil {
			return "", err
		}
		return string(y), nil
	default:
		return "", fmt.Errorf("unknown format: %v", format)
	}
}

func FormatOutputToFile[T Formattable](data []T, format FormatType, filepath string) error {
	formattedData, err := FormatOutput(data, format)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath, []byte(formattedData), 0644)
}

// ParseFormatType converts a string format to a FormatType.
func ParseFormatType(format string) (FormatType, error) {
	normalizedFormat := strings.ToLower(format)
	switch normalizedFormat {
	case "pretty":
		return Pretty, nil
	case "text":
		return Text, nil
	case "json":
		return JSON, nil
	case "yaml":
		return YAML, nil
	default:
		return "", fmt.Errorf("unknown format: %s", format)
	}
}
