import { execFileSync } from 'node:child_process';
import { mkdirSync, rmSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { expect, test, type Page } from '@playwright/test';
import { chooseFormattedPdf, MENU_LABEL, openExportSubmenu } from '../support/placement';
import { loadSeed } from '../support/seed';

// Generates the screenshots of the Outline UI used by the documentation, from
// the seeded dev stack. Run it with `task screenshots`; it writes unoptimised
// PNGs to SCREENSHOTS_RAW_DIR, which optimize.mjs then compresses into
// docs/assets/screens/. Only the fictional seed data is ever on screen.
const seed = loadSeed();
const RAW_DIR = resolve(process.env.SCREENSHOTS_RAW_DIR ?? 'test-results/screens-raw');
const ANY_ACTION = '#olp-pdf-button, .olp-header-button';
const ERROR_PANEL = '[role="alert"]';
const DOCUMENT_TITLE = '[role="textbox"][aria-label="Document title"]';
const COLLECTION_TITLE = 'span[role="textbox"]';
const CODE_BLOCK = '.ProseMirror pre';
const FIRST_PARAGRAPH = '.ProseMirror p:not(.caption)';
const PADDING = 16;
// The Collection icon sits left of the title textbox.
const COLLECTION_ICON_WIDTH = 48;

test.use({
  viewport: { width: 1280, height: 800 },
  deviceScaleFactor: 2,
  colorScheme: 'light',
  reducedMotion: 'reduce',
  locale: 'en-US',
  timezoneId: 'UTC',
});

async function signIn(page: Page): Promise<void> {
  await page.goto('/');
  await page.locator('input[name="login"]').fill(seed.email);
  await page.locator('input[name="password"]').fill(seed.password);
  await page.locator('button[type="submit"]').click();
  await page.waitForURL((url) => url.origin === new URL(seed.baseUrl).origin && !url.pathname.startsWith('/auth'));
  await expect(page.locator('a[href^="/collection/"]').first()).toBeVisible();
}

// Opens a page and waits until it has settled: network idle, fonts loaded,
// the title shown, and the mouse parked on an empty spot so that no hover
// state is captured.
async function open(page: Page, path: string, titleSelector: string, title: string): Promise<void> {
  await page.goto(path);
  await page.waitForLoadState('networkidle');
  await expect(page.locator(titleSelector).first()).toHaveText(title);
  await page.evaluate(() => document.fonts.ready);
  await page.mouse.move(130, 540);
}

// The line under a document title ("You updated 3 minutes ago, viewed by ...")
// changes between runs. It is hidden, not removed, so nothing shifts.
async function hideDocumentTimestamps(page: Page): Promise<void> {
  await page.evaluate((titleSelector) => {
    const title = document.querySelector(titleSelector);
    const ownText = (el: Element) =>
      [...el.childNodes].filter((n) => n.nodeType === Node.TEXT_NODE).map((n) => n.textContent).join('');
    const leaves = [...document.querySelectorAll('body *')].filter((el) =>
      /^\s*(You )?(updated|created|published|saved)|^\s*Viewed by/i.test(ownText(el)) || /^Viewed by/.test(el.textContent ?? '') && el.children.length === 0,
    );
    if (!title || leaves.length === 0) {
      throw new Error('the line with the update time was not found; the Outline UI changed');
    }
    for (const leaf of leaves) {
      // Climb to the widest element that does not yet contain the title.
      let top: Element = leaf;
      while (top.parentElement && !top.parentElement.contains(title) && top.parentElement.tagName !== 'BODY') {
        top = top.parentElement;
      }
      (top as HTMLElement).style.visibility = 'hidden';
    }
  }, DOCUMENT_TITLE);
}

async function shot(page: Page, name: string, clip?: { x: number; y: number; width: number; height: number }): Promise<void> {
  await page.screenshot({ path: join(RAW_DIR, name), animations: 'disabled', caret: 'hide', clip });
}

// A padded clip around the union of the given elements, inside the viewport.
async function around(page: Page, selectors: string[], extraLeft = 0) {
  const boxes = [];
  for (const selector of selectors) {
    const box = await page.locator(selector).first().boundingBox();
    expect(box, `${selector} has a box`).not.toBeNull();
    boxes.push(box!);
  }
  const view = page.viewportSize()!;
  const x = Math.max(0, Math.min(...boxes.map((b) => b.x)) - PADDING - extraLeft);
  const y = Math.max(0, Math.min(...boxes.map((b) => b.y)) - PADDING);
  const right = Math.min(view.width, Math.max(...boxes.map((b) => b.x + b.width)) + PADDING);
  const bottom = Math.min(view.height, Math.max(...boxes.map((b) => b.y + b.height)) + PADDING);
  return { x, y, width: right - x, height: bottom - y };
}

// The first page of a PDF as a PNG, with pdftoppm from the host or, where
// that is not installed (or SCREENSHOTS_PDFTOPPM=docker), from Alpine's
// poppler-utils in Docker.
function firstPageToPng(pdfPath: string, pngPath: string): void {
  const base = pngPath.replace(/\.png$/, '');
  const args = ['-png', '-r', '110', '-f', '1', '-l', '1', '-singlefile'];
  const hasHostTool = (() => {
    try {
      execFileSync('pdftoppm', ['-v'], { stdio: 'ignore' });
      return true;
    } catch {
      return false;
    }
  })();
  if (hasHostTool && process.env.SCREENSHOTS_PDFTOPPM !== 'docker') {
    execFileSync('pdftoppm', [...args, pdfPath, base]);
    return;
  }
  const dir = resolve(pdfPath, '..');
  execFileSync('docker', [
    'run', '--rm', '--user', `${process.getuid?.() ?? 0}:${process.getgid?.() ?? 0}`,
    '-v', `${dir}:/work`, '-w', '/work', '--entrypoint', 'sh', 'alpine:3.22',
    '-c', `apk add --no-cache poppler-utils >/dev/null && pdftoppm ${args.join(' ')} /work/letter.pdf /work/${base.split('/').pop()}`,
  ], { stdio: 'inherit' });
}

test('screenshots of the Outline UI for the documentation', async ({ page }) => {
  rmSync(RAW_DIR, { recursive: true, force: true });
  mkdirSync(RAW_DIR, { recursive: true });
  await signIn(page);

  // A document in a bound Collection, with the button.
  const status = page.waitForResponse((r) => new URL(r.url()).pathname.startsWith('/pdf/status/'));
  await open(page, seed.bound.documentPath, DOCUMENT_TITLE, seed.bound.documentTitle);
  expect((await status).status()).toBe(200);
  // The action sits in the Export menu: show the menu open.
  await openExportSubmenu(page);
  await expect(page.getByRole('menuitem', { name: MENU_LABEL })).toBeVisible();
  await hideDocumentTimestamps(page);
  await shot(page, 'bound-document-with-button.png');
  await page.keyboard.press('Escape');
  await page.mouse.move(130, 540);

  // The leading Metadata Block with a line of body text below it. The title
  // is left out: the hidden byline would leave a gap between the two.
  const metadata = await around(page, [CODE_BLOCK, FIRST_PARAGRAPH]);
  await shot(page, 'document-metadata-block.png', metadata);

  // The example letter: the PDF of this document, first page.
  const [download] = await Promise.all([page.waitForEvent('download'), chooseFormattedPdf(page)]);
  const pdfPath = join(RAW_DIR, 'letter.pdf');
  await download.saveAs(pdfPath);
  firstPageToPng(pdfPath, join(RAW_DIR, 'example-letter.png'));
  rmSync(pdfPath);

  // A document in a Collection without a Binding: no button.
  const unboundStatus = page.waitForResponse((r) => new URL(r.url()).pathname.startsWith('/pdf/status/'));
  await open(page, seed.unbound.documentPath, DOCUMENT_TITLE, seed.unbound.documentTitle);
  expect(await (await unboundStatus).json()).toMatchObject({ available: false });
  await expect(page.locator(ANY_ACTION)).toHaveCount(0);
  await hideDocumentTimestamps(page);
  await shot(page, 'unbound-document-no-button.png');

  // The Collection overview with the Binding block.
  await open(page, `/collection/${seed.bound.collectionId}`, COLLECTION_TITLE, seed.bound.collectionName);
  await expect(page.locator(CODE_BLOCK)).toBeVisible();
  await shot(page, 'collection-binding-block.png', await around(page, [COLLECTION_TITLE, CODE_BLOCK], COLLECTION_ICON_WIDTH));

  // What an author sees when the Metadata Block is wrong.
  await open(page, seed.invalid.documentPath, DOCUMENT_TITLE, seed.invalid.documentTitle);
  await hideDocumentTimestamps(page);
  await chooseFormattedPdf(page);
  await expect(page.locator(ERROR_PANEL)).toBeVisible();
  await expect(page.locator(ERROR_PANEL)).toContainText('subjekt');
  await page.mouse.move(130, 540);
  await shot(page, 'error-shown-to-author.png');
});
