/**
 * Google AIP HTTP Adapter (`aipClient.ts`).
 * Supports standard GET, POST, PATCH (with `updateMask` query parameter), DELETE,
 * and custom AIP-136 methods (`:login`, `:check`, `:scrub`, `:snapshot`, `:start`,
 * `:stop`, `:reboot`, `:openConsole`, `:restart`, `:freeze`), plus JWT Bearer
 * token injection and `protojson` int64 string/number normalization (`Number(val ?? 0)`).
 */

export interface AipErrorPayload {
  code?: number;
  message?: string;
  status?: string;
  error?: {
    code?: number;
    message?: string;
    status?: string;
  };
}

export class AipHttpError extends Error {
  constructor(
    public readonly statusCode: number,
    message: string,
    public readonly status?: string
  ) {
    super(message);
    this.name = 'AipHttpError';
  }
}

export type FetchLike = typeof fetch;

export function normalizeInt64(val: unknown): number {
  if (val === null || val === undefined || val === '') return 0;
  const parsed = Number(val);
  return Number.isFinite(parsed) ? parsed : 0;
}

export class AipClient {
  private token: string | null = null;
  private readonly baseUrl: string;
  private readonly fetchImpl: FetchLike;

  constructor(baseUrl = '/v1', fetchImpl?: FetchLike) {
    this.baseUrl = baseUrl.replace(/\/+$/, '');
    this.fetchImpl = fetchImpl || ((...args) => globalThis.fetch(...args));
    try {
      if (
        typeof window !== 'undefined' &&
        typeof window.localStorage !== 'undefined' &&
        typeof window.localStorage.getItem === 'function'
      ) {
        this.token = window.localStorage.getItem('nas_jwt_token');
      }
    } catch {
      this.token = null;
    }
  }

  getToken(): string | null {
    return this.token;
  }

  setToken(token: string | null): void {
    this.token = token;
    try {
      if (
        typeof window !== 'undefined' &&
        typeof window.localStorage !== 'undefined' &&
        typeof window.localStorage.setItem === 'function'
      ) {
        if (token) {
          window.localStorage.setItem('nas_jwt_token', token);
        } else {
          window.localStorage.removeItem('nas_jwt_token');
        }
      }
    } catch {
      // ignore in headless test environments
    }
  }

  buildUrl(
    resourcePath: string,
    query?: Record<string, string | number | boolean | undefined>
  ): string {
    const cleanPath = resourcePath.replace(/^\/+/, '');
    const fullPath = `${this.baseUrl}/${cleanPath}`;
    if (!query) return fullPath;

    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== null && value !== '') {
        params.set(key, String(value));
      }
    }
    const qs = params.toString();
    return qs ? `${fullPath}?${qs}` : fullPath;
  }

  async get<T>(
    resourcePath: string,
    query?: Record<string, string | number | boolean | undefined>
  ): Promise<T> {
    return this.request<T>('GET', this.buildUrl(resourcePath, query));
  }

  async post<T>(
    resourcePath: string,
    body?: unknown,
    query?: Record<string, string | number | boolean | undefined>
  ): Promise<T> {
    return this.request<T>('POST', this.buildUrl(resourcePath, query), body ?? {});
  }

  async patch<T>(
    resourcePath: string,
    body: unknown,
    updateMask: string[] = []
  ): Promise<T> {
    const maskParam = updateMask && updateMask.length > 0 ? updateMask.join(',') : undefined;
    return this.request<T>(
      'PATCH',
      this.buildUrl(resourcePath, { updateMask: maskParam }),
      body
    );
  }

  async delete(
    resourcePath: string,
    query?: Record<string, string | number | boolean | undefined>
  ): Promise<void> {
    await this.request<unknown>('DELETE', this.buildUrl(resourcePath, query));
  }

  /**
   * Executes a Google AIP-136 custom method: `POST /v1/{resource}:{customVerb}`
   */
  async customMethod<T>(
    resourcePath: string,
    customVerb:
      | 'login'
      | 'check'
      | 'scrub'
      | 'snapshot'
      | 'start'
      | 'stop'
      | 'reboot'
      | 'openConsole'
      | 'restart'
      | 'freeze'
      | 'sync'
      | (string & {}),
    body: unknown = {}
  ): Promise<T> {
    const cleanPath = resourcePath.replace(/^\/+/, '');
    const url = `${this.baseUrl}/${cleanPath}:${customVerb}`;
    return this.request<T>('POST', url, body);
  }

  private async request<T>(method: string, url: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = {
      Accept: 'application/json',
    };
    if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
    }
    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    }

    const response = await this.fetchImpl(url, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });

    if (response.status === 204) {
      return {} as T;
    }

    const text = await response.text();
    let parsed: any = {};
    if (text) {
      try {
        parsed = JSON.parse(text);
      } catch {
        parsed = { message: text };
      }
    }

    if (!response.ok) {
      const payload = parsed as AipErrorPayload;
      const errMsg =
        payload?.error?.message ||
        payload?.message ||
        `HTTP ${response.status} (${response.statusText})`;
      const errStatus = payload?.error?.status || payload?.status;
      throw new AipHttpError(response.status, errMsg, errStatus);
    }

    return parsed as T;
  }
}
