import { defineConfig } from "@playwright/test"

export default defineConfig({
  testDir: "./visual",
  testMatch: "*.spec.mjs",
  reporter: [["list"], ["./reporters/journey.mjs"]],
  projects: [{ name: "desktop", use: { viewport: { width: 1440, height: 900 } } }],
  webServer: [
    { command: "node serve.mjs", url: "http://127.0.0.1:8875" },
    { command: "node harness/api-stub.mjs", url: "http://127.0.0.1:8876" },
  ],
})
