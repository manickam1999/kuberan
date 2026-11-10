#!/usr/bin/env node
// Captures every Kuberan route in light and dark at 1440x900 and 390x844.
//
// It logs in to a RUNNING instance (it starts nothing) and writes viewport
// screenshots, animations disabled, to a directory:
//
//   <out>/<route>-<width>-<theme>.png      e.g. transactions-1440-dark.png
//
// Usage (from apps/web):
//   pnpm screenshots --email demo@kuberan.example.com --password '...' \
//     --web http://localhost:3000 --api http://localhost:8080 --out ./shots
//
//   --seed        register the account if the login fails, then load the sample
//                 household (POST /demo/load; the api needs DEMO_ENABLED=true).
//                 An already-loaded account is left alone.
//   --routes a,b  only these route slugs (see ROUTES below), e.g. dashboard,budgets
//   --themes      light,dark (default both)
//   --viewports   desktop,mobile (default both)
//   --settle MS   wait after each page load for charts to finish animating (1500)
//   --scale N     device scale factor (default 1; 2 for retina-sharp mobile)
//   --chromium P  browser executable. Default: Playwright's own browser, then
//                 /opt/pw-browsers/chromium when that one is not installed.
//
// Every flag also reads an env var: KUBERAN_WEB_URL, KUBERAN_API_URL,
// KUBERAN_EMAIL, KUBERAN_PASSWORD, KUBERAN_SHOTS_DIR. See docs/screenshots.md.

import { existsSync, mkdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { chromium } from "playwright";

const VIEWPORTS = {
  desktop: { width: 1440, height: 900, isMobile: false, hasTouch: false },
  mobile: { width: 390, height: 844, isMobile: true, hasTouch: true },
};

// slug -> path. `:account`, `:investment`, `:security` are filled in from the
// api (first of each). `public` routes are captured signed out.
const ROUTES = [
  { slug: "login", path: "/login", public: true },
  { slug: "register", path: "/register", public: true },
  { slug: "dashboard", path: "/" },
  { slug: "accounts", path: "/accounts" },
  { slug: "account-detail", path: "/accounts/:account" },
  { slug: "transactions", path: "/transactions" },
  { slug: "categories", path: "/categories" },
  { slug: "investments", path: "/investments" },
  { slug: "investment-detail", path: "/investments/:investment" },
  { slug: "securities", path: "/securities" },
  { slug: "security-detail", path: "/securities/:security" },
  { slug: "analytics", path: "/analytics" },
  { slug: "budgets", path: "/budgets" },
  { slug: "receivables", path: "/receivables" },
  { slug: "rules", path: "/rules" },
  { slug: "settings", path: "/settings" },
  { slug: "more", path: "/more" },
];

function parseArgs(argv) {
  const out = {};
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (!a.startsWith("--")) continue;
    const key = a.slice(2);
    const next = argv[i + 1];
    if (next === undefined || next.startsWith("--")) out[key] = true;
    else {
      out[key] = next;
      i++;
    }
  }
  return out;
}

const args = parseArgs(process.argv.slice(2));
const env = process.env;
const webUrl = String(args.web ?? env.KUBERAN_WEB_URL ?? "http://localhost:3000").replace(/\/$/, "");
const apiUrl = String(args.api ?? env.KUBERAN_API_URL ?? "http://localhost:8080").replace(/\/$/, "");
const email = args.email ?? env.KUBERAN_EMAIL;
const password = args.password ?? env.KUBERAN_PASSWORD;
const outDir = resolve(String(args.out ?? env.KUBERAN_SHOTS_DIR ?? "./screenshots"));
const scale = Number(args.scale ?? 1);
const settleMs = Number(args.settle ?? 1500);
const themes = String(args.themes ?? "light,dark").split(",");
const viewportNames = String(args.viewports ?? "desktop,mobile").split(",");
const only = args.routes ? String(args.routes).split(",") : null;

if (!email || !password) {
  console.error("Missing credentials: pass --email and --password (or KUBERAN_EMAIL / KUBERAN_PASSWORD).");
  process.exit(2);
}
for (const t of themes) if (!["light", "dark"].includes(t)) throw new Error(`unknown theme ${t}`);
for (const v of viewportNames) if (!VIEWPORTS[v]) throw new Error(`unknown viewport ${v}`);

async function api(method, path, { token, body } = {}) {
  const res = await fetch(`${apiUrl}/api/v1${path}`, {
    method,
    headers: {
      ...(body ? { "Content-Type": "application/json" } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let json = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    // non-JSON body
  }
  return { status: res.status, json };
}

async function login() {
  return api("POST", "/auth/login", { body: { email, password } });
}

/** Log in, registering and loading the sample household first when --seed. */
async function prepareAccount() {
  let res = await login();
  if (res.status !== 200 && args.seed) {
    const reg = await api("POST", "/auth/register", {
      body: { email, password, first_name: "Demo" },
    });
    if (reg.status !== 201 && reg.status !== 200) {
      throw new Error(`register failed (HTTP ${reg.status}): ${JSON.stringify(reg.json)}`);
    }
    res = await login();
  }
  if (res.status !== 200) {
    throw new Error(`login failed (HTTP ${res.status}): ${JSON.stringify(res.json)}`);
  }
  const token = res.json.access_token;
  if (args.seed) {
    const demo = await api("POST", "/demo/load", { token });
    if (demo.status === 201) console.log(`seeded the sample household (${demo.json?.total ?? "?"} rows)`);
    else if (demo.status === 409) console.log(`sample household not loaded: ${demo.json?.error?.code}`);
    else throw new Error(`POST /demo/load failed (HTTP ${demo.status}): ${JSON.stringify(demo.json)} (DEMO_ENABLED=true on the api?)`);
  }
  return token;
}

function firstId(payload, keys) {
  const list = Array.isArray(payload)
    ? payload
    : (keys.map((k) => payload?.[k]).find(Array.isArray) ?? Object.values(payload ?? {}).find(Array.isArray) ?? []);
  return list[0]?.id;
}

/** Resolve the dynamic route params (first account, investment and security). */
async function discoverParams(token) {
  const [accounts, investments, securities] = await Promise.all([
    api("GET", "/accounts", { token }),
    api("GET", "/investments", { token }),
    api("GET", "/securities", { token }),
  ]);
  return {
    account: firstId(accounts.json, ["accounts", "data"]),
    investment: firstId(investments.json, ["investments", "data"]),
    security: firstId(securities.json, ["securities", "data"]),
  };
}

function resolvePath(route, params) {
  const m = route.path.match(/:(\w+)/);
  if (!m) return route.path;
  const id = params[m[1]];
  return id ? route.path.replace(`:${m[1]}`, id) : null;
}

async function launch() {
  const explicit = args.chromium ?? env.KUBERAN_CHROMIUM;
  if (explicit) return chromium.launch({ executablePath: String(explicit) });
  try {
    return await chromium.launch();
  } catch (err) {
    // Playwright's own browser is not installed; fall back to a known path.
    const fallback = "/opt/pw-browsers/chromium";
    if (existsSync(fallback)) return chromium.launch({ executablePath: fallback });
    throw err;
  }
}

const NO_MOTION_CSS = `
  *, *::before, *::after {
    animation: none !important;
    transition: none !important;
    caret-color: transparent !important;
    scroll-behavior: auto !important;
  }
`;

async function newPage(browser, viewportName, theme) {
  const vp = VIEWPORTS[viewportName];
  const context = await browser.newContext({
    viewport: { width: vp.width, height: vp.height },
    isMobile: vp.isMobile,
    hasTouch: vp.hasTouch,
    deviceScaleFactor: scale,
    reducedMotion: "reduce",
    colorScheme: theme,
    locale: "en-MY",
  });
  // next-themes reads this key before first paint.
  await context.addInitScript((t) => {
    try {
      localStorage.setItem("theme", t);
    } catch {
      // storage unavailable
    }
  }, theme);
  const page = await context.newPage();
  page.on("pageerror", (e) => console.warn(`  page error: ${e.message}`));
  return { context, page };
}

async function settle(page) {
  await page.waitForLoadState("networkidle").catch(() => {});
  await page.addStyleTag({ content: NO_MOTION_CSS });
  await page.evaluate(() => document.fonts.ready);
  // Chart libraries animate in JS; give them a beat after the data arrives.
  await page.waitForTimeout(settleMs);
}

async function signIn(page) {
  await page.goto(`${webUrl}/login`);
  await page.fill("#email", email);
  await page.fill("#password", password);
  await Promise.all([
    page.waitForURL((u) => !u.pathname.startsWith("/login"), { timeout: 20000 }),
    page.click('button[type="submit"]'),
  ]);
}

async function main() {
  mkdirSync(outDir, { recursive: true });
  const token = await prepareAccount();
  const params = await discoverParams(token);
  const routes = ROUTES.filter((r) => !only || only.includes(r.slug));
  const browser = await launch();
  let count = 0;
  try {
    for (const viewportName of viewportNames) {
      for (const theme of themes) {
        const width = VIEWPORTS[viewportName].width;
        console.log(`${viewportName} ${width}px, ${theme}`);

        // Signed-out routes first (a signed-in session would redirect them).
        const pub = await newPage(browser, viewportName, theme);
        for (const route of routes.filter((r) => r.public)) {
          await pub.page.goto(`${webUrl}${route.path}`);
          await settle(pub.page);
          await pub.page.screenshot({ path: join(outDir, `${route.slug}-${width}-${theme}.png`) });
          count++;
        }
        await pub.context.close();

        const app = await newPage(browser, viewportName, theme);
        await signIn(app.page);
        for (const route of routes.filter((r) => !r.public)) {
          const path = resolvePath(route, params);
          if (!path) {
            console.warn(`  skip ${route.slug}: no ${route.path.match(/:(\w+)/)[1]} to open`);
            continue;
          }
          await app.page.goto(`${webUrl}${path}`);
          await settle(app.page);
          await app.page.screenshot({ path: join(outDir, `${route.slug}-${width}-${theme}.png`) });
          count++;
        }
        await app.context.close();
      }
    }
  } finally {
    await browser.close();
  }
  console.log(`wrote ${count} screenshots to ${outDir}`);
}

main().catch((err) => {
  console.error(err.message ?? err);
  process.exit(1);
});
