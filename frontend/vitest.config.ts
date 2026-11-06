import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    environment: "jsdom",
    setupFiles: ["@testing-library/jest-dom/vitest"],
    include: ["src/**/*.test.{ts,tsx}"],
    coverage: {
      provider: "v8",
      reportOnFailure: true,
      include: ["src/pages/moderator/**", "src/services/appeals*.ts"],
      reporter: ["text", "html"],
    },
  },
});
