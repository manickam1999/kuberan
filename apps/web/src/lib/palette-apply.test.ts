import { describe, expect, it } from "vitest";
import {
  PALETTE_COOKIE,
  PALETTE_INIT_SCRIPT,
  PALETTE_STORAGE_KEY,
} from "./palette-apply";

interface FakeEnv {
  cookie: string;
  storage?: Record<string, string> | "throws";
}

/** Runs the init script against a fake document and returns data-palette. */
function run({ cookie, storage }: FakeEnv): string | undefined {
  const dataset: Record<string, string> = {};
  const document = { cookie, documentElement: { dataset } };
  const localStorage = {
    getItem(key: string): string | null {
      if (storage === "throws") throw new Error("blocked");
      return storage?.[key] ?? null;
    },
  };
  new Function("document", "localStorage", PALETTE_INIT_SCRIPT)(
    document,
    localStorage
  );
  return dataset.palette;
}

describe("palette init script", () => {
  it("applies the palette from the cookie", () => {
    expect(run({ cookie: `a=1; ${PALETTE_COOKIE}=nord; b=2` })).toBe("nord");
  });

  it("prefers the cookie over localStorage", () => {
    expect(
      run({
        cookie: `${PALETTE_COOKIE}=gruvbox`,
        storage: { [PALETTE_STORAGE_KEY]: "nord" },
      })
    ).toBe("gruvbox");
  });

  it("falls back to localStorage without a usable cookie", () => {
    const storage = { [PALETTE_STORAGE_KEY]: "lagoon" };
    expect(run({ cookie: "", storage })).toBe("lagoon");
    expect(run({ cookie: `${PALETTE_COOKIE}=bogus`, storage })).toBe("lagoon");
  });

  it("leaves the attribute off for unknown or missing values", () => {
    expect(run({ cookie: "" })).toBeUndefined();
    expect(run({ cookie: `${PALETTE_COOKIE}=bogus` })).toBeUndefined();
    expect(
      run({ cookie: "", storage: { [PALETTE_STORAGE_KEY]: "<script>" } })
    ).toBeUndefined();
  });

  it("survives blocked storage", () => {
    expect(run({ cookie: "", storage: "throws" })).toBeUndefined();
    expect(run({ cookie: `${PALETTE_COOKIE}=nord`, storage: "throws" })).toBe(
      "nord"
    );
  });
});
