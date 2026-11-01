# Markdown to LaTeX fidelity: Outline-flavoured Markdown via Pandoc (LuaLaTeX, scrlttr2)

- Date: 2026-10-15
- Scope: failure modes when converting Markdown that Outline produces into a KOMA-Script `scrlttr2` letter PDF with Pandoc + LuaLaTeX, and the robust fix for each.
- Environment used for experiments: pandoc 3.12 (Homebrew build, "compiled without HTTP support"), LuaHBTeX 1.24.0 / TeX Live 2026 (basic), macOS. Outline source: `github.com/outline/outline` at commit `cb02072` (v1.11.0, checked out locally).
- Legend: **[V]** verified by a local experiment (result recorded), **[S]** verified by reading primary source code/docs, **[I]** inferred / proposal, not tested.
- Experiments live in the session scratchpad (not committed); every command result quoted below was observed.

## 0. Summary of the most important findings

1. **A template that does not include Pandoc's table support fails: `! LaTeX Error: Environment longtable undefined.`** [V]. The packages (`longtable, booktabs, array, caption, calc, etc.`) come from `common.latex`, guarded by the Pandoc variable `tables`. A custom template must either include `$common.latex()$` (preferred) or load those packages itself.
2. **`-raw_tex` does not block raw LaTeX.** [V] Three leaks:
   - ```` ```{=latex} ```` fenced blocks and `` `...`{=latex} `` spans still pass through, because they belong to the separate `raw_attribute` extension. Use `-raw_tex-raw_attribute`.
   - **Math is raw TeX.** With `tex_math_dollars` on (default, and used by Outline for its math nodes), `$\input{/abs/path}$` in the document body read an arbitrary local file into the PDF, and `$\directlua{...}$` executed Lua. Neither `-raw_tex` nor `-raw_attribute` stops this. `openin_any=p` in the environment did not stop the absolute-path read.
   - Fix: neutralise `Math` nodes in a Lua filter (or drop `tex_math_dollars`), and run the engine in a locked-down container.
3. **Outline's `documents.info` Markdown differs from its export Markdown.** [S] `documents.info` serialises with `commonMark` unset: hard breaks are the two literal characters `\n` (backslash, `n`), which Pandoc renders as `\textbackslash n` [V]. `documents.export` sets `commonMark: true` and writes a CommonMark hard break (two spaces + newline) [S]. Hard breaks inside table cells are always `<br>`.
4. **`<br>` in table cells becomes the literal text `<br>` under `-raw_html`** [V]. Keep `raw_html` enabled (the LaTeX writer drops unknown HTML anyway [V]) and convert `<br>` to `LineBreak` in a Lua filter [V].
5. **Images**: Outline writes the size inside the title string. A size-only image is `![alt](src " =300x200")`; the title begins with a space and **Pandoc then fails to parse it as an image at all** (literal text in the PDF) [V]. Needs pre-processing. Attachment URLs (`/api/attachments.redirect?id=...`) are not files: Pandoc warns and replaces the image with its description, **exit code 0** [V]. Fetch them first and rewrite to local paths; use `--fail-if-warnings`.
6. **`scrlttr2` has no `\section`**: any Markdown heading yields `! Undefined control sequence.` [V]. Convert headings in a filter or define `\section` etc. in the template.
7. **Multi-line address blocks**: a YAML block scalar (`|`) becomes a `MetaBlocks` paragraph whose newlines are `SoftBreak`, i.e. a single line in LaTeX [V]. Use a YAML list and join with `\\` in the template [V], or convert `SoftBreak` to `LineBreak` in the metadata.
8. Tables: Outline column widths come from the **header text length**, so a long-content column with a short header is squeezed [V]. A small Lua filter that recomputes widths from content works [V].

## 1. Where Outline's Markdown comes from (primary source, [S])

| Aspect | Behaviour | Source |
|---|---|---|
| `documents.info` `text` field | `DocumentHelper.toMarkdown(data, { includeTitle: false })`: **no `commonMark`, no signed URLs** | `server/presenters/document.ts` |
| `documents.export` | `toMarkdown({ signedUrls, teamId, commonMark: true })`; `signedUrls` is a number of seconds (optional); title is included unless `includeTitle:false` (it is not passed here, so expect a leading `# Title`) | `server/routes/api/documents/documents.ts` (`documents.export`), `.../schema.ts` (`DocumentsExportSchema`) |
| Hard break | `inTable ? "<br>" : commonMark ? "  \n" : "\\n"` (the last is backslash + letter n) | `shared/editor/nodes/HardBreak.ts` |
| Empty paragraph | `commonMark ? "\n" : "\\\n"`; post-processed by `.replace(/(^|\n)\\(\n\|$)/g, "\n\n")` | `Paragraph.ts`, `server/models/helpers/DocumentHelper.tsx` |
| Table | Always pipe tables, one line per row; header separator dash count = `max(header text length, 3) + 2`; alignment via `:`; cell newlines become `<br>`; `\|` escaped; lists in cells become `* a <br> * b` inline | `shared/editor/lib/markdown/serializer.ts` (`renderTable`, `renderList`) |
| Image | `![alt](src "title =WxH")`; size lives inside the quoted title; `title` may be empty so the quote starts with a space | `shared/editor/nodes/Image.tsx` (`toMarkdown`) |
| Attachment / file | `[title size](href)` (a plain link) | `Attachment.tsx` |
| Video | `[title WxH](src)` | `Video.tsx` |
| Notice | `:::info` ... `:::` (also `warning`, `tip`, ...) | `Notice.tsx` |
| Toggle block | `+++` ... `+++` (more `+` when nested) | `ToggleBlock.ts` |
| Math | inline `$...$`, block `$$` ... `$$` | `Math.ts`, `MathBlock.ts` |
| Escaping | `esc()` backslash-escapes `` ` * \ ~ `` and a `[` that precedes `](`; at line start also `: # - * +`, `1.`, `a.`; in tables also `\|`. It does **not** escape `$`, `<`, `&`, `_`, `{`, `%`, `#` mid-line | `serializer.ts` (`esc`) |
| Checklist | `[x] ` / `[ ] ` items | `CheckboxItem.ts` |
| Ordered list | alpha styles are rendered with `a.`/`A.` markers and 4-space indent; start number preserved | `OrderedList.ts` |
| Code fence | fence is longer than any backtick run in the content; language sanitised | `CodeFence.ts` |
| Attachment URL | `/api/attachments.redirect?id=<uuid>` answers **302** to a (signed) storage URL; private attachments need an authenticated user of the same team (`auth({ optional: true })`, `AuthorizationError` otherwise) | `server/routes/api/attachments/attachments.ts` (`handleAttachmentsRedirect`) |
| Signing | `ProsemirrorHelper.signAttachmentUrls` rewrites redirect URLs to storage-signed URLs (short expiry, default 60 s) when `signedUrls` is given | `server/models/helpers/ProsemirrorHelper.tsx` |

Recommendation [I, based on the above]: prefer `documents.export` (CommonMark mode, `signedUrls` optional) over `documents.info` for the Markdown body. It gives portable hard breaks. If `documents.info` must stay, normalise literal `\n` hard breaks in a pre-processing step (lossy: a legitimate `\n` text is indistinguishable), so export is the cleaner choice. Either way keep a pre-processing stage in the service for image sizes and attachments (section 6).

## 2. Pandoc invocation contract

Recommended reader: `-f markdown-raw_tex-raw_attribute` (keep `raw_html` on, see 3.2). The existing service uses `markdown-raw_tex-raw_html`; it should add `-raw_attribute` and reconsider `-raw_html` (section 3.2).

Behaviours confirmed from the Pandoc manual (`man pandoc`, 3.12; online: https://pandoc.org/MANUAL.html):

- `--sandbox`: limits IO in readers/writers, **but "does not limit IO operations by filters or in the production of PDF documents"**. So it does not stop the TeX-level leaks in section 3.1 [S].
- Pipe tables: "If any line of the Markdown source is longer than the column width (see `--columns`), then the table will take up the full text width and the cell contents will wrap, with the relative cell widths determined by the number of dashes in the line separating the table header from the table body. ... if no lines are wider than column width, then cell contents will not be wrapped, and the cells will be sized to their contents." [S] and [V]: a short table gave `{@{}ll@{}}`, a long one `p{... \real{0.xxxx}}` columns.
- `--fail-if-warnings` exits 3 on any warning [V] (used for missing images).

## 3. Failure modes and fixes

### 3.1 Raw TeX safety (`-raw_tex`)

Experiments (all with the default Pandoc LaTeX writer, then `lualatex`):

| Input | Reader flags | Result |
|---|---|---|
| `a \input{/etc/passwd} b`, `\begin{verbatim}` | `markdown` | passed through verbatim to LaTeX [V] |
| same | `markdown-raw_tex` | escaped as text (`\textbackslash input\{...\}`) [V] |
| ```` ```{=latex} \input{evil} ``` ```` | `markdown-raw_tex` | **still emitted raw** [V] |
| same | `markdown-raw_tex-raw_attribute` | rendered as an inline code span, harmless [V] |
| `x $\input{leak}$ y` (file `leak.tex` containing a secret-looking string) | `markdown-raw_tex-raw_html` | **PDF contained the file's text** ("x secret - token - 123 y") [V] |
| same with an absolute path | with env `openin_any=p` | still leaked [V] |
| `$\directlua{tex.print("LUAEXEC")}$` | `markdown-raw_tex-raw_html` | Lua executed (text "LU AEXEC" in PDF) [V] |
| `$\directlua{os.execute("echo EXEC > dl.txt")}$`, `$\immediate\write18{...}$` | same | no file created: TeX Live default `shell_escape=p` (restricted) blocked the shell [V] (`kpsewhich -var-value shell_escape` returned `p`) |
| same `\input` input | `-tex_math_dollars` | literal text, harmless [V] |

Fix, in order of strength:

1. Reader: `-raw_tex-raw_attribute`. [V]
2. Lua filter replacing `Math` nodes. Two options:
   - Cheapest: return the math source as plain `Str` (loses typesetting; tested: `function Math(el) return pandoc.Str(el.text) end`).
   - Better [I]: allow-list. Accept the math only if every `\command` in it is in a small allow-list (`frac sqrt sum alpha ...`) and braces balance, otherwise degrade to text. Do not use a deny-list: TeX has too many primitives (`\input`, `\include`, `\openin`, `\read`, `\directlua`, `\luaexec`, `\catcode`, `\csname`, `\expandafter`, ...).
   - Or drop `tex_math_dollars` entirely if Outline math is out of scope. Note that without it, the Outline syntax `$x$` shows as literal text and `$5 and $10` is safe either way [V: `Cost \$5 and \$10 total.`].
3. Defence in depth around the process, not inside Pandoc: run `pandoc` in a container with a read-only root, an empty working directory per job, no secrets mounted, no network. Per-job temp directory. Keep `shell_escape` restricted (the default). `lualatex --safer` was tried and made the font load fail in this setup (`Font ... not loadable`), so it needs separate evaluation before relying on it [V for the failure, I for the rest].
4. Metadata values also go through the reader, so apply the same rules to the YAML front matter that the service generates (title, address). They are rendered through the LaTeX writer (escaped) [V: `Smith \& Sons \\ A\_B Street \#5 \\ 10\% Ltd`].

Test: golden/negative tests with the payloads above; assert the PDF text does not contain the leaked marker and that `pdftotext` shows the payload as text.

### 3.2 HTML in Outline Markdown (`<br>` and friends)

- With `-raw_html`, `line1<br>line2` in a cell renders as `line1\textless br\textgreater line2` [V].
- With `raw_html` on and a filter `RawInline html '<br>' -> LineBreak`, the cell compiles; Pandoc emits `\vtop{\hbox{\strut line1}\hbox{\strut line2}}` in short cells [V].
- With `raw_html` on, `<b>`, `<script>x</script>` are dropped by the LaTeX writer and only the text remains (script body `x` also dropped) [V], so raw HTML is not a TeX risk.
- Fix: keep `raw_html` on, add the `<br>` filter. Test: a table with `<br>`, assert no `<br>` text in `pdftotext`.

### 3.3 Soft vs hard line breaks

- Outline paragraphs hold no soft newlines; Shift+Enter creates a hard break [S: `HardBreak.ts`]. Outline's parser also treats a text-only `\` as an empty paragraph [S: `shared/editor/rules/breaks.ts`].
- Pandoc: single newline = `SoftBreak` = space in LaTeX (`Line one Line two`) [V]; `\`+newline and two trailing spaces = `\\` [V]; `+hard_line_breaks` turns every newline into `\\` and empty lines produce `\strut \\` [V].
- `documents.info` hard break = literal `\n` text -> `\textbackslash nBeta` [V]. `documents.export` is correct (CommonMark).
- Fix: do not enable `hard_line_breaks` (Outline paragraphs would still be right, but externally authored Markdown with wrapped lines would break). Use `documents.export` or normalise `\n`. Test: fixture with a hard break inside a paragraph and inside a list item; assert two lines in `pdftotext -layout`.
- A paragraph containing only `\` parses to `Para [LineBreak]` [V]. Outline strips these on export [S], but guard: drop paragraphs that contain only `LineBreak` in the filter [I].

### 3.4 Tables (pipe tables, longtable, booktabs)

Observed Pandoc output [V]: `longtable` with `\toprule`, `\midrule`, `\endhead`, `\bottomrule\noalign{}\endlastfoot`, `\bottomrule`, column spec `>{\raggedright\arraybackslash}p{(\linewidth - 2\tabcolsep) * \real{0.5}}` for wrapped tables, `ll`-style for short ones.

Failure modes and fixes:

| Failure | Evidence | Fix |
|---|---|---|
| `! LaTeX Error: Environment longtable undefined.` | Bare template without `common.latex`, one-table input [V] | Template contract: include `$common.latex()$`, or load `longtable, booktabs, array, caption, calc` (and `etoolbox` for the `\patchcmd` fix; `multirow` if `multirow` var) guarded by `$if(tables)$`. With `common.latex()` the same input compiled to 1 page [V] |
| Missing `calc`/`array` -> `\real`/`p{}` errors | `\real` and `\tcolsep` arithmetic are used in the column spec [S: `common.latex`] | Same as above |
| Long cell not wrapping when source rows are short | Pipe tables with no source line wider than `--columns` are `ll` (no wrapping) [S, V] | Pass `--columns=1` so all tables get fixed `p{}` columns; verified: a tiny table gets `\real{0.5000}` columns [V] |
| Column widths follow header text length (Outline pads dashes to the header) | Outline-style `\|------\|------\|-----\|` with a long note under a 4-letter header gave `0.3529 / 0.3529 / 0.2941` [V] | Lua `Table` filter recomputing relative widths from content length (sqrt-damped, floor 0.08): result `0.1462 / 0.7272 / 0.1266` [V]. Code was a ~25-line filter; place it in the service's filter set |
| Tables crossing pages | 89-row table: 8 pages, header repeated on page 2 (`Name Description Qty` shown on page 2) [V] | Nothing to do; `longtable` handles it. Do not wrap tables in `table`/`tabular` |
| Unbreakable content (URLs, IDs) overflows the column | URL table: 2 `Overfull \hbox` (42 pt) in the log [V]; wide 8-column table: 8 overfull (13 pt) [V] | Load `xurl` or `url` + `\UrlBreaks` in the template so `\url{}` can break; for the 8-column case enable `\sloppy` or `\emergencystretch` (already in `common.latex`: `\setlength{\emergencystretch}{3em}`) and consider `\small` tables. Leave residual overfull to a log check, see tests |
| Block content in cells (lists, paragraphs) | Pipe tables cannot hold blocks [S]; Outline flattens them to `<br>` and ` * a <br> * b` [S] | `<br>` filter (3.2). Bullets in cells stay `* a` as text after the flatten; add a filter that splits `Str "*"`/`"-"` after `LineBreak` into a bullet glyph if this matters [I] |
| Table caption numbering ("Table 1") | `\def\LTcaptype{none}` in output [V] | Fine: captions are unnumbered because Pandoc sets `none`. Needs the `\newcounter{none}` from `common.latex` |
| Table directly after `\paragraph` | Pandoc patches `\longtable` (`\patchcmd`) [S] | Comes with `common.latex` |
| Grid tables | `grid_tables` supported (multi-paragraph, lists in cells) [S: manual]; Outline never emits them | Not needed from Outline; supporting them only costs nothing since the `Table` AST is identical |

Test: golden-file test of the generated `.tex` for (a) short table, (b) long-cell table, (c) 100-row table, (d) table with `<br>`; compile test for each; `pdfinfo` page count; `grep -c Overfull <log>` threshold.

### 3.5 Multi-line address blocks in YAML metadata

Experiments (YAML metadata, then template `$for(r)$...`) [V]:

| YAML form | Pandoc meta | Output |
|---|---|---|
| list of lines | `MetaList` of `MetaInlines` | each item separate, joinable with `\\` (verified: `Smith \& Sons \\ A\_B Street \#5 \\ 10\% Ltd`) |
| `r: \|` block scalar | `MetaBlocks [Para [... SoftBreak ...]]` | one line, plus a trailing blank paragraph |
| `"a\nb\nc"` quoted string | `MetaInlines` with `SoftBreak` | one line |
| block scalar with `\\` | `\\` becomes `\` | `Jane Doe \ Example ...`, wrong |
| special chars (`& _ # %`) in list items | escaped by writer | correct |

Fix:
1. Contract: the address is a YAML **list**; the template joins it with ` \\ ` inside `\begin{letter}{...}` (scrlttr2 requires `\\` line separators there). This needs no filter and is robust against special characters.
2. For authors who write block scalars (Outline fenced ```yaml block at the document start is the existing convention in the service), add a metadata filter: for given keys, convert `SoftBreak` to `LineBreak` / split `MetaBlocks` into a list [I]. Cheaper: make the Go side (`internal/document`) normalise string values containing `\n` into lists before writing the front matter [I].
3. Empty `\begin{letter}{}` yields `! LaTeX Error: There's no line here to end.` at `\opening` in this setup [V]; the template should guard `$if(recipient)$` or the service should reject missing recipients.

Test: unit test on the document-preparation code (list, block scalar, quoted string with `\n`, special characters), plus a compile test for a 4-line address and a 1-line address.

### 3.6 Headings in a letter class

`# Heading` -> `\section{Heading}\label{heading}`; `scrlttr2` does not define `\section`, so compilation fails with `! Undefined control sequence.` [V].

Fix (either):
- Lua filter `Header -> Para(Strong(content))` (tested; letter compiled, both headings present in the PDF) [V].
- Template: `\providecommand{\section}[1]{\par\medskip\noindent\textbf{#1}\par}` and likewise `\subsection` (tested, compiles) [V]. A plain `\renewcommand{\label}` breaks (already defined) [V], so do not touch `\label`.
Prefer the template variant if headings should be styled by the letter design, the filter if the service must be template-agnostic. Pandoc also needs a mapping for deeper levels (`\subsubsection`, `\paragraph`) [I].

### 3.7 Lists

Pandoc output [V]: `itemize`/`enumerate` with `\tightlist`; nested lists work; start numbers map to `\setcounter{enumi}{2}`; lowercase alpha list gives `\def\labelenumi{\alph{enumi}.}`; task lists produce `\item[$\boxtimes$]` / `$\square$` and need `amssymb` (the sample template loaded it) [V].
Failure modes:
- `\tightlist` undefined if the template omits `common.latex` (it is a `\providecommand` there [S]). The bare template defined it manually.
- Uppercase letter and roman lists (`A. one`, `I. roman`) were **not** recognised as lists in my one-line test (rendered `A. one B. two`) [V]. Pandoc's `fancy_lists` requires two spaces after a capital-letter marker. Outline uses `A.`/`a.` with 4-space indent [S], so check real Outline output with a fixture before relying on it.
- A list directly after a paragraph with no blank line is not a list (`lists_without_preceding_blankline` is off) [V]. Outline writes blank lines [S].
- Checkbox list needs `amssymb` in the template contract.

Test: fixture with nested bullet, ordered with start=3, task list, alpha list; assert `\begin{enumerate}` counts in generated `.tex`.

### 3.8 Code blocks

- Default highlighting emits `\begin{Shaded}\begin{Highlighting}` and needs the macros from `highlighting-macros`/`fancyvrb` [V in generated tex: `\DefineVerbatimEnvironment{Highlighting}{Verbatim}...` with `\newenvironment{Shaded}{}{}`]; `common.latex` provides them.
- Long unbreakable code lines overflow: a 200-char line gave `Overfull \hbox (731.49pt too wide)` with `--no-highlight` (note: that flag is deprecated, use `--syntax-highlighting=none`) [V].
- Fix: either wrap in the template (`\fvset{breaklines=true}` for fancyvrb `Verbatim`, or `listings` with `breaklines`), and/or hard-wrap in a filter; and set a monospace font that exists in the container [I].
- Inline code `\texttt{...}`. Unicode in code needs a font with those glyphs.

Test: fixture with a 200-char line and a tab character; check the Overfull budget.

### 3.9 Images and Outline attachment URLs

Verified behaviours:
- Image with alt text becomes a `figure` with caption = alt [V]. Outline's alt text is mostly a file name or empty, so captions will be noise: use `-implicit_figures` [S: `implicit_figures` is a documented extension; I did not run the flag].
- `![](/api/attachments.redirect?id=3f2b "x")` : Pandoc tries to read it as a local file: `[WARNING] Could not fetch resource ...: does not exist. Replacing image with description.` and **exits 0**, PDF produced without image [V]. With `--fail-if-warnings` the run fails with exit 3 [V]. A remote `https://` URL: this Pandoc build was "compiled without HTTP support", same warning [V]; another build may fetch it.
- `![](u.png " =x200")` and `![a](u.png " =x200")` (size only, as Outline writes when there is no title) are **not parsed as images** (literal text) [V]. `"t =300x200"` parses, with the size left inside the title (and no size applied) [V].
- Fix for sizes [V]: pre-process `![alt](src "title =WxH")` to `![alt](src "title"){width=Wpx height=Hpx}` (regex tested; Pandoc then emits `width=3.125in,height=2.08333in`, and height-only gives `width=\linewidth,height=...,keepaspectratio`).
- Fix for URLs [I, design]: in the service, before Pandoc, find `/api/attachments.redirect?id=<uuid>` (regex as in `shared/utils/ProsemirrorHelper.ts` `attachmentRedirectRegex`, which also tolerates extra query parameters), download with the **caller's authentication** (the redirect needs a user of the team for private files [S]), follow the 302 to storage, write into the per-job directory, rewrite the Markdown to the local filename, and pass `--resource-path=<jobdir>`. Alternatively call `documents.export` with `signedUrls` and still download, since Pandoc cannot fetch HTTPS in every build. Reject non-image content types; limit size and count; convert SVG/WebP to PNG/PDF because `includegraphics` in LuaLaTeX cannot embed SVG or WebP (SVG: LuaLaTeX needs conversion, as the existing service already does for its logo; WebP not tested).
- `\pandocbounded` (from `common.latex`) scales images to the text width/height [S].
- Non-image attachments (`[report.pdf 1.2 MB](/api/attachments.redirect?...)`) become `\href{/api/...}{report.pdf 1.2 MB}` [V]: a broken relative link in the PDF. Fix: rewrite to plain text (drop the link) or to an absolute URL [I]. Same for document links `/doc/...` and mentions.

Test: fixture with (a) sized image, (b) unsized, (c) missing attachment (must fail the job, not silently drop), (d) non-image attachment link. Assert on the generated `.tex` and Pandoc exit code.

### 3.10 Special characters and escaping

Pandoc's LaTeX writer escapes correctly [V]: `50% & $5 #1 _x_ a_b {c} ~tilde ^caret` -> `50\% \& \$5 \#1 \emph{x} a\_b \{c\} \textasciitilde tilde \^{}caret`. Typographic quotes (`"q"` -> ``` ``q'' ```) and `...` -> `\ldots{}` come from `smart` [V]. Outline's escapes (`\#`, `1\.`, `\-`, `\*`, `` \` ``, `\~`, `\\`) are plain Markdown escapes and decode properly [V].
Failure modes:
- `_` inside words, `<`, `>`, `|` are fine in text [V].
- `$` pairs can become math: "Cost $5 and $10" is safe [V] (Pandoc's rule: no space after the opening `$`, and the closing `$` must not be followed by a digit), but `$a` ... `b$` pairs on the same paragraph become math, and then run through TeX (3.1). Hence the math policy.
- Emoji and symbols missing from the font: `[WARNING] Missing character: There is no U+1F600 / U+2713 in font [lmroman10-regular]` and the character silently drops from the PDF (exit 0) [V]. Fix: select fonts with `fontspec` plus a fallback list (LuaLaTeX `\directlua` font fallback or `luaotfload` `fallback`), strip/replace Outline emoji shortcodes (`:smile:` stays literal text, Pandoc does not decode them without `+emoji` [V]); treat "Missing character" in the TeX log as a test failure.
- Outline-only syntax passes through as literal text [V]: `==highlight==`, `++underline++`, `+++` toggles; `:::info` notice parsed as a `Div` and rendered as plain paragraphs (marker lost) [V]. Fix in a filter: map `Div.info/warning/tip` to a framed or indented block, toggle markers to nothing, highlight/underline to `\hl`/`\underline` [I].
- Typographic dash handling: `--` stays `--` in the LaTeX output (renders as an en dash) [V].
- Strikethrough: `~~del~~` -> `\st{del}` needs `soul` or `lua-ul` from `common.latex` (`strikeout` variable) [S].
- Horizontal rule `---` -> `\begin{center}\rule{0.5\linewidth}{0.5pt}\end{center}` [V].
- Hyphenation/Language: set `lang` (e.g. `de-DE`) so `babel`/`polyglossia` is loaded by the template [S: `babel-lang` variable] and `selnolig`/ligatures behave.

### 3.11 Template contract summary (what a letter template must provide)

For the Pandoc LaTeX writer output to compile in a `scrlttr2` template:

- `$common.latex()$` or the same blocks (tables, graphics, highlighting, strikeout, `\tightlist`, `\pandocbounded`, `\emergencystretch`) [S: `data/templates/common.latex`; the 3.12 default template calls `$common.latex()$`].
- `amsmath`/`amssymb` (task-list glyphs, math), `fontspec` for LuaLaTeX.
- `\section`, `\subsection`, ... or a heading filter (3.6).
- `\url` breaking (`xurl`).
- A `recipient` list joined with `\\` (3.5).
- `$body$` placed between `\opening`/`\closing`; guard empty recipient.
- A way to fail loudly: LaTeX run with `-halt-on-error`/`-interaction=nonstopmode` and a log check for `Overfull`, `Missing character`, `undefined` (the lualatex run through Pandoc already stops on errors: `Error producing PDF.` [V]).

## 4. What belongs where

| Concern | Pandoc flags | Lua filter | Pre-processing (service) | Template | Test level |
|---|---|---|---|---|---|
| Raw TeX / raw attribute | `-f markdown-raw_tex-raw_attribute`; `--sandbox` for reader/writer IO | `Math` neutralise/allow-list | - | - | negative unit tests with payloads; PDF text assertion |
| Process isolation | - | - | container, empty cwd, no network | - | CI job with canary file outside job dir |
| `<br>` in cells | keep `raw_html` | `RawInline html br -> LineBreak` | - | - | golden `.tex` + pdftotext |
| Hard breaks | `documents.export` (CommonMark) | drop line-break-only paragraphs | normalise literal `\n` if `documents.info` | - | fixture with break in para/list |
| Table packages | - | - | - | `$common.latex()$` / `tables` guard | compile test with a table (reproduces `Environment longtable undefined` if broken) |
| Table wrapping | `--columns=1` | content-based width filter | - | - | golden `.tex`; page-count test for 100 rows |
| Long unbreakable text | - | - | - | `xurl`, `\emergencystretch`, code `breaklines` | log scan `Overfull` budget |
| Address block | - | (optional) SoftBreak -> LineBreak in meta | normalise multi-line strings to a list | `\\` join, empty guard | unit test + compile test |
| Headings | - | `Header -> bold Para` (or) | - | `\providecommand{\section}...` | compile test with `#`, `##` |
| Lists / task lists | - | - | - | `amssymb`, `\tightlist` | golden `.tex` |
| Code blocks | `--syntax-highlighting=none` or default | - | - | fancyvrb `breaklines`, mono font | fixture long line |
| Images: size | `link_attributes` (default on) | - | rewrite `" =WxH"` to `{width= height=}` | `graphicx` | golden `.tex` |
| Images: captions | `-implicit_figures` | - | - | - | golden `.tex` |
| Attachments (auth, download, local path, types, limits) | `--resource-path`, `--fail-if-warnings` | - | **yes**: authenticated download and rewrite | - | integration test against a stub server returning 302 |
| Non-image attachment / internal links | - | `Link` rewrite to text | - | - | golden `.tex` |
| Missing glyphs | - | emoji shortcode strip | - | font fallback | log scan `Missing character` |
| Outline extensions (notice, toggle, highlight, underline) | `fenced_divs` is default | `Div` to box, strip toggles | - | box macro | golden `.tex` |
| Any warning | `--fail-if-warnings` | - | - | - | exit-code test |

## 5. Suggested test matrix

1. **Fixtures** (Markdown + expected flags): one per row of section 3, generated from real `documents.export` output where possible, not hand-written Outline syntax (see 3.7 alpha lists).
2. **Pandoc-level golden tests**: `pandoc -t latex` output compared against checked-in `.tex` fragments (fast, no LaTeX needed).
3. **Compile tests**: same inputs through the production template with `lualatex`; assert exit code, `pdfinfo` page count, `pdftotext` content, and scan the `.log` for `Overfull \hbox` above a threshold, `Missing character`, `undefined`.
4. **Negative/security tests**: payloads from 3.1; the `$\input{}$` canary file must not appear in `pdftotext`.
5. **Contract test for the template**: a minimal document using every AST feature (table, image, list, code, heading, task list) must compile; this is the test that catches `Environment longtable undefined` [V as the failure].
6. **Service tests**: attachment rewriting against a stub that returns 302 with and without auth.

## 6. Open questions / not verified

- Alpha/roman list round trip from real Outline output (only a synthetic fixture tried).
- WebP and SVG handling by `includegraphics` in LuaLaTeX (not tested; SVG known to need conversion).
- `lualatex --safer` as hardening: failed font loading in this environment; needs its own evaluation.
- Whether other Pandoc builds fetch `https://` images (this one did not).
- Behaviour of `documents.info` vs `documents.export` on a live Outline instance (analysis is from source at v1.11.0; the hard-break difference should be confirmed with one real document).
- Exact rendering of Pandoc table `\vtop{\hbox...}` for `<br>` in long cells in the wrapped (`p{}`) form: only the short form was observed.

## 7. Sources

- Pandoc manual: https://pandoc.org/MANUAL.html (sections: Pipe tables, Extension raw_attribute, Extension tex_math_dollars, Extension implicit_figures, Extension link_attributes, `--sandbox`, `--fail-if-warnings`, `--columns`); read locally via `man pandoc` for 3.12.
- Pandoc LaTeX templates: https://github.com/jgm/pandoc/blob/main/data/templates/default.latex and https://github.com/jgm/pandoc/blob/main/data/templates/common.latex (read locally: `pandoc -D latex`, `pandoc --print-default-data-file templates/common.latex`).
- Outline source (v1.11.0, commit cb02072): https://github.com/outline/outline, files `shared/editor/lib/markdown/serializer.ts`, `shared/editor/nodes/{HardBreak,Paragraph,Image,Attachment,Notice,CodeFence,Math,MathBlock,ListItem,CheckboxItem,OrderedList,ToggleBlock}.ts*`, `shared/editor/rules/breaks.ts`, `server/models/helpers/DocumentHelper.tsx`, `server/models/helpers/ProsemirrorHelper.tsx`, `server/presenters/document.ts`, `server/routes/api/documents/{documents,schema}.ts`, `server/routes/api/attachments/attachments.ts`, `shared/utils/ProsemirrorHelper.ts`.
- KOMA-Script (`scrlttr2`): https://ctan.org/pkg/koma-script (class installed locally; `\begin{letter}{recipient}` requires `\\` line separators; `\section` is not defined by the letter class, confirmed by the compile error above). The KOMA-Script guide was not re-read for this note; claims about it are limited to what the experiments showed.
- TeX Live restricted shell escape: `kpsewhich -var-value shell_escape` returned `p` locally.

## Outcome

The failure modes of section 3 are covered by fixtures in `testdata/fixtures/` and goldens in `testdata/golden/` (see [../testing.md](../testing.md)), and by the Worker filters described in [../worker.md](../worker.md): reader flags, math neutralisation, `<br>` handling, table widths, headings, image size pre-processing and attachment download ([../images.md](../images.md)). The open questions of section 6 were resolved by implementation; where a behaviour differs from the proposals above, the docs linked here are authoritative. One known gap: a captioned figure in the `neutral` letter Template is tracked in issue #43.
