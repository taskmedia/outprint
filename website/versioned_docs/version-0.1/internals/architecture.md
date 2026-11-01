---
sidebar_position: 2
---

# Architecture

One image, `outprint`, runs in two roles. The reasons for the split are in
[ADR 0001](../adr/0001-split-renderer-into-gateway-and-worker.md); this page
describes the roles, the contract between them, the request flow and how
errors are mapped.

```
browser --> injector --> Outline          (adds the button script to HTML pages)
   |           |
   |           +-- /pdf/* --cookie--> Gateway --cookie--> Outline
   |                                     |
   |                                     | job bundle (no cookie, no URL, no secret)
   |                                     v
   |                                  Worker (no network, no secrets) --> PDF
```

A third deployable, the **injector**, is a separate slim image
([Injector and PDF action](../deploy/injector.md)). It is not a role of the outprint image.

## Roles

| Role | Command | Talks to | Holds |
|------|---------|----------|-------|
| Gateway | `outprint gateway` | the browser, Outline, the Worker | the user's cookie, for one request |
| Worker | `outprint worker` | the Gateway only | nothing; at most a shared bearer token |

Both load and validate all packages at startup and refuse to start, naming
every cause, if anything is wrong (the same check as `outprint validate`). The
Gateway needs the packages to resolve the Metadata Block; the Worker needs
them to render.

### Gateway

`GET /render/{documentId}` renders the Document and answers with the PDF. It
is a GET with the id in the path so that the "Download PDF" button can be a
plain link: the browser sends the user's cookie by itself and downloads the
file. The endpoint changes nothing on the server, and the response is
`Cache-Control: no-store`. `GET /healthz` answers 200. `GET /button.js` serves the button script (embedded
from `web/button.js`; `text/javascript`, `nosniff`, `max-age=300`, no cookie
needed). The Gateway sends no CORS headers: it is reached through the injector's
`/pdf` prefix, on the origin of Outline ([Injector and PDF action](../deploy/injector.md)).

`GET /status/{documentId}` tells the button script whether to show the button
(see [Status endpoint](#status-endpoint)). The Template and Configuration come
from the Collection's Binding block ([Binding](../configure/choose-a-template.md)) and nowhere
else: no query parameter or header selects or overrides them.

Configuration (environment):

| Variable | Meaning | Default |
|----------|---------|---------|
| `OUTLINE_URL` | base URL of Outline; the only origin the cookie is sent to | required |
| `OUTLINE_PUBLIC_URL` | Outline's own `URL` setting, when `OUTLINE_URL` is an internal address; a redirect to this origin is fetched from `OUTLINE_URL` ([Image fetching](../deploy/image-fetching.md#outline-behind-an-internal-address)) | none |
| `OUTPRINT_WORKER_URL` | base URL of the Worker | required |
| `OUTPRINT_WORKER_TOKEN` | optional shared bearer token | none |
| `OUTPRINT_LISTEN` | listen address | `:8080` |
| `OUTPRINT_TIMEOUT` | render limit; the Gateway waits 30 s longer for the Worker | `60s` |
| `OUTPRINT_SERVICE_CONFIG` | service configuration file | |
| `OUTPRINT_ADMISSION_SLOTS`, `_QUEUE_DEPTH`, `_PER_USER`, `_MAX_WAIT` | override the `admission` keys of the service configuration, see [Limits and admission control](../deploy/limits-and-admission.md) | `1`, `4`, `1`, `60s` |
| `OUTPRINT_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |

### Worker

`POST /render` takes a bundle and answers `application/pdf` or a JSON error
`{"phase", "kind", "job_id"}`. `GET /healthz` answers 200 and needs no token.
The pipeline behind it is described in [Worker pipeline](worker.md).

- One job per instance. While a job runs, a second request gets `503` with
  kind `busy`.
- A fresh `0700` job directory below `OUTPRINT_WORK_DIR` per job, removed
  afterwards. Production mounts the work root as a tmpfs.
- After a timeout, a kill, a panic or any unexpected failure the Worker sends
  its response, then shuts down and exits non-zero. While it is shutting down it
  answers `busy`. The container runtime restarts it (`restart: always`), so the
  next job starts in a clean process. Ordinary document failures (a TeX error,
  an image problem) do not restart it.
- If `OUTPRINT_WORKER_TOKEN` is set, `/render` requires
  `Authorization: Bearer <token>`; the comparison is constant-time and a
  mismatch is `401`.
- It refuses to start as UID 0 and logs its security posture: uid, gid,
  `no_new_privs`, whether the root file system is read-only, the effective
  capabilities, the number of network interfaces that are up, and the *names*
  (never values) of environment variables that look like secrets. Anything weaker
  than the shipped baseline is logged as a warning.
- Server limits: request body capped by `bundle.MaxEncodedBytes`, header read
  10 s, body read 2 min, idle 60 s.

## The job bundle

The bundle (`internal/bundle`) is the only thing that crosses from the Gateway
to the Worker. It is JSON, `Content-Type: application/json`:

```json
{
  "markdown": "Dear ...\n",
  "values": {"title": "Spring plant sale", "recipient": ["Ada Example", "1 Sample Lane"]},
  "template": "neutral",
  "documentType": "letter",
  "configuration": "example",
  "images": {"fig-1.png": "<base64>"}
}
```

| Field | Meaning |
|-------|---------|
| `markdown` | the Document body, without its Metadata Block |
| `values` | the merged, typed field values of the Document Type: string, list of strings or bool |
| `template`, `configuration` | package names; the Worker also checks the pairing against its allow-list |
| `documentType` | one of the Template's Document Types |
| `images` | image files by bundle name (`images/<name>` in the Markdown). Fetched and canonicalised by the Gateway (an SVG is checked and passed on as `.svg`; the Worker converts it to a vector PDF), see [Images in the pipeline](images.md) |

There is no field for a cookie, a URL or a secret, and unknown fields are
rejected, so none can be added by accident. A test serialises a bundle built
from a request with a canary cookie and asserts that neither the canary nor the
Outline host reaches the Worker.

The Worker validates every bundle: names must be identifiers, value names and
types are checked, and the limits below apply. The limits live in
`internal/bundle`:

| Limit | Value |
|-------|-------|
| Markdown | 1 MiB |
| images | 50, 10 MiB each, 50 MiB in total |
| values | 200 names, 64 KiB per value, 200 list items, 1 MiB in total |
| PDF returned by the Worker | 50 MiB |
| pages of the PDF | 500 |

The per-job limits (time, pages, PDF size, TeX runs) and the container limits
are named in one place, `internal/limits`; see [Security](security.md).

## Request flow

1. The browser calls `GET /render/{documentId}` with the user's cookie.
2. The Gateway checks the id (`[A-Za-z0-9_-]`, at most 100 characters; an
   Outline UUID or the urlId at the end of a `/doc/<title>-<urlId>` path) and
   that a cookie is present.
3. It calls Outline `documents.info` with that cookie and no other credential.
   Outline's own permissions decide whether the user may read the Document.
4. It selects the Binding. It reads the overview of the Document's Collection
   with `collections.info` and the same cookie, on every render and without a
   cache, and `binding.Resolve` turns it into a Binding: the first `yaml`
   block with a `binding:` key, or the default Binding of the service configuration only
   when the overview has no block at all. A broken block, a second block, an
   unknown name or a pair off the allow-list is a 422 with `source:
   collection`, never a fallback to the default. No block and no default is a
   422 `no_binding`. A Document without a Collection has no block. See
   [Binding](../configure/choose-a-template.md).
5. `metadata.Resolve` merges the Metadata Block with the derived values: `title`
   is the Outline title and `date` is the last-modified date (`updatedAt`, in
   UTC, as an ISO date).
6. The Gateway builds and validates the bundle and posts it to the Worker.
7. It streams the PDF back with `Content-Disposition` and `Cache-Control:
   no-store`.

The user's cookie leaves the Gateway at steps 3 and 4 only, and only to the
origin in `OUTLINE_URL`. The Outline client never follows a redirect, so a redirect
cannot carry the cookie elsewhere.

### Status endpoint

`GET /status/{documentId}` (cookie forwarded, nothing rendered) answers
`200 {"available": true|false, "reason": "..."}` with `Cache-Control: no-store`:

| available | reason | when |
|-----------|--------|------|
| true | `collection_binding` | the Collection has a valid block |
| true | `collection_binding_invalid` | the Collection has a block that does not validate; the author sees the error on click |
| true | `default_binding` | no block, and the installation has a default Binding |
| false | `no_binding` | no block and no default Binding |

Failures are `{"message","phase"}` with the status of the cause: 400 for a bad
id, 401 without a cookie or when Outline answers 401, 403 and 404 when the
user cannot read the Document or its Collection, 500 otherwise. A client must
treat any non-200 answer as "do not decide" and may keep the button hidden.

Admission control ([Limits and admission control](../deploy/limits-and-admission.md)) sits in `gateway.process`. After the
Binding is selected the Gateway asks Outline for the confirmed user id
(`auth.info`) and counts the render against that user, before the images are
fetched; a slot of the global limit is taken right before the Worker call. Both
are released by `defer` on every path. A refusal is a 429 with `Retry-After`
and an error list with `source: service`, code `busy` or `user_limit`. The
status endpoint does neither.

### File name

The file name comes from the Outline title: control characters (CR and LF
included), path separators, quotes and `: * ? < > | ; %` are replaced by a
space, whitespace is collapsed, leading and trailing dots and spaces are
trimmed, and the name is cut at 100 characters. An empty result is `document`.
The header carries an ASCII fallback and the UTF-8 name per RFC 5987:

```
Content-Disposition: attachment; filename="Gruesse aus Koeln.pdf"; filename*=UTF-8''Gr%C3%BC%C3%9Fe%20aus%20K%C3%B6ln.pdf
```

## Error mapping

Validation problems are the structured body of [Metadata Block reference](../templates/metadata-reference.md):
`{"errors":[{"field","source","code","message","line"}]}`. Every other error is
`{"message", "phase", "job_id"}`, where the message is a fixed sentence. No
response carries a log excerpt, TeX output, Document content or a cause.

Every error also carries `docs_url`: a ready-made link to its entry in
[When something goes wrong](../write/when-something-goes-wrong.md), in the docs of
the version that is running. The Gateway adds it to each entry of the error
list (from `code`) and to the other bodies (from the failure kind), so the
button needs no URL logic. A response without a code or kind has none. The
`render --json-errors` output of the CLI does not carry it.

The link is `https://outprint.task.media/<path>/write/when-something-goes-wrong/#<code>`.
The base address is a constant (`internal/docslink`), not a setting. `<path>`
comes from the embedded version:

| Version | Docs path |
|---------|-----------|
| `0.3.2`, `v0.3.2` | `0.3`, the docs of the minor release |
| `dev`, `0.3.2-dev`, `-dirty`, `v0.3.2-5-gabc1234`, a bare commit | `dev`, the unreleased docs |
| a pre-release such as `0.4.0-rc.1`, an empty or unparsable version | none: the latest docs at the site root |

The site serves the newest release at its root, so `/<newest minor>/` is a
redirect there that keeps the `#<code>` anchor, and until the first release
snapshot exists `/dev/` redirects to the root ([site setup](https://github.com/taskmedia/outprint/blob/main/website/README.md#versions)).
The button renders only `https:` links to that origin, as "How to fix" next
to each error and a "Help" link to the docs home of the same version. A test
fails when a code in the source has no anchor on the page
(`internal/docslink`, with `task docs:errors-check`).

| Situation | Status | Notes |
|-----------|--------|-------|
| invalid document id | 400 | |
| no cookie, or Outline answers 401 | 401 | |
| Outline answers 403 | 403 | |
| Outline answers 404 | 404 | |
| metadata errors | 422 | the error list; all problems at once |
| no Binding | 422 | one error with `source: collection`, `code: no_binding` |
| invalid Binding block | 422 | errors with `source: collection` and a `binding_*` code, see [Binding](../configure/choose-a-template.md) |
| Outline answers 403 or 404 for the Collection | 403 / 404 | `phase: binding`, no fallback |
| Markdown, bundle or PDF over a limit | 413 | |
| too many pages (`pages_exceeded`), PDF too large (`pdf_too_large`) | 413 | `phase: output` |
| the job ran out of memory or scratch space (`resource`) | 413 | the Worker restarts |
| Worker busy | 429 | with `Retry-After` |
| renderer at capacity (`busy`), user already has a render (`user_limit`) | 429 | admission control, with `Retry-After`; error list with `source: service`, see [Limits and admission control](../deploy/limits-and-admission.md) |
| Outline does not confirm the user | 401 / 403 / 500 | never counted by address |
| Worker timeout | 504 | `phase: timeout` |
| Outline or Worker unreachable, TeX error, any other failure | 500 | only phase and job id |

## Logging

Both roles log with `log/slog` to stderr.

- The Gateway logs one line per request: `job`, `document` (Outline's id),
  `phase`, `kind`, `status`, `duration_ms`. A 5xx adds the cause, which is an
  error of the clients and carries no response text. The `job` is the Worker's
  job id when the Worker answered, else a Gateway request id.
- The Worker logs, per failed job, `job`, `phase`, `duration_ms`, `exit_code`
  and the first TeX error line, see [Worker pipeline](worker.md#logging). The first
  TeX error line stays in the Worker's log and does not travel to the Gateway;
  find it by the job id.
- Neither role logs Document content, metadata values or the cookie.
