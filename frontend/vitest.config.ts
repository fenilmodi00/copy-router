import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// Vitest shares the tsconfig `@/*` path alias, so component tests can import
// via the app's module specifier (same resolution as next build/tsc).
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": new URL("./src", import.meta.url).pathname,
    },
  },
  test: {
    // globals needed by @testing-library/jest-dom's expect.extend auto-import
    globals: true,
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
  },
});