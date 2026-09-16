#!/usr/bin/env bash

set -euo pipefail

SQS_ENDPOINT_URL="${SQS_ENDPOINT_URL:-http://localhost:4566}"
QUEUE_URL="${SQS_WAGER_QUEUE_URL:-}"
WALLET_ID="${WALLET_ID:?WALLET_ID must be set}"
PLAYER_ID="${PLAYER_ID:-player-001}"
PROVIDER_ID="${PROVIDER_ID:-provider-a}"
KIND="${KIND:-BET}"
AMOUNT="${AMOUNT:-5.00}"
CURRENCY="${CURRENCY:-BRL}"
MESSAGE_COUNT="${MESSAGE_COUNT:-2}"
DUPLICATE_COUNT="${DUPLICATE_COUNT:-1}"
INTERVAL_SECONDS="${INTERVAL_SECONDS:-30}"
MESSAGE_GROUP_ID="${MESSAGE_GROUP_ID:-$WALLET_ID}"

if [[ -z "$QUEUE_URL" ]]; then
	QUEUE_URL="$(aws \
		--endpoint-url="$SQS_ENDPOINT_URL" \
		sqs get-queue-url \
		--queue-name wager-transactions.fifo \
		--query QueueUrl \
		--output text)"
fi

for index in $(seq 1 "$MESSAGE_COUNT"); do
	identifier="$(uuidgen | tr '[:upper:]' '[:lower:]')"
	message_id="sqs-message-$identifier"
	idempotency_key="sqs-idempotency-$identifier"
	external_transaction_id="sqs-transaction-$identifier"
	round_id="sqs-round-$identifier"
	game_id="sqs-game-$identifier"
	occurred_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

	message_body="$(printf '{
  "messageId": "%s",
  "type": "WagerTransactionRequested",
  "occurredAt": "%s",
  "data": {
    "idempotencyKey": "%s",
    "providerId": "%s",
    "externalTransactionId": "%s",
    "playerId": "%s",
    "walletId": "%s",
    "roundId": "%s",
    "gameId": "%s",
    "kind": "%s",
    "money": {
      "amount": "%s",
      "currency": "%s"
    }
  }
}' \
		"$message_id" \
		"$occurred_at" \
		"$idempotency_key" \
		"$PROVIDER_ID" \
		"$external_transaction_id" \
		"$PLAYER_ID" \
		"$WALLET_ID" \
		"$round_id" \
		"$game_id" \
		"$KIND" \
		"$AMOUNT" \
		"$CURRENCY")"

	for duplicate_index in $(seq 1 "$DUPLICATE_COUNT"); do
		aws \
			--endpoint-url="$SQS_ENDPOINT_URL" \
			sqs send-message \
			--queue-url "$QUEUE_URL" \
			--message-group-id "$MESSAGE_GROUP_ID" \
			--message-deduplication-id "$message_id-$duplicate_index" \
			--message-body "$message_body"

		echo "sent messageId=$message_id externalTransactionId=$external_transaction_id duplicate=$duplicate_index"
	done

	if [[ "$index" -lt "$MESSAGE_COUNT" ]]; then
		sleep "$INTERVAL_SECONDS"
	fi
done
