import { describe, expect, it } from "vitest";
import { getMcpConnectorUrl } from "./mcp-url";

describe("getMcpConnectorUrl", () => {
  it("prefers the configured URL", () => {
    expect(
      getMcpConnectorUrl({ url: "https://mcp.example.com/mcp", dev: false })
    ).toBe("https://mcp.example.com/mcp");
  });
  it("derives it in development", () => {
    expect(
      getMcpConnectorUrl({ dev: true, hostname: "localhost", protocol: "http:" })
    ).toBe("http://localhost:8081/mcp");
  });
  it("is unknown in production without config", () => {
    expect(getMcpConnectorUrl({ dev: false, hostname: "kuberan.app" })).toBeNull();
  });
});
