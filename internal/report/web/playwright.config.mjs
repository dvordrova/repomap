import {defineConfig} from '@playwright/test';

// REPOMAP_TEST_PORT runs a second suite beside one already serving 8875.
const port=Number(process.env.REPOMAP_TEST_PORT||8875);

export default defineConfig({
  testDir: './visual',
  testMatch: '*.spec.mjs',
  fullyParallel: true,
  workers: 2,
  timeout: 30_000,
  forbidOnly: !!process.env.CI,
  retries: 0,
  outputDir: './test-results',
  reporter: [['list'], ['html', {open: 'never'}], ['./visual/journey-reporter.mjs']],
  expect: {timeout: 10_000},
  use: {
    browserName: 'chromium',
    baseURL: `http://127.0.0.1:${port}`,
    deviceScaleFactor: 1,
    colorScheme: 'light',
    locale: 'en-US',
    timezoneId: 'UTC',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    {name: 'desktop', use: {viewport: {width: 1440, height: 900}}},
  ],
  webServer: {
    command: 'node visual/server.mjs',
    url: `http://127.0.0.1:${port}`,
    reuseExistingServer: false,
    timeout: 10_000,
  },
});
