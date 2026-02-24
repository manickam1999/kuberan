import type { ApiError } from "@/types/api";
import { clearAuthCookie } from "./auth-cookie";
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

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  query?: Record<string, string | number | boolean | undefined | null>
): Promise<T> {
  const url = `${API_BASE_URL}${path}${buildQueryString(query)}`;
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };

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
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });

  // Attempt token refresh on 401 (unless this is already an auth request)
  if (res.status === 401 && !NO_REFRESH_PATHS.includes(path)) {
    const refreshed = await attemptTokenRefresh();
    if (refreshed) {
      headers["Authorization"] = `Bearer ${getAccessToken()}`;
      res = await fetch(url, {
        method,
        headers,
        body: body !== undefined ? JSON.stringify(body) : undefined,
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
};

// Re-export token utilities for use by auth provider
export const tokenStore = {
  getAccessToken,
  setAccessToken,
  getRefreshToken,
  setRefreshToken,
  clearTokens,
};
