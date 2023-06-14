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
	unparsedSources := c.Query("sources")
	var statusCodes []int
	var httpMethods []string
	var sources []string
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

	if unparsedSources != "" {
		for _, source := range strings.Split(unparsedSources, ",") {
			if db.IsValidSource(source) {
				sources = append(sources, source)
			} else {
				log.Warn().Str("source", source).Msg("Invalid filter source provided")
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
		Sources:		 sources,
	})

	if err != nil {
		// Should handle this better
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": issues, "count": count})
}
