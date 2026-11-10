import { test, expect, uid } from "./lib/fixtures";
import { isoDay, pickOption } from "./lib/ui";
import type { Api, Json } from "./lib/api";

/** Rate (decimal string) of a pair on a UTC day: the latest observation on or before it. */
async function dayRate(api: Api, base: string, day: string): Promise<string> {
  if (base === "MYR") return "1";
  const r = await api.get("/api/v1/fx-rates", {
    base,
    quote: "MYR",
    from: `${isoDay(60)}T00:00:00Z`,
    to: `${day}T23:59:59Z`,
  });
  const points: Json[] = r.rates;
  const upTo = points
    .filter((p) => p.recorded_at.slice(0, 10) <= day)
    .sort((a, b) => a.recorded_at.localeCompare(b.recorded_at));
  return String(upTo[upTo.length - 1].rate);
}

/**
 * cents x rate with exact integer math, rounded half to even like the API
 * (internal/fx: money is converted once per group with banker's rounding).
 */
function convert(cents: number, rate: string): number {
  const [whole, frac = ""] = rate.split(".");
  const one = BigInt(1);
  const two = BigInt(2);
  const scale = BigInt("1" + "0".repeat(frac.length));
  const num = BigInt(cents) * (BigInt(whole) * scale + BigInt(frac || "0"));
  let q = num / scale;
  const r2 = (num % scale) * two;
  if (r2 > scale || (r2 === scale && q % two === one)) q += one;
  return Number(q);
}

const money = (cents: number): string =>
  (cents / 100).toLocaleString("en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

test.describe("budgets", () => {
  test("a MYR budget counts USD spending converted per transaction day", async ({
    app,
    api,
    mkCash,
  }) => {
    const cat = (
      await api.post("/api/v1/categories", { name: uid("BudCat"), type: "expense", color: "#a855f7" })
    ).category;
    const myr = await mkCash(uid("BudMyr"), "MYR", 1_000_000);
    const usd = await mkCash(uid("BudUsd"), "USD", 100_000);
    const name = uid("Budget");

    await app.goto("/budgets");
    await app.getByRole("button", { name: "New budget" }).first().click();
    const dlg = app.getByRole("dialog");
    await dlg.locator("#budget-name").fill(name);
    await pickOption(app, dlg.locator("#budget-category"), cat.name);
    await expect(dlg.locator("#budget-currency")).toContainText("MYR");
    await dlg.locator("#budget-amount").fill("500");
    await dlg.getByRole("button", { name: "Create Budget" }).click();
    await expect(dlg).toBeHidden();

    const budget = (await api.get("/api/v1/budgets")).data.find((b: Json) => b.name === name);
    expect(budget.currency).toBe("MYR");
    expect(budget.amount).toBe(50_000);

    // RM 100.00 plus US$20.00 today (4.5) = RM 190.00 spent.
    for (const [acc, amount] of [
      [myr.id, 10_000],
      [usd.id, 2_000],
    ] as const) {
      await api.post("/api/v1/transactions", {
        account_id: acc,
        category_id: cat.id,
        type: "expense",
        amount,
        description: "budget spend",
        date: isoDay(0),
      });
    }
    const progress = (await api.get(`/api/v1/budgets/${budget.id}/progress`)).progress;
    expect(progress.spent).toBe(19_000);
    expect(progress.remaining).toBe(31_000);
    expect(progress.currency).toBe("MYR");
    expect(Math.round(progress.percentage)).toBe(38);

    await app.goto("/budgets");
    const card = app.locator("[class*=rounded-card]").filter({ hasText: name }).first();
    await expect(card).toContainText(/RM\s*190\.00/);
    await expect(card).toContainText(/RM\s*500\.00/);
    await expect(card).toContainText("On track");
    // RM 310.00 left over the 22 days from today to month end.
    await expect(card).toContainText(/RM\s*310\.00 left/);
  });

  test("spending near the limit is flagged Close", async ({ app, api, mkCash }) => {
    const cat = (
      await api.post("/api/v1/categories", { name: uid("PaceCat"), type: "expense", color: "#0ea5e9" })
    ).category;
    const acc = await mkCash(uid("PaceAcc"), "MYR", 1_000_000);
    const name = uid("PaceBud");
    const budget = (
      await api.post("/api/v1/budgets", {
        category_id: cat.id,
        name,
        amount: 10_000,
        period: "monthly",
        currency: "MYR",
      })
    ).budget;
    await api.post("/api/v1/transactions", {
      account_id: acc.id,
      category_id: cat.id,
      type: "expense",
      amount: 9_500,
      description: "pace spend",
      date: isoDay(0),
    });
    const progress = (await api.get(`/api/v1/budgets/${budget.id}/progress`)).progress;
    expect(progress.spent).toBe(9_500);
    await app.goto("/budgets");
    const card = app.locator("[class*=rounded-card]").filter({ hasText: name }).first();
    await expect(card).toContainText("Close");
    await expect(card).toContainText(/RM\s*95\.00/);
  });
});

test.describe("insights", () => {
  test("the month's KPIs match the API and an independent conversion", async ({ app, api }) => {
    const now = new Date();
    const first = `${now.getUTCFullYear()}-${String(now.getUTCMonth() + 1).padStart(2, "0")}-01`;
    const accounts = await api.accounts();
    const ccy = new Map<string, string>(accounts.map((a) => [a.id, a.currency]));

    // Expenses and income this month (transfers don't count), page through all.
    const rows: Json[] = [];
    for (let page = 1; ; page++) {
      const r = await api.get("/api/v1/transactions", {
        page_size: 100,
        page,
        from_date: first,
        to_date: isoDay(0),
      });
      rows.push(...r.data);
      if (page >= r.total_pages) break;
    }
    const sumBase = async (type: string): Promise<number> => {
      const groups = new Map<string, number>();
      for (const t of rows) {
        if (t.type !== type || t.to_account_id) continue;
        if (t.description === "Initial balance") continue;
        const key = `${t.date.slice(0, 10)}|${ccy.get(t.account_id)}`;
        groups.set(key, (groups.get(key) ?? 0) + t.amount);
      }
      let total = 0;
      for (const [key, cents] of groups) {
        const [day, c] = key.split("|");
        total += convert(cents, await dayRate(api, c, day));
      }
      return total;
    };
    const expectedSpent = await sumBase("expense");
    const expectedEarned = await sumBase("income");

    const summary = (await api.get("/api/v1/transactions/monthly-summary")).data.find(
      (m: Json) => m.month === first.slice(0, 7)
    );
    expect(summary.expenses).toBe(expectedSpent);
    expect(summary.income).toBe(expectedEarned);

    await app.goto("/analytics");
    await expect(
      app.getByRole("link", { name: `RM ${money(expectedSpent)}`, exact: true }).first()
    ).toBeVisible();
    await expect(
      app.getByRole("link", { name: `RM ${money(expectedEarned)}`, exact: true }).first()
    ).toBeVisible();
    await expect(app.getByRole("heading", { name: "Insights" })).toBeVisible();
    await expect(app.getByText("Largest expenses")).toBeVisible();
  });

  test("previous month navigation updates the figures", async ({ app, api }) => {
    await app.goto("/analytics");
    await app.getByRole("button", { name: "Previous month" }).click();
    const prev = new Date();
    prev.setUTCDate(1);
    prev.setUTCMonth(prev.getUTCMonth() - 1);
    const key = `${prev.getUTCFullYear()}-${String(prev.getUTCMonth() + 1).padStart(2, "0")}`;
    const summary = (await api.get("/api/v1/transactions/monthly-summary")).data.find(
      (m: Json) => m.month === key
    );
    await expect(
      app.getByRole("link", { name: `RM ${money(summary.expenses)}`, exact: true }).first()
    ).toBeVisible();
  });
});
