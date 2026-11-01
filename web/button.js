/*
 * "Download PDF" button for Outline document pages.
 *
 * The injector adds this script to Outline's HTML. It asks the Gateway,
 * same-origin and with the user's own cookie, whether the open document has a
 * Binding, and shows a floating button only if it does. Plain ES2019, no
 * dependencies. All server text reaches the page through textContent.
 */
(function (root) {
  'use strict';

  var BUTTON_ID = 'olp-pdf-button';
  var PANEL_ID = 'olp-pdf-panel';
  var STYLE_ID = 'olp-pdf-style';
  var LOCATION_EVENT = 'olp:locationchange';
  var DEFAULT_BASE = '/pdf';
  var DEFAULT_FILENAME = 'document.pdf';
  var MAX_LISTED_ERRORS = 50;
  var ID_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_-]{0,99}$/;
  // The only origin a docs link may point at. The Gateway adds docs_url to its
  // errors (internal/docslink); a link to anywhere else is not rendered.
  var DOCS_ORIGIN = 'https://outprint.task.media';
  var DOCS_PAGE_SUFFIX = '/write/when-something-goes-wrong/';
  var UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

  var LABEL = 'Download PDF';
  var LABEL_BUSY = 'Creating PDF…';
  var ARIA_LABEL = 'Download this document as PDF';

  // ---- pure helpers (exported for the tests) -----------------------------

  // Outline document URLs look like /doc/<title-slug>-<urlId>, optionally
  // followed by /edit or /history. A bare UUID or urlId also works.
  function documentIdFromPath(pathname) {
    var match = /^\/doc\/([^/]+)/.exec(pathname || '');
    if (!match) return null;
    var slug;
    try {
      slug = decodeURIComponent(match[1]);
    } catch (e) {
      return null;
    }
    var id = UUID_PATTERN.test(slug) ? slug : slug.split('-').pop();
    return ID_PATTERN.test(id) ? id : null;
  }

  function sanitizeFilename(name, fallback) {
    var base = String(name || '').replace(/[\u0000-\u001f\u007f]/g, '').split(/[\\/]/).pop().trim();
    return base && base !== '.' && base !== '..' ? base : fallback;
  }

  // filename* (RFC 5987, UTF-8) wins over filename.
  function parseContentDisposition(header, fallback) {
    if (!header) return fallback;
    var star = /filename\*\s*=\s*([^;]+)/i.exec(header);
    if (star) {
      var parts = /^\s*([^']*)'[^']*'(.*?)\s*$/.exec(star[1]);
      if (parts && /^utf-?8$/i.test(parts[1])) {
        try {
          return sanitizeFilename(decodeURIComponent(parts[2]), fallback);
        } catch (e) {
          /* fall through to the plain filename */
        }
      }
    }
    var plain = /filename\s*=\s*(?:"((?:[^"\\]|\\.)*)"|([^;]+))/i.exec(header);
    if (!plain) return fallback;
    var value = plain[1] !== undefined ? plain[1].replace(/\\(.)/g, '$1') : plain[2].trim();
    return sanitizeFilename(value, fallback);
  }

  function isText(value) {
    return typeof value === 'string' && value !== '';
  }

  // A docs link from the server, or '' when it is not an https URL on the docs
  // origin. Only the parsed, normalised URL is ever used.
  function safeDocsUrl(value) {
    if (typeof value !== 'string' || value === '') return '';
    var url;
    try {
      url = new URL(value);
    } catch (e) {
      return '';
    }
    if (url.protocol !== 'https:' || url.origin !== DOCS_ORIGIN || url.username !== '' || url.password !== '') return '';
    return url.href;
  }

  // The start page of the docs of the version a link points into:
  // <origin>/<version>/write/when-something-goes-wrong/#code -> <origin>/<version>/.
  function docsHome(docsUrl) {
    var safe = safeDocsUrl(docsUrl);
    if (!safe) return '';
    var path = new URL(safe).pathname;
    if (path.slice(-DOCS_PAGE_SUFFIX.length) !== DOCS_PAGE_SUFFIX) return '';
    return DOCS_ORIGIN + path.slice(0, path.length - DOCS_PAGE_SUFFIX.length) + '/';
  }

  // One entry of the Gateway's {"errors":[...]} list: the message, and the
  // field, line and source as short details, and the docs link if it is safe.
  function formatErrorItem(error) {
    var e = error && typeof error === 'object' ? error : {};
    var details = [];
    if (isText(e.field)) details.push('Field: ' + e.field);
    if (typeof e.line === 'number' && e.line > 0) details.push('Line: ' + e.line);
    if (isText(e.source)) details.push('Source: ' + e.source);
    var item = { message: isText(e.message) ? e.message : 'Unknown problem.', details: details };
    var docsUrl = safeDocsUrl(e.docs_url);
    if (docsUrl) item.docsUrl = docsUrl;
    return item;
  }

  // Retry-After is a number of seconds or an HTTP date.
  function parseRetryAfter(value, nowMs) {
    if (typeof value !== 'string') return null;
    var v = value.trim();
    if (/^\d+$/.test(v)) return parseInt(v, 10);
    if (!/[A-Za-z]/.test(v)) return null;
    var at = Date.parse(v);
    if (isNaN(at)) return null;
    return Math.max(0, Math.ceil((at - nowMs) / 1000));
  }

  function plural(n, word) {
    return n + ' ' + word + (n === 1 ? '' : 's');
  }

  function busyTitle(retryAfter) {
    var seconds = parseRetryAfter(retryAfter, Date.now());
    var wait = seconds === null ? 'Try again in a moment.' : 'Retry in ' + plural(seconds, 'second') + '.';
    return 'The PDF service is busy. ' + wait;
  }

  var STATUS_TITLES = {
    0: 'Could not reach the PDF service. Check your connection and try again.',
    401: 'Your session has expired. Sign in again and retry.',
    403: 'You do not have access to this document.',
    404: 'The document was not found.',
    413: 'The document is too large to render: a size or page limit was exceeded.',
    504: 'Creating the PDF took too long and was stopped.'
  };

  // {status, retryAfter, body} of a failed render -> {title, items, omitted}.
  function describeFailure(response) {
    var status = response.status;
    var body = response.body;
    if (status === 429) return { title: busyTitle(response.retryAfter), items: [], omitted: 0 };
    var errors = body && Array.isArray(body.errors) ? body.errors : [];
    if (errors.length > 0 && status >= 400 && status < 500 && status !== 401) {
      var items = errors.slice(0, MAX_LISTED_ERRORS).map(formatErrorItem);
      return withHelp({
        title: 'The document cannot be rendered yet. Fix the following and try again.',
        items: items,
        omitted: Math.max(0, errors.length - MAX_LISTED_ERRORS)
      }, items.map(function (item) { return item.docsUrl; }));
    }
    var title = STATUS_TITLES[status];
    if (!title) {
      title = status >= 500 ? 'The PDF could not be created. Try again later.'
        : 'The PDF could not be created (status ' + status + ').';
    }
    var failure = { title: title, items: [], omitted: 0 };
    var docsUrl = body && !Array.isArray(body.errors) ? safeDocsUrl(body.docs_url) : '';
    if (docsUrl) failure.docsUrl = docsUrl;
    return withHelp(failure, [docsUrl]);
  }

  // The "Help" link of the panel: the docs home of the version of the first
  // docs link there is.
  function withHelp(failure, docsUrls) {
    for (var i = 0; i < docsUrls.length; i++) {
      var home = docsHome(docsUrls[i]);
      if (home) {
        failure.helpUrl = home;
        break;
      }
    }
    return failure;
  }

  // The script is served at <prefix>/button.js; the API lives next to it.
  function basePathFromScriptSrc(src) {
    var match = /^(?:[a-z][a-z0-9+.-]*:\/\/[^/]+)?(\/(?:[^?#]*\/)?)button\.js(?:[?#].*)?$/i.exec(src || '');
    return match ? match[1].replace(/\/$/, '') : DEFAULT_BASE;
  }

  // ---- browser part -----------------------------------------------------

  var CSS = [
    '#olp-pdf-button,#olp-pdf-panel{--olp-accent:#1d4ed8;--olp-on-accent:#fff;--olp-surface:#fff;--olp-text:#1f2933;',
    '--olp-border:#c3c9d1;--olp-danger:#b42318;--olp-focus:#1d4ed8;font:600 14px/1.4 system-ui,-apple-system,"Segoe UI",sans-serif}',
    '@media (prefers-color-scheme:dark){#olp-pdf-button,#olp-pdf-panel{--olp-accent:#93c5fd;--olp-on-accent:#0b1220;',
    '--olp-surface:#1b1f27;--olp-text:#e6e8eb;--olp-border:#3a414d;--olp-danger:#f97066;--olp-focus:#93c5fd}}',
    '#olp-pdf-button{position:fixed;right:20px;bottom:20px;z-index:900;padding:10px 18px;border:0;border-radius:8px;',
    'background:var(--olp-accent);color:var(--olp-on-accent);cursor:pointer;box-shadow:0 2px 10px rgba(0,0,0,.3)}',
    '#olp-pdf-button:hover:not(:disabled){filter:brightness(1.08)}',
    '#olp-pdf-button:focus-visible,#olp-pdf-panel button:focus-visible{outline:3px solid var(--olp-focus);outline-offset:2px}',
    '#olp-pdf-button:disabled{cursor:progress;opacity:.7}',
    '@media (prefers-reduced-motion:no-preference){#olp-pdf-button{transition:filter .15s,opacity .15s}}',
    '#olp-pdf-panel{position:fixed;right:20px;bottom:72px;z-index:900;box-sizing:border-box;width:28rem;max-width:calc(100vw - 40px);',
    'max-height:60vh;overflow:auto;padding:14px 16px;border:1px solid var(--olp-border);border-left:4px solid var(--olp-danger);',
    'border-radius:8px;background:var(--olp-surface);color:var(--olp-text);box-shadow:0 4px 18px rgba(0,0,0,.3);font-weight:400}',
    '#olp-pdf-panel .olp-head{display:flex;gap:12px;align-items:flex-start;justify-content:space-between}',
    '#olp-pdf-panel .olp-title{margin:0;font-weight:600}',
    '#olp-pdf-panel .olp-close{flex:none;border:0;background:transparent;color:inherit;font:inherit;font-size:20px;line-height:1;',
    'padding:2px 6px;cursor:pointer;border-radius:4px}',
    '#olp-pdf-panel ul{margin:10px 0 0;padding-left:20px}',
    '#olp-pdf-panel li{margin:0 0 8px}',
    '#olp-pdf-panel .olp-detail{display:block;font-size:12px;opacity:.8}',
    '#olp-pdf-panel a{color:var(--olp-accent);font-weight:600;text-underline-offset:2px}',
    '#olp-pdf-panel a:focus-visible{outline:3px solid var(--olp-focus);outline-offset:2px;border-radius:2px}',
    '#olp-pdf-panel .olp-fix{display:inline-block;margin-top:2px;font-size:13px}',
    '#olp-pdf-panel .olp-foot{margin:12px 0 0;padding-top:8px;border-top:1px solid var(--olp-border);font-size:13px}'
  ].join('');

  var state = { id: null, lastShown: null, token: 0, busy: false, base: DEFAULT_BASE };

  function installStyles() {
    if (document.getElementById(STYLE_ID)) return;
    // A constructed stylesheet is not blocked by a style-src policy; the
    // <style> element is the fallback for older browsers.
    try {
      var sheet = new CSSStyleSheet();
      sheet.replaceSync(CSS);
      document.adoptedStyleSheets = document.adoptedStyleSheets.concat([sheet]);
      var marker = document.createElement('meta');
      marker.id = STYLE_ID;
      document.head.appendChild(marker);
      return;
    } catch (e) {
      /* fall back below */
    }
    var style = document.createElement('style');
    style.id = STYLE_ID;
    style.textContent = CSS;
    document.head.appendChild(style);
  }

  function removeById(id) {
    var el = document.getElementById(id);
    if (el) el.remove();
  }

  function setBusy(button, busy) {
    state.busy = busy;
    button.disabled = busy;
    button.setAttribute('aria-busy', busy ? 'true' : 'false');
    button.textContent = busy ? LABEL_BUSY : LABEL;
  }

  function createButton(id) {
    var button = document.createElement('button');
    button.id = BUTTON_ID;
    button.type = 'button';
    button.textContent = LABEL;
    button.setAttribute('aria-label', ARIA_LABEL);
    button.addEventListener('click', function () { download(button, id); });
    return button;
  }

  function showButton(id) {
    installStyles();
    removeById(BUTTON_ID);
    document.body.appendChild(createButton(id));
  }

  function hideButton() {
    removeById(BUTTON_ID);
    dismissPanel();
  }

  function dismissPanel() {
    document.removeEventListener('keydown', onPanelKey, true);
    removeById(PANEL_ID);
  }

  function onPanelKey(event) {
    if (event.key !== 'Escape') return;
    dismissPanel();
    var button = document.getElementById(BUTTON_ID);
    if (button) button.focus();
  }

  function element(tag, className, text) {
    var el = document.createElement(tag);
    if (className) el.className = className;
    if (text !== undefined) el.textContent = text;
    return el;
  }

  // A link to the docs. href comes only from safeDocsUrl, never from raw
  // server text.
  function docsLink(href, text, className) {
    var a = element('a', className, text);
    a.href = href;
    a.target = '_blank';
    a.rel = 'noopener noreferrer';
    if (text !== 'Help') a.setAttribute('aria-label', text + ' (opens in a new tab)');
    return a;
  }

  function buildList(failure) {
    var list = element('ul');
    failure.items.forEach(function (item) {
      var li = element('li', '', item.message);
      item.details.forEach(function (detail) { li.appendChild(element('span', 'olp-detail', detail)); });
      if (item.docsUrl) li.appendChild(docsLink(item.docsUrl, 'How to fix', 'olp-fix'));
      list.appendChild(li);
    });
    if (failure.omitted > 0) list.appendChild(element('li', '', '…and ' + failure.omitted + ' more.'));
    return list;
  }

  function showFailure(failure) {
    dismissPanel();
    installStyles();
    var panel = element('section');
    panel.id = PANEL_ID;
    panel.setAttribute('role', 'alert');
    var head = element('div', 'olp-head');
    head.appendChild(element('p', 'olp-title', failure.title));
    var close = element('button', 'olp-close', '×');
    close.type = 'button';
    close.setAttribute('aria-label', 'Dismiss');
    close.addEventListener('click', function () {
      dismissPanel();
      var button = document.getElementById(BUTTON_ID);
      if (button) button.focus();
    });
    head.appendChild(close);
    panel.appendChild(head);
    if (failure.docsUrl) panel.appendChild(docsLink(failure.docsUrl, 'How to fix', 'olp-fix'));
    if (failure.items.length > 0) panel.appendChild(buildList(failure));
    if (failure.helpUrl) {
      var foot = element('p', 'olp-foot');
      foot.appendChild(docsLink(failure.helpUrl, 'Help'));
      panel.appendChild(foot);
    }
    document.body.appendChild(panel);
    document.addEventListener('keydown', onPanelKey, true);
  }

  function saveBlob(blob, filename) {
    var url = URL.createObjectURL(blob);
    var link = document.createElement('a');
    link.href = url;
    link.download = filename;
    link.rel = 'noopener';
    link.style.display = 'none';
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(function () { URL.revokeObjectURL(url); }, 10000);
  }

  function readJSON(response) {
    return response.text().then(function (text) {
      try {
        return JSON.parse(text);
      } catch (e) {
        return null;
      }
    }, function () { return null; });
  }

  function isPDF(response) {
    return /^application\/pdf\b/i.test(response.headers.get('Content-Type') || '');
  }

  function download(button, id) {
    if (state.busy) return;
    dismissPanel();
    setBusy(button, true);
    fetch(state.base + '/render/' + encodeURIComponent(id), { credentials: 'same-origin', cache: 'no-store' })
      .then(function (response) {
        if (response.status === 200 && isPDF(response)) {
          var name = parseContentDisposition(response.headers.get('Content-Disposition'), DEFAULT_FILENAME);
          return response.blob().then(function (blob) { saveBlob(blob, name); });
        }
        // A 200 that is not a PDF is a proxy or login page in the way.
        var status = response.status === 200 ? 502 : response.status;
        return readJSON(response).then(function (body) {
          report(id, { status: status, retryAfter: response.headers.get('Retry-After'), body: body });
        });
      })
      .catch(function () { report(id, { status: 0, retryAfter: null, body: null }); })
      .then(function () { setBusy(button, false); });
  }

  function report(id, response) {
    if (state.id === id) showFailure(describeFailure(response));
  }

  function checkStatus(id, token) {
    fetch(state.base + '/status/' + encodeURIComponent(id), {
      credentials: 'same-origin',
      cache: 'no-store',
      headers: { Accept: 'application/json' }
    })
      .then(function (response) { return response.ok ? response.json() : null; })
      .catch(function () { return null; })
      .then(function (body) {
        if (token !== state.token) return;
        if (body && body.available === true) showButton(id);
        else hideButton();
      });
  }

  // Outline is a single-page app: act when the document in the URL changes,
  // and look again when the tab comes back, since an admin may have changed
  // the Binding meanwhile.
  function sync(force) {
    var id = documentIdFromPath(root.location.pathname);
    if (id === state.id && !force) return;
    state.id = id;
    state.token += 1;
    if (!id) {
      hideButton();
      return;
    }
    if (id !== state.lastShown) hideButton();
    state.lastShown = id;
    checkStatus(id, state.token);
  }

  function patchHistory() {
    ['pushState', 'replaceState'].forEach(function (name) {
      var original = root.history[name];
      root.history[name] = function () {
        var result = original.apply(this, arguments);
        root.dispatchEvent(new Event(LOCATION_EVENT));
        return result;
      };
    });
  }

  function start() {
    if (root.__olpPdfButton) return;
    root.__olpPdfButton = true;
    var script = document.currentScript;
    state.base = basePathFromScriptSrc(script && script.src);
    patchHistory();
    var onRoute = function () { sync(false); };
    root.addEventListener('popstate', onRoute);
    root.addEventListener('hashchange', onRoute);
    root.addEventListener(LOCATION_EVENT, onRoute);
    document.addEventListener('visibilitychange', function () {
      if (document.visibilityState === 'visible') sync(true);
    });
    sync(false);
  }

  if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
      documentIdFromPath: documentIdFromPath,
      parseContentDisposition: parseContentDisposition,
      sanitizeFilename: sanitizeFilename,
      formatErrorItem: formatErrorItem,
      parseRetryAfter: parseRetryAfter,
      describeFailure: describeFailure,
      basePathFromScriptSrc: basePathFromScriptSrc,
      safeDocsUrl: safeDocsUrl,
      docsHome: docsHome,
      DOCS_ORIGIN: DOCS_ORIGIN
    };
  }

  if (typeof document !== 'undefined' && root.location) {
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
    else start();
  }
})(typeof window !== 'undefined' ? window : this);
