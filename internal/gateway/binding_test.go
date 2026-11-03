package gateway

import (
	"bytes"
	"encoding/json"
	"github.com/taskmedia/outprint/internal/button"
	"reflect"
	"strings"
	"testing"

	"github.com/taskmedia/outprint/internal/bundle"
)

const validBlock = "```yaml\nbinding:\n  template: neutral\n  configuration: example\n```\n"

type errorBody struct {
	Errors []struct {
		Field, Source, Code, Message string
		Line                         *int
	} `json:"errors"`
}

func decodeErrors(t *testing.T, body []byte) errorBody {
	t.Helper()
	var list errorBody
	if err := json.Unmarshal(body, &list); err != nil || len(list.Errors) == 0 {
		t.Fatalf("body = %s (%v)", body, err)
	}
	return list
}

func (e *env) assertRenderedWith(t *testing.T, template, configuration string) {
	t.Helper()
	if len(e.worker.bodies) != 1 {
		t.Fatalf("worker got %d requests", len(e.worker.bodies))
	}
	got, err := bundle.Decode(bytes.NewReader(e.worker.bodies[0]))
	if err != nil {
		t.Fatal(err)
	}
	if got.Template != template || got.Configuration != configuration {
		t.Errorf("bundle = %s/%s, want %s/%s", got.Template, got.Configuration, template, configuration)
	}
}

func TestCollectionBlockSelectsTheBindingWithoutDefault(t *testing.T) {
	e := newEnv(t, false)
	e.outline.add("d", 200, "T", docText)
	e.outline.addCollection("c1", 200, "# Team\n\nIntro\n\n"+validBlock+"\nOutro\n")

	resp, body := e.get(t, "d", cookieCanary)

	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	e.assertRenderedWith(t, "neutral", "example")
	for _, seen := range e.outline.seen {
		if seen.path == "/api/collections.info" && seen.cookie == cookieCanary {
			return
		}
	}
	t.Errorf("no collection call with the user's cookie: %+v", e.outline.seen)
}

func TestBrokenBlockIs422AndNeverFallsBackToTheDefault(t *testing.T) {
	tests := map[string]struct{ overview, code string }{
		"missing key":           {"```yaml\nbinding:\n  template: neutral\n```\n", "binding_malformed"},
		"extra key":             {"```yaml\nbinding:\n  template: neutral\n  configuration: example\n  font: x\n```\n", "binding_malformed"},
		"malformed yaml":        {"```yaml\nbinding:\n  template: [neutral\n```\n", "binding_malformed"},
		"unknown template":      {"```yaml\nbinding:\n  template: nope\n  configuration: example\n```\n", "binding_unknown"},
		"unknown configuration": {"```yaml\nbinding:\n  template: neutral\n  configuration: nope\n```\n", "binding_unknown"},
		"second block":          {validBlock + "\n" + validBlock, "binding_duplicate"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, true)
			e.outline.add("d", 200, "T", docText)
			e.outline.addCollection("c1", 200, tt.overview)

			resp, body := e.get(t, "d", cookieCanary)

			if resp.StatusCode != 422 {
				t.Fatalf("status %d: %s", resp.StatusCode, body)
			}
			list := decodeErrors(t, body)
			if list.Errors[0].Source != "collection" || list.Errors[0].Code != tt.code || list.Errors[0].Line == nil {
				t.Errorf("errors = %+v, want source collection, code %s, a line", list.Errors, tt.code)
			}
			if len(e.worker.bodies) != 0 {
				t.Error("the Worker was called with a broken Binding")
			}
			e.assertNoLeak(t, resp, body)
		})
	}
}

func TestNoBlockUsesTheDefaultBinding(t *testing.T) {
	e := newEnv(t, true)
	e.outline.add("d", 200, "T", docText)
	e.outline.addCollection("c1", 200, "# Team\nNo binding here.\n")
	if resp, body := e.get(t, "d", cookieCanary); resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	e.assertRenderedWith(t, "neutral", "example")
}

func TestNoBlockAndNoDefaultIs422(t *testing.T) {
	e := newEnv(t, false)
	e.outline.add("d", 200, "T", docText)
	e.outline.addCollection("c1", 200, "# Team\n")
	resp, body := e.get(t, "d", cookieCanary)
	list := decodeErrors(t, body)
	if resp.StatusCode != 422 || list.Errors[0].Code != "no_binding" || list.Errors[0].Source != "collection" {
		t.Errorf("status %d body %s", resp.StatusCode, body)
	}
}

func TestCollectionThatCannotBeReadIsAClearErrorWithoutFallback(t *testing.T) {
	tests := []struct {
		name         string
		status, want int
	}{
		{"forbidden", 403, 403}, {"not found", 404, 404}, {"unauthorized", 401, 401}, {"server error", 500, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, true)
			e.outline.add("d", 200, "T", docText)
			e.outline.addCollection("c1", tt.status, "")
			resp, body := e.get(t, "d", cookieCanary)
			if resp.StatusCode != tt.want {
				t.Errorf("status %d, want %d: %s", resp.StatusCode, tt.want, body)
			}
			if tt.want == 403 || tt.want == 404 {
				if !strings.Contains(string(body), "collection") || !strings.Contains(string(body), `"phase":"binding"`) {
					t.Errorf("body %s does not name the collection", body)
				}
			}
			if len(e.worker.bodies) != 0 {
				t.Error("the Worker was called although the Collection could not be read")
			}
			e.assertNoLeak(t, resp, body)
		})
	}
}

func TestOverviewChangeTakesEffectOnTheNextRender(t *testing.T) {
	e := newEnv(t, false)
	e.outline.add("d", 200, "T", docText)
	e.outline.addCollection("c1", 200, validBlock)
	if resp, _ := e.get(t, "d", cookieCanary); resp.StatusCode != 200 {
		t.Fatalf("first render: %d", resp.StatusCode)
	}
	e.outline.addCollection("c1", 200, "No block any more.\n")
	if resp, _ := e.get(t, "d", cookieCanary); resp.StatusCode != 422 {
		t.Errorf("second render: %d, want 422 (no cache)", resp.StatusCode)
	}
	if n := e.outline.calls("/api/collections.info"); n != 2 {
		t.Errorf("collections.info called %d times, want once per render", n)
	}
}

func TestPerCallOverridesHaveNoEffect(t *testing.T) {
	override := "?template=other&configuration=other&binding=other%2Fother"
	t.Run("valid block stays", func(t *testing.T) {
		e := newEnv(t, false)
		e.outline.add("d", 200, "T", docText)
		e.outline.addCollection("c1", 200, validBlock)
		resp, body := e.getPath(t, "/render/d"+override, cookieCanary)
		if resp.StatusCode != 200 {
			t.Fatalf("status %d: %s", resp.StatusCode, body)
		}
		e.assertRenderedWith(t, "neutral", "example")
	})
	t.Run("no binding stays an error", func(t *testing.T) {
		e := newEnv(t, false)
		e.outline.add("d", 200, "T", docText)
		resp, _ := e.getPath(t, "/render/d?template=neutral&configuration=example", cookieCanary)
		if resp.StatusCode != 422 || len(e.worker.bodies) != 0 {
			t.Errorf("status %d, worker calls %d", resp.StatusCode, len(e.worker.bodies))
		}
	})
	t.Run("broken block stays an error", func(t *testing.T) {
		e := newEnv(t, true)
		e.outline.add("d", 200, "T", docText)
		e.outline.addCollection("c1", 200, "```yaml\nbinding:\n  template: nope\n  configuration: example\n```\n")
		resp, _ := e.getPath(t, "/render/d?template=neutral&configuration=example", cookieCanary)
		if resp.StatusCode != 422 {
			t.Errorf("status %d", resp.StatusCode)
		}
	})
}

func TestDocumentWithoutCollectionHasNoBlock(t *testing.T) {
	e := newEnv(t, true)
	e.outline.docs["d"] = outlineDoc{status: 200, doc: map[string]any{"id": "d", "title": "T", "text": docText, "collectionId": ""}}
	if resp, body := e.get(t, "d", cookieCanary); resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	if n := e.outline.calls("/api/collections.info"); n != 0 {
		t.Errorf("collections.info called %d times for a Document without Collection", n)
	}
}

type statusBody struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Language  string `json:"language"`
}

func (e *env) status(t *testing.T, id, cookie string) (int, statusBody) {
	t.Helper()
	resp, body := e.getPath(t, "/status/"+id, cookie)
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", resp.Header.Get("Cache-Control"))
	}
	var sb statusBody
	if err := json.Unmarshal(body, &sb); err != nil {
		t.Fatalf("body = %s (%v)", body, err)
	}
	return resp.StatusCode, sb
}

func TestStatusSemantics(t *testing.T) {
	tests := []struct {
		name        string
		defaultBind bool
		overview    string
		available   bool
		reason      string
	}{
		{"valid block", false, validBlock, true, "collection_binding"},
		{"malformed block shows the button", true, "```yaml\nbinding:\n  nonsense: 1\n```\n", true, "collection_binding_invalid"},
		{"unknown names show the button", false, "```yaml\nbinding:\n  template: x\n  configuration: y\n```\n", true, "collection_binding_invalid"},
		{"no block with default", true, "# Team\n", true, "default_binding"},
		{"no block no default", false, "# Team\n", false, "no_binding"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, tt.defaultBind)
			e.outline.add("d", 200, "T", docText)
			e.outline.addCollection("c1", 200, tt.overview)
			code, sb := e.status(t, "d", cookieCanary)
			if code != 200 || sb.Available != tt.available || sb.Reason != tt.reason {
				t.Errorf("status %d body %+v, want available=%v reason=%s", code, sb, tt.available, tt.reason)
			}
			if len(e.worker.bodies) != 0 {
				t.Error("the status endpoint rendered")
			}
		})
	}
}

func TestStatusCarriesTheButtonOptions(t *testing.T) {
	options := button.Options{Placement: "button", Labels: map[string]map[string]string{"menu": {"de": "PDF mit Briefkopf"}}}
	e := newEnvConfigured(t, true, nil, nil, func(c *Config) { c.Button = options })
	e.outline.add("d", 200, "T", docText)
	e.outline.addCollection("c1", 200, "# Team\n")
	_, body := e.getPath(t, "/status/d", cookieCanary)
	var got struct {
		Button button.Options `json:"button"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body = %s (%v)", body, err)
	}
	if !reflect.DeepEqual(got.Button, options) {
		t.Errorf("button = %+v, want %+v (body %s)", got.Button, options, body)
	}
}

func TestStatusOmitsTheButtonWithoutSettings(t *testing.T) {
	e := newEnv(t, true)
	e.outline.add("d", 200, "T", docText)
	e.outline.addCollection("c1", 200, "# Team\n")
	_, body := e.getPath(t, "/status/d", cookieCanary)
	if strings.Contains(string(body), "button") {
		t.Errorf("body = %s, want no button key", body)
	}
}

func TestStatusMapsOutlineErrors(t *testing.T) {
	tests := []struct {
		name           string
		doc, col, want int
	}{
		{"document 403", 403, 200, 403}, {"document 404", 404, 200, 404}, {"document 401", 401, 200, 401},
		{"collection 403", 200, 403, 403}, {"collection 404", 200, 404, 404}, {"collection 401", 200, 401, 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, true)
			e.outline.add("d", tt.doc, "T", docText)
			e.outline.addCollection("c1", tt.col, "")
			code, sb := e.status(t, "d", cookieCanary)
			if code != tt.want || sb.Available || sb.Message == "" {
				t.Errorf("status %d body %+v, want %d with a message", code, sb, tt.want)
			}
		})
	}
}

func TestStatusNeedsCookieAndValidID(t *testing.T) {
	e := newEnv(t, true)
	if resp, _ := e.getPath(t, "/status/d", ""); resp.StatusCode != 401 {
		t.Errorf("no cookie: %d", resp.StatusCode)
	}
	if resp, _ := e.getPath(t, "/status/-x", cookieCanary); resp.StatusCode != 400 {
		t.Errorf("bad id: %d", resp.StatusCode)
	}
	if len(e.outline.seen) != 0 {
		t.Error("an invalid request reached Outline")
	}
}

func TestStatusReportsTheUsersLanguage(t *testing.T) {
	e := newEnv(t, true)
	e.outline.add("d", 200, "T", docText)
	e.outline.language = "de_DE"
	code, sb := e.status(t, "d", cookieCanary)
	if code != 200 || sb.Language != "de_DE" {
		t.Errorf("status %d body %+v, want language de_DE", code, sb)
	}
}

func TestStatusStaysAvailableWhenTheLanguageIsUnknown(t *testing.T) {
	e := newEnv(t, true)
	e.outline.add("d", 200, "T", docText)
	e.outline.authStatus = 500
	code, sb := e.status(t, "d", cookieCanary)
	if code != 200 || !sb.Available || sb.Language != "" {
		t.Errorf("status %d body %+v, want available without a language", code, sb)
	}
}
