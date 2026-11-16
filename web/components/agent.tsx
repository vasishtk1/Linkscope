"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { motion } from "motion/react";
import { Check, ChevronDown, FastForward, Pause, Play, RotateCcw, X } from "lucide-react";
import {
  DEFAULT_SITE,
  RUNS,
  STAGES,
  TYPE_RATE,
  diffLines,
  pathOf,
  realTime,
  schedule,
  stageOf,
  type Problem,
  type Run,
  type Step,
} from "@/lib/agent";

type Status = "idle" | "running" | "paused" | "done";

export function Agent() {
  const [input, setInput] = useState(DEFAULT_SITE);
  const [run, setRun] = useState<Run>(RUNS[DEFAULT_SITE]);
  const [unknown, setUnknown] = useState("");
  const [status, setStatus] = useState<Status>("idle");
  const [speed, setSpeed] = useState(1);
  const [now, setNow] = useState(0);
  const times = useMemo(() => schedule(run.steps), [run]);
  const end = useMemo(() => {
    const last = run.steps.length - 1;
    return times[last] + (run.steps[last].thinking?.length ?? 0) / TYPE_RATE + 0.2;
  }, [run, times]);

  const start = useCallback((site: string) => {
    const key = site.trim().toLowerCase().replace(/^https?:\/\//, "").replace(/^www\./, "").replace(/\/.*$/, "");
    const next = RUNS[key];
    if (!next) {
      setUnknown(key);
      return;
    }
    setUnknown("");
    setInput(key);
    setRun(next);
    setNow(0);
    setStatus("running");
  }, []);

  useEffect(() => {
    const t = setTimeout(() => start(DEFAULT_SITE), 700);
    return () => clearTimeout(t);
  }, [start]);

  useEffect(() => {
    if (status !== "running") return;
    let frame = 0;
    let last = performance.now();
    const tick = (t: number) => {
      const dt = ((t - last) / 1000) * speed;
      last = t;
      setNow((n) => {
        const v = n + dt;
        if (v >= end) {
          setStatus("done");
          return end;
        }
        return v;
      });
      frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [status, speed, end]);

  const shown = run.steps.filter((_, i) => status !== "idle" && times[i] <= now);
  const clock = status === "done" ? run.tookMs / 1000 : realTime(run.steps, times, now);
  const current = shown.length ? stageOf(shown[shown.length - 1].step) : -1;
  const found = run.problems.filter((p) => shown.some((s) => s.step === "check" && s.page === p.page));
  const states = found.map((p) => ({ p, state: problemState(p, shown) }));
  const redirects = states.filter((s) => s.state === "redirect").length;
  const asked = shown.filter((s) => s.step === "ask").length;
  const checked = shown.filter((s) => s.step === "check").length;

  return (
    <main className="relative mx-auto max-w-6xl px-4 pb-20 sm:px-6">
      <section className="pt-8 sm:pt-12">
        <p className="font-mono text-xs uppercase tracking-[0.2em] text-blue-400">Pull request agent</p>
        <h1 className="mt-3 max-w-3xl text-3xl font-semibold tracking-tight text-zinc-50 sm:text-4xl">
          Finds pages other sites still link to that are now broken, and proposes the fix
        </h1>
        <p className="mt-3 max-w-2xl text-zinc-400">
          It checks a site&apos;s most linked pages from the backlink index, matches each dead page to a live one, asks Gemini
          only when the match is unclear, and writes the redirect into the site&apos;s config.
        </p>
      </section>

      <form
        className="mt-8 flex flex-col gap-3 sm:flex-row sm:items-center"
        onSubmit={(e) => {
          e.preventDefault();
          start(input);
        }}
      >
        <div className="flex flex-1 items-center gap-3 rounded-xl border border-zinc-800 bg-zinc-900/60 px-4 py-3 focus-within:border-blue-500">
          <span className="font-mono text-sm text-zinc-500">site</span>
          <input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            aria-label="Website to check"
            spellCheck={false}
            className="min-w-0 flex-1 bg-transparent font-mono text-zinc-100 outline-none"
          />
          <StatusPill status={status} clock={clock} />
        </div>
        <button
          type="submit"
          className="rounded-xl bg-blue-500 px-5 py-3 text-sm font-medium text-white transition hover:bg-blue-400"
        >
          {status === "done" ? "Run again" : "Run agent"}
        </button>
      </form>
      {unknown && (
        <p className="mt-2 text-sm text-zinc-400">
          This demo has a run for {Object.keys(RUNS).join(", ")}. {unknown} isn&apos;t one of them yet.
        </p>
      )}

      <div className="mt-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat label="Pages checked" value={`${checked}`} total={`/ ${run.checked}`} />
        <Stat label="Problems found" value={`${found.length}`} />
        <Stat label="Redirects proposed" value={`${redirects}`} accent />
        <Stat label="Gemini asked" value={`${asked}`} />
      </div>

      <Stages current={status === "idle" ? -1 : current} done={status === "done"} />

      <div className="mt-6 grid gap-6 lg:grid-cols-[1.3fr_1fr]">
        <Timeline run={run} shown={shown} times={times} now={now} status={status} />
        <div className="flex flex-col gap-6">
          <Findings run={run} states={states} />
          <Change run={run} ready={states.some((s) => s.state === "redirect")} />
        </div>
      </div>

      <div className="mt-6 flex flex-wrap items-center justify-end gap-3 text-xs text-zinc-500">
        <div className="flex items-center gap-2">
          {status === "running" || status === "paused" ? (
            <Control onClick={() => setStatus(status === "running" ? "paused" : "running")} label={status === "running" ? "Pause" : "Resume"}>
              {status === "running" ? <Pause className="size-3.5" /> : <Play className="size-3.5" />}
            </Control>
          ) : (
            <Control onClick={() => start(run.site)} label="Replay">
              <RotateCcw className="size-3.5" />
            </Control>
          )}
          <Control onClick={() => setSpeed(speed === 1 ? 2 : 1)} label={`${speed}x`} />
          <Control
            onClick={() => {
              setNow(end);
              setStatus("done");
            }}
            label="Skip to end"
          >
            <FastForward className="size-3.5" />
          </Control>
        </div>
      </div>
    </main>
  );
}

type State = "checking" | "matching" | "asking" | "redirect" | "none";

// problemState says where the agent is with one broken page, from the steps shown so far.
function problemState(p: Problem, shown: Step[]): State {
  const mine = shown.filter((s) => s.page === p.page);
  if (mine.some((s) => s.step === "decide")) return p.suggestion?.target ? "redirect" : "none";
  const match = mine.find((s) => s.step === "match");
  if (match) {
    if (match.text.includes("clear winner")) return "redirect";
    return p.suggestion?.ai ? "asking" : "none";
  }
  return shown.some((s) => s.step === "sitemap") ? "matching" : "checking";
}

function StatusPill({ status, clock }: { status: Status; clock: number }) {
  const label = { idle: "ready", running: "running", paused: "paused", done: "done" }[status];
  const dot = { idle: "bg-zinc-600", running: "bg-blue-400 animate-pulse", paused: "bg-amber-400", done: "bg-emerald-400" }[status];
  return (
    <span className="flex shrink-0 items-center gap-2 font-mono text-xs text-zinc-400">
      <span className={`size-2 rounded-full ${dot}`} />
      {label}
      <span className="tabular-nums text-zinc-500">{formatClock(clock)}</span>
    </span>
  );
}

function Stat({ label, value, total, accent }: { label: string; value: string; total?: string; accent?: boolean }) {
  return (
    <div className="rounded-2xl border border-zinc-800 bg-zinc-900/40 px-4 py-3">
      <div className="text-xs text-zinc-500">{label}</div>
      <div className={`mt-1 text-2xl font-semibold tabular-nums ${accent ? "text-emerald-400" : "text-zinc-50"}`}>
        {value}
        {total && <span className="ml-1 text-sm font-normal text-zinc-500">{total}</span>}
      </div>
    </div>
  );
}

function Stages({ current, done }: { current: number; done: boolean }) {
  return (
    <ol className="mt-6 grid grid-cols-2 gap-2 sm:grid-cols-5">
      {STAGES.map((s, i) => {
        const soon = "soon" in s && s.soon;
        const active = !done && i === current;
        const passed = (done && !soon) || (current > i && !soon);
        return (
          <li
            key={s.id}
            className={`flex items-center gap-2 rounded-xl border px-3 py-2 text-xs transition ${
              active
                ? "border-blue-500/60 bg-blue-500/10 text-blue-200"
                : passed
                  ? "border-zinc-800 bg-zinc-900/40 text-zinc-300"
                  : "border-dashed border-zinc-800 text-zinc-500"
            }`}
          >
            <span className="font-mono text-zinc-500">{i + 1}</span>
            <span className="flex-1">{s.label}</span>
            {passed && <Check className="size-3.5 text-emerald-400" aria-hidden="true" />}
            {active && <span className="size-1.5 animate-pulse rounded-full bg-blue-400" />}
            {soon && <span className="text-[10px] uppercase tracking-wide text-zinc-600">next</span>}
          </li>
        );
      })}
    </ol>
  );
}

function Timeline({ run, shown, times, now, status }: { run: Run; shown: Step[]; times: number[]; now: number; status: Status }) {
  const box = useRef<HTMLDivElement>(null);
  const last = shown[shown.length - 1];
  const waiting = status !== "done" && last && (last.step === "ask" || (last.step === "sitemap" && !last.text.includes("files read")));
  useEffect(() => {
    const el = box.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [shown.length, now]);

  return (
    <section className="flex min-h-0 flex-col rounded-2xl border border-zinc-800 bg-zinc-950/60">
      <header className="flex items-center justify-between border-b border-zinc-800 px-4 py-3">
        <h2 className="text-sm font-medium text-zinc-200">Agent</h2>
        <span className="font-mono text-xs text-zinc-500">{run.site}</span>
      </header>
      <div ref={box} className="h-[460px] overflow-y-auto px-4 py-3 font-mono text-[13px] leading-relaxed sm:h-[540px]">
        {status === "idle" && <p className="text-zinc-600">starting…</p>}
        {shown.map((s) => {
          const i = run.steps.indexOf(s);
          return <StepRow key={i} s={s} site={run.site} typed={s.thinking ? Math.floor((now - times[i]) * TYPE_RATE) : 0} />;
        })}
        {waiting && (
          <div className="mt-1 flex items-center gap-2 pl-[4.5rem] text-zinc-500">
            <span className="size-1.5 animate-pulse rounded-full bg-blue-400" />
            {last.step === "ask" ? "waiting for Gemini" : "reading sitemap files"}
          </div>
        )}
      </div>
    </section>
  );
}

const TAG: Record<string, string> = {
  check: "text-zinc-400",
  sitemap: "text-sky-300",
  match: "text-sky-300",
  archive: "text-violet-300",
  read: "text-violet-300",
  ask: "text-violet-300",
  decide: "text-violet-300",
  done: "text-emerald-300",
  start: "text-zinc-400",
};

function StepRow({ s, site, typed }: { s: Step; site: string; typed: number }) {
  const [open, setOpen] = useState(true);
  const bad = s.step === "check" && !s.text.startsWith("ok");
  return (
    <motion.div initial={{ opacity: 0, y: 4 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.25 }} className="py-0.5">
      <div className="grid grid-cols-[3.25rem_4.25rem_1fr] gap-x-2">
        <span className="text-right tabular-nums text-zinc-600">{s.at.toFixed(1)}s</span>
        <span className={TAG[s.step] ?? "text-zinc-400"}>{s.step}</span>
        <span className={bad ? "text-amber-300" : s.step === "check" ? "text-zinc-400" : "text-zinc-200"}>
          {s.page && <span className="text-zinc-500">{pathOf(s.page, site)} </span>}
          {s.text}
        </span>
      </div>
      {s.thinking && (
        <div className="ml-[7.5rem] mt-2 mb-2">
          <button
            onClick={() => setOpen(!open)}
            className="flex items-center gap-1 text-xs text-zinc-500 transition hover:text-zinc-300"
            aria-expanded={open}
          >
            <ChevronDown className={`size-3.5 transition ${open ? "" : "-rotate-90"}`} aria-hidden="true" />
            thinking
          </button>
          {open && <Thinking text={s.thinking} typed={typed} />}
        </div>
      )}
    </motion.div>
  );
}

function Thinking({ text, typed }: { text: string; typed: number }) {
  const shownText = text.slice(0, Math.max(0, typed));
  const typing = typed < text.length;
  const paras = shownText.split(/\n\n+/);
  return (
    <div className="mt-1.5 border-l border-zinc-800 pl-3 font-sans text-[13px] leading-relaxed text-zinc-500">
      {paras.map((p, i) => {
        const heading = /^\*\*.*\*\*$/.test(p.trim()) || (i === 0 && p.startsWith("**"));
        return (
          <p key={i} className={`${i ? "mt-2" : ""} ${heading ? "font-medium text-zinc-400" : ""}`}>
            {p.replace(/\*\*/g, "").replace(/\*/g, "").replace(/`/g, "")}
            {typing && i === paras.length - 1 && <span className="ml-0.5 inline-block h-3.5 w-1.5 translate-y-0.5 animate-pulse bg-zinc-500" />}
          </p>
        );
      })}
    </div>
  );
}

const STATE_LABEL: Record<State, string> = {
  checking: "found, waiting to match",
  matching: "comparing with live pages",
  asking: "unclear, asking Gemini",
  redirect: "redirect found",
  none: "no good replacement",
};

function Findings({ run, states }: { run: Run; states: { p: Problem; state: State }[] }) {
  return (
    <section className="rounded-2xl border border-zinc-800 bg-zinc-900/30">
      <header className="border-b border-zinc-800 px-4 py-3">
        <h2 className="text-sm font-medium text-zinc-200">Broken pages that still get links</h2>
      </header>
      {states.length === 0 ? (
        <p className="px-4 py-6 text-sm text-zinc-600">Nothing yet. Checking pages one per second.</p>
      ) : (
        <ul className="divide-y divide-zinc-800/70">
          {states.map(({ p, state }) => (
            <motion.li key={p.page} initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="px-4 py-3">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="truncate font-mono text-sm text-zinc-100">{pathOf(p.page, run.site)}</div>
                  <div className="mt-0.5 text-xs text-zinc-500">
                    {p.result === "homepage" ? "sends visitors to the homepage" : `broken (${p.status || p.error || "error"})`} ·{" "}
                    {p.sites} linking {p.sites === 1 ? "site" : "sites"}
                  </div>
                </div>
                <StateBadge state={state} />
              </div>
              {state === "redirect" && p.suggestion?.target && (
                <div className="mt-2 font-mono text-xs text-emerald-300/90">
                  → {pathOf(p.suggestion.target.replace(/^https?:\/\/(www\.)?/, ""), run.site)}
                  <span className="ml-2 font-sans text-zinc-500">picked by {p.suggestion.by === "gemini" ? "Gemini" : "address match"}</span>
                </div>
              )}
              {state === "none" && p.suggestion?.ai?.reason && (
                <p className="mt-2 text-xs leading-relaxed text-zinc-500">Gemini: {p.suggestion.ai.reason}</p>
              )}
            </motion.li>
          ))}
        </ul>
      )}
    </section>
  );
}

function StateBadge({ state }: { state: State }) {
  const style = {
    checking: "text-zinc-400 border-zinc-700",
    matching: "text-sky-300 border-sky-500/40",
    asking: "text-violet-300 border-violet-500/40",
    redirect: "text-emerald-300 border-emerald-500/40",
    none: "text-zinc-400 border-zinc-700",
  }[state];
  return (
    <span className={`flex shrink-0 items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] ${style}`}>
      {state === "redirect" && <Check className="size-3" aria-hidden="true" />}
      {state === "none" && <X className="size-3" aria-hidden="true" />}
      {(state === "matching" || state === "asking") && <span className="size-1.5 animate-pulse rounded-full bg-current" />}
      {STATE_LABEL[state]}
    </span>
  );
}

function Change({ run, ready }: { run: Run; ready: boolean }) {
  const lines = useMemo(() => diffLines(run.file.before, run.file.after), [run]);
  return (
    <section className="rounded-2xl border border-zinc-800 bg-zinc-900/30">
      <header className="flex items-center justify-between border-b border-zinc-800 px-4 py-3">
        <h2 className="text-sm font-medium text-zinc-200">Proposed change</h2>
        <span className="font-mono text-xs text-zinc-500">{run.file.path}</span>
      </header>
      <pre className="overflow-x-auto px-0 py-3 font-mono text-[12.5px] leading-relaxed">
        {(ready ? lines : lines.filter((l) => l.kind !== "add").map((l) => ({ ...l, kind: "same" as const }))).map((l, i) => (
          <motion.div
            key={`${ready}-${i}`}
            initial={l.kind === "add" ? { opacity: 0, x: -4 } : false}
            animate={{ opacity: 1, x: 0 }}
            transition={{ delay: l.kind === "add" ? i * 0.04 : 0 }}
            className={`px-4 ${l.kind === "add" ? "bg-emerald-500/10 text-emerald-300" : l.kind === "del" ? "bg-red-500/10 text-red-300/80" : "text-zinc-500"}`}
          >
            <span className="mr-3 select-none text-zinc-600">{l.kind === "add" ? "+" : l.kind === "del" ? "-" : " "}</span>
            {l.text}
          </motion.div>
        ))}
      </pre>
      <p className="border-t border-zinc-800 px-4 py-2.5 text-xs text-zinc-500">
        {ready
          ? "Permanent redirect, added above existing rules. Next: open this as a pull request for the site owner to review."
          : "Waiting for a redirect to propose."}
      </p>
    </section>
  );
}

function Control({ onClick, label, children }: { onClick: () => void; label: string; children?: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className="flex items-center gap-1.5 rounded-lg border border-zinc-800 px-2.5 py-1.5 text-zinc-400 transition hover:border-zinc-700 hover:text-zinc-100"
    >
      {children}
      {label}
    </button>
  );
}

function formatClock(s: number) {
  const m = Math.floor(s / 60);
  return `${m}:${String(Math.floor(s % 60)).padStart(2, "0")}`;
}
