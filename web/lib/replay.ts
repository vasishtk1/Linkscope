import type { RunEvent } from "./types";
import type { SavedSearch } from "./searches";

type Ev = RunEvent & { t: number };

export type Segment = { worker: string; path: string; from: number; to: number; outcome: "done" | "failed" | "open" };

// Run is a pipeline run prepared for playback: events on one clock, plus what each claim led to.
export type Run = {
  name: string;
  derived: boolean;
  total: number;
  workers: string[];
  events: Ev[];
  start: number;
  end: number;
  closeAt: Map<number, number>;
  segments: Segment[];
  typicalMs: number;
};

export function makeRun(opts: { name: string; derived: boolean; total: number; workers: string[]; events: RunEvent[]; now?: number }): Run {
  const events = opts.events.map((e) => ({ ...e, t: Date.parse(e.at) })).sort((a, b) => a.t - b.t || a.id - b.id);
  const closeAt = new Map<number, number>();
  const segments: Segment[] = [];
  const open = new Map<number, Ev>();
  const durations: number[] = [];
  for (const e of events) {
    if (e.kind === "claimed") {
      open.set(e.job, e);
      continue;
    }
    const claim = open.get(e.job);
    if (!claim) continue;
    open.delete(e.job);
    closeAt.set(claim.id, e.t);
    segments.push({ worker: claim.worker, path: claim.path, from: claim.t, to: e.t, outcome: e.kind === "completed" ? "done" : "failed" });
    if (e.kind === "completed") durations.push(e.t - claim.t);
  }
  const start = events.length ? events[0].t : (opts.now ?? 0);
  const end = Math.max(opts.now ?? 0, events.length ? events[events.length - 1].t : start);
  for (const claim of open.values()) segments.push({ worker: claim.worker, path: claim.path, from: claim.t, to: end, outcome: "open" });
  durations.sort((a, b) => a - b);
  const workers = [...new Set([...opts.workers, ...events.map((e) => e.worker)])].filter(Boolean).sort();
  return {
    name: opts.name,
    derived: opts.derived,
    total: Math.max(opts.total, new Set(events.map((e) => e.job)).size),
    workers,
    events,
    start,
    end,
    closeAt,
    segments,
    typicalMs: durations.length ? durations[Math.floor(durations.length / 2)] : 0,
  };
}

export type WorkerView = {
  name: string;
  job?: { id: number; path: string; attempt: number; progress: number | null };
  lost: boolean;
};

export type Flight = { key: string; to: "worker" | "merge" | "queue"; worker: string; path: string; p: number; failed: boolean };

export type Frame = {
  workers: WorkerView[];
  queued: number;
  working: number;
  done: number;
  failed: number;
  pages: number;
  crossLinks: number;
  edges: number;
  failedAttempts: number;
  flights: Flight[];
  completed: string[];
  lastError?: string;
};

// frameAt replays every event up to time t; flightMs is how long, in run time, a file takes to move between boxes.
export function frameAt(run: Run, t: number, flightMs: number): Frame {
  const jobs = new Map<string, { id: number; path: string; attempt: number; since: number; claimId: number }>();
  const lost = new Set<string>();
  const finished = new Set<number>();
  const gaveUp = new Set<number>();
  const f: Frame = { workers: [], queued: 0, working: 0, done: 0, failed: 0, pages: 0, crossLinks: 0, edges: 0, failedAttempts: 0, flights: [], completed: [] };

  for (const e of run.events) {
    if (e.t > t) break;
    lost.delete(e.worker);
    switch (e.kind) {
      case "claimed":
        jobs.set(e.worker, { id: e.job, path: e.path, attempt: e.attempt, since: e.t, claimId: e.id });
        break;
      case "completed":
        if (jobs.get(e.worker)?.id === e.job) jobs.delete(e.worker);
        if (!finished.has(e.job)) f.completed.push(e.path);
        finished.add(e.job);
        f.pages += e.pages ?? 0;
        f.crossLinks += e.crossLinks ?? 0;
        f.edges += e.edges ?? 0;
        break;
      case "failed":
      case "expired":
        if (jobs.get(e.worker)?.id === e.job) jobs.delete(e.worker);
        f.failedAttempts++;
        f.lastError = e.error;
        if (e.final) gaveUp.add(e.job);
        if (e.kind === "expired") lost.add(e.worker);
        break;
      case "released":
        if (jobs.get(e.worker)?.id === e.job) jobs.delete(e.worker);
        break;
    }
    if (t - e.t < flightMs) {
      const p = flightMs > 0 ? (t - e.t) / flightMs : 1;
      const to = e.kind === "claimed" ? "worker" : e.kind === "completed" ? "merge" : "queue";
      f.flights.push({ key: `${e.id}`, to, worker: e.worker, path: e.path, p, failed: e.kind === "failed" || e.kind === "expired" });
    }
  }

  f.done = finished.size;
  f.failed = gaveUp.size;
  for (const name of run.workers) {
    const j = jobs.get(name);
    let progress: number | null = null;
    if (j) {
      const until = run.closeAt.get(j.claimId);
      if (until !== undefined && until > j.since) progress = Math.min(1, (t - j.since) / (until - j.since));
      else if (run.typicalMs > 0) progress = Math.min(0.95, (t - j.since) / run.typicalMs);
    }
    f.workers.push({ name, lost: lost.has(name), job: j && { id: j.id, path: j.path, attempt: j.attempt, progress } });
  }
  f.working = jobs.size;
  f.queued = Math.max(0, run.total - f.done - f.failed - f.working);
  return f;
}

// Focus is one saved search laid over a run: which of the run's files had links to the site, and how many pages each.
export type Focus = {
  domain: string;
  pages: Map<string, number>;
  matched: number;
  totalPages: number;
  originFiles: number;
  stored: number;
  score: number | null;
  tookMs: number;
};

// makeFocus matches a search's origins to the run's files by name; matched is 0 when the search came from other data.
export function makeFocus(run: Run, s: SavedSearch): Focus {
  const want = new Map(s.origins.map((o) => [o.file, o.pages]));
  const pages = new Map<string, number>();
  for (const e of run.events) {
    const label = fileLabel(e.path);
    const n = want.get(label);
    if (n !== undefined) pages.set(label, n);
  }
  return {
    domain: s.domain,
    pages,
    matched: pages.size,
    totalPages: s.linkingPages,
    originFiles: s.originFiles,
    stored: s.origins.length,
    score: s.score,
    tookMs: s.tookMs,
  };
}

// focusProgress counts the focused files done and the linking pages they've added so far.
export function focusProgress(f: Focus, frame: Frame) {
  let files = 0;
  let pages = 0;
  for (const p of frame.completed) {
    const n = f.pages.get(fileLabel(p));
    if (n !== undefined) {
      files++;
      pages += n;
    }
  }
  return { files, pages };
}

export function fileLabel(path: string) {
  return (path.split("/").pop() ?? path).replace(/\.warc\.wat\.gz$/, "");
}

export function fmt(n: number) {
  return Math.round(n).toLocaleString("en-US");
}

export function clock(ms: number) {
  const s = Math.max(0, Math.round(ms / 1000));
  const m = Math.floor(s / 60);
  return `${m}:${String(s % 60).padStart(2, "0")}`;
}
