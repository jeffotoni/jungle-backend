package wager

import (
	"testing"

	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/money"
)

func TestTransactionTransitions(t *testing.T) {
	amount, _ := money.New(1000, "BRL")
	providerID := "provider-a"
	externalID := "bet-1"
	idempotencyKey := "key-1"
	transaction, err := New(Input{
		ID:                    "transaction-1",
		ProviderID:            &providerID,
		ExternalTransactionID: &externalID,
		IdempotencyKey:        &idempotencyKey,
		WalletID:              "wallet-1",
		PlayerID:              "player-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  KindBet,
		Money:                 amount,
		PayloadHash:           "hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	balance, _ := money.New(1500, "BRL")
	if err := transaction.MarkProcessed(balance); err != nil {
		t.Fatal(err)
	}
	if transaction.Status() != StatusProcessed || transaction.ResultBalance().MinorUnits() != 1500 {
		t.Fatalf("unexpected transaction: %+v", transaction)
	}
	if err := transaction.MarkProcessed(balance); err != ErrInvalidTransition {
		t.Fatalf("error=%v want=%v", err, ErrInvalidTransition)
	}
}

func TestLossHasNoFinancialEffect(t *testing.T) {
	amount, _ := money.New(0, "BRL")
	providerID := "provider-a"
	externalID := "loss-1"
	idempotencyKey := "key-loss-1"
	transaction, err := New(Input{
		ID:                    "transaction-loss",
		ProviderID:            &providerID,
		ExternalTransactionID: &externalID,
		IdempotencyKey:        &idempotencyKey,
		WalletID:              "wallet-1",
		PlayerID:              "player-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  KindLoss,
		Money:                 amount,
		PayloadHash:           "hash-loss",
	})
	if err != nil {
		t.Fatal(err)
	}
	effect, err := transaction.FinancialEffect(nil)
	if err != nil || effect.Applied {
		t.Fatalf("unexpected LOSS effect: %+v %v", effect, err)
	}
}

func TestReversalReferenceValidation(t *testing.T) {
	amount, _ := money.New(1000, "BRL")
	providerID := "provider-a"
	betExternalID := "bet-1"
	betKey := "bet-key"
	bet, err := New(Input{
		ID:                    "bet-transaction",
		ProviderID:            &providerID,
		ExternalTransactionID: &betExternalID,
		IdempotencyKey:        &betKey,
		WalletID:              "wallet-1",
		PlayerID:              "player-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  KindBet,
		Money:                 amount,
		PayloadHash:           "bet-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	balance, _ := money.New(0, "BRL")
	if err := bet.MarkProcessed(balance); err != nil {
		t.Fatal(err)
	}
	refundExternalID := "refund-1"
	refundKey := "refund-key"
	refund, err := New(Input{
		ID:                             "refund-transaction",
		ProviderID:                     &providerID,
		ExternalTransactionID:          &refundExternalID,
		IdempotencyKey:                 &refundKey,
		WalletID:                       "wallet-1",
		PlayerID:                       "player-1",
		RoundID:                        "round-1",
		GameID:                         "game-1",
		Kind:                           KindRefund,
		Money:                          amount,
		ReferenceExternalTransactionID: &betExternalID,
		PayloadHash:                    "refund-hash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := refund.ValidateReference(bet); err != nil {
		t.Fatal(err)
	}
	if err := refund.ResolveReference(bet.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := refund.FinancialEffect(nil); err != ErrReferenceNotValid {
		t.Fatalf("error=%v want=%v", err, ErrReferenceNotValid)
	}
}
