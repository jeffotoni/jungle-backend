export const config = {
  apiURL: (__ENV.API_URL || 'http://localhost:8080').replace(/\/$/, ''),
  tokenURL:
    __ENV.TOKEN_URL ||
    'http://localhost:8081/realms/jungle/protocol/openid-connect/token',
  providerClientID: __ENV.PROVIDER_CLIENT_ID || 'provider-a',
  providerClientSecret: __ENV.PROVIDER_CLIENT_SECRET || 'PROVIDER_CLIENT_SECRET',
  providerToken: __ENV.PROVIDER_TOKEN || '',
  providerID: __ENV.PROVIDER_ID || 'provider-a',
  kind: __ENV.KIND || 'BET',
  amount: __ENV.AMOUNT || '0.01',
  currency: __ENV.CURRENCY || 'BRL',
  walletIDs: (__ENV.WALLET_IDS || __ENV.WALLET_ID || '')
    .split(',')
    .map((value) => value.trim())
    .filter(Boolean),
  playerIDs: (__ENV.PLAYER_IDS || __ENV.PLAYER_ID || '')
    .split(',')
    .map((value) => value.trim())
    .filter(Boolean),
};

export function validateConfig() {
  if (config.walletIDs.length === 0) {
    throw new Error('WALLET_ID or WALLET_IDS must be set');
  }
  if (config.playerIDs.length === 0) {
    throw new Error('PLAYER_ID or PLAYER_IDS must be set');
  }
  if (config.playerIDs.length !== 1 && config.playerIDs.length !== config.walletIDs.length) {
    throw new Error('PLAYER_IDS must contain one value or one value per wallet');
  }
}

export function walletForIteration() {
  const index = (__VU - 1) % config.walletIDs.length;
  const playerIndex = config.playerIDs.length === 1 ? 0 : index;
  return {
    walletID: config.walletIDs[index],
    playerID: config.playerIDs[playerIndex],
  };
}
