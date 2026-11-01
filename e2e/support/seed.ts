import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

export interface SeedItem {
  collectionName: string;
  documentId: string;
  documentTitle: string;
  documentPath: string;
}

export interface Seed {
  baseUrl: string;
  email: string;
  password: string;
  bound: SeedItem;
  unbound: SeedItem;
  captions: string[];
}

// The seed command (cmd/seed, run by `make seed` and `make e2e`) writes this
// file; the tests never create content themselves.
export function seedFile(): string {
  return resolve(process.env.E2E_SEED_FILE ?? '../deploy/dev/.generated/seed.json');
}

export function loadSeed(): Seed {
  const file = seedFile();
  try {
    return JSON.parse(readFileSync(file, 'utf8')) as Seed;
  } catch (err) {
    throw new Error(`cannot read the seed result ${file}: run \`make seed\` (or \`make e2e\`) first (${String(err)})`);
  }
}
