package api

import (
	"net/http"
	"strconv"
	"strings"
	"sukyan/db"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func FindHistory(c *gin.Context) {
	unparsedPageSize := c.DefaultQuery("page-size", "50")
	unparsedPage := c.DefaultQuery("page", "1")
	unparsedStatusCodes := c.Query("status")
	var statusCodes []int
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

	issues, count, err := db.Connection.ListHistory(db.HistoryFilter{
		Pagination: db.Pagination{
			Page: page, PageSize: pageSize,
		},
		StatusCodes: statusCodes,
	})

	if err != nil {
		// Should handle this better
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": issues, "count": count})
}
