'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const b = require('./button.js');

test('documentIdFromPath takes the urlId at the end of the slug', () => {
  assert.equal(b.documentIdFromPath('/doc/spring-plant-sale-aB3dE5fGh1'), 'aB3dE5fGh1');
  assert.equal(b.documentIdFromPath('/doc/untitled-aB3dE5fGh1/edit'), 'aB3dE5fGh1');
  assert.equal(b.documentIdFromPath('/doc/spring-plant-sale-aB3dE5fGh1/history/'), 'aB3dE5fGh1');
  assert.equal(b.documentIdFromPath('/doc/aB3dE5fGh1'), 'aB3dE5fGh1');
});

test('documentIdFromPath keeps a UUID whole', () => {
  const uuid = '123e4567-e89b-12d3-a456-426614174000';
  assert.equal(b.documentIdFromPath('/doc/' + uuid), uuid);
});

test('documentIdFromPath decodes the slug', () => {
  assert.equal(b.documentIdFromPath('/doc/Gr%C3%BC%C3%9Fe-aB3dE5fGh1'), 'aB3dE5fGh1');
});

test('documentIdFromPath ignores other pages and unsafe ids', () => {
  for (const p of ['/', '/home', '/doc', '/doc/', '/collection/abc-123', '/search/doc/x-abc', '/doc/title-', '/doc/%E0%A4%A', '/doc/a-%2F..%2Fb']) {
    assert.equal(b.documentIdFromPath(p), null, p);
  }
});

test('parseContentDisposition prefers filename* and decodes UTF-8', () => {
  const header = 'attachment; filename="Gruesse aus Koeln.pdf"; filename*=UTF-8\'\'Gr%C3%BC%C3%9Fe%20aus%20K%C3%B6ln.pdf';
  assert.equal(b.parseContentDisposition(header, 'x.pdf'), 'Grüße aus Köln.pdf');
});

test('parseContentDisposition falls back to filename, then the default', () => {
  assert.equal(b.parseContentDisposition('attachment; filename="Report 1.pdf"', 'x.pdf'), 'Report 1.pdf');
  assert.equal(b.parseContentDisposition('attachment; filename=plain.pdf', 'x.pdf'), 'plain.pdf');
  assert.equal(b.parseContentDisposition('attachment; filename="a \\"q\\" b.pdf"', 'x.pdf'), 'a "q" b.pdf');
  assert.equal(b.parseContentDisposition('attachment', 'x.pdf'), 'x.pdf');
  assert.equal(b.parseContentDisposition('', 'x.pdf'), 'x.pdf');
  assert.equal(b.parseContentDisposition(null, 'x.pdf'), 'x.pdf');
});

test('parseContentDisposition survives a broken filename*', () => {
  assert.equal(b.parseContentDisposition('attachment; filename="ok.pdf"; filename*=UTF-8\'\'%E0%A4%A', 'x.pdf'), 'ok.pdf');
});

test('parseContentDisposition never returns a path', () => {
  assert.equal(b.parseContentDisposition('attachment; filename="../../etc/passwd"', 'x.pdf'), 'passwd');
  assert.equal(b.parseContentDisposition('attachment; filename="a\\\\b.pdf"', 'x.pdf'), 'b.pdf');
  assert.equal(b.parseContentDisposition('attachment; filename="/"', 'x.pdf'), 'x.pdf');
});

test('formatErrorItem lists field, line and source', () => {
  const item = b.formatErrorItem({ field: 'recipient', source: 'document', code: 'missing', message: 'is required', line: 3 });
  assert.equal(item.message, 'is required');
  assert.deepEqual(item.details, ['Field: recipient', 'Line: 3', 'Source: document']);
});

test('formatErrorItem copes with missing and odd values', () => {
  assert.deepEqual(b.formatErrorItem({ message: 'x' }), { message: 'x', details: [] });
  assert.deepEqual(b.formatErrorItem({ message: 'x', line: 0, field: '' }), { message: 'x', details: [] });
  assert.equal(b.formatErrorItem({}).message, 'Unknown problem.');
  assert.equal(b.formatErrorItem(null).message, 'Unknown problem.');
  assert.equal(b.formatErrorItem('text').message, 'Unknown problem.');
  assert.equal(b.formatErrorItem({ message: { a: 1 }, field: 5 }).message, 'Unknown problem.');
});

test('parseRetryAfter reads seconds and dates', () => {
  assert.equal(b.parseRetryAfter('7', 0), 7);
  assert.equal(b.parseRetryAfter(' 12 ', 0), 12);
  assert.equal(b.parseRetryAfter('Thu, 01 Jan 1970 00:00:30 GMT', 0), 30);
  assert.equal(b.parseRetryAfter('Thu, 01 Jan 1970 00:00:30 GMT', 40000), 0);
  assert.equal(b.parseRetryAfter('soon', 0), null);
  assert.equal(b.parseRetryAfter(null, 0), null);
  assert.equal(b.parseRetryAfter('-3', 0), null);
});

test('describeFailure lists validation errors', () => {
  const f = b.describeFailure({ status: 422, retryAfter: null, body: { errors: [{ message: 'one', line: 2 }, { message: 'two' }] } });
  assert.match(f.title, /cannot be rendered/i);
  assert.equal(f.items.length, 2);
  assert.equal(f.items[0].details[0], 'Line: 2');
});

test('describeFailure tells the user to retry on 429', () => {
  const f = b.describeFailure({ status: 429, retryAfter: '5', body: { errors: [{ source: 'service', code: 'busy', message: 'busy' }] } });
  assert.match(f.title, /busy/i);
  assert.match(f.title, /5 seconds/);
  assert.deepEqual(f.items, []);
  assert.match(b.describeFailure({ status: 429, retryAfter: '1', body: null }).title, /1 second\b/);
  assert.doesNotMatch(b.describeFailure({ status: 429, retryAfter: null, body: null }).title, /NaN|null/);
});

test('describeFailure has a clear message per status', () => {
  const title = (status, body) => b.describeFailure({ status, retryAfter: null, body: body === undefined ? null : body }).title;
  assert.match(title(401), /sign in/i);
  assert.match(title(403), /access/i);
  assert.match(title(404), /not found/i);
  assert.match(title(413), /too large|limit/i);
  assert.match(title(504), /too long/i);
  assert.match(title(500), /could not be created/i);
  assert.match(title(502), /could not be created/i);
  assert.match(title(418), /418/);
  assert.match(title(0), /could not reach/i);
});

test('describeFailure ignores a malformed error list', () => {
  const f = b.describeFailure({ status: 422, retryAfter: null, body: { errors: 'nope' } });
  assert.deepEqual(f.items, []);
  assert.ok(f.title.length > 0);
});

test('basePathFromScriptSrc finds the prefix of the script', () => {
  assert.equal(b.basePathFromScriptSrc('https://wiki.example.invalid/pdf/button.js?v=2'), '/pdf');
  assert.equal(b.basePathFromScriptSrc('/_pdf/button.js'), '/_pdf');
  assert.equal(b.basePathFromScriptSrc('/button.js'), '');
  assert.equal(b.basePathFromScriptSrc(''), '/pdf');
  assert.equal(b.basePathFromScriptSrc(null), '/pdf');
  assert.equal(b.basePathFromScriptSrc('/static/other.js'), '/pdf');
});

test('sanitizeFilename keeps a plain name', () => {
  assert.equal(b.sanitizeFilename('Report 1.pdf', 'x.pdf'), 'Report 1.pdf');
  assert.equal(b.sanitizeFilename('', 'x.pdf'), 'x.pdf');
});

// ---- the browser part, against a minimal DOM stub -------------------------

const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

class FakeElement {
  constructor(tag) {
    this.tag = tag;
    this.children = [];
    this.attributes = {};
    this.listeners = {};
    this.id = '';
    this.className = '';
    this.textContent = '';
    this.disabled = false;
    this.style = {};
    this.parent = null;
  }
  appendChild(child) { child.parent = this; this.children.push(child); return child; }
  remove() { if (this.parent) this.parent.children = this.parent.children.filter((c) => c !== this); this.parent = null; }
  setAttribute(k, v) { this.attributes[k] = String(v); }
  getAttribute(k) { return k in this.attributes ? this.attributes[k] : null; }
  addEventListener(type, fn) { (this.listeners[type] = this.listeners[type] || []).push(fn); }
  click() { (this.listeners.click || []).forEach((fn) => fn()); }
  focus() { this.focused = true; }
  find(id) {
    for (const c of this.children) {
      if (c.id === id) return c;
      const deeper = c.find(id);
      if (deeper) return deeper;
    }
    return null;
  }
  text() { return this.textContent + this.children.map((c) => c.text()).join('|'); }
}

function setup(pathname, responses, scriptAttributes) {
  const body = new FakeElement('body');
  const head = new FakeElement('head');
  const calls = [];
  const warnings = [];
  const windowListeners = {};
  const history = { pushState() {}, replaceState() {} };
  const document = {
    readyState: 'complete',
    visibilityState: 'visible',
    body, head,
    // No Outline header in this stub: the anchor is never found, so the
    // script ends in its floating fallback after the (zero) wait.
    currentScript: {
      src: 'https://wiki.example.invalid/pdf/button.js',
      getAttribute: (name) => Object.assign({ 'data-fallback-delay': '0' }, scriptAttributes)[name] ?? null,
    },
    getElementById: (id) => body.find(id) || head.find(id),
    createElement: (tag) => new FakeElement(tag),
    querySelector: () => null,
    querySelectorAll: () => [],
    addEventListener() {},
    removeEventListener() {},
  };
  const win = {
    location: { pathname },
    history,
    document,
    navigator: { language: 'en-US' },
    console: { warn: (...args) => warnings.push(args.join(' ')) },
    addEventListener: (type, fn) => { (windowListeners[type] = windowListeners[type] || []).push(fn); },
    removeEventListener() {},
    dispatchEvent: (e) => { (windowListeners[e.type] || []).forEach((fn) => fn(e)); return true; },
  };
  const fetch = (url, options) => {
    calls.push({ url, options });
    const answer = responses[url.split('?')[0]];
    return answer ? Promise.resolve(answer) : Promise.reject(new Error('network'));
  };
  const context = vm.createContext({
    window: win, document, fetch, history, Event: class { constructor(type) { this.type = type; } },
    CSSStyleSheet: undefined, setTimeout, clearTimeout, URL, Date, console,
    MutationObserver: class { observe() {} disconnect() {} },
  });
  vm.runInContext(fs.readFileSync(path.join(__dirname, 'button.js'), 'utf8'), context);
  return { body, calls, win, history, warnings };
}

const json = (status, data) => ({
  ok: status >= 200 && status < 300, status,
  headers: { get: () => null },
  json: () => Promise.resolve(data),
  text: () => Promise.resolve(JSON.stringify(data)),
});
const tick = () => new Promise((resolve) => setTimeout(resolve, 20));

test('the button appears when the status says available, with the cookie sent', async () => {
  const env = setup('/doc/plan-aB3dE5fGh1', { '/pdf/status/aB3dE5fGh1': json(200, { available: true, reason: 'collection_binding' }) });
  await tick();
  const button = env.body.find('olp-pdf-button');
  assert.ok(button, 'button is shown');
  assert.equal(button.tag, 'button');
  assert.equal(button.type, 'button');
  assert.equal(button.attributes['aria-label'], 'Download formatted PDF');
  assert.equal(env.calls[0].options.credentials, 'same-origin');
});

test('the button stays hidden when nothing is bound or the status call fails', async () => {
  for (const response of [json(200, { available: false, reason: 'no_binding' }), json(403, { message: 'no' }), json(500, {}), json(200, 'garbage')]) {
    const env = setup('/doc/plan-aB3dE5fGh1', { '/pdf/status/aB3dE5fGh1': response });
    await tick();
    assert.equal(env.body.find('olp-pdf-button'), null);
  }
  const env = setup('/doc/plan-aB3dE5fGh1', {});
  await tick();
  assert.equal(env.body.find('olp-pdf-button'), null, 'network failure hides it');
});

test('no request is made outside document pages', async () => {
  const env = setup('/collection/x', {});
  await tick();
  assert.equal(env.calls.length, 0);
});

test('pushState to another page removes the button and to a document asks again', async () => {
  const env = setup('/doc/plan-aB3dE5fGh1', {
    '/pdf/status/aB3dE5fGh1': json(200, { available: true }),
    '/pdf/status/zZ9yY8xX7w': json(200, { available: false }),
  });
  await tick();
  assert.ok(env.body.find('olp-pdf-button'));
  env.win.location.pathname = '/home';
  env.history.pushState({}, '', '/home');
  await tick();
  assert.equal(env.body.find('olp-pdf-button'), null);
  env.win.location.pathname = '/doc/other-zZ9yY8xX7w';
  env.history.pushState({}, '', '/doc/other-zZ9yY8xX7w');
  await tick();
  assert.equal(env.body.find('olp-pdf-button'), null);
  assert.equal(env.calls.length, 2);
});

test('a failed render shows an escaped, dismissible error list and re-enables the button', async () => {
  const hostile = '<img src=x onerror=alert(1)>';
  const env = setup('/doc/plan-aB3dE5fGh1', {
    '/pdf/status/aB3dE5fGh1': json(200, { available: true }),
    '/pdf/render/aB3dE5fGh1': Object.assign(json(422, { errors: [{ message: hostile, field: 'recipient', line: 4, source: 'document' }] }), {
      headers: { get: (h) => (h === 'Content-Type' ? 'application/json' : null) },
    }),
  });
  await tick();
  const button = env.body.find('olp-pdf-button');
  button.click();
  assert.equal(button.disabled, true);
  assert.equal(button.attributes['aria-busy'], 'true');
  await tick();
  const panel = env.body.find('olp-pdf-panel');
  assert.ok(panel, 'panel is shown');
  assert.equal(panel.attributes.role, 'alert');
  assert.ok(panel.text().includes(hostile), 'text is present verbatim as text');
  assert.ok(panel.text().includes('Line: 4'));
  assert.equal(button.disabled, false);
  panel.children[0].children[1].click();
  assert.equal(env.body.find('olp-pdf-panel'), null);
  assert.equal(button.focused, true);
});

// ---- links to the docs ----------------------------------------------------

const DOCS = 'https://outprint.task.media';
const PAGE = '/write/when-something-goes-wrong/';

test('safeDocsUrl accepts only https links to the docs site', () => {
  const ok = [
    DOCS + '/0.3' + PAGE + '#enum',
    DOCS + PAGE + '#enum',
    DOCS + '/dev' + PAGE + '#no_binding',
  ];
  ok.forEach((url) => assert.equal(b.safeDocsUrl(url), url));
  const hostile = [
    'javascript:alert(1)',
    'JaVaScRiPt:alert(1)',
    'data:text/html,<script>alert(1)</script>',
    'http://outprint.task.media/write/',
    'https://evil.example/write/',
    'https://outprint.task.media.evil.example/write/',
    'https://outprint.task.media@evil.example/write/',
    'https://user:pw@outprint.task.media/write/',
    'https://outprint.task.media:8443/write/',
    '//outprint.task.media/write/',
    '/write/when-something-goes-wrong/',
    'not a url',
    '',
    null,
    undefined,
    42,
    { href: DOCS },
  ];
  hostile.forEach((url) => assert.equal(b.safeDocsUrl(url), '', String(url)));
});

test('formatErrorItem carries the docs link only when it is safe', () => {
  const url = DOCS + '/0.3' + PAGE + '#enum';
  assert.equal(b.formatErrorItem({ message: 'x', docs_url: url }).docsUrl, url);
  assert.equal(b.formatErrorItem({ message: 'x', docs_url: 'javascript:alert(1)' }).docsUrl, undefined);
  assert.equal(b.formatErrorItem({ message: 'x' }).docsUrl, undefined);
});

test('docsHome is the start page of the same version', () => {
  assert.equal(b.docsHome(DOCS + '/0.3' + PAGE + '#enum'), DOCS + '/0.3/');
  assert.equal(b.docsHome(DOCS + '/dev' + PAGE + '#enum'), DOCS + '/dev/');
  assert.equal(b.docsHome(DOCS + PAGE + '#enum'), DOCS + '/');
  assert.equal(b.docsHome(DOCS + '/somewhere/else/'), '');
  assert.equal(b.docsHome(''), '');
});

test('describeFailure finds the docs links of an error list and of a plain failure', () => {
  const url = DOCS + '/0.3' + PAGE + '#enum';
  const list = b.describeFailure({ status: 422, retryAfter: null, body: { errors: [{ message: 'one', docs_url: url }] } });
  assert.equal(list.items[0].docsUrl, url);
  assert.equal(list.helpUrl, DOCS + '/0.3/');

  const plain = b.describeFailure({ status: 504, retryAfter: null, body: { message: 'slow', docs_url: DOCS + '/0.3' + PAGE + '#timeout' } });
  assert.equal(plain.docsUrl, DOCS + '/0.3' + PAGE + '#timeout');
  assert.equal(plain.helpUrl, DOCS + '/0.3/');

  const none = b.describeFailure({ status: 422, retryAfter: null, body: { errors: [{ message: 'one' }] } });
  assert.equal(none.helpUrl, undefined);
  assert.equal(none.items[0].docsUrl, undefined);
});

function linksOf(el, out) {
  out = out || [];
  if (el.tag === 'a') out.push(el);
  el.children.forEach((c) => linksOf(c, out));
  return out;
}

async function renderFailure(body, status) {
  const env = setup('/doc/plan-aB3dE5fGh1', {
    '/pdf/status/aB3dE5fGh1': json(200, { available: true }),
    '/pdf/render/aB3dE5fGh1': Object.assign(json(status || 422, body), {
      headers: { get: (h) => (h === 'Content-Type' ? 'application/json' : null) },
    }),
  });
  await tick();
  env.body.find('olp-pdf-button').click();
  await tick();
  return env.body.find('olp-pdf-panel');
}

test('each error with a docs link gets a "How to fix" link that opens safely in a new tab, and the panel a "Help" link', async () => {
  const url = DOCS + '/0.3' + PAGE + '#enum';
  const panel = await renderFailure({ errors: [
    { message: 'first', docs_url: url },
    { message: 'second', docs_url: DOCS + '/0.3' + PAGE + '#required' },
    { message: 'third' },
  ] });
  const links = linksOf(panel);
  assert.deepEqual(links.map((a) => a.textContent), ['How to fix', 'How to fix', 'Help']);
  assert.deepEqual(links.map((a) => a.href), [url, DOCS + '/0.3' + PAGE + '#required', DOCS + '/0.3/']);
  links.forEach((a) => {
    assert.equal(a.target, '_blank');
    assert.equal(a.rel, 'noopener noreferrer');
  });
});

test('a hostile docs_url is shown as no link at all', async () => {
  const panel = await renderFailure({ errors: [
    { message: 'first', docs_url: 'javascript:alert(1)' },
    { message: 'second', docs_url: 'https://evil.example/write/when-something-goes-wrong/#enum' },
  ] });
  assert.deepEqual(linksOf(panel), []);
  assert.ok(panel.text().includes('first'));
});

test('a plain failure with a docs link shows it under the title', async () => {
  const url = DOCS + PAGE + '#timeout';
  const panel = await renderFailure({ message: 'slow', docs_url: url }, 504);
  assert.deepEqual(linksOf(panel).map((a) => a.href), [url, DOCS + '/']);
});

test('the script trusts the same docs origin as the Gateway', () => {
  assert.equal(b.DOCS_ORIGIN, DOCS);
});

// ---- placement, language and texts ---------------------------------------

const attrs = (map) => (name) => (name in map ? map[name] : null);

test('parseOptions defaults to the menu, icon and label, fallback on', () => {
  const o = b.parseOptions(attrs({}));
  assert.equal(o.placement, 'menu');
  assert.equal(o.style, 'icon-label');
  assert.equal(o.fallback, true);
  assert.equal(o.fallbackDelay, 3000);
  assert.deepEqual(o.labels, {});
});

test('parseOptions reads the data attributes of the tag', () => {
  const o = b.parseOptions(attrs({
    'data-placement': 'button', 'data-style': 'icon', 'data-fallback': 'off', 'data-fallback-delay': '1500',
    'data-labels': '{"menu":{"de":"PDF mit Briefkopf"}}',
  }));
  assert.equal(o.placement, 'button');
  assert.equal(o.style, 'icon');
  assert.equal(o.fallback, false);
  assert.equal(o.fallbackDelay, 1500);
  assert.deepEqual(o.labels, { menu: { de: 'PDF mit Briefkopf' } });
});

test('parseOptions ignores unusable values', () => {
  const o = b.parseOptions(attrs({
    'data-placement': 'floating', 'data-style': 'big', 'data-fallback-delay': '-5', 'data-labels': '{not json',
  }));
  assert.equal(o.placement, 'menu');
  assert.equal(o.style, 'icon-label');
  assert.equal(o.fallbackDelay, 3000);
  assert.deepEqual(o.labels, {});
  const odd = b.parseOptions(attrs({ 'data-labels': '{"menu":{"de":"","DE":"x","en":' + JSON.stringify('x'.repeat(81)) + '},"other":{"en":"x"},"tooltip":{"en":7}}' }));
  assert.deepEqual(odd.labels, {});
  assert.equal(b.parseOptions(attrs({ 'data-fallback-delay': '99999999' })).fallbackDelay, 60000);
});

test('resolveLanguage follows the Outline language, then the browser, then English', () => {
  assert.equal(b.resolveLanguage('de_DE', 'en-US'), 'de');
  assert.equal(b.resolveLanguage('en_US', 'de-DE'), 'en');
  assert.equal(b.resolveLanguage(undefined, 'de-AT'), 'de');
  assert.equal(b.resolveLanguage('', 'de'), 'de');
  assert.equal(b.resolveLanguage(undefined, undefined), 'en');
  assert.equal(b.resolveLanguage(undefined, 'xx'), 'en');
});

test('resolveLanguage falls back to English for a language without texts', () => {
  assert.equal(b.resolveLanguage('fr_FR', 'de-DE'), 'en');
  assert.equal(b.resolveLanguage('fr_FR', 'de-DE', ['fr']), 'fr');
});

test('textsFor gives the built-in texts in English and German', () => {
  const en = b.textsFor('en', {});
  assert.equal(en.menu, 'Formatted PDF');
  assert.equal(en.button, 'PDF');
  assert.equal(en.tooltip, 'Download formatted PDF');
  const de = b.textsFor('de', {});
  assert.equal(de.menu, 'Formatiertes PDF');
  assert.equal(de.tooltip, 'Formatiertes PDF herunterladen');
});

test('textsFor prefers a configured text for the language, then the built-in one, then configured English', () => {
  const labels = { menu: { de: 'PDF mit Briefkopf', en: 'Letterhead PDF' }, tooltip: { en: 'Get the PDF' }, button: { fr: 'PDF' } };
  assert.equal(b.textsFor('de', labels).menu, 'PDF mit Briefkopf');
  assert.equal(b.textsFor('de', labels).tooltip, 'Formatiertes PDF herunterladen');
  assert.equal(b.textsFor('en', labels).tooltip, 'Get the PDF');
  assert.equal(b.textsFor('fr', labels).menu, 'Letterhead PDF');
  assert.equal(b.textsFor('fr', labels).button, 'PDF');
  assert.equal(b.textsFor('zz', {}).menu, 'Formatted PDF');
});

test('isExportItemText recognises the file types of the Export items, not their translated names', () => {
  for (const text of ['Markdown.md', 'HTML.html', 'TextBundle.textpack', 'PDF.pdf', ' Markdown.md ']) assert.equal(b.isExportItemText(text), true, text);
  for (const text of ['Print⌘P', 'Copy link', 'Copy as Markdown', 'Pin to home', '', null, undefined]) assert.equal(b.isExportItemText(text), false, String(text));
});

test('without an anchor the floating button appears after the wait, with a console warning', async () => {
  const env = setup('/doc/plan-aB3dE5fGh1', { '/pdf/status/aB3dE5fGh1': json(200, { available: true, language: 'de_DE' }) });
  await tick();
  const button = env.body.find('olp-pdf-button');
  assert.ok(button);
  assert.equal(button.attributes['aria-label'], 'Formatiertes PDF herunterladen');
  assert.equal(env.warnings.length, 1);
  assert.match(env.warnings[0], /menu/);
});

test('fallback off shows nothing when the anchor is missing', async () => {
  const env = setup('/doc/plan-aB3dE5fGh1', { '/pdf/status/aB3dE5fGh1': json(200, { available: true }) }, { 'data-fallback': 'off' });
  await tick();
  assert.equal(env.body.find('olp-pdf-button'), null);
  assert.equal(env.warnings.length, 0);
});

test('firstMetadataBlockIndex picks the first yaml block', () => {
  assert.equal(b.firstMetadataBlockIndex(['yaml', 'js']), 0);
  assert.equal(b.firstMetadataBlockIndex(['js', 'YAML', 'yaml']), 1);
  assert.equal(b.firstMetadataBlockIndex(['sh', 'yml']), 1);
});

test('firstMetadataBlockIndex finds nothing without a yaml block', () => {
  assert.equal(b.firstMetadataBlockIndex([]), -1);
  assert.equal(b.firstMetadataBlockIndex(['js', '', null, undefined]), -1);
  assert.equal(b.firstMetadataBlockIndex(null), -1);
});

test('shouldHideMetadataBlock hides only in the reading view', () => {
  assert.equal(b.shouldHideMetadataBlock({ found: true, editable: false }), true);
  assert.equal(b.shouldHideMetadataBlock({ found: true, editable: true }), false);
  assert.equal(b.shouldHideMetadataBlock({ found: false, editable: false }), false);
});
