package handlers

import (
	"github.com/jackc/pgx/v5/pgxpool"

	apiauth "github.com/jeffotoni/jungle-backend-challenge/cmd/api/auth"
	appwager "github.com/jeffotoni/jungle-backend-challenge/internal/application/wagering"
	appwallet "github.com/jeffotoni/jungle-backend-challenge/internal/application/wallet"
)

type Routes struct {
	wallets *appwallet.Service
	wagers  *appwager.Service
	auth    *apiauth.Verifier
	pool    *pgxpool.Pool
}
