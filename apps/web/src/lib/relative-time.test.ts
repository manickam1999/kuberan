import { describe, expect, it } from "vitest";
import { formatRelativeTime } from "./relative-time";

const now = new Date("2026-11-09T12:00:00Z");

describe("formatRelativeTime", () => {
  it("formats recent times", () => {
    expect(formatRelativeTime("2026-11-09T11:59:40Z", now)).toBe("just now");
    expect(formatRelativeTime("2026-11-09T11:59:00Z", now)).toBe("1 minute ago");
    expect(formatRelativeTime("2026-11-09T10:00:00Z", now)).toBe("2 hours ago");
    expect(formatRelativeTime("2026-10-31T12:00:00Z", now)).toBe("9 days ago");
  });
  it("falls back to a date, and to empty for junk", () => {
    expect(formatRelativeTime("2026-06-01T12:00:00Z", now)).toMatch(/2026/);
    expect(formatRelativeTime("nope", now)).toBe("");
  });
});
