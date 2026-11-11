"use client";

import { useState } from "react";
import { ChevronDown } from "lucide-react";
import type { Backlink } from "@/lib/types";
import { badgeColor, faviconUrl } from "@/lib/badge";

const FIRST_ROWS = 15;

export function BacklinksTable({ backlinks, total }: { backlinks: Backlink[]; total: number }) {
  const [all, setAll] = useState(false);
  const rows = all ? backlinks : backlinks.slice(0, FIRST_ROWS);

  return (
    <div className="mt-10">
      <h3 className="text-lg font-medium text-zinc-100">Backlinks</h3>
      <div className="mt-4 overflow-x-auto rounded-2xl border border-zinc-800">
        <table className="w-full min-w-[640px] text-left text-sm">
          <thead className="bg-zinc-900/60 text-xs uppercase tracking-wider text-zinc-500">
            <tr>
              <th className="px-4 py-3 font-medium">Linking site</th>
              <th className="px-4 py-3 text-right font-medium">Pages</th>
              <th className="px-4 py-3 font-medium">Type</th>
              <th className="px-4 py-3 font-medium">Example page</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-800/70">
            {rows.map((b) => (
              <tr key={b.domain} className="transition hover:bg-zinc-900/50">
                <td className="px-4 py-3">
                  <div className="flex items-center gap-3">
                    <SiteIcon domain={b.domain} />
                    <span className="font-mono text-zinc-200">{b.domain}</span>
                  </div>
                </td>
                <td className="px-4 py-3 text-right tabular-nums text-zinc-300">{b.pages}</td>
                <td className="px-4 py-3">
                  <LinkType b={b} />
                </td>
                <td className="max-w-[320px] px-4 py-3">
                  <a
                    href={b.exampleSource}
                    target="_blank"
                    rel="noopener noreferrer nofollow"
                    className="block truncate text-zinc-500 transition hover:text-blue-400"
                  >
                    {b.exampleSource.replace(/^https?:\/\//, "")}
                  </a>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {backlinks.length > FIRST_ROWS && (
        <button
          onClick={() => setAll(!all)}
          className="mx-auto mt-4 flex items-center gap-1 text-sm text-zinc-400 transition hover:text-zinc-100"
        >
          {all
            ? "Show fewer"
            : backlinks.length === total
              ? `Show all ${total.toLocaleString()}`
              : `Show top ${backlinks.length.toLocaleString()}`}
          <ChevronDown className={`size-4 transition ${all ? "rotate-180" : ""}`} aria-hidden="true" />
        </button>
      )}
      {all && backlinks.length < total && (
        <p className="mt-2 text-center text-xs text-zinc-500">
          Showing the top {backlinks.length.toLocaleString()} of {total.toLocaleString()} linking sites. The CSV
          download has all of them.
        </p>
      )}
    </div>
  );
}

function LinkType({ b }: { b: Backlink }) {
  if (b.nofollowPages === 0) {
    return <span className="rounded-full bg-blue-500/10 px-2 py-0.5 text-xs text-blue-300">Followed</span>;
  }
  if (b.nofollowPages === b.pages) {
    return <span className="rounded-full bg-orange-500/10 px-2 py-0.5 text-xs text-orange-300">Nofollow</span>;
  }
  return <span className="rounded-full bg-zinc-700/40 px-2 py-0.5 text-xs text-zinc-300">Mixed</span>;
}

export function SiteIcon({ domain }: { domain: string }) {
  const [failed, setFailed] = useState(false);
  if (failed) {
    return (
      <span
        className="flex size-6 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold text-white"
        style={{ background: badgeColor(domain) }}
        aria-hidden="true"
      >
        {domain[0].toUpperCase()}
      </span>
    );
  }
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={faviconUrl(domain)}
      alt=""
      width={24}
      height={24}
      loading="lazy"
      onError={() => setFailed(true)}
      className="size-6 shrink-0 rounded-full bg-zinc-800 object-contain p-0.5"
    />
  );
}
