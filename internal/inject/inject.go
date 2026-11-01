package inject

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	dialTimeout           = 10 * time.Second
	upstreamHeaderTimeout = 2 * time.Minute
	// Rendering can wait in the admission queue and then run to its limit.
	gatewayHeaderTimeout = 5 * time.Minute
	idleConnTimeout      = 90 * time.Second
	maxIdleConns         = 64
)

// handler routes between the health check, the Gateway and Outline.
type handler struct {
	prefix   string
	gateway  http.Handler // nil when no Gateway is configured
	upstream http.Handler
}

// New returns the injector handler.
func New(cfg Config) (http.Handler, error) {
	s, err := cfg.validate()
	if err != nil {
		return nil, err
	}
	logger := slog.Default()
	h := &handler{prefix: s.prefix, upstream: newUpstreamProxy(s, logger)}
	if s.gateway != nil {
		h.gateway = newGatewayProxy(s, logger)
	}
	return h, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == HealthPath:
		serveHealth(w, r)
	case h.gateway != nil && underPrefix(r.URL.Path, h.prefix):
		h.serveGateway(w, r)
	default:
		h.upstream.ServeHTTP(w, r)
	}
}

func serveHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

func underPrefix(p, prefix string) bool {
	return p == prefix || strings.HasPrefix(p, prefix+"/")
}

// serveGateway only forwards reads of clean paths below the prefix. The
// Gateway has no write endpoint, and a path that needs cleaning is a probe.
func (h *handler) serveGateway(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, h.prefix)
	if rest == "" || rest == "/" || path.Clean(rest) != rest {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.gateway.ServeHTTP(w, r)
}

func transport(headerTimeout time.Duration) *http.Transport {
	return &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext,
		ResponseHeaderTimeout: headerTimeout,
		MaxIdleConns:          maxIdleConns,
		IdleConnTimeout:       idleConnTimeout,
		// The proxy must see the wire bytes: it decides about the encoding.
		DisableCompression: true,
	}
}

// singleHostDirector points the request at target and keeps the public Host,
// which Outline uses for its origin checks, including on WebSockets.
func singleHostDirector(target *url.URL, keepHost bool) func(*http.Request) {
	return func(r *http.Request) {
		r.URL.Scheme = target.Scheme
		r.URL.Host = target.Host
		r.URL.Path, r.URL.RawPath = joinPath(target, r.URL)
		if !keepHost {
			r.Host = target.Host
		}
	}
}

func joinPath(base, in *url.URL) (string, string) {
	basePath := strings.TrimSuffix(base.EscapedPath(), "/")
	joined := basePath + in.EscapedPath()
	p, err := url.PathUnescape(joined)
	if err != nil {
		return in.Path, in.RawPath
	}
	return p, joined
}

func newGatewayProxy(s settings, logger *slog.Logger) http.Handler {
	direct := singleHostDirector(s.gateway, false)
	return &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, s.prefix)
			if raw := r.URL.RawPath; raw != "" {
				r.URL.RawPath = strings.TrimPrefix(raw, s.prefix)
			}
			direct(r)
		},
		Transport: transport(gatewayHeaderTimeout),
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			logProxyError(logger, "gateway", r, err)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"message": "the PDF service is not reachable", "phase": "proxy",
			})
		},
	}
}

func newUpstreamProxy(s settings, logger *slog.Logger) http.Handler {
	inj := newInjector(s, logger)
	direct := singleHostDirector(s.upstream, true)
	return &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			direct(r)
			prepareNavigation(r)
		},
		Transport:      transport(upstreamHeaderTimeout),
		ModifyResponse: inj.modifyResponse,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			logProxyError(logger, "outline", r, err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}
}

// prepareNavigation asks Outline for a fresh, uncompressed page whenever the
// browser asks for HTML. A conditional request could end in a 304 that sends
// the browser back to a copy cached before the injector was there.
func prepareNavigation(r *http.Request) {
	if !strings.Contains(r.Header.Get("Accept"), "text/html") {
		return
	}
	r.Header.Set("Accept-Encoding", "identity")
	r.Header.Del("If-None-Match")
	r.Header.Del("If-Modified-Since")
}

func logProxyError(logger *slog.Logger, target string, r *http.Request, err error) {
	if r.Context().Err() != nil {
		return
	}
	// The error text names the target address; keep it in the log only.
	logger.Warn("proxy error", "target", target, "method", r.Method, "path", r.URL.Path, "err", err.Error())
}
