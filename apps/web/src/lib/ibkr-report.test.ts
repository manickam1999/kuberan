import { describe, expect, it } from "vitest";
import type { IbkrImportReport } from "@/types/api";
import { reportFromErrorDetails, summarizeIbkrReport } from "./ibkr-report";

function report(over: Partial<IbkrImportReport> = {}): IbkrImportReport {
  return {
    applied: false,
    account: "U1",
    base_currency: "USD",
    periods: null,
    ignored_sections: null,
    securities: [
      {
        symbol: "VOO",
        security_id: "s1",
        investment_id: null,
        existing_transactions: 0,
        new_transactions: 3,
        checkpoints: [
          {
            date: "2025-12-31",
            ibkr_quantity: 2,
            quantity: 2,
            quantity_ok: true,
            ibkr_cost_basis: 0,
            cost_basis: 0,
            cost_basis_diff: 0,
          },
        ],
      },
    ],
    cash: {
      opening_balance: 0,
      legs: 4,
      dividends: 1,
      fees: 2,
      deposits_matched: null,
      unmatched_deposits: null,
      checkpoints: null,
    },
    errors: null,
    warnings: ["a"],
    ...over,
  };
}

describe("summarizeIbkrReport", () => {
  it("allows apply for a clean report", () => {
    const s = summarizeIbkrReport(report());
    expect(s).toMatchObject({
      canApply: true,
      newTransactions: 3,
      cashRows: 7,
      warningCount: 1,
      securityCount: 1,
    });
  });

  it("blocks apply on errors", () => {
    const s = summarizeIbkrReport(
      report({ errors: [{ code: "unknown_security", detail: "X" }] })
    );
    expect(s.canApply).toBe(false);
    expect(s.errorCount).toBe(1);
  });

  it("blocks apply on a quantity mismatch and names the symbol", () => {
    const r = report();
    r.securities![0].checkpoints![0].quantity_ok = false;
    const s = summarizeIbkrReport(r);
    expect(s.canApply).toBe(false);
    expect(s.quantityMismatches).toEqual(["VOO"]);
  });

  it("tolerates null lists", () => {
    const s = summarizeIbkrReport(report({ securities: null }));
    expect(s.securityCount).toBe(0);
  });
});

describe("reportFromErrorDetails", () => {
  it("extracts the report", () => {
    const r = report();
    expect(reportFromErrorDetails({ report: r })).toBe(r);
  });
  it("returns null without one", () => {
    expect(reportFromErrorDetails(undefined)).toBeNull();
    expect(reportFromErrorDetails({ report: "x" })).toBeNull();
  });
});
