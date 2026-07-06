"use client";

import { useState } from "react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import {
  useCreateCashAccount,
  useCreateInvestmentAccount,
  useCreateCreditCardAccount,
} from "@/hooks/use-accounts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CurrencyInput } from "@/components/ui/currency-input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

const CURRENCIES = ["MYR", "USD", "EUR", "GBP", "CAD", "AUD", "JPY", "INR"];

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
  const [tab, setTab] = useState<string>("cash");

  // Cash account fields
  const [cashName, setCashName] = useState("");
  const [cashDescription, setCashDescription] = useState("");
  const [cashCurrency, setCashCurrency] = useState("MYR");
  const [cashBalance, setCashBalance] = useState(0);

  // Investment account fields
  const [investName, setInvestName] = useState("");
  const [investDescription, setInvestDescription] = useState("");
  const [investCurrency, setInvestCurrency] = useState("MYR");
  const [investBroker, setInvestBroker] = useState("");
  const [investAccountNumber, setInvestAccountNumber] = useState("");

  // Credit card account fields
  const [ccName, setCcName] = useState("");
  const [ccDescription, setCcDescription] = useState("");
  const [ccCurrency, setCcCurrency] = useState("MYR");
  const [ccCreditLimit, setCcCreditLimit] = useState(0);
  const [ccInterestRate, setCcInterestRate] = useState("");
  const [ccDueDate, setCcDueDate] = useState("");

  const [error, setError] = useState("");

  const createCash = useCreateCashAccount();
  const createInvestment = useCreateInvestmentAccount();
  const createCreditCard = useCreateCreditCardAccount();

  const isSubmitting =
    createCash.isPending ||
    createInvestment.isPending ||
    createCreditCard.isPending;

  function resetForm() {
    setCashName("");
    setCashDescription("");
    setCashCurrency("MYR");
    setCashBalance(0);
    setInvestName("");
    setInvestDescription("");
    setInvestCurrency("MYR");
    setInvestBroker("");
    setInvestAccountNumber("");
    setCcName("");
    setCcDescription("");
    setCcCurrency("MYR");
    setCcCreditLimit(0);
    setCcInterestRate("");
    setCcDueDate("");
    setError("");
    setTab("cash");
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
      }
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
      }
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
        interest_rate: interestRate > 0 ? interestRate : undefined,
        due_date: ccDueDate || undefined,
      },
      {
        onSuccess: (account) => {
          toast.success(`Account "${account.name}" created`);
          handleOpenChange(false);
        },
        onError: (err) => setError(getErrorMessage(err)),
      }
    );
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Create Account</DialogTitle>
          <DialogDescription>
            Add a new cash, investment, or credit card account.
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            {error}
          </div>
        )}

        <Tabs value={tab} onValueChange={setTab}>
          <TabsList className="w-full">
            <TabsTrigger value="cash" className="flex-1">
              Cash
            </TabsTrigger>
            <TabsTrigger value="investment" className="flex-1">
              Investment
            </TabsTrigger>
            <TabsTrigger value="credit-card" className="flex-1">
              Credit Card
            </TabsTrigger>
          </TabsList>

          <TabsContent value="cash">
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
                    {CURRENCIES.map((c) => (
                      <SelectItem key={c} value={c}>
                        {c}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="cash-balance">Initial Balance</Label>
                <CurrencyInput
                  id="cash-balance"
                  value={cashBalance}
                  onChange={setCashBalance}
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
          </TabsContent>

          <TabsContent value="investment">
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
                    {CURRENCIES.map((c) => (
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
          </TabsContent>

          <TabsContent value="credit-card">
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
                    {CURRENCIES.map((c) => (
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
                <Label htmlFor="cc-due-date">Due Date</Label>
                <Input
                  id="cc-due-date"
                  type="date"
                  value={ccDueDate}
                  onChange={(e) => setCcDueDate(e.target.value)}
                  disabled={isSubmitting}
                />
              </div>
              <DialogFooter>
                <Button type="submit" disabled={isSubmitting}>
                  {isSubmitting ? "Creating..." : "Create Account"}
                </Button>
              </DialogFooter>
            </form>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}
