// Copies the logo files the site needs from the logo kit (docs/assets/logo,
// the single source) into website/static/brand/, which is generated and
// git-ignored. Runs before `start` and `build`. Only the listed files are
// copied, so the kit's build script and lock files never reach the site.
import {copyFile, mkdir} from 'node:fs/promises';
import {dirname, join} from 'node:path';
import {fileURLToPath} from 'node:url';

const kit = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'docs', 'assets', 'logo');
const out = join(dirname(fileURLToPath(import.meta.url)), '..', 'static', 'brand');

const files = [
  'svg/outprint-lockup.svg',
  'svg/outprint-lockup-dark.svg',
  'svg/outprint-banner.svg',
  'svg/outprint-banner-dark.svg',
  'favicon/favicon.svg',
  'favicon/favicon.ico',
  'favicon/apple-touch-icon.png',
  'social/social-preview.png',
];

await mkdir(out, {recursive: true});
for (const file of files) {
  await copyFile(join(kit, file), join(out, file.split('/').pop()));
}
console.log(`brand: copied ${files.length} files to static/brand/`);
