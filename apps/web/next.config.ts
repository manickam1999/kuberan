import type { NextConfig } from "next";

const isProd = process.env.NODE_ENV === "production";

// Static security headers for every response. The Content-Security-Policy is
// per-request (it carries a script nonce), so it is set in src/middleware.ts.
const securityHeaders = [
  // Legacy clickjacking guard alongside CSP frame-ancestors 'none' (which is
  // what protects the OAuth consent page in modern browsers).
  { key: "X-Frame-Options", value: "DENY" },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  {
    key: "Permissions-Policy",
    value: "camera=(), microphone=(), geolocation=()",
  },
  // Browsers ignore HSTS over plain http, but only send it from production
  // builds so a local `next start` on another port never pins anything.
  ...(isProd
    ? [
        {
          key: "Strict-Transport-Security",
          value: "max-age=63072000",
        },
      ]
    : []),
];

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
  async headers() {
    return [
      {
        source: "/:path*",
        headers: securityHeaders,
      },
    ];
  },
};

export default nextConfig;
