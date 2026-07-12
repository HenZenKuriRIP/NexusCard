package httpserver

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:web
var webFS embed.FS

func (s *Server) registerWeb(r *gin.Engine) {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/shop/")
	})
	r.GET("/favicon.ico", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	// Hash-router SPAs: only index HTML is needed (no /admin/* catch-all → no conflict with /admin/api)
	r.GET("/shop", func(c *gin.Context) { c.Redirect(http.StatusFound, "/shop/") })
	r.GET("/shop/", func(c *gin.Context) { serveFile(c, sub, "shop/index.html", "text/html; charset=utf-8") })
	r.GET("/admin", func(c *gin.Context) { c.Redirect(http.StatusFound, "/admin/") })
	r.GET("/admin/", func(c *gin.Context) { serveFile(c, sub, "admin/index.html", "text/html; charset=utf-8") })

	r.GET("/assets/*filepath", func(c *gin.Context) {
		p := strings.TrimPrefix(c.Param("filepath"), "/")
		// prevent path escape
		if strings.Contains(p, "..") {
			c.Status(http.StatusBadRequest)
			return
		}
		data, err := fs.ReadFile(sub, "assets/"+p)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, contentType(p), data)
	})
}

func serveFile(c *gin.Context, sub fs.FS, name, ct string) {
	data, err := fs.ReadFile(sub, name)
	if err != nil {
		c.String(http.StatusInternalServerError, "web assets missing: "+err.Error())
		return
	}
	c.Data(http.StatusOK, ct, data)
}

func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		return "application/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	default:
		return "application/octet-stream"
	}
}
