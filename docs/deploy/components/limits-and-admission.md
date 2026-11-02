---
sidebar_position: 3
---

# Limits and admission control

The renderer bounds what one request can cost, and what all requests together
can cost. Every limit ends in a clear 4xx or 504 response. Nothing is ever
truncated: a Document that does not fit fails, it is not cut short.

## Limits

| Limit | Value | Enforced by | Response | Configurable |
|-------|-------|-------------|----------|--------------|
| Markdown size | 1 MiB | Gateway, before binding and images | 413 `the document is too large to render` | no |
| Images per Document | 50 | Gateway | 422 error list (`images`) | no |
| Size of one image | 10 MiB | Gateway | 422 error list | no |
| All canonical images | 50 MiB | Gateway | 422 error list | no |
| Image pixels | 40 million | Gateway, on the header | 422 error list | no |
| SVG source / conversion | 2 MiB / 20 s | Gateway pre-check, Worker | 422 / 413 | no |
| Render time | 60 s | Worker | 504 `rendering took too long; shorten the document` | `OUTPRINT_TIMEOUT` (Worker and Gateway) |
| Pages | 500 | Worker | 413 `the document has too many pages; shorten it` | `OUTPRINT_MAX_PAGES` (Worker, lower only in practice) |
| PDF size | 50 MiB | Worker, Gateway | 413 `the resulting PDF is too large; remove or shrink images` | `OUTPRINT_MAX_PDF_BYTES` (Worker, up to 50 MiB) |
| TeX runs | 2 | Worker | | no |
| Worker memory, CPUs, PIDs, scratch space | 1 GiB, 1, 128, 256 MiB | container runtime ([Deploying with Docker Compose](../platforms/docker-compose.md)) | 413 `resource` | Compose file |
| Concurrent renders | 1 | Gateway admission control | 429 | see below |
| Queue depth | 4 | Gateway admission control | 429 | see below |
| Renders per user | 1 | Gateway admission control | 429 | see below |
| Wait in the queue | 60 s | Gateway admission control | 429 | see below |

The container limits are authoritative; the application checks first so that
the answer is a clear message and not a killed process. The values are in
`internal/limits`.

## Admission control

A render is a heavy job, and one Worker runs one job at a time. The Gateway
decides who may start.

- **Global slots** (`slots`): how many renders may run at once. Set it to the
  number of Worker replicas. A request that finds all slots taken waits in a
  queue.
- **Queue** (`queueDepth`): how many requests may wait. When the queue is full
  the request is refused at once. `0` means nothing waits. Requests are served
  in order of arrival. A queued request that does not get a slot within
  `maxWait`, or whose client disconnects, leaves the queue.
- **Per user** (`perUser`): how many renders one user may have, queued and
  running together. The key is the user id that **Outline confirms** for the
  session (`auth.info`, called with the forwarded cookie). Nothing the client
  sends besides the cookie takes part: not a header, a query parameter, an
  address or the cookie text. If Outline does not confirm a user, the request
  fails (401, 403 or 500); it is never counted by address.

Where each check sits in a render:

1. The cheap checks come first: document id, cookie, the Document itself, the
   Markdown size and the Binding. They hold nothing.
2. The user is counted (`perUser`) *before* the images are fetched. Fetching
   and converting images is the expensive part of the Gateway's work, and a
   user must not be able to start many of them in parallel. A request that
   later fails validation (a metadata or image error) therefore counts against
   its user while it is being checked, but never takes a slot.
3. A slot is taken right before the Worker is called, and only then. Documents
   with errors never wait in the queue.
4. Slot and user count are given back when the request ends, whatever way it
   ends: success, error, panic, a client that disconnects, or a timeout.

The status endpoint (`GET /status/{documentId}`) renders nothing and neither
takes a slot nor counts against the user.

### The refusal

A refusal is `429 Too Many Requests` with a `Retry-After` header in whole
seconds (at least 1) and the usual error list:

```json
{"errors":[{"field":"","source":"service","code":"busy","message":"busy, retry later: the renderer is at capacity","line":null}]}
```

`code` is `busy` (no slot and no place in the queue, or the wait ran out) or
`user_limit` (the user already has a render). `source: service` means nobody
has to fix the Document: waiting helps. `Retry-After` is an estimate: about one
render, taken from a moving average of the recent render times, capped at the
render time limit. A Worker that answers busy by itself (it was reached by
more than one Gateway, or is restarting) also becomes a 429 with `Retry-After`,
with the body `{"message","phase"}`.

### Configuration

In the service configuration (strict: an unknown key is an error), every key
optional, defaults shown:

<!-- validate: service-fragment -->
```yaml
admission:
  slots: 1        # positive integer
  queueDepth: 4   # zero or a positive integer
  perUser: 1      # positive integer
  maxWait: 60s    # positive duration
```

Environment variables override the file and are validated the same way, so a
typo stops the Gateway at startup instead of silently keeping the default:

| Variable | Key |
|----------|-----|
| `OUTPRINT_ADMISSION_SLOTS` | `slots` |
| `OUTPRINT_ADMISSION_QUEUE_DEPTH` | `queueDepth` |
| `OUTPRINT_ADMISSION_PER_USER` | `perUser` |
| `OUTPRINT_ADMISSION_MAX_WAIT` | `maxWait` |

The shipped Compose file passes them through to the Gateway. The defaults
match one Worker replica. To scale, add Worker replicas and raise `slots` by
the same number; a larger `queueDepth` trades longer waits for fewer refusals.
Admission state lives in the Gateway process: with several Gateway replicas
each has its own limits, and the Worker's own busy answer stays the backstop.

## No render cache

A PDF is rendered anew on every request and every response is
`Cache-Control: no-store`. Nothing is kept between requests, so a PDF never
outlives the permission of the user to read the Document it came from.
