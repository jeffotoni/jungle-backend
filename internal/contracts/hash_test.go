package contracts

import (
	"strings"
	"testing"
)

func TestCanonicalHashNormalizesAllowedFields(t *testing.T) {
	first := WagerRequest{
		ProviderID:            "provider-a",
		ExternalTransactionID: "external-1",
		IdempotencyKey:        "key-1",
		WalletID:              "wallet-1",
		PlayerID:              "player-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  "bet",
		Money:                 MoneyInput{Amount: " 25 ", Currency: " brl "},
	}
	second := first
	second.IdempotencyKey = "key-2"
	second.Kind = "BET"
	second.Money = MoneyInput{Amount: "25.00", Currency: "BRL"}

	firstHash, err := CanonicalHash(first)
	if err != nil {
		t.Fatal(err)
	}
	secondHash, err := CanonicalHash(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash {
		t.Fatalf("equivalent requests have different hashes: %s != %s", firstHash, secondHash)
	}
}

func TestCanonicalJSONUsesLexicographicKeys(t *testing.T) {
	request := WagerRequest{
		ProviderID:            "provider-a",
		ExternalTransactionID: "external-1",
		WalletID:              "wallet-1",
		PlayerID:              "player-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  "BET",
		Money:                 MoneyInput{Amount: "25.00", Currency: "BRL"},
	}
	normalized, err := NormalizeWagerRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := canonicalJSON(normalized)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"externalTransactionId":"external-1","gameId":"game-1","kind":"BET","money":{"amount":"25.00","currency":"BRL"},"playerId":"player-1","providerId":"provider-a","roundId":"round-1","walletId":"wallet-1"}`
	if string(encoded) != want {
		t.Fatalf("canonical JSON=%s want=%s", encoded, want)
	}
}

func TestNormalizeWagerRequestRejectsIdentifierWhitespace(t *testing.T) {
	request := WagerRequest{
		ProviderID:            " provider-a",
		ExternalTransactionID: "external-1",
		WalletID:              "wallet-1",
		PlayerID:              "player-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  "BET",
		Money:                 MoneyInput{Amount: "25.00", Currency: "BRL"},
	}
	if _, err := NormalizeWagerRequest(request); err == nil || !strings.Contains(err.Error(), "identifier") {
		t.Fatalf("expected identifier whitespace error, got %v", err)
	}
}
