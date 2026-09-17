package wallet

import (
	"testing"

	"github.com/jeffotoni/jungle-backend/internal/domain/money"
)

func TestNewWallet(t *testing.T) {
	balance, err := money.New(2500, "BRL")
	if err != nil {
		t.Fatal(err)
	}

	created, err := New("wallet-1", "player-1", balance)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID() != "wallet-1" || created.PlayerID() != "player-1" {
		t.Fatalf("unexpected identity: %+v", created)
	}
	if created.Balance() != balance {
		t.Fatalf("unexpected balance: %+v", created.Balance())
	}
	if created.Version() != 1 {
		t.Fatalf("version=%d want=1", created.Version())
	}
}

func TestNewWalletRejectsNegativeBalance(t *testing.T) {
	balance, err := money.New(-1, "BRL")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := New("wallet-1", "player-1", balance); err == nil {
		t.Fatal("expected negative balance to be rejected")
	}
}

func TestWalletDebitCreditControlsBalanceAndVersion(t *testing.T) {
	balance, _ := money.New(2500, "BRL")
	wallet, err := New("wallet-1", "player-1", balance)
	if err != nil {
		t.Fatal(err)
	}
	movement, _ := money.New(1000, "BRL")

	if err := wallet.Debit(movement); err != nil {
		t.Fatal(err)
	}
	if wallet.Balance().MinorUnits() != 1500 || wallet.Version() != 2 {
		t.Fatalf("unexpected debit state: balance=%d version=%d", wallet.Balance().MinorUnits(), wallet.Version())
	}
	if err := wallet.Credit(movement); err != nil {
		t.Fatal(err)
	}
	if wallet.Balance().MinorUnits() != 2500 || wallet.Version() != 3 {
		t.Fatalf("unexpected credit state: balance=%d version=%d", wallet.Balance().MinorUnits(), wallet.Version())
	}
}

func TestWalletRejectsInsufficientDebit(t *testing.T) {
	balance, _ := money.New(500, "BRL")
	wallet, err := New("wallet-1", "player-1", balance)
	if err != nil {
		t.Fatal(err)
	}
	movement, _ := money.New(1000, "BRL")

	if err := wallet.Debit(movement); err != ErrInsufficientBalance {
		t.Fatalf("error=%v want=%v", err, ErrInsufficientBalance)
	}
	if wallet.Balance().MinorUnits() != 500 || wallet.Version() != 1 {
		t.Fatalf("wallet changed after rejected debit: %+v", wallet)
	}
}
