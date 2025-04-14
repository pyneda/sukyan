package db

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pyneda/sukyan/lib"
)

// PrintMaxURLLength max length a URL can have when printing as table
const PrintMaxURLLength = 65

// PrintMaxDescriptionLength max length a description can have when printing as table
const PrintMaxDescriptionLength = 150

func formatUintPointer(ptr *uint) string {
	if ptr != nil {
		return fmt.Sprintf("%d", *ptr)
	}
	return "N/A"
}

func PrintIssue(issue Issue) {
	var sb strings.Builder

	sb.WriteString(lib.Colorize("Title: ", lib.Blue) + issue.Title + "\n")
	sb.WriteString(lib.Colorize("Code: ", lib.Blue) + issue.Code + "\n")
	sb.WriteString(lib.Colorize("Severity: ", lib.Blue) + issue.Severity.String() + "\n")
	sb.WriteString(lib.Colorize("Info: ", lib.Blue))

	sb.WriteString("\n- " + lib.Colorize("URL: ", lib.Cyan) + issue.URL)
	sb.WriteString("\n- " + lib.Colorize("Method: ", lib.Cyan) + issue.HTTPMethod)
	sb.WriteString("\n- " + lib.Colorize("Payload: ", lib.Cyan) + issue.Payload)
	sb.WriteString("\n- " + lib.Colorize("Status code: ", lib.Cyan) + strconv.FormatInt(int64(issue.StatusCode), 10))
	sb.WriteString("\n- " + lib.Colorize("Confidence: ", lib.Cyan) + strconv.FormatInt(int64(issue.Confidence), 10) + "%")
	sb.WriteString("\n- " + lib.Colorize("False positive: ", lib.Cyan) + strconv.FormatBool(issue.FalsePositive) + "\n")

	sb.WriteString(lib.Colorize("Description: ", lib.Blue) + issue.Description + "\n")

	if issue.Note != "" {
		sb.WriteString(lib.Colorize("Note: ", lib.Yellow) + issue.Note + "\n")
	}

	if issue.CURLCommand != "" {
		sb.WriteString(lib.Colorize("CURL Command: ", lib.Blue) + issue.CURLCommand + "\n")
	}

	if issue.References != nil && len(issue.References) > 0 {
		sb.WriteString(lib.Colorize("References: ", lib.Blue))
		for _, ref := range issue.References {
			sb.WriteString("\n- " + ref)
		}
		sb.WriteString("\n")
	}

	if issue.Details != "" {

		sb.WriteString(lib.Colorize("Details: ", lib.Blue))
		sb.WriteString(issue.Details)
	}

	if len(issue.Interactions) > 0 {
		sb.WriteString(lib.Colorize("\nOOB Interactions: ", lib.Blue))
		for _, interaction := range issue.Interactions {
			sb.WriteString(PrintInteraction(interaction))
		}
	}

	if len(issue.Request) > 0 {
		sb.WriteString(lib.Colorize("\nRequest: ", lib.Blue) + "\n" + string(issue.Request) + "\n")
	}

	if len(issue.Response) > 0 {
		sb.WriteString(lib.Colorize("Response: ", lib.Blue) + "\n" + string(issue.Response) + "\n")
	}

	fmt.Print(sb.String())
}

func PrintInteraction(interaction OOBInteraction) string {
	var sb strings.Builder
	sb.WriteString("\n- " + lib.Colorize("Protocol: ", lib.Cyan) + interaction.Protocol)
	sb.WriteString("\n- " + lib.Colorize("Full ID: ", lib.Cyan) + interaction.FullID)
	sb.WriteString("\n- " + lib.Colorize("Unique ID: ", lib.Cyan) + interaction.UniqueID)
	sb.WriteString("\n- " + lib.Colorize("QType: ", lib.Cyan) + interaction.QType)
	// sb.WriteString("\n  " + lib.Colorize("Raw Response: ", lib.Cyan) + interaction.RawResponse)
	sb.WriteString("\n- " + lib.Colorize("Remote Address: ", lib.Cyan) + interaction.RemoteAddress)
	sb.WriteString("\n- " + lib.Colorize("Timestamp: ", lib.Cyan) + interaction.Timestamp.String())
	sb.WriteString("\n- " + lib.Colorize("Interaction Request: ", lib.Cyan) + interaction.RawRequest)
	return sb.String()
}

// PrintHistory prints a history record
func PrintHistory(history History) {
	var sb strings.Builder
	sb.WriteString("URL: " + history.URL)
	sb.WriteString("\nMethod: " + history.Method)
	sb.WriteString("\nContent Type: " + history.ResponseContentType)
	sb.WriteString("\nResponse Body:\n" + string(history.ResponseBodySize))
	fmt.Print(sb.String())
}
