# Documentation site

The Outprint docs site: [Docusaurus](https://docusaurus.io) 3, published to
GitHub Pages at <https://outprint.task.media/> (custom domain). The pages live in
[`../docs`](../docs), so GitHub's rendering and the site share the same files;
this directory holds only the site shell (config, theme, sidebar).

## Structure of the docs

`docs/` has four sections on the site, one folder each, each with an intro page
(`README.md`) and a `_category_.json`: `write/`, `install/`, `configure/` and
`templates/` (the sidebar order). `internals/` and `adr/` are on GitHub only: they
are not in the sidebar or the navbar (they stay in the build, so a released version
that lists them keeps building) and published pages link to them with absolute GitHub
URLs. `research/` is excluded from the build. Every page opens with one line that says what it is
(how-to, reference or background); titles of Install pages are verbs, titles of
Reference pages are nouns.

- **Landing page.** `src/pages/index.js` is the designed home page: the
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

Until the first release tag exists the current docs are the only version and
live at the site root (`/`).

### How the product links into the site

Errors the product shows carry a link into the docs of the version that is
running (`internal/docslink`, see "Error mapping" in
`docs/internals/architecture.md`): `/<major>.<minor>/...` for a release,
`/dev/...` for a development build, the site root for anything else. The anchor
is the error code. These paths must always work, whatever the versions are:

| Link | Served as |
|------|-----------|
| `/<older minor>/...` | the docs of that minor, exported from its tag |
| `/<newest minor>/...` | a redirect to `/...` (Docusaurus serves the newest release at the root and does not also serve it under its own number) |
| `/dev/...` | the unreleased docs; before the first release tag a redirect to `/...` |

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

### Permalinks for readers

On the newest release the address bar shows the unpinned `/...`, so a copied
link would follow the docs to the next minor. Every docs page therefore ends with
a "Copy link to this version" button (keyboard reachable, confirmed with a
"Link copied" status message). It copies `https://outprint.task.media/<minor>/<path>#<anchor>`
for the version shown (`/dev/...` on the unreleased docs), built from the
version Docusaurus reports for the page (`useDocsVersion`), not from
`window.location`. Clicking a heading's `#` link copies the same pinned form
with that heading's anchor. The pinned form of the newest release is the
redirect described above, and becomes a real folder with the next minor.

The code is the swizzled `DocItem/Footer` in `src/theme/` and
`src/lib/permalink.js`; no extra dependency.

### How the older versions are built

The repository holds one copy of the docs, `docs/`. The older versions are
exported from Git when the site is built: `npm run build` and `npm start` first
run `scripts/docs-versions.mjs` (the `versions` script), which

1. finds the released minors: the tags `vX.Y.0` without a pre-release suffix
   (`v0.2.0-rc.1` and patch tags `v0.2.1` are not versions),
2. takes the source of each minor from the branch `docs-release/X.Y` if it exists
   (local or on `origin`), otherwise from the tag,
3. exports `docs/` of that ref into `versioned_docs/version-X.Y` with
   `git archive`, so the version is the docs as they were, not as they are now,
4. removes what the site does not publish from the copy: the logo kit
   (`assets/logo/`) and every file in the `exclude` list of
   `docusaurus.config.js` (read from the config, no second list); the screens stay,
5. writes the version's sidebar (`versioned_sidebars/version-X.Y-sidebars.json`)
   from `website/sidebars.js` of that ref, or the current one when the ref has
   none (Docusaurus shows no sidebar for a version without a sidebar file), and
   `versions.json`, newest first.

All of it is git-ignored and rebuilt on every run; `docusaurus.config.js` reads
`versions.json` as before. The script fails when a ref has no `docs/` folder. A
version that no longer builds is corrected on `docs-release/X.Y`
(`docs/releasing.md`, "Docs versions"). `npm test` tests the script against a
throwaway repository.

The build needs the tags and branches: a CI checkout uses `fetch-depth: 0`. A
clone without tags builds only the current docs.

The quality gates (`docs:examples-check`, `errors-check`, `catalogue-check`) read
`docs/` only and never look at `versioned_docs/`; `docs:sanitize-site` scans the
whole built site, older versions included.

### What changes at the first release

Nothing in `docusaurus.config.js`: it reads `versions.json`. As soon as a tag
`vX.Y.0` exists

- the unreleased docs move to `/dev/` (`versions.current.path`), the newest
  release takes the root and `/X.Y/...` redirects to it,
- the version dropdown appears in the navbar,
- the search index is built per version.

Do **not** set `lastVersion`: the newest release is the default
automatically. The config also builds with no release tag (the root shows the
current docs, `/dev/...` redirects to it).

Check after the first release: `npm run build`, then open
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
