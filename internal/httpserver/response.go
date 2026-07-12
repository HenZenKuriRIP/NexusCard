package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func JSONOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": data})
}

func JSONErr(c *gin.Context, httpStatus, code int, message string) {
	c.JSON(httpStatus, gin.H{"code": code, "message": message, "data": nil})
}
