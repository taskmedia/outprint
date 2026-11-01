// Package inject is the injector: a reverse proxy in front of Outline that
// adds the "Download PDF" script tag to HTML pages and, optionally, routes a
// path prefix to the Gateway so the browser stays on one origin. Everything
// that is not a plain HTML page, including WebSocket upgrades and event
// streams, passes through untouched. See docs/injector.md.
package inject
