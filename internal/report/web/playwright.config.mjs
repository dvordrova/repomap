import {defineConfig} from '@playwright/test';

export default defineConfig({
  testDir: './visual',
  testMatch: '*.spec.mjs',
  fullyParallel: true,
  workers: 2,
  timeout: 30_000,
  forbidOnly: !!process.env.CI,
  retries: 0,
  // A missing reference must fail a normal run, including on a new OS.
  updateSnapshots: 'none',
  snapshotPathTemplate: '{testDir}/snapshots/{platform}/{projectName}/{arg}{ext}',
  outputDir: './test-results',
  reporter: [['list'], ['html', {open: 'never'}], ['./visual/journey-reporter.mjs']],
  expect: {timeout: 10_000, toHaveScreenshot: {animations: 'disabled', maxDiffPixels: 0}},
  use: {
    browserName: 'chromium',
    baseURL: 'http://127.0.0.1:8875',
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
    url: 'http://127.0.0.1:8875',
    reuseExistingServer: false,
    timeout: 10_000,
  },
});
