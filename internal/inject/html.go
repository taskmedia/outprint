package inject

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"html"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

const injectedMarker = "data-outline-pdf-injector"

var (
	closingBody = []byte("</body>")
	closingHead = []byte("</head>")
)

// injector rewrites HTML responses of Outline.
type injector struct {
	tag             []byte
	maxHTML         int64
	allowScriptSelf bool
	logger          *slog.Logger
}

func newInjector(s settings, logger *slog.Logger) *injector {
	tag := fmt.Sprintf(`<script src="%s" defer %s%s></script>`, html.EscapeString(s.scriptURL), injectedMarker, s.buttonAttrs)
	return &injector{tag: []byte(tag), maxHTML: s.maxHTML, allowScriptSelf: s.allowScriptSelf, logger: logger}
}

// modifyResponse injects the tag into a complete HTML page and leaves every
// other response alone: other types, partial content, 304, 204, HEAD, error
// pages and protocol switches such as WebSockets.
func (in *injector) modifyResponse(resp *http.Response) error {
	if !isFullHTMLPage(resp) {
		return nil
	}
	body, closeBody, encoded, err := decodedBody(resp)
	if err != nil {
		return err
	}
	if !encoded && !isIdentity(resp.Header.Get("Content-Encoding")) {
		in.logger.Warn("html page not injected: unsupported content encoding",
			"encoding", resp.Header.Get("Content-Encoding"), "path", resp.Request.URL.Path)
		return nil
	}
	page, rest, err := readPage(body, in.maxHTML)
	if err != nil {
		_ = closeBody()
		return fmt.Errorf("read html page: %w", err)
	}
	if rest != nil {
		in.passThroughLarge(resp, page, rest, closeBody, encoded)
		return nil
	}
	if err := closeBody(); err != nil {
		return fmt.Errorf("close html page: %w", err)
	}
	out, injected := in.insert(page)
	if encoded || injected {
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Etag")
		resp.Header.Del("Content-Md5")
	}
	if injected && in.allowScriptSelf {
		rewriteCSP(resp.Header)
	}
	resp.Body = io.NopCloser(bytes.NewReader(out))
	resp.ContentLength = int64(len(out))
	resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
	return nil
}

func isFullHTMLPage(resp *http.Response) bool {
	if resp.StatusCode != http.StatusOK || resp.Request == nil || resp.Request.Method == http.MethodHead {
		return false
	}
	if resp.Header.Get("Content-Range") != "" {
		return false
	}
	mt, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	return err == nil && mt == "text/html"
}

func isIdentity(encoding string) bool {
	e := strings.ToLower(strings.TrimSpace(encoding))
	return e == "" || e == "identity"
}

// decodedBody returns a reader of the plain page. encoded reports that the
// upstream sent gzip. Any other encoding is returned as is, with the
// original encoding header, for the caller to refuse.
func decodedBody(resp *http.Response) (r io.Reader, closeFn func() error, encoded bool, err error) {
	if strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) != "gzip" {
		return resp.Body, resp.Body.Close, false, nil
	}
	raw := resp.Body
	zr, err := gzip.NewReader(raw)
	if err != nil {
		_ = raw.Close()
		return nil, nil, false, fmt.Errorf("decode gzip html page: %w", err)
	}
	closeAll := func() error {
		zerr := zr.Close()
		if berr := raw.Close(); berr != nil {
			return berr
		}
		return zerr
	}
	return zr, closeAll, true, nil
}

// readPage reads up to limit bytes. When the page is longer, rest continues
// where page stopped.
func readPage(r io.Reader, limit int64) (page []byte, rest io.Reader, err error) {
	page, err = io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(page)) <= limit {
		return page, nil, nil
	}
	return page, r, nil
}

// passThroughLarge streams a page that is too large to hold, decoded when it
// came as gzip, without the tag.
func (in *injector) passThroughLarge(resp *http.Response, head []byte, rest io.Reader, closeFn func() error, decoded bool) {
	in.logger.Warn("html page not injected: larger than the limit", "limit", in.maxHTML, "path", resp.Request.URL.Path)
	resp.Body = readCloser{Reader: io.MultiReader(bytes.NewReader(head), rest), closeFn: closeFn}
	if decoded {
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
		resp.Header.Del("Etag")
		resp.ContentLength = -1
	}
}

type readCloser struct {
	io.Reader
	closeFn func() error
}

func (r readCloser) Close() error { return r.closeFn() }

// insert puts the tag before the last </body>, or before the first </head>
// when the page has no body end. It reports false when the tag is already
// there or the page has no place for it.
func (in *injector) insert(page []byte) ([]byte, bool) {
	lower := asciiLower(page)
	if bytes.Contains(lower, []byte(injectedMarker)) {
		return page, false
	}
	at := bytes.LastIndex(lower, closingBody)
	if at < 0 {
		at = bytes.Index(lower, closingHead)
	}
	if at < 0 {
		return page, false
	}
	out := make([]byte, 0, len(page)+len(in.tag))
	out = append(out, page[:at]...)
	out = append(out, in.tag...)
	return append(out, page[at:]...), true
}

// asciiLower lower-cases ASCII only, so offsets stay valid in any encoding
// that keeps ASCII bytes as they are.
func asciiLower(b []byte) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return out
}

func rewriteCSP(h http.Header) {
	values := h.Values("Content-Security-Policy")
	if len(values) == 0 {
		return
	}
	rewritten := make([]string, len(values))
	for i, v := range values {
		rewritten[i] = allowSelfScript(v)
	}
	h["Content-Security-Policy"] = rewritten
}
