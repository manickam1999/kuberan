import { describe, expect, it } from "vitest";
import { parseSettingsTab } from "./settings-tabs";

describe("parseSettingsTab", () => {
  it("accepts every known tab", () => {
    for (const t of [
      "profile",
      "appearance",
      "telegram",
      "connected-apps",
      "data",
    ]) {
      expect(parseSettingsTab(t)).toBe(t);
    }
  });
  it("falls back to profile", () => {
    expect(parseSettingsTab(null)).toBe("profile");
    expect(parseSettingsTab("nope")).toBe("profile");
    expect(parseSettingsTab("")).toBe("profile");
  });
});
