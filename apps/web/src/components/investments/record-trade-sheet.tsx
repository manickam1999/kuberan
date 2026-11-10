"use client";

import { useMemo, useState } from "react";
import { toast } from "sonner";
import { ApiClientError, getFxAmountRequired } from "@/lib/api-client";
import {
  useAllInvestments,
  useInvestment,
  useRecordBuy,
  useRecordDividend,
  useRecordSell,
  useRecordSplit,
} from "@/hooks/use-investments";
import { useAccounts } from "@/hooks/use-accounts";
import { useBaseCurrency } from "@/hooks/use-profile";
import { useAuth } from "@/hooks/use-auth";
import { formatMoney, toLocalDateString, toRFC3339 } from "@/lib/format";
import {
  buyTotal,
  lineAmount,
  netDividend,
  previewSell,
  validateTrade,
  type TradeKind,
} from "@/lib/trade-math";
import {
  CrossCurrencyAmountField,
  FxRateField,
  isValidFxRate,
  useCrossCurrencyAmount,
} from "@/components/investments/cross-currency-field";
import { Money } from "@/components/ui/money";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { CurrencyInput } from "@/components/ui/currency-input";
import { Segmented } from "@/components/ui/segmented";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

const KIND_OPTIONS: { value: TradeKind; label: string }[] = [
  { value: "buy", label: "Buy" },
  { value: "sell", label: "Sell" },
  { value: "dividend", label: "Dividend" },
  { value: "split", label: "Split" },
];

/** "Record sale", "Record buy", ... */
const KIND_NOUN: Record<TradeKind, string> = {
  buy: "buy",
  sell: "sale",
  dividend: "dividend",
  split: "split",
};

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    if (error.code === "INSUFFICIENT_SHARES") {
      return "You do not have enough shares to sell this quantity";
    }
    return error.message;
  }
  return "An unexpected error occurred";
}

function units(n: number): string {
  return n.toLocaleString(undefined, { maximumFractionDigits: 6 });
}

export interface RecordTradeSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The holding to trade; without it the sheet first asks which one. */
  investmentId?: string;
  /** Which tab opens first (Sell and Split fall back to Buy on a closed position). */
  initialKind?: TradeKind;
}

/**
 * Buy, Sell, Dividend and Split in one sheet (a bottom sheet on phones). Keeps
 * every field of the four dialogs it replaced: the cash account that funds or
 * receives the trade, the amount in that account's currency when it differs
 * from the security's (`funding_amount` / `deposit_amount`, pre-filled and
 * corrected from FX_AMOUNT_REQUIRED), the optional exchange rate, dividend
 * withholding (`fee`) and notes. A sale previews proceeds, the cost of the
 * shares sold (average cost) and the realised gain before saving.
 */
export function RecordTradeSheet({
  open,
  onOpenChange,
  investmentId,
  initialKind = "buy",
}: RecordTradeSheetProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        {open && (
          <SheetBody
            investmentId={investmentId}
            initialKind={initialKind}
            onDone={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function SheetBody({
  investmentId,
  initialKind,
  onDone,
}: {
  investmentId?: string;
  initialKind: TradeKind;
  onDone: () => void;
}) {
  const [picked, setPicked] = useState("");
  const id = investmentId ?? picked;
  if (id) {
    return (
      <TradeForm
        key={id}
        investmentId={id}
        initialKind={initialKind}
        onDone={onDone}
      />
    );
  }
  return <HoldingPicker onPick={setPicked} />;
}

/** Step zero when the sheet opens from the Investments page: which holding. */
function HoldingPicker({ onPick }: { onPick: (id: string) => void }) {
  const { data, isLoading } = useAllInvestments({
    page: 1,
    page_size: 100,
    status: "all",
  });
  const holdings = data?.data ?? [];
  return (
    <>
      <DialogHeader>
        <DialogTitle>Record trade</DialogTitle>
        <DialogDescription>Choose the holding this trade is for.</DialogDescription>
      </DialogHeader>
      {isLoading ? (
        <Skeleton className="h-9 w-full" />
      ) : holdings.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          You have no holdings yet. Add one from an investment account first.
        </p>
      ) : (
        <div className="flex flex-col gap-2">
          <Label htmlFor="trade-holding">Holding</Label>
          <Select onValueChange={onPick}>
            <SelectTrigger id="trade-holding" className="w-full">
              <SelectValue placeholder="Select a holding" />
            </SelectTrigger>
            <SelectContent>
              {holdings.map((h) => (
                <SelectItem key={h.id} value={h.id}>
                  {h.security.symbol} · {h.security.name}
                  {h.account?.name ? ` (${h.account.name})` : ""}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}
    </>
  );
}

function PreviewRow({
  label,
  children,
  strong,
}: {
  label: string;
  children: React.ReactNode;
  strong?: boolean;
}) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <span className={cn("text-muted-foreground text-sm", strong && "text-foreground font-medium")}>
        {label}
      </span>
      <span className={cn("text-sm tabular-nums", strong && "font-medium")}>{children}</span>
    </div>
  );
}

function TradeForm({
  investmentId,
  initialKind,
  onDone,
}: {
  investmentId: string;
  initialKind: TradeKind;
  onDone: () => void;
}) {
  const { data: investment } = useInvestment(investmentId);
  const { data: accountsData } = useAccounts({ page: 1, page_size: 100 });
  const { hideBalances } = useAuth();
  const baseCurrency = useBaseCurrency();

  const position = investment?.quantity ?? 0;
  const closed = !!investment && position <= 0;
  const securityCurrency = investment?.currency || baseCurrency;
  const symbol = investment?.security.symbol ?? "";

  const [kindState, setKind] = useState<TradeKind>(initialKind);
  const kind: TradeKind =
    closed && (kindState === "sell" || kindState === "split") ? "buy" : kindState;

  const [date, setDate] = useState(() => toLocalDateString());
  const [quantity, setQuantity] = useState("");
  // Until edited, the price field offers the latest known price.
  const [priceEdit, setPriceEdit] = useState<number | null>(null);
  const pricePerUnit = priceEdit ?? investment?.current_price_native ?? 0;
  const [fee, setFee] = useState(0);
  const [amount, setAmount] = useState(0);
  const [dividendType, setDividendType] = useState("cash");
  const [splitRatio, setSplitRatio] = useState("");
  const [notes, setNotes] = useState("");
  const [accountId, setAccountId] = useState("");
  const [fxRate, setFxRate] = useState("");
  const [error, setError] = useState("");

  const recordBuy = useRecordBuy(investmentId);
  const recordSell = useRecordSell(investmentId);
  const recordDividend = useRecordDividend(investmentId);
  const recordSplit = useRecordSplit(investmentId);
  const isSubmitting =
    recordBuy.isPending ||
    recordSell.isPending ||
    recordDividend.isPending ||
    recordSplit.isPending;

  const cashAccounts = useMemo(
    () => accountsData?.data?.filter((a) => a.type === "cash" && a.is_active) ?? [],
    [accountsData?.data]
  );
  const selectedAccount = cashAccounts.find((a) => a.id === accountId);

  const qtyNum = parseFloat(quantity);
  const qty = Number.isFinite(qtyNum) ? qtyNum : 0;
  const ratioNum = parseFloat(splitRatio);
  const ratio = Number.isFinite(ratioNum) ? ratioNum : 0;

  const sell = useMemo(
    () =>
      kind === "sell" && investment
        ? previewSell({
            quantity: qty,
            pricePerUnit,
            fee,
            position,
            costBasis: investment.cost_basis,
          })
        : null,
    [kind, investment, qty, pricePerUnit, fee, position]
  );
  const totalCost = buyTotal(qty, pricePerUnit, fee);
  const net = netDividend(amount, fee);
  const nativeTotal =
    kind === "buy"
      ? totalCost
      : kind === "sell"
        ? Math.max(lineAmount(qty, pricePerUnit) - fee, 0)
        : kind === "dividend"
          ? net
          : 0;

  const cross = useCrossCurrencyAmount({
    securityCurrency,
    account: kind === "split" ? undefined : selectedAccount,
    nativeTotal,
  });

  function changeKind(next: TradeKind) {
    setKind(next);
    setError("");
    cross.reset();
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    const problem = validateTrade({
      kind,
      date,
      quantity: qty,
      pricePerUnit,
      fee,
      amount,
      splitRatio: ratio,
      position,
    });
    if (problem) {
      setError(problem);
      return;
    }
    if (kind !== "split") {
      if (!isValidFxRate(fxRate)) {
        setError("Exchange rate must be a positive number");
        return;
      }
      if (cross.needed && cross.value <= 0) {
        setError(`Enter the amount in ${cross.currency}`);
        return;
      }
    }

    const account = accountId && accountId !== "none" ? accountId : undefined;
    const accountAmount = cross.needed ? cross.value : undefined;
    const callbacks = {
      onSuccess: () => {
        toast.success(
          `${KIND_OPTIONS.find((o) => o.value === kind)?.label} recorded`
        );
        onDone();
      },
      onError: (err: unknown) => {
        const required = getFxAmountRequired(err);
        if (required) cross.applyHint(required);
        setError(getErrorMessage(err));
      },
    };
    const isoDate = toRFC3339(date);

    switch (kind) {
      case "buy":
        recordBuy.mutate(
          {
            date: isoDate,
            quantity: qty,
            price_per_unit: pricePerUnit,
            fee: fee > 0 ? fee : undefined,
            notes: notes.trim() || undefined,
            funding_account_id: account,
            funding_amount: accountAmount,
            fx_rate: fxRate.trim() || undefined,
          },
          callbacks
        );
        break;
      case "sell":
        recordSell.mutate(
          {
            date: isoDate,
            quantity: qty,
            price_per_unit: pricePerUnit,
            fee: fee > 0 ? fee : undefined,
            notes: notes.trim() || undefined,
            deposit_account_id: account,
            deposit_amount: accountAmount,
            fx_rate: fxRate.trim() || undefined,
          },
          callbacks
        );
        break;
      case "dividend":
        recordDividend.mutate(
          {
            date: isoDate,
            amount,
            fee: fee > 0 ? fee : undefined,
            deposit_account_id: account,
            deposit_amount: accountAmount,
            fx_rate: fxRate.trim() || undefined,
            dividend_type: dividendType || undefined,
            notes: notes.trim() || undefined,
          },
          callbacks
        );
        break;
      case "split":
        recordSplit.mutate(
          { date: isoDate, split_ratio: ratio, notes: notes.trim() || undefined },
          callbacks
        );
        break;
    }
  }

  const accountLabel =
    kind === "buy"
      ? "Pay from (optional)"
      : kind === "sell"
        ? "Proceeds go to (optional)"
        : "Deposit to (optional)";
  const crossLabel =
    kind === "buy"
      ? `Amount charged to ${selectedAccount?.name ?? "account"} (${cross.currency})`
      : `Amount received in ${selectedAccount?.name ?? "account"} (${cross.currency})`;
  const description =
    kind === "buy"
      ? "Record a purchase of additional shares."
      : kind === "sell"
        ? `Record a sale of shares. You hold ${units(position)} units.`
        : kind === "dividend"
          ? "Record a dividend payment for this investment."
          : "Record a split. A 2:1 split means each unit becomes 2 units.";
  const submitLabel = `Record ${KIND_NOUN[kind]}`;

  return (
    <form
      onSubmit={handleSubmit}
      onKeyDown={(e) => {
        if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
          e.preventDefault();
          e.currentTarget.requestSubmit();
        }
      }}
      className="flex min-h-0 flex-col gap-4"
    >
      <DialogHeader>
        <DialogTitle>
          Record {KIND_NOUN[kind]}
          {symbol ? ` · ${symbol}` : ""}
        </DialogTitle>
        <DialogDescription>{description}</DialogDescription>
      </DialogHeader>

      <Segmented
        full
        aria-label="Trade type"
        value={kind}
        onValueChange={changeKind}
        options={KIND_OPTIONS.map((o) => ({
          ...o,
          disabled: closed && (o.value === "sell" || o.value === "split"),
        }))}
      />

      {error && (
        <div
          role="alert"
          className="bg-destructive/10 text-destructive rounded-md p-3 text-sm"
        >
          {error}
        </div>
      )}

      <div className="grid gap-4 sm:grid-cols-2">
        {(kind === "buy" || kind === "sell") && (
          <>
            <div className="flex flex-col gap-2">
              <Label htmlFor="trade-quantity">Quantity</Label>
              <Input
                id="trade-quantity"
                type="number"
                inputMode="decimal"
                step="any"
                min="0"
                placeholder="0"
                value={quantity}
                onChange={(e) => setQuantity(e.target.value)}
                disabled={isSubmitting}
              />
              {kind === "sell" && (
                <p className="text-muted-foreground flex items-center gap-1 text-xs">
                  of {units(position)} held
                  <Button
                    type="button"
                    variant="link"
                    size="sm"
                    className="h-auto p-0 text-xs"
                    onClick={() => setQuantity(String(position))}
                    disabled={isSubmitting}
                  >
                    Sell all
                  </Button>
                </p>
              )}
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="trade-price">Price per unit</Label>
              <CurrencyInput
                id="trade-price"
                value={pricePerUnit}
                onChange={setPriceEdit}
                currency={securityCurrency}
                placeholder="0.00"
                disabled={isSubmitting}
              />
            </div>
          </>
        )}

        {kind === "dividend" && (
          <div className="flex flex-col gap-2">
            <Label htmlFor="trade-amount">Gross amount</Label>
            <CurrencyInput
              id="trade-amount"
              value={amount}
              onChange={setAmount}
              currency={securityCurrency}
              placeholder="0.00"
              disabled={isSubmitting}
            />
          </div>
        )}

        {kind === "split" && (
          <div className="flex flex-col gap-2">
            <Label htmlFor="trade-ratio">Split ratio</Label>
            <Input
              id="trade-ratio"
              type="number"
              inputMode="decimal"
              step="any"
              min="0"
              placeholder="e.g., 2 for a 2:1 split"
              value={splitRatio}
              onChange={(e) => setSplitRatio(e.target.value)}
              disabled={isSubmitting}
            />
          </div>
        )}

        {kind !== "split" && (
          <div className="flex flex-col gap-2">
            <Label htmlFor="trade-fee">
              {kind === "dividend" ? "Withholding / fees (optional)" : "Fee (optional)"}
            </Label>
            <CurrencyInput
              id="trade-fee"
              value={fee}
              onChange={setFee}
              currency={securityCurrency}
              placeholder="0.00"
              disabled={isSubmitting}
            />
          </div>
        )}

        <div className="flex flex-col gap-2">
          <Label htmlFor="trade-date">Date</Label>
          <Input
            id="trade-date"
            type="date"
            value={date}
            onChange={(e) => setDate(e.target.value)}
            disabled={isSubmitting}
          />
        </div>

        {kind === "dividend" && (
          <div className="flex flex-col gap-2">
            <Label htmlFor="trade-dividend-type">Dividend type</Label>
            <Select
              value={dividendType}
              onValueChange={setDividendType}
              disabled={isSubmitting}
            >
              <SelectTrigger id="trade-dividend-type" className="w-full">
                <SelectValue placeholder="Select type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="cash">Cash</SelectItem>
                <SelectItem value="stock">Stock</SelectItem>
                <SelectItem value="special">Special</SelectItem>
              </SelectContent>
            </Select>
          </div>
        )}

        {kind !== "split" && cashAccounts.length > 0 && (
          <div className="flex flex-col gap-2 sm:col-span-2">
            <Label htmlFor="trade-account">{accountLabel}</Label>
            <Select
              value={accountId}
              onValueChange={setAccountId}
              disabled={isSubmitting}
            >
              <SelectTrigger id="trade-account" className="w-full">
                <SelectValue placeholder="None" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">None</SelectItem>
                {cashAccounts.map((account) => (
                  <SelectItem key={account.id} value={account.id}>
                    {account.name} (
                    {formatMoney(account.balance, account.currency, hideBalances)})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {kind !== "split" && (
          <div className="flex flex-col gap-4 sm:col-span-2 empty:hidden">
            <CrossCurrencyAmountField
              id="trade-account-amount"
              label={crossLabel}
              state={cross}
              disabled={isSubmitting}
            />
            <FxRateField
              id="trade-fx-rate"
              securityCurrency={securityCurrency}
              value={fxRate}
              onChange={setFxRate}
              disabled={isSubmitting}
            />
          </div>
        )}

        <div className="flex flex-col gap-3 sm:col-span-2 empty:hidden">
          {kind === "buy" && totalCost > 0 && (
            <div className="bg-background space-y-2 rounded-card border p-3.5">
              <PreviewRow label="Total cost" strong>
                <Money amount={totalCost} currency={securityCurrency} />
              </PreviewRow>
            </div>
          )}

          {kind === "sell" && sell && (
            <div className="bg-background space-y-2 rounded-card border p-3.5" aria-live="polite">
              <PreviewRow label="Proceeds">
                <Money amount={sell.proceeds} currency={securityCurrency} />
              </PreviewRow>
              <PreviewRow label="Cost of shares sold">
                <Money amount={sell.costSold} currency={securityCurrency} />
              </PreviewRow>
              <PreviewRow label="Realised gain" strong>
                <Money
                  amount={sell.realizedGain}
                  currency={securityCurrency}
                  sign="always"
                  className={sell.realizedGain >= 0 ? "text-positive" : "text-negative"}
                />
              </PreviewRow>
              {sell.closesPosition && (
                <p className="text-muted-foreground text-xs">
                  This sells the whole position.
                </p>
              )}
            </div>
          )}

          {kind === "dividend" && net > 0 && (
            <div className="bg-background space-y-2 rounded-card border p-3.5">
              <PreviewRow label="Net dividend" strong>
                <Money amount={net} currency={securityCurrency} />
              </PreviewRow>
            </div>
          )}

          {kind === "split" && ratio > 0 && position > 0 && (
            <div className="bg-background rounded-card border p-3.5 text-sm">
              {units(position)} units become {units(position * ratio)} units.
            </div>
          )}
        </div>

        <div className="flex flex-col gap-2 sm:col-span-2">
          <Label htmlFor="trade-notes">Notes (optional)</Label>
          <Textarea
            id="trade-notes"
            placeholder="Optional notes..."
            rows={2}
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            disabled={isSubmitting}
          />
        </div>
      </div>

      <DialogFooter className="gap-2">
        <Button type="button" variant="ghost" onClick={onDone} disabled={isSubmitting}>
          Cancel
        </Button>
        <Button type="submit" disabled={isSubmitting || !investment}>
          {isSubmitting ? "Recording..." : submitLabel}
        </Button>
      </DialogFooter>
    </form>
  );
}
