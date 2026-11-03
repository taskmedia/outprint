---
sidebar_position: 7
---

# Development stack and end-to-end tests

One command gives you Outline with a login, a bound and an unbound Collection
and an example Document, with the PDF button in front of it:

```sh
task dev
```

It prints the URL and the test login:

```
Outline (dev):  http://localhost:8090
Sign in with:   sam@example.invalid / dev-only-password   (DEV ONLY, fictional)
```

Open the URL, sign in, and open **Bound / Spring plant sale**: the *Download
PDF* button sits at the bottom right. **Unbound / Garden notes** has none.

**Prerequisites**: Docker with Compose v2.24 or newer, Go (to run the seed),
`openssl`. The browser tests also need Node 22 or newer. Nothing here is for
production: the credentials are published in the repository and the stack
listens on `127.0.0.1` only.

## Commands

| Command | What it does |
|---------|--------------|
| `task dev` | generate the dev secrets once, build and start the stack (`up -d --build --wait`), run the seed, print URL and login |
| `task dev:seed` | run only the seed (safe to repeat) |
| `task dev:logs` | follow the logs of all services |
| `task dev:down` | stop the stack, keep the database and files |
| `task dev:down-volumes` | stop and delete the database and files; the next `task dev` starts empty |
| `task dev:clean` | delete the generated secrets and the seed output (stop with `dev:down-volumes` first) |
| `task e2e:install` | `npm ci` and the Playwright Chromium in `e2e/` |
| `task e2e:run` | start the stack if needed, seed, run the browser tests |
| `task e2e:run-nginx` | the same tests with nginx (`deploy/dev/nginx/nginx.conf`) instead of the injector; stop the other stack first, both use port 8090 |

## What runs

`deploy/dev/compose.yaml` is layered over `deploy/compose/compose.yaml`
(`-f` twice, see the `DEV_COMPOSE` variable of `deploy/dev/Taskfile.yml`). The shipped
Gateway, Worker and injector definitions, including the hardened Worker
baseline, are reused unchanged; the dev file adds Outline, Postgres, Redis and
Dex and points the Gateway and the injector at Outline.

```
browser --> http://localhost:8090  injector --> Outline --> Postgres, Redis
                                       |
                                       +-- /pdf/* --> Gateway --> Worker (no network)
browser --> http://localhost:5556  Dex (sign-in page only)
```

- **The injector is the only public entry.** Outline has no published port, the
  Gateway's port is not published, and Outline's `URL` is
  `http://localhost:8090`, so sign-in redirects and cookies stay on that origin.
- **One service configuration for the dev stack** (`deploy/dev/service.yaml`):
  `neutral` with `example` is allowed and there is **no `defaultBinding`**. A
  Collection without a `binding:` block therefore has no button; the second
  browser test relies on it.
- Files are stored by Outline itself (`FILE_STORAGE=local`, a volume).

### Sign-in: one issuer, two addresses

Outline and the browser must both be able to use Dex, but `localhost` means
something different inside a container. The stack avoids a host-name trick by
not using OIDC discovery: Outline gets the three endpoints separately.

| Endpoint | Address | Used by |
|----------|---------|---------|
| authorization (`OIDC_AUTH_URI`) | `http://localhost:5556/dex/auth` | the browser |
| token (`OIDC_TOKEN_URI`) | `http://dex:5556/dex/token` | Outline, over the compose network |
| user info (`OIDC_USERINFO_URI`) | `http://dex:5556/dex/userinfo` | Outline |

Dex's published port `5556` makes the first one work from the host, and
Dex's `issuer` is `http://localhost:5556/dex`. The redirect back is
`http://localhost:8090/auth/oidc.callback`.

### Images: the Gateway reaching files

Outline answers `/api/attachments.redirect?id=...` with a redirect to
`http://localhost:8090/api/files.get?sig=...` (its public URL). Inside the
Gateway container `localhost` is the Gateway itself, so the Gateway gets
`OUTLINE_PUBLIC_URL=http://localhost:8090` and fetches that redirect from
`OUTLINE_URL=http://outline:3000` instead. See
[Image fetching](../install/components/image-fetching.md#outline-behind-an-internal-address). No `imageHosts` is
needed.

### Secrets

`deploy/dev/gen-secrets.sh` writes `deploy/dev/.generated/outline.env` (Outline's
`SECRET_KEY`, `UTILS_SECRET` and OIDC client secret) and `dex.env` (the same
client secret) with `openssl rand`, once, into an untracked directory. Only two
dev values are committed: the fictional user's password hash in
`deploy/dev/dex/config.yaml` and the Postgres password on a network that has no
route out. Delete the generated directory (`task dev:clean`) to roll the secrets.

## Seeding

`cmd/seed` (package `internal/devseed`) is the only seeding implementation; `task
dev` and `task e2e:run` both run it, and the browser tests read what it wrote. It
works through Outline's own interfaces:

1. **Sign in like a browser.** It opens `/auth/oidc`, follows the redirects to
   Dex, posts the credentials to Dex's form and follows the redirects back.
   Outline creates the workspace and the user at the first sign-in. The session
   cookie it receives is also accepted as a bearer token.
2. **Create the content through the API**, only what is missing:
   - Collection **Bound**: its overview holds the Binding block
     (`template: neutral`, `configuration: example`), and Document **Spring plant
     sale** with body text, a table and two images (a PNG and an SVG, uploaded
     with `attachments.create`, sized with Outline's `=240x` syntax).
   - Collection **Unbound**: no Binding block, and Document **Garden notes** with
     two *captioned* images.
3. **Write `deploy/dev/.generated/seed.json`** with the user, the ids and the
   paths, for the tests.

Run it again at any time: existing Collections and Documents are found by name
and left alone; the Binding block is restored if someone edited it away.

Two Outline behaviours shaped the seed, both found by running against a real
Outline and both tracked:

- Outline keeps only the first word of a code fence's info string (issue #42).
  The Binding and the Metadata Block are therefore plain `yaml` fences, told
  apart by the root key `binding:` and by their place at the top of the
  Document, so the seed's text is what the editor would store as well.
- The neutral letter cannot typeset a captioned figure (issue #43), so the bound
  Document has images without captions and the captioned ones sit in the
  unbound Document, where they are only inspected, not rendered.

## Browser tests

`e2e/` holds Playwright (Chromium, one worker). `tests/download.spec.ts` has two
tests:

1. **A bound document downloads as a PDF.** Sign in through Dex's form, open
   *Spring plant sale*, wait for the `/pdf/status/<id>` answer, open the **…**
   menu, **Export**, click **Formatted PDF**, take the download, check the
   `%PDF` header and the text (the opening sentence, the name of the example
   Configuration, a table cell). It also fetches the export of the captioned
   Document through the API and checks that the captions are the alt text (the
   verification recorded in [Images in the pipeline](images.md#notes)).
2. **No Binding, no action.** Open *Garden notes*, wait for the status answer
   (`available: false`) and check that there is no floating button, no header
   button and no menu item.

`tests/placement.spec.ts` covers the placement of the action in Outline's real
markup, with one shared sign-in because Outline rate-limits logins:

- the Export menu entry downloads a PDF, closes the menu and returns the focus;
- the entry appears once however often the menu opens, and the arrow keys and
  Enter reach and use it;
- no entry for a document without a Binding;
- `placement=button`: the header button sits directly before Share, with the
  `icon` style keeping an accessible name and a tooltip;
- operator labels, and the German label of a user whose Outline language is
  German;
- a second user with the viewer role (the seed makes `riley@example.invalid` a
  viewer, with the same password): the menu entry works, and without Share the
  header button sits before "…";
- a markup change that removes the anchor: the floating fallback and a console
  warning, or nothing with `fallback=off`.

The tests change the script options the way the injector does, by adding `data-`
attributes to the script tag in the page they receive. To try a change to
`web/button.js` without rebuilding the stack, serve it from disk:
`E2E_BUTTON_JS=../web/button.js npx playwright test`.

Everything goes through the injector origin; the only other host the browser
visits is Dex's sign-in page. No test sleeps: they wait for the network answers
and the elements. Failures keep a trace and a screenshot
(`e2e/test-results`, plus an HTML report in CI).

The PDF text comes from `pdftotext -layout` (poppler) when it is installed, as
in the golden tests, and from `pdfjs-dist` otherwise or with `E2E_PDF_TEXT=js`.
CI installs poppler.

```sh
task e2e:install   # once: npm ci, Chromium
task e2e:run
cd e2e && npx playwright test --ui   # while writing tests (stack running, seeded)
```

## Documentation screenshots

`task screenshots` regenerates the six PNGs in `docs/assets/screens/` that the
documentation shows. They are committed; run the task before a release and
whenever the Outline UI or the button changes, and commit the result.
The seed gives the workspace and the user fixed avatar images (Outline would
colour them from their random ids), and the optimise step keeps a committed PNG
whose pixels are unchanged, so a rerun only shows real changes in `git status`.

```sh
task dev            # the stack must be up
task screenshots    # seeds, drives it, writes and optimises the PNGs
```

It is its own Playwright config (`e2e/playwright.screenshots.config.ts`), so
`task e2e:run` never runs it. It signs in as the seeded user, uses a fixed
1280x800 viewport at 2x, the light theme and reduced motion, waits for network
idle and the button rather than sleeping, and shows only the fictional seed
data (the byline with the update time is hidden; the one date in the example
letter is painted over). The seed adds a third Document, *Autumn bulb order*,
with a misspelled Metadata Block, for the error shot. The example letter is the
first page of the PDF from the button, converted with `pdftoppm` (host) or
Alpine's poppler in Docker.

The PNGs are palette-quantised (256 colours, no dithering: lossy, but flat UI
stays flat) and recompressed with oxipng, through the npm packages `sharp` and
`@napi-rs/image`. Each file stays under 150 KB or the task fails. Two runs
produce byte-identical files.

`task screenshots` also reports what it changed: after the optimise step it
lists the screenshots that differ from the committed ones. With
`SCREENSHOTS_DIFF_DIR` set it copies them there, and in GitHub Actions it adds
the list to the job summary.

That is the visual check. `e2e.yml` runs `task screenshots` after the tests, on
the stack that is already up, and uploads the changed images as the
`screenshots-changed` artifact (14 days). The step is informational: it is
`continue-on-error` and cannot fail the job, because the committed images come
from a developer machine and the runner's fonts may render slightly
differently. A difference after an intended UI change means: run
`task screenshots` and commit the result.

## CI

`.github/workflows/e2e.yml` runs the suite on every push to `main`, on
`workflow_dispatch`, and on pull requests that touch the injector, the Outline
client, the Gateway, the seed, `deploy/`, `web/`, `e2e/`, the Dockerfile or the
`Taskfile.yml`, or the committed screenshots. It uploads the Playwright report,
the traces and the compose logs when it fails.

`.github/workflows/e2e-latest-outline.yml` runs weekly (Monday) and on demand
with the newest stable Outline release instead of the pinned one (it exports
`OUTLINE_IMAGE`, which wins over the pin file). It is not a required check: a
failure opens an issue titled "E2E against latest Outline failed", or comments
on the open one, with the run URL.

### Link checks

Internal links, anchors and markdown links are checked on every pull request:
the strict build in `docs.yml` ("docs build (strict)", a required check) fails
on a broken one.

`.github/workflows/docs-links.yml` checks the external links of the built site
with lychee, monthly (on the 1st) and on demand. It is deliberately not a required
check and never fails a pull request: findings go into one issue, "docs: broken
external links", which the next run rewrites and closes once the links are
fine. Decision (#92): a third-party host that is down or rate-limiting must not
block an unrelated pull request, so no blocking external check is added. The
site's own domain is excluded from that run; its links are internal ones and
the strict build already covers them.

## Pinned versions

`deploy/dev/versions.env` pins Outline, Dex, Postgres and Redis by explicit tag,
never `latest`:

```
OUTLINE_IMAGE=outlinewiki/outline:1.12.0
DEX_IMAGE=ghcr.io/dexidp/dex:v2.46.1
POSTGRES_IMAGE=postgres:17.11-alpine
REDIS_IMAGE=redis:7.4.12-alpine
```

The file is not called `.env` (which is git-ignored) and is passed with
`--env-file`. The lines are plain `NAME=repo:tag` pairs with a Renovate
annotation above each; a Renovate regex manager can bump them. Dependabot does
not read env files.

To bump one: check the tag exists (`docker manifest inspect <image>:<tag>`),
edit the line, run `task dev:down-volumes dev e2e:run`, and commit if it passes. Outline
migrates its database on start, so test a bump on an empty volume.

## Troubleshooting

- **`task dev` waits at Outline.** The first start runs migrations and takes up
  to a minute. `task dev:logs` shows why if it fails.
- **Port 8090 or 5556 is taken.** Set `OUTPRINT_INJECTOR_PORT` for the first, and stop
  the other program for Dex: its port is part of the issuer address.
  `OUTPRINT_INJECTOR_PORT` also needs a matching `DEV_URL` and Outline `URL`, so
  changing it is not supported yet.
- **"sign-in did not produce a session".** The seed could not log in as the dev
  user: the stack was started with other secrets or another Dex. Run
  `task dev:down-volumes dev:clean dev`.
- **Outline answers but the button is missing.** The browser may have a cached
  page from before the injector; do a hard reload. Check
  `curl -s localhost:8090/pdf/button.js | head`.
- **A render fails.** `task dev:logs`, look for `render failed` in the gateway
  and worker. TeX errors name the phase.
- **Reset everything.** `task dev:down-volumes dev:clean`.
