package api

import (
	"github.com/pyneda/sukyan/db"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func FindInteractions(c *gin.Context) {
	unparsedPageSize := c.DefaultQuery("page-size", "50")
	unparsedPage := c.DefaultQuery("page", "1")
	unparsedProtocols := c.Query("protocols")
	var protocols []string
	log.Warn().Str("protocols", unparsedProtocols).Msg("protocols unparsed")

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

	if unparsedProtocols != "" {
		for _, protocol := range strings.Split(unparsedProtocols, ",") {
			if err != nil {
				log.Error().Err(err).Msg("Error parsing page parameter query")
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid status parameter"})
				return
			} else {
				protocols = append(protocols, protocol)
			}
		}
	}

	issues, count, err := db.Connection.ListInteractions(db.InteractionsFilter{
		Pagination: db.Pagination{
			Page: page, PageSize: pageSize,
		},
		Protocols: protocols,
	})

	if err != nil {
		// Should handle this better
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": issues, "count": count})
}
