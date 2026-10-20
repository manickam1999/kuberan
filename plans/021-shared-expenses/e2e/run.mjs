import { chromium } from "playwright";
const S = process.argv[2];
const step = process.argv[3] || "all";
const browser = await chromium.launch({ executablePath: "/opt/pw-browsers/chromium-1194/chrome-linux/chrome" });
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, colorScheme: "dark" });
const page = await ctx.newPage();
const errors = [];
page.on("pageerror", (e) => errors.push("pageerror: " + e.message));
page.on("console", (m) => { if (m.type() === "error") errors.push("console: " + m.text()); });
const shot = async (name, opts = {}) => { await page.waitForTimeout(600); await page.screenshot({ path: `${S}/shots/${name}.png`, ...opts }); console.log("shot", name); };
try {
  await page.goto("http://localhost:3000/login");
  await page.getByLabel("Email").fill("demo@kuberan.app");
  await page.getByLabel("Password").fill("Password123!");
  await page.getByRole("button", { name: /sign in|log in/i }).click();
  await page.waitForURL((u) => !u.pathname.startsWith("/login"), { timeout: 30000 });
  await page.waitForLoadState("networkidle");
  const mod = await import(`./steps-${step}.mjs?${Date.now()}`);
  await mod.default({ page, shot, S });
} catch (e) {
  console.error("FAILED:", e.message.split("\n").slice(0, 6).join("\n"));
  await page.screenshot({ path: `${S}/shots/_failure.png` });
  process.exitCode = 1;
} finally {
  if (errors.length) console.log("BROWSER ERRORS:\n" + [...new Set(errors)].slice(0, 15).join("\n"));
  await browser.close();
}
