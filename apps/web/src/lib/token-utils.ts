// JWT claims (only the fields we care about)
interface JwtPayload {
  exp?: number; // expiry as unix timestamp (seconds)
  sub?: string; // subject (user id)
}

/**
 * Parse JWT payload without verifying signature.
 * Returns null if the token is malformed.
 */
export function parseJwtPayload(token: string): JwtPayload | null {
  try {
    const parts = token.split(".");
    if (parts.length !== 3) return null;
    const payload = JSON.parse(atob(parts[1])) as JwtPayload;
    return payload;
  } catch {
    return null;
  }
}

/**
 * Check if a JWT token is expired (or will expire within bufferMs).
 * Returns true if expired/invalid, false if still valid.
 */
export function isTokenExpired(token: string, bufferMs = 30_000): boolean {
  const payload = parseJwtPayload(token);
  if (!payload?.exp) return true;
  return Date.now() >= payload.exp * 1000 - bufferMs;
}
