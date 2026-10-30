import type { ToolPayload } from "./types";

// ChatFailure describes a refused or failed chat request. The UI branches on
// code — the server's message is prose and may change.
export interface ChatFailure {
  // status is the HTTP status of a refused request; 0 for a mid-stream error.
  status: number;
  // code is the machine-readable error code, e.g. workforce_budget_exceeded.
  code?: string;
}

// newChatThreadId names one conversation. The id groups the turns a surface
// sends; the server scopes the thread to the caller.
export function newChatThreadId(): string {
  return withRandom("thread");
}

// newChatRunId names one turn. A retried turn reuses the id, and the server
// charges it once.
export function newChatRunId(): string {
  return withRandom("run");
}

function withRandom(prefix: string): string {
  const uuid = globalThis.crypto?.randomUUID?.();
  if (uuid) return `${prefix}-${uuid}`;
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

interface ChatStreamEvent {
  type?: string;
  delta?: unknown;
  message?: unknown;
  code?: unknown;
  toolCallId?: string;
  toolCallName?: string;
}

export async function streamChatResponse({
  messages,
  threadId,
  runId,
  agentId,
  signal,
  onThinking,
  onText,
  onTool,
  onError,
  onDone,
}: {
  messages: Array<{ role: string; content: string }>;
  // threadId groups the turns of one conversation; runId names this turn and
  // is the server's billing idempotency key.
  threadId?: string;
  runId?: string;
  // agentId attributes the turn to a workforce agent, which prices and
  // charges it to that agent's monthly budget.
  agentId?: string;
  signal?: AbortSignal;
  onThinking: (delta: string, tokens?: number) => void;
  onText: (delta: string) => void;
  onTool: (toolPayload: ToolPayload) => void;
  onError: (err: string, failure?: ChatFailure) => void;
  onDone: () => void;
}) {
  try {
    const res = await fetch("/api/ai/chat/stream", {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
      body: JSON.stringify({
        threadId: threadId ?? newChatThreadId(),
        runId: runId ?? newChatRunId(),
        messages,
        ...(agentId ? { agentId } : {}),
      }),
      signal,
    });

    if (!res.ok) {
      let message = `HTTP ${res.status}: ${res.statusText}`;
      let code: string | undefined;
      try {
        const body = (await res.json()) as { code?: string; message?: string; detail?: string };
        if (typeof body.code === "string") code = body.code;
        if (typeof body.message === "string" && body.message) message = body.message;
        else if (typeof body.detail === "string" && body.detail) message = body.detail;
      } catch {
        // Not JSON — keep the status line.
      }
      onError(message, { status: res.status, code });
      return;
    }

    if (!res.body) {
      onError("No response body");
      return;
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    let dataLines: string[] = [];
    // settled turns true on the terminal event, RUN_FINISHED or RUN_ERROR.
    let settled = false;

    // Tool calls arrive as start → args → result. The args are collected so
    // the UI receives one payload per call, after the call is complete.
    const calls = new Map<string, { name: string; args: string }>();

    const flush = () => {
      if (dataLines.length === 0) return;
      const raw = dataLines.join("\n");
      dataLines = [];

      let event: ChatStreamEvent;
      try {
        event = JSON.parse(raw) as ChatStreamEvent;
      } catch {
        return;
      }

      switch (event.type) {
        case "THINKING_TEXT_MESSAGE_CONTENT":
          onThinking(String(event.delta ?? ""));
          return;
        case "TEXT_MESSAGE_CONTENT":
          onText(String(event.delta ?? ""));
          return;
        case "TOOL_CALL_START":
          if (event.toolCallId) {
            calls.set(event.toolCallId, { name: String(event.toolCallName ?? ""), args: "" });
          }
          return;
        case "TOOL_CALL_ARGS": {
          const call = event.toolCallId ? calls.get(event.toolCallId) : undefined;
          if (call) call.args += String(event.delta ?? "");
          return;
        }
        case "TOOL_CALL_RESULT": {
          const call = event.toolCallId ? calls.get(event.toolCallId) : undefined;
          if (event.toolCallId) calls.delete(event.toolCallId);
          if (call) onTool(toolPayload(event.toolCallId ?? "", call.name, call.args));
          return;
        }
        case "RUN_ERROR": {
          const code = typeof event.code === "string" ? event.code : undefined;
          settled = true;
          onError(String(event.message ?? "Request failed"), { status: 0, code });
          return;
        }
        case "RUN_FINISHED":
          settled = true;
          onDone();
          return;
        default:
          return;
      }
    };

    while (true) {
      const { done: streamDone, value } = await reader.read();
      if (streamDone) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split(/\r?\n/);
      buffer = lines.pop() || "";

      for (const line of lines) {
        if (line.startsWith("data:")) {
          dataLines.push(line.slice(5).trimStart());
        } else if (line.trim() === "") {
          flush();
        }
        // ":" comments are keep-alive pings. An "id:" line names the frame;
        // the client does not need it.
      }
    }
    flush();
    if (!settled) onDone();
  } catch (err: unknown) {
    if (err && typeof err === "object" && "name" in err && err.name === "AbortError") {
      return;
    }
    const msg = err instanceof Error ? err.message : "Network error";
    onError(msg);
  }
}

// toolPayload flattens the call arguments into the payload a widget reads. A
// chart or a diff arrives as its own kind, so the renderer picks it up without
// a protocol-specific wrapper.
function toolPayload(id: string, name: string, rawArgs: string): ToolPayload {
  let args: unknown = {};
  try {
    args = rawArgs ? JSON.parse(rawArgs) : {};
  } catch {
    args = { args: rawArgs };
  }
  if (args && typeof args === "object" && !Array.isArray(args)) {
    return { ...(args as Record<string, unknown>), id, name } as ToolPayload;
  }
  return { id, name, args } as ToolPayload;
}
