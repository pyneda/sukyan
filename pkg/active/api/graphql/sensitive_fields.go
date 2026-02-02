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

// SensitiveFieldsAudit tests for exposed sensitive GraphQL fields
type SensitiveFieldsAudit struct {
	Options     *GraphQLAuditOptions
	Definition  *db.APIDefinition
	BaseHistory *db.History
}

// sensitiveFieldProbe represents a sensitive field to probe
type sensitiveFieldProbe struct {
	field       string
	category    string
	description string
	severity    string
}

// getSensitiveFieldProbes returns fields to probe for sensitive data
func getSensitiveFieldProbes() []sensitiveFieldProbe {
	return []sensitiveFieldProbe{
		// Authentication/Authorization
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

		// PII
		{field: "ssn", category: "pii", description: "Social Security Number", severity: "critical"},
		{field: "socialSecurityNumber", category: "pii", description: "Social Security Number", severity: "critical"},
		{field: "creditCard", category: "pii", description: "Credit card field", severity: "critical"},
		{field: "creditCardNumber", category: "pii", description: "Credit card number", severity: "critical"},
		{field: "cvv", category: "pii", description: "CVV field", severity: "critical"},
		{field: "cardNumber", category: "pii", description: "Card number field", severity: "critical"},
		{field: "bankAccount", category: "pii", description: "Bank account field", severity: "high"},
		{field: "taxId", category: "pii", description: "Tax ID field", severity: "high"},

		// Internal/Debug
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

		// Admin/Privileged
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

// Run executes the sensitive fields audit
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
	discoveredFields := make(map[string]sensitiveFieldProbe)

	for _, probe := range probes {
		if a.Options.Ctx != nil {
			select {
			case <-a.Options.Ctx.Done():
				return
			default:
			}
		}

		// Test as query field
		queryResult := a.probeField(baseURL, client, probe, "query")
		if queryResult != nil {
			discoveredFields[probe.field] = probe
		}

		// Test as mutation (for action-based fields)
		if probe.category == "admin" {
			mutationResult := a.probeField(baseURL, client, probe, "mutation")
			if mutationResult != nil && queryResult == nil {
				discoveredFields[probe.field] = probe
			}
		}
	}

	// Group findings by severity
	if len(discoveredFields) > 0 {
		criticalFields := []string{}
		highFields := []string{}
		mediumFields := []string{}
		lowFields := []string{}

		for field, probe := range discoveredFields {
			switch probe.severity {
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

		// Create single consolidated report
		confidence := 65
		if len(criticalFields) > 0 {
			confidence = 90
		} else if len(highFields) > 0 {
			confidence = 80
		}

		details := fmt.Sprintf(`Potentially sensitive GraphQL fields are accessible.

Request URL: %s

Discovered Fields by Severity:
%s
Impact:
- Information disclosure
- Credential/token exposure
- Privacy violations (PII)
- Privilege escalation vectors

Note: Field existence was confirmed through query probing.
Manual verification recommended to assess actual data exposure.

Remediation:
- Implement field-level authorization
- Remove sensitive fields from public schema
- Use field filtering/masking
- Audit all exposed fields`, baseURL, formatFieldsBySeverity(criticalFields, highFields, mediumFields, lowFields))

		// We need a history for reporting - use the first discovered field's history
		// For now, we'll create a minimal issue without history
		reportIssue(nil, db.GraphqlIntrospectionEnabledCode, details, confidence, a.Options)
	}

	auditLog.Info().Int("fields_discovered", len(discoveredFields)).Msg("Completed GraphQL sensitive fields audit")
}

// probeField tests if a specific field exists and is accessible
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

	// Check if data was returned for this field
	if data, ok := response["data"].(map[string]interface{}); ok {
		if _, fieldExists := data[probe.field]; fieldExists {
			return result.History
		}
	}

	// Also check if field exists but returned null (still means it's in schema)
	if data, ok := response["data"].(map[string]interface{}); ok {
		// Field exists if it's a key, even with null value
		for key := range data {
			if key == probe.field {
				return result.History
			}
		}
	}

	return nil
}

// formatFieldsBySeverity formats fields grouped by severity
func formatFieldsBySeverity(critical, high, medium, low []string) string {
	var sb strings.Builder

	if len(critical) > 0 {
		sb.WriteString("\n🔴 CRITICAL:\n")
		for _, f := range critical {
			sb.WriteString(fmt.Sprintf("   - %s\n", f))
		}
	}
	if len(high) > 0 {
		sb.WriteString("\n🟠 HIGH:\n")
		for _, f := range high {
			sb.WriteString(fmt.Sprintf("   - %s\n", f))
		}
	}
	if len(medium) > 0 {
		sb.WriteString("\n🟡 MEDIUM:\n")
		for _, f := range medium {
			sb.WriteString(fmt.Sprintf("   - %s\n", f))
		}
	}
	if len(low) > 0 {
		sb.WriteString("\n🟢 LOW:\n")
		for _, f := range low {
			sb.WriteString(fmt.Sprintf("   - %s\n", f))
		}
	}

	if sb.Len() == 0 {
		return "  (none detected)"
	}
	return sb.String()
}
