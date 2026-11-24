// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "../../../i18n";
import { api } from "../../../api";
import { useAICopilotStore } from "../../../shell/AICopilotDrawer";
import {
  ASK_HELP_EXCERPT_LIMIT,
  AskHelp,
  buildHelpPrompt,
  helpExcerpt,
} from "./AskHelp";

// @ts-ignore
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const HTML = "<h1>Start</h1><p>Create your first workspace.</p>";

function renderAsk() {
  return render(
    <I18nProvider>
      <AskHelp slug="quickstart" lang="en" html={HTML} />
    </I18nProvider>,
  );
}

beforeEach(() => {
  useAICopilotStore.getState().resetDrawer();
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  useAICopilotStore.getState().resetDrawer();
});

describe("AskHelp", () => {
  it("renders nothing when no model is configured", async () => {
    vi.spyOn(api, "aiAssistStatus").mockResolvedValue({ configured: false, provider: "" });
    renderAsk();

    await waitFor(() => expect(api.aiAssistStatus).toHaveBeenCalledTimes(1));
    expect(screen.queryByTestId("help-ask")).toBeNull();
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  it("renders nothing when the status request fails", async () => {
    vi.spyOn(api, "aiAssistStatus").mockRejectedValue(new Error("401 Unauthorized"));
    renderAsk();

    await waitFor(() => expect(api.aiAssistStatus).toHaveBeenCalledTimes(1));
    expect(screen.queryByTestId("help-ask")).toBeNull();
  });

  it("seeds the copilot with the article context and the question", async () => {
    vi.spyOn(api, "aiAssistStatus").mockResolvedValue({ configured: true, provider: "openai" });
    renderAsk();

    const input = await screen.findByRole("textbox", { name: "Question about this page" });
    fireEvent.change(input, { target: { value: "How do I start?" } });
    fireEvent.click(screen.getByRole("button", { name: "Ask" }));

    const state = useAICopilotStore.getState();
    expect(state.isOpen).toBe(true);
    expect(state.messages).toHaveLength(1);
    expect(state.messages[0].role).toBe("user");
    expect(state.messages[0].content).toContain("Article: quickstart");
    expect(state.messages[0].content).toContain("Locale: en");
    expect(state.messages[0].content).toContain("Create your first workspace.");
    expect(state.messages[0].content).toContain("Question: How do I start?");
  });

  it("truncates long article text and marks the cut", () => {
    const html = `<p>${"word ".repeat(3000)}</p>`;
    const excerpt = helpExcerpt(html);

    expect(excerpt.endsWith("[truncated]")).toBe(true);
    expect(excerpt.slice(0, -"\n[truncated]".length)).toHaveLength(ASK_HELP_EXCERPT_LIMIT);

    const prompt = buildHelpPrompt({ slug: "long-doc", lang: "zh", html, question: "Why?" });
    expect(prompt).toContain("Article: long-doc");
    expect(prompt).toContain("Locale: zh");
    expect(prompt).toContain("Question: Why?");
    expect(prompt).toContain("[truncated]");
  });

  it("leaves short article text unmarked", () => {
    const excerpt = helpExcerpt(HTML);
    expect(excerpt).not.toContain("[truncated]");
    expect(excerpt).toBe("Start Create your first workspace.");
  });
});
