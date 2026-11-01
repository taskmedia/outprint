// Compresses the raw screenshots into docs/assets/screens/.
//
// The PNGs are palette-quantised with libimagequant (through sharp, which
// ships it) and then recompressed losslessly with oxipng (@napi-rs/image),
// like the logo kit. Quantisation is LOSSY: 256 colours per image, with
// dithering off so that flat UI areas stay flat and two runs on the same
// input give the same bytes. Each file must end up under MAX_BYTES, or this
// script fails.
import { mkdirSync, readdirSync, statSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { losslessCompressPng } from '@napi-rs/image';
import sharp from 'sharp';

const RAW_DIR = resolve(process.env.SCREENSHOTS_RAW_DIR ?? 'test-results/screens-raw');
const OUT_DIR = resolve(process.env.SCREENSHOTS_OUT_DIR ?? '../docs/assets/screens');
const MAX_BYTES = 150 * 1024;
const EXPECTED = [
  'bound-document-with-button.png',
  'unbound-document-no-button.png',
  'document-metadata-block.png',
  'collection-binding-block.png',
  'error-shown-to-author.png',
  'example-letter.png',
];

// The letter prints the date of the render, which would change the PNG every
// day. The date is painted over with paper white (page 910x1287 at 110 dpi),
// so the committed file never depends on when it was made.
const LETTER = 'example-letter.png';
const DATE_BOX = { left: 600, top: 428, width: 190, height: 28 };
const DATE_COVER = await sharp({
  create: { width: DATE_BOX.width, height: DATE_BOX.height, channels: 3, background: '#ffffff' },
}).png().toBuffer();

const present =new Set(readdirSync(RAW_DIR));
const missing = EXPECTED.filter((name) => !present.has(name));
if (missing.length > 0) {
  console.error(`missing raw screenshots in ${RAW_DIR}: ${missing.join(', ')}`);
  process.exit(1);
}

mkdirSync(OUT_DIR, { recursive: true });
let failed = false;
for (const name of EXPECTED) {
  let input = join(RAW_DIR, name);
  if (name === LETTER) {
    input = await sharp(input).composite([{ input: DATE_COVER, left: DATE_BOX.left, top: DATE_BOX.top }]).png().toBuffer();
  }
  const quantised = await sharp(input)
    .png({ palette: true, quality: 90, colours: 256, dither: 0, effort: 10, compressionLevel: 9 })
    .toBuffer();
  const data = await losslessCompressPng(quantised);
  writeFileSync(join(OUT_DIR, name), data);
  const { width, height } = await sharp(data).metadata();
  const kb = (statSync(join(OUT_DIR, name)).size / 1024).toFixed(1);
  console.log(`${name.padEnd(34)} ${width}x${height}  ${kb} KB`);
  if (data.length > MAX_BYTES) {
    console.error(`${name} is larger than ${MAX_BYTES / 1024} KB`);
    failed = true;
  }
}
process.exit(failed ? 1 : 0);
