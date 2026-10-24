// @vitest-environment happy-dom
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import {
  streamAGUIChat,
  AGUI_ERROR_MESSAGES,
  getCsrfToken,
  errorFromAGUIStatus,
} from "./agui";
import { AIStreamError } from "./api";
import { useCopilotStore } from "./store";

function createMockStream(chunks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(encoder.encode(chunk));
      }
      controller.close();
    },
  });
}

describe("AG-UI Protocol Client (streamAGUIChat)", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
    document.cookie = "";
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("extracts CSRF token from document.cookie and sends credentials and CSRF header", async () => {
    document.cookie = "octarq_csrf=csrf-sec-token-123; path=/";
    expect(getCsrfToken()).toBe("csrf-sec-token-123");

    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      body: createMockStream([
        'data: {"type":"RUN_STARTED","threadId":"t-1"}\n\n',
        'data: {"type":"RUN_FINISHED","threadId":"t-1"}\n\n',
      ]),
    });
    vi.stubGlobal("fetch", fetchMock);

    const onDone = vi.fn();
    await streamAGUIChat({
      messages: [{ role: "user", content: "hello" }],
      threadId: "thread-abc",
      runId: "run-xyz",
      onDone,
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("/api/agui");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("same-origin");
    expect(init.headers["X-CSRF-Token"]).toBe("csrf-sec-token-123");
    expect(init.headers["Content-Type"]).toBe("application/json");

    const parsedBody = JSON.parse(init.body as string);
    expect(parsedBody.threadId).toBe("thread-abc");
    expect(parsedBody.runId).toBe("run-xyz");
    expect(parsedBody.messages).toEqual([{ role: "user", content: "hello" }]);
    expect(onDone).toHaveBeenCalledTimes(1);
  });

  it("parses SSE stream chunk events for TEXT_MESSAGE_CHUNK and RUN_FINISHED", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      body: createMockStream([
        'data: {"type":"RUN_STARTED","threadId":"t-1","runId":"r-1"}\n\n',
        'data: {"type":"TEXT_MESSAGE_CHUNK","delta":"Hello"}\n\n',
        'data: {"type":"TEXT_MESSAGE_CHUNK","delta":", "}\n\n',
        'data: {"type":"TEXT_MESSAGE_CONTENT","delta":"world!"}\n\n',
        'data: {"type":"RUN_FINISHED","threadId":"t-1","runId":"r-1"}\n\n',
      ]),
    });
    vi.stubGlobal("fetch", fetchMock);

    const textDeltas: string[] = [];
    const onDone = vi.fn();
    const onError = vi.fn();

    await streamAGUIChat({
      messages: [{ role: "user", content: "hi" }],
      onText: (delta) => textDeltas.push(delta),
      onDone,
      onError,
    });

    expect(textDeltas).toEqual(["Hello", ", ", "world!"]);
    expect(onDone).toHaveBeenCalledTimes(1);
    expect(onError).not.toHaveBeenCalled();
  });

  it("handles thinking content events", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      body: createMockStream([
        'data: {"type":"RUN_STARTED"}\n\n',
        'data: {"type":"THINKING_TEXT_MESSAGE_CONTENT","delta":"Analyzing links..."}\n\n',
        'data: {"type":"TEXT_MESSAGE_CHUNK","delta":"Done"}\n\n',
        'data: {"type":"RUN_FINISHED"}\n\n',
      ]),
    });
    vi.stubGlobal("fetch", fetchMock);

    const thinkingDeltas: string[] = [];
    const textDeltas: string[] = [];

    await streamAGUIChat({
      messages: [{ role: "user", content: "analyze" }],
      onThinking: (delta) => thinkingDeltas.push(delta),
      onText: (delta) => textDeltas.push(delta),
    });

    expect(thinkingDeltas).toEqual(["Analyzing links..."]);
    expect(textDeltas).toEqual(["Done"]);
  });

  it("maps TOOL_CALL_STARTED and TOOL_CALL_START events to onToolCall", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      body: createMockStream([
        'data: {"type":"RUN_STARTED"}\n\n',
        'data: {"type":"TOOL_CALL_STARTED","id":"call-101","name":"list_links","args":{"active":true}}\n\n',
        'data: {"type":"TOOL_CALL_START","toolCallId":"call-102","toolCallName":"delete_dns","args":"{\\"domain\\":\\"api.octarq.com\\"}"}\n\n',
        'data: {"type":"TOOL_CALL_RESULT","toolCallId":"call-101","content":{"count":5}}\n\n',
        'data: {"type":"RUN_FINISHED"}\n\n',
      ]),
    });
    vi.stubGlobal("fetch", fetchMock);

    const toolCalls: { id?: string; name: string; args: Record<string, unknown> }[] = [];
    const toolResults: { id?: string; content?: unknown }[] = [];

    await streamAGUIChat({
      messages: [{ role: "user", content: "list" }],
      onToolCall: (call) => toolCalls.push(call),
      onToolResult: (res) => toolResults.push(res),
    });

    expect(toolCalls).toHaveLength(2);
    expect(toolCalls[0]).toEqual({
      id: "call-101",
      name: "list_links",
      args: { active: true },
    });
    expect(toolCalls[1]).toEqual({
      id: "call-102",
      name: "delete_dns",
      args: { domain: "api.octarq.com" },
    });
    expect(toolResults).toHaveLength(1);
    expect(toolResults[0]).toEqual({
      id: "call-101",
      content: { count: 5 },
    });
  });

  it("handles streamed tool call args with TOOL_CALL_ARGS and TOOL_CALL_END", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      body: createMockStream([
        'data: {"type":"RUN_STARTED"}\n\n',
        'data: {"type":"TOOL_CALL_START","id":"call-stream-1","name":"query_db"}\n\n',
        'data: {"type":"TOOL_CALL_ARGS","id":"call-stream-1","delta":"{\\"query\\":\\"SELECT "}\n\n',
        'data: {"type":"TOOL_CALL_ARGS","id":"call-stream-1","delta":"1\\"}"}\n\n',
        'data: {"type":"TOOL_CALL_END","id":"call-stream-1"}\n\n',
        'data: {"type":"RUN_FINISHED"}\n\n',
      ]),
    });
    vi.stubGlobal("fetch", fetchMock);

    const toolCalls: { id?: string; name: string; args: Record<string, unknown> }[] = [];
    await streamAGUIChat({
      messages: [{ role: "user", content: "query" }],
      onToolCall: (call) => toolCalls.push(call),
    });

    expect(toolCalls.length).toBeGreaterThanOrEqual(1);
    const finalCall = toolCalls[toolCalls.length - 1];
    expect(finalCall).toEqual({
      id: "call-stream-1",
      name: "query_db",
      args: { query: "SELECT 1" },
    });
  });

  describe("Fail-Closed HTTP Status Error Mapping (ASD-STE100)", () => {
    it("invokes onError with typed AIStreamError on HTTP 401 Unauthorized", async () => {
      const fetchMock = vi.fn().mockResolvedValue({
        ok: false,
        status: 401,
        text: async () => JSON.stringify({ error: AGUI_ERROR_MESSAGES[401] }),
      });
      vi.stubGlobal("fetch", fetchMock);

      const onError = vi.fn();
      const onDone = vi.fn();

      await streamAGUIChat({
        messages: [{ role: "user", content: "hi" }],
        onError,
        onDone,
      });

      expect(onError).toHaveBeenCalledTimes(1);
      const err = onError.mock.calls[0][0];
      expect(err).toBeInstanceOf(AIStreamError);
      expect(err.status).toBe(401);
      expect(err.code).toBe("unauthorized");
      expect(err.message).toBe("Authentication required. Sign in with an operator account.");
      expect(onDone).toHaveBeenCalledTimes(1);
    });

    it("falls back to canonical ASD-STE100 message when 401 body is empty", async () => {
      const fetchMock = vi.fn().mockResolvedValue({
        ok: false,
        status: 401,
        text: async () => "",
      });
      vi.stubGlobal("fetch", fetchMock);

      const onError = vi.fn();
      await streamAGUIChat({
        messages: [{ role: "user", content: "hi" }],
        onError,
      });

      expect(onError).toHaveBeenCalledTimes(1);
      const err = onError.mock.calls[0][0];
      expect(err.status).toBe(401);
      expect(err.code).toBe("unauthorized");
      expect(err.message).toBe("Authentication required. Sign in with an operator account.");
    });

    it("invokes onError with typed AIStreamError on HTTP 402 Payment Required", async () => {
      const fetchMock = vi.fn().mockResolvedValue({
        ok: false,
        status: 402,
        text: async () => JSON.stringify({ error: AGUI_ERROR_MESSAGES[402] }),
      });
      vi.stubGlobal("fetch", fetchMock);

      const onError = vi.fn();
      await streamAGUIChat({
        messages: [{ role: "user", content: "hi" }],
        onError,
      });

      expect(onError).toHaveBeenCalledTimes(1);
      const err = onError.mock.calls[0][0];
      expect(err).toBeInstanceOf(AIStreamError);
      expect(err.status).toBe(402);
      expect(err.code).toBe("locked");
      expect(err.message).toBe("An Octarq Pro Elite license is required to use this capability.");
    });

    it("falls back to canonical ASD-STE100 message when 402 body is empty", async () => {
      const fetchMock = vi.fn().mockResolvedValue({
        ok: false,
        status: 402,
        text: async () => "",
      });
      vi.stubGlobal("fetch", fetchMock);

      const onError = vi.fn();
      await streamAGUIChat({
        messages: [{ role: "user", content: "hi" }],
        onError,
      });

      expect(onError).toHaveBeenCalledTimes(1);
      const err = onError.mock.calls[0][0];
      expect(err.status).toBe(402);
      expect(err.code).toBe("locked");
      expect(err.message).toBe("An Octarq Pro Elite license is required to use this capability.");
    });

    it("invokes onError with typed AIStreamError on HTTP 503 Service Unavailable", async () => {
      const fetchMock = vi.fn().mockResolvedValue({
        ok: false,
        status: 503,
        text: async () => JSON.stringify({ error: AGUI_ERROR_MESSAGES[503] }),
      });
      vi.stubGlobal("fetch", fetchMock);

      const onError = vi.fn();
      await streamAGUIChat({
        messages: [{ role: "user", content: "hi" }],
        onError,
      });

      expect(onError).toHaveBeenCalledTimes(1);
      const err = onError.mock.calls[0][0];
      expect(err).toBeInstanceOf(AIStreamError);
      expect(err.status).toBe(503);
      expect(err.code).toBe("not_configured");
      expect(err.message).toBe("Configure an LLM provider in AI Settings first.");
    });

    it("falls back to canonical ASD-STE100 message when 503 body is empty", async () => {
      const fetchMock = vi.fn().mockResolvedValue({
        ok: false,
        status: 503,
        text: async () => "",
      });
      vi.stubGlobal("fetch", fetchMock);

      const onError = vi.fn();
      await streamAGUIChat({
        messages: [{ role: "user", content: "hi" }],
        onError,
      });

      expect(onError).toHaveBeenCalledTimes(1);
      const err = onError.mock.calls[0][0];
      expect(err.status).toBe(503);
      expect(err.code).toBe("not_configured");
      expect(err.message).toBe("Configure an LLM provider in AI Settings first.");
    });

    it("delivers RUN_ERROR event from SSE stream as typed AIStreamError", async () => {
      const fetchMock = vi.fn().mockResolvedValue({
        ok: true,
        body: createMockStream([
          'data: {"type":"RUN_STARTED"}\n\n',
          'data: {"type":"RUN_ERROR","message":"Model context limit reached."}\n\n',
        ]),
      });
      vi.stubGlobal("fetch", fetchMock);

      const onError = vi.fn();
      await streamAGUIChat({
        messages: [{ role: "user", content: "overflow" }],
        onError,
      });

      expect(onError).toHaveBeenCalledTimes(1);
      const err = onError.mock.calls[0][0];
      expect(err).toBeInstanceOf(AIStreamError);
      expect(err.code).toBe("server");
      expect(err.message).toBe("Model context limit reached.");
    });
  });

  describe("AbortSignal Handling", () => {
    it("cleanly stops reading without errors when signal is already aborted", async () => {
      const controller = new AbortController();
      controller.abort();

      const onError = vi.fn();
      const onDone = vi.fn();

      await streamAGUIChat({
        messages: [{ role: "user", content: "hello" }],
        signal: controller.signal,
        onError,
        onDone,
      });

      expect(onError).not.toHaveBeenCalled();
      expect(onDone).not.toHaveBeenCalled();
    });

    it("cleanly stops reading without errors when aborted mid-stream", async () => {
      const controller = new AbortController();
      const encoder = new TextEncoder();

      const stream = new ReadableStream<Uint8Array>({
        start(streamController) {
          streamController.enqueue(
            encoder.encode('data: {"type":"TEXT_MESSAGE_CHUNK","delta":"Partial"}\n\n'),
          );
        },
        pull() {
          // Wait for abort
          return new Promise<void>((resolve) => {
            controller.signal.addEventListener("abort", () => resolve());
          });
        },
      });

      const fetchMock = vi.fn().mockResolvedValue({
        ok: true,
        body: stream,
      });
      vi.stubGlobal("fetch", fetchMock);

      const textDeltas: string[] = [];
      const onError = vi.fn();

      const promise = streamAGUIChat({
        messages: [{ role: "user", content: "stream" }],
        signal: controller.signal,
        onText: (d) => {
          textDeltas.push(d);
          controller.abort();
        },
        onError,
      });

      await expect(promise).resolves.toBeUndefined();
      expect(textDeltas).toEqual(["Partial"]);
      expect(onError).not.toHaveBeenCalled();
    });
  });

  describe("Integration Seam & Store", () => {
    it("allows switching store transport to agui and legacy", () => {
      const store = useCopilotStore.getState();
      expect(store.transport).toBe("legacy");

      store.setTransport("agui");
      expect(useCopilotStore.getState().transport).toBe("agui");

      store.setTransport("legacy");
      expect(useCopilotStore.getState().transport).toBe("legacy");
    });
  });
});
