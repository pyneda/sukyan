package db

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/olekukonko/tablewriter"
)

// PrintMaxURLLength max length a URL can have when printing as table
const PrintMaxURLLength = 65

// PrintMaxDescriptionLength max length a description can have when printing as table
const PrintMaxDescriptionLength = 200

// PrintHistoryTable prints a list of history records as a table
func PrintHistoryTable(records []*History) {
	var tableData [][]string
	for _, record := range records {
		formattedURL := record.URL
		if len(record.URL) > PrintMaxURLLength {
			formattedURL = record.URL[0:PrintMaxURLLength] + "..."
		}

		tableData = append(tableData, []string{
			strconv.FormatUint(uint64(record.ID), 10),
			formattedURL,
			strconv.Itoa(record.StatusCode),
			record.Method,
			record.ContentType,
		})
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "URL", "Status", "Method", "Content Type"})
	table.SetBorder(true)
	table.AppendBulk(tableData)
	table.Render()
}

// PrintIssueTable print a list of issues as a table
func PrintIssueTable(records []*Issue) {
	var tableData [][]string
	for _, record := range records {
		formattedURL := record.URL
		if len(record.URL) > PrintMaxURLLength {
			formattedURL = record.URL[0:PrintMaxURLLength] + "..."
		}
		formattedDescription := record.Description
		if len(record.Description) > PrintMaxDescriptionLength {
			formattedDescription = record.Description[0:PrintMaxDescriptionLength] + "..."
		}
		tableData = append(tableData, []string{
			strconv.FormatUint(uint64(record.ID), 10),
			record.Code,
			record.Title,
			formattedURL,
			record.HTTPMethod,
			formattedDescription,
		})
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Code", "Title", "URL", "Method", "Description"})
	table.SetBorder(true)
	table.AppendBulk(tableData)
	table.Render()
}

// PrintIssue prints an issue record
func PrintIssue(issue Issue) {
	var sb strings.Builder
	sb.WriteString("Title: " + issue.Title + "\nCode: " + issue.Code + "\n")
	sb.WriteString("Description: " + issue.Description)
	if issue.Note != "" {
		sb.WriteString("Note: " + issue.Note)
	}
	sb.WriteString("Details: \n- URL: " + issue.URL + "\n- Method: " + issue.HTTPMethod + "\n- Payload: " + issue.Payload + "\n- Status code: " + strconv.FormatInt(int64(issue.StatusCode), 10))
	sb.WriteString("\n- Confidence: " + strconv.FormatInt(int64(issue.Confidence), 10) + "%\n- False positive: " + strconv.FormatBool(issue.FalsePositive))
	sb.WriteString("\nRequest: \n" + issue.Request)
	sb.WriteString("\nResponse: \n" + issue.Response)
	sb.WriteString("\n")
	fmt.Print(sb.String())
}

// PrintHistory prints a history record
func PrintHistory(history History) {
	var sb strings.Builder
	sb.WriteString("URL: " + history.URL)
	sb.WriteString("\nMethod: " + history.Method)
	sb.WriteString("\nContent Type: " + history.ContentType)
	sb.WriteString("\nResponse Body:\n" + history.ResponseBody)
	fmt.Print(sb.String())
}
