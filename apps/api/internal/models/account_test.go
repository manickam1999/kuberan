package models

import "testing"

func TestAccountType_IsSpending(t *testing.T) {
	tests := map[AccountType]bool{
		AccountTypeCash:         true,
		AccountTypeCreditCard:   true,
		AccountTypeDebt:         true,
		AccountTypeInvestment:   false,
		AccountTypeFixedDeposit: false,
		AccountType("unknown"):  false,
	}
	for accountType, want := range tests {
		if got := accountType.IsSpending(); got != want {
			t.Errorf("%s.IsSpending() = %v, want %v", accountType, got, want)
		}
	}
}
