import { test, expect, uid } from "./lib/fixtures";
import { isoDay } from "./lib/ui";
import type { Json } from "./lib/api";

test.describe("categories", () => {
  test("create an expense category and a sub-category", async ({ app, api }) => {
    const name = uid("Cat");
    await app.goto("/categories");
    await app.getByRole("button", { name: "New category" }).click();
    const dlg = app.getByRole("dialog");
    await dlg.getByRole("textbox", { name: "Name" }).fill(name);
    await dlg.getByRole("radio", { name: "wallet" }).click();
    await dlg.getByRole("button", { name: "Create category" }).click();
    await expect(dlg).toBeHidden();

    const cats: Json[] = (await api.get("/api/v1/categories", { page_size: 100 })).data;
    const created = cats.find((c) => c.name === name);
    expect(created?.type).toBe("expense");
    await expect(app.getByRole("button", { name: new RegExp(name) })).toBeVisible();

    const sub = uid("Sub");
    await app.getByRole("button", { name: "New category" }).click();
    await dlg.getByRole("textbox", { name: "Name" }).fill(sub);
    await dlg.getByRole("combobox", { name: "Parent" }).click();
    await app.getByRole("option", { name }).click();
    await dlg.getByRole("button", { name: "Create category" }).click();
    await expect(dlg).toBeHidden();
    const after: Json[] = (await api.get("/api/v1/categories", { page_size: 100 })).data;
    const flat = after.flatMap((c) => [c, ...(c.children ?? [])]);
    expect(flat.find((c) => c.name === sub)?.parent_id).toBe(created!.id);
  });
});

test.describe("rules", () => {
  test("create a rule from a transaction, preview, and recategorise", async ({
    app,
    api,
    mkCash,
    state,
  }) => {
    const token = uid("zq");
    const acc = await mkCash(uid("RuleAcc"), "MYR", 100_000);
    const mk = (description: string, category: string | null) =>
      api.post("/api/v1/transactions", {
        account_id: acc.id,
        type: "expense",
        amount: 1_000,
        description,
        category_id: category ?? undefined,
        date: isoDay(1),
      });
    // Three look-alikes (one already Transport), plus an unrelated row.
    await mk(`${token} cafe one`, state.categories["Transport"]);
    await mk(`${token} cafe two`, state.categories["Dining"]);
    await mk(`${token} cafe three`, null);
    await mk("unrelated purchase", null);

    await app.goto(`/transactions?account=${acc.id}`);
    await app.getByRole("button", { name: `${token} cafe one` }).click();
    await app.getByRole("button", { name: "Create rule" }).click();
    const dlg = app.getByRole("dialog", { name: "New rule" });
    const text = dlg.getByRole("textbox", { name: "Text to match" });
    await text.fill(token);
    // The preview counts what the rule would match today.
    await expect(dlg.getByText(/Matches 3 existing transactions/)).toBeVisible();
    await dlg.getByRole("checkbox", { name: /Recategorise them too/ }).check();
    await dlg.getByRole("button", { name: "Save rule" }).click();
    await expect(dlg).toBeHidden();

    const rows: Json[] = await api.transactions({ q: token });
    expect(rows).toHaveLength(3);
    for (const r of rows) expect(r.category_id).toBe(state.categories["Transport"]);
    const other = (await api.transactions({ q: "unrelated purchase" })).find(
      (r) => r.account_id === acc.id
    );
    expect(other?.category_id ?? null).toBeNull();

    // The rule now categorises a brand-new transaction by itself.
    const fresh = (await mk(`${token} cafe four`, null)).transaction;
    expect((await api.get(`/api/v1/transactions/${fresh.id}`)).transaction.category_id).toBe(
      state.categories["Transport"]
    );

    // And it is listed on the rules page, where Apply is available.
    await app.goto("/rules");
    await expect(app.getByText(new RegExp(token)).first()).toBeVisible();
  });
});
