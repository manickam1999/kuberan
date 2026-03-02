"use client";

import { useState, useEffect } from "react";
import { Check, ChevronsUpDown } from "lucide-react";
import { toast } from "sonner";
import { ApiClientError } from "@/lib/api-client";
import { toRFC3339 } from "@/lib/format";
import { useAddInvestment } from "@/hooks/use-investments";
import { useSecurities } from "@/hooks/use-securities";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import { CurrencyInput } from "@/components/ui/currency-input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { cn } from "@/lib/utils";
import type { AssetType, Security } from "@/types/models";

const ASSET_TYPE_LABELS: Record<AssetType, string> = {
  stock: "Stock",
  etf: "ETF",
  bond: "Bond",
  crypto: "Crypto",
  reit: "REIT",
  commodity: "Commodity",
};

function getErrorMessage(error: unknown): string {
  if (error instanceof ApiClientError) {
    return error.message;
  }
  return "An unexpected error occurred";
}

interface AddInvestmentDialogProps {
  accountId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function AddInvestmentDialog({
  accountId,
  open,
  onOpenChange,
}: AddInvestmentDialogProps) {
  const [selectedSecurity, setSelectedSecurity] = useState<Security | null>(
    null
  );
  const [securitySearch, setSecuritySearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [comboboxOpen, setComboboxOpen] = useState(false);
  const [quantity, setQuantity] = useState("");
  const [purchasePrice, setPurchasePrice] = useState(0);
  const [walletAddress, setWalletAddress] = useState("");
  const [date, setDate] = useState(new Date().toISOString().split("T")[0]);
  const [fee, setFee] = useState(0);
  const [notes, setNotes] = useState("");
  const [error, setError] = useState("");

  const addInvestment = useAddInvestment();
  const { data: securitiesData, isLoading: securitiesLoading } = useSecurities({
    search: debouncedSearch || undefined,
    page_size: 20,
  });

  const securities = securitiesData?.data ?? [];
  const isSubmitting = addInvestment.isPending;
  const isCrypto = selectedSecurity?.asset_type === "crypto";

  // Debounce search input
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(securitySearch);
    }, 300);
    return () => clearTimeout(timer);
  }, [securitySearch]);

  function resetForm() {
    setSelectedSecurity(null);
    setSecuritySearch("");
    setDebouncedSearch("");
    setComboboxOpen(false);
    setQuantity("");
    setPurchasePrice(0);
    setWalletAddress("");
    setDate(new Date().toISOString().split("T")[0]);
    setFee(0);
    setNotes("");
    setError("");
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      resetForm();
    }
    onOpenChange(nextOpen);
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    if (!selectedSecurity) {
      setError("Please select a security");
      return;
    }

    const qty = parseFloat(quantity);
    if (!qty || qty <= 0) {
      setError("Quantity must be greater than zero");
      return;
    }

    if (purchasePrice <= 0) {
      setError("Purchase price must be greater than zero");
      return;
    }

    addInvestment.mutate(
      {
        account_id: accountId,
        security_id: selectedSecurity.id,
        quantity: qty,
        purchase_price: purchasePrice,
        wallet_address: isCrypto && walletAddress.trim() ? walletAddress.trim() : undefined,
        date: date ? toRFC3339(date) : undefined,
        fee: fee > 0 ? fee : undefined,
        notes: notes.trim() || undefined,
      },
      {
        onSuccess: () => {
          toast.success("Investment added");
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
          <DialogTitle>Add Investment</DialogTitle>
          <DialogDescription>
            Add a new investment holding to this account.
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="bg-destructive/10 text-destructive rounded-md p-3 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          {/* Security selector */}
          <div className="flex flex-col gap-2">
            <Label>Security</Label>
            <Popover open={comboboxOpen} onOpenChange={setComboboxOpen}>
              <PopoverTrigger asChild>
                <Button
                  type="button"
                  variant="outline"
                  role="combobox"
                  aria-expanded={comboboxOpen}
                  className="w-full justify-between font-normal"
                  disabled={isSubmitting}
                >
                  {selectedSecurity ? (
                    <span className="flex items-center gap-2">
                      <span className="font-mono font-semibold">
                        {selectedSecurity.symbol}
                      </span>
                      <span className="text-muted-foreground truncate">
                        {selectedSecurity.name}
                      </span>
                    </span>
                  ) : (
                    <span className="text-muted-foreground">
                      Search securities...
                    </span>
                  )}
                  <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                </Button>
              </PopoverTrigger>
              <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0" align="start">
                <Command shouldFilter={false}>
                  <CommandInput
                    placeholder="Search by symbol or name..."
                    value={securitySearch}
                    onValueChange={setSecuritySearch}
                  />
                  <CommandList>
                    <CommandEmpty>
                      {securitiesLoading
                        ? "Searching..."
                        : "No securities found."}
                    </CommandEmpty>
                    <CommandGroup>
                      {securities.map((sec) => (
                        <CommandItem
                          key={sec.id}
                          value={sec.id}
                          onSelect={() => {
                            setSelectedSecurity(sec);
                            setComboboxOpen(false);
                            setSecuritySearch("");
                          }}
                        >
                          <Check
                            className={cn(
                              "mr-2 h-4 w-4",
                              selectedSecurity?.id === sec.id
                                ? "opacity-100"
                                : "opacity-0"
                            )}
                          />
                          <span className="font-mono font-semibold mr-2">
                            {sec.symbol}
                          </span>
                          <span className="truncate flex-1">{sec.name}</span>
                          <Badge variant="secondary" className="ml-2">
                            {ASSET_TYPE_LABELS[sec.asset_type] ?? sec.asset_type}
                          </Badge>
                          {sec.exchange && (
                            <span className="text-muted-foreground ml-1 text-xs">
                              {sec.exchange}
                            </span>
                          )}
                        </CommandItem>
                      ))}
                    </CommandGroup>
                  </CommandList>
                </Command>
              </PopoverContent>
            </Popover>
          </div>

          {/* Quantity */}
          <div className="flex flex-col gap-2">
            <Label htmlFor="inv-quantity">Quantity</Label>
            <Input
              id="inv-quantity"
              type="number"
              step="any"
              min="0"
              placeholder="0"
              value={quantity}
              onChange={(e) => setQuantity(e.target.value)}
              disabled={isSubmitting}
            />
          </div>

          {/* Purchase Price (per unit) */}
          <div className="flex flex-col gap-2">
            <Label htmlFor="inv-price">Purchase Price (per unit)</Label>
            <CurrencyInput
              id="inv-price"
              value={purchasePrice}
              onChange={setPurchasePrice}
              placeholder="0.00"
              disabled={isSubmitting}
            />
          </div>

          {/* Date */}
          <div className="flex flex-col gap-2">
            <Label htmlFor="inv-date">Date</Label>
            <Input
              id="inv-date"
              type="date"
              value={date}
              onChange={(e) => setDate(e.target.value)}
              disabled={isSubmitting}
            />
          </div>

          {/* Fee (optional) */}
          <div className="flex flex-col gap-2">
            <Label htmlFor="inv-fee">Fee (optional)</Label>
            <CurrencyInput
              id="inv-fee"
              value={fee}
              onChange={setFee}
              placeholder="0.00"
              disabled={isSubmitting}
            />
          </div>

          {/* Notes (optional) */}
          <div className="flex flex-col gap-2">
            <Label htmlFor="inv-notes">Notes (optional)</Label>
            <Textarea
              id="inv-notes"
              placeholder="e.g., Initial purchase"
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              disabled={isSubmitting}
              rows={2}
            />
          </div>

          {/* Wallet Address (crypto only) */}
          {isCrypto && (
            <div className="flex flex-col gap-2">
              <Label htmlFor="inv-wallet">Wallet Address</Label>
              <Input
                id="inv-wallet"
                placeholder="Optional wallet address"
                value={walletAddress}
                onChange={(e) => setWalletAddress(e.target.value)}
                disabled={isSubmitting}
              />
            </div>
          )}

          <DialogFooter>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? "Adding..." : "Add Investment"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
