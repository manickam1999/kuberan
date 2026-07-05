import {
  ArrowLeftRight,
  ArrowDownRight,
  ArrowUpRight,
  Banknote,
  CreditCard,
  Landmark,
  TrendingUp,
  Wallet,
  type LucideIcon,
} from "lucide-react";
import type { AccountType, TransactionType } from "@/types/models";

/**
 * Central visual vocabulary for domain entities so account and transaction
 * types read identically everywhere (dashboard, lists, detail pages). Colors
 * reference semantic theme tokens rather than hardcoded hex.
 */

export interface AccountVisual {
  readonly label: string;
  readonly icon: LucideIcon;
  /** Text color class for the value/icon. */
  readonly text: string;
  /** Soft tinted chip background + foreground for the icon medallion. */
  readonly chip: string;
  /** Solid accent used for bars/sparklines (CSS var). */
  readonly accent: string;
}

export const ACCOUNT_VISUALS: Record<AccountType, AccountVisual> = {
  cash: {
    label: "Cash",
    icon: Banknote,
    text: "text-foreground",
    chip: "bg-primary/12 text-primary",
    accent: "var(--primary)",
  },
  investment: {
    label: "Investment",
    icon: Landmark,
    text: "text-investment",
    chip: "bg-investment-muted text-investment",
    accent: "var(--investment)",
  },
  credit_card: {
    label: "Credit Card",
    icon: CreditCard,
    text: "text-warning",
    chip: "bg-warning-muted text-warning",
    accent: "var(--warning)",
  },
  debt: {
    label: "Debt",
    icon: Landmark,
    text: "text-negative",
    chip: "bg-negative-muted text-negative",
    accent: "var(--negative)",
  },
};

export function accountVisual(type: string): AccountVisual {
  return ACCOUNT_VISUALS[type as AccountType] ?? ACCOUNT_VISUALS.cash;
}

export interface TransactionVisual {
  readonly label: string;
  readonly icon: LucideIcon;
  readonly text: string;
  readonly chip: string;
  /** Sign shown before the amount. Transfers are neutral. */
  readonly sign: "+" | "-" | "";
}

export const TRANSACTION_VISUALS: Record<TransactionType, TransactionVisual> = {
  income: {
    label: "Income",
    icon: ArrowUpRight,
    text: "text-positive",
    chip: "bg-positive-muted text-positive",
    sign: "+",
  },
  expense: {
    label: "Expense",
    icon: ArrowDownRight,
    text: "text-negative",
    chip: "bg-negative-muted text-negative",
    sign: "-",
  },
  transfer: {
    label: "Transfer",
    icon: ArrowLeftRight,
    text: "text-info",
    chip: "bg-info-muted text-info",
    sign: "",
  },
  investment: {
    label: "Investment",
    icon: TrendingUp,
    text: "text-investment",
    chip: "bg-investment-muted text-investment",
    sign: "",
  },
};

export function transactionVisual(type: string): TransactionVisual {
  return (
    TRANSACTION_VISUALS[type as TransactionType] ?? {
      label: type,
      icon: Wallet,
      text: "text-muted-foreground",
      chip: "bg-muted text-muted-foreground",
      sign: "",
    }
  );
}
