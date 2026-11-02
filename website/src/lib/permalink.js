/**
 * Build the version-pinned URL of a docs page.
 *
 * The newest release is served at the site root, so the location of the page
 * is unpinned there. The pinned form is `/<minor>/<path>`, which keeps showing
 * that version after the next minor ships (see website/README.md, "Versions").
 * The unreleased docs are pinned as `/dev/<path>`; before the first release
 * that path redirects to the root.
 *
 * @param {object} args
 * @param {string} args.siteUrl      site origin, e.g. https://outprint.task.media
 * @param {string} args.baseUrl      site base path, ends with a slash
 * @param {string} args.pathname     current location, including baseUrl
 * @param {string} args.hash         anchor, with or without the leading #
 * @param {string} args.versionName  Docusaurus version name ("current" or "0.2")
 * @param {boolean} args.isLast      the version is served at the site root
 */
export function buildPermalink({
  siteUrl,
  baseUrl,
  pathname,
  hash,
  versionName,
  isLast,
}) {
  const folder = versionName === 'current' ? 'dev' : versionName;
  const versionPrefix = isLast ? baseUrl : `${baseUrl}${folder}/`;
  const rest = pathname.startsWith(versionPrefix)
    ? pathname.slice(versionPrefix.length)
    : pathname.slice(baseUrl.length);
  const anchor = hash ? `#${hash.replace(/^#/, '')}` : '';
  return `${siteUrl.replace(/\/$/, '')}${baseUrl}${folder}/${rest}${anchor}`;
}

/** Copy text to the clipboard; resolves to true on success. */
export async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    // Insecure context or denied permission: fall back to a selection copy.
  }
  const field = document.createElement('textarea');
  field.value = text;
  field.setAttribute('readonly', '');
  field.style.position = 'fixed';
  field.style.opacity = '0';
  document.body.appendChild(field);
  field.select();
  try {
    return document.execCommand('copy');
  } catch {
    return false;
  } finally {
    document.body.removeChild(field);
  }
}
