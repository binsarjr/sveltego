// k6 load profile for bench/ssr-constrained.
//
// Drives a single SSR route at the rps level set by RPS env var, with an
// explicit warmup -> sustain shape. run.sh sweeps RPS values in a binary
// search until p99 ≥ 100 ms; per-step JSON summaries get stitched into
// RESULTS.md.
//
// run.sh runs warmup as a separate k6 invocation so summary.json captures
// only the sustain phase. This file is the sustain config; warmup uses
// MODE=warmup and a shorter SUSTAIN_S override.

import http from 'k6/http';
import { check } from 'k6';

const TARGET_URL = __ENV.TARGET_URL || 'http://127.0.0.1:3000/longlist';
const RPS = parseInt(__ENV.RPS || '500', 10);
const DURATION_S = parseInt(__ENV.DURATION_S || '30', 10);
const COOLDOWN_S = parseInt(__ENV.COOLDOWN_S || '5', 10);

// Pre-allocate enough VUs to keep the constant-arrival-rate executor from
// starving when the server stalls. maxVUs = 3x rps tolerates tail
// latencies up to ~3 s while keeping the VU pool finite.
const PRE_VUS = Math.max(50, Math.ceil(RPS * 0.5));
const MAX_VUS = Math.max(200, RPS * 3);

export const options = {
  discardResponseBodies: true,
  // Default summary omits p(50)/p(99); request the full set so run.sh can
  // read them out of summary.json.
  summaryTrendStats: ['avg', 'min', 'med', 'p(50)', 'p(90)', 'p(95)', 'p(99)', 'p(99.9)', 'max'],
  scenarios: {
    sustain: {
      executor: 'constant-arrival-rate',
      rate: RPS,
      timeUnit: '1s',
      duration: `${DURATION_S}s`,
      preAllocatedVUs: PRE_VUS,
      maxVUs: MAX_VUS,
      gracefulStop: `${COOLDOWN_S}s`,
    },
  },
  // Soft check; run.sh reads p99 out of summary.json regardless.
  thresholds: {
    'http_req_duration': ['p(99)<=100'],
  },
};

export default function () {
  const res = http.get(TARGET_URL);
  check(res, { 'status 200': (r) => r.status === 200 });
}
