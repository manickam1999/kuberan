"use client";

import { useState } from "react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import {
  useCreateCashAccount,
  useCreateInvestmentAccount,
  useCreateCreditCardAccount,
  useCreateDebtAccount,
  useCreateFixedDepositAccount,
  useAccounts,
} from "@/hooks/use-accounts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useBaseCurrency } from "@/hooks/use-profile";
import { REPORTING_CURRENCIES } from "@/lib/currencies";
import { CurrencyInput } from "@/components/ui/currency-input";
import { DueDaySelect } from "@/components/accounts/reminder-control";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tile, type TileTone } from "@/components/ui/tile";
import {
  ArrowLeft,
  Banknote,
  Building2,
  CreditCard,
  Landmark,
  Vault,
  type LucideIcon,
} from "lucide-react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { PayoutType, PayoutFrequency, Compounding } from "@/types/models";

type AccountKind =
  | "cash"
  | "credit-card"
  | "investment"
  | "fixed-deposit"
  | "loan";

const KIND_OPTIONS: {
  kind: AccountKind;
  title: string;
  hint: string;
  icon: LucideIcon;
  tone: TileTone;
}[] = [
  {
    kind: "cash",
    title: "Cash",
    hint: "Current or savings account",
    icon: Banknote,
    tone: "accent",
  },
  {
    kind: "credit-card",
    title: "Credit card",
    hint: "Balance is what you owe",
    icon: CreditCard,
    tone: "warn",
  },
  {
    kind: "investment",
    title: "Investment",
    hint: "Brokerage with holdings",
    icon: Landmark,
    tone: "investment",
  },
  {
    kind: "fixed-deposit",
    title: "Fixed deposit",
    hint: "Locked until maturity",
    icon: Vault,
    tone: "info",
  },
  {
    kind: "loan",
    title: "Loan",
    hint: "Mortgage, car or personal loan",
    icon: Building2,
    tone: "neutral",
  },
];

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    return error.message;
  }
  return "An unexpected error occurred";
}

interface CreateAccountDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CreateAccountDialog({
  open,
  onOpenChange,
}: CreateAccountDialogProps) {
  const [kind, setKind] = useState<AccountKind | null>(null);

  // Cash account fields
  const [cashName, setCashName] = useState("");
  const [cashDescription, setCashDescription] = useState("");
  // null = follow the reporting currency until the user picks one.
  const [cashCurrencyPick, setCashCurrency] = useState<string | null>(null);
  const [cashBalance, setCashBalance] = useState(0);

  // Investment account fields
  const [investName, setInvestName] = useState("");
  const [investDescription, setInvestDescription] = useState("");
  // null = follow the reporting currency until the user picks one.
  const [investCurrencyPick, setInvestCurrency] = useState<string | null>(null);
  const [investBroker, setInvestBroker] = useState("");
  const [investAccountNumber, setInvestAccountNumber] = useState("");

  // Credit card account fields
  const [ccName, setCcName] = useState("");
  const [ccDescription, setCcDescription] = useState("");
  // null = follow the reporting currency until the user picks one.
  const [ccCurrencyPick, setCcCurrency] = useState<string | null>(null);
  const [ccCreditLimit, setCcCreditLimit] = useState(0);
  const [ccCurrentBalance, setCcCurrentBalance] = useState(0);
  const [ccInterestRate, setCcInterestRate] = useState("");
  // 0 = no due day (no reminder).
  const [ccDueDay, setCcDueDay] = useState(0);

  // Loan fields
  const [loanName, setLoanName] = useState("");
  const [loanDescription, setLoanDescription] = useState("");
  // null = follow the reporting currency until the user picks one.
  const [loanCurrencyPick, setLoanCurrency] = useState<string | null>(null);
  const [loanOwed, setLoanOwed] = useState(0);
  const [loanInterestRate, setLoanInterestRate] = useState("");
  const [loanPayment, setLoanPayment] = useState(0);
  const [loanDueDay, setLoanDueDay] = useState(0);

  // Fixed deposit account fields
  const [fdName, setFdName] = useState("");
  const [fdDescription, setFdDescription] = useState("");
  const [fdSourceAccountId, setFdSourceAccountId] = useState("");
  const [fdPrincipal, setFdPrincipal] = useState(0);
  const [fdInterestRate, setFdInterestRate] = useState("");
  const [fdStartDate, setFdStartDate] = useState("");
  const [fdMaturityDate, setFdMaturityDate] = useState("");
  const [fdPayoutType, setFdPayoutType] = useState<PayoutType>("cumulative");
  const [fdPayoutFrequency, setFdPayoutFrequency] =
    useState<PayoutFrequency>("monthly");
  const [fdCompounding, setFdCompounding] = useState<Compounding>("annual");
  const [fdLinkedAccountId, setFdLinkedAccountId] = useState("");

  const [error, setError] = useState("");

  const createCash = useCreateCashAccount();
  const createInvestment = useCreateInvestmentAccount();
  const createCreditCard = useCreateCreditCardAccount();
  const createLoan = useCreateDebtAccount();
  const createFD = useCreateFixedDepositAccount();

  const baseCurrency = useBaseCurrency();
  const cashCurrency = cashCurrencyPick ?? baseCurrency;
  const investCurrency = investCurrencyPick ?? baseCurrency;
  const ccCurrency = ccCurrencyPick ?? baseCurrency;
  const loanCurrency = loanCurrencyPick ?? baseCurrency;
  const currencyOptions = (selected: string) =>
    Array.from(
      new Set([
        baseCurrency,
        ...REPORTING_CURRENCIES.map((c) => c.code),
        selected,
      ]),
    );

  const selected = KIND_OPTIONS.find((o) => o.kind === kind);

  const { data: accountsData } = useAccounts({ page_size: 100 });
  const cashAccounts = (accountsData?.data ?? []).filter(
    (a) => a.type === "cash" && a.is_active,
  );

  const isSubmitting =
    createCash.isPending ||
    createInvestment.isPending ||
    createCreditCard.isPending ||
    createLoan.isPending ||
    createFD.isPending;

  function resetForm() {
    setCashName("");
    setCashDescription("");
    setCashCurrency(null);
    setCashBalance(0);
    setInvestName("");
    setInvestDescription("");
    setInvestCurrency(null);
    setInvestBroker("");
    setInvestAccountNumber("");
    setCcName("");
    setCcDescription("");
    setCcCurrency(null);
    setCcCreditLimit(0);
    setCcCurrentBalance(0);
    setCcInterestRate("");
    setCcDueDay(0);
    setLoanName("");
    setLoanDescription("");
    setLoanCurrency(null);
    setLoanOwed(0);
    setLoanInterestRate("");
    setLoanPayment(0);
    setLoanDueDay(0);
    setFdName("");
    setFdDescription("");
    setFdSourceAccountId("");
    setFdPrincipal(0);
    setFdInterestRate("");
    setFdStartDate("");
    setFdMaturityDate("");
    setFdPayoutType("cumulative");
    setFdPayoutFrequency("monthly");
    setFdCompounding("annual");
    setFdLinkedAccountId("");
    setError("");
    setKind(null);
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      resetForm();
    }
    onOpenChange(nextOpen);
  }

  async function handleSubmitCash(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    const name = cashName.trim();
    if (!name) {
      setError("Name is required");
      return;
    }
    if (name.length > 100) {
      setError("Name must be 100 characters or less");
      return;
    }
    if (cashDescription.length > 500) {
      setError("Description must be 500 characters or less");
      return;
    }

    createCash.mutate(
      {
        name,
        description: cashDescription.trim() || undefined,
        currency: cashCurrency,
        initial_balance: cashBalance > 0 ? cashBalance : undefined,
      },
      {
        onSuccess: (account) => {
          toast.success(`Account "${account.name}" created`);
          handleOpenChange(false);
        },
        onError: (err) => setError(getErrorMessage(err)),
      },
    );
  }

  async function handleSubmitInvestment(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    const name = investName.trim();
    if (!name) {
      setError("Name is required");
      return;
    }
    if (name.length > 100) {
      setError("Name must be 100 characters or less");
      return;
    }
    if (investDescription.length > 500) {
      setError("Description must be 500 characters or less");
      return;
    }
    if (investBroker.length > 100) {
      setError("Broker must be 100 characters or less");
      return;
    }
    if (investAccountNumber.length > 50) {
      setError("Account number must be 50 characters or less");
      return;
    }

    createInvestment.mutate(
      {
        name,
        description: investDescription.trim() || undefined,
        currency: investCurrency,
        broker: investBroker.trim() || undefined,
        account_number: investAccountNumber.trim() || undefined,
      },
      {
        onSuccess: (account) => {
          toast.success(`Account "${account.name}" created`);
          handleOpenChange(false);
        },
        onError: (err) => setError(getErrorMessage(err)),
      },
    );
  }

  async function handleSubmitCreditCard(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    const name = ccName.trim();
    if (!name) {
      setError("Name is required");
      return;
    }
    if (name.length > 100) {
      setError("Name must be 100 characters or less");
      return;
    }
    if (ccDescription.length > 500) {
      setError("Description must be 500 characters or less");
      return;
    }

    const interestRate = ccInterestRate ? parseFloat(ccInterestRate) : 0;
    if (isNaN(interestRate) || interestRate < 0 || interestRate > 100) {
      setError("Interest rate must be between 0 and 100");
      return;
    }

    createCreditCard.mutate(
      {
        name,
        description: ccDescription.trim() || undefined,
        currency: ccCurrency,
        credit_limit: ccCreditLimit > 0 ? ccCreditLimit : undefined,
        current_balance: ccCurrentBalance > 0 ? ccCurrentBalance : undefined,
        interest_rate: interestRate > 0 ? interestRate : undefined,
        due_day: ccDueDay > 0 ? ccDueDay : undefined,
      },
      {
        onSuccess: (account) => {
          toast.success(`Account "${account.name}" created`);
          handleOpenChange(false);
        },
        onError: (err) => setError(getErrorMessage(err)),
      },
    );
  }

  async function handleSubmitLoan(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    const name = loanName.trim();
    if (!name) {
      setError("Name is required");
      return;
    }
    if (name.length > 100) {
      setError("Name must be 100 characters or less");
      return;
    }
    if (loanDescription.length > 500) {
      setError("Description must be 500 characters or less");
      return;
    }
    const interestRate = loanInterestRate ? parseFloat(loanInterestRate) : 0;
    if (isNaN(interestRate) || interestRate < 0 || interestRate > 100) {
      setError("Interest rate must be between 0 and 100");
      return;
    }

    createLoan.mutate(
      {
        name,
        description: loanDescription.trim() || undefined,
        currency: loanCurrency,
        amount_owed: loanOwed > 0 ? loanOwed : undefined,
        interest_rate: interestRate > 0 ? interestRate : undefined,
        monthly_payment: loanPayment > 0 ? loanPayment : undefined,
        due_day: loanDueDay > 0 ? loanDueDay : undefined,
      },
      {
        onSuccess: (account) => {
          toast.success(`Loan "${account.name}" created`);
          handleOpenChange(false);
        },
        onError: (err) => setError(getErrorMessage(err)),
      },
    );
  }

  async function handleSubmitFixedDeposit(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    const name = fdName.trim();
    if (!name) {
      setError("Name is required");
      return;
    }
    if (name.length > 100) {
      setError("Name must be 100 characters or less");
      return;
    }
    if (fdDescription.length > 500) {
      setError("Description must be 500 characters or less");
      return;
    }
    if (!fdSourceAccountId) {
      setError("Source bank account is required");
      return;
    }
    if (fdPrincipal <= 0) {
      setError("Principal must be greater than 0");
      return;
    }
    const interestRate = fdInterestRate ? parseFloat(fdInterestRate) : 0;
    if (isNaN(interestRate) || interestRate < 0 || interestRate > 100) {
      setError("Interest rate must be between 0 and 100");
      return;
    }
    if (!fdStartDate) {
      setError("Start date is required");
      return;
    }
    if (!fdMaturityDate) {
      setError("Maturity date is required");
      return;
    }
    if (fdMaturityDate <= fdStartDate) {
      setError("Maturity date must be after start date");
      return;
    }
    if (fdPayoutType === "periodic" && !fdPayoutFrequency) {
      setError("Payout frequency is required for periodic payout type");
      return;
    }

    const currency =
      cashAccounts.find((a) => a.id === fdSourceAccountId)?.currency ??
      baseCurrency;

    createFD.mutate(
      {
        name,
        description: fdDescription.trim() || undefined,
        currency,
        source_account_id: fdSourceAccountId,
        principal: fdPrincipal,
        interest_rate: interestRate > 0 ? interestRate : undefined,
        start_date: fdStartDate,
        maturity_date: fdMaturityDate,
        payout_type: fdPayoutType,
        payout_frequency:
          fdPayoutType === "periodic" ? fdPayoutFrequency : undefined,
        compounding: fdCompounding,
        linked_account_id: fdLinkedAccountId || undefined,
      },
      {
        onSuccess: (account) => {
          toast.success(`Account "${account.name}" created`);
          handleOpenChange(false);
        },
        onError: (err) => setError(getErrorMessage(err)),
      },
    );
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {selected
              ? `New ${selected.title.toLowerCase()} account`
              : "New account"}
          </DialogTitle>
          <DialogDescription>
            {selected
              ? selected.hint
              : "What kind of account is it? You can add more later."}
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div
            role="alert"
            className="bg-destructive/10 text-destructive rounded-md p-3 text-sm"
          >
            {error}
          </div>
        )}

        {!selected && (
          <div
            role="group"
            aria-label="Account type"
            className="grid gap-2 sm:grid-cols-2"
          >
            {KIND_OPTIONS.map((o) => {
              const Icon = o.icon;
              return (
                <button
                  key={o.kind}
                  type="button"
                  onClick={() => {
                    setError("");
                    setKind(o.kind);
                  }}
                  className="hover:border-primary/50 hover:bg-muted/50 focus-visible:ring-ring/50 flex items-center gap-3 rounded-card border p-3 text-left outline-none transition-colors focus-visible:ring-[3px]"
                >
                  <Tile tone={o.tone} size="lg">
                    <Icon />
                  </Tile>
                  <span className="grid gap-0.5">
                    <span className="text-sm font-medium">{o.title}</span>
                    <span className="text-muted-foreground text-xs">
                      {o.hint}
                    </span>
                  </span>
                </button>
              );
            })}
          </div>
        )}

        {selected && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="-mt-2 w-fit"
            onClick={() => {
              setError("");
              setKind(null);
            }}
          >
            <ArrowLeft className="size-4" />
            Account type
          </Button>
        )}

        <>
          {kind === "cash" && (
            <form onSubmit={handleSubmitCash} className="flex flex-col gap-5">
              <div className="flex flex-col gap-2">
                <Label htmlFor="cash-name">Name</Label>
                <Input
                  id="cash-name"
                  placeholder="e.g. Checking Account"
                  value={cashName}
                  onChange={(e) => setCashName(e.target.value)}
                  disabled={isSubmitting}
                  maxLength={100}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="cash-description">Description</Label>
                <Input
                  id="cash-description"
                  placeholder="Optional description"
                  value={cashDescription}
                  onChange={(e) => setCashDescription(e.target.value)}
                  disabled={isSubmitting}
                  maxLength={500}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="cash-currency">Currency</Label>
                <Select
                  value={cashCurrency}
                  onValueChange={setCashCurrency}
                  disabled={isSubmitting}
                >
                  <SelectTrigger id="cash-currency" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {currencyOptions(cashCurrency).map((c) => (
                      <SelectItem key={c} value={c}>
                        {c}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="cash-balance">
                  Opening balance
                  <span className="ml-2 text-xs font-normal text-muted-foreground">
                    (not counted as income)
                  </span>
                </Label>
                <CurrencyInput
                  id="cash-balance"
                  value={cashBalance}
                  onChange={setCashBalance}
                  currency={cashCurrency}
                  placeholder="0.00"
                  disabled={isSubmitting}
                />
              </div>
              <DialogFooter>
                <Button type="submit" disabled={isSubmitting}>
                  {isSubmitting ? "Creating..." : "Create Account"}
                </Button>
              </DialogFooter>
            </form>
          )}

          {kind === "investment" && (
            <form
              onSubmit={handleSubmitInvestment}
              className="flex flex-col gap-5"
            >
              <div className="flex flex-col gap-2">
                <Label htmlFor="invest-name">Name</Label>
                <Input
                  id="invest-name"
                  placeholder="e.g. Brokerage Account"
                  value={investName}
                  onChange={(e) => setInvestName(e.target.value)}
                  disabled={isSubmitting}
                  maxLength={100}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="invest-description">Description</Label>
                <Input
                  id="invest-description"
                  placeholder="Optional description"
                  value={investDescription}
                  onChange={(e) => setInvestDescription(e.target.value)}
                  disabled={isSubmitting}
                  maxLength={500}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="invest-currency">Currency</Label>
                <Select
                  value={investCurrency}
                  onValueChange={setInvestCurrency}
                  disabled={isSubmitting}
                >
                  <SelectTrigger id="invest-currency" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {currencyOptions(investCurrency).map((c) => (
                      <SelectItem key={c} value={c}>
                        {c}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="invest-broker">Broker</Label>
                <Input
                  id="invest-broker"
                  placeholder="e.g. Fidelity"
                  value={investBroker}
                  onChange={(e) => setInvestBroker(e.target.value)}
                  disabled={isSubmitting}
                  maxLength={100}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="invest-account-number">Account Number</Label>
                <Input
                  id="invest-account-number"
                  placeholder="Optional account number"
                  value={investAccountNumber}
                  onChange={(e) => setInvestAccountNumber(e.target.value)}
                  disabled={isSubmitting}
                  maxLength={50}
                />
              </div>
              <DialogFooter>
                <Button type="submit" disabled={isSubmitting}>
                  {isSubmitting ? "Creating..." : "Create Account"}
                </Button>
              </DialogFooter>
            </form>
          )}

          {kind === "credit-card" && (
            <form
              onSubmit={handleSubmitCreditCard}
              className="flex flex-col gap-5"
            >
              <div className="flex flex-col gap-2">
                <Label htmlFor="cc-name">Name</Label>
                <Input
                  id="cc-name"
                  placeholder="e.g. Visa Signature"
                  value={ccName}
                  onChange={(e) => setCcName(e.target.value)}
                  disabled={isSubmitting}
                  maxLength={100}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="cc-description">Description</Label>
                <Input
                  id="cc-description"
                  placeholder="Optional description"
                  value={ccDescription}
                  onChange={(e) => setCcDescription(e.target.value)}
                  disabled={isSubmitting}
                  maxLength={500}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="cc-currency">Currency</Label>
                <Select
                  value={ccCurrency}
                  onValueChange={setCcCurrency}
                  disabled={isSubmitting}
                >
                  <SelectTrigger id="cc-currency" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {currencyOptions(ccCurrency).map((c) => (
                      <SelectItem key={c} value={c}>
                        {c}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="cc-credit-limit">Credit Limit</Label>
                <CurrencyInput
                  id="cc-credit-limit"
                  value={ccCreditLimit}
                  onChange={setCcCreditLimit}
                  currency={ccCurrency}
                  placeholder="0.00"
                  disabled={isSubmitting}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="cc-current-balance">
                  Current balance owed
                  <span className="ml-2 text-xs font-normal text-muted-foreground">
                    (optional)
                  </span>
                </Label>
                <CurrencyInput
                  id="cc-current-balance"
                  value={ccCurrentBalance}
                  onChange={setCcCurrentBalance}
                  currency={ccCurrency}
                  placeholder="0.00"
                  disabled={isSubmitting}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="cc-interest-rate">Interest Rate (%)</Label>
                <Input
                  id="cc-interest-rate"
                  type="number"
                  placeholder="e.g. 24.99"
                  value={ccInterestRate}
                  onChange={(e) => setCcInterestRate(e.target.value)}
                  disabled={isSubmitting}
                  min={0}
                  max={100}
                  step={0.01}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="cc-due-day">
                  Payment due day
                  <span className="ml-2 text-xs font-normal text-muted-foreground">
                    (we&apos;ll remind you on Telegram)
                  </span>
                </Label>
                <DueDaySelect
                  id="cc-due-day"
                  value={ccDueDay}
                  onChange={setCcDueDay}
                  disabled={isSubmitting}
                  allowNone
                />
              </div>
              <DialogFooter>
                <Button type="submit" disabled={isSubmitting}>
                  {isSubmitting ? "Creating..." : "Create Account"}
                </Button>
              </DialogFooter>
            </form>
          )}

          {kind === "loan" && (
            <form onSubmit={handleSubmitLoan} className="flex flex-col gap-5">
              <div className="flex flex-col gap-2">
                <Label htmlFor="loan-name">Name</Label>
                <Input
                  id="loan-name"
                  placeholder="e.g. Car loan"
                  value={loanName}
                  onChange={(e) => setLoanName(e.target.value)}
                  disabled={isSubmitting}
                  maxLength={100}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="loan-description">Description</Label>
                <Input
                  id="loan-description"
                  placeholder="Optional description"
                  value={loanDescription}
                  onChange={(e) => setLoanDescription(e.target.value)}
                  disabled={isSubmitting}
                  maxLength={500}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="loan-currency">Currency</Label>
                <Select
                  value={loanCurrency}
                  onValueChange={setLoanCurrency}
                  disabled={isSubmitting}
                >
                  <SelectTrigger id="loan-currency" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {currencyOptions(loanCurrency).map((c) => (
                      <SelectItem key={c} value={c}>
                        {c}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="loan-owed">
                  Amount owed
                  <span className="ml-2 text-xs font-normal text-muted-foreground">
                    (what&apos;s left to pay)
                  </span>
                </Label>
                <CurrencyInput
                  id="loan-owed"
                  value={loanOwed}
                  onChange={setLoanOwed}
                  currency={loanCurrency}
                  placeholder="0.00"
                  disabled={isSubmitting}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="loan-interest-rate">Interest rate (% a year)</Label>
                <Input
                  id="loan-interest-rate"
                  type="number"
                  placeholder="e.g. 3.5"
                  value={loanInterestRate}
                  onChange={(e) => setLoanInterestRate(e.target.value)}
                  disabled={isSubmitting}
                  min={0}
                  max={100}
                  step={0.01}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="loan-payment">Monthly payment</Label>
                <CurrencyInput
                  id="loan-payment"
                  value={loanPayment}
                  onChange={setLoanPayment}
                  currency={loanCurrency}
                  placeholder="0.00"
                  disabled={isSubmitting}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="loan-due-day">
                  Payment due day
                  <span className="ml-2 text-xs font-normal text-muted-foreground">
                    (we&apos;ll remind you on Telegram)
                  </span>
                </Label>
                <DueDaySelect
                  id="loan-due-day"
                  value={loanDueDay}
                  onChange={setLoanDueDay}
                  disabled={isSubmitting}
                  allowNone
                />
              </div>
              <DialogFooter>
                <Button type="submit" disabled={isSubmitting}>
                  {isSubmitting ? "Creating..." : "Create Loan"}
                </Button>
              </DialogFooter>
            </form>
          )}

          {kind === "fixed-deposit" && (
            <form
              onSubmit={handleSubmitFixedDeposit}
              className="flex flex-col gap-5"
            >
              <div className="flex flex-col gap-2">
                <Label htmlFor="fd-name">Name</Label>
                <Input
                  id="fd-name"
                  placeholder="e.g. Maybank 12-month FD"
                  value={fdName}
                  onChange={(e) => setFdName(e.target.value)}
                  disabled={isSubmitting}
                  maxLength={100}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="fd-description">Description</Label>
                <Input
                  id="fd-description"
                  placeholder="Optional description"
                  value={fdDescription}
                  onChange={(e) => setFdDescription(e.target.value)}
                  disabled={isSubmitting}
                  maxLength={500}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="fd-source-account">Source Bank Account</Label>
                <Select
                  value={fdSourceAccountId}
                  onValueChange={setFdSourceAccountId}
                  disabled={isSubmitting}
                >
                  <SelectTrigger id="fd-source-account" className="w-full">
                    <SelectValue placeholder="Select account..." />
                  </SelectTrigger>
                  <SelectContent>
                    {cashAccounts.length === 0 ? (
                      <SelectItem value="__none__" disabled>
                        No active cash accounts
                      </SelectItem>
                    ) : (
                      cashAccounts.map((a) => (
                        <SelectItem key={a.id} value={a.id}>
                          {a.name} ({a.currency})
                        </SelectItem>
                      ))
                    )}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="fd-principal">Principal</Label>
                <CurrencyInput
                  id="fd-principal"
                  value={fdPrincipal}
                  onChange={setFdPrincipal}
                  currency={
                    cashAccounts.find((a) => a.id === fdSourceAccountId)
                      ?.currency ?? baseCurrency
                  }
                  placeholder="0.00"
                  disabled={isSubmitting}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="fd-interest-rate">Interest Rate (%)</Label>
                <Input
                  id="fd-interest-rate"
                  type="number"
                  placeholder="e.g. 3.5"
                  value={fdInterestRate}
                  onChange={(e) => setFdInterestRate(e.target.value)}
                  disabled={isSubmitting}
                  min={0}
                  max={100}
                  step={0.01}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="fd-start-date">Start Date</Label>
                <Input
                  id="fd-start-date"
                  type="date"
                  value={fdStartDate}
                  onChange={(e) => setFdStartDate(e.target.value)}
                  disabled={isSubmitting}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="fd-maturity-date">Maturity Date</Label>
                <Input
                  id="fd-maturity-date"
                  type="date"
                  value={fdMaturityDate}
                  onChange={(e) => setFdMaturityDate(e.target.value)}
                  disabled={isSubmitting}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="fd-payout-type">Payout Type</Label>
                <Select
                  value={fdPayoutType}
                  onValueChange={(v) => setFdPayoutType(v as PayoutType)}
                  disabled={isSubmitting}
                >
                  <SelectTrigger id="fd-payout-type" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="cumulative">
                      Cumulative (lump at maturity)
                    </SelectItem>
                    <SelectItem value="periodic">
                      Periodic (interest payouts)
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              {fdPayoutType === "periodic" && (
                <div className="flex flex-col gap-2">
                  <Label htmlFor="fd-payout-frequency">Payout Frequency</Label>
                  <Select
                    value={fdPayoutFrequency}
                    onValueChange={(v) =>
                      setFdPayoutFrequency(v as PayoutFrequency)
                    }
                    disabled={isSubmitting}
                  >
                    <SelectTrigger id="fd-payout-frequency" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="monthly">Monthly</SelectItem>
                      <SelectItem value="quarterly">Quarterly</SelectItem>
                      <SelectItem value="semiannual">Semi-annual</SelectItem>
                      <SelectItem value="annual">Annual</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              )}
              <div className="flex flex-col gap-2">
                <Label htmlFor="fd-compounding">Compounding</Label>
                <Select
                  value={fdCompounding}
                  onValueChange={(v) => setFdCompounding(v as Compounding)}
                  disabled={isSubmitting}
                >
                  <SelectTrigger id="fd-compounding" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="monthly">Monthly</SelectItem>
                    <SelectItem value="quarterly">Quarterly</SelectItem>
                    <SelectItem value="semiannual">Semi-annual</SelectItem>
                    <SelectItem value="annual">Annual</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              {fdPayoutType === "periodic" && (
                <div className="flex flex-col gap-2">
                  <Label htmlFor="fd-linked-account">
                    Linked Payout Account
                    <span className="ml-2 text-xs font-normal text-muted-foreground">
                      (optional)
                    </span>
                  </Label>
                  <Select
                    value={fdLinkedAccountId || "none"}
                    onValueChange={(v) =>
                      setFdLinkedAccountId(v === "none" ? "" : v)
                    }
                    disabled={isSubmitting}
                  >
                    <SelectTrigger id="fd-linked-account" className="w-full">
                      <SelectValue placeholder="None" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">None</SelectItem>
                      {cashAccounts.map((a) => (
                        <SelectItem key={a.id} value={a.id}>
                          {a.name} ({a.currency})
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              )}
              <DialogFooter>
                <Button type="submit" disabled={isSubmitting}>
                  {isSubmitting ? "Creating..." : "Create Account"}
                </Button>
              </DialogFooter>
            </form>
          )}
        </>
      </DialogContent>
    </Dialog>
  );
}
