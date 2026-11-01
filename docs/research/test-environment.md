# Research: local test environment and PDF verification

Date: 2026-10-15. Status legend: **[V]** verified against a primary source or a local experiment, **[I]** inferred / not directly confirmed.

## Recommendation in one paragraph

Use three tiers. Tier 1 (unit, every push, seconds): Go tests against a fake Outline HTTP server (`httptest`) plus a text-level check of the Markdown-to-LaTeX pipeline. Tier 2 (PDF golden, every push, ~1-3 min, needs the TeX image only): render fixed Markdown fixtures to PDF, assert with `pdftotext -layout` against golden text files and `pdfinfo` (page count, embedded fonts), and optionally compare rasterised pages with a tolerance, run inside the same pinned container image everywhere. Tier 3 (E2E, PR and main, ~5-10 min): a single `docker compose` stack (Outline + Postgres + Redis + a static-password OIDC provider + the injector + the PDF service), driven by Playwright, which logs in once, creates content through the Outline API and clicks the injected button. Drop minikube/Traefik for contributors; keep them, if at all, as an optional "production-like" smoke check.

## (a) Running Outline locally with authentication

### Compose vs kind/minikube

- Outline's own docs describe Docker Compose self-hosting with Outline, PostgreSQL, Redis and an HTTPS portal; minimum env is `URL`, `DATABASE_URL`, `REDIS_URL`, `SECRET_KEY`; migrations run automatically at container start. [V] https://docs.getoutline.com/s/hosting/doc/docker-7pfeLP5a8t
- The upstream repo's own dev `docker-compose.yml` is only Redis + Postgres. [V] https://github.com/outline/outline/blob/main/docker-compose.yml
- Nothing in the E2E needs Kubernetes features (the Traefik routing is two path rules: `/pdf` to the service, everything else through the injector). A compose network with the injector as the single entry point reproduces it. [I, based on the routing of an earlier proof of concept]
- Docker is available locally (Docker 29.x, desktop-linux context); `kind` and `minikube` are installed locally as well, but contributors should not need them. [V, `command -v`]
- Cost of kind/minikube: needs a cluster bootstrap, image loading (`minikube image build` / `kind load`), port-forward restarts after pod restarts (documented pain in the proof of concept). Compose gives `up --wait`, a healthcheck gate and `down -v` for a clean slate. [I]

Verdict: compose. Use kind only if Helm/manifest correctness itself becomes a test target.

### Authentication options Outline supports

- Outline has no native email+password; it offers SSO/OIDC/SAML depending on edition. [V] https://docs.getoutline.com/s/hosting/doc/authentication-7ViKRmRY5o
- Providers shipped as plugins: azure, discord, email, figma(not auth), github, gitlab, google, notion, oidc, passkeys, slack, ... [V] https://github.com/outline/outline/tree/main/plugins
- OIDC env vars: `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, and either manual `OIDC_AUTH_URI` / `OIDC_TOKEN_URI` / `OIDC_USERINFO_URI` (+ optional `OIDC_LOGOUT_URI`, `OIDC_USERNAME_CLAIM`) or `OIDC_ISSUER_URL` for discovery. [V] https://github.com/outline/outline/blob/main/.env.sample and `plugins/oidc/server/auth/oidc.ts`
- Email magic link (SMTP): works for existing users only. The proof of concept observed `email.signin` returning 404 when no team exists. [V in the notes of an earlier proof of concept]; consistent with the code path below.
- First sign-in creates the team: `teamProvisioner` creates a new team (and `isNewTeam: true`) when no authentication provider/team exists; a second provider is attached to the single existing team only if the email domain is allowed. [V] https://github.com/outline/outline/blob/main/server/commands/teamProvisioner.ts

So OIDC is the only option that bootstraps an empty instance without touching the database. Choice of OIDC provider:

| Provider | Fit | Notes |
|---|---|---|
| Dex with `enablePasswordDB` + `staticPasswords` | Good, proof of concept already uses it | One small YAML file; users are static (email, bcrypt hash, username, userID). [V] https://dexidp.io/docs/connectors/local/ |
| Keycloak | Overkill | Heavy JVM image, realm import needed [I] |
| Mock OIDC servers (e.g. `mock-oauth2-server`, `oidc-server-mock`) | Possible, lighter, auto-login options | Not verified here; evaluate in implementation [I] |

Recommendation: keep Dex (already known to work in the proof of concept), run it as a compose service with a static config file. Constraint: browser and Outline container must resolve the same issuer host. Simplest: set the Dex issuer to a compose hostname and have Playwright reach it via a published port plus `extra_hosts`/`/etc/hosts` alias, or use Playwright's `--host-resolver-rules` launch arg. [I; this host-name duality is the main fiddly part, and the reason for the proof of concept fixing `URL` and redirect URI to `http://localhost:8080`.]

### Seeding team / user / collection / document non-interactively

1. **Team and user**: created by the first OIDC login (see above), which Playwright performs once in a setup project (fill Dex form, land in Outline). [V for team creation logic; the form fill is trivial]
2. **API key**: API keys are `ol_api_` + 38 chars, created in Settings > API & Apps, used as `Authorization: Bearer <key>`. [V] https://www.getoutline.com/developers  Two ways to obtain one non-interactively:
   - Via Playwright UI automation in the setup project (robust against schema changes, slightly slower). [I]
   - Direct SQL insert: the `ApiKey` model stores `hash = sha256(hex)` of the full key and `last4`; the plaintext `secret` column is deprecated. So `INSERT INTO api_keys (...)` with `hash = sha256('ol_api_<fixed>')` works after the user exists. [V for model fields and hash function: `server/models/ApiKey.ts`, `server/utils/crypto.ts`; exact table/column list and NOT NULL constraints are [I], inspect the migrations before relying on it.]
3. **Collection / document**: `collections.create`, `documents.create` (JSON POST to `/api/<method>`). [V] https://www.getoutline.com/developers  The proof of concept also forwarded the user session cookie to the Outline API, so a cookie from the Playwright login works as an alternative to an API key. [V in the proof of concept design; cookie-based API auth otherwise [I]]
4. Avoid: Outline's `bootstrap.ts` and test factories are for Outline's own test suite and need its source tree; not a supported seeding path. [V that no seed script exists in `server/scripts`; "not supported" is [I]]

Fixtures: keep Markdown fixtures in the repo (the same files feed tier 2), and have the E2E create them through `documents.create`, so tier 2 and tier 3 share input.

### Image size and startup

- Local `outlinewiki/outline:latest` is 1.66 GB on disk (uncompressed, as reported by `docker images`); `postgres:17-alpine` 415 MB, `redis:7-alpine` 58.7 MB. [V, local]
- Compressed pull size is considerably smaller (typically 40-50% of disk size) [I, not measured; Docker daemon was not queried for registry manifests]. Dex image size was not measured [I: tens of MB].
- The PDF service image with TeX Live (`texlive-luatex`, `-latex-extra`, `-lang-german`) is large (likely 1 GB+); the proof of concept Dockerfile already notes the injector should be split into a slim image. [V Dockerfile exists; size [I]]
- Cold start: Outline runs DB migrations on first boot, expect ~30-60 s before `/_health` answers. [I]

## (b) Browser E2E with Playwright

- Downloads: register `page.waitForEvent('download')` before the click, then `download.saveAs(...)` / `download.path()` / `suggestedFilename()`. Downloads are deleted when the browser context closes, so save explicitly. [V] https://playwright.dev/docs/downloads
- Important detail: the injected script fetches the PDF and triggers a download (per the proof of concept). If it instead navigated to the PDF URL, headless Chromium would render it in a viewer rather than emit a `download` event. Keep `Content-Disposition: attachment` (the proof of concept already does). [I]
- Auth: use a setup project that logs in once and saves `storageState`; keep the file in a gitignored `playwright/.auth` directory. [V] https://playwright.dev/docs/auth
- DOM injected by the proxy: run the real injector in the stack and assert on the real page, e.g. wait for the button locator and click it. That tests the injection itself, including compression handling the proof of concept verified for identity/gzip/br. `page.route()` could fake it, but defeats the purpose. [I, plus proof of concept section 6.2]
- Outline is an SPA: assert on the document URL and heading before searching for the button, since the injected script must wait for route changes. [I]
- CI recommendations: `npm ci`, `npx playwright install --with-deps`, `workers: 1` for reliability, a global timeout, upload the HTML report as artifact. Playwright discourages caching browser binaries (restore time ~ download time). [V] https://playwright.dev/docs/ci
- Docker image alternative: `mcr.microsoft.com/playwright:v<version>-noble`, pinned to the same version as the npm package, with `--init --ipc=host`. [V] https://playwright.dev/docs/docker  Not needed when Playwright runs on the host runner and compose runs the rest.
- Playwright is not installed locally (`npx` offered to fetch playwright 1.64.1), so no experiment was run. [V, local]
- Chromium only is enough for this feature; skipping Firefox/WebKit cuts install time. [I]

## (c) Verifying PDFs automatically

Local tools present: `pdftotext`, `pdftoppm`, `pdftocairo`, `pdfinfo`, `pdffonts` (poppler 26.10), ImageMagick `compare`/`magick`, `lualatex`, `pandoc`. [V, `command -v`]

Layered checks, strongest signal per flakiness first:

1. **Structural**: `pdfinfo` page count, page size (A4), `pdffonts` all fonts embedded (`emb = yes`). Embedded fonts matter: they make rendering independent of machine fonts. In an experiment, a trivial LaTeX PDF showed an embedded Type 1 font. [V, local experiment]
2. **Text**: `pdftotext -layout file.pdf -` compared with a golden `.txt` (normalise trailing whitespace and form feeds). Non-ASCII text (umlaut) extracted correctly in the experiment. [V, local] Use assertions for key strings (subject, date, table cells in order) rather than full-text equality when wording is data-driven. Layout mode keeps columns of tables aligned, so it is usable as a coarse layout check. [I]
3. **Image golden (optional, narrow)**: rasterise with `pdftoppm -r 100 -png`; compare to golden PNGs with ImageMagick `compare -metric AE -fuzz 2%` or Playwright's `toHaveScreenshot` style thresholds (`threshold` default 0.2, `maxDiffPixels` unset). [V for Playwright option semantics: https://playwright.dev/docs/test-snapshots] Two rasterisations on the same machine were byte-identical (same SHA-1). [V, local] Across poppler versions and OSes, anti-aliasing can change; Playwright itself warns that rendering differs by OS/environment and says to generate baselines in the same environment as the test. [V, same page] Hence: generate and compare goldens only inside the pinned container image, never on a contributor laptop; allow a small pixel budget (for example 0.1% of pixels with fuzz 2%) [I thresholds].
4. **Reproducibility**: LaTeX embeds `CreationDate` and trailer IDs (observed `CreationDate` in the experiment). Set `SOURCE_DATE_EPOCH` and `FORCE_SOURCE_DATE=1` for deterministic bytes; this is a documented TeX Live convention, but not confirmed here from a primary page (tug.org returned 403). [I] Do not compare PDF bytes; compare derived text/images.
5. Font rendering across machines: because TeX output embeds fonts, the layout (glyph positions) is deterministic for a given TeX Live version; the variable part is only the rasteriser. Pin the TeX Live version via the container image to stop line-break changes. [I, standard TeX behaviour]

## CI cost and time (GitHub Actions)

- Standard GitHub-hosted runners are free and unlimited on public repositories; on private repos they consume the plan's included minutes, then Linux 2-core x64 is billed at $0.006/min. [V] https://docs.github.com/en/billing/managing-billing-for-your-products/about-billing-for-github-actions and https://docs.github.com/en/billing/reference/actions-minute-multipliers
- `ubuntu-latest` specs: public repo 4 vCPU / 16 GB / 14 GB SSD; private repo 2 vCPU / 8 GB / 14 GB SSD. [V] https://docs.github.com/en/actions/reference/runners/github-hosted-runners  The 14 GB disk is relevant: Outline (1.66 GB) + Postgres + Redis + TeX image fits, but keep it lean.
- Artifact storage $0.25/GB-month, cache beyond 10 GB $0.07/GB-month. [V, same billing page]
- Estimated runtime (all [I], not measured): tier 1 under 1 min; tier 2 about 2-4 min (mostly pulling or building the TeX image; use `docker/build-push-action` with GHA cache or a prebuilt GHCR image); tier 3 about 6-10 min (pulls ~1 GB compressed, Outline migrations, Playwright browser install ~1 min). At the private-repo rate that is roughly $0.05-0.10 per full run.
- Suggested triggers: tier 1+2 on every push; tier 3 on pull requests touching the injector/service/profile and on main, plus `workflow_dispatch`.

## Three-tier plan

| Tier | What | Environment | Runtime | Failure meaning |
|---|---|---|---|---|
| 1 Unit | Go tests with `httptest` fake Outline (documents.info, auth cookie pass-through, 401/403/404 mapping, filename sanitising, metadata parsing, injector HTML rewrite incl. gzip/br) | none | seconds | logic regression |
| 2 PDF golden | Markdown fixtures through the real Pandoc+LuaLaTeX pipeline in the pinned TeX image; `pdfinfo`/`pdffonts` structural checks, `pdftotext -layout` golden text, optional low-tolerance PNG diff | single container | 1-3 min | template/rendering regression |
| 3 E2E | compose stack: Outline, Postgres, Redis, Dex, PDF service, injector; Playwright: login via Dex (setup project), seed via API, open doc, click injected button, capture download, then run tier-2 text assertions on the file | compose + Playwright on host | 5-10 min | integration regression (auth, injection, routing, download) |

Fixtures are shared across tiers 2 and 3; tier 3 should assert only a few strings from the PDF (the heavy golden work belongs to tier 2).

## Open points / risks

- Issuer hostname must be identical for browser and Outline container (see (a)); decide between `localhost` plus fixed ports or a compose alias with host resolver rules. [I]
- SQL-seeded API key needs table schema verification against the Outline version pinned in compose; pin the Outline image tag (not `latest`). [I]
- Outline CSP: the proof of concept found a script served from `/pdf/button.js` works without CSP changes. [V in the proof of concept]
- Not done: no Docker pull/size measurements, no Playwright run, no stack boot (cost/benefit: the claims above are from docs and source).

## Sources

- https://docs.getoutline.com/s/hosting/doc/docker-7pfeLP5a8t
- https://docs.getoutline.com/s/hosting/doc/authentication-7ViKRmRY5o
- https://www.getoutline.com/developers
- https://github.com/outline/outline (`.env.sample`, `docker-compose.yml`, `plugins/oidc/server/auth/oidc.ts`, `server/commands/teamProvisioner.ts`, `server/models/ApiKey.ts`, `server/utils/crypto.ts`)
- https://dexidp.io/docs/connectors/local/
- https://playwright.dev/docs/downloads, /auth, /ci, /docker, /test-snapshots
- https://docs.github.com/en/billing/managing-billing-for-your-products/about-billing-for-github-actions
- https://docs.github.com/en/billing/reference/actions-minute-multipliers
- https://docs.github.com/en/actions/reference/runners/github-hosted-runners
- Local: notes and Dockerfile of an earlier proof of concept, tool presence and a PDF experiment.

## Outcome

Implemented as planned, with these differences. The tiers are described in [../testing.md](../testing.md) and the local stack in [../development.md](../development.md). Dex, Postgres, Redis and Outline run as pinned images in Compose (`deploy/dev/versions.env`); the seeding is one Go implementation shared with the browser tests (`task dev:seed`); PDF goldens compare `pdfinfo`/`pdffonts` facts and `pdftotext -layout` text only. A rasterised PNG diff was not implemented (see [../template-author-guide.md](../template-author-guide.md#goldens-for-template-authors)).
