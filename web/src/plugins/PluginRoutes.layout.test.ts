// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import { routeLayoutFor } from "./PluginRoutes";

const routes = [
  { path: "/workspace", layout: "full" as const },
  { path: "/workspace/inbox", layout: "full" as const },
  { path: "/workspace/chat/:agentId", layout: "full" as const },
  { path: "/workspace/settings" },
  { path: "/licenses", layout: "full" as const },
];

describe("routeLayoutFor", () => {
  it("gives a full route the viewport", () => {
    expect(routeLayoutFor(routes, "/workspace")).toBe("full");
    expect(routeLayoutFor(routes, "/workspace/chat/bot-1")).toBe("full");
  });

  it("keeps the shell for a route that did not ask", () => {
    expect(routeLayoutFor(routes, "/workspace/settings")).toBe("default");
    expect(routeLayoutFor(routes, "/overview")).toBe("default");
  });

  it("does not let a prefix claim a longer path", () => {
    expect(routeLayoutFor([{ path: "/workspace", layout: "full" }], "/workspace/tasks")).toBe("default");
  });
});
