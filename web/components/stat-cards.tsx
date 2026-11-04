"use client";

import { useEffect, useRef, useState } from "react";
import { animate, useInView } from "motion/react";
import type { Lookup } from "@/lib/types";

export function StatCards({ data }: { data: Lookup }) {
  const followed = data.linkingPages - data.nofollowPages;
  return (
    <div className="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <Card label="Authority score" note="Calculated with PageRank once the full link graph is built">
        <span className="text-zinc-500">{data.score ?? "—"}</span>
      </Card>
      <Card label="Linking sites" note="Different websites that link here">
        <CountUp value={data.linkingDomains} />
      </Card>
      <Card label="Linking pages" note={`${followed.toLocaleString()} followed, counted once per page`}>
        <CountUp value={data.linkingPages} />
      </Card>
      <Card label="Nofollow only" note="Pages search engines give less weight">
        <span className="text-orange-400">
          <CountUp value={data.nofollowPages} />
        </span>
      </Card>
    </div>
  );
}

function Card({ label, note, children }: { label: string; note: string; children: React.ReactNode }) {
  return (
    <div className="rounded-2xl border border-zinc-800 bg-zinc-900/40 p-5">
      <div className="text-sm text-zinc-400">{label}</div>
      <div className="mt-2 text-4xl font-semibold tracking-tight text-zinc-50 tabular-nums">{children}</div>
      <div className="mt-2 text-xs text-zinc-500">{note}</div>
    </div>
  );
}

// CountUp animates from 0 to value the first time it scrolls into view.
function CountUp({ value }: { value: number }) {
  const ref = useRef<HTMLSpanElement>(null);
  const inView = useInView(ref, { once: true });
  const [shown, setShown] = useState(0);
  useEffect(() => {
    if (!inView) return;
    const controls = animate(0, value, {
      duration: 1.1,
      ease: "easeOut",
      onUpdate: (v) => setShown(Math.round(v)),
    });
    return () => controls.stop();
  }, [inView, value]);
  return <span ref={ref}>{shown.toLocaleString()}</span>;
}
