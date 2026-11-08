"use client";

import { useEffect, useMemo, useState } from "react";
import { AlertTriangle } from "lucide-react";
import type { FinishedJob, Pipeline as Data, PipelineWorker, WorkerState } from "@/lib/types";

const LIVE_MS = 1000;
const OFFLINE_MS = 3000;

// usePipeline polls the website's pipeline route, faster while a coordinator answers, and stops while the tab is hidden.
function usePipeline() {
  const [data, setData] = useState<Data | null>(null);
  const [offline, setOffline] = useState(false);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    let busy = false;
    let stopped = false;
    const ctrl = new AbortController();
    const schedule = (ms: number) => {
      clearTimeout(timer);
      timer = setTimeout(poll, ms);
    };
    async function poll() {
      if (busy || stopped) return;
      busy = true;
      let ok = false;
      try {
        const res = await fetch("/api/pipeline", { cache: "no-store", signal: ctrl.signal });
        if (res.ok) {
          setData(await res.json());
          ok = true;
        }
      } catch {
        if (ctrl.signal.aborted) return;
      } finally {
        busy = false;
      }
      setOffline(!ok);
      setLoaded(true);
      if (!document.hidden) schedule(ok ? LIVE_MS : OFFLINE_MS);
    }
    const onVisibility = () => (document.hidden ? clearTimeout(timer) : schedule(0));

    poll();
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      stopped = true;
      ctrl.abort();
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, []);

  return { data, offline, loaded };
}

type Bar = { job?: FinishedJob; worker: string; start: number; end: number; running: boolean; path: string; attempt: number };

// runShape places every job on the coordinator's clock: when the run started, when it ended (or now), and one bar per job.
function runShape(d: Data) {
  const now = Date.parse(d.status.now);
  const bars: Bar[] = d.jobs.map((j) => {
    const end = Date.parse(j.finishedAt);
    return { job: j, worker: j.worker, start: end - j.durationMs, end, running: false, path: j.path, attempt: j.attempt };
  });
  for (const w of d.status.workers) {
    if (w.state === "working" && w.path) {
      bars.push({ worker: w.name, start: now - (w.jobSeconds ?? 0) * 1000, end: now, running: true, path: w.path, attempt: 0 });
    }
  }
  const { pending, running, total } = d.status;
  const finished = total > 0 && pending === 0 && running === 0;
  const start = bars.length ? Math.min(...bars.map((b) => b.start)) : now;
  const end = finished && d.jobs.length ? Math.max(...d.jobs.map((j) => Date.parse(j.finishedAt))) : now;
  const lanes = [...new Set([...d.status.workers.map((w) => w.name), ...d.jobs.map((j) => j.worker)])].filter(Boolean).sort();
  const pages = d.jobs.reduce((n, j) => n + j.pages, 0);
  return { now, bars, start, end, span: Math.max(end - start, 1000), finished, lanes, pages };
}

type Shape = ReturnType<typeof runShape>;

export function Pipeline() {
  const { data, offline, loaded } = usePipeline();
  const shape = useMemo(() => (data ? runShape(data) : null), [data]);

  return (
    <main className="relative z-10 mx-auto max-w-6xl px-6 pb-24">
      <div className="flex flex-col gap-4 pt-6 pb-10 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight text-zinc-50">Pipeline</h1>
          <p className="mt-2 max-w-xl text-zinc-400">
            Common Crawl files moving through the coordinator and its workers, updated every second.
          </p>
        </div>
        {data && shape && <RunPill data={data} shape={shape} offline={offline} />}
      </div>

      {!loaded && <p className="text-sm text-zinc-500">Connecting to the coordinator…</p>}
      {loaded && !data && <NoPipeline />}
      {data && shape && (
        <>
          {offline && (
            <p className="mb-6 rounded-lg border border-zinc-800 bg-zinc-900/60 px-4 py-3 text-sm text-zinc-400">
              Lost contact with the coordinator. Showing the last update; this page reconnects on its own.
            </p>
          )}
          <Stats data={data} shape={shape} />
          <Progress data={data} />
          <Section title="Workers">
            <Workers workers={data.status.workers} />
          </Section>
          <Section title="Timeline" note="Each bar is one file, on the row of the worker that processed it.">
            <Timeline shape={shape} />
          </Section>
          <Section title="Pages processed over time">
            <PagesChart data={data} shape={shape} />
          </Section>
          {data.status.failures.length > 0 && (
            <Section title="Failed files">
              <Failures failures={data.status.failures} />
            </Section>
          )}
          <Section title="Recently finished">
            <Recent jobs={data.jobs} />
          </Section>
        </>
      )}
    </main>
  );
}

function RunPill({ data, shape, offline }: { data: Data; shape: Shape; offline: boolean }) {
  const { label, dot } = offline
    ? { label: "Disconnected", dot: "bg-zinc-500" }
    : data.status.total === 0
      ? { label: "Waiting for jobs", dot: "bg-zinc-500" }
      : shape.finished
        ? { label: "Run finished", dot: "bg-emerald-400" }
        : { label: "Running", dot: "bg-blue-400 animate-pulse" };
  return (
    <span className="inline-flex items-center gap-2 self-start rounded-full border border-zinc-800 bg-zinc-900/60 px-3 py-1 text-sm text-zinc-300 sm:self-auto">
      <span className={`h-2 w-2 rounded-full ${dot}`} aria-hidden="true" />
      {label}
    </span>
  );
}

function Stats({ data, shape }: { data: Data; shape: Shape }) {
  const s = data.status;
  const seconds = (shape.end - shape.start) / 1000;
  const tiles = [
    { label: "Files done", value: `${fmt(s.done)} / ${fmt(s.total)}` },
    { label: "Pages processed", value: fmt(s.pages) },
    { label: "Links found", value: fmt(s.crossLinks), hint: "between different sites" },
    { label: "Unique site pairs", value: fmt(s.edges), hint: "summed per file" },
    { label: "Pages / second", value: shape.bars.length ? fmt(Math.round(shape.pages / Math.max(seconds, 1))) : "–", hint: "average over the run" },
    { label: shape.finished ? "Run took" : "Elapsed", value: shape.bars.length ? duration(seconds) : "–" },
  ];
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
      {tiles.map((t) => (
        <div key={t.label} className="rounded-xl border border-zinc-800 bg-zinc-900/50 px-4 py-3">
          <div className="text-xs text-zinc-500">{t.label}</div>
          <div className="mt-1 font-mono text-xl text-zinc-100 tabular-nums">{t.value}</div>
          {t.hint && <div className="mt-0.5 text-[11px] text-zinc-600">{t.hint}</div>}
        </div>
      ))}
    </div>
  );
}

function Progress({ data }: { data: Data }) {
  const s = data.status;
  const parts = [
    { label: "Done", n: s.done, color: "bg-blue-500" },
    { label: "Running", n: s.running, color: "bg-blue-300/60" },
    { label: "Waiting", n: s.pending, color: "bg-zinc-700" },
    { label: "Failed", n: s.failed, color: "bg-red-400" },
  ];
  return (
    <div className="mt-6">
      <div className="flex h-2.5 gap-[2px] overflow-hidden rounded-full bg-zinc-900" role="img" aria-label={parts.map((p) => `${p.label} ${p.n}`).join(", ")}>
        {parts.filter((p) => p.n > 0).map((p) => (
          <div key={p.label} className={`${p.color} transition-[flex-grow] duration-700`} style={{ flexGrow: p.n }} />
        ))}
      </div>
      <div className="mt-2 flex flex-wrap gap-x-5 gap-y-1 text-xs text-zinc-400">
        {parts.map((p) => (
          <span key={p.label} className="inline-flex items-center gap-1.5">
            <span className={`h-2 w-2 rounded-full ${p.color}`} aria-hidden="true" />
            {p.label} <span className="font-mono text-zinc-200 tabular-nums">{fmt(p.n)}</span>
          </span>
        ))}
      </div>
    </div>
  );
}

const STATES: Record<WorkerState, { label: string; dot: string }> = {
  working: { label: "Working", dot: "bg-blue-400 animate-pulse" },
  idle: { label: "Idle", dot: "bg-zinc-500" },
  finished: { label: "Finished", dot: "bg-emerald-400" },
  gone: { label: "No contact", dot: "bg-red-400" },
  offline: { label: "Offline", dot: "bg-zinc-600" },
};

function Workers({ workers }: { workers: PipelineWorker[] }) {
  if (!workers.length) return <p className="text-sm text-zinc-500">No workers have checked in yet.</p>;
  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
      {workers.map((w) => {
        const st = STATES[w.state];
        return (
          <div key={w.name} className="rounded-xl border border-zinc-800 bg-zinc-900/50 px-4 py-3">
            <div className="flex items-center justify-between gap-2">
              <span className="truncate font-mono text-sm text-zinc-100" title={w.name}>{w.name}</span>
              <span className="inline-flex shrink-0 items-center gap-1.5 text-xs text-zinc-400">
                <span className={`h-2 w-2 rounded-full ${st.dot}`} aria-hidden="true" />
                {st.label}
              </span>
            </div>
            <div className="mt-2 h-4 truncate text-xs text-zinc-500" title={w.path}>
              {w.state === "working" && w.path
                ? <>On <span className="font-mono text-zinc-300">{fileLabel(w.path)}</span> for {duration(w.jobSeconds ?? 0)}</>
                : w.state === "gone"
                  ? `Last heard from ${duration(w.lastSeenSeconds)} ago`
                  : " "}
            </div>
            <div className="mt-2 text-xs text-zinc-400">
              <span className="font-mono text-zinc-200 tabular-nums">{fmt(w.done)}</span> files ·{" "}
              <span className="font-mono text-zinc-200 tabular-nums">{fmt(w.pages)}</span> pages
            </div>
          </div>
        );
      })}
    </div>
  );
}

type Tip = { x: number; y: number; lines: string[] };

function Tooltip({ tip }: { tip: Tip | null }) {
  if (!tip) return null;
  return (
    <div
      className="pointer-events-none fixed z-50 rounded-lg border border-zinc-700 bg-zinc-900/95 px-3 py-2 text-xs shadow-xl"
      style={{ left: tip.x + 14, top: tip.y + 14 }}
    >
      {tip.lines.map((l, i) => (
        <div key={i} className={i === 0 ? "font-mono text-zinc-100" : "text-zinc-400"}>{l}</div>
      ))}
    </div>
  );
}

function Timeline({ shape }: { shape: Shape }) {
  const [tip, setTip] = useState<Tip | null>(null);
  if (!shape.bars.length) return <p className="text-sm text-zinc-500">No files started yet.</p>;
  const pct = (t: number) => ((t - shape.start) / shape.span) * 100;
  const show = (b: Bar) => (e: React.MouseEvent) =>
    setTip({
      x: e.clientX,
      y: e.clientY,
      lines: [
        fileLabel(b.path),
        `${b.worker} · ${b.running ? `running for ${duration((b.end - b.start) / 1000)}` : `took ${duration((b.end - b.start) / 1000)}`}`,
        ...(b.job ? [`${fmt(b.job.pages)} pages · ${fmt(b.job.edges)} site pairs`] : []),
        ...(b.attempt > 1 ? [`attempt ${b.attempt}, an earlier try failed or timed out`] : []),
      ],
    });

  return (
    <div>
      <div className="space-y-1.5">
        {shape.lanes.map((lane) => (
          <div key={lane} className="flex items-center gap-3">
            <div className="w-28 shrink-0 truncate text-right font-mono text-xs text-zinc-400" title={lane}>{lane}</div>
            <div className="relative h-6 flex-1 rounded bg-zinc-900/70">
              {shape.bars.filter((b) => b.worker === lane).map((b, i) => (
                <div
                  key={b.job ? b.job.id : `run-${i}`}
                  onMouseMove={show(b)}
                  onMouseLeave={() => setTip(null)}
                  className={`absolute inset-y-1 rounded-sm transition-[width] duration-700 ${
                    b.running ? "animate-pulse bg-blue-300/50" : b.attempt > 1 ? "bg-blue-500 ring-1 ring-amber-300" : "bg-blue-500/85 hover:bg-blue-400"
                  }`}
                  style={{ left: `${pct(b.start)}%`, width: `max(calc(${pct(b.end) - pct(b.start)}% - 2px), 2px)` }}
                />
              ))}
            </div>
          </div>
        ))}
      </div>
      <div className="mt-1 flex gap-3">
        <div className="w-28 shrink-0" />
        <Axis span={shape.span} />
      </div>
      <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-xs text-zinc-400">
        <Key className="bg-blue-500/85">Finished file</Key>
        <Key className="bg-blue-300/50">Running</Key>
        <Key className="bg-blue-500 ring-1 ring-amber-300">Finished on a retry</Key>
      </div>
      <Tooltip tip={tip} />
    </div>
  );
}

function Key({ className, children }: { className: string; children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className={`h-2.5 w-4 rounded-sm ${className}`} aria-hidden="true" />
      {children}
    </span>
  );
}

const STEPS = [1, 2, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200];

function Axis({ span }: { span: number }) {
  const seconds = span / 1000;
  const step = STEPS.find((s) => seconds / s <= 6) ?? 7200;
  const ticks: number[] = [];
  for (let t = 0; t <= seconds; t += step) ticks.push(t);
  return (
    <div className="relative h-4 flex-1 text-[11px] text-zinc-600">
      {ticks.map((t) => (
        <span key={t} className="absolute -translate-x-1/2 tabular-nums first:translate-x-0" style={{ left: `${(t / seconds) * 100}%` }}>
          {duration(t)}
        </span>
      ))}
    </div>
  );
}

const W = 1000;
const H = 180;

function PagesChart({ data, shape }: { data: Data; shape: Shape }) {
  const [hover, setHover] = useState<{ x: number; t: number; pages: number; cx: number; cy: number } | null>(null);
  const steps = useMemo(() => {
    const out: { t: number; pages: number }[] = [];
    let total = 0;
    for (const j of [...data.jobs].sort((a, b) => Date.parse(a.finishedAt) - Date.parse(b.finishedAt))) {
      total += j.pages;
      out.push({ t: Date.parse(j.finishedAt), pages: total });
    }
    return out;
  }, [data.jobs]);

  if (!steps.length) return <p className="text-sm text-zinc-500">The first file hasn&apos;t finished yet.</p>;
  const max = steps[steps.length - 1].pages;
  const x = (t: number) => ((t - shape.start) / shape.span) * W;
  const y = (p: number) => H - (p / max) * (H - 8);
  let d = `M0 ${H}`;
  for (const s of steps) d += ` H${x(s.t).toFixed(1)} V${y(s.pages).toFixed(1)}`;
  d += ` H${x(shape.end).toFixed(1)}`;
  const pagesAt = (t: number) => steps.filter((s) => s.t <= t).at(-1)?.pages ?? 0;

  const onMove = (e: React.MouseEvent<HTMLDivElement>) => {
    const r = e.currentTarget.getBoundingClientRect();
    const f = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width));
    const t = shape.start + f * shape.span;
    setHover({ x: f * 100, t: (t - shape.start) / 1000, pages: pagesAt(t), cx: e.clientX, cy: e.clientY });
  };

  return (
    <div className="flex gap-3">
      <div className="flex w-28 shrink-0 flex-col justify-between pb-5 text-right font-mono text-[11px] text-zinc-600 tabular-nums">
        <span>{fmt(max)}</span>
        <span>{fmt(Math.round(max / 2))}</span>
        <span>0</span>
      </div>
      <div className="flex-1">
        <div className="relative h-44 cursor-crosshair" onMouseMove={onMove} onMouseLeave={() => setHover(null)}>
          <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="absolute inset-0 h-full w-full" aria-label={`Pages processed, reaching ${fmt(max)}`} role="img">
            {[0, 0.5, 1].map((f) => (
              <line key={f} x1={0} x2={W} y1={y(max * f)} y2={y(max * f)} stroke="#27272a" strokeWidth={1} vectorEffect="non-scaling-stroke" />
            ))}
            <path d={`${d} V${H} Z`} fill="rgba(59,130,246,0.12)" />
            <path d={d} fill="none" stroke="#60a5fa" strokeWidth={2} vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
          </svg>
          {hover && <div className="pointer-events-none absolute inset-y-0 w-px bg-zinc-500" style={{ left: `${hover.x}%` }} />}
        </div>
        <div className="mt-1 flex">
          <Axis span={shape.span} />
        </div>
        <Tooltip tip={hover && { x: hover.cx, y: hover.cy, lines: [`${fmt(hover.pages)} pages`, `after ${duration(hover.t)}`] }} />
      </div>
    </div>
  );
}

function Failures({ failures }: { failures: Data["status"]["failures"] }) {
  return (
    <ul className="space-y-2">
      {failures.map((f) => (
        <li key={f.id} className="flex gap-3 rounded-xl border border-zinc-800 bg-zinc-900/50 px-4 py-3 text-sm">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-red-400" aria-hidden="true" />
          <div className="min-w-0">
            <div className="text-zinc-200">
              Failed after {f.attempts} attempts: <span className="font-mono">{fileLabel(f.path)}</span>
            </div>
            <div className="mt-0.5 truncate text-xs text-zinc-500" title={f.error}>{f.error}</div>
          </div>
        </li>
      ))}
    </ul>
  );
}

function Recent({ jobs }: { jobs: FinishedJob[] }) {
  if (!jobs.length) return <p className="text-sm text-zinc-500">Nothing finished yet.</p>;
  const rows = jobs.slice(-8).reverse();
  return (
    <div className="overflow-x-auto rounded-xl border border-zinc-800">
      <table className="w-full text-sm">
        <thead className="bg-zinc-900/70 text-left text-xs text-zinc-500">
          <tr>
            <th className="px-4 py-2 font-medium">File</th>
            <th className="px-4 py-2 font-medium">Worker</th>
            <th className="px-4 py-2 text-right font-medium">Pages</th>
            <th className="px-4 py-2 text-right font-medium">Site pairs</th>
            <th className="px-4 py-2 text-right font-medium">Took</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-zinc-900">
          {rows.map((j) => (
            <tr key={j.id} className="text-zinc-300">
              <td className="max-w-[18rem] truncate px-4 py-2 font-mono text-xs" title={j.path}>
                {fileLabel(j.path)}
                {j.attempt > 1 && <span className="ml-2 rounded bg-zinc-800 px-1.5 py-0.5 text-[10px] text-amber-300">attempt {j.attempt}</span>}
              </td>
              <td className="px-4 py-2 font-mono text-xs text-zinc-400">{j.worker}</td>
              <td className="px-4 py-2 text-right font-mono tabular-nums">{fmt(j.pages)}</td>
              <td className="px-4 py-2 text-right font-mono tabular-nums">{fmt(j.edges)}</td>
              <td className="px-4 py-2 text-right font-mono tabular-nums">{duration(j.durationMs / 1000)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function NoPipeline() {
  return (
    <div className="rounded-xl border border-zinc-800 bg-zinc-900/50 px-6 py-8">
      <h2 className="text-lg font-medium text-zinc-100">No pipeline is running right now</h2>
      <p className="mt-2 max-w-2xl text-sm text-zinc-400">
        This page shows the coordinator live while a run is going. Start one and it shows up here within a few seconds.
      </p>
      <pre className="mt-5 overflow-x-auto rounded-lg bg-black/40 px-4 py-3 font-mono text-xs leading-6 text-zinc-300">
        <span className="text-zinc-500"># small local run on copies of the sample file, no downloads</span>
        {"\n"}make demo{"\n\n"}
        <span className="text-zinc-500"># Docker: a coordinator and 4 workers on real Common Crawl files</span>
        {"\n"}make up
      </pre>
    </div>
  );
}

function Section({ title, note, children }: { title: string; note?: string; children: React.ReactNode }) {
  return (
    <section className="mt-14">
      <h2 className="text-sm font-medium uppercase tracking-widest text-zinc-500">{title}</h2>
      {note && <p className="mt-1 text-xs text-zinc-600">{note}</p>}
      <div className="mt-4">{children}</div>
    </section>
  );
}

function fmt(n: number) {
  return n.toLocaleString("en-US");
}

function duration(seconds: number) {
  if (seconds < 10) return `${Math.max(0, seconds).toFixed(seconds < 1 && seconds > 0 ? 1 : 0)}s`;
  const total = Math.round(seconds);
  if (total < 60) return `${total}s`;
  const m = Math.floor(total / 60), s = total % 60;
  if (m < 60) return s ? `${m}m ${s}s` : `${m}m`;
  return `${Math.floor(m / 60)}h ${m % 60}m`;
}

// fileLabel drops the folders and the ".warc.wat.gz" ending from a Common Crawl path.
function fileLabel(path: string) {
  return (path.split("/").pop() ?? path).replace(/\.warc\.wat\.gz$/, "");
}
