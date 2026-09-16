package contracts

import (
	"encoding/json"
	"testing"
)

func TestMarshalWalletBalanceChangedEvent(t *testing.T) {
	payload, err := MarshalEvent(
		EventWalletBalanceChanged,
		"wallet-1",
		"transaction-1",
		2,
		WalletBalanceChangedData{
			WalletID:      "wallet-1",
			TransactionID: "transaction-1",
			Direction:     "DEBIT",
			Money:         MoneyInput{Amount: "10.00", Currency: "BRL"},
			BalanceBefore: MoneyInput{Amount: "25.00", Currency: "BRL"},
			BalanceAfter:  MoneyInput{Amount: "15.00", Currency: "BRL"},
			WalletVersion: 2,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	var event EventEnvelope[WalletBalanceChangedData]
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	if event.EventID == "" || event.EventType != EventWalletBalanceChanged {
		t.Fatalf("unexpected event identity: %+v", event)
	}
	if event.AggregateID != "wallet-1" || event.CorrelationID != "transaction-1" || event.Version != 2 {
		t.Fatalf("unexpected event metadata: %+v", event)
	}
	if event.Data.BalanceAfter.Amount != "15.00" || event.Data.WalletVersion != 2 {
		t.Fatalf("unexpected event data: %+v", event.Data)
	}
}
