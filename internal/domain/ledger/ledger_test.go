package ledger

import (
	"testing"

	"github.com/jeffotoni/jungle-backend/internal/domain/money"
)

func TestNewValidatesDebitBalanceFlow(t *testing.T) {
	amount, _ := money.New(1000, "BRL")
	before, _ := money.New(2500, "BRL")
	after, _ := money.New(1500, "BRL")

	entry, err := New(EntryInput{
		ID:            "entry-1",
		WalletID:      "wallet-1",
		TransactionID: "transaction-1",
		Direction:     DirectionDebit,
		Amount:        amount,
		BalanceBefore: before,
		BalanceAfter:  after,
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.BalanceAfter().MinorUnits() != 1500 {
		t.Fatalf("balanceAfter=%d", entry.BalanceAfter().MinorUnits())
	}
}

func TestNewRejectsInvalidBalanceFlow(t *testing.T) {
	amount, _ := money.New(1000, "BRL")
	before, _ := money.New(2500, "BRL")
	after, _ := money.New(2400, "BRL")

	if _, err := New(EntryInput{
		ID:            "entry-1",
		WalletID:      "wallet-1",
		TransactionID: "transaction-1",
		Direction:     DirectionDebit,
		Amount:        amount,
		BalanceBefore: before,
		BalanceAfter:  after,
	}); err != ErrInvalidBalanceFlow {
		t.Fatalf("error=%v want=%v", err, ErrInvalidBalanceFlow)
	}
}
