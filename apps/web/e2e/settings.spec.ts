import { test, expect } from "./lib/fixtures";
import { expectToast } from "./lib/ui";

test.describe("settings", () => {
  test("edit the profile name", async ({ freshApp }) => {
    const { page, api } = freshApp;
    await page.goto("/settings?tab=profile");
    await page.locator("#profile-first-name").fill("Renamed");
    await page.locator("#profile-last-name").fill("Person");
    await page.getByRole("button", { name: "Save", exact: true }).click();
    await expectToast(page, /saved|updated/i);
    const me = (await api.get("/api/v1/profile")).user ?? (await api.get("/api/v1/profile"));
    expect(me.first_name).toBe("Renamed");
    expect(me.last_name).toBe("Person");
    await page.reload();
    await expect(page.locator("#profile-first-name")).toHaveValue("Renamed");
  });

  test("change password, stay signed in, sign in with the new one", async ({
    freshApp,
    browser,
  }) => {
    const { page, email, password } = freshApp;
    await page.goto("/settings?tab=profile");
    await page.getByRole("button", { name: "Change password" }).click();
    const dlg = page.getByRole("dialog");
    await dlg.locator("#pw-current").fill(password);
    await dlg.locator("#pw-new").fill("brand-new-pass-2");
    await dlg.locator("#pw-confirm").fill("brand-new-pass-2");
    await dlg.getByRole("button", { name: /change password|save|update/i }).last().click();
    await expect(dlg).toBeHidden();

    // This device keeps its session.
    await page.reload();
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    await expect(page).not.toHaveURL(/login/);

    // The old password no longer works; the new one does.
    const ctx = await browser.newContext();
    const other = await ctx.newPage();
    await other.goto("/login");
    await other.locator("#email").fill(email);
    await other.locator("#password").fill(password);
    await other.getByRole("button", { name: /sign in/i }).click();
    await expect(other.getByText(/invalid|incorrect|wrong/i).first()).toBeVisible();
    await other.locator("#password").fill("brand-new-pass-2");
    await other.getByRole("button", { name: /sign in/i }).click();
    await other.waitForURL((u) => !u.pathname.startsWith("/login"));
    await ctx.close();
  });

  test("signed-in devices list shows this device and can sign out others", async ({
    freshApp,
    browser,
  }) => {
    const { page, email, password } = freshApp;
    // A second device.
    const ctx = await browser.newContext();
    const second = await ctx.newPage();
    await second.goto("/login");
    await second.locator("#email").fill(email);
    await second.locator("#password").fill(password);
    await second.getByRole("button", { name: /sign in/i }).click();
    await second.waitForURL((u) => !u.pathname.startsWith("/login"));

    await page.goto("/settings?tab=profile");
    await expect(page.getByText("This device")).toBeVisible();
    await expect(page.getByRole("button", { name: /^Sign out/ })).not.toHaveCount(0);
    await page.getByRole("button", { name: "Sign out everywhere else" }).click();
    const confirm = page.getByRole("alertdialog");
    if (await confirm.count()) await confirm.getByRole("button", { name: /sign out/i }).click();
    await expect(page.getByRole("button", { name: /^Sign out Unknown device|^Sign out (Linux|Windows|Mac)/ })).toHaveCount(0);

    // The other device is signed out; this one is not.
    await second.reload();
    await expect(second).toHaveURL(/login/);
    await page.reload();
    await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
    await ctx.close();
  });

  test("palette choice persists after reload and on the API", async ({ freshApp }) => {
    const { page, api } = freshApp;
    await page.goto("/settings?tab=appearance");
    await page.getByRole("button", { name: /^Nord/ }).click();
    await expect
      .poll(async () => {
        const p = await api.get("/api/v1/profile");
        return (p.user ?? p).appearance?.palette;
      })
      .toBe("nord");
    const accentBefore = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue("--accent").trim()
    );
    await page.reload();
    await expect(page.getByRole("button", { name: /^Nord/ })).toHaveAttribute("aria-pressed", "true");
    const accentAfter = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue("--accent").trim()
    );
    expect(accentAfter).toBe(accentBefore);
    expect(accentAfter).not.toBe("");
  });

  test("hide balances masks amounts and persists", async ({ freshApp }) => {
    const { page, api } = freshApp;
    await api.post("/api/v1/accounts/cash", { name: "Mask me", currency: "MYR", initial_balance: 123_456 });
    await page.goto("/accounts");
    await expect(page.getByText(/RM\s*1,234\.56/).first()).toBeVisible();
    await page.getByRole("button", { name: "Hide balances" }).click();
    await expect(page.getByText(/1,234\.56/)).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Show balances" })).toBeVisible();
    await expect
      .poll(async () => ((await api.get("/api/v1/profile")).user ?? {}).hide_balances)
      .toBe(true);
    await page.reload();
    await expect(page.getByText(/1,234\.56/)).toHaveCount(0);
    await page.getByRole("button", { name: "Show balances" }).click();
    await expect(page.getByText(/RM\s*1,234\.56/).first()).toBeVisible();
  });

  test("export downloads a zip of CSVs", async ({ app }) => {
    await app.goto("/settings?tab=data");
    const [download] = await Promise.all([
      app.waitForEvent("download"),
      app.getByRole("button", { name: "Export all data" }).click(),
    ]);
    expect(download.suggestedFilename()).toMatch(/\.zip$/);
    const path = await download.path();
    const { readFileSync } = await import("node:fs");
    const head = readFileSync(path!).subarray(0, 4);
    // "PK\x03\x04": a zip local file header.
    expect([...head]).toEqual([0x50, 0x4b, 0x03, 0x04]);
  });

  test("log out returns to the login page", async ({ freshApp }) => {
    const { page } = freshApp;
    await page.goto("/settings?tab=profile");
    await page.getByRole("button", { name: "Log out" }).click();
    const confirm = page.getByRole("alertdialog");
    if (await confirm.count()) await confirm.getByRole("button", { name: /log out/i }).click();
    await expect(page).toHaveURL(/login/);
    await page.goto("/transactions");
    await expect(page).toHaveURL(/login/);
  });
});
