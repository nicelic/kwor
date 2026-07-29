package api

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

func parseCoreVersionWindowPagination(c *gin.Context) (string, int, int) {
	channel := strings.TrimSpace(c.Query("channel"))
	if channel == "" {
		channel = "stable"
	}

	offset := 0
	limit := 5
	if offsetRaw := strings.TrimSpace(c.Query("offset")); offsetRaw != "" {
		if parsed, err := strconv.Atoi(offsetRaw); err == nil && parsed >= 0 {
			offset = parsed
		}
		if limitRaw := strings.TrimSpace(c.Query("limit")); limitRaw != "" {
			if parsed, err := strconv.Atoi(limitRaw); err == nil && parsed > 0 {
				limit = parsed
			}
		} else if perPageRaw := strings.TrimSpace(c.Query("per_page")); perPageRaw != "" {
			if parsed, err := strconv.Atoi(perPageRaw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
	} else {
		page := 1
		if pageRaw := strings.TrimSpace(c.Query("page")); pageRaw != "" {
			if parsed, err := strconv.Atoi(pageRaw); err == nil && parsed > 0 {
				page = parsed
			}
		}
		if perPageRaw := strings.TrimSpace(c.Query("per_page")); perPageRaw != "" {
			if parsed, err := strconv.Atoi(perPageRaw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		offset = (page - 1) * limit
	}

	return channel, offset, limit
}

func parseCoreIntervalHours(raw string) (int, error) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "h"))
	if trimmed == "" {
		return 0, fmt.Errorf("interval is required")
	}
	intervalHours, err := strconv.Atoi(trimmed)
	if err != nil || intervalHours <= 0 {
		return 0, fmt.Errorf("interval must be a positive hour value, e.g. 12 or 12h")
	}
	return intervalHours, nil
}

func (a *ApiService) GetCoreDownloadProgress(c *gin.Context) {
	id := strings.TrimSpace(c.Query("id"))
	if id == "" {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	jsonObj(c, service.GetCoreDownloadProgress(id), nil)
}
