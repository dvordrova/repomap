import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["src/**/*.test.ts"],
    exclude: ["src/excluded/**/*.test.ts"],
    globalSetup: ["src/test-setup.ts"],
  },
});
