package lib

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io/ioutil"
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

func FormatOutput[T Formattable](data T, format FormatType) (string, error) {
	switch format {
	case Text:
		return data.String(), nil
	case Pretty:
		return data.Pretty(), nil
	case JSON:
		j, err := json.MarshalIndent(data, "", "  ")
		return string(j), err
	case YAML:
		y, err := yaml.Marshal(data)
		return string(y), err
	default:
		return "", fmt.Errorf("unknown format: %v", format)
	}
}

func FormatOutputToFile[T Formattable](data T, format FormatType, filepath string) error {
	formattedData, err := FormatOutput(data, format)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filepath, []byte(formattedData), 0644)
}
