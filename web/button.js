/*
 * "Formatted PDF" action for Outline document pages.
 *
 * The injector adds this script to Outline's HTML. It asks the Gateway,
 * same-origin and with the user's own cookie, whether the open document has a
 * Binding, and only then places the action: as an item of the Export submenu
 * of the document menu (default), or as a header button before Share. When the
 * anchor cannot be found it falls back to a small floating button. The
 * operator chooses through data attributes the injector writes on the script
 * tag. Plain ES2019, no dependencies. All server text reaches the page through
 * textContent.
 */
(function (root) {
  'use strict';

  // Only set while the script runs for the first time, before DOMContentLoaded.
  var bootScript = typeof document !== 'undefined' ? document.currentScript : null;

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

  var ITEM_ATTR = 'data-olp-item';
  var HEADER_WRAP_ATTR = 'data-olp-header';
  var DEFAULT_FALLBACK_DELAY_MS = 3000;
  var MAX_FALLBACK_DELAY_MS = 60000;
  var MAX_LABEL_LENGTH = 80;
  var PANEL_GAP = 8;
  // The document header is the topmost bar; a trigger further down is not its.
  var HEADER_MAX_TOP = 100;
  var ACTION_ROW_DEPTH = 3;
  // The "…" button ends at the right edge of the header, within this distance.
  var HEADER_RIGHT_EDGE = 120;
  var PLACEMENTS = ['menu', 'button'];
  var STYLES = ['icon-label', 'icon'];
  // The items of Outline's Export submenu carry the file type as text that is
  // not translated (Markdown.md, HTML.html, ...); the labels are.
  var EXPORT_ITEM_PATTERN = /\.(md|html|textpack|pdf)$/i;

  // Built-in texts. 'busy' and 'dismiss' are not configurable.
  var BUILTIN_TEXTS = {
    en: { menu: 'Formatted PDF', button: 'PDF', tooltip: 'Download formatted PDF', busy: 'Creating PDF…', dismiss: 'Dismiss' },
    de: { menu: 'Formatiertes PDF', button: 'PDF', tooltip: 'Formatiertes PDF herunterladen', busy: 'PDF wird erstellt…', dismiss: 'Schließen' }
  };
  var FALLBACK_LANGUAGE = 'en';

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

  // "de_DE" and "de-AT" are German: only the primary subtag counts.
  function primaryLanguage(tag) {
    var match = /^([a-z]{2,3})(?:[-_]|$)/i.exec(typeof tag === 'string' ? tag.trim() : '');
    return match ? match[1].toLowerCase() : '';
  }

  // The language of the labels: the user's Outline language when the Gateway
  // reports one, otherwise the browser's. A language there is no text for
  // (built in or configured) means English.
  function resolveLanguage(outlineLanguage, browserLanguage, extraLanguages) {
    var known = Object.keys(BUILTIN_TEXTS).concat(extraLanguages || []);
    var wanted = primaryLanguage(outlineLanguage) || primaryLanguage(browserLanguage);
    return wanted && known.indexOf(wanted) !== -1 ? wanted : FALLBACK_LANGUAGE;
  }

  function cleanLabels(raw) {
    var out = {};
    if (!raw || typeof raw !== 'object') return out;
    ['menu', 'button', 'tooltip'].forEach(function (key) {
      var byLanguage = raw[key];
      if (!byLanguage || typeof byLanguage !== 'object') return;
      Object.keys(byLanguage).forEach(function (lang) {
        var text = byLanguage[lang];
        if (!/^[a-z]{2,3}$/.test(lang) || typeof text !== 'string') return;
        text = text.trim();
        if (text === '' || text.length > MAX_LABEL_LENGTH) return;
        out[key] = out[key] || {};
        out[key][lang] = text;
      });
    });
    return out;
  }

  function oneOf(value, allowed, fallback) {
    return allowed.indexOf(value) !== -1 ? value : fallback;
  }

  // The data attributes of the script tag -> the options. get(name) returns
  // the attribute value or null. Anything unusable falls back to the default.
  function parseOptions(get) {
    var labels = {};
    try {
      labels = cleanLabels(JSON.parse(get('data-labels') || '{}'));
    } catch (e) {
      /* the injector validates the labels; ignore a damaged value */
    }
    var delay = parseInt(get('data-fallback-delay'), 10);
    return {
      placement: oneOf(get('data-placement'), PLACEMENTS, 'menu'),
      style: oneOf(get('data-style'), STYLES, 'icon-label'),
      fallback: get('data-fallback') !== 'off',
      fallbackDelay: delay >= 0 ? Math.min(delay, MAX_FALLBACK_DELAY_MS) : DEFAULT_FALLBACK_DELAY_MS,
      labels: labels
    };
  }

  // The texts for one language: a configured text for the language, the
  // built-in text for it, a configured English text, the built-in English one.
  function textsFor(language, labels) {
    var texts = {};
    var builtin = BUILTIN_TEXTS[language] || {};
    Object.keys(BUILTIN_TEXTS[FALLBACK_LANGUAGE]).forEach(function (key) {
      var configured = (labels && labels[key]) || {};
      texts[key] = configured[language] || builtin[key] || configured[FALLBACK_LANGUAGE] || BUILTIN_TEXTS[FALLBACK_LANGUAGE][key];
    });
    return texts;
  }

  // The visible text of an Export item without its file type: for a menu item
  // "Markdown.md" this is true, for "Print" and "Copy link" it is not.
  function isExportItemText(text) {
    return EXPORT_ITEM_PATTERN.test(String(text || '').trim());
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
    '#olp-pdf-button{position:fixed;right:16px;bottom:16px;z-index:900;padding:6px 12px;border:0;border-radius:6px;font-size:13px;',
    'background:var(--olp-accent);color:var(--olp-on-accent);cursor:pointer;box-shadow:0 1px 6px rgba(0,0,0,.25)}',
    '#olp-pdf-button:hover:not(:disabled){filter:brightness(1.08)}',
    '#olp-pdf-button:focus-visible,#olp-pdf-panel button:focus-visible,.olp-header-button:focus-visible{outline:3px solid var(--olp-focus,#1d4ed8);outline-offset:2px}',
    '#olp-pdf-button:disabled,.olp-header-button:disabled{cursor:progress;opacity:.7}',
    '.olp-header-button{display:inline-flex;align-items:center;gap:6px;cursor:pointer}',
    '.olp-header-button svg,.olp-icon{flex:none;width:18px;height:18px}',
    '@media (prefers-reduced-motion:no-preference){#olp-pdf-button{transition:filter .15s,opacity .15s}}',
    '#olp-pdf-panel{position:fixed;right:20px;bottom:56px;z-index:900;box-sizing:border-box;width:28rem;max-width:calc(100vw - 40px);',
    'max-height:60vh;overflow:auto;padding:14px 16px;border:1px solid var(--olp-border);border-left:4px solid var(--olp-danger);',
    'border-radius:8px;background:var(--olp-surface);color:var(--olp-text);box-shadow:0 4px 18px rgba(0,0,0,.3);font-weight:400}',
    '#olp-pdf-panel[data-kind="busy"]{border-left-color:var(--olp-accent);width:auto;max-width:calc(100vw - 40px)}',
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

  var state = {
    id: null,
    lastShown: null,
    token: 0,
    busy: false,
    busyId: null,
    base: DEFAULT_BASE,
    options: parseOptions(function () { return null; }),
    language: FALLBACK_LANGUAGE,
    texts: textsFor(FALLBACK_LANGUAGE, {}),
    available: false,
    anchored: false,
    observer: null,
    fallbackTimer: null,
    placePending: false,
    refocus: null,
    panelAnchor: null
  };

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

  function element(tag, className, text) {
    var el = document.createElement(tag);
    if (className) el.className = className;
    if (text !== undefined) el.textContent = text;
    return el;
  }

  var SVG_NS = 'http://www.w3.org/2000/svg';

  // A page with a folded corner and a down arrow.
  function createIcon(size) {
    var svg = document.createElementNS(SVG_NS, 'svg');
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('width', size || '18');
    svg.setAttribute('height', size || '18');
    svg.setAttribute('fill', 'none');
    svg.setAttribute('stroke', 'currentColor');
    svg.setAttribute('stroke-width', '2');
    svg.setAttribute('stroke-linecap', 'round');
    svg.setAttribute('stroke-linejoin', 'round');
    svg.setAttribute('aria-hidden', 'true');
    svg.setAttribute('focusable', 'false');
    svg.setAttribute('class', 'olp-icon');
    ['M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z', 'M14 3v5h5', 'M12 11v6', 'M9.5 14.5 12 17l2.5-2.5'].forEach(function (d) {
      var path = document.createElementNS(SVG_NS, 'path');
      path.setAttribute('d', d);
      svg.appendChild(path);
    });
    return svg;
  }

  // ---- finding Outline's anchors -----------------------------------------
  // Outline's DOM has no stable classes or test ids and its texts are
  // translated, so everything here goes by role, aria-haspopup and structure.

  function isInsideMenu(el) {
    return !!el.closest('[role="menu"]');
  }

  // The "…" button of the document header: of the menu triggers at the top of
  // the page, the right-most. The sidebar has triggers of its own (the team
  // and user menus, the document tree); none of them sits at the right edge
  // of the header.
  function findOptionsButton() {
    var triggers = document.querySelectorAll('button[aria-haspopup="menu"]');
    var best = null;
    var bestRight = -1;
    for (var i = 0; i < triggers.length; i++) {
      var el = triggers[i];
      if (isInsideMenu(el) || el.hasAttribute(ITEM_ATTR)) continue;
      var rect = el.getBoundingClientRect();
      if (rect.width === 0 || rect.height === 0 || rect.top >= HEADER_MAX_TOP || rect.right < root.innerWidth - HEADER_RIGHT_EDGE) continue;
      if (rect.right > bestRight) {
        best = el;
        bestRight = rect.right;
      }
    }
    return best;
  }

  // The header's action row: the nearest ancestor of the "…" button that also
  // holds a dialog trigger (Share, the collaborators), looking at most
  // ACTION_ROW_DEPTH levels up. Without one (a reader on a small screen) the
  // row is the button's grandparent: button, its wrapper, the row.
  function findActionRow(options) {
    var el = options.parentElement;
    for (var depth = 0; el && depth < ACTION_ROW_DEPTH; depth++, el = el.parentElement) {
      if (el.querySelector('button[aria-haspopup="dialog"]')) return el;
    }
    var wrapper = options.parentElement;
    return wrapper && wrapper.parentElement !== document.body ? wrapper.parentElement : null;
  }

  // Share opens a dialog and has a text label; the collaborators' avatars
  // open one too but contain an element with an aria-label.
  function findShareButton(row) {
    var dialogs = row.querySelectorAll('button[aria-haspopup="dialog"]');
    for (var i = dialogs.length - 1; i >= 0; i--) {
      if (!dialogs[i].hasAttribute('aria-label') && !dialogs[i].querySelector('[aria-label]')) return dialogs[i];
    }
    return null;
  }

  // The child of row that holds el.
  function rowChild(row, el) {
    var node = el;
    while (node && node.parentElement !== row) node = node.parentElement;
    return node;
  }

  // An open menu whose items are file exports is the Export submenu.
  function findExportMenus() {
    var found = [];
    document.querySelectorAll('[role="menu"]').forEach(function (menu) {
      var items = menu.querySelectorAll('[role="menuitem"]');
      for (var i = 0; i < items.length; i++) {
        if (isExportItemText(items[i].textContent)) {
          found.push(menu);
          return;
        }
      }
    });
    return found;
  }

  // ---- placing the action ------------------------------------------------

  function setBusy(busy) {
    state.busy = busy;
    // A disabled button loses the focus; give it back when it is usable again.
    var refocus = busy ? document.activeElement : state.refocus;
    state.refocus = busy ? refocus : null;
    var button = document.getElementById(BUTTON_ID);
    if (button) setButtonBusy(button, busy);
    document.querySelectorAll('.olp-header-button').forEach(function (b) { setButtonBusy(b, busy); });
    if (!busy) {
      var panel = document.getElementById(PANEL_ID);
      if (panel && panel.getAttribute('data-kind') === 'busy') dismissPanel();
      if (refocus && refocus.isConnected && refocus.getAttribute && refocus.hasAttribute(ITEM_ATTR) && refocus.focus) refocus.focus();
    }
  }

  function setButtonBusy(button, busy) {
    button.disabled = busy;
    button.setAttribute('aria-busy', busy ? 'true' : 'false');
    var text = busy ? state.texts.busy : state.texts.button;
    if (button.id === BUTTON_ID) {
      button.textContent = text;
      return;
    }
    var label = button.querySelector('.olp-label');
    if (label) label.textContent = text;
  }

  function startDownload(anchor) {
    if (!state.id || (state.busy && state.busyId === state.id)) return;
    state.panelAnchor = anchor;
    download(state.id);
  }

  function createFloatingButton() {
    var button = element('button', '', state.texts.button);
    button.id = BUTTON_ID;
    button.type = 'button';
    button.setAttribute('aria-label', state.texts.tooltip);
    button.title = state.texts.tooltip;
    button.addEventListener('click', function () { startDownload(button); });
    return button;
  }

  function showFloatingButton() {
    installStyles();
    removeById(BUTTON_ID);
    document.body.appendChild(createFloatingButton());
  }

  // The header button looks like its neighbour: it takes over the
  // neighbour's classes and variant and builds its own content.
  function createHeaderButton(neighbour) {
    var button = neighbour.cloneNode(false);
    ['id', 'aria-haspopup', 'aria-expanded', 'aria-controls', 'aria-label', 'data-state', 'width', 'height'].forEach(function (name) {
      button.removeAttribute(name);
    });
    button.className = neighbour.className + ' olp-header-button';
    button.type = 'button';
    button.setAttribute(ITEM_ATTR, 'header');
    button.setAttribute('aria-label', state.texts.tooltip);
    button.title = state.texts.tooltip;
    button.appendChild(createIcon());
    if (state.options.style === 'icon-label') button.appendChild(element('span', 'olp-label', state.texts.button));
    button.addEventListener('click', function () { startDownload(button); });
    return button;
  }

  // Makes sure the header button sits before Share, or before "…" when there
  // is no Share (no edit permission, narrow screens). Reports whether the
  // anchor exists.
  function ensureHeaderButton() {
    var options = findOptionsButton();
    var row = options && findActionRow(options);
    if (!row) return false;
    var share = findShareButton(row);
    var before = rowChild(row, share || options);
    var existing = row.querySelector('[' + HEADER_WRAP_ATTR + ']');
    if (existing && existing.nextElementSibling === before) return true;
    document.querySelectorAll('[' + HEADER_WRAP_ATTR + ']').forEach(function (el) { el.remove(); });
    var wrap = before.cloneNode(false);
    wrap.removeAttribute('id');
    wrap.setAttribute(HEADER_WRAP_ATTR, '');
    wrap.appendChild(createHeaderButton(share || options));
    row.insertBefore(wrap, before);
    installStyles();
    return true;
  }

  function createMenuItem(template) {
    var item = template.cloneNode(true);
    ['id', 'data-highlighted', 'data-state', 'data-radix-collection-item', 'href', 'aria-haspopup', 'aria-expanded'].forEach(function (name) {
      item.removeAttribute(name);
    });
    item.setAttribute(ITEM_ATTR, 'menu');
    item.setAttribute('role', 'menuitem');
    item.setAttribute('tabindex', '-1');
    // Items of a root menu start with an icon; the Export items do not. The
    // label is the first element that is not the icon.
    var icon = item.querySelector(':scope > [aria-hidden="true"]');
    var label = icon ? icon.nextElementSibling : item.firstElementChild;
    if (label) label.textContent = state.texts.menu;
    else item.textContent = state.texts.menu;
    if (icon) {
      var old = icon.querySelector('svg');
      icon.replaceChildren(createIcon((old && old.getAttribute('width')) || '24'));
    }
    // Everything after the label (shortcuts, file types) does not apply.
    while (label && label.nextElementSibling) label.nextElementSibling.remove();
    item.addEventListener('pointermove', function () { if (document.activeElement !== item) item.focus({ preventScroll: true }); });
    item.addEventListener('focus', function () { item.setAttribute('data-highlighted', ''); });
    item.addEventListener('blur', function () { item.removeAttribute('data-highlighted'); });
    item.addEventListener('click', function (event) {
      event.preventDefault();
      selectMenuItem(item);
    });
    return item;
  }

  // Closes the menu the way Radix does on Escape and starts the download
  // anchored at the "…" button. Radix hands the focus back to that button as
  // the menu unmounts; doing the same afterwards makes sure of it.
  function selectMenuItem(item) {
    var menu = item.closest('[role="menu"]');
    var options = findOptionsButton();
    if (menu) menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }));
    root.setTimeout(function () {
      if (options && options.isConnected && !document.querySelector('[role="menu"]')) options.focus();
    }, 0);
    startDownload(options);
  }

  // Radix moves between the items it knows; ours is not one of them. Steer
  // the arrow keys across it.
  function onMenuKey(event) {
    if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return;
    var items = Array.prototype.slice.call(event.currentTarget.querySelectorAll('[role="menuitem"]'));
    var from = items.indexOf(document.activeElement);
    if (from === -1) return;
    var step = event.key === 'ArrowDown' ? 1 : -1;
    var target = items[from + step];
    var involvesOurs = document.activeElement.hasAttribute(ITEM_ATTR) || (target && target.hasAttribute(ITEM_ATTR));
    if (!involvesOurs) return;
    event.preventDefault();
    event.stopPropagation();
    if (target) target.focus();
  }

  function ensureMenuItems() {
    findExportMenus().forEach(function (menu) {
      if (menu.querySelector('[' + ITEM_ATTR + ']')) return;
      var items = menu.querySelectorAll('[role="menuitem"]');
      var last = null;
      for (var i = 0; i < items.length; i++) {
        if (isExportItemText(items[i].textContent)) last = items[i];
      }
      if (!last) return;
      last.parentNode.insertBefore(createMenuItem(last), last.nextSibling);
      menu.addEventListener('keydown', onMenuKey, true);
    });
  }

  // Looks for the anchor of the chosen placement and keeps it in place.
  function place() {
    if (!state.available) return;
    var anchored;
    if (state.options.placement === 'button') {
      anchored = ensureHeaderButton();
    } else {
      anchored = !!findOptionsButton();
      if (anchored) ensureMenuItems();
    }
    if (anchored) {
      state.anchored = true;
      removeById(BUTTON_ID);
    }
  }

  // The editor changes the page on every keystroke and place() reads layout:
  // look once per frame at most.
  function schedulePlace() {
    if (state.placePending) return;
    state.placePending = true;
    var run = function () {
      state.placePending = false;
      place();
    };
    if (typeof root.requestAnimationFrame === 'function') root.requestAnimationFrame(run);
    else root.setTimeout(run, 16);
  }

  function startPlacing() {
    stopPlacing();
    // Radix mounts the menu when it opens and removes it when it closes, and
    // React may redraw the header: look again whenever the page changes.
    state.observer = new MutationObserver(schedulePlace);
    state.observer.observe(document.body, { childList: true, subtree: true });
    place();
    if (!state.options.fallback || state.anchored) return;
    state.fallbackTimer = setTimeout(function () {
      state.fallbackTimer = null;
      if (state.anchored || !state.available) return;
      if (root.console && root.console.warn) {
        root.console.warn('Outprint: the "' + state.options.placement + '" anchor was not found in Outline; showing the floating PDF button.');
      }
      showFloatingButton();
    }, state.options.fallbackDelay);
  }

  function stopPlacing() {
    if (state.observer) state.observer.disconnect();
    state.observer = null;
    clearTimeout(state.fallbackTimer);
    state.fallbackTimer = null;
    state.anchored = false;
  }

  function removeInjected() {
    stopPlacing();
    removeById(BUTTON_ID);
    document.querySelectorAll('[' + HEADER_WRAP_ATTR + '],[' + ITEM_ATTR + ']').forEach(function (el) { el.remove(); });
    dismissPanel();
  }

  // ---- feedback panel ----------------------------------------------------

  function dismissPanel() {
    document.removeEventListener('keydown', onPanelKey, true);
    root.removeEventListener('resize', positionPanel);
    root.removeEventListener('scroll', positionPanel, true);
    removeById(PANEL_ID);
  }

  function returnFocus() {
    var target = state.panelAnchor;
    if (!target || !target.isConnected) target = document.getElementById(BUTTON_ID) || document.querySelector('.olp-header-button') || findOptionsButton();
    if (target && target.focus) target.focus();
  }

  function onPanelKey(event) {
    if (event.key !== 'Escape') return;
    // Escape belongs to an open Outline menu or dialog first.
    if (document.querySelector('[role="menu"], [role="dialog"]')) return;
    dismissPanel();
    returnFocus();
  }

  // Under the anchor, right-aligned with it; bottom right of the window when
  // there is no anchor on the page (the floating button).
  function positionPanel() {
    var panel = document.getElementById(PANEL_ID);
    var anchor = state.panelAnchor;
    if (!panel) return;
    if (!anchor || !anchor.isConnected || anchor.id === BUTTON_ID) {
      panel.style.top = '';
      panel.style.right = '';
      return;
    }
    var rect = anchor.getBoundingClientRect();
    panel.style.bottom = 'auto';
    panel.style.top = Math.round(rect.bottom + PANEL_GAP) + 'px';
    panel.style.right = Math.max(PANEL_GAP, Math.round(root.innerWidth - rect.right)) + 'px';
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

  function mountPanel(panel) {
    document.body.appendChild(panel);
    positionPanel();
    root.addEventListener('resize', positionPanel);
    root.addEventListener('scroll', positionPanel, true);
  }

  // Menu placement has no control that can show the busy state: the menu has
  // closed. A small status panel stands in.
  function showBusyPanel() {
    dismissPanel();
    installStyles();
    var panel = element('section');
    panel.id = PANEL_ID;
    panel.setAttribute('data-kind', 'busy');
    panel.setAttribute('role', 'status');
    panel.appendChild(element('p', 'olp-title', state.texts.busy));
    mountPanel(panel);
  }

  function showFailure(failure) {
    dismissPanel();
    installStyles();
    var panel = element('section');
    panel.id = PANEL_ID;
    panel.setAttribute('data-kind', 'error');
    panel.setAttribute('role', 'alert');
    var head = element('div', 'olp-head');
    head.appendChild(element('p', 'olp-title', failure.title));
    var close = element('button', 'olp-close', '×');
    close.type = 'button';
    close.setAttribute('aria-label', state.texts.dismiss);
    close.addEventListener('click', function () {
      dismissPanel();
      returnFocus();
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
    mountPanel(panel);
    document.addEventListener('keydown', onPanelKey, true);
  }

  // ---- download ----------------------------------------------------------

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

  function download(id) {
    dismissPanel();
    state.busyId = id;
    setBusy(true);
    if (!document.getElementById(BUTTON_ID) && !document.querySelector('.olp-header-button')) showBusyPanel();
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
      // A render of another document may have started since: leave its state alone.
      .then(function () { if (state.busyId === id) setBusy(false); });
  }

  function report(id, response) {
    if (state.id !== id) return;
    // Dismiss the busy panel first: setBusy(false) would remove the error.
    dismissPanel();
    showFailure(describeFailure(response));
  }

  // ---- availability and routing -------------------------------------------

  function show(body) {
    state.language = resolveLanguage(body.language, root.navigator && root.navigator.language, Object.keys(languagesOf(state.options.labels)));
    state.texts = textsFor(state.language, state.options.labels);
    removeInjected();
    state.available = true;
    startPlacing();
  }

  function languagesOf(labels) {
    var langs = {};
    Object.keys(labels).forEach(function (key) {
      Object.keys(labels[key]).forEach(function (lang) { langs[lang] = true; });
    });
    return langs;
  }

  function hide() {
    state.available = false;
    removeInjected();
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
        if (body && body.available === true) show(body);
        else hide();
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
      hide();
      return;
    }
    if (id !== state.lastShown) hide();
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
    var script = bootScript || document.querySelector('script[data-outline-pdf-injector]');
    state.base = basePathFromScriptSrc(script && script.src);
    state.options = parseOptions(function (name) { return script ? script.getAttribute(name) : null; });
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
      parseOptions: parseOptions,
      resolveLanguage: resolveLanguage,
      textsFor: textsFor,
      isExportItemText: isExportItemText,
      DOCS_ORIGIN: DOCS_ORIGIN
    };
  }

  if (typeof document !== 'undefined' && root.location) {
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', start);
    else start();
  }
})(typeof window !== 'undefined' ? window : this);
