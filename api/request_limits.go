package api

import (
	"net/http"

	"github.com/alireza0/s-ui/database"

	"github.com/gin-gonic/gin"
)

const apiDefaultRequestMaxBytes int64 = 8 * 1024 * 1024

func applyAPIRequestBodyLimit(c *gin.Context, action string) {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, apiRequestBodyLimit(action))
}

func apiRequestBodyLimit(action string) int64 {
	switch action {
	case "importdb":
		return database.MaxDatabaseImportUploadBytes()
	case "restore-db-backup":
		return database.MaxDBBackupArchiveUploadBytes()
	default:
		return apiDefaultRequestMaxBytes
	}
}
