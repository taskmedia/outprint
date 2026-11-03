---
sidebar_position: 0
---

# The script tag

**Reference.** The one script tag that draws the PDF action, and the options it reads.

Every way to the **Formatted PDF** action in Outline's interface does the same
two things. Only the last step differs: who puts the tag into Outline's pages.
To install it, see [Add the button](../install/add-the-button.mdx).

1. `/pdf/*` on the address of Outline goes to the Gateway, with the prefix
   removed. The Gateway needs the cookie of the Outline user, so it has to be
   on the same host.
2. Outline's HTML pages carry this tag:

   ```html
   <script src="/pdf/button.js" defer></script>
   ```

That tag is the whole contract. How it gets into the pages is covered in
[Add the button](../install/add-the-button.mdx).

## The contract

| | |
|---|---|
| `src` | `/pdf/button.js`, on the same origin as Outline. The script finds the Gateway from the path of its own `src`, so a different prefix works as long as `button.js` is below it. |
| `defer` | required. The script waits for the page. |
| attributes | none. The options come from the Gateway. `data-` attributes are an override, see below. |
| once per page | a second copy does nothing: the script starts once. |
| Content-Security-Policy | `script-src` must allow `'self'`, or the page has no `script-src` and `default-src 'self'`. The styles need `style-src 'unsafe-inline'`, or they fall back to the defaults of the browser. With `'strict-dynamic'`, `'self'` is ignored and a nonce would be needed, which no way adds. |
| cookie | the script sends none by itself. The browser sends Outline's cookie to `/pdf/status/...` and `/pdf/render/...` because they are the same origin. |

## Options come from the Gateway

The button options are settings of the Gateway. Set them where you set
`OUTLINE_URL`:

| Variable | Meaning | Default |
|----------|---------|---------|
| `OUTPRINT_BUTTON_PLACEMENT` | `menu`: Export → Formatted PDF in the document menu. `button`: a header button before Share | `menu` |
| `OUTPRINT_BUTTON_LABELS` | override the texts, see [Injector and PDF action](injector.md#placement-of-the-action) | built-in |

More options exist for rare cases, such as the style of the button; they are
listed under [Advanced](../reference/settings.md#advanced) in the Reference.

The values apply to the whole installation. They are not per Collection or per
document. A wrong value stops the Gateway at startup with a message naming the
variable. The script gets them in the answer of `GET /status/<document id>` (the
`button` key, only the values that differ from the defaults), so a change needs
a restart of the Gateway and takes effect when the browser asks again, not
after a cache expires.

### `data-` attributes override

A `data-` attribute on the tag wins over the setting of the Gateway. The names
are the variable without the prefix, in lower case with dashes:
`data-placement`, `data-style`, `data-fallback`, `data-fallback-delay` (in
milliseconds), `data-metadata-block` and `data-labels` (JSON).

The [injector](injector.md) still reads the same `OUTPRINT_BUTTON_*` variables
and writes them as `data-` attributes, so an install that sets them on the
injector keeps working unchanged. New installs set them on the Gateway only.
You never need the attributes in a proxy rule or a template.

## Check that it works {#check}

Every way ends with this check:

1. The tag is in the page:

   ```sh
   curl -s -H 'Accept: text/html' https://wiki.example.test/ | grep -c 'pdf/button.js'
   ```

   `1` is right. `0` means the page was not changed (the injector or proxy is
   not in the path, the page came from a cache, or the proxy rule that adds
   the tag is missing).
2. `curl -i https://wiki.example.test/pdf/healthz` answers `200`.
3. Sign in and open a document of a Collection that has a Binding. **Export →
   Formatted PDF** is listed after the file formats (or the header button
   before **Share**, if you set the placement to `button`).
4. Click it. A PDF downloads.

If the action is not there, open the browser's developer tools (F12) and read
the **Console** and the **Network** tab:

| What you see | What it means |
|--------------|---------------|
| no request for `/pdf/button.js` | the tag is not in the page; see step 1 |
| `/pdf/button.js` is blocked and the console names a policy | Outline's Content-Security-Policy; see the table above |
| `/pdf/status/<id>` answers `401` | the cookie does not reach the Gateway; `/pdf` is not on the same host as Outline |
| `/pdf/status/<id>` answers `200` with `"available": false` | the Collection has no Binding; see [Configure](../configure/README.md) |
| the console says the anchor was not found and a floating button shows | Outline's markup changed; see [Compatibility](../install/operate/compatibility.md) |
