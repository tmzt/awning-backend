package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// StaticProxyMiddleware returns a Gin middleware that proxies non-API requests
// to the provided upstream `proxyAddr`.
//
// Behavior:
// - Requests with path starting with `/api/` are left untouched.
// - Other requests are forwarded to the upstream (preserving path and query).
func StaticProxyMiddleware(proxyAddr string) gin.HandlerFunc {
	target, err := url.Parse(proxyAddr)
	if err != nil {
		// If the provided proxy address is invalid, return a middleware that
		// simply logs and continues so the app can still start.
		slog.Error("invalid static proxy address", "proxy", proxyAddr, "error", err)
		return func(c *gin.Context) { c.Next() }
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Replace Director to preserve the original request path while routing
	// through the target host and scheme.
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Ensure the forwarded request Host is set to the target host so
		// virtual-hosted upstreams work correctly.
		req.Host = target.Host
		// If target has a path prefix, join it with the incoming path.
		if target.Path != "" && target.Path != "/" {
			// Avoid double slashes
			req.URL.Path = strings.TrimRight(target.Path, "/") + "/" + strings.TrimLeft(req.URL.Path, "/")
		}
	}

	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		slog.Error("static proxy error", "error", err, "url", req.URL.String())
		rw.WriteHeader(http.StatusBadGateway)
		rw.Write([]byte("Bad Gateway"))
	}

	return func(c *gin.Context) {
		// Only proxy non-API requests
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Next()
			return
		}

		// For everything else, proxy the request to the upstream
		proxy.ServeHTTP(c.Writer, c.Request)
		// Stop further handlers; response already written by proxy
		c.Abort()
	}
}
