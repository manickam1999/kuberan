import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { login, pipeline, register, type Json } from "./lib/api";

export const RUN_DIR = join(__dirname, ".run");
export const STATE_FILE = join(RUN_DIR, "state.json");

/** Rates the suite relies on (latest day). Earlier days wobble around them. */
export const LATEST_RATES = { USD: "4.5000", SGD: "3.3500" } as const;

const DAY = 86_400_000;

function utcMidnight(daysAgo: number): string {
  const d = new Date(Date.now() - daysAgo * DAY);
  d.setUTCHours(0, 0, 0, 0);
  return d.toISOString();
}

/** ~200 daily USD->MYR and SGD->MYR quotes; day 0 is exactly LATEST_RATES. */
async function seedFx() {
  const rates: Json[] = [];
  for (let i = 0; i < 200; i++) {
    const wobble = (i % 7) * 0.01;
    rates.push({
      base: "USD",
      quote: "MYR",
      rate: i === 0 ? LATEST_RATES.USD : (4.38 + wobble).toFixed(4),
      recorded_at: utcMidnight(i),
      source: "manual",
    });
    rates.push({
      base: "SGD",
      quote: "MYR",
      rate: i === 0 ? LATEST_RATES.SGD : (3.28 + wobble / 2).toFixed(4),
      recorded_at: utcMidnight(i),
      source: "manual",
    });
  }
  await pipeline("/api/v1/pipeline/fx-rates", { rates });
}

async function seedSecurities(stamp: string) {
  const make = async (body: Json) => {
    const r = await pipeline("/api/v1/pipeline/securities", body);
    return r.security as Json;
  };
  const usd = await make({
    symbol: `E2U${stamp}`.toUpperCase(),
    name: "E2E Dollar Corp",
    asset_type: "stock",
    currency: "USD",
  });
  const myr = await make({
    symbol: `E2M${stamp}`.toUpperCase(),
    name: "E2E Ringgit Bhd",
    asset_type: "stock",
    currency: "MYR",
  });
  const prices: Json[] = [];
  // 30 days of prices so charts have a line. USD security: $200.00 today.
  for (let i = 29; i >= 0; i--) {
    prices.push({
      security_id: usd.id,
      price: 20000 - i * 10,
      currency: "USD",
      recorded_at: utcMidnight(i),
    });
    prices.push({
      security_id: myr.id,
      price: 1000 - i,
      currency: "MYR",
      recorded_at: utcMidnight(i),
    });
  }
  await pipeline("/api/v1/pipeline/securities/prices", { prices });
  return { usd, myr };
}

export interface E2EState {
  email: string;
  password: string;
  usdSymbol: string;
  myrSymbol: string;
  usdSecurityId: string;
  myrSecurityId: string;
  /** Account ids by seed name. */
  accounts: Record<string, string>;
  categories: Record<string, string>;
}

export default async function globalSetup() {
  mkdirSync(RUN_DIR, { recursive: true });
  await seedFx();
  const stamp = Date.now().toString(36);
  const { usd, myr } = await seedSecurities(stamp);

  const email = `e2e+${stamp}@kuberan.test`;
  const password = "e2e-password-1";
  await register({
    email,
    password,
    first_name: "Ellis",
    last_name: "Tester",
    base_currency: "MYR",
  });
  const api = await login(email, password);

  const accounts: Record<string, string> = {};
  const mkCash = async (name: string, currency: string, balance: number) => {
    const r = await api.post("/api/v1/accounts/cash", {
      name,
      currency,
      initial_balance: balance,
    });
    accounts[name] = r.account.id;
  };
  await mkCash("Main MYR", "MYR", 1_000_000);
  await mkCash("USD Wallet", "USD", 500_000);
  await mkCash("SGD Wallet", "SGD", 300_000);
  const inv = await api.post("/api/v1/accounts/investment", {
    name: "Broker USD",
    currency: "USD",
    broker: "E2E Broker",
  });
  accounts["Broker USD"] = inv.account.id;

  const categories: Record<string, string> = {};
  const mkCat = async (name: string, type: string, color = "#3b82f6") => {
    const r = await api.post("/api/v1/categories", { name, type, color });
    categories[name] = r.category.id;
  };
  await mkCat("Groceries", "expense", "#22c55e");
  await mkCat("Transport", "expense", "#f59e0b");
  await mkCat("Dining", "expense", "#ef4444");
  await mkCat("Salary", "income", "#10b981");
  await mkCat("Bonus", "income", "#06b6d4");

  // Some history so lists, analytics and budgets have something to show.
  const day = (n: number) => new Date(Date.now() - n * DAY).toISOString().slice(0, 10);
  const seedTx = async (
    account: string,
    type: string,
    amount: number,
    category: string,
    description: string,
    daysAgo: number
  ) => {
    await api.post("/api/v1/transactions", {
      account_id: accounts[account],
      category_id: categories[category],
      type,
      amount,
      description,
      date: day(daysAgo),
    });
  };
  await seedTx("Main MYR", "income", 500_000, "Salary", "Seed salary", 20);
  await seedTx("Main MYR", "expense", 12_000, "Groceries", "Seed groceries one", 15);
  await seedTx("Main MYR", "expense", 8_000, "Transport", "Seed transport", 10);
  await seedTx("USD Wallet", "expense", 5_000, "Dining", "Seed dinner USD", 5);
  await seedTx("SGD Wallet", "expense", 2_500, "Dining", "Seed lunch SGD", 3);

  const state: E2EState = {
    email,
    password,
    usdSymbol: usd.symbol,
    myrSymbol: myr.symbol,
    usdSecurityId: usd.id,
    myrSecurityId: myr.id,
    accounts,
    categories,
  };
  writeFileSync(STATE_FILE, JSON.stringify(state, null, 2));
}
