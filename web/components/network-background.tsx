"use client";

import { useEffect, useRef } from "react";
import type { Network } from "@/lib/types";
import { badgeColor, faviconUrl } from "@/lib/badge";

const NEAR = 0.35;
const FAR = 5;
const SPEED = 0.12;
const FRAME_MS = 1000 / 30;
const MAX_DPR = 1.5;
const DUST = 140;

type Point = { x: number; y: number; z: number };
type Site = Point & { domain: string; size: number; sx: number; sy: number; r: number; alpha: number; seen: boolean };

const clamp = (v: number) => Math.max(0, Math.min(1, v));
const smooth = (a: number, b: number, v: number) => {
  const t = clamp((v - a) / (b - a));
  return t * t * (3 - 2 * t);
};

// place puts a point on a ring around the view direction, so it flies past the search box instead of into it.
function place(p: Point, z: number, aspect: number, inner: number, outer: number) {
  const a = Math.random() * Math.PI * 2;
  const r = inner + Math.random() * (outer - inner);
  p.x = Math.cos(a) * r * aspect;
  p.y = Math.sin(a) * r;
  p.z = z;
}

// NetworkBackground flies slowly through the top sites in the index, drawn with their real icons and links.
export function NetworkBackground() {
  const ref = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = ref.current!;
    const ctx = canvas.getContext("2d")!;
    const still = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const sans = getComputedStyle(document.documentElement).getPropertyValue("--font-geist-sans") || "system-ui, sans-serif";

    let w = 0, h = 0, dpr = 1;
    const resize = () => {
      dpr = Math.min(window.devicePixelRatio || 1, MAX_DPR);
      w = window.innerWidth;
      h = window.innerHeight;
      canvas.width = Math.round(w * dpr);
      canvas.height = Math.round(h * dpr);
      if (still) draw();
    };

    const aspect = () => Math.max(1, w / Math.max(h, 1));
    const sites: Site[] = [];
    const byDomain = new Map<string, Site>();
    let links: Network["links"] = [];
    const icons = new Map<string, HTMLImageElement | null>();
    const dust: Point[] = [];

    const project = (p: Point, focal: number) => ({
      sx: w / 2 + (p.x / p.z) * focal,
      sy: h / 2 + (p.y / p.z) * focal,
    });
    // depthAlpha fades points in from the distance and out just before they reach the camera.
    const depthAlpha = (z: number) => clamp((FAR - z) / 1.2) * clamp((z - NEAR) / 0.5);
    // centerAlpha keeps the middle of the screen quiet so the title and search box stay readable.
    const centerAlpha = (sx: number, sy: number) =>
      0.15 + 0.85 * smooth(0.3, 0.95, Math.hypot(sx - w / 2, (sy - h / 2) * 1.4) / (Math.min(w, h) * 0.6));

    function draw() {
      const focal = h * 0.5;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, w, h);

      for (const s of sites) {
        const { sx, sy } = project(s, focal);
        s.sx = sx;
        s.sy = sy;
        s.r = (s.size * focal * 0.045) / s.z;
        s.seen = sx > -s.r && sx < w + s.r && sy > -s.r && sy < h + s.r;
        s.alpha = depthAlpha(s.z) * centerAlpha(sx, sy) * 0.85;
      }

      ctx.lineWidth = 1;
      for (const l of links) {
        const a = byDomain.get(l.source), b = byDomain.get(l.target);
        if (!a || !b || !(a.seen || b.seen)) continue;
        const alpha = Math.min(a.alpha, b.alpha) * 0.35;
        if (alpha < 0.01) continue;
        ctx.strokeStyle = `rgba(96,165,250,${alpha})`;
        ctx.beginPath();
        ctx.moveTo(a.sx, a.sy);
        ctx.lineTo(b.sx, b.sy);
        ctx.stroke();
      }

      ctx.fillStyle = "#a1a1aa";
      for (const d of dust) {
        const { sx, sy } = project(d, focal);
        const alpha = depthAlpha(d.z) * centerAlpha(sx, sy) * 0.5;
        if (alpha < 0.01) continue;
        ctx.globalAlpha = alpha;
        ctx.beginPath();
        ctx.arc(sx, sy, Math.min(2, 0.9 / d.z + 0.3), 0, Math.PI * 2);
        ctx.fill();
      }

      const order = sites.filter((s) => s.seen && s.alpha > 0.01).sort((a, b) => b.z - a.z);
      for (const s of order) {
        ctx.globalAlpha = s.alpha;
        const icon = icons.get(s.domain);
        ctx.beginPath();
        ctx.arc(s.sx, s.sy, s.r, 0, Math.PI * 2);
        if (s.r < 6) {
          ctx.fillStyle = icon ? "#60a5fa" : badgeColor(s.domain);
          ctx.fill();
          continue;
        }
        ctx.fillStyle = icon ? "#18181b" : badgeColor(s.domain);
        ctx.fill();
        if (icon) {
          ctx.save();
          ctx.beginPath();
          ctx.arc(s.sx, s.sy, s.r * 0.72, 0, Math.PI * 2);
          ctx.clip();
          const size = s.r * 1.15;
          ctx.drawImage(icon, s.sx - size / 2, s.sy - size / 2, size, size);
          ctx.restore();
        } else {
          ctx.fillStyle = "rgba(255,255,255,0.92)";
          ctx.font = `600 ${s.r * 0.8}px ${sans}`;
          ctx.textAlign = "center";
          ctx.textBaseline = "middle";
          ctx.fillText(s.domain[0].toUpperCase(), s.sx, s.sy + 1);
        }
        ctx.beginPath();
        ctx.arc(s.sx, s.sy, s.r, 0, Math.PI * 2);
        ctx.strokeStyle = "#3f3f46";
        ctx.lineWidth = 1.5;
        ctx.stroke();
      }
      ctx.globalAlpha = 1;
    }

    function step(dt: number) {
      const dz = SPEED * dt;
      for (const s of sites) {
        s.z -= dz;
        if (s.z < NEAR || (!s.seen && s.z < FAR - 1.5)) place(s, FAR, aspect(), 0.45, 1.6);
      }
      for (const d of dust) {
        d.z -= dz;
        if (d.z < NEAR) place(d, FAR, aspect(), 0.1, 2.2);
      }
    }

    let raf = 0, last = 0;
    const tick = (now: number) => {
      raf = requestAnimationFrame(tick);
      if (now - last < FRAME_MS - 1) return;
      step(Math.min(now - last, 100) / 1000);
      last = now;
      draw();
    };
    const play = () => {
      if (still || raf || document.hidden) return;
      last = performance.now();
      raf = requestAnimationFrame(tick);
    };
    const pause = () => {
      cancelAnimationFrame(raf);
      raf = 0;
    };
    const onVisibility = () => (document.hidden ? pause() : play());

    resize();
    for (let i = 0; i < DUST; i++) {
      const p = { x: 0, y: 0, z: 0 };
      place(p, NEAR + Math.random() * (FAR - NEAR), aspect(), 0.1, 2.2);
      dust.push(p);
    }

    const ctrl = new AbortController();
    fetch("/api/network", { signal: ctrl.signal })
      .then((res) => (res.ok ? (res.json() as Promise<Network>) : null))
      .then((nw) => {
        if (!nw) return;
        const top = nw.nodes[0]?.score || 1;
        for (const n of nw.nodes) {
          const s: Site = { domain: n.domain, size: 0.7 + 0.6 * (n.score / top), x: 0, y: 0, z: 0, sx: 0, sy: 0, r: 0, alpha: 0, seen: true };
          place(s, NEAR + 0.4 + Math.random() * (FAR - NEAR - 0.4), aspect(), 0.45, 1.6);
          sites.push(s);
          byDomain.set(s.domain, s);
          icons.set(n.domain, null);
          const img = new Image();
          img.onload = () => {
            icons.set(n.domain, img);
            if (still) draw();
          };
          img.src = faviconUrl(n.domain);
        }
        links = nw.links;
        if (still) draw();
      })
      .catch(() => {});

    window.addEventListener("resize", resize);
    document.addEventListener("visibilitychange", onVisibility);
    if (still) draw();
    play();

    return () => {
      ctrl.abort();
      pause();
      window.removeEventListener("resize", resize);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, []);

  return <canvas ref={ref} aria-hidden="true" className="pointer-events-none fixed inset-0 h-full w-full" />;
}
