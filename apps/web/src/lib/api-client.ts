import type { ApiError } from "@/types/api";
import { clearAuthCookie } from "./auth-cookie";
import { isTokenExpired } from "./token-utils";

declare global {
  interface Window {
    __KUBERAN_CONFIG__?: { apiUrl?: string };
  }
}

/**
 * Dynamically determine the API base URL based on the current environment.
 *
 * Priority:
 * 1. Server-side: API_URL env var (runtime, set in Docker), fallback to localhost
 * 2. Client-side: runtime config injected by server component (window.__KUBERAN_CONFIG__)
 * 3. Client-side fallback: auto-detect from window.location (same host, port 8080)
 */
function getApiBaseUrl(): string {
  // 1. Server-side: use API_URL env var directly
  if (typeof window === "undefined") {
    return process.env.NEXT_PUBLIC_API_URL || process.env.API_URL || "http://localhost:8080";
  }

  // 2. Client-side: check runtime config injected by server component
  if (window.__KUBERAN_CONFIG__?.apiUrl) {
    return window.__KUBERAN_CONFIG__.apiUrl;
  }

  // 3. Fallback: dynamic detection (same hostname, port 8080)
  const protocol = window.location.protocol;
  const hostname = window.location.hostname;
  return `${protocol}//${hostname}:8080`;
}

const API_BASE_URL = getApiBaseUrl();

// Token storage keys
const ACCESS_TOKEN_KEY = "kuberan_access_token";
const REFRESH_TOKEN_KEY = "kuberan_refresh_token";

// Token management (localStorage)
function getAccessToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(ACCESS_TOKEN_KEY);
}

function setAccessToken(token: string): void {
  localStorage.setItem(ACCESS_TOKEN_KEY, token);
}

function getRefreshToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(REFRESH_TOKEN_KEY);
}

function setRefreshToken(token: string): void {
  localStorage.setItem(REFRESH_TOKEN_KEY, token);
}

function clearTokens(): void {
  localStorage.removeItem(ACCESS_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
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

// Paths that should never trigger token refresh
const NO_REFRESH_PATHS = ["/api/v1/auth/login", "/api/v1/auth/register", "/api/v1/auth/refresh"];

let refreshPromise: Promise<boolean> | null = null;

async function attemptTokenRefresh(): Promise<boolean> {
  // Deduplicate concurrent refresh attempts
  if (refreshPromise) return refreshPromise;

  refreshPromise = (async () => {
    const refreshToken = getRefreshToken();
    if (!refreshToken) return false;

    try {
      const res = await fetch(`${API_BASE_URL}/api/v1/auth/refresh`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refresh_token: refreshToken }),
      });

      if (!res.ok) return false;

      const data = await res.json();
      setAccessToken(data.access_token);
      setRefreshToken(data.refresh_token);
      return true;
    } catch {
      return false;
    }
  })();

  try {
    return await refreshPromise;
  } finally {
    refreshPromise = null;
  }
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

  let token = getAccessToken();

  // Proactive refresh: if the token is expired (or about to expire within 30s),
  // attempt a refresh before sending the request to avoid a wasted 401 round-trip.
  if (token && isTokenExpired(token) && !NO_REFRESH_PATHS.includes(path)) {
    const refreshed = await attemptTokenRefresh();
    if (refreshed) {
      token = getAccessToken();
    }
    // If refresh failed, proceed anyway — the 401 handler below will handle cleanup.
  }

  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  let res = await fetch(url, {
    method,
    headers,
    body: encodedBody,
  });

  // Attempt token refresh on 401 (unless this is already an auth request)
  if (res.status === 401 && !NO_REFRESH_PATHS.includes(path)) {
    const refreshed = await attemptTokenRefresh();
    if (refreshed) {
      headers["Authorization"] = `Bearer ${getAccessToken()}`;
      res = await fetch(url, {
        method,
        headers,
        body: encodedBody,
      });
    } else {
      clearTokens();
      clearAuthCookie(); // Prevent middleware redirect loop on expired sessions
      if (typeof window !== "undefined") {
        window.location.href = "/login";
      }
      throw new ApiClientError("UNAUTHORIZED", "Session expired", 401);
    }
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

// Re-export token utilities for use by auth provider
export const tokenStore = {
  getAccessToken,
  setAccessToken,
  getRefreshToken,
  setRefreshToken,
  clearTokens,
};
