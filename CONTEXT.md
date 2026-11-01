# Outprint

Outprint turns Outline documents into typeset PDFs. A Template provides the look and structure; a Template Configuration adapts it to an organisation or purpose.

## Language

**Template**:
A self-contained style family: structure, typography and page design, offering one or more Document Types and declaring which fields it accepts. May be generic or belong to one organisation; the model does not distinguish.
_Avoid_: Theme, Style, Layout

**Document Type**:
A kind of output a Template offers (for example a letter or a report), each with its own formatting and accepted fields. A Template Configuration sets a default; a Document may override it.
_Avoid_: Mode, Variant, Kind

**Template Configuration**:
A named set of values that adapts a Template: name, logo, colours, Defaults, optionally a font override. Pure data. One Template can have many Template Configurations, and one Template Configuration can serve several Templates.
_Avoid_: Brand, Profile, Theme

**Fixed Values**:
The part of a Template Configuration a Document cannot change: logo, colours, optional font override. The counterpart to Defaults, which a Document may override.
_Avoid_: Brand values, Locked fields

**Defaults**:
The field values a Template Configuration supplies when a Document does not set them itself. A Document's own values win.
_Avoid_: Presets

**Metadata Block**:
The fenced block at the very start of a Document in which its author sets field values (recipient, subject, …). It is not part of the rendered content. In Outline's interface the injector script keeps it out of the way: readers do not see it, and authors see it folded behind a toggle called "PDF details" (an operator setting can change this).
_Avoid_: Frontmatter, Header

**Standard Fields**:
A documented set of common field names (for example subject, recipient) that Templates are encouraged to reuse so Template Configurations stay portable. A convention, not a requirement: a Template may declare any fields it likes. Where the pipeline derives a default, it applies to every Template that declares the field: `title`, `date`, and `subject` (the Document's title, unless the author sets it, even to empty).
_Avoid_: Core fields, Reserved fields

**Binding**:
The pairing of one Template and one Template Configuration that a Collection selects. Every Document in the Collection, including nested ones, is rendered with it.
_Avoid_: Assignment, Mapping

**Collection**:
An Outline collection; the unit that carries a Binding for its Documents.

**Document**:
An Outline document; the content that is rendered.

**Attachment**:
A file uploaded to Outline and referenced from a Document, in practice an image. Only Attachments of the configured Outline are rendered; images from anywhere else are rejected.
_Avoid_: Upload, Asset
