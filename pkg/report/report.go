package report

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"time"

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
	WorkspaceID    uint
	Issues         []*db.Issue
	Title          string
	Format         ReportFormat
	TaskID         uint
	ScanID         uint
	MaxRequestSize int // Maximum size for requests in bytes (0 = no limit)
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

func mustAsset(name string) string {
	content, err := templates.ReadFile("templates/" + name)
	if err != nil {
		panic("report: missing embedded asset " + name + ": " + err.Error())
	}
	return string(content)
}

func generateHTMLReport(options ReportOptions, w io.Writer) error {
	funcMap := template.FuncMap{
		"add":    func(a, b int) int { return a + b },
		"styles": func() template.CSS { return template.CSS(mustAsset("report.css")) },
		"script": func() template.JS { return template.JS(mustAsset("report.js")) },
	}

	// Parse the template with the custom function map
	tmpl, err := template.New("report.tmpl").Funcs(funcMap).ParseFS(templates, "templates/report.tmpl")
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse report template")
		return err
	}

	if tmpl.DefinedTemplates() == "" {
		return fmt.Errorf("no defined templates found")
	}

	// Process and organize the issues
	reportIssues := processIssues(options.Issues, options.MaxRequestSize)
	groupedIssues := groupIssuesByType(reportIssues)

	summary := generateSummary(reportIssues)

	generatedAt := time.Now().Format("2006-01-02 15:04:05")

	// Prepare data for the template
	data := HTMLReportData{
		Title:         options.Title,
		Summary:       summary,
		Issues:        reportIssues,
		GroupedIssues: groupedIssues,
		GeneratedAt:   generatedAt,
		SeverityDonut: severityDonut(summary),
		TopTypes:      topTypeBars(summary),
		Payload: ReportPayload{
			Title:         options.Title,
			GeneratedAt:   generatedAt,
			Summary:       summary,
			GroupedIssues: groupedIssues,
		},
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Error().Err(err).Msg("Failed to execute report template")
		return err
	}

	return nil
}

func generateJSONReport(options ReportOptions, w io.Writer) error {
	// Process issues with max request size
	reportIssues := processIssues(options.Issues, options.MaxRequestSize)

	data := map[string]interface{}{
		"title":       options.Title,
		"workspaceID": options.WorkspaceID,
		"issues":      reportIssues,
	}

	encoder := json.NewEncoder(w)
	if err := encoder.Encode(data); err != nil {
		return err
	}

	return nil
}
