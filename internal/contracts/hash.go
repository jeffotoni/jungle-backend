package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func CanonicalHash(request WagerRequest) (string, error) {
	payload := struct {
		ProviderID                     string     `json:"providerId"`
		ExternalTransactionID          string     `json:"externalTransactionId"`
		WalletID                       string     `json:"walletId"`
		PlayerID                       string     `json:"playerId"`
		RoundID                        string     `json:"roundId"`
		GameID                         string     `json:"gameId"`
		Kind                           string     `json:"kind"`
		Money                          MoneyInput `json:"money"`
		ReferenceExternalTransactionID *string    `json:"referenceExternalTransactionId,omitempty"`
	}{
		ProviderID:                     request.ProviderID,
		ExternalTransactionID:          request.ExternalTransactionID,
		WalletID:                       request.WalletID,
		PlayerID:                       request.PlayerID,
		RoundID:                        request.RoundID,
		GameID:                         request.GameID,
		Kind:                           request.Kind,
		Money:                          request.Money,
		ReferenceExternalTransactionID: request.ReferenceExternalTransactionID,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(b)
	return hex.EncodeToString(digest[:]), nil
}
