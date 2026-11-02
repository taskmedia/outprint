import { readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test, type Page } from '@playwright/test';
import { MENU_LABEL, breakMenuAnchor, clickSubmenuItem, enableNativePdfExport, openExportSubmenu, setOutlineLanguage, setScriptAttributes, signIn, statusResponse, useLocalScript } from '../support/placement';
import { loadSeed } from '../support/seed';

// Cases the placement has to survive in Outline itself: its own PDF entry
// (PDF_EXPORT_ENABLED) and a user language other than English.
const seed = loadSeed();
const SESSION_FILE = join(tmpdir(), 'outprint-e2e-compat-session.json');
const OURS = '[role="menuitem"][data-olp-item]';
const HEADER_BUTTON = '.olp-header-button';
const FLOATING_BUTTON = '#olp-pdf-button';

test.use({ storageState: SESSION_FILE });

test.beforeAll(async ({ browser }) => {
  const context = await browser.newContext({ baseURL: seed.baseUrl, storageState: undefined });
  await signIn(await context.newPage());
  await context.storageState({ path: SESSION_FILE });
  await context.close();
});

test.beforeEach(async ({ page }) => {
  await useLocalScript(page);
});

async function openBound(page: Page): Promise<void> {
  const status = statusResponse(page);
  await page.goto(seed.bound.documentPath);
  expect((await status).status()).toBe(200);
}

async function expectPdfDownload(page: Page, trigger: Promise<unknown>, outputPath: string): Promise<void> {
  const [download] = await Promise.all([page.waitForEvent('download'), trigger]);
  await download.saveAs(outputPath);
  expect(readFileSync(outputPath).subarray(0, 4).toString('latin1')).toBe('%PDF');
}

test.describe("with Outline's own PDF entry (PDF_EXPORT_ENABLED)", () => {
  test.beforeEach(async ({ page }) => {
    await enableNativePdfExport(page);
  });

  // The items of the open Export submenu that end in .pdf, and Outline's own.
  const pdfItems = (page: Page) => page.getByRole('menuitem').filter({ hasText: /\.pdf$/ });
  const nativeItem = (page: Page) => pdfItems(page).and(page.locator(':not([data-olp-item])'));

  test('both entries are there, told apart by label, and neither is doubled', async ({ page }) => {
    await openBound(page);
    for (let i = 0; i < 3; i++) {
      await openExportSubmenu(page);
      await expect(pdfItems(page)).toHaveCount(2);
      await expect(page.locator(OURS)).toHaveCount(1);
      await expect(page.locator(OURS)).toHaveText(`${MENU_LABEL}.pdf`);
      await expect(nativeItem(page)).toHaveCount(1);
      await expect(nativeItem(page)).toHaveText('PDF.pdf');
      await page.keyboard.press('Escape');
      await expect(page.getByRole('menu')).toHaveCount(0);
    }
    await expect(page.locator(FLOATING_BUTTON)).toHaveCount(0);
  });

  test("our entry downloads the formatted PDF and does not go through Outline's export", async ({ page }, testInfo) => {
    await openBound(page);
    const outlineExports: string[] = [];
    page.on('request', (r) => {
      if (new URL(r.url()).pathname === '/api/documents.export') outlineExports.push(r.url());
    });
    await openExportSubmenu(page);
    await expectPdfDownload(page, clickSubmenuItem(page, page.locator(OURS)), testInfo.outputPath('formatted.pdf'));
    expect(outlineExports).toHaveLength(0);
  });

  test("Outline's own entry is left to Outline", async ({ page }) => {
    await openBound(page);
    await openExportSubmenu(page);
    const request = page.waitForRequest((r) => new URL(r.url()).pathname === '/api/documents.export');
    await clickSubmenuItem(page, nativeItem(page));
    await request;
    await expect(page.getByRole('menu')).toHaveCount(0);
  });

  test('the entry is still offered after the native one was used', async ({ page }) => {
    await openBound(page);
    await openExportSubmenu(page);
    await clickSubmenuItem(page, nativeItem(page));
    await expect(page.getByRole('menu')).toHaveCount(0);
    await openExportSubmenu(page);
    await expect(page.locator(OURS)).toHaveCount(1);
    await expect(pdfItems(page)).toHaveCount(2);
  });

  test('the header button is not confused by the extra entry', async ({ page }, testInfo) => {
    await setScriptAttributes(page, { placement: 'button' });
    await openBound(page);
    const button = page.locator(HEADER_BUTTON);
    await expect(button).toHaveCount(1);
    await openExportSubmenu(page);
    await expect(page.locator(OURS)).toHaveCount(0);
    await page.keyboard.press('Escape');
    await expectPdfDownload(page, button.click(), testInfo.outputPath('button.pdf'));
  });
});

test.describe('with a German Outline', () => {
  let restoreLanguage: () => Promise<void>;

  test.beforeEach(async ({ page }) => {
    restoreLanguage = await setOutlineLanguage(page, 'de_DE');
  });

  test.afterEach(async () => {
    await restoreLanguage();
  });

  async function openGermanExportSubmenu(page: Page): Promise<void> {
    // The shared helper goes by the English name of the button.
    await page.getByRole('button', { name: 'Dokument-Einstellungen' }).last().click();
    await page.getByRole('menuitem', { name: 'Exportieren' }).hover();
    await expect(page.getByRole('menuitem', { name: 'Markdown' })).toBeVisible();
  }

  test('the Export entry is found, labelled in German, and downloads the PDF', async ({ page }, testInfo) => {
    await openBound(page);
    await openGermanExportSubmenu(page);
    const item = page.getByRole('menuitem', { name: 'Formatiertes PDF' });
    await expect(item).toHaveCount(1);
    await expect(item).toHaveText('Formatiertes PDF.pdf');
    await expect(page.locator(FLOATING_BUTTON)).toHaveCount(0);
    await expectPdfDownload(page, clickSubmenuItem(page, item), testInfo.outputPath('german-menu.pdf'));
  });

  test('the entry is added once however often the menu is opened', async ({ page }) => {
    await openBound(page);
    for (let i = 0; i < 3; i++) {
      await openGermanExportSubmenu(page);
      await expect(page.locator(OURS)).toHaveCount(1);
      await page.keyboard.press('Escape');
      await expect(page.getByRole('menu')).toHaveCount(0);
    }
  });

  test('the header button sits before the share button, is named in German and downloads the PDF', async ({ page }, testInfo) => {
    await setScriptAttributes(page, { placement: 'button' });
    await openBound(page);
    const button = page.locator(HEADER_BUTTON);
    await expect(button).toBeVisible();
    await expect(button).toHaveAccessibleName('Formatiertes PDF herunterladen');
    await expect(page.locator(FLOATING_BUTTON)).toHaveCount(0);

    const beforeShare = await button.evaluate((el) => {
      const next = el.parentElement?.nextElementSibling?.querySelector('button');
      return next?.getAttribute('aria-haspopup') === 'dialog' && !!next.textContent?.trim();
    });
    expect(beforeShare, 'the button sits directly before the share button').toBe(true);
    await expectPdfDownload(page, button.click(), testInfo.outputPath('german-button.pdf'));
  });

  test('the fallback button appears, with the German name, when the anchor is missing', async ({ page }) => {
    await breakMenuAnchor(page);
    await setScriptAttributes(page, { 'fallback-delay': '500' });
    await openBound(page);
    await expect(page.locator(FLOATING_BUTTON)).toBeVisible();
    await expect(page.locator(FLOATING_BUTTON)).toHaveAttribute('title', 'Formatiertes PDF herunterladen');
  });
});
