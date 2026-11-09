import type { ReactNode } from "react";

/**
 * Layout for the OAuth login/consent pages. Unlike the (auth) group, it never
 * redirects based on the app's own session: these pages resolve Hydra's login
 * and consent challenges and must render even when a challenge-less user (or a
 * signed-in user completing consent) lands here. Each page brings its own
 * frame (split sign-in layout, or the centred consent card). See
 * plans/015-mcp-oauth-hydra.
 */
export default function OAuthLayout({ children }: { children: ReactNode }) {
  return <div className="min-h-dvh">{children}</div>;
}
