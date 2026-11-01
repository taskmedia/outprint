import { readFileSync } from 'node:fs';
import { expect, test, type Page } from '@playwright/test';
import { pdfText, squash } from '../support/pdf';
import { loadSeed } from '../support/seed';

const seed = loadSeed();
const BUTTON = '#olp-pdf-button';
const BUTTON_NAME = 'Download this document as PDF';

// Signs in through the dev Dex, the way a person does.
async function signIn(page: Page): Promise<void> {
  // With one sign-in provider, Outline sends a visitor straight to it.
  await page.goto('/');
  await page.locator('input[name="login"]').fill(seed.email);
  await page.locator('input[name="password"]').fill(seed.password);
  await page.locator('button[type="submit"]').click();
  // Back on the Outline origin and signed in.
  await page.waitForURL((url) => url.origin === new URL(seed.baseUrl).origin && !url.pathname.startsWith('/auth'));
  await expect(page.locator('a[href^="/collection/"]').first()).toBeVisible();
}

// The Gateway's answer about the open document, which decides whether the
// button shows. Register it before navigating.
function statusResponse(page: Page) {
  return page.waitForResponse((r) => new URL(r.url()).pathname.startsWith('/pdf/status/'));
}

test('a bound document downloads as a PDF with its text', async ({ page, context }, testInfo) => {
  await signIn(page);

  const status = statusResponse(page);
  await page.goto(seed.bound.documentPath);
  expect((await status).status()).toBe(200);
  const button = page.locator(BUTTON);
  await expect(button).toBeVisible();
  await expect(page.getByRole('button', { name: BUTTON_NAME })).toBeVisible();

  const [download] = await Promise.all([page.waitForEvent('download'), button.click()]);
  expect(download.suggestedFilename()).toMatch(/\.pdf$/);
  const file = testInfo.outputPath('download.pdf');
  await download.saveAs(file);
  const pdf = readFileSync(file);
  expect(pdf.subarray(0, 4).toString('latin1')).toBe('%PDF');

  const text = squash(await pdfText(pdf));
  expect(text).toContain('Thank you for your enquiry about the spring plant sale.');
  expect(text).toContain('Example Garden Society'); // the name in the example Configuration
  expect(text).toContain('Basil');
  // The Metadata Block is a plain yaml fence at the top of the Document: it is
  // read (the subject is typeset) and removed from the body (no raw YAML left).
  expect(text).toContain('Spring plant sale');
  expect(text).not.toContain('type: letter');

  // The Gateway reads a figure caption from the image's alt text. Check on
  // Outline's real export that this is where a caption is stored (docs/internals/images.md,
  // "Captions"). The captioned figures live in the unbound document: the
  // neutral letter cannot typeset a captioned figure yet.
  const token = (await context.cookies()).find((c) => c.name === 'accessToken')?.value;
  expect(token, 'the sign-in left a session cookie').toBeTruthy();
  const info = await page.request.post('/api/documents.info', {
    headers: { Authorization: `Bearer ${token}` },
    data: { id: seed.unbound.documentId },
  });
  expect(info.ok()).toBeTruthy();
  const markdown: string = (await info.json()).data.text;
  for (const caption of seed.captions) {
    expect(markdown).toContain(`![${caption}](/api/attachments.redirect?id=`);
  }
  // The size follows in the title position, in Outline's syntax.
  expect(markdown).toContain('" =240x")');
});

test('a document in a collection without a binding has no button', async ({ page }) => {
  await signIn(page);

  const status = statusResponse(page);
  await page.goto(seed.unbound.documentPath);
  await expect(page).toHaveTitle(new RegExp(seed.unbound.documentTitle));
  const answer = await status;
  expect(await answer.json()).toMatchObject({ available: false });
  await expect(page.locator(BUTTON)).toHaveCount(0);
});
