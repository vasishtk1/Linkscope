"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Pause, Play, RotateCcw } from "lucide-react";
import type { Pipeline as Live, Recording } from "@/lib/types";
import { clock, fileLabel, fmt, focusProgress, frameAt, makeFocus, makeRun, type Focus, type Run } from "@/lib/replay";
import { clearSearches, useSearches, type SavedSearch } from "@/lib/searches";
import { PipelineScene } from "./pipeline-scene";

const FRAME_MS = 1000 / 30;
const FLIGHT_DISPLAY_MS = 700;
const SPEEDS = [
  { key: "fit", label: "Fit to 30s" },
  { key: "1", label: "1×" },
  { key: "10", label: "10×" },
  { key: "60", label: "60×" },
] as const;
type Speed = (typeof SPEEDS)[number]["key"];

// useLive polls the coordinator through the website every second, or every 4 seconds while nothing answers.
function useLive() {
  const [live, setLive] = useState<{ data: Live; at: number } | null>(null);
  const [checked, setChecked] = useState(false);
  const [sawActive, setSawActive] = useState(false);
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    let busy = false;
    const ctrl = new AbortController();
    const schedule = (ms: number) => {
      clearTimeout(timer);
      timer = setTimeout(poll, ms);
    };
    async function poll() {
      if (busy) return;
      busy = true;
      let ok = false;
      try {
        const res = await fetch("/api/pipeline", { cache: "no-store", signal: ctrl.signal });
        if (res.ok) {
          const data: Live = await res.json();
          setLive({ data, at: performance.now() });
          if (data.status.pending + data.status.running > 0) setSawActive(true);
          ok = true;
        }
      } catch {
        if (ctrl.signal.aborted) return;
      } finally {
        busy = false;
      }
      if (!ok) setLive(null);
      setChecked(true);
      if (!document.hidden) schedule(ok ? 1000 : 4000);
    }
    const onVisibility = () => (document.hidden ? clearTimeout(timer) : schedule(0));
    poll();
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      ctrl.abort();
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, []);
  return { live, checked, sawActive };
}

function useRecording() {
  const [rec, setRec] = useState<Recording | null>(null);
  const [checked, setChecked] = useState(false);
  useEffect(() => {
    const ctrl = new AbortController();
    fetch("/api/run", { signal: ctrl.signal })
      .then((res) => (res.ok ? res.json() : null))
      .then((r) => {
        setRec(r);
        setChecked(true);
      })
      .catch(() => {
        if (!ctrl.signal.aborted) setChecked(true);
      });
    return () => ctrl.abort();
  }, []);
  return { rec, checked };
}

export function Pipeline() {
  const { live, checked: liveChecked, sawActive } = useLive();
  const { rec, checked: recChecked } = useRecording();
  const [picked, setChoice] = useState<"saved" | "coordinator" | null>(null);
  const searches = useSearches();
  const router = useRouter();
  const pathname = usePathname();
  const wanted = useSearchParams().get("search");
  const selected = searches.find((s) => s.domain === wanted) ?? null;
  const select = (domain: string | null) =>
    router.replace(domain ? `${pathname}?search=${encodeURIComponent(domain)}` : pathname, { scroll: false });

  const liveActive = !!live && live.data.status.pending + live.data.status.running > 0;
  const liveHasRun = !!live && live.data.events.length > 0;
  // After watching a run live, keep showing that run instead of jumping back to the saved one.
  const choice = picked ?? (sawActive ? "coordinator" : "saved");
  // A search's links come from the saved run's index, so a picked search replays the saved run, even during a live one.
  const source = selected && rec
    ? "saved"
    : liveActive
      ? "live"
      : choice === "coordinator" && liveHasRun ? "coordinator" : rec ? "saved" : liveHasRun ? "coordinator" : null;

  const run = useMemo<Run | null>(() => {
    if ((source === "live" || source === "coordinator") && live) {
      const s = live.data.status;
      return makeRun({
        name: source === "live" ? "Live run" : "Last run on the coordinator",
        derived: false,
        total: s.total,
        workers: s.workers.map((w) => w.name),
        events: live.data.events,
        now: source === "live" ? Date.parse(s.now) : undefined,
      });
    }
    if (source === "saved" && rec) {
      return makeRun({ name: rec.name, derived: rec.derived, total: rec.totals.files, workers: rec.workers, events: rec.events });
    }
    return null;
  }, [source, live, rec]);

  const focus = useMemo<Focus | null>(() => (run && selected && source !== "live" ? makeFocus(run, selected) : null), [run, selected, source]);
  const loaded = liveChecked && recChecked;

  return (
    <main className="relative z-10 mx-auto max-w-6xl px-6 pb-24">
      <div className="pt-6 pb-8">
        <h1 className="text-3xl font-semibold tracking-tight text-zinc-50">Pipeline</h1>
        <p className="mt-2 max-w-2xl text-zinc-400">
          How the data behind Linkscope gets built. Common Crawl files are handed out to workers, processed side by side, retried
          when something breaks, then merged, scored and indexed. Press play to watch a real run.
        </p>
      </div>

      <div className="grid gap-6 lg:grid-cols-[200px_minmax(0,1fr)]">
        <SearchList searches={searches} selected={selected?.domain ?? null} onSelect={select} />
        <div className="min-w-0">
          {!loaded && <p className="text-sm text-zinc-500">Loading…</p>}
          {loaded && !run && <NothingYet />}
          {run && source === "live" && live && <LiveView run={run} receivedAt={live.at} serverNow={Date.parse(live.data.status.now)} />}
          {run && source !== "live" && (
            <>
              {selected && focus ? (
                <FocusBar search={selected} focus={focus} total={run.total} onClear={() => select(null)} />
              ) : (
                <SourceBar
                  run={run}
                  rec={source === "saved" ? rec : null}
                  canSwitch={!!rec && liveHasRun}
                  source={source as "saved" | "coordinator"}
                  onSwitch={setChoice}
                />
              )}
              <Replay key={`${source}-${run.events.length}-${selected?.domain ?? ""}`} run={run} focus={focus && focus.matched > 0 ? focus : undefined} />
            </>
          )}
        </div>
      </div>
    </main>
  );
}

// SearchList is "Full run" plus every site searched on this device, newest first; a row on phones, a column on wide screens.
function SearchList({ searches, selected, onSelect }: { searches: SavedSearch[]; selected: string | null; onSelect: (d: string | null) => void }) {
  const item = (on: boolean) =>
    `shrink-0 rounded-lg px-3 py-2 text-left text-sm transition ${on ? "bg-zinc-800 text-zinc-100" : "text-zinc-400 hover:bg-zinc-900 hover:text-zinc-200"}`;
  return (
    <aside className="min-w-0">
      <div className="flex gap-1 overflow-x-auto pb-1 lg:flex-col lg:overflow-visible">
        <button onClick={() => onSelect(null)} className={item(!selected)} aria-pressed={!selected}>
          Full run
        </button>
        <div className="flex shrink-0 items-center justify-between px-3 pt-3 pb-1 text-[11px] font-medium uppercase tracking-widest text-zinc-500 max-lg:hidden">
          Your searches
          {searches.length > 0 && (
            <button onClick={() => { clearSearches(); onSelect(null); }} className="normal-case tracking-normal text-zinc-600 hover:text-zinc-300">
              Clear
            </button>
          )}
        </div>
        {searches.map((s) => (
          <button key={s.domain} onClick={() => onSelect(s.domain)} className={item(s.domain === selected)} aria-pressed={s.domain === selected}>
            <span className="block max-w-[11rem] truncate font-mono text-[13px]">{s.domain}</span>
            <span className="block text-[11px] text-zinc-500 max-lg:hidden">
              {s.originFiles ? `${fmt(s.originFiles)} ${s.originFiles === 1 ? "file" : "files"}` : "no file data"}
              {s.score !== null && ` · score ${s.score}`}
            </span>
          </button>
        ))}
      </div>
      {searches.length === 0 && (
        <p className="mt-2 px-3 text-xs text-zinc-500 max-lg:hidden">
          Sites you <Link href="/" className="text-zinc-300 underline-offset-2 hover:underline">search</Link> show up here, on this device, so you can replay how each one was found.
        </p>
      )}
    </aside>
  );
}

// FocusBar sits above the scene while a search is picked and says how its files fit into this run.
function FocusBar({ search, focus, total, onClear }: { search: SavedSearch; focus: Focus; total: number; onClear: () => void }) {
  const capped = focus.originFiles > focus.stored;
  return (
    <div className="mb-4 flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
      <div className="text-sm text-zinc-400">
        <span className="text-zinc-200">Following <span className="font-mono">{search.domain}</span></span>
        {focus.matched > 0 ? (
          <span className="text-zinc-500">
            {" "}· its links were found in {fmt(focus.matched)} of {fmt(total)} files
          </span>
        ) : (
          <div className="mt-1 text-xs text-amber-300/90">
            This search came from different data than the run saved here, so none of its files are in this replay. Search it again after the site is restarted on this run.
          </div>
        )}
        {capped && focus.matched > 0 && (
          <div className="mt-1 text-xs text-zinc-500">
            Showing the {fmt(focus.stored)} files with the most links out of {fmt(focus.originFiles)}.
          </div>
        )}
      </div>
      <button onClick={onClear} className="self-start rounded-lg border border-zinc-800 px-2.5 py-1 text-xs text-zinc-400 transition hover:border-zinc-600 hover:text-zinc-200">
        Show full run
      </button>
    </div>
  );
}

function SourceBar({ run, rec, canSwitch, source, onSwitch }: {
  run: Run; rec: Recording | null; canSwitch: boolean; source: "saved" | "coordinator"; onSwitch: (s: "saved" | "coordinator") => void;
}) {
  return (
    <div className="mb-4 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div className="text-sm text-zinc-400">
        <span className="text-zinc-200">{run.name}</span>
        {rec && <span className="text-zinc-500"> · recorded {new Date(rec.recordedAt).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" })}</span>}
        {run.derived && (
          <div className="mt-1 text-xs text-zinc-500">This run was saved before failures were logged, so retries from it aren&apos;t shown.</div>
        )}
      </div>
      {canSwitch && (
        <div className="flex gap-1 self-start rounded-lg border border-zinc-800 p-0.5 text-xs">
          {(["saved", "coordinator"] as const).map((s) => (
            <button
              key={s}
              onClick={() => onSwitch(s)}
              className={`rounded-md px-2.5 py-1 transition ${source === s ? "bg-zinc-800 text-zinc-100" : "text-zinc-400 hover:text-zinc-200"}`}
            >
              {s === "saved" ? "Saved run" : "Last run on the coordinator"}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

// Replay plays a finished run: the clock moves only while playing, so a paused page does no work.
function Replay({ run, focus }: { run: Run; focus?: Focus }) {
  const span = Math.max(run.end - run.start, 1);
  const [t, setT] = useState(run.start);
  const [playing, setPlaying] = useState(false);
  const [speed, setSpeed] = useState<Speed>("fit");
  const rate = speed === "fit" ? span / 30_000 : Number(speed);
  const tRef = useRef(t);
  useEffect(() => {
    tRef.current = t;
  }, [t]);

  useEffect(() => {
    if (!playing) return;
    let raf = 0;
    let last = performance.now();
    const tick = (now: number) => {
      raf = requestAnimationFrame(tick);
      if (now - last < FRAME_MS - 1) return;
      const next = Math.min(run.end, tRef.current + (now - last) * rate);
      last = now;
      setT(next);
      if (next >= run.end) setPlaying(false);
    };
    raf = requestAnimationFrame(tick);
    const onVisibility = () => document.hidden && setPlaying(false);
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      cancelAnimationFrame(raf);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [playing, rate, run.end]);

  const frame = frameAt(run, t, FLIGHT_DISPLAY_MS * rate);
  const atEnd = t >= run.end;
  const toggle = () => {
    if (atEnd) setT(run.start);
    setPlaying((p) => !p || atEnd);
  };

  return (
    <div>
      <div className="relative rounded-2xl border border-zinc-800 bg-zinc-950/60 p-4">
        <PipelineScene frame={frame} total={run.total} focus={focus} />
        {!playing && t === run.start && (
          <button
            onClick={toggle}
            className="absolute inset-0 m-auto flex h-16 w-16 items-center justify-center rounded-full bg-blue-500 text-white shadow-lg shadow-blue-500/30 transition hover:scale-105 hover:bg-blue-400"
            aria-label="Play the run"
          >
            <Play className="ml-1 h-7 w-7" fill="currentColor" />
          </button>
        )}
      </div>

      <div className="mt-4 flex flex-wrap items-center gap-3">
        <button
          onClick={toggle}
          className="inline-flex items-center gap-2 rounded-lg bg-zinc-100 px-3 py-1.5 text-sm font-medium text-zinc-900 transition hover:bg-white"
        >
          {playing ? <Pause className="h-4 w-4" fill="currentColor" /> : <Play className="h-4 w-4" fill="currentColor" />}
          {playing ? "Pause" : atEnd ? "Replay" : "Play"}
        </button>
        <button
          onClick={() => { setPlaying(false); setT(run.start); }}
          className="inline-flex items-center gap-1.5 rounded-lg border border-zinc-800 px-3 py-1.5 text-sm text-zinc-300 transition hover:border-zinc-600"
        >
          <RotateCcw className="h-4 w-4" /> Restart
        </button>
        <div className="flex gap-1 rounded-lg border border-zinc-800 p-0.5 text-xs">
          {SPEEDS.map((s) => (
            <button
              key={s.key}
              onClick={() => setSpeed(s.key)}
              className={`rounded-md px-2.5 py-1 transition ${speed === s.key ? "bg-zinc-800 text-zinc-100" : "text-zinc-400 hover:text-zinc-200"}`}
            >
              {s.label}
            </button>
          ))}
        </div>
        <span className="ml-auto font-mono text-sm text-zinc-400 tabular-nums">
          {clock(t - run.start)} <span className="text-zinc-600">/ {clock(span)} real time</span>
        </span>
      </div>

      <Scrubber run={run} t={t} focus={focus} onSeek={(v) => { setPlaying(false); setT(v); }} />
      <Counters frame={frame} total={run.total} focus={focus} />
    </div>
  );
}

// LiveView follows a run as it happens, moving the clock smoothly between the once-a-second updates.
function LiveView({ run, receivedAt, serverNow }: { run: Run; receivedAt: number; serverNow: number }) {
  const [now, setNow] = useState(() => performance.now());
  useEffect(() => {
    let raf = 0;
    let last = 0;
    const tick = (n: number) => {
      raf = requestAnimationFrame(tick);
      if (n - last < FRAME_MS - 1 || document.hidden) return;
      last = n;
      setNow(n);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, []);
  const t = serverNow + Math.max(0, now - receivedAt);
  const frame = frameAt(run, t, FLIGHT_DISPLAY_MS);
  return (
    <div>
      <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-zinc-800 bg-zinc-900/60 px-3 py-1 text-sm text-zinc-300">
        <span className="h-2 w-2 animate-pulse rounded-full bg-blue-400" aria-hidden="true" />
        Live: a run is happening right now
        <span className="font-mono text-zinc-500 tabular-nums">{clock(t - run.start)}</span>
      </div>
      <div className="rounded-2xl border border-zinc-800 bg-zinc-950/60 p-4">
        <PipelineScene frame={frame} total={run.total} />
      </div>
      <Counters frame={frame} total={run.total} />
    </div>
  );
}

function Scrubber({ run, t, focus, onSeek }: { run: Run; t: number; focus?: Focus; onSeek: (t: number) => void }) {
  const span = Math.max(run.end - run.start, 1);
  const ref = useRef<HTMLDivElement>(null);
  const lanes = run.workers.length || 1;
  const seek = (clientX: number) => {
    const r = ref.current!.getBoundingClientRect();
    onSeek(run.start + Math.min(1, Math.max(0, (clientX - r.left) / r.width)) * span);
  };
  const pct = (v: number) => ((v - run.start) / span) * 100;
  return (
    <div className="mt-4">
      <div
        ref={ref}
        role="slider"
        aria-label="Position in the run"
        aria-valuemin={0}
        aria-valuemax={Math.round(span / 1000)}
        aria-valuenow={Math.round((t - run.start) / 1000)}
        tabIndex={0}
        onPointerDown={(e) => { e.currentTarget.setPointerCapture(e.pointerId); seek(e.clientX); }}
        onPointerMove={(e) => e.buttons === 1 && seek(e.clientX)}
        onKeyDown={(e) => {
          if (e.key === "ArrowRight") onSeek(Math.min(run.end, t + span / 50));
          if (e.key === "ArrowLeft") onSeek(Math.max(run.start, t - span / 50));
        }}
        className="relative cursor-pointer touch-none rounded-lg bg-zinc-900/70 py-2 outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
      >
        <div className="relative mx-2" style={{ height: lanes * 6 - 2 }}>
          {run.segments.map((s, i) => (
            <div
              key={i}
              className={`absolute h-1 rounded-full ${s.outcome === "failed" ? "bg-red-400/80" : focus && !focus.pages.has(fileLabel(s.path)) ? "bg-zinc-700" : "bg-blue-500/50"}`}
              style={{ left: `${pct(s.from)}%`, width: `max(${pct(s.to) - pct(s.from)}%, 2px)`, top: Math.max(0, run.workers.indexOf(s.worker)) * 6 }}
            />
          ))}
          <div className="absolute -inset-y-1 w-0.5 rounded bg-zinc-100" style={{ left: `${pct(t)}%` }} />
        </div>
      </div>
      <div className="mt-1.5 flex gap-4 text-[11px] text-zinc-500">
        <span>One row per worker, one bar per file. Drag to jump.</span>
        <span className="inline-flex items-center gap-1"><span className="h-1 w-3 rounded-full bg-blue-500/50" /> processed</span>
        <span className="inline-flex items-center gap-1"><span className="h-1 w-3 rounded-full bg-red-400/80" /> failed, retried</span>
      </div>
    </div>
  );
}

function Counters({ frame, total, focus }: { frame: ReturnType<typeof frameAt>; total: number; focus?: Focus }) {
  const got = focus && focusProgress(focus, frame);
  const tiles = [
    ...(focus && got
      ? [{ label: `Links to ${focus.domain}`, value: `${fmt(got.pages)} / ${fmt(focus.totalPages)}`, hint: `${fmt(got.files)} of ${fmt(focus.matched)} files in` }]
      : []),
    { label: "Files done", value: `${fmt(frame.done)} / ${fmt(total)}` },
    { label: "Pages read", value: fmt(frame.pages) },
    { label: "Links found", value: fmt(frame.crossLinks), hint: "between different sites" },
    { label: "Site pairs", value: fmt(frame.edges), hint: "per file, before merging" },
    { label: "Failed attempts", value: fmt(frame.failedAttempts), hint: frame.failed ? `${frame.failed} gave up` : "each one retried" },
  ];
  return (
    <div className={`mt-6 grid grid-cols-2 gap-3 sm:grid-cols-3 ${focus ? "lg:grid-cols-6" : "lg:grid-cols-5"}`}>
      {tiles.map((x) => (
        <div key={x.label} className="rounded-xl border border-zinc-800 bg-zinc-900/50 px-4 py-3">
          <div className="truncate text-xs text-zinc-500">{x.label}</div>
          <div className="mt-1 font-mono text-xl text-zinc-100 tabular-nums">{x.value}</div>
          {x.hint && <div className="mt-0.5 text-[11px] text-zinc-600">{x.hint}</div>}
        </div>
      ))}
    </div>
  );
}

function NothingYet() {
  return (
    <div className="rounded-xl border border-zinc-800 bg-zinc-900/50 px-6 py-8">
      <h2 className="text-lg font-medium text-zinc-100">No run to show yet</h2>
      <p className="mt-2 max-w-2xl text-sm text-zinc-400">
        Nothing is running, and the API wasn&apos;t started with a saved run. Do a small demo run, save it, and restart the site:
      </p>
      <pre className="mt-5 overflow-x-auto rounded-lg bg-black/40 px-4 py-3 font-mono text-xs leading-6 text-zinc-300">
        <span className="text-zinc-500"># a small, light run on slices of the sample file; this page shows it live</span>
        {"\n"}make demo{"\n\n"}
        <span className="text-zinc-500"># then Ctrl+C, save it, and restart the site to replay it any time</span>
        {"\n"}make demo-save{"\n"}make web
      </pre>
    </div>
  );
}
