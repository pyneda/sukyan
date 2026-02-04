package graphql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/rs/zerolog/log"
)

type SensitiveFieldsAudit struct {
	Options     *GraphQLAuditOptions
	Definition  *db.APIDefinition
	BaseHistory *db.History
}

type sensitiveFieldProbe struct {
	field       string
	category    string
	description string
	severity    string
}

func getSensitiveFieldProbes() []sensitiveFieldProbe {
	return []sensitiveFieldProbe{
		{field: "password", category: "auth", description: "Password field", severity: "critical"},
		{field: "passwordHash", category: "auth", description: "Password hash field", severity: "critical"},
		{field: "hashedPassword", category: "auth", description: "Hashed password field", severity: "critical"},
		{field: "secret", category: "auth", description: "Secret field", severity: "critical"},
		{field: "secretKey", category: "auth", description: "Secret key field", severity: "critical"},
		{field: "apiKey", category: "auth", description: "API key field", severity: "critical"},
		{field: "apiSecret", category: "auth", description: "API secret field", severity: "critical"},
		{field: "token", category: "auth", description: "Token field", severity: "high"},
		{field: "accessToken", category: "auth", description: "Access token field", severity: "critical"},
		{field: "refreshToken", category: "auth", description: "Refresh token field", severity: "critical"},
		{field: "sessionToken", category: "auth", description: "Session token field", severity: "critical"},
		{field: "privateKey", category: "auth", description: "Private key field", severity: "critical"},
		{field: "encryptionKey", category: "auth", description: "Encryption key field", severity: "critical"},

		{field: "ssn", category: "pii", description: "Social Security Number", severity: "critical"},
		{field: "socialSecurityNumber", category: "pii", description: "Social Security Number", severity: "critical"},
		{field: "creditCard", category: "pii", description: "Credit card field", severity: "critical"},
		{field: "creditCardNumber", category: "pii", description: "Credit card number", severity: "critical"},
		{field: "cvv", category: "pii", description: "CVV field", severity: "critical"},
		{field: "cardNumber", category: "pii", description: "Card number field", severity: "critical"},
		{field: "bankAccount", category: "pii", description: "Bank account field", severity: "high"},
		{field: "taxId", category: "pii", description: "Tax ID field", severity: "high"},

		{field: "__debug", category: "internal", description: "Debug field", severity: "medium"},
		{field: "__internal", category: "internal", description: "Internal field", severity: "medium"},
		{field: "_private", category: "internal", description: "Private field", severity: "medium"},
		{field: "debug", category: "internal", description: "Debug field", severity: "low"},
		{field: "internal", category: "internal", description: "Internal field", severity: "low"},
		{field: "config", category: "internal", description: "Config field", severity: "medium"},
		{field: "configuration", category: "internal", description: "Configuration field", severity: "medium"},
		{field: "settings", category: "internal", description: "Settings field", severity: "low"},
		{field: "env", category: "internal", description: "Environment field", severity: "medium"},
		{field: "environment", category: "internal", description: "Environment field", severity: "medium"},

		{field: "admin", category: "admin", description: "Admin field", severity: "medium"},
		{field: "adminPanel", category: "admin", description: "Admin panel field", severity: "medium"},
		{field: "isAdmin", category: "admin", description: "Admin flag field", severity: "medium"},
		{field: "isSuperuser", category: "admin", description: "Superuser flag field", severity: "medium"},
		{field: "role", category: "admin", description: "Role field", severity: "low"},
		{field: "permissions", category: "admin", description: "Permissions field", severity: "medium"},
		{field: "allUsers", category: "admin", description: "All users query", severity: "medium"},
		{field: "deleteUser", category: "admin", description: "Delete user mutation", severity: "high"},
	}
}

func (a *SensitiveFieldsAudit) Run() {
	auditLog := log.With().
		Str("audit", "graphql-sensitive-fields").
		Uint("workspace", a.Options.WorkspaceID).
		Logger()

	if a.Options.Ctx != nil {
		select {
		case <-a.Options.Ctx.Done():
			auditLog.Debug().Msg("Context cancelled, skipping sensitive fields audit")
			return
		default:
		}
	}

	if a.Definition == nil {
		return
	}

	baseURL := a.Definition.BaseURL
	if baseURL == "" {
		baseURL = a.Definition.SourceURL
	}

	auditLog.Info().Str("url", baseURL).Msg("Starting GraphQL sensitive fields audit")

	client := a.Options.HTTPClient
	if client == nil {
		client = http_utils.CreateHttpClient()
	}

	probes := getSensitiveFieldProbes()

	type discoveredField struct {
		probe   sensitiveFieldProbe
		history *db.History
	}
	discovered := make(map[string]discoveredField)

	for _, probe := range probes {
		if a.Options.Ctx != nil {
			select {
			case <-a.Options.Ctx.Done():
				return
			default:
			}
		}

		queryHistory := a.probeField(baseURL, client, probe, "query")
		if queryHistory != nil {
			discovered[probe.field] = discoveredField{probe: probe, history: queryHistory}
		}

		if probe.category == "admin" {
			mutationHistory := a.probeField(baseURL, client, probe, "mutation")
			if mutationHistory != nil && queryHistory == nil {
				discovered[probe.field] = discoveredField{probe: probe, history: mutationHistory}
			}
		}
	}

	if len(discovered) == 0 {
		auditLog.Info().Msg("No sensitive fields discovered")
		return
	}

	var criticalFields, highFields, mediumFields, lowFields []string
	for field, df := range discovered {
		switch df.probe.severity {
		case "critical":
			criticalFields = append(criticalFields, field)
		case "high":
			highFields = append(highFields, field)
		case "medium":
			mediumFields = append(mediumFields, field)
		case "low":
			lowFields = append(lowFields, field)
		}
	}

	confidence := 65
	if len(criticalFields) > 0 {
		confidence = 90
	} else if len(highFields) > 0 {
		confidence = 80
	}

	details := fmt.Sprintf("Discovered %d potentially sensitive fields through query probing.\n\n"+
		"Fields by severity:\n%s\n"+
		"Field existence was confirmed through query probing. Manual verification recommended to assess actual data exposure.",
		len(discovered), formatFieldsBySeverity(criticalFields, highFields, mediumFields, lowFields))

	var firstHistory *db.History
	var additionalHistories []*db.History
	for _, df := range discovered {
		if firstHistory == nil {
			firstHistory = df.history
		} else {
			additionalHistories = append(additionalHistories, df.history)
		}
	}

	issue, err := db.CreateIssueFromHistoryAndTemplate(
		firstHistory,
		db.GraphqlSensitiveFieldsExposedCode,
		details,
		confidence,
		"",
		&a.Options.WorkspaceID,
		&a.Options.TaskID,
		&a.Options.TaskJobID,
		&a.Options.ScanID,
		&a.Options.ScanJobID,
	)
	if err != nil {
		auditLog.Error().Err(err).Msg("Failed to create sensitive fields issue")
		return
	}

	if len(additionalHistories) > 0 {
		if err := issue.AppendHistories(additionalHistories); err != nil {
			auditLog.Warn().Err(err).Uint("issue_id", issue.ID).Msg("Failed to link additional histories to issue")
		}
	}

	auditLog.Info().Uint("issue_id", issue.ID).Int("fields_discovered", len(discovered)).Msg("Created sensitive fields issue")
}

func (a *SensitiveFieldsAudit) probeField(baseURL string, client *http.Client, probe sensitiveFieldProbe, opType string) *db.History {
	var query string
	if opType == "mutation" {
		query = fmt.Sprintf(`{"query":"mutation{%s}"}`, probe.field)
	} else {
		query = fmt.Sprintf(`{"query":"query{%s}"}`, probe.field)
	}

	req, err := http.NewRequestWithContext(a.Options.Ctx, "POST", baseURL, bytes.NewBufferString(query))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")

	result := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		Client:        client,
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: a.Options.WorkspaceID,
			ScanID:      a.Options.ScanID,
			ScanJobID:   a.Options.ScanJobID,
		},
	})

	if result.Err != nil || result.History == nil {
		return nil
	}

	body, _ := result.History.ResponseBody()

	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil
	}

	if data, ok := response["data"].(map[string]interface{}); ok {
		if _, fieldExists := data[probe.field]; fieldExists {
			return result.History
		}
	}

	return nil
}

func formatFieldsBySeverity(critical, high, medium, low []string) string {
	var sb strings.Builder

	if len(critical) > 0 {
		sb.WriteString("CRITICAL:\n")
		for _, f := range critical {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}
	if len(high) > 0 {
		sb.WriteString("HIGH:\n")
		for _, f := range high {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}
	if len(medium) > 0 {
		sb.WriteString("MEDIUM:\n")
		for _, f := range medium {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}
	if len(low) > 0 {
		sb.WriteString("LOW:\n")
		for _, f := range low {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}

	if sb.Len() == 0 {
		return "(none detected)\n"
	}
	return sb.String()
}
