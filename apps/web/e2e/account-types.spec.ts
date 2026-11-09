import { test, expect, uid } from "./lib/fixtures";
import { isoDay } from "./lib/ui";

// Investment balances come from holdings and a fixed deposit's principal is
// locked: plain income, expenses and transfers must not target them.
test.describe("accounts that don't take plain activity", () => {
  test("pickers don't offer them and the API refuses", async ({ app, api, mkCash }) => {
    const cash = await mkCash(uid("Cash"), "MYR", 500_000);
    const broker = (
      await api.post("/api/v1/accounts/investment", { name: uid("NoTx"), currency: "MYR" })
    ).account;
    const fd = (
      await api.post("/api/v1/accounts/fixed-deposit", {
        name: uid("NoTxFD"),
        currency: "MYR",
        source_account_id: cash.id,
        principal: 100_000,
        interest_rate: 3,
        start_date: isoDay(0),
        maturity_date: isoDay(-365),
        payout_type: "cumulative",
        compounding: "annual",
      })
    ).account;

    await app.goto("/transactions");
    await app.getByRole("button", { name: /^New\b/ }).click();
    const dlg = app.getByRole("dialog");

    // Expense / income account picker.
    await dlg.locator("#tx-account").click();
    await expect(app.getByRole("option", { name: new RegExp(cash.name) })).toBeVisible();
    await expect(app.getByRole("option", { name: new RegExp(broker.name) })).toHaveCount(0);
    await expect(app.getByRole("option", { name: new RegExp(fd.name) })).toHaveCount(0);
    await app.keyboard.press("Escape");

    // Transfer from / to pickers.
    await dlg.getByRole("radio", { name: "Transfer" }).click();
    for (const picker of ["#tx-from-account", "#tx-to-account"]) {
      await dlg.locator(picker).click();
      await expect(app.getByRole("option", { name: new RegExp(broker.name) })).toHaveCount(0);
      await expect(app.getByRole("option", { name: new RegExp(fd.name) })).toHaveCount(0);
      await app.keyboard.press("Escape");
    }

    // The API is the authority.
    const refused = { status: 400, body: { error: { code: "ACCOUNT_TYPE_NOT_ALLOWED" } } };
    for (const target of [broker.id, fd.id]) {
      await expect(
        api.post("/api/v1/transactions", {
          account_id: target,
          type: "income",
          amount: 1_000,
          description: "phantom",
        })
      ).rejects.toMatchObject(refused);
      await expect(
        api.post("/api/v1/transactions/transfer", {
          from_account_id: cash.id,
          to_account_id: target,
          amount: 1_000,
        })
      ).rejects.toMatchObject(refused);
    }
    expect(await api.balance(fd.id)).toBe(100_000);
    expect(await api.balance(broker.id)).toBe(0);
    expect(await api.balance(cash.id)).toBe(400_000);
  });
});
