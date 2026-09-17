import flow, { setup as prepare } from '../scenarios/wagering-flow.js';

export const options = {
  scenarios: {
    wagering_flow: {
      executor: 'constant-vus',
      vus: 1,
      duration: '10s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    'http_req_duration{endpoint:wager_post}': ['p(95)<500'],
    'http_req_duration{endpoint:wager_get_by_id}': ['p(95)<300'],
    'http_req_duration{endpoint:wager_get_by_external}': ['p(95)<300'],
    wagering_unexpected_statuses: ['count<1'],
  },
};

export function setup() {
  return prepare();
}

export default function (data) {
  return flow(data);
}
