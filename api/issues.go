package api

import (
	"github.com/pyneda/sukyan/db"
	"net/http"

	"github.com/gin-gonic/gin"
)

func FindIssues(c *gin.Context) {
	issues, count, err := db.Connection.ListIssues(db.IssueFilter{})
	if err != nil {
		// Should handle this better
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
	c.JSON(http.StatusOK, gin.H{"data": issues, "count": count})
}

func FindIssuesGrouped(c *gin.Context) {
	issues, err := db.Connection.ListIssuesGrouped(db.IssueFilter{})
	if err != nil {
		// Should handle this better
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
	c.JSON(http.StatusOK, gin.H{"data": issues})
}

func IssuesUI(c *gin.Context) {
	issues, count, err := db.Connection.ListIssues(db.IssueFilter{})
	if err != nil {
		// Should handle this better
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
	c.HTML(http.StatusOK, "issues.tmpl", gin.H{
		"title":  "Issues",
		"count":  count,
		"issues": issues,
	})
}
