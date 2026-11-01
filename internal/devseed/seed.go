// Package devseed provisions the development stack and the browser tests: it
// signs in as the fictional user (which makes Outline create the workspace),
// then creates a Collection with a Binding and a Document with a table and
// two images, and a Collection without a Binding whose Document has two
// captioned figures. It is safe to run again.
package devseed

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// Defaults match deploy/dev/dex/config.yaml. They are published in the
// repository and valid for the local development stack only.
const (
	DefaultEmail    = "sam@example.invalid"
	DefaultPassword = "dev-only-password"
)

// Config is what Run needs.
type Config struct {
	// BaseURL is the origin of the injector, the only public entry.
	BaseURL  string
	Email    string
	Password string
	// Content holds chart.png and diagram.svg (deploy/dev/seed).
	Content fs.FS
	Logf    func(format string, args ...any)
}

// Item is a created Collection with its Document.
type Item struct {
	CollectionID   string `json:"collectionId"`
	CollectionName string `json:"collectionName"`
	DocumentID     string `json:"documentId"`
	DocumentTitle  string `json:"documentTitle"`
	// DocumentPath is /doc/<slug>-<urlId>, relative to BaseURL.
	DocumentPath string `json:"documentPath"`
}

// Result is written to a JSON file for the browser tests.
type Result struct {
	BaseURL  string `json:"baseUrl"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Bound    Item   `json:"bound"`
	Unbound  Item   `json:"unbound"`
	// Captions are the captions of the figures in the Unbound Document.
	Captions []string `json:"captions"`
}

type collection struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type document struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Text  string `json:"text"`
}

// Run signs in and makes sure the content exists.
func Run(ctx context.Context, cfg Config) (Result, error) {
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	token, err := Login(ctx, cfg.BaseURL, cfg.Email, cfg.Password)
	if err != nil {
		return Result{}, err
	}
	s := &seeder{c: NewClient(cfg.BaseURL, token), content: cfg.Content, logf: cfg.Logf}
	res := Result{BaseURL: strings.TrimRight(cfg.BaseURL, "/"), Email: cfg.Email, Password: cfg.Password,
		Captions: []string{ChartCaption, DiagramCaption}}

	if res.Bound, err = s.ensureItem(ctx, itemSpec{
		collection: BoundCollectionName, overview: BoundCollectionOverview(),
		title: BoundDocumentTitle, body: boundBody(), withImages: boundFigures,
	}); err != nil {
		return Result{}, err
	}
	if res.Unbound, err = s.ensureItem(ctx, itemSpec{
		collection: UnboundCollectionName, overview: UnboundCollectionOverview(),
		title: UnboundDocumentTitle, body: unboundBody(), withImages: unboundFigures,
	}); err != nil {
		return Result{}, err
	}
	return res, nil
}

type seeder struct {
	c       *Client
	content fs.FS
	logf    func(string, ...any)
}

// itemSpec describes one Collection with its Document. withImages builds the
// text of the Document once the images are uploaded.
type itemSpec struct {
	collection, overview, title, body string
	withImages                        func(chartURL, diagramURL string) string
}

// ensureItem creates or repairs one Collection and its Document.
func (s *seeder) ensureItem(ctx context.Context, spec itemSpec) (Item, error) {
	col, err := s.ensureCollection(ctx, spec.collection, spec.overview)
	if err != nil {
		return Item{}, err
	}
	doc, err := s.ensureDocument(ctx, col.ID, spec.title, spec.body)
	if err != nil {
		return Item{}, err
	}
	if !hasImages(doc.Text) {
		s.logf("adding images to %q", spec.title)
		if doc, err = s.addImages(ctx, doc, spec.withImages); err != nil {
			return Item{}, err
		}
	}
	return Item{CollectionID: col.ID, CollectionName: col.Name, DocumentID: doc.ID, DocumentTitle: doc.Title, DocumentPath: doc.URL}, nil
}

func (s *seeder) ensureCollection(ctx context.Context, name, overview string) (collection, error) {
	var list []collection
	if err := s.c.Call(ctx, "collections.list", map[string]any{"limit": 100}, &list); err != nil {
		return collection{}, err
	}
	for _, c := range list {
		if c.Name != name {
			continue
		}
		if strings.TrimSpace(c.Description) != strings.TrimSpace(overview) {
			s.logf("updating the overview of %q", name)
			var updated collection
			if err := s.c.Call(ctx, "collections.update", map[string]any{"id": c.ID, "description": overview}, &updated); err != nil {
				return collection{}, err
			}
			return updated, nil
		}
		return c, nil
	}
	s.logf("creating collection %q", name)
	var created collection
	err := s.c.Call(ctx, "collections.create", map[string]any{"name": name, "description": overview, "permission": "read_write"}, &created)
	return created, err
}

func (s *seeder) ensureDocument(ctx context.Context, collectionID, title, text string) (document, error) {
	// collections.documents reads the Collection's own tree. documents.list
	// goes through a per-user cache of readable Collections and misses a
	// Collection created a moment ago, which would make the seed create the
	// Document twice.
	var tree []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := s.c.Call(ctx, "collections.documents", map[string]any{"id": collectionID}, &tree); err != nil {
		return document{}, err
	}
	for _, n := range tree {
		if n.Title == title {
			var found document
			err := s.c.Call(ctx, "documents.info", map[string]any{"id": n.ID}, &found)
			return found, err
		}
	}
	s.logf("creating document %q", title)
	var created document
	err := s.c.Call(ctx, "documents.create", map[string]any{
		"title": title, "collectionId": collectionID, "text": text, "publish": true}, &created)
	return created, err
}

// addImages uploads the two images as attachments of the Document and
// rewrites its text with them.
func (s *seeder) addImages(ctx context.Context, doc document, build func(chartURL, diagramURL string) string) (document, error) {
	chart, err := s.upload(ctx, doc.ID, chartFile, "image/png")
	if err != nil {
		return document{}, err
	}
	diagram, err := s.upload(ctx, doc.ID, diagramFile, "image/svg+xml")
	if err != nil {
		return document{}, err
	}
	var updated document
	err = s.c.Call(ctx, "documents.update", map[string]any{
		"id": doc.ID, "text": build(chart.URL, diagram.URL), "publish": true}, &updated)
	if err != nil {
		return document{}, err
	}
	if !hasImages(updated.Text) {
		return document{}, errors.New("outline stored the document without its images")
	}
	return updated, nil
}

func (s *seeder) upload(ctx context.Context, documentID, name, contentType string) (Attachment, error) {
	data, err := fs.ReadFile(s.content, name)
	if err != nil {
		return Attachment{}, fmt.Errorf("seed content: %w", err)
	}
	return s.c.Upload(ctx, documentID, name, contentType, data)
}
