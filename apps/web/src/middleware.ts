import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

const AUTH_COOKIE_NAME = "kuberan_auth";

const AUTH_ROUTES = ["/login", "/register"];

// OAuth login/consent pages drive Hydra's flow and must stay reachable
// regardless of the app's own auth cookie: the caller (e.g. a Claude connector)
// may have no session, and even a signed-in user must be able to complete
// consent. See plans/015-mcp-oauth-hydra Phase 1.
const OAUTH_ROUTES = ["/oauth/login", "/oauth/consent"];

export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;
  const hasAuthCookie = request.cookies.has(AUTH_COOKIE_NAME);

  // OAuth flow pages are always public — never redirect either way.
  if (OAUTH_ROUTES.includes(pathname)) {
    return NextResponse.next();
  }

  // Authenticated user visiting auth pages → redirect to dashboard
  if (hasAuthCookie && AUTH_ROUTES.includes(pathname)) {
    return NextResponse.redirect(new URL("/", request.url));
  }

  // Unauthenticated user visiting protected pages → redirect to login
  if (!hasAuthCookie && !AUTH_ROUTES.includes(pathname)) {
    return NextResponse.redirect(new URL("/login", request.url));
  }

  return NextResponse.next();
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
