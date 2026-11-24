// @vitest-environment happy-dom
import React from "react";
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import { I18nProvider } from "../i18n";
import {
  AICopilotButton,
  AICopilotDrawer,
  useAICopilotStore,
} from "./AICopilotDrawer";

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


describe("AICopilotButton", () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("renders the trigger button with shortcut indicator and toggles drawer on click", () => {
    render(
      <I18nProvider>
        <AICopilotButton />
      </I18nProvider>,
    );

    const btn = screen.getByTestId("ai-copilot-trigger-btn");
    expect(btn).not.toBeNull();
    expect(btn.getAttribute("aria-pressed")).toBe("false");
    expect(screen.getByText("⌘J")).not.toBeNull();

    fireEvent.click(btn);
    expect(useAICopilotStore.getState().isOpen).toBe(true);
    expect(btn.getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(btn);
    expect(useAICopilotStore.getState().isOpen).toBe(false);
  });
});

describe("AICopilotDrawer", () => {
  beforeEach(() => {
    useAICopilotStore.getState().resetDrawer();
    vi.restoreAllMocks();
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it("1. does not render when closed, renders accessible dialog when open", () => {
    const { rerender } = render(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    expect(screen.queryByRole("dialog")).toBeNull();

    useAICopilotStore.getState().openDrawer();
    rerender(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    const dialog = screen.getByRole("dialog");
    expect(dialog).not.toBeNull();
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    expect(screen.getAllByText("AI Copilot").length).toBeGreaterThan(0);
  });

  it("2. renders empty state with 3 quick prompt cards", () => {
    useAICopilotStore.getState().openDrawer();

    render(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    expect(screen.getByTestId("copilot-empty-state")).not.toBeNull();

    expect(screen.getByText(/Review pending approvals/)).not.toBeNull();
    expect(screen.getByText(/Summarize recent activity/)).not.toBeNull();
    expect(screen.getByText(/Check workspace health/)).not.toBeNull();
  });

  it("3. clicking a prompt card immediately sends the message and streams response", async () => {
    const streamChunks = [
      'data: {"type":"TEXT_MESSAGE_CONTENT","messageId":"m1","delta":"DNS records are currently healthy across all edge locations."}\n\n',
      'data: {"type":"RUN_FINISHED","threadId":"t1","runId":"r1"}\n\n',
    ];

    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      statusText: "OK",
      body: createMockSSEStream(streamChunks),
    });
    global.fetch = mockFetch;

    useAICopilotStore.getState().openDrawer();

    render(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    const dnsPill = screen.getByText(/Check workspace health/);
    fireEvent.click(dnsPill);

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/ai/chat/stream",
        expect.objectContaining({
          method: "POST",
          headers: expect.objectContaining({ "Content-Type": "application/json" }),
        }),
      );
    });

    await waitFor(() => {
      expect(
        screen.getByText("DNS records are currently healthy across all edge locations."),
      ).not.toBeNull();
    });
  });

  it("4. supports manual input, Enter to send, Shift+Enter for newline", async () => {
    const streamChunks = [
      'data: {"type":"THINKING_TEXT_MESSAGE_CONTENT","delta":"Analyzing query..."}\n\n',
      'data: {"type":"TEXT_MESSAGE_CONTENT","messageId":"m1","delta":"Here is the response."}\n\n',
      'data: {"type":"RUN_FINISHED","threadId":"t1","runId":"r1"}\n\n',
    ];

    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      statusText: "OK",
      body: createMockSSEStream(streamChunks),
    });
    global.fetch = mockFetch;

    useAICopilotStore.getState().openDrawer();

    render(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    const textarea = screen.getByPlaceholderText(/Ask anything about your workspace/);

    fireEvent.change(textarea, { target: { value: "Line 1" } });
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: true });
    expect(mockFetch.mock.calls.filter(([url]) => url === "/api/ai/chat/stream")).toHaveLength(0);

    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: false });
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalled();
    });

    await waitFor(() => {
      expect(screen.getByText("Line 1")).not.toBeNull();
      expect(screen.getByText("Here is the response.")).not.toBeNull();
    });
  });

  it("5. closes on Escape key, backdrop click, and close button click", () => {
    useAICopilotStore.getState().openDrawer();

    const { rerender } = render(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    const closeBtn = screen.getByRole("button", { name: "Close" });
    fireEvent.click(closeBtn);
    expect(useAICopilotStore.getState().isOpen).toBe(false);

    useAICopilotStore.getState().openDrawer();
    rerender(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );
    const backdrop = screen.getByTestId("copilot-backdrop");
    fireEvent.click(backdrop);
    expect(useAICopilotStore.getState().isOpen).toBe(false);

    useAICopilotStore.getState().openDrawer();
    rerender(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );
    const textarea = screen.getByPlaceholderText(/Ask anything about your workspace/);
    fireEvent.keyDown(textarea, { key: "Escape" });
    expect(useAICopilotStore.getState().isOpen).toBe(false);
  });

  it("6. responds to octarq:open-copilot event and Mod+J / Mod+/ global shortcuts", () => {
    render(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    expect(useAICopilotStore.getState().isOpen).toBe(false);

    window.dispatchEvent(new CustomEvent("octarq:open-copilot"));
    expect(useAICopilotStore.getState().isOpen).toBe(true);

    fireEvent.keyDown(window, { key: "j", metaKey: true });
    expect(useAICopilotStore.getState().isOpen).toBe(false);

    fireEvent.keyDown(window, { key: "j", metaKey: true });
    expect(useAICopilotStore.getState().isOpen).toBe(true);

    fireEvent.keyDown(window, { key: "/", metaKey: true });
    expect(useAICopilotStore.getState().isOpen).toBe(false);
  });

  it("7. handles stream error gracefully with role='alert'", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 503,
      statusText: "Service Unavailable",
    });
    global.fetch = mockFetch;

    useAICopilotStore.getState().openDrawer();

    render(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    const textarea = screen.getByPlaceholderText(/Ask anything about your workspace/);
    fireEvent.change(textarea, { target: { value: "Cause error" } });
    fireEvent.keyDown(textarea, { key: "Enter", shiftKey: false });

    await waitFor(() => {
      const alert = screen.getByRole("alert");
      expect(alert).not.toBeNull();
      expect(screen.getByText(/503/)).not.toBeNull();
    });
  });

  it("8. clear chat clears all history and resets to empty state", () => {
    useAICopilotStore.getState().openDrawer();
    useAICopilotStore.getState().addMessage({
      id: "m-1",
      role: "user",
      content: "Hello AI",
    });

    render(
      <I18nProvider>
        <AICopilotDrawer />
      </I18nProvider>,
    );

    expect(screen.getByText("Hello AI")).not.toBeNull();

    const clearBtn = screen.getByRole("button", { name: "Clear chat" });
    fireEvent.click(clearBtn);

    expect(useAICopilotStore.getState().messages).toHaveLength(0);
    expect(screen.getByTestId("copilot-empty-state")).not.toBeNull();
  });

});
