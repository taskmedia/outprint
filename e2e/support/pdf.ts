import { spawnSync } from 'node:child_process';

// Text of a PDF. pdftotext (poppler) is used when it is installed, because it
// is what the golden tests use too. Without it, or with E2E_PDF_TEXT=js, the
// text comes from pdfjs-dist.
export async function pdfText(pdf: Buffer): Promise<string> {
  if (process.env.E2E_PDF_TEXT !== 'js') {
    const viaPoppler = pdftotext(pdf);
    if (viaPoppler !== undefined) return viaPoppler;
  }
  return pdfjsText(pdf);
}

function pdftotext(pdf: Buffer): string | undefined {
  const run = spawnSync('pdftotext', ['-layout', '-', '-'], { input: pdf, maxBuffer: 32 << 20 });
  if (run.error || run.status !== 0) return undefined;
  return run.stdout.toString('utf8');
}

async function pdfjsText(pdf: Buffer): Promise<string> {
  const pdfjs = await import('pdfjs-dist/legacy/build/pdf.mjs');
  const task = pdfjs.getDocument({ data: new Uint8Array(pdf), useSystemFonts: false, verbosity: 0 });
  const doc = await task.promise;
  const pages: string[] = [];
  for (let n = 1; n <= doc.numPages; n++) {
    const content = await (await doc.getPage(n)).getTextContent();
    pages.push(content.items.map((item) => ('str' in item ? item.str : '')).join(' '));
  }
  await task.destroy();
  return pages.join('\n');
}

// Line breaks and runs of spaces differ between extractors and layouts.
export function squash(text: string): string {
  return text.replace(/\s+/g, ' ').trim();
}
