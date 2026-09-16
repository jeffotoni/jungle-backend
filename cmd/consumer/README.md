# Consumer

The Consumer is the asynchronous worker responsible for reading wagering requests from the SQS FIFO queue and forwarding them to the same Application and Domain use cases used by the HTTP API.

The Consumer does not duplicate BET, WIN, LOSS, REFUND or ROLLBACK rules. It also does not apply financial changes in memory. The durable transaction is executed by PostgreSQL, and the SQS message is deleted only after the transaction commits successfully.

## Execution flow

### SQS processing flow

```text
SQS FIFO
wager-transactions.fifo
        |
        v
cmd/consumer
        |
        v
ReceiveMessage
        |
        v
Parse envelope
        |
        v
Inbox
(consumerName + messageId)
        |
        +--> already processed -> do not execute again
        |
        v
Application / Domain
        |
        v
PostgreSQL transaction
        |
        +--> persistent idempotency
        +--> wallet row lock
        +--> process BET/WIN/LOSS/REFUND/ROLLBACK
        +--> wager_transaction
        +--> ledger when applicable
        +--> outbox
        +--> inbox completion
        |
        v
COMMIT
        |
        v
DeleteMessage SQS
```

The message remains available for SQS retry when parsing, processing or deletion fails. Permanent failures are not deleted and are eventually handled by the queue redrive policy and DLQ.

### Shared API and SQS application flow

```text
HTTP API                       SQS Consumer
    |                               |
    v                               v
Handler                       SQS message
    |                               |
    +---------------+---------------+
                    |
                    v
            SAME Application
                    |
                    v
                 Domain
                    |
                    v
               PostgreSQL
```

The HTTP API and the Consumer share the same Application/Domain behavior. Transport concerns remain at the edges: HTTP authentication and request parsing belong to the API, while envelope parsing, SQS polling and Inbox handling belong to the Consumer.

## Local prerequisites

Run these commands from the repository root.

### 1. Start PostgreSQL, Keycloak and LocalStack

```bash
docker compose up -d
docker compose ps
```

Wait for the dependencies:

```bash
until docker compose exec -T postgres pg_isready -U jungle -d jungle; do
  sleep 2
done

until curl -fsS http://localhost:4566/_localstack/health >/dev/null; do
  sleep 2
done
```

LocalStack creates these queues through `scripts/localstack-init.sh`:

- `wager-transactions.fifo`
- `wager-transactions-dlq.fifo`
- `jungle-wager`
- `jungle-events`

The wagering queue is FIFO and has a redrive policy to `wager-transactions-dlq.fifo` after five receives.

### 2. Apply the PostgreSQL migrations

Apply all migrations in order before starting the Consumer:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  < migrations/000001_init.up.sql

docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  < migrations/000002_api_financial_guarantees.up.sql

docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  < migrations/000003_inbox_consumer_identity.up.sql
```

Confirm the tables:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "\dt"
```

The required tables are:

- `wallets`
- `wager_transactions`
- `ledger_entries`
- `inbox_messages`
- `outbox_events`

### 3. Select a wallet for the test

The message must use an existing wallet and the matching player. Create one through the API or use an existing UUID:

```bash
export WALLET_ID="<wallet-id>"
export PLAYER_ID="player-001"
echo "$WALLET_ID"
```

To inspect available wallets:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT id, player_id, balance, currency, version FROM wallets ORDER BY created_at;"
```

## Consumer configuration

The following values are the local defaults. Export them explicitly when running the Consumer so the complete runtime configuration is visible:

```bash
export DATABASE_URL="postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable"
export AWS_REGION="us-east-1"
export SQS_ENDPOINT_URL="http://localhost:4566"
export SQS_WAGER_QUEUE_URL="http://localhost:4566/000000000000/wager-transactions.fifo"
export VISIBILITY_TIMEOUT="30s"
export SQS_WAIT_TIME="20s"
export SQS_MAX_MESSAGES="10"
export CONSUMER_NAME="wager-consumer"
export LOG_LEVEL="DEBUG"
export TRACE_ID="traceId"
```

Configuration is loaded from environment variables in `cmd/consumer/config`. The default log level is `DEBUG`, so the Consumer shows receive and delete diagnostics in addition to startup, processing and error logs.

## Start the Consumer

Run it in the foreground:

```bash
go run ./cmd/consumer
```

With an empty queue, the Consumer stays in SQS long polling and normally prints only the startup log. When a message is received, the structured JSON logs include the action, message identifier, transaction identifier, status, queue and consumer name when applicable.

Typical actions are:

- `startup`: Consumer started and configuration accepted.
- `receive`: one or more messages were returned by SQS.
- `process`: message committed through Application/Domain and PostgreSQL.
- `duplicate`: Inbox found the same `consumerName + messageId` and skipped it.
- `delete`: message was removed from SQS after the durable commit.
- `process`, `receive` or `delete` with `level=ERROR`: the message remains available for retry unless SQS moves it to the DLQ.

## Send test messages

Use the local script to send dynamic wagering envelopes to the FIFO queue. It generates unique identifiers for each logical message and can send the same envelope more than once to verify Inbox deduplication.

### Send one BET three times

```bash
WALLET_ID="$WALLET_ID" \
PLAYER_ID="$PLAYER_ID" \
MESSAGE_COUNT=1 \
DUPLICATE_COUNT=3 \
INTERVAL_SECONDS=0 \
AMOUNT=1.00 \
./scripts/send-wager-messages.sh
```

The script sends one logical envelope three times. The copies keep the same envelope `messageId`, `idempotencyKey` and business payload, while using different SQS deduplication identifiers so the Consumer can receive the repeated deliveries. Only one financial effect must be applied.

Other useful parameters:

- `MESSAGE_COUNT`: number of logical messages; default `2`.
- `DUPLICATE_COUNT`: number of copies per logical message; default `1`.
- `INTERVAL_SECONDS`: delay between logical messages; default `30`.
- `AMOUNT`: money amount as a string; default `5.00`.
- `KIND`: `BET`, `WIN`, `LOSS`, `REFUND` or `ROLLBACK`; default `BET`.
- `CURRENCY`: ISO currency; default `BRL`.
- `PROVIDER_ID`: provider identity; default `provider-a`.
- `SQS_WAGER_QUEUE_URL`: optional explicit queue URL.

The script requires `WALLET_ID` and uses `aws` CLI against LocalStack. For a different wallet, set `PLAYER_ID` to the wallet's `player_id`.

## Validate processing in PostgreSQL

After sending a message, use the identifier printed by the script to narrow the queries. For example:

```bash
export EXTERNAL_TRANSACTION_ID="sqs-transaction-<identifier>"
```

### Inbox

There should be one Inbox record for the envelope, even when the same message is delivered repeatedly:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT consumer_name, message_id, status, processed_at, created_at
      FROM inbox_messages
      WHERE consumer_name = 'wager-consumer'
      ORDER BY created_at DESC
      LIMIT 5;"
```

Expected successful status: `COMPLETED`.

### Wager transaction and idempotency

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT id, provider_id, external_transaction_id, idempotency_key,
      status, amount, result_balance
      FROM wager_transactions
      WHERE external_transaction_id = '$EXTERNAL_TRANSACTION_ID';"
```

For duplicate delivery, exactly one row should exist for the same provider and external transaction identity:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT COUNT(*) AS transaction_count
      FROM wager_transactions
      WHERE external_transaction_id = '$EXTERNAL_TRANSACTION_ID';"
```

### Wallet balance

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT id, player_id, balance, currency, version
      FROM wallets
      WHERE id = '$WALLET_ID';"
```

For a `BET`, the balance decreases exactly once. For a `LOSS`, it remains unchanged.

### Ledger

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT id, wager_transaction_id, direction, amount,
      balance_before, balance_after, created_at
      FROM ledger_entries
      WHERE wager_transaction_id IN (
        SELECT id FROM wager_transactions
        WHERE external_transaction_id = '$EXTERNAL_TRANSACTION_ID'
      );"
```

A successful `BET`, `WIN`, `REFUND` or `ROLLBACK` creates one applicable ledger entry. `LOSS` and rejected operations do not create ledger entries.

### Outbox

The API/Consumer transaction stores events in the outbox but does not publish them. The Publisher will process these records later:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT id, aggregate_id, event_type, status, attempts,
      next_attempt_at, published_at, created_at
      FROM outbox_events
      WHERE aggregate_id = '$WALLET_ID'
      ORDER BY created_at DESC
      LIMIT 20;"
```

Successful processing normally creates the corresponding `WagerTransactionProcessed` and `WalletBalanceChanged` events. `LOSS` does not create `WalletBalanceChanged`.

## Duplicate delivery expectation

```text
First delivery
    |
    +--> Inbox INSERT PROCESSING
    +--> Application/Domain transaction
    +--> wallet + wager + ledger + outbox
    +--> Inbox COMPLETED
    +--> COMMIT
    +--> DeleteMessage

Repeated delivery
    |
    +--> Inbox finds consumerName + messageId
    +--> no Application/Domain execution
    +--> no wallet, ledger, wager or outbox mutation
    +--> DeleteMessage
```

The database is the source of truth for idempotency and financial correctness. Process-local state or an in-memory mutex is not used to protect duplicate or concurrent messages.
