import { beforeEach, describe, expect, it } from "vitest";
import type {
  ArtifactEditorComponent,
  ToolCardComponent,
  UIPlugin,
} from "./types";
import {
  registerUIPlugin,
  resetRegistry,
  resolveArtifactEditor,
  resolveToolCard,
  uiArtifactEditors,
  uiToolCards,
} from "./registry";

const component = <T,>(marker: string): T =>
  ({ marker }) as unknown as T;

const card = (marker: string): ToolCardComponent => component<ToolCardComponent>(marker);
const editor = (marker: string): ArtifactEditorComponent =>
  component<ArtifactEditorComponent>(marker);

const markerOf = (c: unknown): string => (c as { marker: string }).marker;

const plugin = (name: string, rest: Partial<UIPlugin> = {}): UIPlugin => ({
  name,
  routes: [],
  ...rest,
});

beforeEach(() => {
  resetRegistry();
});

describe("tool cards", () => {
  it("returns null for an unknown tool", () => {
    expect(resolveToolCard("octarq_network__create_link")).toBeNull();
    expect(uiToolCards()).toEqual([]);
  });

  it("resolves an exact tool name", () => {
    const Component = card("link-card");
    registerUIPlugin(
      plugin("links", { cards: [{ match: "octarq_network__create_link", Component }] }),
    );
    expect(resolveToolCard("octarq_network__create_link")).toBe(Component);
    expect(resolveToolCard("octarq_network__update_link")).toBeNull();
    expect(uiToolCards()).toHaveLength(1);
  });

  it("resolves a trailing wildcard and prefers the longest prefix", () => {
    const broad = card("broad");
    const narrow = card("narrow");
    registerUIPlugin(
      plugin("links", {
        cards: [
          { match: "octarq_network__*", Component: narrow },
          { match: "octarq_*", Component: broad },
        ],
      }),
    );
    expect(resolveToolCard("octarq_network__create_link")).toBe(narrow);
    expect(resolveToolCard("octarq_mail__send")).toBe(broad);
    expect(resolveToolCard("other_tool")).toBeNull();
  });

  it("prefers an exact match over a wildcard match", () => {
    const exact = card("exact");
    const wildcard = card("wildcard");
    registerUIPlugin(
      plugin("links", {
        cards: [
          { match: "octarq_network__*", Component: wildcard },
          { match: "octarq_network__create_link", Component: exact },
        ],
      }),
    );
    expect(resolveToolCard("octarq_network__create_link")).toBe(exact);
  });

  it("throws on a duplicate exact match across plugins", () => {
    registerUIPlugin(
      plugin("links", {
        cards: [{ match: "octarq_network__create_link", Component: card("a") }],
      }),
    );
    registerUIPlugin(
      plugin("dns", {
        cards: [{ match: "octarq_network__create_link", Component: card("b") }],
      }),
    );
    expect(() => resolveToolCard("octarq_network__create_link")).toThrow(
      /Tool card match collision/,
    );
  });

  it("throws on a duplicate wildcard across plugins", () => {
    registerUIPlugin(
      plugin("links", { cards: [{ match: "octarq_network__*", Component: card("a") }] }),
    );
    registerUIPlugin(
      plugin("dns", { cards: [{ match: "octarq_network__*", Component: card("b") }] }),
    );
    expect(() => uiToolCards()).toThrow(/Tool card match collision/);
  });

  it("throws on a malformed wildcard that is not a single trailing star", () => {
    registerUIPlugin(
      plugin("links", {
        cards: [{ match: "octarq_*__create_link", Component: card("a") }],
      }),
    );
    expect(() => uiToolCards()).toThrow(/invalid: use an exact tool name or one trailing/);
  });

  it("keeps card registration lazy so a replaced plugin's cards disappear", () => {
    registerUIPlugin(
      plugin("links", {
        cards: [
          { match: "octarq_network__create_link", Component: card("core") },
          { match: "octarq_network__*", Component: card("core-wild") },
        ],
      }),
    );
    registerUIPlugin(
      plugin("linksPro", {
        replaces: ["links"],
        cards: [{ match: "octarq_network__create_link", Component: card("pro") }],
      }),
    );
    expect(markerOf(resolveToolCard("octarq_network__create_link"))).toBe("pro");
    expect(resolveToolCard("octarq_network__update_link")).toBeNull();
  });
});

describe("artifact editors", () => {
  it("returns null for an unknown type and an empty list without registrations", () => {
    expect(resolveArtifactEditor("links.email_draft")).toBeNull();
    expect(uiArtifactEditors()).toEqual([]);
  });

  it("resolves a namespaced artifact type", () => {
    const Component = editor("email-draft");
    registerUIPlugin(
      plugin("mail", { artifacts: [{ type: "mail.email_draft", Component }] }),
    );
    expect(resolveArtifactEditor("mail.email_draft")).toBe(Component);
    expect(resolveArtifactEditor("email_draft")).toBeNull();
    expect(uiArtifactEditors()).toHaveLength(1);
  });

  it("throws when the artifact type is not namespaced by the owning plugin", () => {
    registerUIPlugin(
      plugin("mail", { artifacts: [{ type: "email_draft", Component: editor("a") }] }),
    );
    expect(() => uiArtifactEditors()).toThrow(/must start with "mail\."/);
  });

  it("throws when a plugin uses another plugin's namespace", () => {
    registerUIPlugin(
      plugin("mail", { artifacts: [{ type: "links.email_draft", Component: editor("a") }] }),
    );
    expect(() => resolveArtifactEditor("links.email_draft")).toThrow(
      /must start with "mail\."/,
    );
  });

  it("drops a replaced plugin's artifact editors", () => {
    registerUIPlugin(
      plugin("mail", { artifacts: [{ type: "mail.email_draft", Component: editor("a") }] }),
    );
    registerUIPlugin(plugin("mailPro", { replaces: ["mail"] }));
    expect(resolveArtifactEditor("mail.email_draft")).toBeNull();
    expect(uiArtifactEditors()).toEqual([]);
  });

  it("throws on a duplicate artifact type within one plugin", () => {
    registerUIPlugin(
      plugin("mail", {
        artifacts: [
          { type: "mail.email_draft", Component: editor("a") },
          { type: "mail.email_draft", Component: editor("b") },
        ],
      }),
    );
    expect(() => uiArtifactEditors()).toThrow(/Artifact editor type collision/);
  });
});

describe("resetRegistry", () => {
  it("clears cards and artifacts so they can be re-registered", () => {
    registerUIPlugin(
      plugin("mail", {
        cards: [{ match: "octarq_mail__send", Component: card("a") }],
        artifacts: [{ type: "mail.email_draft", Component: editor("a") }],
      }),
    );
    expect(uiToolCards()).toHaveLength(1);
    expect(uiArtifactEditors()).toHaveLength(1);

    resetRegistry();

    expect(uiToolCards()).toEqual([]);
    expect(uiArtifactEditors()).toEqual([]);
    expect(resolveToolCard("octarq_mail__send")).toBeNull();
    expect(resolveArtifactEditor("mail.email_draft")).toBeNull();

    registerUIPlugin(
      plugin("mail", {
        cards: [{ match: "octarq_mail__send", Component: card("b") }],
        artifacts: [{ type: "mail.email_draft", Component: editor("b") }],
      }),
    );
    expect(markerOf(resolveToolCard("octarq_mail__send"))).toBe("b");
    expect(markerOf(resolveArtifactEditor("mail.email_draft"))).toBe("b");
  });
});
