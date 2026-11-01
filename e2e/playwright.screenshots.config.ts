import { defineConfig } from '@playwright/test';
import { loadSeed } from './support/seed';

// Documentation screenshots (docs/development.md, "Documentation
// screenshots"). A separate config, so `playwright test` and `task e2e:run` never
// run it. Each shot sets its own viewport, colour scheme and motion.
const seed = loadSeed();

export default defineConfig({
  testDir: './screenshots',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 180_000,
  expect: { timeout: 30_000 },
  reporter: [['list']],
  use: {
    baseURL: seed.baseUrl,
    acceptDownloads: true,
    browserName: 'chromium',
  },
});
