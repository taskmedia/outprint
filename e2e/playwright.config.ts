import { defineConfig, devices } from '@playwright/test';
import { loadSeed } from './support/seed';

// Everything goes through the injector origin of the seeded dev stack; the
// one exception is the sign-in page of the dev Dex (localhost:5556).
const seed = loadSeed();

export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  forbidOnly: !!process.env.CI,
  // A render of the example document takes a few seconds; leave room for a
  // cold TeX cache on a slow runner.
  timeout: 120_000,
  expect: { timeout: 30_000 },
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : [['list']],
  use: {
    baseURL: seed.baseUrl,
    acceptDownloads: true,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});
