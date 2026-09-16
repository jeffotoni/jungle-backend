package handlers

import (
	"net/http"

	"github.com/jeffotoni/jungle-backend-challenge/cmd/api/models"
	"github.com/jeffotoni/quick"
)

func (r *Routes) getWallet(c *quick.Ctx) error {
	if err := r.requireInternal(c); err != nil {
		return writeError(c, err)
	}
	if !validID(c.Param("walletId")) {
		return writeError(c, errInvalid())
	}
	result, err := r.wallets.Get(c.Ctx(), c.Param("walletId"))
	if err != nil {
		return writeError(c, err)
	}
	return c.Status(http.StatusOK).JSON(models.WalletResponse{
		ID:       result.ID,
		PlayerID: result.PlayerID,
		Balance: models.Money{
			Amount:   result.Balance.String(),
			Currency: result.Balance.Currency,
		},
		Version: result.Version,
	})
}
