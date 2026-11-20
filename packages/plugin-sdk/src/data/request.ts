import { detectLang } from "../i18n";

export class ApiError extends Error {
  status: number;
  body?: unknown;
  requestId?: string;

  constructor(status: number, message: string, body?: unknown, requestId?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
    this.requestId = requestId;
  }
}

export interface PluginHttpConfig {
  basePath?: string;
  language?: () => string;
}

let config: PluginHttpConfig = {};

export function configurePluginHttp(next: PluginHttpConfig): void {
  config = { ...config, ...next };
}

export function resetPluginHttp(): void {
  config = {};
}

export function getAppLang(): string {
  return config.language ? config.language() : detectLang();
}

interface ErrorEnvelope {
  message?: string;
  error?: string;
  detail?: string;
  details?: Array<string | { location?: string; message?: string }>;
}

function resolvePath(path: string): string {
  const base = config.basePath;
  if (!base || !path.startsWith("/")) return path;
  const trimmed = base.endsWith("/") ? base.slice(0, -1) : base;
  if (path === trimmed || path.startsWith(trimmed + "/")) return path;
  return trimmed + path;
}

export function pluginURL(path: string): string {
  return resolvePath(path);
}

function errorMessage(res: Response, parsed: ErrorEnvelope | null): string {
  if (!parsed) return res.statusText || `HTTP ${res.status}`;
  let msg = parsed.message ?? parsed.error ?? parsed.detail ?? res.statusText ?? `HTTP ${res.status}`;
  if (Array.isArray(parsed.details) && parsed.details.length > 0) {
    const parts = parsed.details
      .map((d) => {
        if (typeof d === "string") return d;
        if (d?.message) {
          const loc = d.location ? `${d.location.replace(/^body\./, "")}: ` : "";
          return `${loc}${d.message}`;
        }
        return "";
      })
      .filter(Boolean);
    if (parts.length > 0) msg = `${msg}: ${parts.join(", ")}`;
  }
  return msg;
}

export async function req<T>(
  method: string,
  path: string,
  body?: unknown,
  lang?: string,
  signal?: AbortSignal,
): Promise<T> {
  const headers: Record<string, string> = {
    "Accept-Language": lang ?? getAppLang(),
  };
  if (body !== undefined) headers["Content-Type"] = "application/json";

  const res = await fetch(resolvePath(path), {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: "same-origin",
    signal,
  });

  if (!res.ok) {
    let parsed: ErrorEnvelope | null = null;
    try {
      parsed = (await res.json()) as ErrorEnvelope;
    } catch {
    }
    throw new ApiError(
      res.status,
      errorMessage(res, parsed),
      parsed,
      res.headers.get("x-request-id") ?? undefined,
    );
  }

  if (res.status === 204) return undefined as T;
  const contentType = res.headers.get("content-type") ?? "";
  if (!contentType.includes("application/json")) return undefined as T;
  return (await res.json()) as T;
}
