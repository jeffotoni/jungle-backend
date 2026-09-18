package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jeffotoni/log"

	appwager "github.com/jeffotoni/jungle-backend/internal/application/wagering"
	"github.com/jeffotoni/jungle-backend/internal/contracts"
	"github.com/jeffotoni/jungle-backend/internal/repository/postgres"
)

func TestIntegrationConsumerHappyPath(t *testing.T) {
	requireIntegration(t)
	env := newConsumerIntegrationEnv(t, false)
	walletID, playerID := createIntegrationWallet(t, env.pool, 10000)
	consumer := newIntegrationConsumer(env, t.Name())
	request := integrationWagerRequest(walletID, playerID, "happy")

	sendIntegrationMessage(t, env.client, env.queueURL, request, uuid.NewString(), uuid.NewString())
	runIntegrationConsumer(t, consumer, func() bool {
		return inboxCompleted(t, env.pool, consumer.consumerName) && queueEmpty(t, env.client, env.queueURL)
	})

	assertFinancialResult(t, env.pool, walletID, request.ExternalTransactionID, 9900, 1, 1)
	assertQueueEmpty(t, env.client, env.queueURL)
}

func TestIntegrationConsumerInboxRedelivery(t *testing.T) {
	requireIntegration(t)
	env := newConsumerIntegrationEnv(t, false)
	walletID, playerID := createIntegrationWallet(t, env.pool, 10000)
	consumer := newIntegrationConsumer(env, t.Name())
	request := integrationWagerRequest(walletID, playerID, "redelivery")
	messageID := uuid.NewString()

	sendIntegrationMessage(t, env.client, env.queueURL, request, messageID, uuid.NewString())
	sendIntegrationMessage(t, env.client, env.queueURL, request, messageID, uuid.NewString())
	runIntegrationConsumer(t, consumer, func() bool {
		return inboxCompleted(t, env.pool, consumer.consumerName) && queueEmpty(t, env.client, env.queueURL)
	})

	assertFinancialResult(t, env.pool, walletID, request.ExternalTransactionID, 9900, 1, 1)
	assertInboxCount(t, env.pool, consumer.consumerName, messageID, 1)
}

func TestIntegrationConsumerInvalidMessageReachesDLQ(t *testing.T) {
	requireIntegration(t)
	env := newConsumerIntegrationEnv(t, true)
	consumer := newIntegrationConsumer(env, t.Name())

	_, err := env.client.SendMessageWithContext(context.Background(), &sqs.SendMessageInput{
		QueueUrl:               aws.String(env.queueURL),
		MessageBody:            aws.String(`{"invalid":true}`),
		MessageGroupId:         aws.String("integration"),
		MessageDeduplicationId: aws.String(uuid.NewString()),
	})
	if err != nil {
		t.Fatal(err)
	}

	runIntegrationConsumer(t, consumer, func() bool {
		return queueHasMessage(t, env.client, env.dlqURL)
	})
}

func TestIntegrationHTTPAndSQSShareFinancialIdempotency(t *testing.T) {
	requireIntegration(t)
	apiURL := os.Getenv("INTEGRATION_API_URL")
	providerToken := os.Getenv("INTEGRATION_PROVIDER_TOKEN")
	if apiURL == "" || providerToken == "" {
		t.Skip("INTEGRATION_API_URL and INTEGRATION_PROVIDER_TOKEN are required")
	}
	env := newConsumerIntegrationEnv(t, false)
	walletID, playerID := createIntegrationWallet(t, env.pool, 10000)
	consumer := newIntegrationConsumer(env, t.Name())
	request := integrationWagerRequest(walletID, playerID, "cross-channel")

	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest, err := http.NewRequest(http.MethodPost, strings.TrimRight(apiURL, "/")+"/wagering/transactions", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatal(err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+providerToken)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Idempotency-Key", request.IdempotencyKey)
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(httpRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("HTTP status=%d want=%d", response.StatusCode, http.StatusOK)
	}

	sendIntegrationMessage(t, env.client, env.queueURL, request, uuid.NewString(), uuid.NewString())
	runIntegrationConsumer(t, consumer, func() bool {
		return inboxCompleted(t, env.pool, consumer.consumerName)
	})

	assertFinancialResult(t, env.pool, walletID, request.ExternalTransactionID, 9900, 1, 1)
}

type consumerIntegrationEnv struct {
	pool     *pgxpool.Pool
	client   *sqs.SQS
	queueURL string
	dlqURL   string
}

func requireIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("JUNGLE_INTEGRATION") != "1" {
		t.Skip("set JUNGLE_INTEGRATION=1 to run integration tests")
	}
}

func newConsumerIntegrationEnv(t *testing.T, withDLQ bool) consumerIntegrationEnv {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	databaseURL := envValue("DATABASE_URL", "postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable")
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Skipf("PostgreSQL unavailable: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("PostgreSQL unavailable: %v", err)
	}

	awsConfig := aws.NewConfig().WithRegion(envValue("AWS_REGION", "us-east-1"))
	endpoint := envValue("SQS_ENDPOINT_URL", "http://localhost:4566")
	awsConfig = awsConfig.WithEndpoint(endpoint).
		WithCredentials(credentials.NewStaticCredentials("test", "test", ""))
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		t.Skipf("SQS unavailable: %v", err)
	}
	client := sqs.New(sess)
	name := "jungle-test-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	queueURL := createFIFOQueue(t, ctx, client, name+".fifo", nil)
	var dlqURL string
	if withDLQ {
		dlqURL = createFIFOQueue(t, ctx, client, name+"-dlq.fifo", nil)
		dlqARN := queueAttribute(t, ctx, client, dlqURL, "QueueArn")
		policy, err := json.Marshal(map[string]string{
			"deadLetterTargetArn": dlqARN,
			"maxReceiveCount":     "2",
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.SetQueueAttributesWithContext(ctx, &sqs.SetQueueAttributesInput{
			QueueUrl: aws.String(queueURL),
			Attributes: map[string]*string{
				"RedrivePolicy": aws.String(string(policy)),
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = client.DeleteQueueWithContext(context.Background(), &sqs.DeleteQueueInput{QueueUrl: aws.String(queueURL)})
		if dlqURL != "" {
			_, _ = client.DeleteQueueWithContext(context.Background(), &sqs.DeleteQueueInput{QueueUrl: aws.String(dlqURL)})
		}
	})
	return consumerIntegrationEnv{pool: pool, client: client, queueURL: queueURL, dlqURL: dlqURL}
}

func createFIFOQueue(t *testing.T, ctx context.Context, client *sqs.SQS, name string, redrive *string) string {
	t.Helper()
	attributes := map[string]*string{
		"FifoQueue":                 aws.String("true"),
		"ContentBasedDeduplication": aws.String("false"),
		"VisibilityTimeout":         aws.String("1"),
	}
	if redrive != nil {
		attributes["RedrivePolicy"] = redrive
	}
	output, err := client.CreateQueueWithContext(ctx, &sqs.CreateQueueInput{
		QueueName:  aws.String(name),
		Attributes: attributes,
	})
	if err != nil {
		t.Skipf("SQS unavailable: %v", err)
	}
	return aws.StringValue(output.QueueUrl)
}

func queueAttribute(t *testing.T, ctx context.Context, client *sqs.SQS, queueURL, name string) string {
	t.Helper()
	output, err := client.GetQueueAttributesWithContext(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []*string{aws.String(name)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return aws.StringValue(output.Attributes[name])
}

func newIntegrationConsumer(env consumerIntegrationEnv, name string) *Consumer {
	store := postgres.NewStore(env.pool)
	tx := postgres.NewTxManager(env.pool)
	return &Consumer{
		client:           env.client,
		tx:               tx,
		inbox:            store,
		wagers:           appwager.NewService(tx, store, store),
		logger:           log.New(log.Config{Level: log.DEBUG, ServiceName: "consumer-test"}),
		queueURL:         env.queueURL,
		consumerName:     "consumer-test-" + strings.ReplaceAll(name, "/", "-"),
		visibilitySecond: 1,
		waitSecond:       1,
		maxMessages:      1,
		retryBase:        100 * time.Millisecond,
		retryMax:         time.Second,
		done:             make(chan struct{}),
	}
}

func runIntegrationConsumer(t *testing.T, consumer *Consumer, condition func() bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	go consumer.run(ctx)
	t.Cleanup(func() {
		cancel()
		select {
		case <-consumer.done:
		case <-time.After(5 * time.Second):
			t.Error("consumer did not stop")
		}
	})
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("integration condition was not reached")
}

func sendIntegrationMessage(t *testing.T, client *sqs.SQS, queueURL string, request contracts.WagerRequest, messageID, deduplicationID string) {
	t.Helper()
	body, err := json.Marshal(messageEnvelope{
		MessageID:  messageID,
		Type:       "WagerTransactionRequested",
		OccurredAt: time.Now().UTC(),
		Data:       request,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendMessageWithContext(context.Background(), &sqs.SendMessageInput{
		QueueUrl:               aws.String(queueURL),
		MessageBody:            aws.String(string(body)),
		MessageGroupId:         aws.String("integration"),
		MessageDeduplicationId: aws.String(deduplicationID),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func integrationWagerRequest(walletID, playerID, suffix string) contracts.WagerRequest {
	id := uuid.NewString()
	return contracts.WagerRequest{
		ProviderID:            "provider-a",
		ExternalTransactionID: "integration-external-" + suffix + "-" + id,
		IdempotencyKey:        "integration-key-" + suffix + "-" + id,
		WalletID:              walletID,
		PlayerID:              playerID,
		RoundID:               "integration-round-" + id,
		GameID:                "integration-game",
		Kind:                  "BET",
		Money:                 contracts.MoneyInput{Amount: "1.00", Currency: "BRL"},
	}
}

func createIntegrationWallet(t *testing.T, pool *pgxpool.Pool, balance int64) (string, string) {
	t.Helper()
	walletID := uuid.NewString()
	playerID := "integration-player-" + walletID
	_, err := pool.Exec(context.Background(), `
		INSERT INTO wallets (id, player_id, balance, currency, version)
		VALUES ($1, $2, $3, 'BRL', 1)`, walletID, playerID, balance)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM outbox_events WHERE aggregate_id = $1`, walletID)
		_, _ = pool.Exec(ctx, `DELETE FROM inbox_messages WHERE consumer_name LIKE 'consumer-test-%'`)
		_, _ = pool.Exec(ctx, `DELETE FROM ledger_entries WHERE wallet_id = $1`, walletID)
		_, _ = pool.Exec(ctx, `DELETE FROM wager_transactions WHERE wallet_id = $1`, walletID)
		_, _ = pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)
	})
	return walletID, playerID
}

func inboxCompleted(t *testing.T, pool *pgxpool.Pool, consumerName string) bool {
	t.Helper()
	var completed bool
	if err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM inbox_messages
			WHERE consumer_name = $1 AND status = 'COMPLETED')`, consumerName).Scan(&completed); err != nil {
		return false
	}
	return completed
}

func assertFinancialResult(t *testing.T, pool *pgxpool.Pool, walletID, externalID string, wantBalance int64, wantWagers, wantLedger int) {
	t.Helper()
	var balance int64
	if err := pool.QueryRow(context.Background(), `SELECT balance FROM wallets WHERE id = $1`, walletID).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != wantBalance {
		t.Fatalf("balance=%d want=%d", balance, wantBalance)
	}
	var wagers, ledger int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM wager_transactions WHERE wallet_id = $1 AND external_transaction_id = $2`, walletID, externalID).Scan(&wagers); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM ledger_entries WHERE wallet_id = $1 AND amount = 100`, walletID).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if wagers != wantWagers || ledger != wantLedger {
		t.Fatalf("wagers=%d ledger=%d want=%d,%d", wagers, ledger, wantWagers, wantLedger)
	}
}

func assertInboxCount(t *testing.T, pool *pgxpool.Pool, consumerName, messageID string, want int) {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM inbox_messages WHERE consumer_name = $1 AND message_id = $2`, consumerName, messageID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("inbox rows=%d want=%d", count, want)
	}
}

func assertQueueEmpty(t *testing.T, client *sqs.SQS, queueURL string) {
	t.Helper()
	if !queueEmpty(t, client, queueURL) {
		t.Fatal("queue still contains a message")
	}
}

func queueEmpty(t *testing.T, client *sqs.SQS, queueURL string) bool {
	t.Helper()
	output, err := client.GetQueueAttributesWithContext(context.Background(), &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []*string{aws.String("ApproximateNumberOfMessages")},
	})
	if err != nil {
		return false
	}
	return aws.StringValue(output.Attributes["ApproximateNumberOfMessages"]) == "0"
}

func queueHasMessage(t *testing.T, client *sqs.SQS, queueURL string) bool {
	t.Helper()
	output, err := client.GetQueueAttributesWithContext(context.Background(), &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []*string{aws.String("ApproximateNumberOfMessages")},
	})
	if err != nil {
		return false
	}
	return aws.StringValue(output.Attributes["ApproximateNumberOfMessages"]) != "0"
}

func envValue(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
