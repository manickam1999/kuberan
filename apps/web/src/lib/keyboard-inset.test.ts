import { describe, expect, it } from "vitest";
import { keyboardInset } from "./keyboard-inset";

describe("keyboardInset", () => {
  it("is 0 with no keyboard", () => {
    expect(keyboardInset(844, { height: 844, offsetTop: 0, scale: 1 })).toBe(0);
  });

  it("is the height below the visual viewport when the keyboard is up", () => {
    expect(keyboardInset(844, { height: 508, offsetTop: 0, scale: 1 })).toBe(336);
  });

  it("subtracts the pan iOS applies to bring the input into view", () => {
    // The page was panned up 120px; the keyboard still covers 336px.
    expect(keyboardInset(844, { height: 388, offsetTop: 120, scale: 1 })).toBe(336);
  });

  it("ignores pinch zoom, which also shrinks the visual viewport", () => {
    expect(keyboardInset(844, { height: 422, offsetTop: 0, scale: 2 })).toBe(0);
  });

  it("ignores small differences from the URL bar collapsing", () => {
    expect(keyboardInset(844, { height: 790, offsetTop: 0, scale: 1 })).toBe(0);
  });
});
