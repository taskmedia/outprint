---
sidebar_position: 1
---

# The injector and the PDF action

**Background.** How the injector adds the PDF action to Outline, and every setting it reads.

To install it, see [Add the button](../install/add-the-button.mdx). The injector is an optional part of the install: it is what puts the action into
Outline's interface. Without it the
Gateway and the Worker still work. Outline has no
plug-in point for a toolbar button, and Outprint does not fork Outline. The **injector** is a small reverse proxy in front of Outline that adds
one `<script>` tag to Outline's HTML pages. The script puts a **Formatted PDF**
action into Outline's interface on document pages, but only where the Gateway
says a Binding applies.

```
browser --> edge (TLS) --> injector --> Outline
                              |
                              +-- /pdf/*  --> Gateway   (script, status, render)
```

The injector is its own image (`injector` target of the Dockerfile): a static
binary on a distroless base, non-root, with no shell and no TeX toolchain.

## How it works

1. Requests below the Gateway prefix (`/pdf`) go to the Gateway with the prefix
   removed. Only `GET` and `HEAD` are forwarded, and only clean paths. The
   browser's `Cookie` header travels along, which is how the Gateway acts as the
   user towards Outline. This keeps the browser on one origin: no CORS, no
   second cookie.
2. Every other request goes to Outline, with the public `Host` header kept and
   `X-Forwarded-*` appended.
3. A response is rewritten only when it is a complete page: status 200,
   `Content-Type: text/html`, no `Content-Range`, and not an answer to `HEAD`.
   The tag is inserted before the last `</body>`, or before `</head>` if the page
   has no body end. A page that already carries the tag is not touched again.
   Everything else is streamed unchanged: JSON, images, scripts, 204, 206, 304,
   error pages, event streams and WebSocket upgrades (Outline uses a WebSocket
   for collaboration).

The injector adds this tag to every complete page of Outline. Options that differ
from the defaults are added as `data-` attributes (see [Configuration](#configuration)):

```html
<script src="/pdf/button.js" defer data-outline-pdf-injector></script>
```

### Compression

The injector does not need to decode what the browser asks for, so it avoids
compressed HTML:

- For a request that accepts `text/html` (a navigation) it asks Outline for
  `Accept-Encoding: identity` and drops `If-None-Match` and `If-Modified-Since`.
  The conditional headers matter: a `304` would send the browser back to a copy
  cached before the injector was in place.
- If Outline sends gzip HTML anyway, the injector decodes it, injects the tag and
  sends the page uncompressed, with `Content-Encoding`, `ETag` and `Content-MD5`
  removed and `Content-Length` recomputed. Compress at the edge (Traefik, nginx).
- A page in another encoding (`br`, `zstd`, ...) cannot be rewritten. It is passed
  through byte for byte without the button, and a warning is logged.
- A page larger than 8 MiB is streamed without the tag (decoded if it was gzip).

A corrupt gzip body is answered with `502`.

### Content-Security-Policy

The script is served from the page's own origin (`/pdf/button.js`), so a policy
that allows `'self'` for scripts needs no change. If Outline's policy lists
`script-src` without `'self'`, set `OUTPRINT_INJECTOR_CSP_ALLOW_SELF=true`: the injector
then adds `'self'` to `script-src` and `script-src-elem` (or creates `script-src`
from `default-src`) in the `Content-Security-Policy` header of the pages it
injects. `Content-Security-Policy-Report-Only` is left alone. Two limits:

- With `'strict-dynamic'` browsers ignore `'self'`; that needs a nonce, which the
  injector does not add.
- The script's styles use a constructed stylesheet and fall back to a `<style>`
  element, which needs `style-src 'unsafe-inline'`.

Check the policy of your Outline with the browser's developer tools; if the action
does not appear and the console reports a blocked script, this is the cause.

## Configuration

| Variable | Meaning | Default |
|----------|---------|---------|
| `OUTPRINT_INJECTOR_UPSTREAM` | base URL of Outline (the only place requests go, besides the Gateway); required | |
| `OUTPRINT_INJECTOR_LISTEN` | listen address | `:8090` |
| `OUTPRINT_INJECTOR_GATEWAY_URL` | base URL of the Gateway; empty disables the prefix route | none |
| `OUTPRINT_INJECTOR_GATEWAY_PREFIX` | path prefix routed to the Gateway | `/pdf` |
| `OUTPRINT_INJECTOR_SCRIPT_URL` | `src` of the injected script | `<prefix>/button.js` |
| `OUTPRINT_INJECTOR_CSP_ALLOW_SELF` | add `'self'` to Outline's script policy | `false` |
| `OUTPRINT_INJECTOR_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |

### Placement of the action

These variables apply to the whole installation. They are not per Collection or
per document: where the action sits is an operator decision.

Set them on the **Gateway**: it sends them to the script
([The script tag](script-tag.md#options-come-from-the-gateway)). The injector
still reads the same variables and writes them as `data-` attributes on the
tag, which win over the Gateway. That keeps an install that sets them on the
injector working. Setting them on the Gateway only is enough and the tag stays
bare.

| Variable | Meaning | Default |
|----------|---------|---------|
| `OUTPRINT_BUTTON_PLACEMENT` | `menu`: Export → Formatted PDF in the document menu. `button`: a header button before Share | `menu` |
| `OUTPRINT_BUTTON_STYLE` | `icon-label` or `icon`; applies to the header button | `icon-label` |
| `OUTPRINT_BUTTON_FALLBACK` | `on` or `off`: show a floating button when the anchor is missing | `on` |
| `OUTPRINT_BUTTON_FALLBACK_DELAY` | how long to wait for the anchor before the fallback, for example `3s` | `3s` |
| `OUTPRINT_BUTTON_METADATA_BLOCK` | `collapse`, `hide` or `show`: what readers and authors see of the [metadata block](#hiding-the-metadata-block) | `collapse` |
| `OUTPRINT_BUTTON_LABELS` | override the texts, see below | built-in |

A wrong value stops the Gateway (and the injector, if you set it there) at startup with a message naming the variable.

`OUTPRINT_BUTTON_LABELS` is a JSON object. The keys are `menu` (the menu item),
`button` (the text of the header button) and `tooltip` (the tooltip and
accessible name of the header button). Each holds a text per language code. A
text is 1 to 80 characters:

```sh
OUTPRINT_BUTTON_LABELS='{"menu":{"en":"Letterhead PDF","de":"PDF mit Briefkopf"},"tooltip":{"de":"PDF mit Briefkopf herunterladen"}}'
```

For a language the order is: your text, the built-in text, your English text,
the built-in English text. English and German are built in; another language
needs your text for every label you care about.

`GET /_injector/healthz` answers 200 without calling Outline. The image has no
shell, so the container health check runs `/outprint-injector healthcheck <url>`.

If an edge proxy already routes `/pdf` to the Gateway, leave `OUTPRINT_INJECTOR_GATEWAY_URL`
empty and set `OUTPRINT_INJECTOR_SCRIPT_URL` if the Gateway is reachable under another path.

## The PDF action

`web/button.js` is embedded in the outprint image and served by the Gateway at
`GET /button.js` (`text/javascript`, `nosniff`, `Cache-Control: public, max-age=300`).
Plain ES2019, no dependencies. For where the action appears and how it looks to
users, see [the PDF action settings](../reference/settings.md#pdf-action).

- It recognises `/doc/<title>-<urlId>` (and `/doc/<uuid>`, with `/edit` or
  `/history` after it) and follows Outline's client-side navigation.
- It calls `GET <prefix>/status/<id>` and places the action only for
  `{"available": true}`. Any other answer or failure shows nothing. A malformed
  Binding block counts as available, so the author sees the error on click.
  The id may be the urlId from the URL; the Gateway passes it to Outline's
  `documents.info`, which accepts both. The answer also carries `language`, the
  Outline language of the user (for example `de_DE`), read from `auth.info`
  with the user's own cookie. It is left out when Outline names none.
- **Finding the anchors.** Outline has no extension point for its menus, so the
  script reads the page. It does not rely on classes (they are generated) or on
  translated text:
  - the **…** button of the document header is the right-most button with
    `aria-haspopup="menu"` at the top of the page;
  - **Share** is the dialog trigger (`aria-haspopup="dialog"`) in the same row
    that has no `aria-label` of its own and no avatar inside;
  - the **Export** submenu is the open menu whose items end in `.md`, `.html`
    or `.textpack`.
- **Menu placement.** Outline mounts a menu when it opens and removes it when it
  closes. A `MutationObserver` on the page adds the item whenever the Export
  submenu appears, once per menu. The item copies its neighbour, reacts to the
  pointer and the arrow keys like the other items, and on selection closes the
  menu the way Outline does, which also returns the focus to **…**.
- **Header button placement.** The button copies the look of **Share** and is
  put back if Outline redraws the header.
- **Fallback.** If no anchor shows up within the wait, the floating button
  appears (smaller than before) and `console.warn` says so. The floating button
  is a native `<button>` with an `aria-label`, keyboard focusable, with a visible
  focus ring, light and dark colours, and motion only when the user has not asked
  for less.
- On click it fetches `<prefix>/render/<id>` and saves the PDF under the name
  from `Content-Disposition` (`filename*` first, then `filename`). The header
  button and the floating button are disabled while the render runs; in menu
  placement a "Creating PDF…" panel under **…** stands in.
- Failures show a dismissible panel (close button or Escape) anchored under the
  control that was used:
  - a validation error list (422): message, with field, line and source;
  - `429`: "busy, retry in N seconds" from `Retry-After`;
  - `413`, `504`, `500`, `401`, `403`, `404` and a lost connection each have a
    plain sentence.
  All text is set with `textContent`; nothing from the server is parsed as HTML.
- It talks to its own origin only. No id or content goes to a third party.

Tests: `task test-web` runs `web/button.test.js` with Node's built-in runner (pure
functions plus a small DOM stub). `task test` runs it when Node is installed and
prints a note when it is not; CI always runs it. The placement against Outline's
real markup is covered by the browser tests, see
[Compatibility](../install/operate/compatibility.md).

## Hiding the metadata block

On documents where a Binding applies, the script takes the [metadata
block](../reference/metadata-block.md) (the `yaml` code block at the very
start of the document) out of sight. Outline opens a document for everyone who
may edit it straight in the editor ("seamless editing"), so there is no separate
reading view for authors. The script therefore tells two kinds of visitors apart
by whether the editor is editable:

| Visitor | `collapse` (default) | `hide` | `show` |
|---------|----------------------|--------|--------|
| Reader (no edit permission) | block hidden | block hidden | block shown |
| Author (can edit) | block folded behind a **PDF details** toggle above the document | block shown | block shown |

With `collapse` an author opens the block with one click on the toggle and folds
it again with a second. In Outline the toggle is called **PDF details**
(**PDF-Details** in German); the docs call the block itself the metadata block.
The state is not remembered: each document starts folded.

The operator decides with `OUTPRINT_BUTTON_METADATA_BLOCK`. For `hide` and `show`
the injector adds `data-metadata-block="hide"` or `"show"` to the script tag; the
default adds nothing. Like the other `OUTPRINT_BUTTON_*` options it applies to the
whole installation, not per Collection or Document, and a wrong value stops the
injector at startup.

This is a cosmetic convenience, not access control:

- It depends on Outline's markup (a `.ProseMirror` editor with `contenteditable`
  and code blocks carrying their language). An Outline update can break it; the
  block is then simply shown again.
- The block can flash briefly before the script hides it.
- Share links, Outline's own exports and Outline's search still show it.

## Security notes

- **Fixed upstream.** The injector forwards to `OUTPRINT_INJECTOR_UPSTREAM` and the
  Gateway only. A request line or `Host` header naming another host does not
  change where it goes.
- **Hop-by-hop headers** (`Connection` and the headers it names, `Keep-Alive`,
  `Proxy-Authorization`, `TE`, `Upgrade` except for a real upgrade) are not
  forwarded in either direction.
- **Run it behind your edge.** `X-Forwarded-For` is appended to what the client
  sent; a client that reaches the injector directly can forge it. Terminate TLS
  and set trusted forwarding headers in the proxy in front.
- **Cookies.** The cookie goes to Outline (as always) and to the Gateway, which
  sends it on to Outline only. The injector does not read or log it.
- **The Gateway must not be exposed cross-origin.** It sends no CORS headers on
  purpose and has no authentication of its own; the user's Outline cookie is the
  credential, so it has to arrive from the Outline origin. In production publish
  only the injector (or route `/pdf` to the Gateway at your edge), not the
  Gateway's own port. The `ports` entry of the Gateway in the Compose file is for
  trying it out.
- **Timeouts.** Header read 10 s, idle 2 min, Outline response headers 2 min,
  Gateway response headers 5 min (a render may wait in the queue). Request and
  response bodies have no overall timeout, so uploads, event streams and
  WebSockets work. Request headers are capped at 64 KiB.
- **Error bodies** do not contain internal addresses; they are logged instead.

## Routing example

The next example puts Traefik in front of the injector, with invented hosts. TLS
ends at Traefik, which also compresses (the injector sends HTML uncompressed).
This is the Traefik configuration:

<!-- validate: skip Traefik dynamic configuration, a third-party format -->
```yaml
# dynamic configuration
http:
  routers:
    wiki:
      rule: Host(`wiki.example.invalid`)
      entryPoints: [websecure]
      service: injector
      tls: {}
  services:
    injector:
      loadBalancer:
        servers:
          - url: http://injector.internal.example.invalid:8090
```

These are the injector's settings for that setup:

```sh
OUTPRINT_INJECTOR_UPSTREAM=http://outline.internal.example.invalid:3000
OUTPRINT_INJECTOR_GATEWAY_URL=http://gateway.internal.example.invalid:8080
```

Outline keeps its public URL (`URL=https://wiki.example.invalid`) and its
`Host`-based origin checks, because the injector forwards the public `Host`.

With Docker Compose, `deploy/compose/compose.yaml` has an `injector` service next
to `gateway` and `worker`. It publishes port 8090 (`OUTPRINT_INJECTOR_PORT`) and needs
`OUTPRINT_INJECTOR_UPSTREAM`; see [Run Outprint](../install/run-outprint.mdx).
