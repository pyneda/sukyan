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