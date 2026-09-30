"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

type TraceError = { name?: string; message?: string };
type Trace = {
  trace_id: string;
  provider_response_id?: string | null;
  model: string;
  prompt_tokens?: number | null;
  completion_tokens?: number | null;
  total_tokens?: number | null;
  duration_ms: number;
  success: boolean;
  error?: TraceError | null;
  timestamp: string;
};

type Metric = { name: string; value: number; labels: Record<string, string> };

const API = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

function parseMetrics(text: string): Metric[] {
  return text.split("\n").flatMap((line) => {
    if (!line || line.startsWith("#")) return [];
    const match = line.match(/^([a-zA-Z_:][a-zA-Z0-9_:]*)(?:\{([^}]*)\})?\s+([-+]?\d*\.?\d+(?:e[-+]?\d+)?)$/);
    if (!match) return [];
    const labels: Record<string, string> = {};
    match[2]?.split(",").forEach((pair) => {
      const [key, value] = pair.split("=");
      if (key && value) labels[key] = value.replace(/^"|"$/g, "");
    });
    return [{ name: match[1], value: Number(match[3]), labels }];
  });
}

function metric(metrics: Metric[], name: string, labels: Record<string, string> = {}) {
  return metrics.find((item) => item.name === name && Object.entries(labels).every(([key, value]) => item.labels[key] === value))?.value ?? 0;
}

function histogramAverage(metrics: Metric[], baseName: string) {
  const sum = metric(metrics, `${baseName}_sum`);
  const count = metric(metrics, `${baseName}_count`);
  return count > 0 ? sum / count : 0;
}

const number = (value: number | null | undefined) => value == null ? "—" : value.toLocaleString();
const shortId = (value: string) => `${value.slice(0, 8)}…${value.slice(-4)}`;

export default function Home() {
  const [traces, setTraces] = useState<Trace[]>([]);
  const [metrics, setMetrics] = useState<Metric[]>([]);
  const [selected, setSelected] = useState<Trace | null>(null);
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading");
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);
  const [expanded, setExpanded] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const [traceResponse, metricResponse] = await Promise.all([
        fetch(`${API}/traces`, { cache: "no-store" }),
        fetch(`${API}/metrics`, { cache: "no-store" }),
      ]);
      if (!traceResponse.ok || !metricResponse.ok) throw new Error("API unavailable");
      setTraces(await traceResponse.json());
      setMetrics(parseMetrics(await metricResponse.text()));
      setStatus("ready");
      setLastUpdated(new Date());
    } catch {
      setStatus("error");
    }
  }, []);

  useEffect(() => {
    refresh();
    const timer = setInterval(refresh, 5000);
    return () => clearInterval(timer);
  }, [refresh]);

  const filtered = useMemo(() => traces.filter((trace) => {
    const haystack = `${trace.trace_id} ${trace.model} ${trace.provider_response_id ?? ""}`.toLowerCase();
    return haystack.includes(query.toLowerCase());
  }), [traces, query]);

  const PREVIEW_COUNT = 10;
  const visible = expanded ? filtered : filtered.slice(0, PREVIEW_COUNT);

  const totalTokens = traces.reduce((sum, trace) => sum + (trace.total_tokens ?? 0), 0);
  const averageLatency = traces.length ? Math.round(traces.reduce((sum, trace) => sum + trace.duration_ms, 0) / traces.length) : 0;
  const successRate = traces.length ? Math.round((traces.filter((trace) => trace.success).length / traces.length) * 100) : 0;

  const processedTotal = metric(metrics, "tracelight_trace_events_processed_total", { status: "success" });
  const avgProcessingMs = Math.round(histogramAverage(metrics, "tracelight_trace_processing_duration_seconds") * 1000);
  const avgBatchSize = histogramAverage(metrics, "tracelight_trace_batch_size");

  return (
    <main className="shell">
      <aside className="sidebar">
        <div className="brand"><div className="brand-mark">✦</div><div><strong>TraceLight</strong><span>LLM observability</span></div></div>
        <nav><a className="nav-active">▦ <span>Overview</span></a></nav>
        <div className="side-bottom"><div className={`connection ${status}`}><i /> <span>{status === "error" ? "API disconnected" : "API connected"}</span></div><small>v0.1.0 · local workspace</small></div>
      </aside>

      <section className="content">
        <header className="topbar"><div><p className="eyebrow">WORKSPACE / OVERVIEW</p><h1>Trace operations</h1><p className="subtitle">Monitor every model request from ingestion to persistence.</p></div><div className="top-actions"><span className="live"><i /> Live</span><button onClick={refresh}>↻ Refresh</button><div className="avatar">TL</div></div></header>

        <div className="stats-grid">
          <Stat label="Total traces" value={number(traces.length)} detail="Stored in PostgreSQL" accent="blue" />
          <Stat label="Success rate" value={`${successRate}%`} detail="Across captured requests" accent="green" />
          <Stat label="Avg. latency" value={`${number(averageLatency)} ms`} detail="Provider round trip" accent="violet" />
          <Stat label="Total tokens" value={number(totalTokens)} detail="Prompt + completion" accent="orange" />
        </div>

        <div className="section-heading"><div><h2>Trace activity</h2><p>{lastUpdated ? `Updated ${lastUpdated.toLocaleTimeString()}` : "Loading telemetry…"}</p></div><div className="search">⌕<input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search traces…" /></div></div>
        <div className="table-card"><table><thead><tr><th>Trace ID</th><th>Model</th><th>Tokens</th><th>Latency</th><th>Status</th><th>Captured</th><th /></tr></thead><tbody>{visible.map((trace) => <tr key={trace.trace_id} onClick={() => setSelected(trace)}><td><code>{shortId(trace.trace_id)}</code></td><td><span className="model"><i />{trace.model}</span></td><td>{number(trace.total_tokens)}</td><td>{number(trace.duration_ms)} ms</td><td><span className={trace.success ? "badge success" : "badge failure"}>{trace.success ? "Success" : "Failed"}</span></td><td className="muted">{new Date(trace.timestamp).toLocaleString()}</td><td className="arrow">→</td></tr>)}{!filtered.length && <tr><td colSpan={7} className="empty">{status === "error" ? "Could not reach the TraceLight API." : "No traces match your search."}</td></tr>}</tbody></table>{filtered.length > PREVIEW_COUNT && <button className="expand" onClick={() => setExpanded(!expanded)}>{expanded ? "Show less" : `Show all ${filtered.length.toLocaleString()} traces`}</button>}</div>

        <div className="lower-grid">
          <section className="panel">
            <div className="panel-heading"><div><h2>Pipeline health</h2><p>Live ingestion telemetry</p></div><span className="healthy">Healthy</span></div>
            <div className="health-row">
              <Health label="Accepted" value={metric(metrics, "tracelight_trace_events_accepted_total")} />
              <Health label="Queue depth" value={metric(metrics, "tracelight_trace_queue_depth")} />
              <Health label="Overflowed" value={metric(metrics, "tracelight_trace_events_overflowed_total")} />
            </div>
            <div className="health-row">
              <Health label="Processed (workers)" value={processedTotal} />
              <Health label="Avg. write latency" value={avgProcessingMs} suffix=" ms" />
              <Health label="Avg. batch size" value={Math.round(avgBatchSize * 10) / 10} />
            </div>
          </section>
          <section className="panel"><div className="panel-heading"><div><h2>Service topology</h2><p>Connected components</p></div><span className="healthy">4 online</span></div><div className="services"><Service name="Go ingestion" port=":8080" /><Service name="PostgreSQL" port=":5432" /><Service name="Redis stream" port=":6379" /><Service name="Prometheus" port=":9090" /></div></section>
        </div>
      </section>

      {selected && <div className="modal-backdrop" onClick={() => setSelected(null)}><article className="drawer" onClick={(event) => event.stopPropagation()}><button className="close" onClick={() => setSelected(null)}>×</button><p className="eyebrow">TRACE DETAIL</p><h2>{shortId(selected.trace_id)}</h2><span className={selected.success ? "badge success" : "badge failure"}>{selected.success ? "Successful request" : "Failed request"}</span><div className="detail-grid"><Detail label="Model" value={selected.model} /><Detail label="Duration" value={`${selected.duration_ms} ms`} /><Detail label="Prompt tokens" value={number(selected.prompt_tokens)} /><Detail label="Completion tokens" value={number(selected.completion_tokens)} /><Detail label="Total tokens" value={number(selected.total_tokens)} /><Detail label="Provider response" value={selected.provider_response_id ?? "—"} /></div><div className="json"><div>RAW EVENT <button onClick={() => navigator.clipboard.writeText(JSON.stringify(selected, null, 2))}>Copy JSON</button></div><pre>{JSON.stringify(selected, null, 2)}</pre></div></article></div>}
    </main>
  );
}

function Stat({ label, value, detail, accent }: { label: string; value: string; detail: string; accent: string }) { return <div className={`stat ${accent}`}><div className="stat-icon">✦</div><p>{label}</p><strong>{value}</strong><span>{detail}</span></div> }
function Health({ label, value, suffix }: { label: string; value: number; suffix?: string }) { return <div className="health"><span>{label}</span><strong>{number(value)}{suffix ?? ""}</strong><div className="meter"><i style={{ width: `${Math.min(100, Math.max(6, value ? 42 : 8))}%` }} /></div></div> }
function Service({ name, port }: { name: string; port: string }) { return <div className="service"><i /><span>{name}</span><code>{port}</code></div> }
function Detail({ label, value }: { label: string; value: string }) { return <div><span>{label}</span><strong>{value}</strong></div> }