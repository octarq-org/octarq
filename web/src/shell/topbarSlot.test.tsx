// @vitest-environment happy-dom
//
// Guards the `topbar-right` extension point in the REAL TopBar (never a copy —
// a stand-in built here would stay green while TopBar.tsx rots). Two things are
// load-bearing: an empty slot costs nothing visually, and a contributed widget
// sits at the LEFT end of the right-hand cluster, i.e. before the create button.
import { lazy } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, cleanup } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Action } from "../api";
import { I18nProvider } from "../i18n";
import { registerUIPlugin, resetRegistry, LazyPage } from "../plugin-sdk";
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

    // Nothing at all sits between the spacer and the create button: an empty
    // slot must not even add a zero-content container, which would still take
    // the header's gap-3.
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

    // DOM order, not mere presence: moving the slot elsewhere in the header
    // must fail here.
    expect(widget.compareDocumentPosition(create) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});
