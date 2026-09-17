import http from 'k6/http';
import { check } from 'k6';

import { config } from './config.js';

export function providerToken() {
  if (config.providerToken !== '') {
    return config.providerToken;
  }

  const response = http.post(
    config.tokenURL,
    {
      grant_type: 'client_credentials',
      client_id: config.providerClientID,
      client_secret: config.providerClientSecret,
    },
    {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      tags: { name: 'oidc_token', endpoint: 'oidc_token' },
    },
  );

  check(response, {
    'OIDC token request succeeded': (value) => value.status === 200,
  });

  if (response.status !== 200) {
    throw new Error(`OIDC token request failed with status ${response.status}`);
  }

  const body = response.json();
  if (!body || !body.access_token) {
    throw new Error('OIDC token response did not contain access_token');
  }
  return body.access_token;
}
