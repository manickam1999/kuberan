interface McpUrlEnv {
  url?: string;
  dev: boolean;
  hostname?: string;
  protocol?: string;
}

function currentEnv(): McpUrlEnv {
  return {
    url: process.env.NEXT_PUBLIC_MCP_URL,
    dev: process.env.NODE_ENV === "development",
    hostname:
      typeof window === "undefined" ? undefined : window.location.hostname,
    protocol:
      typeof window === "undefined" ? undefined : window.location.protocol,
  };
}

/**
 * The MCP connector address people paste into their assistant.
 * `NEXT_PUBLIC_MCP_URL` when set (production); in development the MCP server
 * runs next to the API on :8081. Null when it can't be known, so the UI points
 * at the setup docs instead of showing a guess.
 */
export function getMcpConnectorUrl(env: McpUrlEnv = currentEnv()): string | null {
  if (env.url) return env.url;
  if (env.dev && env.hostname) {
    return `${env.protocol ?? "http:"}//${env.hostname}:8081/mcp`;
  }
  return null;
}
