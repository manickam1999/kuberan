package ibkr

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const stmtHead = "Statement,Header,Field Name,Field Value\n" +
	"Statement,Data,BrokerName,Interactive Brokers LLC\n" +
	"Statement,Data,Period,\"%PERIOD%\"\n" +
	"Account Information,Header,Field Name,Field Value\n" +
	"Account Information,Data,Name,Test User\n" +
	"Account Information,Data,Account,%ACCT%\n" +
	"Account Information,Data,Base Currency,%CCY%\n"

func doc(period, acct, ccy, body string) string {
	r := strings.NewReplacer("%PERIOD%", period, "%ACCT%", acct, "%CCY%", ccy)
	return r.Replace(stmtHead) + body
}

func parseDoc(t *testing.T, body string) *Statement {
	t.Helper()
	st, err := Parse(strings.NewReader(doc("January 1, 2025 - December 31, 2025", "U00000000", "USD", body)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return st
}

func buildDoc(t *testing.T, body string) *EventSet {
	t.Helper()
	set, err := BuildEvents([]*Statement{parseDoc(t, body)})
	if err != nil {
		t.Fatalf("BuildEvents: %v", err)
	}
	return set
}

func TestParseMetadata(t *testing.T) {
	st := parseDoc(t, "")
	if st.AccountID != "U00000000" || st.BaseCurrency != "USD" {
		t.Errorf("metadata: %q %q", st.AccountID, st.BaseCurrency)
	}
	if st.PeriodStart.Format("2006-01-02") != "2025-01-01" || st.PeriodEnd.Format("2006-01-02") != "2025-12-31" {
		t.Errorf("period: %v - %v", st.PeriodStart, st.PeriodEnd)
	}
}

func TestParseBOMAndCRLF(t *testing.T) {
	in := "\ufeff" + strings.ReplaceAll(doc("March 5, 2025", "U1", "USD", ""), "\n", "\r\n")
	st, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if st.AccountID != "U1" || !st.PeriodStart.Equal(st.PeriodEnd) {
		t.Errorf("single-day period: %+v", st)
	}
}

func TestParseMissingMetadata(t *testing.T) {
	for name, in := range map[string]string{
		"no account":  "Statement,Data,Period,\"January 1, 2025 - January 2, 2025\"\n",
		"no period":   "Account Information,Data,Account,U1\nAccount Information,Data,Base Currency,USD\n",
		"bad period":  doc("someday", "U1", "USD", ""),
		"no currency": "Account Information,Data,Account,U1\n",
	} {
		if _, err := Parse(strings.NewReader(in)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

// Header re-keying: the second Trades header lists the columns in another order.
func TestHeaderRekeyAndNameMapping(t *testing.T) {
	body := "Trades,Header,DataDiscriminator,Asset Category,Currency,Symbol,Date/Time,Quantity,T. Price,C. Price,Proceeds,Comm/Fee,Basis,Realized P/L,MTM P/L,Code\n" +
		"Trades,Data,Order,Stocks,USD,AAA,\"2025-01-02, 10:00:00\",10,5,5,-50,-1,51,0,0,O\n" +
		"Trades,SubTotal,,Stocks,USD,AAA,,10,,,-50,-1,51,0,0,\n" +
		"Trades,Header,Symbol,DataDiscriminator,Asset Category,Currency,Quantity,Date/Time,Comm/Fee,Proceeds,T. Price\n" +
		"Trades,Data,BBB,Order,Stocks,USD,-2,\"2025-01-03, 10:00:00\",-0.5,30,15\n" +
		"Trades,Total,,Stocks,USD,,,,,,\n"
	set := buildDoc(t, body)
	if len(set.Events) != 2 {
		t.Fatalf("events = %d, want 2", len(set.Events))
	}
	a, b := set.Events[0], set.Events[1]
	if a.Symbol != "AAA" || a.Kind != KindBuy || a.Quantity != 10 || a.PricePerUnit != 500 || a.Fee != 100 || a.TotalAmount != 5100 {
		t.Errorf("AAA: %+v", a)
	}
	if b.Symbol != "BBB" || b.Kind != KindSell || b.Quantity != 2 || b.PricePerUnit != 1500 || b.Fee != 50 || b.TotalAmount != 2950 {
		t.Errorf("BBB: %+v", b)
	}
}

func TestTradeTimeNewYork(t *testing.T) {
	body := "Trades,Header,DataDiscriminator,Asset Category,Currency,Symbol,Date/Time,Quantity,T. Price,Proceeds,Comm/Fee\n" +
		"Trades,Data,Order,Stocks,USD,AAA,\"2025-01-02, 10:00:00\",1,5,-5,-1\n" +
		"Trades,Data,Order,Stocks,USD,AAA,\"2025-07-02, 10:00:00\",1,5,-5,-1\n"
	set := buildDoc(t, body)
	if got := set.Events[0].Time.Format(time.RFC3339); got != "2025-01-02T15:00:00Z" {
		t.Errorf("winter time = %s", got)
	}
	if got := set.Events[1].Time.Format(time.RFC3339); got != "2025-07-02T14:00:00Z" {
		t.Errorf("summer time = %s", got)
	}
}

func TestTradeLotRowsIgnored(t *testing.T) {
	body := "Trades,Header,DataDiscriminator,Asset Category,Currency,Symbol,Date/Time,Quantity,T. Price,Proceeds,Comm/Fee\n" +
		"Trades,Data,Order,Stocks,USD,AAA,\"2025-01-02, 10:00:00\",1,5,-5,-1\n" +
		"Trades,Data,ClosedLot,Stocks,USD,AAA,\"2025-01-02, 10:00:00\",1,5,-5,-1\n"
	if n := len(buildDoc(t, body).Events); n != 1 {
		t.Errorf("events = %d, want 1", n)
	}
}

func TestExactRounding(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"0.005", 1}, {"-0.005", -1}, {"0.004999999999", 0}, {"2.675", 268}, {"-2.675", -268},
		{"1007.76", 100776}, {"0.1", 10}, {"0.285", 29}, {"1.0049999999999999999", 100}, {"0", 0}, {"123456789.125", 12345678913},
	}
	for _, c := range cases {
		r, err := parseRat(c.in)
		if err != nil {
			t.Fatal(err)
		}
		got, err := cents(r)
		if err != nil || got != c.want {
			t.Errorf("cents(%s) = %d, %v; want %d", c.in, got, err, c.want)
		}
	}
	if _, err := parseRat("abc"); err == nil {
		t.Error("expected invalid number error")
	}
	if q, _ := parseQty("0.12345678905"); q != 0.1234567891 {
		t.Errorf("parseQty rounding = %v", q)
	}
}

func TestTradeTotalsSingleRounding(t *testing.T) {
	// buy: (|-1498.8126| + 0.35165795) * 100 = 149916.42... -> 149916 (single rounding)
	// sell: (1007.76 - 0.264952395) * 100 = 100749.5047605 -> 100750
	body := "Trades,Header,DataDiscriminator,Asset Category,Currency,Symbol,Date/Time,Quantity,T. Price,Proceeds,Comm/Fee\n" +
		"Trades,Data,Order,Stocks,USD,AAA,\"2025-01-02, 10:00:00\",6.9,217.219217391,-1498.8126,-0.35165795\n" +
		"Trades,Data,Order,Stocks,USD,AAA,\"2025-01-03, 10:00:00\",-102,9.88,1007.76,-0.264952395\n"
	ev := buildDoc(t, body).Events
	if ev[0].TotalAmount != 149916 || ev[0].Fee != 35 || ev[0].PricePerUnit != 21722 {
		t.Errorf("buy: %+v", ev[0])
	}
	if ev[1].TotalAmount != 100750 || ev[1].Fee != 26 || ev[1].PricePerUnit != 988 {
		t.Errorf("sell: %+v", ev[1])
	}
}

const divHeader = "Dividends,Header,Currency,Date,Description,Amount\n"
const whtHeader = "Withholding Tax,Header,Currency,Date,Description,Amount,Code\n"
const feeHeader = "Fees,Header,Subtitle,Currency,Date,Description,Amount\n"

func TestDividendReversalNetting(t *testing.T) {
	body := divHeader +
		"Dividends,Data,USD,2025-06-12,IBKR(US45841N1072) Payment in Lieu of Dividend (Ordinary Dividend),0.09\n" +
		"Dividends,Data,USD,2025-06-12,IBKR(US45841N1072) Cash Dividend USD 0.0875 per Share (Ordinary Dividend),0.08\n" +
		"Dividends,Data,USD,2025-06-12,IBKR(US45841N1072) Payment in Lieu of Dividend - Reversal (Ordinary Dividend),-0.09\n" +
		"Dividends,Data,USD,2025-06-12,IBKR(US45841N1072) Payment in Lieu of Dividend (Ordinary Dividend),0.1\n" +
		"Dividends,Data,Total,,,0.18\n"
	ev := buildDoc(t, body).Events
	if len(ev) != 1 {
		t.Fatalf("events = %d", len(ev))
	}
	e := ev[0]
	if e.Kind != KindDividend || e.Symbol != "IBKR" || e.TotalAmount != 18 || e.Fee != 0 || e.Date != "2025-06-12" || !e.DateOnly {
		t.Errorf("dividend: %+v", e)
	}
	if !strings.Contains(e.Note, "net of reversal") || e.Time.Format(time.RFC3339) != "2025-06-12T04:00:00Z" {
		t.Errorf("note/time: %q %v", e.Note, e.Time)
	}
}

func TestFullyReversedDividendHasNoEvent(t *testing.T) {
	body := divHeader +
		"Dividends,Data,USD,2025-06-12,IBKR(US1) Payment in Lieu of Dividend (Ordinary Dividend),0.09\n" +
		"Dividends,Data,USD,2025-06-12,IBKR(US1) Payment in Lieu of Dividend - Reversal (Ordinary Dividend),-0.09\n"
	if n := len(buildDoc(t, body).Events); n != 0 {
		t.Errorf("events = %d, want 0", n)
	}
}

func TestWithholdingAndDividendFeeMatching(t *testing.T) {
	body := divHeader +
		"Dividends,Data,USD,2025-08-26,NVO(US6701002056) Cash Dividend USD 0.58432 per Share (Ordinary Dividend),18.12\n" +
		whtHeader +
		"Withholding Tax,Data,USD,2025-08-26,NVO(US6701002056) Cash Dividend USD 0.58432 per Share - DK Tax,-4.89,\n" +
		feeHeader +
		"Fees,Data,Other Fees,USD,2025-08-26,NVO(US6701002056) Cash Dividend USD 0.58432 per Share - FEE,-0.47\n" +
		"Fees,Data,Total,,,,-0.47\n"
	ev := buildDoc(t, body).Events
	if len(ev) != 1 {
		t.Fatalf("events = %d", len(ev))
	}
	e := ev[0]
	if e.TotalAmount != 1812 || e.Fee != 536 || e.CashDelta() != 1276 {
		t.Errorf("dividend: %+v net=%d", e, e.CashDelta())
	}
}

func TestStandaloneWithholdingAndADRFee(t *testing.T) {
	body := whtHeader +
		"Withholding Tax,Data,USD,2025-03-03,XYZ(US0000) Cash Dividend USD 1 per Share - US Tax,-0.31,\n" +
		"Withholding Tax,Data,USD,2025-03-03,XYZ(US0000) Cash Dividend USD 1 per Share - US Tax,-0.2,\n" +
		feeHeader +
		"Fees,Data,Other Fees,USD,2025-12-09,CHA(15743P104) ADR Fee USD 0.02 per Share,-1.94\n"
	ev := buildDoc(t, body).Events
	if len(ev) != 2 {
		t.Fatalf("events = %d", len(ev))
	}
	if e := ev[0]; e.Kind != KindExpense || e.Source != SourceWithholding || e.TotalAmount != 51 || e.Symbol != "XYZ" {
		t.Errorf("withholding: %+v", e)
	}
	e := ev[1]
	if e.Kind != KindExpense || e.Source != SourceFee || e.TotalAmount != 194 || e.Symbol != "CHA" ||
		e.Note != "CHA(15743P104) ADR Fee USD 0.02 per Share" || e.CashDelta() != -194 {
		t.Errorf("ADR fee: %+v", e)
	}
}

func TestGrantTypes(t *testing.T) {
	body := "Grant Activity,Header,Symbol,Report Date,Description,Award Date,Vesting Date,Quantity,Price,Value\n" +
		"Grant Activity,Data,IBKR,2025-01-29,Stock Award Grant for Cash Deposit,2025-01-29,2026-01-29,0.1749,207.75,36.34\n" +
		"Grant Activity,Data,IBKR,2025-08-28,Stock Award Return for Cash Withdrawal,2025-01-29,2026-01-29,-0.0002,51.20655376,-0.01\n" +
		"Grant Activity,Data,IBKR,2026-01-29,Stock Award Vesting,2025-01-29,2026-01-29,0.6994,75.66,52.9167\n" +
		"Grant Activity,Data,IBKR,2026-01-29,Stock Award Withholding,2025-01-29,2026-01-29,-0.2098,75.66,-15.87\n" +
		"Grant Activity,Data,Total,,,,,1.6623,,138.5129\n"
	ev := buildDoc(t, body).Events
	if len(ev) != 3 {
		t.Fatalf("events = %d, want 3 (vesting ignored)", len(ev))
	}
	g, r, w := ev[0], ev[1], ev[2]
	if g.Kind != KindBuy || g.Source != SourceGrant || g.Quantity != 0.1749 || g.TotalAmount != 3634 || g.PricePerUnit != 20775 || !g.NoCash || g.CashDelta() != 0 {
		t.Errorf("grant: %+v", g)
	}
	if r.Kind != KindSell || r.Source != SourceGrantReturn || r.Quantity != 0.0002 || r.TotalAmount != 1 || r.PricePerUnit != 5121 || !r.NoCash {
		t.Errorf("return: %+v", r)
	}
	if w.Kind != KindSell || w.Source != SourceGrantWithholding || w.Quantity != 0.2098 || w.TotalAmount != 1587 || w.Note != "Stock award tax withholding" || !w.NoCash {
		t.Errorf("withholding: %+v", w)
	}
}

func TestSplit(t *testing.T) {
	body := "Corporate Actions,Header,Asset Category,Currency,Report Date,Date/Time,Description,Quantity,Proceeds,Value,Realized P/L,Code\n" +
		"Corporate Actions,Data,Stocks,USD,2025-06-18,\"2025-06-17, 20:25:00\",\"IBKR(US45841N1072) Split 4 for 1 (IBKR, INTERACTIVE BROKERS GRO-CL A, US45841N1072)\",0.9663,0,0,0,\n" +
		"Corporate Actions,Data,Total,,,,,,0,0,0,\n"
	ev := buildDoc(t, body).Events
	if len(ev) != 1 {
		t.Fatalf("events = %d", len(ev))
	}
	e := ev[0]
	if e.Kind != KindSplit || e.Symbol != "IBKR" || e.SplitNumerator != 4 || e.SplitDenominator != 1 || e.SplitRatio != 4 ||
		e.Date != "2025-06-17" || e.Time.Format(time.RFC3339) != "2025-06-18T00:25:00Z" || e.CashDelta() != 0 {
		t.Errorf("split: %+v", e)
	}
}

func TestDeposits(t *testing.T) {
	body := "Deposits & Withdrawals,Header,Currency,Settle Date,Description,Amount\n" +
		"Deposits & Withdrawals,Data,USD,2025-01-28,Electronic Fund Transfer,3632.79\n" +
		"Deposits & Withdrawals,Data,USD,2025-08-28,Disbursement Initiated by Test User,-1\n" +
		"Deposits & Withdrawals,Data,Total,,,3631.79\n"
	ev := buildDoc(t, body).Events
	if len(ev) != 2 {
		t.Fatalf("events = %d", len(ev))
	}
	if d := ev[0]; d.Kind != KindDeposit || d.TotalAmount != 363279 || d.CashDelta() != 363279 || d.Symbol != "" || d.Note != "Electronic Fund Transfer" {
		t.Errorf("deposit: %+v", d)
	}
	if w := ev[1]; w.Kind != KindWithdrawal || w.TotalAmount != 100 || w.CashDelta() != -100 {
		t.Errorf("withdrawal: %+v", w)
	}
}

func TestCheckpointsInstrumentsAndIgnored(t *testing.T) {
	body := "Cash Report,Header,Currency Summary,Currency,Total,Securities,Futures,\n" +
		"Cash Report,Data,Starting Cash,Base Currency Summary,0,0,0,\n" +
		"Cash Report,Data,Ending Cash,USD,99,99,0,\n" +
		"Cash Report,Data,Ending Cash,Base Currency Summary,4.65331474,4.65331474,0,\n" +
		"Open Positions,Header,DataDiscriminator,Asset Category,Currency,Symbol,Open,Quantity,Mult,Cost Price,Cost Basis\n" +
		"Open Positions,Data,Summary,Stocks,USD,CCJ,-,30.0511,1,82.66,2484.304519\n" +
		"Open Positions,Data,Lot,Stocks,USD,CCJ,x,1,,1,1\n" +
		"Open Positions,Total,,Stocks,USD,,,,,,2484.3\n" +
		"Financial Instrument Information,Header,Asset Category,Symbol,Description,Conid,Security ID,Underlying,Listing Exch,Multiplier,Type,Code\n" +
		"Financial Instrument Information,Data,Stocks,VUAA,VANG S&P500 USDA,366244335,IE00BFMXXD54,VUAA,LSEETF,1,ETF,\n" +
		"Codes,Header,Code,Meaning\n" +
		"Codes,Data,Pr,Something\n" +
		"Mark-to-Market Performance Summary,Header,Asset Category,Symbol\n" +
		"Mark-to-Market Performance Summary,Data,Stocks,CCJ\n" +
		"Total P/L for Statement Period,,,,,1\n"
	st := parseDoc(t, body)
	if !st.HasEndingCash || st.EndingCashCents != 465 {
		t.Errorf("ending cash: %v %d", st.HasEndingCash, st.EndingCashCents)
	}
	if p := st.Positions["CCJ"]; p.Quantity != 30.0511 || p.CostBasisCents != 248430 || len(st.Positions) != 1 {
		t.Errorf("positions: %+v", st.Positions)
	}
	if in := st.Instruments["VUAA"]; in.ISIN != "IE00BFMXXD54" || in.ListingExchange != "LSEETF" || in.Type != "ETF" {
		t.Errorf("instrument: %+v", in)
	}
	want := []string{"Statement", "Account Information", "Codes", "Mark-to-Market Performance Summary", "Total P/L for Statement Period"}
	if strings.Join(st.IgnoredSections, "|") != strings.Join(want, "|") {
		t.Errorf("ignored = %v", st.IgnoredSections)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"unsupported section": "Interest,Header,Currency,Amount\nInterest,Data,USD,1\n",
		"unsupported asset":   "Trades,Header,DataDiscriminator,Asset Category,Currency,Symbol,Date/Time,Quantity,T. Price,Proceeds,Comm/Fee\nTrades,Data,Order,Forex,USD,EUR.USD,\"2025-01-02, 10:00:00\",1,1,1,1\n",
		"corporate action": "Corporate Actions,Header,Asset Category,Currency,Report Date,Date/Time,Description,Quantity\n" +
			"Corporate Actions,Data,Stocks,USD,2025-06-18,\"2025-06-17, 20:25:00\",ABC(US1) Merged (Acquisition) for 1,1\n",
		"missing column": "Dividends,Header,Currency,Date,Amount\nDividends,Data,USD,2025-01-01,1\n",
		"no header":      "Dividends,Data,USD,2025-01-01,x,1\n",
		"bad number":     divHeader + "Dividends,Data,USD,2025-01-01,A(1) Cash Dividend,abc\n",
		"open pos asset": "Open Positions,Header,DataDiscriminator,Asset Category,Currency,Symbol,Quantity,Cost Basis\nOpen Positions,Data,Summary,Options,USD,X,1,1\n",
	}
	for name, body := range cases {
		_, err := Parse(strings.NewReader(doc("January 1, 2025 - December 31, 2025", "U1", "USD", body)))
		var pe *ParseError
		if !errors.As(err, &pe) {
			t.Errorf("%s: err = %v, want *ParseError", name, err)
			continue
		}
		if name != "bad number" && name != "missing column" && name != "no header" && pe.Line == 0 {
			t.Errorf("%s: no line number: %v", name, err)
		}
	}
	// Spot-check messages.
	_, err := Parse(strings.NewReader(doc("January 1, 2025 - December 31, 2025", "U1", "USD", cases["unsupported section"])))
	if err == nil || !strings.Contains(err.Error(), `unsupported section "Interest"`) {
		t.Errorf("message: %v", err)
	}
	_, err = Parse(strings.NewReader(doc("January 1, 2025 - December 31, 2025", "U1", "USD", cases["corporate action"])))
	if err == nil || !strings.Contains(err.Error(), "unsupported corporate action") || !strings.Contains(err.Error(), "line 9") {
		t.Errorf("message: %v", err)
	}
}

func TestBuildEventsErrors(t *testing.T) {
	mk := func(period, acct, ccy string) *Statement {
		st, err := Parse(strings.NewReader(doc(period, acct, ccy, "")))
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	a := mk("January 1, 2025 - December 31, 2025", "U1", "USD")
	b := mk("January 1, 2026 - November 2, 2026", "U1", "USD")
	if set, err := BuildEvents([]*Statement{b, a}); err != nil || len(set.Periods) != 2 || set.Periods[0].File != 1 {
		t.Fatalf("out-of-order contiguous: %v %+v", err, set)
	}
	cases := map[string][]*Statement{
		"gap":      {a, mk("January 2, 2026 - November 2, 2026", "U1", "USD")},
		"overlap":  {a, mk("December 31, 2025 - November 2, 2026", "U1", "USD")},
		"account":  {a, mk("January 1, 2026 - November 2, 2026", "U2", "USD")},
		"currency": {a, mk("January 1, 2026 - November 2, 2026", "U1", "EUR")},
		"empty":    {},
		"nil":      {a, nil},
	}
	for name, in := range cases {
		if _, err := BuildEvents(in); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestBuildEventsRowErrors(t *testing.T) {
	cases := map[string]string{
		"grant":    "Grant Activity,Header,Symbol,Report Date,Description,Quantity,Price,Value\nGrant Activity,Data,X,2025-01-01,Stock Award Mystery,1,1,1\n",
		"dividend": divHeader + "Dividends,Data,USD,2025-01-01,A(1) Interest,1\n",
		"fee":      feeHeader + "Fees,Data,Other Fees,USD,2025-01-01,A(1) Rebate,1\n",
		"refund":   whtHeader + "Withholding Tax,Data,USD,2025-01-01,A(1) Cash Dividend - US Tax,0.5,\n",
	}
	for name, body := range cases {
		_, err := BuildEvents([]*Statement{parseDoc(t, body)})
		var ee *EventError
		if !errors.As(err, &ee) {
			t.Errorf("%s: err = %v, want *EventError", name, err)
			continue
		}
		if ee.Ref.File != 0 || ee.Ref.Line == 0 || !strings.Contains(err.Error(), "file 1 line") {
			t.Errorf("%s: ref %v", name, ee.Ref)
		}
	}
}

func TestOrderingAndKeys(t *testing.T) {
	// Same instant (00:00 New York on 2025-03-03): grant < split < dividend < deposit, regardless of row order.
	body := "Deposits & Withdrawals,Header,Currency,Settle Date,Description,Amount\n" +
		"Deposits & Withdrawals,Data,USD,2025-03-03,Electronic Fund Transfer,10\n" +
		divHeader + "Dividends,Data,USD,2025-03-03,AAA(US1) Cash Dividend USD 1 per Share,1\n" +
		"Grant Activity,Header,Symbol,Report Date,Description,Quantity,Price,Value\n" +
		"Grant Activity,Data,AAA,2025-03-03,Stock Award Grant for Cash Deposit,1,1,1\n" +
		"Grant Activity,Data,AAA,2025-03-03,Stock Award Grant for Cash Deposit,2,1,2\n" +
		"Trades,Header,DataDiscriminator,Asset Category,Currency,Symbol,Date/Time,Quantity,T. Price,Proceeds,Comm/Fee\n" +
		"Trades,Data,Order,Stocks,USD,AAA,\"2025-03-03, 09:30:00\",1,1,-1,0\n" +
		"Trades,Data,Order,Stocks,USD,AAA,\"2025-03-03, 09:30:00\",1,1,-1,0\n"
	st := parseDoc(t, body)
	set1, err := BuildEvents([]*Statement{st})
	if err != nil {
		t.Fatal(err)
	}
	set2, err := BuildEvents([]*Statement{st})
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, e := range set1.Events {
		keys = append(keys, e.Key)
	}
	want := []string{
		"grant:20250303:AAA:1", "grant:20250303:AAA:2", "dividend:20250303:AAA:1", "deposit:20250303:cash:1",
		"trade:20250303T093000:AAA:1", "trade:20250303T093000:AAA:2",
	}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("keys = %v\nwant   %v", keys, want)
	}
	for i := range set1.Events {
		if set1.Events[i].Key != set2.Events[i].Key {
			t.Errorf("keys not deterministic at %d", i)
		}
	}
	if len(set1.Symbols()) != 1 {
		t.Errorf("symbols = %v", set1.Symbols())
	}
}

func TestFixtureKeysUniqueAndShort(t *testing.T) {
	set := fixtureSet(t)
	seen := map[string]bool{}
	for _, e := range set.Events {
		if seen[e.Key] {
			t.Errorf("duplicate key %s", e.Key)
		}
		seen[e.Key] = true
		if len(e.Key)+len("ibkr:U00000000:") > 64 {
			t.Errorf("key too long for a 64-char ref: %s", e.Key)
		}
	}
	if len(set.IgnoredSections) == 0 || len(set.Instruments) < 8 {
		t.Errorf("ignored=%v instruments=%d", set.IgnoredSections, len(set.Instruments))
	}
}
