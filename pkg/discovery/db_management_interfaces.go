package discovery

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
)

var DBManagementPaths = []string{
	// phpMyAdmin
	"phpmyadmin/",
	"pma/",
	"myadmin/",
	"mysql/",
	"phpMyAdmin/",
	"MySQLAdmin/",
	"phpMyAdmin-latest/",
	"phpmyadmin4/",
	"sql/",
	"db/",
	"database/",

	// pgAdmin
	"pgadmin/",
	"pgadmin4/",
	"pgsql/",
	"postgres/",
	"postgresql/",
	"phppgadmin/",

	// MongoDB
	"mongo-express/",
	"mongodb/",
	"mongo/",
	"mongoadmin/",

	// Adminer
	"adminer/",
	"adminer.php",
	"adminer-4.8.1.php",
	"adminer-4.php",
	"db-admin/",

	// SQLite
	"sqlite/",
	"sqlitemanager/",
	"sqlite-browser/",
	"sqlitebrowser/",

	// Redis
	"redis-commander/",
	"phpredisadmin/",
	"redis-admin/",
	"redisadmin/",
	"redis/",

	// Elasticsearch
	"elasticsearch/",
	"elastic/",
	"kibana/",
	"_cat/indices",
	"_cluster/health",

	// Common extra paths
	"dbadmin/",
	"admin/db/",
	"database-admin/",
	"db-manager/",
}

func IsDBManagementValidationFunc(history *db.History) (bool, string, int) {
	bodyStr := strings.ToLower(string(history.ResponseBody))
	details := fmt.Sprintf("Database management interface found: %s\n", history.URL)
	confidence := 0

	// Check response headers for DB management indicators
	headers, _ := history.GetResponseHeadersAsMap()
	for header, values := range headers {
		headerStr := strings.ToLower(strings.Join(values, " "))
		if strings.Contains(headerStr, "phpmyadmin") ||
			strings.Contains(headerStr, "pgadmin") ||
			strings.Contains(headerStr, "adminer") {
			confidence += 20
			details += fmt.Sprintf("- DB management header found: %s\n", header)
		}
	}

	switch history.StatusCode {
	case 200, 301, 302, 307, 308:
		confidence += 10
	case 401, 403:
		confidence += 15
		details += "- Authentication required (typical for DB management interface)\n"
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(history.ResponseBody))
	if err == nil {
		forms := doc.Find("form")
		forms.Each(func(i int, form *goquery.Selection) {
			formMatches := 0
			loginIndicators := 0

			action, _ := form.Attr("action")
			actionLower := strings.ToLower(action)
			if strings.Contains(actionLower, "login") ||
				strings.Contains(actionLower, "auth") ||
				strings.Contains(actionLower, "authenticate") {
				loginIndicators++
			}

			inputs := form.Find("input")
			dbFormFields := map[string]struct{}{
				"username":     {},
				"user":         {},
				"password":     {},
				"db":           {},
				"database":     {},
				"host":         {},
				"port":         {},
				"server":       {},
				"auth_type":    {},
				"login":        {},
				"pma_username": {}, // phpMyAdmin specific
				"pma_password": {},
				"pg_username":  {}, // pgAdmin specific
				"pg_password":  {},
			}

			inputs.Each(func(j int, input *goquery.Selection) {
				if name, exists := input.Attr("name"); exists {
					nameLower := strings.ToLower(name)
					if _, isDbField := dbFormFields[nameLower]; isDbField {
						formMatches++
					}

					// Check placeholder and value attributes
					if placeholder, exists := input.Attr("placeholder"); exists {
						placeholderLower := strings.ToLower(placeholder)
						if strings.Contains(placeholderLower, "database") ||
							strings.Contains(placeholderLower, "server") ||
							strings.Contains(placeholderLower, "host") {
							formMatches++
						}
					}
				}
			})

			// Check for database-related select fields
			selects := form.Find("select")
			selects.Each(func(j int, select_ *goquery.Selection) {
				if name, exists := select_.Attr("name"); exists {
					nameLower := strings.ToLower(name)
					if strings.Contains(nameLower, "database") ||
						strings.Contains(nameLower, "auth_type") ||
						strings.Contains(nameLower, "server") {
						formMatches++
					}
				}
			})

			// Check for database-related labels
			form.Find("label").Each(func(j int, label *goquery.Selection) {
				labelText := strings.ToLower(label.Text())
				if strings.Contains(labelText, "database") ||
					strings.Contains(labelText, "server") ||
					strings.Contains(labelText, "host") ||
					strings.Contains(labelText, "port") {
					formMatches++
				}
			})

			if formMatches >= 2 {
				confidence += 20
				details += fmt.Sprintf("- Found login form with %d database-related elements\n", formMatches)
			}
			if loginIndicators > 0 {
				confidence += 10
				details += "- Form contains login indicators\n"
			}
		})

		interfaceElements := map[string]string{
			"#pma_navigation":    "phpMyAdmin navigation",
			"#pma_console":       "phpMyAdmin console",
			"#pgAdminContainer":  "pgAdmin container",
			".adminer":           "Adminer interface",
			".mongo-express":     "Mongo Express interface",
			"#redis-commander":   "Redis Commander",
			".elastic-container": "Elasticsearch interface",
		}

		for selector, description := range interfaceElements {
			if doc.Find(selector).Length() > 0 {
				confidence += 25
				details += fmt.Sprintf("- Found %s\n", description)
			}
		}

		title := doc.Find("title").Text()
		titleLower := strings.ToLower(title)
		if strings.Contains(titleLower, "database") ||
			strings.Contains(titleLower, "phpmyadmin") ||
			strings.Contains(titleLower, "pgadmin") ||
			strings.Contains(titleLower, "adminer") ||
			strings.Contains(titleLower, "sql") {
			confidence += 15
			details += fmt.Sprintf("- Database-related page title: %s\n", title)
		}
	}

	dbPatterns := map[string]map[string]string{
		"phpMyAdmin": {
			"phpmyadmin":       "main identifier",
			"mysql":            "database reference",
			"mariadb":          "database reference",
			"server_databases": "database listing",
		},
		"pgAdmin": {
			"pgadmin":    "main identifier",
			"postgresql": "database reference",
			"psql":       "command reference",
		},
		"Adminer": {
			"adminer":    "main identifier",
			"login-form": "login form",
		},
		"MongoDB": {
			"mongo-express": "main identifier",
			"mongodb":       "database reference",
		},
		"Redis": {
			"redis-commander": "main identifier",
			"redis":           "database reference",
		},
		"Elasticsearch": {
			"elasticsearch": "main identifier",
			"kibana":        "interface reference",
		},
	}

	for system, patterns := range dbPatterns {
		systemMatches := 0
		for pattern, description := range patterns {
			if strings.Contains(bodyStr, pattern) {
				systemMatches++
				details += fmt.Sprintf("- Found %s %s\n", system, description)
			}
		}
		if systemMatches > 0 {
			confidence += min(systemMatches*10, 30)
		}
	}

	if confidence >= 40 {
		return true, details, min(confidence, 100)
	}

	return false, "", 0
}

func DiscoverDBManagementInterfaces(baseURL string, opts http_utils.HistoryCreationOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         baseURL,
			Method:      "GET",
			Paths:       DBManagementPaths,
			Concurrency: 10,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
			},
			HistoryCreationOptions: opts,
		},
		ValidationFunc: IsDBManagementValidationFunc,
		IssueCode:      db.DbManagementInterfaceDetectedCode,
	})
}
