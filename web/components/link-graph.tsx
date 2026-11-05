"use client";

import { useEffect, useRef, useState } from "react";
import {
  forceCollide,
  forceLink,
  forceManyBody,
  forceSimulation,
  type SimulationLinkDatum,
  type SimulationNodeDatum,
} from "d3-force";
import { zoom, zoomIdentity, type ZoomTransform } from "d3-zoom";
import { drag } from "d3-drag";
import { pointer, select } from "d3-selection";
import { RotateCcw } from "lucide-react";
import type { Backlink } from "@/lib/types";
import { badgeColor, faviconUrl } from "@/lib/badge";

type Node = SimulationNodeDatum & {
  id: string;
  r: number;
  pages: number;
  nofollow: boolean;
  center: boolean;
  pinned: boolean;
  born: number;
};
type Link = SimulationLinkDatum<Node> & { nofollow: boolean; born: number };

const DEFAULT_NODES = 60;
const MIN_NODES = 0;
const ADD_EVERY_MS = 60;
const GROW_MS = 450;

type Hover = { x: number; y: number; node: Node } | null;

export function LinkGraph({ domain, backlinks, total }: { domain: string; backlinks: Backlink[]; total: number }) {
  const wrapRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [hover, setHover] = useState<Hover>(null);
  const [added, setAdded] = useState(0);
  const maxShown = backlinks.length;
  const [limit, setLimit] = useState(Math.min(DEFAULT_NODES, maxShown));
  const limitRef = useRef(limit);
  const resetRef = useRef<() => void>(() => {});
  useEffect(() => {
    limitRef.current = limit;
  }, [limit]);

  useEffect(() => {
    const wrap = wrapRef.current!;
    const canvas = canvasRef.current!;
    const ctx = canvas.getContext("2d")!;
    const dpr = window.devicePixelRatio || 1;
    let width = 0;
    let height = 0;
    let transform: ZoomTransform = zoomIdentity;
    let hovered: Node | null = null;
    const icons = new Map<string, HTMLImageElement | null>();
    const css = getComputedStyle(document.documentElement);
    const sans = css.getPropertyValue("--font-geist-sans") || "system-ui, sans-serif";
    const mono = css.getPropertyValue("--font-geist-mono") || "ui-monospace, monospace";

    const loadIcon = (d: string) => {
      if (icons.has(d)) return;
      icons.set(d, null);
      const img = new Image();
      img.onload = () => icons.set(d, img);
      img.src = faviconUrl(d);
    };

    const start = performance.now();
    const center: Node = { id: domain, r: 30, pages: 0, nofollow: false, center: true, pinned: false, born: start, x: 0, y: 0, fx: 0, fy: 0 };
    const nodes: Node[] = [center];
    const links: Link[] = [];
    loadIcon(domain);

    const linkForce = forceLink<Node, Link>(links)
      .id((d) => d.id)
      .distance((l) => 120 + 90 / Math.sqrt((l.target as Node).pages));
    const sim = forceSimulation<Node>(nodes)
      .force("charge", forceManyBody<Node>().strength(-160))
      .force("link", linkForce)
      .force("collide", forceCollide<Node>((d) => d.r + 8))
      .alphaDecay(0.02);

    // Grows or shrinks the graph toward the slider's value, strongest sites first, so it builds in front of the user.
    const list = backlinks;
    const step = () => {
      const target = Math.min(limitRef.current, list.length);
      const have = nodes.length - 1;
      if (have === target) return;
      if (have > target) {
        nodes.splice(target + 1);
        links.splice(target);
      } else {
        const batch = Math.max(1, Math.ceil((target - have) / 20));
        const now = performance.now();
        for (let i = have; i < Math.min(target, have + batch); i++) {
          const b = list[i];
          const angle = Math.random() * Math.PI * 2;
          const node: Node = {
            id: b.domain,
            r: 10 + 4 * Math.log2(b.pages + 1),
            pages: b.pages,
            nofollow: b.nofollowPages === b.pages,
            center: false,
            pinned: false,
            born: now,
            x: Math.cos(angle) * 40,
            y: Math.sin(angle) * 40,
          };
          nodes.push(node);
          links.push({ source: center, target: node, nofollow: node.nofollow, born: now });
          loadIcon(b.domain);
        }
      }
      sim.nodes(nodes);
      linkForce.links(links);
      sim.alpha(0.6).restart();
      setAdded(nodes.length - 1);
    };
    const adder = window.setInterval(step, ADD_EVERY_MS);

    resetRef.current = () => {
      for (const n of nodes) {
        n.pinned = false;
        n.fx = n.center ? 0 : null;
        n.fy = n.center ? 0 : null;
      }
      sim.alpha(0.8).restart();
    };

    const nodeAt = (sx: number, sy: number) => {
      const [x, y] = transform.invert([sx, sy]);
      for (let i = nodes.length - 1; i >= 0; i--) {
        const n = nodes[i];
        if ((n.x! - x) ** 2 + (n.y! - y) ** 2 <= n.r ** 2) return n;
      }
      return null;
    };

    const zoomer = zoom<HTMLCanvasElement, unknown>()
      .scaleExtent([0.3, 4])
      .on("zoom", (e) => {
        transform = e.transform;
        setHover(null);
      });
    const dragger = drag<HTMLCanvasElement, unknown, Node | null>()
      .subject((e) => nodeAt(e.x, e.y))
      .on("start", (e) => {
        if (!e.active) sim.alphaTarget(0.3).restart();
      })
      .on("drag", (e) => {
        const [x, y] = transform.invert(pointer(e.sourceEvent, canvas));
        // A dragged node stays where it's dropped until "Reset layout".
        e.subject!.fx = x;
        e.subject!.fy = y;
        e.subject!.pinned = true;
      })
      .on("end", (e) => {
        if (!e.active) sim.alphaTarget(0);
      });
    const sel = select(canvas);
    sel.call(dragger).call(zoomer);

    const resize = () => {
      const first = width === 0;
      width = wrap.clientWidth;
      height = wrap.clientHeight;
      canvas.width = width * dpr;
      canvas.height = height * dpr;
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      if (first) sel.call(zoomer.transform, zoomIdentity.translate(width / 2, height / 2));
    };
    const observer = new ResizeObserver(resize);
    observer.observe(wrap);
    resize();

    const onMove = (e: PointerEvent) => {
      const [sx, sy] = pointer(e, canvas);
      hovered = nodeAt(sx, sy);
      canvas.style.cursor = hovered ? "grab" : "default";
      setHover(hovered ? { x: sx, y: sy, node: hovered } : null);
    };
    const onLeave = () => {
      hovered = null;
      setHover(null);
    };
    canvas.addEventListener("pointermove", onMove);
    canvas.addEventListener("pointerleave", onLeave);

    let frame = 0;
    const draw = () => {
      const now = performance.now();
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, width, height);
      ctx.translate(transform.x, transform.y);
      ctx.scale(transform.k, transform.k);

      for (const l of links) {
        const s = l.source as Node;
        const t = l.target as Node;
        const p = easeOut(Math.min(1, (now - l.born) / GROW_MS));
        const active = hovered === t;
        ctx.beginPath();
        ctx.moveTo(s.x!, s.y!);
        ctx.lineTo(s.x! + (t.x! - s.x!) * p, s.y! + (t.y! - s.y!) * p);
        ctx.setLineDash(l.nofollow ? [5, 4] : []);
        ctx.strokeStyle = l.nofollow
          ? `rgba(251,146,60,${active ? 0.9 : 0.45})`
          : `rgba(96,165,250,${active ? 0.9 : 0.3})`;
        ctx.lineWidth = active ? 2 : 1.2;
        ctx.stroke();
      }
      ctx.setLineDash([]);

      // Pulse around the center while the graph is still building.
      if (nodes.length - 1 < Math.min(limitRef.current, list.length)) {
        const phase = ((now - start) % 1200) / 1200;
        ctx.beginPath();
        ctx.arc(0, 0, center.r + phase * 40, 0, Math.PI * 2);
        ctx.strokeStyle = `rgba(96,165,250,${0.5 * (1 - phase)})`;
        ctx.lineWidth = 2;
        ctx.stroke();
      }

      for (let i = 0; i < nodes.length; i++) {
        const n = nodes[i];
        const a = easeOutBack(Math.min(1, (now - n.born) / GROW_MS));
        const r = n.r * a;
        if (r <= 0) continue;
        ctx.save();
        if (n.center) {
          ctx.shadowColor = "rgba(59,130,246,0.6)";
          ctx.shadowBlur = 30;
        }
        ctx.beginPath();
        ctx.arc(n.x!, n.y!, r, 0, Math.PI * 2);
        const icon = icons.get(n.id);
        ctx.fillStyle = icon ? "#18181b" : badgeColor(n.id);
        ctx.fill();
        ctx.restore();

        if (icon) {
          ctx.save();
          ctx.beginPath();
          ctx.arc(n.x!, n.y!, r * 0.72, 0, Math.PI * 2);
          ctx.clip();
          const s = r * 1.15;
          ctx.drawImage(icon, n.x! - s / 2, n.y! - s / 2, s, s);
          ctx.restore();
        } else {
          ctx.fillStyle = "rgba(255,255,255,0.92)";
          ctx.font = `600 ${Math.max(9, r * 0.8)}px ${sans}`;
          ctx.textAlign = "center";
          ctx.textBaseline = "middle";
          ctx.fillText(n.id[0].toUpperCase(), n.x!, n.y! + 1);
        }

        ctx.beginPath();
        ctx.arc(n.x!, n.y!, r, 0, Math.PI * 2);
        ctx.strokeStyle = n.center ? "#93c5fd" : n.nofollow ? "#fb923c" : hovered === n ? "#93c5fd" : "#3f3f46";
        ctx.lineWidth = n.center ? 2.5 : 1.5;
        ctx.stroke();

        if (n.pinned) {
          ctx.beginPath();
          ctx.arc(n.x! + r * 0.75, n.y! - r * 0.75, 3, 0, Math.PI * 2);
          ctx.fillStyle = "#e4e4e7";
          ctx.fill();
        }

        if (n.center || i <= 12 || hovered === n) {
          ctx.globalAlpha = Math.min(1, a);
          ctx.fillStyle = n.center ? "#f4f4f5" : "#a1a1aa";
          ctx.font = `${n.center ? 600 : 400} ${n.center ? 13 : 11}px ${mono}`;
          ctx.textAlign = "center";
          ctx.textBaseline = "top";
          ctx.fillText(n.id, n.x!, n.y! + r + 6);
          ctx.globalAlpha = 1;
        }
      }
      frame = requestAnimationFrame(draw);
    };
    frame = requestAnimationFrame(draw);

    return () => {
      cancelAnimationFrame(frame);
      window.clearInterval(adder);
      sim.stop();
      observer.disconnect();
      canvas.removeEventListener("pointermove", onMove);
      canvas.removeEventListener("pointerleave", onLeave);
      sel.on(".zoom", null).on(".drag", null);
    };
  }, [domain, backlinks]);

  const shown = Math.min(limit, maxShown);
  const building = added < shown;

  return (
    <section aria-label={`Graph of sites linking to ${domain}`} className="relative">
      <div
        ref={wrapRef}
        className="relative h-[68vh] min-h-[460px] w-full overflow-hidden rounded-3xl border border-zinc-800/80 bg-[radial-gradient(circle_at_center,rgba(39,39,42,0.55),rgba(9,9,11,0.9)_70%)]"
      >
        <canvas ref={canvasRef} className="block touch-none" />

        <div className="pointer-events-none absolute left-5 top-4 flex items-center gap-2 text-xs text-zinc-400">
          <span className={`size-2 rounded-full ${building ? "animate-pulse bg-blue-400" : "bg-emerald-400"}`} />
          {building
            ? `Mapping backlinks… ${added} / ${shown}`
            : shown === total
              ? `All ${shown.toLocaleString()} linking sites`
              : `Top ${shown.toLocaleString()} of ${total.toLocaleString()} linking sites`}
        </div>

        <div className="absolute right-4 top-3 flex flex-wrap items-center justify-end gap-3 rounded-xl border border-zinc-800 bg-zinc-950/80 px-3 py-2 text-xs text-zinc-400 backdrop-blur">
          {maxShown > MIN_NODES && (
            <label className="flex items-center gap-3">
              Sites shown
              <input
                type="range"
                min={MIN_NODES}
                max={maxShown}
                step={1}
                value={limit}
                onChange={(e) => setLimit(Number(e.target.value))}
                className="w-32 accent-blue-500"
              />
              <NodeCount value={limit} min={MIN_NODES} max={maxShown} onChange={setLimit} />
            </label>
          )}
          <button
            onClick={() => resetRef.current()}
            className="flex items-center gap-1.5 rounded-md px-2 py-1 text-zinc-300 transition hover:bg-zinc-800 hover:text-zinc-50"
          >
            <RotateCcw className="size-3.5" aria-hidden="true" />
            Reset layout
          </button>
        </div>

        <div className="pointer-events-none absolute bottom-4 left-5 flex flex-wrap gap-4 text-xs text-zinc-500">
          <span className="flex items-center gap-2">
            <span className="h-px w-6 bg-blue-400" /> followed
          </span>
          <span className="flex items-center gap-2">
            <span className="h-px w-6 border-t border-dashed border-orange-400" /> nofollow
          </span>
          <span>bigger = more linking pages · drag a site to move it, scroll to zoom</span>
        </div>

        {hover && !hover.node.center && (
          <div
            className="pointer-events-none absolute z-10 rounded-lg border border-zinc-700 bg-zinc-900/95 px-3 py-2 text-xs shadow-xl"
            style={{ left: hover.x + 14, top: hover.y + 14 }}
          >
            <div className="font-mono text-zinc-100">{hover.node.id}</div>
            <div className="mt-1 text-zinc-400">
              {hover.node.pages} linking {hover.node.pages === 1 ? "page" : "pages"}
              {hover.node.nofollow && <span className="text-orange-400"> · nofollow</span>}
            </div>
          </div>
        )}
      </div>

      <div className="mt-6 flex justify-center">
        <a href="#stats" className="text-xs text-zinc-500 transition hover:text-zinc-300">
          Scroll for stats ↓
        </a>
      </div>
    </section>
  );
}

// NodeCount shows the slider's value as text that turns into an editable box on hover and focus.
function NodeCount({
  value,
  min,
  max,
  onChange,
}: {
  value: number;
  min: number;
  max: number;
  onChange: (n: number) => void;
}) {
  const [draft, setDraft] = useState<string | null>(null);

  const commit = () => {
    if (draft === null) return;
    const n = Math.round(Number(draft));
    if (draft.trim() !== "" && Number.isFinite(n)) onChange(Math.min(max, Math.max(min, n)));
    setDraft(null);
  };

  return (
    <input
      type="text"
      inputMode="numeric"
      aria-label={`Sites shown, ${min} to ${max}`}
      value={draft ?? String(value)}
      onFocus={(e) => {
        setDraft(String(value));
        e.target.select();
      }}
      onChange={(e) => setDraft(e.target.value.replace(/[^0-9]/g, ""))}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === "Enter") e.currentTarget.blur();
        if (e.key === "Escape") {
          setDraft(null);
          e.currentTarget.blur();
        }
      }}
      className="w-11 rounded-md border border-transparent bg-transparent px-1 py-0.5 text-right font-mono text-zinc-200 tabular-nums outline-none transition hover:border-zinc-600 hover:bg-zinc-900 focus:border-blue-500 focus:bg-zinc-900"
    />
  );
}

function easeOut(t: number) {
  return 1 - (1 - t) ** 3;
}

function easeOutBack(t: number) {
  const c = 1.7;
  return 1 + (c + 1) * (t - 1) ** 3 + c * (t - 1) ** 2;
}
