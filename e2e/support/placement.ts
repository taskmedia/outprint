import { expect, type Locator, type Page } from '@playwright/test';
import { loadSeed } from './seed';

const seed = loadSeed();

export const MENU_LABEL = 'Formatted PDF';

// Signs in through the dev Dex, the way a person does.
export async function signIn(page: Page, user: { email: string; password: string } = seed): Promise<void> {
  await page.goto('/');
  await page.locator('input[name="login"]').fill(user.email);
  await page.locator('input[name="password"]').fill(user.password);
  await page.locator('button[type="submit"]').click();
  await page.waitForURL((url) => url.origin === new URL(seed.baseUrl).origin && !url.pathname.startsWith('/auth'));
  await expect(page.locator('a[href^="/collection/"]').first()).toBeVisible();
}

// Serves the script from a local file instead of the one in the running
// image, to try a change without rebuilding the stack: E2E_BUTTON_JS=../web/button.js.
export async function useLocalScript(page: Page): Promise<void> {
  const file = process.env.E2E_BUTTON_JS;
  if (!file) return;
  await page.route('**/pdf/button.js', (route) => route.fulfill({ path: file, contentType: 'text/javascript' }));
}

// Adds data attributes to the script tag, as the injector does for
// OUTPRINT_BUTTON_*; the dev stack runs with the defaults. The injector marks
// its tag with data-outline-pdf-injector, an edge proxy adds the bare tag.
export async function setScriptAttributes(page: Page, attributes: Record<string, string>): Promise<void> {
  const extra = Object.entries(attributes)
    .map(([name, value]) => ` data-${name}="${value.replace(/&/g, '&amp;').replace(/"/g, '&quot;')}"`)
    .join('');
  await page.route(
    (url) => url.origin === new URL(seed.baseUrl).origin,
    async (route) => {
      if (route.request().resourceType() !== 'document') return route.fallback();
      const response = await route.fetch();
      const body = (await response.text()).replace(
        /(<script src="\/pdf\/button\.js" defer)( data-outline-pdf-injector)?/,
        (_tag, start: string, marker = '') => `${start}${marker}${extra}`,
      );
      return route.fulfill({ response, body });
    },
  );
}

// The Gateway's answer about the open document.
export function statusResponse(page: Page) {
  return page.waitForResponse((r) => new URL(r.url()).pathname.startsWith('/pdf/status/'));
}

// The "…" button of the document header (the sidebar's document tree has
// buttons with the same name, earlier in the page).
export function optionsButton(page: Page) {
  return page.getByRole('button', { name: 'Document options' }).last();
}

export async function openExportSubmenu(page: Page): Promise<void> {
  await optionsButton(page).click();
  await page.getByRole('menuitem', { name: 'Export' }).hover();
  await expect(page.getByRole('menuitem', { name: 'Markdown' })).toBeVisible();
}

// Clicks an item of an open submenu with a gradual pointer move, as a person
// does. Radix closes a submenu when the pointer leaves it in one jump.
export async function clickSubmenuItem(page: Page, item: Locator): Promise<void> {
  const box = await item.boundingBox();
  expect(box, 'the item is on screen').not.toBeNull();
  await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2, { steps: 10 });
  await page.mouse.down();
  await page.mouse.up();
}

// Opens the document menu, then Export, and clicks Formatted PDF.
export async function chooseFormattedPdf(page: Page): Promise<void> {
  await openExportSubmenu(page);
  await clickSubmenuItem(page, page.getByRole('menuitem', { name: MENU_LABEL }));
}

// Outline's own PDF entry in the Export submenu is switched on by
// window.env.PDF_EXPORT_ENABLED in the page. The community image does not set
// it from any environment variable (checked against 1.12.0: the server does not
// read PDF_EXPORT_ENABLED), so the test sets it where the client reads it.
export async function enableNativePdfExport(page: Page): Promise<void> {
  await page.route(
    (url) => url.origin === new URL(seed.baseUrl).origin,
    async (route) => {
      if (route.request().resourceType() !== 'document') return route.fallback();
      const response = await route.fetch();
      const body = (await response.text()).replace('window.env = {', 'window.env = {"PDF_EXPORT_ENABLED":true,');
      return route.fulfill({ response, body });
    },
  );
}

// Sets the language of the signed-in Outline user (for example "de_DE") and
// returns a function that puts the previous one back.
export async function setOutlineLanguage(page: Page, language: string): Promise<() => Promise<void>> {
  const token = (await page.context().cookies()).find((c) => c.name === 'accessToken')?.value;
  expect(token, 'the sign-in left a session cookie').toBeTruthy();
  const headers = { Authorization: `Bearer ${token}` };
  const info = await page.request.post('/api/auth.info', { headers, data: {} });
  const previous: string = (await info.json()).data.user.language ?? 'en_US';
  const update = (value: string) => page.request.post('/api/users.update', { headers, data: { language: value } });
  expect((await update(language)).ok()).toBeTruthy();
  return async () => {
    await update(previous);
  };
}

// Simulates a markup change in Outline: the menu triggers lose the attribute
// the script finds the "…" button by.
export async function breakMenuAnchor(page: Page): Promise<void> {
  await page.addInitScript(() => {
    document.addEventListener('DOMContentLoaded', () => {
      const strip = () => document.querySelectorAll('button[aria-haspopup="menu"]').forEach((b) => b.removeAttribute('aria-haspopup'));
      new MutationObserver(strip).observe(document.body, { childList: true, subtree: true, attributes: true });
      strip();
    });
  });
}
