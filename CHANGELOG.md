# Changelog

## [1.1.1](https://github.com/taskmedia/outprint/compare/v1.1.0...v1.1.1) (2026-11-08)


### Bug Fixes

* **gateway:** send X-Forwarded-Proto when Outline is reached over http but names itself https ([#166](https://github.com/taskmedia/outprint/issues/166)) ([5cf797b](https://github.com/taskmedia/outprint/commit/5cf797ba423faca07e68b79900dc84f993b849e8))

## [1.1.0](https://github.com/taskmedia/outprint/compare/v1.0.1...v1.1.0) (2026-11-08)


### Features

* **image:** add German hyphenation and babel support to the Worker image ([#163](https://github.com/taskmedia/outprint/issues/163)) ([f25799d](https://github.com/taskmedia/outprint/commit/f25799dc847f3e56a94f23def5352450e7bfcae5))

## [1.0.1](https://github.com/taskmedia/outprint/compare/v1.0.0...v1.0.1) (2026-11-06)


### Bug Fixes

* **gateway:** reject cross-site render requests and extend blocked address ranges ([#152](https://github.com/taskmedia/outprint/issues/152)) ([36894f7](https://github.com/taskmedia/outprint/commit/36894f7ed8e43d5012e193635941979d7678ca89))
* require the worker token in compose and warn when it is empty ([#153](https://github.com/taskmedia/outprint/issues/153)) ([5041b6e](https://github.com/taskmedia/outprint/commit/5041b6e35abc8834e5514e7adaea66345df80422))
* restrict template input targets to scanned tex extensions ([#146](https://github.com/taskmedia/outprint/issues/146)) ([2eceacb](https://github.com/taskmedia/outprint/commit/2eceacb1ae710e3bca4fb048e68c3dbd018b407a))

## [1.0.0](https://github.com/taskmedia/outprint/compare/v0.1.0...v1.0.0) (2026-11-03)


### Features

* add a copy-permalink button and pinned heading links to the docs ([#99](https://github.com/taskmedia/outprint/issues/99)) ([541e9f2](https://github.com/taskmedia/outprint/commit/541e9f208a99945a7994ebb3fb33c196eeb49a6d))
* add a settings registry and generate the settings reference ([#120](https://github.com/taskmedia/outprint/issues/120)) ([#126](https://github.com/taskmedia/outprint/issues/126)) ([cdef257](https://github.com/taskmedia/outprint/commit/cdef257b28df8dc5c4f0b7c27dd2c5265b88a33e))
* build older docs versions from git tags instead of committed snapshots ([#84](https://github.com/taskmedia/outprint/issues/84)) ([aca0080](https://github.com/taskmedia/outprint/commit/aca00800117cf4c807c4a63c57462225ad3a6fe8))
* default the letter subject to the page title unless subject is explicitly empty ([#90](https://github.com/taskmedia/outprint/issues/90)) ([f672f82](https://github.com/taskmedia/outprint/commit/f672f82ddee4d618b81d0a442822709e2e015206))
* edge-proxy recipes (nginx, Caddy, Traefik) to load the PDF action without the injector ([#108](https://github.com/taskmedia/outprint/issues/108)) ([d9f8e62](https://github.com/taskmedia/outprint/commit/d9f8e62801f6e18aac49e6ca96e52fe107cc71ed))
* hide the metadata block in outline's reading view ([#89](https://github.com/taskmedia/outprint/issues/89)) ([b737ef3](https://github.com/taskmedia/outprint/commit/b737ef34c6c78855f2aaaf73a72825deea35e2f1))
* load the pdf action through a mounted outline page template ([#110](https://github.com/taskmedia/outprint/issues/110)) ([8404d96](https://github.com/taskmedia/outprint/commit/8404d96994ac3424c8510ebb9aca66a6c27ddb02))
* send the button options with the status and document the script tag contract ([#112](https://github.com/taskmedia/outprint/issues/112)) ([c84ab2c](https://github.com/taskmedia/outprint/commit/c84ab2c5a58329616ae2bee65ed9aed906d7c023))
* show a .pdf file-type hint on the PDF item in the Export menu ([#88](https://github.com/taskmedia/outprint/issues/88)) ([819b06e](https://github.com/taskmedia/outprint/commit/819b06e6d2db05318155e606a7cc2e2a6bbefeb0))


### Bug Fixes

* copy the docs snapshot into versioned_docs/version-X.Y ([#81](https://github.com/taskmedia/outprint/issues/81)) ([eb6537b](https://github.com/taskmedia/outprint/commit/eb6537b1be79345dc05270430864e4af2dd674b1))
* make the documentation screenshots reproducible ([#91](https://github.com/taskmedia/outprint/issues/91)) ([4da32fd](https://github.com/taskmedia/outprint/commit/4da32fd223d813542358ab542642abdf7ac6116f))
* publish the outprint image and let release-please pick the next version ([#109](https://github.com/taskmedia/outprint/issues/109)) ([e4f1e17](https://github.com/taskmedia/outprint/commit/e4f1e17f1570ec0ebee9049a4641f555b6af0cb5))
* render diagrams under reduced motion, add traefik on kubernetes tab ([#133](https://github.com/taskmedia/outprint/issues/133)) ([e995079](https://github.com/taskmedia/outprint/commit/e995079fe78f97cc2456c49df9a315a998bffd5f))

## 0.1.0 (2026-11-01)


### Features

* add landing page to the docs site ([#67](https://github.com/taskmedia/outprint/issues/67)) ([f52e558](https://github.com/taskmedia/outprint/commit/f52e558b46eab9a4c1feb2bdbe76a5933fc787b3))
* add project scaffolding and sanitize gate ([3426b30](https://github.com/taskmedia/outprint/commit/3426b30537e36365a8aaf288d7e8e60f1b813925))
* admission control with slot, queue and per-user limits ([2685a72](https://github.com/taskmedia/outprint/commit/2685a727f6f89572b2c21d69a86d4a0c359164a2))
* allow reviewed comments in the pre-publish audit ([#78](https://github.com/taskmedia/outprint/issues/78)) ([fb812d6](https://github.com/taskmedia/outprint/commit/fb812d631dd56e8b8b1a16526333c63676f1cf34))
* docusaurus docs site skeleton ([#56](https://github.com/taskmedia/outprint/issues/56)) ([1957d81](https://github.com/taskmedia/outprint/commit/1957d81a6947463898d3003d5c5bd70320603387))
* generate documentation screenshots from the dev stack ([#66](https://github.com/taskmedia/outprint/issues/66)) ([4ef1515](https://github.com/taskmedia/outprint/commit/4ef15157459c9e31e8e4e79d8687e43664206a53))
* harden worker pipeline and pin fidelity fixtures ([4327661](https://github.com/taskmedia/outprint/commit/43276612d26740f2df11ad5848858a6824ddd843))
* injector, download button and status endpoint ([b800e85](https://github.com/taskmedia/outprint/commit/b800e856f7dc79e9bd6b8bd5af73b0dc22dc2a8c))
* link errors to the docs of the running version ([#71](https://github.com/taskmedia/outprint/issues/71)) ([3eae7b4](https://github.com/taskmedia/outprint/commit/3eae7b4c1f172ce9e3d96b4eb360c7bf4e23f99e))
* metadata block, layering and typed fields in render ([1b3c3f4](https://github.com/taskmedia/outprint/commit/1b3c3f4c2c7b85c97767df2650e1373d703f1c7d))
* package loader and validate cli ([f427da8](https://github.com/taskmedia/outprint/commit/f427da8bf0f78abf15e14a877d55f3dde2c16d50))
* place the PDF action in Outline's Export menu ([#76](https://github.com/taskmedia/outprint/issues/76)) ([580182d](https://github.com/taskmedia/outprint/commit/580182d1abc2d16dbc32f3601c860149d822edae))
* release chain with version embedding and multi-arch image publishing ([#57](https://github.com/taskmedia/outprint/issues/57)) ([cec783a](https://github.com/taskmedia/outprint/commit/cec783a0a39ad28c64f0a423a41d2b00249e1c4d))
* render cli tracer bullet with neutral letter template ([7ff4bf2](https://github.com/taskmedia/outprint/commit/7ff4bf2a2868a4a2fe6805562bfad7beb37cc55d))
* render raster images from outline attachments ([cb646e6](https://github.com/taskmedia/outprint/commit/cb646e661c88e50b937071055b7448149023b195))
* render svg images and logos as vectors ([1c879eb](https://github.com/taskmedia/outprint/commit/1c879eb59f238a7ff6e5353e55f85ca994994f8b))
* seeded dev stack and browser e2e ([b82357d](https://github.com/taskmedia/outprint/commit/b82357d7b33c0c464d21c3d92d465923a15bac0c))
* select template via collection binding block ([254dd43](https://github.com/taskmedia/outprint/commit/254dd4350c41096cf9c984652ca3ca92ba967737))
* snapshot the docs for every new minor release ([#75](https://github.com/taskmedia/outprint/issues/75)) ([63c4f24](https://github.com/taskmedia/outprint/commit/63c4f24e9a1aea75b9a9d9c1b1488580383435ed))
* split renderer into gateway and worker roles ([790601b](https://github.com/taskmedia/outprint/commit/790601b9f459c04e4928ac67fcce098867d81307))


### Bug Fixes

* mark yaml blocks by position and root key instead of info string ([#50](https://github.com/taskmedia/outprint/issues/50)) ([e86fbc4](https://github.com/taskmedia/outprint/commit/e86fbc49d97dbdcc7e9ba17131343457e94c2298))
* typeset captioned figures in the neutral letter ([b9a217b](https://github.com/taskmedia/outprint/commit/b9a217bdcb4fee62a6e9547c0584d2081ed895c2))
