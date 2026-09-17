package fxmodules

import (
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jeffotoni/log"
	"go.uber.org/fx"

	apiauth "github.com/jeffotoni/jungle-backend/cmd/api/auth"
	apiconfig "github.com/jeffotoni/jungle-backend/cmd/api/config"
	"github.com/jeffotoni/jungle-backend/cmd/api/handlers"
	"github.com/jeffotoni/jungle-backend/internal/application/ports"
	appwager "github.com/jeffotoni/jungle-backend/internal/application/wagering"
	appwallet "github.com/jeffotoni/jungle-backend/internal/application/wallet"
	"github.com/jeffotoni/jungle-backend/internal/platform/httpserver"
	"github.com/jeffotoni/jungle-backend/internal/repository/postgres"
)

var API = fx.Module(
	"api",
	fx.Provide(
		func() *log.Logger {
			return log.New(log.Config{
				Format:      log.FormatJSON,
				Level:       log.Level(apiconfig.LOG_LEVEL),
				ServiceName: "api",
				TraceIDKey:  apiconfig.TRACE_ID,
			})
		},
		fx.Annotate(
			func() string { return apiconfig.HTTP_ADDR },
			fx.ResultTags(`name:"httpAddress"`),
		),
		func() httpserver.TraceKey {
			return httpserver.TraceKey(apiconfig.TRACE_ID)
		},
		func() bool {
			level := strings.ToUpper(strings.TrimSpace(apiconfig.LOG_LEVEL))
			return level == string(log.DEBUG) || level == string(log.TRACE)
		},
		fx.Annotate(
			func() string { return apiconfig.SQS_WAGER_QUEUE_URL },
			fx.ResultTags(`name:"sqsQueueURL"`),
		),
		func() time.Duration { return apiconfig.SQS_HEALTH_TIMEOUT },
		func() handlers.SQSHealthClient {
			client, err := newAPIHealthSQSClient()
			if err != nil {
				return nil
			}
			return client
		},
		func(lc fx.Lifecycle) (*pgxpool.Pool, error) { return postgres.NewPool(lc, apiconfig.DATABASE_URL) },
		postgres.NewStore,
		func(store *postgres.Store) ports.WalletStore { return store },
		func(store *postgres.Store) ports.WagerStore { return store },
		postgres.NewTxManager,
		func() *apiauth.Verifier {
			return apiauth.NewVerifier(apiconfig.OIDC_ISSUER, apiconfig.OIDC_AUDIENCE, apiconfig.OIDC_INTERNAL_ROLE)
		},
		appwallet.NewService,
		appwager.NewService,
		fx.Annotate(
			handlers.NewRouter,
			fx.ParamTags("", "", "", "", "", `name:"sqsQueueURL"`, ""),
		),
		fx.Annotate(
			httpserver.NewServer,
			fx.ParamTags("", `name:"httpAddress"`, "", "", "", ""),
		),
	),
	fx.Invoke(func(*httpserver.Server) {}),
)

func newAPIHealthSQSClient() (*sqs.SQS, error) {
	awsConfig := aws.NewConfig().WithRegion(apiconfig.AWS_REGION)
	if apiconfig.SQS_ENDPOINT_URL != "" {
		awsConfig = awsConfig.WithEndpoint(apiconfig.SQS_ENDPOINT_URL).
			WithCredentials(credentials.NewStaticCredentials("test", "test", ""))
	}
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, err
	}
	return sqs.New(sess), nil
}
