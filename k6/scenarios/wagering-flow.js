import http from 'k6/http';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

import { providerToken } from '../auth.js';
import { config, validateConfig, walletForIteration } from '../config.js';

http.setResponseCallback(http.expectedStatuses(200, 202, 422));

const unexpectedStatuses = new Counter('wagering_unexpected_statuses');
const rejectedTransactions = new Counter('wagering_rejected_transactions');
const missingTransactionIDs = new Counter('wagering_missing_transaction_ids');

export function setup() {
  validateConfig();
  return { token: providerToken() };
}

export default function (data) {
  const identifiers = uniqueIdentifiers();
  const wallet = walletForIteration();
  const headers = {
    Authorization: `Bearer ${data.token}`,
    'Content-Type': 'application/json',
  };
  const request = {
    providerId: config.providerID,
    externalTransactionId: identifiers.externalTransactionID,
    walletId: wallet.walletID,
    playerId: wallet.playerID,
    roundId: identifiers.roundID,
    gameId: identifiers.gameID,
    kind: config.kind,
    money: {
      amount: config.amount,
      currency: config.currency,
    },
  };

  const postResponse = http.post(
    `${config.apiURL}/wagering/transactions`,
    JSON.stringify(request),
    {
      headers: { ...headers, 'Idempotency-Key': identifiers.idempotencyKey },
      tags: { name: 'wager_post', endpoint: 'wager_post' },
    },
  );

  const postBody = jsonBody(postResponse);
  const acceptedStatus = [200, 202, 422].includes(postResponse.status);
  const postChecks = check(postResponse, {
    'wager POST returns a business status': () => acceptedStatus,
    'wager POST returns transactionId': () => Boolean(postBody && postBody.transactionId),
  });

  if (postResponse.status === 422) {
    rejectedTransactions.add(1);
  }
  if (!acceptedStatus) {
    unexpectedStatuses.add(1);
  }
  if (!postBody || !postBody.transactionId) {
    missingTransactionIDs.add(1);
    return;
  }
  if (!postChecks && postResponse.status !== 422) {
    return;
  }

  const transactionID = encodeURIComponent(postBody.transactionId);
  const externalTransactionID = encodeURIComponent(identifiers.externalTransactionID);
  const providerID = encodeURIComponent(config.providerID);
  const params = { headers };
  const responses = http.batch([
    [
      'GET',
      `${config.apiURL}/wagering/transactions/${transactionID}`,
      null,
      { ...params, tags: { name: 'wager_get_by_id', endpoint: 'wager_get_by_id' } },
    ],
    [
      'GET',
      `${config.apiURL}/providers/${providerID}/wagering/transactions/${externalTransactionID}`,
      null,
      {
        ...params,
        tags: { name: 'wager_get_by_external', endpoint: 'wager_get_by_external' },
      },
    ],
  ]);

  check(responses[0], {
    'wager GET by transaction ID returns 200': (response) => response.status === 200,
    'wager GET by transaction ID returns the same transaction': (response) => {
      const body = jsonBody(response);
      return body && body.transactionId === postBody.transactionId;
    },
  });
  check(responses[1], {
    'wager GET by provider and external ID returns 200': (response) => response.status === 200,
    'wager GET by provider and external ID returns the same transaction': (response) => {
      const body = jsonBody(response);
      return body && body.transactionId === postBody.transactionId;
    },
  });
}

function uniqueIdentifiers() {
  const suffix = `${Date.now()}-${__VU}-${__ITER}-${Math.floor(Math.random() * 1000000)}`;
  return {
    idempotencyKey: `k6-idempotency-${suffix}`,
    externalTransactionID: `k6-external-${suffix}`,
    roundID: `k6-round-${suffix}`,
    gameID: `k6-game-${suffix}`,
  };
}

function jsonBody(response) {
  try {
    return response.json();
  } catch (_) {
    return null;
  }
}
