import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

const AUTH_COOKIE_NAME = "kuberan_auth";

const AUTH_ROUTES = ["/login", "/register"];

// OAuth login/consent pages drive Hydra's flow and must stay reachable
// regardless of the app's own auth cookie: the caller (e.g. a Claude connector)
// may have no session, and even a signed-in user must be able to complete
// consent. See plans/015-mcp-oauth-hydra Phase 1.
const OAUTH_ROUTES = ["/oauth/login", "/oauth/consent"];

const isDev = process.env.NODE_ENV === "development";

/**
 * Origin the browser calls the API on, for connect-src. Mirrors
 * getApiBaseUrl() in lib/api-client.ts: NEXT_PUBLIC_API_URL when set (always
 * in production builds), otherwise the page's host on port 8080.
 */
function apiOrigin(request: NextRequest): string {
  const configured = process.env.NEXT_PUBLIC_API_URL;
  if (configured) {
    try {
      return new URL(configured).origin;
    } catch {
      // Fall through to the dynamic default.
    }
  }
  return `${request.nextUrl.protocol}//${request.nextUrl.hostname}:8080`;
}

/**
 * Per-request Content-Security-Policy. Scripts must carry the nonce (Next.js
 * adds it to its own scripts by reading this header from the request; the root
 * layout passes it to the theme scripts); 'strict-dynamic' lets those scripts
 * load the app's chunks. Styles keep 'unsafe-inline' because Tailwind/Radix
 * set inline style attributes.
 */
function buildCsp(nonce: string, request: NextRequest): string {
  const directives = [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'${isDev ? " 'unsafe-eval'" : ""}`,
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' data: blob:",
    "font-src 'self' data:",
    `connect-src 'self' ${apiOrigin(request)}`,
    "object-src 'none'",
    "base-uri 'none'",
    // No page may be framed by another site. This blocks clickjacking, most
    // importantly of the OAuth consent page, where a click can grant an app
    // access to the user's data.
    "frame-ancestors 'none'",
    // The OAuth pages submit via fetch and then navigate to Hydra with
    // window.location, which form-action does not govern.
    "form-action 'self'",
  ];
  return directives.join("; ");
}

function generateNonce(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  let binary = "";
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary);
}

function withCsp(response: NextResponse, csp: string): NextResponse {
  response.headers.set("Content-Security-Policy", csp);
  return response;
}

export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;
  const hasAuthCookie = request.cookies.has(AUTH_COOKIE_NAME);

  const nonce = generateNonce();
  const csp = buildCsp(nonce, request);

  // Authenticated user visiting auth pages → redirect to dashboard.
  // Unauthenticated user visiting protected pages → redirect to login.
  // OAuth flow pages are always public — never redirect either way.
  if (!OAUTH_ROUTES.includes(pathname)) {
    if (hasAuthCookie && AUTH_ROUTES.includes(pathname)) {
      return withCsp(NextResponse.redirect(new URL("/", request.url)), csp);
    }
    if (!hasAuthCookie && !AUTH_ROUTES.includes(pathname)) {
      return withCsp(NextResponse.redirect(new URL("/login", request.url)), csp);
    }
  }

  // Next.js reads the nonce from the request's CSP header; the root layout
  // reads x-nonce for its own inline scripts.
  const requestHeaders = new Headers(request.headers);
  requestHeaders.set("x-nonce", nonce);
  requestHeaders.set("Content-Security-Policy", csp);

  return withCsp(
    NextResponse.next({ request: { headers: requestHeaders } }),
    csp
  );
}

export const config = {
  matcher: [
    /*
     * Match all request paths except:
     * - _next/static (static files)
     * - _next/image (image optimization)
     * - favicon.ico, sitemap.xml, robots.txt (metadata files)
     * - api routes (handled by backend)
     */
    "/((?!_next/static|_next/image|favicon\\.ico|sitemap\\.xml|robots\\.txt|api).*)",
  ],
};
