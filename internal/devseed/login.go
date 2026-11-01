package devseed

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	sessionCookie   = "accessToken"
	maxLoginPageLen = 1 << 20
)

var formAction = regexp.MustCompile(`(?is)<form[^>]*\saction\s*=\s*"([^"]*)"`)

// loginFormAction returns the address the sign-in form of the identity
// provider posts to, resolved against the page it was found on.
func loginFormAction(page string, pageURL *url.URL) (*url.URL, error) {
	m := formAction.FindStringSubmatch(page)
	if m == nil {
		return nil, errors.New("no sign-in form on the page")
	}
	ref, err := url.Parse(html.UnescapeString(m[1]))
	if err != nil {
		return nil, fmt.Errorf("sign-in form action: %w", err)
	}
	return pageURL.ResolveReference(ref), nil
}

// Login signs in the way a browser does: it starts at Outline's OIDC entry,
// follows the redirects to the identity provider, posts the credentials to
// its form and follows the redirects back. Outline creates the workspace and
// the user on the first sign-in. The result is the session token, which
// Outline also accepts as a bearer token.
func Login(ctx context.Context, base, email, password string) (string, error) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("base url: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}

	page, pageURL, err := get(ctx, client, baseURL.JoinPath("auth", "oidc").String())
	if err != nil {
		return "", fmt.Errorf("start sign-in: %w", err)
	}
	action, err := loginFormAction(page, pageURL)
	if err != nil {
		return "", fmt.Errorf("start sign-in at %s: %w", pageURL.Host, err)
	}

	form := url.Values{"login": {email}, "password": {password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, action.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("post credentials: %w", err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxLoginPageLen))
	_ = resp.Body.Close()

	for _, c := range jar.Cookies(baseURL) {
		if c.Name == sessionCookie && c.Value != "" {
			return c.Value, nil
		}
	}
	return "", errors.New("sign-in did not produce a session (wrong credentials, or the identity provider is not the dev Dex)")
}

func get(ctx context.Context, client *http.Client, target string) (string, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLoginPageLen))
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("%s answered %d", resp.Request.URL.Host, resp.StatusCode)
	}
	return string(body), resp.Request.URL, nil
}

// WaitReady polls until the stack answers: Outline's health endpoint and the
// button script behind the injector.
func WaitReady(ctx context.Context, base string, every time.Duration) error {
	paths := []string{"/_health", "/pdf/button.js"}
	client := &http.Client{Timeout: 5 * time.Second}
	for _, p := range paths {
		for {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+p, nil)
			if err != nil {
				return err
			}
			if resp, err := client.Do(req); err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					break
				}
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("waiting for %s%s: %w", base, p, ctx.Err())
			case <-time.After(every):
			}
		}
	}
	return nil
}
