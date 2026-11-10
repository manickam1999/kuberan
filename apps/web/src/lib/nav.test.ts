import { describe, expect, it } from "vitest";
import { MAIN_NAV, activeTab, isNavActive, pageTitle } from "./nav";

const byTitle = (t: string) => MAIN_NAV.find((n) => n.title === t)!;

describe("nav", () => {
  it("labels /analytics as Insights", () => {
    expect(byTitle("Insights").href).toBe("/analytics");
    expect(pageTitle("/analytics")).toBe("Insights");
  });

  it("has a unique go-to key for each main destination", () => {
    const keys = MAIN_NAV.map((n) => n.goKey);
    expect(keys.every(Boolean)).toBe(true);
    expect(new Set(keys).size).toBe(MAIN_NAV.length);
  });

  it("matches nested paths but not look-alike prefixes", () => {
    const accounts = byTitle("Accounts");
    expect(isNavActive("/accounts/abc", accounts)).toBe(true);
    expect(isNavActive("/accounts-old", accounts)).toBe(false);
    expect(isNavActive("/", byTitle("Home"))).toBe(true);
    expect(isNavActive("/accounts", byTitle("Home"))).toBe(false);
  });

  it("keeps Securities under Investments", () => {
    expect(isNavActive("/securities/x", byTitle("Investments"))).toBe(true);
  });

  it("maps paths to the bottom tab", () => {
    expect(activeTab("/")).toBe("home");
    expect(activeTab("/transactions")).toBe("activity");
    expect(activeTab("/accounts/1")).toBe("wealth");
    expect(activeTab("/investments")).toBe("wealth");
    expect(activeTab("/budgets")).toBe("more");
    expect(activeTab("/settings")).toBe("more");
  });

  it("titles detail pages", () => {
    expect(pageTitle("/accounts/1")).toBe("Account");
    expect(pageTitle("/nope")).toBe("Kuberan");
  });
});
