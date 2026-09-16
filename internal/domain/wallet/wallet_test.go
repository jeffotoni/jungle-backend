package wallet

import (
	"testing"

	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/money"
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
	if created.ID != "wallet-1" || created.PlayerID != "player-1" {
		t.Fatalf("unexpected identity: %+v", created)
	}
	if created.Balance != balance {
		t.Fatalf("unexpected balance: %+v", created.Balance)
	}
	if created.Version != 1 {
		t.Fatalf("version=%d want=1", created.Version)
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
