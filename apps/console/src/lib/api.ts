/**
 * The console's HTTP client. Every request goes to the console's own origin;
 * proxy.ts forwards /v1 to vetod. Requests that change state carry the CSRF
 * header the API requires for cookie-authenticated calls.
 */

import type { Tool, ToolInput } from "./types";

export const CSRF_HEADER = "X-Veto-Requested-With";

export interface ApiErrorBody {
  error: { code: string; message: string; details?: unknown };
  request_id?: string;
}

/** An error the API returned, or a network failure (status 0). */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: unknown;
  readonly requestId: string | undefined;

  constructor(status: number, code: string, message: string, details?: unknown, requestId?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
    this.requestId = requestId;
  }

  get unauthorized() {
    return this.status === 401;
  }
}

type Query = Record<string, string | number | boolean | undefined | null>;

export function withQuery(path: string, query?: Query): string {
  if (!query) return path;
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v === undefined || v === null || v === "" || v === false) continue;
    params.set(k, String(v));
  }
  const qs = params.toString();
  return qs ? `${path}?${qs}` : path;
}

async function request<T>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (method !== "GET") headers[CSRF_HEADER] = "console";
  let payload: BodyInit | undefined;
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    payload = typeof body === "string" ? body : JSON.stringify(body);
  }
  let res: Response;
  try {
    res = await fetch(path, { method, headers, body: payload, credentials: "same-origin", signal, cache: "no-store" });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") throw err;
    throw new ApiError(0, "network", "Veto could not be reached. Check your connection and that the server is running.");
  }
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  let data: unknown = undefined;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      throw new ApiError(res.status, "bad_response", `The server answered ${res.status} with a body that is not JSON.`);
    }
  }
  if (!res.ok) {
    const e = (data as ApiErrorBody | undefined)?.error;
    throw new ApiError(
      res.status,
      e?.code ?? "unknown",
      e?.message ?? `The request failed with status ${res.status}.`,
      e?.details,
      (data as ApiErrorBody | undefined)?.request_id,
    );
  }
  return data as T;
}

export const api = {
  get: <T>(path: string, query?: Query, signal?: AbortSignal) => request<T>("GET", withQuery(path, query), undefined, signal),
  post: <T>(path: string, body?: unknown) => request<T>("POST", path, body ?? {}),
  put: <T>(path: string, body: unknown) => request<T>("PUT", path, body),
  patch: <T>(path: string, body: unknown) => request<T>("PATCH", path, body),
  del: <T = void>(path: string) => request<T>("DELETE", path),
};

/** A message suitable for showing to a person. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error) return err.message;
  return "Something went wrong.";
}

/** Encodes each segment of a tool name for use in a path; tool names may contain slashes. */
export function toolPath(name: string): string {
  return name.split("/").map(encodeURIComponent).join("/");
}

/** The editable fields of a tool, for saving it back with changes. */
export function toolInput(t: Tool): ToolInput {
  return {
    description: t.description,
    risk_class: t.risk_class,
    reversible: t.reversible,
    pack: t.pack,
    safer: t.safer,
    auto_confidence: t.auto_confidence,
    risk_ceiling: t.risk_ceiling,
    block_confidence: t.block_confidence,
    double_check: t.double_check,
    fail_mode: t.fail_mode,
    max_per_hour: t.max_per_hour,
    step_up: t.step_up,
  };
}
