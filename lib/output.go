package lib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/olekukonko/tablewriter"

	"gopkg.in/yaml.v3"
)

type FormatType string

const (
	Pretty FormatType = "pretty"
	Text   FormatType = "text"
	JSON   FormatType = "json"
	YAML   FormatType = "yaml"
	Table  FormatType = "table"
)

type Formattable interface {
	String() string
	Pretty() string
	TableHeaders() []string
	TableRow() []string
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
	case Table:
		var tableData [][]string
		for _, item := range data {
			row := item.TableRow()
			tableData = append(tableData, row)
		}

		buffer := new(bytes.Buffer)
		table := tablewriter.NewWriter(buffer)

		if len(data) > 0 {
			table.SetHeader(data[0].TableHeaders())
		}
		table.SetBorder(true)
		table.AppendBulk(tableData)
		table.Render()

		return buffer.String(), nil
	default:
		return "", fmt.Errorf("unknown format: %v", format)
	}
}

func FormatSingleOutput[T Formattable](data T, format FormatType) (string, error) {
	switch format {
	case Text:
		return data.String(), nil
	case Pretty:
		return data.Pretty(), nil
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
	case Table:
		buffer := new(bytes.Buffer)
		table := tablewriter.NewWriter(buffer)
		table.SetHeader(data.TableHeaders())
		table.Append(data.TableRow())
		table.SetBorder(true)
		table.Render()
		return buffer.String(), nil
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
	case "table":
		return Table, nil
	default:
		return "", fmt.Errorf("unknown format: %s", format)
	}
}
