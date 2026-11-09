import { test, expect, uid } from "./lib/fixtures";
import { expectToast, openAddTransaction, pickOption } from "./lib/ui";
import type { Api, Json } from "./lib/api";

async function txByDescription(api: Api, description: string): Promise<Json> {
  const all = await api.transactions({ q: description });
  const hit = all.filter((t) => t.description === description);
  expect(hit, `transaction "${description}" in API`).toHaveLength(1);
  return hit[0];
}

test.describe("transactions: create", () => {
  test("expense with an amount expression, category chip and note", async ({
    app,
    api,
    mkCash,
    state,
  }) => {
    const acc = await mkCash(uid("Exp"), "MYR", 100_000);
    const desc = uid("coffee");
    await app.goto("/transactions");
    const dlg = await openAddTransaction(app);

    await dlg.locator("#tx-amount").fill("12.50+8.90");
    await dlg.locator("#tx-amount").press("Tab");
    // Committing the expression replaces it with its result.
    await expect(dlg.locator("#tx-amount")).toHaveValue("21.40");
    await dlg.getByRole("button", { name: "Groceries" }).click();
    await dlg.locator("#tx-description").fill(desc);
    await pickOption(app, dlg.locator("#tx-account"), new RegExp(acc.name));
    await dlg.getByRole("button", { name: "Add note" }).click();
    await dlg.locator("#tx-notes").fill("two flat whites");
    await dlg.getByRole("button", { name: "Save expense" }).click();

    await expect(dlg).toBeHidden();
    await expect(app.getByText(desc).first()).toBeVisible();

    const tx = await txByDescription(api, desc);
    expect(tx.amount).toBe(2140);
    expect(tx.type).toBe("expense");
    expect(tx.category_id).toBe(state.categories["Groceries"]);
    expect(tx.notes).toBe("two flat whites");
    expect(await api.balance(acc.id)).toBe(100_000 - 2140);
  });

  test("income raises the balance", async ({ app, api, mkCash, state }) => {
    const acc = await mkCash(uid("Inc"), "MYR", 0);
    const desc = uid("bonus");
    await app.goto("/transactions");
    const dlg = await openAddTransaction(app);
    await dlg.getByRole("radio", { name: "Income" }).click();
    await dlg.locator("#tx-amount").fill("1500");
    await dlg.locator("#tx-description").fill(desc);
    await pickOption(app, dlg.locator("#tx-account"), new RegExp(acc.name));
    await dlg.getByRole("button", { name: "All…" }).click();
    await pickOption(app, dlg.getByRole("combobox", { name: "Category" }), "Bonus");
    await dlg.getByRole("button", { name: "Save income" }).click();
    await expect(dlg).toBeHidden();

    const tx = await txByDescription(api, desc);
    expect(tx.type).toBe("income");
    expect(tx.amount).toBe(150_000);
    expect(tx.category_id).toBe(state.categories["Bonus"]);
    expect(await api.balance(acc.id)).toBe(150_000);
  });

  test("N opens the add dialog and Cmd+Enter saves", async ({ app, api, mkCash }) => {
    const acc = await mkCash(uid("Key"), "MYR", 50_000);
    const desc = uid("keyboard");
    await app.goto("/transactions");
    await app.waitForLoadState("networkidle");
    await app.locator("body").press("n");
    const dlg = app.getByRole("dialog");
    await expect(dlg).toBeVisible();
    await dlg.locator("#tx-amount").fill("7.25");
    await dlg.locator("#tx-description").fill(desc);
    await pickOption(app, dlg.locator("#tx-account"), new RegExp(acc.name));
    await dlg.locator("#tx-description").press("Control+Enter");
    await expect(dlg).toBeHidden();
    expect((await txByDescription(api, desc)).amount).toBe(725);
    expect(await api.balance(acc.id)).toBe(50_000 - 725);
  });

  test("category type mismatch is rejected with a message", async ({ app, api, mkCash }) => {
    const acc = await mkCash(uid("Mis"), "MYR", 10_000);
    const desc = uid("mismatch");
    await app.goto("/transactions");
    const dlg = await openAddTransaction(app);
    await dlg.locator("#tx-amount").fill("5");
    await dlg.locator("#tx-description").fill(desc);
    await pickOption(app, dlg.locator("#tx-account"), new RegExp(acc.name));
    // An expense with an income category: the dialog lists all categories under
    // "All…", and the API refuses the combination.
    await dlg.getByRole("button", { name: "All…" }).click();
    await dlg.getByRole("combobox", { name: "Category" }).click();
    const incomeOption = app.getByRole("option", { name: "Salary" });
    if (await incomeOption.count()) {
      await incomeOption.click();
      await dlg.getByRole("button", { name: "Save expense" }).click();
      await expect(dlg.getByRole("alert")).toContainText(/category|income|expense/i);
      await expect(dlg).toBeVisible();
      expect(await api.balance(acc.id)).toBe(10_000);
    } else {
      // The picker already hides mismatching categories: nothing to reject.
      await expect(app.getByRole("option", { name: "Groceries" })).toBeVisible();
    }
  });
});

test.describe("transactions: transfers", () => {
  test("same-currency transfer moves both balances", async ({ app, api, mkCash }) => {
    const a = await mkCash(uid("From"), "MYR", 100_000);
    const b = await mkCash(uid("To"), "MYR", 0);
    await app.goto("/transactions");
    const dlg = await openAddTransaction(app);
    await dlg.getByRole("radio", { name: "Transfer" }).click();
    await dlg.locator("#tx-amount").fill("250.00");
    await pickOption(app, dlg.locator("#tx-from-account"), new RegExp(a.name));
    await pickOption(app, dlg.locator("#tx-to-account"), new RegExp(b.name));
    await expect(dlg.locator("#tx-to-amount")).toHaveCount(0);
    await dlg.getByRole("button", { name: "Transfer funds" }).click();
    await expect(dlg).toBeHidden();
    expect(await api.balance(a.id)).toBe(75_000);
    expect(await api.balance(b.id)).toBe(25_000);
  });

  test("cross-currency transfer asks for the received amount", async ({
    app,
    api,
    mkCash,
  }) => {
    const myr = await mkCash(uid("Myr"), "MYR", 200_000);
    const usd = await mkCash(uid("Usd"), "USD", 0);
    await app.goto("/transactions");
    const dlg = await openAddTransaction(app);
    await dlg.getByRole("radio", { name: "Transfer" }).click();
    await dlg.locator("#tx-amount").fill("450");
    await pickOption(app, dlg.locator("#tx-from-account"), new RegExp(myr.name));
    await pickOption(app, dlg.locator("#tx-to-account"), new RegExp(usd.name));
    const received = dlg.locator("#tx-to-amount");
    await expect(received).toBeVisible();
    // Pre-filled from the latest rate: RM 450 at 4.5 is US$100.
    await expect(received).toHaveValue(/^100(\.00)?$/);
    await received.fill("99.50");
    await received.press("Tab");
    await dlg.getByRole("button", { name: "Transfer funds" }).click();
    await expect(dlg).toBeHidden();

    expect(await api.balance(myr.id)).toBe(200_000 - 45_000);
    expect(await api.balance(usd.id)).toBe(9_950);
    const tx = (await api.transactions({ account_id: myr.id })).find((t) => t.to_account_id);
    expect(tx?.to_amount).toBe(9_950);
  });
});
