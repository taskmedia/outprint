# Documentation site

The Outprint docs site: [Docusaurus](https://docusaurus.io) 3, published to
GitHub Pages at <https://outprint.task.media/> (custom domain). The pages live in
[`../docs`](../docs), so GitHub's rendering and the site share the same files;
this directory holds only the site shell (config, theme, sidebar).

## Structure of the docs

`docs/` has five sections, one folder each, each with an intro page
(`README.md`) and a `_category_.json`: `write/`, `configure/`, `deploy/`,
`templates/` and `internals/`. The decision records stay in `docs/adr/` next to
their index and are shown under Internals (`sidebars.js`).

- **Landing page.** `src/pages/index.js` is the designed home page: the five
  sections as numbered rows. `docs/README.md` is the same list as a plain table
  that reads well on GitHub; it is excluded from the site so that the two do not
  fight over `/`. If you add a section, change both and `sidebars.js`.
- **Generated pages.** `docs/templates/catalogue/*.md` come from the Template
  packages: `task docs:catalogue` writes them, `task docs:catalogue-check`
  fails when they are out of date. The preview images in
  `docs/assets/screens/template-<name>.png` are rendered by
  `task docs:catalogue-preview` (needs Docker) and committed; the site build
  never renders.
- **Error catalogue.** `docs/write/when-something-goes-wrong.md` has one
  heading per error code. `task docs:errors-check` fails when the headings and the
  codes in the source differ.
- **Images in pages.** Screenshots live in `docs/assets/screens/` and are
  referenced with relative paths. `assets/**` is excluded from the docs pages,
  but images that a page references are still bundled by the build.

Repo-only pages are not published (`exclude` in `docusaurus.config.js`):
`README.md` (the plain index), `sanitize.md`, `publishing.md`, `releasing.md`,
`testing.md`, `research/**` and `assets/**`. Published pages that point at them
use absolute GitHub URLs, so the strict build stays green. Links from published
pages to anything outside `docs/` must be absolute
`https://github.com/taskmedia/outprint/blob/main/...` URLs.

Needs Node 22 (`nvm use` reads `.nvmrc`).

## Quality gates

All of these run on every pull request in `.github/workflows/docs.yml` (jobs
`docs consistency` and `docs build (strict)`); run them locally from the
repository root:

| Task | Checks |
|------|--------|
| `task docs:build` | strict build: broken links, anchors, Markdown links |
| `task docs:catalogue-check` | generated Template pages match the packages |
| `task docs:errors-check` | error catalogue and error codes in the source agree |
| `task docs:examples-check` | every YAML example is validated or consciously skipped |
| `task docs:sanitize-site` | denylist scan over `website/build` (build first) |
| `task docs:test-gates` | tests of the two scripts above |

### YAML examples

Every fenced `yaml` block in `docs/` needs a marker line directly before the
fence, an HTML comment that is invisible on GitHub and on the site (inside a
list item, indent it like the fence):

```
<!-- validate: metadata template=neutral configuration=example -->
<!-- validate: skip Traefik dynamic configuration, a third-party format -->
```

| Mode | Block is | Checked by |
|------|----------|------------|
| `metadata template=T configuration=C` | a Metadata Block (a `yaml` block, or a `markdown` block that shows one) | the metadata resolver, as the Worker runs it |
| `binding` | a Binding block (a `yaml` block, or a `markdown` block showing a Collection description) | the Binding resolver, against `config/service.yaml` |
| `configuration template=T configuration=C` | the `configuration.yaml` of a Configuration package | `outprint validate` on a copy of that package |
| `service-config` | a complete service configuration | `outprint validate --service-config` |
| `service-fragment` | part of one (`admission`, `imageHosts`) | the same, completed with `config/service.yaml` |
| `skip REASON` | an illustration, a fragment, a foreign format | nothing; the reason is mandatory |

The check lives in `scripts/docs-validate-examples/` (Go, because the binary
has no command for Metadata and Binding blocks; the package modes do run the
built binary). It fails with `file:line` and the tool's message. A `yaml` block
without a marker, an unknown mode or a marker that is not directly before a
fence also fails. `docs/templates/catalogue/` is generated from checked data
and is not scanned. Add a mode in `scripts/docs-validate-examples/check.go`.

## Run locally

```sh
cd website
npm ci
npm start        # dev server with live reload, http://localhost:3000/
```

## Build

```sh
npm run build    # strict: broken links, anchors and markdown links fail
npm run serve    # serves the build at http://localhost:3000/
```

From the repository root: `task docs:build` and `task docs:serve`. Search only works
in the built site, not under `npm start`.

## Add a locale (for example German)

The `i18n` block in `docusaurus.config.js` lists one locale, `en`; the
`i18n/` folder does not exist yet.

1. Add `'de'` to `i18n.locales`.
2. Add the locale dropdown to `themeConfig.navbar.items`:
   `{type: 'localeDropdown', position: 'right'}`. Do not add it before a second
   locale exists.
3. Generate the translation files: `npm run write-translations -- --locale de`
   (add the script `"write-translations": "docusaurus write-translations"`).
4. Copy the pages to translate into
   `i18n/de/docusaurus-plugin-content-docs/current/` (same file names as in
   `docs/`); untranslated pages fall back to English.
5. Build all locales with `npm run build`, one locale with
   `npm run build -- --locale de`.

The search plugin needs `language: ['en', 'de']` in its options.

## Versions

Until the first release exists the current docs are the only version and
live at the site root (`/`).

### How the product links into the site

Errors the product shows carry a link into the docs of the version that is
running (`internal/docslink`, see "Error mapping" in
`docs/internals/architecture.md`): `/<major>.<minor>/...` for a release,
`/dev/...` for a development build, the site root for anything else. The anchor
is the error code. These paths must always work, whatever the snapshots are:

| Link | Served as |
|------|-----------|
| `/<older minor>/...` | the snapshot of that minor |
| `/<newest minor>/...` | a redirect to `/...` (Docusaurus serves the newest release at the root and does not also serve it under its own number) |
| `/dev/...` | the unreleased docs; before the first snapshot a redirect to `/...` |

The redirects are generated by `@docusaurus/plugin-client-redirects` in
`docusaurus.config.js` from `versions.json`, so nothing changes by hand at a
release. The redirect page rewrites the location with the original query and
`#anchor`; a headless Chromium and WebKit land on the same anchor
(`/0.2/write/when-something-goes-wrong/#enum` ends at
`/write/when-something-goes-wrong/#enum` with the entry in view). A new minor
moves the old newest one from the root to its own folder, so `/0.1/...` goes
from redirect to real pages without a link breaking.

The page `write/when-something-goes-wrong` and its headings are part of this
contract: `go test ./internal/docslink` fails when a code in the source has no
heading, or when the page moves or gets a `slug`.

### Create a snapshot

```sh
npm run docs:version -- 0.1
```

This writes `versions.json`, `versioned_docs/version-0.1/` and
`versioned_sidebars/version-0.1-sidebars.json`. One snapshot per minor release;
patch releases do not get their own. Commit all three.

### What changes at the first snapshot

Nothing in `docusaurus.config.js`: it reads `versions.json`. With a snapshot

- the unreleased docs move to `/dev/` (`versions.current.path`), the snapshot
  takes the root and `/0.1/...` redirects to it,
- the version dropdown appears in the navbar,
- the search index is built per version.

Do **not** set `lastVersion`: the newest snapshot is the default
automatically. The config also builds with no snapshot (the root shows the
current docs, `/dev/...` redirects to it).

Check after the first snapshot: `npm run build`, then open
`/0.1/write/when-something-goes-wrong/#enum` and `/dev/write/when-something-goes-wrong/#enum`
in the built site (`npm run serve`); both must end on the entry.

## Logo

The logo kit lives in `docs/assets/logo/` (the single source, with its own
README and rebuild script). `npm start` and `npm run build` first run
`npm run brand`, which copies only the files the site uses (lockup in light and
dark, favicons, social preview) into `static/brand/`. That folder is generated
and git-ignored; the kit is excluded from the docs pages.

## Custom domain

The site is served at `outprint.task.media`:

- DNS: a `CNAME` record `outprint.task.media` pointing to `taskmedia.github.io`
  (the host only, no path). No wildcard records.
- Repository Settings > Pages > Custom domain: `outprint.task.media`, then
  **Enforce HTTPS** once GitHub has issued the certificate. Because the site is
  published with a GitHub Actions workflow, a `CNAME` file in the repository is
  ignored; the setting is what counts.
- If `task.media` has CAA records, `letsencrypt.org` must be allowed to issue.
- `docusaurus.config.js` has `url: https://outprint.task.media` and
  `baseUrl: '/'`. Links from the product to the docs are built from this
  address, so change it only together with those links.
