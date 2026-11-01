# License options for a public Pandoc/TeX-based service

Date: 2026-10-15. Not legal advice; this is a reading of primary license texts. Have a lawyer confirm if the service is commercial or the stakes are high.

## Recommendation

License the service's own code under **Apache-2.0** (MIT is an acceptable, simpler alternative). Do not use AGPL-3.0.

- Pandoc is run as a separate program through a subprocess boundary. Nothing in the Go code is a "work based on" Pandoc, so GPL copyleft does not reach the service's source.
- The obligations that do exist (GPL source offer, font/TeX notices) attach to what you *redistribute* in container images, not to the license of your own code.
- Apache-2.0 adds an explicit patent grant and a NOTICE mechanism, which fits a service that bundles third-party components. It is also the license Outline itself converts to on its Change Date, so the ecosystem story is consistent.
- AGPL-3.0 would only matter if the code incorporated AGPL/GPL code, and it would deter adopters without protecting anything the project needs protected.

## 1. Outline's license

Source: https://raw.githubusercontent.com/outline/outline/main/LICENSE (fetched 2026-10-15); README says "Outline is BSL 1.1 licensed" (https://github.com/outline/outline#license); `package.json` has `"license": "Business Source License 1.1"`.

Verbatim parameters:

- Licensor: General Outline, Inc.; Licensed Work: Outline 1.11.0
- Additional Use Grant: "You may make use of the Licensed Work, provided that you may not use the Licensed Work for a Document Service. A 'Document Service' is a commercial offering that allows third parties (other than your employees and contractors) to access the functionality of the Licensed Work by creating teams and documents controlled by such third parties."
- Change Date: 2030-10-12; Change License: Apache License, Version 2.0
- The license states it "is not an Open Source license" until the Change Date.
- Terms: "All copies of the original and modified Licensed Work, and derivative works of the Licensed Work, are subject to this License."

Implications for a companion service (my reading of the text, not a statement from Outline):

- The restriction is on *using the Licensed Work* (running Outline) as a hosted Document Service for third parties. A separate program that calls Outline's HTTP API, or a user script injected via a reverse proxy, is not Outline's code and is not a "derivative work" merely by interoperating. The restriction does not mention API clients, plugins-by-API or proxies.
- The BSL applies if the repo *copies* Outline source (components, types, assets, API schema code). Do not vendor Outline code into the repo; write own client code against the documented API. Anything copied would be BSL-licensed and would conflict with an MIT/Apache label.
- The service must not itself become a "Document Service" (e.g. offering hosted Outline-like teams and documents to third parties). A PDF-export helper does not do that.
- Operators running the service against their own Outline instance are bound by Outline's terms for *their instance*; this repo's license does not change that. Say so in the README.
- Do not use "Outline" branding in a way that implies endorsement; the license text grants no trademark rights (it contains no trademark grant). Name the project descriptively ("for Outline").
- I found no primary Outline document (docs, FAQ) that adds companion-tooling rules; an attempted docs URL returned 404. If certainty is needed, ask the licensor (the LICENSE points to https://www.getoutline.com).

## 2. Pandoc invoked as a subprocess

Pandoc license: GPL v2 **or later**. Source: https://raw.githubusercontent.com/jgm/pandoc/main/COPYRIGHT ("released under the GPL, version 2 or later") and COPYING.md (GPLv2 text). Pandoc's templates in `data/templates` are dual-licensed GPL-2+ or BSD-3-Clause (same COPYRIGHT file). `pandoc-types` modules are BSD-3-Clause.

GPLv2 §2 (https://raw.githubusercontent.com/jgm/pandoc/main/COPYING.md): the copyleft applies to "any work that you distribute or publish, that in whole or in part contains or is derived from the Program"; "If identifiable sections of that work are not derived from the Program, and can be reasonably considered independent and separate works in themselves, then this License, and its terms, do not apply to those sections when you distribute them as separate works." And: "mere aggregation of another work not based on the Program with the Program ... on a volume of a storage or distribution medium does not bring the other work under the scope of this License."

Consequences:

- Spawning `pandoc` via `os/exec`, with stdin/stdout/args, is use of a separate program. Go code that neither links to nor includes Pandoc is an independent work; no GPL obligation on the service's source or license. (Running GPL software, including as a network service, carries no obligations under GPLv2; the GPL regulates copying, modifying and distributing.)
- Copyleft would attach if the repo included Pandoc code, linked it as a library, or shipped a modified Pandoc.
- The FSF GPL FAQ entries on "mere aggregation" and on pipes/sockets/command-line communication are the canonical interpretation (https://www.gnu.org/licenses/gpl-faq.html, anchors `MereAggregation` and the "combining two parts" entry). **I could not retrieve the FAQ from this environment (gnu.org was unreachable), so this report relies on the GPLv2 text itself and not on the FAQ.** Re-check those entries before publishing.
- Lua filters and custom templates are the grey zone: a Lua filter runs inside the Pandoc process. Filters/templates you author are generally independent works, but if you copy Pandoc's default templates, the BSD-3 option applies (keep the notice), which is permissive.

### Redistributing Pandoc in a container image

- If the image contains a Pandoc binary (distro package or upstream release), you are *distributing* GPL object code. GPLv2 §3 requires one of: (a) accompany with complete corresponding source, (b) a written offer valid at least three years, or (c) pass on the offer you received (noncommercial only). Source for exact versions: Pandoc says it is at https://hackage.haskell.org/package/pandoc-X.Y.Z/pandoc-X.Y.Z.tar.gz and https://github.com/jgm/pandoc/tree/X.Y.Z (COPYRIGHT).
- Practical approach: pin the Pandoc version in the Dockerfile, include the GPL text and a short `THIRD_PARTY_NOTICES` stating the Pandoc version, license (GPL-2.0-or-later), and the source URL / written offer. Do not modify Pandoc.
- Pandoc's Haskell dependencies in the static binary carry their own (GPL-compatible, per COPYRIGHT) licenses with notice requirements; shipping the upstream release's license files covers this.
- Alternative that avoids redistribution entirely: publish only the Dockerfile/source and let users build (the image is then built by them). Publishing a built image to a public registry is distribution.

## 3. TeX Live packages

TeX Live statement (https://www.tug.org/texlive/copying.html, fetched via the TUG page): all software in TeX Live "meets the requirements of the Free Software Foundation's definition of free software, and the Debian Free Software Guidelines"; material "may be freely used, copied, modified, and/or redistributed, subject to (in many cases) the sources remaining freely available." The note adds this is not true of all software on CTAN, so installing arbitrary packages outside TeX Live needs per-package checks.

- Packages carry mixed licenses (LPPL, GPL, MIT, OFL, public domain, etc.); each is listed in the TeX Live package database (`tlmgr info <pkg>` shows `license`). Knuth's original TeX files carry a "rename if modified" convention (same TUG page).
- LPPL-licensed files (most LaTeX packages) may be redistributed unmodified; modified versions must be renamed. Do not patch packages in the image.
- GPL-licensed components in the image (e.g. some binaries/Ghostscript if installed) trigger the same source-offer duty as Pandoc. Prefer a distro/TeX Live scheme that ships the sources or document where to fetch them (Debian publishes source packages).
- Output (PDFs) is not covered by the tools' licenses; TeX Live's statement raises no output restriction. For fonts see below.

## 4. Fonts and example templates

Source: SIL OFL FAQ, https://openfontlicense.org/ofl-faq/ (fetched 2026-10-15).

- Q1.4: OFL fonts can be sold as part of software packages, original or modified.
- Q1.5: the font cannot be sold on its own, only bundled with other font(s) or software.
- Q1.10: the OFL text must accompany distributed fonts (except when embedded in documents or bundled within programs; a link to the OFL suffices for metadata, full text is recommended). Ship the font's copyright line and license file in the image next to the font files.
- Q1.3: "Only the portions based on the Font Software are required to be released under the OFL", so bundling with MIT/Apache code is fine.
- Q1.13: embedding an OFL font in a document does not change the license of the document, so generated PDFs are unaffected.
- Q5.1-5.4: Reserved Font Names: if you modify an OFL font that declares RFNs you must rename it. Ship fonts unmodified.
- Fonts under other licenses (e.g. Bitstream Vera, Ubuntu Font Licence, GUST Font License for Latin Modern, Apache for Roboto) have their own notice rules; document each in `THIRD_PARTY_NOTICES`. Do not ship proprietary fonts (e.g. those installed with a Mac/Windows system, or corporate brand fonts) in a public image.
- Example LaTeX templates you write: license them with the repo (Apache/MIT). If derived from Pandoc's default templates, the BSD-3 option applies (keep the copyright and conditions). If derived from third-party templates, keep their license; verify each before including.

## 5. MIT vs Apache-2.0 vs AGPL-3.0 for this case

Texts: Apache-2.0 §3-4 (https://raw.githubusercontent.com/spdx/license-list-data/main/text/Apache-2.0.txt), AGPL-3.0 §5(c), §13 (https://raw.githubusercontent.com/spdx/license-list-data/main/text/AGPL-3.0-only.txt).

| | MIT | Apache-2.0 | AGPL-3.0 |
|---|---|---|---|
| Permissive | yes | yes | no, strong copyleft incl. network use |
| Patent grant | none explicit | explicit, terminates on patent suit (§3) | yes (v3) |
| Attribution duty | keep notice | keep license, NOTICE, mark changes (§4) | keep notices; license whole work under AGPL (§5c) |
| Network clause | none | none | §13: modified versions must offer source to remote users |
| Pandoc subprocess | fine | fine | fine, adds nothing |
| Compat. with Outline (BSL, converting to Apache-2.0 in 2030) | fine | best alignment | fine for a separate program, but restrictive for adopters |
| Deployer friction (companies running it next to Outline) | lowest | low | high (many orgs ban AGPL) |

Why AGPL is not justified here: the main reason to pick it is to force network-service users to publish modifications. The service has no GPL/AGPL code to inherit from, and Pandoc is not linked, so AGPL is a choice, not a requirement. If the goal were protecting a hosted offering from closed forks, AGPL would serve that, at the cost of adoption.

Why Apache-2.0 over MIT: patent grant and clear NOTICE handling for a project that ships a bill of third-party materials; slightly more paperwork. MIT is fine if minimal text matters more.

## 6. Checklist for the repo

1. `LICENSE` = Apache-2.0 (or MIT) for own code, templates and docs you wrote.
2. `THIRD_PARTY_NOTICES` listing Pandoc (GPL-2.0-or-later, version, source URL/written offer), TeX Live packages shipped (name, license), fonts (name, license, copyright line).
3. Copy license files for fonts and GPL text into the image (e.g. `/usr/share/doc/...`).
4. Pin and do not modify Pandoc, TeX packages or fonts; otherwise RFN/LPPL renaming and GPL source duties grow.
5. No Outline source copied into the repo; no Outline logo/branding beyond descriptive name.
6. README note: operators remain bound by Outline's BSL for their own instance (Additional Use Grant forbids a "Document Service").
7. Re-verify the GPL FAQ entries (MereAggregation, pipes/sockets) which could not be fetched here.

## Outcome

- The project is licensed **MIT** (see `LICENSE`), the "acceptable, simpler alternative" named in the recommendation. Nothing in the analysis depends on the choice between MIT and Apache-2.0.
- `THIRD_PARTY_NOTICES` implements the checklist of section 6, with the source offer for Pandoc.
- The GPL FAQ entries that could not be fetched in the original run were rechecked on 2026-11-01: see the next section.

## GPL FAQ recheck (2026-11-01)

Source: https://www.gnu.org/licenses/gpl-faq.html, read in full on 2026-11-01. The entries are quoted from the page as served that day; the anchors are the element ids on the page.

1. **`#MereAggregation`**: "An 'aggregate' consists of a number of separate programs, distributed together on the same CD-ROM or other media. The GPL permits you to create and distribute an aggregate, even when the licenses of the other software are nonfree or GPL-incompatible. The only condition is that you cannot release the aggregate under a license that prohibits users from exercising rights that each program's individual license would grant them." The same entry on the line between two programs and one program with two parts: "pipes, sockets and command-line arguments are communication mechanisms normally used between two separate programs. So when they are used for communication, the modules normally are separate programs. But if the semantics of the communication are intimate enough, exchanging complex internal data structures, that too could be a basis to consider the two parts as combined into a larger program."
2. **`#GPLPlugins`**: "A main program that uses simple fork and exec to invoke plug-ins and does not establish intimate communication between them results in the plug-ins being a separate program."
3. **`#GPLInProprietarySystem`**: GPL-covered software can be distributed "alongside" another system if "the free and nonfree programs communicate at arms length, that they are not combined in a way that would make them effectively a single program", and the collection should be described as separate programs side by side.
4. **`#AggregateContainers`**: "the analysis of whether they are a single work or an aggregate is unchanged by the involvement of containers." A container image is treated like any other medium.
5. **`#UnreleasedMods`** and **`#GPLRequireSourcePostedPublic`**: running a GPL program, also as a service, without distributing it creates no source obligation; the duty arises when the program is released to others.

Conclusion for this project:

- **Calling Pandoc via `exec`** (the Worker starts `pandoc` and `lualatex` as child processes with arguments, files and a Lua filter that Pandoc itself loads) matches the "simple fork and exec" and "command-line arguments" cases of entries 1 and 2. The data exchanged are documents and PDFs, not shared internal data structures. The Go code is therefore a separate work and the MIT licence of the project is unaffected. This is the FSF's stated view, "a legal question, which ultimately judges will decide" (entry 1); it is not legal advice.
- **Shipping Pandoc, librsvg and TeX in the same image** as the Go binary is an aggregate (entries 1, 3 and 4): the image licence is not a single licence, each component keeps its own, and the notices say so. The obligation that does attach is the one for the GPL programs themselves: a written source offer and the licence texts (`THIRD_PARTY_NOTICES`).
- The Lua filters shipped with the Worker are loaded by Pandoc's embedded Lua interpreter. They are our own files, MIT licensed, and run inside the Pandoc process. This is the grey zone named in section 2; it is not a case of linking Pandoc code into our binary. If this is a concern for a downstream user, the filters can be dual-licensed or replaced; nothing in the Go code changes.
- Not verified: how a court would decide. Not verified: the licences of the Haskell libraries inside the Debian `pandoc` binary beyond the Debian `copyright` file (see `THIRD_PARTY_NOTICES`).

## Source list

- https://raw.githubusercontent.com/outline/outline/main/LICENSE
- https://github.com/outline/outline#license
- https://raw.githubusercontent.com/jgm/pandoc/main/COPYRIGHT
- https://raw.githubusercontent.com/jgm/pandoc/main/COPYING.md
- https://www.tug.org/texlive/copying.html
- https://openfontlicense.org/ofl-faq/
- https://raw.githubusercontent.com/spdx/license-list-data/main/text/AGPL-3.0-only.txt
- https://raw.githubusercontent.com/spdx/license-list-data/main/text/Apache-2.0.txt
- https://www.gnu.org/licenses/gpl-faq.html (not retrievable in this run)
