# Outline surface for a collection-to-template binding and per-document render settings

Question: which Outline facilities can carry a collection-to-template binding and per-document render settings, and what does the document menu DOM look like for UI injection?

## Version pinned

- Outline release v1.11.0 (latest release, published 2026-10-12), commit `cb0207201ed2c8b641809c26e41fbe2bde297072`, inspected from a shallow clone of the tag.
- Source URL base: `https://github.com/outline/outline/blob/v1.11.0/<path>`
- Official docs consulted: https://www.getoutline.com/developers (API reference). The fetch returned only a summary, so field-level claims are verified against source, not the docs page.
- Legend: **[V]** = verified in the cited source at v1.11.0. **[I]** = inferred, not directly confirmed.

## 1. Fields exposed by the API

### documents.info [V]

- Handler: `server/routes/api/documents/documents.ts` (route `documents.info`).
- Presenter: `server/presenters/document.ts`.
- Request body is `id` or `shareId` (`server/routes/api/documents/schema.ts`, `DocumentsInfoSchema`).
- Response fields:
  - Identity and content: `id`, `url`, `urlId`, `title`, `data` (ProseMirror JSON), `text` (Markdown).
  - Appearance: `icon`, `color`, `language`, `fullWidth`.
  - `preferences`, typed as `{ headingPrefix?: HeadingPrefixStyle }` only (`shared/types/user.ts`, `DocumentPreferences`; the zod schema in `documents/schema.ts` accepts only `headingPrefix`).
  - Timestamps and users: `createdAt`, `updatedAt`, `publishedAt`, `archivedAt`, `deletedAt`, `createdBy`, `updatedBy`, `collaboratorIds`, `revision`.
  - Relations: `collectionId`, `parentDocumentId`, `templateId`.
  - Others: `tasks`, `insightsEnabled`, `summary`, and `sourceMetadata` for imported documents.
- There is no free-form metadata, key/value or custom-field column on a document. `preferences` is a closed schema, so it cannot carry render settings.
- `icon`, `color` and `language` could be abused as a side channel. That is hacky, and they are user-visible.

### Parent collection info inside documents.info [V]

- `documents.info` returns **only `collectionId`** (plus `parentDocumentId` and `isCollectionDeleted`), not a collection object.
- A second call to `collections.info` is needed to get collection fields.
- The `policies` block in the response covers the document only.

### collections.info [V]

- Handler: `server/routes/api/collections/collections.ts`.
- Presenter: `server/presenters/collection.ts`.
- Fields:
  - Identity: `id`, `url`, `urlId`, `name`.
  - Content: `data` (ProseMirror JSON of the collection overview, returned when header `x-api-version >= 3`), `description` (deprecated Markdown column, max length `CollectionValidation.maxDescriptionLength`).
  - Appearance and ordering: `icon`, `color`, `sort`, `index`.
  - Access and behaviour: `permission`, `sharing`, `commenting`, `templateManagement`.
  - Lifecycle: `createdAt`, `updatedAt`, `archivedAt`, `deletedAt`, `deprecatedReason`, `archivedBy`.
  - Import provenance: `sourceMetadata`.
- No free-form metadata field exists here either.
- The overview body is rich text and can hold a fenced code block. `collections.update` accepts `data` or `description` (`collections/schema.ts`, `CollectionsUpdateSchema`).
- `collections.info` does not include `documentStructure`. Use `collections.documents` (policy `readDocument`) for the published tree.

### Templates (separate model in v1.11) [V]

- A `Template` is its own model, not a document with `template=true` (`server/models/Template.ts`, `server/presenters/template.ts`, `server/routes/api/templates/templates.ts`).
- Endpoints: `templates.create/list/info/update/delete/restore/duplicate`. `templates.list` filters by `collectionId`.
- Fields: `id`, `title`, `data`, `icon`, `color`, `fullWidth`, `collectionId`, `publishedAt`, `createdBy`, `updatedBy`.
- `collectionId` is nullable; a null value means a workspace-wide template.
- `documents.info` exposes `templateId`, the source template used at creation. That is a provenance link only, not a render binding.

### Official docs cross-check [V, shallow]

- https://www.getoutline.com/developers confirms the endpoints exist: `documents.info`, `collections.info`, templates (create, list, info, update, delete, restore, duplicate), and webhook subscriptions (admin-only).
- The page lists API-key scopes: `read`, `write`, `documents:read`, `collections:write`, and endpoint-level patterns such as `documents.info` or `documents.*`.
- A least-privilege key for a renderer could therefore be limited to read endpoints. The endpoint-level scope format is confirmed in source (`server/models/ApiKey.ts`, `scope` column validated by `IsScope`) and in the docs summary.

## 2. Can a config document inside a collection carry the binding?

Four candidate carriers, in order of fit:

1. **Collection overview body (`collections.info` `data`).**
   - Pros:
     - It needs no extra document, and it is already fetched by collection id.
     - It does not show up in the sidebar tree, in the `collections.documents` structure, or in a collection export as a document.
     - A single `collections.info` call returns it.
     - It can hold a structured fenced block, such as a YAML or JSON code block.
   - Cons:
     - It is visible to every reader as part of the collection overview.
     - It is intended as prose, so users may edit or reformat it. The parser must be tolerant.
     - Edit rights are collection Admin or team admin (see section 3).
2. **A template scoped to the collection (`templates.list` with `collectionId`).** [V for API and policy, I for suitability]
   - Pros:
     - It does not appear in the sidebar tree.
     - Write access is separately configurable via `templateManagement` (default Admin).
     - Template webhooks exist (`templates.create/update/delete`).
     - It can be found by title convention.
   - Cons:
     - It is semantically a template, and users can apply it to new documents. Its content would end up in other documents unless it is deliberately empty or marked.
     - Naming is the only discriminator.
3. **A normal, published config document in the collection.**
   - Pros: ordinary document webhooks and search.
   - Cons:
     - It is visible in the sidebar and in search.
     - It would be included in collection exports and in any "render every document in the collection" flow, so the renderer must exclude it.
     - Collection editors can edit or delete it.
4. **An unpublished draft inside the collection.** [I]
   - A draft has `collectionId` set and `publishedAt = null`.
   - The `read` policy (`server/policies/document.ts`) grants read to the draft's creator, and also via `readDocument` on the collection.
   - Draft listing semantics were not fully traced, so this is not recommended. Drafts are tied to a creator and can be lost when that user is removed.

Per-document settings have no first-class home:

- Inside the document body, for example a leading fenced block or front-matter-like first paragraph. This is visible and editable by anyone with update rights.
- In the config carrier above, keyed by document id or path, which gets stale when documents move.
- Outside Outline, in the service's own store.

## 3. Permission semantics [V]

- `server/policies/collection.ts`:
  - `read` and `readDocument` apply to team members, except private collections (membership required) and guests (membership required). Team admins can always read.
  - `update` and `archive` of the collection (including its description or overview) require team admin or a collection membership with Admin permission.
  - `createDocument`, `updateDocument` and `deleteDocument` require collection permission ReadWrite (via the default permission or membership) or Admin.
  - `createTemplate` and `manageTemplate` require team admin, collection Admin membership, or `templateManagement == ReadWrite` together with ReadWrite access.
- `server/policies/template.ts`: `read` follows the collection's `readDocument`; `update` requires `manageTemplate` on the collection.
- `server/policies/document.ts`:
  - `read` depends on document membership, being the creator of a draft, or collection `readDocument`.
  - `update` depends on document ReadWrite/Admin membership, or `updateDocument` on the collection.
- Consequence for a binding stored in the overview:
  - Only collection admins and team admins can change it. This is a sensible control point.
  - A service reading it through an API key sees whatever the key's user can read. A private collection invisible to that user is invisible to the service.
- Consequence for a binding stored in a template:
  - Changeable by whoever has `manageTemplate`, which is configurable per collection.

## 4. Webhooks [V]

- Source: `plugins/webhooks/server/` (the `WebhookProcessor` matches `subscription.events` against event names, with prefix and `*` matching).
- Management is limited to workspace admins (docs summary, https://www.getoutline.com/developers).
- Signature header is `Outline-Signature` with value `t=<ms>,s=<hex HMAC-SHA256 of "<t>.<payload>">`, set only when a secret is stored. See `DeliverWebhookTask.ts` and `WebhookSubscription.signature()`.
- Handled event families:
  - `documents.*`: create, publish, unpublish, delete, permanent_delete, archive, unarchive, restore, move, update, title_change, plus add/remove user/group.
  - `collections.*`: create, update, delete, move, permission_changed, archive, restore, plus add/remove user/group.
  - `templates.create`, `templates.update`, `templates.delete`.
  - `revisions.create`, and more.
- Payloads carry a presented model (`presentDocument` or `presentCollection`), so `collectionId` and `data` are available. A change to the overview body fires `collections.update` [I; the event path is not traced to the `collection.save` hook, though the web UI saves via `collections.update`].
- Webhooks are push notifications of change only. They are not a render trigger by themselves. The binding is not delivered anywhere else, so the service has to cache or re-read it.
- Gaps and tooling constraints:
  - No webhook event for a menu click.
  - Subscriptions are created in the admin UI or via the `webhookSubscriptions.*` API, so they are an install-time step.

## 5. Plugin system limits (without forking) [V]

- Server plugins: `server/utils/PluginManager.ts`, `loadPlugins()` globs `build/plugins/*/server/index.[jt]s`.
  - Plugins are compiled at image build time (`build.js` runs swc over `./plugins`).
  - An external plugin therefore requires a custom build or fork. It cannot be added at runtime.
  - Server hooks: API (mounts a Router), AuthProvider, EmailTemplate, IssueProvider, Processor, SearchProvider, MentionProvider, Task, UnfurlProvider, Uninstall, GroupSyncProvider.
- Client plugins: `app/utils/PluginManager.ts`, hooks are only `Settings`, `Imports`, `Icon`.
  - There is **no hook for the document menu, document header, editor toolbar or export actions**.
  - The document menu items are fixed in `app/hooks/useDocumentMenuAction.tsx`.
- No env or setting for custom JS or CSS injection was found in `server/env.ts` (search for `CUSTOM_`, `INJECT`, `EXTRA_SCRIPT`, `HEAD_HTML`). [V for what was searched, I for the absence overall]
- There is already a built-in PDF export (`downloadDocumentAsPDF`, visible when `env.PDF_EXPORT_ENABLED`), but with no template or setting hooks. It is not extensible from outside.
- The Outline Integrations area (unfurl, mention, link provider) cannot add menu items either.

## 6. Document menu components and DOM (injection feasibility)

React components, all in `app/` at v1.11.0:

- `scenes/Document/components/Header.tsx` renders `<DocumentMenu document align="end" neutral showDisplayOptions .../>` inside an `<Action>` wrapper in the document header.
- `menus/DocumentMenu.tsx`:
  - It builds a `DropdownMenu` with `ariaLabel={t("Document options")}`. The label is translated, so it is locale-dependent (`shared/i18n/locales/en_US/translation.json`, `"Document options"`).
  - The trigger is `OverflowMenuButton`, a `NudeButton` or `Button` with `MoreIcon`.
  - `append` renders the "Full width", "Enable embeds", "Enable viewer insights" toggles and the heading prefix submenu.
- `components/Menu/DropdownMenu.tsx`:
  - It uses Radix `DropdownMenu` via `components/primitives/Menu/index.tsx`.
  - `MenuContent` is wrapped in a Radix **Portal**, so the open menu is attached to `document.body`, not inside the header.
  - Menu items are computed only while the menu is open (`useComputed(... if (!open) return [])`). Before opening, the item DOM does not exist.
  - On mobile (`useMobile()`) a Vaul `Drawer` is used instead, with different markup.
- `hooks/useDocumentMenuAction.tsx` defines the item order and sections: starred/subscribe, Edit, Rename, Share, Template, Duplicate, Publish, Archive, Move, Apply template, Import, New nested doc, Pin, Comments, History, Insights, Split view, Desktop, Present, **Export** (submenu: Markdown, HTML, TextBundle, PDF, print), Copy, Search, Delete.
- The "Export" submenu children include `downloadDocumentAsPDF`, whose `perform` calls `document.download({ contentType: Pdf })`. Source: `app/actions/definitions/documents.tsx`.

DOM facts from Radix (Radix primitives source at `main`, not pinned to the installed version, which is `@radix-ui/react-dropdown-menu ^2.1.24` per `package.json`; treat attribute names as [V for Radix main, I for exact installed version]):

- Trigger: `<button aria-haspopup="menu" aria-expanded="true|false" data-state="open|closed" aria-label="Document options">`.
- Content: `<div role="menu" aria-orientation="vertical" data-radix-menu-content data-state="open" aria-label="Document options" ...>` under a Portal wrapper in `body`.
- Items: `role="menuitem"`, `data-highlighted`, and Radix collection attribute; separators `role="separator"`; groups `role="group"`.
- Styling uses styled-components, so class names are hashed and unstable between builds. There are **no `data-testid` attributes** in `app/components/Menu` or `app/components/primitives/Menu` (grep returned nothing).

Feasibility of injecting a "Render with template X" item via an external script:

- A MutationObserver on `body` could detect `[role="menu"][aria-label]`, then append a cloned item. This is technically possible but fragile [I]:
  - It depends on a locale-dependent label, on Radix internals, and on a portal that mounts and unmounts per open.
  - React re-renders may remove foreign nodes.
  - The mobile drawer needs different handling.
  - Keyboard navigation (roving focus) will not include foreign nodes unless wired manually.
  - Cloned styles come from hashed classes that change with each build.
- Getting the script to load in the first place has no supported path: no custom-JS setting was found, so it would need a reverse-proxy rewrite of the HTML (self-hosted only) or a browser extension. A proxy rewrite would break CSP unless the CSP is adjusted.
- Safer UI surface candidates (all [I]):
  - Outline's own "Export" menu is not extensible.
  - A link inside the document or the overview body pointing to the service's render URL is robust, because it is ordinary content. The URL can carry the document id.
  - A browser bookmarklet or extension on the document page is separate from Outline internals.
  - A slash-command or link unfurl would need a plugin and therefore a fork.

## 7. Conclusion: where the binding can live

| Option | Works without fork | Editable by | Resilience | Notes |
|---|---|---|---|---|
| Install-time config only (service's own config maps collection id to template) | yes | operator | high | Needs redeploy or config reload for each change. Unknown collections have no binding. Ids are stable (uuid; `urlId` may change). |
| In Outline: collection overview body (fenced block) | yes | collection admin / team admin | medium | One `collections.info` read. Changes announced by `collections.update` webhook [I]. Free text can be malformed, so validate and fall back. |
| In Outline: collection-scoped template | yes | `manageTemplate` holders | medium | Hidden from sidebar. May be applied to new documents by users. |
| In Outline: normal config doc | yes | collection editors | low | Visible, exportable, can be deleted or moved. |
| Both (install-time default plus in-Outline override) | yes | operator plus collection admin | highest | Precedence rule: in-Outline value overrides install-time default, install-time config is the allow-list of valid template names. |

- The binding **can** live in Outline without forking. The best-fit carrier is the collection overview body, with a collection-scoped template as an alternative when separate write permissions are needed.
- Per-document settings have no clean structured home in Outline. Options are in-document text, an entry keyed by document id inside the collection-level carrier, or service-side state.
- A first-class UI entry in the document menu is **not** achievable without forking Outline (client plugin hooks are limited to Settings, Imports and Icon). Fragile DOM injection is possible but unsupported and depends on a script-loading mechanism that Outline does not provide.
- Recommended shape (opinion): a hybrid.
  - The install-time config is the authority for which templates exist and the default.
  - A validated block in the collection overview selects a template from that allow-list.
  - Per-document overrides go in a small block inside the document or the collection carrier.
  - The service falls back to the default when the block is absent or invalid.
  - Entry point for rendering is a plain link or a webhook-driven job, not a menu injection.

## Sources

- https://github.com/outline/outline/tree/v1.11.0 (commit `cb0207201ed2c8b641809c26e41fbe2bde297072`)
- https://github.com/outline/outline/blob/v1.11.0/server/presenters/document.ts
- https://github.com/outline/outline/blob/v1.11.0/server/presenters/collection.ts
- https://github.com/outline/outline/blob/v1.11.0/server/presenters/template.ts
- https://github.com/outline/outline/blob/v1.11.0/server/routes/api/documents/documents.ts
- https://github.com/outline/outline/blob/v1.11.0/server/routes/api/documents/schema.ts
- https://github.com/outline/outline/blob/v1.11.0/server/routes/api/collections/collections.ts
- https://github.com/outline/outline/blob/v1.11.0/server/routes/api/collections/schema.ts
- https://github.com/outline/outline/blob/v1.11.0/server/routes/api/templates/templates.ts
- https://github.com/outline/outline/blob/v1.11.0/server/policies/collection.ts
- https://github.com/outline/outline/blob/v1.11.0/server/policies/document.ts
- https://github.com/outline/outline/blob/v1.11.0/server/policies/template.ts
- https://github.com/outline/outline/blob/v1.11.0/plugins/webhooks/server/tasks/DeliverWebhookTask.ts
- https://github.com/outline/outline/blob/v1.11.0/plugins/webhooks/server/processors/WebhookProcessor.ts
- https://github.com/outline/outline/blob/v1.11.0/server/models/WebhookSubscription.ts
- https://github.com/outline/outline/blob/v1.11.0/server/utils/PluginManager.ts
- https://github.com/outline/outline/blob/v1.11.0/app/utils/PluginManager.ts
- https://github.com/outline/outline/blob/v1.11.0/build.js
- https://github.com/outline/outline/blob/v1.11.0/app/menus/DocumentMenu.tsx
- https://github.com/outline/outline/blob/v1.11.0/app/components/Menu/DropdownMenu.tsx
- https://github.com/outline/outline/blob/v1.11.0/app/components/primitives/Menu/index.tsx
- https://github.com/outline/outline/blob/v1.11.0/app/hooks/useDocumentMenuAction.tsx
- https://github.com/outline/outline/blob/v1.11.0/app/actions/definitions/documents.tsx
- https://github.com/outline/outline/blob/v1.11.0/app/scenes/Document/components/Header.tsx
- https://github.com/radix-ui/primitives/blob/main/packages/react/menu/src/menu.tsx
- https://github.com/radix-ui/primitives/blob/main/packages/react/dropdown-menu/src/dropdown-menu.tsx
- https://www.getoutline.com/developers

## Outcome

The hybrid shape of section 7 was implemented: the service configuration is the allow-list, a fenced block in the Collection overview selects a Template ([Choose a Template and Configuration](../configure/choose-a-template.md)), and the Download PDF button is injected by a reverse proxy ([Injector and download button](../deploy/injector.md)).

One assumption of section 2 turned out to be wrong when tested against a real Outline in the seeded dev stack: the editor keeps only the first word of a code fence's info string, so a block opened with an info string of two words is stored with the first word alone. This affects the markers used for the Binding and the Metadata Block and was tracked in issue #42.

Resolution (issue #42): both blocks are plain `yaml` fences. The Metadata Block is the first thing in the Document; the Binding block is told apart by its root key `binding:` ([Choose a Template and Configuration](../configure/choose-a-template.md), [Metadata Block reference](../templates/metadata-reference.md)). Checked against Outline 1.12.0 in the dev stack, through the paths the editor uses:

- `collections.update` with `data` (a ProseMirror `code_block` with `language: yaml`) comes back from `collections.info` (without `x-api-version`) as a `yaml` fence with the content unchanged, nested indentation included.
- `documents.update` with `text` starting with a `yaml` fence that holds a comment, a list and nested keys comes back from `documents.info` with the fence still at the top and its content unchanged, comment and key order included.
