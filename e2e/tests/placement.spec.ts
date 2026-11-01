import { readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test, type Download, type Page } from '@playwright/test';
import { MENU_LABEL, breakMenuAnchor, clickSubmenuItem, openExportSubmenu, optionsButton, setScriptAttributes, signIn, statusResponse, useLocalScript } from '../support/placement';
import { loadSeed } from '../support/seed';

const seed = loadSeed();
// Outline rate-limits sign-ins, so the tests sign in once and share the session.
const SESSION_FILE = join(tmpdir(), 'outprint-e2e-placement-session.json');
const HEADER_BUTTON = '.olp-header-button';
const FLOATING_BUTTON = '#olp-pdf-button';

test.use({ storageState: SESSION_FILE });

test.beforeAll(async ({ browser }) => {
  const context = await browser.newContext({ baseURL: seed.baseUrl, storageState: undefined });
  await signIn(await context.newPage());
  await context.storageState({ path: SESSION_FILE });
  await context.close();
});

async function openBound(page: Page): Promise<void> {
  const status = statusResponse(page);
  await page.goto(seed.bound.documentPath);
  expect((await status).status()).toBe(200);
}

function expectPDF(download: Download, outputPath: string) {
  return download.saveAs(outputPath).then(() => {
    expect(download.suggestedFilename()).toMatch(/\.pdf$/);
    expect(readFileSync(outputPath).subarray(0, 4).toString('latin1')).toBe('%PDF');
  });
}

test.beforeEach(async ({ page }) => {
  await useLocalScript(page);
});

test('the Export menu offers the formatted PDF and a click downloads it', async ({ page }, testInfo) => {
  await openBound(page);
  await openExportSubmenu(page);
  const item = page.getByRole('menuitem', { name: MENU_LABEL });
  await expect(item).toBeVisible();
  await expect(page.locator(FLOATING_BUTTON)).toHaveCount(0);

  const [download] = await Promise.all([page.waitForEvent('download'), clickSubmenuItem(page, item)]);
  await expectPDF(download, testInfo.outputPath('menu.pdf'));
  // Like its neighbours, the item closes the menu and the focus returns to the trigger.
  await expect(page.getByRole('menu')).toHaveCount(0);
  await expect(optionsButton(page)).toBeFocused();
});

test('the entry is added once, however often the menu is opened', async ({ page }) => {
  await openBound(page);
  for (let i = 0; i < 3; i++) {
    await openExportSubmenu(page);
    await expect(page.getByRole('menuitem', { name: MENU_LABEL })).toHaveCount(1);
    await page.keyboard.press('Escape');
    await expect(page.getByRole('menu')).toHaveCount(0);
  }
});

test('the entry can be reached and used with the keyboard', async ({ page }, testInfo) => {
  await openBound(page);
  await openExportSubmenu(page);
  const item = page.getByRole('menuitem', { name: MENU_LABEL });
  await page.getByRole('menuitem', { name: 'Markdown' }).focus();
  // Wait for each move: Radix updates the focus after the key event.
  for (const name of ['HTML', 'TextBundle']) {
    await page.keyboard.press('ArrowDown');
    await expect(page.getByRole('menuitem', { name })).toBeFocused();
  }
  await page.keyboard.press('ArrowDown');
  await expect(item).toBeFocused();
  await page.keyboard.press('ArrowUp');
  await expect(page.getByRole('menuitem', { name: 'TextBundle' })).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await expect(item).toBeFocused();
  const [download] = await Promise.all([page.waitForEvent('download'), page.keyboard.press('Enter')]);
  await expectPDF(download, testInfo.outputPath('keyboard.pdf'));
});

test('a document without a Binding gets no entry', async ({ page }) => {
  const status = statusResponse(page);
  await page.goto(seed.unbound.documentPath);
  expect(await (await status).json()).toMatchObject({ available: false });
  await openExportSubmenu(page);
  await expect(page.getByRole('menuitem', { name: MENU_LABEL })).toHaveCount(0);
});

test('placement=button puts a header button before Share', async ({ page }, testInfo) => {
  await setScriptAttributes(page, { placement: 'button' });
  await openBound(page);
  const button = page.locator(HEADER_BUTTON);
  await expect(button).toBeVisible();
  await expect(button).toHaveAccessibleName('Download formatted PDF');
  await expect(button).toContainText('PDF');
  await expect(page.locator(FLOATING_BUTTON)).toHaveCount(0);

  const beforeShare = await button.evaluate((el) => {
    const next = el.parentElement?.nextElementSibling?.querySelector('button');
    return next?.getAttribute('aria-haspopup') === 'dialog' && !!next.textContent?.trim();
  });
  expect(beforeShare, 'the button sits directly before Share').toBe(true);

  const [download] = await Promise.all([page.waitForEvent('download'), button.click()]);
  await expectPDF(download, testInfo.outputPath('button.pdf'));
});

test('the icon style keeps an accessible name and a tooltip', async ({ page }) => {
  await setScriptAttributes(page, { placement: 'button', style: 'icon' });
  await openBound(page);
  const button = page.locator(HEADER_BUTTON);
  await expect(button).toBeVisible();
  await expect(button).toHaveAccessibleName('Download formatted PDF');
  await expect(button).toHaveAttribute('title', 'Download formatted PDF');
  await expect(button).not.toContainText('PDF');
});

test('operator labels replace the built-in texts', async ({ page }) => {
  await setScriptAttributes(page, { labels: JSON.stringify({ menu: { en: 'Letterhead PDF' } }) });
  await openBound(page);
  await openExportSubmenu(page);
  await expect(page.getByRole('menuitem', { name: 'Letterhead PDF' })).toBeVisible();
});

test('a missing anchor falls back to the floating button and warns', async ({ page }) => {
  await breakMenuAnchor(page);
  await setScriptAttributes(page, { 'fallback-delay': '500' });
  const warning = page.waitForEvent('console', (m) => m.type() === 'warning' && m.text().includes('Outprint'));
  await openBound(page);
  await expect(page.locator(FLOATING_BUTTON)).toBeVisible();
  await warning;
});

test('fallback=off shows nothing when the anchor is missing', async ({ page }) => {
  await breakMenuAnchor(page);
  await setScriptAttributes(page, { fallback: 'off', 'fallback-delay': '200' });
  await openBound(page);
  await page.waitForTimeout(1000); // the absence of something needs a wait; the delay above is 200 ms
  await expect(page.locator(FLOATING_BUTTON)).toHaveCount(0);
  await expect(page.locator(HEADER_BUTTON)).toHaveCount(0);
});

test('the labels follow the language of the Outline user', async ({ page, context }) => {
  const token = (await context.cookies()).find((c) => c.name === 'accessToken')?.value;
  expect(token, 'the sign-in left a session cookie').toBeTruthy();
  const headers = { Authorization: `Bearer ${token}` };
  const info = await page.request.post('/api/auth.info', { headers, data: {} });
  const previous: string = (await info.json()).data.user.language ?? 'en_US';
  const setLanguage = (language: string) => page.request.post('/api/users.update', { headers, data: { language } });
  try {
    expect((await setLanguage('de_DE')).ok()).toBeTruthy();
    await setScriptAttributes(page, { placement: 'button' });
    const status = statusResponse(page);
    await page.goto(seed.bound.documentPath);
    expect(await (await status).json()).toMatchObject({ available: true, language: 'de_DE' });
    await expect(page.locator(HEADER_BUTTON)).toHaveAttribute('title', 'Formatiertes PDF herunterladen');
  } finally {
    await setLanguage(previous);
  }
});

test.describe('a viewer, who cannot edit', () => {
  const VIEWER_SESSION_FILE = join(tmpdir(), 'outprint-e2e-viewer-session.json');
  test.use({ storageState: VIEWER_SESSION_FILE });

  test.beforeAll(async ({ browser }) => {
    const context = await browser.newContext({ baseURL: seed.baseUrl, storageState: undefined });
    await signIn(await context.newPage(), seed.viewer);
    await context.storageState({ path: VIEWER_SESSION_FILE });
    await context.close();
  });

  test('can use the Export menu entry', async ({ page }, testInfo) => {
    await openBound(page);
    await openExportSubmenu(page);
    const item = page.getByRole('menuitem', { name: MENU_LABEL });
    await expect(item).toBeVisible();
    const [download] = await Promise.all([page.waitForEvent('download'), clickSubmenuItem(page, item)]);
    await expectPDF(download, testInfo.outputPath('viewer.pdf'));
  });

  test('gets the header button before the "…" button, as there is no Share', async ({ page }) => {
    await setScriptAttributes(page, { placement: 'button' });
    await openBound(page);
    const button = page.locator(HEADER_BUTTON);
    await expect(button).toBeVisible();
    await expect(page.getByRole('button', { name: 'Share' })).toHaveCount(0);
    const beforeOptions = await button.evaluate((el) => {
      const next = el.parentElement?.nextElementSibling?.querySelector('button');
      return next?.getAttribute('aria-haspopup') === 'menu';
    });
    expect(beforeOptions, 'the button sits directly before the menu button').toBe(true);
  });
});
