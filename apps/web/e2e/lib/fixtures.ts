import { readFileSync } from "node:fs";
import { join } from "node:path";
import { test as base, expect, type Page } from "@playwright/test";
import { Api, login, type Json } from "./api";
import type { E2EState } from "../global-setup";

export { expect };

export function readState(): E2EState {
  return JSON.parse(readFileSync(join(__dirname, "..", ".run", "state.json"), "utf8"));
}

interface Fixtures {
  state: E2EState;
  api: Api;
  /** A page signed in as the run's user via the real login form. */
  app: Page;
  /** Create a cash account through the API with a unique name. */
  mkCash: (name: string, currency: string, balance: number) => Promise<Json>;
  consoleErrors: string[];
}

/**
 * Console errors fail the test, except the expected 401 of the session
 * bootstrap (POST /auth/refresh while signed out).
 */
function watchConsole(page: Page, sink: string[]) {
  page.on("console", (msg) => {
    if (msg.type() !== "error") return;
    const url = msg.location().url;
    if (/401/.test(msg.text()) && /\/auth\/refresh/.test(url)) return;
    sink.push(`${msg.text()} (${url})`);
  });
  page.on("pageerror", (err) => sink.push(`pageerror: ${err.message}`));
}

export const test = base.extend<Fixtures>({
  state: async ({}, use) => {
    await use(readState());
  },
  api: async ({ state }, use) => {
    await use(await login(state.email, state.password));
  },
  consoleErrors: async ({}, use) => {
    const errors: string[] = [];
    await use(errors);
    expect(errors, "console errors").toEqual([]);
  },
  app: async ({ page, state, consoleErrors }, use) => {
    watchConsole(page, consoleErrors);
    await page.goto("/login");
    await page.locator("#email").fill(state.email);
    await page.locator("#password").fill(state.password);
    await page.getByRole("button", { name: /sign in|log in/i }).click();
    await page.waitForURL((u) => !u.pathname.startsWith("/login"));
    await use(page);
  },
  mkCash: async ({ api }, use) => {
    await use(async (name, currency, balance) => {
      const r = await api.post("/api/v1/accounts/cash", {
        name,
        currency,
        initial_balance: balance,
      });
      return r.account;
    });
  },
});

let counter = 0;
/** A short unique suffix so names never collide across tests and re-runs. */
export function uid(prefix = "x"): string {
  counter += 1;
  return `${prefix}${Date.now().toString(36).slice(-4)}${counter}`;
}
