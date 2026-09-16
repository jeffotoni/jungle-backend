package handlers

import (
	"net/http"

	"github.com/jeffotoni/jungle-backend-challenge/cmd/api/models"
	appwager "github.com/jeffotoni/jungle-backend-challenge/internal/application/wagering"
	"github.com/jeffotoni/jungle-backend-challenge/internal/contracts"
	"github.com/jeffotoni/quick"
)

func (r *Routes) processWager(c *quick.Ctx) error {
	var request contracts.WagerRequest
	if err := decodeJSON(c.Body(), &request); err != nil {
		return writeError(c, errInvalid())
	}
	if request.ProviderID == "" {
		return writeError(c, errInvalid())
	}
	if err := r.requireProvider(c, request.ProviderID); err != nil {
		return writeError(c, err)
	}
	request.IdempotencyKey = c.GetHeader("Idempotency-Key")
	if request.IdempotencyKey == "" {
		return writeError(c, errInvalid())
	}
	result, err := r.wagers.Process(c.Ctx(), request)
	if err != nil {
		return writeError(c, err)
	}
	response := wagerResponse(result)
	if result.Status == "PENDING_REFERENCE" {
		return c.Status(http.StatusAccepted).JSON(response)
	}
	if result.Status == "REJECTED" {
		return c.Status(http.StatusUnprocessableEntity).JSON(response)
	}
	if result.Status == "FAILED" {
		return c.Status(http.StatusServiceUnavailable).JSON(response)
	}
	return c.Status(http.StatusOK).JSON(response)
}

func wagerResponse(result appwager.Result) models.WagerResponse {
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
