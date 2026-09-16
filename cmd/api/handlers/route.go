package handlers

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jeffotoni/quick"

	apiauth "github.com/jeffotoni/jungle-backend-challenge/cmd/api/auth"
	appwager "github.com/jeffotoni/jungle-backend-challenge/internal/application/wagering"
	appwallet "github.com/jeffotoni/jungle-backend-challenge/internal/application/wallet"
)

func NewRouter(
	wallets *appwallet.Service,
	wagers *appwager.Service,
	verifier *apiauth.Verifier,
	pool *pgxpool.Pool,
) *quick.Quick {
	q := quick.New()
	routes := &Routes{wallets: wallets, wagers: wagers, auth: verifier, pool: pool}

	q.Get("/health/live", routes.live)
	q.Get("/health/ready", routes.ready)
	q.Post("/wallets", routes.createWallet)
	q.Get("/wallets/:walletId", routes.getWallet)
	q.Get("/wallets/:walletId/ledger", routes.getLedger)
	q.Post("/wallets/:walletId/reconciliation", routes.reconcile)
	q.Post("/wagering/transactions", routes.processWager)
	q.Get("/wagering/transactions/:transactionId", routes.getWager)
	q.Get("/providers/:providerId/wagering/transactions/:externalTransactionId", routes.getWagerByExternal)
	return q
}
