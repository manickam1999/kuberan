import type { ApiError, RefreshResponse } from "@/types/api";
import type { User } from "@/types/models";
import { clearAuthCookie, hasAuthCookie } from "./auth-cookie";
import { isTokenExpired } from "./token-utils";

/**
 * Dynamically determine the API base URL based on the current environment.
 * - In production or when explicitly set: uses NEXT_PUBLIC_API_URL
 * - Server-side (SSR): defaults to localhost
 * - Client-side: detects hostname and protocol from window.location
 *
 * This allows seamless API access from:
 * - Desktop: http://localhost:3000 → http://localhost:8080
 * - Mobile (internal IP): http://192.168.x.x:3000 → http://192.168.x.x:8080
 * - Production: https://domain.com → https://domain.com:8080 (or custom via env)
 */
function getApiBaseUrl(): string {
  // 1. Explicit override for production or special configurations
  if (process.env.NEXT_PUBLIC_API_URL) {
    return process.env.NEXT_PUBLIC_API_URL;
  }

  // 2. Server-side rendering fallback
  if (typeof window === "undefined") {
    return "http://localhost:8080";
  }

  // 3. Dynamic client-side detection
  const protocol = window.location.protocol; // Matches frontend (http: or https:)
  const hostname = window.location.hostname; // localhost, 192.168.x.x, or domain
  const port = "8080";

  return `${protocol}//${hostname}:${port}`;
}

const API_BASE_URL = getApiBaseUrl();

// Session model:
// - The access token lives only in this module's memory, so an XSS payload
//   cannot lift a long-lived credential out of localStorage.
// - The refresh token is an HttpOnly `kuberan_refresh` cookie scoped to
//   /api/v1/auth; JS never sees it. Auth endpoints are called with
//   `credentials: "include"` so the browser sends and stores it.
// - On app start the session is restored with POST /auth/refresh, whose
//   response also carries the user (no separate /profile round trip).

// Legacy localStorage keys from before the cookie migration. The refresh token
// is sent once in the body of the first refresh so existing sessions survive
// the upgrade; both keys are then deleted.
const LEGACY_ACCESS_TOKEN_KEY = "kuberan_access_token";
const LEGACY_REFRESH_TOKEN_KEY = "kuberan_refresh_token";

let accessToken: string | null = null;

function getAccessToken(): string | null {
  return accessToken;
}

function setAccessToken(token: string | null): void {
  accessToken = token;
}

function readLegacyRefreshToken(): string | null {
  if (typeof window === "undefined") return null;
  try {
    return localStorage.getItem(LEGACY_REFRESH_TOKEN_KEY);
  } catch {
    return null;
  }
}

function removeLegacyTokens(): void {
  if (typeof window === "undefined") return;
  try {
    localStorage.removeItem(LEGACY_ACCESS_TOKEN_KEY);
    localStorage.removeItem(LEGACY_REFRESH_TOKEN_KEY);
  } catch {
    // Storage unavailable: nothing to migrate.
  }
}

function clearTokens(): void {
  accessToken = null;
  removeLegacyTokens();
}

// Error class for API errors
export class ApiClientError extends Error {
  code: string;
  status: number;

  constructor(code: string, message: string, status: number) {
    super(message);
    this.name = "ApiClientError";
    this.code = code;
    this.status = status;
  }
}

// Build query string from params object, omitting undefined/null values
function buildQueryString(
  params?: Record<string, string | number | boolean | undefined | null>
): string {
  if (!params) return "";
  const entries = Object.entries(params).filter(
    ([, v]) => v !== undefined && v !== null
  );
  if (entries.length === 0) return "";
  const searchParams = new URLSearchParams();
  for (const [key, value] of entries) {
    searchParams.set(key, String(value));
  }
  return `?${searchParams.toString()}`;
}

// Endpoints that authenticate with credentials (or a Hydra challenge) rather
// than a Bearer token. They never trigger a token refresh and never wait for
// the session bootstrap: a 401 from them means "wrong password", not "session
// expired" (the OAuth login page must show the error, not bounce to /login).
const PUBLIC_PATH_PREFIXES = [
  "/api/v1/auth/",
  "/api/v1/oauth/login",
  "/api/v1/oauth/consent",
];

function isPublicPath(path: string): boolean {
  return PUBLIC_PATH_PREFIXES.some((prefix) => path.startsWith(prefix));
}

// The refresh cookie is scoped to /api/v1/auth, so only those calls need to
// carry credentials; everything else authenticates with the Bearer header.
function credentialsFor(path: string): RequestCredentials {
  return path.startsWith("/api/v1/auth/") ? "include" : "same-origin";
}

// Outcome of a refresh attempt. "rejected" means the server refused the
// refresh cookie (the session is really over); "failed" means the attempt
// could not complete (offline, server error) and the session may still be
// valid.
type RefreshStatus = "ok" | "rejected" | "failed";

interface RefreshResult {
  status: RefreshStatus;
  /** The session user, when the server included it. */
  user: User | null;
}

// Cross-tab lock name. Tabs share the refresh cookie, so serialising refreshes
// makes each one present the most recently rotated cookie.
const REFRESH_LOCK = "kuberan-token-refresh";

let refreshPromise: Promise<RefreshResult> | null = null;

async function refreshTokens(): Promise<RefreshResult> {
  const legacyRefreshToken = readLegacyRefreshToken();
  try {
    const res = await fetch(`${API_BASE_URL}/api/v1/auth/refresh`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      // The cookie carries the token. The body is only used once, to migrate
      // a session that predates the cookie.
      body: JSON.stringify(
        legacyRefreshToken ? { refresh_token: legacyRefreshToken } : {}
      ),
    });

    // 4xx means the server refused the token; 5xx, timeouts and rate
    // limits are transient.
    if (res.status >= 500 || res.status === 408 || res.status === 429) {
      return { status: "failed", user: null };
    }
    // The legacy token has now been presented and answered, so it is never
    // needed again (on success the server has set the cookie).
    if (legacyRefreshToken) removeLegacyTokens();
    if (!res.ok) {
      accessToken = null;
      return { status: "rejected", user: null };
    }

    const data = (await res.json()) as RefreshResponse;
    accessToken = data.access_token;
    return { status: "ok", user: data.user ?? null };
  } catch {
    return { status: "failed", user: null };
  }
}

function attemptTokenRefresh(): Promise<RefreshResult> {
  // Deduplicate concurrent refresh attempts within this tab.
  if (refreshPromise) return refreshPromise;

  const run =
    typeof navigator !== "undefined" && navigator.locks
      ? navigator.locks.request(REFRESH_LOCK, () => refreshTokens())
      : refreshTokens();
  refreshPromise = run.finally(() => {
    refreshPromise = null;
  });
  return refreshPromise;
}

/** Result of restoring the session on app start. */
export type SessionBootstrap =
  | { status: "authenticated"; user: User | null }
  | { status: "unauthenticated" }
  // The server could not be reached; the session may still be valid.
  | { status: "error" };

let bootstrapPromise: Promise<SessionBootstrap> | null = null;

const BOOTSTRAP_ATTEMPTS = 3;

async function runBootstrap(): Promise<SessionBootstrap> {
  // Without the auth flag cookie or a legacy token there is no session to
  // restore, so skip the request (and its 401) entirely.
  if (!hasAuthCookie() && !readLegacyRefreshToken()) {
    return { status: "unauthenticated" };
  }
  for (let attempt = 0; attempt < BOOTSTRAP_ATTEMPTS; attempt++) {
    const result = await attemptTokenRefresh();
    if (result.status === "ok") {
      return { status: "authenticated", user: result.user };
    }
    if (result.status === "rejected") {
      clearTokens();
      clearAuthCookie();
      return { status: "unauthenticated" };
    }
    if (attempt < BOOTSTRAP_ATTEMPTS - 1) {
      await new Promise((resolve) => setTimeout(resolve, 1000 * 2 ** attempt));
    }
  }
  return { status: "error" };
}

/**
 * Restore the session from the refresh cookie, once per page load. Requests
 * made before it settles wait for it, so pages can issue their queries in
 * parallel with the bootstrap instead of after it.
 */
export function bootstrapSession(): Promise<SessionBootstrap> {
  if (!bootstrapPromise) {
    bootstrapPromise = runBootstrap();
  }
  return bootstrapPromise;
}

/** Record a session established by login or register. */
function startSession(token: string, user: User): void {
  accessToken = token;
  removeLegacyTokens();
  bootstrapPromise = Promise.resolve({ status: "authenticated", user });
}

/** Forget the session locally (logout or a refused refresh). */
function endSession(): void {
  clearTokens();
  clearAuthCookie();
  bootstrapPromise = Promise.resolve({ status: "unauthenticated" });
}

function sessionExpired(): never {
  endSession();
  if (typeof window !== "undefined") {
    window.location.href = "/login";
  }
  throw new ApiClientError("UNAUTHORIZED", "Session expired", 401);
}

// How the caller wants the successful response body decoded.
type ResponseKind = "json" | "blob";

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  query?: Record<string, string | number | boolean | undefined | null>,
  responseKind: ResponseKind = "json"
): Promise<T> {
  const url = `${API_BASE_URL}${path}${buildQueryString(query)}`;
  const publicPath = isPublicPath(path);
  const credentials = credentialsFor(path);

  // FormData bodies must be sent as-is: the browser sets a multipart
  // Content-Type with the correct boundary, so we must NOT set it ourselves
  // (and must NOT JSON.stringify the body).
  const isFormData =
    typeof FormData !== "undefined" && body instanceof FormData;

  const headers: Record<string, string> = {};
  if (!isFormData && body !== undefined) {
    headers["Content-Type"] = "application/json";
  }

  const encodedBody =
    body === undefined
      ? undefined
      : isFormData
        ? (body as FormData)
        : JSON.stringify(body);

  if (!publicPath && !accessToken) {
    // Wait for the app-start refresh instead of sending a request that is
    // bound to 401.
    const session = await bootstrapSession();
    if (session.status === "unauthenticated") {
      throw new ApiClientError("UNAUTHORIZED", "Not signed in", 401);
    }
  }

  let token = accessToken;

  // Proactive refresh: if the token is expired (or about to expire within
  // 30s), refresh before sending to avoid a wasted 401 round-trip.
  if (token && !publicPath && isTokenExpired(token)) {
    const refreshed = await attemptTokenRefresh();
    if (refreshed.status === "ok") {
      token = accessToken;
    } else if (refreshed.status === "rejected") {
      // The session is over: do not send the request with a dead token.
      sessionExpired();
    } else if (isTokenExpired(token, 0)) {
      // Offline and the token has already expired: the request cannot
      // succeed, but the session may recover once the server is reachable.
      throw new ApiClientError("NETWORK_ERROR", "Could not reach the server", 0);
    }
    // Otherwise the token is still valid for a few seconds: use it.
  }

  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  let res = await fetch(url, {
    method,
    headers,
    body: encodedBody,
    credentials,
  });

  // Attempt token refresh on 401 (unless this is a credential endpoint).
  if (res.status === 401 && !publicPath) {
    let retryToken: string | null;
    if (accessToken && accessToken !== token) {
      // Another request refreshed while this one was in flight: retry with
      // the newer token instead of rotating the session again.
      retryToken = accessToken;
    } else {
      const refreshed = await attemptTokenRefresh();
      if (refreshed.status === "failed") {
        // Keep the session: the refresh cookie may still be valid once the
        // network or server recovers.
        throw new ApiClientError("NETWORK_ERROR", "Could not reach the server", 0);
      }
      if (refreshed.status === "rejected" || !accessToken) {
        sessionExpired();
      }
      retryToken = accessToken;
    }
    headers["Authorization"] = `Bearer ${retryToken}`;
    res = await fetch(url, {
      method,
      headers,
      body: encodedBody,
      credentials,
    });
  }

  if (!res.ok) {
    let code = "UNKNOWN_ERROR";
    let message = `Request failed with status ${res.status}`;
    try {
      const errorBody: ApiError = await res.json();
      code = errorBody.error.code;
      message = errorBody.error.message;
    } catch {
      // Response body is not valid JSON; keep defaults
    }
    throw new ApiClientError(code, message, res.status);
  }

  // Handle 204 No Content
  if (res.status === 204) {
    return undefined as T;
  }

  if (responseKind === "blob") {
    // A successful binary fetch must not be a JSON error masquerading as 200;
    // guard so a caller never renders a JSON body as a broken image.
    const contentType = res.headers.get("Content-Type") ?? "";
    if (contentType.includes("application/json")) {
      throw new ApiClientError(
        "UNEXPECTED_RESPONSE",
        "Expected binary content but received JSON",
        res.status
      );
    }
    return res.blob() as Promise<T>;
  }

  return res.json() as Promise<T>;
}

// Public API
export const apiClient = {
  get<T>(
    path: string,
    query?: Record<string, string | number | boolean | undefined | null>
  ): Promise<T> {
    return request<T>("GET", path, undefined, query);
  },

  post<T>(path: string, body?: unknown): Promise<T> {
    return request<T>("POST", path, body);
  },

  put<T>(path: string, body?: unknown): Promise<T> {
    return request<T>("PUT", path, body);
  },

  patch<T>(path: string, body?: unknown): Promise<T> {
    return request<T>("PATCH", path, body);
  },

  del<T>(path: string): Promise<T> {
    return request<T>("DELETE", path);
  },

  // Multipart upload. Pass a FormData body; the browser sets the multipart
  // Content-Type + boundary. Shares the auth/refresh path with other requests.
  upload<T>(path: string, form: FormData): Promise<T> {
    return request<T>("POST", path, form);
  },

  // Authenticated binary fetch (e.g. for rendering a private <img> via
  // URL.createObjectURL). Sends the Bearer token, returns the raw Blob.
  getBlob(path: string): Promise<Blob> {
    return request<Blob>("GET", path, undefined, undefined, "blob");
  },
};

/**
 * Revoke this device's refresh session on the server (which also clears the
 * refresh cookie). Best effort: logout proceeds locally even if it fails.
 */
async function revokeSession(): Promise<void> {
  const legacyRefreshToken = readLegacyRefreshToken();
  try {
    await fetch(`${API_BASE_URL}/api/v1/auth/logout`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(
        legacyRefreshToken ? { refresh_token: legacyRefreshToken } : {}
      ),
      keepalive: true,
    });
  } catch {
    // Ignore: the session expires on its own.
  }
}

// Session helpers for the auth provider.
export const tokenStore = {
  getAccessToken,
  setAccessToken,
  clearTokens,
  startSession,
  endSession,
  revokeSession,
};
