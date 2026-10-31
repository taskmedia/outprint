# Packages

The renderer reads two kinds of packages, each from its own root directory:

- a **Template** package, one directory per Template under the templates root;
- a **Template Configuration** package, one directory per Configuration under
  the configurations root.

Directory names match `^[a-z0-9][a-z0-9-]*$`. Packages are self-contained:
nothing is shared between packages, and every path inside a package is
confined to it (see [Path confinement](#path-confinement)).

Fill the roots from an image, a volume or a git-sync sidecar; the renderer
does not care which.

## Service configuration

The service configuration names the roots and the allow-list of Bindings, the
only Template and Configuration pairings a Collection may select.

```yaml
templatesRoot: /opt/renderer/templates
configurationsRoot: /opt/renderer/configurations
bindings:
  - template: neutral
    configuration: example
defaultBinding:          # optional, must be one of the bindings
  template: neutral
  configuration: example
```

Unknown keys are errors. `RENDERER_TEMPLATES_ROOT` and
`RENDERER_CONFIGURATIONS_ROOT` override the roots from the file. The file is
located by `RENDERER_SERVICE_CONFIG` (or a path argument where a command takes
one). The shipped image sets it to `/opt/renderer/service.yaml`.

Packages on disk but not on the allow-list are loaded and checked, and
produce a warning; their problems are warnings too, since nothing can use
them. An allow-list entry that points at a missing package is an error.

## Template package

```
templates/neutral/
  manifest.yaml        fields and Document Types
  letter.latex         one Pandoc template per Document Type
  shared/neutral.tex   shared TeX parts (inside the package only)
  fonts/               optional; copied to ./fonts/ of every job
  assets/              optional
```

Fonts are loaded by file path, never by name: the Worker's TeX engine cannot
use the system font database (see [worker.md](worker.md)). A Template loads
its fonts with the fontspec option `Path=./fonts/`:

```tex
\setmainfont{texgyrepagella-regular.otf}[Path=./fonts/,
  BoldFont=texgyrepagella-bold.otf, ItalicFont=texgyrepagella-italic.otf,
  BoldItalicFont=texgyrepagella-bolditalic.otf]
```

### manifest.yaml

```yaml
manifest_version: 1

fields:
  accent_color:
    type: color
    description: Colour of the margin stripe.
    pattern: '^[0-9A-Fa-f]{6}$'
  subject:
    type: string
    description: Subject line.
  recipient:
    type: list
    description: Address lines.

document_types:
  letter:
    template: letter.latex
    fields:
      - accent_color
      - subject:
          required: true          # override for this Document Type only
      - recipient
```

`manifest_version` must be `1`; any other value is rejected, before anything
else in the file is read. Unknown keys are errors.

Each entry of `fields` declares a field once:

| Key           | Meaning                                                              |
| ------------- | -------------------------------------------------------------------- |
| `type`        | one of the field types below (required)                              |
| `required`    | the Document must supply it (default `false`)                        |
| `description` | free text                                                            |
| `default`     | value used when nothing else supplies one; it must pass the checks  |
| `pattern`     | typed pattern, see below                                             |
| `values`      | allowed values, `enum` only                                          |

Field names match `^[a-z][a-z0-9_]*$`.

A Document Type lists the fields it uses. An entry is a name, or a one-key
mapping from the name to `required` and/or `default` to override the
declaration for that Document Type.

### Field types

| Type     | Value                                         | Position         |
| -------- | --------------------------------------------- | ---------------- |
| `string` | one line of text                              | text             |
| `text`   | multi-line text                               | text             |
| `list`   | list of strings                               | text             |
| `date`   | a date (free text or a YAML date)             | text             |
| `bool`   | `true` or `false`                             | condition        |
| `enum`   | one of `values`                               | non-text         |
| `color`  | a colour, checked by `pattern`                | non-text         |
| `asset`  | a package-relative file path, checked by `pattern` | non-text    |

### Typed patterns

Values in non-text positions (colour, length, file name, option, enum) are
the way TeX could be injected, so they must be constrained by a strict typed
pattern:

- `color` and `asset` fields require a `pattern`; `enum` fields require
  `values` (each made of letters, digits, space, `.`, `_`, `-`).
- Any other field that is interpolated into a non-text position of a Pandoc
  template also requires a pattern. A non-text position is an argument of a
  command such as `\definecolor`, `\textcolor`, `\includegraphics`,
  `\setlength`, `\input`, `\usepackage` or `\setmainfont`, or any option list
  in square brackets. The list is `nonTextCommands` in
  `internal/packages/pandocvars.go`.
- A pattern is an RE2 regular expression matched against the whole value (it
  is wrapped in `\A(?:...)\z`). It must not match any of a set of hostile
  probes: backslash, braces, `%`, `$`, `&`, `#`, `~`, `^`, `|`, quotes,
  newlines, `..`, a leading `/`. A pattern like `.*` or `[^ ]+` is a load
  error ("too permissive").
- A field's `default` must pass its own type and pattern.

An asset value must also be a clean package-relative path: plain segments, no
`..`, not absolute.

### Pandoc templates

Each Document Type names one Pandoc template. Variables use the Pandoc
syntax: `$name$`, `${name}`, `$if(name)$`, `$elseif(name)$`, `$for(name)$`,
`$sep$`, `$else$`, `$endif$`, `$endfor$`, `$$` for a literal dollar and `$--`
comments. Partials (`$name()$`) and applied functions are not supported.

A variable is never allowed to build a TeX control sequence name
(`\foo$name$` or `\$name$` are errors).

### Drift guard

`validate` and startup compare the variables a Pandoc template uses with the
manifest:

- a variable that is not declared in `fields`: **error**;
- a variable that is declared but not listed by the Document Type using it:
  **error**;
- a Document Type listing a field that is not declared: **error**;
- a declared field no Pandoc template uses: **warning**.

Variables Pandoc provides itself are ignored: `body`, `highlighting-macros`,
`header-includes`, `include-before`, `include-after`, `meta-json`,
`pagetitle`, `titleblock`, `lang`, `babel-lang`, `babel-otherlangs`, `title`,
`date` and the loop variable `it`. The list lives in one place,
`pandocBuiltins` in `internal/packages/pandocvars.go`.

### TeX deny-list

Every TeX file in a Template package (`.latex` Pandoc templates and
`.tex`, `.sty`, `.cls`, `.def`, `.cfg`, `.ltx`, `.clo` files) is scanned after
comments are removed. Any of the following is an error:

- Lua and shell execution: `\directlua`, `\latelua`, `\luaexec`, `\ShellEscape`,
  the `luacode` environment and package, and the `shellesc`, `catchfile`,
  `minted`, `pythontex`, `sagetex`, `gnuplottex` and `markdown` packages;
- file writing: `\write` (so `\write18` and `\immediate\write18`), `\newwrite`,
  `\openout`, `\closeout`, `\immediate`;
- file reading: `\read`, `\readline`, `\newread`, `\openin`, `\verbatiminput`,
  `\VerbatimInput`, `\lstinputlisting`, `\CatchFileDef`, `\CatchFileEdef`,
  `\pdffiledump`, `\pdfmdfivesum`, `\pdffilesize`;
- file inclusion: `\include`, `\includeonly`, `\InputIfFileExists`, `\@@input`,
  `\@input`, `\@iinput`;
- obfuscation: `^^` character notation, `\catcode`, `\csname` and
  `\scantokens`;
- `\input` unless it is written exactly `\input{shared/NAME}` where `NAME`
  (letters, digits, `.`, `_`, `-`) is a file in the package's own `shared/`
  directory. Anything else (absolute, `..`, another directory, a macro, no
  braces) is rejected;
- `\includegraphics` with a literal path that is absolute or contains `..`
  (a path that comes from a variable is covered by the field's pattern).

Lua files (`.lua`) anywhere in a Template are rejected.

This is a safety net against accidents and careless Templates, not a
sandbox. The Worker still runs without network or secrets and with shell
escape disabled.

## Configuration package

```
configurations/example/
  configuration.yaml
  logo.svg
```

```yaml
manifest_version: 1

# A Document may override these.
defaults:
  document_type: letter     # the Document Type used when a Document names none
  closing: Kind regards

# Fixed Values: a Document cannot change these.
fields:
  name: Example Garden Society
  logo: logo.svg
  accent_color: "2F6F5E"
```

- `extends` is reserved: it fails with "extends is not supported in v1".
- Values are strings, booleans, dates or lists of strings.
- Defaults may carry fields a given Template does not declare, so that one
  Configuration can serve several Templates. A Fixed Value the Template does
  not declare has no effect and produces a warning.
- When a Configuration is bound to a Template, every value of a declared
  field is checked against that field: type, pattern or enum values, and for
  assets the file path. `defaults.document_type` must name a Document Type of
  the Template.
- A Default that is shadowed by a Fixed Value of the same name produces a
  warning.

How a Document's Metadata Block, the Configuration Defaults and the manifest
defaults combine into field values is described in [metadata.md](metadata.md).

## Path confinement

For both package kinds:

- asset and Document Type template paths must be relative and stay inside the
  package, also after resolving symlinks;
- any symlink in a package that points outside it is an error, even if
  nothing references it;
- a file that is a Git LFS pointer stub (it starts with
  `version https://git-lfs.github.com/spec/v1`) is an error: fetch the real
  content with `git lfs pull`.

## Standard Fields

A documented set of common field names (`title`, `date`, `language`, `author`,
`recipient`, `subject`, `accent_color`, ...) is listed in
[standard-fields.md](standard-fields.md). Reusing them keeps Configurations
portable. They are a convention: any field may be declared. `validate` warns
when a Template declares a Standard Field name with a different type.

## validate

```sh
renderer validate                       # as the service would: image service config
renderer validate --service-config renderer.yaml
renderer validate --templates-root ./templates --configs-root ./configurations
renderer validate --templates-root ./templates --configs-root ./configurations \
  --template mine --config example      # one pair, including the pair checks
```

`validate` uses the same loader and checks as service startup:

- with a service configuration (`--service-config`, or `RENDERER_SERVICE_CONFIG`
  when no root or name flags are given) it checks the allow-list and every
  package on it, and warns about packages that are not listed;
- without one it checks every package found in the roots on its own; with
  `--template` and `--config` it checks just those and their pairing;
- `--template` alone or `--config` alone checks a single package.

Output has one finding per line:

```
error   template/mine letter.latex:14: drift: variable "ghost" is used but not declared in the manifest fields
warning template/mine manifest.yaml:9: field "orphan" is declared but never used in a Pandoc template
validate: failed with 1 error(s), 1 warning(s)
```

All findings are collected, not just the first. The exit code is 1 if there is
any error, 0 if there are none (warnings only is success), 2 for bad usage.

The service uses the same loader at startup (`internal/startup`) and refuses
to start, naming every cause, when anything is an error.
