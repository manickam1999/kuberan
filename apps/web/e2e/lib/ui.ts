import { expect, type Locator, type Page } from "@playwright/test";

/** Pick an option of a Radix/ShadCN Select by its trigger and the option text. */
export async function pickOption(
  page: Page,
  trigger: Locator,
  option: string | RegExp
) {
  await trigger.click();
  await page.getByRole("option", { name: option }).first().click();
}

/** Format cents like the app does for en-MY money, e.g. 123450 -> "1,234.50". */
export function fmt(cents: number): string {
  const sign = cents < 0 ? "-" : "";
  const abs = Math.abs(cents);
  const whole = Math.floor(abs / 100).toLocaleString("en-US");
  return `${sign}${whole}.${String(abs % 100).padStart(2, "0")}`;
}

/** Today as yyyy-mm-dd in UTC (the suite runs the browser in UTC). */
export function isoDay(daysAgo = 0): string {
  return new Date(Date.now() - daysAgo * 86_400_000).toISOString().slice(0, 10);
}

/** Wait for a toast containing the text (Sonner). */
export async function expectToast(page: Page, text: string | RegExp) {
  await expect(page.locator("[data-sonner-toast]").filter({ hasText: text }).first()).toBeVisible();
}

/** Open the add-transaction dialog from the page (desktop button or tab bar +). */
export async function openAddTransaction(page: Page) {
  const header = page.getByRole("button", { name: /^New\b/ });
  if (await header.isVisible().catch(() => false)) {
    await header.click();
  } else {
    await page.getByRole("button", { name: /add|new transaction/i }).first().click();
  }
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  return dialog;
}
