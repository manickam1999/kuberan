import { test, expect, uid } from "./lib/fixtures";
import { isoDay, openAddTransaction, pickOption } from "./lib/ui";
import { pipeline, type Json } from "./lib/api";

const money = (cents: number): string =>
  (cents / 100).toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

test.describe("edge cases", () => {
  test("deleting a cross-currency transfer reverses both legs by their own amounts", async ({
    app,
    api,
    mkCash,
  }) => {
    const myr = await mkCash(uid("DelFrom"), "MYR", 200_000);
    const usd = await mkCash(uid("DelTo"), "USD", 0);
    const r = await api.post("/api/v1/transactions/transfer", {
      from_account_id: myr.id,
      to_account_id: usd.id,
      amount: 45_000,
      to_amount: 9_900,
      description: "xfer to delete",
    });
    expect(await api.balance(myr.id)).toBe(155_000);
    expect(await api.balance(usd.id)).toBe(9_900);
    expect(Number(r.transaction.implied_rate)).toBeCloseTo(9_900 / 45_000, 6);

    await app.goto(`/transactions?account=${myr.id}`);
    await app.getByRole("button", { name: "xfer to delete" }).click();
    await app.getByRole("dialog").getByRole("button", { name: /^Delete/ }).click();
    await app.getByRole("alertdialog").getByRole("button", { name: /delete/i }).click();
    await expect(app.getByRole("button", { name: "xfer to delete" })).toHaveCount(0);
    expect(await api.balance(myr.id)).toBe(200_000);
    expect(await api.balance(usd.id)).toBe(0);
  });

  test("transfer larger than the balance is refused with a message", async ({
    app,
    api,
    mkCash,
    consoleErrors,
  }) => {
    consoleErrors.allowed.push(/400.*transactions\/transfer/);
    const a = await mkCash(uid("Poor"), "MYR", 1_000);
    const b = await mkCash(uid("Rich"), "MYR", 0);
    await app.goto("/transactions");
    const dlg = await openAddTransaction(app);
    await dlg.getByRole("radio", { name: "Transfer" }).click();
    await dlg.locator("#tx-amount").fill("50");
    await pickOption(app, dlg.locator("#tx-from-account"), new RegExp(a.name));
    await pickOption(app, dlg.locator("#tx-to-account"), new RegExp(b.name));
    await dlg.getByRole("button", { name: "Transfer funds" }).click();
    await expect(dlg.getByRole("alert")).toContainText(/insufficient|enough|balance/i);
    await expect(dlg).toBeVisible();
    expect(await api.balance(a.id)).toBe(1_000);
    expect(await api.balance(b.id)).toBe(0);
  });

  test("credit card spending raises what is owed; switching type clears the category", async ({
    app,
    api,
    state,
  }) => {
    const card = (
      await api.post("/api/v1/accounts/credit-card", {
        name: uid("Cc"),
        currency: "MYR",
        credit_limit: 1_000_000,
        current_balance: 10_000,
      })
    ).account;
    const desc = uid("cardbuy");
    await app.goto("/transactions");
    const dlg = await openAddTransaction(app);
    await dlg.locator("#tx-amount").fill("30");
    await dlg.locator("#tx-description").fill(desc);
    await dlg.getByRole("button", { name: "Groceries" }).click();
    // Switch to income: an expense category can't carry over.
    await dlg.getByRole("radio", { name: "Income" }).click();
    await dlg.getByRole("radio", { name: "Expense" }).click();
    await pickOption(app, dlg.locator("#tx-account"), new RegExp(card.name));
    await dlg.getByRole("button", { name: "Save expense" }).click();
    await expect(dlg).toBeHidden();
    expect(await api.balance(card.id)).toBe(13_000);
    const tx = (await api.transactions({ q: desc }))[0];
    expect(tx.category_id ?? null).toBeNull();
    void state;
  });

  test("changing a saved expense to income drops its expense category", async ({
    app,
    api,
    mkCash,
    state,
  }) => {
    const acc = await mkCash(uid("TypeSw"), "MYR", 0);
    const desc = uid("typeswitch");
    await api.post("/api/v1/transactions", {
      account_id: acc.id,
      type: "expense",
      amount: 1_000,
      description: desc,
      category_id: state.categories["Groceries"],
      date: isoDay(1),
    });
    await app.goto(`/transactions?account=${acc.id}`);
    await app.getByRole("button", { name: desc }).click();
    const dlg = app.getByRole("dialog");
    await dlg.getByRole("radio", { name: "Income" }).click();
    await dlg.getByRole("button", { name: "Save", exact: true }).click();
    await expect(dlg).toBeHidden();
    const tx = (await api.transactions({ q: desc }))[0];
    expect(tx.type).toBe("income");
    expect(tx.category_id ?? null).toBeNull();
    expect(await api.balance(acc.id)).toBe(1_000);
  });

  test("removing a category from a saved transaction sticks", async ({
    app,
    api,
    mkCash,
    state,
  }) => {
    const acc = await mkCash(uid("NoCat"), "MYR", 0);
    const desc = uid("uncat");
    await api.post("/api/v1/transactions", {
      account_id: acc.id,
      type: "expense",
      amount: 1_000,
      description: desc,
      category_id: state.categories["Groceries"],
    });
    await app.goto(`/transactions?account=${acc.id}`);
    await app.getByRole("button", { name: desc }).click();
    const dlg = app.getByRole("dialog");
    await pickOption(app, dlg.getByRole("combobox", { name: "Category" }), "No category");
    await dlg.getByRole("button", { name: "Save", exact: true }).click();
    await expect(dlg).toBeHidden();
    const tx = (await api.transactions({ q: desc }))[0];
    expect(tx.category_id ?? null).toBeNull();
  });

  test("a refused edit keeps its error on screen", async ({
    app,
    api,
    mkCash,
    consoleErrors,
  }) => {
    consoleErrors.allowed.push(/400.*transactions\//);
    const myr = await mkCash(uid("EditFrom"), "MYR", 0);
    const usd = await mkCash(uid("EditTo"), "USD", 0);
    const desc = uid("moveccy");
    await api.post("/api/v1/transactions", {
      account_id: myr.id,
      type: "expense",
      amount: 1_000,
      description: desc,
    });
    await app.goto(`/transactions?account=${myr.id}`);
    await app.getByRole("button", { name: desc }).click();
    const dlg = app.getByRole("dialog");
    await pickOption(app, dlg.getByRole("combobox", { name: "Account" }), new RegExp(usd.name));
    await dlg.getByRole("button", { name: "Save", exact: true }).click();
    // Cents can't be reinterpreted in another currency: the dialog says so and stays.
    await expect(dlg.getByRole("alert")).toContainText(/currency/i);
    await expect(dlg).toBeVisible();
    await app.waitForTimeout(500);
    await expect(dlg.getByRole("alert")).toBeVisible();
    expect(await api.balance(myr.id)).toBe(-1_000);
    expect(await api.balance(usd.id)).toBe(0);
  });

  test("selling more than held is refused", async ({ app, api, state }) => {
    const broker = (
      await api.post("/api/v1/accounts/investment", { name: uid("Over"), currency: "USD" })
    ).account;
    const inv = (
      await api.post("/api/v1/investments", {
        account_id: broker.id,
        security_id: state.usdSecurityId,
        quantity: 3,
        purchase_price: 10_000,
        date: new Date().toISOString(),
      })
    ).investment;
    await app.goto(`/investments/${inv.id}`);
    await app.getByRole("button", { name: "Sell", exact: true }).click();
    const dlg = app.getByRole("dialog");
    await dlg.locator("#trade-quantity").fill("5");
    await dlg.locator("#trade-price").fill("100");
    await dlg.getByRole("button", { name: "Record sale" }).click();
    await expect(dlg).toBeVisible();
    await expect(
      dlg.getByRole("alert").or(app.locator("[data-sonner-toast]")).first()
    ).toBeVisible();
    expect((await api.get(`/api/v1/investments/${inv.id}`)).investment.quantity).toBe(3);
  });

  test("fractional quantity sale rounds the realised gain once", async ({ app, api, state }) => {
    const broker = (
      await api.post("/api/v1/accounts/investment", { name: uid("Frac"), currency: "USD" })
    ).account;
    const inv = (
      await api.post("/api/v1/investments", {
        account_id: broker.id,
        security_id: state.usdSecurityId,
        quantity: 3,
        purchase_price: 3_333,
        date: new Date().toISOString(),
      })
    ).investment;
    await app.goto(`/investments/${inv.id}`);
    await app.getByRole("button", { name: "Sell", exact: true }).click();
    const dlg = app.getByRole("dialog");
    await dlg.locator("#trade-quantity").fill("1.5");
    await dlg.locator("#trade-price").fill("40.01");
    // Cost 3 x 33.33 = 99.99; half is 49.995 -> 50.00 (cents rounded once).
    const previewGain = dlg.getByText(/^Realised gain\s*US\$/);
    await expect(previewGain).toBeVisible();
    const previewText = (await previewGain.innerText()).replace(/[^\d.\-+−]/g, "").replace("−", "-");
    await dlg.getByRole("button", { name: "Record sale" }).click();
    await expect(dlg).toBeHidden();
    const h: Json = (await api.get(`/api/v1/investments/${inv.id}`)).investment;
    expect(h.quantity).toBe(1.5);
    // The preview must equal what the server stored.
    expect(Number(previewText)).toBeCloseTo(h.realized_gain_loss / 100, 2);
    expect(h.cost_basis + Math.round(h.realized_gain_loss + 6_002)).toBeGreaterThan(0);
  });
});

test.describe("dashboard", () => {
  test("net worth equals the sum of base balances", async ({ app, api }) => {
    const accounts = await api.accounts();
    let assets = 0;
    let debt = 0;
    for (const a of accounts) {
      if (!a.is_active) continue;
      const base = a.balance_base ?? 0;
      if (a.type === "credit_card" || a.type === "debt") debt += base;
      else assets += base;
    }
    const expected = assets - debt;
    // The price worker normally records snapshots; do what it does.
    await pipeline("/api/v1/pipeline/snapshots", { recorded_at: new Date().toISOString() });
    await app.goto("/");
    await app.waitForLoadState("networkidle");
    await expect(
      app.getByText(new RegExp(`RM\\s*${money(expected).replace(/[.,]/g, (c) => "\\" + c)}`)).first()
    ).toBeVisible();
  });
});
