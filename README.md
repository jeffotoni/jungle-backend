# Jungle Backend

Distributed Go backend for wallet and wagering operations.

The project is divided into three primary independent services and one dedicated reference worker. Each process has its own responsibility. Shared Application, Domain, contracts, ports and PostgreSQL adapters are kept under `internal/`.

## Primary services and reference worker

```text
        HTTP Client
         |
         v
+------------------+
|     cmd/api      |
+--------+---------+
         |
         |
         |                wager-transactions.fifo
         |                         |
         |                         v
         |                +------------------+
         |                |  cmd/consumer    |
         |                +--------+---------+
         |                         |
         +------------+------------+
                      |
                      v
            +----------------------+
            | Application / Domain |
            +----------+-----------+
                       |
                       v
            +----------------------+
            |      PostgreSQL      |
            |                      |
            | wallets              |
            | wager_transactions   |
            | ledger_entries       |
            | inbox_messages       |
            | outbox_events        |
            +----------+-----------+
                       |
                       | outbox_events
                       | PENDING
                       v
            +----------------------+
            |    cmd/publisher     |
            +----------+-----------+
                       |
                       v
                SQS event queue   
```

```text
API --------\
             \
              > Application / Domain -> PostgreSQL -> Publisher -> SQS
             /
Consumer ---/
```

Pending-reference continuation:

```text
wager_transactions
status = PENDING_REFERENCE
        |
        v
cmd/reference-worker
        |
        v
SAME Application / Domain -> PostgreSQL
        |
        +--> resolved: REFUND/ROLLBACK and outbox events
        +--> expired: REJECTED and WagerTransactionRejected
```

## Service responsibilities

### `cmd/api`

The HTTP entry point of the system.

- Exposes `POST /wagering/transactions`.
- Exposes wallet, ledger, transaction, reconciliation and health endpoints.
- Handles HTTP authentication and authorization.
- Validates HTTP input and `Idempotency-Key`.
- Calls the shared Application/Domain use cases.
- Persists financial changes in PostgreSQL.
- Does not publish directly to SQS.

Documentation: [`cmd/api/README.md`](cmd/api/README.md)

### `cmd/consumer`

The asynchronous wagering worker.

- Receives messages from `wager-transactions.fifo`.
- Parses the SQS envelope.
- Uses Inbox deduplication with `consumerName + messageId`.
- Calls the same Application/Domain use cases used by the API.
- Persists the financial operation, ledger and outbox atomically in PostgreSQL.
- Deletes the SQS message only after durable processing.

Documentation: [`cmd/consumer/README.md`](cmd/consumer/README.md)

### `cmd/publisher`

The transactional outbox worker.

- Reads committed events from `outbox_events`.
- Claims events using PostgreSQL coordination.
- Publishes events to the SQS output queue.
- Retries failed publications using attempts and backoff metadata.
- Recovers abandoned claims after lease expiration.
- Marks events as `PUBLISHED` only after SQS confirms publication.
- Does not expose HTTP endpoints.

Documentation: [`cmd/publisher/README.md`](cmd/publisher/README.md)

### `cmd/reference-worker`

The pending-reference continuation worker.

- Finds eligible `PENDING_REFERENCE` transactions in PostgreSQL.
- Resolves references for `REFUND` and `ROLLBACK`.
- Uses the same Application/Domain use cases as the API and Consumer.
- Applies persistent retry, exponential backoff, TTL and maximum attempts.
- Finalizes expired references as `REJECTED` with `REFERENCE_NOT_FOUND`.
- Writes resulting events to the transactional outbox.
- Does not consume or publish SQS messages directly.

Documentation: [`cmd/reference-worker/README.md`](cmd/reference-worker/README.md)

### `cmd/swagger`

The embedded Swagger service serves the API documentation independently from the API process.

- Serves the OpenAPI UI on `http://localhost:8082`.
- Serves the OpenAPI specification from [`cmd/swagger/api.yaml`](cmd/swagger/api.yaml).
- Runs as a separate Compose service and container.

## Load and performance testing

The k6 suite is dedicated to HTTP load and performance validation for the API. It exercises the wagering flow through the POST endpoint and the related transaction GET endpoints using smoke, load and stress profiles.

These scenarios complement the Go integration tests: k6 measures runtime behavior under traffic, while the integration tests validate persistence, messaging, idempotency and concurrency correctness.

Documentation: [`k6/README.md`](k6/README.md)

## Shared application flow

API and Consumer use the same Application/Domain path. The transport is different, but the financial behavior is shared:

```text
HTTP API                         SQS Consumer
    |                                 |
    v                                 v
HTTP request                    SQS message
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
```

The Application and Domain are independent of HTTP, Quick, SQS, PostgreSQL, AWS SDK, Keycloak and Uber Fx. Those technologies belong to the service boundaries and infrastructure adapters.

## End-to-end event flow

```text
Client
  |
  v
cmd/api
  |
  v
Application / Domain
  |
  v
PostgreSQL transaction
  |
  +--> wallet state when applicable
  +--> wager transaction
  +--> ledger entry when applicable
  +--> outbox event
  |
  v
COMMIT
  |
  v
outbox_events
  |
  v
cmd/publisher
  |
  v
SQS event queue
```

For asynchronous wagering:

```text
wager-transactions.fifo
  |
  v
cmd/consumer
  |
  v
Inbox
  |
  v
SAME Application / Domain
  |
  v
PostgreSQL transaction
  |
  +--> wager transaction
  +--> wallet state when applicable
  +--> ledger entry when applicable
  +--> outbox event
  +--> Inbox completion
  |
  v
COMMIT
  |
  v
DeleteMessage SQS
```

## Shared components

```text
internal/
├── application/       Shared use cases and ports
├── contracts/         Shared HTTP/SQS/event contracts
├── domain/            Money, Wallet, Wager and financial rules
├── platform/          HTTP server and infrastructure support
└── repository/        Shared PostgreSQL persistence
```

The primary services and reference worker remain independently deployable while using the shared components required for consistent financial behavior.

## Local stack

The Docker Compose stack runs all application processes and their local dependencies on the same Docker network:

```text
                           backend network

+-------------+       +-------------+       +----------------+
| PostgreSQL  |       |  Keycloak   |       |   LocalStack    |
| port 5432   |       | port 8081   |       | port 4566       |
+------+------+       +------+------+       +--------+-------+
       |                     |                        |
       +---------------------+------------------------+
                             |
       +---------------------+------------------------+
       |                     |                        |
       v                     v                        v
   cmd/api             cmd/consumer              cmd/publisher
   port 8080           SQS consumer              Outbox publisher
       |
       v
   cmd/reference-worker

   cmd/swagger
   port 8082
```

The Compose services are:

- `postgres`: PostgreSQL 16 with persistent local storage.
- `keycloak`: local OAuth2/OIDC identity provider with the `jungle` realm.
- `localstack`: local SQS implementation.
- `api`: authenticated HTTP API on `http://localhost:8080`.
- `consumer`: SQS wagering consumer.
- `publisher`: transactional outbox publisher.
- `reference-worker`: pending-reference processor.
- `swagger`: embedded API documentation on `http://localhost:8082`.
- `api2`: second API instance on `http://localhost:8083`, used for distributed concurrency validation.

## Run the complete stack from zero

Run the following steps from the repository root.

### 1. Remove the local environment

This command is destructive. It removes all containers, the Compose network and the PostgreSQL volume, including all local wallets, wagering transactions, ledger entries, Inbox records and outbox events.

```bash
docker compose down -v --remove-orphans
```

### 2. Start PostgreSQL and initialize the database

The migrations run automatically inside the PostgreSQL container. There is no separate migration service or manual migration command in the first-time setup.

The seven migration `up` files are mounted by Docker Compose into PostgreSQL's initialization directory. The official PostgreSQL image executes them in filename order while creating the `postgres_data` volume:

```text
docker compose
      |
      v
postgres container
      |
      v
/docker-entrypoint-initdb.d/
      |
      +--> 001_init.sql
      +--> 002_api_financial_guarantees.sql
      +--> 003_inbox_consumer_identity.sql
      +--> 004_outbox_claim_lease.sql
      +--> 005_pending_reference_retry.sql
      +--> 006_financial_constraints.sql
      +--> 007_reversal_reference_constraint.sql
      |
      v
Complete PostgreSQL schema
```

```bash
docker compose up -d postgres
```

Therefore, the command above both starts PostgreSQL and initializes the complete database schema when the volume is new.

Wait until PostgreSQL is healthy:

```bash
docker compose ps postgres
```

The expected status contains:

```text
Up ... (healthy)
```

Confirm that the required tables were created:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "\\dt"
```

The expected tables are:

- `wallets`
- `wager_transactions`
- `ledger_entries`
- `inbox_messages`
- `outbox_events`

If the PostgreSQL volume already exists, initialization scripts are not executed again. To run all migrations again, remove the local volume and recreate PostgreSQL:

```bash
docker compose down -v --remove-orphans
docker compose up -d postgres
```

The `down -v` command permanently removes all local PostgreSQL data. Use it only when a complete reset is intended.

### 3. Start Keycloak and LocalStack

```bash
docker compose up -d keycloak localstack
```

Confirm both services:

```bash
docker compose ps keycloak localstack
```

Confirm that LocalStack created the queues:

```bash
aws --endpoint-url=http://localhost:4566 \
  sqs list-queues \
  --output table
```

The expected queues are:

- `wager-transactions.fifo`
- `wager-transactions-dlq.fifo`
- `jungle-wager`
- `jungle-events`

The wagering FIFO queue uses a redrive policy to `wager-transactions-dlq.fifo`.

### 4. Start all application processes

```bash
docker compose up -d --build api consumer publisher reference-worker swagger
```

Confirm the complete stack:

```bash
docker compose ps
```

All services should be `Up`. The exposed endpoints are:

```text
API:     http://localhost:8080
API 2:   http://localhost:8083 (only for the concurrency test)
Swagger: http://localhost:8082
Keycloak: http://localhost:8081
SQS:     http://localhost:4566
Postgres: localhost:5432
```

### 5. Test liveness, readiness and Swagger

Liveness does not depend on external services:

```bash
curl -i http://localhost:8080/health/live
```

Expected response:

```text
HTTP/1.1 200 OK
{"status":"ok"}
```

Readiness verifies PostgreSQL and the SQS wagering queue:

```bash
curl -i http://localhost:8080/health/ready
```

Expected response:

```text
HTTP/1.1 200 OK
{"status":"ready"}
```

Test the embedded Swagger page:

```bash
curl -I http://localhost:8082/
```

Open `http://localhost:8082/` in a browser to use the API documentation.

## Authenticate with Keycloak

The terminal reaches Keycloak through `localhost:8081`. The API container uses the internal Docker hostname configured by Compose, `keycloak:8080`, to validate the token issuer and fetch the signing keys.

Configure the local variables:

```bash
export API="http://localhost:8080"
export OIDC_ISSUER="http://localhost:8081/realms/jungle"
export KEYCLOAK_TOKEN_URL="$OIDC_ISSUER/protocol/openid-connect/token"

export INTERNAL_CLIENT_ID="wallet-internal"
export INTERNAL_CLIENT_SECRET="INTERNAL_CLIENT_SECRET"
export PROVIDER_CLIENT_ID="provider-a"
export PROVIDER_CLIENT_SECRET="PROVIDER_CLIENT_SECRET"
```

Generate the internal token used by wallet, ledger, reconciliation and health-protected operations:

```bash
export INTERNAL_TOKEN="$(curl -fsS -X POST "$KEYCLOAK_TOKEN_URL" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "grant_type=client_credentials" \
  --data-urlencode "client_id=$INTERNAL_CLIENT_ID" \
  --data-urlencode "client_secret=$INTERNAL_CLIENT_SECRET" \
  | jq -r '.access_token')"

test -n "$INTERNAL_TOKEN" && echo "internal token ok"
```

Generate the provider token used by wagering operations:

```bash
export PROVIDER_TOKEN="$(curl -fsS -X POST "$KEYCLOAK_TOKEN_URL" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "grant_type=client_credentials" \
  --data-urlencode "client_id=$PROVIDER_CLIENT_ID" \
  --data-urlencode "client_secret=$PROVIDER_CLIENT_SECRET" \
  | jq -r '.access_token')"

test -n "$PROVIDER_TOKEN" && echo "provider token ok"
```

## End-to-end API and SQS validation

### 1. Create a wallet through the API

Use a unique player identifier so the request does not conflict with an existing wallet:

```bash
export WALLET_PLAYER_ID="compose-player-$(date +%s)"

curl -i -X POST "$API/wallets" \
  -H "Authorization: Bearer $INTERNAL_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "playerId": "'"$WALLET_PLAYER_ID"'",
    "initialBalance": {
      "amount": "25.00",
      "currency": "BRL"
    }
  }'
```

The expected status is `201 Created`. Copy the returned wallet ID and export it:

```bash
export WALLET_ID="<wallet-id-from-response>"
```

Query PostgreSQL to retrieve the player ID associated with the wallet:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT id, player_id, balance, currency, version FROM wallets WHERE id = '$WALLET_ID';"
```

Copy the `player_id` returned by PostgreSQL and export it for the SQS test:

```bash
export PLAYER_ID="<player-id-from-postgres>"
```

The expected balance is `2500` minor units and the initial version is `1`.

### 2. Send one logical BET three times through SQS

The script sends one envelope three times with the same business identity. The Consumer must apply only one financial effect.

```bash
WALLET_ID="$WALLET_ID" \
PLAYER_ID="$PLAYER_ID" \
MESSAGE_COUNT=1 \
DUPLICATE_COUNT=3 \
INTERVAL_SECONDS=0 \
AMOUNT=1.00 \
./scripts/send-wager-messages.sh
```

The script generates the envelope identifiers dynamically and sends it to `wager-transactions.fifo` with distinct SQS deduplication identifiers.

Inspect the Consumer logs:

```bash
docker compose logs --tail=50 --no-color consumer
```

The expected behavior is:

- one `message processed` entry;
- two `duplicate message ignored` entries;
- `message deleted` only after durable processing.

### 3. Confirm the Consumer financial effect

The opening balance was `2500` and the BET is `100` minor units:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT balance, version FROM wallets WHERE id = '$WALLET_ID';"
```

Expected result:

```text
balance | version
---------+---------
2400    | 2
```

Confirm Inbox deduplication and the processed wagering transaction:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT message_id, status FROM inbox_messages ORDER BY created_at DESC LIMIT 5;"
```

The generated message should be `COMPLETED`.

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT external_transaction_id, idempotency_key, status, amount FROM wager_transactions WHERE player_id = '$PLAYER_ID' ORDER BY created_at DESC;"
```

The generated SQS transaction should appear once with status `PROCESSED` and amount `100`.

Confirm the ledger entry:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT l.direction, l.amount, l.balance_before, l.balance_after
      FROM ledger_entries l
      JOIN wager_transactions w ON w.id = l.wager_transaction_id
      WHERE w.player_id = '$PLAYER_ID'
      ORDER BY l.created_at;"
```

The BET entry must be a `DEBIT` of `100`, changing the balance from `2500` to `2400`.

### 4. Confirm Publisher outbox delivery

The Publisher reads only committed records from `outbox_events` and marks them after successful SQS publication:

```bash
docker compose exec -T postgres psql \
  -U jungle \
  -d jungle \
  -c "SELECT event_type, status, published_at
      FROM outbox_events
      WHERE aggregate_id = '$WALLET_ID'
      ORDER BY created_at;"
```

The opening wallet and the SQS BET should produce outbox events with status `PUBLISHED` and a non-null `published_at`.

Inspect the Publisher logs if necessary:

```bash
docker compose logs --tail=50 --no-color publisher
```

Expected log actions include `startup` and `publish`.

## Minimal integration validation runbook

This runbook records the lean validation performed against the real local stack. It covers the persistence, asynchronous processing, authentication, cross-channel idempotency, pending-reference expiration, and distributed concurrency paths without using k6.

### 1. PostgreSQL repository

Run the repository integration tests against the real PostgreSQL container:

```bash
JUNGLE_INTEGRATION=1 \
go test -v ./cmd/api/repository
```

Validated:

- financial constraints;
- concurrency protecting the wallet balance;
- transactional rollback;
- cursor, UUID, and repository utilities.

Expected result:

```text
🟢 PostgreSQL real
```

Observed validation output:

```text
=== RUN   TestPostgreSQLFinancialConstraints
--- PASS: TestPostgreSQLFinancialConstraints
=== RUN   TestParseUUID
--- PASS: TestParseUUID
=== RUN   TestLedgerCursorRoundTrip
--- PASS: TestLedgerCursorRoundTrip
=== RUN   TestLedgerCursorEmpty
--- PASS: TestLedgerCursorEmpty
=== RUN   TestLedgerCursorRejectsInvalidValues
--- PASS: TestLedgerCursorRejectsInvalidValues
=== RUN   TestNullIf
--- PASS: TestNullIf
=== RUN   TestPostgreSQLConcurrentBetsProtectWalletBalance
--- PASS: TestPostgreSQLConcurrentBetsProtectWalletBalance
=== RUN   TestPostgreSQLTransactionRollsBackFinancialWrites
--- PASS: TestPostgreSQLTransactionRollsBackFinancialWrites
PASS
ok github.com/jeffotoni/jungle-backend/cmd/api/repository
```

### 2. Consumer and LocalStack

Run the real SQS integration tests:

```bash
JUNGLE_INTEGRATION=1 \
go test -v ./cmd/consumer
```

The covered scenarios include:

```text
TestIntegrationConsumerHappyPath
TestIntegrationConsumerInboxRedelivery
TestIntegrationConsumerInvalidMessageReachesDLQ
TestReceiveCount
TestRetryDelayUsesCappedExponentialBackoff
TestRetryMessageChangesVisibilityUsingReceiveCount
```

Expected result:

```text
🟢 Consumer happy path
🟢 Inbox / redelivery
🟢 DLQ
🟢 Retry / visibility timeout
```

Observed validation output:

```text
TestIntegrationConsumerHappyPath              PASS
  message processed: status=PROCESSED duplicate=false
  message deleted after durable processing

TestIntegrationConsumerInboxRedelivery        PASS
  first delivery: status=PROCESSED duplicate=false
  redelivery: duplicate message ignored
  both SQS messages deleted

TestIntegrationConsumerInvalidMessageReachesDLQ PASS
  permanent SQS message error: incomplete envelope
  message remained available for redrive

TestReceiveCount                               PASS
TestRetryDelayUsesCappedExponentialBackoff     PASS
TestRetryMessageChangesVisibilityUsingReceiveCount PASS

PASS
ok github.com/jeffotoni/jungle-backend/cmd/consumer
```

`TestIntegrationHTTPAndSQSShareFinancialIdempotency` was skipped in this execution because `INTEGRATION_API_URL` and `INTEGRATION_PROVIDER_TOKEN` were not provided. Run the dedicated command in section 5 to execute that scenario.

### 3. Generate Keycloak tokens

The API and integration tests use tokens issued by the local Keycloak realm.

Provider token:

```bash
export PROVIDER_TOKEN="$(curl -fsS -X POST "$KEYCLOAK_TOKEN_URL" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "grant_type=client_credentials" \
  --data-urlencode "client_id=$PROVIDER_CLIENT_ID" \
  --data-urlencode "client_secret=$PROVIDER_CLIENT_SECRET" \
  | jq -er '.access_token')"
```

Internal token:

```bash
export INTERNAL_TOKEN="$(curl -fsS -X POST "$KEYCLOAK_TOKEN_URL" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "grant_type=client_credentials" \
  --data-urlencode "client_id=$INTERNAL_CLIENT_ID" \
  --data-urlencode "client_secret=$INTERNAL_CLIENT_SECRET" \
  | jq -er '.access_token')"
```

Check that both variables contain a token without printing the credentials:

```bash
echo ${#PROVIDER_TOKEN}
echo ${#INTERNAL_TOKEN}
```

### 4. Verify the internal API token

Create a wallet through the authenticated API:

```bash
curl -i -X POST "http://localhost:8080/wallets" \
  -H "Authorization: Bearer $INTERNAL_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "playerId":"token-test",
    "initialBalance":{
      "amount":"100.00",
      "currency":"BRL"
    }
  }'
```

Expected result:

```text
HTTP/1.1 201 Created
```

### 5. Cross-channel HTTP and SQS

The same business operation is submitted through the HTTP API and the SQS consumer. The test uses the same business identity and idempotency key and verifies that only one financial effect is persisted:

```bash
JUNGLE_INTEGRATION=1 \
INTEGRATION_API_URL="http://localhost:8080" \
INTEGRATION_PROVIDER_TOKEN="$PROVIDER_TOKEN" \
go test -v ./cmd/consumer \
  -run TestIntegrationHTTPAndSQSShareFinancialIdempotency
```

Expected result:

```text
🟢 HTTP + SQS share financial idempotency
```

Observed validation output:

```text
TestIntegrationHTTPAndSQSShareFinancialIdempotency PASS
  message processed: status=PROCESSED duplicate=false
  message deleted after durable processing

PASS
ok github.com/jeffotoni/jungle-backend/cmd/consumer
```

### 6. Pending-reference expiration

Run the real reference worker integration test for a REFUND or ROLLBACK whose reference remains unavailable:

```bash
JUNGLE_INTEGRATION=1 \
go test -v ./cmd/reference-worker
```

The expected terminal result is:

```text
status=REJECTED
failureCode=REFERENCE_NOT_FOUND
🟢 Pending reference expiration
```

Observed validation output:

```text
TestIntegrationPendingReferenceExpires PASS
  status=REJECTED attempts=1 failureCode=REFERENCE_NOT_FOUND

TestReferenceRetryDelay PASS
TestReferenceExpired PASS

PASS
ok github.com/jeffotoni/jungle-backend/cmd/reference-worker
```

### 7. Two independent API instances

The distributed concurrency check uses two API instances that share PostgreSQL, Keycloak, and LocalStack:

```text
api  -> localhost:8080
api2 -> localhost:8083
```

Start or rebuild both instances:

```bash
docker compose up -d --build api api2
```

Confirm the services:

```bash
docker compose ps
```

Execute the acceptance script:

```bash
INTERNAL_TOKEN="$INTERNAL_TOKEN" \
PROVIDER_TOKEN="$PROVIDER_TOKEN" \
./scripts/test-concurrent-bets.sh docker
```

The script sends two simultaneous BET operations:

```text
initial balance = 100.00

API 1 -> BET 80.00
API 2 -> BET 80.00
```

Expected result:

```text
bet 1: HTTP 200 result=PROCESSED
bet 2: HTTP 422 result=REJECTED
database wallet=2000|2
ledger debits=1

PASS
two independent API processes passed:
processed=1 rejected=1 balance=2000 ledger_debits=1
```

This confirms that only one BET changes the balance, the final balance is `20.00`, and PostgreSQL protects the operation across independent processes.

Observed validation output:

```text
mode=docker
api1=http://127.0.0.1:8080
api2=http://127.0.0.1:8083
both APIs are responding
creating wallet...
wallet created: 1af6fb86-bc4e-4962-a59b-aab37043677d
sending concurrent bets...
bet 1: HTTP 200 result=PROCESSED
bet 2: HTTP 422 result=REJECTED
database wallet=2000|2
ledger debits=1

PASS
two independent API processes passed:
processed=1 rejected=1 balance=2000 ledger_debits=1
```

### Troubleshooting port conflicts

If an endpoint unexpectedly returns `404`, first confirm that the request reaches the intended process. A different local process may be listening on the same port:

```bash
lsof -iTCP:8080 -sTCP:LISTEN
lsof -iTCP:8083 -sTCP:LISTEN
docker compose ps
```

### Validation status

```text
🟢 PostgreSQL real
🟢 Consumer + LocalStack
🟢 Inbox / redelivery
🟢 DLQ / retry
🟢 Cross-channel HTTP + SQS
🟢 Pending reference expiration
🟢 Concurrency between two independent API instances
```

## Stop or reset the local stack

Stop the services while preserving the PostgreSQL volume:

```bash
docker compose down
```

Stop the services and erase the local database so the migrations run again on the next startup:

```bash
docker compose down -v --remove-orphans
docker compose up -d postgres
docker compose up -d keycloak localstack
docker compose up -d --build api consumer publisher reference-worker swagger
```

After PostgreSQL becomes healthy, the commands above start Keycloak, LocalStack and the regular application processes. Start `api2` separately only when running the distributed concurrency test in section 7.
