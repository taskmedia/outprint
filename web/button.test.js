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

function setup(pathname, responses) {
  const body = new FakeElement('body');
  const head = new FakeElement('head');
  const calls = [];
  const windowListeners = {};
  const history = { pushState() {}, replaceState() {} };
  const document = {
    readyState: 'complete',
    visibilityState: 'visible',
    body, head,
    currentScript: { src: 'https://wiki.example.invalid/pdf/button.js' },
    getElementById: (id) => body.find(id) || head.find(id),
    createElement: (tag) => new FakeElement(tag),
    addEventListener() {},
    removeEventListener() {},
  };
  const win = {
    location: { pathname },
    history,
    document,
    addEventListener: (type, fn) => { (windowListeners[type] = windowListeners[type] || []).push(fn); },
    dispatchEvent: (e) => { (windowListeners[e.type] || []).forEach((fn) => fn(e)); return true; },
  };
  const fetch = (url, options) => {
    calls.push({ url, options });
    const answer = responses[url.split('?')[0]];
    return answer ? Promise.resolve(answer) : Promise.reject(new Error('network'));
  };
  const context = vm.createContext({
    window: win, document, fetch, history, Event: class { constructor(type) { this.type = type; } },
    CSSStyleSheet: undefined, setTimeout, URL, Date, console,
  });
  vm.runInContext(fs.readFileSync(path.join(__dirname, 'button.js'), 'utf8'), context);
  return { body, calls, win, history };
}

const json = (status, data) => ({
  ok: status >= 200 && status < 300, status,
  headers: { get: () => null },
  json: () => Promise.resolve(data),
  text: () => Promise.resolve(JSON.stringify(data)),
});
const tick = () => new Promise((resolve) => setTimeout(resolve, 5));

test('the button appears when the status says available, with the cookie sent', async () => {
  const env = setup('/doc/plan-aB3dE5fGh1', { '/pdf/status/aB3dE5fGh1': json(200, { available: true, reason: 'collection_binding' }) });
  await tick();
  const button = env.body.find('olp-pdf-button');
  assert.ok(button, 'button is shown');
  assert.equal(button.tag, 'button');
  assert.equal(button.type, 'button');
  assert.equal(button.attributes['aria-label'], 'Download this document as PDF');
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
