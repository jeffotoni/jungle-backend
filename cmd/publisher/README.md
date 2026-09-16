# Publisher

The Publisher is the independent process responsible for reading committed events from the PostgreSQL transactional outbox and publishing them to the configured SQS event destination.

The API and Consumer write outbox records as part of their financial transactions. They do not publish directly to SQS. The Publisher only handles records that are already committed in `outbox_events`.

## Execution flow

### Transactional outbox flow

```text
API / Consumer
       |
       v
PostgreSQL transaction
       |
       +--> wallet mutation when applicable
       +--> wager transaction when applicable
       +--> ledger entry when applicable
       +--> outbox event INSERT
       |
       v
COMMIT
       |
       v
outbox_events
status = PENDING
       |
       v
cmd/publisher
       |
       v
Claim event
FOR UPDATE SKIP LOCKED
       |
       +--> status = PROCESSING
       +--> claim_token
       +--> lease_until
       |
       v
Publish event to SQS
       |
       +--> failure
       |      |
       |      +--> attempts = attempts + 1
       |      +--> status = PENDING
       |      +--> next_attempt_at
       |      +--> retry with exponential backoff
       |
       +--> success
              |
              +--> status = PUBLISHED
              +--> published_at
              +--> release claim and lease
```

The event is never published before the originating database transaction commits. If the Publisher crashes after claiming an event, another Publisher can recover it after `lease_until` expires.

### Multiple Publishers

```text
Publisher A                         Publisher B
     |                                   |
     v                                   v
PostgreSQL claim transaction       PostgreSQL claim transaction
     |                                   |
     +--> row A locked                  +--> SKIP LOCKED
     +--> row B locked                  +--> claims another row
     |                                   |
     v                                   v
Publish row A to SQS                 Publish row B to SQS
     |                                   |
     v                                   v
Mark row A PUBLISHED                 Mark row B PUBLISHED
```

Claiming is coordinated by PostgreSQL. Correctness does not depend on a process-local mutex or in-memory registry.

### API and Consumer to Publisher flow

```text
HTTP API                         SQS Consumer
    |                                 |
    v                                 v
Application / Domain            Application / Domain
    |                                 |
    +----------------+----------------+
                     |
                     v
             PostgreSQL transaction
                     |
                     +--> financial state
                     +--> ledger
                     +--> outbox_events
                     |
                     v
                  COMMIT
                     |
                     v
                 Publisher
                     |
                     v
                Output SQS
```

## Responsibilities

- Read committed `PENDING` outbox events.
- Claim records safely for multiple Publisher instances.
- Recover records abandoned by a crashed Publisher after lease expiration.
- Publish the immutable event payload to the configured SQS destination.
- Mark an event as `PUBLISHED` only after SQS confirms the send.
- Increment `attempts` and schedule `next_attempt_at` after a publish failure.
- Retry with capped exponential backoff.
- Preserve the event payload and event identity across retries.
- Stop polling and wait for the worker to finish during graceful shutdown.

The Publisher does not execute wallet or wagering rules and does not modify balances or ledger entries.

## Local prerequisites

Run these commands from the repository root.

### 1. Start PostgreSQL, Keycloak and LocalStack

```bash
docker compose up -d
docker compose ps
```

Wait for PostgreSQL and LocalStack:

```bash
until docker compose exec -T postgres pg_isready -U jungle -d jungle; do
  sleep 2
done

until curl -fsS http://localhost:4566/_localstack/health >/dev/null; do
  sleep 2
done
```

The local setup creates these queues:

- `wager-transactions.fifo`
- `wager-transactions-dlq.fifo`
- `jungle-wager`
- `jungle-events`

The default Publisher destination is the standard SQS queue `jungle-events`.

## Database migrations

### First-time database setup

Apply all migrations in order:

```bash
docker compose exec -T postgres psql \
  -v ON_ERROR_STOP=1 \
  -U jungle \
  -d jungle \
  < migrations/000001_init.up.sql

docker compose exec -T postgres psql \
  -v ON_ERROR_STOP=1 \
  -U jungle \
  -d jungle \
  < migrations/000002_api_financial_guarantees.up.sql

docker compose exec -T postgres psql \
  -v ON_ERROR_STOP=1 \
  -U jungle \
  -d jungle \
  < migrations/000003_inbox_consumer_identity.up.sql

docker compose exec -T postgres psql \
  -v ON_ERROR_STOP=1 \
  -U jungle \
  -d jungle \
  < migrations/000004_outbox_claim_lease.up.sql
```

If migrations `000001` through `000003` are already applied, apply only the Publisher migration:

```bash
docker compose exec -T postgres psql \
  -v ON_ERROR_STOP=1 \
  -U jungle \
  -d jungle \
  < migrations/000004_outbox_claim_lease.up.sql
```

The migration is safe to re-run because it uses `IF NOT EXISTS`. Existing columns and indexes produce notices and do not represent errors.

Confirm the outbox structure:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "\\d outbox_events"
```

The table must include:

- `id`
- `aggregate_type`
- `aggregate_id`
- `event_type`
- `payload`
- `status`
- `attempts`
- `next_attempt_at`
- `published_at`
- `claim_token`
- `lease_until`

The reversible migration is:

```bash
docker compose exec -T postgres psql \
  -v ON_ERROR_STOP=1 \
  -U jungle \
  -d jungle \
  < migrations/000004_outbox_claim_lease.down.sql
```

Run the down migration only when Publisher claim and lease columns are no longer needed. It removes `idx_outbox_claimable`, `claim_token` and `lease_until`.

## Publisher configuration

The following values are the local defaults. Export them explicitly when validating the process:

```bash
export DATABASE_URL="postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable"
export AWS_REGION="us-east-1"
export SQS_ENDPOINT_URL="http://localhost:4566"
export SQS_EVENTS_QUEUE_URL="http://localhost:4566/000000000000/jungle-events"
export OUTBOX_BATCH_SIZE="100"
export PUBLISHER_NAME="outbox-publisher"
export POLL_INTERVAL="1s"
export PUBLISHER_LEASE="30s"
export RETRY_BASE="1s"
export RETRY_MAX="1m"
export SQS_EVENT_GROUP_ID="outbox-events"
export LOG_LEVEL="DEBUG"
export TRACE_ID="traceId"
```

All configuration is loaded from environment variables in `cmd/publisher/config`. The default log level is `DEBUG`.

`SQS_EVENT_GROUP_ID` and `MessageDeduplicationId` are used when `SQS_EVENTS_QUEUE_URL` points to a FIFO queue. The local `jungle-events` queue is standard, so FIFO-only message fields are not sent to it.

## Start the Publisher

Run it in the foreground:

```bash
go run ./cmd/publisher
```

The Publisher validates the configured event queue before starting its polling loop. With no pending events, it remains running and waits according to `POLL_INTERVAL`.

The Publisher uses Uber Fx for dependency composition and lifecycle management:

- PostgreSQL pool starts before the worker and is closed after shutdown.
- SQS queue availability is checked during startup.
- The worker receives a cancellation context during shutdown.
- The process waits for the publishing loop to stop.

## Generate an outbox event through the API

The Publisher has no HTTP endpoint. To create an event for testing, use the API to create a wallet and process a wagering operation.

Configure the API values:

```bash
export API="http://localhost:8080"
export OIDC_ISSUER="http://localhost:8081/realms/jungle"
export OIDC_AUDIENCE="jungle-api"
export KEYCLOAK_TOKEN_URL="$OIDC_ISSUER/protocol/openid-connect/token"
export INTERNAL_CLIENT_ID="wallet-internal"
export INTERNAL_CLIENT_SECRET="INTERNAL_CLIENT_SECRET"
export PROVIDER_CLIENT_ID="provider-a"
export PROVIDER_CLIENT_SECRET="PROVIDER_CLIENT_SECRET"
```

Obtain the Keycloak tokens:

```bash
export INTERNAL_TOKEN="$(curl -fsS -X POST "$KEYCLOAK_TOKEN_URL" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "grant_type=client_credentials" \
  --data-urlencode "client_id=$INTERNAL_CLIENT_ID" \
  --data-urlencode "client_secret=$INTERNAL_CLIENT_SECRET" \
  | jq -r '.access_token')"

export PROVIDER_TOKEN="$(curl -fsS -X POST "$KEYCLOAK_TOKEN_URL" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "grant_type=client_credentials" \
  --data-urlencode "client_id=$PROVIDER_CLIENT_ID" \
  --data-urlencode "client_secret=$PROVIDER_CLIENT_SECRET" \
  | jq -r '.access_token')"
```

Create a wallet with an initial balance:

```bash
export PLAYER_ID="player-publisher-$(uuidgen | tr '[:upper:]' '[:lower:]')"
```

```bash
curl -i -X POST "$API/wallets" \
  -H "Authorization: Bearer $INTERNAL_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "playerId": "'"$PLAYER_ID"'",
    "initialBalance": {
      "amount": "25.00",
      "currency": "BRL"
    }
  }'
```

Save the returned wallet ID:

```bash
export WALLET_ID="<wallet-id-returned-by-the-api>"
```

Process a BET to create `WagerTransactionProcessed` and `WalletBalanceChanged` records:

```bash
export EXTERNAL_TRANSACTION_ID="publisher-bet-$(uuidgen | tr '[:upper:]' '[:lower:]')"
export IDEMPOTENCY_KEY="publisher-key-$(uuidgen | tr '[:upper:]' '[:lower:]')"
```

```bash
curl -i -X POST "$API/wagering/transactions" \
  -H "Authorization: Bearer $PROVIDER_TOKEN" \
  -H "Idempotency-Key: $IDEMPOTENCY_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "providerId": "provider-a",
    "externalTransactionId": "'"$EXTERNAL_TRANSACTION_ID"'",
    "walletId": "'"$WALLET_ID"'",
    "playerId": "'"$PLAYER_ID"'",
    "roundId": "publisher-round-001",
    "gameId": "publisher-game-001",
    "kind": "BET",
    "money": {
      "amount": "1.00",
      "currency": "BRL"
    }
  }'
```

The API commits the wallet change, wager transaction, ledger entry and outbox events before returning successfully.

## Validate before publishing

Immediately after the API operation and before starting the Publisher, query the event:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT id, aggregate_id, event_type, status, attempts,
      next_attempt_at, published_at, claim_token, lease_until
      FROM outbox_events
      WHERE aggregate_id = '$WALLET_ID'
      ORDER BY created_at DESC
      LIMIT 10;"
```

Expected state:

```text
status = PENDING
attempts = 0
published_at = NULL
```

## Validate after publishing

After the Publisher logs a successful publication:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT id, aggregate_id, event_type, status, attempts,
      next_attempt_at, published_at, claim_token, lease_until
      FROM outbox_events
      WHERE aggregate_id = '$WALLET_ID'
      ORDER BY created_at DESC
      LIMIT 10;"
```

Expected state:

```text
status = PUBLISHED
attempts = 0
published_at is not NULL
claim_token = NULL
lease_until = NULL
```

Check the event queue:

```bash
docker compose exec -T localstack awslocal sqs get-queue-attributes \
  --queue-url "http://localhost:4566/000000000000/jungle-events" \
  --attribute-names ApproximateNumberOfMessages \
  --query 'Attributes.ApproximateNumberOfMessages' \
  --output text
```

Inspect one event message:

```bash
docker compose exec -T localstack awslocal sqs receive-message \
  --queue-url "http://localhost:4566/000000000000/jungle-events" \
  --max-number-of-messages 1 \
  --wait-time-seconds 1
```

The message body is the immutable event envelope stored in the outbox payload. It includes the event identity, event type, aggregate, correlation metadata, occurrence timestamp, version and typed data.

## Logs

With `LOG_LEVEL=DEBUG`, the Publisher emits structured JSON logs such as:

```json
{"level":"INFO","msg":"publisher started","service":"publisher","component":"outbox","action":"startup","queueUrl":"http://localhost:4566/000000000000/jungle-events","publisherName":"outbox-publisher"}
```

```json
{"level":"INFO","msg":"outbox event published","service":"publisher","component":"outbox","action":"publish","eventId":"<outbox-id>","eventType":"WagerTransactionProcessed","messageId":"<sqs-message-id>"}
```

On a failed send, the log contains the event ID, event type, error, attempt number and next retry time:

```json
{"level":"WARN","msg":"outbox event publish failed","service":"publisher","component":"outbox","action":"retry","eventId":"<outbox-id>","attempts":1,"nextAttemptAt":"<rfc3339-timestamp>"}
```

## Retry and abandoned work

When SQS publication fails:

```text
PROCESSING
    |
    +--> attempts + 1
    +--> next_attempt_at = now + retry delay
    +--> status = PENDING
    +--> claim_token = NULL
    +--> lease_until = NULL
    |
    v
Eligible for a later claim
```

The retry delay is capped exponential backoff:

```text
attempt 0 -> RETRY_BASE
attempt 1 -> RETRY_BASE * 2
attempt 2 -> RETRY_BASE * 4
...
maximum  -> RETRY_MAX
```

If the Publisher stops after claiming but before completing the publish state update, the row remains `PROCESSING` until `lease_until`. A later Publisher can claim it after the lease expires. If SQS accepted the event before the database update failed, republishing is possible; this is intentional at-least-once behavior.

## FIFO output destinations

The local output queue `jungle-events` is standard SQS. If the configured event destination ends in `.fifo`, the Publisher adds:

- `MessageGroupId`: `SQS_EVENT_GROUP_ID`.
- `MessageDeduplicationId`: the stable outbox event ID.

The event body remains unchanged in both cases.

## Shutdown

Stop the foreground process with `Ctrl+C` or send `SIGTERM`:

```bash
kill -TERM <publisher-pid>
```

The Fx lifecycle cancels the polling loop, stops new claims and waits for the worker to terminate. An event that is not marked `PUBLISHED` remains recoverable through its lease and is safe to publish again.

## Troubleshooting

### Publisher exits with `SQS event queue unavailable`

Check LocalStack and the configured queue URL:

```bash
docker compose ps

docker compose exec -T localstack awslocal sqs list-queues
```

### No publication log appears

Check that:

- PostgreSQL migrations are applied through `000004`.
- `outbox_events` contains rows with `status = 'PENDING'`.
- `next_attempt_at` is null or in the past.
- The Publisher points to the same database used by the API.
- `SQS_EVENTS_QUEUE_URL` points to the existing event queue.
- `LOG_LEVEL=DEBUG` or `INFO` is configured.

An idle Publisher normally prints its startup log and waits without repeatedly printing empty polling cycles.

### An event remains `PROCESSING`

Inspect its lease:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT id, status, claim_token, lease_until, attempts
      FROM outbox_events
      WHERE status = 'PROCESSING'
      ORDER BY created_at;"
```

After `lease_until` passes, another Publisher instance can recover the event.
