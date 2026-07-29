package api

import "github.com/gin-gonic/gin"

const dynamicNoStoreCacheControl = "no-store, no-cache, must-revalidate, private"

// SetNoStoreResponseHeaders marks runtime responses and dynamically rendered
// panel HTML as unsuitable for browser or reverse-proxy reuse.
func SetNoStoreResponseHeaders(c *gin.Context) {
	c.Header("Cache-Control", dynamicNoStoreCacheControl)
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
}

// noStoreDynamicAPIResponse prevents browsers and reverse proxies from reusing
// runtime API data after a panel path, port, session, or subscription setting
// has changed.
func noStoreDynamicAPIResponse(c *gin.Context) {
	SetNoStoreResponseHeaders(c)
	c.Next()
}
