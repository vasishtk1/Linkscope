"use client";

import { motion } from "motion/react";
import { Check, ExternalLink, GitBranch, GitPullRequest } from "lucide-react";
import { pathOf, type Problem, type Run } from "@/lib/agent";

export const CONNECT_STEP = 0.9;

// PullRequestPanel is the last stage: a button, then connecting GitHub, then the pull request the agent opened.
export function PullRequestPanel({ run, t, onOpen }: { run: Run; t: number; onOpen: () => void }) {
  const pr = run.pr;
  if (!pr) {
    return (
      <Panel className="mt-6">
        <p className="text-sm text-zinc-400">
          No pull request needed. None of the {run.problems.length} broken pages had a good replacement, so there&apos;s nothing to
          change. The agent leaves them alone rather than send visitors somewhere unrelated.
        </p>
      </Panel>
    );
  }
  const connected = Math.min(3, Math.floor(t / CONNECT_STEP));
  const opened = t >= 3 * CONNECT_STEP + (pr.steps[pr.steps.length - 1].at ?? 0) + 0.5;

  if (t < 0) {
    return (
      <Panel className="mt-6">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 className="text-base font-medium text-zinc-100">The redirect is ready</h2>
            <p className="mt-1 text-sm text-zinc-400">Open it as a pull request on the site&apos;s GitHub repo, for the owner to review and merge.</p>
          </div>
          <button
            onClick={onOpen}
            className="flex shrink-0 items-center justify-center gap-2 rounded-xl bg-zinc-100 px-5 py-3 text-sm font-medium text-zinc-900 transition hover:bg-white"
          >
            <GitHubMark className="size-4" />
            Open pull request
          </button>
        </div>
      </Panel>
    );
  }

  return (
    <div className="mt-6 grid gap-6 lg:grid-cols-[1fr_1.3fr]">
      <Panel>
        <h2 className="flex items-center gap-2 text-sm font-medium text-zinc-200">
          <GitHubMark className="size-4" />
          Connect GitHub
        </h2>
        <ol className="mt-4 space-y-3 text-sm">
          {[
            "Install the linkscope-agent GitHub App on the site's repo",
            `Choose the repo it can use: ${pr.repo}`,
            "Connected. It can open pull requests there, and it can't merge",
          ].map((label, i) => (
            <li key={label} className="flex items-start gap-3">
              <span
                className={`mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full border text-[11px] transition ${
                  i < connected ? "border-emerald-500/50 bg-emerald-500/10 text-emerald-300" : "border-zinc-700 text-zinc-500"
                }`}
              >
                {i < connected ? <Check className="size-3" aria-hidden="true" /> : i + 1}
              </span>
              <span className={i < connected ? "text-zinc-200" : "text-zinc-500"}>{label}</span>
            </li>
          ))}
        </ol>
        <p className="mt-4 text-xs leading-relaxed text-zinc-500">
          The owner picks which repos the app can touch on GitHub&apos;s own install page, and can uninstall it anytime.
        </p>
      </Panel>
      {opened ? <PrCard run={run} /> : <Opening connected={connected === 3} />}
    </div>
  );
}

function Opening({ connected }: { connected: boolean }) {
  return (
    <Panel>
      <div className="flex h-full min-h-40 items-center justify-center gap-2 text-sm text-zinc-500">
        <span className="size-1.5 animate-pulse rounded-full bg-blue-400" />
        {connected ? "opening the pull request" : "waiting for GitHub"}
      </div>
    </Panel>
  );
}

function PrCard({ run }: { run: Run }) {
  const pr = run.pr!;
  const fixed = run.problems.filter((p) => p.suggestion?.target);
  const left = run.problems.filter((p) => !p.suggestion?.target);
  return (
    <motion.section
      initial={{ opacity: 0, y: 6 }}
      animate={{ opacity: 1, y: 0 }}
      className="rounded-2xl border border-emerald-500/30 bg-zinc-900/40"
    >
      <header className="border-b border-zinc-800 px-4 py-3">
        <div className="flex flex-wrap items-center gap-2">
          <span className="flex items-center gap-1 rounded-full bg-emerald-500/15 px-2 py-0.5 text-xs font-medium text-emerald-300">
            <GitPullRequest className="size-3.5" aria-hidden="true" />
            Open
          </span>
          <h2 className="text-sm font-medium text-zinc-100">
            {pr.title} <span className="font-normal text-zinc-500">#{pr.number}</span>
          </h2>
        </div>
        <p className="mt-1.5 flex flex-wrap items-center gap-1 text-xs text-zinc-500">
          <span className="text-zinc-300">{pr.author}</span> wants to merge into <code className="text-zinc-400">main</code> from
          <GitBranch className="size-3" aria-hidden="true" />
          <code className="text-zinc-400">{pr.branch}</code>
        </p>
      </header>
      <div className="overflow-x-auto px-4 py-3">
        <table className="w-full text-left text-xs">
          <thead className="text-zinc-500">
            <tr>
              <th className="py-1.5 pr-3 font-normal">Broken page</th>
              <th className="py-1.5 pr-3 font-normal">Linking sites</th>
              <th className="py-1.5 pr-3 font-normal">Redirects to</th>
              <th className="py-1.5 font-normal">Why</th>
            </tr>
          </thead>
          <tbody className="text-zinc-300">
            {fixed.map((p) => (
              <tr key={p.page} className="border-t border-zinc-800/70">
                <td className="py-2 pr-3 font-mono">{pathOf(p.page, run.site)}</td>
                <td className="py-2 pr-3 tabular-nums">{p.sites}</td>
                <td className="py-2 pr-3 font-mono text-emerald-300/90">
                  {pathOf(p.suggestion!.target!.replace(/^https?:\/\/(www\.)?/, ""), run.site)}
                </td>
                <td className="py-2 text-zinc-400">{why(p)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {left.length > 0 && (
          <p className="mt-3 text-xs leading-relaxed text-zinc-500">
            Not redirected: {left.map((p) => pathOf(p.page, run.site)).join(", ")}, because no live page is a good replacement.
          </p>
        )}
      </div>
      <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-zinc-800 px-4 py-3">
        <span className="text-xs text-zinc-500">Opened on {pr.repo}, a demo site standing in for {run.site}. Never merged automatically.</span>
        <a
          href={pr.url}
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-2 rounded-lg bg-zinc-100 px-3.5 py-2 text-sm font-medium text-zinc-900 transition hover:bg-white"
        >
          <GitHubMark className="size-4" />
          View pull request on GitHub
          <ExternalLink className="size-3.5" aria-hidden="true" />
        </a>
      </footer>
    </motion.section>
  );
}

function why(p: Problem) {
  const s = p.suggestion;
  if (s?.by === "gemini") return `Gemini: ${s.ai?.reason ?? ""}`;
  if (s?.pick?.shared?.length) return `address match: shares "${s.pick.shared.join(", ")}" (score ${s.pick.score?.toFixed(2)})`;
  return "address match";
}

function Panel({ children, className = "" }: { children: React.ReactNode; className?: string }) {
  return <section className={`rounded-2xl border border-zinc-800 bg-zinc-900/30 p-5 ${className}`}>{children}</section>;
}

// GitHubMark is GitHub's logo (Octicons mark-github), used to label links to GitHub.
export function GitHubMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true" className={className}>
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
    </svg>
  );
}
