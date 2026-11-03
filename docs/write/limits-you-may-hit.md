---
sidebar_position: 7
---

# Limits you may hit

**Reference.** The size limits of a PDF and what happens when a document exceeds one.

Outprint keeps a PDF within fixed sizes, so that one large document cannot slow
everybody down. A document that does not fit fails with a message. Outprint
never cuts a document short.

The person who runs Outprint can change some of these values. The numbers
below are the standard ones.

| What | Limit | If you pass it |
|------|-------|----------------|
| Text of the document | 1 MiB of text | [`too_large`](when-something-goes-wrong.md#too_large) |
| Pictures in one document | 50 | [`too_large`](when-something-goes-wrong.md#too_large) |
| One picture | 10 MiB (an SVG: 2 MiB) | [`too_large`](when-something-goes-wrong.md#too_large) |
| All pictures together | 50 MiB | [`too_large`](when-something-goes-wrong.md#too_large) |
| Pixels of one picture | 40 million | [`pixels`](when-something-goes-wrong.md#pixels) |
| Time to make the PDF | 60 seconds | [`timeout`](when-something-goes-wrong.md#timeout) |
| Pages of the PDF | 500 | [`pages_exceeded`](when-something-goes-wrong.md#pages_exceeded) |
| Size of the PDF | 50 MiB | [`pdf_too_large`](when-something-goes-wrong.md#pdf_too_large) |

## Only a few PDFs at a time

Making a PDF takes effort. Outprint makes a few at a time and lets the others
wait in a short line.

- You can have **one PDF in progress**. If you click the button twice, the
  second click gets [`user_limit`](when-something-goes-wrong.md#user_limit).
- If many people click at once, the line can fill up. The message is
  [`busy`](when-something-goes-wrong.md#busy). Wait a few seconds and try again.

Messages about a full line are not your fault. Nothing in your document needs to
change.

## No copy is kept

Outprint makes a new PDF at every click and keeps nothing. A PDF reflects the
document as it is saved now. You can only download PDFs of documents that you
may read in Outline.

## If a document is too big

- Split it into two documents. Each gets its own PDF.
- Scale pictures down before you upload them.
- Remove pictures that the reader does not need.
