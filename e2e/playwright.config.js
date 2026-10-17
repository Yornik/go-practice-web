// Playwright runs the site with scripts/dev.sh in a fresh state directory
// and drives Chromium with a CDP virtual authenticator. Set E2E_BASE_URL
// and GO_PRACTICE_DEV_STATE to test a site that is already running.
import { defineConfig } from "@playwright/test";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

// The config is evaluated again in each worker; the environment carries
// the directory the main process made, so every process uses the same one.
if (!process.env.GO_PRACTICE_DEV_STATE) {
  process.env.GO_PRACTICE_DEV_STATE = fs.mkdtempSync(path.join(os.tmpdir(), "gpw-e2e-"));
}
const port = process.env.GO_PRACTICE_DEV_PORT || "8080";
const baseURL = process.env.E2E_BASE_URL || `http://localhost:${port}`;

export default defineConfig({
  testDir: "./tests",
  globalSetup: "./global-setup.js",
  // A cold build cache makes the first -race run slow.
  timeout: 240_000,
  expect: { timeout: 15_000 },
  fullyParallel: false,
  workers: 1,
  reporter: [["list"]],
  use: {
    baseURL,
    trace: "retain-on-failure",
  },
  webServer: process.env.E2E_BASE_URL
    ? undefined
    : {
        command: "../scripts/dev.sh",
        url: `http://127.0.0.1:${port}/readyz`,
        timeout: 300_000,
        reuseExistingServer: false,
        stdout: "pipe",
      },
  projects: [{ name: "chromium", use: { browserName: "chromium" } }],
});
