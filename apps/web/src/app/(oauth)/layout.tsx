import type { ReactNode } from "react";

/**
 * Layout for the OAuth login/consent pages. Unlike the (auth) group, it never
 * redirects based on the app's own session: these pages resolve Hydra's login
 * and consent challenges and must render even when a challenge-less user (or a
 * signed-in user completing consent) lands here. See plans/015-mcp-oauth-hydra.
 */
export default function OAuthLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <div className="w-full max-w-md">{children}</div>
    </div>
  );
}
