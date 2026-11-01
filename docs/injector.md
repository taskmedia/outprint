# Injector and download button

Outline has no plug-in point for a toolbar button, and the project does not fork
Outline. The **injector** is a small reverse proxy in front of Outline that adds
one `<script>` tag to Outline's HTML pages. The script shows a "Download PDF"
button on document pages, but only where the Gateway says a Binding applies.

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

The injected tag:

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
`script-src` without `'self'`, set `INJECTOR_CSP_ALLOW_SELF=true`: the injector
then adds `'self'` to `script-src` and `script-src-elem` (or creates `script-src`
from `default-src`) in the `Content-Security-Policy` header of the pages it
injects. `Content-Security-Policy-Report-Only` is left alone. Two limits:

- With `'strict-dynamic'` browsers ignore `'self'`; that needs a nonce, which the
  injector does not add.
- The button's styles use a constructed stylesheet and fall back to a `<style>`
  element, which needs `style-src 'unsafe-inline'`.

Check the policy of your Outline with the browser's developer tools; if the button
does not appear and the console reports a blocked script, this is the cause.

## Configuration

| Variable | Meaning | Default |
|----------|---------|---------|
| `INJECTOR_UPSTREAM` | base URL of Outline (the only place requests go, besides the Gateway); required | |
| `INJECTOR_LISTEN` | listen address | `:8090` |
| `INJECTOR_GATEWAY_URL` | base URL of the Gateway; empty disables the prefix route | none |
| `INJECTOR_GATEWAY_PREFIX` | path prefix routed to the Gateway | `/pdf` |
| `INJECTOR_SCRIPT_URL` | `src` of the injected script | `<prefix>/button.js` |
| `INJECTOR_CSP_ALLOW_SELF` | add `'self'` to Outline's script policy | `false` |
| `INJECTOR_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |

`GET /_injector/healthz` answers 200 without calling Outline. The image has no
shell, so the container health check runs `/injector healthcheck <url>`.

If an edge proxy already routes `/pdf` to the Gateway, leave `INJECTOR_GATEWAY_URL`
empty and set `INJECTOR_SCRIPT_URL` if the Gateway is reachable under another path.

## The button

`web/button.js` is embedded in the renderer image and served by the Gateway at
`GET /button.js` (`text/javascript`, `nosniff`, `Cache-Control: public, max-age=300`).
Plain ES2019, no dependencies.

- It recognises `/doc/<title>-<urlId>` (and `/doc/<uuid>`, with `/edit` or
  `/history` after it) and follows Outline's client-side navigation.
- It calls `GET <prefix>/status/<id>` and shows the button only for
  `{"available": true}`. Any other answer or failure hides it. A malformed
  Binding block counts as available, so the author sees the error on click.
  The id may be the urlId from the URL; the Gateway passes it to Outline's
  `documents.info`, which accepts both.
- The floating button sits at the bottom right. It is a native `<button>` with an
  `aria-label`, keyboard focusable, with a visible focus ring, light and dark
  colours, and motion only when the user has not asked for less. Where exactly
  the button belongs in Outline's UI is not settled; this placement needs no
  knowledge of Outline's DOM.
- On click it fetches `<prefix>/render/<id>`, disables itself while the render
  runs, and saves the PDF under the name from `Content-Disposition` (`filename*`
  first, then `filename`).
- Failures show a dismissible panel (close button or Escape):
  - a validation error list (422): message, with field, line and source;
  - `429`: "busy, retry in N seconds" from `Retry-After`;
  - `413`, `504`, `500`, `401`, `403`, `404` and a lost connection each have a
    plain sentence.
  All text is set with `textContent`; nothing from the server is parsed as HTML.
- It talks to its own origin only. No id or content goes to a third party.

Tests: `make test-web` runs `web/button.test.js` with Node's built-in runner (pure
functions plus a small DOM stub). `make test` runs it when Node is installed and
prints a note when it is not; CI always runs it.

## Security notes

- **Fixed upstream.** The injector forwards to `INJECTOR_UPSTREAM` and the
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

Traefik in front of the injector, with invented hosts. TLS ends at Traefik, which
also compresses (the injector sends HTML uncompressed):

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

```sh
INJECTOR_UPSTREAM=http://outline.internal.example.invalid:3000
INJECTOR_GATEWAY_URL=http://gateway.internal.example.invalid:8080
```

Outline keeps its public URL (`URL=https://wiki.example.invalid`) and its
`Host`-based origin checks, because the injector forwards the public `Host`.

With Docker Compose, `deploy/compose/compose.yaml` has an `injector` service next
to `gateway` and `worker`. It publishes port 8090 (`INJECTOR_PORT`) and needs
`INJECTOR_UPSTREAM`; see [deploy-compose.md](deploy-compose.md).
