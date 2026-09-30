
const TARGET_URL = process.env.TARGET_URL ?? "http://localhost:8080/trace";
const CONCURRENCY = Number(process.env.CONCURRENCY ?? 50);
const TOTAL_REQUESTS = Number(process.env.TOTAL_REQUESTS ?? 500);

const MODELS = [
  "meta/muse-glimmer-30b",
  "meta/llama-3.3-70b-instruct",
  "openai/gpt-4o-mini",
];

function randomInt(min: number, max: number) {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

function randomChoice<T>(items: T[]): T {
  return items[randomInt(0, items.length - 1)]!;
}

function buildFakePayload() {
  const success = Math.random() > 0.05; 

  const promptTokens = randomInt(20, 200);
  const completionTokens = success ? randomInt(50, 900) : null;

  return {
    trace_id: crypto.randomUUID(),
    provider_response_id: success ? crypto.randomUUID() : null,
    model: randomChoice(MODELS),
    prompt_tokens: success ? promptTokens : null,
    completion_tokens: completionTokens,
    total_tokens: success && completionTokens != null ? promptTokens + completionTokens : null,
    duration_ms: randomInt(200, 6000),
    success,
    error: success
      ? null
      : {
          name: randomChoice(["TimeoutError", "RateLimitError", "APIError"]),
          message: "synthetic failure injected by load test",
        },
    timestamp: new Date().toISOString(),
  };
}

type RequestResult = {
  ok: boolean;
  status: number;
  latencyMs: number;
};

async function sendOne(): Promise<RequestResult> {
  const start = Date.now();

  try {
    const response = await fetch(TARGET_URL, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(buildFakePayload()),
    });

    return {
      ok: response.ok,
      status: response.status,
      latencyMs: Date.now() - start,
    };
  } catch (error) {
    return {
      ok: false,
      status: 0,
      latencyMs: Date.now() - start,
    };
  }
}

function summarize(results: RequestResult[], totalDurationMs: number) {
  const succeeded = results.filter((r) => r.ok);
  const failed = results.filter((r) => !r.ok);
  const latencies = results.map((r) => r.latencyMs).sort((a, b) => a - b);

  const percentile = (p: number) => {
    if (latencies.length === 0) return 0;
    const index = Math.min(latencies.length - 1, Math.floor((p / 100) * latencies.length));
    return latencies[index];
  };

  const statusCounts = results.reduce<Record<number, number>>((acc, r) => {
    acc[r.status] = (acc[r.status] ?? 0) + 1;
    return acc;
  }, {});

  console.log("\n--- Load test summary ---");
  console.log(`Target:            ${TARGET_URL}`);
  console.log(`Total requests:    ${results.length}`);
  console.log(`Concurrency:       ${CONCURRENCY}`);
  console.log(`Total duration:    ${(totalDurationMs / 1000).toFixed(2)}s`);
  console.log(`Requests/sec:      ${(results.length / (totalDurationMs / 1000)).toFixed(1)}`);
  console.log(`Accepted (2xx):    ${succeeded.length}`);
  console.log(`Rejected/failed:   ${failed.length}`);
  console.log(`Status breakdown:  ${JSON.stringify(statusCounts)}`);
  console.log(`Latency p50:       ${percentile(50)}ms`);
  console.log(`Latency p95:       ${percentile(95)}ms`);
  console.log(`Latency p99:       ${percentile(99)}ms`);
  console.log(`Latency max:       ${latencies[latencies.length - 1] ?? 0}ms`);
  console.log("--------------------------\n");
}

async function run() {
  console.log(`Firing ${TOTAL_REQUESTS} synthetic traces at ${TARGET_URL}, ${CONCURRENCY} at a time...\n`);

  const results: RequestResult[] = [];
  const start = Date.now();

  for (let sent = 0; sent < TOTAL_REQUESTS; sent += CONCURRENCY) {
    const batchSize = Math.min(CONCURRENCY, TOTAL_REQUESTS - sent);
    const batch = await Promise.all(Array.from({ length: batchSize }, () => sendOne()));

    results.push(...batch);
    process.stdout.write(`\rSent ${results.length}/${TOTAL_REQUESTS}`);
  }

  const totalDurationMs = Date.now() - start;

  summarize(results, totalDurationMs);
}

run();
