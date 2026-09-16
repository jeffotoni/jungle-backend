package handlers

import (
	"net/http"

	"github.com/jeffotoni/jungle-backend-challenge/cmd/api/models"
	"github.com/jeffotoni/quick"
)

func (r *Routes) reconcile(c *quick.Ctx) error {
	if err := r.requireInternal(c); err != nil {
		return writeError(c, err)
	}
	result, err := r.wallets.Reconcile(c.Ctx(), c.Param("walletId"))
	if err != nil {
		return writeError(c, err)
	}
	return c.Status(http.StatusOK).JSON(models.ReconciliationResponse{
		WalletID: result.Wallet.ID,
		StoredBalance: models.Money{
			Amount:   result.Wallet.Balance.String(),
			Currency: result.Wallet.Balance.Currency,
		},
		CalculatedBalance: models.Money{
			Amount:   result.Calculated.String(),
			Currency: result.Calculated.Currency,
		},
		Difference: models.Money{
			Amount:   result.Difference.String(),
			Currency: result.Difference.Currency,
		},
		Consistent:     result.Consistent,
		CheckedEntries: result.CheckedEntries,
	})
}
