/** Base service with shared HTTP helpers. */

import { AuthProvider } from "../auth";
import { throwForStatus } from "../errors";

const SDK_VERSION = "0.1.0";

export interface HTTPOptions {
  baseUrl: string;
  auth?: AuthProvider;
  timeout?: number;
}

export class BaseService {
  protected baseUrl: string;
  protected auth?: AuthProvider;
  protected timeout: number;

  constructor(opts: HTTPOptions) {
    this.baseUrl = opts.baseUrl.replace(/\/+$/, "");
    this.auth = opts.auth;
    this.timeout = opts.timeout ?? 30_000;
  }

  protected async request<T>(
    method: string,
    path: string,
    opts?: { body?: unknown; params?: Record<string, string | number | boolean | undefined> }
  ): Promise<T> {
    let url = `${this.baseUrl}${path}`;
    if (opts?.params) {
      const searchParams = new URLSearchParams();
      for (const [k, v] of Object.entries(opts.params)) {
        if (v !== undefined && v !== null && v !== "") {
          searchParams.set(k, String(v));
        }
      }
      const qs = searchParams.toString();
      if (qs) url += `?${qs}`;
    }

    const headers: Record<string, string> = {
      "User-Agent": `kaivue-typescript/${SDK_VERSION}`,
      Accept: "application/json",
    };
    if (opts?.body !== undefined) {
      headers["Content-Type"] = "application/json";
    }
    if (this.auth) {
      this.auth.apply(headers);
    }

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeout);

    try {
      const resp = await fetch(url, {
        method,
        headers,
        body: opts?.body !== undefined ? JSON.stringify(opts.body) : undefined,
        signal: controller.signal,
      });

      const requestId = resp.headers.get("X-Request-Id") ?? undefined;

      if (!resp.ok) {
        let body: Record<string, unknown>;
        try {
          body = await resp.json();
        } catch {
          body = { message: await resp.text() };
        }
        throwForStatus(resp.status, body, requestId);
      }

      if (resp.status === 204) return {} as T;
      return (await resp.json()) as T;
    } finally {
      clearTimeout(timer);
    }
  }

  protected get<T>(
    path: string,
    params?: Record<string, string | number | boolean | undefined>
  ): Promise<T> {
    return this.request("GET", path, { params });
  }

  protected post<T>(path: string, body: unknown): Promise<T> {
    return this.request("POST", path, { body });
  }

  protected patch<T>(path: string, body: unknown): Promise<T> {
    return this.request("PATCH", path, { body });
  }

  protected del<T>(
    path: string,
    params?: Record<string, string | number | boolean | undefined>
  ): Promise<T> {
    return this.request("DELETE", path, { params });
  }
}
