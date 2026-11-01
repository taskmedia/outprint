package images

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/taskmedia/outprint/internal/limits"
	"github.com/taskmedia/outprint/internal/outline"
)

// Errors a Source returns from Fetch for the problems that belong to the
// author. Any other error aborts the render as an infrastructure failure.
var (
	ErrNotFound  = errors.New("image not found")
	ErrForbidden = errors.New("image not readable by this user")
	ErrTooLarge  = errors.New("image too large")
)

// Source tells where an image address leads and fetches it. Locate is pure;
// Fetch is the only place that touches the network or the disk.
type Source interface {
	// Locate classifies an address from the Document. On success it returns
	// the key to fetch by, which is also what duplicates are detected by.
	Locate(address string) (key string, rej *Rejection)
	// Fetch returns the image bytes, at most limits.MaxImageBytes+1 of them.
	Fetch(ctx context.Context, key string) ([]byte, error)
}

// BlockedError is a fetch the source's policy refused, for example a
// redirect to a host that is not allowed. The reason is for the author.
type BlockedError struct{ Reason string }

func (e *BlockedError) Error() string { return "image fetch blocked: " + e.Reason }

const (
	attachmentPath = "/api/attachments.redirect"
	filesPath      = "/api/files.get"
	maxQueryLen    = 2048
	uploadAdvice   = "upload the image to Outline instead"
)

var attachmentIDRE = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z-]{7,63}$`)

// OutlineFetcher is what OutlineSource needs from the Outline client.
type OutlineFetcher interface {
	Image(ctx context.Context, target string, s outline.Session) ([]byte, error)
}

// OutlineSource accepts the images that live in Outline: attachments of the
// configured Outline URL, written relative, absolute or as the file-storage
// path. Everything else is forbidden.
type OutlineSource struct {
	base    *url.URL
	fetcher OutlineFetcher
	session outline.Session
}

// NewOutlineSource returns the Source for the Outline at baseURL, fetching
// with the user's session.
func NewOutlineSource(baseURL string, f OutlineFetcher, s outline.Session) (*OutlineSource, error) {
	base, err := url.Parse(baseURL)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return nil, fmt.Errorf("outline url %q is not an http or https URL", baseURL)
	}
	base.Path = strings.TrimRight(base.Path, "/")
	return &OutlineSource{base: base, fetcher: f, session: s}, nil
}

// Locate implements Source.
func (s *OutlineSource) Locate(address string) (string, *Rejection) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", reject(KindMissing, "the image has no address")
	}
	u, rej := s.parseAddress(address)
	if rej != nil {
		return "", rej
	}
	switch u.Path {
	case s.base.Path + attachmentPath:
		return s.attachmentKey(u)
	case s.base.Path + filesPath:
		return s.filesKey(u)
	}
	return "", reject(KindForbidden, "this address is not an Outline attachment; %s", uploadAdvice)
}

// parseAddress accepts a path on the Outline origin, relative to it or
// absolute with the origin's scheme and host.
func (s *OutlineSource) parseAddress(address string) (*url.URL, *Rejection) {
	if strings.ContainsFunc(address, func(r rune) bool { return r < ' ' || r == 0x7f || r == '\\' }) {
		return nil, reject(KindForbidden, "the image address is not valid; %s", uploadAdvice)
	}
	if strings.HasPrefix(address, "//") {
		return nil, reject(KindForbidden, "images from other sites are not allowed; %s", uploadAdvice)
	}
	u, err := url.Parse(address)
	if err != nil {
		return nil, reject(KindForbidden, "the image address is not valid; %s", uploadAdvice)
	}
	if u.Scheme == "" && u.Host == "" {
		if !strings.HasPrefix(u.Path, "/") {
			return nil, reject(KindForbidden, "this address is not an Outline attachment; %s", uploadAdvice)
		}
		return u, nil
	}
	if u.User != nil || !s.sameOrigin(u) {
		return nil, reject(KindForbidden, "images from %s are not allowed; %s", describeOrigin(u), uploadAdvice)
	}
	return u, nil
}

func (s *OutlineSource) sameOrigin(u *url.URL) bool {
	return u.Scheme == s.base.Scheme && strings.EqualFold(u.Hostname(), s.base.Hostname()) &&
		effectivePort(u) == effectivePort(s.base)
}

func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

// describeOrigin names where an address leads without echoing a long or
// hostile address.
func describeOrigin(u *url.URL) string {
	switch {
	case u.Host != "":
		return truncate(u.Hostname(), 60)
	case u.Scheme != "":
		return u.Scheme + ": addresses"
	}
	return "this address"
}

func (s *OutlineSource) attachmentKey(u *url.URL) (string, *Rejection) {
	ids := u.Query()["id"]
	if len(ids) == 0 || ids[0] == "" {
		return "", reject(KindMissing, "the image has no attachment id")
	}
	if !attachmentIDRE.MatchString(ids[0]) {
		return "", reject(KindMissing, "the attachment id is not valid")
	}
	return s.base.Path + attachmentPath + "?id=" + url.QueryEscape(ids[0]), nil
}

func (s *OutlineSource) filesKey(u *url.URL) (string, *Rejection) {
	if u.RawQuery == "" || len(u.RawQuery) > maxQueryLen {
		return "", reject(KindMissing, "the image has no file key")
	}
	return s.base.Path + filesPath + "?" + u.RawQuery, nil
}

// Fetch implements Source.
func (s *OutlineSource) Fetch(ctx context.Context, key string) ([]byte, error) {
	data, err := s.fetcher.Image(ctx, key, s.session)
	var blocked *outline.BlockedError
	switch {
	case errors.As(err, &blocked):
		return nil, &BlockedError{Reason: blocked.Reason}
	case errors.Is(err, outline.ErrNotFound):
		return nil, ErrNotFound
	case errors.Is(err, outline.ErrForbidden):
		return nil, ErrForbidden
	case errors.Is(err, outline.ErrTooLarge):
		return nil, ErrTooLarge
	}
	return data, err
}

// DirSource reads images from a directory, for the render command, which
// runs the same pipeline without Outline. An address is a relative path
// inside the directory.
type DirSource struct{ Root string }

// Locate implements Source.
func (d DirSource) Locate(address string) (string, *Rejection) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", reject(KindMissing, "the image has no address")
	}
	decoded, err := url.PathUnescape(address)
	if err != nil || strings.ContainsAny(decoded, "\\\x00") || strings.Contains(decoded, ":") ||
		path.IsAbs(decoded) || !isLocal(decoded) {
		return "", reject(KindForbidden, "only images next to the document can be used")
	}
	return path.Clean(decoded), nil
}

func isLocal(p string) bool {
	clean := path.Clean(p)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

// Fetch implements Source. The directory is opened as a root, so a symbolic
// link cannot lead out of it.
func (d DirSource) Fetch(_ context.Context, key string) ([]byte, error) {
	root, err := os.OpenRoot(d.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(key)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, ErrNotFound
	case err != nil:
		return nil, ErrForbidden
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limits.MaxImageBytes+1))
}
