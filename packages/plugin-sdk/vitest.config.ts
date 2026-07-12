import { defineConfig } from "vitest/config";

// jsdom because the suite covers <ExtensionSlot/> rendering as well as the
// plain registry functions. Test files live next to the code they test.
export default defineConfig({
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
