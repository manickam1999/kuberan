import { existsSync } from "node:fs";
import { defineConfig } from "@playwright/test";

// The suite drives a RUNNING web build against a RUNNING api (see docs/e2e.md).
//   E2E_WEB_URL       e.g. http://localhost:3315
//   E2E_API_URL       e.g. http://localhost:8195 (must be what the web build was
//                     compiled against: NEXT_PUBLIC_API_URL)
//   E2E_PIPELINE_KEY  the api's PIPELINE_API_KEY (seeds FX rates and prices)
const WEB_URL = process.env.E2E_WEB_URL ?? "http://localhost:3315";

// Use the browser Playwright installed; fall back to a system chromium.
const FALLBACK_CHROMIUM = "/opt/pw-browsers/chromium";
const executablePath =
  process.env.E2E_CHROMIUM ??
  (existsSync(FALLBACK_CHROMIUM) ? FALLBACK_CHROMIUM : undefined);

const launchOptions = executablePath ? { executablePath } : {};

export default defineConfig({
  testDir: ".",
  testMatch: /.*\.spec\.ts/,
  globalSetup: "./global-setup.ts",
  // One shared user; specs create the accounts they mutate, but run serially so
  // aggregate pages (insights, portfolio) can be compared with the API.
  workers: 1,
  fullyParallel: false,
  retries: 0,
  timeout: 90_000,
  expect: { timeout: 10_000 },
  reporter: [["list"]],
  outputDir: "./.run/artifacts",
  use: {
    baseURL: WEB_URL,
    trace: "off",
    screenshot: "only-on-failure",
    video: "off",
    launchOptions,
    // Fixed so date-dependent assertions are not flaky around local midnight.
    timezoneId: "UTC",
    locale: "en-MY",
    acceptDownloads: true,
  },
  projects: [
    {
      name: "desktop",
      testIgnore: /.*\.mobile\.spec\.ts/,
      use: { viewport: { width: 1440, height: 900 } },
    },
    {
      name: "mobile",
      testMatch: /.*\.(mobile|both)\.spec\.ts/,
      use: {
        viewport: { width: 390, height: 844 },
        isMobile: true,
        hasTouch: true,
        deviceScaleFactor: 2,
      },
    },
  ],
});
