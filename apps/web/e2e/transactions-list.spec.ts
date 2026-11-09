import { test, expect, uid } from "./lib/fixtures";
import { expectToast, isoDay, pickOption } from "./lib/ui";
import type { Api, Json } from "./lib/api";

interface Seed {
  accountId: string;
  accountName: string;
  ids: Record<string, string>;
}

/** An isolated account with a handful of dated transactions made through the API. */
async function seedList(
  api: Api,
  mkCash: (n: string, c: string, b: number) => Promise<Json>,
  cats: Record<string, string>,
  prefix: string
): Promise<Seed> {
  const accountName = uid(prefix);
  const acc = await mkCash(accountName, "MYR", 0);
  const ids: Record<string, string> = {};
  const mk = async (key: string, body: Json) => {
    const r = await api.post("/api/v1/transactions", { account_id: acc.id, ...body });
    ids[key] = r.transaction.id;
  };
  await mk("rent", { type: "expense", amount: 123_456, description: `${prefix} rent`, category_id: cats["Transport"], date: isoDay(40) });
  await mk("lunch", { type: "expense", amount: 1_850, description: `${prefix} lunch`, category_id: cats["Dining"], date: isoDay(2) });
  await mk("shop", { type: "expense", amount: 9_990, description: `${prefix} shop`, category_id: cats["Groceries"], date: isoDay(1) });
  await mk("pay", { type: "income", amount: 400_000, description: `${prefix} pay`, category_id: cats["Salary"], date: isoDay(1) });
  return { accountId: acc.id, accountName, ids };
}

/** "4 transactions" unfiltered, "1 transaction match" with filters. */
function count(n: number): RegExp {
  return new RegExp(`^${n} transactions?( match(es)?)?$`);
}

test.describe("transactions: list", () => {
  test("search by text and by amount", async ({ app, api, mkCash, state }) => {
    const s = await seedList(api, mkCash, state.categories, uid("S"));
    await app.goto(`/transactions?account=${s.accountId}`);
    await expect(app.getByText("rent").first()).toBeVisible();

    const box = app.getByRole("searchbox", { name: "Search transactions" });
    await box.fill("lunch");
    await expect(app.getByRole("button", { name: /lunch$/ })).toBeVisible();
    await expect(app.getByRole("button", { name: /rent$/ })).toHaveCount(0);

    // Amounts are searchable as typed on screen: 1,234.56 and 1234.56.
    await box.fill("1234.56");
    await expect(app.getByRole("button", { name: /rent$/ })).toBeVisible();
    await expect(app.getByRole("button", { name: /lunch$/ })).toHaveCount(0);

    await box.fill("no-such-thing-xyz");
    await expect(app.getByText(/no transactions|nothing|no results/i).first()).toBeVisible();
  });

  test("filters: account chip, type, category, period and amount range", async ({
    app,
    api,
    mkCash,
    state,
  }) => {
    const s = await seedList(api, mkCash, state.categories, uid("F"));
    await app.goto("/transactions");
    await app.getByRole("button", { name: "Account", exact: true }).click();
    await app.getByRole("button", { name: new RegExp(s.accountName) }).or(app.getByRole("option", { name: new RegExp(s.accountName) })).first().click();
    await expect(app).toHaveURL(new RegExp(`account=${s.accountId}`));
    await expect(app.getByText(count(4))).toBeVisible();

    // Type: income only.
    await app.getByRole("button", { name: "Type", exact: true }).click();
    await app.getByText("Income", { exact: true }).last().click();
    await expect(app.getByText(count(1))).toBeVisible();
    await expect(app.getByRole("button", { name: /pay$/ })).toBeVisible();
    await app.getByRole("button", { name: "Clear Income filter" }).or(app.getByRole("button", { name: /Clear Type filter/ })).first().click();
    await expect(app.getByText(count(4))).toBeVisible();

    // Category.
    await app.getByRole("button", { name: "Category", exact: true }).click();
    await app.getByRole("option", { name: "Groceries" }).click();
    await expect(app.getByText(count(1))).toBeVisible();
    await expect(app.getByRole("button", { name: /shop$/ })).toBeVisible();
    await app.getByRole("button", { name: "Clear Category filter" }).click();

    // Period: this month excludes the 40 day old rent; compare against the API.
    await app.getByRole("button", { name: "All time" }).click();
    await app.getByText("Last 90 days").click();
    await expect(app.getByText(count(4))).toBeVisible();
    await app.getByRole("button", { name: "Last 90 days", exact: true }).click();
    await app.getByText("This month", { exact: true }).click();
    const now = new Date();
    const inMonth = [40, 2, 1, 1].filter((d) => {
      const t = new Date(Date.now() - d * 86_400_000);
      return t.getUTCMonth() === now.getUTCMonth() && t.getUTCFullYear() === now.getUTCFullYear();
    }).length;
    await expect(
      app.getByText(count(inMonth))
    ).toBeVisible();
    await app.getByRole("button", { name: /Clear .* filter/ }).first().click();

    // Amount range 15 - 100 => lunch (18.50) and shop (99.90).
    await app.getByRole("button", { name: "Amount", exact: true }).click();
    await app.locator("#tx-filter-min").fill("15");
    await app.locator("#tx-filter-max").fill("100");
    await app.getByRole("button", { name: "Apply" }).click();
    await expect(app.getByText(count(2))).toBeVisible();
    await expect(app.getByRole("button", { name: /lunch$/ })).toBeVisible();
    await expect(app.getByRole("button", { name: /shop$/ })).toBeVisible();
  });

  test("edit amount, category, account, date and note", async ({ app, api, mkCash, state }) => {
    const s = await seedList(api, mkCash, state.categories, uid("E"));
    const other = await mkCash(uid("Other"), "MYR", 0);
    await app.goto(`/transactions?account=${s.accountId}`);
    await app.getByRole("button", { name: /lunch$/ }).click();
    const dlg = app.getByRole("dialog");
    await expect(dlg.locator("#edit-amount, [id$=amount]").first()).toHaveValue("18.50");

    await dlg.getByRole("textbox", { name: "Amount" }).fill("20+2.5");
    await dlg.getByRole("textbox", { name: "Amount" }).press("Tab");
    await pickOption(app, dlg.getByRole("combobox", { name: "Category" }), "Groceries");
    await pickOption(app, dlg.getByRole("combobox", { name: "Account" }), new RegExp(other.name));
    await dlg.getByRole("textbox", { name: "Date" }).fill(isoDay(3));
    await dlg.getByRole("textbox", { name: "Note" }).fill("edited via e2e");
    await dlg.getByRole("button", { name: "Save", exact: true }).click();
    await expect(dlg).toBeHidden();

    const tx = (await api.get(`/api/v1/transactions/${s.ids.lunch}`)).transaction;
    expect(tx.amount).toBe(2_250);
    expect(tx.category_id).toBe(state.categories["Groceries"]);
    expect(tx.account_id).toBe(other.id);
    expect(tx.date.slice(0, 10)).toBe(isoDay(3));
    expect(tx.notes).toBe("edited via e2e");
    // The move re-balanced both accounts: lunch was 1,850 on the first one.
    expect(await api.balance(other.id)).toBe(-2_250);
    expect(await api.balance(s.accountId)).toBe(-123_456 - 9_990 + 400_000);
  });

  test("delete a transaction restores the balance", async ({ app, api, mkCash, state }) => {
    const s = await seedList(api, mkCash, state.categories, uid("D"));
    const before = await api.balance(s.accountId);
    await app.goto(`/transactions?account=${s.accountId}`);
    await app.getByRole("button", { name: /shop$/ }).click();
    const dlg = app.getByRole("dialog");
    await dlg.getByRole("button", { name: "Delete", exact: true }).click();
    // A confirmation step follows.
    await app.getByRole("alertdialog").getByRole("button", { name: /delete/i }).click();
    await expect(dlg).toBeHidden();
    await expect(app.getByRole("button", { name: /shop$/ })).toHaveCount(0);
    expect(await api.balance(s.accountId)).toBe(before + 9_990);
  });

  test("bulk recategorise, mismatch guard, and bulk delete", async ({
    app,
    api,
    mkCash,
    state,
  }) => {
    const s = await seedList(api, mkCash, state.categories, uid("B"));
    await app.goto(`/transactions?account=${s.accountId}`);
    await expect(app.getByRole("button", { name: /lunch$/ })).toBeVisible();

    // Two expenses -> Dining category via the bulk bar.
    await app.getByRole("checkbox", { name: /Select .* lunch/ }).check();
    await app.getByRole("checkbox", { name: /Select .* shop/ }).check();
    const bar = app.getByRole("region", { name: "Bulk actions" });
    await expect(bar).toBeVisible();
    await bar.getByRole("button", { name: "Category" }).click();
    await app.getByRole("option", { name: "Transport" }).click();
    await expectToast(app, "Moved 2 transactions to Transport");
    const [lunch, shop] = await Promise.all([
      api.get(`/api/v1/transactions/${s.ids.lunch}`),
      api.get(`/api/v1/transactions/${s.ids.shop}`),
    ]);
    expect(lunch.transaction.category_id).toBe(state.categories["Transport"]);
    expect(shop.transaction.category_id).toBe(state.categories["Transport"]);

    // The API refuses a mismatched category outright.
    await expect(
      api.post("/api/v1/transactions/bulk", {
        ids: [s.ids.pay],
        set: { category_id: state.categories["Transport"] },
      })
    ).rejects.toMatchObject({ status: 400, body: { error: { code: "CATEGORY_TYPE_MISMATCH" } } });

    // Mixed income + expense selection cannot be categorised in the UI.
    await app.getByRole("checkbox", { name: /Select .* pay/ }).check();
    await app.getByRole("checkbox", { name: /Select .* lunch/ }).check();
    await app.getByRole("checkbox", { name: /Select .* shop/ }).check();
    await expect(bar.getByRole("button", { name: "Category" })).toBeDisabled();

    // Bulk delete the three selected rows.
    await bar.getByRole("button", { name: "Delete selected" }).click();
    await app.getByRole("alertdialog").getByRole("button", { name: /delete/i }).click();
    await expect(app.getByRole("button", { name: /lunch$/ })).toHaveCount(0);
    await expect(app.getByRole("button", { name: /rent$/ })).toBeVisible();
    // Only the 1,234.56 rent remains charged.
    expect(await api.balance(s.accountId)).toBe(-123_456);
  });
});
