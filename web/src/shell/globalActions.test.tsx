// @vitest-environment happy-dom
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { Action } from "../api";
import { I18nProvider } from "../i18n";
import { visibleActions, mergeCommandItems, CommandPaletteItem } from "./globalActions";
import { TopBar } from "./TopBar";

describe("Global Actions pure helpers", () => {
  const sampleActions: Action[] = [
    {
      id: "links.create",
      label: "New Link",
      path: "/links?create=1",
      icon: "link-2",
      category: "Marketing",
      order: 10,
      requiredRole: "",
    },
    {
      id: "dns.create",
      label: "Add Domain",
      path: "/domains?create=1",
      icon: "globe",
      category: "Network",
      order: 10,
      requiredRole: "admin",
    },
    {
      id: "mail.create",
      label: "New Mailbox",
      path: "/mail?create=1",
      icon: "mail",
      category: "Messaging",
      order: 10,
      requiredRole: "owner",
    },
  ];

  it("1. visibleActions filters actions based on role and instance admin bypass", () => {
    // Normal member: only empty requiredRole or member requiredRole
    const memberActions = visibleActions(sampleActions, "member", false);
    expect(memberActions.map((a) => a.id)).toEqual(["links.create"]);

    // Admin: empty, member, or admin requiredRole
    const adminActions = visibleActions(sampleActions, "admin", false);
    expect(adminActions.map((a) => a.id)).toEqual(["links.create", "dns.create"]);

    // Owner: all actions
    const ownerActions = visibleActions(sampleActions, "owner", false);
    expect(ownerActions.map((a) => a.id)).toEqual(["links.create", "dns.create", "mail.create"]);

    // Instance admin bypass: all actions regardless of role
    const bypassActions = visibleActions(sampleActions, "member", true);
    expect(bypassActions.map((a) => a.id)).toEqual(["links.create", "dns.create", "mail.create"]);
  });

  it("2. mergeCommandItems puts action candidates before navigation candidates and handles matching", () => {
    const navItems: CommandPaletteItem[] = [
      {
        id: "/links",
        label: "Links Page",
        path: "/links",
        isAction: false,
        area: "Marketing",
        group: "Links",
      },
    ];

    const merged = mergeCommandItems(sampleActions, navItems);

    // Actions must be placed before nav items
    expect(merged[0].isAction).toBe(true);
    expect(merged[0].id).toBe("links.create");
    expect(merged[1].id).toBe("dns.create");
    expect(merged[2].id).toBe("mail.create");
    expect(merged[3].isAction).toBe(false);
    expect(merged[3].id).toBe("/links");

    // Search matching test
    const needle = "new link";
    const matched = merged.filter((c) => c.label.toLowerCase().includes(needle));
    expect(matched).toHaveLength(1);
    expect(matched[0].id).toBe("links.create");
  });

  it("3. handles empty or undefined actions safely", () => {
    expect(visibleActions(undefined, "member", false)).toEqual([]);
    expect(visibleActions([], "member", false)).toEqual([]);

    const navItems: CommandPaletteItem[] = [
      { id: "/overview", label: "Overview", path: "/overview", isAction: false },
    ];
    const mergedEmpty = mergeCommandItems(undefined as any, navItems);
    expect(mergedEmpty).toEqual(navItems);
  });
});

describe("TopBar component rendering with actions", () => {
  const dummyProps = {
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

  it("does NOT render create (+) button when actions is empty", () => {
    render(
      <MemoryRouter>
        <I18nProvider>
          <TopBar {...dummyProps} actions={[]} />
        </I18nProvider>
      </MemoryRouter>,
    );
    expect(screen.queryByLabelText("Create")).toBeNull();
  });

  it("renders create (+) button when actions are present", () => {
    const actions: Action[] = [
      {
        id: "links.create",
        label: "New Link",
        path: "/links?create=1",
        icon: "link-2",
        category: "Marketing",
      },
    ];

    render(
      <MemoryRouter>
        <I18nProvider>
          <TopBar {...dummyProps} actions={actions} />
        </I18nProvider>
      </MemoryRouter>,
    );
    expect(screen.getByLabelText("Create")).not.toBeNull();
  });
});
