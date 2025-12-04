package openapi

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
)

//go:embed templates/*
var templates embed.FS

// ReportFormat defines the output format
type ReportFormat string

const (
	ReportFormatHTML ReportFormat = "html"
	ReportFormatJSON ReportFormat = "json"
)

// GenerateReport generates a report for the given endpoints
func GenerateReport(endpoints []Endpoint, format ReportFormat, w io.Writer) error {
	switch format {
	case ReportFormatJSON:
		return generateJSONReport(endpoints, w)
	case ReportFormatHTML:
		return generateHTMLReport(endpoints, w)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func generateJSONReport(endpoints []Endpoint, w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(endpoints)
}

func generateHTMLReport(endpoints []Endpoint, w io.Writer) error {
	funcMap := template.FuncMap{
		"toJSON": func(v interface{}) template.JS {
			b, _ := json.Marshal(v)
			return template.JS(b)
		},
	}

	tmpl, err := template.New("openapi_report.tmpl").Funcs(funcMap).ParseFS(templates, "templates/openapi_report.tmpl")
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	data := struct {
		Endpoints []Endpoint
	}{
		Endpoints: endpoints,
	}

	return tmpl.Execute(w, data)
}
