# Reference Worker

The Reference Worker is the independent process responsible for resuming wagering transactions that were persisted as `PENDING_REFERENCE` because a required external reference was not available yet.

It uses the same Application and Domain use cases as the HTTP API and SQS Consumer. It does not contain a second implementation of `REFUND` or `ROLLBACK`, does not consume SQS, and does not publish directly to SQS. Events are written to the transactional outbox and are later published by `cmd/publisher`.

## Execution flow

```text
wager_transactions
status = PENDING_REFERENCE
        |
        v
cmd/reference-worker
        |
        v
Claim eligible rows
FOR UPDATE SKIP LOCKED
        |
        v
Same Application / Domain
        |
        +--> reference found
        |       |
        |       v
        |   Validate provider, player, wallet,
        |   currency, round and amount
        |       |
        |       v
        |   Process REFUND/ROLLBACK
        |       |
        |       +--> wallet mutation when applicable
        |       +--> ledger entry
        |       +--> wager transaction = PROCESSED
        |       +--> outbox events
        |
        +--> reference not found
                |
                v
        attempts + 1
        reference_next_attempt_at
                |
                +--> within TTL and attempts limit
                |       -> retry with exponential backoff
                |
                +--> expired or attempts exhausted
                        |
                        v
                    REJECTED
                    failureCode = REFERENCE_NOT_FOUND
                    WagerTransactionRejected in outbox
```

Each selected record and its financial continuation are handled in one PostgreSQL transaction. A successful resolution commits the wager state, wallet mutation, ledger entry and outbox events together. A retry also commits its attempt metadata durably.

## Shared processing flow

```text
HTTP API                         SQS Consumer
    |                                 |
    v                                 v
Wager request                    SQS data
    |                                 |
    +----------------+----------------+
                     |
                     v
              SAME Application
                     |
                     v
                   Domain
                     |
                     v
                PostgreSQL
                     ^
                     |
              Reference Worker
```

The Reference Worker continues a transaction already accepted by the API or Consumer. It never changes the business rules or bypasses PostgreSQL coordination.

## Responsibilities

- Find eligible `PENDING_REFERENCE` transactions.
- Coordinate multiple worker instances with PostgreSQL row locking.
- Resolve references by `(providerId, referenceExternalTransactionId)`.
- Resume `REFUND` and `ROLLBACK` through the shared Application/Domain path.
- Persist `reference_attempts`, `reference_next_attempt_at` and `reference_pending_at`.
- Retry missing references with capped exponential backoff.
- Recover pending work after a process restart.
- Finalize expired or exhausted references as `REJECTED`.
- Persist the stable `REFERENCE_NOT_FOUND` failure code.
- Write `WagerTransactionRejected` to the transactional outbox.
- Stop polling and wait for the worker goroutine during graceful shutdown.

The worker does not modify the wallet when a reference is missing or when the pending operation expires. Rejected operations do not create ledger entries.

If a reference exists but is not `PROCESSED`, the shared Application rejects the pending operation with the corresponding reference failure code. It is not retried as if the reference were missing.

## Local prerequisites

Run these commands from the repository root.

Start PostgreSQL:

```bash
docker compose up -d postgres
docker compose ps
```

The base schema must already exist. For a new local database, apply the migrations in order:

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

docker compose exec -T postgres psql \
  -v ON_ERROR_STOP=1 \
  -U jungle \
  -d jungle \
  < migrations/000005_pending_reference_retry.up.sql
```

For an existing database, the required migration is:

```bash
docker compose exec -T postgres psql \
  -v ON_ERROR_STOP=1 \
  -U jungle \
  -d jungle \
  < migrations/000005_pending_reference_retry.up.sql
```

Confirm the worker metadata:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT column_name FROM information_schema.columns WHERE table_name = 'wager_transactions' AND column_name IN ('reference_attempts', 'reference_next_attempt_at', 'reference_pending_at') ORDER BY column_name;"
```

The reversible migration is:

```bash
docker compose exec -T postgres psql \
  -v ON_ERROR_STOP=1 \
  -U jungle \
  -d jungle \
  < migrations/000005_pending_reference_retry.down.sql
```

Only run the down migration when the Reference Worker is no longer using these columns.

## Configuration

The worker reads only environment variables. These are the local defaults:

```bash
export DATABASE_URL="postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable"
export POLL_INTERVAL="1s"
export REFERENCE_BATCH_SIZE="10"
export REFERENCE_TTL="15m"
export REFERENCE_MAX_ATTEMPTS="10"
export REFERENCE_RETRY_BASE="1s"
export REFERENCE_RETRY_MAX="1m"
export LOG_LEVEL="DEBUG"
export TRACE_ID="traceId"
```

`POLL_INTERVAL` is used when no eligible record exists. A batch with pending records is processed and the worker immediately checks for more work. Retry timing is persisted in PostgreSQL through `reference_next_attempt_at`.

## Start the worker

Run it in the foreground:

```bash
go run ./cmd/reference-worker
```

Expected startup log:

```json
{"level":"INFO","msg":"reference worker started","service":"reference-worker","component":"reference-worker","action":"startup"}
```

For a pending operation that is retried:

```json
{"level":"INFO","msg":"pending reference will be retried","service":"reference-worker","action":"retry","status":"PENDING_REFERENCE"}
```

For an expired or exhausted operation:

```json
{"level":"INFO","msg":"pending reference processed","service":"reference-worker","status":"REJECTED","failureCode":"REFERENCE_NOT_FOUND"}
```

Press `Ctrl+C` to stop. Fx cancels the polling context and waits for the worker to finish.

## Create a pending reference through the API

The API creates a pending reference when a `REFUND` or `ROLLBACK` refers to an external transaction that does not exist yet. Use the authentication and wallet variables documented in [`cmd/api/README.md`](../api/README.md).

Example `REFUND` request:

```bash
curl -i -X POST "$API/wagering/transactions" \
  -H "Authorization: Bearer $PROVIDER_TOKEN" \
  -H "Idempotency-Key: refund-pending-001" \
  -H "Content-Type: application/json" \
  -d '{
    "providerId": "provider-a",
    "externalTransactionId": "refund-pending-ext-001",
    "walletId": "'$WALLET_ID'",
    "playerId": "player-001",
    "roundId": "round-001",
    "gameId": "game-001",
    "kind": "REFUND",
    "money": {
      "amount": "10.00",
      "currency": "BRL"
    },
    "referenceExternalTransactionId": "bet-not-created-yet-001"
  }'
```

The response is `PENDING_REFERENCE`. The transaction can then be inspected with:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT id, status, reference_external_transaction_id, reference_attempts, reference_next_attempt_at, reference_pending_at, failure_code FROM wager_transactions WHERE external_transaction_id = 'refund-pending-ext-001';"
```

When the referenced transaction becomes available and is processed, the Reference Worker resolves the pending operation on a later attempt. The resulting wallet change and events are committed atomically.

## Validate processing

Inspect pending, processed and rejected references:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT id, provider_id, external_transaction_id, kind, status, reference_external_transaction_id, reference_transaction_id, reference_attempts, reference_next_attempt_at, failure_code FROM wager_transactions WHERE kind IN ('REFUND', 'ROLLBACK') ORDER BY created_at DESC;"
```

Inspect rejection events waiting for the Publisher:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT id, aggregate_id, event_type, status, created_at FROM outbox_events WHERE event_type = 'WagerTransactionRejected' ORDER BY created_at DESC;"
```

The Reference Worker writes events to the outbox with `status = PENDING`. Start [`cmd/publisher`](../publisher/README.md) to publish them to the configured SQS event destination.

## Retry and expiration policy

The default policy is:

```text
reference_attempts = 0
REFERENCE_RETRY_BASE = 1s
REFERENCE_RETRY_MAX = 1m
REFERENCE_MAX_ATTEMPTS = 10
REFERENCE_TTL = 15m
```

The first missing-reference retry is scheduled after the base delay. The delay doubles until it reaches `REFERENCE_RETRY_MAX`. The transaction is rejected when the TTL expires or the maximum number of attempts is reached, whichever happens first.

All retry metadata is stored in PostgreSQL. A restart does not reset attempts or make the operation dependent on process memory. Multiple worker instances use row locks and `SKIP LOCKED` to avoid processing the same row concurrently.

## Shutdown and recovery

During graceful shutdown:

```text
SIGTERM / Ctrl+C
        |
        v
Cancel polling context
        |
        v
Stop new claims
        |
        v
Finish current PostgreSQL transaction
        |
        v
Worker exits
```

If the process stops before committing a transaction, PostgreSQL rolls it back and the row remains eligible. If a retry commit succeeds, its attempt and next execution time remain available to a later worker instance.

## Tests

Run the complete test suite from the repository root:

```bash
go test ./...
go test -race ./...
```

The unit tests for this process cover exponential backoff and TTL evaluation. PostgreSQL integration scenarios should verify reference resolution, retry persistence, expiration, rejection events and restart recovery.
