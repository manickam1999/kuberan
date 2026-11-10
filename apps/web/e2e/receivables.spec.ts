import { test, expect, uid } from "./lib/fixtures";
import { openAddTransaction, pickOption } from "./lib/ui";
import type { Api, Json } from "./lib/api";
import type { Page } from "@playwright/test";

/** Create a USD shared expense (US$90 split two ways) through the dialog. */
async function shareDinner(
  app: Page,
  accountName: string,
  person: string,
  description: string
) {
  await app.goto("/transactions");
  const dlg = await openAddTransaction(app);
  await dlg.locator("#tx-amount").fill("90");
  await dlg.locator("#tx-description").fill(description);
  await pickOption(app, dlg.locator("#tx-account"), new RegExp(accountName));
  await dlg.getByRole("button", { name: "Split with someone" }).click();
  await dlg.getByRole("textbox", { name: "Person 1" }).fill(person);
  await expect(dlg.getByText(/owes\s*US\$\s*45\.00/)).toBeVisible();
  await dlg.getByRole("button", { name: "Save expense" }).click();
  await expect(dlg).toBeHidden();
}

async function receivableFor(api: Api, person: string): Promise<Json> {
  const r = await api.get("/api/v1/receivables", { page_size: 100 });
  const hit = (r.data as Json[]).filter((x) => x.counterparty_name === person);
  expect(hit, `receivable for ${person}`).toHaveLength(1);
  return hit[0];
}

test.describe("receivables", () => {
  test("shared expense creates a receivable for the other share", async ({
    app,
    api,
    mkCash,
  }) => {
    const acc = await mkCash(uid("Shr"), "USD", 100_000);
    const person = uid("Sam");
    await shareDinner(app, acc.name, person, "Shared dinner");

    // The whole US$90 left the account; only the other US$45 is owed back.
    expect(await api.balance(acc.id)).toBe(100_000 - 9_000);
    const r = await receivableFor(api, person);
    expect(r.amount).toBe(4_500);
    expect(r.currency).toBe("USD");
    expect(r.kind).toBe("split");
    expect(r.outstanding).toBe(4_500);
    expect(r.transaction.personal_amount).toBe(4_500);

    await app.goto("/receivables");
    await expect(app.getByText(person).first()).toBeVisible();
    await expect(app.getByText(/US\$\s*45\.00/).first()).toBeVisible();
  });

  test("settle into an account in another currency stores account_amount", async ({
    app,
    api,
    mkCash,
  }) => {
    const usd = await mkCash(uid("ShrUsd"), "USD", 100_000);
    const sgd = await mkCash(uid("ShrSgd"), "SGD", 0);
    const person = uid("Jo");
    await shareDinner(app, usd.name, person, "Dinner for settling");

    await app.goto("/receivables");
    const card = app
      .locator("div")
      .filter({ hasText: "Dinner for settling", has: app.getByRole("button", { name: "Record repayment" }) })
      .last();
    await card.getByRole("button", { name: "Record repayment" }).click();
    const dlg = app.getByRole("dialog", { name: new RegExp(`Repayment from ${person}`) });
    await pickOption(app, dlg.locator("#settle-account"), new RegExp(sgd.name));
    const received = dlg.locator("#settle-account-amount");
    // US$45 at 4.5 -> 3.35 is S$60.45 (the dialog suggests it from the live rate).
    await expect(received).toHaveValue("60.45");
    await received.fill("60.00");
    await received.press("Tab");
    await dlg.getByRole("button", { name: /^Record/ }).click();
    await expect(dlg).toBeHidden();

    expect(await api.balance(sgd.id)).toBe(6_000);
    const r = await receivableFor(api, person).catch(async () => {
      const all = await api.get("/api/v1/receivables", { page_size: 100, status: "settled" });
      return (all.data as Json[]).find((x) => x.counterparty_name === person)!;
    });
    expect(r.paid_amount).toBe(4_500);
    expect(r.outstanding).toBe(0);
    expect(r.status).toBe("settled");

    // Deleting the settlement reverses the SGD account by what it received.
    const detail = (await api.get(`/api/v1/receivables/${r.id}`)).receivable ?? (await api.get(`/api/v1/receivables/${r.id}`));
    const settlement = (detail.settlements ?? [])[0];
    expect(settlement?.account_amount).toBe(6_000);
    await api.del(`/api/v1/receivables/settlements/${settlement.id}`);
    expect(await api.balance(sgd.id)).toBe(0);
  });

  test("write off a receivable", async ({ app, api, mkCash }) => {
    const usd = await mkCash(uid("WoUsd"), "USD", 100_000);
    const person = uid("Wo");
    await shareDinner(app, usd.name, person, "Dinner to write off");
    const before = await receivableFor(api, person);

    await app.goto("/receivables");
    await app.getByRole("button", { name: "More actions for Dinner to write off" }).click();
    await app.getByRole("menuitem", { name: /write off/i }).click();
    const confirm = app.getByRole("alertdialog").or(app.getByRole("dialog")).last();
    await confirm.getByRole("button", { name: /write off/i }).click();

    await expect
      .poll(async () => {
        const all = await api.get("/api/v1/receivables", { page_size: 100, status: "written_off" });
        return (all.data as Json[]).find((x) => x.id === before.id)?.written_off_amount;
      })
      .toBe(4_500);
    // Writing off never touches the account balance.
    expect(await api.balance(usd.id)).toBe(100_000 - 9_000);
  });
});
