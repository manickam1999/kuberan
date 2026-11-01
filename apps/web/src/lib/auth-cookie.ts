// Cookie name used by middleware for fast route protection.
// The cookie is a simple flag ("1"), NOT a credential: the access token lives
// in memory and the refresh token in an HttpOnly cookie set by the API. It is
// not HttpOnly so JS can manage it, and contains no sensitive data.
export const AUTH_COOKIE_NAME = "kuberan_auth";

function secureAttribute(): string {
  return typeof window !== "undefined" && window.location.protocol === "https:"
    ? "; Secure"
    : "";
}

/**
 * Set a simple auth flag cookie for middleware route protection.
 * max-age matches the refresh token lifetime (7 days).
 */
export function setAuthCookie(): void {
  if (typeof document === "undefined") return;
  document.cookie = `${AUTH_COOKIE_NAME}=1; path=/; max-age=${7 * 24 * 60 * 60}; SameSite=Lax${secureAttribute()}`;
}

/**
 * Remove the auth flag cookie (on logout or session expiry).
 */
export function clearAuthCookie(): void {
  if (typeof document === "undefined") return;
  document.cookie = `${AUTH_COOKIE_NAME}=; path=/; max-age=0; SameSite=Lax${secureAttribute()}`;
}

/** Whether the auth flag cookie is present (a session probably exists). */
export function hasAuthCookie(): boolean {
  if (typeof document === "undefined") return false;
  return document.cookie
    .split(";")
    .some((part) => part.trim().startsWith(`${AUTH_COOKIE_NAME}=`));
}
