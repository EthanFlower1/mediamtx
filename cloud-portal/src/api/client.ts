// Same-origin in production (portal embedded in cloudbroker behind cloud.raikada.com).
// In dev, Vite proxies /api/* and /kaivue.v1.* to localhost:8080 — see vite.config.ts.
// `VITE_API_BASE` lets you override (e.g., point dev portal at staging).
const API_BASE = (import.meta.env.VITE_API_BASE ?? '').replace(/\/$/, '');

const DEFAULT_TIMEOUT_MS = 10000;
const MAX_RETRIES = 3;
const INITIAL_BACKOFF_MS = 500;

export class ApiError extends Error {
  constructor(public status: number, public code: string | undefined, message: string, public details?: unknown) {
    super(message);
    this.name = 'ApiError';
  }
}

export class UnauthenticatedError extends ApiError {
  constructor() {
    super(401, 'unauthenticated', 'session expired');
    this.name = 'UnauthenticatedError';
  }
}

let onUnauthenticated: (() => void) | null = null;
export function setUnauthenticatedHandler(fn: (() => void) | null) {
  onUnauthenticated = fn;
}

let refreshPromise: Promise<boolean> | null = null;

async function fetchWithTimeout(url: string, init: RequestInit, timeoutMs: number): Promise<Response> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(url, { ...init, signal: controller.signal });
  } finally {
    clearTimeout(timer);
  }
}

function shouldRetry(error: unknown, response: Response | null): boolean {
  if (error instanceof TypeError) return true;
  if (error instanceof DOMException && error.name === 'AbortError') return true;
  if (response && response.status >= 500) return true;
  return false;
}

async function refreshSession(): Promise<boolean> {
  if (refreshPromise) return refreshPromise;
  refreshPromise = (async () => {
    try {
      const res = await fetchWithTimeout(`${API_BASE}/api/v1/session/refresh`, {
        method: 'POST',
        credentials: 'include',
      }, DEFAULT_TIMEOUT_MS);
      return res.ok;
    } catch {
      return false;
    } finally {
      refreshPromise = null;
    }
  })();
  return refreshPromise;
}

interface RequestOptions extends Omit<RequestInit, 'body'> {
  body?: unknown;
  timeoutMs?: number;
  /** If true, do not attempt to refresh on 401 — used by login/refresh themselves. */
  skipAuthRetry?: boolean;
}

async function request(path: string, options: RequestOptions = {}): Promise<Response> {
  const { body, timeoutMs = DEFAULT_TIMEOUT_MS, skipAuthRetry, headers: headersInit, ...rest } = options;
  const headers = new Headers(headersInit);
  if (body !== undefined && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  const init: RequestInit = {
    ...rest,
    headers,
    credentials: 'include',
    body: body === undefined ? undefined : (typeof body === 'string' ? body : JSON.stringify(body)),
  };
  const url = `${API_BASE}${path}`;

  let res: Response | null = null;
  let lastError: unknown = null;

  for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
    try {
      res = await fetchWithTimeout(url, init, timeoutMs);

      if (res.status === 401 && !skipAuthRetry) {
        const refreshed = await refreshSession();
        if (refreshed) {
          res = await fetchWithTimeout(url, init, timeoutMs);
        } else {
          onUnauthenticated?.();
          throw new UnauthenticatedError();
        }
      }

      if (!shouldRetry(null, res)) break;
      if (attempt < MAX_RETRIES) {
        lastError = new Error(`server ${res.status}`);
        res = null;
      } else {
        break;
      }
    } catch (err) {
      if (err instanceof UnauthenticatedError) throw err;
      lastError = err;
      res = null;
      if (!shouldRetry(err, null) || attempt >= MAX_RETRIES) break;
    }

    if (attempt < MAX_RETRIES) {
      await new Promise((r) => setTimeout(r, INITIAL_BACKOFF_MS * 2 ** attempt));
    }
  }

  if (!res) {
    throw lastError ?? new Error('request failed');
  }
  return res;
}

async function parseError(res: Response): Promise<ApiError> {
  let code: string | undefined;
  let message = res.statusText || `http ${res.status}`;
  let details: unknown;
  try {
    const data = (await res.json()) as { code?: string; message?: string; details?: unknown };
    code = data.code;
    if (data.message) message = data.message;
    details = data.details;
  } catch {
    // body wasn't JSON; keep statusText
  }
  return new ApiError(res.status, code, message, details);
}

/** REST helpers — `/api/v1/*` JSON endpoints on cloudbroker. */
export async function apiGet<T>(path: string, options?: RequestOptions): Promise<T> {
  const res = await request(path, { ...options, method: 'GET' });
  if (!res.ok) throw await parseError(res);
  return res.json() as Promise<T>;
}

export async function apiPost<T>(path: string, body?: unknown, options?: RequestOptions): Promise<T> {
  const res = await request(path, { ...options, method: 'POST', body });
  if (!res.ok) throw await parseError(res);
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export async function apiPut<T>(path: string, body?: unknown, options?: RequestOptions): Promise<T> {
  const res = await request(path, { ...options, method: 'PUT', body });
  if (!res.ok) throw await parseError(res);
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export async function apiDelete<T = void>(path: string, options?: RequestOptions): Promise<T> {
  const res = await request(path, { ...options, method: 'DELETE' });
  if (!res.ok) throw await parseError(res);
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

/**
 * Connect-Go JSON helper — calls `/kaivue.v1.{Service}/{Method}` with a JSON body
 * (Connect's "JSON over POST" wire format, no streaming). Replace this hand-rolled
 * client with @connectrpc/connect-web once KAI-310 ships generated clients.
 *
 * The protobuf namespace is intentionally `kaivue.v1` — the customer-facing brand
 * is "Raikada" but the wire paths are frozen until a coordinated rename.
 */
export async function connectCall<TReq, TRes>(
  service: string,
  method: string,
  req: TReq,
  options?: RequestOptions,
): Promise<TRes> {
  const res = await request(`/${service}/${method}`, {
    ...options,
    method: 'POST',
    body: req,
    headers: { ...(options?.headers ?? {}), 'Connect-Protocol-Version': '1' },
  });
  if (!res.ok) throw await parseError(res);
  return res.json() as Promise<TRes>;
}
