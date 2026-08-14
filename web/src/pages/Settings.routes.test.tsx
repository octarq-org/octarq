// @vitest-environment happy-dom
//
// Guard for the core settings paths that OUT-OF-TREE plugin packages link to.
//
// A plugin package cannot import this route table, so octarq-pro's
// packages/plugin-cloud hard-coded "/admin/settings/members" into its own
// shared.paths.test.ts and asserted against its own copy — a test that stays
// green while the link it protects dies. This file is the real guard: it
// renders the route and fails if the path stops resolving here.
//
// The cross-reference is deliberate. plugin-cloud's shared.paths.test.ts names
// this file; if you move or rename a path below, that plugin's link breaks and
// its comment tells the next reader to come here.
import { describe, it, expect, vi, afterEach } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import SettingsPage from "./Settings";

// Each panel is a lazy chunk; stub them so this test exercises ROUTING, not the
// panels themselves. The members panel gets a marker we can look for.
vi.mock("./settings/members", () => ({ OrgMembersManager: () => <div data-testid="members-panel" /> }));
vi.mock("./settings/plugins", () => ({ PluginsSettings: () => <div /> }));
vi.mock("./settings/general", () => ({ GeneralSettings: () => <div data-testid="general-panel" /> }));
vi.mock("./settings/webhooks", () => ({ WebhooksSettings: () => <div /> }));
vi.mock("./settings/notifications", () => ({ NotificationChannels: () => <div /> }));
vi.mock("./settings/auth", () => ({ AuthenticationSettings: () => <div /> }));
vi.mock("./settings/instance-plugins", () => ({ InstancePluginsSettings: () => <div /> }));
vi.mock("./settings/instance", () => ({ InstanceSettings: () => <div /> }));
vi.mock("./settings/security", () => ({ SecuritySettings: () => <div /> }));
vi.mock("./PersonalSettings", () => ({ ProfileSettings: () => <div />, ApiTokens: () => <div /> }));

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/settings/*" element={<SettingsPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

// Auto-cleanup is not globally configured here, so without this the first
// render's DOM leaks into the next test and the negative assertion below is
// meaningless.
afterEach(cleanup);

describe("core settings paths that plugin packages link to", () => {
  it("/settings/members resolves to the members panel", async () => {
    renderAt("/settings/members");
    await waitFor(() => expect(screen.getByTestId("members-panel")).toBeTruthy());
  });

  // Without this the test above could pass vacuously — e.g. if the route table
  // rendered every panel regardless of path, "members is reachable" would be
  // true for reasons that say nothing about the path.
  it("does not render the members panel on a different settings path", async () => {
    renderAt("/settings/general");
    await waitFor(() => expect(screen.getByTestId("general-panel")).toBeTruthy());
    expect(screen.queryByTestId("members-panel")).toBeNull();
  });
});
