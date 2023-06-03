package db

import (
	"gorm.io/gorm"
)

// Represents a template for an issue
type IssueTemplate struct {
	gorm.Model
	Code        string
	Title       string
	Description string
	Reference   string
}
