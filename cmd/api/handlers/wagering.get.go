package handlers

import (
	"net/http"

	apiauth "github.com/jeffotoni/jungle-backend-challenge/cmd/api/auth"
	"github.com/jeffotoni/jungle-backend-challenge/cmd/api/models"
	appwager "github.com/jeffotoni/jungle-backend-challenge/internal/application/wagering"
	"github.com/jeffotoni/quick"
)

func (r *Routes) getWager(c *quick.Ctx) error {
	principal, err := r.auth.Authenticate(c.Request)
	if err != nil {
		return writeError(c, authError(err))
	}
	if principal.ProviderID == "" {
		return writeError(c, authError(apiauth.ErrForbidden))
	}
	result, err := r.wagers.Get(c.Ctx(), principal.ProviderID, c.Param("transactionId"))
	if err != nil {
		return writeError(c, err)
	}
	return c.Status(http.StatusOK).JSON(wagerResponseValue(result))
}

func (r *Routes) getWagerByExternal(c *quick.Ctx) error {
	providerID := c.Param("providerId")
	if err := r.requireProvider(c, providerID); err != nil {
		return writeError(c, err)
	}
	result, err := r.wagers.GetByExternal(c.Ctx(), providerID, c.Param("externalTransactionId"))
	if err != nil {
		return writeError(c, err)
	}
	return c.Status(http.StatusOK).JSON(wagerResponseValue(result))
}

func wagerResponseValue(result appwager.Result) models.WagerResponse {
	response := models.WagerResponse{
		TransactionID:    result.TransactionID,
		Status:           string(result.Status),
		IdempotentReplay: result.Replay,
		FailureCode:      result.FailureCode,
	}
	if result.Balance != nil {
		response.Balance = &models.Money{Amount: result.Balance.String(), Currency: result.Balance.Currency}
	}
	return response
}
