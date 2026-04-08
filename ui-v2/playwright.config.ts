import { defineConfig, devices } from '@playwright/test';

// KAI-307: Playwright config — placeholder. E2E tests will live in ./e2e.
// Tests are not expected to run yet; this exists to satisfy the
// "test:e2e" script and CI scaffolding.
export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'html',
  use: {
    baseURL: 'http://localhost:5174',
    trace: 'on-first-retry',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
});
