// Thin typed client for the Kuberan API, used by the e2e setup and by tests to
// assert on server state (balances in cents) after a UI action.

export const API_URL = process.env.E2E_API_URL ?? "http://localhost:8195";
export const PIPELINE_KEY = process.env.E2E_PIPELINE_KEY ?? "";

export class ApiError extends Error {
  constructor(
    public status: number,
    public body: unknown,
    message: string
  ) {
    super(message);
  }
}

// API payloads are asserted field by field in tests; a structural type per
// endpoint would only restate the Go DTOs.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type Json = any;

type Query = Record<string, string | number | boolean | undefined>;

async function call(
  method: string,
  path: string,
  opts: { token?: string; body?: unknown; key?: string; query?: Query } = {}
): Promise<Json> {
  const url = new URL(API_URL + path);
  for (const [k, v] of Object.entries(opts.query ?? {})) {
    if (v !== undefined) url.searchParams.set(k, String(v));
  }
  const headers: Record<string, string> = {};
  if (opts.body !== undefined) headers["Content-Type"] = "application/json";
  if (opts.token) headers["Authorization"] = `Bearer ${opts.token}`;
  if (opts.key) headers["X-API-Key"] = opts.key;
  const res = await fetch(url, {
    method,
    headers,
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  });
  const text = await res.text();
  let json: Json = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    json = text;
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      json,
      `${method} ${path} -> ${res.status} ${text.slice(0, 400)}`
    );
  }
  return json;
}

/** An authenticated API session for one user. */
export class Api {
  constructor(public token: string) {}

  get(path: string, query?: Query) {
    return call("GET", path, { token: this.token, query });
  }
  post(path: string, body?: unknown) {
    return call("POST", path, { token: this.token, body: body ?? {} });
  }
  put(path: string, body?: unknown) {
    return call("PUT", path, { token: this.token, body });
  }
  patch(path: string, body?: unknown) {
    return call("PATCH", path, { token: this.token, body });
  }
  del(path: string) {
    return call("DELETE", path, { token: this.token });
  }

  async accounts(): Promise<Json[]> {
    const r = await this.get("/api/v1/accounts");
    return Array.isArray(r) ? r : (r.accounts ?? r.data);
  }
  async account(id: string): Promise<Json> {
    const r = await this.get(`/api/v1/accounts/${id}`);
    return r.account ?? r;
  }
  async balance(id: string): Promise<number> {
    return (await this.account(id)).balance;
  }
  async transactions(query: Query = {}): Promise<Json[]> {
    const r = await this.get("/api/v1/transactions", { page_size: 100, ...query });
    return Array.isArray(r) ? r : (r.transactions ?? r.data ?? r.items);
  }
}

export async function login(email: string, password: string): Promise<Api> {
  const r = await call("POST", "/api/v1/auth/login", { body: { email, password } });
  return new Api(r.access_token ?? r.token);
}

export function pipeline(path: string, body: unknown) {
  return call("POST", path, { key: PIPELINE_KEY, body });
}
export function register(body: unknown) {
  return call("POST", "/api/v1/auth/register", { body });
}
