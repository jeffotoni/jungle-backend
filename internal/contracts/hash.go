package contracts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

func CanonicalHash(request WagerRequest) (string, error) {
	normalized, err := NormalizeWagerRequest(request)
	if err != nil {
		return "", err
	}
	b, err := canonicalJSON(normalized)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(b)
	return hex.EncodeToString(digest[:]), nil
}

func canonicalJSON(request WagerRequest) ([]byte, error) {
	payload := map[string]any{
		"externalTransactionId": request.ExternalTransactionID,
		"gameId":                request.GameID,
		"kind":                  request.Kind,
		"money": map[string]any{
			"amount":   request.Money.Amount,
			"currency": request.Money.Currency,
		},
		"playerId":   request.PlayerID,
		"providerId": request.ProviderID,
		"roundId":    request.RoundID,
		"walletId":   request.WalletID,
	}
	if request.ReferenceExternalTransactionID != nil {
		payload["referenceExternalTransactionId"] = *request.ReferenceExternalTransactionID
	}
	return marshalCanonicalObject(payload)
}

func marshalCanonicalObject(object map[string]any) ([]byte, error) {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var buffer bytes.Buffer
	buffer.WriteByte('{')
	for index, key := range keys {
		if index > 0 {
			buffer.WriteByte(',')
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buffer.Write(encodedKey)
		buffer.WriteByte(':')
		value := object[key]
		if nested, ok := value.(map[string]any); ok {
			encodedValue, err := marshalCanonicalObject(nested)
			if err != nil {
				return nil, err
			}
			buffer.Write(encodedValue)
			continue
		}
		encodedValue, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		buffer.Write(encodedValue)
	}
	buffer.WriteByte('}')
	return buffer.Bytes(), nil
}
