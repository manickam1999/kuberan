import { test, expect, uid } from "./lib/fixtures";
import { isoDay } from "./lib/ui";
import type { Json } from "./lib/api";

const money = (cents: number): string =>
  (cents / 100).toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

test.describe("investments overview", () => {
  test("the portfolio page matches the API's totals", async ({ app, api, state }) => {
    const broker = (
      await api.post("/api/v1/accounts/investment", { name: uid("Ovw"), currency: "USD" })
    ).account;
    await api.post("/api/v1/investments", {
      account_id: broker.id,
      security_id: state.usdSecurityId,
      quantity: 2.5,
      purchase_price: 12_345,
      date: new Date().toISOString(),
    });
    const p = (await api.get("/api/v1/investments/portfolio")).portfolio;
    await app.goto("/investments");
    await app.waitForLoadState("networkidle");
    const re = (cents: number) =>
      new RegExp(`RM\\s*[+\\-\u2212]?${money(Math.abs(cents)).replace(/[.,]/g, (c) => "\\" + c)}`);
    await expect(app.getByText(re(p.total_value)).first()).toBeVisible();
    await expect(app.getByText(re(p.total_gain_loss)).first()).toBeVisible();
    await expect(app.getByText(`${p.gain_loss_pct.toFixed(1)}%`, { exact: false }).first()).toBeVisible();
    // The allocation donut says "class" for one asset class, not "1 classes".
    const classes = Object.keys(p.holdings_by_type).length;
    if (classes === 1) await expect(app.getByText("classes", { exact: true })).toHaveCount(0);
    // Price return and FX return add up to the total gain.
    expect(Math.abs(p.total_price_return + p.total_fx_return - p.total_gain_loss)).toBeLessThanOrEqual(2);
  });
});

test.describe("fixed deposit and loan", () => {
  test("closing a deposit early pays the destination account", async ({
    app,
    api,
    mkCash,
    consoleErrors,
  }) => {
    void consoleErrors;
    const src = await mkCash(uid("FdSrc"), "MYR", 1_000_000);
    const dest = await mkCash(uid("FdDest"), "MYR", 0);
    const fd = (
      await api.post("/api/v1/accounts/fixed-deposit", {
        name: uid("FdClose"),
        currency: "MYR",
        source_account_id: src.id,
        principal: 500_000,
        interest_rate: 3.6,
        start_date: isoDay(30),
        maturity_date: isoDay(-335),
        payout_type: "cumulative",
        compounding: "monthly",
      })
    ).account;
    expect(await api.balance(src.id)).toBe(500_000);

    await app.goto(`/accounts/${fd.id}`);
    await app.getByRole("button", { name: /close early|record payout/i }).first().click();
    const dlg = app.getByRole("dialog");
    await dlg.locator("#close-fd-destination").click();
    await app.getByRole("option", { name: new RegExp(dest.name) }).click();
    const interestField = dlg.locator("#close-fd-interest");
    const interest = Math.round(Number(await interestField.inputValue()) * 100);
    // ~30 days of 3.6% on RM 5,000 is roughly RM 14.8; never zero, never more than a month's worth.
    expect(interest).toBeGreaterThan(1_000);
    expect(interest).toBeLessThan(2_000);
    await dlg.getByRole("button", { name: /^Close early|Record payout/ }).last().click();
    await expect(dlg).toBeHidden();

    expect(await api.balance(dest.id)).toBe(500_000 + interest);
    const closed = await api.account(fd.id);
    expect(closed.is_active).toBe(false);
    expect(closed.balance).toBe(0);
  });

  test("paying a loan from cash lowers what is owed", async ({ app, api, mkCash }) => {
    const cash = await mkCash(uid("LoanSrc"), "MYR", 500_000);
    const loan = (
      await api.post("/api/v1/accounts/debt", {
        name: uid("LoanPay"),
        currency: "MYR",
        amount_owed: 2_000_000,
        interest_rate: 4,
        monthly_payment: 100_000,
      })
    ).account;
    await app.goto(`/accounts/${loan.id}`);
    await app.getByRole("button", { name: "Make a payment" }).click();
    const dlg = app.getByRole("dialog");
    await expect(dlg.locator("#tx-to-account")).toContainText(loan.name);
    await dlg.locator("#tx-amount").fill("1000");
    // Pick the source explicitly: the saved default may be another account.
    await dlg.locator("#tx-from-account").click();
    await app.getByRole("option", { name: new RegExp(cash.name) }).click();
    await dlg.getByRole("button", { name: "Transfer funds" }).click();
    await expect(dlg).toBeHidden();
    expect(await api.balance(cash.id)).toBe(400_000);
    expect(await api.balance(loan.id)).toBe(1_900_000);
    await expect(app.getByText(/RM\s*19,000\.00/).first()).toBeVisible();
  });
});

test.describe("receipts", () => {
  test("attach a receipt to a transaction (needs the blob store)", async ({
    app,
    api,
    mkCash,
  }) => {
    const acc = await mkCash(uid("Rcpt"), "MYR", 100_000);
    const desc = uid("receipt");
    const tx: Json = (
      await api.post("/api/v1/transactions", {
        account_id: acc.id,
        type: "expense",
        amount: 1_500,
        description: desc,
      })
    ).transaction;

    // A 1x1 PNG.
    const png = Buffer.from(
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
      "base64"
    );
    const probe = await fetch(`${process.env.E2E_API_URL ?? "http://localhost:8195"}/api/v1/transactions/${tx.id}/attachments`, {
      method: "POST",
      headers: { Authorization: `Bearer ${api.token}` },
      body: (() => {
        const f = new FormData();
        f.append("file", new Blob([png], { type: "image/png" }), "probe.png");
        return f;
      })(),
    });
    if (probe.status === 503) {
      // No blob store: a clear, typed refusal, not an internal error.
      expect(((await probe.json()) as Json).error.code).toBe("RECEIPTS_DISABLED");
    }
    test.skip(
      probe.status === 503,
      "receipt storage is not configured on this API (RECEIPTS_DISABLED)"
    );
    expect(probe.status).toBe(201);

    await app.goto(`/transactions?account=${acc.id}`);
    await app.getByRole("button", { name: desc }).click();
    await expect(app.getByRole("dialog").getByText("probe.png")).toBeVisible();
    const list = await api.get(`/api/v1/transactions/${tx.id}/attachments`);
    expect((list.data ?? list.attachments ?? list).length).toBe(1);
  });
});
