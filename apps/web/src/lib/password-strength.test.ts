import { describe, expect, it } from "vitest";
import { passwordStrength } from "@/lib/password-strength";

describe("passwordStrength", () => {
  it.each([
    ["", 0],
    ["abc", 1],
    ["abcdefgh", 2],
    ["abcdefghijkl", 3],
    ["abcdefghijklmnop", 4],
  ])("%j -> level %i", (pw, level) => {
    expect(passwordStrength(pw).level).toBe(level);
  });

  it("reports the length", () => {
    expect(passwordStrength("abcdefghijkl").label).toBe("Good · 12 characters");
  });
});
