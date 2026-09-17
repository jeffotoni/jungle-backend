package handlers

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	apiauth "github.com/jeffotoni/jungle-backend/cmd/api/auth"
	appwager "github.com/jeffotoni/jungle-backend/internal/application/wagering"
	appwallet "github.com/jeffotoni/jungle-backend/internal/application/wallet"
)

type Routes struct {
	wallets       *appwallet.Service
	wagers        *appwager.Service
	auth          *apiauth.Verifier
	pool          *pgxpool.Pool
	sqs           SQSHealthClient
	queueURL      string
	healthTimeout time.Duration
}
