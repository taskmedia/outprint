//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"
)

const maxInlineBody = 256 << 10

// outcome is what the test sees of one request to a Worker.
type outcome struct {
	Status         int             `json:"status"`
	ContentType    string          `json:"content_type,omitempty"`
	Error          json.RawMessage `json:"error,omitempty"`
	BodyBytes      int             `json:"body_bytes"`
	Body           string          `json:"body,omitempty"`
	Text           string          `json:"text,omitempty"`
	TransportError string          `json:"transport_error,omitempty"`
	ElapsedMS      int64           `json:"elapsed_ms"`
}

// runPost sends stdin as the body (or a GET with -get) and prints an
// outcome. It always exits 0 once it has printed one.
func runPost(args []string) int {
	fs := flag.NewFlagSet("post", flag.ContinueOnError)
	get := fs.Bool("get", false, "send a GET without a body")
	text := fs.Bool("pdftotext", false, "return the text of a PDF answer")
	timeout := fs.Duration("timeout", 120*time.Second, "request timeout")
	if err := fs.Parse(args); err != nil || fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: sandbox-probe post [-get] [-pdftotext] [-timeout d] URL < bundle.json")
		return 2
	}
	start := time.Now()
	out := send(fs.Arg(0), *get, *text, *timeout)
	out.ElapsedMS = time.Since(start).Milliseconds()
	_ = json.NewEncoder(os.Stdout).Encode(out)
	return 0
}

func send(url string, get, wantText bool, timeout time.Duration) outcome {
	method, body := http.MethodPost, io.Reader(os.Stdin)
	if get {
		method, body = http.MethodGet, nil
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return outcome{TransportError: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return outcome{TransportError: err.Error()}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return outcome{Status: resp.StatusCode, TransportError: err.Error()}
	}
	return describe(resp, data, wantText)
}

func describe(resp *http.Response, data []byte, wantText bool) outcome {
	out := outcome{Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), BodyBytes: len(data)}
	switch {
	case strings.HasPrefix(out.ContentType, "application/json"):
		out.Error = json.RawMessage(data)
	case wantText && bytes.HasPrefix(data, []byte("%PDF")):
		out.Text = pdfText(data)
	case len(data) <= maxInlineBody && utf8.Valid(data):
		out.Body = string(data)
	}
	return out
}

func pdfText(pdf []byte) string {
	cmd := exec.Command("pdftotext", "-layout", "-", "-")
	cmd.Stdin = bytes.NewReader(pdf)
	text, err := cmd.Output()
	if err != nil {
		return "pdftotext failed: " + err.Error()
	}
	return string(text)
}
