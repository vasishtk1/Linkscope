import type { Flight, Focus, Frame } from "@/lib/replay";
import { fileLabel, fmt, focusProgress } from "@/lib/replay";

const W = 1000;
const ROW = 60;
const QX = 30;
const CX = 300;
const WX = 440;
const WW = 250;
const MX = 820;
const MW = 150;
const TILE = 12;
const PER_ROW = 9;
const MAX_TILES = 72;

const ease = (p: number) => (p < 0.5 ? 2 * p * p : 1 - (-2 * p + 2) ** 2 / 2);
const trunc = (s: string, n: number) => (s.length > n ? `…${s.slice(-(n - 1))}` : s);

// PipelineScene draws one frame; with a focus, the searched site's files stay blue and everything else fades.
export function PipelineScene({ frame, total, focus }: { frame: Frame; total: number; focus?: Focus }) {
  const rows = Math.max(frame.workers.length, 3);
  const H = Math.max(400, rows * ROW + 100);
  const cy = H / 2;
  const top = (H - frame.workers.length * ROW) / 2;
  const wy = (i: number) => top + i * ROW + ROW / 2;
  const row = new Map(frame.workers.map((w, i) => [w.name, i]));
  const merge = { x: MX, y: cy - 110 };
  const allIn = total > 0 && frame.done + frame.failed === total;
  const queueTop = cy - 60;

  const flightPos = (fl: Flight) => {
    const y = wy(row.get(fl.worker) ?? 0);
    const p = ease(Math.min(1, Math.max(0, fl.p)));
    const [a, b] =
      fl.to === "worker" ? [[CX + 40, cy], [WX, y]] :
      fl.to === "merge" ? [[WX + WW, y], [merge.x, merge.y + 26]] :
      [[WX, y], [QX + 70, queueTop + 30]];
    return { x: a[0] + (b[0] - a[0]) * p - TILE / 2, y: a[1] + (b[1] - a[1]) * p - TILE / 2 };
  };

  const tiles = Math.min(frame.queued, MAX_TILES);
  const inFocus = (path?: string) => !!focus && !!path && focus.pages.has(fileLabel(path));
  const dim = (path?: string) => !!focus && !inFocus(path);
  const progress = focus && focusProgress(focus, frame);
  const working = focus ? frame.workers.filter((w) => inFocus(w.job?.path)).length : 0;
  const queuedFocus = focus && progress ? Math.max(0, focus.matched - progress.files - working) : 0;
  const site = focus && trunc(focus.domain, 20);

  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="h-auto w-full" role="img" aria-label={`${frame.done} of ${total} files done, ${frame.working} being processed`}>
      <g stroke="#27272a" strokeWidth={1.5} fill="none">
        <path d={`M${QX + 150} ${cy} H${CX - 40}`} />
        {frame.workers.map((w, i) => (
          <g key={w.name}>
            <path d={`M${CX + 40} ${cy} C${CX + 110} ${cy} ${WX - 70} ${wy(i)} ${WX} ${wy(i)}`} stroke={w.job ? "#1e3a8a" : "#27272a"} />
            <path d={`M${WX + WW} ${wy(i)} C${WX + WW + 60} ${wy(i)} ${MX - 60} ${merge.y + 26} ${MX} ${merge.y + 26}`} />
          </g>
        ))}
        <path d={`M${MX + MW / 2} ${merge.y + 52} V${cy - 26}`} />
        <path d={`M${MX + MW / 2} ${cy + 26} V${cy + 58}`} />
      </g>

      <text x={QX} y={queueTop - 34} className="fill-zinc-300 text-[14px] font-medium">Common Crawl files</text>
      <text x={QX} y={queueTop - 14} className="fill-zinc-500 text-[12px]">{frame.queued} waiting</text>
      {Array.from({ length: tiles }, (_, i) => (
        <rect
          key={i}
          x={QX + (i % PER_ROW) * (TILE + 4)}
          y={queueTop + Math.floor(i / PER_ROW) * (TILE + 4)}
          width={TILE}
          height={TILE}
          rx={2}
          className={i < queuedFocus ? "fill-blue-400" : focus ? "fill-zinc-700" : "fill-zinc-600"}
        />
      ))}
      {frame.queued > MAX_TILES && (
        <text x={QX} y={queueTop + (MAX_TILES / PER_ROW) * (TILE + 4) + 14} className="fill-zinc-500 text-[11px]">
          +{frame.queued - MAX_TILES} more
        </text>
      )}

      <circle cx={CX} cy={cy} r={40} className="fill-zinc-900 stroke-blue-400" strokeWidth={2} />
      <text x={CX} y={cy + 4} textAnchor="middle" className="fill-zinc-100 text-[12px] font-medium">Coordinator</text>
      <text x={CX} y={cy + 62} textAnchor="middle" className="fill-zinc-500 text-[11px]">hands out files,</text>
      <text x={CX} y={cy + 76} textAnchor="middle" className="fill-zinc-500 text-[11px]">retries failures</text>

      {frame.workers.map((w, i) => {
        const y = wy(i) - 24;
        const border = w.lost ? "stroke-red-400" : inFocus(w.job?.path) ? "stroke-blue-300" : w.job ? "stroke-blue-500" : "stroke-zinc-700";
        const focusPages = inFocus(w.job?.path) ? focus!.pages.get(fileLabel(w.job!.path)) : undefined;
        const status = w.lost ? "no contact" : w.job ? (w.job.attempt > 1 ? `retry, attempt ${w.job.attempt}` : "working") : "idle";
        return (
          <g key={w.name} opacity={dim(w.job?.path) ? 0.4 : 1}>
            <rect x={WX} y={y} width={WW} height={48} rx={8} className={`fill-zinc-900 ${border}`} strokeWidth={1.5} />
            <text x={WX + 12} y={y + 19} className="fill-zinc-100 font-mono text-[12px]">{trunc(w.name, 18)}</text>
            <text x={WX + WW - 12} y={y + 19} textAnchor="end" className={`text-[11px] ${w.lost ? "fill-red-400" : w.job?.attempt && w.job.attempt > 1 ? "fill-amber-300" : "fill-zinc-500"}`}>
              {focusPages !== undefined ? `+${fmt(focusPages)} links` : status}
            </text>
            {w.job && (
              <>
                <text x={WX + 12} y={y + 35} className="fill-zinc-400 font-mono text-[10px]">{trunc(fileLabel(w.job.path), 34)}</text>
                <rect x={WX + 12} y={y + 40} width={WW - 24} height={3} rx={1.5} className="fill-zinc-800" />
                {w.job.progress === null ? (
                  <rect x={WX + 12} y={y + 40} width={WW - 24} height={3} rx={1.5} className="animate-pulse fill-blue-500/60" />
                ) : (
                  <rect x={WX + 12} y={y + 40} width={(WW - 24) * w.job.progress} height={3} rx={1.5} className="fill-blue-400" />
                )}
              </>
            )}
          </g>
        );
      })}

      {focus && progress ? (
        <>
          <Step x={MX} y={merge.y} title="Merge" sub={`${fmt(progress.pages)} links to ${site}`} lit={progress.files > 0} />
          <Step x={MX} y={cy - 26} title="PageRank" sub={allIn ? (focus.score === null ? "no score" : `${site}: ${focus.score} / 100`) : "after merge"} lit={allIn} />
          <Step x={MX} y={cy + 58} title="Index" sub={allIn ? `your search: ${focus.tookMs.toFixed(2)} ms` : "after PageRank"} lit={allIn} />
        </>
      ) : (
        <>
          <Step x={MX} y={merge.y} title="Merge" sub={`${frame.done} of ${total} files in`} lit={frame.done > 0} />
          <Step x={MX} y={cy - 26} title="PageRank" sub={allIn ? "runs next" : "after merge"} lit={allIn} />
          <Step x={MX} y={cy + 58} title="Index" sub={allIn ? "then this" : "after PageRank"} lit={allIn} />
        </>
      )}

      {frame.flights.map((fl) => {
        const { x, y } = flightPos(fl);
        const color = fl.failed ? "fill-red-400" : dim(fl.path) ? "fill-zinc-600" : "fill-blue-400";
        return <rect key={fl.key} x={x} y={y} width={TILE} height={TILE} rx={2} className={color} />;
      })}
      {focus &&
        frame.flights
          .filter((fl) => fl.to === "merge" && inFocus(fl.path))
          .map((fl) => (
            <text
              key={`pop-${fl.key}`}
              x={MX + MW / 2}
              y={merge.y - 8 - Math.min(1, fl.p) * 14}
              textAnchor="middle"
              opacity={1 - Math.min(1, fl.p)}
              className="fill-blue-300 font-mono text-[12px]"
            >
              +{fmt(focus.pages.get(fileLabel(fl.path)) ?? 0)}
            </text>
          ))}
    </svg>
  );
}

function Step({ x, y, title, sub, lit }: { x: number; y: number; title: string; sub: string; lit: boolean }) {
  return (
    <g>
      <rect x={x} y={y} width={MW} height={52} rx={8} className={`fill-zinc-900 ${lit ? "stroke-blue-500" : "stroke-zinc-800"}`} strokeWidth={1.5} />
      <text x={x + MW / 2} y={y + 22} textAnchor="middle" className={`text-[13px] font-medium ${lit ? "fill-zinc-100" : "fill-zinc-500"}`}>{title}</text>
      <text x={x + MW / 2} y={y + 39} textAnchor="middle" className="fill-zinc-500 text-[11px]">{sub}</text>
    </g>
  );
}
