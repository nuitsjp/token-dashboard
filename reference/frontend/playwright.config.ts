import { defineConfig, devices } from '@playwright/test';
export default defineConfig({
  testDir: './tests/e2e', fullyParallel: false, workers: 1, retries: 0,
  use: {
    baseURL: 'http://127.0.0.1:34115', ...devices['Desktop Chrome'], headless: true, trace: 'retain-on-failure',
    launchOptions: {
      executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH,
      ignoreDefaultArgs: process.env.PLAYWRIGHT_IGNORE_DISABLE_EXTENSIONS === '1' ? ['--disable-extensions'] : undefined,
    },
  },
  webServer: { command: 'node ../scripts/e2e-server.mjs', url: 'http://127.0.0.1:34115/health', reuseExistingServer: false, timeout: 60_000 },
  reporter: [['list']], outputDir: 'test-results',
});
