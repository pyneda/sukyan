package report

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

//go:embed templates/*
var templates embed.FS

type ReportFormat string

const (
	ReportFormatHTML ReportFormat = "html"
	ReportFormatJSON ReportFormat = "json"
)

type ReportOptions struct {
	WorkspaceID uint
	Issues      []*db.Issue
	Title       string
	Format      ReportFormat
	TaskID      uint
}

func GenerateReport(options ReportOptions, w io.Writer) error {
	switch options.Format {
	case ReportFormatHTML:
		return generateHTMLReport(options, w)
	case ReportFormatJSON:
		return generateJSONReport(options, w)
	default:
		return errors.New("invalid report format")
	}
}

func generateHTMLReport(options ReportOptions, w io.Writer) error {
	funcMap := template.FuncMap{
		"toString": toString,
		"toJSON":   toJSON,
	}

	// Parsing the template with the custom function map
	tmpl, err := template.New("report.tmpl").Funcs(funcMap).ParseFS(templates, "templates/report.tmpl")
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse report template")
		return err
	}

	if tmpl.DefinedTemplates() == "" {
		return fmt.Errorf("no defined templates found")
	}

	data := map[string]interface{}{
		"title":  options.Title,
		"issues": options.Issues,
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Error().Err(err).Msg("Failed to execute report template")
		return err
	}

	return nil
}

func generateJSONReport(options ReportOptions, w io.Writer) error {
	data := map[string]interface{}{
		"title":       options.Title,
		"workspaceID": options.WorkspaceID,
		"issues":      options.Issues,
	}

	encoder := json.NewEncoder(w)
	if err := encoder.Encode(data); err != nil {
		return err
	}

	return nil
}

func toString(value interface{}) string {
	switch v := value.(type) {
	case []byte:
		return string(v)
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}
func toJSON(value interface{}) template.JS {
	bytes, err := json.Marshal(value)
	if err != nil {
		return template.JS("{}")
	}
	return template.JS(bytes)
}
