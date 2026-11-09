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
});
