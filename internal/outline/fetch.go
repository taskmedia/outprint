package outline

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/taskmedia/outline-latex-pdf/internal/limits"
)

const (
	// DefaultImageTimeout bounds one image fetch with all its redirects.
	DefaultImageTimeout = 30 * time.Second
	dialTimeout         = 10 * time.Second
	responseHeaderWait  = 15 * time.Second
	imageUserAgent      = "outline-latex-pdf-renderer"
)

// BlockedError is a fetch the image policy refused: a redirect to a host that
// is not allowed, an address that is not public, too many redirects. The
// reason is for the author and names no secret.
type BlockedError struct{ Reason string }

func (e *BlockedError) Error() string { return "outline: image fetch blocked: " + e.Reason }

func blocked(format string, args ...any) *BlockedError {
	return &BlockedError{Reason: fmt.Sprintf(format, args...)}
}

// Resolver looks up the addresses of a host. net.DefaultResolver is one.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// ImageOptions configure the image fetch of a Client.
type ImageOptions struct {
	// Hosts are the hosts besides the Outline origin that a redirect may
	// lead to, for example the file storage. An entry is a host name, which
	// matches the default ports of http and https, or host:port.
	Hosts []string
	// Timeout bounds one fetch with its redirects; zero means
	// DefaultImageTimeout.
	Timeout time.Duration
	// Resolver and Dial exist for tests. Dial replaces the connection
	// attempt to an already validated address; it bypasses the connect-time
	// check, which has its own test.
	Resolver Resolver
	Dial     func(ctx context.Context, network, address string) (net.Conn, error)
}

// WithImages returns a copy of the client that fetches images under opts.
func (c *Client) WithImages(opts ImageOptions) (*Client, error) {
	allowed, err := allowedHostPorts(c.base, opts.Hosts)
	if err != nil {
		return nil, err
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultImageTimeout
	}
	if opts.Resolver == nil {
		opts.Resolver = net.DefaultResolver
	}
	out := *c
	out.images = &imageFetch{origin: c.base, allowed: allowed, opts: opts}
	return &out, nil
}

// imageFetch is the SSRF-safe fetcher of one client.
type imageFetch struct {
	origin  *url.URL
	allowed map[string]bool
	opts    ImageOptions
}

// allowedHostPorts is the set of host:port pairs a request may go to: the
// Outline origin, and the configured hosts.
func allowedHostPorts(origin *url.URL, hosts []string) (map[string]bool, error) {
	set := map[string]bool{hostPort(origin): true}
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" || strings.ContainsAny(h, "/@?# \t") || strings.Contains(h, "://") {
			return nil, fmt.Errorf("image host %q is not a host name or host:port", h)
		}
		if _, _, err := net.SplitHostPort(h); err == nil {
			set[h] = true
			continue
		}
		set[net.JoinHostPort(h, "80")] = true
		set[net.JoinHostPort(h, "443")] = true
	}
	return set, nil
}

func hostPort(u *url.URL) string {
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	return net.JoinHostPort(strings.ToLower(u.Hostname()), port)
}

// Image fetches one image of the Outline, named by target: a path with its
// query that starts under the Outline URL's API, for example
// /api/attachments.redirect?id=... The user's cookie goes to the first
// request only, which is to the Outline origin. A redirect is followed by
// hand, without the cookie, and only to an allowed host whose addresses are
// all public; the connection goes to the address that was checked. At most
// limits.MaxImageBytes+1 bytes are read, so the caller can tell too large.
func (c *Client) Image(ctx context.Context, target string, s Session) ([]byte, error) {
	f := c.images
	if f == nil {
		var err error
		if f, err = c.defaultImages(); err != nil {
			return nil, err
		}
	}
	u, err := f.firstURL(c.base, target)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, f.opts.Timeout)
	defer cancel()
	cookie := s.Cookie
	for hop := 0; ; hop++ {
		resp, err := f.get(ctx, u, cookie)
		if err != nil {
			return nil, err
		}
		next, done, err := f.afterResponse(resp, u, hop)
		if err != nil {
			_ = resp.Body.Close()
			return nil, err
		}
		if done {
			defer resp.Body.Close()
			return readCapped(resp)
		}
		_ = resp.Body.Close()
		u, cookie = next, ""
	}
}

func (c *Client) defaultImages() (*imageFetch, error) {
	d, err := c.WithImages(ImageOptions{})
	if err != nil {
		return nil, err
	}
	return d.images, nil
}

// firstURL builds the URL of the first request from a target path. The path
// must stay under the Outline URL's own /api.
func (f *imageFetch) firstURL(base *url.URL, target string) (*url.URL, error) {
	if !strings.HasPrefix(target, base.Path+"/api/") || strings.ContainsAny(target, "\r\n\x00") {
		return nil, fmt.Errorf("%w: image target is not an Outline API path", ErrUnavailable)
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return nil, fmt.Errorf("%w: image target is not a path", ErrUnavailable)
	}
	u.Scheme, u.Host = base.Scheme, base.Host
	return u, nil
}

// afterResponse decides what a response means: done (a final response to
// read), a redirect to follow, or an error.
func (f *imageFetch) afterResponse(resp *http.Response, from *url.URL, hop int) (next *url.URL, done bool, err error) {
	switch resp.StatusCode {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		if hop >= limits.MaxImageRedirects {
			return nil, false, blocked("more than %d redirects", limits.MaxImageRedirects)
		}
		loc, err := from.Parse(resp.Header.Get("Location"))
		if err != nil || resp.Header.Get("Location") == "" {
			return nil, false, fmt.Errorf("%w: redirect without a usable location", ErrUnavailable)
		}
		return loc, false, nil
	case http.StatusOK:
		return nil, true, nil
	}
	return nil, false, statusError("image fetch", resp.StatusCode)
}

// get sends one request to u without following a redirect.
func (f *imageFetch) get(ctx context.Context, u *url.URL, cookie string) (*http.Response, error) {
	dial, err := f.dialer(ctx, u)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: image request", ErrUnavailable)
	}
	req.Header.Set("Accept", "image/*")
	req.Header.Set("User-Agent", imageUserAgent)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           dial,
			DisableKeepAlives:     true,
			ResponseHeaderTimeout: responseHeaderWait,
			TLSHandshakeTimeout:   dialTimeout,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: image fetch: %s", ErrUnavailable, errorKind(err))
	}
	return resp, nil
}

type dialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// dialer validates the URL and the addresses of its host and returns a dial
// function that connects to one of those addresses, whatever name the
// transport asks for. The name is resolved once, here: it is never resolved
// again, so it cannot change between the check and the connection.
func (f *imageFetch) dialer(ctx context.Context, u *url.URL) (dialFunc, error) {
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, blocked("a redirect to %q is not allowed", truncateScheme(u.Scheme))
	}
	if u.User != nil || u.Hostname() == "" {
		return nil, blocked("a redirect with credentials or without a host is not allowed")
	}
	key := hostPort(u)
	if !f.allowed[key] {
		return nil, blocked("the image is stored on %q, which is not an allowed image host (see imageHosts)", u.Hostname())
	}
	exempt := key == hostPort(f.origin)
	addrs, err := f.resolve(ctx, u.Hostname())
	if err != nil {
		return nil, err
	}
	for _, a := range addrs {
		if !exempt && blockedAddr(a) {
			return nil, blocked("%q resolves to an address that is not public", u.Hostname())
		}
	}
	_, port, _ := net.SplitHostPort(key)
	return f.connector(addrs, port, exempt), nil
}

func truncateScheme(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// resolve returns the addresses of host: itself if it is an address,
// otherwise the answer of the resolver.
func (f *imageFetch) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if a, err := netip.ParseAddr(host); err == nil {
		if a.Zone() != "" {
			return nil, blocked("an address with a zone is not allowed")
		}
		return []netip.Addr{a}, nil
	}
	addrs, err := f.opts.Resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addrs) == 0 {
		return nil, fmt.Errorf("%w: image host did not resolve", ErrUnavailable)
	}
	return addrs, nil
}

// connector dials the validated addresses in order.
func (f *imageFetch) connector(addrs []netip.Addr, port string, exempt bool) dialFunc {
	dial := f.opts.Dial
	if dial == nil {
		d := &net.Dialer{Timeout: dialTimeout, Control: connectControl(exempt)}
		dial = d.DialContext
	}
	return func(ctx context.Context, network, _ string) (net.Conn, error) {
		var last error
		for _, a := range addrs {
			conn, err := dial(ctx, network, net.JoinHostPort(a.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, last
	}
}

// connectControl re-checks the address at connect time, the last moment
// before the socket connects, so that a check made earlier cannot be
// outdated by the time the connection is made.
func connectControl(exempt bool) func(network, address string, c syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return blocked("connect address is not valid")
		}
		a, err := netip.ParseAddr(host)
		if err != nil || (!exempt && blockedAddr(a)) {
			return blocked("connect address is not public")
		}
		return nil
	}
}

// readCapped reads a final response, at most MaxImageBytes+1 bytes.
func readCapped(resp *http.Response) ([]byte, error) {
	if resp.ContentLength > limits.MaxImageBytes {
		return nil, ErrTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limits.MaxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: image fetch: read body", ErrUnavailable)
	}
	if len(data) > limits.MaxImageBytes {
		return nil, ErrTooLarge
	}
	return data, nil
}
