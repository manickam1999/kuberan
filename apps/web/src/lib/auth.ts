import { tokenStore } from "./api-client";
import { isTokenExpired as checkTokenExpired } from "./token-utils";

// Re-export token utilities for convenience
export const {
  getAccessToken,
  setAccessToken,
  getRefreshToken,
  setRefreshToken,
  clearTokens,
} = tokenStore;

// Re-export JWT parsing utilities (single source of truth in token-utils.ts)
export { parseJwtPayload, isTokenExpired } from "./token-utils";

// Re-export cookie utilities (single source of truth in auth-cookie.ts)
export { AUTH_COOKIE_NAME, setAuthCookie, clearAuthCookie } from "./auth-cookie";

/**
 * Check if the user has a non-expired access token stored.
 */
export function isAuthenticated(): boolean {
  const token = getAccessToken();
  if (!token) return false;
  return !checkTokenExpired(token);
}
