"use client";

import { useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { motion, AnimatePresence } from "motion/react";
import type { Lookup } from "@/lib/types";
import { SearchBar } from "./search-bar";
import { LinkGraph } from "./link-graph";
import { StatCards } from "./stat-cards";
import { BacklinksTable } from "./backlinks-table";
import { DownloadCsv } from "./download-csv";
import { NetworkBackground } from "./network-background";

type Outcome = { kind: "found"; data: Lookup } | { kind: "missing" } | { kind: "error" };
type State = { kind: "idle" } | { kind: "loading" } | Outcome;

const EXAMPLES = ["github.com", "wikipedia.org", "openstreetmap.org", "mozilla.org"];

export function Linkscope() {
  const router = useRouter();
  const query = useSearchParams().get("d") ?? "";
  const [outcome, setOutcome] = useState<{ query: string; outcome: Outcome } | null>(null);

  // The page state comes from the URL: no query is idle, a query without its answer yet is loading.
  const state: State = !query
    ? { kind: "idle" }
    : outcome?.query === query
      ? outcome.outcome
      : { kind: "loading" };

  useEffect(() => {
    if (!query) return;
    const ctrl = new AbortController();
    const done = (o: Outcome) => setOutcome({ query, outcome: o });
    fetch(`/api/lookup?domain=${encodeURIComponent(query)}`, { signal: ctrl.signal })
      .then(async (res) => {
        if (res.status === 404) return done({ kind: "missing" });
        if (!res.ok) return done({ kind: "error" });
        done({ kind: "found", data: await res.json() });
      })
      .catch((err) => {
        if (err.name !== "AbortError") done({ kind: "error" });
      });
    return () => ctrl.abort();
  }, [query]);

  const search = (q: string) => router.push(`/?d=${encodeURIComponent(q.trim())}`, { scroll: false });
  const hasResult = state.kind !== "idle";

  return (
    <div className="relative min-h-screen overflow-x-hidden">
      <AnimatePresence>
        {!hasResult && (
          <motion.div
            key="network"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1, transition: { duration: 1.2 } }}
            exit={{ opacity: 0, transition: { duration: 0.4 } }}
          >
            <NetworkBackground />
          </motion.div>
        )}
      </AnimatePresence>
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-0 top-0 h-[600px] bg-[radial-gradient(ellipse_at_top,rgba(59,130,246,0.12),transparent_60%)]"
      />

      <header className="relative z-10 mx-auto flex max-w-6xl items-center justify-between px-6 py-5">
        <button
          onClick={() => router.push("/")}
          className="flex items-center gap-2 text-sm font-semibold tracking-tight text-zinc-100"
        >
          <Logo />
          Linkscope
        </button>
        <a
          href="https://github.com/vasishtk1/Linkscope"
          className="text-sm text-zinc-400 transition hover:text-zinc-100"
        >
          GitHub
        </a>
      </header>

      <main className="relative z-10 mx-auto max-w-6xl px-6">
        <motion.section
          layout
          transition={{ type: "spring", stiffness: 120, damping: 20 }}
          className={`flex flex-col items-center text-center ${hasResult ? "pt-6 pb-8" : "min-h-[72vh] justify-center pb-24"}`}
        >
          <motion.h1
            layout="position"
            className={`font-semibold tracking-tight text-zinc-50 ${hasResult ? "text-3xl" : "text-5xl sm:text-6xl"}`}
          >
            Linkscope
          </motion.h1>
          <AnimatePresence>
            {!hasResult && (
              <motion.p
                initial={{ opacity: 0, y: 8 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, height: 0, marginTop: 0 }}
                className="mt-4 max-w-xl text-lg text-zinc-400"
              >
                See who links to any website. Paste a link and watch its network of backlinks build in
                front of you, mapped from Common Crawl&apos;s public archive of the web.
              </motion.p>
            )}
          </AnimatePresence>
          <motion.div layout="position" className="mt-8 w-full max-w-xl">
            <SearchBar key={query} initial={query} onSearch={search} busy={state.kind === "loading"} />
          </motion.div>
          {!hasResult && (
            <div className="mt-5 flex flex-wrap justify-center gap-2 text-sm">
              <span className="text-zinc-500">Try</span>
              {EXAMPLES.map((d) => (
                <button
                  key={d}
                  onClick={() => search(d)}
                  className="rounded-full border border-zinc-800 px-3 py-1 font-mono text-xs text-zinc-300 transition hover:border-zinc-600 hover:text-zinc-50"
                >
                  {d}
                </button>
              ))}
            </div>
          )}
        </motion.section>

        {state.kind === "missing" && (
          <Message>
            No backlinks found for <span className="font-mono text-zinc-200">{query}</span> in this
            sample. It covers one Common Crawl file (about 22,000 pages), so many sites aren&apos;t in it
            yet.
          </Message>
        )}
        {state.kind === "error" && <Message>Something went wrong looking that up. Try again.</Message>}

        {state.kind === "found" && <Results key={state.data.domain} data={state.data} />}
      </main>
    </div>
  );
}

function Results({ data }: { data: Lookup }) {
  return (
    <div className="pb-16">
      <LinkGraph domain={data.domain} backlinks={data.backlinks} total={data.linkingDomains} />

      <motion.section
        initial={{ opacity: 0, y: 24 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, margin: "-80px" }}
        transition={{ duration: 0.5 }}
        id="stats"
        className="mt-24 scroll-mt-8"
      >
        <h2 className="text-sm font-medium uppercase tracking-widest text-zinc-500">Stats</h2>
        <p className="mt-2 font-mono text-2xl text-zinc-100">{data.domain}</p>
        <StatCards data={data} />
        <BacklinksTable backlinks={data.backlinks} total={data.linkingDomains} />
      </motion.section>

      <section className="mt-24 flex flex-col items-center gap-3 border-t border-zinc-900 pt-12">
        <DownloadCsv domain={data.domain} count={data.linkingDomains} />
        <p className="text-xs text-zinc-600">{data.source}</p>
      </section>
    </div>
  );
}

function Message({ children }: { children: React.ReactNode }) {
  return (
    <motion.p
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      className="mx-auto max-w-xl rounded-xl border border-zinc-800 bg-zinc-900/50 px-5 py-4 text-center text-sm text-zinc-400"
    >
      {children}
    </motion.p>
  );
}

function Logo() {
  return (
    <svg width="22" height="22" viewBox="0 0 28 28" fill="none" stroke="#60a5fa" strokeWidth="2" aria-hidden="true">
      <circle cx="14" cy="14" r="4" />
      <circle cx="5" cy="6" r="2.5" />
      <circle cx="23" cy="6" r="2.5" />
      <circle cx="22" cy="23" r="2.5" />
      <path d="M7 7.5 11 11.5M21 7.5l-4 4M20.5 21l-3.5-4" />
    </svg>
  );
}
