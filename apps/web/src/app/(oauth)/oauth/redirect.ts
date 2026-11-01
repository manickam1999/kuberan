// Navigation helpers shared by the OAuth login and consent pages.

/**
 * Returns redirectTo as an absolute http(s) URL, or null if it is anything
 * else. Hydra's redirect_to always points back at its own public endpoint, so
 * a javascript:, data: or relative value means a broken or tampered response
 * and must never be navigated to.
 */
export function safeRedirectURL(redirectTo: unknown): string | null {
  if (typeof redirectTo !== "string" || redirectTo === "") return null;
  try {
    const url = new URL(redirectTo);
    if (url.protocol !== "https:" && url.protocol !== "http:") return null;
    return url.toString();
  } catch {
    return null;
  }
}

/**
 * Follows Hydra's redirect_to. Throws when it is not an http(s) URL, so the
 * caller shows an error instead of navigating.
 */
export function followHydraRedirect(redirectTo: unknown): void {
  const url = safeRedirectURL(redirectTo);
  if (!url) {
    throw new Error("The authorization server returned an invalid redirect.");
  }
  window.location.assign(url);
}

/**
 * Describes where an authorization will send the user back to, for the
 * consent screen's anti-phishing line.
 */
export function describeRedirect(
  scheme: string | undefined,
  host: string | undefined
): { label: string; note: string | null } | null {
  if (!scheme) return null;
  if ((scheme === "https" || scheme === "http") && host) {
    let note: string | null = null;
    if (scheme === "http") {
      const hostname = host.replace(/:\d+$/, "");
      const loopback =
        hostname === "localhost" ||
        hostname === "[::1]" ||
        hostname.startsWith("127.");
      note = loopback
        ? "This is an app on this device (a local address)."
        : "This address is not encrypted (http).";
    }
    return { label: host, note };
  }
  return {
    label: `${scheme}:`,
    note: "This hands control to an app installed on this device, not a website. Continue only if you are setting up that app.",
  };
}
