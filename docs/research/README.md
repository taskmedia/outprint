# Research notes

These notes were written before and during the design of the project. They
record what was read, tried and decided, with the sources. Each ends with an
**Outcome** section that says what the implementation did with it; where a
note and the docs differ, the docs win.

Legend used in the notes: **[V]** verified by an experiment or a primary
source, **[S]** read in source code or documentation, **[I]** inferred.

| Note | Question | Outcome in short |
|------|----------|------------------|
| [license.md](license.md) | Which licence for the code, and what does shipping Pandoc, TeX and fonts require? Includes the GPL FAQ recheck. | MIT; `THIRD_PARTY_NOTICES` with a source offer |
| [lualatex-safer.md](lualatex-safer.md) | Can `lualatex --safer` harden the engine? | No; paranoid reads and writes inside the job directory instead ([worker.md](../worker.md)) |
| [markdown-latex-fidelity.md](markdown-latex-fidelity.md) | What breaks when Outline Markdown goes through Pandoc to LaTeX, and how to fix each? | Fixtures, goldens and Worker filters |
| [outline-surface.md](outline-surface.md) | Where can a Collection Binding live, and can the UI be extended without a fork? | Overview block plus a reverse-proxy injector; info-string caveat in issue #42 |
| [test-environment.md](test-environment.md) | How to run Outline locally and verify PDFs? | Three tiers, Compose dev stack, text goldens |

The versions in the notes (Outline 1.11, TeX Live 2026 on a laptop) are the
ones of the day. The pinned versions of the shipped image are in the
`Dockerfile` and `THIRD_PARTY_NOTICES`.

None of this is legal advice.
