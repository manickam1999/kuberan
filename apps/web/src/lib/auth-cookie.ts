// Cookie name used by middleware for fast route protection.
// The cookie is a simple flag ("1"), NOT the actual JWT token.
// It is not httpOnly so JS can manage it, but contains no sensitive data.
export const AUTH_COOKIE_NAME = "kuberan_auth";

/**
 * Set a simple auth flag cookie for middleware route protection.
 * max-age matches the refresh token lifetime (7 days).
 */
export function setAuthCookie(): void {
  if (typeof document === "undefined") return;
  document.cookie = `${AUTH_COOKIE_NAME}=1; path=/; max-age=${7 * 24 * 60 * 60}; SameSite=Lax`;
}

/**
 * Remove the auth flag cookie (on logout or session expiry).
 */
export function clearAuthCookie(): void {
  if (typeof document === "undefined") return;
  document.cookie = `${AUTH_COOKIE_NAME}=; path=/; max-age=0`;
}
