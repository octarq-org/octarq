import { describe, it, expect } from "vitest";

// Scope guard for the reserved-slug settings page. The config is one-per-
// deployment (GET/PUT /api/instance-settings, instance-admin gated), so its
// editor belongs in the /instance console — announced by the Go half's
// InstanceMenus() and registered as a UIPlugin.instanceRoutes entry. If it
// ever gets dragged back into the tenant Links shell (the old "settings" tab)
// or the console route stops pointing at /link-settings, this file goes red.
//
// Read the real sources rather than restating them here: a list in this file
// would drift from the pages it claims to guard. Same approach as
// shell/navHardcoding.test.ts.
const SOURCES = import.meta.glob("./{index.ts,pages/index.tsx}", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

const TENANT_PAGE = "./pages/index.tsx";
const PLUGIN_ENTRY = "./index.ts";

function sourceFor(key: string): string {
  const hit = Object.entries(SOURCES).find(([k]) => k === key);
  if (!hit) throw new Error(`import.meta.glob resolved no source at "${key}"`);
  return hit[1];
}

describe("links instance scope", () => {
  it("keeps instance-scope settings out of the tenant Links page", () => {
    const page = sourceFor(TENANT_PAGE);

    // The instance-scope editor (LinkSettings / InstanceLinkSettings) must not
    // be referenced from the tenant page — import or otherwise.
    expect(page).not.toMatch(/\bLinkSettings\b/);
    // Nor any "settings" tab: no tab state, no tab-switch buttons, no branch.
    expect(page).not.toMatch(/tab\s*===\s*'settings'/);
    expect(page).not.toMatch(/setTab\s*\(\s*'settings'/);
  });

  it("registers the instance console route at /link-settings", () => {
    const entry = sourceFor(PLUGIN_ENTRY);

    expect(entry).toMatch(/instanceRoutes\s*:/);
    expect(entry).toMatch(/path:\s*"\/link-settings"/);
  });
});