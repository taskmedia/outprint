// Makes a fresh docs snapshot lean: deletes from <dir> (a versioned_docs/
// version-X.Y folder) every file that the site does not publish. `docs:version`
// copies the whole docs folder, including what `docs.exclude` in
// docusaurus.config.js keeps out of the build. The list is read from the config,
// so there is one source. The logo kit (assets/logo) is removed as well: the
// config excludes the whole assets/ folder from pages, but the screens are
// referenced by pages and must stay.
//
//   node scripts/prune-snapshot.mjs <versioned_docs/version-X.Y>
import {readdir, rm} from 'node:fs/promises';
import {createRequire} from 'node:module';
import {resolve} from 'node:path';

const require = createRequire(import.meta.url);
const {Globby} = require('@docusaurus/utils');
const config = require('../docusaurus.config.js');

const ASSETS_WHOLE = 'assets/**';
const LOGO_KIT = 'assets/logo';

const dir = process.argv[2] ? resolve(process.argv[2]) : '';
if (!dir) {
  console.error('usage: prune-snapshot.mjs <versioned_docs/version-X.Y>');
  process.exit(2);
}

const {exclude} = config.presets[0][1].docs;
const patterns = exclude.filter((pattern) => pattern !== ASSETS_WHOLE);

const files = await Globby(patterns, {cwd: dir, dot: true, onlyFiles: true, followSymbolicLinks: false});
for (const file of files) {
  await rm(resolve(dir, file));
}
await rm(resolve(dir, LOGO_KIT), {recursive: true, force: true});

async function removeEmptyDirs(folder) {
  for (const entry of await readdir(folder, {withFileTypes: true})) {
    if (entry.isDirectory()) await removeEmptyDirs(resolve(folder, entry.name));
  }
  if (folder !== dir && (await readdir(folder)).length === 0) await rm(folder, {recursive: true});
}
await removeEmptyDirs(dir);

console.log(`pruned the logo kit and ${files.length} unpublished files from ${dir}`);
