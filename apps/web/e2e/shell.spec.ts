import { test, expect } from "./lib/fixtures";

const ROUTES = [
  "/",
  "/accounts",
  "/transactions",
  "/categories",
  "/investments",
  "/securities",
  "/analytics",
  "/budgets",
  "/receivables",
  "/rules",
  "/settings",
];

test.describe("shell", () => {
  test("every page loads without console errors", async ({ app }) => {
    for (const route of ROUTES) {
      await app.goto(route);
      await expect(app.locator("main, [role=main]").first()).toBeVisible();
      await app.waitForLoadState("networkidle");
      await expect(app.getByText(/something went wrong|application error/i)).toHaveCount(0);
    }
  });

  test("signed-out visitors are redirected to the login page", async ({ browser }) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await page.goto("/transactions");
    await expect(page).toHaveURL(/login/);
    await ctx.close();
  });

  test("Cmd/Ctrl+K searches transactions and opens the result", async ({ app }) => {
    await app.goto("/");
    await app.waitForLoadState("networkidle");
    await app.keyboard.press("Control+k");
    const palette = app.getByRole("dialog", { name: "Command Palette" });
    await expect(palette).toBeVisible();
    await app.keyboard.type("dinner");
    const hit = palette.getByRole("option", { name: /Seed dinner USD/ });
    await expect(hit).toBeVisible();
    await hit.click();
    await expect(app.getByRole("dialog").getByRole("heading", { name: "Seed dinner USD" })).toBeVisible();
    await expect(app.getByRole("dialog").getByRole("textbox", { name: "Amount" })).toHaveValue("50.00");
  });

  test("palette navigates and the g-key shortcuts work", async ({ app }) => {
    await app.goto("/");
    await app.waitForLoadState("networkidle");
    await app.keyboard.press("Control+k");
    await app.keyboard.type("budgets");
    await app.getByRole("option", { name: /Budgets/ }).first().click();
    await expect(app).toHaveURL(/\/budgets/);
    await app.locator("body").press("g");
    await app.locator("body").press("t");
    await expect(app).toHaveURL(/\/transactions/);
  });

  test("N opens the add dialog from any page", async ({ app }) => {
    await app.goto("/accounts");
    await app.waitForLoadState("networkidle");
    await app.locator("body").press("n");
    await expect(app.getByRole("dialog", { name: "New transaction" })).toBeVisible();
    await app.keyboard.press("Escape");
    await expect(app.getByRole("dialog")).toBeHidden();
  });
});
