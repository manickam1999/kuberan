import { test, expect, uid } from "./lib/fixtures";
import { isoDay, pickOption } from "./lib/ui";
import type { Api, Json } from "./lib/api";
import type { E2EState } from "./global-setup";

// Rates seeded by global-setup for "today": USD->MYR 4.5, SGD->MYR 3.35.
const USD_MYR = 4.5;
const SGD_MYR = 3.35;

async function mkBroker(api: Api, ccy = "USD"): Promise<Json> {
  return (await api.post("/api/v1/accounts/investment", { name: uid("Brk"), currency: ccy })).account;
}

async function mkHolding(
  api: Api,
  state: E2EState,
  accountId: string,
  quantity: number,
  priceCents: number,
  security: "usd" | "myr" = "usd"
): Promise<Json> {
  const r = await api.post("/api/v1/investments", {
    account_id: accountId,
    security_id: security === "usd" ? state.usdSecurityId : state.myrSecurityId,
    quantity,
    purchase_price: priceCents,
    date: new Date().toISOString(),
  });
  return r.investment;
}

async function holding(api: Api, id: string): Promise<Json> {
  const r = await api.get(`/api/v1/investments/${id}`);
  return r.investment ?? r;
}

async function trades(api: Api, id: string): Promise<Json[]> {
  const r = await api.get(`/api/v1/investments/${id}/transactions`);
  return r.data ?? r.transactions ?? r;
}

const rm = (s: string) => new RegExp("RM\\s*" + s.replace(/[.+]/g, "\\$&"));

test.describe("investments: holdings and trades", () => {
  test("add a holding from the account page", async ({ app, api, state }) => {
    const broker = await mkBroker(api);
    await app.goto(`/accounts/${broker.id}`);
    await app.getByRole("button", { name: "Add investment" }).first().click();
    const dlg = app.getByRole("dialog", { name: "Add Investment" });
    await dlg.getByRole("combobox").click();
    await app.keyboard.type(state.usdSymbol);
    await app.getByRole("option", { name: new RegExp(state.usdSymbol) }).click();
    await dlg.locator("#inv-quantity").fill("10");
    await dlg.locator("#inv-price").fill("150");
    await dlg.locator("#inv-fee").fill("5");
    await dlg.getByRole("button", { name: "Add Investment" }).click();
    await expect(dlg).toBeHidden();

    const list = await api.get(`/api/v1/accounts/${broker.id}/investments`);
    const inv = (list.data ?? list.investments)[0];
    expect(inv.quantity).toBe(10);
    // 10 x $150 plus the $5 fee, in the security's currency.
    expect(inv.cost_basis).toBe(150_500);
    // Frozen at the market rate of the trade day (4.5).
    expect(inv.cost_basis_base).toBe(Math.round(150_500 * USD_MYR));
    await expect(app.getByText(state.usdSymbol).first()).toBeVisible();
  });

  test("buy funded from a MYR account asks for the charged amount", async ({
    app,
    api,
    state,
    mkCash,
  }) => {
    const broker = await mkBroker(api);
    const inv0 = await mkHolding(api, state, broker.id, 10, 15_000);
    const myr = await mkCash(uid("Fund"), "MYR", 5_000_000);

    // Without the other-currency amount the API refuses and suggests one.
    await expect(
      api.post(`/api/v1/investments/${inv0.id}/buy`, {
        date: new Date().toISOString(),
        quantity: 2,
        price_per_unit: 16_000,
        funding_account_id: myr.id,
      })
    ).rejects.toMatchObject({
      status: 400,
      body: { error: { code: "FX_AMOUNT_REQUIRED", details: { suggested_amount: 144_000 } } },
    });

    await app.goto(`/investments/${inv0.id}`);
    await app.getByRole("button", { name: "Buy", exact: true }).click();
    const dlg = app.getByRole("dialog");
    await dlg.locator("#trade-quantity").fill("2");
    await dlg.locator("#trade-price").fill("160");
    await pickOption(app, dlg.locator("#trade-account"), new RegExp(myr.name));
    const charged = dlg.locator("#trade-account-amount");
    // 2 x $160 = $320 at 4.5 = RM 1,440.00, suggested from the latest rate.
    await expect(charged).toHaveValue("1440.00");
    await charged.fill("1442.80");
    await charged.press("Tab");
    await dlg.getByRole("button", { name: "Record buy" }).click();
    await expect(dlg).toBeHidden();

    expect(await api.balance(myr.id)).toBe(5_000_000 - 144_280);
    const h = await holding(api, inv0.id);
    expect(h.quantity).toBe(12);
    expect(h.cost_basis).toBe(150_000 + 32_000);
    // The new lot is frozen at the rate the account actually moved at.
    expect(h.cost_basis_base).toBe(675_000 + 144_280);
    const last = (await trades(api, inv0.id)).find((t) => t.type === "buy" && t.quantity === 2);
    expect(last?.base_amount).toBe(144_280);
    expect(Number(last?.fx_rate_to_base)).toBeCloseTo(144_280 / 32_000, 4);
  });

  test("buy from a USD account needs no conversion", async ({ app, api, state, mkCash }) => {
    const broker = await mkBroker(api);
    const inv0 = await mkHolding(api, state, broker.id, 10, 15_000);
    const usd = await mkCash(uid("UsdFund"), "USD", 100_000);
    await app.goto(`/investments/${inv0.id}`);
    await app.getByRole("button", { name: "Buy", exact: true }).click();
    const dlg = app.getByRole("dialog");
    await dlg.locator("#trade-quantity").fill("3");
    await dlg.locator("#trade-price").fill("100");
    await dlg.locator("#trade-fee").fill("2");
    await pickOption(app, dlg.locator("#trade-account"), new RegExp(usd.name));
    await expect(dlg.locator("#trade-account-amount")).toHaveCount(0);
    await expect(dlg.getByText(/Total cost/)).toBeVisible();
    await dlg.getByRole("button", { name: "Record buy" }).click();
    await expect(dlg).toBeHidden();

    // $300 + $2 fee leaves the USD account.
    expect(await api.balance(usd.id)).toBe(100_000 - 30_200);
    const h = await holding(api, inv0.id);
    expect(h.quantity).toBe(13);
    expect(h.cost_basis).toBe(150_000 + 30_200);
  });

  test("sell part shows the realised gain the API then records", async ({
    app,
    api,
    state,
    mkCash,
  }) => {
    const broker = await mkBroker(api);
    const inv0 = await mkHolding(api, state, broker.id, 10, 15_000);
    const usd = await mkCash(uid("SellTo"), "USD", 0);
    await app.goto(`/investments/${inv0.id}`);
    await app.getByRole("button", { name: "Sell", exact: true }).click();
    const dlg = app.getByRole("dialog");
    await dlg.locator("#trade-quantity").fill("4");
    await dlg.locator("#trade-price").fill("200");
    await pickOption(app, dlg.locator("#trade-account"), new RegExp(usd.name));
    // 4 x $200 = $800 proceeds; 4/10 of the $1,500 cost is $600; gain $200.
    await expect(dlg.getByText(/^Proceeds\s*US\$\s*800\.00$/)).toBeVisible();
    await expect(dlg.getByText(/^Cost of shares sold\s*US\$\s*600\.00$/)).toBeVisible();
    await expect(dlg.getByText(/^Realised gain\s*US\$\s*\+?200\.00$/)).toBeVisible();
    await dlg.getByRole("button", { name: "Record sale" }).click();
    await expect(dlg).toBeHidden();

    const h = await holding(api, inv0.id);
    expect(h.quantity).toBe(6);
    expect(h.realized_gain_loss).toBe(20_000);
    expect(h.cost_basis).toBe(90_000);
    // Base: proceeds 800 * 4.5 = 3,600 less 40% of the 6,750 base cost (2,700).
    expect(h.realized_gain_loss_base).toBe(90_000);
    expect(h.cost_basis_base).toBe(405_000);
    expect(await api.balance(usd.id)).toBe(80_000);
  });

  test("dividend with withholding into an account in another currency", async ({
    app,
    api,
    state,
    mkCash,
  }) => {
    const broker = await mkBroker(api);
    const inv0 = await mkHolding(api, state, broker.id, 10, 15_000);
    const sgd = await mkCash(uid("DivSgd"), "SGD", 0);
    await app.goto(`/investments/${inv0.id}`);
    await app.getByRole("button", { name: "Dividend", exact: true }).click();
    const dlg = app.getByRole("dialog");
    await dlg.locator("#trade-amount").fill("50");
    await dlg.locator("#trade-fee").fill("7.50");
    await pickOption(app, dlg.locator("#trade-account"), new RegExp(sgd.name));
    // Net $42.50 deposited. At 4.5 / 3.35 that is S$57.09, suggested by the app.
    await expect(dlg.getByText(/^Net dividend\s*US\$\s*42\.50$/)).toBeVisible();
    const dep = dlg.locator("#trade-account-amount");
    await expect(dep).toHaveValue("57.09");
    await dlg.getByRole("button", { name: /Record dividend/ }).click();
    await expect(dlg).toBeHidden();

    expect(await api.balance(sgd.id)).toBe(5_709);
    const div = (await trades(api, inv0.id)).find((t) => t.type === "dividend");
    expect(div).toBeTruthy();
    expect(div!.total_amount).toBe(5_000);
    expect(div!.fee).toBe(750);
  });

  test("split doubles the quantity and keeps the cost", async ({ app, api, state }) => {
    const broker = await mkBroker(api);
    const inv0 = await mkHolding(api, state, broker.id, 10, 15_000);
    await app.goto(`/investments/${inv0.id}`);
    await app.getByRole("button", { name: "Split", exact: true }).click();
    const dlg = app.getByRole("dialog");
    await dlg.locator("#trade-ratio").fill("2");
    await dlg.getByRole("button", { name: /Record split/ }).click();
    await expect(dlg).toBeHidden();
    const h = await holding(api, inv0.id);
    expect(h.quantity).toBe(20);
    expect(h.cost_basis).toBe(150_000);
    expect(h.cost_basis_base).toBe(675_000);
    await expect(app.getByText(/avg cost US\$\s*75\.00/)).toBeVisible();
  });
});

test.describe("investments: portfolio", () => {
  test("totals in MYR equal the sum of the holdings' base figures", async ({
    app,
    api,
    state,
  }) => {
    const b1 = await mkBroker(api);
    const b2 = await mkBroker(api);
    await mkHolding(api, state, b1.id, 7, 12_345);
    await mkHolding(api, state, b2.id, 3, 999, "myr");

    const all = await api.get("/api/v1/investments", { page_size: 100 });
    const holdings: Json[] = all.data;
    const sumCostBase = holdings.reduce((a, h) => a + h.cost_basis_base, 0);
    const sumValue = holdings.reduce(
      (a, h) => a + Math.round(h.quantity * h.current_price),
      0
    );
    const p = (await api.get("/api/v1/investments/portfolio")).portfolio;
    expect(p.currency).toBe("MYR");
    expect(p.total_cost_basis).toBe(sumCostBase);
    // Value per line is rounded once; allow 1 cent per holding for that.
    expect(Math.abs(p.total_value - sumValue)).toBeLessThanOrEqual(holdings.length);
    expect(p.total_gain_loss).toBe(p.total_value - p.total_cost_basis);
    expect(p.total_price_return + p.total_fx_return).toBeCloseTo(p.total_gain_loss, -1);

    await app.goto("/investments");
    await expect(app.getByText(rmRegex(p.total_value)).first()).toBeVisible();
    await expect(app.getByText(state.usdSymbol).first()).toBeVisible();
  });

  test("holding and securities pages", async ({ app, api, state }) => {
    const broker = await mkBroker(api);
    const inv0 = await mkHolding(api, state, broker.id, 4, 15_000);
    await app.goto(`/investments/${inv0.id}`);
    await expect(app.getByRole("heading", { name: "E2E Dollar Corp" })).toBeVisible();
    await expect(app.getByText(/US\$\s*800\.00/).first()).toBeVisible(); // 4 x $200
    await expect(app.getByText(/RM\s*3,600\.00/).first()).toBeVisible();
    await expect(app.getByRole("heading", { name: "History" })).toBeVisible();

    await app.goto("/securities");
    await app.getByRole("searchbox", { name: "Search securities" }).fill(state.usdSymbol);
    await app.getByRole("link", { name: new RegExp(state.usdSymbol) }).click();
    await expect(app).toHaveURL(/\/securities\//);
    await expect(app.getByText(/US\$\s*200\.00/).first()).toBeVisible();
    void isoDay;
  });
});

function rmRegex(cents: number): RegExp {
  const neg = cents < 0 ? "[-−]" : "";
  const abs = Math.abs(cents);
  const whole = Math.floor(abs / 100).toLocaleString("en-US");
  const frac = String(abs % 100).padStart(2, "0");
  return new RegExp(`RM\\s*${neg}${whole.replace(/,/g, ",")}\\.${frac}`);
}
void rm;
void SGD_MYR;
