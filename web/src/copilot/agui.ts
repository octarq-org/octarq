import { AIStreamError, type AIStreamErrorCode } from "./api";

export interface StreamAGUIParams {
  messages: { role: string; content: string }[];
  threadId?: string;
  runId?: string;
  onThinking?: (delta: string) => void;
  onText?: (delta: string) => void;
  onToolCall?: (toolCall: { id?: string; name: string; args: Record<string, unknown> }) => void;
  onToolResult?: (toolResult: { id?: string; content?: unknown }) => void;
  onError?: (err: Error) => void;
  onDone?: () => void;
  signal?: AbortSignal;
}

export const AGUI_ERROR_MESSAGES = {
  401: "Authentication required. Sign in with an operator account.",
  402: "An Octarq Pro Elite license is required to use this capability.",
  404: "This capability requires an upgraded license tier.",
  503: "Configure an LLM provider in AI Settings first.",
} as const;

export function getCsrfToken(): string | null {
  if (typeof document === "undefined" || !document.cookie) return null;
  const match = document.cookie.match(/(?:^|;\s*)octarq_csrf=([^;]*)/);
  return match ? decodeURIComponent(match[1]) : null;
}

function isGenericStatusText(text: string, generic: string): boolean {
  const lower = text.toLowerCase();
  return lower === generic.toLowerCase() || lower === `http ${generic.toLowerCase()}`;
}

export function errorFromAGUIStatus(status: number, detail?: string): AIStreamError {
  const trimmed = detail?.trim();
  switch (status) {
    case 401:
      return new AIStreamError(
        status,
        "unauthorized",
        trimmed && !isGenericStatusText(trimmed, "Unauthorized")
          ? trimmed
          : AGUI_ERROR_MESSAGES[401],
      );
    case 402:
    case 404:
      return new AIStreamError(
        status,
        "locked",
        trimmed && !isGenericStatusText(trimmed, "Payment Required") && !isGenericStatusText(trimmed, "Not Found")
          ? trimmed
          : AGUI_ERROR_MESSAGES[status as 402 | 404] || AGUI_ERROR_MESSAGES[402],
      );
    case 503:
      return new AIStreamError(
        status,
        "not_configured",
        trimmed && !isGenericStatusText(trimmed, "Service Unavailable")
          ? trimmed
          : AGUI_ERROR_MESSAGES[503],
      );
    default:
      return new AIStreamError(
        status,
        (status >= 500 ? "server" : "bad_request") as AIStreamErrorCode,
        trimmed || `AI request failed (HTTP ${status})`,
      );
  }
}

async function readErrorDetail(res: Response): Promise<string> {
  try {
    const text = await res.text();
    if (!text) return "";
    try {
      const data = JSON.parse(text) as { message?: unknown; error?: unknown };
      if (typeof data.error === "string" && data.error) return data.error;
      if (typeof data.message === "string" && data.message) return data.message;
    } catch {
      return text.slice(0, 500);
    }
    return "";
  } catch {
    return "";
  }
}

/**
 * Streams chat responses from the standard AG-UI Protocol endpoint POST /api/agui.
 *
 * Strict Fail-Closed:
 * - HTTP 401: unauthorized ("Authentication required. Sign in with an operator account.")
 * - HTTP 402: locked ("An Octarq Pro Elite license is required to use this capability.")
 * - HTTP 503: not configured ("Configure an LLM provider in AI Settings first.")
 *
 * Never synthesizes or fakes assistant replies on failure.
 */
export async function streamAGUIChat({
  messages,
  threadId,
  runId,
  onThinking,
  onText,
  onToolCall,
  onToolResult,
  onError,
  onDone,
  signal,
}: StreamAGUIParams): Promise<void> {
  if (signal?.aborted) return;

  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    Accept: "text/event-stream",
  };
  const csrfToken = getCsrfToken();
  if (csrfToken) {
    headers["X-CSRF-Token"] = csrfToken;
  }

  const payload: Record<string, unknown> = {
    messages: messages.map((m) => ({
      role: m.role,
      content: m.content,
    })),
  };
  if (threadId) payload.threadId = threadId;
  if (runId) payload.runId = runId;

  let res: Response;
  try {
    res = await fetch("/api/agui", {
      method: "POST",
      credentials: "same-origin",
      headers,
      body: JSON.stringify(payload),
      signal,
    });
  } catch (err: unknown) {
    if (signal?.aborted || (err instanceof Error && err.name === "AbortError")) {
      return;
    }
    const message = err instanceof Error ? err.message : "network request failed";
    onError?.(new AIStreamError(0, "network", message));
    onDone?.();
    return;
  }

  if (!res.ok) {
    const detail = await readErrorDetail(res);
    onError?.(errorFromAGUIStatus(res.status, detail));
    onDone?.();
    return;
  }

  if (!res.body) {
    onError?.(new AIStreamError(0, "network", "Response body is null"));
    onDone?.();
    return;
  }

  let hasDone = false;
  let hasError = false;

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  const abortHandler = () => {
    try {
      void reader.cancel();
    } catch {
      // ignore abort cancel error
    }
  };
  signal?.addEventListener("abort", abortHandler, { once: true });

  const pendingToolArgs: Record<string, { name: string; argsStr: string }> = {};
  let currentEvent = "";

  const processLine = (line: string) => {
    const trimmed = line.trim();
    if (!trimmed) {
      currentEvent = "";
      return;
    }

    if (trimmed.startsWith("event:")) {
      currentEvent = trimmed.slice(6).trim();
      return;
    }

    if (trimmed.startsWith("data:")) {
      const dataStr = trimmed.slice(5).trim();
      if (!dataStr) return;

      let data: Record<string, unknown> = {};
      try {
        data = JSON.parse(dataStr);
      } catch {
        data = { text: dataStr };
      }

      const eventType = (typeof data.type === "string" ? data.type : currentEvent).toUpperCase();

      switch (eventType) {
        case "RUN_STARTED":
          break;

        case "TEXT_MESSAGE_CHUNK":
        case "TEXT_MESSAGE_CONTENT":
        case "TEXT": {
          const delta =
            typeof data.delta === "string"
              ? data.delta
              : typeof data.content === "string"
                ? data.content
                : typeof data.text === "string"
                  ? data.text
                  : "";
          if (delta) {
            onText?.(delta);
          }
          break;
        }

        case "THINKING_TEXT_MESSAGE_CONTENT":
        case "THINKING_TEXT_MESSAGE_CHUNK":
        case "REASONING_MESSAGE_CONTENT":
        case "REASONING_MESSAGE_CHUNK":
        case "THINKING": {
          const delta =
            typeof data.delta === "string"
              ? data.delta
              : typeof data.content === "string"
                ? data.content
                : typeof data.text === "string"
                  ? data.text
                  : "";
          if (delta) {
            onThinking?.(delta);
          }
          break;
        }

        case "TOOL_CALL_START":
        case "TOOL_CALL_STARTED": {
          const id = (data.id ?? data.toolCallId ?? data.callId) as string | undefined;
          const name = (data.name ?? data.toolCallName ?? data.toolName ?? "unknown_tool") as string;
          let parsedArgs: Record<string, unknown> = {};
          const rawArgs = data.args ?? data.arguments;
          if (typeof rawArgs === "object" && rawArgs !== null) {
            parsedArgs = rawArgs as Record<string, unknown>;
          } else if (typeof rawArgs === "string") {
            try {
              parsedArgs = JSON.parse(rawArgs);
            } catch {
              parsedArgs = { raw: rawArgs };
            }
          }

          if (id) {
            let initialArgsStr = "";
            if (typeof rawArgs === "string") {
              initialArgsStr = rawArgs;
            } else if (typeof rawArgs === "object" && rawArgs !== null && Object.keys(rawArgs).length > 0) {
              initialArgsStr = JSON.stringify(rawArgs);
            }
            pendingToolArgs[id] = {
              name,
              argsStr: initialArgsStr,
            };
          }

          onToolCall?.({
            id,
            name,
            args: parsedArgs,
          });
          break;
        }

        case "TOOL_CALL_ARGS":
        case "TOOL_CALL_CHUNK": {
          const id = (data.id ?? data.toolCallId ?? data.callId) as string | undefined;
          const delta =
            typeof data.delta === "string"
              ? data.delta
              : typeof data.args === "string"
                ? data.args
                : "";
          if (id && delta) {
            if (!pendingToolArgs[id]) {
              pendingToolArgs[id] = { name: "unknown_tool", argsStr: "" };
            }
            pendingToolArgs[id].argsStr += delta;
          }
          break;
        }

        case "TOOL_CALL_END": {
          const id = (data.id ?? data.toolCallId ?? data.callId) as string | undefined;
          if (id && pendingToolArgs[id]) {
            const { name, argsStr } = pendingToolArgs[id];
            if (argsStr) {
              try {
                const parsedArgs = JSON.parse(argsStr);
                onToolCall?.({
                  id,
                  name,
                  args: parsedArgs,
                });
              } catch {
                // ignore if incomplete json
              }
            }
          }
          break;
        }

        case "TOOL_CALL_RESULT":
        case "TOOL_CALL_FINISHED": {
          const id = (data.id ?? data.toolCallId ?? data.callId) as string | undefined;
          const content = data.content ?? data.result;
          onToolResult?.({ id, content });
          break;
        }

        case "RUN_FINISHED": {
          hasDone = true;
          onDone?.();
          break;
        }

        case "RUN_ERROR": {
          hasError = true;
          const errMsg =
            typeof data.message === "string"
              ? data.message
              : typeof data.error === "string"
                ? data.error
                : "AI Stream Error";
          onError?.(new AIStreamError(500, "server", errMsg));
          break;
        }

        default:
          break;
      }
    }
  };

  try {
    while (true) {
      if (signal?.aborted) break;
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      for (const line of lines) {
        processLine(line);
      }
    }

    if (buffer.trim()) {
      const remainingLines = buffer.split("\n");
      buffer = "";
      for (const line of remainingLines) {
        processLine(line);
      }
    }

    if (!hasDone && !hasError && !signal?.aborted) {
      onDone?.();
    }
  } catch (err: unknown) {
    if (signal?.aborted || (err instanceof Error && err.name === "AbortError")) {
      return;
    }
    const message = err instanceof Error ? err.message : "stream read failed";
    onError?.(new AIStreamError(0, "network", message));
    onDone?.();
  } finally {
    signal?.removeEventListener("abort", abortHandler);
  }
}
