import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { parseAIStatus } from "./schemas";
import type { ActionDiff, AIStatus, ChatMessage } from "./types";

export const AI_STATUS_QUERY_KEY = ["ai", "assist", "status"] as const;
export const APPROVALS_QUERY_KEY = ["ai", "approvals"] as const;

export async function fetchAIStatus(): Promise<AIStatus> {
  try {
    const res = await fetch("/api/ai/assist/status", {
      headers: { Accept: "application/json" },
    });
    if (!res.ok) {
      return { configured: false, provider: "" };
    }
    const data = await res.json();
    return parseAIStatus(data);
  } catch {
    return { configured: false, provider: "" };
  }
}

export function useAIStatusQuery() {
  return useQuery({
    queryKey: AI_STATUS_QUERY_KEY,
    queryFn: fetchAIStatus,
    staleTime: 60 * 1000,
  });
}

export interface StreamChatParams {
  messages: { role: string; content: string }[];
  onThinking?: (delta: string) => void;
  onText?: (delta: string) => void;
  onToolCall?: (toolCall: { id?: string; name: string; args: Record<string, unknown> }) => void;
  onError?: (err: Error) => void;
  onDone?: () => void;
  signal?: AbortSignal;
}

export type AIStreamErrorCode =
  | "unauthorized"
  | "locked"
  | "not_configured"
  | "bad_request"
  | "server"
  | "network";

/**
 * Typed failure for the AI chat stream. Fail-Closed: the caller must surface
 * this to the operator — synthesizing a fake assistant reply is forbidden
 * (Pre-v1.0 铁律: 严禁 fallback 仿真).
 */
export class AIStreamError extends Error {
  readonly status: number;
  readonly code: AIStreamErrorCode;

  constructor(status: number, code: AIStreamErrorCode, message: string) {
    super(message);
    this.name = "AIStreamError";
    this.status = status;
    this.code = code;
  }
}

function errorFromStatus(status: number, detail: string): AIStreamError {
  const message = detail || `AI request failed (HTTP ${status})`;
  switch (status) {
    case 401:
      return new AIStreamError(status, "unauthorized", message);
    case 402:
      return new AIStreamError(status, "locked", message);
    case 400:
      return new AIStreamError(status, "not_configured", message);
    default:
      return new AIStreamError(
        status,
        status >= 500 ? "server" : "bad_request",
        message,
      );
  }
}

async function readErrorDetail(res: Response): Promise<string> {
  try {
    const text = await res.text();
    if (!text) return "";
    try {
      const data = JSON.parse(text) as { message?: unknown; error?: unknown };
      if (typeof data.message === "string" && data.message) return data.message;
      if (typeof data.error === "string" && data.error) return data.error;
    } catch {
      return text.slice(0, 500);
    }
    return "";
  } catch {
    return "";
  }
}

/**
 * Streams chat responses from the backend `/api/ai/chat/stream` SSE endpoint.
 *
 * Fail-Closed: when the backend answers 401/402/400/500 or the network fails,
 * the error is delivered to `onError` verbatim. No simulated reply is ever
 * produced on the client.
 */
export async function streamAIChat({
  messages,
  onThinking,
  onText,
  onToolCall,
  onError,
  onDone,
  signal,
}: StreamChatParams): Promise<void> {
  let res: Response;
  try {
    res = await fetch("/api/ai/chat/stream", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        messages,
      }),
      signal,
    });
  } catch (err: unknown) {
    if (signal?.aborted) return;
    const message = err instanceof Error ? err.message : "network request failed";
    onError?.(new AIStreamError(0, "network", message));
    onDone?.();
    return;
  }

  if (!res.ok) {
    // Strict Fail-Closed: surface the backend failure, never fake a reply.
    const detail = await readErrorDetail(res);
    onError?.(errorFromStatus(res.status, detail));
    onDone?.();
    return;
  }

  if (!res.body) {
    onError?.(new AIStreamError(0, "network", "Response body is null"));
    onDone?.();
    return;
  }

  try {
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      let currentEvent = "";
      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed) {
          currentEvent = "";
          continue;
        }

        if (trimmed.startsWith("event: ")) {
          currentEvent = trimmed.slice(7).trim();
          continue;
        }

        if (trimmed.startsWith("data: ")) {
          const dataStr = trimmed.slice(6).trim();
          try {
            const data = JSON.parse(dataStr);
            if (currentEvent === "thinking") {
              onThinking?.(data.delta || "");
            } else if (currentEvent === "text") {
              onText?.(data.delta || "");
            } else if (currentEvent === "tool") {
              let parsedArgs: Record<string, unknown> = {};
              if (typeof data.args === "string") {
                try {
                  parsedArgs = JSON.parse(data.args);
                } catch {
                  parsedArgs = { raw: data.args };
                }
              } else if (typeof data.args === "object" && data.args !== null) {
                parsedArgs = data.args;
              }
              onToolCall?.({
                id: data.id,
                name: data.name || "unknown_tool",
                args: parsedArgs,
              });
            } else if (currentEvent === "error") {
              onError?.(new AIStreamError(500, "server", data.message || "AI Stream Error"));
            } else if (currentEvent === "done") {
              onDone?.();
            }
          } catch {
            // Raw text fallback
            if (currentEvent === "text") {
              onText?.(dataStr);
            }
          }
        }
      }
    }

    onDone?.();
  } catch (err: unknown) {
    if (signal?.aborted) return;
    const message = err instanceof Error ? err.message : "stream read failed";
    onError?.(new AIStreamError(0, "network", message));
    onDone?.();
  }
}

// --- Approval CAS client (bound to backend agent_approvals atomic flow) ---

export type ApprovalDecision = "approve" | "reject";

export type ApprovalResolveOutcome =
  | "approved"
  | "rejected"
  | "already_resolved"
  | "expired"
  | "not_found";

export interface ApprovalDecisionResult {
  outcome: ApprovalResolveOutcome;
  status: string;
  approver?: string;
}

/**
 * decideApproval performs an atomic compare-and-swap decision against
 * `/api/ai/approvals/:id/approve|reject`. The backend only transitions
 * `pending → approved|rejected`; concurrent or replayed decisions surface as
 * 409 (already resolved) or 410 (expired/missing) and are mapped — never
 * optimistically marked locally.
 */
export async function decideApproval(
  approvalId: string,
  decision: ApprovalDecision,
  token?: string,
): Promise<ApprovalDecisionResult> {
  const res = await fetch(`/api/ai/approvals/${encodeURIComponent(approvalId)}/${decision}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(token ? { token } : {}),
  });

  if (res.status === 409) {
    return { outcome: "already_resolved", status: "resolved" };
  }
  if (res.status === 410) {
    return { outcome: "expired", status: "expired" };
  }
  if (res.status === 404) {
    return { outcome: "not_found", status: "not_found" };
  }
  if (!res.ok) {
    throw errorFromStatus(res.status, await readErrorDetail(res));
  }
  const data = (await res.json()) as { status?: string; approver?: string };
  const status = typeof data.status === "string" ? data.status : decision === "approve" ? "approved" : "rejected";
  return {
    outcome: decision === "approve" ? "approved" : "rejected",
    status,
    approver: data.approver,
  };
}

interface BackendApprovalRow {
  id: number | string;
  token?: string;
  tool?: string;
  action?: string;
  title?: string;
  target?: string;
  risk_level?: string;
  riskLevel?: string;
  status?: string;
  reason?: string;
  created_at?: string;
  createdAt?: string;
  expires_at?: string;
  expiresAt?: string;
  approver?: string;
}

function mapBackendApproval(row: BackendApprovalRow): ActionDiff {
  const id = String(row.id);
  const status = row.status === "approved" || row.status === "rejected" || row.status === "expired"
    ? row.status
    : "pending";
  const riskLevel = row.riskLevel === "destructive" || row.risk_level === "destructive"
    ? "destructive"
    : row.riskLevel === "write" || row.risk_level === "write"
      ? "write"
      : "read";
  return {
    id: `approval-${id}`,
    approvalId: id,
    approvalToken: row.token,
    agent: "AI Agent",
    action: row.action || row.tool || "agent_action",
    title: row.title || row.tool || "待审批的智能体操作",
    target: row.target || "",
    riskLevel,
    status,
    createdAt: row.createdAt || row.created_at || new Date().toISOString(),
    expiresAt: row.expiresAt || row.expires_at,
    reason: row.reason,
    approver: row.approver,
    diff: [],
  };
}

/**
 * fetchApprovals lists pending agent approvals for the current workspace.
 * Throws AIStreamError on 401/402 so the caller can render the guide card or
 * the LockedFeature upsell instead of placeholder data.
 */
export async function fetchApprovals(): Promise<ActionDiff[]> {
  const res = await fetch("/api/ai/approvals", {
    headers: { Accept: "application/json" },
  });
  if (!res.ok) {
    throw errorFromStatus(res.status, await readErrorDetail(res));
  }
  const data = (await res.json()) as BackendApprovalRow[] | { items?: BackendApprovalRow[] };
  const rows = Array.isArray(data) ? data : data.items || [];
  return rows.map(mapBackendApproval);
}

export function useApprovalsQuery(enabled: boolean) {
  return useQuery({
    queryKey: APPROVALS_QUERY_KEY,
    queryFn: fetchApprovals,
    enabled,
    staleTime: 15 * 1000,
    retry: false,
  });
}

export function useDecideApprovalMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, decision, token }: { id: string; decision: ApprovalDecision; token?: string }) =>
      decideApproval(id, decision, token),
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: APPROVALS_QUERY_KEY });
    },
  });
}

export type { ActionDiff, AIStatus, ChatMessage };
