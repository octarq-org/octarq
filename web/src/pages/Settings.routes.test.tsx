// @vitest-environment happy-dom
import { describe, it, expect, vi, afterEach } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import SettingsPage from "./Settings";

vi.mock("./settings/members", () => ({ OrgMembersManager: () => <div data-testid="members-panel" /> }));
vi.mock("./settings/plugins", () => ({ PluginsSettings: () => <div /> }));
vi.mock("./settings/general", () => ({ GeneralSettings: () => <div data-testid="general-panel" /> }));
vi.mock("./settings/gateway", () => ({ GatewaySettings: () => <div /> }));
vi.mock("./settings/vault/IntegrationsVaultPage", () => ({ IntegrationsVaultPage: () => <div data-testid="integrations-vault-panel" /> }));
vi.mock("./settings/webhooks", () => ({ WebhooksSettings: () => <div /> }));
vi.mock("./settings/notifications", () => ({ NotificationChannels: () => <div /> }));
vi.mock("./settings/auth", () => ({ AuthenticationSettings: () => <div /> }));
vi.mock("./settings/instance-plugins", () => ({ InstancePluginsSettings: () => <div /> }));
vi.mock("./settings/instance", () => ({ InstanceSettings: () => <div /> }));
vi.mock("./settings/security", () => ({ SecuritySettings: () => <div /> }));
vi.mock("./notifications", () => ({ PreferencesPage: () => <div data-testid="preferences-panel" /> }));
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

afterEach(cleanup);

describe("core settings paths that plugin packages link to", () => {
  it("/settings/members resolves to the members panel", async () => {
    renderAt("/settings/members");
    await waitFor(() => expect(screen.getByTestId("members-panel")).toBeTruthy());
  });

  it("does not render the members panel on a different settings path", async () => {
    renderAt("/settings/general");
    await waitFor(() => expect(screen.getByTestId("general-panel")).toBeTruthy());
    expect(screen.queryByTestId("members-panel")).toBeNull();
  });

  it("/settings/personal/notifications resolves to the personal notifications preferences panel", async () => {
    renderAt("/settings/personal/notifications");
    await waitFor(() => expect(screen.getByTestId("preferences-panel")).toBeTruthy());
  });

  it("/settings/integrations resolves to the built-in IntegrationsVaultPage", async () => {
    renderAt("/settings/integrations");
    await waitFor(() => expect(screen.getByTestId("integrations-vault-panel")).toBeTruthy());
  });
});
