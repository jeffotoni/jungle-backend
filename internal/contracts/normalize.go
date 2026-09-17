package contracts

import (
	"errors"
	"strings"

	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/money"
)

var ErrIdentifierWhitespace = errors.New("identifier contains leading or trailing whitespace")

func NormalizeWagerRequest(request WagerRequest) (WagerRequest, error) {
	for _, value := range []string{
		request.ProviderID,
		request.ExternalTransactionID,
		request.IdempotencyKey,
		request.WalletID,
		request.PlayerID,
		request.RoundID,
		request.GameID,
	} {
		if value != strings.TrimSpace(value) {
			return WagerRequest{}, ErrIdentifierWhitespace
		}
	}
	if request.ReferenceExternalTransactionID != nil &&
		*request.ReferenceExternalTransactionID != strings.TrimSpace(*request.ReferenceExternalTransactionID) {
		return WagerRequest{}, ErrIdentifierWhitespace
	}

	request.Kind = strings.ToUpper(strings.TrimSpace(request.Kind))
	request.Money.Currency = strings.ToUpper(strings.TrimSpace(request.Money.Currency))
	request.Money.Amount = strings.TrimSpace(request.Money.Amount)
	amount, err := money.Parse(request.Money.Amount, request.Money.Currency)
	if err != nil {
		return WagerRequest{}, err
	}
	request.Money.Amount = amount.String()
	return request, nil
}
