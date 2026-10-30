// @vitest-environment happy-dom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import { I18nProvider } from "../i18n";
import { CommandPalette, ThinkingCollapsible } from "./CommandPalette";
import { Area } from "./areas";
import { Action } from "../api";

describe("CommandPalette", () => {
  const dummyAreas: Area[] = [
    {
      id: "operations",
      title: "Operations",
      subtitle: "Daily traffic",
      Icon: () => null,
      groups: [
        {
          label: "Marketing",
          items: [
            { id: "links", label: "Links", path: "/links", Icon: () => null },
          ],
        },
      ],
    },
  ];

  const dummySettingsArea: Area = {
    id: "settings",
    title: "Settings",
    subtitle: "Configurations",
    Icon: () => null,
    groups: [
      {
        label: "Account",
        items: [
          { id: "profile", label: "Profile", path: "/settings/profile", Icon: () => null },
        ],
      },
    ],
  };

  const dummyActions: Action[] = [
    {
      id: "create-link",
      label: "New Link",
      path: "/links?create=1",
      icon: "link-2",
      category: "Marketing",
    },
  ];

  const createMockSSEStream = (chunks: string[]) => {
    const encoder = new TextEncoder();
    return new ReadableStream({
      start(controller) {
        for (const chunk of chunks) {
          controller.enqueue(encoder.encode(chunk));
        }
        controller.close();
      },
    });
  };

  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("1. renders search mode with commands and performs navigation", () => {
    const onNavigate = vi.fn();
    const onClose = vi.fn();

    render(
      <I18nProvider>
        <CommandPalette
          open={true}
          onClose={onClose}
          areas={dummyAreas}
          settingsArea={dummySettingsArea}
          onNavigate={onNavigate}
          actions={dummyActions}
        />
      </I18nProvider>,
    );

    expect(screen.getByPlaceholderText("Search pages…")).not.toBeNull();
    expect(screen.getByText("New link")).not.toBeNull();
    expect(screen.getByText("Links")).not.toBeNull();

    // Click navigation item
    fireEvent.click(screen.getByText("Links"));
    expect(onNavigate).toHaveBeenCalledWith("/links");
  });

  it("2. switches to chat mode when search input begins with / or ?", () => {
    const onNavigate = vi.fn();
    const onClose = vi.fn();

    render(
      <I18nProvider>
        <CommandPalette
          open={true}
          onClose={onClose}
          areas={dummyAreas}
          settingsArea={dummySettingsArea}
          onNavigate={onNavigate}
          actions={dummyActions}
        />
      </I18nProvider>,
    );

    const input = screen.getByPlaceholderText("Search pages…");
    fireEvent.change(input, { target: { value: "/help me configure DNS" } });

    // Should switch to chat mode
    expect(screen.getByText("AI Chat")).not.toBeNull();
    const textarea = screen.getByLabelText("AI chat message input") as HTMLTextAreaElement;
    expect(textarea.value).toBe("help me configure DNS");
  });

  it("3. switches to chat mode via Ask AI button and supports Esc to return to search", () => {
    const onNavigate = vi.fn();
    const onClose = vi.fn();

    render(
      <I18nProvider>
        <CommandPalette
          open={true}
          onClose={onClose}
          areas={dummyAreas}
          settingsArea={dummySettingsArea}
          onNavigate={onNavigate}
          actions={dummyActions}
        />
      </I18nProvider>,
    );

    const askAiBtn = screen.getByRole("button", { name: "Ask AI" });
    fireEvent.click(askAiBtn);

    expect(screen.getByText("AI Chat")).not.toBeNull();

    // Press Escape in textarea to return to search mode
    const textarea = screen.getByLabelText("AI chat message input");
    fireEvent.keyDown(textarea, { key: "Escape" });

    expect(screen.getByPlaceholderText("Search pages…")).not.toBeNull();
  });

  it("4. sends message, streams SSE (thinking, text, A2UI tool), and renders content", async () => {
    const onNavigate = vi.fn();
    const onClose = vi.fn();

    const streamChunks = [
      'data: {"type":"THINKING_TEXT_MESSAGE_CONTENT","delta":"Analyzing traffic data..."}\n\n',
      'data: {"type":"TEXT_MESSAGE_CONTENT","messageId":"m1","delta":"Here is the summary of link visits:"}\n\n',
      'data: {"type":"TOOL_CALL_START","toolCallId":"c1","toolCallName":"render_chart"}\n\n',
      'data: {"type":"TOOL_CALL_ARGS","toolCallId":"c1","delta":"{\\"kind\\":\\"chart\\",\\"title\\":\\"Visit Stats\\",\\"data\\":{\\"labels\\":[\\"Mon\\",\\"Tue\\"],\\"series\\":[{\\"name\\":\\"Visits\\",\\"values\\":[120,250]}]}}"}\n\n',
      'data: {"type":"TOOL_CALL_RESULT","messageId":"m1","toolCallId":"c1","content":"ok"}\n\n',
      'data: {"type":"RUN_FINISHED","threadId":"t1","runId":"r1"}\n\n',
    ];

    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      statusText: "OK",
      body: createMockSSEStream(streamChunks),
    });
    global.fetch = mockFetch;

    render(
      <I18nProvider>
        <CommandPalette
          open={true}
          onClose={onClose}
          areas={dummyAreas}
          settingsArea={dummySettingsArea}
          onNavigate={onNavigate}
          actions={dummyActions}
        />
      </I18nProvider>,
    );

    // Switch to chat mode
    fireEvent.click(screen.getByRole("button", { name: "Ask AI" }));

    const textarea = screen.getByLabelText("AI chat message input");
    fireEvent.change(textarea, { target: { value: "Show link stats" } });
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: false });

    // Verify fetch call payload
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/ai/chat/stream",
        expect.objectContaining({
          method: "POST",
          headers: expect.objectContaining({ "Content-Type": "application/json" }),
          body: expect.any(String),
        }),
      );
    });

    // Wait for streamed response to render
    await waitFor(() => {
      expect(screen.getByText("Here is the summary of link visits:")).not.toBeNull();
      expect(screen.getByText("Visit Stats")).not.toBeNull();
      expect(screen.getByText("Mon")).not.toBeNull();
      expect(screen.getByText("Tue")).not.toBeNull();
      // Usage arrives with the finished run, not with each delta.
    });
  });

  it("5. handles stream error gracefully", async () => {
    const onNavigate = vi.fn();
    const onClose = vi.fn();

    const mockFetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      statusText: "Internal Server Error",
    });
    global.fetch = mockFetch;

    render(
      <I18nProvider>
        <CommandPalette
          open={true}
          onClose={onClose}
          areas={dummyAreas}
          settingsArea={dummySettingsArea}
          onNavigate={onNavigate}
          actions={dummyActions}
        />
      </I18nProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Ask AI" }));

    const textarea = screen.getByLabelText("AI chat message input");
    fireEvent.change(textarea, { target: { value: "Trigger error" } });
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: false });

    await waitFor(() => {
      expect(screen.getByRole("alert")).not.toBeNull();
      expect(screen.getByText(/Failed to get response from AI/)).not.toBeNull();
    });
  });

  it("6. aborts in-flight request when component unmounts or dialog closes", async () => {
    const onNavigate = vi.fn();
    const onClose = vi.fn();

    let streamSignal: AbortSignal | undefined;
    const mockFetch = vi.fn().mockImplementation((_url, opts) => {
      streamSignal = opts?.signal;
      return new Promise(() => {}); // Never resolves to keep it in-flight
    });
    global.fetch = mockFetch;

    const { unmount } = render(
      <I18nProvider>
        <CommandPalette
          open={true}
          onClose={onClose}
          areas={dummyAreas}
          settingsArea={dummySettingsArea}
          onNavigate={onNavigate}
          actions={dummyActions}
        />
      </I18nProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Ask AI" }));

    const textarea = screen.getByLabelText("AI chat message input");
    fireEvent.change(textarea, { target: { value: "Slow stream" } });
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: false });

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalled();
    });

    expect(streamSignal?.aborted).toBe(false);

    // Unmount triggers abort()
    unmount();
    expect(streamSignal?.aborted).toBe(true);
  });

  it("7. executes actionHandler on Enter for quick action items and closes dialog without calling onNavigate", () => {
    const onNavigate = vi.fn();
    const onClose = vi.fn();

    const openLinkSpy = vi.fn();
    const customListener = () => openLinkSpy();
    window.addEventListener("octarq:open-new-link", customListener);

    render(
      <I18nProvider>
        <CommandPalette
          open={true}
          onClose={onClose}
          areas={dummyAreas}
          settingsArea={dummySettingsArea}
          onNavigate={onNavigate}
          actions={dummyActions}
        />
      </I18nProvider>,
    );

    const input = screen.getByPlaceholderText("Search pages…");
    // Filter to the builtin "Create short link"
    fireEvent.change(input, { target: { value: "Create short link" } });

    // Press Enter to trigger the selected item
    fireEvent.keyDown(input, { key: "Enter" });

    // actionHandler dispatched custom event and called onClose
    expect(openLinkSpy).toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();

    window.removeEventListener("octarq:open-new-link", customListener);
  });

  it("8. executes actionHandler on click for quick action items", () => {
    const onNavigate = vi.fn();
    const onClose = vi.fn();

    const exportSpy = vi.fn();
    const customListener = () => exportSpy();
    window.addEventListener("octarq:export-data", customListener);

    render(
      <I18nProvider>
        <CommandPalette
          open={true}
          onClose={onClose}
          areas={dummyAreas}
          settingsArea={dummySettingsArea}
          onNavigate={onNavigate}
          actions={dummyActions}
        />
      </I18nProvider>,
    );

    const input = screen.getByPlaceholderText("Search pages…");
    fireEvent.change(input, { target: { value: "Export workspace data" } });

    const itemBtn = screen.getByText("Export workspace data");
    fireEvent.click(itemBtn);

    expect(exportSpy).toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();

    window.removeEventListener("octarq:export-data", customListener);
  });

  it("9. triggers open-copilot event when action.ask_copilot is executed", () => {
    const onNavigate = vi.fn();
    const onClose = vi.fn();

    const copilotSpy = vi.fn();
    const customListener = () => copilotSpy();
    window.addEventListener("octarq:open-copilot", customListener);

    render(
      <I18nProvider>
        <CommandPalette
          open={true}
          onClose={onClose}
          areas={dummyAreas}
          settingsArea={dummySettingsArea}
          onNavigate={onNavigate}
          actions={dummyActions}
        />
      </I18nProvider>,
    );

    const input = screen.getByPlaceholderText("Search pages…");
    fireEvent.change(input, { target: { value: "Open AI Copilot" } });

    fireEvent.keyDown(input, { key: "Enter" });

    expect(copilotSpy).toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();

    window.removeEventListener("octarq:open-copilot", customListener);
  });

  it("10. leaves attribution to the server and explains a billing refusal", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 409,
      statusText: "Conflict",
      headers: { get: () => "application/json" },
      json: async () => ({
        code: "workforce_budget_exceeded",
        message: "The agent budget is exhausted.",
      }),
    });
    global.fetch = mockFetch as unknown as typeof fetch;

    render(
      <I18nProvider>
        <CommandPalette
          open={true}
          onClose={vi.fn()}
          areas={dummyAreas}
          settingsArea={dummySettingsArea}
          onNavigate={vi.fn()}
          actions={dummyActions}
        />
      </I18nProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Ask AI" }));
    const textarea = screen.getByLabelText("AI chat message input");
    fireEvent.change(textarea, { target: { value: "Show link stats" } });
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: false });

    // The palette names no agent: the server bills the workspace default.
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/ai/chat/stream",
        expect.objectContaining({
          body: expect.any(String),
        }),
      );
      const chatCall = mockFetch.mock.calls.find(([url]) => url === "/api/ai/chat/stream");
      const body = JSON.parse(String(chatCall![1].body));
      expect(body.messages).toEqual([{ role: "user", content: "Show link stats" }]);
      expect(body).not.toHaveProperty("agentId");
    });

    // A refusal becomes the next action, not the raw server prose.
    await waitFor(() => {
      expect(screen.getByText(/This agent reached its monthly budget limit/)).not.toBeNull();
    });
  });
});

describe("ThinkingCollapsible", () => {
  afterEach(() => {
    cleanup();
  });

  it("is collapsed by default and expands upon click or Enter key", () => {
    render(
      <I18nProvider>
        <ThinkingCollapsible thinking="Detailed internal reasoning step 1, 2, 3" tokens={128} />
      </I18nProvider>,
    );

    expect(screen.getByText("Thinking")).not.toBeNull();
    expect(screen.getByText("(128 tokens)")).not.toBeNull();

    // Default collapsed: thinking content is not rendered in the DOM
    expect(screen.queryByText("Detailed internal reasoning step 1, 2, 3")).toBeNull();

    // Click to expand
    const trigger = screen.getByRole("button", { name: /Thinking/ });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(trigger);

    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("Detailed internal reasoning step 1, 2, 3")).not.toBeNull();

    // Press Enter to collapse again
    fireEvent.keyDown(trigger, { key: "Enter" });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText("Detailed internal reasoning step 1, 2, 3")).toBeNull();
  });
});
