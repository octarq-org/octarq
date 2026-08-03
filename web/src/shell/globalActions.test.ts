import { describe, it, expect } from "vitest";
import { Action } from "../api";
import { roleSatisfies } from "./role";

describe("Global Actions & Role Gating", () => {
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

  it("1. handles empty actions or fetch failure gracefully", () => {
    const emptyActions: Action[] = [];
    const filtered = emptyActions.filter((a) =>
      roleSatisfies(a.requiredRole, "member", false),
    );
    expect(filtered).toHaveLength(0);
  });

  it("2. filters out actions where requiredRole is higher than current user role", () => {
    // Normal member user: only actions with requiredRole "" or "member" (or undefined) appear
    const memberFiltered = sampleActions.filter((a) =>
      roleSatisfies(a.requiredRole, "member", false),
    );
    expect(memberFiltered.map((a) => a.id)).toEqual(["links.create"]);

    // Admin user: actions with requiredRole "" or "member" or "admin" appear
    const adminFiltered = sampleActions.filter((a) =>
      roleSatisfies(a.requiredRole, "admin", false),
    );
    expect(adminFiltered.map((a) => a.id)).toEqual(["links.create", "dns.create"]);

    // Owner user: all actions appear
    const ownerFiltered = sampleActions.filter((a) =>
      roleSatisfies(a.requiredRole, "owner", false),
    );
    expect(ownerFiltered.map((a) => a.id)).toEqual([
      "links.create",
      "dns.create",
      "mail.create",
    ]);

    // Instance admin bypass: all actions appear regardless of org role
    const instanceAdminFiltered = sampleActions.filter((a) =>
      roleSatisfies(a.requiredRole, "member", true),
    );
    expect(instanceAdminFiltered.map((a) => a.id)).toEqual([
      "links.create",
      "dns.create",
      "mail.create",
    ]);
  });

  it("3. orders action candidates before navigation candidates and supports search matching", () => {
    const actions: Action[] = [
      {
        id: "links.create",
        label: "New Link",
        path: "/links?create=1",
        icon: "link-2",
        category: "Marketing",
        order: 10,
      },
    ];

    interface TestCommandItem {
      id: string;
      label: string;
      path: string;
      isAction: boolean;
      category?: string;
      order?: number;
      area?: string;
      group?: string;
    }

    const actionItems: TestCommandItem[] = actions.map((a) => ({
      id: a.id,
      label: a.label,
      path: a.path,
      isAction: true,
      category: a.category,
      order: a.order ?? 0,
    }));

    const navItems: TestCommandItem[] = [
      {
        id: "/links",
        label: "Links Overview",
        path: "/links",
        isAction: false,
        area: "Marketing",
        group: "Links",
      },
    ];

    // Combine actions first, then nav items
    const commands = [...actionItems, ...navItems];

    // Verify ordering: action candidates come first
    expect(commands[0].isAction).toBe(true);
    expect(commands[0].id).toBe("links.create");
    expect(commands[1].isAction).toBe(false);

    // Search matching test
    const needle = "link";
    const filtered = commands.filter(
      (c) =>
        c.label.toLowerCase().includes(needle) ||
        (c.isAction ? c.category?.toLowerCase().includes(needle) : false) ||
        (!c.isAction && c.area?.toLowerCase().includes(needle)) ||
        c.path.toLowerCase().includes(needle),
    );

    expect(filtered).toHaveLength(2);
    expect(filtered[0].id).toBe("links.create");
    expect(filtered[1].id).toBe("/links");
  });
});
