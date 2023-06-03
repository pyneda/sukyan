package api

import (
	"net/http"
	"sukyan/db"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// StartAPI starts the api
func StartAPI() {
	db.InitDb()
	r := gin.Default()
	// This allows all cors, should probably allow configure it via config and provide strict default
	r.Use(cors.Default())
	r.LoadHTMLGlob("templates/*")

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": "API Running"})
	})
	r.GET("/index", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl", gin.H{
			"title": "Main website",
		})
	})
	r.GET("/ui/issues", IssuesUI)

	r.GET("/issues", FindIssues)
	r.GET("/issues/grouped", FindIssuesGrouped)
	r.GET("/history", FindHistory)

	r.Run()
}
