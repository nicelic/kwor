package sub

import (
	"github.com/alireza0/s-ui/middleware"
	"github.com/gin-gonic/gin"
)

func fake504Handler(c *gin.Context) {
	middleware.DelayedFake504(c)
}
