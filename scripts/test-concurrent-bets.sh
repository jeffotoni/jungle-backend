#!/usr/bin/env bash

set -euo pipefail

: "${INTERNAL_TOKEN:?INTERNAL_TOKEN is required}"
: "${PROVIDER_TOKEN:?PROVIDER_TOKEN is required}"

MODE="${1:-local}"

temp_dir="$(mktemp -d)"
api1_pid=""
api2_pid=""
start_local_apis=true

if [[ "$MODE" == "docker" ]]; then
    api_url_1="${API_URL_1:-http://127.0.0.1:8080}"
    api_url_2="${API_URL_2:-http://127.0.0.1:8083}"
    start_local_apis=false
else
    api_url_1="${API_URL_1:-http://127.0.0.1:18080}"
    api_url_2="${API_URL_2:-http://127.0.0.1:18081}"
fi

cleanup() {
    if [[ -n "$api1_pid" ]]; then
        kill "$api1_pid" 2>/dev/null || true
    fi

    if [[ -n "$api2_pid" ]]; then
        kill "$api2_pid" 2>/dev/null || true
    fi

    rm -rf "$temp_dir"
}

trap cleanup EXIT

if [[ "$start_local_apis" == "true" ]]; then
    HTTP_ADDR="127.0.0.1:18080" \
        go run ./cmd/api >"$temp_dir/api-1.log" 2>&1 &
    api1_pid=$!

    HTTP_ADDR="127.0.0.1:18081" \
        go run ./cmd/api >"$temp_dir/api-2.log" 2>&1 &
    api2_pid=$!
fi

echo "mode=$MODE"
echo "api1=$api_url_1"
echo "api2=$api_url_2"

check_api() {
    local url="$1"

    curl -sS \
        --max-time 2 \
        -o /dev/null \
        -w '%{http_code}' \
        "$url/" \
        >/tmp/jungle-http-status 2>/dev/null || return 1

    local status
    status="$(cat /tmp/jungle-http-status)"

    # Qualquer resposta HTTP prova que o servidor está escutando.
    # 404 também é válido para este readiness simples.
    [[ "$status" =~ ^[1-5][0-9][0-9]$ ]]
}

ready=false

for attempt in {1..30}; do
    if check_api "$api_url_1" && check_api "$api_url_2"; then
        echo "both APIs are responding"
        ready=true
        break
    fi

    echo "waiting for APIs... attempt=$attempt"
    sleep 1
done

if [[ "$ready" != "true" ]]; then
    echo "APIs did not become ready" >&2

    if [[ "$start_local_apis" == "true" ]]; then
        echo "----- api1 log -----"
        cat "$temp_dir/api-1.log" || true

        echo "----- api2 log -----"
        cat "$temp_dir/api-2.log" || true
    fi

    exit 1
fi

player_id="concurrency-player-$(date +%s)"

echo "creating wallet..."

wallet_response="$(
    curl -fsS \
        --max-time 5 \
        -X POST "$api_url_1/wallets" \
        -H "Authorization: Bearer $INTERNAL_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "playerId":"'"$player_id"'",
            "initialBalance":{
                "amount":"100.00",
                "currency":"BRL"
            }
        }'
)"

wallet_id="$(jq -r '.id' <<<"$wallet_response")"

if [[ -z "$wallet_id" || "$wallet_id" == "null" ]]; then
    echo "wallet creation failed: $wallet_response" >&2
    exit 1
fi

echo "wallet created: $wallet_id"

post_bet() {
    local api_url="$1"
    local external_id="$2"
    local idempotency_key="$3"
    local output_file="$4"

    curl -sS \
        --max-time 10 \
        -o "$output_file" \
        -w '%{http_code}' \
        -X POST "$api_url/wagering/transactions" \
        -H "Authorization: Bearer $PROVIDER_TOKEN" \
        -H "Idempotency-Key: $idempotency_key" \
        -H "Content-Type: application/json" \
        -d '{
            "providerId":"provider-a",
            "externalTransactionId":"'"$external_id"'",
            "walletId":"'"$wallet_id"'",
            "playerId":"'"$player_id"'",
            "roundId":"concurrency-round",
            "gameId":"concurrency-game",
            "kind":"BET",
            "money":{
                "amount":"80.00",
                "currency":"BRL"
            }
        }'
}

echo "sending concurrent bets..."

post_bet \
    "$api_url_1" \
    "concurrent-bet-1-$wallet_id" \
    "concurrent-key-1-$wallet_id" \
    "$temp_dir/bet-1.json" \
    >"$temp_dir/bet-1.status" &

bet1_pid=$!

post_bet \
    "$api_url_2" \
    "concurrent-bet-2-$wallet_id" \
    "concurrent-key-2-$wallet_id" \
    "$temp_dir/bet-2.json" \
    >"$temp_dir/bet-2.status" &

bet2_pid=$!

wait "$bet1_pid" || true
wait "$bet2_pid" || true

status_1="$(cat "$temp_dir/bet-1.status")"
status_2="$(cat "$temp_dir/bet-2.status")"

result_1="$(jq -r '.status // empty' "$temp_dir/bet-1.json")"
result_2="$(jq -r '.status // empty' "$temp_dir/bet-2.json")"

echo "bet 1: HTTP $status_1 result=$result_1"
echo "bet 2: HTTP $status_2 result=$result_2"

processed_count=0
rejected_count=0

for result in "$result_1" "$result_2"; do
    case "$result" in
        PROCESSED)
            processed_count=$((processed_count + 1))
            ;;
        REJECTED)
            rejected_count=$((rejected_count + 1))
            ;;
        *)
            echo "unexpected result: $result" >&2
            echo "bet 1 body: $(cat "$temp_dir/bet-1.json")"
            echo "bet 2 body: $(cat "$temp_dir/bet-2.json")"
            exit 1
            ;;
    esac
done

if [[ "$processed_count" != "1" || "$rejected_count" != "1" ]]; then
    echo "unexpected concurrency result" >&2
    echo "bet 1: HTTP $status_1 $(cat "$temp_dir/bet-1.json")"
    echo "bet 2: HTTP $status_2 $(cat "$temp_dir/bet-2.json")"
    exit 1
fi

database_row="$(
    docker compose exec -T postgres \
        psql -U jungle -d jungle -At \
        -c "SELECT balance, version FROM wallets WHERE id = '$wallet_id';"
)"

ledger_count="$(
    docker compose exec -T postgres \
        psql -U jungle -d jungle -At \
        -c "SELECT COUNT(*) FROM ledger_entries WHERE wallet_id = '$wallet_id' AND direction = 'DEBIT';"
)"

echo "database wallet=$database_row"
echo "ledger debits=$ledger_count"

if [[ "$database_row" != "2000|2" || "$ledger_count" != "1" ]]; then
    echo "database validation failed" >&2
    exit 1
fi

echo
echo "PASS"
echo "two independent API processes passed:"
echo "processed=1 rejected=1 balance=2000 ledger_debits=1"