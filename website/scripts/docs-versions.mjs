// Builds the older docs versions from Git instead of from committed copies.
//
// For every released minor (a tag vX.Y.0, no pre-release) this exports docs/
// from its source ref into versioned_docs/version-X.Y, writes the version's
// sidebar and versions.json. The source ref is the branch docs-release/X.Y when
// it exists (the way to correct old docs), otherwise the tag vX.Y.0. Everything
// written is gitignored and exists only for the build; docusaurus.config.js
// reads versions.json as before. See website/README.md, "Versions".
//
//   node scripts/docs-versions.mjs
import {execFileSync} from 'node:child_process';
import {mkdirSync, mkdtempSync, readdirSync, rmSync, writeFileSync} from 'node:fs';
import {createRequire} from 'node:module';
import {tmpdir} from 'node:os';
import {dirname, join, resolve} from 'node:path';
import {fileURLToPath, pathToFileURL} from 'node:url';

const require = createRequire(import.meta.url);

const RELEASE_TAG = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.0$/;
const BRANCH_PREFIX = 'docs-release/';
const LOGO_KIT = 'assets/logo';

/** @param {string[]} tags @returns {string[]} minors "X.Y", newest first */
export function releasedMinors(tags) {
  return tags
    .map((tag) => RELEASE_TAG.exec(tag))
    .filter(Boolean)
    .map(([, major, minor]) => [Number(major), Number(minor)])
    .sort((a, b) => b[0] - a[0] || b[1] - a[1])
    .map(([major, minor]) => `${major}.${minor}`);
}

/** @param {string} minor @param {string[]} branches names without remote @returns {string} */
export function pickRef(minor, branches) {
  const branch = `${BRANCH_PREFIX}${minor}`;
  return branches.includes(branch) ? branch : `v${minor}.0`;
}

const git = (repoRoot, ...args) =>
  execFileSync('git', ['-C', repoRoot, ...args], {
    encoding: 'utf8',
    maxBuffer: 256 * 1024 * 1024,
    stdio: ['ignore', 'pipe', 'pipe'],
  });

function listTags(repoRoot) {
  return git(repoRoot, 'tag', '--list', 'v*').split('\n').filter(Boolean);
}

/** Local and origin branches, without the remote name. */
function listBranches(repoRoot) {
  const refs = git(repoRoot, 'for-each-ref', '--format=%(refname)', 'refs/heads', 'refs/remotes/origin')
    .split('\n')
    .filter(Boolean)
    .map((ref) => ref.replace(/^refs\/(heads|remotes\/origin)\//, ''));
  return [...new Set(refs)];
}

function refExists(repoRoot, ref, path) {
  try {
    git(repoRoot, 'cat-file', '-e', `${ref}:${path}`);
    return true;
  } catch {
    return false;
  }
}

/** Resolves a branch name to a ref git archive accepts: local first, then origin. */
function resolveRef(repoRoot, ref) {
  for (const candidate of [ref, `origin/${ref}`]) {
    try {
      git(repoRoot, 'rev-parse', '--verify', '--quiet', `${candidate}^{commit}`);
      return candidate;
    } catch {
      // try the next candidate
    }
  }
  throw new Error(`docs-versions: ${ref} does not exist (fetch the tags and branches first)`);
}

function exportDocs(repoRoot, ref, dest) {
  if (!refExists(repoRoot, ref, 'docs')) {
    throw new Error(`docs-versions: ${ref} has no docs/ folder; fix it on a branch ${BRANCH_PREFIX}X.Y`);
  }
  mkdirSync(dest, {recursive: true});
  const archive = execFileSync('git', ['-C', repoRoot, 'archive', '--format=tar', ref, 'docs'], {
    maxBuffer: 256 * 1024 * 1024,
  });
  execFileSync('tar', ['-x', '-C', dest, '--strip-components=1', '-f', '-'], {input: archive});
}

/** Deletes what the site does not publish; the screens stay. */
async function prune(dir, exclude) {
  const {Globby} = require('@docusaurus/utils');
  const patterns = exclude.filter((pattern) => pattern !== 'assets/**');
  const files = await Globby(patterns, {cwd: dir, dot: true, onlyFiles: true, followSymbolicLinks: false});
  for (const file of files) rmSync(join(dir, file));
  rmSync(join(dir, LOGO_KIT), {recursive: true, force: true});
  removeEmptyDirs(dir, dir);
}

function removeEmptyDirs(folder, root) {
  for (const entry of readdirSync(folder, {withFileTypes: true})) {
    if (entry.isDirectory()) removeEmptyDirs(join(folder, entry.name), root);
  }
  if (folder !== root && readdirSync(folder).length === 0) rmSync(folder, {recursive: true});
}

/** The sidebar as the ref had it, or the site's own when the ref has none. */
function sidebarOf(repoRoot, siteDir, ref) {
  const scratch = mkdtempSync(join(tmpdir(), 'docs-sidebar-'));
  try {
    const file = join(scratch, 'sidebars.cjs');
    const path = 'website/sidebars.js';
    if (refExists(repoRoot, ref, path)) {
      writeFileSync(file, git(repoRoot, 'show', `${ref}:${path}`));
    } else {
      writeFileSync(file, `module.exports = require(${JSON.stringify(join(siteDir, 'sidebars.js'))});`);
    }
    return require(file);
  } finally {
    rmSync(scratch, {recursive: true, force: true});
  }
}

/**
 * @param {{repoRoot: string, siteDir: string, exclude: string[]}} options
 * @returns {Promise<string[]>} the versions written, newest first
 */
export async function materializeVersions({repoRoot, siteDir, exclude}) {
  const output = ['versioned_docs', 'versioned_sidebars', 'versions.json'].map((name) => join(siteDir, name));
  for (const path of output) rmSync(path, {recursive: true, force: true});

  const minors = releasedMinors(listTags(repoRoot));
  const branches = listBranches(repoRoot);
  for (const minor of minors) {
    const ref = resolveRef(repoRoot, pickRef(minor, branches));
    const docs = join(siteDir, 'versioned_docs', `version-${minor}`);
    exportDocs(repoRoot, ref, docs);
    await prune(docs, exclude);
    mkdirSync(join(siteDir, 'versioned_sidebars'), {recursive: true});
    writeFileSync(
      join(siteDir, 'versioned_sidebars', `version-${minor}-sidebars.json`),
      `${JSON.stringify(sidebarOf(repoRoot, siteDir, ref), null, 2)}\n`,
    );
    console.log(`docs-versions: ${minor} from ${ref}`);
  }
  if (minors.length > 0) writeFileSync(join(siteDir, 'versions.json'), `${JSON.stringify(minors, null, 2)}\n`);
  return minors;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const siteDir = resolve(dirname(fileURLToPath(import.meta.url)), '..');
  const repoRoot = git(siteDir, 'rev-parse', '--show-toplevel').trim();
  // versions.json is removed first: the config reads it while loading.
  for (const name of ['versioned_docs', 'versioned_sidebars', 'versions.json']) {
    rmSync(join(siteDir, name), {recursive: true, force: true});
  }
  const {exclude} = require(join(siteDir, 'docusaurus.config.js')).presets[0][1].docs;
  const minors = await materializeVersions({repoRoot, siteDir, exclude});
  if (minors.length === 0) console.log('docs-versions: no release tag vX.Y.0; only the current docs are built');
}
