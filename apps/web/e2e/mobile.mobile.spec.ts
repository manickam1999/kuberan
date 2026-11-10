import { test, expect, uid } from "./lib/fixtures";
import { pickOption } from "./lib/ui";
import type { Json } from "./lib/api";

const ROUTES = [
  "/",
  "/accounts",
  "/transactions",
  "/investments",
  "/securities",
  "/analytics",
  "/budgets",
  "/receivables",
  "/categories",
  "/rules",
  "/more",
  "/settings",
];

test.describe("mobile (390px)", () => {
  test("pages fit the screen without sideways scrolling", async ({ app }) => {
    for (const route of ROUTES) {
      await app.goto(route);
      await app.waitForLoadState("networkidle");
      const overflow = await app.evaluate(() => ({
        scroll: document.documentElement.scrollWidth,
        inner: window.innerWidth,
      }));
      expect(overflow.scroll, `horizontal overflow on ${route}`).toBeLessThanOrEqual(overflow.inner);
    }
  });

  test("tab bar navigates between sections", async ({ app }) => {
    await app.goto("/");
    await app.getByRole("link", { name: "Activity" }).click();
    await expect(app).toHaveURL(/\/transactions/);
    await app.getByRole("link", { name: "Wealth" }).click();
    await expect(app).toHaveURL(/\/accounts|\/investments/);
    await app.getByRole("link", { name: "More" }).click();
    await expect(app).toHaveURL(/\/more/);
    await app.getByRole("link", { name: "Budgets" }).click();
    await expect(app).toHaveURL(/\/budgets/);
    await app.getByRole("link", { name: "Home", exact: true }).click();
    await expect(app).toHaveURL(/localhost:\d+\/$/);
  });

  test("add an expense with the + button", async ({ app, api, mkCash, state }) => {
    const acc = await mkCash(uid("Mob"), "MYR", 50_000);
    const desc = uid("mobile");
    await app.goto("/");
    await app.getByRole("button", { name: "Add transaction" }).click();
    const dlg = app.getByRole("dialog", { name: "New transaction" });
    await dlg.locator("#tx-amount").fill("12.50+8.90");
    await dlg.locator("#tx-amount").press("Tab");
    await expect(dlg.locator("#tx-amount")).toHaveValue("21.40");
    await dlg.getByRole("button", { name: "Groceries" }).click();
    await dlg.locator("#tx-description").fill(desc);
    await pickOption(app, dlg.locator("#tx-account"), new RegExp(acc.name));
    await dlg.getByRole("button", { name: "Save expense" }).click();
    await expect(dlg).toBeHidden();

    const tx = (await api.transactions({ q: desc }))[0];
    expect(tx.amount).toBe(2_140);
    expect(tx.category_id).toBe(state.categories["Groceries"]);
    expect(await api.balance(acc.id)).toBe(50_000 - 2_140);
  });

  test("cross-currency transfer on a phone", async ({ app, api, mkCash }) => {
    const myr = await mkCash(uid("MobFrom"), "MYR", 100_000);
    const sgd = await mkCash(uid("MobTo"), "SGD", 0);
    await app.goto("/");
    await app.getByRole("button", { name: "Add transaction" }).click();
    const dlg = app.getByRole("dialog");
    await dlg.getByRole("radio", { name: "Transfer" }).click();
    await dlg.locator("#tx-amount").fill("335");
    await pickOption(app, dlg.locator("#tx-from-account"), new RegExp(myr.name));
    await pickOption(app, dlg.locator("#tx-to-account"), new RegExp(sgd.name));
    // RM 335 at 3.35 is S$100.00.
    await expect(dlg.locator("#tx-to-amount")).toHaveValue("100.00");
    await dlg.getByRole("button", { name: "Transfer funds" }).click();
    await expect(dlg).toBeHidden();
    expect(await api.balance(myr.id)).toBe(100_000 - 33_500);
    expect(await api.balance(sgd.id)).toBe(10_000);
  });

  test("edit and delete from the activity list", async ({ app, api, mkCash, state }) => {
    const acc = await mkCash(uid("MobList"), "MYR", 0);
    const desc = uid("swipe");
    const tx = (
      await api.post("/api/v1/transactions", {
        account_id: acc.id,
        type: "expense",
        amount: 4_000,
        description: desc,
        category_id: state.categories["Dining"],
      })
    ).transaction as Json;
    await app.goto(`/transactions?account=${acc.id}`);
    await app.getByRole("button", { name: desc }).click();
    const dlg = app.getByRole("dialog");
    await dlg.getByRole("textbox", { name: "Amount" }).fill("45");
    await dlg.getByRole("textbox", { name: "Amount" }).press("Tab");
    await dlg.getByRole("button", { name: "Save", exact: true }).click();
    await expect(dlg).toBeHidden();
    expect((await api.get(`/api/v1/transactions/${tx.id}`)).transaction.amount).toBe(4_500);
    expect(await api.balance(acc.id)).toBe(-4_500);

    await app.getByRole("button", { name: desc }).click();
    await app.getByRole("dialog").getByRole("button", { name: "Delete", exact: true }).click();
    await app.getByRole("alertdialog").getByRole("button", { name: /delete/i }).click();
    await expect(app.getByRole("button", { name: desc })).toHaveCount(0);
    expect(await api.balance(acc.id)).toBe(0);
  });

  test("record a buy on a holding", async ({ app, api, state, mkCash }) => {
    const broker = (
      await api.post("/api/v1/accounts/investment", { name: uid("MobBrk"), currency: "USD" })
    ).account;
    const inv = (
      await api.post("/api/v1/investments", {
        account_id: broker.id,
        security_id: state.usdSecurityId,
        quantity: 5,
        purchase_price: 10_000,
        date: new Date().toISOString(),
      })
    ).investment;
    const usd = await mkCash(uid("MobUsd"), "USD", 100_000);
    await app.goto(`/investments/${inv.id}`);
    await app.getByRole("button", { name: "Buy", exact: true }).click();
    const dlg = app.getByRole("dialog");
    await dlg.locator("#trade-quantity").fill("2");
    await dlg.locator("#trade-price").fill("120");
    await pickOption(app, dlg.locator("#trade-account"), new RegExp(usd.name));
    await dlg.getByRole("button", { name: "Record buy" }).click();
    await expect(dlg).toBeHidden();
    expect(await api.balance(usd.id)).toBe(100_000 - 24_000);
    const h = (await api.get(`/api/v1/investments/${inv.id}`)).investment;
    expect(h.quantity).toBe(7);
    expect(h.cost_basis).toBe(50_000 + 24_000);
  });
});
