import { tokenStore } from "./api-client";
import { isTokenExpired as checkTokenExpired } from "./token-utils";

// Re-export session helpers for convenience
export const {
  getAccessToken,
  setAccessToken,
  clearTokens,
  startSession,
  endSession,
  revokeSession,
} = tokenStore;

export { bootstrapSession } from "./api-client";
export type { SessionBootstrap } from "./api-client";

// Re-export JWT parsing utilities (single source of truth in token-utils.ts)
export { parseJwtPayload, isTokenExpired } from "./token-utils";

// Re-export cookie utilities (single source of truth in auth-cookie.ts)
export {
  AUTH_COOKIE_NAME,
  setAuthCookie,
  clearAuthCookie,
  hasAuthCookie,
} from "./auth-cookie";

/**
 * Check if this tab holds a non-expired access token (in memory).
 */
export function isAuthenticated(): boolean {
  const token = getAccessToken();
  if (!token) return false;
  return !checkTokenExpired(token);
}
