package db

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

type severity string

func (s severity) String() string {
	return string(s)
}

const (
	Unknown  severity = "Unknown"
	Info     severity = "Info"
	Low      severity = "Low"
	Medium   severity = "Medium"
	High     severity = "High"
	Critical severity = "Critical"
)

func NewSeverity(s string) severity {
	switch strings.ToLower(s) {
	case "unknown":
		return Unknown
	case "info":
		return Info
	case "low":
		return Low
	case "medium":
		return Medium
	case "high":
		return High
	case "critical":
		return Critical
	default:
		return Unknown // or return an error, if you prefer
	}
}

func (s *severity) Scan(value interface{}) error {
	switch v := value.(type) {
	case []byte:
		*s = severity(v)
	case string:
		*s = severity(v)
	default:
		return fmt.Errorf("unsupported type: %T", v)
	}
	return nil
}

func (s severity) Value() (driver.Value, error) {
	return string(s), nil
}

const severityOrderQuery = `
		CASE 
			WHEN severity = 'Critical' THEN 1
			WHEN severity = 'High' THEN 2
			WHEN severity = 'Medium' THEN 3
			WHEN severity = 'Low' THEN 4
			WHEN severity = 'Info' THEN 5
			WHEN severity = 'Unknown' THEN 6
			ELSE 7
		END
	`

// Helper function to get severity order based on the given severity string
func GetSeverityOrder(severityStr string) int {
	switch severityStr {
	case "Critical":
		return 1
	case "High":
		return 2
	case "Medium":
		return 3
	case "Low":
		return 4
	case "Info":
		return 5
	case "Unknown":
		return 6
	default:
		return 7
	}
}
