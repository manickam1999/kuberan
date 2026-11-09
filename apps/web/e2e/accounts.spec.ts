import { test, expect, uid } from "./lib/fixtures";
import { isoDay, pickOption } from "./lib/ui";

test.describe("accounts", () => {
  test("create a USD cash account with an opening balance", async ({ app, api }) => {
    const name = uid("Cash");
    await app.goto("/accounts");
    await app.getByRole("button", { name: "New account" }).click();
    await app.getByRole("button", { name: /^Cash/ }).click();
    const dlg = app.getByRole("dialog");
    await dlg.locator("#cash-name").fill(name);
    await pickOption(app, dlg.locator("#cash-currency"), /USD/);
    await dlg.locator("#cash-balance").fill("1234.50");
    await dlg.getByRole("button", { name: "Create Account" }).click();
    await expect(dlg).toBeHidden();

    const acc = (await api.accounts()).find((a) => a.name === name);
    expect(acc).toBeTruthy();
    expect(acc!.currency).toBe("USD");
    expect(acc!.type).toBe("cash");
    expect(acc!.balance).toBe(123_450);
    // Listed with its native amount and the converted base amount (4.5 MYR/USD).
    const row = app.getByRole("link", { name: new RegExp(name) });
    await expect(row).toContainText("US$1,234.50");
    await expect(row).toContainText("RM 5,555.25");
    expect(acc!.balance_base).toBe(555_525);
  });

  test("credit card with a balance owed and a due day", async ({ app, api }) => {
    const name = uid("Card");
    await app.goto("/accounts");
    await app.getByRole("button", { name: "New account" }).click();
    await app.getByRole("button", { name: /^Credit card/ }).click();
    const dlg = app.getByRole("dialog");
    await dlg.locator("#cc-name").fill(name);
    await dlg.locator("#cc-credit-limit").fill("5000");
    await dlg.locator("#cc-current-balance").fill("1200.40");
    await dlg.locator("#cc-interest-rate").fill("15.5");
    await pickOption(app, dlg.locator("#cc-due-day"), /^15\b|15th|^15/);
    await dlg.getByRole("button", { name: "Create Account" }).click();
    await expect(dlg).toBeHidden();

    const acc = (await api.accounts()).find((a) => a.name === name)!;
    expect(acc.type).toBe("credit_card");
    expect(acc.balance).toBe(120_040);
    expect(acc.credit_limit).toBe(500_000);
    expect(acc.interest_rate).toBe(15.5);
    expect(acc.due_day).toBe(15);

    await app.getByRole("link", { name: new RegExp(name) }).click();
    await expect(app.getByRole("heading", { name })).toBeVisible();
    await expect(app.getByText(/RM\s*1,200\.40/).first()).toBeVisible();
    // Liabilities count against net worth on the list.
    await app.goto("/accounts");
    await expect(app.getByRole("link", { name: new RegExp(name) })).toContainText("1,200.40");
  });

  test("Pay card opens a transfer into the card", async ({ app, api, mkCash }) => {
    const cashName = uid("PaySrc");
    const cash = await mkCash(cashName, "MYR", 300_000);
    await api.patch("/api/v1/profile", { default_account_id: cash.id });
    const card = (
      await api.post("/api/v1/accounts/credit-card", {
        name: uid("PayCard"),
        currency: "MYR",
        credit_limit: 1_000_000,
        current_balance: 80_000,
      })
    ).account;

    await app.goto(`/accounts/${card.id}`);
    await app.getByRole("button", { name: "Pay card" }).click();
    const dlg = app.getByRole("dialog");
    await expect(dlg.getByRole("heading", { name: "Transfer funds" })).toBeVisible();
    await expect(dlg.locator("#tx-to-account")).toContainText(card.name);
    await expect(dlg.locator("#tx-from-account")).toContainText(cashName);
    await dlg.locator("#tx-amount").fill("500");
    await dlg.getByRole("button", { name: "Transfer funds" }).click();
    await expect(dlg).toBeHidden();

    expect(await api.balance(cash.id)).toBe(300_000 - 50_000);
    // Paying a card lowers what is owed.
    expect(await api.balance(card.id)).toBe(80_000 - 50_000);
    await expect(app.getByText(/RM\s*300\.00/).first()).toBeVisible();
  });

  test("loan account shows what is owed", async ({ app, api }) => {
    const name = uid("Loan");
    await app.goto("/accounts");
    await app.getByRole("button", { name: "New account" }).click();
    await app.getByRole("button", { name: /^Loan/ }).click();
    const dlg = app.getByRole("dialog");
    await dlg.locator("#loan-name").fill(name);
    await dlg.locator("#loan-owed").fill("25000");
    await dlg.locator("#loan-interest-rate").fill("3.5");
    await dlg.locator("#loan-payment").fill("750");
    await dlg.getByRole("button", { name: "Create Loan" }).click();
    await expect(dlg).toBeHidden();

    const acc = (await api.accounts()).find((a) => a.name === name)!;
    expect(acc.type).toBe("debt");
    expect(acc.balance).toBe(2_500_000);
    expect(acc.monthly_payment).toBe(75_000);
    await app.getByRole("link", { name: new RegExp(name) }).click();
    await expect(app.getByText(/RM\s*25,000\.00/).first()).toBeVisible();
  });

  test("fixed deposit is funded from a bank account", async ({ app, api, mkCash }) => {
    const src = await mkCash(uid("FDsrc"), "MYR", 1_000_000);
    const name = uid("FD");
    const maturity = new Date(Date.now() + 365 * 86_400_000).toISOString().slice(0, 10);
    await app.goto("/accounts");
    await app.getByRole("button", { name: "New account" }).click();
    await app.getByRole("button", { name: /^Fixed deposit/ }).click();
    const dlg = app.getByRole("dialog");
    await dlg.locator("#fd-name").fill(name);
    await pickOption(app, dlg.locator("#fd-source-account"), new RegExp(src.name));
    await dlg.locator("#fd-principal").fill("4000");
    await dlg.locator("#fd-interest-rate").fill("3.5");
    await dlg.locator("#fd-start-date").fill(isoDay(0));
    await dlg.locator("#fd-maturity-date").fill(maturity);
    await dlg.getByRole("button", { name: /Create Account/ }).click();
    await expect(dlg).toBeHidden();

    const fd = (await api.accounts()).find((a) => a.name === name)!;
    expect(fd.type).toBe("fixed_deposit");
    expect(fd.balance).toBe(400_000);
    expect(await api.balance(src.id)).toBe(600_000);
    await app.getByRole("link", { name: new RegExp(name) }).click();
    await expect(app.getByRole("heading", { name })).toBeVisible();
    await expect(app.getByText(/RM\s*4,000\.00/).first()).toBeVisible();
  });

  test("account page balance matches the API after activity", async ({
    app,
    api,
    mkCash,
    state,
  }) => {
    const acc = await mkCash(uid("Hist"), "USD", 100_000);
    await api.post("/api/v1/transactions", {
      account_id: acc.id,
      type: "expense",
      amount: 2_550,
      description: "hist spend",
      category_id: state.categories["Dining"],
      date: isoDay(1),
    });
    await app.goto(`/accounts/${acc.id}`);
    await expect(app.getByText(/US\$\s*974\.50/).first()).toBeVisible();
    await expect(app.getByText("hist spend").locator("visible=true").first()).toBeVisible();
    expect(await api.balance(acc.id)).toBe(97_450);
  });

  test("inactive account is hidden from pickers and can be reactivated", async ({
    app,
    api,
    mkCash,
  }) => {
    const acc = await mkCash(uid("Idle"), "MYR", 1_000);
    await app.goto(`/accounts/${acc.id}`);
    await app.getByRole("button", { name: "Edit" }).click();
    const dlg = app.getByRole("dialog");
    await dlg.getByLabel("Active").uncheck();
    await dlg.getByRole("button", { name: /save/i }).click();
    await expect(dlg).toBeHidden();
    expect((await api.account(acc.id)).is_active).toBe(false);

    // Not offered as a transfer source.
    await app.goto("/transactions");
    await app.getByRole("button", { name: /^New\b/ }).click();
    const tx = app.getByRole("dialog");
    await tx.getByRole("radio", { name: "Transfer" }).click();
    await tx.locator("#tx-from-account").click();
    await expect(app.getByRole("option", { name: new RegExp(acc.name) })).toHaveCount(0);
    await app.keyboard.press("Escape");
    await app.keyboard.press("Escape");

    await app.goto(`/accounts/${acc.id}`);
    await app.getByRole("button", { name: "Edit" }).click();
    await app.getByRole("dialog").getByLabel("Active").check();
    await app.getByRole("dialog").getByRole("button", { name: /save/i }).click();
    await expect(app.getByRole("dialog")).toBeHidden();
    expect((await api.account(acc.id)).is_active).toBe(true);
  });
});
