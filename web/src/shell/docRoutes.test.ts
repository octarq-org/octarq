import { describe, expect, it } from "vitest";
import { createDocRoutes } from "./docRoutes";
import type { HelpDocMeta } from "../api";

function doc(slug: string, routes: string[]): HelpDocMeta {
  return { slug, title: slug, category: "settings", order: 1, routes };
}

describe("createDocRoutes", () => {
  it("matches the longest registered prefix", () => {
    const routes = createDocRoutes([
      doc("domains", ["/domains"]),
      doc("domain-dns", ["/domains/dns"]),
    ]);

    expect(routes.match("/domains/dns/records")?.doc.slug).toBe("domain-dns");
    expect(routes.match("/domains/new")?.doc.slug).toBe("domains");
  });

  it("does not match sibling prefixes", () => {
    const routes = createDocRoutes([doc("links", ["/links"])]);

    expect(routes.match("/links")).toBeDefined();
    expect(routes.match("/links-archive")).toBeUndefined();
  });

  it("ignores root, help, and non-absolute routes", () => {
    const routes = createDocRoutes([doc("bad", ["/", "/help", "domains"])]);

    expect(routes.size).toBe(0);
    expect(routes.match("/domains")).toBeUndefined();
  });

  it("normalizes trailing slashes", () => {
    const routes = createDocRoutes([doc("webhooks", ["/webhooks/"])]);

    expect(routes.match("/webhooks/")?.doc.slug).toBe("webhooks");
    expect(routes.match("/webhooks/42")?.doc.slug).toBe("webhooks");
  });
});
