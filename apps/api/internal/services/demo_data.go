package services

import (
	"math"
	"math/rand"
	"time"

	"kuberan/internal/models"
)

// demoSeed makes the sample household deterministic: the same day always
// produces the same transactions.
const demoSeed = 20260101

// demoWindowDays is how much history the sample household covers.
const demoWindowDays = 90

// Keys of the sample accounts.
const (
	demoAcctDBS  = "dbs"
	demoAcctHYS  = "hys"
	demoAcctFD   = "fd"
	demoAcctIBKR = "ibkr"
	demoAcctAmex = "amex"
	demoAcctFX   = "fx"
)

// demoCategorySpec describes a sample category.
type demoCategorySpec struct {
	Name  string
	Type  models.CategoryType
	Icon  string
	Color string
}

// demoCategories are the sample categories. A category the user already has
// (same name and type) is reused and never removed with the demo.
var demoCategories = []demoCategorySpec{
	{"Salary", models.CategoryTypeIncome, "briefcase", "#22c55e"},
	{"Freelance", models.CategoryTypeIncome, "laptop", "#14b8a6"},
	{"Interest", models.CategoryTypeIncome, "coins", "#eab308"},
	{"Rent", models.CategoryTypeExpense, "house", "#f97316"},
	{"Shopping", models.CategoryTypeExpense, "shopping-bag", "#ec4899"},
	{"Groceries", models.CategoryTypeExpense, "shopping-cart", "#84cc16"},
	{"Utilities", models.CategoryTypeExpense, "zap", "#06b6d4"},
	{"Dining", models.CategoryTypeExpense, "utensils", "#ef4444"},
	{"Transport", models.CategoryTypeExpense, "car", "#3b82f6"},
	{"Subscriptions", models.CategoryTypeExpense, "tv", "#8b5cf6"},
	{"Travel", models.CategoryTypeExpense, "plane", "#a855f7"},
	{"Entertainment", models.CategoryTypeExpense, "film", "#f59e0b"},
}

// demoTxn is one sample income, expense, reimbursement or transfer.
type demoTxn struct {
	daysAgo  int
	acct     string
	toAcct   string // transfers only
	typ      models.TransactionType
	amount   int64
	desc     string
	category string
	source   models.TransactionSource
	notes    string
	key      string // lets the receivables and settlements find their expense
}

// demoHolding is one sample position.
type demoHolding struct {
	symbol string
	name   string
	asset  models.AssetType
	qty    float64
	px     float64 // latest price, major units
	cost   float64 // average cost per unit, major units
}

// demoHoldings mirror the prototype's brokerage.
var demoHoldings = []demoHolding{
	{"VWRA", "Vanguard FTSE All-World", models.AssetTypeETF, 210, 137.97, 118},
	{"BTC", "Bitcoin", models.AssetTypeCrypto, 0.18, 95122.84, 61000},
	{"ES3", "SPDR STI ETF", models.AssetTypeETF, 3000, 3.97, 3.40},
	{"NVDA", "NVIDIA Corp.", models.AssetTypeStock, 60, 181.11, 98},
	{"AAPL", "Apple Inc.", models.AssetTypeStock, 40, 227.89, 175},
}

// demoBudgets are the monthly sample budgets (category, amount in major units).
var demoBudgets = []struct {
	category string
	amount   float64
}{
	{"Shopping", 400}, {"Groceries", 800}, {"Transport", 200}, {"Dining", 500},
}

// cents converts a major-unit amount to cents.
func cents(v float64) int64 { return int64(math.Round(v * 100)) }

// demoTransactions builds the sample ledger for the window ending today. It is
// pure: the same today and seed give the same rows. Dates run oldest first.
func demoTransactions(today time.Time, withFX bool) []demoTxn {
	rng := rand.New(rand.NewSource(demoSeed))
	pick := func(xs []string) string { return xs[rng.Intn(len(xs))] }
	between := func(lo, hi float64) int64 { return cents(lo + rng.Float64()*(hi-lo)) }

	groceries := []string{"Whole Foods", "Trader Joe's", "Cold Storage", "FairPrice"}
	dining := []string{"Din Tai Fung", "Ramen Nagi", "Blue Bottle", "Toast Box"}
	transport := []string{"Shell", "Grab", "SBS Transit"}
	shopping := []string{"Amazon", "Uniqlo", "Apple Store", "IKEA"}

	var out []demoTxn
	add := func(t demoTxn) {
		if t.source == "" {
			t.source = models.TransactionSourceWeb
		}
		out = append(out, t)
	}
	for d := demoWindowDays; d >= 0; d-- {
		day := today.AddDate(0, 0, -d)
		dom := day.Day()
		switch dom {
		case 1:
			add(demoTxn{daysAgo: d, acct: demoAcctDBS, typ: models.TransactionTypeIncome, amount: cents(7800), desc: "Acme Corp payroll", category: "Salary"})
		case 2:
			add(demoTxn{daysAgo: d, acct: demoAcctDBS, typ: models.TransactionTypeExpense, amount: cents(3200), desc: "Rent - Tiong Bahru", category: "Rent"})
			add(demoTxn{daysAgo: d, acct: demoAcctAmex, typ: models.TransactionTypeExpense, amount: cents(10.56), desc: "iCloud+", category: "Subscriptions"})
		case 6:
			add(demoTxn{daysAgo: d, acct: demoAcctAmex, typ: models.TransactionTypeExpense, amount: cents(27.90), desc: "Spotify", category: "Subscriptions"})
		case 7:
			add(demoTxn{daysAgo: d, acct: demoAcctAmex, typ: models.TransactionTypeExpense, amount: between(85, 112), desc: "SP Group", category: "Utilities"})
		case 13:
			add(demoTxn{daysAgo: d, acct: demoAcctDBS, typ: models.TransactionTypeExpense, amount: cents(115.88), desc: "Singtel Fibre", category: "Utilities"})
		case 15:
			add(demoTxn{daysAgo: d, acct: demoAcctDBS, typ: models.TransactionTypeIncome, amount: between(450, 900), desc: "Design retainer", category: "Freelance", notes: "Studio Kopi"})
		case 21:
			add(demoTxn{daysAgo: d, acct: demoAcctDBS, toAcct: demoAcctHYS, typ: models.TransactionTypeTransfer, amount: cents(1500), desc: "Monthly savings"})
		case 25:
			add(demoTxn{daysAgo: d, acct: demoAcctDBS, toAcct: demoAcctAmex, typ: models.TransactionTypeTransfer, amount: cents(1500), desc: "Amex payment"})
		case 28:
			add(demoTxn{daysAgo: d, acct: demoAcctHYS, typ: models.TransactionTypeIncome, amount: between(33, 38), desc: "Savings interest", category: "Interest"})
		}
		// Everyday spending: a few random purchases most days.
		if rng.Float64() < 0.20 {
			acct := demoAcctDBS
			if rng.Intn(2) == 0 {
				acct = demoAcctAmex
			}
			add(demoTxn{daysAgo: d, acct: acct, typ: models.TransactionTypeExpense, amount: between(30, 150), desc: pick(groceries), category: "Groceries"})
		}
		if rng.Float64() < 0.24 {
			acct := demoAcctDBS
			if rng.Intn(2) == 0 {
				acct = demoAcctAmex
			}
			add(demoTxn{daysAgo: d, acct: acct, typ: models.TransactionTypeExpense, amount: between(12, 70), desc: pick(dining), category: "Dining"})
		}
		if rng.Float64() < 0.16 {
			acct := demoAcctAmex
			if rng.Intn(3) == 0 {
				acct = demoAcctDBS
			}
			add(demoTxn{daysAgo: d, acct: acct, typ: models.TransactionTypeExpense, amount: between(15, 45), desc: pick(transport), category: "Transport"})
		}
		if rng.Float64() < 0.09 {
			add(demoTxn{daysAgo: d, acct: demoAcctAmex, typ: models.TransactionTypeExpense, amount: between(50, 300), desc: pick(shopping), category: "Shopping"})
		}
		if withFX && rng.Float64() < 0.05 {
			add(demoTxn{daysAgo: d, acct: demoAcctFX, typ: models.TransactionTypeExpense, amount: between(8, 60), desc: "Online order", category: "Shopping"})
		}
	}

	// Hand-placed rows that the screens call out.
	add(demoTxn{daysAgo: 60, acct: demoAcctAmex, typ: models.TransactionTypeExpense, amount: cents(480), desc: "AirAsia flights", category: "Travel"})
	add(demoTxn{daysAgo: 25, acct: demoAcctAmex, typ: models.TransactionTypeExpense, amount: cents(240), desc: "Coldplay tickets", category: "Entertainment", key: "coldplay"})
	add(demoTxn{daysAgo: 12, acct: demoAcctDBS, typ: models.TransactionTypeExpense, amount: cents(400), desc: "Lent to Arjun", category: "", notes: "Laptop repair", key: "arjun"})
	add(demoTxn{daysAgo: 9, acct: demoAcctDBS, typ: models.TransactionTypeExpense, amount: cents(64.73), desc: "Trader Joe's", category: "Groceries", key: "traderjoes"})
	add(demoTxn{daysAgo: 7, acct: demoAcctAmex, typ: models.TransactionTypeExpense, amount: cents(51.66), desc: "Blue Bottle", category: "Dining", source: models.TransactionSourceTelegram})
	add(demoTxn{daysAgo: 3, acct: demoAcctAmex, typ: models.TransactionTypeExpense, amount: cents(18.71), desc: "Blue Bottle", category: "Dining", source: models.TransactionSourceMCP})
	add(demoTxn{daysAgo: 5, acct: demoAcctDBS, typ: models.TransactionTypeReimbursement, amount: cents(60), desc: "Reimbursement: Priya", key: "priya-paid"})
	return out
}

// demoPrices returns the weekly closing prices of a holding, newest first, in
// cents. The first entry is the holding's latest price.
func demoPrices(h demoHolding, idx, weeks int) []int64 {
	out := make([]int64, weeks)
	for k := 0; k < weeks; k++ {
		factor := 1.0
		if k > 0 {
			factor = 1 - 0.011*float64(k) + 0.02*math.Sin(float64(k)*1.3+float64(idx))
		}
		out[k] = cents(h.px * factor)
		if out[k] < 1 {
			out[k] = 1
		}
	}
	return out
}
