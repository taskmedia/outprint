package gateway

import (
	"net/http"

	"github.com/taskmedia/outprint/web"
)

// scriptCacheControl is short on purpose: a new image should reach the
// browsers within minutes, and the script is small.
const scriptCacheControl = "public, max-age=300"

// serveButtonScript serves the button script. It needs no cookie and holds
// nothing secret. The Gateway sends no CORS headers: the script is loaded
// from the same origin the injector serves Outline on.
func serveButtonScript(w http.ResponseWriter, _ *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/javascript; charset=utf-8")
	h.Set("Cache-Control", scriptCacheControl)
	h.Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(web.ButtonJS)
}
