import { registerUIPlugin, resetRegistry, LazyPage } from "@octarq/plugin-sdk";
// @vitest-environment happy-dom
import { lazy } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, cleanup } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Action } from "../api";
import { I18nProvider } from "../i18n";
import { TopBar } from "./TopBar";

const baseProps = {
  areas: [],
  activeArea: "operations" as const,
  settingsActive: false,
  user: "test@example.com",
  panelCollapsed: false,
  onTogglePanel: vi.fn(),
  onSelectArea: vi.fn(),
  onOpenSettings: vi.fn(),
  onOpenCommand: vi.fn(),
  onLogout: vi.fn(),
};

const actions: Action[] = [
  { id: "create-link", label: "New Link", path: "/links?create=1", icon: "link-2", category: "Marketing" },
];

const badge: LazyPage = lazy(async () => ({
  default: () => <span data-testid="slot-widget">widget</span>,
}));

function renderTopBar() {
  return render(
    <MemoryRouter>
      <I18nProvider>
        <TopBar {...baseProps} actions={actions} />
      </I18nProvider>
    </MemoryRouter>,
  );
}

afterEach(() => {
  cleanup();
  resetRegistry();
});

describe("topbar-right extension slot", () => {
  it("leaves no trace — no wrapper, no gap — when no plugin contributes", () => {
    const { container } = renderTopBar();

    const spacer = container.querySelector("header > .flex-1");
    expect(spacer).not.toBeNull();

    expect(spacer!.nextElementSibling).toBe(screen.getByLabelText("Create"));
  });

  it("renders a contributed widget BEFORE the create button", async () => {
    registerUIPlugin({
      name: "slot-test-plugin",
      routes: [],
      widgets: [{ slot: "topbar-right", Component: badge }],
    });

    renderTopBar();

    const widget = await waitFor(() => screen.getByTestId("slot-widget"));
    const create = screen.getByLabelText("Create");

    expect(widget.compareDocumentPosition(create) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});
