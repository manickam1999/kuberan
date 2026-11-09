import { describe, expect, it } from "vitest";
import { spreadLabels } from "./label-layout";

const minGap = (ys: number[]) => {
  const s = [...ys].sort((a, b) => a - b);
  return Math.min(...s.slice(1).map((y, i) => y - s[i]));
};

describe("spreadLabels", () => {
  it("leaves labels that already have room alone", () => {
    expect(spreadLabels([20, 100, 200], 30, 0, 300)).toEqual([20, 100, 200]);
  });

  it("pushes a crowded run apart, keeping order", () => {
    // Five small categories a few px apart, like the Insights flow chart.
    const wanted = [150, 160, 172, 180, 188];
    const out = spreadLabels(wanted, 30, 0, 400);
    expect(minGap(out)).toBeGreaterThanOrEqual(30);
    expect(out).toEqual([...out].sort((a, b) => a - b));
    expect(out[0]).toBe(150);
  });

  it("keeps the input order of the result even when input is unsorted", () => {
    const out = spreadLabels([200, 100, 105], 30, 0, 400);
    expect(out[1]).toBe(100);
    expect(out[2]).toBe(130);
    expect(out[0]).toBe(200);
  });

  it("pulls a stack back up when it would run off the bottom", () => {
    const out = spreadLabels([280, 285, 290], 30, 0, 300);
    expect(Math.max(...out)).toBe(300);
    expect(minGap(out)).toBeGreaterThanOrEqual(30);
  });

  it("clamps into range", () => {
    expect(spreadLabels([-10], 30, 0, 100)).toEqual([0]);
    expect(spreadLabels([500], 30, 0, 100)).toEqual([100]);
  });

  it("handles no labels", () => {
    expect(spreadLabels([], 30, 0, 100)).toEqual([]);
  });
});
