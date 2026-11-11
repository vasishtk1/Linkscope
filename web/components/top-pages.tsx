"use client";

import { useState } from "react";
import { ChevronDown, ExternalLink, Search } from "lucide-react";
import type { TopPage } from "@/lib/types";
import { SiteIcon } from "./backlinks-table";

const FIRST_ROWS = 10;

// pageLabel shortens a page for display: just the path on the site itself, host and path on a subdomain.
export function pageLabel(page: string, domain: string) {
  const slash = page.indexOf("/");
  const host = slash < 0 ? page : page.slice(0, slash);
  const path = slash < 0 ? "/" : page.slice(slash);
  let shown = path;
  try {
    shown = decodeURI(path);
  } catch {}
  return host === domain ? shown : host + (shown === "/" ? "" : shown);
}

export function TopPages({ domain, pages, total }: { domain: string; pages: TopPage[]; total: number }) {
  const [all, setAll] = useState(false);
  const [filter, setFilter] = useState("");
  const [open, setOpen] = useState<string | null>(null);
  const max = pages[0]?.sites || 1;
  const q = filter.trim().toLowerCase();
  const matches = q ? pages.filter((p) => pageLabel(p.page, domain).toLowerCase().includes(q)) : pages;
  const rows = all || q ? matches : matches.slice(0, FIRST_ROWS);

  return (
    <div className="mt-10">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h3 className="text-lg font-medium text-zinc-100">Most linked pages</h3>
          <p className="mt-1 text-sm text-zinc-500">
            {total.toLocaleString()} {total === 1 ? "page" : "pages"} on {domain} {total === 1 ? "gets" : "get"} links
            {pages.length < total && `; here are the top ${pages.length.toLocaleString()}`}
          </p>
        </div>
        {pages.length > FIRST_ROWS && (
          <label className="flex items-center gap-2 rounded-lg border border-zinc-800 bg-zinc-900/60 px-3 py-1.5 text-sm text-zinc-400 focus-within:border-blue-500">
            <Search className="size-4" aria-hidden="true" />
            <input
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Filter pages"
              aria-label="Filter pages"
              className="w-36 bg-transparent text-zinc-200 outline-none placeholder:text-zinc-600"
            />
          </label>
        )}
      </div>

      <ol className="mt-4 divide-y divide-zinc-800/70 rounded-2xl border border-zinc-800">
        {rows.map((p) => (
          <PageRow
            key={p.page}
            p={p}
            rank={pages.indexOf(p) + 1}
            label={pageLabel(p.page, domain)}
            width={(p.sites / max) * 100}
            open={open === p.page}
            onToggle={() => setOpen(open === p.page ? null : p.page)}
          />
        ))}
        {rows.length === 0 && <li className="px-4 py-6 text-center text-sm text-zinc-500">No pages match “{filter}”.</li>}
      </ol>

      {!q && pages.length > FIRST_ROWS && (
        <button
          onClick={() => setAll(!all)}
          className="mx-auto mt-4 flex items-center gap-1 text-sm text-zinc-400 transition hover:text-zinc-100"
        >
          {all ? "Show fewer" : `Show all ${pages.length.toLocaleString()}`}
          <ChevronDown className={`size-4 transition ${all ? "rotate-180" : ""}`} aria-hidden="true" />
        </button>
      )}
    </div>
  );
}

function PageRow({
  p,
  rank,
  label,
  width,
  open,
  onToggle,
}: {
  p: TopPage;
  rank: number;
  label: string;
  width: number;
  open: boolean;
  onToggle: () => void;
}) {
  const more = p.sites - p.sources.length;
  return (
    <li>
      <button
        onClick={onToggle}
        aria-expanded={open}
        title={p.page}
        className="grid w-full grid-cols-[2rem_1fr_auto] items-center gap-x-3 px-4 py-3 text-left transition hover:bg-zinc-900/50"
      >
        <span className="text-right text-xs tabular-nums text-zinc-600">{rank}</span>
        <span className="min-w-0">
          <span className="block truncate font-mono text-sm text-zinc-200">{label}</span>
          <span className="mt-1.5 block h-1.5 rounded-full bg-zinc-800">
            <span className="block h-full rounded-full bg-blue-500/70" style={{ width: `${Math.max(width, 2)}%` }} />
          </span>
        </span>
        <span className="flex items-center gap-2 text-sm tabular-nums text-zinc-300">
          {p.sites.toLocaleString()} {p.sites === 1 ? "site" : "sites"}
          <ChevronDown className={`size-4 text-zinc-500 transition ${open ? "rotate-180" : ""}`} aria-hidden="true" />
        </span>
      </button>

      {open && (
        <div className="border-t border-zinc-800/70 bg-zinc-900/30 px-4 py-4 sm:pl-[3.75rem]">
          <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-zinc-500">
            <span>
              {p.pages.toLocaleString()} linking {p.pages === 1 ? "page" : "pages"}
              {p.nofollowPages > 0 && <span className="text-orange-300"> · {p.nofollowPages.toLocaleString()} nofollow</span>}
            </span>
            <a
              href={`https://${p.page}`}
              target="_blank"
              rel="noopener noreferrer nofollow"
              className="flex items-center gap-1 text-zinc-400 transition hover:text-blue-400"
            >
              Open page <ExternalLink className="size-3.5" aria-hidden="true" />
            </a>
          </div>
          <ul className="mt-3 grid gap-2 sm:grid-cols-2">
            {p.sources.map((s) => (
              <li key={s.domain} className="flex items-center gap-3 text-sm">
                <SiteIcon domain={s.domain} />
                <span className="min-w-0 flex-1 truncate font-mono text-zinc-300">{s.domain}</span>
                <span className="text-xs tabular-nums text-zinc-500">
                  {s.pages.toLocaleString()} {s.pages === 1 ? "page" : "pages"}
                </span>
              </li>
            ))}
          </ul>
          {more > 0 && (
            <p className="mt-3 text-xs text-zinc-500">
              + {more.toLocaleString()} more {more === 1 ? "site" : "sites"}
            </p>
          )}
        </div>
      )}
    </li>
  );
}
