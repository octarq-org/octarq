// @vitest-environment jsdom
import { act } from "react";
import { createRoot, Root } from "react-dom/client";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { McpConnectCard } from "./McpConnectCard";
import { Token } from "../api";
import { I18nProvider } from "../i18n";

// @ts-ignore
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement | null = null;
let root: Root | null = null;

const mockTokens: Token[] = [
  {
    id: 1,
    name: "claude-token",
    prefix: "oct_abc123",
    role: "owner",
    userId: 1,
    lastUsedAt: null,
    expiresAt: null,
    createdAt: "2026-08-28T00:00:00Z",
    note: "for agent",
  },
];

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(() => {
  if (root) {
    act(() => {
      root?.unmount();
    });
  }
  if (container) {
    container.remove();
  }
  container = null;
  root = null;
  vi.restoreAllMocks();
});

describe("McpConnectCard component", () => {
  it("renders MCP title, badge, and tabs", async () => {
    await act(async () => {
      root?.render(
        <I18nProvider>
          <MemoryRouter>
            <McpConnectCard tokens={mockTokens} />
          </MemoryRouter>
        </I18nProvider>
      );
    });

    expect(container?.textContent).toContain("Connect AI Agent (MCP)");
    expect(container?.textContent).toContain("Agent-Native");
    expect(container?.textContent).toContain("Cursor / VS Code");
    expect(container?.textContent).toContain("Claude Desktop");
    expect(container?.textContent).toContain("Claude Code CLI");
  });

  it("switches client tabs and renders appropriate snippets", async () => {
    await act(async () => {
      root?.render(
        <I18nProvider>
          <MemoryRouter>
            <McpConnectCard tokens={mockTokens} />
          </MemoryRouter>
        </I18nProvider>
      );
    });

    // Default is cursor
    expect(container?.textContent).toContain(".cursor/mcp.json");
    expect(container?.textContent).toContain("/api/mcp/sse");

    // Click Claude Desktop
    const claudeTab = Array.from(container?.querySelectorAll("button") || []).find((b) =>
      b.textContent?.includes("Claude Desktop")
    );
    expect(claudeTab).toBeDefined();

    await act(async () => {
      claudeTab?.click();
    });

    expect(container?.textContent).toContain("claude_desktop_config.json");
    expect(container?.textContent).toContain('"command": "octarq"');
    expect(container?.textContent).toContain('"OCTARQ_DB_DSN": "/path/to/octarq.db"');
  });

  it("injects selected token prefix into config", async () => {
    await act(async () => {
      root?.render(
        <I18nProvider>
          <MemoryRouter>
            <McpConnectCard tokens={mockTokens} />
          </MemoryRouter>
        </I18nProvider>
      );
    });

    const select = container?.querySelector("select");
    expect(select).toBeDefined();

    await act(async () => {
      if (select) {
        select.value = "oct_abc123";
        select.dispatchEvent(new Event("change", { bubbles: true }));
      }
    });

    expect(container?.textContent).toContain("Bearer oct_abc123…");
  });
});
