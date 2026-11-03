---
sidebar_position: 6
---

# Template checks

**Reference.** What `outprint validate` and service startup check in a package.

Both use the same loader. All findings are collected, not just the first. An
error stops a package from loading; a warning does not. To run the checks, see
[Try your Template](../templates/try-your-template.md).

## Typed patterns

Values in non-text positions (colour, length, file name, option, enum) are the
way TeX could be injected, so they must be constrained by a strict pattern.

- `color` and `asset` fields require a `pattern`. `enum` fields require
  `values`.
- Any other field that is interpolated into a non-text position of a Pandoc
  template requires a pattern too. A non-text position is an argument of a
  command such as `\definecolor`, `\textcolor`, `\includegraphics`,
  `\setlength`, `\input`, `\usepackage` or `\setmainfont`, or any option list
  in square brackets. The list is `nonTextCommands` in
  `internal/packages/pandocvars.go`.
- A pattern is an RE2 regular expression matched against the whole value. It
  must not match any of a set of hostile probes: backslash, braces, `%`, `$`,
  `&`, `#`, `~`, `^`, `|`, quotes, newlines, `..`, a leading `/`. A pattern
  such as `.*` or `[^ ]+` is a load error ("too permissive").
- An `asset` value must be a clean package-relative path: plain segments, no
  `..`, not absolute.

## Drift guard

The variables a Pandoc template uses are compared with the manifest:

| Finding | Level |
|---------|-------|
| a variable that is not declared in `fields` | error |
| a variable that the Document Type using it does not list | error |
| a Document Type that lists a field that is not declared | error |
| a declared field that no Pandoc template uses | warning |

Variables Pandoc provides itself are ignored: `body`, `highlighting-macros`,
`header-includes`, `include-before`, `include-after`, `meta-json`,
`pagetitle`, `titleblock`, `lang`, `babel-lang`, `babel-otherlangs`, `title`,
`date` and the loop variable `it` (`pandocBuiltins` in
`internal/packages/pandocvars.go`).

## TeX deny-list

Every TeX file in a Template package (`.latex` and `.tex`, `.sty`, `.cls`,
`.def`, `.cfg`, `.ltx`, `.clo`) is scanned after comments are removed. These
are errors:

- Lua and shell execution: `\directlua`, `\latelua`, `\luaexec`, `\ShellEscape`,
  the `luacode` environment and package, and the `shellesc`, `catchfile`,
  `minted`, `pythontex`, `sagetex`, `gnuplottex` and `markdown` packages.
- File writing: `\write` (so `\write18` too), `\newwrite`, `\openout`,
  `\closeout`, `\immediate`.
- File reading: `\read`, `\readline`, `\newread`, `\openin`, `\verbatiminput`,
  `\VerbatimInput`, `\lstinputlisting`, `\CatchFileDef`, `\CatchFileEdef`,
  `\pdffiledump`, `\pdfmdfivesum`, `\pdffilesize`.
- File inclusion: `\include`, `\includeonly`, `\InputIfFileExists`, `\@@input`,
  `\@input`, `\@iinput`.
- Obfuscation: `^^` notation, `\catcode`, `\csname`, `\scantokens`.
- `\input` unless it is written exactly `\input{shared/NAME}`, where `NAME`
  (letters, digits, `.`, `_`, `-`) is a file in the package's own `shared/`
  folder.
- `\includegraphics` with a literal path that is absolute or contains `..`.
- Any `.lua` file in the package.

This is a safety net against accidents and careless Templates, not a sandbox.
The Worker still runs without network or secrets and with shell escape off.

## Path confinement

For both package kinds:

- Asset and Pandoc template paths must be relative and stay inside the
  package, also after resolving symlinks.
- A symlink that points outside the package is an error, even if nothing
  references it.
- A file that is a Git LFS pointer stub (it starts with
  `version https://git-lfs.github.com/spec/v1`) is an error. Fetch the real
  content with `git lfs pull`.

## Standard Fields

`validate` warns when a Template declares a name from
[Standard Fields](standard-fields.md) with a different type. The warning never
fails validation.

## Output of `validate`

One finding per line. The exit code is 1 if there is any error, 0 if there
are none (warnings alone are success) and 2 for bad usage.

```
error   template/mine letter.latex:14: drift: variable "ghost" is used but not declared in the manifest fields
warning template/mine manifest.yaml:9: field "orphan" is declared but never used in a Pandoc template
validate: failed with 1 error(s), 1 warning(s)
```
