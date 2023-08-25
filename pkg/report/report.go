package report

import (
	"embed"
	"encoding/json"
	"errors"
	"github.com/pyneda/sukyan/db"
	"html/template"
	"io"
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
	tmpl, err := template.ParseFS(templates, "templates/report.tmpl")
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"title":  options.Title,
		"issues": options.Issues,
	}

	if err := tmpl.Execute(w, data); err != nil {
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
