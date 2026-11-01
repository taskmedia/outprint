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

### Create a snapshot

```sh
npm run docs:version -- 0.1
```

This writes `versions.json`, `versioned_docs/version-0.1/` and
`versioned_sidebars/version-0.1-sidebars.json`. One snapshot per minor release;
patch releases do not get their own. Commit all three.

### What to change in config at the first snapshot

With a snapshot the unreleased docs move to `/dev/`. A lone version with
`path: 'dev'` would empty the root, which is why this is not set yet. In
`docusaurus.config.js`:

1. In `presets[0].docs`, enable (do **not** set `lastVersion`: the newest
   snapshot is the default automatically):

   ```js
   versions: {
     current: {label: 'dev', path: 'dev', banner: 'unreleased'},
   },
   ```

2. In `themeConfig.navbar.items`, add the version dropdown:
   `{type: 'docsVersionDropdown', position: 'right'}`.

Search is version-aware once versions exist: the index is built per version
and the search box on a versioned page searches that version.

Both states (no snapshot, one snapshot) have been built strictly.

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
