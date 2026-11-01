// Package web holds the browser script the injector adds to Outline's pages.
// The Gateway serves it so that one image carries the script and the API it
// talks to.
package web

import _ "embed"

// ButtonJS is the "Download PDF" button script (web/button.js).
//
//go:embed button.js
var ButtonJS []byte
