package handlers

import (
	"net/http"

	"github.com/jeffotoni/jungle-backend/cmd/api/models"
	"github.com/jeffotoni/jungle-backend/internal/application/ports"
	apirepository "github.com/jeffotoni/jungle-backend/internal/repository/postgres"
	"github.com/jeffotoni/quick"
)

func (r *Routes) getLedger(c *quick.Ctx) error {
	if err := r.requireInternal(c); err != nil {
		return writeError(c, err)
	}
	if err := apirepository.ValidateCursor(c.QueryParam("cursor")); err != nil {
		return writeError(c, err)
	}
	limit := parseLimit(c.QueryParam("limit"))
	entries, err := r.wallets.Ledger(c.Ctx(), c.Param("walletId"), c.QueryParam("cursor"), limit)
	if err != nil {
		return writeError(c, err)
	}
	response := models.LedgerResponse{Entries: make([]models.LedgerEntryResponse, 0, len(entries.Entries))}
	hasNext := len(entries.Entries) > limit
	var next ports.LedgerRecord
	if hasNext {
		next = entries.Entries[limit]
	}
	if hasNext {
		entries.Entries = entries.Entries[:limit]
	}
	for _, entry := range entries.Entries {
		response.Entries = append(response.Entries, models.LedgerEntryResponse{
			ID:            entry.ID,
			TransactionID: entry.TransactionID,
			Direction:     entry.Direction,
			Money: models.Money{
				Amount:   formatMinor(entry.Amount),
				Currency: entry.Currency,
			},
			BalanceBefore: models.Money{
				Amount:   formatMinor(entry.BalanceBefore),
				Currency: entry.Currency,
			},
			BalanceAfter: models.Money{
				Amount:   formatMinor(entry.BalanceAfter),
				Currency: entry.Currency,
			},
			CreatedAt: entry.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		})
	}
	if hasNext {
		response.NextCursor = apirepository.EncodeCursor(next)
	}
	return c.Status(http.StatusOK).JSON(response)
}
