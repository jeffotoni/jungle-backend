# Jungle Backend Challenge

Distributed Go backend for wallet and wagering operations.

The project is divided into three independent services. Each service has its own process and responsibility. Shared Application, Domain, contracts, ports and PostgreSQL adapters are kept under `internal/`.

## Three services

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

The three services remain independently deployable while using the shared components required for consistent financial behavior.
