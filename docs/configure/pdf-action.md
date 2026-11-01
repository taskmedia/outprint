---
sidebar_position: 3
---

# Where the PDF action appears

Outprint adds its action to Outline's own interface. By default it is an item in
the **Export** menu of a document. You can move it to a button in the document
header. The choice is global: it applies to every Collection and every document,
because it is a matter of how your Outline looks, not of a Binding.

The action only appears on documents that have a Binding. Documents in a
Collection without one show nothing.

| Placement | Where the action is | Default |
|-----------|--------------------|---------|
| `menu` | **Export → Formatted PDF**, in the **…** menu of the document | yes |
| `button` | a button in the header, directly before **Share** | no |

The person who runs the injector sets it with `OUTPRINT_BUTTON_PLACEMENT`. See
[Injector and PDF action](../deploy/injector.md#the-pdf-action) for all options.

## Export menu

Open the **…** menu of a document, then **Export**. **Formatted PDF** is listed
after the file formats. The menu is there for everyone who can open the
document, also if they cannot edit it and on a small screen.

While the PDF is made the menu has already closed, so a small panel under the
**…** button says "Creating PDF…". If something is wrong with the document, the
same panel shows what to fix. Press Escape or **×** to close it. The focus goes
back to the **…** button.

## Header button

A button with a PDF icon and the word "PDF" appears before **Share**. Where
Outline does not show **Share** (a reader without edit permission, or a narrow
screen), the button sits before the **…** button instead. The button is disabled
while the PDF is made, and messages appear directly under it.

With `OUTPRINT_BUTTON_STYLE=icon` the button shows only the icon. It keeps an
accessible name and a tooltip.

## Floating button

If Outprint cannot find the place for the chosen placement (for example because
a new version of Outline changed its markup), it waits three seconds and then
shows a small floating button at the bottom right of the page, and writes a
warning to the browser console. Set `OUTPRINT_BUTTON_FALLBACK=off` to show
nothing instead.

## The metadata block in Outline

By default readers do not see the [metadata block](../write/fill-in-the-document.md)
of a document, and authors see it folded behind a **PDF details** toggle (**PDF-Details** in German) that they
can open with a click. Set `OUTPRINT_BUTTON_METADATA_BLOCK` to change this:
`hide` shows the block to authors as is, `show` shows it to everyone. The limits
of hiding (Outline's markup, share links, exports, search) are listed under [Hiding the metadata
block](../deploy/injector.md#hiding-the-metadata-block).

## Not the same as Outline's own PDF

Outline can export a plain PDF itself when its operator sets
`PDF_EXPORT_ENABLED`. That entry is called **PDF** and sits in the same
**Export** submenu. It prints the document as the browser shows it, without a
Template. Outprint's entry is called **Formatted PDF** (**Formatiertes PDF** in
German) so the two cannot be confused. If you want other words, change the
label with `OUTPRINT_BUTTON_LABELS`.

## Language

The labels follow the language of the Outline user (English and German are built
in). A language without a text, and a user without a language, get the language of
the browser, then English.
