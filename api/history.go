package api

import (
	"github.com/pyneda/sukyan/db"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)


func IsValidFilterHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", "TRACE":
		return true
	default:
		return false
	}
}


func FindHistory(c *gin.Context) {
	unparsedPageSize := c.DefaultQuery("page_size", "50")
	unparsedPage := c.DefaultQuery("page", "1")
	unparsedStatusCodes := c.Query("status")
	unparsedHttpMethods := c.Query("methods")
	var statusCodes []int
	var httpMethods []string
	log.Warn().Str("status", unparsedStatusCodes).Msg("status codes unparsed")

	pageSize, err := strconv.Atoi(unparsedPageSize)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing page size parameter query")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid page size parameter"})
		return

	}

	page, err := strconv.Atoi(unparsedPage)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing page parameter query")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid page parameter"})
		return
	}



	if unparsedStatusCodes != "" {
		for _, status := range strings.Split(unparsedStatusCodes, ",") {
			statusInt, err := strconv.Atoi(status)
			if err != nil {
				log.Error().Err(err).Msg("Error parsing page parameter query")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid status parameter"})
				return
			} else {
				statusCodes = append(statusCodes, statusInt)
			}
		}
	}

	if unparsedHttpMethods != "" {
		for _, method := range strings.Split(unparsedHttpMethods, ",") {
			if IsValidFilterHTTPMethod(method) {
				httpMethods = append(httpMethods, method)
			} else {
				log.Warn().Str("method", method).Msg("Invalid filter HTTP method provided")
			}
		}
	}
	issues, count, err := db.Connection.ListHistory(db.HistoryFilter{
		Pagination: db.Pagination{
			Page: page, PageSize: pageSize,
		},
		StatusCodes: statusCodes,
		Methods:     httpMethods,
	})

	if err != nil {
		// Should handle this better
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": issues, "count": count})
}
