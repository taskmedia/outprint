package devseed

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

const fakeToken = "session-token"

// fakeStack stands in for Outline (behind the injector) and Dex. It keeps
// collections and documents in memory, like the parts of the API the seed
// uses.
type fakeStack struct {
	t       *testing.T
	outline *httptest.Server
	dex     *httptest.Server

	mu          sync.Mutex
	collections []map[string]any
	documents   []map[string]any
	uploads     []string
	creates     int
	viewerRole  string
}

func newFakeStack(t *testing.T) *fakeStack {
	f := &fakeStack{t: t}
	f.dex = httptest.NewServer(http.HandlerFunc(f.serveDex))
	f.outline = httptest.NewServer(http.HandlerFunc(f.serveOutline))
	t.Cleanup(f.dex.Close)
	t.Cleanup(f.outline.Close)
	return f
}

func (f *fakeStack) serveDex(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		fmt.Fprint(w, `<html><form method="post" action="/dex/login?back=&amp;state=abc"><input name="login"><input name="password"></form></html>`)
	case http.MethodPost:
		_ = r.ParseForm()
		login := r.PostForm.Get("login")
		if (login != DefaultEmail && login != DefaultViewerEmail) || r.PostForm.Get("password") != DefaultPassword {
			fmt.Fprint(w, `<html>Invalid login</html>`)
			return
		}
		http.Redirect(w, r, f.outline.URL+"/auth/oidc.callback?code=1", http.StatusSeeOther)
	}
}

func (f *fakeStack) serveOutline(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/auth/oidc":
		http.Redirect(w, r, f.dex.URL+"/dex/auth/local/login?state=abc", http.StatusFound)
	case r.URL.Path == "/auth/oidc.callback":
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: fakeToken, Path: "/"})
		http.Redirect(w, r, "/home", http.StatusFound)
	case r.URL.Path == "/home":
		fmt.Fprint(w, "home")
	case strings.HasPrefix(r.URL.Path, "/api/"):
		f.serveAPI(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeStack) serveAPI(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+fakeToken {
		http.Error(w, `{"ok":false,"message":"auth"}`, http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	method := strings.TrimPrefix(r.URL.Path, "/api/")
	if method == "files.create" {
		f.recordUpload(w, r)
		return
	}
	var in map[string]any
	_ = json.NewDecoder(r.Body).Decode(&in)
	reply := func(data any) { _ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": data}) }
	switch method {
	case "collections.list":
		reply(f.collections)
	case "collections.create":
		c := map[string]any{"id": fmt.Sprintf("col-%d", len(f.collections)), "name": in["name"], "description": in["description"]}
		f.collections = append(f.collections, c)
		f.creates++
		reply(c)
	case "collections.update":
		for _, c := range f.collections {
			if c["id"] == in["id"] {
				c["description"] = in["description"]
				reply(c)
				return
			}
		}
	case "collections.documents":
		var out []map[string]any
		for _, d := range f.documents {
			if d["collectionId"] == in["id"] {
				out = append(out, map[string]any{"id": d["id"], "title": d["title"]})
			}
		}
		reply(out)
	case "documents.info":
		for _, d := range f.documents {
			if d["id"] == in["id"] {
				reply(d)
				return
			}
		}
	case "documents.create":
		id := fmt.Sprintf("doc-%d", len(f.documents))
		d := map[string]any{"id": id, "title": in["title"], "text": in["text"], "collectionId": in["collectionId"], "url": "/doc/x-" + id}
		f.documents = append(f.documents, d)
		f.creates++
		reply(d)
	case "documents.update":
		for _, d := range f.documents {
			if d["id"] == in["id"] {
				d["text"] = in["text"]
				reply(d)
				return
			}
		}
	case "users.list":
		reply([]map[string]any{{"id": "u-viewer", "email": DefaultViewerEmail, "role": "member"}})
	case "users.update_role":
		f.viewerRole = in["role"].(string)
		reply(map[string]any{"id": in["id"], "role": f.viewerRole})
	case "attachments.create":
		id := fmt.Sprintf("att-%d", len(f.uploads))
		reply(map[string]any{
			"uploadUrl":  "/api/files.create",
			"form":       map[string]any{"key": "uploads/" + id, "sig": "s"},
			"attachment": map[string]any{"id": id, "url": "/api/attachments.redirect?id=" + id},
		})
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeStack) recordUpload(w http.ResponseWriter, r *http.Request) {
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var order []string
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		order = append(order, p.FormName())
	}
	if len(order) == 0 || order[len(order)-1] != "file" {
		f.t.Errorf("the file must be the last part of the upload, got %v", order)
	}
	f.uploads = append(f.uploads, strings.Join(order, ","))
	fmt.Fprint(w, `{"success":true}`)
}

func seedContent() fstest.MapFS {
	return fstest.MapFS{"chart.png": {Data: []byte("png")}, "diagram.svg": {Data: []byte("<svg/>")}}
}

func runSeed(t *testing.T, f *fakeStack) Result {
	t.Helper()
	res, err := Run(t.Context(), Config{BaseURL: f.outline.URL, Email: DefaultEmail, Password: DefaultPassword, Content: seedContent()})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSeedCreatesTheBoundAndTheUnboundCollection(t *testing.T) {
	f := newFakeStack(t)
	res := runSeed(t, f)

	if res.Bound.CollectionName != BoundCollectionName || res.Unbound.CollectionName != UnboundCollectionName {
		t.Fatalf("collections: %+v", res)
	}
	var boundCol, unbound map[string]any
	for _, c := range f.collections {
		switch c["name"] {
		case BoundCollectionName:
			boundCol = c
		case UnboundCollectionName:
			unbound = c
		}
	}
	if !strings.Contains(boundCol["description"].(string), "```yaml\nbinding:\n  template: neutral\n  configuration: example\n```") {
		t.Errorf("the bound collection has no binding block: %q", boundCol["description"])
	}
	if strings.Contains(unbound["description"].(string), "binding") {
		t.Errorf("the unbound collection must not mention a binding: %q", unbound["description"])
	}
	bound := f.documents[0]["text"].(string)
	for _, want := range []string{"```yaml\ntype: letter\nsubject: " + BoundSubject + "\n```", BodySentence, "| Plant | Price |", "![](/api/attachments.redirect?id=att-0 \" =240x\")", "![](/api/attachments.redirect?id=att-1 \" =240x\")"} {
		if !strings.Contains(bound, want) {
			t.Errorf("the bound document lacks %q:\n%s", want, bound)
		}
	}
	captioned := f.documents[1]["text"].(string)
	for _, caption := range []string{ChartCaption, DiagramCaption} {
		if !strings.Contains(captioned, "!["+caption+"](/api/attachments.redirect?id=att-") {
			t.Errorf("the unbound document lacks the caption %q:\n%s", caption, captioned)
		}
	}
	if !strings.HasPrefix(res.Bound.DocumentPath, "/doc/") || res.Email != DefaultEmail {
		t.Errorf("result: %+v", res)
	}
}

func TestSeedIsIdempotent(t *testing.T) {
	f := newFakeStack(t)
	first := runSeed(t, f)
	creates, uploads := f.creates, len(f.uploads)
	second := runSeed(t, f)

	if f.creates != creates || len(f.uploads) != uploads {
		t.Errorf("the second run created %d things and uploaded %d files", f.creates-creates, len(f.uploads)-uploads)
	}
	if first.Bound != second.Bound || first.Unbound != second.Unbound {
		t.Errorf("results differ:\n%+v\n%+v", first, second)
	}
}

func TestSeedRepairsADocumentThatLacksItsImages(t *testing.T) {
	f := newFakeStack(t)
	runSeed(t, f)
	f.documents[0]["text"] = boundBody()
	runSeed(t, f)
	if !hasImages(f.documents[0]["text"].(string)) {
		t.Error("the images were not restored")
	}
}

func TestSeedRestoresTheBindingBlock(t *testing.T) {
	f := newFakeStack(t)
	runSeed(t, f)
	f.collections[0]["description"] = "```yaml\ntemplate: neutral\n```"
	runSeed(t, f)
	for _, c := range f.collections {
		if c["name"] == BoundCollectionName && !strings.Contains(c["description"].(string), "binding:") {
			t.Errorf("binding block not restored: %q", c["description"])
		}
	}
}

func TestLoginRejectsWrongCredentials(t *testing.T) {
	f := newFakeStack(t)
	if _, err := Login(t.Context(), f.outline.URL, DefaultEmail, "wrong"); err == nil {
		t.Fatal("a wrong password produced a session")
	}
}

func TestLoginFormAction(t *testing.T) {
	page, _ := url.Parse("http://dex.example.invalid:5556/dex/auth/local/login?back=")
	got, err := loginFormAction(`<FORM method="post"
	  action="/dex/auth/local/login?back=&amp;state=q1">`, page)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "http://dex.example.invalid:5556/dex/auth/local/login?back=&state=q1" {
		t.Errorf("got %s", got)
	}
	if _, err := loginFormAction("<html>no form</html>", page); err == nil {
		t.Error("a page without a form was accepted")
	}
}

func TestSeedAddsADocumentWithAFaultyMetadataBlockToTheBoundCollection(t *testing.T) {
	f := newFakeStack(t)
	res := runSeed(t, f)

	if res.Invalid.DocumentTitle != InvalidDocumentTitle || res.Invalid.CollectionID != res.Bound.CollectionID {
		t.Fatalf("invalid item: %+v", res.Invalid)
	}
	if res.Invalid.DocumentID == res.Bound.DocumentID {
		t.Fatal("the faulty document is the bound document")
	}
}

func TestSeedMakesASecondUserAViewer(t *testing.T) {
	f := newFakeStack(t)
	res := runSeed(t, f)
	if f.viewerRole != "viewer" {
		t.Errorf("role of the second user = %q, want viewer", f.viewerRole)
	}
	if res.Viewer.Email != DefaultViewerEmail || res.Viewer.Password != DefaultPassword {
		t.Errorf("viewer = %+v", res.Viewer)
	}
}
