import flow, { setup as prepare } from '../scenarios/wagering-flow.js';

export const options = {
  scenarios: {
    wagering_flow: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: '30s', target: 25 },
        { duration: '1m', target: 50 },
        { duration: '1m', target: 100 },
        { duration: '30s', target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.10'],
    'http_req_duration{endpoint:wager_post}': ['p(95)<1500'],
    'http_req_duration{endpoint:wager_get_by_id}': ['p(95)<1000'],
    'http_req_duration{endpoint:wager_get_by_external}': ['p(95)<1000'],
    wagering_unexpected_statuses: ['count<1'],
  },
};

export function setup() {
  return prepare();
}

export default function (data) {
  return flow(data);
}
