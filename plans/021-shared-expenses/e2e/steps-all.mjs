const W = "http://localhost:3000";
async function openCreate(page) {
  await page.goto(`${W}/transactions`);
  await page.waitForLoadState("networkidle");
  await page.getByRole("button", { name: "Add transaction" }).first().click();
  const dlg = page.getByRole("dialog", { name: "New transaction" });
  await dlg.getByRole("button", { name: "Expense" }).click();
  return dlg;
}
async function fillBasics(page, dlg, { amount, category, note }) {
  await dlg.getByLabel("Amount").fill(amount);
  await dlg.getByLabel("Account").click();
  await page.getByRole("option", { name: /Maybank Savings/ }).click();
  if (category) {
    await dlg.getByLabel("Category").click();
    await page.getByRole("option", { name: category }).click();
  }
  await dlg.getByLabel("Note").fill(note);
  await dlg.getByRole("button", { name: /Money owed back/ }).click();
}
async function submitCreate(page, dlg) {
  await dlg.getByRole("button", { name: "Add transaction" }).click();
  await dlg.waitFor({ state: "hidden", timeout: 15000 });
  await page.waitForLoadState("networkidle");
}
export default async function ({ page, shot }) {
  // A. Split lunch 3 ways
  let dlg = await openCreate(page);
  await fillBasics(page, dlg, { amount: "60.00", category: /Food & Dining/, note: "Team lunch at Nando's" });
  await dlg.getByRole("radio", { name: "Split with others" }).click();
  const names = dlg.locator('input[id^="owed-name-"]');
  await names.nth(0).fill("Alex");
  await dlg.getByRole("button", { name: /Add person/ }).click();
  await names.nth(1).fill("Sam");
  await dlg.getByText("New transaction").click();
  await dlg.getByText("Your share").first().scrollIntoViewIfNeeded();
  await shot("01-create-split-expense");
  await submitCreate(page, dlg);

  // C. Work claim (full amount owed back)
  dlg = await openCreate(page);
  await fillBasics(page, dlg, { amount: "400.00", category: /Travel/, note: "Flight to client site" });
  await dlg.getByRole("radio", { name: "Work or insurance claim" }).click();
  await dlg.locator('input[id^="owed-name-"]').first().fill("Acme Corp");
  await dlg.getByText("New transaction").click();
  await dlg.getByText("Your share").first().scrollIntoViewIfNeeded();
  await shot("02-create-work-claim");
  await submitCreate(page, dlg);

  // D. Rental deposit
  dlg = await openCreate(page);
  await fillBasics(page, dlg, { amount: "1000.00", note: "Apartment deposit" });
  await dlg.getByRole("radio", { name: "Deposit" }).click();
  await dlg.locator('input[id^="owed-name-"]').first().fill("Landlord");
  await dlg.getByText("New transaction").click();
  await submitCreate(page, dlg);

  // List with badges + your share
  await page.goto(`${W}/transactions`);
  await page.waitForLoadState("networkidle");
  await shot("03-transactions-list");

  // B. Open the lunch, record Alex's repayment
  await page.getByText("Team lunch at Nando's").first().click();
  const edit = page.getByRole("dialog").first();
  await edit.getByText("Alex").first().waitFor();
  await shot("04-shared-expense-detail");
  await edit.getByRole("button", { name: "Record repayment" }).first().click();
  const settle = page.getByRole("dialog", { name: "Record repayment" });
  await settle.getByLabel("Amount").waitFor();
  await shot("05-record-repayment");
  await settle.getByRole("button", { name: "Record repayment" }).click();
  await settle.waitFor({ state: "hidden", timeout: 15000 });
  await page.waitForLoadState("networkidle");
  await page.waitForTimeout(800);
  await shot("06-after-repayment");
  await page.keyboard.press("Escape");

  // E. Owed to me + lend money
  await page.goto(`${W}/receivables`);
  await page.waitForLoadState("networkidle");
  await page.getByRole("button", { name: "Lend money" }).first().click();
  const lend = page.getByRole("dialog", { name: "Lend money" });
  await lend.getByLabel("From account").click();
  await page.getByRole("option", { name: /Maybank Savings/ }).click();
  await lend.getByLabel("Borrower").fill("Alex");
  await lend.getByLabel("Amount").fill("500.00");
  await lend.getByText("Lend money").first().click();
  await shot("07-lend-money");
  await lend.getByRole("button", { name: "Record loan" }).click();
  await lend.waitFor({ state: "hidden", timeout: 15000 });
  await page.waitForLoadState("networkidle");
  await page.waitForTimeout(800);
  await shot("08-owed-to-me");

  // F. Deposit returned partially, write off the rest
  await page.getByRole("button", { name: /Landlord/ }).first().click();
  await page.getByRole("button", { name: "Record repayment" }).first().click();
  const s2 = page.getByRole("dialog", { name: "Record repayment" });
  await s2.getByLabel("Amount").fill("850.00");
  await s2.getByText(/Write off the rest/).click();
  await shot("09-deposit-partial-writeoff");
  await s2.getByRole("button", { name: "Record repayment" }).click();
  await s2.waitFor({ state: "hidden", timeout: 15000 });
  await page.waitForLoadState("networkidle");

  // G. All statuses, expand Alex
  await page.getByLabel("Status").click();
  await page.getByRole("option", { name: "All" }).click();
  await page.waitForLoadState("networkidle");
  await page.getByRole("button", { name: /^Alex/ }).first().click();
  await page.waitForLoadState("networkidle");
  await shot("10-owed-to-me-all-expanded", { fullPage: true });

  // H. Budgets, analytics, dashboard
  await page.goto(`${W}/budgets`); await page.waitForLoadState("networkidle");
  await shot("11-budgets");
  await page.goto(`${W}/analytics`); await page.waitForLoadState("networkidle");
  await shot("12-analytics", { fullPage: true });
  await page.goto(`${W}/`); await page.waitForLoadState("networkidle");
  await shot("13-dashboard", { fullPage: true });

  // I. Repayment row is read-only
  await page.goto(`${W}/transactions`); await page.waitForLoadState("networkidle");
  await page.getByText(/Repayment from Alex/).first().click();
  await page.getByRole("dialog").first().waitFor();
  await shot("14-repayment-readonly");
  await page.keyboard.press("Escape");

  // J. Mobile view
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`${W}/receivables`); await page.waitForLoadState("networkidle");
  await shot("15-owed-to-me-mobile", { fullPage: true });
}
