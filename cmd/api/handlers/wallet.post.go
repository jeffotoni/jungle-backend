package handlers

import (
	"net/http"

	"github.com/jeffotoni/quick"

	"github.com/jeffotoni/jungle-backend/cmd/api/models"
	"github.com/jeffotoni/jungle-backend/internal/domain/money"
)

func (r *Routes) createWallet(c *quick.Ctx) error {
	if err := r.requireInternal(c); err != nil {
		return writeError(c, err)
	}
	var request models.CreateWalletRequest
	if err := decodeJSON(c.Body(), &request); err != nil {
		return writeError(c, errInvalid())
	}
	balance, err := money.Parse(request.InitialBalance.Amount, request.InitialBalance.Currency)
	if err != nil || balance.MinorUnits() < 0 {
		return writeError(c, errInvalid())
	}
	result, err := r.wallets.Create(c.Ctx(), request.PlayerID, balance)
	if err != nil {
		return writeError(c, err)
	}
	return c.Status(http.StatusCreated).JSON(models.WalletResponse{
		ID:       result.ID,
		PlayerID: result.PlayerID,
		Balance: models.Money{
			Amount:   result.Balance.String(),
			Currency: result.Balance.Currency(),
		},
		Version: result.Version,
	})
}
