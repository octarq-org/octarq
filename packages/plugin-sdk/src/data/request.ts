// The plugin HTTP surface: one request function, one error type.
//
// Every plugin used to carry its own copy of `req()` + `ApiError`, and the
// copies drifted (one added request ids, another dropped the message detail,
// a third grew a different constructor). This module is the single
// implementation. Plugins call `req` from the SDK instead of defining their
// own, so an error shape fixed here is fixed everywhere.
//
// The request is same-origin with the host dashboard: plugins run inside the
// operator's own console, so the host session cookie is the credential and no
// token is passed in JS.
import { detectLang } from "../i18n";

// ApiError carries the HTTP status and the parsed envelope, so a page can
// branch on 402 (upsell), 403 (forbidden), or 404 (feature not in this build)
// instead of matching on a message string.
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
  // Prefix every root-relative path, e.g. "/api/x/octarq". A path that already
  // starts with the prefix is left alone, so a plugin may pass absolute paths.
  basePath?: string;
  // Override the Accept-Language source. The default reads the host's saved
  // language, falling back to the browser's.
  language?: () => string;
}

let config: PluginHttpConfig = {};

export function configurePluginHttp(next: PluginHttpConfig): void {
  config = { ...config, ...next };
}

// Restores the default configuration. The test harness calls it so one test
// cannot leak a base path into the next.
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

// pluginURL applies the configured base path to a route. Streams that cannot
// go through req() (SSE) call it directly so every plugin route resolves the
// same way.
export function pluginURL(path: string): string {
  return resolvePath(path);
}

// Turns the backend envelope into one human sentence: the top-level message,
// then each field-level detail as "field: message". A body that is not JSON
// leaves the HTTP status text in place.
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

// req sends one JSON request. `lang` overrides the Accept-Language header for
// this call; `signal` aborts it (streams and unmounting hooks pass one).
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
      /* not JSON */
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
