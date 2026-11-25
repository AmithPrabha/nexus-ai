import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./tests",
  workers: 1,
  timeout: 45_000,
  retries: 0,
  use: {
    baseURL: process.env.WEB_URL || "http://localhost:3000",
    channel: process.env.PW_CHANNEL || undefined,
  },
});
