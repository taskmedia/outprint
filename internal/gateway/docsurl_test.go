package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/taskmedia/outprint/internal/docslink"
	"github.com/taskmedia/outprint/internal/version"
	"github.com/taskmedia/outprint/internal/workerapi"
)

const errorsPage = "https://outprint.task.media/0.3/reference/error-messages/"

func setVersion(t *testing.T, v string) {
	t.Helper()
	old := version.Version
	t.Cleanup(func() { version.Version = old })
	version.Version = v
}

func TestEveryEntryOfTheErrorListCarriesADocsURL(t *testing.T) {
	setVersion(t, "0.3.2")
	e := newEnv(t, true)
	e.outline.add("d", 200, "T", "```yaml\nsubjct: oops\ntype: nonsense\n```\nBody\n")
	resp, body := e.get(t, "d", cookieCanary)
	if resp.StatusCode != 422 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	var list struct {
		Errors []map[string]any `json:"errors"`
	}
	if err := json.Unmarshal(body, &list); err != nil || len(list.Errors) == 0 {
		t.Fatalf("body = %s (%v)", body, err)
	}
	for _, entry := range list.Errors {
		for _, key := range []string{"field", "source", "code", "message", "line", "docs_url"} {
			if _, ok := entry[key]; !ok {
				t.Errorf("entry %v lacks %q", entry, key)
			}
		}
		code, _ := entry["code"].(string)
		if want := errorsPage + "#" + code; entry["docs_url"] != want {
			t.Errorf("docs_url = %v, want %q", entry["docs_url"], want)
		}
		if len(entry) != 6 {
			t.Errorf("entry %v has fields besides the contract", entry)
		}
	}
}

func TestNoBindingErrorCarriesADocsURL(t *testing.T) {
	setVersion(t, "0.3.2")
	e := newEnv(t, false)
	e.outline.add("d", 200, "T", docText)
	_, body := e.get(t, "d", cookieCanary)
	if !strings.Contains(string(body), `"docs_url":"`+errorsPage+`#no_binding"`) {
		t.Errorf("body = %s", body)
	}
}

func TestOtherFailuresCarryADocsURLOfTheirKind(t *testing.T) {
	setVersion(t, "0.3.2")
	e := newEnv(t, true)
	e.outline.add("d", 200, "T", docText)
	e.worker.status = 504
	e.worker.errBody = workerapi.ErrorBody{Phase: "timeout", Kind: "timeout", JobID: "j1"}
	resp, body := e.get(t, "d", cookieCanary)
	var m map[string]string
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("body = %s", body)
	}
	if resp.StatusCode != 504 || m["docs_url"] != errorsPage+"#timeout" || m["message"] == "" || m["phase"] != "timeout" || m["job_id"] != "j1" {
		t.Errorf("status %d body %s", resp.StatusCode, body)
	}
}

func TestDocsURLFollowsTheVersion(t *testing.T) {
	tests := []struct{ version, prefix string }{
		{"dev", "https://outprint.task.media/dev/"},
		{"v1.4.0", "https://outprint.task.media/1.4/"},
		{"", "https://outprint.task.media/reference/"},
		{"banana", "https://outprint.task.media/reference/"},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			setVersion(t, tt.version)
			rec := httptest.NewRecorder()
			noBindingFailure().write(rec)
			var list struct {
				Errors []struct {
					DocsURL string `json:"docs_url"`
				} `json:"errors"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Errors) != 1 {
				t.Fatalf("body = %s", rec.Body)
			}
			if got := list.Errors[0].DocsURL; !strings.HasPrefix(got, tt.prefix) || got != docslink.ErrorURL(tt.version, "no_binding") {
				t.Errorf("docs_url = %q, want prefix %q", got, tt.prefix)
			}
		})
	}
}

func TestFailureWithoutAKindHasNoDocsURL(t *testing.T) {
	rec := httptest.NewRecorder()
	fail(http.StatusInternalServerError, PhaseRequest, "", "the document could not be rendered", nil).write(rec)
	if strings.Contains(rec.Body.String(), "docs_url") {
		t.Errorf("body = %s", rec.Body)
	}
}
